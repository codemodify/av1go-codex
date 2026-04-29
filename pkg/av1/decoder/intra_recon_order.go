package decoder

import (
	"fmt"
	"os"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func shouldUseDav1dIntraCoeffOrder(seq av1.SequenceHeader, syntax IntraBlockSyntax) bool {
	if os.Getenv("DEBUG_DISABLE_DAV1D_INTRA_ORDER") != "" {
		return false
	}
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return false
	}
	if seq.ColorConfig.ChromaSubsampling() != av1.Chroma420 {
		return false
	}
	if syntax.UVMode == IntraPredCFL {
		return false
	}
	return syntax.Geometry.Width4() > 16 || syntax.Geometry.Height4() > 16
}

func reconstructParsedIntraBlockAt8Dav1dOrder(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, yPlane []byte, yStride, yWidth, yHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft bool, edge intraEdgeAvailability) (bool, error) {
	if hdr == nil {
		return false, fmt.Errorf("decoder: frame header required")
	}
	if modeCDF == nil || coefCDF == nil {
		return false, fmt.Errorf("decoder: cdf state required")
	}
	if dec == nil {
		return false, fmt.Errorf("decoder: entropy decoder required")
	}

	g := syntax.Geometry
	blockDims := BlockDims(syntax.Size)
	w4 := minInt(g.Width4(), int(blockDims.W4))
	h4 := minInt(g.Height4(), int(blockDims.H4))
	if w4 <= 16 && h4 <= 16 {
		return false, fmt.Errorf("decoder: dav1d-order path is only needed for large intra blocks")
	}

	lumaScratch := takeTransformUnitScratch(g)
	defer putTransformUnitScratch(lumaScratch)
	lumaUnits, err := ReadIntraTransformUnitsInto(lumaScratch.buf[:0], hdr, modeCDF, above, left, g, syntax.TX, syntax.Segment, dec)
	if err != nil {
		return false, err
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	hasChroma := hasChromaPlane(seq, syntax.Size, g.Start4X, g.Start4Y)
	chromaGeom := BlockGeometry{}
	chromaUnits := []BlockGeometry(nil)
	var chromaScratch *scratchBuffer[BlockGeometry]
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	if hasChroma {
		chromaGeom = chromaGeometryForBlock(layout, g)
		countX, countY := transformGridCounts(chromaGeom, syntax.UVTX)
		chromaScratch = takeScratch[BlockGeometry](&blockGeometryScratchPool, countX*countY)
		defer putScratch(&blockGeometryScratchPool, chromaScratch)
		chromaUnits = buildChromaTransformUnitsInto(chromaScratch.buf[:0], layout, g, syntax.UVTX)
	}

	yPredictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	uvPredictFlags := intraChromaPredictFlags(seq, above, left, syntax)
	yBlockX := g.Start4X * 4
	yBlockY := g.Start4Y * 4
	paletteIndexStride := 0
	uvBlockX := 0
	uvBlockY := 0
	if hasChroma {
		paletteIndexStride, _ = chromaPaletteIndexDimensions(layout, g)
		uvBlockX = chromaGeom.Start4X * 4
		uvBlockY = chromaGeom.Start4Y * 4
	}

	ssHor := 1
	ssVer := 1
	if layout == av1.Chroma444 {
		ssHor, ssVer = 0, 0
	} else if layout == av1.Chroma422 {
		ssHor, ssVer = 1, 0
	}

	hasResidual := false
	chromaEdge := edge.forChromaLayout(layout)
	for initY4 := 0; initY4 < h4; initY4 += 16 {
		subH4 := minInt(h4, initY4+16)
		for initX4 := 0; initX4 < w4; initX4 += 16 {
			subW4 := minInt(w4, initX4+16)
			for _, unit := range lumaUnits {
				offX4 := unit.Global.Start4X - g.Start4X
				offY4 := unit.Global.Start4Y - g.Start4Y
				if offX4 < initX4 || offX4 >= subW4 || offY4 < initY4 || offY4 >= subH4 {
					continue
				}
				residual, err := reconstructParsedIntraLumaTU8(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, unit, yPlane, yStride, yWidth, yHeight, g.Start4X, g.Start4Y, dec, haveTop, haveLeft, edge.topHasRight, edge.leftHasBottom, yPredictFlags, w4, h4, yBlockX, yBlockY)
				if err != nil {
					return false, err
				}
				hasResidual = hasResidual || residual
			}

			if !hasChroma {
				continue
			}
			chromaStartX4 := initX4 >> ssHor
			chromaStartY4 := initY4 >> ssVer
			chromaEndX4 := minInt(chromaGeom.Width4(), (initX4+16)>>ssHor)
			chromaEndY4 := minInt(chromaGeom.Height4(), (initY4+16)>>ssVer)
			for plane := 1; plane <= 2; plane++ {
				for _, tu := range chromaUnits {
					offX4 := tu.Start4X - chromaGeom.Start4X
					offY4 := tu.Start4Y - chromaGeom.Start4Y
					if offX4 < chromaStartX4 || offX4 >= chromaEndX4 || offY4 < chromaStartY4 || offY4 >= chromaEndY4 {
						continue
					}
					if err := reconstructParsedIntraChromaTU8(seq, hdr, coefCDF, dq, above, left, syntax, tu, plane, chromaGeom, paletteIndexStride, uvTxType, uvPredictFlags, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, yPlane, yStride, yWidth, yHeight, dec, haveTop, haveLeft, chromaEdge.topHasRight, chromaEdge.leftHasBottom, uvBlockX, uvBlockY); err != nil {
						return false, err
					}
				}
			}
		}
	}
	return hasResidual, nil
}

func reconstructParsedIntraLumaTU8(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, unit TransformUnit, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool, predictFlags uint16, blockWidth4, blockHeight4, blockX, blockY int) (bool, error) {
	tu := unit.Global
	tx := unit.TX
	txInfo := TxfmInfoFor(tx)
	offX4 := tu.Start4X - syntax.Geometry.Start4X
	offY4 := tu.Start4Y - syntax.Geometry.Start4Y
	tuHaveTop := haveTop || offY4 > 0
	tuHaveLeft := haveLeft || offX4 > 0
	bx4 := tu.Start4X & 31
	by4 := tu.Start4Y & 31
	skipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)

	coeffs := takeCoeffScratch(tx)
	defer putCoeffScratch(coeffs)

	currentSkipBit := dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:])
	yTxType := TxTypeDCTDCT
	resCtx := uint8(0x40)
	skip := currentSkipBit != 0
	var err error
	if !skip {
		yTxType, err = ReadIntraTxType(hdr, modeCDF, syntax, tx, tuHaveTop, tuHaveLeft, dec)
		if err != nil {
			return false, err
		}
		resCtx, skip, err = DecodeLumaCoefficientsNoSkipInto(coeffs.buf, syntax.Size, tu, tx, yTxType, syntax.Segment, coefCDF, dq, above, left, dec)
		if err != nil {
			return false, err
		}
	} else if int(syntax.Segment) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[syntax.Segment] {
		yTxType = TxTypeWHTWHT
	}

	px := (planeX4 + offX4) * 4
	py := (planeY4 + offY4) * 4
	if syntax.PalYSize != 0 {
		err = ReconstructPaletteLumaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, px, py, tx, yTxType, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
	} else {
		tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, tx, intraEdgeAvailability{
			topHasRight:   topHasRight,
			leftHasBottom: leftHasBottom,
		})
		err = ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, px, py, tx, yTxType, syntax.YMode, syntax.YAngle, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], predictFlags, tuHaveTop, tuHaveLeft, tuEdge.topHasRight, tuEdge.leftHasBottom)
	}
	if err != nil {
		return false, err
	}
	if err := ApplyLumaResidualContext(above, left, tu, tx, resCtx); err != nil {
		return false, err
	}
	return !skip, nil
}

func reconstructParsedIntraChromaTU8(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, tu BlockGeometry, plane int, chromaGeom BlockGeometry, paletteIndexStride int, uvTxType TxType, predictFlags uint16, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, luma []byte, lumaStride, lumaWidth, lumaHeight int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool, blockX, blockY int) error {
	layout := seq.ColorConfig.ChromaSubsampling()
	planeX := tu.Start4X * 4
	planeY := tu.Start4Y * 4
	offX4 := tu.Start4X - chromaGeom.Start4X
	offY4 := tu.Start4Y - chromaGeom.Start4Y
	tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
		topHasRight:   topHasRight,
		leftHasBottom: leftHasBottom,
	})
	coeffs := takeCoeffScratch(syntax.UVTX)
	defer putCoeffScratch(coeffs)
	resCtx, _, err := DecodeChromaCoefficientsInto(coeffs.buf, syntax.Size, tu, syntax.UVTX, uvTxType, syntax.Segment, plane, layout, coefCDF, dq, above, left, dec)
	if err != nil {
		return err
	}
	if syntax.PalUVSize != 0 {
		if plane == 1 {
			err = ReconstructPaletteChromaCoefficients8WithTxType(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalU, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
		} else {
			err = ReconstructPaletteChromaCoefficients8WithTxType(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalV, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
		}
	} else {
		lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
		targetPlane := uPlane
		targetStride := uStride
		targetWidth := uWidth
		targetHeight := uHeight
		cflAlpha := syntax.CFLAlpha[0]
		if plane == 2 {
			targetPlane = vPlane
			targetStride = vStride
			targetWidth = vWidth
			targetHeight = vHeight
			cflAlpha = syntax.CFLAlpha[1]
		}
		err = ReconstructIntraChromaCoefficients8WithTxTypeFlagsNeighborEdges(targetPlane, targetStride, targetWidth, targetHeight, planeX, planeY, syntax.UVTX, uvTxType, syntax.UVMode, syntax.UVAngle, cflAlpha, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
	}
	if err != nil {
		return err
	}
	return ApplyChromaResidualContextWithLayout(above, left, plane, layout, tu, syntax.UVTX, resCtx)
}

func reconstructParsedIntraBlockAt16Dav1dOrder(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, yPlane []uint16, yStride, yWidth, yHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft bool, edge intraEdgeAvailability) (bool, error) {
	if hdr == nil {
		return false, fmt.Errorf("decoder: frame header required")
	}
	if modeCDF == nil || coefCDF == nil {
		return false, fmt.Errorf("decoder: cdf state required")
	}
	if dec == nil {
		return false, fmt.Errorf("decoder: entropy decoder required")
	}

	g := syntax.Geometry
	blockDims := BlockDims(syntax.Size)
	w4 := minInt(g.Width4(), int(blockDims.W4))
	h4 := minInt(g.Height4(), int(blockDims.H4))
	if w4 <= 16 && h4 <= 16 {
		return false, fmt.Errorf("decoder: dav1d-order path is only needed for large intra blocks")
	}

	lumaScratch := takeTransformUnitScratch(g)
	defer putTransformUnitScratch(lumaScratch)
	lumaUnits, err := ReadIntraTransformUnitsInto(lumaScratch.buf[:0], hdr, modeCDF, above, left, g, syntax.TX, syntax.Segment, dec)
	if err != nil {
		return false, err
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	hasChroma := hasChromaPlane(seq, syntax.Size, g.Start4X, g.Start4Y)
	chromaGeom := BlockGeometry{}
	chromaUnits := []BlockGeometry(nil)
	var chromaScratch *scratchBuffer[BlockGeometry]
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	if hasChroma {
		chromaGeom = chromaGeometryForBlock(layout, g)
		countX, countY := transformGridCounts(chromaGeom, syntax.UVTX)
		chromaScratch = takeScratch[BlockGeometry](&blockGeometryScratchPool, countX*countY)
		defer putScratch(&blockGeometryScratchPool, chromaScratch)
		chromaUnits = buildChromaTransformUnitsInto(chromaScratch.buf[:0], layout, g, syntax.UVTX)
	}

	yPredictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	uvPredictFlags := intraChromaPredictFlags(seq, above, left, syntax)
	yBlockX := g.Start4X * 4
	yBlockY := g.Start4Y * 4
	paletteIndexStride := 0
	uvBlockX := 0
	uvBlockY := 0
	if hasChroma {
		paletteIndexStride, _ = chromaPaletteIndexDimensions(layout, g)
		uvBlockX = chromaGeom.Start4X * 4
		uvBlockY = chromaGeom.Start4Y * 4
	}

	ssHor := 1
	ssVer := 1
	if layout == av1.Chroma444 {
		ssHor, ssVer = 0, 0
	} else if layout == av1.Chroma422 {
		ssHor, ssVer = 1, 0
	}

	hasResidual := false
	chromaEdge := edge.forChromaLayout(layout)
	for initY4 := 0; initY4 < h4; initY4 += 16 {
		subH4 := minInt(h4, initY4+16)
		for initX4 := 0; initX4 < w4; initX4 += 16 {
			subW4 := minInt(w4, initX4+16)
			for _, unit := range lumaUnits {
				offX4 := unit.Global.Start4X - g.Start4X
				offY4 := unit.Global.Start4Y - g.Start4Y
				if offX4 < initX4 || offX4 >= subW4 || offY4 < initY4 || offY4 >= subH4 {
					continue
				}
				residual, err := reconstructParsedIntraLumaTU16(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, unit, yPlane, yStride, yWidth, yHeight, g.Start4X, g.Start4Y, bitDepth, dec, haveTop, haveLeft, edge.topHasRight, edge.leftHasBottom, yPredictFlags, w4, h4, yBlockX, yBlockY)
				if err != nil {
					return false, err
				}
				hasResidual = hasResidual || residual
			}

			if !hasChroma {
				continue
			}
			chromaStartX4 := initX4 >> ssHor
			chromaStartY4 := initY4 >> ssVer
			chromaEndX4 := minInt(chromaGeom.Width4(), (initX4+16)>>ssHor)
			chromaEndY4 := minInt(chromaGeom.Height4(), (initY4+16)>>ssVer)
			for plane := 1; plane <= 2; plane++ {
				for _, tu := range chromaUnits {
					offX4 := tu.Start4X - chromaGeom.Start4X
					offY4 := tu.Start4Y - chromaGeom.Start4Y
					if offX4 < chromaStartX4 || offX4 >= chromaEndX4 || offY4 < chromaStartY4 || offY4 >= chromaEndY4 {
						continue
					}
					if err := reconstructParsedIntraChromaTU16(seq, hdr, coefCDF, dq, above, left, syntax, tu, plane, chromaGeom, paletteIndexStride, uvTxType, uvPredictFlags, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, yPlane, yStride, yWidth, yHeight, bitDepth, dec, haveTop, haveLeft, chromaEdge.topHasRight, chromaEdge.leftHasBottom, uvBlockX, uvBlockY); err != nil {
						return false, err
					}
				}
			}
		}
	}
	return hasResidual, nil
}

func reconstructParsedIntraLumaTU16(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, unit TransformUnit, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool, predictFlags uint16, blockWidth4, blockHeight4, blockX, blockY int) (bool, error) {
	tu := unit.Global
	tx := unit.TX
	txInfo := TxfmInfoFor(tx)
	offX4 := tu.Start4X - syntax.Geometry.Start4X
	offY4 := tu.Start4Y - syntax.Geometry.Start4Y
	tuHaveTop := haveTop || offY4 > 0
	tuHaveLeft := haveLeft || offX4 > 0
	bx4 := tu.Start4X & 31
	by4 := tu.Start4Y & 31
	skipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)

	coeffs := takeCoeffScratch(tx)
	defer putCoeffScratch(coeffs)

	currentSkipBit := dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:])
	yTxType := TxTypeDCTDCT
	resCtx := uint8(0x40)
	skip := currentSkipBit != 0
	var err error
	if !skip {
		yTxType, err = ReadIntraTxType(hdr, modeCDF, syntax, tx, tuHaveTop, tuHaveLeft, dec)
		if err != nil {
			return false, err
		}
		resCtx, skip, err = DecodeLumaCoefficientsNoSkipIntoWithBitDepth(coeffs.buf, syntax.Size, tu, tx, yTxType, syntax.Segment, coefCDF, dq, above, left, dec, bitDepth)
		if err != nil {
			return false, err
		}
	} else if int(syntax.Segment) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[syntax.Segment] {
		yTxType = TxTypeWHTWHT
	}

	px := (planeX4 + offX4) * 4
	py := (planeY4 + offY4) * 4
	if syntax.PalYSize != 0 {
		err = ReconstructPaletteLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, px, py, tx, yTxType, coeffs.buf, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
	} else {
		tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, tx, intraEdgeAvailability{
			topHasRight:   topHasRight,
			leftHasBottom: leftHasBottom,
		})
		err = ReconstructIntraLumaCoefficients16WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, px, py, tx, yTxType, syntax.YMode, syntax.YAngle, coeffs.buf, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], predictFlags, tuHaveTop, tuHaveLeft, tuEdge.topHasRight, tuEdge.leftHasBottom)
	}
	if err != nil {
		return false, err
	}
	if err := ApplyLumaResidualContext(above, left, tu, tx, resCtx); err != nil {
		return false, err
	}
	return !skip, nil
}

func reconstructParsedIntraChromaTU16(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, tu BlockGeometry, plane int, chromaGeom BlockGeometry, paletteIndexStride int, uvTxType TxType, predictFlags uint16, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, luma []uint16, lumaStride, lumaWidth, lumaHeight int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool, blockX, blockY int) error {
	layout := seq.ColorConfig.ChromaSubsampling()
	planeX := tu.Start4X * 4
	planeY := tu.Start4Y * 4
	offX4 := tu.Start4X - chromaGeom.Start4X
	offY4 := tu.Start4Y - chromaGeom.Start4Y
	tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
		topHasRight:   topHasRight,
		leftHasBottom: leftHasBottom,
	})
	coeffs := takeCoeffScratch(syntax.UVTX)
	defer putCoeffScratch(coeffs)
	resCtx, _, err := DecodeChromaCoefficientsIntoWithBitDepth(coeffs.buf, syntax.Size, tu, syntax.UVTX, uvTxType, syntax.Segment, plane, layout, coefCDF, dq, above, left, dec, bitDepth)
	if err != nil {
		return err
	}
	if syntax.PalUVSize != 0 {
		if plane == 1 {
			err = ReconstructPaletteChromaCoefficients16WithTxType(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs.buf, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalU, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
		} else {
			err = ReconstructPaletteChromaCoefficients16WithTxType(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs.buf, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalV, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
		}
	} else {
		lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
		targetPlane := uPlane
		targetStride := uStride
		targetWidth := uWidth
		targetHeight := uHeight
		cflAlpha := syntax.CFLAlpha[0]
		if plane == 2 {
			targetPlane = vPlane
			targetStride = vStride
			targetWidth = vWidth
			targetHeight = vHeight
			cflAlpha = syntax.CFLAlpha[1]
		}
		err = ReconstructIntraChromaCoefficients16WithTxTypeFlagsNeighborEdges(targetPlane, targetStride, targetWidth, targetHeight, planeX, planeY, syntax.UVTX, uvTxType, syntax.UVMode, syntax.UVAngle, cflAlpha, coeffs.buf, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
	}
	if err != nil {
		return err
	}
	return ApplyChromaResidualContextWithLayout(above, left, plane, layout, tu, syntax.UVTX, resCtx)
}
