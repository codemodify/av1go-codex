package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func chooseCompoundFilterMV(mv0, mv1 MotionVector) MotionVector {
	if interBlockHasSubpelFilter(mv0) {
		return mv0
	}
	return mv1
}

func combinedCompoundCandidateWeights(cands0, cands1 []RefMVCandidate) []int {
	n := minInt(len(cands0), len(cands1))
	if n == 0 {
		return []int{0}
	}
	weights := make([]int, n)
	for i := 0; i < n; i++ {
		weights[i] = minInt(cands0[i].Weight, cands1[i].Weight)
	}
	return weights
}

func combinedCompoundCandidateWeightsInto(dst []int, cands0, cands1 []RefMVCandidate) []int {
	n := minInt(len(cands0), len(cands1))
	if n == 0 {
		dst = dst[:1]
		dst[0] = 0
		return dst
	}
	dst = dst[:n]
	for i := 0; i < n; i++ {
		dst[i] = minInt(cands0[i].Weight, cands1[i].Weight)
	}
	return dst
}

func PredictCompoundInterLumaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair) error {
	return PredictWeightedCompoundInterLumaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, 8)
}

func PredictWeightedCompoundInterLumaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, weight int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]int16, visibleWidth*visibleHeight)
	tmp1 := make([]int16, visibleWidth*visibleHeight)
	predictInterLumaPrep8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0)
	predictInterLumaPrep8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1)
	blendWeightedCompound8(dst, dstStride, x, y, visibleWidth, visibleHeight, tmp0, tmp1, visibleWidth, weight)
	return nil
}

func PredictCompoundInterChromaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair) error {
	return PredictWeightedCompoundInterChromaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, 8)
}

func PredictWeightedCompoundInterChromaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, weight int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]int16, visibleWidth*visibleHeight)
	tmp1 := make([]int16, visibleWidth*visibleHeight)
	predictInterChromaPrep8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0)
	predictInterChromaPrep8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1)
	blendWeightedCompound8(dst, dstStride, x, y, visibleWidth, visibleHeight, tmp0, tmp1, visibleWidth, weight)
	return nil
}

func PredictCompoundInterLumaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair,
	bitDepth uint8) error {
	return PredictWeightedCompoundInterLumaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, 8)
}

func PredictWeightedCompoundInterLumaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair,
	bitDepth uint8, weight int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictInterLumaBlock16WithFiltersAt(tmp0, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, bitDepth, filter0); err != nil {
		return err
	}
	if err := predictInterLumaBlock16WithFiltersAt(tmp1, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, bitDepth, filter1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*weight+int(tmp1[py*visibleWidth+px])*(16-weight)+8)>>4, bitDepth)
		}
	}
	return nil
}

func PredictCompoundInterChromaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair,
	bitDepth uint8) error {
	return PredictWeightedCompoundInterChromaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, 8)
}

func PredictWeightedCompoundInterChromaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair,
	bitDepth uint8, weight int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictInterChromaBlock16WithFiltersAt(tmp0, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, bitDepth, filter0); err != nil {
		return err
	}
	if err := predictInterChromaBlock16WithFiltersAt(tmp1, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, bitDepth, filter1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*weight+int(tmp1[py*visibleWidth+px])*(16-weight)+8)>>4, bitDepth)
		}
	}
	return nil
}

func (d *Decoder) selectSkipModeCompoundRefs(parsed *ParsedFrame, hdr *obu.FrameHeader, mvTile *SpatialMVTile, temporal *TemporalMVMap, g BlockGeometry, edge intraEdgeAvailability) (int8, MotionVector, int8, MotionVector, error) {
	if hdr == nil {
		return -1, MotionVector{}, -1, MotionVector{}, fmt.Errorf("decoder: frame header required")
	}
	ref0 := hdr.SkipModeRefs[0]
	ref1 := hdr.SkipModeRefs[1]
	if ref0 < 0 || ref1 < 0 {
		return -1, MotionVector{}, -1, MotionVector{}, fmt.Errorf("decoder: invalid skip mode refs (%d,%d)", ref0, ref1)
	}
	var temporal0 []RefMVCandidate
	if hdr.UseRefFrameMvs && temporal != nil {
		temporal0 = temporal.findCandidatesProjectedInto(nil, d.header, parsed, ref0, hdr, g)
	}
	cands0, _ := mvTile.findSingleRefCandidatesWithEdgeAndTemporalInto(nil, hdr, g, ref0, edge, temporal0)
	var temporal1 []RefMVCandidate
	if hdr.UseRefFrameMvs && temporal != nil {
		temporal1 = temporal.findCandidatesProjectedInto(nil, d.header, parsed, ref1, hdr, g)
	}
	cands1, _ := mvTile.findSingleRefCandidatesWithEdgeAndTemporalInto(nil, hdr, g, ref1, edge, temporal1)
	var temporalComp []CompoundRefMVCandidate
	if hdr.UseRefFrameMvs && temporal != nil {
		temporalComp = temporal.findCompoundCandidatesProjectedInto(nil, d.header, parsed, ref0, ref1, hdr, g)
	}
	compCands, _ := mvTile.findCompoundRefCandidatesBaseWithTemporalInto(nil, d.header, parsed, hdr, g, ref0, ref1, temporalComp)
	compCands = augmentCompoundCandidatesExtended(compCands, mvTile, d.header, parsed, hdr, g, ref0, ref1)
	if len(compCands) > 0 && compCands[0].Weight > 0 {
		mv0, mv1, err := SelectCompoundRefMVCandidate(compCands, drlNearest)
		if err == nil {
			return ref0, mv0, ref1, mv1, nil
		}
	}
	mv0, err := SelectRefMVCandidate(cands0, drlNearest)
	if err != nil {
		return -1, MotionVector{}, -1, MotionVector{}, err
	}
	mv1, err := SelectRefMVCandidate(cands1, drlNearest)
	if err != nil {
		return -1, MotionVector{}, -1, MotionVector{}, err
	}
	return ref0, mv0, ref1, mv1, nil
}

func jointCompoundWeight(seq av1.SequenceHeader, parsed *ParsedFrame, ref0, ref1 int8) (int, bool) {
	if parsed == nil || ref0 < 0 || ref1 < 0 || !seq.EnableOrderHint || !seq.EnableJntComp {
		return 8, false
	}
	if int(ref0) >= len(parsed.RefFramePOC) || int(ref1) >= len(parsed.RefFramePOC) {
		return 8, false
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	if orderHintBits <= 0 {
		return 8, false
	}
	d0 := minInt(absInt(compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[ref1]), int(parsed.Header.FrameOffset))), 31)
	d1 := minInt(absInt(compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[ref0]), int(parsed.Header.FrameOffset))), 31)
	order := boolToInt(d0 <= d1)
	quantDistWeight := [3][2]int{{2, 3}, {2, 5}, {2, 7}}
	quantDistLookup := [4][2]int{{9, 7}, {11, 5}, {12, 4}, {13, 3}}
	k := 0
	for ; k < 3; k++ {
		c0 := quantDistWeight[k][order]
		c1 := quantDistWeight[k][1-order]
		d0c0 := d0 * c0
		d1c1 := d1 * c1
		if (d0 > d1 && d0c0 < d1c1) || (d0 <= d1 && d0c0 > d1c1) {
			break
		}
	}
	return quantDistLookup[k][order], true
}

func compoundPOCDiff(orderHintBits, poc0, poc1 int) int {
	if orderHintBits == 0 {
		return 0
	}
	mask := 1 << (orderHintBits - 1)
	diff := poc0 - poc1
	return (diff & (mask - 1)) - (diff & mask)
}
