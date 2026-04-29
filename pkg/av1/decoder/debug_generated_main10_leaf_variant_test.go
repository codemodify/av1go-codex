package decoder

import (
	"io"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func debugDecodeGeneratedMain10RawWithLeafReaders(t *testing.T, path string, overrides map[int]debugIntraSyntaxReader) ([]uint16, int, []uint16, int, []uint16, int) {
	return debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, nil)
}

type debugLeafStateCapture func(callIdx int, state debugTargetLeafAdvanceState)

func debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t *testing.T, path string, overrides map[int]debugIntraSyntaxReader, capture debugLeafStateCapture) ([]uint16, int, []uint16, int, []uint16, int) {
	t.Helper()

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	width := parsed.Header.Width
	height := parsed.Header.Height
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	yPlane := make([]uint16, width*height)
	uPlane := make([]uint16, chromaWidth*chromaHeight)
	vPlane := make([]uint16, chromaWidth*chromaHeight)
	fillUint16(uPlane, 1<<(dec.header.ColorConfig.BitDepth-1))
	fillUint16(vPlane, 1<<(dec.header.ColorConfig.BitDepth-1))

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	if parsed.Header.AllowIntrabc {
		t.Fatal("debug leaf variant harness does not support intraBC fixtures")
	}

	if err := debugDecodeKeyframeMain10WithLeafReaders(dec.header, &parsed.Header, &parsed.TileGroup, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, cdef, restoration, segState, overrides, capture); err != nil {
		t.Fatalf("debugDecodeKeyframeMain10WithLeafReaders: %v", err)
	}
	return yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth
}

func debugDecodeKeyframeMain10WithLeafReaders(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, plane []uint16, stride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, restoration *RestorationState, segState *SegmentationState, overrides map[int]debugIntraSyntaxReader, capture debugLeafStateCapture) error {
	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	planeWidth := hdr.Width
	planeHeight := hdr.Height
	chromaWidth := (hdr.Width + 1) >> 1
	chromaHeight := (hdr.Height + 1) >> 1
	leafCall := 0
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
				if err := debugDecodeKeyframeMain10SuperBlockWithLeafReaders(seq, hdr, &row, sb, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, chromaWidth, chromaHeight, cdef, segState, overrides, &leafCall, capture); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func debugDecodeKeyframeMain10SuperBlockWithLeafReaders(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState, overrides map[int]debugIntraSyntaxReader, leafCall *int, capture debugLeafStateCapture) error {
	row.Tile.ResetSuperBlockDeltas(hdr)
	root := BlockGeometry{
		Level:   RootBlockLevel(seq),
		Size:    RootBlockSize(seq),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return debugDecodeKeyframeMain10BlockWithLeafReaders(seq, hdr, row, sb, root, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root), overrides, leafCall, capture, nil)
}

func debugLeafPendingRoots(next []BlockGeometry, pending []BlockGeometry) []BlockGeometry {
	if len(next) == 0 && len(pending) == 0 {
		return nil
	}
	out := make([]BlockGeometry, 0, len(next)+len(pending))
	out = append(out, next...)
	out = append(out, pending...)
	return out
}

func debugCopyTraversalFrames(in []debugTraversalFrame) []debugTraversalFrame {
	if len(in) == 0 {
		return nil
	}
	out := make([]debugTraversalFrame, len(in))
	copy(out, in)
	return out
}

func debugAppendTraversalFrame(stack []debugTraversalFrame, g, local BlockGeometry, part BlockPartition, children []BlockGeometry, childIndex int) []debugTraversalFrame {
	frame := debugTraversalFrame{
		Geometry:   g,
		Local:      local,
		Part:       part,
		ChildIndex: childIndex,
	}
	for _, child := range children {
		if child.Width4() == 0 || child.Height4() == 0 {
			continue
		}
		if frame.ChildCount < len(frame.Children) {
			frame.Children[frame.ChildCount] = child
			frame.ChildCount++
		}
	}
	return append(stack, frame)
}

func debugCloneCDEFIndexMap(m *CDEFIndexMap) *CDEFIndexMap {
	if m == nil {
		return nil
	}
	clone := &CDEFIndexMap{
		CellsW: m.CellsW,
		CellsH: m.CellsH,
		MaskW:  m.MaskW,
		MaskH:  m.MaskH,
	}
	if len(m.Values) != 0 {
		clone.Values = append([]int8(nil), m.Values...)
	}
	if len(m.NoSkip) != 0 {
		clone.NoSkip = append([]bool(nil), m.NoSkip...)
	}
	return clone
}

func debugDecodeKeyframeMain10BlockWithLeafReaders(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability, overrides map[int]debugIntraSyntaxReader, leafCall *int, capture debugLeafStateCapture, pending []BlockGeometry) error {
	return debugDecodeKeyframeMain10BlockWithLeafReadersStack(seq, hdr, row, sb, g, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, edge, overrides, leafCall, capture, pending, nil)
}

func debugDecodeKeyframeMain10BlockWithLeafReadersStack(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability, overrides map[int]debugIntraSyntaxReader, leafCall *int, capture debugLeafStateCapture, pending []BlockGeometry, stack []debugTraversalFrame) error {
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
				childPending := debugLeafPendingRoots(children[i+1:], pending)
				childStack := debugAppendTraversalFrame(stack, g, local, part, children, i)
				if err := debugDecodeKeyframeMain10BlockWithLeafReadersStack(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i], overrides, leafCall, capture, childPending, childStack); err != nil {
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
				childPending := debugLeafPendingRoots(children[i+1:], pending)
				childStack := debugAppendTraversalFrame(stack, g, local, part, children, i)
				if err := debugDecodeKeyframeMain10BlockWithLeafReadersStack(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i], overrides, leafCall, capture, childPending, childStack); err != nil {
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
			childPending := debugLeafPendingRoots(children[i+1:], pending)
			childStack := debugAppendTraversalFrame(stack, g, local, part, children, i)
			if err := debugDecodeKeyframeMain10LeafWithReader(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i], overrides, leafCall, capture, childPending, childStack, false, BlockGeometry{}, 0); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugDecodeKeyframeMain10LeafWithReader(seq, hdr, row, sb, g, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, edge, overrides, leafCall, capture, pending, stack, true, local, part); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				childPending := debugLeafPendingRoots(children[i+1:], pending)
				childStack := debugAppendTraversalFrame(stack, g, local, part, children, i)
				if err := debugDecodeKeyframeMain10LeafWithReader(seq, hdr, row, sb, child, dq, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, cdef, segState, childEdges[i], overrides, leafCall, capture, childPending, childStack, false, BlockGeometry{}, 0); err != nil {
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

func debugDecodeKeyframeMain10LeafWithReader(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, plane []uint16, stride, planeWidth, planeHeight int, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability, overrides map[int]debugIntraSyntaxReader, leafCall *int, capture debugLeafStateCapture, pending []BlockGeometry, stack []debugTraversalFrame, returnApply bool, returnLocal BlockGeometry, returnPart BlockPartition) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	entryAbove := *above
	entryLeft := row.Left
	entryMode := row.Tile.Tile.CDF.Clone()
	entryCoef := row.Tile.Tile.Coef.Clone()
	entryDec := row.Tile.Tile.Entropy.Clone()
	entryQIdx := row.Tile.LastQIdx
	entryLastLF := row.Tile.LastLF
	entryReadDeltas := row.Tile.ReadDeltas
	entryDQ := row.Tile.CurrentDQ
	entryCDEF := debugCloneCDEFIndexMap(cdef)
	ctxHaveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	ctxHaveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	reconHaveTop, reconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
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
	cdef.markNoSkip(g, skip)
	if err := readDeltaQAndLF(seq, hdr, row.Tile, g, skip); err != nil {
		return err
	}
	preSyntaxAbove := *above
	preSyntaxLeft := row.Left
	preSyntaxMode := row.Tile.Tile.CDF.Clone()
	preSyntaxCoef := row.Tile.Tile.Coef.Clone()
	preSyntaxDec := row.Tile.Tile.Entropy.Clone()
	preSyntaxQIdx := row.Tile.LastQIdx
	preSyntaxLastLF := row.Tile.LastLF
	preSyntaxReadDeltas := row.Tile.ReadDeltas
	preSyntaxDQ := row.Tile.CurrentDQ
	preSyntaxCDEF := debugCloneCDEFIndexMap(cdef)
	callIdx := *leafCall
	*leafCall++
	reader := ReadIntraBlockSyntax
	if alt, ok := overrides[callIdx]; ok && alt != nil {
		reader = alt
	}
	syntax, err := reader(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	syntax.Geometry = g
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	var capturedState *debugTargetLeafAdvanceState
	if capture != nil {
		var nextRoot BlockGeometry
		if len(pending) > 0 {
			nextRoot = pending[0]
		}
		capturedState = &debugTargetLeafAdvanceState{
			syntax:              syntax,
			entryLocal:          local,
			entryAbove:          entryAbove,
			entryLeft:           entryLeft,
			entryMode:           entryMode,
			entryCoef:           entryCoef,
			entryDec:            entryDec,
			entryQIdx:           entryQIdx,
			entryLastLF:         entryLastLF,
			entryReadDeltas:     entryReadDeltas,
			entryDQ:             entryDQ,
			entryCDEF:           entryCDEF,
			preSyntaxAbove:      preSyntaxAbove,
			preSyntaxLeft:       preSyntaxLeft,
			preSyntaxMode:       preSyntaxMode,
			preSyntaxCoef:       preSyntaxCoef,
			preSyntaxDec:        preSyntaxDec,
			preSyntaxQIdx:       preSyntaxQIdx,
			preSyntaxLastLF:     preSyntaxLastLF,
			preSyntaxReadDeltas: preSyntaxReadDeltas,
			preSyntaxDQ:         preSyntaxDQ,
			preSyntaxCDEF:       preSyntaxCDEF,
			above:               *above,
			left:                row.Left,
			qidx:                row.Tile.LastQIdx,
			lastLF:              row.Tile.LastLF,
			readDeltas:          row.Tile.ReadDeltas,
			mode:                row.Tile.Tile.CDF.Clone(),
			coef:                row.Tile.Tile.Coef.Clone(),
			dq:                  row.Tile.CurrentDQ,
			dec:                 row.Tile.Tile.Entropy.Clone(),
			cdef:                debugCloneCDEFIndexMap(cdef),
			returnApply:         returnApply,
			returnLocal:         returnLocal,
			returnPart:          returnPart,
			traversal:           debugCopyTraversalFrames(stack),
			nextRoot:            nextRoot,
			haveTop:             reconHaveTop,
			haveLeft:            reconHaveLeft,
			edge:                edge,
		}
	}
	if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(seq, hdr, row.Tile.Tile.CDF, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, plane, stride, planeWidth, planeHeight, g.Start4X, g.Start4Y, seq.ColorConfig.BitDepth, skip, row.Tile.Tile.Entropy, reconHaveTop, reconHaveLeft, edge.topHasRight, edge.leftHasBottom); err != nil {
		return err
	}
	chromaEdge := edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling())
	if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(seq, hdr, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, plane, stride, planeWidth, planeHeight, uPlane, uStride, uWidth, uHeight, vPlane, vStride, vWidth, vHeight, seq.ColorConfig.BitDepth, skip, row.Tile.Tile.Entropy, reconHaveTop, reconHaveLeft, chromaEdge.topHasRight, chromaEdge.leftHasBottom); err != nil {
		return err
	}
	syntax.PreserveTXContext = !skip
	if err := ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax); err != nil {
		return err
	}
	if capture != nil && capturedState != nil {
		capturedState.postAbove = *above
		capturedState.postLeft = row.Left
		capturedState.postMode = row.Tile.Tile.CDF.Clone()
		capturedState.postCoef = row.Tile.Tile.Coef.Clone()
		capturedState.postDec = row.Tile.Tile.Entropy.Clone()
		capturedState.postCDEF = debugCloneCDEFIndexMap(cdef)
		capture(callIdx, *capturedState)
	}
	return nil
}

func TestDebugGeneratedMain10LeafVariantRMSE(t *testing.T) {
	if os.Getenv("DEBUG_TARGET_LEAF_VARIANT_RMSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-target-leaf-variant-main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})
	refFrames := testutil.DecodeReferenceFrames420(t, fixture.Path, 1)
	if len(refFrames) != 1 {
		t.Fatalf("reference frames=%d, want 1", len(refFrames))
	}
	ref := refFrames[0]

	variants := []struct {
		name      string
		overrides map[int]debugIntraSyntaxReader
	}{
		{name: "baseline"},
		{name: "leaf4_bounded_y_delta_bits", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxWithCustomYPalette(debugReadPalettePlaneSyntaxBoundedYDeltaBits)}},
		{name: "leaf4_uvmode_no_cfl_row", overrides: map[int]debugIntraSyntaxReader{4: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode[1] = clone.UVMode[0]
			return clone
		})}},
		{name: "leaf4_skip_cfl_alpha", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxSkipCFLAlpha}},
		{name: "leaf4_force_cfl_v_neg1", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxOverrideCFLAlpha(0, -1)}},
		{name: "leaf4_force_cfl_v_zero", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxOverrideCFLAlpha(0, 0)}},
		{name: "leaf4_cfl_after_luma_palette", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxCFLAfterLumaPalette}},
		{name: "leaf4_cfl_after_luma_indices", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxCFLAfterLumaIndices}},
		{name: "leaf4_palette_before_uv_mode", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxPaletteBeforeUVMode}},
		{name: "leaf4_no_cfl", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxNoCFL}},
		{name: "leaf4_default_uv_mode_only", overrides: map[int]debugIntraSyntaxReader{4: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode = defaultModeCDFValue.UVMode
			return clone
		})}},
		{name: "leaf4_default_cfl_sign", overrides: map[int]debugIntraSyntaxReader{4: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLSign = defaultModeCDFValue.CFLSign
			return clone
		})}},
		{name: "leaf4_default_cfl_alpha", overrides: map[int]debugIntraSyntaxReader{4: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			return clone
		})}},
		{name: "leaf4_default_cfl_only", overrides: map[int]debugIntraSyntaxReader{4: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLSign = defaultModeCDFValue.CFLSign
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			return clone
		})}},
		{name: "leaf4_default_uv_mode", overrides: map[int]debugIntraSyntaxReader{4: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode = defaultModeCDFValue.UVMode
			clone.CFLSign = defaultModeCDFValue.CFLSign
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			clone.AngleDelta = defaultModeCDFValue.AngleDelta
			return clone
		})}},
		{name: "leaf4_no_palette_cache", overrides: map[int]debugIntraSyntaxReader{4: debugReadIntraBlockSyntaxNoPaletteCache}},
		{name: "leaf28_no_cfl", overrides: map[int]debugIntraSyntaxReader{28: debugReadIntraBlockSyntaxNoCFL}},
		{name: "leaf31_no_cfl", overrides: map[int]debugIntraSyntaxReader{31: debugReadIntraBlockSyntaxNoCFL}},
		{name: "leaf31_tx_before", overrides: map[int]debugIntraSyntaxReader{31: debugReadIntraBlockSyntaxTxBeforePaletteIndices}},
		{name: "leaf32_no_cfl", overrides: map[int]debugIntraSyntaxReader{32: debugReadIntraBlockSyntaxNoCFL}},
		{name: "leaf32_tx_before", overrides: map[int]debugIntraSyntaxReader{32: debugReadIntraBlockSyntaxTxBeforePaletteIndices}},
	}

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, variant.overrides)
			logReferencePipelineMetrics16(t, "raw", yPlane, yStride, uPlane, uStride, vPlane, vStride, ref)
		})
	}
}
