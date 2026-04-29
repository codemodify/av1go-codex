package decoder

import (
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func (d *Decoder) decodeCurrentPureGoFrameMain10(parsed *ParsedFrame) (*Frame, error) {
	switch parsed.Header.FrameType {
	case obu.FrameTypeKey, obu.FrameTypeIntra:
		return d.decodeCurrentPureGoKeyframe10(parsed)
	case obu.FrameTypeInter, obu.FrameTypeSwitch:
		return d.decodeCurrentPureGoInterFrame10(parsed)
	default:
		return d.clonePureGoReferenceFrame(parsed)
	}
}

func (d *Decoder) decodeCurrentPureGoKeyframe10(parsed *ParsedFrame) (*Frame, error) {
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, d.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	currentMV := (*SpatialMVTile)(nil)
	if parsed.Header.AllowIntrabc {
		currentMV = takeFrameMVField(parsed.Header.Width, parsed.Header.Height)
	}
	d.setCurrentPureGoState(currentMV, segState.Current)

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
	if parsed.Header.AllowIntrabc {
		return d.decodeCurrentPureGoIntraBCFrame10(parsed, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, cdef, restoration, segState)
	}
	if err := decodeKeyframeMain10WithCDFCapture(d.header, &parsed.Header, &parsed.TileGroup, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, cdef, restoration, segState, d.capturePureGoCDFState); err != nil {
		return nil, err
	}
	applySimpleLoopFilter420_16Masked(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, d.header.ColorConfig.BitDepth, cdef)
	applyCDEF420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, d.header.ColorConfig.BitDepth, cdef)
	applyRestoration420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, restoration, d.header.ColorConfig.BitDepth)

	return buildFrameFromPlanar16NoPreview(width, height, d.header.ColorConfig, parsed.PTS, parsed.Duration, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth)
}

func (d *Decoder) decodeCurrentPureGoIntraBCFrame10(parsed *ParsedFrame, dq DequantTables, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, restoration *RestorationState, segState *SegmentationState) (*Frame, error) {
	if err := decodeKeyframeMain10WithIntraBC(d.header, &parsed.Header, &parsed.TileGroup, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, d.currentMVField); err != nil {
		return nil, err
	}
	width := parsed.Header.Width
	height := parsed.Header.Height
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	applySimpleLoopFilter420_16Masked(yPlane, yStride, width, height, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, &parsed.Header, d.header.ColorConfig.BitDepth, cdef)
	applyCDEF420_16(yPlane, yStride, width, height, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, &parsed.Header, d.header.ColorConfig.BitDepth, cdef)
	applyRestoration420_16(yPlane, yStride, width, height, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, &parsed.Header, restoration, d.header.ColorConfig.BitDepth)
	return buildFrameFromPlanar16NoPreview(width, height, d.header.ColorConfig, parsed.PTS, parsed.Duration, yPlane, yStride, uPlane, uStride, vPlane, vStride)
}

func decodeKeyframeMain10(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, plane []uint16, stride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, restoration *RestorationState, segState *SegmentationState) error {
	return decodeKeyframeMain10WithCDFCapture(seq, hdr, group, dq, plane, stride, uPlane, uStride, vPlane, vStride, cdef, restoration, segState, nil)
}

func decodeKeyframeMain10WithCDFCapture(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, plane []uint16, stride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, restoration *RestorationState, segState *SegmentationState, capture func([]TileState)) error {
	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	planeWidth := hdr.Width
	planeHeight := hdr.Height
	chromaWidth := (hdr.Width + 1) >> 1
	chromaHeight := (hdr.Height + 1) >> 1
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, hdr, tile, 0)
		if err != nil {
			return err
		}
		defer syntax.Close()
		restRefs := newTileRestorationRefs()
		var row TileRowSyntaxState
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			if err := syntax.ResetRowState(&row, sbRow); err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if err := parseRestorationUnitsForSuperBlock(hdr, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
					return err
				}
				if err := decodeKeyframeMain10SuperBlock(seq, hdr, &row, sb, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, chromaWidth, chromaHeight, cdef, segState); err != nil {
					return err
				}
			}
		}
	}
	if capture != nil {
		capture(states)
	}
	return nil
}

func decodeKeyframeMain10SuperBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState) error {
	row.Tile.ResetSuperBlockDeltas(hdr)
	root := BlockGeometry{
		Level:   RootBlockLevel(seq),
		Size:    RootBlockSize(seq),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return decodeKeyframeMain10Block(seq, hdr, row, sb, root, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root))
}

func decodeKeyframeMain10Block(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("partition-pre", row.Tile.Tile.Entropy, "xy=(%d,%d) level=%d size=%d", g.Start4X*4, g.Start4Y*4, g.Level, g.Size)
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("partition", row.Tile.Tile.Entropy, "xy=(%d,%d) level=%d size=%d part=%d", g.Start4X*4, g.Start4Y*4, g.Level, g.Size, part)
	}
	var childrenBuf [4]BlockGeometry

	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			children := splitChildrenInto(childrenBuf[:0], g)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain10Block(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i]); err != nil {
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
				if err := decodeKeyframeMain10Block(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i]); err != nil {
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
			if err := decodeKeyframeMain10Leaf(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i]); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := decodeKeyframeMain10Leaf(seq, hdr, row, sb, g, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, edge); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain10Leaf(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i]); err != nil {
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

func decodeKeyframeMain10Leaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	ctxHaveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	ctxHaveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	reconHaveTop, reconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
	chromaReconHaveTop, chromaReconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
	chromaEdge := edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling())
	segID := uint8(0)
	segPred := false
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, ctxHaveTop, ctxHaveLeft, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("leaf-skip", row.Tile.Tile.Entropy, "xy=(%d,%d) size=%d skip=%v skipMode=%v", g.Start4X*4, g.Start4Y*4, g.Size, skip, skipMode)
	}
	_ = skipMode
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, ctxHaveTop, ctxHaveLeft, true, skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	if segmentDataForID(hdr, segID).Skip {
		skip = true
	}
	if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("leaf-cdef", row.Tile.Tile.Entropy, "idx=%d", cdef.at(g.Start4X>>4, g.Start4Y>>4))
	}
	cdef.markNoSkip(g, skip)
	if err := readDeltaQAndLF(seq, hdr, row.Tile, g, skip); err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("leaf-delta", row.Tile.Tile.Entropy, "qidx=%d", row.Tile.LastQIdx)
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("leaf-syntax", row.Tile.Tile.Entropy, "y=%d uv=%d palY=%d palUV=%d tx=%d uvtx=%d", syntax.YMode, syntax.UVMode, syntax.PalYSize, syntax.PalUVSize, syntax.TX, syntax.UVTX)
	}
	syntax.Geometry = g
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	if !skip && shouldUseDav1dIntraCoeffOrder(seq, syntax) {
		if _, err := reconstructParsedIntraBlockAt16Dav1dOrder(
			seq,
			hdr,
			row.Tile.Tile.CDF,
			row.Tile.Tile.Coef,
			row.Tile.CurrentDQ,
			above,
			&row.Left,
			syntax,
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
			seq.ColorConfig.BitDepth,
			row.Tile.Tile.Entropy,
			reconHaveTop,
			reconHaveLeft,
			edge,
		); err != nil {
			return err
		}
	} else {
		if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(seq, hdr, row.Tile.Tile.CDF, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, plane, stride, planeWidth, planeHeight, g.Start4X, g.Start4Y, seq.ColorConfig.BitDepth, skip, row.Tile.Tile.Entropy, reconHaveTop, reconHaveLeft, edge.topHasRight, edge.leftHasBottom); err != nil {
			return err
		}
		if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(seq, hdr, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, seq.ColorConfig.BitDepth, skip, row.Tile.Tile.Entropy, chromaReconHaveTop, chromaReconHaveLeft, chromaEdge.topHasRight, chromaEdge.leftHasBottom); err != nil {
			return err
		}
	}
	syntax.PreserveTXContext = !skip
	return ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax)
}

func fillUint16(dst []uint16, v uint16) {
	for i := range dst {
		dst[i] = v
	}
}
