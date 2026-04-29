package decoder

import (
	"fmt"
	"image"
	"os"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func debugInterDropResidual() bool {
	return os.Getenv("DEBUG_INTER_DROP_RESIDUAL") != ""
}

func debugDisableInterTemporalCompound() bool {
	return os.Getenv("DEBUG_DISABLE_TEMPORAL_COMPOUND") != ""
}

func debugDisableInterTemporalSingle() bool {
	return os.Getenv("DEBUG_DISABLE_TEMPORAL_SINGLE") != ""
}

func (d *Decoder) decodeCurrentPureGoInterFrame(parsed *ParsedFrame) (*Frame, error) {
	if len(parsed.TileGroup.Tiles) == 0 {
		return d.clonePureGoReferenceFrame(parsed)
	}
	if parsed.Header.AllowIntrabc {
		frame := takeReferenceFrame420(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig, parsed.PTS, parsed.Duration)
		fillFrame420Neutral(frame)
		segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, d.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		d.setCurrentPureGoState(takeFrameMVField(parsed.Header.Width, parsed.Header.Height), segState.Current)
		dq := InitDequantTables(d.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
		cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
		edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig.ChromaSubsampling())
		defer putLoopFilterEdgeMaps(edges)
		restoration := NewRestorationState(d.header, &parsed.Header)
		return d.decodeCurrentPureGoIntraBCFrame(parsed, dq, frame, cdef, edges, restoration, segState)
	}
	base := d.resolvePureGoBaseReference(&parsed.Header)
	frame := takeReferenceFrame420(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig, parsed.PTS, parsed.Duration)
	img := frame.Image.(*image.YCbCr)
	copyReferenceFrame420(frame, base)
	dq := InitDequantTables(d.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	field := takeFrameMVField(parsed.Header.Width, parsed.Header.Height)
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig.ChromaSubsampling())
	defer putLoopFilterEdgeMaps(edges)
	restoration := NewRestorationState(d.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, d.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	d.setCurrentPureGoState(field, segState.Current)
	temporal := d.projectPureGoTemporalMVMap(parsed)
	if err := d.decodeInterFrameMain8Luma(parsed, dq, img, field, temporal, cdef, edges, restoration, segState); err != nil {
		_ = frame.Close()
		return nil, err
	}
	debugTraceInterFrameLumaSample(parsed, "post-recon", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-recon", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-recon", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	applyAdaptiveLoopFilter420_8Masked(img, &parsed.Header, field, segState.Current, edges, cdef)
	debugTraceInterFrameLumaSample(parsed, "post-loopfilter", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-loopfilter", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-loopfilter", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	applyCDEF420_8(img, &parsed.Header, cdef)
	debugTraceInterFrameLumaSample(parsed, "post-cdef", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-cdef", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-cdef", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	applyRestoration420_8(img, &parsed.Header, restoration)
	debugTraceInterFrameLumaSample(parsed, "post-restoration", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-restoration", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
	debugTraceInterFrameChromaSample(parsed, "post-restoration", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))

	return frame, nil
}

func (d *Decoder) choosePureGoBaseReference(hdr *obu.FrameHeader) *Frame {
	if hdr == nil {
		return d.lastPureGoFrame
	}
	if hdr.PrimaryRefFrame < uint8(len(hdr.RefIdx)) {
		if frame := d.pureGoSlotFrame(int(hdr.RefIdx[hdr.PrimaryRefFrame])); frame != nil {
			return frame
		}
	}
	for _, refIdx := range hdr.RefIdx {
		if frame := d.pureGoSlotFrame(int(refIdx)); frame != nil {
			return frame
		}
	}
	return d.lastPureGoFrame
}

func (d *Decoder) pureGoSlotFrame(slot int) *Frame {
	if slot >= 0 && slot < len(d.pureGoRefs) {
		return d.pureGoRefs[slot]
	}
	return nil
}

func (d *Decoder) choosePureGoTemporalMVField(hdr *obu.FrameHeader, ref0 int8) *SpatialMVTile {
	if hdr != nil && ref0 >= 0 && int(ref0) < len(hdr.RefIdx) {
		return d.pureGoSlotMVField(int(hdr.RefIdx[ref0]))
	}
	return nil
}

func (d *Decoder) decodeInterFrameMain8Luma(parsed *ParsedFrame, dq DequantTables, img *image.YCbCr, field *SpatialMVTile, temporal *TemporalMVMap, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState) error {
	states, err := d.buildPureGoTileStates(parsed)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(d.header, &parsed.Header, tile, 0)
		if err != nil {
			return err
		}
		defer syntax.Close()
		mvTile := field.tileView(tile.Geometry)
		restRefs := newTileRestorationRefs()
		var row TileRowSyntaxState
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			if err := syntax.ResetRowState(&row, sbRow); err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
					return err
				}
				if err := d.decodeInterSuperBlock(parsed, &row, &mvTile, field, temporal, sb, dq, img, cdef, edges, segState); err != nil {
					return err
				}
			}
		}
	}
	d.capturePureGoCDFState(states)
	return nil
}

func (d *Decoder) decodeInterSuperBlock(parsed *ParsedFrame, row *TileRowSyntaxState, mvTile *SpatialMVTile, field *SpatialMVTile, temporal *TemporalMVMap, sb SuperBlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState) error {
	row.Tile.ResetSuperBlockDeltas(&parsed.Header)
	root := BlockGeometry{
		Level:   RootBlockLevel(d.header),
		Size:    RootBlockSize(d.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return d.decodeInterBlock(parsed, row, mvTile, field, temporal, sb, root, dq, img, cdef, edges, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root))
}

func (d *Decoder) decodeInterBlock(parsed *ParsedFrame, row *TileRowSyntaxState, mvTile *SpatialMVTile, field *SpatialMVTile, temporal *TemporalMVMap, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(d.header, row, sb, g)
	if err != nil {
		return err
	}
	preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
	preState := [4]int{int(preCur), int(preRng), preCnt, prePos}
	var preCDF [10]uint16
	if local.Width4() > 0 && local.Height4() > 0 {
		dims := BlockDims(local.Size)
		haveHSplit := local.Width4() > int(dims.W4)>>1
		haveVSplit := local.Height4() > int(dims.H4)>>1
		if haveHSplit && haveVSplit {
			bx8 := local.Start4X >> 1
			by8 := local.Start4Y >> 1
			ctx := GetPartitionCtx(above, &row.Left, local.Level, by8, bx8)
			copy(preCDF[:], row.Tile.Tile.CDF.PartitionCDF(local.Level, ctx))
		}
	}
	part, err := DecodePartitionDecision(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
	postState := [4]int{int(postCur), int(postRng), postCnt, postPos}
	var postCDF [10]uint16
	if local.Width4() > 0 && local.Height4() > 0 {
		dims := BlockDims(local.Size)
		haveHSplit := local.Width4() > int(dims.W4)>>1
		haveVSplit := local.Height4() > int(dims.H4)>>1
		if haveHSplit && haveVSplit {
			bx8 := local.Start4X >> 1
			by8 := local.Start4Y >> 1
			ctx := GetPartitionCtx(above, &row.Left, local.Level, by8, bx8)
			copy(postCDF[:], row.Tile.Tile.CDF.PartitionCDF(local.Level, ctx))
		}
	}
	debugTraceInterPartition(parsed, g, local, part, above, &row.Left, preState, postState, preCDF, postCDF)
	var childrenBuf [4]BlockGeometry
	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for i, child := range splitChildrenInto(childrenBuf[:0], g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := d.decodeInterBlock(parsed, row, mvTile, field, temporal, sb, child, dq, img, cdef, edges, segState, childEdges[i]); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildrenInto(childrenBuf[:0], g, part)
			if err != nil {
				return err
			}
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := d.decodeInterBlock(parsed, row, mvTile, field, temporal, sb, child, dq, img, cdef, edges, segState, childEdges[i]); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		for i, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := d.decodeInterLeaf(parsed, row, mvTile, field, temporal, child, dq, img, cdef, edges, segState, childEdges[i]); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := d.decodeInterLeaf(parsed, row, mvTile, field, temporal, g, dq, img, cdef, edges, segState, edge); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := d.decodeInterLeaf(parsed, row, mvTile, field, temporal, child, dq, img, cdef, edges, segState, childEdges[i]); err != nil {
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

func (d *Decoder) decodeInterLeaf(parsed *ParsedFrame, row *TileRowSyntaxState, mvTile *SpatialMVTile, field *SpatialMVTile, temporal *TemporalMVMap, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(d.header, row, SuperBlockGeometry{}, g)
	if err != nil {
		return err
	}
	ctxHaveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	ctxHaveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	reconHaveTop, reconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
	chromaReconHaveTop, chromaReconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
	chromaEdge := edge.forChromaLayout(d.header.ColorConfig.ChromaSubsampling())
	segID := uint8(0)
	segPred := false
	if parsed.Header.Segmentation.Enabled && parsed.Header.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(&parsed.Header, row.Tile.Tile.CDF, above, &row.Left, segState, g, ctxHaveTop, ctxHaveLeft, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	preludeBX4 := local.Start4X & 31
	preludeBY4 := local.Start4Y & 31
	preludeDims := BlockDims(local.Size)
	preludeSeg := segmentDataForID(&parsed.Header, segID)
	skipModeEligible := parsed.Header.SkipModeEnabled && minInt(int(preludeDims.W4), int(preludeDims.H4)) > 1 && preludeSeg.Ref < 0 && !preludeSeg.GlobalMV && !preludeSeg.Skip
	skipModeCtx := 0
	if skipModeEligible {
		skipModeCtx = int(above.SkipMode[preludeBX4]) + int(row.Left.SkipMode[preludeBY4])
	}
	skipCtx := int(above.Skip[preludeBX4]) + int(row.Left.Skip[preludeBY4])
	aboveSkipMode := above.SkipMode[preludeBX4]
	leftSkipMode := row.Left.SkipMode[preludeBY4]
	aboveSkip := above.Skip[preludeBX4]
	leftSkip := row.Left.Skip[preludeBY4]
	var preSkipModeCDF, postSkipModeCDF, preSkipCDF, postSkipCDF [2]uint16
	if skipModeEligible {
		copy(preSkipModeCDF[:], row.Tile.Tile.CDF.SkipMode[skipModeCtx][:])
	}
	copy(preSkipCDF[:], row.Tile.Tile.CDF.Skip[skipCtx][:])
	preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
	syntax, err := ReadInterBlockSkipPreludeWithNeighbors(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
	if skipModeEligible {
		copy(postSkipModeCDF[:], row.Tile.Tile.CDF.SkipMode[skipModeCtx][:])
	}
	copy(postSkipCDF[:], row.Tile.Tile.CDF.Skip[skipCtx][:])
	debugTraceInterSkipPrelude(parsed, g, local, ctxHaveTop, ctxHaveLeft, segID, skipModeEligible, skipModeCtx, skipCtx, aboveSkipMode, leftSkipMode, aboveSkip, leftSkip, syntax,
		[4]int{int(preCur), int(preRng), preCnt, prePos},
		[4]int{int(postCur), int(postRng), postCnt, postPos},
		preSkipModeCDF, postSkipModeCDF, preSkipCDF, postSkipCDF)
	syntax.Geometry = g
	defer func() {
		debugTraceInterLeaf(parsed, g, syntax)
	}()
	if parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(&parsed.Header, row.Tile.Tile.CDF, above, &row.Left, segState, g, ctxHaveTop, ctxHaveLeft, true, syntax.Skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		syntax.Segment = segID
	}
	if segmentDataForID(&parsed.Header, segID).Skip {
		syntax.Skip = true
	}
	syntax.SegPred = segPred
	hasChroma := hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y)
	markInterBlockEdges(edges, d.header.ColorConfig.ChromaSubsampling(), g, hasChroma)
	if err := cdef.ensureBlockIndex(g, parsed.Header.CDEF.NBits, syntax.Skip, row.Tile.Tile.Entropy); err != nil {
		return err
	}
	cdef.markNoSkip(g, syntax.Skip)
	if err := readDeltaQAndLF(d.header, &parsed.Header, row.Tile, g, syntax.Skip); err != nil {
		return err
	}
	dq = row.Tile.CurrentDQ
	intraCtx := -1
	preludeSeg = segmentDataForID(&parsed.Header, segID)
	var preIntraCDF, postIntraCDF [2]uint16
	if !syntax.SkipMode && preludeSeg.Ref < 0 && !preludeSeg.GlobalMV {
		intraCtx = GetIntraCtx(above, &row.Left, preludeBY4, preludeBX4, ctxHaveTop, ctxHaveLeft)
		copy(preIntraCDF[:], row.Tile.Tile.CDF.Intra[intraCtx][:])
	}
	preCur, preRng, preCnt, prePos = row.Tile.Tile.Entropy.DebugState()
	intra, err := ReadInterBlockIntraFlagWithNeighbors(&parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, segID, syntax.SkipMode, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	postCur, postRng, postCnt, postPos = row.Tile.Tile.Entropy.DebugState()
	if intraCtx >= 0 {
		copy(postIntraCDF[:], row.Tile.Tile.CDF.Intra[intraCtx][:])
	}
	debugTraceInterIntraFlag(parsed, g, local, ctxHaveTop, ctxHaveLeft, segID, syntax.SkipMode, intraCtx, preludeSeg.Ref, preludeSeg.GlobalMV, intra,
		[4]int{int(preCur), int(preRng), preCnt, prePos},
		[4]int{int(postCur), int(postRng), postCnt, postPos},
		preIntraCDF, postIntraCDF)
	syntax.Intra = intra
	debugTraceInterMSAC(parsed, g, "post-prelude", row.Tile.Tile.Entropy)
	debugTraceInterEntropyExhausted(parsed, g, "post-prelude", row.Tile.Tile.Entropy, len(row.Tile.Tile.Data))
	if syntax.Intra {
		intra, err := ReadIntraBlockSyntax(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, syntax.Segment, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		intra.Geometry = g
		intra.SegPred = segPred
		intra.SkipMode = syntax.SkipMode
		intra.Skip = syntax.Skip
		syntax.IntraBlk = &intra
		if syntax.IntraBlk == nil {
			return fmt.Errorf("decoder: missing intra syntax for intra-coded inter block")
		}
		if !syntax.Skip && shouldUseDav1dIntraCoeffOrder(d.header, *syntax.IntraBlk) {
			if _, err := reconstructParsedIntraBlockAt8Dav1dOrder(
				d.header,
				&parsed.Header,
				row.Tile.Tile.CDF,
				row.Tile.Tile.Coef,
				dq,
				above,
				&row.Left,
				*syntax.IntraBlk,
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
				row.Tile.Tile.Entropy,
				reconHaveTop,
				reconHaveLeft,
				edge,
			); err != nil {
				return err
			}
		} else {
			if err := ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(d.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, above, &row.Left, *syntax.IntraBlk, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X, g.Start4Y, syntax.Skip, row.Tile.Tile.Entropy, reconHaveTop, reconHaveLeft, edge.topHasRight, edge.leftHasBottom); err != nil {
				return err
			}
			if err := reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(d.header, &parsed.Header, row.Tile.Tile.Coef, dq, above, &row.Left, *syntax.IntraBlk, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), row.Tile.Tile.Entropy, syntax.Skip, chromaReconHaveTop, chromaReconHaveLeft, chromaEdge.topHasRight, chromaEdge.leftHasBottom); err != nil {
				return err
			}
		}
		syntax.IntraBlk.PreserveTXContext = !syntax.Skip
		if err := ApplyIntraBlockContext(d.header, &parsed.Header, above, &row.Left, *syntax.IntraBlk); err != nil {
			return err
		}
		if !syntax.Skip {
			markInterTransformEdgesForBlock(edges, d.header.ColorConfig.ChromaSubsampling(), g, syntax.IntraBlk.TX, syntax.IntraBlk.UVTX, hasChroma)
		}
		mvTile.SplatIntra(g)
		field.SplatIntra(g)
		return nil
	}
	if syntax.SkipMode {
		ref0, mv0, ref1, mv1, err := d.selectSkipModeCompoundRefs(parsed, &parsed.Header, mvTile, temporal, g, edge)
		if err != nil {
			return err
		}
		syntax.Ref0 = ref0
		syntax.Ref1 = ref1
		syntax.MV = mv0
		syntax.MV1 = mv1
		syntax.Mode = InterPredNearest
		syntax.Mode1 = InterPredNearest
		syntax.CompType = compoundTypeAvg
		syntax.Filter = interFilterPairForHeader(&parsed.Header)

		refFrame0 := d.resolvePureGoBlockReference(&parsed.Header, ref0)
		refFrame1 := d.resolvePureGoBlockReference(&parsed.Header, ref1)
		if err := PredictWeightedCompoundInterLumaBlock8WithFilters(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refFrame0.Y, refFrame0.YStride, refFrame0.Width, refFrame0.Height, mv0, syntax.Filter, refFrame1.Y, refFrame1.YStride, refFrame1.Width, refFrame1.Height, mv1, syntax.Filter, 8); err != nil {
			return err
		}
		if hasChroma {
			chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
			if err := PredictWeightedCompoundInterChromaBlock8WithFilters(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, refFrame0.U, refFrame0.UStride, (refFrame0.Width+1)>>1, (refFrame0.Height+1)>>1, mv0, syntax.Filter, refFrame1.U, refFrame1.UStride, (refFrame1.Width+1)>>1, (refFrame1.Height+1)>>1, mv1, syntax.Filter, 8); err != nil {
				return err
			}
			if err := PredictWeightedCompoundInterChromaBlock8WithFilters(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, refFrame0.V, refFrame0.VStride, (refFrame0.Width+1)>>1, (refFrame0.Height+1)>>1, mv0, syntax.Filter, refFrame1.V, refFrame1.VStride, (refFrame1.Width+1)>>1, (refFrame1.Height+1)>>1, mv1, syntax.Filter, 8); err != nil {
				return err
			}
		}
		unitsScratch := takeTransformUnitScratch(local)
		defer putTransformUnitScratch(unitsScratch)
		units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, g, syntax.Segment, true, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-vartxtree", row.Tile.Tile.Entropy)
		debugTraceInterTransformUnits(parsed, g, syntax, units)
		for _, tu := range units {
			if err := ApplyLumaResidualContext(above, &row.Left, tu.Local, tu.TX, 0x40); err != nil {
				return err
			}
		}
		if hasChroma {
			uvtx := interChromaTXForBlock(d.header, &parsed.Header, syntax.Size, syntax.Segment)
			chromaGeom := chromaGeometryForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
			countX, countY := transformGridCounts(chromaGeom, uvtx)
			var chromaUnitsBuf [16]BlockGeometry
			chromaDst, chromaScratch := takeBlockGeometryBuffer(chromaUnitsBuf[:0], countX*countY)
			defer putBlockGeometryBuffer(chromaScratch)
			chromaUnits := buildChromaTransformUnitsInto(chromaDst, d.header.ColorConfig.ChromaSubsampling(), g, uvtx)
			for _, tu := range chromaUnits {
				if err := ApplyChromaResidualContextWithLayout(above, &row.Left, 1, d.header.ColorConfig.ChromaSubsampling(), tu, uvtx, 0x40); err != nil {
					return err
				}
				if err := ApplyChromaResidualContextWithLayout(above, &row.Left, 2, d.header.ColorConfig.ChromaSubsampling(), tu, uvtx, 0x40); err != nil {
					return err
				}
			}
		}
		if err := ApplyInterBlockContext(d.header, &parsed.Header, above, &row.Left, syntax); err != nil {
			return err
		}
		refSlot0 := int8(-1)
		refSlot1 := int8(-1)
		if int(ref0) < len(parsed.Header.RefIdx) {
			refSlot0 = parsed.Header.RefIdx[ref0]
		}
		if int(ref1) < len(parsed.Header.RefIdx) {
			refSlot1 = parsed.Header.RefIdx[ref1]
		}
		mvTile.SplatCompoundRef(g, ref0, ref1, refSlot0, refSlot1, mv0, mv1, syntax.Mode, syntax.Mode1, syntax.Filter)
		field.SplatCompoundRef(g, ref0, ref1, refSlot0, refSlot1, mv0, mv1, syntax.Mode, syntax.Mode1, syntax.Filter)
		return nil
	}

	seg := segmentDataForID(&parsed.Header, syntax.Segment)
	isComp, err := ReadCompoundFlag(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, ctxHaveTop, ctxHaveLeft, seg, g, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	debugTraceInterMSAC(parsed, g, "post-compflag", row.Tile.Tile.Entropy)
	if isComp {
		ref0, ref1, err := ReadCompoundRefSelection(row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, ctxHaveTop, ctxHaveLeft, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-comprefs", row.Tile.Tile.Entropy)
		debugTraceInterMVNeighbors(parsed, g, mvTile)
		refCandScratch, cands0Dst, cands1Dst, temporalDst := takeRefMVCandidateTripletScratch()
		defer putRefMVCandidateScratch(refCandScratch)
		compoundCandScratch, compCandsDst, temporalCompDst := takeCompoundMVCandidatePairScratch()
		defer putCompoundMVCandidateScratch(compoundCandScratch)
		refSlot0 := int8(-1)
		refSlot1 := int8(-1)
		if ref0 >= 0 && int(ref0) < len(parsed.Header.RefIdx) {
			refSlot0 = parsed.Header.RefIdx[ref0]
		}
		if ref1 >= 0 && int(ref1) < len(parsed.Header.RefIdx) {
			refSlot1 = parsed.Header.RefIdx[ref1]
		}
		var temporal0 []RefMVCandidate
		if parsed.Header.UseRefFrameMvs && temporal != nil && !debugDisableInterTemporalCompound() {
			temporal0 = temporal.findCandidatesProjectedInto(temporalDst[:0], d.header, parsed, ref0, &parsed.Header, g)
		}
		cands0, _ := mvTile.findSingleRefCandidatesWithEdgeAndTemporalInto(cands0Dst, &parsed.Header, g, ref0, edge, temporal0)
		var temporal1 []RefMVCandidate
		if parsed.Header.UseRefFrameMvs && temporal != nil && !debugDisableInterTemporalCompound() {
			temporal1 = temporal.findCandidatesProjectedInto(temporalDst[:0], d.header, parsed, ref1, &parsed.Header, g)
		}
		cands1, _ := mvTile.findSingleRefCandidatesWithEdgeAndTemporalInto(cands1Dst, &parsed.Header, g, ref1, edge, temporal1)
		var temporalComp []CompoundRefMVCandidate
		if parsed.Header.UseRefFrameMvs && temporal != nil && !debugDisableInterTemporalCompound() {
			temporalComp = temporal.findCompoundCandidatesProjectedInto(temporalCompDst[:0], d.header, parsed, ref0, ref1, &parsed.Header, g)
		}
		compCands, mvModeCtx := mvTile.findCompoundRefCandidatesBaseWithTemporalInto(compCandsDst, d.header, parsed, &parsed.Header, g, ref0, ref1, temporalComp)
		compCands = augmentCompoundCandidatesExtended(compCands, mvTile, d.header, parsed, &parsed.Header, g, ref0, ref1)
		hasPairCands := len(compCands) > 0 && compCands[0].Weight > 0
		var mvWeightsBuf [16]int
		mvWeights := mvWeightsBuf[:0]
		if hasPairCands {
			if len(compCands) > len(mvWeightsBuf) {
				mvWeights = make([]int, len(compCands))
			} else {
				mvWeights = mvWeightsBuf[:len(compCands)]
			}
			for i := range compCands {
				mvWeights[i] = compCands[i].Weight
			}
		} else {
			nWeights := minInt(len(cands0), len(cands1))
			if nWeights == 0 {
				mvWeights = mvWeightsBuf[:1]
			} else if nWeights > len(mvWeightsBuf) {
				mvWeights = make([]int, nWeights)
			} else {
				mvWeights = mvWeightsBuf[:nWeights]
			}
			mvWeights = combinedCompoundCandidateWeightsInto(mvWeights, cands0, cands1)
		}
		nMVs := ternaryInt(hasPairCands, len(compCands), minInt(len(cands0), len(cands1)))
		debugTraceCompoundModeInputs(parsed, g, mvModeCtx, nMVs, hasPairCands, cands0, cands1, compCands, mvWeights, row.Tile.Tile.CDF)
		mode0, mode1, drl, err := ReadCompoundInterMode(row.Tile.Tile.CDF, mvModeCtx, nMVs, mvWeights, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-compintermode", row.Tile.Tile.Entropy)
		var mv0, mv1 MotionVector
		if hasPairCands {
			mv0, mv1, err = SelectCompoundRefMVCandidate(compCands, drl)
			if err != nil {
				return err
			}
		} else {
			mv0, err = SelectRefMVCandidate(cands0, drl)
			if err != nil {
				return err
			}
			mv1, err = SelectRefMVCandidate(cands1, drl)
			if err != nil {
				return err
			}
		}
		mvPrec := mvResidualPrecision(&parsed.Header)
		mv0 = normalizeCompoundBaseMV(&parsed.Header, mode0, mv0, approximateGlobalMotionVectorForBlock(&parsed.Header, ref0, g))
		if mode0 == InterPredNew {
			mv0, err = ReadMVResidual(row.Tile.Tile.MV, mv0, mvPrec, row.Tile.Tile.Entropy)
			if err != nil {
				return err
			}
		}
		mv1 = normalizeCompoundBaseMV(&parsed.Header, mode1, mv1, approximateGlobalMotionVectorForBlock(&parsed.Header, ref1, g))
		if mode1 == InterPredNew {
			mv1, err = ReadMVResidual(row.Tile.Tile.MV, mv1, mvPrec, row.Tile.Tile.Entropy)
			if err != nil {
				return err
			}
		}
		debugTraceInterMSAC(parsed, g, "post-comp-residualmv", row.Tile.Tile.Entropy)
		syntax.Ref0 = ref0
		syntax.Ref1 = ref1
		syntax.Mode = mode0
		syntax.Mode1 = mode1
		syntax.DRLIndex = drl
		syntax.MV = mv0
		syntax.MV1 = mv1
		syntax.Motion = interMotionTranslation
		syntax.CompType, syntax.MaskSign, syntax.WedgeIdx, err = ReadCompoundType(d.header, parsed, row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, g, ref0, ref1, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-comptype", row.Tile.Tile.Entropy)
		syntax.Filter, err = ReadInterFilterPairForCompound(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ref0, ref1, mode0, mode1, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-comp-filter", row.Tile.Tile.Entropy)

		refFrame0 := d.resolvePureGoBlockReference(&parsed.Header, ref0)
		refFrame1 := d.resolvePureGoBlockReference(&parsed.Header, ref1)
		lumaWarp0, useLumaWarp0 := globalMotionWarpForPlane(&parsed.Header, ref0, 0, d.header.ColorConfig.ChromaSubsampling())
		lumaWarp1, useLumaWarp1 := globalMotionWarpForPlane(&parsed.Header, ref1, 0, d.header.ColorConfig.ChromaSubsampling())
		useLumaWarp0 = useLumaWarp0 && mode0 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref0)
		useLumaWarp1 = useLumaWarp1 && mode1 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref1)
		weight, weighted := jointCompoundWeight(d.header, parsed, ref0, ref1)
		switch syntax.CompType {
		case compoundTypeAvg, compoundTypeWeightedAvg:
			useWeight := 8
			if syntax.CompType == compoundTypeWeightedAvg && weighted {
				useWeight = weight
			}
			debugTraceCompoundLumaPrediction(parsed, g, refFrame0, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refFrame1, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, useWeight)
			if err := PredictWeightedCompoundInterLumaBlock8Motion(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refFrame0.Y, refFrame0.YStride, refFrame0.Width, refFrame0.Height, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refFrame1.Y, refFrame1.YStride, refFrame1.Width, refFrame1.Height, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, useWeight); err != nil {
				return err
			}
			if hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
				chromaWarp0, useChromaWarp0 := globalMotionWarpForPlane(&parsed.Header, ref0, 1, d.header.ColorConfig.ChromaSubsampling())
				chromaWarp1, useChromaWarp1 := globalMotionWarpForPlane(&parsed.Header, ref1, 1, d.header.ColorConfig.ChromaSubsampling())
				useChromaWarp0 = useChromaWarp0 && mode0 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref0)
				useChromaWarp1 = useChromaWarp1 && mode1 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref1)
				chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
				if err := PredictWeightedCompoundInterChromaBlock8Motion(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, refFrame0.U, refFrame0.UStride, (refFrame0.Width+1)>>1, (refFrame0.Height+1)>>1, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refFrame1.U, refFrame1.UStride, (refFrame1.Width+1)>>1, (refFrame1.Height+1)>>1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, useWeight); err != nil {
					return err
				}
				if err := PredictWeightedCompoundInterChromaBlock8Motion(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, refFrame0.V, refFrame0.VStride, (refFrame0.Width+1)>>1, (refFrame0.Height+1)>>1, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refFrame1.V, refFrame1.VStride, (refFrame1.Width+1)>>1, (refFrame1.Height+1)>>1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, useWeight); err != nil {
					return err
				}
			}
		case compoundTypeSeg, compoundTypeWedge:
			var mask []uint8
			nominalW, nominalH := blockSampleDimensions(syntax.Size)
			if syntax.CompType == compoundTypeSeg {
				mask, err = PredictDiffWeightedCompoundInterLumaBlock8Motion(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, nominalW, nominalH, refFrame0.Y, refFrame0.YStride, refFrame0.Width, refFrame0.Height, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refFrame1.Y, refFrame1.YStride, refFrame1.Width, refFrame1.Height, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, syntax.MaskSign)
			} else {
				mask = buildCompoundMask(nominalW, nominalH, syntax.CompType, syntax.WedgeIdx, syntax.MaskSign)
				err = PredictMaskedCompoundInterLumaBlock8Motion(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, nominalW, nominalH, refFrame0.Y, refFrame0.YStride, refFrame0.Width, refFrame0.Height, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refFrame1.Y, refFrame1.YStride, refFrame1.Width, refFrame1.Height, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, mask, nominalW)
			}
			if err != nil {
				return err
			}
			if hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
				chromaWarp0, useChromaWarp0 := globalMotionWarpForPlane(&parsed.Header, ref0, 1, d.header.ColorConfig.ChromaSubsampling())
				chromaWarp1, useChromaWarp1 := globalMotionWarpForPlane(&parsed.Header, ref1, 1, d.header.ColorConfig.ChromaSubsampling())
				useChromaWarp0 = useChromaWarp0 && mode0 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref0)
				useChromaWarp1 = useChromaWarp1 && mode1 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref1)
				chromaX, chromaY, _, _ := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
				chromaNominalW, chromaNominalH := blockChromaSampleDimensions(d.header.ColorConfig.ChromaSubsampling(), syntax.Size)
				cmask := subsampleCompoundMask(mask, nominalW, nominalH, d.header.ColorConfig.ChromaSubsampling(), syntax.MaskSign)
				if err := PredictMaskedCompoundInterChromaBlock8Motion(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaNominalW, chromaNominalH, refFrame0.U, refFrame0.UStride, (refFrame0.Width+1)>>1, (refFrame0.Height+1)>>1, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refFrame1.U, refFrame1.UStride, (refFrame1.Width+1)>>1, (refFrame1.Height+1)>>1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, cmask, chromaNominalW); err != nil {
					return err
				}
				if err := PredictMaskedCompoundInterChromaBlock8Motion(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaNominalW, chromaNominalH, refFrame0.V, refFrame0.VStride, (refFrame0.Width+1)>>1, (refFrame0.Height+1)>>1, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refFrame1.V, refFrame1.VStride, (refFrame1.Width+1)>>1, (refFrame1.Height+1)>>1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, cmask, chromaNominalW); err != nil {
					return err
				}
			}
		}
		debugTraceInterLumaSamples(parsed, g, "luma-post-compound", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
		debugTraceInterChromaSamples(parsed, g, "chroma-post-compound", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
		debugTraceInterChromaSamples(parsed, g, "chroma-post-compound", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))

		unitsScratch := takeTransformUnitScratch(local)
		defer putTransformUnitScratch(unitsScratch)
		units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, g, syntax.Segment, syntax.Skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterTransformUnits(parsed, g, syntax, units)
		gridLen := maxIntLocal(0, g.Width4()) * maxIntLocal(0, g.Height4())
		var lumaTxTypesLocal [maxBlockTxTypeEntries]TxType
		lumaTxTypes := lumaTxTypesLocal[:0]
		if gridLen > len(lumaTxTypesLocal) {
			lumaTxTypes = make([]TxType, gridLen)
		} else {
			lumaTxTypes = lumaTxTypesLocal[:gridLen]
		}
		decodeLumaTU := func(tu TransformUnit) error {
			resCtx := uint8(0x40)
			if !syntax.Skip {
				txInfo := TxfmInfoFor(tu.TX)
				bx4 := tu.Local.Start4X & 31
				by4 := tu.Local.Start4Y & 31
				skipCtx := GetSkipCtx(txInfo, syntax.Size, above.LCoef[bx4:], row.Left.LCoef[by4:], false, 0)
				var (
					yTxType TxType
					ctx     uint8
				)
				coeffs := takeCoeffScratch(tu.TX)
				skipCDF := row.Tile.Tile.Coef.Skip[txInfo.Ctx][skipCtx][:]
				preSkipCDF := append([]uint16(nil), skipCDF...)
				preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
				skipBit := row.Tile.Tile.Entropy.DecodeBoolAdapt(skipCDF)
				postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
				debugTraceInterLumaCoeffSkip(parsed, tu.Global, int(txInfo.Ctx), skipCtx, skipBit,
					[4]int{int(preCur), int(preRng), preCnt, prePos},
					[4]int{int(postCur), int(postRng), postCnt, postPos},
					preSkipCDF, append([]uint16(nil), skipCDF...))
				if skipBit != 0 {
					ctx = 0x40
					if int(syntax.Segment) < len(parsed.Header.Segmentation.Lossless) && parsed.Header.Segmentation.Lossless[syntax.Segment] {
						yTxType = TxTypeWHTWHT
					} else {
						yTxType = TxTypeDCTDCT
					}
				} else {
					yTxType, err = ReadInterTxType(&parsed.Header, row.Tile.Tile.CDF, syntax.Segment, tu.TX, row.Tile.Tile.Entropy)
					if err != nil {
						putCoeffScratch(coeffs)
						return err
					}
					ctx, _, err = DecodeLumaCoefficientsNoSkipInto(coeffs.buf, syntax.Size, tu.Local, tu.TX, yTxType, syntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
					if err != nil {
						putCoeffScratch(coeffs)
						return err
					}
				}
				if debugInterDropResidual() {
					clear(coeffs.buf)
				}
				if err := ReconstructInterLumaCoefficients8WithTxType(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.TX, yTxType, coeffs.buf, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
					putCoeffScratch(coeffs)
					return err
				}
				debugTraceInterLumaCoeffDecoded(parsed, tu.Global, yTxType, ctx, coeffs.buf, row.Tile.Tile.Entropy)
				putCoeffScratch(coeffs)
				setBlockTxTypeRange(lumaTxTypes, g, tu.Global, yTxType)
				resCtx = ctx
			}
			if err := ApplyLumaResidualContext(above, &row.Left, tu.Local, tu.TX, resCtx); err != nil {
				return err
			}
			return nil
		}
		hasChromaCompound := hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y)
		if hasChromaCompound {
			uvtx := interChromaTXForBlock(d.header, &parsed.Header, syntax.Size, syntax.Segment)
			layout := d.header.ColorConfig.ChromaSubsampling()
			chromaGeom := chromaGeometryForBlock(layout, g)
			countX, countY := transformGridCounts(chromaGeom, uvtx)
			var chromaUnitsBuf [16]BlockGeometry
			chromaDst, chromaScratch := takeBlockGeometryBuffer(chromaUnitsBuf[:0], countX*countY)
			defer putBlockGeometryBuffer(chromaScratch)
			chromaUnits := buildChromaTransformUnitsInto(chromaDst, layout, g, uvtx)
			if !syntax.Skip {
				markInterTransformEdgesFromUnits(edges, units, chromaUnits)
			}
			decodeChromaPlaneTU := func(plane int, tu BlockGeometry) error {
				resCtx := uint8(0x40)
				if !syntax.Skip {
					yTxType := blockTxTypeForChromaTU(lumaTxTypes, g, chromaGeom, tu, layout)
					uvTxType := InterUVTxType(uvtx, yTxType)
					coeffs := takeCoeffScratch(uvtx)
					ctx, _, err := DecodeChromaCoefficientsInto(coeffs.buf, syntax.Size, tu, uvtx, uvTxType, syntax.Segment, plane, layout, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
					if err != nil {
						putCoeffScratch(coeffs)
						return err
					}
					if debugInterDropResidual() {
						clear(coeffs.buf)
					}
					dst := img.Cb
					if plane == 2 {
						dst = img.Cr
					}
					if err := ReconstructInterChromaCoefficients8WithTxType(dst, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), tu.Start4X*4, tu.Start4Y*4, uvtx, uvTxType, coeffs.buf, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
						putCoeffScratch(coeffs)
						return err
					}
					if plane == 1 {
						debugTraceInterChromaSamples(parsed, g, "post-chroma-tu", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
					} else {
						debugTraceInterChromaSamples(parsed, g, "post-chroma-tu", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
					}
					putCoeffScratch(coeffs)
					resCtx = ctx
				}
				if err := ApplyChromaResidualContextWithLayout(above, &row.Left, plane, layout, tu, uvtx, resCtx); err != nil {
					return err
				}
				return nil
			}
			for offY4 := 0; offY4 < g.Height4(); offY4 += 16 {
				for offX4 := 0; offX4 < g.Width4(); offX4 += 16 {
					lumaChunk := residualChunkLumaGeometry(g, offX4, offY4)
					for _, tu := range units {
						if geometryContainsStart(lumaChunk, tu.Global) {
							if err := decodeLumaTU(tu); err != nil {
								return err
							}
							debugTraceInterResidualStep(parsed, g, 0, tu.Global, "post-luma-tu", row.Tile.Tile.Entropy)
						}
					}
					chromaChunk := residualChunkChromaGeometry(layout, g, offX4, offY4)
					for plane := 1; plane <= 2; plane++ {
						for _, tu := range chromaUnits {
							if geometryContainsStart(chromaChunk, tu) {
								if err := decodeChromaPlaneTU(plane, tu); err != nil {
									return err
								}
								debugTraceInterResidualStep(parsed, g, plane, tu, "post-chroma-tu", row.Tile.Tile.Entropy)
							}
						}
					}
				}
			}
			debugTraceInterMSAC(parsed, g, "post-chroma-coeffs", row.Tile.Tile.Entropy)
		} else {
			for _, tu := range units {
				if err := decodeLumaTU(tu); err != nil {
					return err
				}
				debugTraceInterResidualStep(parsed, g, 0, tu.Global, "post-luma-tu", row.Tile.Tile.Entropy)
			}
			debugTraceInterMSAC(parsed, g, "post-luma-coeffs", row.Tile.Tile.Entropy)
			if !syntax.Skip {
				markInterTransformEdgesFromUnits(edges, units, nil)
			}
		}
		if err := ApplyInterBlockContext(d.header, &parsed.Header, above, &row.Left, syntax); err != nil {
			return err
		}
		mvTile.SplatCompoundRef(g, ref0, ref1, refSlot0, refSlot1, mv0, mv1, syntax.Mode, syntax.Mode1, syntax.Filter)
		field.SplatCompoundRef(g, ref0, ref1, refSlot0, refSlot1, mv0, mv1, syntax.Mode, syntax.Mode1, syntax.Filter)
		return nil
	}

	ref0 := segmentationRefForBlock(seg)
	if ref0 < 0 {
		ref0, err = ReadSingleRefIndexTraced(row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, ctxHaveTop, ctxHaveLeft, row.Tile.Tile.Entropy, parsed, g)
		if err != nil {
			return err
		}
	}
	debugTraceInterMSAC(parsed, g, "post-ref", row.Tile.Tile.Entropy)
	refCandScratch, candsDst, temporalDst := takeRefMVCandidatePairScratch()
	defer putRefMVCandidateScratch(refCandScratch)
	refSlot := int8(-1)
	if ref0 >= 0 && int(ref0) < len(parsed.Header.RefIdx) {
		refSlot = parsed.Header.RefIdx[ref0]
	}
	var temporalCands []RefMVCandidate
	if parsed.Header.UseRefFrameMvs && temporal != nil && !debugDisableInterTemporalSingle() {
		temporalCands = temporal.findCandidatesProjectedInto(temporalDst[:0], d.header, parsed, ref0, &parsed.Header, g)
	}
	cands, mvCtx := mvTile.findSingleRefCandidatesWithEdgeAndTemporalInto(candsDst, &parsed.Header, g, ref0, edge, temporalCands)
	debugTraceSingleRefStack(parsed, g, "base", ref0, mvCtx, cands)
	if parsed.Header.UseRefFrameMvs && temporal != nil && !debugDisableInterTemporalSingle() {
		if globalCtx, ok := temporal.projectedGlobalMVContext(d.header, parsed, ref0, &parsed.Header, g); ok {
			mvCtx = (mvCtx &^ (1 << 3)) | (globalCtx << 3)
		}
		debugTraceSingleRefStack(parsed, g, "after-temporal", ref0, mvCtx, cands)
	}
	cands = augmentSingleRefCandidatesExtended(cands, mvTile, d.header, parsed, &parsed.Header, g, ref0)
	debugTraceSingleRefStack(parsed, g, "after-extended", ref0, mvCtx, cands)
	debugTraceInterMVNeighbors(parsed, g, mvTile)
	debugTraceInterMVCandidates(parsed, g, ref0, mvCtx, cands, temporalCands)
	nMVs := activeRefMVCandidateCount(cands)
	var weightsBuf [16]int
	var weights []int
	if len(cands) > len(weightsBuf) {
		weights = make([]int, len(cands))
	} else {
		weights = weightsBuf[:len(cands)]
	}
	for i := range cands {
		weights[i] = cands[i].Weight
	}
	debugTraceInterModeInputs(parsed, g, mvCtx, nMVs, weights, row.Tile.Tile.CDF)
	var mode InterPredMode
	var drl uint8
	if seg.Skip || seg.GlobalMV {
		mode = InterPredGlobal
		drl = drlNearest
	} else {
		mode, drl, err = ReadSingleRefInterModeTraced(row.Tile.Tile.CDF, mvCtx, nMVs, weights, row.Tile.Tile.Entropy, parsed, g)
		if err != nil {
			return err
		}
	}
	debugTraceInterMSAC(parsed, g, "post-intermode", row.Tile.Tile.Entropy)
	mv, err := SelectRefMVCandidate(cands, drl)
	if err != nil {
		return err
	}
	mv = normalizeSingleRefMV(&parsed.Header, mode, drl, nMVs, mv, approximateGlobalMotionVectorForBlock(&parsed.Header, ref0, g))
	if mode == InterPredNew {
		mvPrec := mvResidualPrecision(&parsed.Header)
		mv, err = ReadMVResidual(row.Tile.Tile.MV, mv, mvPrec, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-residualmv", row.Tile.Tile.Entropy)
	}

	syntax.Ref0 = ref0
	syntax.Ref1 = -1
	syntax.Mode = mode
	syntax.DRLIndex = drl
	syntax.MV = mv
	syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx, err = ReadInterIntraSyntax(d.header, row.Tile.Tile.CDF, g, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	debugTraceInterMSAC(parsed, g, "post-interintra", row.Tile.Tile.Entropy)
	if syntax.InterIntraType == interIntraNone {
		syntax.Motion, err = ReadInterMotionMode(&parsed.Header, row.Tile.Tile.CDF, above, &row.Left, mvTile, g, syntax, ctxHaveTop, ctxHaveLeft, edge, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-motionmode", row.Tile.Tile.Entropy)
	} else {
		syntax.Motion = interMotionTranslation
	}
	if syntax.Motion == interMotionWarp {
		syntax.Filter = defaultInterFilterPair()
	} else {
		syntax.Filter, err = ReadInterFilterPairForSingle(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ref0, mode, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-filter", row.Tile.Tile.Entropy)
	}

	refFrame := d.resolvePureGoBlockReference(&parsed.Header, ref0)
	lumaGlobalMV, lumaGlobalWarp, useLumaGlobalWarp := singleRefGlobalMotionForPlane(&parsed.Header, ref0, 0, d.header.ColorConfig.ChromaSubsampling(), g)
	useLumaGlobalWarp = useLumaGlobalWarp && globalMotionUsesWarp(&parsed.Header, ref0)
	if mode == InterPredGlobal {
		mv = lumaGlobalMV
		syntax.MV = mv
	}
	if syntax.Motion == interMotionWarp {
		warp, ok := deriveLocalAffineWarp(mvTile, g, ref0, mv, edge)
		if ok {
			debugTraceInterWarp(parsed, g, warp)
			if err := PredictInterLumaBlock8Warp(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, warp); err != nil {
				return err
			}
			debugTraceInterLumaSamples(parsed, g, "luma-post-localwarp", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
		} else {
			predictInterLumaBlock8WithFiltersUnchecked(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, mv, syntax.Filter)
			debugTraceInterLumaSamples(parsed, g, "luma-post-localwarp-fallback", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
		}
	} else if useLumaGlobalWarp && mode == InterPredGlobal {
		if err := PredictInterLumaBlock8Warp(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, lumaGlobalWarp); err != nil {
			return err
		}
		debugTraceInterLumaSamples(parsed, g, "luma-post-globalwarp", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
	} else {
		predictInterLumaBlock8WithFiltersUnchecked(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, mv, syntax.Filter)
		debugTraceInterLumaSamples(parsed, g, "luma-post-mc", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
		if syntax.Motion == interMotionOBMC {
			if err := d.applyInterOBMCLuma8(parsed, mvTile, g, img); err != nil {
				return err
			}
			debugTraceInterLumaSamples(parsed, g, "luma-post-obmc", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
		}
	}
	if syntax.InterIntraType != interIntraNone {
		blockW, blockH := blockSampleDimensions(syntax.Size)
		if err := blendInterIntraBlock8(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X*4, g.Start4Y*4, blockW, blockH, syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx); err != nil {
			return err
		}
		debugTraceInterLumaSamples(parsed, g, "luma-post-interintra", img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img))
	}
	if hasChroma {
		chromaGlobalMV, chromaGlobalWarp, useChromaGlobalWarp := singleRefGlobalMotionForPlane(&parsed.Header, ref0, 1, d.header.ColorConfig.ChromaSubsampling(), g)
		useChromaGlobalWarp = useChromaGlobalWarp && globalMotionUsesWarp(&parsed.Header, ref0)
		if mode == InterPredGlobal && chromaGlobalMV != mv {
			chromaGlobalMV = mv
		}
		if useChromaGlobalWarp && mode == InterPredGlobal {
			chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
			if err := PredictInterChromaBlock8Warp(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, refFrame.U, refFrame.UStride, (refFrame.Width+1)>>1, (refFrame.Height+1)>>1, chromaGlobalWarp); err != nil {
				return err
			}
			if err := PredictInterChromaBlock8Warp(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaW, chromaH, refFrame.V, refFrame.VStride, (refFrame.Width+1)>>1, (refFrame.Height+1)>>1, chromaGlobalWarp); err != nil {
				return err
			}
		} else {
			if err := d.predictInterChroma420Block8(parsed, above, &row.Left, mvTile, g, syntax, img, refFrame); err != nil {
				return err
			}
			debugTraceInterChromaSamples(parsed, g, "chroma-post-mc", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
			debugTraceInterChromaSamples(parsed, g, "chroma-post-mc", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
			if syntax.Motion == interMotionOBMC {
				if err := d.applyInterOBMCChroma8(parsed, mvTile, g, img); err != nil {
					return err
				}
				debugTraceInterChromaSamples(parsed, g, "chroma-post-obmc", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
				debugTraceInterChromaSamples(parsed, g, "chroma-post-obmc", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
			}
		}
		if syntax.InterIntraType != interIntraNone {
			layout := d.header.ColorConfig.ChromaSubsampling()
			chromaX, chromaY, _, _ := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
			chromaBlockW, chromaBlockH := blockChromaSampleDimensions(layout, syntax.Size)
			if err := blendInterIntraChromaBlock8(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaBlockW, chromaBlockH, layout, syntax.Size, syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx); err != nil {
				return err
			}
			if err := blendInterIntraChromaBlock8(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), chromaX, chromaY, chromaBlockW, chromaBlockH, layout, syntax.Size, syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx); err != nil {
				return err
			}
			debugTraceInterChromaSamples(parsed, g, "chroma-post-interintra", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
			debugTraceInterChromaSamples(parsed, g, "chroma-post-interintra", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
		}
	}

	unitsScratch := takeTransformUnitScratch(local)
	defer putTransformUnitScratch(unitsScratch)
	units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, g, syntax.Segment, syntax.Skip, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	debugTraceInterMSAC(parsed, g, "post-vartxtree", row.Tile.Tile.Entropy)
	debugTraceInterTransformUnits(parsed, g, syntax, units)
	gridLen := maxIntLocal(0, g.Width4()) * maxIntLocal(0, g.Height4())
	var lumaTxTypesLocal [maxBlockTxTypeEntries]TxType
	lumaTxTypes := lumaTxTypesLocal[:0]
	if gridLen > len(lumaTxTypesLocal) {
		lumaTxTypes = make([]TxType, gridLen)
	} else {
		lumaTxTypes = lumaTxTypesLocal[:gridLen]
	}
	decodeLumaTU := func(tu TransformUnit) error {
		resCtx := uint8(0x40)
		if !syntax.Skip {
			txInfo := TxfmInfoFor(tu.TX)
			bx4 := tu.Local.Start4X & 31
			by4 := tu.Local.Start4Y & 31
			skipCtx := GetSkipCtx(txInfo, syntax.Size, above.LCoef[bx4:], row.Left.LCoef[by4:], false, 0)
			var (
				yTxType TxType
				ctx     uint8
			)
			coeffs := takeCoeffScratch(tu.TX)
			skipCDF := row.Tile.Tile.Coef.Skip[txInfo.Ctx][skipCtx][:]
			preSkipCDF := append([]uint16(nil), skipCDF...)
			preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
			skipBit := row.Tile.Tile.Entropy.DecodeBoolAdapt(skipCDF)
			postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
			debugTraceInterLumaCoeffSkip(parsed, tu.Global, int(txInfo.Ctx), skipCtx, skipBit,
				[4]int{int(preCur), int(preRng), preCnt, prePos},
				[4]int{int(postCur), int(postRng), postCnt, postPos},
				preSkipCDF, append([]uint16(nil), skipCDF...))
			if skipBit != 0 {
				ctx = 0x40
				if int(syntax.Segment) < len(parsed.Header.Segmentation.Lossless) && parsed.Header.Segmentation.Lossless[syntax.Segment] {
					yTxType = TxTypeWHTWHT
				} else {
					yTxType = TxTypeDCTDCT
				}
			} else {
				yTxType, err = ReadInterTxType(&parsed.Header, row.Tile.Tile.CDF, syntax.Segment, tu.TX, row.Tile.Tile.Entropy)
				if err != nil {
					putCoeffScratch(coeffs)
					return err
				}
				ctx, _, err = DecodeLumaCoefficientsNoSkipInto(coeffs.buf, syntax.Size, tu.Local, tu.TX, yTxType, syntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
				if err != nil {
					putCoeffScratch(coeffs)
					return err
				}
			}
			if debugInterDropResidual() {
				clear(coeffs.buf)
			}
			if err := ReconstructInterLumaCoefficients8WithTxType(img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.TX, yTxType, coeffs.buf, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
				putCoeffScratch(coeffs)
				return err
			}
			debugTraceInterLumaCoeffDecoded(parsed, tu.Global, yTxType, ctx, coeffs.buf, row.Tile.Tile.Entropy)
			putCoeffScratch(coeffs)
			setBlockTxTypeRange(lumaTxTypes, g, tu.Global, yTxType)
			resCtx = ctx
		}
		if err := ApplyLumaResidualContext(above, &row.Left, tu.Local, tu.TX, resCtx); err != nil {
			return err
		}
		return nil
	}
	if hasChroma {
		uvtx := interChromaTXForBlock(d.header, &parsed.Header, syntax.Size, syntax.Segment)
		layout := d.header.ColorConfig.ChromaSubsampling()
		chromaGeom := chromaGeometryForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
		countX, countY := transformGridCounts(chromaGeom, uvtx)
		var chromaUnitsBuf [16]BlockGeometry
		chromaDst, chromaScratch := takeBlockGeometryBuffer(chromaUnitsBuf[:0], countX*countY)
		defer putBlockGeometryBuffer(chromaScratch)
		chromaUnits := buildChromaTransformUnitsInto(chromaDst, d.header.ColorConfig.ChromaSubsampling(), g, uvtx)
		if !syntax.Skip {
			markInterTransformEdgesFromUnits(edges, units, chromaUnits)
		}
		decodeChromaPlaneTU := func(plane int, tu BlockGeometry) error {
			resCtx := uint8(0x40)
			if !syntax.Skip {
				yTxType := blockTxTypeForChromaTU(lumaTxTypes, g, chromaGeom, tu, layout)
				uvTxType := InterUVTxType(uvtx, yTxType)
				coeffs := takeCoeffScratch(uvtx)
				ctx, _, err := DecodeChromaCoefficientsInto(coeffs.buf, syntax.Size, tu, uvtx, uvTxType, syntax.Segment, plane, layout, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
				if err != nil {
					putCoeffScratch(coeffs)
					return err
				}
				if debugInterDropResidual() {
					clear(coeffs.buf)
				}
				dst := img.Cb
				if plane == 2 {
					dst = img.Cr
				}
				if err := ReconstructInterChromaCoefficients8WithTxType(dst, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), tu.Start4X*4, tu.Start4Y*4, uvtx, uvTxType, coeffs.buf, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
					putCoeffScratch(coeffs)
					return err
				}
				if plane == 1 {
					debugTraceInterChromaSamples(parsed, g, "post-chroma-tu", "U", img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
				} else {
					debugTraceInterChromaSamples(parsed, g, "post-chroma-tu", "V", img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img))
				}
				putCoeffScratch(coeffs)
				resCtx = ctx
			}
			if err := ApplyChromaResidualContextWithLayout(above, &row.Left, plane, d.header.ColorConfig.ChromaSubsampling(), tu, uvtx, resCtx); err != nil {
				return err
			}
			return nil
		}
		for offY4 := 0; offY4 < g.Height4(); offY4 += 16 {
			for offX4 := 0; offX4 < g.Width4(); offX4 += 16 {
				lumaChunk := residualChunkLumaGeometry(g, offX4, offY4)
				for _, tu := range units {
					if geometryContainsStart(lumaChunk, tu.Global) {
						if err := decodeLumaTU(tu); err != nil {
							return err
						}
						debugTraceInterResidualStep(parsed, g, 0, tu.Global, "post-luma-tu", row.Tile.Tile.Entropy)
					}
				}
				chromaChunk := residualChunkChromaGeometry(layout, g, offX4, offY4)
				for plane := 1; plane <= 2; plane++ {
					for _, tu := range chromaUnits {
						if geometryContainsStart(chromaChunk, tu) {
							if err := decodeChromaPlaneTU(plane, tu); err != nil {
								return err
							}
							debugTraceInterResidualStep(parsed, g, plane, tu, "post-chroma-tu", row.Tile.Tile.Entropy)
						}
					}
				}
			}
		}
		debugTraceInterMSAC(parsed, g, "post-chroma-coeffs", row.Tile.Tile.Entropy)
	} else {
		for _, tu := range units {
			if err := decodeLumaTU(tu); err != nil {
				return err
			}
			debugTraceInterResidualStep(parsed, g, 0, tu.Global, "post-luma-tu", row.Tile.Tile.Entropy)
		}
		debugTraceInterMSAC(parsed, g, "post-luma-coeffs", row.Tile.Tile.Entropy)
		if !syntax.Skip {
			markInterTransformEdgesFromUnits(edges, units, nil)
		}
	}
	if err := ApplyInterBlockContext(d.header, &parsed.Header, above, &row.Left, syntax); err != nil {
		return err
	}
	interIntra := syntax.InterIntraType != interIntraNone
	mvTile.SplatSingleRefWithInterIntra(g, ref0, refSlot, mv, mode, syntax.Filter, interIntra)
	field.SplatSingleRefWithInterIntra(g, ref0, refSlot, mv, mode, syntax.Filter, interIntra)
	return nil
}

func (d *Decoder) resolvePureGoBlockReference(hdr *obu.FrameHeader, ref0 int8) *Frame {
	if hdr != nil && ref0 >= 0 && int(ref0) < len(hdr.RefIdx) {
		if frame := d.pureGoSlotFrame(int(hdr.RefIdx[ref0])); frame != nil {
			return frame
		}
	}
	return d.resolvePureGoBaseReference(hdr)
}

func (d *Decoder) resolvePureGoBaseReference(hdr *obu.FrameHeader) *Frame {
	if frame := d.choosePureGoBaseReference(hdr); frame != nil {
		return frame
	}
	if hdr == nil {
		return nil
	}
	return d.blankPureGoReferenceFrame(hdr.Width, hdr.Height)
}

func (d *Decoder) blankPureGoReferenceFrame(width, height int) *Frame {
	if d.header.ColorConfig.BitDepth > 8 {
		yPlane := make([]uint16, width*height)
		chromaWidth := (width + 1) >> 1
		chromaHeight := (height + 1) >> 1
		uPlane := make([]uint16, chromaWidth*chromaHeight)
		vPlane := make([]uint16, chromaWidth*chromaHeight)
		fillUint16(uPlane, 1<<(d.header.ColorConfig.BitDepth-1))
		fillUint16(vPlane, 1<<(d.header.ColorConfig.BitDepth-1))
		frame, err := buildFrameFromPlanar16NoPreview(width, height, d.header.ColorConfig, 0, 0, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth)
		if err == nil {
			return frame
		}
		return &Frame{
			Width:    width,
			Height:   height,
			BitDepth: d.header.ColorConfig.BitDepth,
			Layout:   d.header.ColorConfig.ChromaSubsampling(),
			Y16:      yPlane,
			U16:      uPlane,
			V16:      vPlane,
			YStride:  width,
			UStride:  chromaWidth,
			VStride:  chromaWidth,
		}
	}
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	return buildFrameFromYCbCr(img, d.header.ColorConfig, 0, 0)
}

func (d *Decoder) predictInterChroma420Block8(parsed *ParsedFrame, above, left *BlockContext, mvTile *SpatialMVTile, g BlockGeometry, syntax InterBlockSyntax, img *image.YCbCr, refFrame *Frame) error {
	cx, cy, cw, ch := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
	if cw <= 0 || ch <= 0 {
		return nil
	}
	if g.Width4() == 1 || g.Height4() == 1 {
		chroma := chromaGeometryForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
		subX := chroma.Start4X * 4
		subY := chroma.Start4Y * 4
		subW := chroma.Width4() * 4
		subH := chroma.Height4() * 4
		if ok, err := d.predictInterSub8x8Chroma420Block8(parsed, above, left, mvTile, g, syntax, img, subX, subY, subW, subH); err != nil {
			return err
		} else if ok {
			return nil
		}
		cx, cy, cw, ch = subX, subY, subW, subH
	}
	predictInterChromaBlock8WithFiltersUnchecked(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), cx, cy, cw, ch, refFrame.U, refFrame.UStride, (refFrame.Width+1)>>1, (refFrame.Height+1)>>1, syntax.MV, syntax.Filter)
	predictInterChromaBlock8WithFiltersUnchecked(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), cx, cy, cw, ch, refFrame.V, refFrame.VStride, (refFrame.Width+1)>>1, (refFrame.Height+1)>>1, syntax.MV, syntax.Filter)
	return nil
}

func (d *Decoder) predictInterSub8x8Chroma420Block8(parsed *ParsedFrame, above, left *BlockContext, mvTile *SpatialMVTile, g BlockGeometry, syntax InterBlockSyntax, img *image.YCbCr, cx, cy, cw, ch int) (bool, error) {
	if parsed == nil || above == nil || left == nil || mvTile == nil {
		return false, nil
	}
	bw4 := g.Width4()
	bh4 := g.Height4()
	cur := interSub8x8Segment{
		ref:    syntax.Ref0,
		mv:     syntax.MV,
		filter: syntax.Filter,
	}

	switch {
	case bw4 == 1 && bh4 == 1:
		tlBlock := mvTile.block(g.Start4X-1, g.Start4Y-1)
		leftBlock := mvTile.block(g.Start4X-1, g.Start4Y)
		topBlock := mvTile.block(g.Start4X, g.Start4Y-1)
		if !validSub8x8SegmentBlock(tlBlock) || !validSub8x8SegmentBlock(leftBlock) || !validSub8x8SegmentBlock(topBlock) {
			return false, nil
		}
		tl := interSub8x8Segment{
			ref:    tlBlock.Ref[0],
			mv:     tlBlock.MV[0],
			filter: tlBlock.Filter,
		}
		lseg := interSub8x8Segment{
			ref:    leftBlock.Ref[0],
			mv:     leftBlock.MV[0],
			filter: leftBlock.Filter,
		}
		tseg := interSub8x8Segment{
			ref:    topBlock.Ref[0],
			mv:     topBlock.MV[0],
			filter: topBlock.Filter,
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx, cy, 2, 2, tl); err != nil {
			return false, err
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx, cy+2, 2, ch-2, lseg); err != nil {
			return false, err
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx+2, cy, cw-2, 2, tseg); err != nil {
			return false, err
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx+2, cy+2, cw-2, ch-2, cur); err != nil {
			return false, err
		}
		return true, nil
	case bw4 == 1:
		leftBlock := mvTile.block(g.Start4X-1, g.Start4Y)
		if !validSub8x8SegmentBlock(leftBlock) {
			return false, nil
		}
		lseg := interSub8x8Segment{
			ref:    leftBlock.Ref[0],
			mv:     leftBlock.MV[0],
			filter: leftBlock.Filter,
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx, cy, 2, ch, lseg); err != nil {
			return false, err
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx+2, cy, cw-2, ch, cur); err != nil {
			return false, err
		}
		return true, nil
	case bh4 == 1:
		topBlock := mvTile.block(g.Start4X, g.Start4Y-1)
		if !validSub8x8SegmentBlock(topBlock) {
			return false, nil
		}
		tseg := interSub8x8Segment{
			ref:    topBlock.Ref[0],
			mv:     topBlock.MV[0],
			filter: topBlock.Filter,
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx, cy, cw, 2, tseg); err != nil {
			return false, err
		}
		if err := d.predictInterChromaSegment8(parsed, img, cx, cy+2, cw, ch-2, cur); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, nil
	}
}

type interSub8x8Segment struct {
	ref    int8
	mv     MotionVector
	filter InterFilterPair
}

func validSub8x8SegmentBlock(b *SpatialMVBlock) bool {
	return b != nil && b.Valid && !b.Intra && b.Ref[0] >= 0
}

func interFilterPairFromAbove(above *BlockContext, idx int, fallback InterFilterPair) InterFilterPair {
	if above == nil || idx < 0 || idx >= len(above.Filter[0]) {
		return fallback
	}
	return InterFilterPair{above.Filter[0][idx], above.Filter[1][idx]}
}

func interFilterPairFromLeft(left *BlockContext, idx int, fallback InterFilterPair) InterFilterPair {
	if left == nil || idx < 0 || idx >= len(left.Filter[0]) {
		return fallback
	}
	return InterFilterPair{left.Filter[0][idx], left.Filter[1][idx]}
}

func (d *Decoder) predictInterChromaSegment8(parsed *ParsedFrame, img *image.YCbCr, x, y, width, height int, seg interSub8x8Segment) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	refFrame := d.resolvePureGoBlockReference(&parsed.Header, seg.ref)
	if refFrame == nil {
		return fmt.Errorf("decoder: missing sub8x8 chroma reference for ref %d", seg.ref)
	}
	predictInterChromaBlock8WithFiltersUnchecked(img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), x, y, width, height, refFrame.U, refFrame.UStride, (refFrame.Width+1)>>1, (refFrame.Height+1)>>1, seg.mv, seg.filter)
	predictInterChromaBlock8WithFiltersUnchecked(img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), x, y, width, height, refFrame.V, refFrame.VStride, (refFrame.Width+1)>>1, (refFrame.Height+1)>>1, seg.mv, seg.filter)
	return nil
}
