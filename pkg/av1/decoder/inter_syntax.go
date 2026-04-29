package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type InterBlockSyntax struct {
	Geometry       BlockGeometry
	Size           BlockSize
	Segment        uint8
	SegPred        bool
	SkipMode       bool
	Skip           bool
	Intra          bool
	IntraBlk       *IntraBlockSyntax
	Ref0           int8
	Ref1           int8
	Mode           InterPredMode
	Mode1          InterPredMode
	DRLIndex       uint8
	MV             MotionVector
	MV1            MotionVector
	Motion         InterMotionMode
	Filter         InterFilterPair
	CompType       CompoundType
	MaskSign       bool
	WedgeIdx       uint8
	InterIntraType InterIntraType
	InterIntraMode IntraPredMode
}

func ReadInterBlockPrelude(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, segID uint8, dec intraEntropyDecoder) (InterBlockSyntax, error) {
	return ReadInterBlockPreludeWithNeighbors(seq, hdr, cdf, above, left, g, g.Start4Y > 0, g.Start4X > 0, segID, dec)
}

func ReadInterBlockPreludeWithNeighbors(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (InterBlockSyntax, error) {
	syntax, err := ReadInterBlockSkipPreludeWithNeighbors(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	if err != nil {
		return InterBlockSyntax{}, err
	}
	intra, err := ReadInterBlockIntraFlagWithNeighbors(hdr, cdf, above, left, g, haveTop, haveLeft, segID, syntax.SkipMode, dec)
	if err != nil {
		return InterBlockSyntax{}, err
	}
	syntax.Intra = intra
	return syntax, nil
}

func ReadInterBlockSkipPreludeWithNeighbors(_ av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (InterBlockSyntax, error) {
	if hdr == nil {
		return InterBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if !isInterOrSwitchFrameType(hdr.FrameType) {
		return InterBlockSyntax{}, fmt.Errorf("decoder: inter syntax requires inter/switch frame type")
	}
	if cdf == nil {
		return InterBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return InterBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return InterBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}
	syntax := InterBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		Ref0:     -1,
		Ref1:     -1,
	}
	seg := segmentDataForID(hdr, segID)

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	dims := BlockDims(g.Size)

	if hdr.SkipModeEnabled && minInt(int(dims.W4), int(dims.H4)) > 1 && seg.Ref < 0 && !seg.GlobalMV && !seg.Skip {
		smctx := int(above.SkipMode[bx4]) + int(left.SkipMode[by4])
		syntax.SkipMode = dec.DecodeBoolAdapt(cdf.SkipMode[smctx][:]) != 0
	}
	if syntax.SkipMode {
		syntax.Skip = true
	} else if seg.Skip {
		syntax.Skip = true
	} else {
		sctx := int(above.Skip[bx4]) + int(left.Skip[by4])
		syntax.Skip = dec.DecodeBoolAdapt(cdf.Skip[sctx][:]) != 0
	}
	return syntax, nil
}

func ReadInterBlockIntraFlagWithNeighbors(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, skipMode bool, dec intraEntropyDecoder) (bool, error) {
	if hdr == nil {
		return false, fmt.Errorf("decoder: frame header required")
	}
	if !isInterOrSwitchFrameType(hdr.FrameType) {
		return false, fmt.Errorf("decoder: inter syntax requires inter/switch frame type")
	}
	if cdf == nil {
		return false, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return false, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return false, fmt.Errorf("decoder: entropy decoder required")
	}
	if skipMode {
		return false, nil
	}
	seg := segmentDataForID(hdr, segID)
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	if seg.Ref >= 0 || seg.GlobalMV {
		return seg.Ref == 0, nil
	}
	ictx := GetIntraCtx(above, left, by4, bx4, haveTop, haveLeft)
	bit := dec.DecodeBoolAdapt(cdf.Intra[ictx][:])
	return bit == 0, nil
}

func ReadInterBlockSyntax(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, segID uint8, dec intraEntropyDecoder) (InterBlockSyntax, error) {
	return ReadInterBlockSyntaxWithNeighbors(seq, hdr, cdf, above, left, g, g.Start4Y > 0, g.Start4X > 0, segID, dec)
}

func ReadInterBlockSyntaxWithNeighbors(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (InterBlockSyntax, error) {
	syntax, err := ReadInterBlockPreludeWithNeighbors(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	if err != nil {
		return InterBlockSyntax{}, err
	}
	if !syntax.Intra {
		return syntax, nil
	}

	intra, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	if err != nil {
		return InterBlockSyntax{}, err
	}
	intra.SkipMode = syntax.SkipMode
	intra.Skip = syntax.Skip
	syntax.IntraBlk = &intra
	return syntax, nil
}

func ApplyInterBlockContext(seq av1.SequenceHeader, hdr *obu.FrameHeader, above, left *BlockContext, syntax InterBlockSyntax) error {
	if hdr == nil {
		return fmt.Errorf("decoder: frame header required")
	}
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	dims := BlockDims(syntax.Size)
	bx4 := syntax.Geometry.Start4X & 31
	by4 := syntax.Geometry.Start4Y & 31

	if syntax.Intra && syntax.IntraBlk != nil {
		if err := ApplyIntraBlockContext(seq, hdr, above, left, *syntax.IntraBlk); err != nil {
			return err
		}
	} else {
		ref1 := syntax.Ref1
		if syntax.CompType == compoundTypeNone {
			ref1 = -1
		}
		fillRunInt8(above.TxIntra[:], bx4, 1<<dims.Log2W4, int8(dims.Log2W4))
		fillRunInt8(left.TxIntra[:], by4, 1<<dims.Log2H4, int8(dims.Log2H4))
		fillRunUint8(above.Mode[:], bx4, 1<<dims.Log2W4, uint8(syntax.Mode))
		fillRunUint8(left.Mode[:], by4, 1<<dims.Log2H4, uint8(syntax.Mode))
		fillRunUint8(above.PalSz[:], bx4, 1<<dims.Log2W4, 0)
		fillRunUint8(left.PalSz[:], by4, 1<<dims.Log2H4, 0)
		fillRunUint8(above.UVPalSz[:], bx4, 1<<dims.Log2W4, 0)
		fillRunUint8(left.UVPalSz[:], by4, 1<<dims.Log2H4, 0)
		fillRunUint8(above.Intra[:], bx4, 1<<dims.Log2W4, 0)
		fillRunUint8(left.Intra[:], by4, 1<<dims.Log2H4, 0)
		fillRunUint8(above.CompType[:], bx4, 1<<dims.Log2W4, uint8(syntax.CompType))
		fillRunUint8(left.CompType[:], by4, 1<<dims.Log2H4, uint8(syntax.CompType))
		fillRunInt8(above.Ref[0][:], bx4, 1<<dims.Log2W4, syntax.Ref0)
		fillRunInt8(left.Ref[0][:], by4, 1<<dims.Log2H4, syntax.Ref0)
		fillRunInt8(above.Ref[1][:], bx4, 1<<dims.Log2W4, ref1)
		fillRunInt8(left.Ref[1][:], by4, 1<<dims.Log2H4, ref1)
		fillRunUint8(above.Filter[0][:], bx4, 1<<dims.Log2W4, syntax.Filter[0])
		fillRunUint8(left.Filter[0][:], by4, 1<<dims.Log2H4, syntax.Filter[0])
		fillRunUint8(above.Filter[1][:], bx4, 1<<dims.Log2W4, syntax.Filter[1])
		fillRunUint8(left.Filter[1][:], by4, 1<<dims.Log2H4, syntax.Filter[1])
		if hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
			cw4, ch4 := chromaBlockDims(seq.ColorConfig.ChromaSubsampling(), dims)
			cbx4 := bx4
			cby4 := by4
			if seq.ColorConfig.ChromaSubsampling() != av1.Chroma444 {
				cbx4 >>= 1
			}
			if seq.ColorConfig.ChromaSubsampling() == av1.Chroma420 {
				cby4 >>= 1
			}
			fillRunUint8(above.UVMode[:], cbx4, cw4, uint8(IntraPredDC))
			fillRunUint8(left.UVMode[:], cby4, ch4, uint8(IntraPredDC))
		}
	}

	fillRunUint8(above.SegPred[:], bx4, 1<<dims.Log2W4, boolToByte(syntax.SegPred))
	fillRunUint8(left.SegPred[:], by4, 1<<dims.Log2H4, boolToByte(syntax.SegPred))
	fillRunUint8(above.SkipMode[:], bx4, 1<<dims.Log2W4, boolToByte(syntax.SkipMode))
	fillRunUint8(left.SkipMode[:], by4, 1<<dims.Log2H4, boolToByte(syntax.SkipMode))
	fillRunUint8(above.Skip[:], bx4, 1<<dims.Log2W4, boolToByte(syntax.Skip))
	fillRunUint8(left.Skip[:], by4, 1<<dims.Log2H4, boolToByte(syntax.Skip))
	return nil
}
