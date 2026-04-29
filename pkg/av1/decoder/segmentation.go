package decoder

import (
	"fmt"
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

var generatedSegPredCDF = [3][2]uint16{
	{16384, 0},
	{16384, 0},
	{16384, 0},
}

var generatedSegIDCDF = [3][8]uint16{
	{5622, 7893, 16093, 18233, 27809, 28373, 32533, 0},
	{14274, 18230, 22557, 24935, 29980, 30851, 32344, 0},
	{27527, 28487, 28723, 28890, 32397, 32647, 32679, 0},
}

type SegmentationMap struct {
	Width4  int
	Height4 int
	Stride  int
	Data    []uint8
	scratch *scratchBuffer[uint8]
}

type SegmentationState struct {
	Current *SegmentationMap
	Prev    *SegmentationMap
}

var segmentationDataScratchPool sync.Pool

func NewSegmentationMap(width, height int) *SegmentationMap {
	width4 := miGridSize4(width)
	height4 := miGridSize4(height)
	return &SegmentationMap{
		Width4:  width4,
		Height4: height4,
		Stride:  width4,
		Data:    make([]uint8, width4*height4),
	}
}

func takeSegmentationMap(width, height int) *SegmentationMap {
	width4 := miGridSize4(width)
	height4 := miGridSize4(height)
	data := takeScratch[uint8](&segmentationDataScratchPool, width4*height4)
	return &SegmentationMap{
		Width4:  width4,
		Height4: height4,
		Stride:  width4,
		Data:    data.buf,
		scratch: data,
	}
}

func putSegmentationMap(m *SegmentationMap) {
	if m == nil || m.scratch == nil {
		return
	}
	putZeroScratch(&segmentationDataScratchPool, m.scratch)
}

func CloneSegmentationMap(src *SegmentationMap) *SegmentationMap {
	if src == nil {
		return nil
	}
	clone := &SegmentationMap{
		Width4:  src.Width4,
		Height4: src.Height4,
		Stride:  src.Stride,
		Data:    append([]uint8(nil), src.Data...),
		scratch: nil,
	}
	return clone
}

func NewSegmentationState(width, height int, prev *SegmentationMap, copyPrev bool) *SegmentationState {
	cur := takeSegmentationMap(width, height)
	if copyPrev && prev != nil {
		copy(cur.Data, prev.Data)
	}
	return &SegmentationState{
		Current: cur,
		Prev:    prev,
	}
}

func (m *SegmentationMap) At(x4, y4 int) uint8 {
	if m == nil || x4 < 0 || y4 < 0 || x4 >= m.Width4 || y4 >= m.Height4 {
		return 0
	}
	return m.Data[y4*m.Stride+x4]
}

func (m *SegmentationMap) FillBlock(g BlockGeometry, segID uint8) {
	if m == nil {
		return
	}
	startX := maxIntLocal(0, g.Start4X)
	startY := maxIntLocal(0, g.Start4Y)
	endX := min(m.Width4, g.End4X)
	endY := min(m.Height4, g.End4Y)
	for y4 := startY; y4 < endY; y4++ {
		row := y4 * m.Stride
		for x4 := startX; x4 < endX; x4++ {
			m.Data[row+x4] = segID
		}
	}
}

func (m *SegmentationMap) MinBlock(g BlockGeometry) uint8 {
	if m == nil {
		return 0
	}
	startX := maxIntLocal(0, g.Start4X)
	startY := maxIntLocal(0, g.Start4Y)
	endX := min(m.Width4, g.End4X)
	endY := min(m.Height4, g.End4Y)
	if startX >= endX || startY >= endY {
		return 0
	}
	segID := uint8(8)
	for y4 := startY; y4 < endY && segID != 0; y4++ {
		row := y4 * m.Stride
		for x4 := startX; x4 < endX; x4++ {
			if v := m.Data[row+x4]; v < segID {
				segID = v
			}
		}
	}
	if segID > 7 {
		return 0
	}
	return segID
}

func segmentDataForID(hdr *obu.FrameHeader, segID uint8) obu.SegmentData {
	seg := obu.SegmentData{Ref: -1}
	if hdr == nil || !hdr.Segmentation.Enabled || int(segID) >= len(hdr.Segmentation.Data) {
		return seg
	}
	return hdr.Segmentation.Data[segID]
}

func segmentationRefForBlock(seg obu.SegmentData) int8 {
	if seg.Ref > 0 {
		return seg.Ref - 1
	}
	if seg.GlobalMV || seg.Skip {
		return 0
	}
	return -1
}

func getCurrentFrameSegID(g BlockGeometry, haveTop, haveLeft bool, cur *SegmentationMap) (uint8, int) {
	if cur == nil {
		return 0, 0
	}
	if haveLeft && haveTop {
		l := cur.At(g.Start4X-1, g.Start4Y)
		a := cur.At(g.Start4X, g.Start4Y-1)
		al := cur.At(g.Start4X-1, g.Start4Y-1)
		switch {
		case l == a && al == l:
			return ifEqualUint8(a, al, a, l), 2
		case l == a || al == l || a == al:
			return ifEqualUint8(a, al, a, l), 1
		default:
			return l, 0
		}
	}
	if haveLeft {
		return cur.At(g.Start4X-1, g.Start4Y), 0
	}
	if haveTop {
		return cur.At(g.Start4X, g.Start4Y-1), 0
	}
	return 0, 0
}

func ifEqualUint8(a, b, yes, no uint8) uint8 {
	if a == b {
		return yes
	}
	return no
}

func negDeinterleave(diff, ref, max int) int {
	if ref == 0 {
		return diff
	}
	if ref >= max-1 {
		return max - diff - 1
	}
	if 2*ref < max {
		if diff <= 2*ref {
			if diff&1 != 0 {
				return ref + ((diff + 1) >> 1)
			}
			return ref - (diff >> 1)
		}
		return diff
	}
	if diff <= 2*(max-ref-1) {
		if diff&1 != 0 {
			return ref + ((diff + 1) >> 1)
		}
		return ref - (diff >> 1)
	}
	return max - (diff + 1)
}

func readBlockSegmentID(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, state *SegmentationState, g BlockGeometry, haveTop, haveLeft, skipKnown, skip bool, dec intraEntropyDecoder) (uint8, bool, error) {
	if hdr == nil {
		return 0, false, fmt.Errorf("decoder: frame header required")
	}
	if !hdr.Segmentation.Enabled {
		return 0, false, nil
	}
	if cdf == nil || above == nil || left == nil || dec == nil {
		return 0, false, fmt.Errorf("decoder: segmentation decode requires cdf, contexts, and entropy decoder")
	}
	if state == nil || state.Current == nil {
		return 0, false, fmt.Errorf("decoder: segmentation state required")
	}

	if !hdr.Segmentation.UpdateMap {
		segID := uint8(0)
		if state.Prev != nil {
			segID = state.Prev.MinBlock(g)
		}
		state.Current.FillBlock(g, segID)
		return segID, false, nil
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	predSegID, segCtx := getCurrentFrameSegID(g, haveTop, haveLeft, state.Current)
	if hdr.Segmentation.Preskip {
		segPred := false
		if hdr.Segmentation.Temporal {
			ctx := int(above.SegPred[bx4]) + int(left.SegPred[by4])
			segPred = dec.DecodeBoolAdapt(cdf.SegPred[ctx][:]) != 0
		}
		segID := predSegID
		if segPred {
			if state.Prev != nil {
				segID = state.Prev.MinBlock(g)
			} else {
				segID = 0
			}
		} else if hdr.Segmentation.LastActiveSegID >= 0 {
			diff := int(dec.DecodeSymbolAdapt(cdf.SegID[segCtx][:], 7))
			maxSeg := int(hdr.Segmentation.LastActiveSegID) + 1
			segID = uint8(negDeinterleave(diff, int(predSegID), maxSeg))
			if segID > uint8(hdr.Segmentation.LastActiveSegID) {
				segID = 0
			}
		}
		state.Current.FillBlock(g, segID)
		return segID, segPred, nil
	}

	if !skipKnown {
		return 0, false, fmt.Errorf("decoder: post-skip segmentation requires skip state")
	}
	segPred := false
	if !skip && hdr.Segmentation.Temporal {
		ctx := int(above.SegPred[bx4]) + int(left.SegPred[by4])
		segPred = dec.DecodeBoolAdapt(cdf.SegPred[ctx][:]) != 0
	}
	segID := predSegID
	switch {
	case segPred:
		if state.Prev != nil {
			segID = state.Prev.MinBlock(g)
		} else {
			segID = 0
		}
	case skip:
		segID = predSegID
	case hdr.Segmentation.LastActiveSegID >= 0:
		diff := int(dec.DecodeSymbolAdapt(cdf.SegID[segCtx][:], 7))
		maxSeg := int(hdr.Segmentation.LastActiveSegID) + 1
		segID = uint8(negDeinterleave(diff, int(predSegID), maxSeg))
		if segID > uint8(hdr.Segmentation.LastActiveSegID) {
			segID = 0
		}
	}
	state.Current.FillBlock(g, segID)
	return segID, segPred, nil
}
