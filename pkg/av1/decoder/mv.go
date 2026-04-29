package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type MotionVector struct {
	Y int
	X int
}

type MVJoint uint8

const (
	MVJointZero MVJoint = iota
	MVJointH
	MVJointV
	MVJointHV
)

func ReadMVComponentDiff(cdf *MVComponentCDF, mvPrec int, dec intraEntropyDecoder) (int, error) {
	if cdf == nil {
		return 0, fmt.Errorf("decoder: mv component cdf required")
	}
	if dec == nil {
		return 0, fmt.Errorf("decoder: entropy decoder required")
	}

	sign := dec.DecodeBoolAdapt(cdf.Sign[:])
	cl := int(dec.DecodeSymbolAdapt(cdf.Classes[:], 10))
	up, fp, hp := 0, 3, 1

	if cl == 0 {
		up = int(dec.DecodeBoolAdapt(cdf.Class0[:]))
		if mvPrec >= 0 {
			fp = int(dec.DecodeSymbolAdapt(cdf.Class0FP[up][:], 3))
			if mvPrec > 0 {
				hp = int(dec.DecodeBoolAdapt(cdf.Class0HP[:]))
			}
		}
	} else {
		up = 1 << cl
		for n := 0; n < cl; n++ {
			up |= int(dec.DecodeBoolAdapt(cdf.ClassN[n][:])) << n
		}
		if mvPrec >= 0 {
			fp = int(dec.DecodeSymbolAdapt(cdf.ClassNFP[:], 3))
			if mvPrec > 0 {
				hp = int(dec.DecodeBoolAdapt(cdf.ClassNHP[:]))
			}
		}
	}

	diff := ((up << 3) | (fp << 1) | hp) + 1
	if sign != 0 {
		return -diff, nil
	}
	return diff, nil
}

func ReadMVResidual(cdf *MVCDF, ref MotionVector, mvPrec int, dec intraEntropyDecoder) (MotionVector, error) {
	if cdf == nil {
		return MotionVector{}, fmt.Errorf("decoder: mv cdf required")
	}
	if dec == nil {
		return MotionVector{}, fmt.Errorf("decoder: entropy decoder required")
	}

	mv := ref
	joint := MVJoint(dec.DecodeSymbolAdapt(cdf.Joint[:], 3))
	if joint&MVJointV != 0 {
		diff, err := ReadMVComponentDiff(&cdf.Comp[0], mvPrec, dec)
		if err != nil {
			return MotionVector{}, err
		}
		mv.Y += diff
	}
	if joint&MVJointH != 0 {
		diff, err := ReadMVComponentDiff(&cdf.Comp[1], mvPrec, dec)
		if err != nil {
			return MotionVector{}, err
		}
		mv.X += diff
	}
	return mv, nil
}

func activeRefMVCandidateCount(cands []RefMVCandidate) int {
	count := 0
	for i := range cands {
		if cands[i].Weight > 0 {
			count++
		}
	}
	return count
}

func mvResidualPrecision(hdr *obu.FrameHeader) int {
	if hdr == nil || hdr.ForceIntegerMV {
		return -1
	}
	if hdr.HighPrecisionMV {
		return 1
	}
	return 0
}

func normalizeSingleRefMV(hdr *obu.FrameHeader, mode InterPredMode, drl uint8, nMVs int, mv MotionVector, global MotionVector) MotionVector {
	if mode == InterPredGlobal {
		return global
	}
	switch mode {
	case InterPredNearest:
		return fixMVPrecision(hdr, mv)
	case InterPredNear:
		if drl < drlNear {
			return fixMVPrecision(hdr, mv)
		}
	case InterPredNew:
		if nMVs <= 1 {
			return fixMVPrecision(hdr, mv)
		}
	}
	return mv
}

func normalizeCompoundBaseMV(hdr *obu.FrameHeader, mode InterPredMode, mv MotionVector, global MotionVector) MotionVector {
	if mode == InterPredGlobal {
		return global
	}
	if mode == InterPredNearest || mode == InterPredNear {
		return fixMVPrecision(hdr, mv)
	}
	return mv
}

func applyTemporalGlobalMVContext(mvCtx int, temporal []RefMVCandidate, hdr *obu.FrameHeader, ref0 int8, g BlockGeometry) int {
	if hdr == nil || !hdr.UseRefFrameMvs || len(temporal) == 0 {
		return mvCtx
	}
	gmv := approximateGlobalMotionVectorForBlock(hdr, ref0, g)
	globalmvCtx := 1
	if absInt(temporal[0].MV.X-gmv.X)|absInt(temporal[0].MV.Y-gmv.Y) < 16 {
		globalmvCtx = 0
	}
	return (mvCtx &^ (1 << 3)) | (globalmvCtx << 3)
}

func fixMVPrecision(hdr *obu.FrameHeader, mv MotionVector) MotionVector {
	if hdr == nil {
		return mv
	}
	if hdr.ForceIntegerMV {
		mv.X = fixIntMVPrecisionComponent(mv.X)
		mv.Y = fixIntMVPrecisionComponent(mv.Y)
		return mv
	}
	if !hdr.HighPrecisionMV {
		mv.X = fixLowPrecisionComponent(mv.X)
		mv.Y = fixLowPrecisionComponent(mv.Y)
	}
	return mv
}

func fixIntMVPrecisionComponent(v int) int {
	if v >= 0 {
		return (v + 3) &^ 7
	}
	return -(((-v) + 3) &^ 7)
}

func fixLowPrecisionComponent(v int) int {
	if v >= 0 {
		return v &^ 1
	}
	return -((-v) &^ 1)
}
