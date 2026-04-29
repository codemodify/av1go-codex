package decoder

import "fmt"

type InterPredMode uint8

const (
	InterPredNearest InterPredMode = iota
	InterPredNear
	InterPredGlobal
	InterPredNew
)

const (
	drlNearest = 0
	drlNearer  = 1
	drlNear    = 2
	drlNearish = 3
)

type SingleRefInterSyntax struct {
	Ref0     int8
	Ref1     int8
	Mode     InterPredMode
	DRLIndex uint8
}

func ReadSingleRefInterPayload(modeCDF *ModeCDF, mvCDF *MVCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, mvModeCtx, nMVs int, mvWeights []int, refMV MotionVector, mvPrec int, dec intraEntropyDecoder) (SingleRefInterSyntax, MotionVector, error) {
	syntax, err := ReadSingleRefInterSyntax(modeCDF, above, left, yb4, xb4, haveTop, haveLeft, mvModeCtx, nMVs, mvWeights, dec)
	if err != nil {
		return SingleRefInterSyntax{}, MotionVector{}, err
	}
	mv := refMV
	if syntax.Mode == InterPredNew {
		mv, err = ReadMVResidual(mvCDF, refMV, mvPrec, dec)
		if err != nil {
			return SingleRefInterSyntax{}, MotionVector{}, err
		}
	}
	return syntax, mv, nil
}

func ReadSingleRefInterSyntax(cdf *ModeCDF, above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool, mvModeCtx, nMVs int, mvWeights []int, dec intraEntropyDecoder) (SingleRefInterSyntax, error) {
	ref0, err := ReadSingleRefIndex(cdf, above, left, yb4, xb4, haveTop, haveLeft, dec)
	if err != nil {
		return SingleRefInterSyntax{}, err
	}
	mode, drl, err := ReadSingleRefInterMode(cdf, mvModeCtx, nMVs, mvWeights, dec)
	if err != nil {
		return SingleRefInterSyntax{}, err
	}
	return SingleRefInterSyntax{
		Ref0:     ref0,
		Ref1:     -1,
		Mode:     mode,
		DRLIndex: drl,
	}, nil
}

func ReadSingleRefInterMode(cdf *ModeCDF, mvModeCtx, nMVs int, mvWeights []int, dec intraEntropyDecoder) (InterPredMode, uint8, error) {
	return readSingleRefInterModeWithTrace(cdf, mvModeCtx, nMVs, mvWeights, dec, nil)
}

func ReadSingleRefInterModeTraced(cdf *ModeCDF, mvModeCtx, nMVs int, mvWeights []int, dec intraEntropyDecoder, parsed *ParsedFrame, g BlockGeometry) (InterPredMode, uint8, error) {
	return readSingleRefInterModeWithTrace(cdf, mvModeCtx, nMVs, mvWeights, dec, func(stage string, ctx int, before [2]uint16, after []uint16, bit int) {
		debugTraceSingleRefModeBit(parsed, g, stage, ctx, before, after, bit, dec)
	})
}

func readSingleRefInterModeWithTrace(cdf *ModeCDF, mvModeCtx, nMVs int, mvWeights []int, dec intraEntropyDecoder, trace func(stage string, ctx int, before [2]uint16, after []uint16, bit int)) (InterPredMode, uint8, error) {
	if cdf == nil {
		return 0, 0, fmt.Errorf("decoder: mode cdf required")
	}
	if dec == nil {
		return 0, 0, fmt.Errorf("decoder: entropy decoder required")
	}
	if nMVs < 0 {
		return 0, 0, fmt.Errorf("decoder: invalid mv candidate count %d", nMVs)
	}

	newCtx := minInt(mvModeCtx&7, len(cdf.NewMVMode)-1)
	if decodeSingleRefInterModeBool(cdf.NewMVMode[newCtx][:], "newmv", newCtx, dec, trace) != 0 {
		globalCtx := minInt((mvModeCtx>>3)&1, len(cdf.GlobalMVMode)-1)
		if decodeSingleRefInterModeBool(cdf.GlobalMVMode[globalCtx][:], "globalmv", globalCtx, dec, trace) == 0 {
			return InterPredGlobal, drlNearest, nil
		}
		refCtx := minInt((mvModeCtx>>4)&15, len(cdf.RefMVMode)-1)
		if decodeSingleRefInterModeBool(cdf.RefMVMode[refCtx][:], "refmv", refCtx, dec, trace) != 0 {
			drl := uint8(drlNearer)
			if nMVs > 2 {
				drlCtx := minInt(GetDRLContext(mvWeights, 1), len(cdf.DRLBit)-1)
				drl += uint8(decodeSingleRefInterModeBool(cdf.DRLBit[drlCtx][:], "drl-near-1", drlCtx, dec, trace))
				if drl == drlNear && nMVs > 3 {
					drlCtx = minInt(GetDRLContext(mvWeights, 2), len(cdf.DRLBit)-1)
					drl += uint8(decodeSingleRefInterModeBool(cdf.DRLBit[drlCtx][:], "drl-near-2", drlCtx, dec, trace))
				}
			}
			return InterPredNear, drl, nil
		}
		return InterPredNearest, drlNearest, nil
	}

	drl := uint8(drlNearest)
	if nMVs > 1 {
		drlCtx := minInt(GetDRLContext(mvWeights, 0), len(cdf.DRLBit)-1)
		drl += uint8(decodeSingleRefInterModeBool(cdf.DRLBit[drlCtx][:], "drl-new-0", drlCtx, dec, trace))
		if drl == drlNearer && nMVs > 2 {
			drlCtx = minInt(GetDRLContext(mvWeights, 1), len(cdf.DRLBit)-1)
			drl += uint8(decodeSingleRefInterModeBool(cdf.DRLBit[drlCtx][:], "drl-new-1", drlCtx, dec, trace))
		}
	}
	return InterPredNew, drl, nil
}

func decodeSingleRefInterModeBool(cdf []uint16, stage string, ctx int, dec intraEntropyDecoder, trace func(stage string, ctx int, before [2]uint16, after []uint16, bit int)) uint32 {
	before := [2]uint16{}
	if len(cdf) >= 2 {
		before = [2]uint16{cdf[0], cdf[1]}
	}
	bit := dec.DecodeBoolAdapt(cdf)
	if trace != nil {
		trace(stage, ctx, before, cdf, int(bit))
	}
	return bit
}
