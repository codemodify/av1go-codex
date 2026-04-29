package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func ReadIntraSkipPrelude(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, dec intraEntropyDecoder) (bool, bool, error) {
	if hdr == nil {
		return false, false, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return false, false, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return false, false, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return false, false, fmt.Errorf("decoder: entropy decoder required")
	}
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	dims := BlockDims(g.Size)
	skipMode := false
	if hdr.SkipModeEnabled && minInt(int(dims.W4), int(dims.H4)) > 1 {
		smctx := int(above.SkipMode[bx4]) + int(left.SkipMode[by4])
		skipMode = dec.DecodeBoolAdapt(cdf.SkipMode[smctx][:]) != 0
	}
	if skipMode {
		return true, true, nil
	}
	sctx := int(above.Skip[bx4]) + int(left.Skip[by4])
	skip := dec.DecodeBoolAdapt(cdf.Skip[sctx][:]) != 0
	return false, skip, nil
}

func ApplyIntraSkipFlags(above, left *BlockContext, g BlockGeometry, skipMode, skip bool) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	dims := BlockDims(g.Size)
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	fillRunUint8(above.SkipMode[:], bx4, 1<<dims.Log2W4, boolToByte(skipMode))
	fillRunUint8(left.SkipMode[:], by4, 1<<dims.Log2H4, boolToByte(skipMode))
	fillRunUint8(above.Skip[:], bx4, 1<<dims.Log2W4, boolToByte(skip))
	fillRunUint8(left.Skip[:], by4, 1<<dims.Log2H4, boolToByte(skip))
	return nil
}

func ReconstructParsedIntraLumaBlockAt8WithSkip(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, skip bool, dec keyIntraBlockEntropyDecoder) error {
	return ReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, skip, dec, true, true)
}

func ReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, skip bool, dec keyIntraBlockEntropyDecoder, topHasRight, leftHasBottom bool) error {
	return ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, skip, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, topHasRight, leftHasBottom)
}

func ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, skip bool, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if !skip {
		return ReconstructParsedIntraLumaBlockAt8NeighborEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	}
	if hdr == nil {
		return fmt.Errorf("decoder: frame header required")
	}

	g := syntax.Geometry
	blockX := planeX4 * 4
	blockY := planeY4 * 4
	predictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	txInfo := TxfmInfoFor(syntax.TX)
	if blockWidth4%int(txInfo.W4) != 0 || blockHeight4%int(txInfo.H4) != 0 {
		return fmt.Errorf("decoder: block size %d (%dx%d in 4x4 units) cannot be divided into tx %d", syntax.Size, blockWidth4, blockHeight4, syntax.TX)
	}

	zeroScratch := takeZeroCoeffScratch(syntax.TX)
	defer putCoeffScratch(zeroScratch)
	zero := zeroScratch.buf
	for offY4 := 0; offY4 < blockHeight4; offY4 += int(txInfo.H4) {
		for offX4 := 0; offX4 < blockWidth4; offX4 += int(txInfo.W4) {
			tu := BlockGeometry{
				Level:   g.Level,
				Size:    g.Size,
				Start4X: g.Start4X + offX4,
				Start4Y: g.Start4Y + offY4,
				End4X:   g.Start4X + offX4 + int(txInfo.W4),
				End4Y:   g.Start4Y + offY4 + int(txInfo.H4),
			}
			px := (planeX4 + offX4) * 4
			py := (planeY4 + offY4) * 4
			var err error
			if syntax.PalYSize != 0 {
				err = ReconstructPaletteLumaCoefficients8(plane, stride, planeWidth, planeHeight, px, py, syntax.TX, zero, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
			} else {
				tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, syntax.TX, intraEdgeAvailability{
					topHasRight:   topHasRight,
					leftHasBottom: leftHasBottom,
				})
				err = ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
					plane,
					stride,
					planeWidth,
					planeHeight,
					px,
					py,
					syntax.TX,
					TxTypeDCTDCT,
					syntax.YMode,
					syntax.YAngle,
					zero,
					hdr.Segmentation.Lossless[syntax.Segment],
					predictFlags,
					haveTop || offY4 > 0,
					haveLeft || offX4 > 0,
					tuEdge.topHasRight,
					tuEdge.leftHasBottom,
				)
			}
			if err != nil {
				return err
			}
			if err := ApplyLumaResidualContext(above, left, tu, syntax.TX, 0x40); err != nil {
				return err
			}
		}
	}
	return nil
}

func reconstructParsedIntraChromaBlockAt8WithSkip(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []byte, lumaStride, lumaWidth, lumaHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, skip bool, dec keyIntraBlockEntropyDecoder) error {
	return reconstructParsedIntraChromaBlockAt8WithSkipEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, dec, skip, true, true)
}

func reconstructParsedIntraChromaBlockAt8WithSkipEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []byte, lumaStride, lumaWidth, lumaHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, dec keyIntraBlockEntropyDecoder, skip bool, topHasRight, leftHasBottom bool) error {
	return reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, dec, skip, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, topHasRight, leftHasBottom)
}

func reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []byte, lumaStride, lumaWidth, lumaHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, dec keyIntraBlockEntropyDecoder, skip bool, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if !skip {
		return reconstructParsedIntraChromaBlockAt8NeighborEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	}
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	layout := seq.ColorConfig.ChromaSubsampling()
	chromaGeom := chromaGeometryForBlock(layout, syntax.Geometry)
	paletteIndexStride, _ := chromaPaletteIndexDimensions(layout, syntax.Geometry)
	txInfo := TxfmInfoFor(syntax.UVTX)
	zeroScratch := takeZeroCoeffScratch(syntax.UVTX)
	defer putCoeffScratch(zeroScratch)
	zero := zeroScratch.buf
	blockX := chromaGeom.Start4X * 4
	blockY := chromaGeom.Start4Y * 4
	predictFlags := intraChromaPredictFlags(seq, above, left, syntax)
	for offY4 := 0; offY4 < chromaGeom.Height4(); offY4 += int(txInfo.H4) {
		for offX4 := 0; offX4 < chromaGeom.Width4(); offX4 += int(txInfo.W4) {
			tu := BlockGeometry{
				Level:   chromaGeom.Level,
				Size:    chromaGeom.Size,
				Start4X: chromaGeom.Start4X + offX4,
				Start4Y: chromaGeom.Start4Y + offY4,
				End4X:   min(chromaGeom.Start4X+offX4+int(txInfo.W4), chromaGeom.End4X),
				End4Y:   min(chromaGeom.Start4Y+offY4+int(txInfo.H4), chromaGeom.End4Y),
			}
			planeX := tu.Start4X * 4
			planeY := tu.Start4Y * 4
			var err error
			if syntax.PalUVSize != 0 {
				err = ReconstructPaletteChromaCoefficients8(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, zero, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalU, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
			} else {
				lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
				tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
					topHasRight:   topHasRight,
					leftHasBottom: leftHasBottom,
				})
				err = ReconstructIntraChromaCoefficients8WithTxTypeFlagsNeighborEdges(
					uPlane,
					uStride,
					uWidth,
					uHeight,
					planeX,
					planeY,
					syntax.UVTX,
					TxTypeDCTDCT,
					syntax.UVMode,
					syntax.UVAngle,
					syntax.CFLAlpha[0],
					zero,
					hdr.Segmentation.Lossless[syntax.Segment],
					luma,
					lumaStride,
					lumaWidth,
					lumaHeight,
					lumaX,
					lumaY,
					predictFlags,
					haveTop || offY4 > 0,
					haveLeft || offX4 > 0,
					tuEdge.topHasRight,
					tuEdge.leftHasBottom,
				)
			}
			if err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, left, 1, layout, tu, syntax.UVTX, 0x40); err != nil {
				return err
			}

			err = nil
			if syntax.PalUVSize != 0 {
				err = ReconstructPaletteChromaCoefficients8(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, zero, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalV, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
			} else {
				lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
				tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
					topHasRight:   topHasRight,
					leftHasBottom: leftHasBottom,
				})
				err = ReconstructIntraChromaCoefficients8WithTxTypeFlagsNeighborEdges(
					vPlane,
					vStride,
					vWidth,
					vHeight,
					planeX,
					planeY,
					syntax.UVTX,
					TxTypeDCTDCT,
					syntax.UVMode,
					syntax.UVAngle,
					syntax.CFLAlpha[1],
					zero,
					hdr.Segmentation.Lossless[syntax.Segment],
					luma,
					lumaStride,
					lumaWidth,
					lumaHeight,
					lumaX,
					lumaY,
					predictFlags,
					haveTop || offY4 > 0,
					haveLeft || offX4 > 0,
					tuEdge.topHasRight,
					tuEdge.leftHasBottom,
				)
			}
			if err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, left, 2, layout, tu, syntax.UVTX, 0x40); err != nil {
				return err
			}
		}
	}
	return nil
}

func ReconstructParsedIntraLumaBlockAt16WithSkip(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, skip bool, dec keyIntraBlockEntropyDecoder) error {
	return ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, skip, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, true, true)
}

func ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, skip bool, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if !skip {
		return ReconstructParsedIntraLumaBlockAt16NeighborEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	}
	if hdr == nil {
		return fmt.Errorf("decoder: frame header required")
	}

	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	txInfo := TxfmInfoFor(syntax.TX)
	g := syntax.Geometry
	if blockWidth4%int(txInfo.W4) != 0 || blockHeight4%int(txInfo.H4) != 0 {
		return fmt.Errorf("decoder: block size %d (%dx%d in 4x4 units) cannot be divided into tx %d", syntax.Size, blockWidth4, blockHeight4, syntax.TX)
	}
	blockX := planeX4 * 4
	blockY := planeY4 * 4
	predictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	zeroScratch := takeZeroCoeffScratch(syntax.TX)
	defer putCoeffScratch(zeroScratch)
	zero := zeroScratch.buf
	for offY4 := 0; offY4 < blockHeight4; offY4 += int(txInfo.H4) {
		for offX4 := 0; offX4 < blockWidth4; offX4 += int(txInfo.W4) {
			tu := BlockGeometry{
				Level:   g.Level,
				Size:    g.Size,
				Start4X: g.Start4X + offX4,
				Start4Y: g.Start4Y + offY4,
				End4X:   g.Start4X + offX4 + int(txInfo.W4),
				End4Y:   g.Start4Y + offY4 + int(txInfo.H4),
			}
			px := (planeX4 + offX4) * 4
			py := (planeY4 + offY4) * 4
			var err error
			if syntax.PalYSize != 0 {
				err = ReconstructPaletteLumaCoefficients16(plane, stride, planeWidth, planeHeight, px, py, syntax.TX, zero, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
			} else {
				tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, syntax.TX, intraEdgeAvailability{
					topHasRight:   topHasRight,
					leftHasBottom: leftHasBottom,
				})
				err = ReconstructIntraLumaCoefficients16WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, px, py, syntax.TX, TxTypeDCTDCT, syntax.YMode, syntax.YAngle, zero, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
			}
			if err != nil {
				return err
			}
			if err := ApplyLumaResidualContext(above, left, tu, syntax.TX, 0x40); err != nil {
				return err
			}
		}
	}
	return nil
}

func reconstructParsedIntraChromaBlockAt16WithSkip(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []uint16, lumaStride, lumaWidth, lumaHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, bitDepth uint8, skip bool, dec keyIntraBlockEntropyDecoder) error {
	return reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, bitDepth, skip, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, true, true)
}

func reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []uint16, lumaStride, lumaWidth, lumaHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, bitDepth uint8, skip bool, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if !skip {
		return reconstructParsedIntraChromaBlockAt16NeighborEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, bitDepth, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	}
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	layout := seq.ColorConfig.ChromaSubsampling()
	chromaGeom := chromaGeometryForBlock(layout, syntax.Geometry)
	paletteIndexStride, _ := chromaPaletteIndexDimensions(layout, syntax.Geometry)
	txInfo := TxfmInfoFor(syntax.UVTX)
	zeroScratch := takeZeroCoeffScratch(syntax.UVTX)
	defer putCoeffScratch(zeroScratch)
	zero := zeroScratch.buf
	blockX := chromaGeom.Start4X * 4
	blockY := chromaGeom.Start4Y * 4
	predictFlags := intraChromaPredictFlags(seq, above, left, syntax)
	for offY4 := 0; offY4 < chromaGeom.Height4(); offY4 += int(txInfo.H4) {
		for offX4 := 0; offX4 < chromaGeom.Width4(); offX4 += int(txInfo.W4) {
			tu := BlockGeometry{
				Level:   chromaGeom.Level,
				Size:    chromaGeom.Size,
				Start4X: chromaGeom.Start4X + offX4,
				Start4Y: chromaGeom.Start4Y + offY4,
				End4X:   min(chromaGeom.Start4X+offX4+int(txInfo.W4), chromaGeom.End4X),
				End4Y:   min(chromaGeom.Start4Y+offY4+int(txInfo.H4), chromaGeom.End4Y),
			}
			planeX := tu.Start4X * 4
			planeY := tu.Start4Y * 4

			var err error
			if syntax.PalUVSize != 0 {
				err = ReconstructPaletteChromaCoefficients16(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, zero, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalU, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
			} else {
				lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
				tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
					topHasRight:   topHasRight,
					leftHasBottom: leftHasBottom,
				})
				err = ReconstructIntraChromaCoefficients16WithTxTypeFlagsNeighborEdges(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, TxTypeDCTDCT, syntax.UVMode, syntax.UVAngle, syntax.CFLAlpha[0], zero, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
			}
			if err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, left, 1, layout, tu, syntax.UVTX, 0x40); err != nil {
				return err
			}

			err = nil
			if syntax.PalUVSize != 0 {
				err = ReconstructPaletteChromaCoefficients16(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, zero, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalV, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
			} else {
				lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
				tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
					topHasRight:   topHasRight,
					leftHasBottom: leftHasBottom,
				})
				err = ReconstructIntraChromaCoefficients16WithTxTypeFlagsNeighborEdges(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, TxTypeDCTDCT, syntax.UVMode, syntax.UVAngle, syntax.CFLAlpha[1], zero, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
			}
			if err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, left, 2, layout, tu, syntax.UVTX, 0x40); err != nil {
				return err
			}
		}
	}
	return nil
}
