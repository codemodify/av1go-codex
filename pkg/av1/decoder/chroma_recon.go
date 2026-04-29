package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

const cflLocalACScratchLen = 512

var cflIntScratchPool sync.Pool

type debugChromaSkipPolicy uint8

const (
	debugChromaSkipPolicyNone debugChromaSkipPolicy = iota
	debugChromaSkipPolicyResetAll
	debugChromaSkipPolicyResetAllProb
	debugChromaSkipPolicyResetAllCount
	debugChromaSkipPolicyResetEntry
	debugChromaSkipPolicyResetEntryProb
	debugChromaSkipPolicyResetEntryCount
	debugChromaSkipPolicyCopySource
	debugChromaSkipPolicyCopySourceProb
	debugChromaSkipPolicyCopyQCatProb
)

func debugChromaSkipPolicyForBlock(g BlockGeometry, plane int) debugChromaSkipPolicy {
	if !debugChromaSkipPlaneEnabled(plane) {
		return debugChromaSkipPolicyNone
	}
	target := strings.TrimSpace(os.Getenv("DEBUG_CHROMA_SKIP_TARGET"))
	if target == "" {
		return debugChromaSkipPolicyNone
	}
	parts := strings.Split(target, ",")
	if len(parts) < 2 {
		return debugChromaSkipPolicyNone
	}
	wantX, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	wantY, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return debugChromaSkipPolicyNone
	}
	if g.Start4X*4 != wantX || g.Start4Y*4 != wantY {
		return debugChromaSkipPolicyNone
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_CHROMA_SKIP_POLICY"))) {
	case "reset_all", "all":
		return debugChromaSkipPolicyResetAll
	case "reset_all_prob", "all_prob":
		return debugChromaSkipPolicyResetAllProb
	case "reset_all_count", "all_count":
		return debugChromaSkipPolicyResetAllCount
	case "reset_entry", "entry":
		return debugChromaSkipPolicyResetEntry
	case "reset_entry_prob", "entry_prob":
		return debugChromaSkipPolicyResetEntryProb
	case "reset_entry_count", "entry_count":
		return debugChromaSkipPolicyResetEntryCount
	case "copy_source", "source":
		return debugChromaSkipPolicyCopySource
	case "copy_source_prob", "source_prob":
		return debugChromaSkipPolicyCopySourceProb
	case "copy_qcat_prob", "qcat_prob":
		return debugChromaSkipPolicyCopyQCatProb
	default:
		return debugChromaSkipPolicyNone
	}
}

func debugSwapChromaPlaneOrderForBlock(g BlockGeometry) bool {
	target := strings.TrimSpace(os.Getenv("DEBUG_SWAP_CHROMA_PLANE_ORDER_TARGET"))
	if target == "" {
		return false
	}
	parts := strings.Split(target, ",")
	if len(parts) < 2 {
		return false
	}
	wantX, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	wantY, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return false
	}
	return g.Start4X*4 == wantX && g.Start4Y*4 == wantY
}

func debugResetChromaCoefBeforePlane2ForBlock(g BlockGeometry) bool {
	target := strings.TrimSpace(os.Getenv("DEBUG_RESET_CHROMA_COEF_BEFORE_PLANE2_TARGET"))
	if target == "" {
		return false
	}
	parts := strings.Split(target, ",")
	if len(parts) < 2 {
		return false
	}
	wantX, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	wantY, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return false
	}
	return g.Start4X*4 == wantX && g.Start4Y*4 == wantY
}

func debugChromaSkipPlaneEnabled(plane int) bool {
	spec := strings.TrimSpace(os.Getenv("DEBUG_CHROMA_SKIP_PLANES"))
	if spec == "" {
		return plane == 2
	}
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		v, err := strconv.Atoi(item)
		if err == nil && v == plane {
			return true
		}
	}
	return false
}

func debugChromaSkipSourceEntry() (int, int, bool) {
	spec := strings.TrimSpace(os.Getenv("DEBUG_CHROMA_SKIP_SOURCE"))
	if spec == "" {
		return 0, 0, false
	}
	parts := strings.Split(spec, ",")
	if len(parts) < 2 {
		return 0, 0, false
	}
	row, errRow := strconv.Atoi(strings.TrimSpace(parts[0]))
	ctx, errCtx := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errRow != nil || errCtx != nil {
		return 0, 0, false
	}
	return row, ctx, true
}

func debugChromaSkipSourceQCat() (int, bool) {
	spec := strings.TrimSpace(os.Getenv("DEBUG_CHROMA_SKIP_SOURCE_QCAT"))
	if spec == "" {
		return 0, false
	}
	qcat, err := strconv.Atoi(spec)
	if err != nil {
		return 0, false
	}
	return clampDefaultCoefQCat(qcat), true
}

func debugCopySkipCDFAll(dst, src *CoefCDF, copyProb, copyCount bool) {
	if dst == nil || src == nil {
		return
	}
	for row := range dst.Skip {
		for ctx := range dst.Skip[row] {
			if copyProb {
				dst.Skip[row][ctx][0] = src.Skip[row][ctx][0]
			}
			if copyCount {
				dst.Skip[row][ctx][1] = src.Skip[row][ctx][1]
			}
		}
	}
}

func debugCopySkipCDFEntry(dst, src *CoefCDF, row, ctx int, copyProb, copyCount bool) {
	if dst == nil || src == nil {
		return
	}
	if row < 0 || row >= len(dst.Skip) || ctx < 0 || ctx >= len(dst.Skip[row]) {
		return
	}
	if copyProb {
		dst.Skip[row][ctx][0] = src.Skip[row][ctx][0]
	}
	if copyCount {
		dst.Skip[row][ctx][1] = src.Skip[row][ctx][1]
	}
}

func debugCopySkipCDFSource(dst *CoefCDF, dstRow, dstCtx int, src *CoefCDF, srcRow, srcCtx int, copyProb, copyCount bool) {
	if dst == nil || src == nil {
		return
	}
	if dstRow < 0 || dstRow >= len(dst.Skip) || dstCtx < 0 || dstCtx >= len(dst.Skip[dstRow]) {
		return
	}
	if srcRow < 0 || srcRow >= len(src.Skip) || srcCtx < 0 || srcCtx >= len(src.Skip[srcRow]) {
		return
	}
	if copyProb {
		dst.Skip[dstRow][dstCtx][0] = src.Skip[srcRow][srcCtx][0]
	}
	if copyCount {
		dst.Skip[dstRow][dstCtx][1] = src.Skip[srcRow][srcCtx][1]
	}
}

func debugApplyChromaSkipPolicy(policy debugChromaSkipPolicy, coefCDF, baseline *CoefCDF, skipRow, skipCtx int) {
	switch policy {
	case debugChromaSkipPolicyResetAll:
		debugCopySkipCDFAll(coefCDF, baseline, true, true)
	case debugChromaSkipPolicyResetAllProb:
		debugCopySkipCDFAll(coefCDF, baseline, true, false)
	case debugChromaSkipPolicyResetAllCount:
		debugCopySkipCDFAll(coefCDF, baseline, false, true)
	case debugChromaSkipPolicyResetEntry:
		debugCopySkipCDFEntry(coefCDF, baseline, skipRow, skipCtx, true, true)
	case debugChromaSkipPolicyResetEntryProb:
		debugCopySkipCDFEntry(coefCDF, baseline, skipRow, skipCtx, true, false)
	case debugChromaSkipPolicyResetEntryCount:
		debugCopySkipCDFEntry(coefCDF, baseline, skipRow, skipCtx, false, true)
	case debugChromaSkipPolicyCopySource, debugChromaSkipPolicyCopySourceProb:
		srcRow, srcCtx, ok := debugChromaSkipSourceEntry()
		if !ok {
			return
		}
		copyProb := policy == debugChromaSkipPolicyCopySource || policy == debugChromaSkipPolicyCopySourceProb
		copyCount := policy == debugChromaSkipPolicyCopySource
		debugCopySkipCDFSource(coefCDF, skipRow, skipCtx, coefCDF, srcRow, srcCtx, copyProb, copyCount)
	case debugChromaSkipPolicyCopyQCatProb:
		qcat, ok := debugChromaSkipSourceQCat()
		if !ok {
			return
		}
		src := NewDefaultCoefCDFForQCat(qcat)
		debugCopySkipCDFSource(coefCDF, skipRow, skipCtx, src, skipRow, skipCtx, true, false)
	}
}

func chromaGeometryForBlock(layout av1.ChromaSubsampling, g BlockGeometry) BlockGeometry {
	ssHor := 0
	ssVer := 0
	if layout != av1.Chroma444 {
		ssHor = 1
	}
	if layout == av1.Chroma420 {
		ssVer = 1
	}
	width4 := g.Width4()
	height4 := g.Height4()
	cw4 := width4
	ch4 := height4
	if ssHor != 0 {
		cw4 = (cw4 + 1) >> 1
	}
	if ssVer != 0 {
		ch4 = (ch4 + 1) >> 1
	}
	start4X := g.Start4X >> ssHor
	start4Y := g.Start4Y >> ssVer
	return BlockGeometry{
		Level:   g.Level,
		Size:    g.Size,
		Start4X: start4X,
		Start4Y: start4Y,
		End4X:   start4X + cw4,
		End4Y:   start4Y + ch4,
	}
}

func chromaSampleStep(layout av1.ChromaSubsampling) (int, int) {
	switch layout {
	case av1.Chroma400:
		return 0, 0
	case av1.Chroma444:
		return 4, 4
	case av1.Chroma422:
		return 2, 4
	default:
		return 2, 2
	}
}

func chromaSampleRectForBlock(layout av1.ChromaSubsampling, g BlockGeometry) (int, int, int, int) {
	stepX, stepY := chromaSampleStep(layout)
	return g.Start4X * stepX, g.Start4Y * stepY, g.Width4() * stepX, g.Height4() * stepY
}

func reconstructParsedIntraChromaBlockAt8(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []byte, lumaStride, lumaWidth, lumaHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, dec keyIntraBlockEntropyDecoder) error {
	return reconstructParsedIntraChromaBlockAt8Edges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, dec, true, true)
}

func reconstructParsedIntraChromaBlockAt8Edges(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []byte, lumaStride, lumaWidth, lumaHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, dec keyIntraBlockEntropyDecoder, topHasRight, leftHasBottom bool) error {
	return reconstructParsedIntraChromaBlockAt8NeighborEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, topHasRight, leftHasBottom)
}

func reconstructParsedIntraChromaBlockAt8NeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []byte, lumaStride, lumaWidth, lumaHeight int, uPlane []byte, uStride, uWidth, uHeight int, vPlane []byte, vStride, vWidth, vHeight int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	layout := seq.ColorConfig.ChromaSubsampling()
	chromaGeom := chromaGeometryForBlock(layout, syntax.Geometry)
	paletteIndexStride, _ := chromaPaletteIndexDimensions(layout, syntax.Geometry)
	blockX := chromaGeom.Start4X * 4
	blockY := chromaGeom.Start4Y * 4
	coefBeforeChroma := coefCDF.Clone()
	countX, countY := transformGridCounts(chromaGeom, syntax.UVTX)
	var unitsBuf [16]BlockGeometry
	unitsDst, unitsScratch := takeBlockGeometryBuffer(unitsBuf[:0], countX*countY)
	defer putBlockGeometryBuffer(unitsScratch)
	units := buildChromaTransformUnitsInto(unitsDst, layout, syntax.Geometry, syntax.UVTX)
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	predictFlags := intraChromaPredictFlags(seq, above, left, syntax)
	coefEntry := coefCDF.Clone()
	reconstructPlane := func(plane int) error {
		policy := debugChromaSkipPolicyForBlock(syntax.Geometry, plane)
		for _, tu := range units {
			planeX := tu.Start4X * 4
			planeY := tu.Start4Y * 4
			offX4 := tu.Start4X - chromaGeom.Start4X
			offY4 := tu.Start4Y - chromaGeom.Start4Y
			tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
				topHasRight:   topHasRight,
				leftHasBottom: leftHasBottom,
			})
			lumaX, lumaY := chromaToLumaOrigin(layout, planeX, planeY)
			traceChroma := os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != "" && debugTraceMovieBlock(lumaX, lumaY, 4, 4)
			if traceChroma {
				bx4 := tu.Start4X & 31
				by4 := tu.Start4Y & 31
				if state, ok := dec.(interface {
					DebugState() (uint32, uint32, int, int)
				}); ok {
					cur, rng, cnt, pos := state.DebugState()
					fmt.Fprintf(os.Stderr, "trace chroma pre plane=%d owner=(%d,%d)-(%d,%d) tu=(%d,%d)-(%d,%d) uvtx=%d uvtxtype=%d current=(%d,%d,%d,%d) aboveC=%v leftC=%v\n",
						plane,
						syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4, syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
						lumaX, lumaY, lumaX+(tu.End4X-tu.Start4X)*8, lumaY+(tu.End4Y-tu.Start4Y)*8,
						syntax.UVTX, uvTxType,
						cur, rng, cnt, pos,
						append([]uint8(nil), above.CCoef[plane-1][bx4:bx4+minInt(len(above.CCoef[plane-1][bx4:]), 4)]...),
						append([]uint8(nil), left.CCoef[plane-1][by4:by4+minInt(len(left.CCoef[plane-1][by4:]), 4)]...),
					)
				}
			}
			if policy != debugChromaSkipPolicyNone {
				bx4 := tu.Start4X & 31
				by4 := tu.Start4Y & 31
				skipRow := coeffCDFRow(int(TxfmInfoFor(syntax.UVTX).Ctx), "skip", len(coefCDF.Skip)-1)
				skipCtx := GetSkipCtx(TxfmInfoFor(syntax.UVTX), syntax.Size, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], true, layout)
				debugApplyChromaSkipPolicy(policy, coefCDF, coefBeforeChroma, skipRow, skipCtx)
			}
			coeffs := takeCoeffScratch(syntax.UVTX)
			resCtx, skip, err := DecodeChromaCoefficientsInto(coeffs.buf, syntax.Size, tu, syntax.UVTX, uvTxType, syntax.Segment, plane, seq.ColorConfig.ChromaSubsampling(), coefCDF, dq, above, left, dec)
			if traceChroma {
				if state, ok := dec.(interface {
					DebugState() (uint32, uint32, int, int)
				}); ok {
					cur, rng, cnt, pos := state.DebugState()
					fmt.Fprintf(os.Stderr, "trace chroma post plane=%d tu=(%d,%d)-(%d,%d) skip=%v resCtx=%d current=(%d,%d,%d,%d) coeffs=%v\n",
						plane,
						lumaX, lumaY, lumaX+(tu.End4X-tu.Start4X)*8, lumaY+(tu.End4Y-tu.Start4Y)*8,
						skip, resCtx, cur, rng, cnt, pos,
						append([]int32(nil), coeffs.buf[:minInt(len(coeffs.buf), 16)]...),
					)
				}
			}
			if err != nil {
				putCoeffScratch(coeffs)
				return err
			}
			if syntax.PalUVSize != 0 {
				if plane == 1 {
					err = ReconstructPaletteChromaCoefficients8WithTxType(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalU, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
				} else {
					err = ReconstructPaletteChromaCoefficients8WithTxType(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs.buf, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalV, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
				}
			} else {
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
				err = ReconstructIntraChromaCoefficients8WithTxTypeFlagsNeighborEdges(
					targetPlane,
					targetStride,
					targetWidth,
					targetHeight,
					planeX,
					planeY,
					syntax.UVTX,
					uvTxType,
					syntax.UVMode,
					syntax.UVAngle,
					cflAlpha,
					coeffs.buf,
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
				putCoeffScratch(coeffs)
				return err
			}
			putCoeffScratch(coeffs)
			if err := ApplyChromaResidualContextWithLayout(above, left, plane, layout, tu, syntax.UVTX, resCtx); err != nil {
				return err
			}
		}
		return nil
	}
	planes := []int{1, 2}
	if debugSwapChromaPlaneOrderForBlock(syntax.Geometry) {
		planes = []int{2, 1}
	}
	for _, plane := range planes {
		if plane == 2 && debugResetChromaCoefBeforePlane2ForBlock(syntax.Geometry) && coefEntry != nil {
			*coefCDF = *coefEntry
		}
		if err := reconstructPlane(plane); err != nil {
			return err
		}
	}
	return nil
}

func reconstructParsedIntraChromaBlockAt16(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []uint16, lumaStride, lumaWidth, lumaHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, bitDepth uint8, dec keyIntraBlockEntropyDecoder) error {
	return reconstructParsedIntraChromaBlockAt16NeighborEdges(seq, hdr, coefCDF, dq, above, left, syntax, luma, lumaStride, lumaWidth, lumaHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, bitDepth, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, true, true)
}

func reconstructParsedIntraChromaBlockAt16NeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, luma []uint16, lumaStride, lumaWidth, lumaHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	layout := seq.ColorConfig.ChromaSubsampling()
	chromaGeom := chromaGeometryForBlock(layout, syntax.Geometry)
	paletteIndexStride, _ := chromaPaletteIndexDimensions(layout, syntax.Geometry)
	blockX := chromaGeom.Start4X * 4
	blockY := chromaGeom.Start4Y * 4
	coefBeforeChroma := coefCDF.Clone()
	countX, countY := transformGridCounts(chromaGeom, syntax.UVTX)
	var unitsBuf [16]BlockGeometry
	unitsDst, unitsScratch := takeBlockGeometryBuffer(unitsBuf[:0], countX*countY)
	defer putBlockGeometryBuffer(unitsScratch)
	units := buildChromaTransformUnitsInto(unitsDst, layout, syntax.Geometry, syntax.UVTX)
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	predictFlags := intraChromaPredictFlags(seq, above, left, syntax)
	coefEntry := coefCDF.Clone()
	reconstructPlane := func(plane int) error {
		policy := debugChromaSkipPolicyForBlock(syntax.Geometry, plane)
		for _, tu := range units {
			planeX := tu.Start4X * 4
			planeY := tu.Start4Y * 4
			offX4 := tu.Start4X - chromaGeom.Start4X
			offY4 := tu.Start4Y - chromaGeom.Start4Y
			tuEdge := intraTUNodeEdgeAvailability(chromaGeom.Width4(), chromaGeom.Height4(), offX4, offY4, syntax.UVTX, intraEdgeAvailability{
				topHasRight:   topHasRight,
				leftHasBottom: leftHasBottom,
			})
			if policy != debugChromaSkipPolicyNone {
				bx4 := tu.Start4X & 31
				by4 := tu.Start4Y & 31
				skipRow := coeffCDFRow(int(TxfmInfoFor(syntax.UVTX).Ctx), "skip", len(coefCDF.Skip)-1)
				skipCtx := GetSkipCtx(TxfmInfoFor(syntax.UVTX), syntax.Size, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], true, layout)
				debugApplyChromaSkipPolicy(policy, coefCDF, coefBeforeChroma, skipRow, skipCtx)
			}
			coeffs, resCtx, _, err := DecodeChromaCoefficientsWithBitDepth(syntax.Size, tu, syntax.UVTX, uvTxType, syntax.Segment, plane, seq.ColorConfig.ChromaSubsampling(), coefCDF, dq, above, left, dec, bitDepth)
			if err != nil {
				return err
			}
			if syntax.PalUVSize != 0 {
				if plane == 1 {
					err = ReconstructPaletteChromaCoefficients16WithTxType(uPlane, uStride, uWidth, uHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalU, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
				} else {
					err = ReconstructPaletteChromaCoefficients16WithTxType(vPlane, vStride, vWidth, vHeight, planeX, planeY, syntax.UVTX, uvTxType, coeffs, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax.PalV, syntax.PalUVIdx, paletteIndexStride, blockX, blockY)
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
				err = ReconstructIntraChromaCoefficients16WithTxTypeFlagsNeighborEdges(targetPlane, targetStride, targetWidth, targetHeight, planeX, planeY, syntax.UVTX, uvTxType, syntax.UVMode, syntax.UVAngle, cflAlpha, coeffs, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
			}
			if err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, left, plane, layout, tu, syntax.UVTX, resCtx); err != nil {
				return err
			}
		}
		return nil
	}
	planes := []int{1, 2}
	if debugSwapChromaPlaneOrderForBlock(syntax.Geometry) {
		planes = []int{2, 1}
	}
	for _, plane := range planes {
		if plane == 2 && debugResetChromaCoefBeforePlane2ForBlock(syntax.Geometry) && coefEntry != nil {
			*coefCDF = *coefEntry
		}
		if err := reconstructPlane(plane); err != nil {
			return err
		}
	}
	return nil
}

func ReconstructIntraChromaCoefficients8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, lossless bool, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int) error {
	return ReconstructIntraChromaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, mode, angle, cflAlpha, coeffs, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY)
}

func ReconstructIntraChromaCoefficients8WithTxType(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, lossless bool, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int) error {
	return ReconstructIntraChromaCoefficients8WithTxTypeFlags(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, cflAlpha, coeffs, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, 0)
}

func ReconstructIntraChromaCoefficients8WithTxTypeFlags(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, lossless bool, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, flags uint16) error {
	return ReconstructIntraChromaCoefficients8WithTxTypeFlagsEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, cflAlpha, coeffs, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, flags, true, true)
}

func ReconstructIntraChromaCoefficients8WithTxTypeFlagsEdges(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, lossless bool, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, flags uint16, topHasRight, leftHasBottom bool) error {
	return ReconstructIntraChromaCoefficients8WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, cflAlpha, coeffs, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, flags, y > 0, x > 0, topHasRight, leftHasBottom)
}

func ReconstructIntraChromaCoefficients8WithTxTypeFlagsNeighborEdges(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, lossless bool, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, flags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d uv tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	defer writeback.release()
	traceTarget := os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != "" && debugTraceMovieBlock(lumaX, lumaY, 4, 4)
	if mode == IntraPredCFL {
		if err := predictCFL8(dst, dstStride, width, height, top, left, haveTop, haveLeft, cflAlpha, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, inferCFLLayout(planeWidth, planeHeight, lumaWidth, lumaHeight)); err != nil {
			return err
		}
	} else {
		if err := PredictIntraLuma8WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, flags); err != nil {
			return err
		}
	}
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace chroma recon xy=(%d,%d) luma=(%d,%d) before_residual tx=%d txType=%d lossless=%v mode=%d angle=%d cflAlpha=%d topLeft=%d haveTop=%v haveLeft=%v top=%v left=%v pred=%v coeffs=%v\n",
			x, y, lumaX, lumaY, tx, txType, lossless, mode, angle, cflAlpha, topLeft, haveTop, haveLeft,
			append([]byte(nil), top...),
			append([]byte(nil), left...),
			debugTraceRows8(dst, dstStride, width, height),
			append([]int32(nil), coeffs[:minInt(len(coeffs), width*height)]...),
		)
	}
	if err := InverseAddResidualTx8(dst, dstStride, tx, txType, coeffs, lossless); err != nil {
		return fmt.Errorf("decoder: inverse chroma transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace chroma recon xy=(%d,%d) luma=(%d,%d) after_residual out=%v\n",
			x, y, lumaX, lumaY, debugTraceRows8(dst, dstStride, width, height))
	}
	writeback.commit()
	return nil
}

func PredictIntraChromaBlock8WithFlagsNeighborEdges(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int, mode IntraPredMode, angle int8, cflAlpha int8, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, flags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	defer writeback.release()
	if mode == IntraPredCFL {
		if err := predictCFL8(dst, dstStride, width, height, top, left, haveTop, haveLeft, cflAlpha, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, inferCFLLayout(planeWidth, planeHeight, lumaWidth, lumaHeight)); err != nil {
			return err
		}
	} else {
		if err := PredictIntraLuma8WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, flags); err != nil {
			return err
		}
	}
	writeback.commit()
	return nil
}

func allZeroInt32(v []int32) bool {
	for _, x := range v {
		if x != 0 {
			return false
		}
	}
	return true
}

func ReconstructIntraChromaCoefficients16(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, bitDepth uint8, lossless bool, luma []uint16, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int) error {
	return ReconstructIntraChromaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, mode, angle, cflAlpha, coeffs, bitDepth, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY)
}

func ReconstructIntraChromaCoefficients16WithTxType(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, bitDepth uint8, lossless bool, luma []uint16, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int) error {
	return ReconstructIntraChromaCoefficients16WithTxTypeFlags(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, cflAlpha, coeffs, bitDepth, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, 0)
}

func ReconstructIntraChromaCoefficients16WithTxTypeFlags(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, bitDepth uint8, lossless bool, luma []uint16, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, flags uint16) error {
	return ReconstructIntraChromaCoefficients16WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, cflAlpha, coeffs, bitDepth, lossless, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, flags, y > 0, x > 0, true, true)
}

func ReconstructIntraChromaCoefficients16WithTxTypeFlagsNeighborEdges(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, cflAlpha int8, coeffs []int32, bitDepth uint8, lossless bool, luma []uint16, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, flags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d uv tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, commit, err := prepareIntraLumaTarget16EdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, bitDepth, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	if mode == IntraPredCFL {
		if err := predictCFL16(dst, dstStride, width, height, top, left, haveTop, haveLeft, cflAlpha, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, bitDepth, inferCFLLayout(planeWidth, planeHeight, lumaWidth, lumaHeight)); err != nil {
			return err
		}
	} else {
		if err := PredictIntraLuma16WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, bitDepth, flags); err != nil {
			return err
		}
	}
	if err := InverseAddResidualTx16(dst, dstStride, tx, txType, coeffs, bitDepth, lossless); err != nil {
		return fmt.Errorf("decoder: inverse chroma transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	commit()
	return nil
}

func predictCFL8(dst []byte, stride, width, height int, top, left []byte, haveTop, haveLeft bool, alpha int8, luma []byte, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, layout av1.ChromaSubsampling) error {
	if err := PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft); err != nil {
		return err
	}
	if alpha == 0 {
		return nil
	}
	var acLocal [cflLocalACScratchLen]int
	ac, acScratch := takeCFLACScratch(acLocal[:0], width*height)
	defer putCFLACScratch(acScratch)
	ac = deriveCFLACFromLuma8Into(ac, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, width, height, layout)
	for py := 0; py < height; py++ {
		row := dst[py*stride:]
		for px := 0; px < width; px++ {
			diff := int(alpha) * ac[py*width+px]
			row[px] = clipByte(int(row[px]) + cflDelta(diff))
		}
	}
	return nil
}

func predictCFL16(dst []uint16, stride, width, height int, top, left []uint16, haveTop, haveLeft bool, alpha int8, luma []uint16, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY int, bitDepth uint8, layout av1.ChromaSubsampling) error {
	if err := PredictDCLuma16(dst, stride, width, height, top, left, haveTop, haveLeft, bitDepth); err != nil {
		return err
	}
	if alpha == 0 {
		return nil
	}
	var acLocal [cflLocalACScratchLen]int
	ac, acScratch := takeCFLACScratch(acLocal[:0], width*height)
	defer putCFLACScratch(acScratch)
	ac = deriveCFLACFromLuma16Into(ac, luma, lumaStride, lumaWidth, lumaHeight, lumaX, lumaY, width, height, layout)
	for py := 0; py < height; py++ {
		row := dst[py*stride:]
		for px := 0; px < width; px++ {
			diff := int(alpha) * ac[py*width+px]
			row[px] = clipSample(int(row[px])+cflDelta(diff), bitDepth)
		}
	}
	return nil
}

func takeCFLACScratch(local []int, n int) ([]int, *scratchBuffer[int]) {
	if cap(local) >= n {
		return local[:n], nil
	}
	scratch := takeScratch[int](&cflIntScratchPool, n)
	return scratch.buf[:n], scratch
}

func putCFLACScratch(scratch *scratchBuffer[int]) {
	putScratch(&cflIntScratchPool, scratch)
}

func deriveCFLACFromLuma8Into(ac []int, luma []byte, stride, width, height, x, y, cw, ch int, layout av1.ChromaSubsampling) []int {
	ac = ac[:cw*ch]
	sum := 0
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			lx, ly := cflLumaCoords(layout, x, y, cx, cy)
			s, shift := cflLumaSum8(layout, luma, stride, width, height, lx, ly)
			v := s << shift
			ac[cy*cw+cx] = v
			sum += v
		}
	}
	avg := (sum + (cw*ch)/2) / maxIntLocal(cw*ch, 1)
	for i := range ac {
		ac[i] -= avg
	}
	return ac
}

func deriveCFLACFromLuma16Into(ac []int, luma []uint16, stride, width, height, x, y, cw, ch int, layout av1.ChromaSubsampling) []int {
	ac = ac[:cw*ch]
	sum := 0
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			lx, ly := cflLumaCoords(layout, x, y, cx, cy)
			s, shift := cflLumaSum16(layout, luma, stride, width, height, lx, ly)
			v := s << shift
			ac[cy*cw+cx] = v
			sum += v
		}
	}
	avg := (sum + (cw*ch)/2) / maxIntLocal(cw*ch, 1)
	for i := range ac {
		ac[i] -= avg
	}
	return ac
}

func chromaToLumaOrigin(layout av1.ChromaSubsampling, x, y int) (int, int) {
	switch layout {
	case av1.Chroma444:
		return x, y
	case av1.Chroma422:
		return x * 2, y
	default:
		return x * 2, y * 2
	}
}

func inferCFLLayout(chromaWidth, chromaHeight, lumaWidth, lumaHeight int) av1.ChromaSubsampling {
	switch {
	case lumaWidth == chromaWidth && lumaHeight == chromaHeight:
		return av1.Chroma444
	case lumaWidth == chromaWidth*2 && lumaHeight == chromaHeight:
		return av1.Chroma422
	default:
		return av1.Chroma420
	}
}

func cflLumaCoords(layout av1.ChromaSubsampling, x, y, cx, cy int) (int, int) {
	switch layout {
	case av1.Chroma444:
		return x + cx, y + cy
	case av1.Chroma422:
		return x + cx*2, y + cy
	default:
		return x + cx*2, y + cy*2
	}
}

func cflLumaSum8(layout av1.ChromaSubsampling, luma []byte, stride, width, height, x, y int) (int, int) {
	switch layout {
	case av1.Chroma444:
		return int(samplePlane8(luma, stride, width, height, x, y)), 3
	case av1.Chroma422:
		return int(samplePlane8(luma, stride, width, height, x, y)) +
			int(samplePlane8(luma, stride, width, height, x+1, y)), 2
	default:
		return int(samplePlane8(luma, stride, width, height, x, y)) +
			int(samplePlane8(luma, stride, width, height, x+1, y)) +
			int(samplePlane8(luma, stride, width, height, x, y+1)) +
			int(samplePlane8(luma, stride, width, height, x+1, y+1)), 1
	}
}

func cflLumaSum16(layout av1.ChromaSubsampling, luma []uint16, stride, width, height, x, y int) (int, int) {
	switch layout {
	case av1.Chroma444:
		return int(samplePlane16(luma, stride, width, height, x, y)), 3
	case av1.Chroma422:
		return int(samplePlane16(luma, stride, width, height, x, y)) +
			int(samplePlane16(luma, stride, width, height, x+1, y)), 2
	default:
		return int(samplePlane16(luma, stride, width, height, x, y)) +
			int(samplePlane16(luma, stride, width, height, x+1, y)) +
			int(samplePlane16(luma, stride, width, height, x, y+1)) +
			int(samplePlane16(luma, stride, width, height, x+1, y+1)), 1
	}
}

func cflDelta(diff int) int {
	if diff < 0 {
		return -(((-diff) + 32) >> 6)
	}
	return (diff + 32) >> 6
}

func samplePlane8(plane []byte, stride, width, height, x, y int) byte {
	if x < 0 {
		x = 0
	} else if x >= width {
		x = width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= height {
		y = height - 1
	}
	return plane[y*stride+x]
}

func samplePlane16(plane []uint16, stride, width, height, x, y int) uint16 {
	if x < 0 {
		x = 0
	} else if x >= width {
		x = width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= height {
		y = height - 1
	}
	return plane[y*stride+x]
}
