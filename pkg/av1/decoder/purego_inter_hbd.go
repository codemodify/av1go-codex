package decoder

func (d *Decoder) decodeCurrentPureGoInterFrame10(parsed *ParsedFrame) (*Frame, error) {
	if len(parsed.TileGroup.Tiles) == 0 {
		return d.clonePureGoReferenceFrame(parsed)
	}
	if parsed.Header.AllowIntrabc {
		segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, d.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		d.setCurrentPureGoState(takeFrameMVField(parsed.Header.Width, parsed.Header.Height), segState.Current)
		width := parsed.Header.Width
		height := parsed.Header.Height
		yPlane := make([]uint16, width*height)
		chromaWidth := (width + 1) >> 1
		chromaHeight := (height + 1) >> 1
		uPlane := make([]uint16, chromaWidth*chromaHeight)
		vPlane := make([]uint16, chromaWidth*chromaHeight)
		fillUint16(uPlane, 1<<(d.header.ColorConfig.BitDepth-1))
		fillUint16(vPlane, 1<<(d.header.ColorConfig.BitDepth-1))
		dq := InitDequantTables(d.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
		cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
		restoration := NewRestorationState(d.header, &parsed.Header)
		return d.decodeCurrentPureGoIntraBCFrame10(parsed, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, cdef, restoration, segState)
	}
	base := d.resolvePureGoBaseReference(&parsed.Header)

	width := parsed.Header.Width
	height := parsed.Header.Height
	yPlane := cloneReferenceLuma16(base, width, height, d.header.ColorConfig.BitDepth)
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	uPlane := cloneReferenceChroma16(base, 1, chromaWidth, chromaHeight, d.header.ColorConfig.BitDepth)
	vPlane := cloneReferenceChroma16(base, 2, chromaWidth, chromaHeight, d.header.ColorConfig.BitDepth)

	dq := InitDequantTables(d.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	field := takeFrameMVField(parsed.Header.Width, parsed.Header.Height)
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig.ChromaSubsampling())
	defer putLoopFilterEdgeMaps(edges)
	restoration := NewRestorationState(d.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, d.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	d.setCurrentPureGoState(field, segState.Current)
	temporal := d.projectPureGoTemporalMVMap(parsed)
	if err := d.decodeInterFrameMain10(parsed, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, field, temporal, cdef, edges, restoration, segState); err != nil {
		return nil, err
	}
	debugTraceInterFrameLumaSample16(parsed, "post-recon", yPlane, width, width, height)
	debugTraceInterFrameChromaSample16(parsed, "post-recon", "U", uPlane, chromaWidth, chromaWidth, chromaHeight)
	debugTraceInterFrameChromaSample16(parsed, "post-recon", "V", vPlane, chromaWidth, chromaWidth, chromaHeight)
	applyAdaptiveLoopFilter420_16Masked(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, d.header.ColorConfig.BitDepth, field, segState.Current, edges, cdef)
	debugTraceInterFrameLumaSample16(parsed, "post-loopfilter", yPlane, width, width, height)
	debugTraceInterFrameChromaSample16(parsed, "post-loopfilter", "U", uPlane, chromaWidth, chromaWidth, chromaHeight)
	debugTraceInterFrameChromaSample16(parsed, "post-loopfilter", "V", vPlane, chromaWidth, chromaWidth, chromaHeight)
	applyCDEF420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, d.header.ColorConfig.BitDepth, cdef)
	debugTraceInterFrameLumaSample16(parsed, "post-cdef", yPlane, width, width, height)
	debugTraceInterFrameChromaSample16(parsed, "post-cdef", "U", uPlane, chromaWidth, chromaWidth, chromaHeight)
	debugTraceInterFrameChromaSample16(parsed, "post-cdef", "V", vPlane, chromaWidth, chromaWidth, chromaHeight)
	applyRestoration420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, restoration, d.header.ColorConfig.BitDepth)
	debugTraceInterFrameLumaSample16(parsed, "post-restoration", yPlane, width, width, height)
	debugTraceInterFrameChromaSample16(parsed, "post-restoration", "U", uPlane, chromaWidth, chromaWidth, chromaHeight)
	debugTraceInterFrameChromaSample16(parsed, "post-restoration", "V", vPlane, chromaWidth, chromaWidth, chromaHeight)

	return buildFrameFromPlanar16NoPreview(width, height, d.header.ColorConfig, parsed.PTS, parsed.Duration, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth)
}

func cloneReferenceLuma16(ref *Frame, width, height int, bitDepth uint8) []uint16 {
	plane := make([]uint16, width*height)
	if ref == nil {
		return plane
	}
	switch {
	case len(ref.Y16) != 0:
		srcStride := ref.YStride
		if srcStride <= 0 {
			srcStride = width
		}
		if srcStride >= width && len(ref.Y16) >= (height-1)*srcStride+width {
			for y := 0; y < height; y++ {
				copy(plane[y*width:y*width+width], ref.Y16[y*srcStride:y*srcStride+width])
			}
		}
	case len(ref.Y) != 0:
		shift := maxIntLocal(int(bitDepth)-8, 0)
		srcStride := ref.YStride
		if srcStride <= 0 {
			srcStride = width
		}
		if srcStride >= width && len(ref.Y) >= (height-1)*srcStride+width {
			for y := 0; y < height; y++ {
				dstRow := plane[y*width : y*width+width]
				srcRow := ref.Y[y*srcStride : y*srcStride+width]
				for x := 0; x < width; x++ {
					dstRow[x] = uint16(srcRow[x]) << shift
				}
			}
		}
	}
	return plane
}

func cloneReferenceChroma16(ref *Frame, planeIndex int, width, height int, bitDepth uint8) []uint16 {
	plane := make([]uint16, width*height)
	if ref == nil {
		fillUint16(plane, 1<<(bitDepth-1))
		return plane
	}
	switch planeIndex {
	case 1:
		if len(ref.U16) != 0 {
			srcStride := ref.UStride
			if srcStride <= 0 {
				srcStride = width
			}
			if srcStride >= width && len(ref.U16) >= (height-1)*srcStride+width {
				for y := 0; y < height; y++ {
					copy(plane[y*width:y*width+width], ref.U16[y*srcStride:y*srcStride+width])
				}
			}
			return plane
		}
		if len(ref.U) != 0 {
			shift := maxIntLocal(int(bitDepth)-8, 0)
			srcStride := ref.UStride
			if srcStride <= 0 {
				srcStride = width
			}
			if srcStride >= width && len(ref.U) >= (height-1)*srcStride+width {
				for y := 0; y < height; y++ {
					dstRow := plane[y*width : y*width+width]
					srcRow := ref.U[y*srcStride : y*srcStride+width]
					for x := 0; x < width; x++ {
						dstRow[x] = uint16(srcRow[x]) << shift
					}
				}
			}
			return plane
		}
	case 2:
		if len(ref.V16) != 0 {
			srcStride := ref.VStride
			if srcStride <= 0 {
				srcStride = width
			}
			if srcStride >= width && len(ref.V16) >= (height-1)*srcStride+width {
				for y := 0; y < height; y++ {
					copy(plane[y*width:y*width+width], ref.V16[y*srcStride:y*srcStride+width])
				}
			}
			return plane
		}
		if len(ref.V) != 0 {
			shift := maxIntLocal(int(bitDepth)-8, 0)
			srcStride := ref.VStride
			if srcStride <= 0 {
				srcStride = width
			}
			if srcStride >= width && len(ref.V) >= (height-1)*srcStride+width {
				for y := 0; y < height; y++ {
					dstRow := plane[y*width : y*width+width]
					srcRow := ref.V[y*srcStride : y*srcStride+width]
					for x := 0; x < width; x++ {
						dstRow[x] = uint16(srcRow[x]) << shift
					}
				}
			}
			return plane
		}
	}
	fillUint16(plane, 1<<(bitDepth-1))
	return plane
}

func referenceLuma16(ref *Frame, bitDepth uint8) ([]uint16, int, int, int) {
	if ref == nil {
		return nil, 0, 0, 0
	}
	width := ref.Width
	height := ref.Height
	stride := ref.YStride
	if stride <= 0 {
		stride = width
	}
	if len(ref.Y16) != 0 && stride >= width && len(ref.Y16) >= (height-1)*stride+width {
		return ref.Y16, stride, width, height
	}
	return cloneReferenceLuma16(ref, width, height, bitDepth), width, width, height
}

func referenceChroma16(ref *Frame, planeIndex int, bitDepth uint8) ([]uint16, int, int, int) {
	if ref == nil {
		return nil, 0, 0, 0
	}
	width := (ref.Width + 1) >> 1
	height := (ref.Height + 1) >> 1
	var plane []uint16
	var stride int
	switch planeIndex {
	case 1:
		plane = ref.U16
		stride = ref.UStride
	case 2:
		plane = ref.V16
		stride = ref.VStride
	}
	if stride <= 0 {
		stride = width
	}
	if len(plane) != 0 && stride >= width && len(plane) >= (height-1)*stride+width {
		return plane, stride, width, height
	}
	return cloneReferenceChroma16(ref, planeIndex, width, height, bitDepth), width, width, height
}

func (d *Decoder) decodeInterFrameMain10(parsed *ParsedFrame, dq DequantTables, plane []uint16, stride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, field *SpatialMVTile, temporal *TemporalMVMap, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState) error {
	states, err := d.buildPureGoTileStates(parsed)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	planeWidth := parsed.Header.Width
	planeHeight := parsed.Header.Height
	chromaWidth := (parsed.Header.Width + 1) >> 1
	chromaHeight := (parsed.Header.Height + 1) >> 1
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
				if err := d.decodeInterSuperBlock10(parsed, &row, &mvTile, field, temporal, sb, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, chromaWidth, chromaHeight, cdef, edges, segState); err != nil {
					return err
				}
			}
		}
	}
	d.capturePureGoCDFState(states)
	return nil
}

func (d *Decoder) decodeInterSuperBlock10(parsed *ParsedFrame, row *TileRowSyntaxState, mvTile *SpatialMVTile, field *SpatialMVTile, temporal *TemporalMVMap, sb SuperBlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState) error {
	row.Tile.ResetSuperBlockDeltas(&parsed.Header)
	root := BlockGeometry{
		Level:   RootBlockLevel(d.header),
		Size:    RootBlockSize(d.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return d.decodeInterBlock10(parsed, row, mvTile, field, temporal, sb, root, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, edges, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root))
}

func (d *Decoder) decodeInterBlock10(parsed *ParsedFrame, row *TileRowSyntaxState, mvTile *SpatialMVTile, field *SpatialMVTile, temporal *TemporalMVMap, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, edge intraEdgeAvailability) error {
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
				if err := d.decodeInterBlock10(parsed, row, mvTile, field, temporal, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, edges, segState, childEdges[i]); err != nil {
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
				if err := d.decodeInterBlock10(parsed, row, mvTile, field, temporal, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, edges, segState, childEdges[i]); err != nil {
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
			if err := d.decodeInterLeaf10(parsed, row, mvTile, field, temporal, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, edges, segState, childEdges[i]); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := d.decodeInterLeaf10(parsed, row, mvTile, field, temporal, g, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, edges, segState, edge); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := d.decodeInterLeaf10(parsed, row, mvTile, field, temporal, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, edges, segState, childEdges[i]); err != nil {
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

func (d *Decoder) decodeInterLeaf10(parsed *ParsedFrame, row *TileRowSyntaxState, mvTile *SpatialMVTile, field *SpatialMVTile, temporal *TemporalMVMap, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, edge intraEdgeAvailability) error {
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
	skipModeEligible := parsed.Header.SkipModeEnabled && minInt(int(preludeDims.W4), int(preludeDims.H4)) > 1 && segmentDataForID(&parsed.Header, segID).Ref < 0 && !segmentDataForID(&parsed.Header, segID).GlobalMV && !segmentDataForID(&parsed.Header, segID).Skip
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
	intra, err := ReadInterBlockIntraFlagWithNeighbors(&parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, segID, syntax.SkipMode, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	syntax.Intra = intra
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
		if !syntax.Skip && shouldUseDav1dIntraCoeffOrder(d.header, *syntax.IntraBlk) {
			if _, err := reconstructParsedIntraBlockAt16Dav1dOrder(
				d.header,
				&parsed.Header,
				row.Tile.Tile.CDF,
				row.Tile.Tile.Coef,
				dq,
				above,
				&row.Left,
				*syntax.IntraBlk,
				plane,
				stride,
				planeWidth,
				planeHeight,
				uPlane,
				uStride,
				uWidth,
				uHeight,
				vPlane,
				vStride,
				vWidth,
				vHeight,
				d.header.ColorConfig.BitDepth,
				row.Tile.Tile.Entropy,
				reconHaveTop,
				reconHaveLeft,
				edge,
			); err != nil {
				return err
			}
		} else {
			if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(d.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, above, &row.Left, *syntax.IntraBlk, plane, stride, planeWidth, planeHeight, g.Start4X, g.Start4Y, d.header.ColorConfig.BitDepth, syntax.Skip, row.Tile.Tile.Entropy, reconHaveTop, reconHaveLeft, edge.topHasRight, edge.leftHasBottom); err != nil {
				return err
			}
			if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(d.header, &parsed.Header, row.Tile.Tile.Coef, dq, above, &row.Left, *syntax.IntraBlk, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, d.header.ColorConfig.BitDepth, syntax.Skip, row.Tile.Tile.Entropy, chromaReconHaveTop, chromaReconHaveLeft, chromaEdge.topHasRight, chromaEdge.leftHasBottom); err != nil {
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
		refPlane0, refStride0, refWidth0, refHeight0 := referenceLuma16(refFrame0, d.header.ColorConfig.BitDepth)
		refPlane1, refStride1, refWidth1, refHeight1 := referenceLuma16(refFrame1, d.header.ColorConfig.BitDepth)
		if err := PredictWeightedCompoundInterLumaBlock16WithFilters(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refPlane0, refStride0, refWidth0, refHeight0, mv0, syntax.Filter, refPlane1, refStride1, refWidth1, refHeight1, mv1, syntax.Filter, d.header.ColorConfig.BitDepth, 8); err != nil {
			return err
		}
		debugTraceInterFrameLumaSample16(parsed, "post-skipmode-luma-mc", plane, stride, planeWidth, planeHeight)
		if hasChroma {
			chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
			refU0, refUStride0, refUWidth0, refUHeight0 := referenceChroma16(refFrame0, 1, d.header.ColorConfig.BitDepth)
			refU1, refUStride1, refUWidth1, refUHeight1 := referenceChroma16(refFrame1, 1, d.header.ColorConfig.BitDepth)
			refV0, refVStride0, refVWidth0, refVHeight0 := referenceChroma16(refFrame0, 2, d.header.ColorConfig.BitDepth)
			refV1, refVStride1, refVWidth1, refVHeight1 := referenceChroma16(refFrame1, 2, d.header.ColorConfig.BitDepth)
			if err := PredictWeightedCompoundInterChromaBlock16WithFilters(uPlane, uStride, uWidth, uHeight, chromaX, chromaY, chromaW, chromaH, refU0, refUStride0, refUWidth0, refUHeight0, mv0, syntax.Filter, refU1, refUStride1, refUWidth1, refUHeight1, mv1, syntax.Filter, d.header.ColorConfig.BitDepth, 8); err != nil {
				return err
			}
			if err := PredictWeightedCompoundInterChromaBlock16WithFilters(vPlane, vStride, vWidth, vHeight, chromaX, chromaY, chromaW, chromaH, refV0, refVStride0, refVWidth0, refVHeight0, mv0, syntax.Filter, refV1, refVStride1, refVWidth1, refVHeight1, mv1, syntax.Filter, d.header.ColorConfig.BitDepth, 8); err != nil {
				return err
			}
		}
		unitsScratch := takeTransformUnitScratch(local)
		defer putTransformUnitScratch(unitsScratch)
		units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, g, syntax.Segment, true, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
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
		if !syntax.Skip {
			markInterTransformEdgesFromUnits(edges, units, nil)
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
	if isComp {
		ref0, ref1, err := ReadCompoundRefSelection(row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, ctxHaveTop, ctxHaveLeft, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
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
		mode0, mode1, drl, err := ReadCompoundInterMode(row.Tile.Tile.CDF, mvModeCtx, nMVs, mvWeights, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
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
		syntax.Filter, err = ReadInterFilterPairForCompound(d.header, &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, ref0, ref1, mode0, mode1, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}

		refFrame0 := d.resolvePureGoBlockReference(&parsed.Header, ref0)
		refFrame1 := d.resolvePureGoBlockReference(&parsed.Header, ref1)
		refPlane0, refStride0, refWidth0, refHeight0 := referenceLuma16(refFrame0, d.header.ColorConfig.BitDepth)
		refPlane1, refStride1, refWidth1, refHeight1 := referenceLuma16(refFrame1, d.header.ColorConfig.BitDepth)
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
			debugTraceCompoundLumaPrediction16(parsed, g, refPlane0, refStride0, refWidth0, refHeight0, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refPlane1, refStride1, refWidth1, refHeight1, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, d.header.ColorConfig.BitDepth, useWeight)
			if err := PredictWeightedCompoundInterLumaBlock16Motion(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refPlane0, refStride0, refWidth0, refHeight0, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refPlane1, refStride1, refWidth1, refHeight1, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, d.header.ColorConfig.BitDepth, useWeight); err != nil {
				return err
			}
			debugTraceInterFrameLumaSample16(parsed, "post-comp-luma-mc", plane, stride, planeWidth, planeHeight)
			if hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
				chromaWarp0, useChromaWarp0 := globalMotionWarpForPlane(&parsed.Header, ref0, 1, d.header.ColorConfig.ChromaSubsampling())
				chromaWarp1, useChromaWarp1 := globalMotionWarpForPlane(&parsed.Header, ref1, 1, d.header.ColorConfig.ChromaSubsampling())
				useChromaWarp0 = useChromaWarp0 && mode0 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref0)
				useChromaWarp1 = useChromaWarp1 && mode1 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref1)
				chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
				refU0, refUStride0, refUWidth0, refUHeight0 := referenceChroma16(refFrame0, 1, d.header.ColorConfig.BitDepth)
				refU1, refUStride1, refUWidth1, refUHeight1 := referenceChroma16(refFrame1, 1, d.header.ColorConfig.BitDepth)
				refV0, refVStride0, refVWidth0, refVHeight0 := referenceChroma16(refFrame0, 2, d.header.ColorConfig.BitDepth)
				refV1, refVStride1, refVWidth1, refVHeight1 := referenceChroma16(refFrame1, 2, d.header.ColorConfig.BitDepth)
				if err := PredictWeightedCompoundInterChromaBlock16Motion(uPlane, uStride, uWidth, uHeight, chromaX, chromaY, chromaW, chromaH, refU0, refUStride0, refUWidth0, refUHeight0, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refU1, refUStride1, refUWidth1, refUHeight1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, d.header.ColorConfig.BitDepth, useWeight); err != nil {
					return err
				}
				if err := PredictWeightedCompoundInterChromaBlock16Motion(vPlane, vStride, vWidth, vHeight, chromaX, chromaY, chromaW, chromaH, refV0, refVStride0, refVWidth0, refVHeight0, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refV1, refVStride1, refVWidth1, refVHeight1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, d.header.ColorConfig.BitDepth, useWeight); err != nil {
					return err
				}
			}
		case compoundTypeSeg, compoundTypeWedge:
			var mask []uint8
			nominalW, nominalH := blockSampleDimensions(syntax.Size)
			if syntax.CompType == compoundTypeSeg {
				mask, err = PredictDiffWeightedCompoundInterLumaBlock16Motion(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, nominalW, nominalH, refPlane0, refStride0, refWidth0, refHeight0, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refPlane1, refStride1, refWidth1, refHeight1, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, d.header.ColorConfig.BitDepth, syntax.MaskSign)
			} else {
				mask = buildCompoundMask(nominalW, nominalH, syntax.CompType, syntax.WedgeIdx, syntax.MaskSign)
				err = PredictMaskedCompoundInterLumaBlock16Motion(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, nominalW, nominalH, refPlane0, refStride0, refWidth0, refHeight0, mv0, syntax.Filter, useLumaWarp0, lumaWarp0, refPlane1, refStride1, refWidth1, refHeight1, mv1, syntax.Filter, useLumaWarp1, lumaWarp1, d.header.ColorConfig.BitDepth, mask, nominalW)
			}
			if err != nil {
				return err
			}
			debugTraceInterFrameLumaSample16(parsed, "post-comp-luma-mc", plane, stride, planeWidth, planeHeight)
			if hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
				chromaWarp0, useChromaWarp0 := globalMotionWarpForPlane(&parsed.Header, ref0, 1, d.header.ColorConfig.ChromaSubsampling())
				chromaWarp1, useChromaWarp1 := globalMotionWarpForPlane(&parsed.Header, ref1, 1, d.header.ColorConfig.ChromaSubsampling())
				useChromaWarp0 = useChromaWarp0 && mode0 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref0)
				useChromaWarp1 = useChromaWarp1 && mode1 == InterPredGlobal && globalMotionUsesWarp(&parsed.Header, ref1)
				chromaX, chromaY, _, _ := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
				chromaNominalW, chromaNominalH := blockChromaSampleDimensions(d.header.ColorConfig.ChromaSubsampling(), syntax.Size)
				cmask := subsampleCompoundMask(mask, nominalW, nominalH, d.header.ColorConfig.ChromaSubsampling(), syntax.MaskSign)
				refU0, refUStride0, refUWidth0, refUHeight0 := referenceChroma16(refFrame0, 1, d.header.ColorConfig.BitDepth)
				refU1, refUStride1, refUWidth1, refUHeight1 := referenceChroma16(refFrame1, 1, d.header.ColorConfig.BitDepth)
				refV0, refVStride0, refVWidth0, refVHeight0 := referenceChroma16(refFrame0, 2, d.header.ColorConfig.BitDepth)
				refV1, refVStride1, refVWidth1, refVHeight1 := referenceChroma16(refFrame1, 2, d.header.ColorConfig.BitDepth)
				if err := PredictMaskedCompoundInterChromaBlock16Motion(uPlane, uStride, uWidth, uHeight, chromaX, chromaY, chromaNominalW, chromaNominalH, refU0, refUStride0, refUWidth0, refUHeight0, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refU1, refUStride1, refUWidth1, refUHeight1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, d.header.ColorConfig.BitDepth, cmask, chromaNominalW); err != nil {
					return err
				}
				if err := PredictMaskedCompoundInterChromaBlock16Motion(vPlane, vStride, vWidth, vHeight, chromaX, chromaY, chromaNominalW, chromaNominalH, refV0, refVStride0, refVWidth0, refVHeight0, mv0, syntax.Filter, useChromaWarp0, chromaWarp0, refV1, refVStride1, refVWidth1, refVHeight1, mv1, syntax.Filter, useChromaWarp1, chromaWarp1, d.header.ColorConfig.BitDepth, cmask, chromaNominalW); err != nil {
					return err
				}
			}
		}

		unitsScratch := takeTransformUnitScratch(local)
		defer putTransformUnitScratch(unitsScratch)
		debugTraceInterMSAC(parsed, g, "pre-inter-tx-units", row.Tile.Tile.Entropy)
		units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, g, syntax.Segment, syntax.Skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		debugTraceInterMSAC(parsed, g, "post-inter-tx-units", row.Tile.Tile.Entropy)
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
				skipCDF := row.Tile.Tile.Coef.Skip[txInfo.Ctx][skipCtx][:]
				preSkipCDF := append([]uint16(nil), skipCDF...)
				preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
				skipBit := row.Tile.Tile.Entropy.DecodeBoolAdapt(skipCDF)
				postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
				debugTraceInterLumaCoeffSkip(parsed, tu.Global, int(txInfo.Ctx), skipCtx, skipBit,
					[4]int{int(preCur), int(preRng), preCnt, prePos},
					[4]int{int(postCur), int(postRng), postCnt, postPos},
					preSkipCDF, append([]uint16(nil), skipCDF...))
				var (
					yTxType TxType
					coeffs  []int32
					ctx     uint8
					err     error
					zero    *scratchBuffer[int32]
				)
				if skipBit != 0 {
					ctx = 0x40
					zero = takeZeroCoeffScratch(tu.TX)
					coeffs = zero.buf
					if int(syntax.Segment) < len(parsed.Header.Segmentation.Lossless) && parsed.Header.Segmentation.Lossless[syntax.Segment] {
						yTxType = TxTypeWHTWHT
					} else {
						yTxType = TxTypeDCTDCT
					}
				} else {
					yTxType, err = ReadInterTxType(&parsed.Header, row.Tile.Tile.CDF, syntax.Segment, tu.TX, row.Tile.Tile.Entropy)
					if err != nil {
						return err
					}
					coeffs, ctx, _, err = DecodeLumaCoefficientsNoSkipWithBitDepth(syntax.Size, tu.Local, tu.TX, yTxType, syntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, d.header.ColorConfig.BitDepth)
					if err != nil {
						return err
					}
				}
				debugTraceInterLumaCoeffDecoded(parsed, tu.Global, yTxType, ctx, coeffs, row.Tile.Tile.Entropy)
				if err := ReconstructInterLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.TX, yTxType, coeffs, d.header.ColorConfig.BitDepth, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
					if zero != nil {
						putCoeffScratch(zero)
					}
					return err
				}
				if zero != nil {
					putCoeffScratch(zero)
				}
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
			decodeChromaPlaneTU := func(planeIdx int, tu BlockGeometry) error {
				resCtx := uint8(0x40)
				if !syntax.Skip {
					yTxType := blockTxTypeForChromaTU(lumaTxTypes, g, chromaGeom, tu, layout)
					uvTxType := InterUVTxType(uvtx, yTxType)
					coeffs, ctx, _, err := DecodeChromaCoefficientsWithBitDepth(syntax.Size, tu, uvtx, uvTxType, syntax.Segment, planeIdx, layout, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, d.header.ColorConfig.BitDepth)
					if err != nil {
						return err
					}
					dst, dstStride, dstWidth, dstHeight := uPlane, uStride, uWidth, uHeight
					if planeIdx == 2 {
						dst, dstStride, dstWidth, dstHeight = vPlane, vStride, vWidth, vHeight
					}
					if err := ReconstructInterChromaCoefficients16WithTxType(dst, dstStride, dstWidth, dstHeight, tu.Start4X*4, tu.Start4Y*4, uvtx, uvTxType, coeffs, d.header.ColorConfig.BitDepth, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
						return err
					}
					resCtx = ctx
				}
				if err := ApplyChromaResidualContextWithLayout(above, &row.Left, planeIdx, layout, tu, uvtx, resCtx); err != nil {
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
					for planeIdx := 1; planeIdx <= 2; planeIdx++ {
						for _, tu := range chromaUnits {
							if geometryContainsStart(chromaChunk, tu) {
								if err := decodeChromaPlaneTU(planeIdx, tu); err != nil {
									return err
								}
								debugTraceInterResidualStep(parsed, g, planeIdx, tu, "post-chroma-tu", row.Tile.Tile.Entropy)
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
		ref0, err = ReadSingleRefIndex(row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, ctxHaveTop, ctxHaveLeft, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
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
	cands, mvModeCtx := mvTile.findSingleRefCandidatesWithEdgeAndTemporalInto(candsDst, &parsed.Header, g, ref0, edge, temporalCands)
	debugTraceSingleRefStack(parsed, g, "base", ref0, mvModeCtx, cands)
	if parsed.Header.UseRefFrameMvs && temporal != nil && !debugDisableInterTemporalSingle() {
		if globalCtx, ok := temporal.projectedGlobalMVContext(d.header, parsed, ref0, &parsed.Header, g); ok {
			mvModeCtx = (mvModeCtx &^ (1 << 3)) | (globalCtx << 3)
		}
		debugTraceSingleRefStack(parsed, g, "after-temporal", ref0, mvModeCtx, cands)
	}
	cands = augmentSingleRefCandidatesExtended(cands, mvTile, d.header, parsed, &parsed.Header, g, ref0)
	debugTraceSingleRefStack(parsed, g, "after-extended", ref0, mvModeCtx, cands)
	debugTraceInterMVCandidates(parsed, g, ref0, mvModeCtx, cands, temporalCands)
	nMVs := activeRefMVCandidateCount(cands)
	var mvWeightsBuf [16]int
	var mvWeights []int
	if len(cands) > len(mvWeightsBuf) {
		mvWeights = make([]int, len(cands))
	} else {
		mvWeights = mvWeightsBuf[:len(cands)]
	}
	for i := range cands {
		mvWeights[i] = cands[i].Weight
	}
	var mode InterPredMode
	var drl uint8
	if seg.Skip || seg.GlobalMV {
		mode = InterPredGlobal
		drl = drlNearest
	} else {
		mode, drl, err = ReadSingleRefInterModeTraced(row.Tile.Tile.CDF, mvModeCtx, nMVs, mvWeights, row.Tile.Tile.Entropy, parsed, g)
		if err != nil {
			return err
		}
	}
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
	refPlane, refStride, refWidth, refHeight := referenceLuma16(refFrame, d.header.ColorConfig.BitDepth)
	if syntax.Motion == interMotionWarp {
		warp, ok := deriveLocalAffineWarp(mvTile, g, ref0, mv, edge)
		if ok {
			debugTraceInterWarp(parsed, g, warp)
			if err := PredictInterLumaBlock16Warp(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refPlane, refStride, refWidth, refHeight, warp, d.header.ColorConfig.BitDepth); err != nil {
				return err
			}
		} else {
			if err := PredictInterLumaBlock16WithFilters(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refPlane, refStride, refWidth, refHeight, mv, d.header.ColorConfig.BitDepth, syntax.Filter); err != nil {
				return err
			}
		}
	} else if useLumaGlobalWarp && mode == InterPredGlobal {
		if err := PredictInterLumaBlock16Warp(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refPlane, refStride, refWidth, refHeight, lumaGlobalWarp, d.header.ColorConfig.BitDepth); err != nil {
			return err
		}
	} else {
		if err := PredictInterLumaBlock16WithFilters(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, refPlane, refStride, refWidth, refHeight, mv, d.header.ColorConfig.BitDepth, syntax.Filter); err != nil {
			return err
		}
	}
	debugTraceInterFrameLumaSample16(parsed, "post-single-luma-mc", plane, stride, planeWidth, planeHeight)
	debugTraceSingleRefPrediction16(parsed, g, refSlot, debugPureGoSlotPOC(d.pureGoRefPOC, refSlot), refFrame, refPlane, refStride, refWidth, refHeight, mv, syntax.Filter, plane, stride, planeWidth, planeHeight)
	if syntax.Motion == interMotionOBMC {
		if err := d.applyInterOBMCLuma16(parsed, mvTile, g, plane, stride, planeWidth, planeHeight); err != nil {
			return err
		}
		debugTraceInterFrameLumaSample16(parsed, "post-single-obmc", plane, stride, planeWidth, planeHeight)
	}
	if syntax.InterIntraType != interIntraNone {
		blockW, blockH := blockSampleDimensions(syntax.Size)
		if err := blendInterIntraBlock16(plane, stride, planeWidth, planeHeight, g.Start4X*4, g.Start4Y*4, blockW, blockH, syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx, d.header.ColorConfig.BitDepth); err != nil {
			return err
		}
		debugTraceInterFrameLumaSample16(parsed, "post-single-interintra", plane, stride, planeWidth, planeHeight)
	}
	hasChroma = hasChromaPlane(d.header, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y)
	if hasChroma {
		refU, refUStride, refUWidth, refUHeight := referenceChroma16(refFrame, 1, d.header.ColorConfig.BitDepth)
		refV, refVStride, refVWidth, refVHeight := referenceChroma16(refFrame, 2, d.header.ColorConfig.BitDepth)
		_, chromaGlobalWarp, useChromaGlobalWarp := singleRefGlobalMotionForPlane(&parsed.Header, ref0, 1, d.header.ColorConfig.ChromaSubsampling(), g)
		useChromaGlobalWarp = useChromaGlobalWarp && globalMotionUsesWarp(&parsed.Header, ref0)
		chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(d.header.ColorConfig.ChromaSubsampling(), g)
		if useChromaGlobalWarp && mode == InterPredGlobal {
			if err := PredictInterChromaBlock16Warp(uPlane, uStride, uWidth, uHeight, chromaX, chromaY, chromaW, chromaH, refU, refUStride, refUWidth, refUHeight, chromaGlobalWarp, d.header.ColorConfig.BitDepth); err != nil {
				return err
			}
			if err := PredictInterChromaBlock16Warp(vPlane, vStride, vWidth, vHeight, chromaX, chromaY, chromaW, chromaH, refV, refVStride, refVWidth, refVHeight, chromaGlobalWarp, d.header.ColorConfig.BitDepth); err != nil {
				return err
			}
		} else {
			if err := PredictInterChromaBlock16WithFilters(uPlane, uStride, uWidth, uHeight, chromaX, chromaY, chromaW, chromaH, refU, refUStride, refUWidth, refUHeight, mv, d.header.ColorConfig.BitDepth, syntax.Filter); err != nil {
				return err
			}
			if err := PredictInterChromaBlock16WithFilters(vPlane, vStride, vWidth, vHeight, chromaX, chromaY, chromaW, chromaH, refV, refVStride, refVWidth, refVHeight, mv, d.header.ColorConfig.BitDepth, syntax.Filter); err != nil {
				return err
			}
			if syntax.Motion == interMotionOBMC {
				if err := d.applyInterOBMCChroma16(parsed, mvTile, g, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight); err != nil {
					return err
				}
			}
		}
		if syntax.InterIntraType != interIntraNone {
			layout := d.header.ColorConfig.ChromaSubsampling()
			chromaBlockW, chromaBlockH := blockChromaSampleDimensions(layout, syntax.Size)
			if err := blendInterIntraChromaBlock16(uPlane, uStride, uWidth, uHeight, chromaX, chromaY, chromaBlockW, chromaBlockH, layout, syntax.Size, syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx, d.header.ColorConfig.BitDepth); err != nil {
				return err
			}
			if err := blendInterIntraChromaBlock16(vPlane, vStride, vWidth, vHeight, chromaX, chromaY, chromaBlockW, chromaBlockH, layout, syntax.Size, syntax.InterIntraType, syntax.InterIntraMode, syntax.WedgeIdx, d.header.ColorConfig.BitDepth); err != nil {
				return err
			}
		}
	}

	unitsScratch := takeTransformUnitScratch(local)
	defer putTransformUnitScratch(unitsScratch)
	debugTraceInterMSAC(parsed, g, "pre-inter-tx-units", row.Tile.Tile.Entropy)
	units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], &parsed.Header, row.Tile.Tile.CDF, above, &row.Left, local, g, syntax.Segment, syntax.Skip, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	debugTraceInterMSAC(parsed, g, "post-inter-tx-units", row.Tile.Tile.Entropy)
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
			skipCDF := row.Tile.Tile.Coef.Skip[txInfo.Ctx][skipCtx][:]
			preSkipCDF := append([]uint16(nil), skipCDF...)
			preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
			skipBit := row.Tile.Tile.Entropy.DecodeBoolAdapt(skipCDF)
			postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
			debugTraceInterLumaCoeffSkip(parsed, tu.Global, int(txInfo.Ctx), skipCtx, skipBit,
				[4]int{int(preCur), int(preRng), preCnt, prePos},
				[4]int{int(postCur), int(postRng), postCnt, postPos},
				preSkipCDF, append([]uint16(nil), skipCDF...))
			var (
				yTxType TxType
				coeffs  []int32
				ctx     uint8
				err     error
				zero    *scratchBuffer[int32]
			)
			if skipBit != 0 {
				ctx = 0x40
				zero = takeZeroCoeffScratch(tu.TX)
				coeffs = zero.buf
				if int(syntax.Segment) < len(parsed.Header.Segmentation.Lossless) && parsed.Header.Segmentation.Lossless[syntax.Segment] {
					yTxType = TxTypeWHTWHT
				} else {
					yTxType = TxTypeDCTDCT
				}
			} else {
				yTxType, err = ReadInterTxType(&parsed.Header, row.Tile.Tile.CDF, syntax.Segment, tu.TX, row.Tile.Tile.Entropy)
				if err != nil {
					return err
				}
				coeffs, ctx, _, err = DecodeLumaCoefficientsNoSkipWithBitDepth(syntax.Size, tu.Local, tu.TX, yTxType, syntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, d.header.ColorConfig.BitDepth)
				if err != nil {
					return err
				}
			}
			debugTraceInterLumaCoeffDecoded(parsed, tu.Global, yTxType, ctx, coeffs, row.Tile.Tile.Entropy)
			if err := ReconstructInterLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.TX, yTxType, coeffs, d.header.ColorConfig.BitDepth, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
				if zero != nil {
					putCoeffScratch(zero)
				}
				return err
			}
			debugTraceInterFrameLumaSample16(parsed, "post-luma-tu", plane, stride, planeWidth, planeHeight)
			if zero != nil {
				putCoeffScratch(zero)
			}
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
		chromaGeom := chromaGeometryForBlock(layout, g)
		countX, countY := transformGridCounts(chromaGeom, uvtx)
		var chromaUnitsBuf [16]BlockGeometry
		chromaDst, chromaScratch := takeBlockGeometryBuffer(chromaUnitsBuf[:0], countX*countY)
		defer putBlockGeometryBuffer(chromaScratch)
		chromaUnits := buildChromaTransformUnitsInto(chromaDst, layout, g, uvtx)
		decodeChromaPlaneTU := func(planeIdx int, tu BlockGeometry) error {
			resCtx := uint8(0x40)
			if !syntax.Skip {
				yTxType := blockTxTypeForChromaTU(lumaTxTypes, g, chromaGeom, tu, layout)
				uvTxType := InterUVTxType(uvtx, yTxType)
				coeffs, ctx, _, err := DecodeChromaCoefficientsWithBitDepth(syntax.Size, tu, uvtx, uvTxType, syntax.Segment, planeIdx, layout, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, d.header.ColorConfig.BitDepth)
				if err != nil {
					return err
				}
				dst, dstStride, dstWidth, dstHeight := uPlane, uStride, uWidth, uHeight
				if planeIdx == 2 {
					dst, dstStride, dstWidth, dstHeight = vPlane, vStride, vWidth, vHeight
				}
				if err := ReconstructInterChromaCoefficients16WithTxType(dst, dstStride, dstWidth, dstHeight, tu.Start4X*4, tu.Start4Y*4, uvtx, uvTxType, coeffs, d.header.ColorConfig.BitDepth, parsed.Header.Segmentation.Lossless[syntax.Segment]); err != nil {
					return err
				}
				resCtx = ctx
			}
			if err := ApplyChromaResidualContextWithLayout(above, &row.Left, planeIdx, d.header.ColorConfig.ChromaSubsampling(), tu, uvtx, resCtx); err != nil {
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
				for planeIdx := 1; planeIdx <= 2; planeIdx++ {
					for _, tu := range chromaUnits {
						if geometryContainsStart(chromaChunk, tu) {
							if err := decodeChromaPlaneTU(planeIdx, tu); err != nil {
								return err
							}
							debugTraceInterResidualStep(parsed, g, planeIdx, tu, "post-chroma-tu", row.Tile.Tile.Entropy)
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
	}
	if err := ApplyInterBlockContext(d.header, &parsed.Header, above, &row.Left, syntax); err != nil {
		return err
	}
	interIntra := syntax.InterIntraType != interIntraNone
	mvTile.SplatSingleRefWithInterIntra(g, ref0, refSlot, mv, mode, syntax.Filter, interIntra)
	field.SplatSingleRefWithInterIntra(g, ref0, refSlot, mv, mode, syntax.Filter, interIntra)
	return nil
}
