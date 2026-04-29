package decoder

import (
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type loopFilterLevelMap struct {
	Width4  int
	Height4 int
	Stride  int
	Level   [4][]uint8
	scratch [4]*scratchBuffer[uint8]
}

var loopFilterLevelScratchPool sync.Pool

func newLoopFilterLevelMap(width, height int) *loopFilterLevelMap {
	width4 := (width + 3) >> 2
	height4 := (height + 3) >> 2
	size := width4 * height4
	m := &loopFilterLevelMap{
		Width4:  width4,
		Height4: height4,
		Stride:  width4,
	}
	for i := range m.Level {
		scratch := takeScratch[uint8](&loopFilterLevelScratchPool, size)
		m.Level[i] = scratch.buf[:size]
		m.scratch[i] = scratch
	}
	return m
}

func buildLoopFilterLevelMap(hdr *obu.FrameHeader, field *SpatialMVTile, seg *SegmentationMap) *loopFilterLevelMap {
	if hdr == nil || hdr.Width <= 0 || hdr.Height <= 0 {
		return nil
	}
	m := newLoopFilterLevelMap(hdr.Width, hdr.Height)
	for y4 := 0; y4 < m.Height4; y4++ {
		for x4 := 0; x4 < m.Width4; x4++ {
			idx := y4*m.Stride + x4
			segID := uint8(0)
			if seg != nil {
				segID = seg.At(x4, y4)
			}
			refIdx, isGlobal := blockLoopFilterRefInfo(field.block(x4, y4))
			segData := segmentDataForID(hdr, segID)
			m.Level[0][idx] = calcBlockLoopFilterLevel(int(hdr.LoopFilter.LevelY[0]), segData.DeltaLFYV, refIdx, isGlobal, hdr.LoopFilter, false)
			m.Level[1][idx] = calcBlockLoopFilterLevel(int(hdr.LoopFilter.LevelY[1]), segData.DeltaLFYH, refIdx, isGlobal, hdr.LoopFilter, false)
			m.Level[2][idx] = calcBlockLoopFilterLevel(int(hdr.LoopFilter.LevelU), segData.DeltaLFU, refIdx, isGlobal, hdr.LoopFilter, true)
			m.Level[3][idx] = calcBlockLoopFilterLevel(int(hdr.LoopFilter.LevelV), segData.DeltaLFV, refIdx, isGlobal, hdr.LoopFilter, true)
		}
	}
	return m
}

func putLoopFilterLevelMap(m *loopFilterLevelMap) {
	if m == nil {
		return
	}
	for i := range m.Level {
		putZeroScratch(&loopFilterLevelScratchPool, m.scratch[i])
		m.Level[i] = nil
		m.scratch[i] = nil
	}
}

func blockLoopFilterRefInfo(b *SpatialMVBlock) (int, bool) {
	if b == nil || !b.Valid || b.Intra || b.Ref[0] < 0 {
		return 0, false
	}
	refIdx := minInt(7, maxIntLocal(0, int(b.Ref[0])+1))
	isGlobal := b.RefFlags[0]&refMVFlagGlobal != 0
	if b.Ref[1] >= 0 {
		isGlobal = isGlobal && b.RefFlags[1]&refMVFlagGlobal != 0
	}
	return refIdx, isGlobal
}

func calcBlockLoopFilterLevel(base int, segDelta int8, refIdx int, isGlobal bool, lf obu.LoopFilter, chroma bool) uint8 {
	if base <= 0 {
		return 0
	}
	if chroma && base == 0 {
		return 0
	}
	base = clipLoopFilterLevel(base + int(segDelta))
	if !lf.ModeRefDeltaEnable {
		return uint8(base)
	}
	shift := ternaryInt(base >= 32, 1, 0)
	if refIdx <= 0 {
		base = clipLoopFilterLevel(base + (int(lf.ModeRefDeltas.Ref[0]) << shift))
		return uint8(base)
	}
	refIdx = minInt(refIdx, len(lf.ModeRefDeltas.Ref)-1)
	modeIdx := ternaryInt(isGlobal, 0, 1)
	delta := int(lf.ModeRefDeltas.Ref[refIdx]) + int(lf.ModeRefDeltas.Mode[modeIdx])
	base = clipLoopFilterLevel(base + (delta << shift))
	return uint8(base)
}

func clipLoopFilterLevel(v int) int {
	if v < 0 {
		return 0
	}
	if v > 63 {
		return 63
	}
	return v
}

func (m *loopFilterLevelMap) levelAt(plane, x4, y4 int) int {
	if m == nil || plane < 0 || plane >= len(m.Level) || m.Stride <= 0 {
		return 0
	}
	x4 = minInt(m.Width4-1, maxIntLocal(0, x4))
	y4 = minInt(m.Height4-1, maxIntLocal(0, y4))
	return int(m.Level[plane][y4*m.Stride+x4])
}

func (m *loopFilterLevelMap) verticalLevel(plane, x, y, ssX, ssY int) int {
	if m == nil {
		return 0
	}
	lumaX := x << ssX
	lumaY := y << ssY
	left4 := (lumaX - 1) >> 2
	right4 := lumaX >> 2
	y4 := lumaY >> 2
	return maxIntLocal(m.levelAt(plane, left4, y4), m.levelAt(plane, right4, y4))
}

func (m *loopFilterLevelMap) horizontalLevel(plane, x, y, ssX, ssY int) int {
	if m == nil {
		return 0
	}
	lumaX := x << ssX
	lumaY := y << ssY
	x4 := lumaX >> 2
	top4 := (lumaY - 1) >> 2
	bottom4 := lumaY >> 2
	return maxIntLocal(m.levelAt(plane, x4, top4), m.levelAt(plane, x4, bottom4))
}
