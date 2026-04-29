package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func compoundRefsAllowed(seq av1.SequenceHeader, hdr *obu.FrameHeader, seg obu.SegmentData, g BlockGeometry) bool {
	if hdr == nil || !isInterOrSwitchFrameType(hdr.FrameType) || hdr.AllowIntrabc {
		return false
	}
	if !hdr.SwitchableCompRefs || minInt(g.Width4(), g.Height4()) <= 1 {
		return false
	}
	if seg.Ref >= 0 || seg.GlobalMV || seg.Skip {
		return false
	}
	return true
}

func ReadCompoundFlag(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, seg obu.SegmentData, g BlockGeometry, dec intraEntropyDecoder) (bool, error) {
	if !compoundRefsAllowed(seq, hdr, seg, g) {
		return false, nil
	}
	if cdf == nil || above == nil || left == nil || dec == nil {
		return false, fmt.Errorf("decoder: compound flag decode requires cdf, contexts, and entropy decoder")
	}
	ctx := GetCompCtx(above, left, yb4, xb4, haveTop, haveLeft)
	return dec.DecodeBoolAdapt(cdf.Comp[ctx][:]) != 0, nil
}

func ReadCompoundRefSelection(cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, dec intraEntropyDecoder) (int8, int8, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return -1, -1, fmt.Errorf("decoder: compound ref decode requires cdf, contexts, and entropy decoder")
	}
	dirCtx := GetCompDirCtx(above, left, yb4, xb4, haveTop, haveLeft)
	if dec.DecodeBoolAdapt(cdf.CompDir[dirCtx][:]) != 0 {
		ctx1 := GetFwdRefCtx(above, left, yb4, xb4, haveTop, haveLeft)
		ref0 := int8(0)
		if dec.DecodeBoolAdapt(cdf.CompFwdRef[0][ctx1][:]) != 0 {
			ctx2 := GetFwdRef2Ctx(above, left, yb4, xb4, haveTop, haveLeft)
			ref0 = 2 + int8(dec.DecodeBoolAdapt(cdf.CompFwdRef[2][ctx2][:]))
		} else {
			ctx2 := GetFwdRef1Ctx(above, left, yb4, xb4, haveTop, haveLeft)
			ref0 = int8(dec.DecodeBoolAdapt(cdf.CompFwdRef[1][ctx2][:]))
		}

		ctx3 := GetBwdRefCtx(above, left, yb4, xb4, haveTop, haveLeft)
		if dec.DecodeBoolAdapt(cdf.CompBwdRef[0][ctx3][:]) != 0 {
			return ref0, 6, nil
		}
		ctx4 := GetBwdRef1Ctx(above, left, yb4, xb4, haveTop, haveLeft)
		return ref0, 4 + int8(dec.DecodeBoolAdapt(cdf.CompBwdRef[1][ctx4][:])), nil
	}

	uctxP := GetRefCtx(above, left, yb4, xb4, haveTop, haveLeft)
	if dec.DecodeBoolAdapt(cdf.CompUniRef[0][uctxP][:]) != 0 {
		return 4, 6, nil
	}
	uctxP1 := GetUniP1Ctx(above, left, yb4, xb4, haveTop, haveLeft)
	ref1 := 1 + int8(dec.DecodeBoolAdapt(cdf.CompUniRef[1][uctxP1][:]))
	if ref1 == 2 {
		uctxP2 := GetFwdRef2Ctx(above, left, yb4, xb4, haveTop, haveLeft)
		ref1 += int8(dec.DecodeBoolAdapt(cdf.CompUniRef[2][uctxP2][:]))
	}
	return 0, ref1, nil
}

func ReadCompoundInterMode(cdf *ModeCDF, mvModeCtx, nMVs int, mvWeights []int, dec intraEntropyDecoder) (InterPredMode, InterPredMode, uint8, error) {
	if cdf == nil || dec == nil {
		return 0, 0, 0, fmt.Errorf("decoder: compound inter mode decode requires cdf and entropy decoder")
	}
	ctx := minInt(mvModeCtx, len(cdf.CompInterMode)-1)
	sym := dec.DecodeSymbolAdapt(cdf.CompInterMode[ctx][:], len(generatedCompInterPredModes)-1)
	if int(sym) >= len(generatedCompInterPredModes) {
		return 0, 0, 0, fmt.Errorf("decoder: invalid compound inter mode symbol %d", sym)
	}
	modes := generatedCompInterPredModes[sym]
	drl := uint8(drlNearest)
	if modes[0] == InterPredNew && modes[1] == InterPredNew {
		if nMVs > 1 {
			drl += uint8(dec.DecodeBoolAdapt(cdf.DRLBit[minInt(GetDRLContext(mvWeights, 0), len(cdf.DRLBit)-1)][:]))
			if drl == drlNearer && nMVs > 2 {
				drl += uint8(dec.DecodeBoolAdapt(cdf.DRLBit[minInt(GetDRLContext(mvWeights, 1), len(cdf.DRLBit)-1)][:]))
			}
		}
	} else if modes[0] == InterPredNear || modes[1] == InterPredNear {
		drl = uint8(drlNearer)
		if nMVs > 2 {
			drl += uint8(dec.DecodeBoolAdapt(cdf.DRLBit[minInt(GetDRLContext(mvWeights, 1), len(cdf.DRLBit)-1)][:]))
			if drl == drlNear && nMVs > 3 {
				drl += uint8(dec.DecodeBoolAdapt(cdf.DRLBit[minInt(GetDRLContext(mvWeights, 2), len(cdf.DRLBit)-1)][:]))
			}
		}
	}
	return modes[0], modes[1], drl, nil
}

func ReadCompoundType(seq av1.SequenceHeader, parsed *ParsedFrame, cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, g BlockGeometry, ref0, ref1 int8, dec intraEntropyDecoder) (CompoundType, bool, uint8, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return compoundTypeNone, false, 0, fmt.Errorf("decoder: compound type decode requires cdf, contexts, and entropy decoder")
	}
	isSegWedge := false
	if seq.EnableMaskedCompound {
		maskCtx := GetMaskCompCtx(above, left, yb4, xb4)
		isSegWedge = dec.DecodeBoolAdapt(cdf.MaskComp[maskCtx][:]) != 0
	}
	if !isSegWedge {
		if seq.EnableJntComp {
			jntCtx := GetJntCompCtx(seq, parsed, above, left, yb4, xb4, ref0, ref1)
			if dec.DecodeBoolAdapt(cdf.JntComp[jntCtx][:]) != 0 {
				return compoundTypeAvg, false, 0, nil
			}
			return compoundTypeWeightedAvg, false, 0, nil
		}
		return compoundTypeAvg, false, 0, nil
	}
	if wedgeAllowedForBlock(g.Size) {
		ctx := wedgeContextForBlock(g.Size)
		if dec.DecodeBoolAdapt(cdf.WedgeComp[ctx][:]) == 0 {
			wedgeIdx := uint8(dec.DecodeSymbolAdapt(cdf.WedgeIdx[ctx][:], 15))
			return compoundTypeWedge, dec.DecodeBoolEqui() != 0, wedgeIdx, nil
		}
		return compoundTypeSeg, dec.DecodeBoolEqui() != 0, 0, nil
	}
	return compoundTypeSeg, dec.DecodeBoolEqui() != 0, 0, nil
}

func ReadInterIntraSyntax(seq av1.SequenceHeader, cdf *ModeCDF, g BlockGeometry, dec intraEntropyDecoder) (InterIntraType, IntraPredMode, uint8, error) {
	if !seq.EnableInterIntraCompound || !interIntraAllowedForBlock(g.Size) {
		return interIntraNone, IntraPredDC, 0, nil
	}
	if cdf == nil || dec == nil {
		return interIntraNone, IntraPredDC, 0, fmt.Errorf("decoder: interintra decode requires cdf and entropy decoder")
	}
	szGrp := interIntraSizeGroup(g.Size)
	if dec.DecodeBoolAdapt(cdf.InterIntra[szGrp][:]) == 0 {
		return interIntraNone, IntraPredDC, 0, nil
	}
	modeSym := dec.DecodeSymbolAdapt(cdf.InterIntraMode[szGrp][:], len(generatedInterIntraModes)-1)
	if int(modeSym) >= len(generatedInterIntraModes) {
		return interIntraNone, IntraPredDC, 0, fmt.Errorf("decoder: invalid interintra mode symbol %d", modeSym)
	}
	mode := generatedInterIntraModes[modeSym]
	wedgeCtx := wedgeContextForBlock(g.Size)
	if dec.DecodeBoolAdapt(cdf.InterIntraWedge[wedgeCtx][:]) != 0 {
		wedgeIdx := uint8(dec.DecodeSymbolAdapt(cdf.WedgeIdx[wedgeCtx][:], 15))
		return interIntraWedge, mode, wedgeIdx, nil
	}
	return interIntraBlend, mode, 0, nil
}
