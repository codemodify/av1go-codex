package decoder

import (
	"fmt"
	"image"
	"os"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func (d *Decoder) decodePureGoFrame(parsed *ParsedFrame) (*Frame, error) {
	if parsed == nil {
		return nil, fmt.Errorf("decoder: parsed frame required")
	}
	if parsed.Header.ShowExistingFrame {
		oldFrames := appendUniqueFrame(nil, d.lastPureGoFrame)
		slot := int(parsed.Header.ExistingFrameIdx)
		oldMVs := appendUniqueMVField(nil, d.currentMVField)
		oldMVs = appendUniqueMVField(oldMVs, d.lastPureGoMVField)
		oldSegs := appendUniqueSegMap(nil, d.currentSegField)
		frame, err := d.clonePureGoSlotFrame(slot, parsed, "show_existing_frame")
		if err != nil {
			return nil, err
		}
		currentSeg := (*SegmentationMap)(nil)
		if slot >= 0 && slot < len(d.pureGoSegRefs) {
			currentSeg = d.pureGoSegRefs[slot]
		}
		d.setCurrentPureGoState(d.pureGoSlotMVField(slot), currentSeg)
		if slot >= 0 && slot < len(d.pureGoRefs) && d.pureGoRefs[slot] != nil {
			d.lastPureGoFrame = d.pureGoRefs[slot]
		} else {
			d.lastPureGoFrame = cloneFrameForReference(frame)
		}
		d.lastPureGoMVField = d.currentMVField
		d.releaseUnreferencedFrames(oldFrames)
		d.releaseUnreferencedMVFields(oldMVs)
		d.releaseUnreferencedSegMaps(oldSegs)
		return frame, nil
	}
	layout := d.header.ColorConfig.ChromaSubsampling()
	bitDepth := d.header.ColorConfig.BitDepth
	if layout != av1.Chroma420 || bitDepth == 0 || bitDepth > 10 {
		return nil, fmt.Errorf("decoder: pure-Go frame reconstruction supports only 8/10-bit 4:2:0 currently: %w", av1.ErrUnsupportedCodec)
	}
	if bitDepth > 8 {
		d.currentCDFState = nil
		frame, err := d.decodeCurrentPureGoFrameMain10(parsed)
		if err != nil {
			return nil, err
		}
		d.annotateCurrentMVField(parsed)
		d.snapshotPureGoState(parsed.Header.RefreshFrameFlags, frame, d.currentMVField, d.currentSegMap(parsed), parsed.Header.FrameOffset, buildTemporalMVMap(d.header, parsed, d.currentMVField))
		d.snapshotPureGoCDFState(parsed.Header.RefreshFrameFlags, &parsed.Header)
		return frame, nil
	}
	d.currentCDFState = nil
	frame, err := d.decodeCurrentPureGoFrame(parsed)
	if err != nil {
		return nil, err
	}
	d.annotateCurrentMVField(parsed)
	d.snapshotPureGoState(parsed.Header.RefreshFrameFlags, frame, d.currentMVField, d.currentSegMap(parsed), parsed.Header.FrameOffset, buildTemporalMVMap(d.header, parsed, d.currentMVField))
	d.snapshotPureGoCDFState(parsed.Header.RefreshFrameFlags, &parsed.Header)
	return frame, nil
}

func (d *Decoder) cloneLastPureGoFrame(parsed *ParsedFrame, reason string) (*Frame, error) {
	if d.lastPureGoFrame == nil {
		if parsed == nil {
			return nil, fmt.Errorf("decoder: %s reconstruction requires parsed frame context", reason)
		}
		frame := d.blankPureGoReferenceFrame(parsed.Header.Width, parsed.Header.Height)
		frame.PTS = parsed.PTS
		frame.Duration = parsed.Duration
		return frame, nil
	}
	frame := cloneFrame(d.lastPureGoFrame)
	frame.PTS = parsed.PTS
	frame.Duration = parsed.Duration
	return frame, nil
}

func (d *Decoder) clonePureGoSlotFrame(slot int, parsed *ParsedFrame, reason string) (*Frame, error) {
	if slot >= 0 && slot < len(d.pureGoRefs) && d.pureGoRefs[slot] != nil {
		frame := cloneFrame(d.pureGoRefs[slot])
		frame.PTS = parsed.PTS
		frame.Duration = parsed.Duration
		return frame, nil
	}
	return d.cloneLastPureGoFrame(parsed, fmt.Sprintf("%s slot %d", reason, slot))
}

func (d *Decoder) choosePureGoSegmentationRef(hdr *obu.FrameHeader) *SegmentationMap {
	if hdr == nil {
		return nil
	}
	if hdr.PrimaryRefFrame < uint8(len(hdr.RefIdx)) {
		slot := int(hdr.RefIdx[hdr.PrimaryRefFrame])
		if slot >= 0 && slot < len(d.pureGoSegRefs) && d.pureGoSegRefs[slot] != nil {
			return d.pureGoSegRefs[slot]
		}
	}
	for _, refIdx := range hdr.RefIdx {
		slot := int(refIdx)
		if slot >= 0 && slot < len(d.pureGoSegRefs) && d.pureGoSegRefs[slot] != nil {
			return d.pureGoSegRefs[slot]
		}
	}
	return nil
}

func (d *Decoder) pureGoSlotMVField(slot int) *SpatialMVTile {
	if slot >= 0 && slot < len(d.pureGoMVRefs) {
		return d.pureGoMVRefs[slot]
	}
	return nil
}

func debugPureGoSlotPOC(pocs [8]uint8, slot int8) uint8 {
	if slot >= 0 && int(slot) < len(pocs) {
		return pocs[slot]
	}
	return 0
}

func (d *Decoder) projectPureGoTemporalMVMap(parsed *ParsedFrame) *TemporalMVMap {
	if parsed == nil {
		return nil
	}
	if os.Getenv("DEBUG_DISABLE_TEMPORAL_MVS") != "" {
		return nil
	}
	var refs [7]*TemporalMVMap
	for i, slot := range parsed.Header.RefIdx {
		if slot >= 0 && int(slot) < len(d.pureGoTemporalRefs) {
			refs[i] = d.pureGoTemporalRefs[slot]
		}
	}
	return projectTemporalMVMap(d.header, parsed, refs)
}

func (d *Decoder) decodeCurrentPureGoFrame(parsed *ParsedFrame) (*Frame, error) {
	switch parsed.Header.FrameType {
	case obu.FrameTypeKey:
		return d.decodeCurrentPureGoKeyframe(parsed)
	case obu.FrameTypeInter, obu.FrameTypeSwitch:
		return d.decodeCurrentPureGoInterFrame(parsed)
	case obu.FrameTypeIntra:
		return d.decodeCurrentPureGoKeyframe(parsed)
	default:
		return d.clonePureGoReferenceFrame(parsed)
	}
}

func (d *Decoder) decodeCurrentPureGoKeyframe(parsed *ParsedFrame) (*Frame, error) {
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, d.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	currentMV := (*SpatialMVTile)(nil)
	if parsed.Header.AllowIntrabc {
		currentMV = takeFrameMVField(parsed.Header.Width, parsed.Header.Height)
	}
	d.setCurrentPureGoState(currentMV, segState.Current)

	frame := takeReferenceFrame420(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig, parsed.PTS, parsed.Duration)
	img := frame.Image.(*image.YCbCr)
	fillFrame420Neutral(frame)

	dq := InitDequantTables(d.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, d.header.ColorConfig.ChromaSubsampling())
	defer putLoopFilterEdgeMaps(edges)
	restoration := NewRestorationState(d.header, &parsed.Header)
	if parsed.Header.AllowIntrabc {
		return d.decodeCurrentPureGoIntraBCFrame(parsed, dq, frame, cdef, edges, restoration, segState)
	}
	if err := decodeKeyframeMain8DCLumaWithCDFCapture(d.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, restoration, segState, d.capturePureGoCDFState); err != nil {
		_ = frame.Close()
		return nil, err
	}
	applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
	applyCDEF420_8(img, &parsed.Header, cdef)
	applyRestoration420_8(img, &parsed.Header, restoration)

	return frame, nil
}

func (d *Decoder) decodeCurrentPureGoIntraBCFrame(parsed *ParsedFrame, dq DequantTables, frame *Frame, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState) (*Frame, error) {
	img := frame.Image.(*image.YCbCr)
	if err := decodeKeyframeMain8WithIntraBC(d.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, segState, d.currentMVField); err != nil {
		_ = frame.Close()
		return nil, err
	}
	applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
	applyCDEF420_8(img, &parsed.Header, cdef)
	applyRestoration420_8(img, &parsed.Header, restoration)
	return frame, nil
}

func (d *Decoder) clonePureGoReferenceFrame(parsed *ParsedFrame) (*Frame, error) {
	hdr := &parsed.Header
	if hdr.PrimaryRefFrame < uint8(len(hdr.RefIdx)) {
		slot := int(hdr.RefIdx[hdr.PrimaryRefFrame])
		if slot >= 0 && slot < len(d.pureGoRefs) && d.pureGoRefs[slot] != nil {
			return d.clonePureGoSlotFrame(slot, parsed, fmt.Sprintf("primary_ref_frame %d", hdr.PrimaryRefFrame))
		}
	}
	for i, refIdx := range hdr.RefIdx {
		slot := int(refIdx)
		if slot < 0 || slot >= len(d.pureGoRefs) || d.pureGoRefs[slot] == nil {
			continue
		}
		return d.clonePureGoSlotFrame(slot, parsed, fmt.Sprintf("ref_idx[%d]", i))
	}
	return d.cloneLastPureGoFrame(parsed, fmt.Sprintf("frame type %d", hdr.FrameType))
}

func (d *Decoder) currentSegMap(parsed *ParsedFrame) *SegmentationMap {
	_ = parsed
	return d.currentSegField
}

func (d *Decoder) annotateCurrentMVField(parsed *ParsedFrame) {
	if d.currentMVField == nil || parsed == nil {
		return
	}
	d.currentMVField.FramePOC = parsed.Header.FrameOffset
	d.currentMVField.SlotPOC = d.pureGoRefPOC
}

func appendUniqueMVField(dst []*SpatialMVTile, field *SpatialMVTile) []*SpatialMVTile {
	if field == nil {
		return dst
	}
	for _, existing := range dst {
		if existing == field {
			return dst
		}
	}
	return append(dst, field)
}

func appendUniqueFrame(dst []*Frame, frame *Frame) []*Frame {
	if frame == nil {
		return dst
	}
	for _, existing := range dst {
		if existing == frame {
			return dst
		}
	}
	return append(dst, frame)
}

func appendUniqueSegMap(dst []*SegmentationMap, seg *SegmentationMap) []*SegmentationMap {
	if seg == nil {
		return dst
	}
	for _, existing := range dst {
		if existing == seg {
			return dst
		}
	}
	return append(dst, seg)
}

func (d *Decoder) mvFieldReferenced(field *SpatialMVTile) bool {
	if field == nil {
		return false
	}
	if d.currentMVField == field || d.lastPureGoMVField == field {
		return true
	}
	for _, ref := range d.pureGoMVRefs {
		if ref == field {
			return true
		}
	}
	return false
}

func (d *Decoder) segMapReferenced(seg *SegmentationMap) bool {
	if seg == nil {
		return false
	}
	if d.currentSegField == seg {
		return true
	}
	for _, ref := range d.pureGoSegRefs {
		if ref == seg {
			return true
		}
	}
	return false
}

func (d *Decoder) frameReferenced(frame *Frame) bool {
	if frame == nil {
		return false
	}
	if d.lastPureGoFrame == frame {
		return true
	}
	for _, ref := range d.pureGoRefs {
		if ref == frame {
			return true
		}
	}
	return false
}

func (d *Decoder) releaseUnreferencedFrames(frames []*Frame) {
	for _, frame := range frames {
		if !d.frameReferenced(frame) {
			putFrameScratch(frame)
		}
	}
}

func (d *Decoder) releaseUnreferencedMVFields(fields []*SpatialMVTile) {
	for _, field := range fields {
		if !d.mvFieldReferenced(field) {
			putSpatialMVTileScratch(field)
		}
	}
}

func (d *Decoder) releaseUnreferencedSegMaps(segs []*SegmentationMap) {
	for _, seg := range segs {
		if !d.segMapReferenced(seg) {
			putSegmentationMap(seg)
		}
	}
}

func (d *Decoder) setCurrentPureGoState(field *SpatialMVTile, seg *SegmentationMap) {
	oldMV := d.currentMVField
	oldSeg := d.currentSegField
	d.currentMVField = field
	d.currentSegField = seg
	d.releaseUnreferencedMVFields(appendUniqueMVField(nil, oldMV))
	d.releaseUnreferencedSegMaps(appendUniqueSegMap(nil, oldSeg))
}

func (d *Decoder) applyPureGoRefresh(flags uint8, frame *Frame, field *SpatialMVTile, seg *SegmentationMap, temporal *TemporalMVMap, framePOC uint8) {
	for i := 0; i < len(d.pureGoRefs); i++ {
		if flags&(1<<i) == 0 {
			continue
		}
		d.pureGoRefs[i] = frame
		d.pureGoMVRefs[i] = field
		d.pureGoSegRefs[i] = seg
		d.pureGoTemporalRefs[i] = temporal
		d.pureGoRefPOC[i] = framePOC
	}
}

func (d *Decoder) snapshotPureGoState(refreshFlags uint8, frame *Frame, field *SpatialMVTile, seg *SegmentationMap, framePOC uint8, temporal *TemporalMVMap) {
	oldFrames := appendUniqueFrame(nil, d.lastPureGoFrame)
	oldMVs := appendUniqueMVField(nil, d.lastPureGoMVField)
	oldSegs := make([]*SegmentationMap, 0, len(d.pureGoSegRefs))
	for i := 0; i < len(d.pureGoRefs); i++ {
		if refreshFlags&(1<<i) == 0 {
			continue
		}
		oldFrames = appendUniqueFrame(oldFrames, d.pureGoRefs[i])
		oldMVs = appendUniqueMVField(oldMVs, d.pureGoMVRefs[i])
		oldSegs = appendUniqueSegMap(oldSegs, d.pureGoSegRefs[i])
	}
	frameSnapshot := cloneFrameForReference(frame)

	d.lastPureGoFrame = frameSnapshot
	d.lastPureGoMVField = field
	d.applyPureGoRefresh(refreshFlags, frameSnapshot, field, seg, temporal, framePOC)
	d.releaseUnreferencedFrames(oldFrames)
	d.releaseUnreferencedMVFields(oldMVs)
	d.releaseUnreferencedSegMaps(oldSegs)
}

func (d *Decoder) releasePureGoResources() {
	oldFrames := appendUniqueFrame(nil, d.lastPureGoFrame)
	for i := range d.pureGoRefs {
		oldFrames = appendUniqueFrame(oldFrames, d.pureGoRefs[i])
		d.pureGoRefs[i] = nil
	}
	d.lastPureGoFrame = nil
	d.releaseUnreferencedFrames(oldFrames)

	oldMVs := appendUniqueMVField(nil, d.currentMVField)
	oldMVs = appendUniqueMVField(oldMVs, d.lastPureGoMVField)
	for i := range d.pureGoMVRefs {
		oldMVs = appendUniqueMVField(oldMVs, d.pureGoMVRefs[i])
		d.pureGoMVRefs[i] = nil
		d.pureGoTemporalRefs[i] = nil
	}
	d.currentMVField = nil
	d.lastPureGoMVField = nil
	d.releaseUnreferencedMVFields(oldMVs)

	oldSegs := appendUniqueSegMap(nil, d.currentSegField)
	for i := range d.pureGoSegRefs {
		oldSegs = appendUniqueSegMap(oldSegs, d.pureGoSegRefs[i])
		d.pureGoSegRefs[i] = nil
	}
	d.currentSegField = nil
	d.releaseUnreferencedSegMaps(oldSegs)
}

func decodeKeyframeMain8DCLuma(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState) error {
	return decodeKeyframeMain8DCLumaWithCDEFOrder(seq, hdr, group, dq, img, cdef, edges, restoration, segState, true)
}

func decodeKeyframeMain8DCLumaWithCDFCapture(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState, capture func([]TileState)) error {
	return decodeKeyframeMain8DCLumaWithCDEFOrderAndCDFCapture(seq, hdr, group, dq, img, cdef, edges, restoration, segState, true, capture)
}

func decodeKeyframeMain8DCLumaWithCDEFOrder(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState, cdefBeforeSyntax bool) error {
	return decodeKeyframeMain8DCLumaWithCDEFOrderAndCDFCapture(seq, hdr, group, dq, img, cdef, edges, restoration, segState, cdefBeforeSyntax, nil)
}

func decodeKeyframeMain8DCLumaWithCDEFOrderAndCDFCapture(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, restoration *RestorationState, segState *SegmentationState, cdefBeforeSyntax bool, capture func([]TileState)) error {
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
		restRefs := newTileRestorationRefs()
		var row TileRowSyntaxState
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			if err := syntax.ResetRowState(&row, sbRow); err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if os.Getenv("DEBUG_SKIP_REST_PARSE") == "" {
					if err := parseRestorationUnitsForSuperBlock(hdr, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
						return err
					}
				}
				if err := decodeKeyframeMain8DCSuperBlock(seq, hdr, &row, sb, dq, img, cdef, edges, segState, cdefBeforeSyntax); err != nil {
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

func decodeKeyframeMain8DCSuperBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, cdefBeforeSyntax bool) error {
	row.Tile.ResetSuperBlockDeltas(hdr)
	root := BlockGeometry{
		Level:   RootBlockLevel(seq),
		Size:    RootBlockSize(seq),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	if err := decodeKeyframeMain8DCBlock(seq, hdr, row, sb, root, dq, img, cdef, edges, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root), cdefBeforeSyntax); err != nil {
		return err
	}
	if !cdefBeforeSyntax && !hdr.AllLossless {
		return cdef.readSuperBlockIndexes(root, hdr.CDEF.NBits, row.Tile.Tile.Entropy)
	}
	return nil
}

func decodeKeyframeMain8DCBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, edge intraEdgeAvailability, cdefBeforeSyntax bool) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
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
				if err := decodeKeyframeMain8DCBlock(seq, hdr, row, sb, child, dq, img, cdef, edges, segState, childEdges[i], cdefBeforeSyntax); err != nil {
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
				if err := decodeKeyframeMain8DCBlock(seq, hdr, row, sb, child, dq, img, cdef, edges, segState, childEdges[i], cdefBeforeSyntax); err != nil {
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
			if err := decodeKeyframeMain8DCLeaf(seq, hdr, row, sb, child, dq, img, cdef, edges, segState, childEdges[i], cdefBeforeSyntax); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := decodeKeyframeMain8DCLeaf(seq, hdr, row, sb, g, dq, img, cdef, edges, segState, edge, cdefBeforeSyntax); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8DCLeaf(seq, hdr, row, sb, child, dq, img, cdef, edges, segState, childEdges[i], cdefBeforeSyntax); err != nil {
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

func decodeKeyframeMain8DCLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, edges *loopFilterEdgeMaps, segState *SegmentationState, edge intraEdgeAvailability, cdefBeforeSyntax bool) error {
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
	if cdefBeforeSyntax {
		if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
			return err
		}
		if debugBlockTraceEnabled(g) {
			debugTraceEntropy("leaf-cdef", row.Tile.Tile.Entropy, "idx=%d", cdef.at(g.Start4X>>4, g.Start4Y>>4))
		}
		cdef.markNoSkip(g, skip)
	}
	if err := readDeltaQAndLF(seq, hdr, row.Tile, g, skip); err != nil {
		return err
	}
	if debugBlockTraceEnabled(g) {
		debugTraceEntropy("leaf-delta", row.Tile.Tile.Entropy, "qidx=%d", row.Tile.LastQIdx)
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, g, ctxHaveTop, ctxHaveLeft, segID, row.Tile.Tile.Entropy)
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
	if !cdefBeforeSyntax {
		cdef.markNoSkip(g, skip)
	}
	if !skip && shouldUseDav1dIntraCoeffOrder(seq, syntax) {
		if _, err := reconstructParsedIntraBlockAt8Dav1dOrder(
			seq,
			hdr,
			row.Tile.Tile.CDF,
			row.Tile.Tile.Coef,
			row.Tile.CurrentDQ,
			above,
			&row.Left,
			syntax,
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
		if err := ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(
			seq,
			hdr,
			row.Tile.Tile.CDF,
			row.Tile.Tile.Coef,
			row.Tile.CurrentDQ,
			above,
			&row.Left,
			syntax,
			img.Y,
			img.YStride,
			parsedPlaneWidth(img),
			parsedPlaneHeight(img),
			g.Start4X,
			g.Start4Y,
			skip,
			row.Tile.Tile.Entropy,
			reconHaveTop,
			reconHaveLeft,
			edge.topHasRight,
			edge.leftHasBottom,
		); err != nil {
			return err
		}
		if err := reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(
			seq,
			hdr,
			row.Tile.Tile.Coef,
			row.Tile.CurrentDQ,
			above,
			&row.Left,
			syntax,
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
			skip,
			chromaReconHaveTop,
			chromaReconHaveLeft,
			chromaEdge.topHasRight,
			chromaEdge.leftHasBottom,
		); err != nil {
			return err
		}
	}
	syntax.PreserveTXContext = !skip
	hasChroma := hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y)
	markInterBlockEdges(edges, seq.ColorConfig.ChromaSubsampling(), g, hasChroma)
	markInterTransformEdgesForBlock(edges, seq.ColorConfig.ChromaSubsampling(), g, syntax.TX, syntax.UVTX, hasChroma)
	return ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax)
}

func contextForBlock(seq av1.SequenceHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry) (*BlockContext, BlockGeometry, error) {
	if row == nil || row.Tile == nil {
		return nil, BlockGeometry{}, fmt.Errorf("decoder: tile row state required")
	}
	sbShift4 := int(SuperBlockShift4(seq))
	sbx := g.Start4X >> sbShift4
	aboveIdx := row.Tile.AboveContextIndex(seq, sbx)
	if aboveIdx < 0 || aboveIdx >= len(row.Tile.Above) {
		return nil, BlockGeometry{}, fmt.Errorf("decoder: above context index %d out of range", aboveIdx)
	}
	// Context arrays are 32 4x4 units wide/high. In 64x64 superblock mode the
	// vertical phase spans paired SB rows, matching dav1d's 128-pixel bands.
	groupBase4X := (g.Start4X >> 5) << 5
	rowBase4Y := (g.Start4Y >> 5) << 5
	local := g
	local.Start4X -= groupBase4X
	local.End4X -= groupBase4X
	local.Start4Y -= rowBase4Y
	local.End4Y -= rowBase4Y
	_ = sb
	return &row.Tile.Above[aboveIdx], local, nil
}

func parsedPlaneWidth(img *image.YCbCr) int {
	return img.Rect.Dx()
}

func parsedPlaneHeight(img *image.YCbCr) int {
	return img.Rect.Dy()
}

func parsedChromaPlaneWidth(img *image.YCbCr) int {
	return (img.Rect.Dx() + 1) >> 1
}

func parsedChromaPlaneHeight(img *image.YCbCr) int {
	return (img.Rect.Dy() + 1) >> 1
}
