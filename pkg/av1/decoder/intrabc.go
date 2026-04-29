package decoder

import (
	"fmt"
	"image"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type ScreenContentBlockSyntax struct {
	Geometry BlockGeometry
	Size     BlockSize
	Segment  uint8
	SegPred  bool
	SkipMode bool
	Skip     bool
	Intra    bool
	IntraBlk *IntraBlockSyntax
}

func ReadScreenContentBlockSyntax(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, segID uint8, dec intraEntropyDecoder) (ScreenContentBlockSyntax, error) {
	if hdr == nil {
		return ScreenContentBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if !hdr.AllowIntrabc {
		return ScreenContentBlockSyntax{}, fmt.Errorf("decoder: screen-content block syntax requires allow_intrabc")
	}
	if cdf == nil {
		return ScreenContentBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return ScreenContentBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return ScreenContentBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := ScreenContentBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
	}
	seg := segmentDataForID(hdr, segID)
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	dims := BlockDims(g.Size)
	if hdr.SkipModeEnabled && minInt(int(dims.W4), int(dims.H4)) > 1 {
		smctx := int(above.SkipMode[bx4]) + int(left.SkipMode[by4])
		syntax.SkipMode = dec.DecodeBoolAdapt(cdf.SkipMode[smctx][:]) != 0
	}
	if syntax.SkipMode {
		syntax.Skip = true
		syntax.Intra = false
		return syntax, nil
	}

	if seg.Skip {
		syntax.Skip = true
	} else {
		sctx := int(above.Skip[bx4]) + int(left.Skip[by4])
		syntax.Skip = dec.DecodeBoolAdapt(cdf.Skip[sctx][:]) != 0
	}
	syntax.Intra = dec.DecodeBoolAdapt(cdf.IntraBC[:]) == 0
	if !syntax.Intra {
		return syntax, nil
	}

	intra, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, g.Start4Y > 0, g.Start4X > 0, segID, dec)
	if err != nil {
		return ScreenContentBlockSyntax{}, err
	}
	intra.SegPred = syntax.SegPred
	intra.SkipMode = syntax.SkipMode
	intra.Skip = syntax.Skip
	syntax.IntraBlk = &intra
	return syntax, nil
}

func decodeKeyframeMain8WithIntraBC(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, field *SpatialMVTile) error {
	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, hdr, tile, 0)
		if err != nil {
			return err
		}
		defer syntax.Close()
		mvTile := field.tileView(tile.Geometry)
		var row TileRowSyntaxState
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			if err := syntax.ResetRowState(&row, sbRow); err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if err := decodeKeyframeMain8ScreenContentSuperBlock(seq, hdr, &row, &mvTile, sb, dq, img, cdef, edges, segState); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func decodeKeyframeMain8ScreenContentSuperBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState) error {
	root := BlockGeometry{
		Level:   RootBlockLevel(seq),
		Size:    RootBlockSize(seq),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return decodeKeyframeMain8ScreenContentBlock(seq, hdr, row, mvTile, sb, root, dq, img, cdef, edges, segState)
}

func decodeKeyframeMain8ScreenContentBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	var childrenBuf [4]BlockGeometry

	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for _, child := range splitChildrenInto(childrenBuf[:0], g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8ScreenContentBlock(seq, hdr, row, mvTile, sb, child, dq, img, cdef, edges, segState); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildrenInto(childrenBuf[:0], g, part)
			if err != nil {
				return err
			}
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8ScreenContentBlock(seq, hdr, row, mvTile, sb, child, dq, img, cdef, edges, segState); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := decodeKeyframeMain8ScreenContentLeaf(seq, hdr, row, mvTile, sb, child, dq, img, cdef, edges, segState); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := decodeKeyframeMain8ScreenContentLeaf(seq, hdr, row, mvTile, sb, g, dq, img, cdef, edges, segState); err != nil {
				return err
			}
		} else {
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8ScreenContentLeaf(seq, hdr, row, mvTile, sb, child, dq, img, cdef, edges, segState); err != nil {
					return err
				}
			}
		}
	}

	if part != PartitionSplit || g.Level == BlockLevel8x8 {
		if err := ApplyPartitionContext(above, &row.Left, local, part); err != nil {
			return err
		}
	}
	return nil
}

func decodeKeyframeMain8ScreenContentLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	if row.Tile.Tile.CDF == nil || row.Tile.Tile.Entropy == nil {
		return fmt.Errorf("decoder: intrabc leaf requires tile entropy state")
	}

	segID := uint8(0)
	segPred := false
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, g.Start4Y > row.Tile.Tile.Geometry.Start4Y, g.Start4X > row.Tile.Tile.Geometry.Start4X, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	screenSyntax, err := ReadScreenContentBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, g.Start4Y > row.Tile.Tile.Geometry.Start4Y, g.Start4X > row.Tile.Tile.Geometry.Start4X, true, screenSyntax.Skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		screenSyntax.Segment = segID
	}
	screenSyntax.SegPred = segPred

	if screenSyntax.Intra {
		if screenSyntax.IntraBlk == nil {
			return fmt.Errorf("decoder: missing screen-content intra syntax")
		}
		screenSyntax.IntraBlk.SegPred = segPred
		screenSyntax.IntraBlk.SkipMode = screenSyntax.SkipMode
		screenSyntax.IntraBlk.Skip = screenSyntax.Skip
		if err := ReconstructParsedIntraLumaBlockAt8WithSkip(
			seq,
			hdr,
			row.Tile.Tile.CDF,
			row.Tile.Tile.Coef,
			dq,
			above,
			&row.Left,
			*screenSyntax.IntraBlk,
			img.Y,
			img.YStride,
			parsedPlaneWidth(img),
			parsedPlaneHeight(img),
			g.Start4X,
			g.Start4Y,
			screenSyntax.Skip,
			row.Tile.Tile.Entropy,
		); err != nil {
			return err
		}
		screenSyntax.IntraBlk.PreserveTXContext = !screenSyntax.Skip
		if err := ApplyIntraBlockContext(seq, hdr, above, &row.Left, *screenSyntax.IntraBlk); err != nil {
			return err
		}
		if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, screenSyntax.Skip, row.Tile.Tile.Entropy); err != nil {
			return err
		}
		cdef.markNoSkip(g, screenSyntax.Skip)
		if err := reconstructParsedIntraChromaBlockAt8WithSkip(
			seq,
			hdr,
			row.Tile.Tile.Coef,
			dq,
			above,
			&row.Left,
			*screenSyntax.IntraBlk,
			img.Y,
			img.YStride,
			parsedPlaneWidth(img),
			parsedPlaneHeight(img),
			img.Cb,
			img.CStride,
			parsedChromaPlaneWidth(img),
			parsedChromaPlaneHeight(img),
			img.Cr,
			img.CStride,
			parsedChromaPlaneWidth(img),
			parsedChromaPlaneHeight(img),
			screenSyntax.Skip,
			row.Tile.Tile.Entropy,
		); err != nil {
			return err
		}
		hasChroma := hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y)
		markInterBlockEdges(edges, seq.ColorConfig.ChromaSubsampling(), g, hasChroma)
		markInterTransformEdgesForBlock(edges, seq.ColorConfig.ChromaSubsampling(), g, screenSyntax.IntraBlk.TX, screenSyntax.IntraBlk.UVTX, hasChroma)
		mvTile.SplatIntra(g)
		return nil
	}

	cands, _ := mvTile.FindSingleRefCandidates(hdr, g, 0)
	refMV := chooseIntraBCReferenceMV(seq, row.Tile.Tile.Geometry, g, cands)
	mv, err := ReadMVResidual(row.Tile.Tile.MV, refMV, -1, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	hasChroma := hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y)
	mv, err = resolveIntraBCMotionVector(seq, row.Tile.Tile.Geometry, sb, g, refMV, mv, cands, hasChroma)
	if err != nil {
		return err
	}

	if err := PredictInterLumaBlock8(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), mv); err != nil {
		return err
	}
	if hasChroma {
		chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if err := PredictInterChromaBlock8(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), mv); err != nil {
			return err
		}
		if err := PredictInterChromaBlock8(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), mv); err != nil {
			return err
		}
	}

	unitsScratch := takeTransformUnitScratch(local)
	defer putTransformUnitScratch(unitsScratch)
	units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], hdr, row.Tile.Tile.CDF, above, &row.Left, local, g, screenSyntax.Segment, screenSyntax.Skip, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	for _, tu := range units {
		resCtx := uint8(0x40)
		if !screenSyntax.Skip {
			coeffs, ctx, _, err := DecodeLumaCoefficientsDCT2D(g.Size, tu.Local, tu.TX, screenSyntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
			if err != nil {
				return err
			}
			if err := ReconstructInterLumaCoefficients8(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.TX, coeffs, hdr.Segmentation.Lossless[screenSyntax.Segment]); err != nil {
				return err
			}
			resCtx = ctx
		}
		if err := ApplyLumaResidualContext(above, &row.Left, tu.Local, tu.TX, resCtx); err != nil {
			return err
		}
	}
	if hasChroma {
		uvtx := interChromaTXForBlock(seq, hdr, g.Size, screenSyntax.Segment)
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		countX, countY := transformGridCounts(chromaGeom, uvtx)
		var chromaUnitsBuf [16]BlockGeometry
		chromaDst, chromaScratch := takeBlockGeometryBuffer(chromaUnitsBuf[:0], countX*countY)
		defer putBlockGeometryBuffer(chromaScratch)
		chromaUnits := buildChromaTransformUnitsInto(chromaDst, seq.ColorConfig.ChromaSubsampling(), g, uvtx)
		for _, tu := range chromaUnits {
			uResCtx := uint8(0x40)
			vResCtx := uint8(0x40)
			if !screenSyntax.Skip {
				uCoeffs, ctx, _, err := DecodeChromaCoefficientsDCT2D(g.Size, tu, uvtx, screenSyntax.Segment, 1, seq.ColorConfig.ChromaSubsampling(), row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
				if err != nil {
					return err
				}
				if err := ReconstructInterChromaCoefficients8(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), tu.Start4X*4, tu.Start4Y*4, uvtx, uCoeffs, hdr.Segmentation.Lossless[screenSyntax.Segment]); err != nil {
					return err
				}
				uResCtx = ctx

				vCoeffs, ctx, _, err := DecodeChromaCoefficientsDCT2D(g.Size, tu, uvtx, screenSyntax.Segment, 2, seq.ColorConfig.ChromaSubsampling(), row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
				if err != nil {
					return err
				}
				if err := ReconstructInterChromaCoefficients8(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), tu.Start4X*4, tu.Start4Y*4, uvtx, vCoeffs, hdr.Segmentation.Lossless[screenSyntax.Segment]); err != nil {
					return err
				}
				vResCtx = ctx
			}
			if err := ApplyChromaResidualContextWithLayout(above, &row.Left, 1, seq.ColorConfig.ChromaSubsampling(), tu, uvtx, uResCtx); err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, &row.Left, 2, seq.ColorConfig.ChromaSubsampling(), tu, uvtx, vResCtx); err != nil {
				return err
			}
		}
		markInterBlockEdges(edges, seq.ColorConfig.ChromaSubsampling(), g, true)
		markInterTransformEdgesFromUnits(edges, units, chromaUnits)
	} else {
		markInterBlockEdges(edges, seq.ColorConfig.ChromaSubsampling(), g, false)
		markInterTransformEdgesFromUnits(edges, units, nil)
	}
	if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, screenSyntax.Skip, row.Tile.Tile.Entropy); err != nil {
		return err
	}
	cdef.markNoSkip(g, screenSyntax.Skip)

	if err := ApplyIntraBCContext(seq, above, &row.Left, g, screenSyntax.Skip, screenSyntax.SegPred); err != nil {
		return err
	}
	mvTile.SplatIntraBC(g, mv)
	return nil
}

func chooseIntraBCReferenceMV(seq av1.SequenceHeader, tile TileGeometry, g BlockGeometry, cands []RefMVCandidate) MotionVector {
	if len(cands) > 0 && cands[0].MV != (MotionVector{}) {
		return cands[0].MV
	}
	if len(cands) > 1 && cands[1].MV != (MotionVector{}) {
		return cands[1].MV
	}
	return defaultIntraBCReferenceMV(seq, tile, g)
}

func defaultIntraBCReferenceMV(seq av1.SequenceHeader, tile TileGeometry, g BlockGeometry) MotionVector {
	sb128 := boolToInt(seq.Use128x128SuperBlock)
	if g.Start4Y-(16<<sb128) < tile.Start4Y {
		return MotionVector{
			Y: 0,
			X: -(512 << sb128) - 2048,
		}
	}
	return MotionVector{
		Y: -(512 << sb128),
		X: 0,
	}
}

func resolveIntraBCMotionVector(seq av1.SequenceHeader, tile TileGeometry, sb SuperBlockGeometry, g BlockGeometry, baseRefMV, decodedMV MotionVector, cands []RefMVCandidate, hasChroma bool) (MotionVector, error) {
	diff := MotionVector{
		Y: decodedMV.Y - baseRefMV.Y,
		X: decodedMV.X - baseRefMV.X,
	}
	var refs []MotionVector
	appendRef := func(mv MotionVector) {
		for _, ref := range refs {
			if ref == mv {
				return
			}
		}
		refs = append(refs, mv)
	}

	appendRef(baseRefMV)
	for _, cand := range cands {
		appendRef(cand.MV)
	}
	appendRef(defaultIntraBCReferenceMV(seq, tile, g))

	var firstErr error
	for _, ref := range refs {
		mv := MotionVector{
			Y: ref.Y + diff.Y,
			X: ref.X + diff.X,
		}
		clipped, err := clipIntraBCMotionVector(seq, tile, sb, g, mv, hasChroma)
		if err == nil {
			return clipped, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("decoder: unable to resolve intrabc motion vector")
	}
	if fallback, ok := fallbackIntraBCMotionVector(seq, tile, sb, g, decodedMV, hasChroma); ok {
		return fallback, nil
	}
	return MotionVector{}, firstErr
}

func fallbackIntraBCMotionVector(seq av1.SequenceHeader, tile TileGeometry, sb SuperBlockGeometry, g BlockGeometry, decodedMV MotionVector, hasChroma bool) (MotionVector, bool) {
	bw4 := g.Width4()
	bh4 := g.Height4()
	if bw4 <= 0 || bh4 <= 0 {
		return MotionVector{}, false
	}
	_ = sb

	curX := g.Start4X * 4
	curY := g.Start4Y * 4
	bw := bw4 * 4
	bh := bh4 * 4

	borderLeft := tile.Start4X * 4
	borderTop := tile.Start4Y * 4
	if hasChroma {
		if bw4 < 2 && seq.ColorConfig.ChromaSubsampling() != av1.Chroma444 {
			borderLeft += 4
		}
		if bh4 < 2 && seq.ColorConfig.ChromaSubsampling() == av1.Chroma420 {
			borderTop += 4
		}
	}
	borderRight := ((tile.End4X + (bw4 - 1)) & ^(bw4 - 1)) * 4
	borderBottom := tile.End4Y * 4
	if borderRight-bw < borderLeft || borderBottom-bh < borderTop {
		return MotionVector{}, false
	}

	baseLeft := clampInt(curX+(decodedMV.X>>3), borderLeft, borderRight-bw)
	baseTop := clampInt(curY+(decodedMV.Y>>3), borderTop, borderBottom-bh)
	maxSafeLeft := curX - bw
	maxSafeTop := curY - bh

	type candidate struct {
		left int
		top  int
	}
	var candidates []candidate
	if maxSafeTop >= borderTop {
		candidates = append(candidates, candidate{left: baseLeft, top: clampInt(baseTop, borderTop, maxSafeTop)})
	}
	if maxSafeLeft >= borderLeft {
		candidates = append(candidates, candidate{left: clampInt(baseLeft, borderLeft, maxSafeLeft), top: baseTop})
	}
	if maxSafeLeft >= borderLeft && maxSafeTop >= borderTop {
		candidates = append(candidates, candidate{
			left: clampInt(baseLeft, borderLeft, maxSafeLeft),
			top:  clampInt(baseTop, borderTop, maxSafeTop),
		})
	}
	if maxSafeLeft >= borderLeft {
		candidates = append(candidates, candidate{left: borderLeft, top: baseTop})
	}
	if maxSafeTop >= borderTop {
		candidates = append(candidates, candidate{left: baseLeft, top: borderTop})
	}

	for _, cand := range candidates {
		mv := MotionVector{
			X: (cand.left - curX) << 3,
			Y: (cand.top - curY) << 3,
		}
		clipped, err := clipIntraBCMotionVector(seq, tile, sb, g, mv, hasChroma)
		if err == nil {
			return clipped, true
		}
	}
	return MotionVector{}, false
}

func clipIntraBCMotionVector(seq av1.SequenceHeader, tile TileGeometry, sb SuperBlockGeometry, g BlockGeometry, mv MotionVector, hasChroma bool) (MotionVector, error) {
	bw4 := g.Width4()
	bh4 := g.Height4()
	ssHor := boolToInt(seq.ColorConfig.ChromaSubsampling() != av1.Chroma444)
	ssVer := boolToInt(seq.ColorConfig.ChromaSubsampling() == av1.Chroma420)

	borderLeft := tile.Start4X * 4
	borderTop := tile.Start4Y * 4
	if hasChroma {
		if bw4 < 2 && ssHor != 0 {
			borderLeft += 4
		}
		if bh4 < 2 && ssVer != 0 {
			borderTop += 4
		}
	}

	curX := g.Start4X * 4
	curY := g.Start4Y * 4
	srcLeft := curX + (mv.X >> 3)
	srcTop := curY + (mv.Y >> 3)
	srcRight := srcLeft + bw4*4
	srcBottom := srcTop + bh4*4
	borderRight := ((tile.End4X + (bw4 - 1)) & ^(bw4 - 1)) * 4

	if srcLeft < borderLeft {
		delta := borderLeft - srcLeft
		srcLeft += delta
		srcRight += delta
	} else if srcRight > borderRight {
		delta := srcRight - borderRight
		srcLeft -= delta
		srcRight -= delta
	}
	if srcTop < borderTop {
		delta := borderTop - srcTop
		srcTop += delta
		srcBottom += delta
	}

	sbX := sb.StartX
	sbY := sb.StartY
	sbSize := SuperBlockSizePx(seq)
	if srcBottom > sbY && srcRight > sbX {
		if srcTop-borderTop >= srcBottom-sbY {
			delta := srcBottom - sbY
			srcTop -= delta
			srcBottom -= delta
		} else if srcLeft-borderLeft >= srcRight-sbX {
			delta := srcRight - sbX
			srcLeft -= delta
			srcRight -= delta
		}
	}
	if srcBottom > sbY+sbSize {
		delta := srcBottom - (sbY + sbSize)
		srcTop -= delta
		srcBottom -= delta
	}
	if srcBottom > sbY && srcRight > sbX {
		decodedBeforeCurrent := srcBottom <= curY || srcRight <= curX
		if decodedBeforeCurrent {
			mv.X = (srcLeft - curX) << 3
			mv.Y = (srcTop - curY) << 3
			return mv, nil
		}
		return MotionVector{}, fmt.Errorf("decoder: intrabc source overlaps current superblock mv=(%d,%d) block=[%d,%d)-[%d,%d) src=[%d,%d)-[%d,%d) sb=[%d,%d)-[%d,%d) tile=[%d,%d)-[%d,%d)", mv.Y, mv.X, curX, curY, curX+bw4*4, curY+bh4*4, srcLeft, srcTop, srcRight, srcBottom, sbX, sbY, sbX+sbSize, sbY+sbSize, tile.Start4X*4, tile.Start4Y*4, tile.End4X*4, tile.End4Y*4)
	}

	mv.X = (srcLeft - curX) << 3
	mv.Y = (srcTop - curY) << 3
	return mv, nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func ApplyIntraBCContext(seq av1.SequenceHeader, above, left *BlockContext, g BlockGeometry, skip bool, segPred bool) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	dims := BlockDims(g.Size)
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31

	fillRunInt8(above.TxIntra[:], bx4, 1<<dims.Log2W4, int8(dims.Log2W4))
	fillRunInt8(left.TxIntra[:], by4, 1<<dims.Log2H4, int8(dims.Log2H4))
	fillRunUint8(above.Mode[:], bx4, 1<<dims.Log2W4, uint8(IntraPredDC))
	fillRunUint8(left.Mode[:], by4, 1<<dims.Log2H4, uint8(IntraPredDC))
	fillRunUint8(above.PalSz[:], bx4, 1<<dims.Log2W4, 0)
	fillRunUint8(left.PalSz[:], by4, 1<<dims.Log2H4, 0)
	fillRunUint8(above.UVPalSz[:], bx4, 1<<dims.Log2W4, 0)
	fillRunUint8(left.UVPalSz[:], by4, 1<<dims.Log2H4, 0)
	fillRunUint8(above.SegPred[:], bx4, 1<<dims.Log2W4, boolToByte(segPred))
	fillRunUint8(left.SegPred[:], by4, 1<<dims.Log2H4, boolToByte(segPred))
	fillRunUint8(above.SkipMode[:], bx4, 1<<dims.Log2W4, 0)
	fillRunUint8(left.SkipMode[:], by4, 1<<dims.Log2H4, 0)
	fillRunUint8(above.Intra[:], bx4, 1<<dims.Log2W4, 0)
	fillRunUint8(left.Intra[:], by4, 1<<dims.Log2H4, 0)
	fillRunUint8(above.Skip[:], bx4, 1<<dims.Log2W4, boolToByte(skip))
	fillRunUint8(left.Skip[:], by4, 1<<dims.Log2H4, boolToByte(skip))
	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
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
	return nil
}
