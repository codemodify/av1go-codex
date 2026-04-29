package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func GetFilterCtx(above, left *BlockContext, comp, dir int, ref int8, yb4, xb4 int) int {
	aFilter := int(switchableFilterUnset)
	if above != nil && (above.Ref[0][xb4] == ref || above.Ref[1][xb4] == ref) {
		aFilter = int(above.Filter[dir][xb4])
	}
	lFilter := int(switchableFilterUnset)
	if left != nil && (left.Ref[0][yb4] == ref || left.Ref[1][yb4] == ref) {
		lFilter = int(left.Filter[dir][yb4])
	}
	switch {
	case aFilter == lFilter:
		return comp*4 + aFilter
	case aFilter == int(switchableFilterUnset):
		return comp*4 + lFilter
	case lFilter == int(switchableFilterUnset):
		return comp*4 + aFilter
	default:
		return comp*4 + int(switchableFilterUnset)
	}
}

func GetRefCtx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [2]int
	if haveTop && above.Intra[xb4] == 0 {
		cnt[boolToInt(above.Ref[0][xb4] >= 4)]++
		if above.CompType[xb4] != 0 {
			cnt[boolToInt(above.Ref[1][xb4] >= 4)]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		cnt[boolToInt(left.Ref[0][yb4] >= 4)]++
		if left.CompType[yb4] != 0 {
			cnt[boolToInt(left.Ref[1][yb4] >= 4)]++
		}
	}
	switch {
	case cnt[0] == cnt[1]:
		return 1
	case cnt[0] < cnt[1]:
		return 0
	default:
		return 2
	}
}

func GetCompCtx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	if haveTop {
		if haveLeft {
			if above.CompType[xb4] != 0 {
				if left.CompType[yb4] != 0 {
					return 4
				}
				return 2 + boolToInt(left.Ref[0][yb4] >= 4 || left.Ref[0][yb4] < 0)
			}
			if left.CompType[yb4] != 0 {
				return 2 + boolToInt(above.Ref[0][xb4] >= 4 || above.Ref[0][xb4] < 0)
			}
			return boolToInt((left.Ref[0][yb4] >= 4) != (above.Ref[0][xb4] >= 4))
		}
		if above.CompType[xb4] != 0 {
			return 3
		}
		return boolToInt(above.Ref[0][xb4] >= 4)
	}
	if haveLeft {
		if left.CompType[yb4] != 0 {
			return 3
		}
		return boolToInt(left.Ref[0][yb4] >= 4)
	}
	return 1
}

func hasUniComp(ctx *BlockContext, idx int) bool {
	if ctx == nil || idx < 0 || idx >= len(ctx.Ref[0]) {
		return false
	}
	return (ctx.Ref[0][idx] < 4) == (ctx.Ref[1][idx] < 4)
}

func GetCompDirCtx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	if haveTop && haveLeft {
		aIntra := above.Intra[xb4] != 0
		lIntra := left.Intra[yb4] != 0
		if aIntra && lIntra {
			return 2
		}
		if aIntra || lIntra {
			edge := above
			off := xb4
			if aIntra {
				edge = left
				off = yb4
			}
			if edge.CompType[off] == uint8(compoundTypeNone) {
				return 2
			}
			return 1 + 2*boolToInt(hasUniComp(edge, off))
		}

		aComp := above.CompType[xb4] != uint8(compoundTypeNone)
		lComp := left.CompType[yb4] != uint8(compoundTypeNone)
		aRef0 := above.Ref[0][xb4]
		lRef0 := left.Ref[0][yb4]
		switch {
		case !aComp && !lComp:
			return 1 + 2*boolToInt((aRef0 >= 4) == (lRef0 >= 4))
		case !aComp || !lComp:
			edge := above
			off := xb4
			if !aComp {
				edge = left
				off = yb4
			}
			if !hasUniComp(edge, off) {
				return 1
			}
			return 3 + boolToInt((aRef0 >= 4) == (lRef0 >= 4))
		default:
			aUni := hasUniComp(above, xb4)
			lUni := hasUniComp(left, yb4)
			if !aUni && !lUni {
				return 0
			}
			if !aUni || !lUni {
				return 2
			}
			return 3 + boolToInt((aRef0 == 4) == (lRef0 == 4))
		}
	}
	if haveTop || haveLeft {
		edge := above
		off := xb4
		if haveLeft {
			edge = left
			off = yb4
		}
		if edge.Intra[off] != 0 {
			return 2
		}
		if edge.CompType[off] == uint8(compoundTypeNone) {
			return 2
		}
		return 4 * boolToInt(hasUniComp(edge, off))
	}
	return 2
}

func GetFwdRefCtx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [4]int
	if haveTop && above.Intra[xb4] == 0 {
		if above.Ref[0][xb4] >= 0 && above.Ref[0][xb4] < 4 {
			cnt[above.Ref[0][xb4]]++
		}
		if above.CompType[xb4] != 0 && above.Ref[1][xb4] >= 0 && above.Ref[1][xb4] < 4 {
			cnt[above.Ref[1][xb4]]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		if left.Ref[0][yb4] >= 0 && left.Ref[0][yb4] < 4 {
			cnt[left.Ref[0][yb4]]++
		}
		if left.CompType[yb4] != 0 && left.Ref[1][yb4] >= 0 && left.Ref[1][yb4] < 4 {
			cnt[left.Ref[1][yb4]]++
		}
	}
	cnt[0] += cnt[1]
	cnt[2] += cnt[3]
	switch {
	case cnt[0] == cnt[2]:
		return 1
	case cnt[0] < cnt[2]:
		return 0
	default:
		return 2
	}
}

func GetFwdRef1Ctx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [2]int
	if haveTop && above.Intra[xb4] == 0 {
		if above.Ref[0][xb4] >= 0 && above.Ref[0][xb4] < 2 {
			cnt[above.Ref[0][xb4]]++
		}
		if above.CompType[xb4] != 0 && above.Ref[1][xb4] >= 0 && above.Ref[1][xb4] < 2 {
			cnt[above.Ref[1][xb4]]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		if left.Ref[0][yb4] >= 0 && left.Ref[0][yb4] < 2 {
			cnt[left.Ref[0][yb4]]++
		}
		if left.CompType[yb4] != 0 && left.Ref[1][yb4] >= 0 && left.Ref[1][yb4] < 2 {
			cnt[left.Ref[1][yb4]]++
		}
	}
	switch {
	case cnt[0] == cnt[1]:
		return 1
	case cnt[0] < cnt[1]:
		return 0
	default:
		return 2
	}
}

func GetFwdRef2Ctx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [2]int
	if haveTop && above.Intra[xb4] == 0 {
		if above.Ref[0][xb4] >= 2 && above.Ref[0][xb4] < 4 {
			cnt[above.Ref[0][xb4]-2]++
		}
		if above.CompType[xb4] != 0 && above.Ref[1][xb4] >= 2 && above.Ref[1][xb4] < 4 {
			cnt[above.Ref[1][xb4]-2]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		if left.Ref[0][yb4] >= 2 && left.Ref[0][yb4] < 4 {
			cnt[left.Ref[0][yb4]-2]++
		}
		if left.CompType[yb4] != 0 && left.Ref[1][yb4] >= 2 && left.Ref[1][yb4] < 4 {
			cnt[left.Ref[1][yb4]-2]++
		}
	}
	switch {
	case cnt[0] == cnt[1]:
		return 1
	case cnt[0] < cnt[1]:
		return 0
	default:
		return 2
	}
}

func GetBwdRefCtx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [3]int
	if haveTop && above.Intra[xb4] == 0 {
		if above.Ref[0][xb4] >= 4 && above.Ref[0][xb4] < 7 {
			cnt[above.Ref[0][xb4]-4]++
		}
		if above.CompType[xb4] != 0 && above.Ref[1][xb4] >= 4 && above.Ref[1][xb4] < 7 {
			cnt[above.Ref[1][xb4]-4]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		if left.Ref[0][yb4] >= 4 && left.Ref[0][yb4] < 7 {
			cnt[left.Ref[0][yb4]-4]++
		}
		if left.CompType[yb4] != 0 && left.Ref[1][yb4] >= 4 && left.Ref[1][yb4] < 7 {
			cnt[left.Ref[1][yb4]-4]++
		}
	}
	cnt[1] += cnt[0]
	switch {
	case cnt[2] == cnt[1]:
		return 1
	case cnt[1] < cnt[2]:
		return 0
	default:
		return 2
	}
}

func GetBwdRef1Ctx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [3]int
	if haveTop && above.Intra[xb4] == 0 {
		if above.Ref[0][xb4] >= 4 && above.Ref[0][xb4] < 7 {
			cnt[above.Ref[0][xb4]-4]++
		}
		if above.CompType[xb4] != 0 && above.Ref[1][xb4] >= 4 && above.Ref[1][xb4] < 7 {
			cnt[above.Ref[1][xb4]-4]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		if left.Ref[0][yb4] >= 4 && left.Ref[0][yb4] < 7 {
			cnt[left.Ref[0][yb4]-4]++
		}
		if left.CompType[yb4] != 0 && left.Ref[1][yb4] >= 4 && left.Ref[1][yb4] < 7 {
			cnt[left.Ref[1][yb4]-4]++
		}
	}
	switch {
	case cnt[0] == cnt[1]:
		return 1
	case cnt[0] < cnt[1]:
		return 0
	default:
		return 2
	}
}

func GetUniP1Ctx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	var cnt [3]int
	if haveTop && above.Intra[xb4] == 0 {
		if above.Ref[0][xb4]-1 >= 0 && above.Ref[0][xb4]-1 < 3 {
			cnt[above.Ref[0][xb4]-1]++
		}
		if above.CompType[xb4] != 0 && above.Ref[1][xb4]-1 >= 0 && above.Ref[1][xb4]-1 < 3 {
			cnt[above.Ref[1][xb4]-1]++
		}
	}
	if haveLeft && left.Intra[yb4] == 0 {
		if left.Ref[0][yb4]-1 >= 0 && left.Ref[0][yb4]-1 < 3 {
			cnt[left.Ref[0][yb4]-1]++
		}
		if left.CompType[yb4] != 0 && left.Ref[1][yb4]-1 >= 0 && left.Ref[1][yb4]-1 < 3 {
			cnt[left.Ref[1][yb4]-1]++
		}
	}
	cnt[1] += cnt[2]
	switch {
	case cnt[0] == cnt[1]:
		return 1
	case cnt[0] < cnt[1]:
		return 0
	default:
		return 2
	}
}

func GetJntCompCtx(seq av1.SequenceHeader, parsed *ParsedFrame, above, left *BlockContext, yb4, xb4 int, ref0, ref1 int8) int {
	offset := 0
	if parsed != nil && ref0 >= 0 && ref1 >= 0 && seq.EnableOrderHint {
		d0 := absInt(compoundPOCDiff(int(seq.OrderHintBitsMinus1)+1, int(parsed.RefFramePOC[ref0]), int(parsed.Header.FrameOffset)))
		d1 := absInt(compoundPOCDiff(int(seq.OrderHintBitsMinus1)+1, int(parsed.Header.FrameOffset), int(parsed.RefFramePOC[ref1])))
		offset = boolToInt(d0 == d1)
	}
	aCtx := boolToInt((above != nil && above.CompType[xb4] >= uint8(compoundTypeAvg)) || (above != nil && above.Ref[0][xb4] == 6))
	lCtx := boolToInt((left != nil && left.CompType[yb4] >= uint8(compoundTypeAvg)) || (left != nil && left.Ref[0][yb4] == 6))
	return 3*offset + aCtx + lCtx
}

func GetMaskCompCtx(above, left *BlockContext, yb4, xb4 int) int {
	aCtx := 0
	lCtx := 0
	if above != nil {
		if above.CompType[xb4] >= uint8(compoundTypeSeg) {
			aCtx = 1
		} else if above.Ref[0][xb4] == 6 {
			aCtx = 3
		}
	}
	if left != nil {
		if left.CompType[yb4] >= uint8(compoundTypeSeg) {
			lCtx = 1
		} else if left.Ref[0][yb4] == 6 {
			lCtx = 3
		}
	}
	return minInt(aCtx+lCtx, 5)
}

func GetDRLContext(weights []int, refIdx int) int {
	if refIdx+1 >= len(weights) {
		return 0
	}
	if weights[refIdx] >= 640 {
		return boolToInt(weights[refIdx+1] < 640)
	}
	if weights[refIdx+1] < 640 {
		return 2
	}
	return 0
}

func ReadSingleRefIndex(cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, dec intraEntropyDecoder) (int8, error) {
	return readSingleRefIndexWithTrace(cdf, above, left, yb4, xb4, haveTop, haveLeft, dec, nil)
}

func ReadSingleRefIndexTraced(cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, dec intraEntropyDecoder, parsed *ParsedFrame, g BlockGeometry) (int8, error) {
	return readSingleRefIndexWithTrace(cdf, above, left, yb4, xb4, haveTop, haveLeft, dec, func(stage string, ctx int, cdf []uint16, bit int) {
		debugTraceSingleRefBit(parsed, g, stage, ctx, cdf, bit, dec)
	})
}

func readSingleRefIndexWithTrace(cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, dec intraEntropyDecoder, trace func(stage string, ctx int, cdf []uint16, bit int)) (int8, error) {
	if cdf == nil {
		return -1, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil || dec == nil {
		return -1, fmt.Errorf("decoder: block contexts and entropy decoder required")
	}
	ctx1 := GetRefCtx(above, left, yb4, xb4, haveTop, haveLeft)
	bit := dec.DecodeBoolAdapt(cdf.Ref[0][ctx1][:])
	if trace != nil {
		trace("ref", ctx1, cdf.Ref[0][ctx1][:], int(bit))
	}
	if bit != 0 {
		ctx2 := GetBwdRefCtx(above, left, yb4, xb4, haveTop, haveLeft)
		bit = dec.DecodeBoolAdapt(cdf.Ref[1][ctx2][:])
		if trace != nil {
			trace("bwdref", ctx2, cdf.Ref[1][ctx2][:], int(bit))
		}
		if bit != 0 {
			return 6, nil
		}
		ctx3 := GetBwdRef1Ctx(above, left, yb4, xb4, haveTop, haveLeft)
		bit = dec.DecodeBoolAdapt(cdf.Ref[5][ctx3][:])
		if trace != nil {
			trace("bwdref1", ctx3, cdf.Ref[5][ctx3][:], int(bit))
		}
		return int8(4 + bit), nil
	}
	ctx2 := GetFwdRefCtx(above, left, yb4, xb4, haveTop, haveLeft)
	bit = dec.DecodeBoolAdapt(cdf.Ref[2][ctx2][:])
	if trace != nil {
		trace("fwdref", ctx2, cdf.Ref[2][ctx2][:], int(bit))
	}
	if bit != 0 {
		ctx3 := GetFwdRef2Ctx(above, left, yb4, xb4, haveTop, haveLeft)
		bit = dec.DecodeBoolAdapt(cdf.Ref[4][ctx3][:])
		if trace != nil {
			trace("fwdref2", ctx3, cdf.Ref[4][ctx3][:], int(bit))
		}
		return int8(2 + bit), nil
	}
	ctx3 := GetFwdRef1Ctx(above, left, yb4, xb4, haveTop, haveLeft)
	bit = dec.DecodeBoolAdapt(cdf.Ref[3][ctx3][:])
	if trace != nil {
		trace("fwdref1", ctx3, cdf.Ref[3][ctx3][:], int(bit))
	}
	return int8(bit), nil
}
