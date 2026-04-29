package decoder

import (
	"fmt"
	"os"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

const (
	interFilterRegular uint8 = iota
	interFilterSmooth
	interFilterSharp
	interFilterBilinear
	interFilterSwitchable
)

// InterFilterPair stores AV1 switchable filter syntax order:
// index 0 is vertical, index 1 is horizontal.
type InterFilterPair [2]uint8

var regular8TapSubpel = [15][8]int{
	{0, 1, -3, 63, 4, -1, 0, 0},
	{0, 1, -5, 61, 9, -2, 0, 0},
	{0, 1, -6, 58, 14, -4, 1, 0},
	{0, 1, -7, 55, 19, -5, 1, 0},
	{0, 1, -7, 51, 24, -6, 1, 0},
	{0, 1, -8, 47, 29, -6, 1, 0},
	{0, 1, -7, 42, 33, -6, 1, 0},
	{0, 1, -7, 38, 38, -7, 1, 0},
	{0, 1, -6, 33, 42, -7, 1, 0},
	{0, 1, -6, 29, 47, -8, 1, 0},
	{0, 1, -6, 24, 51, -7, 1, 0},
	{0, 1, -5, 19, 55, -7, 1, 0},
	{0, 1, -4, 14, 58, -6, 1, 0},
	{0, 0, -2, 9, 61, -5, 1, 0},
	{0, 0, -1, 4, 63, -3, 1, 0},
}

var regular8TapSubpelSmall = [15][8]int{
	{0, 0, -2, 63, 4, -1, 0, 0},
	{0, 0, -4, 61, 9, -2, 0, 0},
	{0, 0, -5, 58, 14, -3, 0, 0},
	{0, 0, -6, 55, 19, -4, 0, 0},
	{0, 0, -6, 51, 24, -5, 0, 0},
	{0, 0, -7, 47, 29, -5, 0, 0},
	{0, 0, -6, 42, 33, -5, 0, 0},
	{0, 0, -6, 38, 38, -6, 0, 0},
	{0, 0, -5, 33, 42, -6, 0, 0},
	{0, 0, -5, 29, 47, -7, 0, 0},
	{0, 0, -5, 24, 51, -6, 0, 0},
	{0, 0, -4, 19, 55, -6, 0, 0},
	{0, 0, -3, 14, 58, -5, 0, 0},
	{0, 0, -2, 9, 61, -4, 0, 0},
	{0, 0, -1, 4, 63, -2, 0, 0},
}

func interFilterPairForHeader(hdr *obu.FrameHeader) InterFilterPair {
	if hdr == nil || hdr.SubpelFilterMode == interFilterSwitchable {
		return InterFilterPair{interFilterRegular, interFilterRegular}
	}
	return InterFilterPair{hdr.SubpelFilterMode, hdr.SubpelFilterMode}
}

func inter8TapCoeffs(filterMode uint8, frac16, span int) *[8]int {
	if frac16 <= 0 || frac16 >= 16 {
		return nil
	}
	switch filterMode {
	case interFilterRegular:
		if span <= 4 {
			return &regular8TapSubpelSmall[frac16-1]
		}
		return &regular8TapSubpel[frac16-1]
	case interFilterSmooth:
		if span <= 4 {
			return &smooth8TapSubpelSmall[frac16-1]
		}
		return &smooth8TapSubpel[frac16-1]
	case interFilterSharp:
		if span <= 4 {
			return &regular8TapSubpelSmall[frac16-1]
		}
		return &sharp8TapSubpel[frac16-1]
	default:
		return nil
	}
}

func interBlockHasSubpelFilter(mv MotionVector) bool {
	return (mv.X&7) != 0 || (mv.Y&7) != 0
}

func ReadInterFilterPair(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, ref0 int8, isComp bool, mv MotionVector, dec intraEntropyDecoder) (InterFilterPair, error) {
	return readInterFilterPairWithEligibility(seq, hdr, cdf, above, left, g, ref0, isComp, interBlockHasSubpelFilter(mv), dec)
}

func ReadInterFilterPairForSingle(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, ref0 int8, mode InterPredMode, dec intraEntropyDecoder) (InterFilterPair, error) {
	return readInterFilterPairWithEligibility(seq, hdr, cdf, above, left, g, ref0, false, singleRefNeedsSubpelFilter(hdr, g, ref0, mode), dec)
}

func ReadInterFilterPairForCompound(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, ref0, ref1 int8, mode0, mode1 InterPredMode, dec intraEntropyDecoder) (InterFilterPair, error) {
	return readInterFilterPairWithEligibility(seq, hdr, cdf, above, left, g, ref0, true, compoundRefNeedsSubpelFilter(hdr, g, ref0, ref1, mode0, mode1), dec)
}

func singleRefNeedsSubpelFilter(hdr *obu.FrameHeader, g BlockGeometry, ref0 int8, mode InterPredMode) bool {
	if mode != InterPredGlobal {
		return true
	}
	if minInt(g.Width4(), g.Height4()) == 1 {
		return true
	}
	return globalMotionType(hdr, ref0) == 1
}

func compoundRefNeedsSubpelFilter(hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8, mode0, mode1 InterPredMode) bool {
	if minInt(g.Width4(), g.Height4()) == 1 {
		return true
	}
	if mode0 != InterPredGlobal || mode1 != InterPredGlobal {
		return true
	}
	return globalMotionType(hdr, ref0) == 1 || globalMotionType(hdr, ref1) == 1
}

func globalMotionType(hdr *obu.FrameHeader, ref int8) uint8 {
	if hdr == nil || ref < 0 || int(ref) >= len(hdr.GMV) {
		return 0
	}
	return hdr.GMV[ref].Type
}

func readInterFilterPairWithEligibility(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, ref0 int8, isComp bool, needsSubpelFilter bool, dec intraEntropyDecoder) (InterFilterPair, error) {
	pair := interFilterPairForHeader(hdr)
	if hdr == nil || hdr.SubpelFilterMode != interFilterSwitchable {
		return pair, nil
	}
	if cdf == nil || above == nil || left == nil || dec == nil {
		return InterFilterPair{}, fmt.Errorf("decoder: filter decode requires cdf, contexts, and entropy decoder")
	}
	if !needsSubpelFilter {
		return pair, nil
	}
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	comp := boolToInt(isComp)
	ctx0 := GetFilterCtx(above, left, comp, 0, ref0, by4, bx4)
	pre0 := append([]uint16(nil), cdf.Filter[0][ctx0][:]...)
	preCur, preRng, preCnt, prePos := debugEntropyState(dec)
	pair[0] = uint8(dec.DecodeSymbolAdapt(cdf.Filter[0][ctx0][:], 2))
	postCur, postRng, postCnt, postPos := debugEntropyState(dec)
	if os.Getenv("DEBUG_INTER_FILTER_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "DEBUG_INTER_FILTER plane=0 geom=(%d,%d)-(%d,%d) ref=%d comp=%v ctx=%d tok=%d a_ref=(%d,%d) l_ref=(%d,%d) a_f=(%d,%d) l_f=(%d,%d) pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
			g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, ref0, isComp, ctx0, pair[0],
			above.Ref[0][bx4], above.Ref[1][bx4], left.Ref[0][by4], left.Ref[1][by4],
			above.Filter[0][bx4], above.Filter[1][bx4], left.Filter[0][by4], left.Filter[1][by4],
			preCur, preRng, preCnt, prePos, postCur, postRng, postCnt, postPos, pre0, cdf.Filter[0][ctx0][:])
	}
	if seq.EnableDualFilter {
		ctx1 := GetFilterCtx(above, left, comp, 1, ref0, by4, bx4)
		pre1 := append([]uint16(nil), cdf.Filter[1][ctx1][:]...)
		preCur, preRng, preCnt, prePos = debugEntropyState(dec)
		pair[1] = uint8(dec.DecodeSymbolAdapt(cdf.Filter[1][ctx1][:], 2))
		postCur, postRng, postCnt, postPos = debugEntropyState(dec)
		if os.Getenv("DEBUG_INTER_FILTER_TRACE") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG_INTER_FILTER plane=1 geom=(%d,%d)-(%d,%d) ref=%d comp=%v ctx=%d tok=%d pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
				g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, ref0, isComp, ctx1, pair[1],
				preCur, preRng, preCnt, prePos, postCur, postRng, postCnt, postPos, pre1, cdf.Filter[1][ctx1][:])
		}
	} else {
		pair[1] = pair[0]
	}
	return pair, nil
}

func debugEntropyState(dec intraEntropyDecoder) (uint32, uint32, int, int) {
	state, ok := dec.(interface {
		DebugState() (uint32, uint32, int, int)
	})
	if !ok {
		return 0, 0, 0, 0
	}
	return state.DebugState()
}
