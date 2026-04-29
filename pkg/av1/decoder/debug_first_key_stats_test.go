package decoder

import (
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

var debugTUTrace []string

func resetDebugTUTrace() {
	debugTUTrace = debugTUTrace[:0]
}

func appendDebugTUTrace(format string, args ...any) {
	debugTUTrace = append(debugTUTrace, fmt.Sprintf(format, args...))
}

func debugApplyIntraBlockContextReplay(seq av1.SequenceHeader, hdr *obu.FrameHeader, above, left *BlockContext, syntax IntraBlockSyntax) error {
	syntax.PreserveTXContext = !syntax.Skip
	return ApplyIntraBlockContext(seq, hdr, above, left, syntax)
}

func debugSPBTVPath() string {
	return filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
}

func TestDebugFirstKeyStats(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_STATS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	stats := firstKeyStats{
		yModes:  map[IntraPredMode]int{},
		uvModes: map[IntraPredMode]int{},
		tx:      map[TxfmSize]int{},
		uvTx:    map[TxfmSize]int{},
	}
	t.Logf("quant: yac=%d ydc=%d udc=%d uac=%d vdc=%d vac=%d qm=%v qmy=%d qmu=%d qmv=%d",
		parsed.Header.Quantization.YAC,
		parsed.Header.Quantization.YDCDelta,
		parsed.Header.Quantization.UDCDelta,
		parsed.Header.Quantization.UACDelta,
		parsed.Header.Quantization.VDCDelta,
		parsed.Header.Quantization.VACDelta,
		parsed.Header.Quantization.QM,
		parsed.Header.Quantization.QMY,
		parsed.Header.Quantization.QMU,
		parsed.Header.Quantization.QMV,
	)

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	_ = edges
	_ = restoration

	if err := decodeKeyframeMain8Stats(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, segState, &stats); err != nil {
		t.Fatal(err)
	}

	t.Logf("blocks=%d skips=%d filter_intra=%d palette_y=%d palette_uv=%d cfl=%d", stats.blocks, stats.skips, stats.filterIntra, stats.paletteY, stats.paletteUV, stats.cfl)
	logSortedMap(t, "Y modes", stats.yModes, func(v IntraPredMode) string { return intraModeName(v) })
	logSortedMap(t, "UV modes", stats.uvModes, func(v IntraPredMode) string { return intraModeName(v) })
	logSortedMap(t, "TX", stats.tx, func(v TxfmSize) string { return txName(v) })
	logSortedMap(t, "UVTX", stats.uvTx, func(v TxfmSize) string { return txName(v) })
}

func TestDebugSPBTVFirstFrameProblemBlocks(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	targets := [][2]int{
		{16, 16},
		{48, 8},
		{48, 16},
		{16, 48},
		{48, 48},
		{256, 160}, {272, 160}, {288, 160},
		{256, 176}, {272, 176}, {288, 176},
		{256, 192}, {272, 192}, {288, 192},
		{256, 200}, {272, 200}, {288, 200},
		{256, 208}, {272, 208}, {288, 208},
		{256, 224}, {272, 224}, {288, 224},
		{256, 240}, {272, 240}, {288, 240},
		{352, 208}, {368, 208}, {384, 208},
		{352, 224}, {368, 224}, {384, 224},
		{352, 240}, {368, 240}, {384, 240}, {400, 240},
		{352, 256}, {368, 256},
		{352, 272}, {368, 272},
		{240, 240},
		{432, 160}, {447, 160}, {480, 160}, {480, 144}, {480, 176},
		{480, 112}, {480, 96}, {479, 112}, {479, 96}, {480, 80}, {479, 80}, {480, 64}, {479, 64},
		{832, 48},
	}
	if err := debugKeyframeProblemBlocks(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, segState, targets, t); err != nil {
		t.Fatal(err)
	}
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err == nil && len(ref) >= parsed.Header.Width*parsed.Header.Height {
		refY := ref[:parsed.Header.Width*parsed.Header.Height]
		rootMSE := compareBlockToRefY(img.Y, refY, img.YStride, 0, 0, 128, 128)
		t.Logf("root replay current part mse=%.2f", rootMSE)
		var forcedRootPart BlockPartition = PartitionSplit
		imgSplit := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
		fillUint8(imgSplit.Cb, 128)
		fillUint8(imgSplit.Cr, 128)
		segStateSplit := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		if err := debugKeyframeProblemBlocksWithRootPart(dec.header, &parsed.Header, &parsed.TileGroup, dq, imgSplit, segStateSplit, nil, &forcedRootPart, t); err == nil {
			rootSplitMSE := compareBlockToRefY(imgSplit.Y, refY, imgSplit.YStride, 0, 0, 128, 128)
			t.Logf("root replay forced split mse=%.2f", rootSplitMSE)
		}
	}
}

func TestDebugSPBTVFirstFrameRootOffsetProbe(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatal(err)
	}
	rawSamplePos := -1
	for i, s := range dec.track.Samples {
		if s.Offset == parsed.SampleOffset {
			rawSamplePos = i
			break
		}
	}
	if rawSamplePos < 0 {
		t.Fatalf("sample offset %d not found", parsed.SampleOffset)
	}
	rawSample, err := dec.track.ReadSample(rawSamplePos)
	if err != nil {
		t.Fatal(err)
	}
	rawOBUs, err := obu.Split(rawSample)
	if err != nil {
		t.Fatal(err)
	}
	var rawTilePayload []byte
	ctx := &obu.FrameContext{Sequence: dec.header}
	for _, unit := range rawOBUs {
		if unit.Header.Type != obu.OBUFrame {
			continue
		}
		_, payload, err := obu.ParseFrameOBU(unit.Payload, ctx, unit.Header.TemporalID, unit.Header.SpatialID)
		if err != nil {
			t.Fatal(err)
		}
		rawTilePayload = payload
		break
	}
	if len(rawTilePayload) == 0 {
		t.Fatal("no raw tile payload found")
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	tile := states[0]
	syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, tile, 0)
	if err != nil {
		t.Fatal(err)
	}
	row, err := syntax.NewRowState(tile.Geometry.StartSBY)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.SuperBlocks) == 0 {
		t.Fatal("no superblocks")
	}
	sb := row.SuperBlocks[0]
	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if restoration != nil {
		for plane := range restoration.Planes {
			planeState := &restoration.Planes[plane]
			if !planeState.Enabled || planeState.UnitSize <= 0 {
				continue
			}
			ssX, ssY := restorationSubsampling(plane)
			x := sb.StartX >> ssX
			y := sb.StartY >> ssY
			unitX, unitY, ok := planeState.unitCoordsAt(x, y)
			if !ok {
				continue
			}
			idx := planeState.index(unitX, unitY)
			if idx < 0 || idx >= len(planeState.Units) || planeState.Units[idx].Parsed {
				continue
			}
			unit, err := readRestorationUnit(row.Tile.Tile.CDF, row.Tile.Tile.Entropy, plane, parsed.Header.Restoration.Type[plane], restRefs[plane])
			if err != nil {
				t.Fatal(err)
			}
			unit.Parsed = true
			planeState.Units[idx] = unit
			if unit.Type != restorationTypeNone {
				restRefs[plane] = unit
			}
			curP, rngP, cntP, posP := row.Tile.Tile.Entropy.DebugState()
			t.Logf("rest plane=%d type=%d parsed=%v fh=%v fv=%v sgr=%v current=%d rng=%d cnt=%d pos=%d",
				plane, unit.Type, unit.Parsed, unit.FilterH, unit.FilterV, unit.SGRWeights, curP, rngP, cntP, posP)
		}
	}
	root := BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	above, _, err := contextForBlock(dec.header, row, sb, root)
	if err != nil {
		t.Fatal(err)
	}
	by8 := root.Start4Y >> 1
	bx8 := root.Start4X >> 1
	partCtx := GetPartitionCtx(above, &row.Left, root.Level, by8, bx8)
	pcdf := append([]uint16(nil), row.Tile.Tile.CDF.PartitionCDF(root.Level, partCtx)...)
	cur, rng, cnt, pos := row.Tile.Tile.Entropy.DebugState()

	groupBytes := parsed.TileGroup.Data
	firstSizes := make([]int, 0, minInt(4, len(parsed.TileGroup.Tiles)))
	for i := 0; i < minInt(4, len(parsed.TileGroup.Tiles)); i++ {
		firstSizes = append(firstSizes, len(parsed.TileGroup.Tiles[i].Data))
	}
	t.Logf("headerBytes=%d cols=%d rows=%d sizeBytes=%d tileCount=%d tileLens=%v rawTilePayloadFirstBytes=% x groupFirstBytes=% x tileFirstBytes=% x ctx=%d pcdf=%v current=%d rng=%d cnt=%d pos=%d",
		parsed.Header.HeaderBytes,
		parsed.Header.Tiling.Cols,
		parsed.Header.Tiling.Rows,
		parsed.Header.Tiling.SizeBytes,
		len(parsed.TileGroup.Tiles),
		firstSizes,
		rawTilePayload[:minInt(16, len(rawTilePayload))],
		groupBytes[:minInt(16, len(groupBytes))],
		tile.Data[:minInt(16, len(tile.Data))],
		partCtx,
		pcdf,
		cur,
		rng,
		cnt,
		pos)
	if clone := row.Tile.Tile.Entropy.Clone(); clone != nil {
		curCDF := append([]uint16(nil), pcdf...)
		curPart := BlockPartition(clone.DecodeSymbolAdapt(curCDF, int(PartitionTypeCount(root.Level))))
		t.Logf("root current-state part=%d", curPart)
	}
	for off := 0; off < minInt(8, len(tile.Data)); off++ {
		dec := msac.New(tile.Data[off:], parsed.Header.DisableCDFUpdate)
		part := BlockPartition(dec.DecodeSymbolAdapt(append([]uint16(nil), pcdf...), int(PartitionTypeCount(root.Level))))
		t.Logf("root offset=%d part=%d", off, part)
	}
}

func TestDebugSPBTVFirstFrameChainBlocks(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	resetDebugTUTrace()
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	targets := [][2]int{
		{0, 0}, {1, 0}, {0, 16}, {1, 16},
		{32, 0}, {33, 0}, {32, 16}, {33, 16},
		{48, 0}, {49, 0}, {48, 16}, {49, 16},
		{64, 0}, {65, 0}, {64, 16}, {65, 16},
		{128, 0}, {129, 0}, {128, 16}, {129, 16},
		{192, 0}, {193, 0}, {192, 16}, {193, 16},
		{224, 0}, {225, 0}, {224, 16}, {225, 16},
		{288, 0}, {289, 0}, {288, 16}, {289, 16},
		{320, 0}, {321, 0}, {320, 16}, {321, 16},
		{336, 0}, {337, 0}, {336, 16}, {337, 16},
		{352, 0}, {353, 0}, {352, 16}, {353, 16},
		{384, 0}, {385, 0}, {384, 16}, {385, 16}, {384, 32}, {385, 32}, {384, 64}, {385, 64}, {384, 80}, {385, 80},
		{400, 0}, {401, 0}, {400, 16}, {401, 16},
		{416, 0}, {417, 0}, {416, 16}, {417, 16}, {416, 32}, {417, 32}, {416, 64}, {417, 64}, {416, 80}, {417, 80},
		{432, 32}, {433, 32}, {432, 48}, {433, 48}, {432, 64}, {433, 64},
		{448, 32}, {449, 32}, {448, 48}, {449, 48}, {448, 64}, {449, 64}, {448, 80}, {449, 80},
		{464, 48}, {465, 48}, {464, 64}, {465, 64}, {464, 80}, {465, 80},
		{480, 64}, {479, 64}, {480, 80}, {479, 80}, {480, 112}, {479, 112},
	}
	if err := debugKeyframeProblemBlocks(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, segState, targets, t); err != nil {
		t.Fatal(err)
	}
	for _, line := range debugTUTrace {
		t.Log(line)
	}
}

func TestDebugSPBTVSyntheticSkipPaethBlock(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	w, h := 960, 540
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) < w*h {
		t.Fatalf("reference frame too small: %d", len(ref))
	}
	refY := ref[:w*h]
	plane := make([]byte, w*h)
	copy(plane, refY)

	for _, tc := range []struct {
		x, y int
		size BlockSize
		tx   TxfmSize
	}{
		{x: 384, y: 128, size: Block64x128, tx: TX16X16},
		{x: 448, y: 128, size: Block64x64, tx: TX16X16},
	} {
		g := BlockGeometry{
			Size:    tc.size,
			Start4X: tc.x / 4,
			Start4Y: tc.y / 4,
			End4X:   tc.x/4 + blockWidth4ForSize(tc.size),
			End4Y:   tc.y/4 + blockHeight4ForSize(tc.size),
		}
		clearBlockY(plane, w, tc.x, tc.y, g.End4X*4-tc.x, g.End4Y*4-tc.y)
		var above, left BlockContext
		above.Reset(true, 0)
		left.Reset(true, 0)
		syntax := IntraBlockSyntax{
			Geometry: g,
			Size:     tc.size,
			Segment:  0,
			YMode:    IntraPredPaeth,
			UVMode:   IntraPredPaeth,
			TX:       tc.tx,
			UVTX:     TX32X32,
		}
		if err := ReconstructParsedIntraLumaBlockAt8WithSkip(av1.SequenceHeader{}, &obu.FrameHeader{}, nil, nil, DequantTables{}, &above, &left, syntax, plane, w, w, h, g.Start4X, g.Start4Y, true, nil); err != nil {
			t.Fatalf("synthetic skip paeth at (%d,%d): %v", tc.x, tc.y, err)
		}
		t.Logf("synthetic skip paeth block (%d,%d) mse=%.2f", tc.x, tc.y, compareBlockToRefY(plane, refY, w, tc.x, tc.y, minInt(g.End4X*4-tc.x, 16), minInt(g.End4Y*4-tc.y, 16)))
	}
}

func TestDebugSPBTVSyntheticSkipPaethBlockEdgeSource(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	const w, h = 960, 540
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		t.Fatal(err)
	}
	ours, err := os.ReadFile("/tmp/spbtv_frame0_ours.yuv")
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) < w*h || len(ours) < w*h {
		t.Fatalf("reference/ours frame too small: ref=%d ours=%d", len(ref), len(ours))
	}
	refY := ref[:w*h]
	oursY := ours[:w*h]

	type blockCase struct {
		x, y int
		size BlockSize
		tx   TxfmSize
	}
	blocks := []blockCase{
		{x: 384, y: 128, size: Block64x128, tx: TX16X16},
		{x: 448, y: 128, size: Block64x64, tx: TX16X16},
		{x: 480, y: 96, size: Block8x32, tx: TX8X8},
	}
	for _, tc := range blocks {
		for _, edgeCase := range []struct {
			name    string
			useTop  bool
			useLeft bool
		}{
			{name: "ours", useTop: false, useLeft: false},
			{name: "ref_top", useTop: true, useLeft: false},
			{name: "ref_left", useTop: false, useLeft: true},
			{name: "ref_both", useTop: true, useLeft: true},
		} {
			plane := make([]byte, len(oursY))
			copy(plane, oursY)
			if edgeCase.useTop && tc.y > 0 {
				copy(plane[(tc.y-1)*w:tc.y*w], refY[(tc.y-1)*w:tc.y*w])
			}
			if edgeCase.useLeft && tc.x > 0 {
				for py := 0; py < h; py++ {
					plane[py*w+tc.x-1] = refY[py*w+tc.x-1]
				}
			}
			g := BlockGeometry{
				Size:    tc.size,
				Start4X: tc.x / 4,
				Start4Y: tc.y / 4,
				End4X:   tc.x/4 + blockWidth4ForSize(tc.size),
				End4Y:   tc.y/4 + blockHeight4ForSize(tc.size),
			}
			clearBlockY(plane, w, tc.x, tc.y, g.End4X*4-tc.x, g.End4Y*4-tc.y)
			var above, left BlockContext
			above.Reset(true, 0)
			left.Reset(true, 0)
			syntax := IntraBlockSyntax{
				Geometry: g,
				Size:     tc.size,
				Segment:  0,
				YMode:    IntraPredPaeth,
				UVMode:   IntraPredPaeth,
				TX:       tc.tx,
				UVTX:     TX32X32,
			}
			if err := ReconstructParsedIntraLumaBlockAt8WithSkip(av1.SequenceHeader{}, &obu.FrameHeader{}, nil, nil, DequantTables{}, &above, &left, syntax, plane, w, w, h, g.Start4X, g.Start4Y, true, nil); err != nil {
				t.Fatalf("%s synthetic skip paeth at (%d,%d): %v", edgeCase.name, tc.x, tc.y, err)
			}
			t.Logf("%s synthetic skip paeth block (%d,%d) mse=%.2f", edgeCase.name, tc.x, tc.y, compareBlockToRefY(plane, refY, w, tc.x, tc.y, minInt(g.End4X*4-tc.x, 16), minInt(g.End4Y*4-tc.y, 16)))
		}
	}
}

func TestDebugSPBTVFirstFrameCDEFOrderComparison(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		t.Skipf("reference frame unavailable: %v", err)
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	ySize := parsed.Header.Width * parsed.Header.Height
	uvW := (parsed.Header.Width + 1) >> 1
	uvH := (parsed.Header.Height + 1) >> 1
	uvSize := uvW * uvH
	if len(ref) < ySize+2*uvSize {
		t.Fatalf("reference frame too small: len=%d want>=%d", len(ref), ySize+2*uvSize)
	}
	refY := ref[:ySize]
	refU := ref[ySize : ySize+uvSize]
	refV := ref[ySize+uvSize : ySize+2*uvSize]

	for _, cdefBefore := range []bool{false, true} {
		img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
		fillUint8(img.Cb, 128)
		fillUint8(img.Cr, 128)
		dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
		cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
		edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
		restoration := NewRestorationState(dec.header, &parsed.Header)
		segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		if err := decodeKeyframeMain8DCLumaWithCDEFOrder(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, restoration, segState, cdefBefore); err != nil {
			t.Fatalf("decode cdefBefore=%v: %v", cdefBefore, err)
		}
		applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
		applyCDEF420_8(img, &parsed.Header, cdef)
		applyRestoration420_8(img, &parsed.Header, restoration)
		t.Logf("cdefBefore=%v Y rmse=%.4f U rmse=%.4f V rmse=%.4f",
			cdefBefore,
			debugPlaneRMSE(img.Y, refY),
			debugPlaneRMSE(img.Cb, refU),
			debugPlaneRMSE(img.Cr, refV),
		)
	}
}

func TestDebugSPBTVFirstBlockNextModeVariants(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	row, err := syntax.NewRowState(states[0].Geometry.StartSBY)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.SuperBlocks) == 0 {
		t.Fatal("no superblocks")
	}
	sb := row.SuperBlocks[0]
	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
		t.Fatal(err)
	}
	root := BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	part, err := DecodePartitionDecision(dec.header, &parsed.Header, row.Tile.Tile.CDF, &row.Tile.Above[0], &row.Left, root, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	if part != PartitionSplit {
		t.Fatalf("root partition = %v, want split", part)
	}
	children := splitChildren(root)
	if len(children) < 2 {
		t.Fatal("not enough split children")
	}
	g := children[0]
	nextG := children[1]

	above := row.Tile.Above[0]
	left := row.Left
	segID := uint8(0)
	segPred := false
	skipMode, skip, err := ReadIntraSkipPrelude(&parsed.Header, row.Tile.Tile.CDF, &above, &left, g, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	_ = skipMode
	if parsed.Header.CDEF.NBits > 0 {
		if err := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height).ensureBlockIndex(g, parsed.Header.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
			t.Fatal(err)
		}
	}
	if err := readDeltaQAndLFAtSuperBlockStart(dec.header, &parsed.Header, row.Tile, g, skip); err != nil {
		t.Fatal(err)
	}
	firstSyntax, err := ReadIntraBlockSyntax(dec.header, &parsed.Header, row.Tile.Tile.CDF, &above, &left, g, false, false, segID, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	firstSyntax.Geometry = g
	firstSyntax.SegPred = segPred
	firstSyntax.Skip = skip
	t.Logf("first block tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d", txName(firstSyntax.TX), txName(firstSyntax.UVTX), intraModeName(firstSyntax.YMode), intraModeName(firstSyntax.UVMode), firstSyntax.PalYSize, firstSyntax.PalUVSize)

	baseDec := row.Tile.Tile.Entropy.Clone()
	baseAbove := above
	baseLeft := left

	lumaOnlyDec := baseDec.Clone()
	lumaOnlyAbove := baseAbove
	lumaOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &lumaOnlyAbove, &lumaOnlyLeft, firstSyntax, lumaOnlyDec); err != nil {
		t.Fatal(err)
	}
	mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &lumaOnlyAbove, &lumaOnlyLeft, nextG, lumaOnlyDec, 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after luma-only: %s", intraModeName(mode))

	uOnlyDec := baseDec.Clone()
	uOnlyAbove := baseAbove
	uOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &uOnlyAbove, &uOnlyLeft, firstSyntax, uOnlyDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChromaPlanes(dec.header, &parsed.Header, row.Tile.Tile.Coef, dq, &uOnlyAbove, &uOnlyLeft, firstSyntax, uOnlyDec, []int{1}); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &uOnlyAbove, &uOnlyLeft, nextG, uOnlyDec, 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after U-only chroma: %s", intraModeName(mode))

	vOnlyDec := baseDec.Clone()
	vOnlyAbove := baseAbove
	vOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &vOnlyAbove, &vOnlyLeft, firstSyntax, vOnlyDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChromaPlanes(dec.header, &parsed.Header, row.Tile.Tile.Coef, dq, &vOnlyAbove, &vOnlyLeft, firstSyntax, vOnlyDec, []int{2}); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &vOnlyAbove, &vOnlyLeft, nextG, vOnlyDec, 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after V-only chroma: %s", intraModeName(mode))

	currentDec := baseDec.Clone()
	currentAbove := baseAbove
	currentLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &currentAbove, &currentLeft, firstSyntax, currentDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, dq, &currentAbove, &currentLeft, firstSyntax, currentDec, false); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &currentAbove, &currentLeft, nextG, currentDec, 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after current chroma order: %s", intraModeName(mode))

	swappedDec := baseDec.Clone()
	swappedAbove := baseAbove
	swappedLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &swappedAbove, &swappedLeft, firstSyntax, swappedDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, dq, &swappedAbove, &swappedLeft, firstSyntax, swappedDec, true); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &swappedAbove, &swappedLeft, nextG, swappedDec, 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after swapped chroma order: %s", intraModeName(mode))
}

func TestDebugSPBTVFirstLeafNextModeVariantsCDEFBefore(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	row, err := syntax.NewRowState(states[0].Geometry.StartSBY)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.SuperBlocks) == 0 {
		t.Fatal("no superblocks")
	}
	sb := row.SuperBlocks[0]
	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
		t.Fatal(err)
	}
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	state, err := debugAdvanceToTargetLeafCDEFBefore(dec.header, &parsed.Header, row, sb, BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}, segState, 0, 0, 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	firstSyntax := state.syntax
	t.Logf("cdef-before first leaf tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d",
		txName(firstSyntax.TX), txName(firstSyntax.UVTX), intraModeName(firstSyntax.YMode), intraModeName(firstSyntax.UVMode), firstSyntax.PalYSize, firstSyntax.PalUVSize)

	baseDec := state.dec.Clone()
	baseAbove := state.above
	baseLeft := state.left
	var mode IntraPredMode
	var nextSyntax IntraBlockSyntax
	var nextSkip bool

	lumaOnlyDec := baseDec.Clone()
	lumaOnlyAbove := baseAbove
	lumaOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &lumaOnlyAbove, &lumaOnlyLeft, firstSyntax, lumaOnlyDec); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &lumaOnlyAbove, &lumaOnlyLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &lumaOnlyAbove, &lumaOnlyLeft, state.nextRoot, lumaOnlyDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after luma-only: %s", intraModeName(mode))
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &lumaOnlyAbove, &lumaOnlyLeft, state.nextRoot, segState, lumaOnlyDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after luma-only: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d", intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)

	for _, maxTUs := range []int{1, 4, 8} {
		partialDec := baseDec.Clone()
		partialAbove := baseAbove
		partialLeft := baseLeft
		if err := debugConsumeFirstBlockLumaLimit(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &partialAbove, &partialLeft, firstSyntax, partialDec, maxTUs); err != nil {
			t.Fatalf("consume first block luma limit=%d: %v", maxTUs, err)
		}
		if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &partialAbove, &partialLeft, firstSyntax); err != nil {
			t.Fatal(err)
		}
		mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &partialAbove, &partialLeft, state.nextRoot, partialDec.Clone(), 32, 0)
		if err != nil {
			t.Fatalf("peek next block after luma limit=%d: %v", maxTUs, err)
		}
		t.Logf("cdef-before next block after luma-first-%d-tus: %s", maxTUs, intraModeName(mode))
		nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &partialAbove, &partialLeft, state.nextRoot, segState, partialDec.Clone(), 32, 0)
		if err != nil {
			t.Fatalf("peek next block syntax after luma limit=%d: %v", maxTUs, err)
		}
		t.Logf("cdef-before next block syntax after luma-first-%d-tus: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
			maxTUs, intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
	}

	uOnlyDec := baseDec.Clone()
	uOnlyAbove := baseAbove
	uOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &uOnlyAbove, &uOnlyLeft, firstSyntax, uOnlyDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChromaPlanes(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &uOnlyAbove, &uOnlyLeft, firstSyntax, uOnlyDec, []int{1}); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &uOnlyAbove, &uOnlyLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &uOnlyAbove, &uOnlyLeft, state.nextRoot, uOnlyDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after U-only chroma: %s", intraModeName(mode))
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &uOnlyAbove, &uOnlyLeft, state.nextRoot, segState, uOnlyDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after U-only chroma: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d", intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)

	vOnlyDec := baseDec.Clone()
	vOnlyAbove := baseAbove
	vOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &vOnlyAbove, &vOnlyLeft, firstSyntax, vOnlyDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChromaPlanes(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &vOnlyAbove, &vOnlyLeft, firstSyntax, vOnlyDec, []int{2}); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &vOnlyAbove, &vOnlyLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &vOnlyAbove, &vOnlyLeft, state.nextRoot, vOnlyDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after V-only chroma: %s", intraModeName(mode))
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &vOnlyAbove, &vOnlyLeft, state.nextRoot, segState, vOnlyDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after V-only chroma: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d", intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)

	currentDec := baseDec.Clone()
	currentAbove := baseAbove
	currentLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &currentAbove, &currentLeft, firstSyntax, currentDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &currentAbove, &currentLeft, firstSyntax, currentDec, false); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &currentAbove, &currentLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &currentAbove, &currentLeft, state.nextRoot, currentDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after current chroma order: %s", intraModeName(mode))
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &currentAbove, &currentLeft, state.nextRoot, segState, currentDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after current chroma order: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d", intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)

	swappedDec := baseDec.Clone()
	swappedAbove := baseAbove
	swappedLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &swappedAbove, &swappedLeft, firstSyntax, swappedDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &swappedAbove, &swappedLeft, firstSyntax, swappedDec, true); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &swappedAbove, &swappedLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	mode, err = debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &swappedAbove, &swappedLeft, state.nextRoot, swappedDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after swapped chroma order: %s", intraModeName(mode))
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &swappedAbove, &swappedLeft, state.nextRoot, segState, swappedDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after swapped chroma order: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d", intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
}

func TestDebugSPBTVFirstLeafNoCFLSyntaxVariantCDEFBefore(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	targetState := func(readSyntax debugIntraSyntaxReader) (*TileRowSyntaxState, SuperBlockGeometry, *SegmentationState, debugTargetLeafAdvanceState) {
		states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
		if err != nil {
			t.Fatal(err)
		}
		if len(states) == 0 {
			t.Fatal("no tile states")
		}
		syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
		if err != nil {
			t.Fatal(err)
		}
		row, err := syntax.NewRowState(states[0].Geometry.StartSBY)
		if err != nil {
			t.Fatal(err)
		}
		if len(row.SuperBlocks) == 0 {
			t.Fatal("no superblocks")
		}
		sb := row.SuperBlocks[0]
		restoration := NewRestorationState(dec.header, &parsed.Header)
		restRefs := newTileRestorationRefs()
		if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
			t.Fatal(err)
		}
		root := BlockGeometry{
			Level:   RootBlockLevel(dec.header),
			Size:    RootBlockSize(dec.header),
			Start4X: sb.Start4X,
			Start4Y: sb.Start4Y,
			End4X:   sb.End4X,
			End4Y:   sb.End4Y,
		}
		segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		state, err := debugAdvanceToTargetLeafCDEFBeforeWithReader(dec.header, &parsed.Header, row, sb, root, segState, 0, 0, 32, 0, readSyntax)
		if err != nil {
			t.Fatal(err)
		}
		return row, sb, segState, state
	}

	_, _, _, actualState := targetState(nil)
	altRow, _, altSegState, noCFLState := targetState(debugReadIntraBlockSyntaxNoCFL)

	t.Logf("cdef-before actual first leaf tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d",
		txName(actualState.syntax.TX), txName(actualState.syntax.UVTX), intraModeName(actualState.syntax.YMode), intraModeName(actualState.syntax.UVMode), actualState.syntax.PalYSize, actualState.syntax.PalUVSize)
	t.Logf("cdef-before no_cfl first leaf tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d nextRoot=(%d,%d)-(%d,%d)",
		txName(noCFLState.syntax.TX), txName(noCFLState.syntax.UVTX), intraModeName(noCFLState.syntax.YMode), intraModeName(noCFLState.syntax.UVMode), noCFLState.syntax.PalYSize, noCFLState.syntax.PalUVSize,
		noCFLState.nextRoot.Start4X*4, noCFLState.nextRoot.Start4Y*4, noCFLState.nextRoot.End4X*4, noCFLState.nextRoot.End4Y*4)

	noCFLLumaDec := noCFLState.dec.Clone()
	noCFLLumaAbove := noCFLState.above
	noCFLLumaLeft := noCFLState.left
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, altRow.Tile.Tile.CDF, altRow.Tile.Tile.Coef, noCFLState.dq, &noCFLLumaAbove, &noCFLLumaLeft, noCFLState.syntax, noCFLLumaDec); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &noCFLLumaAbove, &noCFLLumaLeft, noCFLState.syntax); err != nil {
		t.Fatal(err)
	}
	mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, altRow.Tile.Tile.CDF, &noCFLLumaAbove, &noCFLLumaLeft, noCFLState.nextRoot, noCFLLumaDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after no_cfl first-leaf syntax + luma-only: %s", intraModeName(mode))
	nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, altRow.Tile.Tile.CDF, &noCFLLumaAbove, &noCFLLumaLeft, noCFLState.nextRoot, altSegState, noCFLLumaDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after no_cfl first-leaf syntax + luma-only: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
		intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
}

func TestDebugSPBTVFirstLeafTxBeforePaletteIndicesVariantCDEFBefore(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	targetState := func(readSyntax debugIntraSyntaxReader) (*TileRowSyntaxState, *SegmentationState, debugTargetLeafAdvanceState) {
		states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
		if err != nil {
			t.Fatal(err)
		}
		if len(states) == 0 {
			t.Fatal("no tile states")
		}
		syntaxState, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
		if err != nil {
			t.Fatal(err)
		}
		row, err := syntaxState.NewRowState(states[0].Geometry.StartSBY)
		if err != nil {
			t.Fatal(err)
		}
		if len(row.SuperBlocks) == 0 {
			t.Fatal("no superblocks")
		}
		sb := row.SuperBlocks[0]
		restoration := NewRestorationState(dec.header, &parsed.Header)
		restRefs := newTileRestorationRefs()
		if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
			t.Fatal(err)
		}
		root := BlockGeometry{
			Level:   RootBlockLevel(dec.header),
			Size:    RootBlockSize(dec.header),
			Start4X: sb.Start4X,
			Start4Y: sb.Start4Y,
			End4X:   sb.End4X,
			End4Y:   sb.End4Y,
		}
		segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		state, err := debugAdvanceToTargetLeafCDEFBeforeWithReader(dec.header, &parsed.Header, row, sb, root, segState, 0, 0, 32, 0, readSyntax)
		if err != nil {
			t.Fatal(err)
		}
		return row, segState, state
	}

	_, _, actualState := targetState(nil)
	altRow, altSegState, altState := targetState(debugReadIntraBlockSyntaxTxBeforePaletteIndices)

	t.Logf("cdef-before actual first leaf tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d",
		txName(actualState.syntax.TX), txName(actualState.syntax.UVTX), intraModeName(actualState.syntax.YMode), intraModeName(actualState.syntax.UVMode), actualState.syntax.PalYSize, actualState.syntax.PalUVSize)
	t.Logf("cdef-before tx_before_palette_indices first leaf tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d nextRoot=(%d,%d)-(%d,%d)",
		txName(altState.syntax.TX), txName(altState.syntax.UVTX), intraModeName(altState.syntax.YMode), intraModeName(altState.syntax.UVMode), altState.syntax.PalYSize, altState.syntax.PalUVSize,
		altState.nextRoot.Start4X*4, altState.nextRoot.Start4Y*4, altState.nextRoot.End4X*4, altState.nextRoot.End4Y*4)

	altLumaDec := altState.dec.Clone()
	altLumaAbove := altState.above
	altLumaLeft := altState.left
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, altRow.Tile.Tile.CDF, altRow.Tile.Tile.Coef, altState.dq, &altLumaAbove, &altLumaLeft, altState.syntax, altLumaDec); err != nil {
		t.Fatal(err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &altLumaAbove, &altLumaLeft, altState.syntax); err != nil {
		t.Fatal(err)
	}
	mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, altRow.Tile.Tile.CDF, &altLumaAbove, &altLumaLeft, altState.nextRoot, altLumaDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block after tx_before_palette_indices first-leaf syntax + luma-only: %s", intraModeName(mode))
	nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, altRow.Tile.Tile.CDF, &altLumaAbove, &altLumaLeft, altState.nextRoot, altSegState, altLumaDec.Clone(), 32, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cdef-before next block syntax after tx_before_palette_indices first-leaf syntax + luma-only: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
		intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
}

func TestDebugSPBTVFirstBlockUVTXVariants(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("set DEBUG_FIRST_KEY_BLOCKS=1 to enable first-keyframe block tracing")
	}

	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	row, err := syntax.NewRowState(states[0].Geometry.StartSBY)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.SuperBlocks) == 0 {
		t.Fatal("no superblocks")
	}
	sb := row.SuperBlocks[0]
	root := BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}

	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
		t.Fatal(err)
	}
	part, err := DecodePartitionDecision(dec.header, &parsed.Header, row.Tile.Tile.CDF, &row.Tile.Above[0], &row.Left, root, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	if part != PartitionSplit {
		t.Fatalf("root partition = %v, want split", part)
	}
	children := splitChildren(root)
	if len(children) < 2 {
		t.Fatal("not enough split children")
	}
	g := children[0]
	nextG := children[1]

	above := row.Tile.Above[0]
	left := row.Left
	segID := uint8(0)
	segPred := false
	skipMode, skip, err := ReadIntraSkipPrelude(&parsed.Header, row.Tile.Tile.CDF, &above, &left, g, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	_ = skipMode
	if parsed.Header.CDEF.NBits > 0 {
		if err := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height).ensureBlockIndex(g, parsed.Header.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
			t.Fatal(err)
		}
	}
	if err := readDeltaQAndLFAtSuperBlockStart(dec.header, &parsed.Header, row.Tile, g, skip); err != nil {
		t.Fatal(err)
	}
	firstSyntax, err := ReadIntraBlockSyntax(dec.header, &parsed.Header, row.Tile.Tile.CDF, &above, &left, g, false, false, segID, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	firstSyntax.Geometry = g
	firstSyntax.SegPred = segPred
	firstSyntax.Skip = skip
	t.Logf("first block tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d", txName(firstSyntax.TX), txName(firstSyntax.UVTX), intraModeName(firstSyntax.YMode), intraModeName(firstSyntax.UVMode), firstSyntax.PalYSize, firstSyntax.PalUVSize)

	baseDec := row.Tile.Tile.Entropy.Clone()
	baseAbove := above
	baseLeft := left

	variants := []TxfmSize{firstSyntax.UVTX}
	if firstSyntax.TX != firstSyntax.UVTX {
		variants = append(variants, firstSyntax.TX)
	}
	if firstSyntax.TX != TX4X4 && firstSyntax.UVTX != TX4X4 {
		variants = append(variants, TX4X4)
	}
	seen := map[TxfmSize]bool{}
	for _, uvtx := range variants {
		if seen[uvtx] {
			continue
		}
		seen[uvtx] = true
		altDec := baseDec.Clone()
		altAbove := baseAbove
		altLeft := baseLeft
		altSyntax := firstSyntax
		altSyntax.UVTX = uvtx
		if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &altAbove, &altLeft, altSyntax, altDec); err != nil {
			t.Fatalf("consume first block luma uvtx=%s: %v", txName(uvtx), err)
		}
		if err := debugConsumeFirstBlockChromaPlanes(dec.header, &parsed.Header, row.Tile.Tile.Coef, dq, &altAbove, &altLeft, altSyntax, altDec, []int{2}); err != nil {
			t.Logf("uvtx=%s V-only consume err=%v", txName(uvtx), err)
			continue
		}
		mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &altAbove, &altLeft, nextG, altDec, 32, 0)
		if err != nil {
			t.Logf("uvtx=%s next block peek err=%v", txName(uvtx), err)
			continue
		}
		t.Logf("uvtx=%s next block after V-only chroma: %s", txName(uvtx), intraModeName(mode))
	}
}

func TestDebugSPBTVFirstBlockUVTxTypeVariants(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("set DEBUG_FIRST_KEY_BLOCKS=1 to enable first-keyframe block tracing")
	}

	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	row, err := syntax.NewRowState(states[0].Geometry.StartSBY)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.SuperBlocks) == 0 {
		t.Fatal("no superblocks")
	}
	sb := row.SuperBlocks[0]
	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
		t.Fatal(err)
	}
	root := BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	part, err := DecodePartitionDecision(dec.header, &parsed.Header, row.Tile.Tile.CDF, &row.Tile.Above[0], &row.Left, root, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	if part != PartitionSplit {
		t.Fatalf("root partition = %v, want split", part)
	}
	children := splitChildren(root)
	if len(children) < 2 {
		t.Fatal("not enough split children")
	}
	g := children[0]
	nextG := children[1]

	above := row.Tile.Above[0]
	left := row.Left
	segID := uint8(0)
	segPred := false
	skipMode, skip, err := ReadIntraSkipPrelude(&parsed.Header, row.Tile.Tile.CDF, &above, &left, g, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	_ = skipMode
	if parsed.Header.CDEF.NBits > 0 {
		if err := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height).ensureBlockIndex(g, parsed.Header.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
			t.Fatal(err)
		}
	}
	if err := readDeltaQAndLFAtSuperBlockStart(dec.header, &parsed.Header, row.Tile, g, skip); err != nil {
		t.Fatal(err)
	}
	firstSyntax, err := ReadIntraBlockSyntax(dec.header, &parsed.Header, row.Tile.Tile.CDF, &above, &left, g, false, false, segID, row.Tile.Tile.Entropy)
	if err != nil {
		t.Fatal(err)
	}
	firstSyntax.Geometry = g
	firstSyntax.SegPred = segPred
	firstSyntax.Skip = skip

	chromaGeom := chromaGeometryForBlock(dec.header.ColorConfig.ChromaSubsampling(), firstSyntax.Geometry)
	txInfo := TxfmInfoFor(firstSyntax.UVTX)
	baseDec := row.Tile.Tile.Entropy.Clone()
	baseAbove := above
	baseLeft := left
	t.Logf("first block uvtx=%s uvmode=%s current_uvtxtype=%d", txName(firstSyntax.UVTX), intraModeName(firstSyntax.UVMode), IntraUVTxType(&parsed.Header, firstSyntax.Segment, firstSyntax.UVTX, firstSyntax.UVMode))

	for _, uvTxType := range debugReplayAltTxTypes(firstSyntax.UVTX) {
		altDec := baseDec.Clone()
		altAbove := baseAbove
		altLeft := baseLeft
		if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, dq, &altAbove, &altLeft, firstSyntax, altDec); err != nil {
			t.Fatalf("consume first block luma txType=%d: %v", uvTxType, err)
		}
		if err := debugConsumeFirstBlockChromaPlanesWithGeom(dec.header, &parsed.Header, row.Tile.Tile.Coef, dq, &altAbove, &altLeft, firstSyntax, chromaGeom, txInfo, uvTxType, altDec, []int{2}); err != nil {
			t.Logf("uvtxType=%d V-only consume err=%v", uvTxType, err)
			continue
		}
		mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &altAbove, &altLeft, nextG, altDec, 32, 0)
		if err != nil {
			t.Logf("uvtxType=%d next block peek err=%v", uvTxType, err)
			continue
		}
		t.Logf("uvtxType=%d next block after V-only chroma: %s", uvTxType, intraModeName(mode))
	}
}

func debugConsumeFirstBlockLuma(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder) error {
	return debugConsumeFirstBlockLumaLimit(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, dec, -1)
}

func debugConsumeFirstBlockLumaLimit(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder, maxTUs int) error {
	return debugConsumeFirstBlockLumaLimitWithTXTypeOverrides(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, dec, maxTUs, nil)
}

func debugConsumeFirstBlockLumaLimitWithTXTypeOverrides(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder, maxTUs int, txTypeOverrides map[int]TxType) error {
	g := syntax.Geometry
	units, err := ReadIntraTransformUnitsInto(nil, hdr, modeCDF, above, left, g, syntax.TX, syntax.Segment, dec)
	if err != nil {
		return err
	}
	limit := len(units)
	if maxTUs >= 0 && maxTUs < limit {
		limit = maxTUs
	}
	for i := 0; i < limit; i++ {
		tu := units[i].Global
		tx := units[i].TX
		txInfo := TxfmInfoFor(tx)
		offX4 := tu.Start4X - g.Start4X
		offY4 := tu.Start4Y - g.Start4Y
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		tuHaveTop := g.Start4Y > 0 || offY4 > 0
		tuHaveLeft := g.Start4X > 0 || offX4 > 0
		skipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		if dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
			if err := ApplyLumaResidualContext(above, left, tu, tx, 0x40); err != nil {
				return err
			}
			continue
		}
		yTxType, err := ReadIntraTxType(hdr, modeCDF, syntax, tx, tuHaveTop, tuHaveLeft, dec)
		if err != nil {
			return err
		}
		if forced, ok := txTypeOverrides[i]; ok {
			yTxType = forced
		}
		_, resCtx, _, err := DecodeLumaCoefficientsNoSkip(syntax.Size, tu, tx, yTxType, syntax.Segment, coefCDF, dq, above, left, dec)
		if err != nil {
			return err
		}
		if err := ApplyLumaResidualContext(above, left, tu, tx, resCtx); err != nil {
			return err
		}
	}
	return nil
}

func debugConsumeFirstBlockChroma(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder, swapUV bool) error {
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), syntax.Geometry)
	txInfo := TxfmInfoFor(syntax.UVTX)
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	planes := []int{1, 2}
	if swapUV {
		planes = []int{2, 1}
	}
	return debugConsumeFirstBlockChromaPlanesWithGeomAndCtx(seq, hdr, coefCDF, dq, above, left, syntax, chromaGeom, txInfo, uvTxType, dec, planes, true)
}

func debugConsumeFirstBlockChromaPlanes(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder, planes []int) error {
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), syntax.Geometry)
	txInfo := TxfmInfoFor(syntax.UVTX)
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	return debugConsumeFirstBlockChromaPlanesWithGeomAndCtx(seq, hdr, coefCDF, dq, above, left, syntax, chromaGeom, txInfo, uvTxType, dec, planes, true)
}

func debugConsumeFirstBlockChromaPlanesWithGeom(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, chromaGeom BlockGeometry, txInfo TxfmInfo, uvTxType TxType, dec keyIntraBlockEntropyDecoder, planes []int) error {
	return debugConsumeFirstBlockChromaPlanesWithGeomAndCtx(seq, hdr, coefCDF, dq, above, left, syntax, chromaGeom, txInfo, uvTxType, dec, planes, true)
}

func debugConsumeFirstBlockChromaPlanesNoResidualContext(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder, planes []int) error {
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), syntax.Geometry)
	txInfo := TxfmInfoFor(syntax.UVTX)
	uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
	return debugConsumeFirstBlockChromaPlanesWithGeomAndCtx(seq, hdr, coefCDF, dq, above, left, syntax, chromaGeom, txInfo, uvTxType, dec, planes, false)
}

func debugConsumeFirstBlockChromaPlanesWithGeomAndCtx(seq av1.SequenceHeader, hdr *obu.FrameHeader, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, chromaGeom BlockGeometry, txInfo TxfmInfo, uvTxType TxType, dec keyIntraBlockEntropyDecoder, planes []int, applyCtx bool) error {
	for _, plane := range planes {
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
				_, resCtx, _, err := DecodeChromaCoefficients(syntax.Size, tu, syntax.UVTX, uvTxType, syntax.Segment, plane, seq.ColorConfig.ChromaSubsampling(), coefCDF, dq, above, left, dec)
				if err != nil {
					return err
				}
				if applyCtx {
					if err := ApplyChromaResidualContext(above, left, plane, tu, syntax.UVTX, resCtx); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func debugPeekNextBlockYMode(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, dec *msac.Context) (IntraPredMode, error) {
	if hdr.CDEF.NBits > 0 {
		if err := NewCDEFIndexMap(hdr.Width, hdr.Height).ensureBlockIndex(g, hdr.CDEF.NBits, false, msacEntropyDecoder{ctx: dec}); err != nil {
			return 0, err
		}
	}
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	return IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1))), nil
}

func debugPeekNextLeafYMode(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, dec *msac.Context, targetX, targetY int) (IntraPredMode, error) {
	leaf, err := debugDescendToPointLeaf(seq, hdr, cdf, above, left, g, dec, targetX, targetY)
	if err != nil {
		return 0, err
	}
	return debugPeekNextBlockYMode(seq, hdr, cdf, above, left, leaf, dec)
}

func debugPeekNextLeafSyntaxCDEFBefore(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, segState *SegmentationState, dec *msac.Context, targetX, targetY int) (IntraBlockSyntax, bool, error) {
	leaf, err := debugDescendToPointLeaf(seq, hdr, cdf, above, left, g, dec, targetX, targetY)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	local := leaf
	segID := uint8(0)
	segPred := false
	haveTop := leaf.Start4Y > 0
	haveLeft := leaf.Start4X > 0
	msacDec := msacEntropyDecoder{ctx: dec}
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, cdf, above, left, segState, leaf, haveTop, haveLeft, false, false, msacDec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
	}
	_, skip, err := ReadIntraSkipPrelude(hdr, cdf, above, left, local, msacDec)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, cdf, above, left, segState, leaf, haveTop, haveLeft, true, skip, msacDec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
	}
	if segmentDataForID(hdr, segID).Skip {
		skip = true
	}
	if err := NewCDEFIndexMap(hdr.Width, hdr.Height).ensureBlockIndex(leaf, hdr.CDEF.NBits, skip, msacDec); err != nil {
		return IntraBlockSyntax{}, false, err
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, local, haveTop, haveLeft, segID, msacDec)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	syntax.Geometry = leaf
	syntax.SegPred = segPred
	syntax.Skip = skip
	return syntax, skip, nil
}

type debugIntraSyntaxReader func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error)

func debugAdvanceToTargetLeafCDEFBefore(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState, targetX, targetY, nextX, nextY int) (debugTargetLeafAdvanceState, error) {
	return debugAdvanceToTargetLeafCDEFBeforeWithReader(seq, hdr, row, sb, g, segState, targetX, targetY, nextX, nextY, nil)
}

type debugMSACCombinedDecoder struct {
	ctx *msac.Context
}

func (d debugMSACCombinedDecoder) DecodeBool(prob uint32) uint32 { return d.ctx.DecodeBool(prob) }
func (d debugMSACCombinedDecoder) DecodeBoolEqui() uint32        { return d.ctx.DecodeBoolEqui() }
func (d debugMSACCombinedDecoder) DecodeBoolAdapt(cdf []uint16) uint32 {
	return d.ctx.DecodeBoolAdapt(cdf)
}
func (d debugMSACCombinedDecoder) DecodeUniform(n uint32) int { return d.ctx.DecodeUniform(n) }
func (d debugMSACCombinedDecoder) DecodeBools(n uint) uint32  { return d.ctx.DecodeBools(n) }
func (d debugMSACCombinedDecoder) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	return d.ctx.DecodeSymbolAdapt(cdf, n)
}
func (d debugMSACCombinedDecoder) DecodeHiTok(cdf []uint16) uint32 { return d.ctx.DecodeHiTok(cdf) }

func debugApplySkipResidualContexts(seq av1.SequenceHeader, above, left *BlockContext, syntax IntraBlockSyntax) error {
	g := syntax.Geometry
	txInfo := TxfmInfoFor(syntax.TX)
	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
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
			if err := ApplyLumaResidualContext(above, left, tu, syntax.TX, 0x40); err != nil {
				return err
			}
		}
	}
	if !hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		return nil
	}
	chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), syntax.Geometry)
	uvInfo := TxfmInfoFor(syntax.UVTX)
	for offY4 := 0; offY4 < chromaGeom.Height4(); offY4 += int(uvInfo.H4) {
		for offX4 := 0; offX4 < chromaGeom.Width4(); offX4 += int(uvInfo.W4) {
			tu := BlockGeometry{
				Level:   chromaGeom.Level,
				Size:    chromaGeom.Size,
				Start4X: chromaGeom.Start4X + offX4,
				Start4Y: chromaGeom.Start4Y + offY4,
				End4X:   min(chromaGeom.Start4X+offX4+int(uvInfo.W4), chromaGeom.End4X),
				End4Y:   min(chromaGeom.Start4Y+offY4+int(uvInfo.H4), chromaGeom.End4Y),
			}
			if err := ApplyChromaResidualContext(above, left, 1, tu, syntax.UVTX, 0x40); err != nil {
				return err
			}
			if err := ApplyChromaResidualContext(above, left, 2, tu, syntax.UVTX, 0x40); err != nil {
				return err
			}
		}
	}
	return nil
}

func debugConsumeLeafCDEFBeforeWithReader(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState, readSyntax debugIntraSyntaxReader) error {
	state, err := debugAdvanceLeafSyntaxCDEFBeforeWithReader(seq, hdr, row, sb, g, segState, readSyntax)
	if err != nil {
		return err
	}
	if state.syntax.Skip {
		if err := debugApplySkipResidualContexts(seq, &state.above, &state.left, state.syntax); err != nil {
			return err
		}
	} else {
		dec := debugMSACCombinedDecoder{ctx: state.dec}
		if err := debugConsumeFirstBlockLuma(seq, hdr, state.mode, state.coef, state.dq, &state.above, &state.left, state.syntax, dec); err != nil {
			return err
		}
		if err := debugConsumeFirstBlockChroma(seq, hdr, state.coef, state.dq, &state.above, &state.left, state.syntax, dec, false); err != nil {
			return err
		}
	}
	state.syntax.PreserveTXContext = !state.syntax.Skip
	if err := ApplyIntraBlockContext(seq, hdr, &state.above, &state.left, state.syntax); err != nil {
		return err
	}
	above, _, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	*above = state.above
	row.Left = state.left
	row.Tile.LastQIdx = state.qidx
	row.Tile.CurrentDQ = state.dq
	if state.mode != nil {
		row.Tile.Tile.CDF = state.mode
	}
	if state.coef != nil {
		row.Tile.Tile.Coef = state.coef
	}
	if state.dec != nil {
		row.Tile.Tile.Entropy = state.dec
	}
	return nil
}

func debugConsumeBlockCDEFBeforeWithReader(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState, readSyntax debugIntraSyntaxReader) error {
	if g.Start4X == sb.Start4X && g.Start4Y == sb.Start4Y && g.End4X == sb.End4X && g.End4Y == sb.End4Y {
		row.Tile.ResetSuperBlockDeltas(hdr)
	}
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}

	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for _, child := range splitChildren(g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugConsumeBlockCDEFBeforeWithReader(seq, hdr, row, sb, child, segState, readSyntax); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildren(g, part)
			if err != nil {
				return err
			}
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugConsumeBlockCDEFBeforeWithReader(seq, hdr, row, sb, child, segState, readSyntax); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := debugConsumeLeafCDEFBeforeWithReader(seq, hdr, row, sb, child, segState, readSyntax); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugConsumeLeafCDEFBeforeWithReader(seq, hdr, row, sb, g, segState, readSyntax); err != nil {
				return err
			}
		} else {
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugConsumeLeafCDEFBeforeWithReader(seq, hdr, row, sb, child, segState, readSyntax); err != nil {
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

func debugAdvanceToTargetLeafCDEFBeforeWithReader(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState, targetX, targetY, nextX, nextY int, readSyntax debugIntraSyntaxReader) (debugTargetLeafAdvanceState, error) {
	if g.Start4X == sb.Start4X && g.Start4Y == sb.Start4Y && g.End4X == sb.End4X && g.End4Y == sb.End4Y {
		row.Tile.ResetSuperBlockDeltas(hdr)
	}
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	children, split, err := partitionChildren(g, part)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if !split {
		return debugAdvanceLeafSyntaxCDEFBeforeWithReader(seq, hdr, row, sb, g, segState, readSyntax)
	}

	targetIdx := -1
	nextIdx := -1
	for i, child := range children {
		if child.Width4() == 0 || child.Height4() == 0 {
			continue
		}
		if targetX >= child.Start4X*4 && targetX < child.End4X*4 && targetY >= child.Start4Y*4 && targetY < child.End4Y*4 {
			targetIdx = i
		}
		if nextX >= child.Start4X*4 && nextX < child.End4X*4 && nextY >= child.Start4Y*4 && nextY < child.End4Y*4 {
			nextIdx = i
		}
	}
	if targetIdx < 0 {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: no partition child covers target (%d,%d)", targetX, targetY)
	}
	if nextIdx < 0 {
		nextIdx = targetIdx
	}
	if nextIdx < targetIdx {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: next target (%d,%d) precedes target (%d,%d) in traversal", nextX, nextY, targetX, targetY)
	}
	for i := 0; i < targetIdx; i++ {
		if children[i].Width4() == 0 || children[i].Height4() == 0 {
			continue
		}
		if err := debugConsumeBlockCDEFBeforeWithReader(seq, hdr, row, sb, children[i], segState, nil); err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}

	state, err := debugAdvanceToTargetLeafCDEFBeforeWithReader(seq, hdr, row, sb, children[targetIdx], segState, targetX, targetY, nextX, nextY, readSyntax)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if targetIdx != nextIdx {
		state.nextRoot = children[nextIdx]
	}
	return state, nil
}

func debugAdvanceLeafSyntaxCDEFBefore(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState) (debugTargetLeafAdvanceState, error) {
	return debugAdvanceLeafSyntaxCDEFBeforeWithReader(seq, hdr, row, sb, g, segState, nil)
}

func debugAdvanceLeafSyntaxCDEFBeforeWithReader(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState, readSyntax debugIntraSyntaxReader) (debugTargetLeafAdvanceState, error) {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
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
	haveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	haveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	segID := uint8(0)
	segPred := false
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, haveTop, haveLeft, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, haveTop, haveLeft, true, skip, row.Tile.Tile.Entropy)
		if err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}
	if segmentDataForID(hdr, segID).Skip {
		skip = true
	}
	cdef := NewCDEFIndexMap(hdr.Width, hdr.Height)
	entryCDEF := debugCloneCDEFIndexMap(cdef)
	if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if err := readDeltaQAndLFAtSuperBlockStart(seq, hdr, row.Tile, g, skip); err != nil {
		return debugTargetLeafAdvanceState{}, err
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
	if readSyntax == nil {
		readSyntax = ReadIntraBlockSyntax
	}
	syntax, err := readSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, haveTop, haveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	syntax.Geometry = g
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	return debugTargetLeafAdvanceState{
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
		mode:                row.Tile.Tile.CDF.Clone(),
		coef:                row.Tile.Tile.Coef.Clone(),
		dq:                  row.Tile.CurrentDQ,
		dec:                 row.Tile.Tile.Entropy.Clone(),
	}, nil
}

func debugReadIntraBlockSyntaxNoCFL(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[0][syntax.YMode][:], int(numIntraPredModes-1)))
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}
		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
			palCtx := boolToInt(syntax.PalYSize > 0)
			if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
					return syntax, nil
				}
				size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVSize = size
				syntax.PalU = palU
				syntax.PalV = palV
			}
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
		}
	}

	if syntax.PalYSize != 0 {
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
		}
	}
	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
		return syntax, nil
	}

	if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}
	return syntax, nil
}

func debugReadIntraBlockSyntaxSkipCFLAlpha(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}
		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
			palCtx := boolToInt(syntax.PalYSize > 0)
			if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
					return syntax, nil
				}
				size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVSize = size
				syntax.PalU = palU
				syntax.PalV = palV
			}
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
		}
	}

	if syntax.PalYSize != 0 {
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
		}
	}
	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
		return syntax, nil
	}

	if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}
	return syntax, nil
}

func debugReadIntraBlockSyntaxOverrideCFLAlpha(uAlpha, vAlpha int8) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
		if err != nil {
			return syntax, err
		}
		if syntax.UVMode == IntraPredCFL {
			syntax.CFLAlpha[0] = uAlpha
			syntax.CFLAlpha[1] = vAlpha
		}
		return syntax, nil
	}
}

func debugReadIntraBlockSyntaxForceTX(tx TxfmSize) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
		if err != nil {
			return syntax, err
		}
		syntax.TX = tx
		return syntax, nil
	}
}

func debugReadDeferredCFLAlpha(syntax *IntraBlockSyntax, cdf *ModeCDF, dec intraEntropyDecoder) error {
	if syntax == nil || cdf == nil || dec == nil || syntax.UVMode != IntraPredCFL {
		return nil
	}
	sign := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7)) + 1
	signU := sign * 0x56 >> 8
	signV := sign - signU*3
	if signU != 0 {
		ctx := boolToInt(signU == 2)*3 + signV
		alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
		if signU == 1 {
			alpha = -alpha
		}
		syntax.CFLAlpha[0] = int8(alpha)
	}
	if signV != 0 {
		ctx := boolToInt(signV == 2)*3 + signU
		alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
		if signV == 1 {
			alpha = -alpha
		}
		syntax.CFLAlpha[1] = int8(alpha)
	}
	return nil
}

func debugReadIntraBlockSyntaxCFLAfterLumaPalette(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}
	}

	if err := debugReadDeferredCFLAlpha(&syntax, cdf, dec); err != nil {
		return syntax, err
	}

	if paletteAllowedForBlock(hdr, g) && hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
		sizeCtx := paletteSizeContext(g)
		palCtx := boolToInt(syntax.PalYSize > 0)
		if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
			chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
			if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
				return syntax, nil
			}
			size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVSize = size
			syntax.PalU = palU
			syntax.PalV = palV
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
		}
	}

	if syntax.PalYSize != 0 {
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
		}
	}
	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
		return syntax, nil
	}

	if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}
	return syntax, nil
}

func debugReadIntraBlockSyntaxCFLAfterLumaIndices(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}
		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
			palCtx := boolToInt(syntax.PalYSize > 0)
			if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
					return syntax, nil
				}
				size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVSize = size
				syntax.PalU = palU
				syntax.PalV = palV
			}
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
		}
	}

	if syntax.PalYSize != 0 {
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
	}
	if err := debugReadDeferredCFLAlpha(&syntax, cdf, dec); err != nil {
		return syntax, err
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
		}
	}
	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
		return syntax, nil
	}

	if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}
	return syntax, nil
}

func debugReadIntraBlockSyntaxNoPaletteCache(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	return debugReadIntraBlockSyntaxWithCustomYPalette(debugReadPalettePlaneSyntaxNoCache)(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
}

type debugPalettePlaneSyntaxReader func(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, error)

func debugReadIntraBlockSyntaxWithCustomYPalette(yPaletteReader debugPalettePlaneSyntaxReader) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if hdr == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
		}
		if cdf == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
		}
		if above == nil || left == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
		}
		if dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
		}

		syntax := IntraBlockSyntax{
			Geometry: g,
			Size:     g.Size,
			Segment:  segID,
			YMode:    IntraPredDC,
			UVMode:   IntraPredDC,
			TX:       MaxLumaTXSizeForBlock(g.Size),
			UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
		}

		bx4 := g.Start4X & 31
		by4 := g.Start4Y & 31
		syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
		if needsIntraAngleDelta(syntax.YMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
			if err != nil {
				return syntax, err
			}
			syntax.YAngle = angle
		}
		if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
			syntax.YMode = IntraPredDC
			syntax.YAngle = 0
		}

		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
			cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
			lastSymbol := int(numIntraPredModes - 1)
			if cflAllowed {
				lastSymbol++
			}
			syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
			if syntax.UVMode == IntraPredCFL {
				sign := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7)) + 1
				signU := sign * 0x56 >> 8
				signV := sign - signU*3
				if signU != 0 {
					ctx := boolToInt(signU == 2)*3 + signV
					alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
					if signU == 1 {
						alpha = -alpha
					}
					syntax.CFLAlpha[0] = int8(alpha)
				}
				if signV != 0 {
					ctx := boolToInt(signV == 2)*3 + signU
					alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
					if signV == 1 {
						alpha = -alpha
					}
					syntax.CFLAlpha[1] = int8(alpha)
				}
			}
			if needsIntraAngleDelta(syntax.UVMode, g.Size) {
				angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
				if err != nil {
					return syntax, err
				}
				syntax.UVAngle = angle
			}
		}

		if paletteAllowedForBlock(hdr, g) {
			sizeCtx := paletteSizeContext(g)
			if syntax.YMode == IntraPredDC {
				palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
				if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
					reader := yPaletteReader
					if reader == nil {
						reader = readPalettePlaneSyntax
					}
					size, values, err := reader(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
					if err != nil {
						return syntax, err
					}
					syntax.PalYSize = size
					syntax.PalY = values
				}
			}
			if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
				palCtx := boolToInt(syntax.PalYSize > 0)
				if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
					chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
					if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
						return syntax, nil
					}
					size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
					if err != nil {
						return syntax, err
					}
					syntax.PalUVSize = size
					syntax.PalU = palU
					syntax.PalV = palV
				}
			}
		}

		if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
			if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
				syntax.YMode = IntraPredFilter
				syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
			}
		}

		if syntax.PalYSize != 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalYIdx = indices
		}
		if syntax.PalUVSize != 0 {
			chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
			if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
				indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVIdx = indices
			}
		}
		if hdr.Segmentation.Lossless[segID] {
			syntax.TX = TX4X4
			syntax.UVTX = TX4X4
			return syntax, nil
		}

		if hdr.TxfmMode == txModeSwitchable {
			info := TxfmInfoFor(syntax.TX)
			if info.Max > 0 {
				txCtx := GetTxCtx(above, left, info, by4, bx4)
				txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
				for txDepth > 0 {
					syntax.TX = TxfmInfoFor(syntax.TX).Sub
					txDepth--
				}
			}
		}
		return syntax, nil
	}
}

func debugReadPalettePlaneSyntaxNoCache(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, error) {
	noCacheAbove := *above
	noCacheLeft := *left
	noCacheAbove.PalSz[bx4] = 0
	noCacheLeft.PalSz[by4] = 0
	noCacheAbove.Pal[plane][bx4] = [8]uint16{}
	noCacheLeft.Pal[plane][by4] = [8]uint16{}
	return readPalettePlaneSyntax(bitDepth, cdf, &noCacheAbove, &noCacheLeft, plane, sizeCtx, bx4, by4, dec)
}

func debugPaletteDeltaBitsCap(bits, maxValue, prev, minStep, remainingAfter int) int {
	if bits <= 0 {
		return 0
	}
	if remainingAfter < 0 {
		remainingAfter = 0
	}
	room := maxValue - prev - minStep*remainingAfter
	if room <= 0 {
		return 0
	}
	return minInt(bits, ulog2(room+1))
}

func debugReadPalettePlaneSyntaxBoundedYDeltaBits(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return 0, [8]uint16{}, fmt.Errorf("decoder: palette plane syntax requires decoder state")
	}
	if bitDepth == 0 {
		bitDepth = 8
	}

	palSize := int(dec.DecodeSymbolAdapt(cdf.PalSize[plane][sizeCtx][:], 6)) + 2
	cache, nCache := paletteCache(above, left, plane, bx4, by4)
	usedCache := [8]uint16{}
	nUsed := 0
	for i := 0; i < nCache && nUsed < palSize; i++ {
		if dec.DecodeBoolEqui() != 0 {
			usedCache[nUsed] = cache[i]
			nUsed++
		}
	}

	var pal [8]uint16
	if nUsed < palSize {
		bpc := int(bitDepth)
		next := nUsed
		prev := int(dec.DecodeBools(uint(bpc)))
		pal[next] = uint16(prev)
		next++

		if next < palSize {
			bits := bpc - 3 + int(dec.DecodeBools(2))
			maxValue := (1 << bpc) - 1
			minStep := 1
			if plane != 0 {
				minStep = 0
			}
			if plane == 0 {
				bits = debugPaletteDeltaBitsCap(bits, maxValue, prev, minStep, palSize-next)
			}
			for next < palSize {
				delta := int(dec.DecodeBools(uint(bits)))
				prev = minInt(prev+delta+minStep, maxValue)
				pal[next] = uint16(prev)
				next++
				if prev+minStep >= maxValue {
					for next < palSize {
						pal[next] = uint16(maxValue)
						next++
					}
					break
				}
				if plane == 0 {
					bits = debugPaletteDeltaBitsCap(bits, maxValue, prev, minStep, palSize-next)
				} else {
					bits = minInt(bits, 1+ulog2(maxValue-prev-minStep))
				}
			}
		}

		merged := [8]uint16{}
		a := 0
		b := nUsed
		for i := 0; i < palSize; i++ {
			if a < nUsed && (b >= palSize || usedCache[a] <= pal[b]) {
				merged[i] = usedCache[a]
				a++
			} else {
				merged[i] = pal[b]
				b++
			}
		}
		pal = merged
	} else {
		copy(pal[:], usedCache[:nUsed])
	}

	return uint8(palSize), pal, nil
}

func debugReadPalettePlaneSyntaxWithForcedCacheMask(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder, mask uint8) (uint8, [8]uint16, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return 0, [8]uint16{}, fmt.Errorf("decoder: palette plane syntax requires decoder state")
	}
	if bitDepth == 0 {
		bitDepth = 8
	}

	palSize := int(dec.DecodeSymbolAdapt(cdf.PalSize[plane][sizeCtx][:], 6)) + 2
	cache, nCache := paletteCache(above, left, plane, bx4, by4)
	usedCache := [8]uint16{}
	nUsed := 0
	for i := 0; i < nCache && nUsed < palSize; i++ {
		_ = dec.DecodeBoolEqui()
		if (mask>>i)&1 == 0 {
			continue
		}
		usedCache[nUsed] = cache[i]
		nUsed++
	}

	var pal [8]uint16
	if nUsed < palSize {
		bpc := int(bitDepth)
		next := nUsed
		prev := int(dec.DecodeBools(uint(bpc)))
		pal[next] = uint16(prev)
		next++

		if next < palSize {
			bits := bpc - 3 + int(dec.DecodeBools(2))
			maxValue := (1 << bpc) - 1
			minStep := 1
			if plane != 0 {
				minStep = 0
			}
			for next < palSize {
				delta := int(dec.DecodeBools(uint(bits)))
				prev = minInt(prev+delta+minStep, maxValue)
				pal[next] = uint16(prev)
				next++
				if prev+minStep >= maxValue {
					for next < palSize {
						pal[next] = uint16(maxValue)
						next++
					}
					break
				}
				bits = minInt(bits, 1+ulog2(maxValue-prev-minStep))
			}
		}

		merged := [8]uint16{}
		a := 0
		b := nUsed
		for i := 0; i < palSize; i++ {
			if a < nUsed && (b >= palSize || usedCache[a] <= pal[b]) {
				merged[i] = usedCache[a]
				a++
			} else {
				merged[i] = pal[b]
				b++
			}
		}
		pal = merged
	} else {
		copy(pal[:], usedCache[:nUsed])
	}

	return uint8(palSize), pal, nil
}

func debugReadIntraBlockSyntaxForcePaletteCacheMask(mask uint8) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if hdr == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
		}
		if cdf == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
		}
		if above == nil || left == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
		}
		if dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
		}

		syntax := IntraBlockSyntax{
			Geometry: g,
			Size:     g.Size,
			Segment:  segID,
			YMode:    IntraPredDC,
			UVMode:   IntraPredDC,
			TX:       MaxLumaTXSizeForBlock(g.Size),
			UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
		}

		bx4 := g.Start4X & 31
		by4 := g.Start4Y & 31
		syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
		if needsIntraAngleDelta(syntax.YMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
			if err != nil {
				return syntax, err
			}
			syntax.YAngle = angle
		}
		if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
			syntax.YMode = IntraPredDC
			syntax.YAngle = 0
		}

		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
			cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
			lastSymbol := int(numIntraPredModes - 1)
			if cflAllowed {
				lastSymbol++
			}
			syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
			if syntax.UVMode == IntraPredCFL {
				sign := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7)) + 1
				signU := sign * 0x56 >> 8
				signV := sign - signU*3
				if signU != 0 {
					ctx := boolToInt(signU == 2)*3 + signV
					alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
					if signU == 1 {
						alpha = -alpha
					}
					syntax.CFLAlpha[0] = int8(alpha)
				}
				if signV != 0 {
					ctx := boolToInt(signV == 2)*3 + signU
					alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
					if signV == 1 {
						alpha = -alpha
					}
					syntax.CFLAlpha[1] = int8(alpha)
				}
			}
			if needsIntraAngleDelta(syntax.UVMode, g.Size) {
				angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
				if err != nil {
					return syntax, err
				}
				syntax.UVAngle = angle
			}
		}

		if paletteAllowedForBlock(hdr, g) {
			sizeCtx := paletteSizeContext(g)
			if syntax.YMode == IntraPredDC {
				palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
				if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
					size, values, err := debugReadPalettePlaneSyntaxWithForcedCacheMask(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec, mask)
					if err != nil {
						return syntax, err
					}
					syntax.PalYSize = size
					syntax.PalY = values
				}
			}
			if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
				palCtx := boolToInt(syntax.PalYSize > 0)
				if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
					chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
					if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
						return syntax, nil
					}
					size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
					if err != nil {
						return syntax, err
					}
					syntax.PalUVSize = size
					syntax.PalU = palU
					syntax.PalV = palV
				}
			}
		}

		if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
			if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
				syntax.YMode = IntraPredFilter
				syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
			}
		}

		if syntax.PalYSize != 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalYIdx = indices
		}
		if syntax.PalUVSize != 0 {
			chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
			if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
				indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVIdx = indices
			}
		}
		if hdr.Segmentation.Lossless[segID] {
			syntax.TX = TX4X4
			syntax.UVTX = TX4X4
			return syntax, nil
		}

		if hdr.TxfmMode == txModeSwitchable {
			info := TxfmInfoFor(syntax.TX)
			if info.Max > 0 {
				txCtx := GetTxCtx(above, left, info, by4, bx4)
				txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
				for txDepth > 0 {
					syntax.TX = TxfmInfoFor(syntax.TX).Sub
					txDepth--
				}
			}
		}
		return syntax, nil
	}
}

func debugReadIntraBlockSyntaxPaletteBeforeUVMode(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
		}
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		if syntax.UVMode == IntraPredCFL {
			sign := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7)) + 1
			signU := sign * 0x56 >> 8
			signV := sign - signU*3
			if signU != 0 {
				ctx := boolToInt(signU == 2)*3 + signV
				alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
				if signU == 1 {
					alpha = -alpha
				}
				syntax.CFLAlpha[0] = int8(alpha)
			}
			if signV != 0 {
				ctx := boolToInt(signV == 2)*3 + signU
				alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
				if signV == 1 {
					alpha = -alpha
				}
				syntax.CFLAlpha[1] = int8(alpha)
			}
		}
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
		if paletteAllowedForBlock(hdr, g) && syntax.UVMode == IntraPredDC {
			sizeCtx := paletteSizeContext(g)
			palCtx := boolToInt(syntax.PalYSize > 0)
			if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
					return syntax, nil
				}
				size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVSize = size
				syntax.PalU = palU
				syntax.PalV = palV
			}
		}
	}

	if syntax.PalYSize != 0 {
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
		}
	}
	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
		return syntax, nil
	}

	if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}
	return syntax, nil
}

func debugReadIntraBlockSyntaxTxBeforePaletteIndices(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		if syntax.UVMode == IntraPredCFL {
			sign := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7)) + 1
			signU := sign * 0x56 >> 8
			signV := sign - signU*3
			if signU != 0 {
				ctx := boolToInt(signU == 2)*3 + signV
				alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
				if signU == 1 {
					alpha = -alpha
				}
				syntax.CFLAlpha[0] = int8(alpha)
			}
			if signV != 0 {
				ctx := boolToInt(signV == 2)*3 + signU
				alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
				if signV == 1 {
					alpha = -alpha
				}
				syntax.CFLAlpha[1] = int8(alpha)
			}
		}
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}
		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
			palCtx := boolToInt(syntax.PalYSize > 0)
			if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
					return syntax, nil
				}
				size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVSize = size
				syntax.PalU = palU
				syntax.PalV = palV
			}
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
		}
	}

	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
	} else if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}

	if syntax.PalYSize != 0 {
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
		}
	}
	return syntax, nil
}

func debugDescendToPointLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, dec *msac.Context, targetX, targetY int) (BlockGeometry, error) {
	for {
		part, err := DecodePartitionDecision(seq, hdr, cdf, above, left, g, msacPartitionDecoder{ctx: dec})
		if err != nil {
			return BlockGeometry{}, err
		}
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return BlockGeometry{}, err
		}
		if !split {
			return g, nil
		}
		found := false
		for _, child := range children {
			if targetX >= child.Start4X*4 && targetX < child.End4X*4 && targetY >= child.Start4Y*4 && targetY < child.End4Y*4 {
				g = child
				found = true
				break
			}
		}
		if !found {
			return BlockGeometry{}, fmt.Errorf("decoder: no partition child covers target (%d,%d)", targetX, targetY)
		}
	}
}

func debugDescendFirstLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, dec *msac.Context) (BlockGeometry, error) {
	for {
		local := debugLocalGeometryForLeafResume(seq, g)
		part, err := DecodePartitionDecision(seq, hdr, cdf, above, left, local, msacPartitionDecoder{ctx: dec})
		if err != nil {
			return BlockGeometry{}, err
		}
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return BlockGeometry{}, err
		}
		if !split {
			return g, nil
		}
		var child BlockGeometry
		found := false
		for _, candidate := range children {
			if candidate.Width4() == 0 || candidate.Height4() == 0 {
				continue
			}
			child = candidate
			found = true
			break
		}
		if !found {
			return BlockGeometry{}, fmt.Errorf("decoder: no partition child available while descending from (%d,%d)-(%d,%d)", g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4)
		}
		if part != PartitionSplit || g.Level == BlockLevel8x8 {
			return child, nil
		}
		g = child
	}
}

func debugLocalGeometryForLeafResume(seq av1.SequenceHeader, g BlockGeometry) BlockGeometry {
	local := g
	groupBase4X := (g.Start4X >> 5) << 5
	rowBase4Y := (g.Start4Y >> 5) << 5
	local.Start4X -= groupBase4X
	local.End4X -= groupBase4X
	local.Start4Y -= rowBase4Y
	local.End4Y -= rowBase4Y
	return local
}

func debugResumeTraversalToNextRoot(above, left *BlockContext, stack []debugTraversalFrame) (BlockGeometry, error) {
	for i := len(stack) - 1; i >= 0; i-- {
		frame := stack[i]
		if frame.ChildIndex+1 < frame.ChildCount {
			return frame.Children[frame.ChildIndex+1], nil
		}
		if frame.Part != PartitionSplit || frame.Geometry.Level == BlockLevel8x8 {
			if err := ApplyPartitionContext(above, left, frame.Local, frame.Part); err != nil {
				return BlockGeometry{}, err
			}
		}
	}
	return BlockGeometry{}, fmt.Errorf("decoder: no next root after traversal unwind")
}

func debugResumeNextLeafEntry(seq av1.SequenceHeader, hdr *obu.FrameHeader, base debugTargetLeafAdvanceState, modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) (debugTargetLeafAdvanceState, error) {
	if modeCDF == nil || coefCDF == nil || above == nil || left == nil || dec == nil {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: resume entry requires non-nil mode/coef/context state")
	}
	if hdr == nil {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: frame header required")
	}
	if base.returnApply {
		if err := ApplyPartitionContext(above, left, base.returnLocal, base.returnPart); err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}
	nextRoot, err := debugResumeTraversalToNextRoot(above, left, base.traversal)
	if err != nil {
		if base.nextRoot.Width4() == 0 || base.nextRoot.Height4() == 0 {
			return debugTargetLeafAdvanceState{}, err
		}
		nextRoot = base.nextRoot
	}
	leaf, err := debugDescendFirstLeaf(seq, hdr, modeCDF, above, left, nextRoot, dec)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	local := debugLocalGeometryForLeafResume(seq, leaf)
	return debugTargetLeafAdvanceState{
		syntax:     IntraBlockSyntax{Geometry: leaf},
		entryLocal: local,
		entryAbove: *above,
		entryLeft:  *left,
		entryMode:  modeCDF.Clone(),
		entryCoef:  coefCDF.Clone(),
		entryDec:   dec.Clone(),
		cdef:       debugCloneCDEFIndexMap(base.postCDEF),
		nextRoot:   nextRoot,
	}, nil
}

func debugResumeNextLeafCDEFBefore(seq av1.SequenceHeader, hdr *obu.FrameHeader, segState *SegmentationState, base debugTargetLeafAdvanceState, modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) (debugTargetLeafAdvanceState, error) {
	if hdr == nil {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: frame header required")
	}
	if modeCDF == nil || coefCDF == nil || above == nil || left == nil || dec == nil {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: resume requires non-nil mode/coef/context state")
	}
	entry, err := debugResumeNextLeafEntry(seq, hdr, base, modeCDF, coefCDF, above, left, dec)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	leaf := entry.syntax.Geometry
	local := entry.entryLocal
	segID := uint8(0)
	segPred := false
	haveTop := leaf.Start4Y > 0
	haveLeft := leaf.Start4X > 0
	msacDec := msacEntropyDecoder{ctx: dec}
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, modeCDF, above, left, segState, leaf, haveTop, haveLeft, false, false, msacDec)
		if err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, modeCDF, above, left, local, msacDec)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, modeCDF, above, left, segState, leaf, haveTop, haveLeft, true, skip, msacDec)
		if err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}
	if segmentDataForID(hdr, segID).Skip {
		skip = true
	}
	cdef := base.postCDEF
	if cdef == nil {
		cdef = base.cdef
	}
	cdef = debugCloneCDEFIndexMap(cdef)
	if cdef == nil {
		cdef = NewCDEFIndexMap(hdr.Width, hdr.Height)
	}
	if err := cdef.ensureBlockIndex(leaf, hdr.CDEF.NBits, skip, msacDec); err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	tileSyntax := &TileSyntaxState{
		Tile: TileState{
			Entropy: dec,
			CDF:     modeCDF,
			Coef:    coefCDF,
		},
		LastQIdx:   base.qidx,
		LastLF:     base.lastLF,
		ReadDeltas: base.readDeltas,
		CurrentDQ:  base.dq,
	}
	tileSyntax.Tile.CoefByQ[DefaultCoefQCat(uint8(clipQIndexRuntime(base.qidx)))] = coefCDF
	if err := readDeltaQAndLF(seq, hdr, tileSyntax, leaf, skip); err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, tileSyntax.Tile.CDF, above, left, local, haveTop, haveLeft, segID, msacDec)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	syntax.Geometry = leaf
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	return debugTargetLeafAdvanceState{
		syntax:     syntax,
		above:      *above,
		left:       *left,
		qidx:       tileSyntax.LastQIdx,
		lastLF:     tileSyntax.LastLF,
		readDeltas: tileSyntax.ReadDeltas,
		mode:       tileSyntax.Tile.CDF.Clone(),
		coef:       tileSyntax.Tile.Coef.Clone(),
		dq:         tileSyntax.CurrentDQ,
		dec:        tileSyntax.Tile.Entropy.Clone(),
		cdef:       cdef,
		nextRoot:   entry.nextRoot,
	}, nil
}

type firstKeyStats struct {
	blocks      int
	skips       int
	filterIntra int
	paletteY    int
	paletteUV   int
	cfl         int
	yModes      map[IntraPredMode]int
	uvModes     map[IntraPredMode]int
	tx          map[TxfmSize]int
	uvTx        map[TxfmSize]int
}

type debugLumaTURecord struct {
	offX4  int
	offY4  int
	txType TxType
	coeffs []int32
	tuSkip bool
}

type debugTraversalFrame struct {
	Geometry   BlockGeometry
	Local      BlockGeometry
	Part       BlockPartition
	ChildIndex int
	ChildCount int
	Children   [4]BlockGeometry
}

type debugTargetLeafAdvanceState struct {
	syntax              IntraBlockSyntax
	entryLocal          BlockGeometry
	entryAbove          BlockContext
	entryLeft           BlockContext
	entryMode           *ModeCDF
	entryCoef           *CoefCDF
	entryDec            *msac.Context
	entryQIdx           int
	entryLastLF         [4]int8
	entryReadDeltas     bool
	entryDQ             DequantTables
	entryCDEF           *CDEFIndexMap
	preSyntaxAbove      BlockContext
	preSyntaxLeft       BlockContext
	preSyntaxMode       *ModeCDF
	preSyntaxCoef       *CoefCDF
	preSyntaxDec        *msac.Context
	preSyntaxQIdx       int
	preSyntaxLastLF     [4]int8
	preSyntaxReadDeltas bool
	preSyntaxDQ         DequantTables
	preSyntaxCDEF       *CDEFIndexMap
	above               BlockContext
	left                BlockContext
	qidx                int
	lastLF              [4]int8
	readDeltas          bool
	mode                *ModeCDF
	coef                *CoefCDF
	dq                  DequantTables
	dec                 *msac.Context
	cdef                *CDEFIndexMap
	postAbove           BlockContext
	postLeft            BlockContext
	postMode            *ModeCDF
	postCoef            *CoefCDF
	postDec             *msac.Context
	postCDEF            *CDEFIndexMap
	returnApply         bool
	returnLocal         BlockGeometry
	returnPart          BlockPartition
	traversal           []debugTraversalFrame
	nextRoot            BlockGeometry
	haveTop             bool
	haveLeft            bool
	edge                intraEdgeAvailability
}

func decodeKeyframeMain8Stats(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, stats *firstKeyStats) error {
	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		return err
	}
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, hdr, tile, 0)
		if err != nil {
			return err
		}
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			row, err := syntax.NewRowState(sbRow)
			if err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if err := decodeKeyframeMain8StatsBlock(seq, hdr, row, sb, BlockGeometry{
					Level:   RootBlockLevel(seq),
					Size:    RootBlockSize(seq),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}, dq, img, cdef, segState, stats, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, BlockGeometry{
					Level:   RootBlockLevel(seq),
					Size:    RootBlockSize(seq),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				})); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func debugKeyframeProblemBlocks(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, segState *SegmentationState, targets [][2]int, t *testing.T) error {
	return debugKeyframeProblemBlocksWithRootPart(seq, hdr, group, dq, img, segState, targets, nil, t)
}

func debugKeyframeProblemBlocksWithRootPart(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, img *image.YCbCr, segState *SegmentationState, targets [][2]int, forcedRootPart *BlockPartition, t *testing.T) error {
	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		return err
	}
	cdef := NewCDEFIndexMap(hdr.Width, hdr.Height)
	restoration := NewRestorationState(seq, hdr)
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, hdr, tile, 0)
		if err != nil {
			return err
		}
		restRefs := newTileRestorationRefs()
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			row, err := syntax.NewRowState(sbRow)
			if err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if err := parseRestorationUnitsForSuperBlock(hdr, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
					return err
				}
				if err := debugKeyframeProblemBlock(seq, hdr, row, sb, BlockGeometry{
					Level:   RootBlockLevel(seq),
					Size:    RootBlockSize(seq),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}, dq, img, cdef, segState, targets, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, BlockGeometry{
					Level:   RootBlockLevel(seq),
					Size:    RootBlockSize(seq),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}), forcedRootPart, t); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func debugKeyframeProblemBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, targets [][2]int, edge intraEdgeAvailability, forcedRootPart *BlockPartition, t *testing.T) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	var part BlockPartition
	if forcedRootPart != nil && g.Start4X == 0 && g.Start4Y == 0 && g.Level == RootBlockLevel(seq) {
		part = *forcedRootPart
	} else {
		part, err = DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	if g.Start4X == 0 && g.Start4Y == 0 && g.Level == RootBlockLevel(seq) {
		for altCtx := 0; altCtx < 4; altCtx++ {
			altDec := row.Tile.Tile.Entropy.Clone()
			altCDF := append([]uint16(nil), row.Tile.Tile.CDF.PartitionCDF(g.Level, altCtx)...)
			altPart := BlockPartition(altDec.DecodeSymbolAdapt(altCDF, int(PartitionTypeCount(g.Level))))
			t.Logf("root alt-partctx=%d part=%d", altCtx, altPart)
		}
	}
	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			children := splitChildren(g)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugKeyframeProblemBlock(seq, hdr, row, sb, child, dq, img, cdef, segState, targets, childEdges[i], forcedRootPart, t); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildren(g, part)
			if err != nil {
				return err
			}
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugKeyframeProblemBlock(seq, hdr, row, sb, child, dq, img, cdef, segState, targets, childEdges[i], forcedRootPart, t); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		for i, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := debugKeyframeProblemLeaf(seq, hdr, row, sb, child, dq, img, cdef, segState, targets, childEdges[i], t); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugKeyframeProblemLeaf(seq, hdr, row, sb, g, dq, img, cdef, segState, targets, edge, t); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugKeyframeProblemLeaf(seq, hdr, row, sb, child, dq, img, cdef, segState, targets, childEdges[i], t); err != nil {
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

func debugKeyframeProblemLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, targets [][2]int, edge intraEdgeAvailability, t *testing.T) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	traceTargetWrites := os.Getenv("DEBUG_SPBTV_TRACE_TARGET_WRITES") != ""
	var targetBefore []uint8
	if traceTargetWrites {
		targetBefore = make([]uint8, len(targets))
		for i, pt := range targets {
			if pt[0] >= 0 && pt[0] < parsedPlaneWidth(img) && pt[1] >= 0 && pt[1] < parsedPlaneHeight(img) {
				targetBefore[i] = img.Y[pt[1]*img.YStride+pt[0]]
			}
		}
	}
	haveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	haveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	segID := uint8(0)
	segPred := false
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, haveTop, haveLeft, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	_ = skipMode
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, haveTop, haveLeft, true, skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	if segmentDataForID(hdr, segID).Skip {
		skip = true
	}
	if g.Start4X == 0 && g.Start4Y == 0 && row.Tile.Tile.Entropy != nil {
		debugFirstLeafSyntaxTrace(t, seq, hdr, row.Tile.Tile.CDF, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, local, haveTop, haveLeft, segID, skip, row.Tile.Tile.Entropy)
	}
	if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
		return err
	}
	cdef.markNoSkip(g, skip)
	if err := readDeltaQAndLFAtSuperBlockStart(seq, hdr, row.Tile, g, skip); err != nil {
		return err
	}
	if g.Start4X == 0 && g.Start4Y == 0 && row.Tile.Tile.Entropy != nil {
		for topCtx := 0; topCtx < 5; topCtx++ {
			for leftCtx := 0; leftCtx < 5; leftCtx++ {
				altDec := row.Tile.Tile.Entropy.Clone()
				altCDF := append([]uint16(nil), row.Tile.Tile.CDF.KFYMode[topCtx][leftCtx][:]...)
				altMode := IntraPredMode(altDec.DecodeSymbolAdapt(altCDF, int(numIntraPredModes-1)))
				t.Logf("root alt-ymode topctx=%d leftctx=%d mode=%s", topCtx, leftCtx, intraModeName(altMode))
			}
		}
	}
	logTargetYModeTrace := false
	var targetPreSyntaxClone *msac.Context
	var targetPreSyntaxCDF *ModeCDF
	for _, pt := range targets {
		if pointInBlock(pt[0], pt[1], g) {
			logTargetYModeTrace = true
			break
		}
	}
	if logTargetYModeTrace && row.Tile.Tile.Entropy != nil {
		targetPreSyntaxClone = row.Tile.Tile.Entropy.Clone()
		targetPreSyntaxCDF = row.Tile.Tile.CDF.Clone()
		bx4 := local.Start4X & 31
		by4 := local.Start4Y & 31
		cur, rng, cnt, pos := row.Tile.Tile.Entropy.DebugState()
		t.Logf("target pre-ymode block=(%d,%d)-(%d,%d) state current=%d rng=%d cnt=%d pos=%d topCtx=%d leftCtx=%d",
			g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
			cur, rng, cnt, pos,
			generatedIntraModeContext[above.Mode[bx4]],
			generatedIntraModeContext[row.Left.Mode[by4]],
		)
		for topCtx := 0; topCtx < 5; topCtx++ {
			for leftCtx := 0; leftCtx < 5; leftCtx++ {
				altDec := row.Tile.Tile.Entropy.Clone()
				altCDF := append([]uint16(nil), row.Tile.Tile.CDF.KFYMode[topCtx][leftCtx][:]...)
				altMode := IntraPredMode(altDec.DecodeSymbolAdapt(altCDF, int(numIntraPredModes-1)))
				t.Logf("target alt-ymode block=(%d,%d)-(%d,%d) topctx=%d leftctx=%d mode=%s",
					g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, topCtx, leftCtx, intraModeName(altMode))
			}
		}
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, haveTop, haveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	syntax.Geometry = g
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	if err := debugReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq, hdr, row.Tile.Tile.CDF, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X, g.Start4Y, skip, row.Tile.Tile.Entropy, edge.topHasRight, edge.leftHasBottom, targets, t); err != nil {
		return err
	}
	chromaEdge := edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling())
	if err := reconstructParsedIntraChromaBlockAt8WithSkipEdges(seq, hdr, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), row.Tile.Tile.Entropy, skip, chromaEdge.topHasRight, chromaEdge.leftHasBottom); err != nil {
		return err
	}
	if traceTargetWrites {
		for i, pt := range targets {
			if pt[0] < 0 || pt[0] >= parsedPlaneWidth(img) || pt[1] < 0 || pt[1] >= parsedPlaneHeight(img) {
				continue
			}
			after := img.Y[pt[1]*img.YStride+pt[0]]
			if after == targetBefore[i] {
				continue
			}
			scope := "outside"
			if pointInBlock(pt[0], pt[1], g) {
				scope = "inside"
			}
			t.Logf("target-write target=(%d,%d) %s block=(%d,%d)-(%d,%d) size=%s before=%d after=%d mode=%s tx=%s skip=%v palY=%d",
				pt[0], pt[1], scope,
				g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, blockName(g.Size),
				targetBefore[i], after, intraModeName(syntax.YMode), txName(syntax.TX), skip, syntax.PalYSize)
		}
	}
	for _, pt := range targets {
		if pointInBlock(pt[0], pt[1], g) {
			topMode := IntraPredMode(above.Mode[g.Start4X&31])
			leftMode := IntraPredMode(row.Left.Mode[g.Start4Y&31])
			t.Logf("target=(%d,%d) block=(%d,%d)-(%d,%d) size=%s mode=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d seg=%d meanY=%.2f",
				pt[0], pt[1], g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, blockName(g.Size), intraModeName(syntax.YMode), intraModeName(syntax.UVMode), txName(syntax.TX), txName(syntax.UVTX), skip, syntax.PalYSize, syntax.PalUVSize, syntax.Segment, blockMeanY(img, g))
			t.Logf("syntax geometry=(%d,%d)-(%d,%d) size=%s", syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4, syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4, blockName(syntax.Geometry.Size))
			t.Logf("target=(%d,%d) ctx topMode=%s leftMode=%s topCtx=%d leftCtx=%d haveTop=%v haveLeft=%v",
				pt[0], pt[1],
				intraModeName(topMode),
				intraModeName(leftMode),
				generatedIntraModeContext[above.Mode[g.Start4X&31]],
				generatedIntraModeContext[row.Left.Mode[g.Start4Y&31]],
				haveTop,
				haveLeft,
			)
			if syntax.PalYSize != 0 {
				if targetPreSyntaxClone != nil && targetPreSyntaxCDF != nil {
					debugTraceTargetClonedIntraSyntax(t, seq, hdr, targetPreSyntaxCDF, above, &row.Left, local, g, haveTop, haveLeft, segID, targetPreSyntaxClone.Clone())
					debugTraceTargetPaletteDecision(t, seq, hdr, targetPreSyntaxCDF, above, &row.Left, local, g, haveTop, haveLeft, segID, targetPreSyntaxClone.Clone())
				}
				head := minInt(16, len(syntax.PalYIdx))
				t.Logf("target=(%d,%d) palY values=%v idx0=%v abovePalSz=%d leftPalSz=%d", pt[0], pt[1], syntax.PalY[:syntax.PalYSize], syntax.PalYIdx[:head], above.PalSz[g.Start4X&31], row.Left.PalSz[g.Start4Y&31])
				cache, nCache := paletteCache(above, &row.Left, 0, g.Start4X&31, g.Start4Y&31)
				t.Logf("target=(%d,%d) palY above=%v left=%v cache=%v",
					pt[0], pt[1],
					above.Pal[0][g.Start4X&31][:above.PalSz[g.Start4X&31]],
					row.Left.Pal[0][g.Start4Y&31][:row.Left.PalSz[g.Start4Y&31]],
					cache[:nCache],
				)
			}
			if syntax.PalUVSize != 0 {
				head := minInt(16, len(syntax.PalUVIdx))
				t.Logf("target=(%d,%d) palUV U=%v V=%v idx0=%v aboveUVPalSz=%d leftUVPalSz=%d", pt[0], pt[1], syntax.PalU[:syntax.PalUVSize], syntax.PalV[:syntax.PalUVSize], syntax.PalUVIdx[:head], above.UVPalSz[g.Start4X&31], row.Left.UVPalSz[g.Start4Y&31])
			}
		}
	}
	return ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax)
}

func debugFirstLeafSyntaxTrace(t *testing.T, seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, skip bool, dec *msac.Context) {
	if t == nil || hdr == nil || cdf == nil || coefCDF == nil || above == nil || left == nil || dec == nil {
		return
	}
	cur, rng, cnt, pos := dec.DebugState()
	t.Logf("first-leaf after-skip current=%d rng=%d cnt=%d pos=%d skip=%v", cur, rng, cnt, pos, skip)
	debugTraceFirstLeafPrefix(t, "actual-order", seq, hdr, cdf, coefCDF, dq, above, left, g, haveTop, haveLeft, segID, dec.Clone(), false)
	debugTraceFirstLeafPrefix(t, "cdef-first", seq, hdr, cdf, coefCDF, dq, above, left, g, haveTop, haveLeft, segID, dec.Clone(), true)
}

func debugTraceFirstLeafPrefix(t *testing.T, label string, seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec *msac.Context, consumeCDEF bool) {
	if t == nil || hdr == nil || cdf == nil || coefCDF == nil || above == nil || left == nil || dec == nil {
		return
	}
	cdfClone := cdf.Clone()
	aboveClone := *above
	leftClone := *left

	if consumeCDEF && hdr.CDEF.NBits > 0 {
		idx := dec.DecodeBools(uint(hdr.CDEF.NBits))
		cur, rng, cnt, pos := dec.DebugState()
		t.Logf("first-leaf %s cdef_idx=%d current=%d rng=%d cnt=%d pos=%d", label, idx, cur, rng, cnt, pos)
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	yMode := IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdfClone, &aboveClone, &leftClone, g, by4, bx4), int(numIntraPredModes-1)))
	yAngle := int8(0)
	if needsIntraAngleDelta(yMode, g.Size) {
		angle, err := readIntraAngleDelta(cdfClone, msacEntropyDecoder{ctx: dec}, yMode)
		if err != nil {
			t.Logf("first-leaf %s yangle err=%v", label, err)
			return
		}
		yAngle = angle
	}
	if !supportsLumaIntraPredictor(yMode, yAngle) {
		yMode = IntraPredDC
		yAngle = 0
	}
	cur, rng, cnt, pos := dec.DebugState()
	t.Logf("first-leaf %s ymode=%s angle=%d current=%d rng=%d cnt=%d pos=%d", label, intraModeName(yMode), yAngle, cur, rng, cnt, pos)

	uvMode := IntraPredDC
	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		uvMode = IntraPredMode(dec.DecodeSymbolAdapt(cdfClone.UVMode[boolToInt(cflAllowed)][yMode][:], lastSymbol))
		if uvMode == IntraPredCFL {
			sign := int(dec.DecodeSymbolAdapt(cdfClone.CFLSign[:], 7)) + 1
			signU := sign * 0x56 >> 8
			signV := sign - signU*3
			if signU != 0 {
				ctx := boolToInt(signU == 2)*3 + signV
				_ = dec.DecodeSymbolAdapt(cdfClone.CFLAlpha[ctx][:], 15)
			}
			if signV != 0 {
				ctx := boolToInt(signV == 2)*3 + signU
				_ = dec.DecodeSymbolAdapt(cdfClone.CFLAlpha[ctx][:], 15)
			}
		}
		if needsIntraAngleDelta(uvMode, g.Size) {
			if _, err := readIntraAngleDelta(cdfClone, msacEntropyDecoder{ctx: dec}, uvMode); err != nil {
				t.Logf("first-leaf %s uvangle err=%v", label, err)
				return
			}
		}
		cur, rng, cnt, pos = dec.DebugState()
		t.Logf("first-leaf %s uvmode=%s current=%d rng=%d cnt=%d pos=%d", label, intraModeName(uvMode), cur, rng, cnt, pos)
	}

	if paletteAllowedForBlock(hdr, g) && yMode == IntraPredDC {
		sizeCtx := paletteSizeContext(g)
		palCtx := boolToInt(aboveClone.PalSz[bx4] > 0) + boolToInt(leftClone.PalSz[by4] > 0)
		palY := dec.DecodeBoolAdapt(cdfClone.PalY[sizeCtx][palCtx][:]) != 0
		cur, rng, cnt, pos = dec.DebugState()
		t.Logf("first-leaf %s palY=%v sizeCtx=%d palCtx=%d current=%d rng=%d cnt=%d pos=%d", label, palY, sizeCtx, palCtx, cur, rng, cnt, pos)
		if palY {
			uvPalSize := uint8(0)
			size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdfClone, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: dec})
			if err != nil {
				t.Logf("first-leaf %s palY read err=%v", label, err)
				return
			}
			cur, rng, cnt, pos = dec.DebugState()
			t.Logf("first-leaf %s palY size=%d values=% x current=%d rng=%d cnt=%d pos=%d", label, size, values[:size], cur, rng, cnt, pos)
			if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && uvMode == IntraPredDC {
				palUVCtx := boolToInt(size > 0)
				palUV := dec.DecodeBoolAdapt(cdfClone.PalUV[palUVCtx][:]) != 0
				cur, rng, cnt, pos = dec.DebugState()
				t.Logf("first-leaf %s palUV=%v ctx=%d current=%d rng=%d cnt=%d pos=%d", label, palUV, palUVCtx, cur, rng, cnt, pos)
				if palUV {
					uvSize, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdfClone, &aboveClone, &leftClone, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: dec})
					if err != nil {
						t.Logf("first-leaf %s palUV read err=%v", label, err)
						return
					}
					uvPalSize = uvSize
					cur, rng, cnt, pos = dec.DebugState()
					t.Logf("first-leaf %s palUV size=%d U=% x V=% x current=%d rng=%d cnt=%d pos=%d", label, uvSize, palU[:uvSize], palV[:uvSize], cur, rng, cnt, pos)
				}
			}
			indices, err := readPaletteIndices(cdfClone, msacEntropyDecoder{ctx: dec}, int(size), 0, g.Width4()*4, g.Height4()*4)
			if err != nil {
				t.Logf("first-leaf %s palY idx err=%v", label, err)
				return
			}
			cur, rng, cnt, pos = dec.DebugState()
			head := minInt(8, len(indices))
			t.Logf("first-leaf %s palY idx0=%v current=%d rng=%d cnt=%d pos=%d", label, indices[:head], cur, rng, cnt, pos)
			if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && uvPalSize != 0 {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
					uvIndices, err := readPaletteIndices(cdfClone, msacEntropyDecoder{ctx: dec}, int(uvPalSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
					if err != nil {
						t.Logf("first-leaf %s palUV idx err=%v", label, err)
						return
					}
					cur, rng, cnt, pos = dec.DebugState()
					uvHead := minInt(8, len(uvIndices))
					t.Logf("first-leaf %s palUV idx0=%v current=%d rng=%d cnt=%d pos=%d", label, uvIndices[:uvHead], cur, rng, cnt, pos)
				}
			}

			tx := MaxLumaTXSizeForBlock(g.Size)
			if int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
				tx = TX4X4
			} else if hdr.TxfmMode == txModeSwitchable {
				info := TxfmInfoFor(tx)
				if info.Max > 0 {
					txCtx := GetTxCtx(&aboveClone, &leftClone, info, by4, bx4)
					txDepth := dec.DecodeSymbolAdapt(cdfClone.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
					for txDepth > 0 {
						tx = TxfmInfoFor(tx).Sub
						txDepth--
					}
				}
			}
			cur, rng, cnt, pos = dec.DebugState()
			t.Logf("first-leaf %s tx=%s current=%d rng=%d cnt=%d pos=%d", label, txName(tx), cur, rng, cnt, pos)

			syntax := IntraBlockSyntax{
				Geometry:  g,
				Size:      g.Size,
				Segment:   segID,
				YMode:     yMode,
				UVMode:    uvMode,
				YAngle:    yAngle,
				PalYSize:  size,
				PalUVSize: uvPalSize,
				TX:        tx,
				UVTX:      MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
			}
			txInfo := TxfmInfoFor(tx)
			tu := childTXGeometry(g, tx, 0, 0)
			skipCtx := GetSkipCtx(txInfo, g.Size, aboveClone.LCoef[bx4:], leftClone.LCoef[by4:], false, av1.Chroma420)
			tuSkip := dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0
			cur, rng, cnt, pos = dec.DebugState()
			t.Logf("first-leaf %s first-tu skip=%v skipCtx=%d current=%d rng=%d cnt=%d pos=%d", label, tuSkip, skipCtx, cur, rng, cnt, pos)
			if !tuSkip {
				yTxType, err := ReadIntraTxType(hdr, cdfClone, syntax, tx, haveTop, haveLeft, msacEntropyDecoder{ctx: dec})
				if err != nil {
					t.Logf("first-leaf %s first-tu txtp err=%v", label, err)
					return
				}
				cur, rng, cnt, pos = dec.DebugState()
				t.Logf("first-leaf %s first-tu txtp=%d current=%d rng=%d cnt=%d pos=%d", label, yTxType, cur, rng, cnt, pos)
				coeffs, _, _, err := DecodeLumaCoefficientsNoSkip(g.Size, tu, tx, yTxType, segID, coefCDF, dq, &aboveClone, &leftClone, msacCoefficientDecoder{ctx: dec})
				if err != nil {
					t.Logf("first-leaf %s first-tu coeff err=%v", label, err)
					return
				}
				cur, rng, cnt, pos = dec.DebugState()
				t.Logf("first-leaf %s first-tu dc=%d nonzero=%d maxAbs=%d current=%d rng=%d cnt=%d pos=%d", label, coeffDC(coeffs), coeffNonZero(coeffs), coeffMaxAbs(coeffs), cur, rng, cnt, pos)
			}
			if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
				chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
				uvTxType := IntraUVTxType(hdr, segID, syntax.UVTX, uvMode)
				uvInfo := TxfmInfoFor(syntax.UVTX)
				chromaTU := BlockGeometry{
					Level:   chromaGeom.Level,
					Size:    chromaGeom.Size,
					Start4X: chromaGeom.Start4X,
					Start4Y: chromaGeom.Start4Y,
					End4X:   min(chromaGeom.Start4X+int(uvInfo.W4), chromaGeom.End4X),
					End4Y:   min(chromaGeom.Start4Y+int(uvInfo.H4), chromaGeom.End4Y),
				}
				cur, rng, cnt, pos = dec.DebugState()
				t.Logf("first-leaf %s first-chroma uvtx=%s uvtxtype=%d current=%d rng=%d cnt=%d pos=%d", label, txName(syntax.UVTX), uvTxType, cur, rng, cnt, pos)
				for plane := 1; plane <= 2; plane++ {
					coeffs, resCtx, skip, err := DecodeChromaCoefficients(syntax.Size, chromaTU, syntax.UVTX, uvTxType, segID, plane, seq.ColorConfig.ChromaSubsampling(), coefCDF, dq, &aboveClone, &leftClone, msacCoefficientDecoder{ctx: dec})
					if err != nil {
						t.Logf("first-leaf %s first-chroma plane=%d coeff err=%v", label, plane, err)
						return
					}
					cur, rng, cnt, pos = dec.DebugState()
					t.Logf("first-leaf %s first-chroma plane=%d skip=%v resCtx=%d dc=%d nonzero=%d maxAbs=%d current=%d rng=%d cnt=%d pos=%d", label, plane, skip, resCtx, coeffDC(coeffs), coeffNonZero(coeffs), coeffMaxAbs(coeffs), cur, rng, cnt, pos)
					if err := ApplyChromaResidualContext(&aboveClone, &leftClone, plane, chromaTU, syntax.UVTX, resCtx); err != nil {
						t.Logf("first-leaf %s first-chroma plane=%d ctx err=%v", label, plane, err)
						return
					}
				}
			}
		}
	}

	_ = segID
}

func debugTraceTargetClonedIntraSyntax(t *testing.T, seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, syntaxGeom, actualGeom BlockGeometry, haveTop, haveLeft bool, segID uint8, dec *msac.Context) {
	if t == nil || hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
		return
	}
	aboveClone := *above
	leftClone := *left
	syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf.Clone(), &aboveClone, &leftClone, syntaxGeom, haveTop, haveLeft, segID, msacEntropyDecoder{ctx: dec.Clone()})
	if err != nil {
		t.Logf("target cloned syntax block=(%d,%d)-(%d,%d) err=%v", actualGeom.Start4X*4, actualGeom.Start4Y*4, actualGeom.End4X*4, actualGeom.End4Y*4, err)
		return
	}
	t.Logf("target cloned syntax block=(%d,%d)-(%d,%d) y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s",
		actualGeom.Start4X*4, actualGeom.Start4Y*4, actualGeom.End4X*4, actualGeom.End4Y*4,
		intraModeName(syntax.YMode), intraModeName(syntax.UVMode), syntax.PalYSize, syntax.PalUVSize, txName(syntax.TX), txName(syntax.UVTX))
	if syntax.PalYSize != 0 {
		head := minInt(16, len(syntax.PalYIdx))
		t.Logf("target cloned syntax palY values=%v idx0=%v", syntax.PalY[:syntax.PalYSize], syntax.PalYIdx[:head])
	}
	if syntax.PalUVSize != 0 {
		head := minInt(16, len(syntax.PalUVIdx))
		t.Logf("target cloned syntax palUV U=%v V=%v idx0=%v", syntax.PalU[:syntax.PalUVSize], syntax.PalV[:syntax.PalUVSize], syntax.PalUVIdx[:head])
	}
}

func debugTraceTargetPaletteDecision(t *testing.T, seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, syntaxGeom, actualGeom BlockGeometry, haveTop, haveLeft bool, segID uint8, dec *msac.Context) {
	if t == nil || hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
		return
	}
	cdfClone := cdf.Clone()
	aboveClone := *above
	leftClone := *left
	bx4 := syntaxGeom.Start4X & 31
	by4 := syntaxGeom.Start4Y & 31

	yMode := IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdfClone, &aboveClone, &leftClone, syntaxGeom, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(yMode, syntaxGeom.Size) {
		angle, err := readIntraAngleDelta(cdfClone, msacEntropyDecoder{ctx: dec}, yMode)
		if err != nil {
			t.Logf("target palette trace yangle err=%v", err)
			return
		}
		_ = angle
	}
	if !supportsLumaIntraPredictor(yMode, 0) {
		yMode = IntraPredDC
	}
	type debugCFLTraceState struct {
		label          string
		state          *msac.Context
		cdf            *ModeCDF
		uvMode         IntraPredMode
		uvAngle        int8
		signTok        int
		signU          int
		signV          int
		alphaCtxU      int
		alphaTokU      int
		alphaValU      int
		alphaCtxV      int
		alphaTokV      int
		alphaValV      int
		paletteSize    uint8
		paletteValues  [8]uint16
		paletteIndices []uint8
	}
	logCFLTrace := func(info debugCFLTraceState) {
		if info.state == nil || info.cdf == nil {
			return
		}
		cur, rng, cnt, pos := info.state.DebugState()
		t.Logf("target cfl trace %s uvmode=%s uvangle=%d signTok=%d signU=%d signV=%d alphaU(ctx=%d tok=%d val=%d) alphaV(ctx=%d tok=%d val=%d) palSize=%d current=%d rng=%d cnt=%d pos=%d",
			info.label,
			intraModeName(info.uvMode),
			info.uvAngle,
			info.signTok,
			info.signU,
			info.signV,
			info.alphaCtxU,
			info.alphaTokU,
			info.alphaValU,
			info.alphaCtxV,
			info.alphaTokV,
			info.alphaValV,
			info.paletteSize,
			cur,
			rng,
			cnt,
			pos,
		)
		if info.paletteSize != 0 {
			head := minInt(16, len(info.paletteIndices))
			t.Logf("target cfl trace %s pal=%v idx0=%v", info.label, info.paletteValues[:info.paletteSize], info.paletteIndices[:head])
		}
	}
	readCFLDetails := func(label string, state *msac.Context, modeCDF *ModeCDF, uvMode IntraPredMode, uvAngle int8) debugCFLTraceState {
		out := debugCFLTraceState{
			label:     label,
			state:     state,
			cdf:       modeCDF,
			uvMode:    uvMode,
			uvAngle:   uvAngle,
			signTok:   -1,
			alphaCtxU: -1,
			alphaTokU: -1,
			alphaCtxV: -1,
			alphaTokV: -1,
		}
		if state == nil || modeCDF == nil || uvMode != IntraPredCFL {
			return out
		}
		out.signTok = int(state.DecodeSymbolAdapt(modeCDF.CFLSign[:], 7))
		sign := out.signTok + 1
		out.signU = sign * 0x56 >> 8
		out.signV = sign - out.signU*3
		if out.signU != 0 {
			out.alphaCtxU = boolToInt(out.signU == 2)*3 + out.signV
			out.alphaTokU = int(state.DecodeSymbolAdapt(modeCDF.CFLAlpha[out.alphaCtxU][:], 15))
			out.alphaValU = out.alphaTokU + 1
			if out.signU == 1 {
				out.alphaValU = -out.alphaValU
			}
		}
		if out.signV != 0 {
			out.alphaCtxV = boolToInt(out.signV == 2)*3 + out.signU
			out.alphaTokV = int(state.DecodeSymbolAdapt(modeCDF.CFLAlpha[out.alphaCtxV][:], 15))
			out.alphaValV = out.alphaTokV + 1
			if out.signV == 1 {
				out.alphaValV = -out.alphaValV
			}
		}
		return out
	}
	type paletteTraceProbe struct {
		label string
		state *msac.Context
		cdf   *ModeCDF
	}
	probes := make([]paletteTraceProbe, 0, 2)
	afterYState := dec.Clone()
	afterYCDF := cdfClone.Clone()
	if hasChromaPlane(seq, syntaxGeom.Size, syntaxGeom.Start4X, syntaxGeom.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, syntaxGeom.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		uvMode := IntraPredMode(dec.DecodeSymbolAdapt(cdfClone.UVMode[boolToInt(cflAllowed)][yMode][:], lastSymbol))
		probeState := dec.Clone()
		probeCDF := cdfClone.Clone()
		probes = append(probes, paletteTraceProbe{
			label: "skip_uv_angle",
			state: probeState,
			cdf:   probeCDF,
		})
		baselineCFLState := dec.Clone()
		baselineCFLCDF := cdfClone.Clone()
		uvAngle := int8(0)
		if uvMode == IntraPredCFL {
			sign := int(dec.DecodeSymbolAdapt(cdfClone.CFLSign[:], 7)) + 1
			signU := sign * 0x56 >> 8
			signV := sign - signU*3
			if signU != 0 {
				ctx := boolToInt(signU == 2)*3 + signV
				_ = dec.DecodeSymbolAdapt(cdfClone.CFLAlpha[ctx][:], 15)
			}
			if signV != 0 {
				ctx := boolToInt(signV == 2)*3 + signU
				_ = dec.DecodeSymbolAdapt(cdfClone.CFLAlpha[ctx][:], 15)
			}
		}
		if needsIntraAngleDelta(uvMode, syntaxGeom.Size) {
			angle, err := readIntraAngleDelta(cdfClone, msacEntropyDecoder{ctx: dec}, uvMode)
			if err != nil {
				t.Logf("target palette trace uvangle err=%v", err)
				return
			}
			uvAngle = angle
		}
		cur, rng, cnt, pos := dec.DebugState()
		t.Logf("target palette trace uvmode=%s uvangle=%d current=%d rng=%d cnt=%d pos=%d", intraModeName(uvMode), uvAngle, cur, rng, cnt, pos)
		logCFLTrace(readCFLDetails("current_immediate", baselineCFLState, baselineCFLCDF, uvMode, uvAngle))
		logCFLTrace(debugCFLTraceState{
			label:     "skip_cfl_alpha",
			state:     baselineCFLState.Clone(),
			cdf:       baselineCFLCDF.Clone(),
			uvMode:    uvMode,
			uvAngle:   uvAngle,
			signTok:   -1,
			alphaCtxU: -1,
			alphaTokU: -1,
			alphaCtxV: -1,
			alphaTokV: -1,
		})
		if cflAllowed {
			noCFLState := afterYState.Clone()
			noCFLCDF := afterYCDF.Clone()
			noCFLMode := IntraPredMode(noCFLState.DecodeSymbolAdapt(noCFLCDF.UVMode[0][yMode][:], int(numIntraPredModes-1)))
			probes = append(probes, paletteTraceProbe{
				label: "no_cfl_skip_uv_angle",
				state: noCFLState.Clone(),
				cdf:   noCFLCDF.Clone(),
			})
			noCFLAngle := int8(0)
			if needsIntraAngleDelta(noCFLMode, syntaxGeom.Size) {
				angle, err := readIntraAngleDelta(noCFLCDF, msacEntropyDecoder{ctx: noCFLState}, noCFLMode)
				if err != nil {
					t.Logf("target palette trace no_cfl uvangle err=%v", err)
				} else {
					noCFLAngle = angle
				}
			}
			probes = append(probes, paletteTraceProbe{
				label: "no_cfl",
				state: noCFLState,
				cdf:   noCFLCDF,
			})
			cur, rng, cnt, pos = noCFLState.DebugState()
			t.Logf("target palette trace no_cfl uvmode=%s uvangle=%d current=%d rng=%d cnt=%d pos=%d", intraModeName(noCFLMode), noCFLAngle, cur, rng, cnt, pos)
		}
		if paletteAllowedForBlock(hdr, syntaxGeom) && yMode == IntraPredDC {
			sizeCtx := paletteSizeContext(syntaxGeom)
			palCtx := boolToInt(aboveClone.PalSz[bx4] > 0) + boolToInt(leftClone.PalSz[by4] > 0)
			afterPaletteState := baselineCFLState.Clone()
			afterPaletteCDF := baselineCFLCDF.Clone()
			pal := afterPaletteState.DecodeBoolAdapt(afterPaletteCDF.PalY[sizeCtx][palCtx][:]) != 0
			if pal {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, afterPaletteCDF, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: afterPaletteState})
				if err != nil {
					t.Logf("target cfl trace after_luma_palette palette err=%v", err)
				} else {
					info := readCFLDetails("after_luma_palette", afterPaletteState, afterPaletteCDF, uvMode, uvAngle)
					info.paletteSize = size
					info.paletteValues = values
					logCFLTrace(info)
				}
			}
			afterIdxState := baselineCFLState.Clone()
			afterIdxCDF := baselineCFLCDF.Clone()
			pal = afterIdxState.DecodeBoolAdapt(afterIdxCDF.PalY[sizeCtx][palCtx][:]) != 0
			if pal {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, afterIdxCDF, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: afterIdxState})
				if err != nil {
					t.Logf("target cfl trace after_luma_indices palette err=%v", err)
				} else {
					indices, err := readPaletteIndices(afterIdxCDF, msacEntropyDecoder{ctx: afterIdxState}, int(size), 0, syntaxGeom.Width4()*4, syntaxGeom.Height4()*4)
					if err != nil {
						t.Logf("target cfl trace after_luma_indices idx err=%v", err)
					} else {
						info := readCFLDetails("after_luma_indices", afterIdxState, afterIdxCDF, uvMode, uvAngle)
						info.paletteSize = size
						info.paletteValues = values
						info.paletteIndices = indices
						logCFLTrace(info)
					}
				}
			}
		}
	}
	if !paletteAllowedForBlock(hdr, syntaxGeom) || yMode != IntraPredDC {
		return
	}
	sizeCtx := paletteSizeContext(syntaxGeom)
	palCtx := boolToInt(aboveClone.PalSz[bx4] > 0) + boolToInt(leftClone.PalSz[by4] > 0)
	palState := dec.Clone()
	palCurrent := palState.DecodeBoolAdapt(cdfClone.PalY[sizeCtx][palCtx][:]) != 0
	palAfterFlagState := palState.Clone()
	palAfterFlagCDF := cdfClone.Clone()
	palDefaultState := dec.Clone()
	defaultCDF := NewDefaultModeCDF()
	palDefault := palDefaultState.DecodeBoolAdapt(defaultCDF.PalY[sizeCtx][palCtx][:]) != 0
	cur, rng, cnt, pos := dec.DebugState()
	t.Logf("target palette trace block=(%d,%d)-(%d,%d) state current=%d rng=%d cnt=%d pos=%d sizeCtx=%d palCtx=%d currentPal=%v defaultPal=%v",
		actualGeom.Start4X*4, actualGeom.Start4Y*4, actualGeom.End4X*4, actualGeom.End4Y*4, cur, rng, cnt, pos, sizeCtx, palCtx, palCurrent, palDefault)
	for altPalCtx := 0; altPalCtx < 3; altPalCtx++ {
		altState := dec.Clone()
		altPal := altState.DecodeBoolAdapt(cdfClone.PalY[sizeCtx][altPalCtx][:]) != 0
		cur, rng, cnt, pos = altState.DebugState()
		t.Logf("target palette trace altPalCtx=%d pal=%v current=%d rng=%d cnt=%d pos=%d", altPalCtx, altPal, cur, rng, cnt, pos)
	}
	if palCurrent {
		palCurrentState := palAfterFlagState.Clone()
		palCurrentCDF := palAfterFlagCDF.Clone()
		size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, palCurrentCDF, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: palCurrentState})
		if err != nil {
			t.Logf("target palette trace current read err=%v", err)
		} else {
			cur, rng, cnt, pos = palCurrentState.DebugState()
			t.Logf("target palette trace current size=%d values=%v current=%d rng=%d cnt=%d pos=%d", size, values[:size], cur, rng, cnt, pos)
		}
		traceState := palAfterFlagState.Clone()
		traceCDF := palAfterFlagCDF.Clone()
		if _, _, err := debugReadPalettePlaneSyntaxTrace(t, "target palette trace current", seq.ColorConfig.BitDepth, traceCDF, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: traceState}); err != nil {
			t.Logf("target palette trace current trace err=%v", err)
		}
		noCacheAbove := aboveClone
		noCacheLeft := leftClone
		noCacheAbove.PalSz[bx4] = 0
		noCacheLeft.PalSz[by4] = 0
		noCacheAbove.Pal[0][bx4] = [8]uint16{}
		noCacheLeft.Pal[0][by4] = [8]uint16{}
		palNoCacheState := palAfterFlagState.Clone()
		palNoCacheCDF := palAfterFlagCDF.Clone()
		size, values, err = readPalettePlaneSyntax(seq.ColorConfig.BitDepth, palNoCacheCDF, &noCacheAbove, &noCacheLeft, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: palNoCacheState})
		if err != nil {
			t.Logf("target palette trace no_cache read err=%v", err)
		} else {
			cur, rng, cnt, pos = palNoCacheState.DebugState()
			t.Logf("target palette trace no_cache size=%d values=%v current=%d rng=%d cnt=%d pos=%d", size, values[:size], cur, rng, cnt, pos)
		}
	}
	if palDefault {
		defaultCDF2 := NewDefaultModeCDF()
		size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, defaultCDF2, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: palDefaultState})
		if err != nil {
			t.Logf("target palette trace default read err=%v", err)
		} else {
			cur, rng, cnt, pos = palDefaultState.DebugState()
			t.Logf("target palette trace default size=%d values=%v current=%d rng=%d cnt=%d pos=%d", size, values[:size], cur, rng, cnt, pos)
		}
	}
	for _, probe := range probes {
		if probe.state == nil || probe.cdf == nil {
			continue
		}
		probePalState := probe.state.Clone()
		pal := probePalState.DecodeBoolAdapt(probe.cdf.PalY[sizeCtx][palCtx][:]) != 0
		cur, rng, cnt, pos := probePalState.DebugState()
		t.Logf("target palette trace %s pal=%v current=%d rng=%d cnt=%d pos=%d", probe.label, pal, cur, rng, cnt, pos)
		if !pal {
			continue
		}
		size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, probe.cdf, &aboveClone, &leftClone, 0, sizeCtx, bx4, by4, msacEntropyDecoder{ctx: probePalState})
		if err != nil {
			t.Logf("target palette trace %s read err=%v", probe.label, err)
			continue
		}
		cur, rng, cnt, pos = probePalState.DebugState()
		t.Logf("target palette trace %s size=%d values=%v current=%d rng=%d cnt=%d pos=%d", probe.label, size, values[:size], cur, rng, cnt, pos)
	}
	_ = segID
}

func debugReadPalettePlaneSyntaxTrace(t *testing.T, label string, bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return 0, [8]uint16{}, fmt.Errorf("decoder: palette plane syntax requires decoder state")
	}
	if bitDepth == 0 {
		bitDepth = 8
	}
	palSize := int(dec.DecodeSymbolAdapt(cdf.PalSize[plane][sizeCtx][:], 6)) + 2
	cache, nCache := paletteCache(above, left, plane, bx4, by4)
	t.Logf("%s cache=%v nCache=%d palSize=%d", label, cache[:nCache], nCache, palSize)
	usedCache := [8]uint16{}
	nUsed := 0
	for i := 0; i < nCache && nUsed < palSize; i++ {
		use := dec.DecodeBoolEqui() != 0
		t.Logf("%s cacheBit[%d]=%v value=%d", label, i, use, cache[i])
		if use {
			usedCache[nUsed] = cache[i]
			nUsed++
		}
	}

	var pal [8]uint16
	if nUsed < palSize {
		bpc := int(bitDepth)
		next := nUsed
		prev := int(dec.DecodeBools(uint(bpc)))
		t.Logf("%s new[0]=%d", label, prev)
		pal[next] = uint16(prev)
		next++

		if next < palSize {
			bits := bpc - 3 + int(dec.DecodeBools(2))
			maxValue := (1 << bpc) - 1
			minStep := 1
			if plane != 0 {
				minStep = 0
			}
			for next < palSize {
				delta := int(dec.DecodeBools(uint(bits)))
				prev = minInt(prev+delta+minStep, maxValue)
				t.Logf("%s new[%d]=%d delta=%d bits=%d", label, next-nUsed, prev, delta, bits)
				pal[next] = uint16(prev)
				next++
				if prev+minStep >= maxValue {
					for next < palSize {
						pal[next] = uint16(maxValue)
						t.Logf("%s fill max new[%d]=%d", label, next-nUsed, maxValue)
						next++
					}
					break
				}
				bits = minInt(bits, 1+ulog2(maxValue-prev-minStep))
			}
		}

		merged := [8]uint16{}
		a := 0
		b := nUsed
		for i := 0; i < palSize; i++ {
			if a < nUsed && (b >= palSize || usedCache[a] <= pal[b]) {
				merged[i] = usedCache[a]
				a++
			} else {
				merged[i] = pal[b]
				b++
			}
		}
		pal = merged
	} else {
		copy(pal[:], usedCache[:nUsed])
	}
	t.Logf("%s usedCache=%v final=%v", label, usedCache[:nUsed], pal[:palSize])
	return uint8(palSize), pal, nil
}

func pointInBlock(x, y int, g BlockGeometry) bool {
	return x >= g.Start4X*4 && x < g.End4X*4 && y >= g.Start4Y*4 && y < g.End4Y*4
}

func blockMeanY(img *image.YCbCr, g BlockGeometry) float64 {
	x0 := g.Start4X * 4
	y0 := g.Start4Y * 4
	x1 := minInt(g.End4X*4, img.Rect.Dx())
	y1 := minInt(g.End4Y*4, img.Rect.Dy())
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	sum := 0
	count := 0
	for y := y0; y < y1; y++ {
		row := img.Y[y*img.YStride:]
		for x := x0; x < x1; x++ {
			sum += int(row[x])
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

func debugReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, skip bool, dec keyIntraBlockEntropyDecoder, topHasRight, leftHasBottom bool, targets [][2]int, t *testing.T) error {
	if skip {
		return ReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, skip, dec, topHasRight, leftHasBottom)
	}
	if hdr == nil {
		return io.ErrUnexpectedEOF
	}
	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	txInfo := TxfmInfoFor(syntax.TX)
	g := syntax.Geometry
	blockX := planeX4 * 4
	blockY := planeY4 * 4
	predictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	logBlockTUs := false
	var prePlane []byte
	var tuRecords []debugLumaTURecord
	var refY []byte
	var paletteCacheVals []uint16
	for _, pt := range targets {
		if pointInBlock(pt[0], pt[1], g) {
			logBlockTUs = true
			break
		}
	}
	if !logBlockTUs {
		return ReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, skip, dec, topHasRight, leftHasBottom)
	}
	if logBlockTUs {
		prePlane = append([]byte(nil), plane...)
		if ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv"); err == nil && len(ref) >= planeWidth*planeHeight {
			refY = ref[:planeWidth*planeHeight]
		}
		if syntax.PalYSize != 0 {
			cache, nCache := paletteCache(above, left, 0, g.Start4X&31, g.Start4Y&31)
			paletteCacheVals = append([]uint16(nil), cache[:nCache]...)
		}
	}
	if syntax.PalYSize == 0 {
		if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
			plane,
			stride,
			planeWidth,
			planeHeight,
			blockX,
			blockY,
			blockWidth4*4,
			blockHeight4*4,
			syntax.YMode,
			syntax.YAngle,
			predictFlags,
			syntax.Geometry.Start4Y > 0,
			syntax.Geometry.Start4X > 0,
			topHasRight,
			leftHasBottom,
		); err != nil {
			return err
		}
		if logBlockTUs {
			_, _, topDbg, leftDbg, topLeftDbg, haveTopDbg, haveLeftDbg, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(prePlane, stride, planeWidth, planeHeight, blockX, blockY, blockWidth4*4, blockHeight4*4, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, topHasRight, leftHasBottom)
			if err == nil {
				t.Logf("block edges block=(%d,%d)-(%d,%d) topLeft=%d haveTop=%v haveLeft=%v top=%v left=%v",
					blockX, blockY, blockX+blockWidth4*4, blockY+blockHeight4*4, topLeftDbg, haveTopDbg, haveLeftDbg,
					topDbg[:minInt(len(topDbg), 8)], leftDbg[:minInt(len(leftDbg), 8)])
				writeback.release()
			}
			patchW := minInt(8, blockWidth4*4)
			patchH := minInt(8, blockHeight4*4)
			var rows []string
			for py := 0; py < patchH; py++ {
				rows = append(rows, fmt.Sprintf("%v", plane[(blockY+py)*stride+blockX:(blockY+py)*stride+blockX+patchW]))
			}
			t.Logf("block after_predict block=(%d,%d)-(%d,%d) rows=%v", blockX, blockY, blockX+patchW, blockY+patchH, rows)
		}
	}
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
			bx4 := tu.Start4X & 31
			by4 := tu.Start4Y & 31
			skipCtx := GetSkipCtx(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420)
			if blockX == 0 && blockY == 0 && offX4 == 0 && offY4 == 0 {
				if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
					for altCtx := 0; altCtx < len(coefCDF.Skip[txInfo.Ctx]); altCtx++ {
						altDec := cloneable.Clone()
						altCDF := coefCDF.Skip[txInfo.Ctx][altCtx]
						altBit := altDec.DecodeBoolAdapt(altCDF[:])
						t.Logf("root first-tu alt-skipctx=%d bit=%d prob=%d", altCtx, altBit, altCDF[0])
						if logBlockTUs && len(refY) >= planeWidth*planeHeight && altBit == 0 {
							altTxType, err := ReadIntraTxType(hdr, modeCDF, syntax, syntax.TX, syntax.Geometry.Start4Y > 0 || offY4 > 0, syntax.Geometry.Start4X > 0 || offX4 > 0, msacEntropyDecoder{ctx: altDec})
							if err != nil {
								t.Logf("root first-tu alt-skipctx=%d tx-read err=%v", altCtx, err)
								continue
							}
							altCoeffs, _, _, err := DecodeLumaCoefficientsNoSkip(syntax.Size, tu, syntax.TX, altTxType, syntax.Segment, coefCDF, dq, above, left, msacCoefficientDecoder{ctx: altDec})
							if err != nil {
								t.Logf("root first-tu alt-skipctx=%d coeff err=%v", altCtx, err)
								continue
							}
							scratch := append([]byte(nil), prePlane...)
							tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, syntax.TX, intraEdgeAvailability{
								topHasRight:   topHasRight,
								leftHasBottom: leftHasBottom,
							})
							if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsEdges(scratch, stride, planeWidth, planeHeight, blockX, blockY, syntax.TX, altTxType, syntax.YMode, syntax.YAngle, altCoeffs, hdr.Segmentation.Lossless[syntax.Segment], predictFlags, tuEdge.topHasRight, tuEdge.leftHasBottom); err != nil {
								t.Logf("root first-tu alt-skipctx=%d replay err=%v", altCtx, err)
								continue
							}
							mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(int(txInfo.W4)*4, planeWidth-blockX), minInt(int(txInfo.H4)*4, planeHeight-blockY))
							t.Logf("root first-tu alt-skipctx=%d nonskip txType=%d dc=%d nonzero=%d maxAbs=%d mse=%.2f", altCtx, altTxType, coeffDC(altCoeffs), coeffNonZero(altCoeffs), coeffMaxAbs(altCoeffs), mse)
						}
					}
				}
			}
			var (
				yTxType TxType
				coeffs  []int32
				resCtx  uint8
				tuSkip  bool
				err     error
			)
			if dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
				tuSkip = true
				resCtx = 0x40
				coeffs = make([]int32, int(txInfo.W4)*4*int(txInfo.H4)*4)
				if int(syntax.Segment) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[syntax.Segment] {
					yTxType = TxTypeWHTWHT
				} else {
					yTxType = TxTypeDCTDCT
				}
			} else {
				yTxType, err = ReadIntraTxType(hdr, modeCDF, syntax, syntax.TX, syntax.Geometry.Start4Y > 0 || offY4 > 0, syntax.Geometry.Start4X > 0 || offX4 > 0, dec)
				if err != nil {
					return err
				}
				coeffs, resCtx, tuSkip, err = DecodeLumaCoefficientsNoSkip(syntax.Size, tu, syntax.TX, yTxType, syntax.Segment, coefCDF, dq, above, left, dec)
				if err != nil {
					return err
				}
			}
			if logBlockTUs {
				t.Logf("block TU tu=(%d,%d)-(%d,%d) txType=%d tuSkip=%v dc=%d nonzero=%d maxAbs=%d",
					tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4, yTxType, tuSkip, coeffDC(coeffs), coeffNonZero(coeffs), coeffMaxAbs(coeffs))
				appendDebugTUTrace("block TU tu=(%d,%d)-(%d,%d) txType=%d tuSkip=%v dc=%d nonzero=%d maxAbs=%d",
					tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4, yTxType, tuSkip, coeffDC(coeffs), coeffNonZero(coeffs), coeffMaxAbs(coeffs))
				tuRecords = append(tuRecords, debugLumaTURecord{
					offX4:  offX4,
					offY4:  offY4,
					txType: yTxType,
					coeffs: append([]int32(nil), coeffs...),
					tuSkip: tuSkip,
				})
			}
			px := (planeX4 + offX4) * 4
			py := (planeY4 + offY4) * 4
			if syntax.PalYSize != 0 {
				err = ReconstructPaletteLumaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, px, py, syntax.TX, yTxType, coeffs, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
			} else {
				if !tuSkip {
					err = InverseAddResidualTx8At(plane, stride, planeWidth, planeHeight, px, py, syntax.TX, yTxType, coeffs, hdr.Segmentation.Lossless[syntax.Segment])
				}
			}
			if err != nil {
				return err
			}
			if logBlockTUs && len(refY) >= planeWidth*planeHeight {
				tuW := minInt(int(txInfo.W4)*4, planeWidth-px)
				tuH := minInt(int(txInfo.H4)*4, planeHeight-py)
				if tuW > 0 && tuH > 0 {
					mse := compareBlockToRefY(plane, refY, stride, px, py, tuW, tuH)
					t.Logf("block TU mse tu=(%d,%d)-(%d,%d) mse=%.2f", px, py, px+tuW, py+tuH, mse)
					appendDebugTUTrace("block TU mse tu=(%d,%d)-(%d,%d) mse=%.2f", px, py, px+tuW, py+tuH, mse)
				}
			}
			if err := ApplyLumaResidualContext(above, left, tu, syntax.TX, resCtx); err != nil {
				return err
			}
		}
	}
	if logBlockTUs {
		if err := debugReplayIntraBlockVariants(hdr, syntax, prePlane, planeWidth, planeHeight, stride, blockX, blockY, predictFlags, topHasRight, leftHasBottom, tuRecords, paletteCacheVals, targets, t); err != nil {
			return err
		}
	}
	return nil
}

func debugReplayIntraBlockVariants(hdr *obu.FrameHeader, syntax IntraBlockSyntax, prePlane []byte, planeWidth, planeHeight, stride, blockX, blockY int, predictFlags uint16, topHasRight, leftHasBottom bool, tus []debugLumaTURecord, paletteCacheVals []uint16, targets [][2]int, t *testing.T) error {
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		return err
	}
	if len(ref) < planeWidth*planeHeight {
		return io.ErrUnexpectedEOF
	}
	refY := ref[:planeWidth*planeHeight]
	blockW := syntax.Geometry.End4X*4 - syntax.Geometry.Start4X*4
	blockH := syntax.Geometry.End4Y*4 - syntax.Geometry.Start4Y*4
	makeVariant := func(name string, useTop, useLeft bool) error {
		scratch := append([]byte(nil), prePlane...)
		if useTop && blockY > 0 {
			copy(scratch[(blockY-1)*stride+blockX:(blockY-1)*stride+blockX+blockW], refY[(blockY-1)*planeWidth+blockX:(blockY-1)*planeWidth+blockX+blockW])
		}
		if useLeft && blockX > 0 {
			for py := blockY; py < blockY+blockH; py++ {
				scratch[py*stride+blockX-1] = refY[py*planeWidth+blockX-1]
			}
		}
		if syntax.PalYSize == 0 {
			if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
				scratch,
				stride,
				planeWidth,
				planeHeight,
				blockX,
				blockY,
				blockW,
				blockH,
				syntax.YMode,
				syntax.YAngle,
				predictFlags,
				blockY > 0,
				blockX > 0,
				topHasRight,
				leftHasBottom,
			); err != nil {
				return err
			}
		}
		for _, tuRec := range tus {
			px := blockX + tuRec.offX4*4
			py := blockY + tuRec.offY4*4
			var err error
			if syntax.PalYSize != 0 {
				err = ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
			} else {
				if !tuRec.tuSkip {
					err = InverseAddResidualTx8At(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment])
				}
			}
			if err != nil {
				return err
			}
		}
		mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
		t.Logf("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
		appendDebugTUTrace("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
		debugLogTargetPatchMSEs(t, name, scratch, refY, stride, planeWidth, blockX, blockY, blockW, blockH, targets)
		return nil
	}
	for _, tc := range []struct {
		name    string
		useTop  bool
		useLeft bool
	}{
		{name: "current_edges", useTop: false, useLeft: false},
		{name: "ref_top", useTop: true, useLeft: false},
		{name: "ref_left", useTop: false, useLeft: true},
		{name: "ref_both", useTop: true, useLeft: true},
	} {
		if err := makeVariant(tc.name, tc.useTop, tc.useLeft); err != nil {
			return err
		}
	}
	if syntax.PalYSize == 0 && len(tus) != 0 {
		scratch := append([]byte(nil), prePlane...)
		if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
			scratch,
			stride,
			planeWidth,
			planeHeight,
			blockX,
			blockY,
			blockW,
			blockH,
			syntax.YMode,
			syntax.YAngle,
			predictFlags,
			blockY > 0,
			blockX > 0,
			topHasRight,
			leftHasBottom,
		); err != nil {
			return err
		}
		mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
		t.Logf("base_only replay block (%d,%d)-(%d,%d) mse=%.2f", blockX, blockY, blockX+blockW, blockY+blockH, mse)
		debugLogTargetPatchMSEs(t, "base_only", scratch, refY, stride, planeWidth, blockX, blockY, blockW, blockH, targets)

		for idx, tuRec := range tus {
			scratch := append([]byte(nil), prePlane...)
			if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
				scratch,
				stride,
				planeWidth,
				planeHeight,
				blockX,
				blockY,
				blockW,
				blockH,
				syntax.YMode,
				syntax.YAngle,
				predictFlags,
				blockY > 0,
				blockX > 0,
				topHasRight,
				leftHasBottom,
			); err != nil {
				return err
			}
			if !tuRec.tuSkip {
				px := blockX + tuRec.offX4*4
				py := blockY + tuRec.offY4*4
				if err := InverseAddResidualTx8At(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment]); err != nil {
					return err
				}
			}
			mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
			t.Logf("keep_only_tu_%d replay block (%d,%d)-(%d,%d) tu=(%d,%d) mse=%.2f", idx, blockX, blockY, blockX+blockW, blockY+blockH, blockX+tuRec.offX4*4, blockY+tuRec.offY4*4, mse)
			debugLogTargetPatchMSEs(t, fmt.Sprintf("keep_only_tu_%d", idx), scratch, refY, stride, planeWidth, blockX, blockY, blockW, blockH, targets)
		}
	}
	if len(tus) != 0 {
		for _, mode := range []IntraPredMode{
			IntraPredDC,
			IntraPredVertical,
			IntraPredHorizontal,
			IntraPredDiagDownLeft,
			IntraPredDiagDownRight,
			IntraPredVertRight,
			IntraPredHorDown,
			IntraPredHorUp,
			IntraPredVertLeft,
			IntraPredSmooth,
			IntraPredSmoothVertical,
			IntraPredSmoothHorizontal,
			IntraPredPaeth,
		} {
			if err := debugReplayBlockForcedMode(hdr, syntax, prePlane, planeWidth, planeHeight, stride, blockX, blockY, predictFlags, topHasRight, leftHasBottom, tus, mode, t); err != nil {
				return err
			}
		}
		if syntax.PalYSize == 0 && (syntax.YMode == IntraPredDC || syntax.YMode == IntraPredFilter) {
			for filterIdx := int8(0); filterIdx < 5; filterIdx++ {
				if err := debugReplayBlockForcedFilterIntra(hdr, syntax, prePlane, planeWidth, planeHeight, stride, blockX, blockY, topHasRight, leftHasBottom, tus, filterIdx, t); err != nil {
					return err
				}
			}
		}
	}
	for _, txType := range debugReplayAltTxTypes(syntax.TX) {
		if err := debugReplayBlockForcedTxType(hdr, syntax, prePlane, planeWidth, planeHeight, stride, blockX, blockY, predictFlags, topHasRight, leftHasBottom, tus, txType, t); err != nil {
			return err
		}
	}
	if syntax.PalYSize != 0 && len(paletteCacheVals) >= int(syntax.PalYSize) {
		bestMSE := 1e30
		var best [8]uint16
		var search func(start, depth int, cur [8]uint16)
		search = func(start, depth int, cur [8]uint16) {
			if depth == int(syntax.PalYSize) {
				alt := syntax
				alt.PalY = cur
				scratch := append([]byte(nil), prePlane...)
				for _, tuRec := range tus {
					px := blockX + tuRec.offX4*4
					py := blockY + tuRec.offY4*4
					if err := ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, alt.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[alt.Segment], alt, blockX, blockY); err != nil {
						return
					}
				}
				mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
				if mse < bestMSE {
					bestMSE = mse
					best = cur
				}
				return
			}
			need := int(syntax.PalYSize) - depth
			for i := start; i <= len(paletteCacheVals)-need; i++ {
				cur[depth] = paletteCacheVals[i]
				search(i+1, depth+1, cur)
			}
		}
		search(0, 0, [8]uint16{})
		t.Logf("best_cache_palette replay block (%d,%d)-(%d,%d) mse=%.2f pal=%v cache=%v", blockX, blockY, blockX+blockW, blockY+blockH, bestMSE, best[:syntax.PalYSize], paletteCacheVals)
	}
	if syntax.PalYSize != 0 && len(paletteCacheVals) > 0 && len(paletteCacheVals) < int(syntax.PalYSize) {
		alt := syntax
		var pal [8]uint16
		copy(pal[:], paletteCacheVals)
		tailCount := int(syntax.PalYSize) - len(paletteCacheVals)
		if tailCount > 0 {
			copy(pal[len(paletteCacheVals):], syntax.PalY[int(syntax.PalYSize)-tailCount:int(syntax.PalYSize)])
		}
		alt.PalY = pal
		scratch := append([]byte(nil), prePlane...)
		for _, tuRec := range tus {
			px := blockX + tuRec.offX4*4
			py := blockY + tuRec.offY4*4
			if err := ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, alt.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[alt.Segment], alt, blockX, blockY); err != nil {
				return err
			}
		}
		mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
		t.Logf("cache_prefix_palette replay block (%d,%d)-(%d,%d) mse=%.2f pal=%v cache=%v", blockX, blockY, blockX+blockW, blockY+blockH, mse, pal[:syntax.PalYSize], paletteCacheVals)
	}
	if syntax.PalYSize != 0 {
		if syntax.PalYSize <= 7 {
			bestMSE := math.MaxFloat64
			best := syntax.PalY
			cur := syntax.PalY
			var used [8]bool
			var search func(depth int)
			search = func(depth int) {
				if depth == int(syntax.PalYSize) {
					alt := syntax
					alt.PalY = cur
					scratch := append([]byte(nil), prePlane...)
					for _, tuRec := range tus {
						px := blockX + tuRec.offX4*4
						py := blockY + tuRec.offY4*4
						if err := ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, alt.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[alt.Segment], alt, blockX, blockY); err != nil {
							return
						}
					}
					mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
					if mse < bestMSE {
						bestMSE = mse
						best = cur
					}
					return
				}
				for i := 0; i < int(syntax.PalYSize); i++ {
					if used[i] {
						continue
					}
					used[i] = true
					cur[depth] = syntax.PalY[i]
					search(depth + 1)
					used[i] = false
				}
			}
			search(0)
			t.Logf("best_palette_permutation replay block (%d,%d)-(%d,%d) mse=%.2f pal=%v", blockX, blockY, blockX+blockW, blockY+blockH, bestMSE, best[:syntax.PalYSize])
		}
		if len(paletteCacheVals) != 0 {
			valueSetMap := make(map[uint16]struct{}, len(paletteCacheVals)+int(syntax.PalYSize))
			valueSet := make([]uint16, 0, len(paletteCacheVals)+int(syntax.PalYSize))
			appendValue := func(v uint16) {
				if _, ok := valueSetMap[v]; ok {
					return
				}
				valueSetMap[v] = struct{}{}
				valueSet = append(valueSet, v)
			}
			for _, v := range paletteCacheVals {
				appendValue(v)
			}
			for _, v := range syntax.PalY[:syntax.PalYSize] {
				appendValue(v)
			}
			if len(valueSet) >= int(syntax.PalYSize) && syntax.PalYSize <= 7 && len(valueSet) <= 10 {
				bestMSE := math.MaxFloat64
				best := syntax.PalY
				var combo [8]uint16
				var choose func(start, depth int)
				choose = func(start, depth int) {
					if depth == int(syntax.PalYSize) {
						cur := combo
						var used [8]bool
						var permute func(depth int)
						permute = func(depth int) {
							if depth == int(syntax.PalYSize) {
								alt := syntax
								alt.PalY = cur
								scratch := append([]byte(nil), prePlane...)
								for _, tuRec := range tus {
									px := blockX + tuRec.offX4*4
									py := blockY + tuRec.offY4*4
									if err := ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, alt.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[alt.Segment], alt, blockX, blockY); err != nil {
										return
									}
								}
								mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
								if mse < bestMSE {
									bestMSE = mse
									best = cur
								}
								return
							}
							for i := 0; i < int(syntax.PalYSize); i++ {
								if used[i] {
									continue
								}
								used[i] = true
								cur[depth] = combo[i]
								permute(depth + 1)
								used[i] = false
							}
						}
						permute(0)
						return
					}
					need := int(syntax.PalYSize) - depth
					for i := start; i <= len(valueSet)-need; i++ {
						combo[depth] = valueSet[i]
						choose(i+1, depth+1)
					}
				}
				choose(0, 0)
				t.Logf("best_cache_mixed_palette replay block (%d,%d)-(%d,%d) mse=%.2f pal=%v cache=%v values=%v",
					blockX, blockY, blockX+blockW, blockY+blockH, bestMSE, best[:syntax.PalYSize], paletteCacheVals, valueSet)
			}
		}

		replayAltIndices := func(name string, idx []uint8) error {
			if len(idx) != len(syntax.PalYIdx) {
				return nil
			}
			alt := syntax
			alt.PalYIdx = idx
			scratch := append([]byte(nil), prePlane...)
			for _, tuRec := range tus {
				px := blockX + tuRec.offX4*4
				py := blockY + tuRec.offY4*4
				if err := ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, alt.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[alt.Segment], alt, blockX, blockY); err != nil {
					return err
				}
			}
			mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
			t.Logf("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
			return nil
		}
		if err := replayAltIndices("palette_idx_reverse_diagonals", paletteIndicesReverseDiagonals(syntax.PalYIdx, syntax.Geometry.Width4()*4, syntax.Geometry.Height4()*4)); err != nil {
			return err
		}
		if err := replayAltIndices("palette_idx_diag_to_row_major", paletteIndicesDiagonalToRowMajor(syntax.PalYIdx, syntax.Geometry.Width4()*4, syntax.Geometry.Height4()*4)); err != nil {
			return err
		}

		scratch := append([]byte(nil), prePlane...)
		for _, tuRec := range tus {
			px := blockX + tuRec.offX4*4
			py := blockY + tuRec.offY4*4
			zeroCoeffs := make([]int32, len(tuRec.coeffs))
			if err := ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, zeroCoeffs, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY); err != nil {
				return err
			}
		}
		mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
		t.Logf("palette_base_only replay block (%d,%d)-(%d,%d) mse=%.2f", blockX, blockY, blockX+blockW, blockY+blockH, mse)
		debugLogTargetPatchMSEs(t, "palette_base_only", scratch, refY, stride, planeWidth, blockX, blockY, blockW, blockH, targets)

		// Diagnostic only: reuse the already-decoded residual but disable palette
		// prediction to distinguish local palette corruption from earlier syntax drift.
		scratch = append([]byte(nil), prePlane...)
		if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
			scratch,
			stride,
			planeWidth,
			planeHeight,
			blockX,
			blockY,
			blockW,
			blockH,
			syntax.YMode,
			syntax.YAngle,
			predictFlags,
			blockY > 0,
			blockX > 0,
			topHasRight,
			leftHasBottom,
		); err != nil {
			return err
		}
		for _, tuRec := range tus {
			if tuRec.tuSkip {
				continue
			}
			px := blockX + tuRec.offX4*4
			py := blockY + tuRec.offY4*4
			if err := InverseAddResidualTx8At(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment]); err != nil {
				return err
			}
		}
		mse = compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
		t.Logf("force_no_palette replay block (%d,%d)-(%d,%d) mse=%.2f", blockX, blockY, blockX+blockW, blockY+blockH, mse)
		debugLogTargetPatchMSEs(t, "force_no_palette", scratch, refY, stride, planeWidth, blockX, blockY, blockW, blockH, targets)
	}
	return nil
}

func paletteIndicesReverseDiagonals(src []uint8, width, height int) []uint8 {
	if width <= 0 || height <= 0 || len(src) != width*height {
		return nil
	}
	seq := make([]uint8, 0, len(src))
	for diag := 0; diag < width+height-1; diag++ {
		first := minInt(diag, width-1)
		last := maxIntLocal(0, diag-height+1)
		for x := first; x >= last; x-- {
			y := diag - x
			seq = append(seq, src[y*width+x])
		}
	}
	out := make([]uint8, len(src))
	pos := 0
	for diag := 0; diag < width+height-1; diag++ {
		first := minInt(diag, width-1)
		last := maxIntLocal(0, diag-height+1)
		for x := last; x <= first; x++ {
			y := diag - x
			out[y*width+x] = seq[pos]
			pos++
		}
	}
	return out
}

func paletteIndicesDiagonalToRowMajor(src []uint8, width, height int) []uint8 {
	if width <= 0 || height <= 0 || len(src) != width*height {
		return nil
	}
	seq := make([]uint8, 0, len(src))
	for diag := 0; diag < width+height-1; diag++ {
		first := minInt(diag, width-1)
		last := maxIntLocal(0, diag-height+1)
		for x := first; x >= last; x-- {
			y := diag - x
			seq = append(seq, src[y*width+x])
		}
	}
	out := make([]uint8, len(src))
	copy(out, seq)
	return out
}

func debugLogTargetPatchMSEs(t *testing.T, label string, ours, ref []byte, stride, planeWidth, blockX, blockY, blockW, blockH int, targets [][2]int) {
	if t == nil {
		return
	}
	for _, pt := range targets {
		px, py := pt[0], pt[1]
		if px < blockX || px >= blockX+blockW || py < blockY || py >= blockY+blockH {
			continue
		}
		patchX := (px / 16) * 16
		patchY := (py / 16) * 16
		if patchX < blockX {
			patchX = blockX
		}
		if patchY < blockY {
			patchY = blockY
		}
		patchW := minInt(16, planeWidth-patchX)
		patchH := minInt(16, (len(ref)/planeWidth)-patchY)
		if patchX+patchW > blockX+blockW {
			patchW = blockX + blockW - patchX
		}
		if patchY+patchH > blockY+blockH {
			patchH = blockY + blockH - patchY
		}
		if patchW <= 0 || patchH <= 0 {
			continue
		}
		mse := compareBlockToRefY(ours, ref, stride, patchX, patchY, patchW, patchH)
		t.Logf("%s target_patch=(%d,%d)-(%d,%d) mse=%.2f", label, patchX, patchY, patchX+patchW, patchY+patchH, mse)
	}
}

func debugAllTUsZero(tus []debugLumaTURecord) bool {
	if len(tus) == 0 {
		return false
	}
	for _, tu := range tus {
		for _, c := range tu.coeffs {
			if c != 0 {
				return false
			}
		}
	}
	return true
}

func debugReplayAltTxTypes(tx TxfmSize) []TxType {
	info := TxfmInfoFor(tx)
	candidates := []TxType{
		TxTypeDCTDCT,
		TxTypeADSTDCT,
		TxTypeDCTADST,
		TxTypeADSTADST,
		TxTypeVDCT,
		TxTypeHDCT,
	}
	out := make([]TxType, 0, len(candidates))
	for _, txType := range candidates {
		row, col, err := tx1DTypesFor(txType)
		if err != nil {
			continue
		}
		if !debugTxTypeFitsDim(row, int(info.W4)*4) || !debugTxTypeFitsDim(col, int(info.H4)*4) {
			continue
		}
		out = append(out, txType)
	}
	return out
}

func debugTxTypeFitsDim(kind tx1DKind, n int) bool {
	switch kind {
	case tx1DKindDCT, tx1DKindIdentity:
		return n == 4 || n == 8 || n == 16 || n == 32 || n == 64
	case tx1DKindADST, tx1DKindFlipADST:
		return n == 4 || n == 8 || n == 16
	default:
		return false
	}
}

func debugReplayBlockForcedTxType(hdr *obu.FrameHeader, syntax IntraBlockSyntax, prePlane []byte, planeWidth, planeHeight, stride, blockX, blockY int, predictFlags uint16, topHasRight, leftHasBottom bool, tus []debugLumaTURecord, forced TxType, t *testing.T) error {
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		return err
	}
	if len(ref) < planeWidth*planeHeight {
		return io.ErrUnexpectedEOF
	}
	refY := ref[:planeWidth*planeHeight]
	scratch := append([]byte(nil), prePlane...)
	blockW := syntax.Geometry.End4X*4 - syntax.Geometry.Start4X*4
	blockH := syntax.Geometry.End4Y*4 - syntax.Geometry.Start4Y*4
	if syntax.PalYSize == 0 {
		if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
			scratch,
			stride,
			planeWidth,
			planeHeight,
			blockX,
			blockY,
			blockW,
			blockH,
			syntax.YMode,
			syntax.YAngle,
			predictFlags,
			blockY > 0,
			blockX > 0,
			topHasRight,
			leftHasBottom,
		); err != nil {
			return err
		}
	}
	for _, tuRec := range tus {
		px := blockX + tuRec.offX4*4
		py := blockY + tuRec.offY4*4
		var err error
		if syntax.PalYSize != 0 {
			err = ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, forced, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
		} else {
			if !tuRec.tuSkip {
				err = InverseAddResidualTx8At(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, forced, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment])
			}
		}
		if err != nil {
			return err
		}
	}
	mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
	name := fmt.Sprintf("force_tx_%d", forced)
	t.Logf("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
	appendDebugTUTrace("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
	return nil
}

func debugReplayBlockForcedMode(hdr *obu.FrameHeader, syntax IntraBlockSyntax, prePlane []byte, planeWidth, planeHeight, stride, blockX, blockY int, predictFlags uint16, topHasRight, leftHasBottom bool, tus []debugLumaTURecord, mode IntraPredMode, t *testing.T) error {
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		return err
	}
	if len(ref) < planeWidth*planeHeight {
		return io.ErrUnexpectedEOF
	}
	refY := ref[:planeWidth*planeHeight]
	scratch := append([]byte(nil), prePlane...)
	blockW := syntax.Geometry.End4X*4 - syntax.Geometry.Start4X*4
	blockH := syntax.Geometry.End4Y*4 - syntax.Geometry.Start4Y*4
	if syntax.PalYSize == 0 {
		if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
			scratch,
			stride,
			planeWidth,
			planeHeight,
			blockX,
			blockY,
			blockW,
			blockH,
			mode,
			0,
			predictFlags,
			blockY > 0,
			blockX > 0,
			topHasRight,
			leftHasBottom,
		); err != nil {
			return err
		}
	}
	for _, tuRec := range tus {
		px := blockX + tuRec.offX4*4
		py := blockY + tuRec.offY4*4
		var err error
		if syntax.PalYSize != 0 {
			err = ReconstructPaletteLumaCoefficients8WithTxType(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
		} else {
			if !tuRec.tuSkip {
				err = InverseAddResidualTx8At(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment])
			}
		}
		if err != nil {
			return err
		}
	}
	mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
	name := fmt.Sprintf("force_mode_%d", mode)
	t.Logf("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
	appendDebugTUTrace("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
	return nil
}

func debugReplayBlockForcedFilterIntra(hdr *obu.FrameHeader, syntax IntraBlockSyntax, prePlane []byte, planeWidth, planeHeight, stride, blockX, blockY int, topHasRight, leftHasBottom bool, tus []debugLumaTURecord, filterIdx int8, t *testing.T) error {
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		return err
	}
	if len(ref) < planeWidth*planeHeight {
		return io.ErrUnexpectedEOF
	}
	refY := ref[:planeWidth*planeHeight]
	scratch := append([]byte(nil), prePlane...)
	blockW := syntax.Geometry.End4X*4 - syntax.Geometry.Start4X*4
	blockH := syntax.Geometry.End4Y*4 - syntax.Geometry.Start4Y*4
	if syntax.PalYSize == 0 {
		if err := PredictIntraLumaBlock8WithFlagsNeighborEdges(
			scratch,
			stride,
			planeWidth,
			planeHeight,
			blockX,
			blockY,
			blockW,
			blockH,
			IntraPredFilter,
			filterIdx,
			0,
			blockY > 0,
			blockX > 0,
			topHasRight,
			leftHasBottom,
		); err != nil {
			return err
		}
	}
	for _, tuRec := range tus {
		px := blockX + tuRec.offX4*4
		py := blockY + tuRec.offY4*4
		if tuRec.tuSkip {
			continue
		}
		if err := InverseAddResidualTx8At(scratch, stride, planeWidth, planeHeight, px, py, syntax.TX, tuRec.txType, tuRec.coeffs, hdr.Segmentation.Lossless[syntax.Segment]); err != nil {
			return err
		}
	}
	mse := compareBlockToRefY(scratch, refY, stride, blockX, blockY, minInt(blockW, 16), minInt(blockH, 16))
	name := fmt.Sprintf("force_filter_%d", filterIdx)
	t.Logf("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
	appendDebugTUTrace("%s replay block (%d,%d)-(%d,%d) mse=%.2f", name, blockX, blockY, blockX+blockW, blockY+blockH, mse)
	return nil
}

func coeffDC(coeffs []int32) int32 {
	if len(coeffs) == 0 {
		return 0
	}
	return coeffs[0]
}

func coeffNonZero(coeffs []int32) int {
	n := 0
	for _, c := range coeffs {
		if c != 0 {
			n++
		}
	}
	return n
}

func coeffMaxAbs(coeffs []int32) int32 {
	var m int32
	for _, c := range coeffs {
		if c < 0 {
			c = -c
		}
		if c > m {
			m = c
		}
	}
	return m
}

func clearBlockY(plane []byte, stride, x, y, width, height int) {
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			plane[(y+row)*stride+x+col] = 0
		}
	}
}

func compareBlockToRefY(ours, ref []byte, stride, x, y, width, height int) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	sum := 0.0
	n := 0
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			d := int(ours[(y+row)*stride+x+col]) - int(ref[(y+row)*stride+x+col])
			sum += float64(d * d)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func debugPlaneRMSE(a, b []byte) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	sum := 0.0
	for i := range a {
		d := float64(int(a[i]) - int(b[i]))
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(a)))
}

func blockWidth4ForSize(size BlockSize) int {
	return int(BlockDims(size).W4)
}

func blockHeight4ForSize(size BlockSize) int {
	return int(BlockDims(size).H4)
}

func blockName(size BlockSize) string {
	switch size {
	case Block128x128:
		return "128x128"
	case Block128x64:
		return "128x64"
	case Block64x128:
		return "64x128"
	case Block64x64:
		return "64x64"
	case Block64x32:
		return "64x32"
	case Block64x16:
		return "64x16"
	case Block32x64:
		return "32x64"
	case Block32x32:
		return "32x32"
	case Block32x16:
		return "32x16"
	case Block32x8:
		return "32x8"
	case Block16x64:
		return "16x64"
	case Block16x32:
		return "16x32"
	case Block16x16:
		return "16x16"
	case Block16x8:
		return "16x8"
	case Block16x4:
		return "16x4"
	case Block8x32:
		return "8x32"
	case Block8x16:
		return "8x16"
	case Block8x8:
		return "8x8"
	case Block8x4:
		return "8x4"
	case Block4x16:
		return "4x16"
	case Block4x8:
		return "4x8"
	case Block4x4:
		return "4x4"
	default:
		return "unknown"
	}
}

func decodeKeyframeMain8StatsBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, stats *firstKeyStats, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			children := splitChildren(g)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8StatsBlock(seq, hdr, row, sb, child, dq, img, cdef, segState, stats, childEdges[i]); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildren(g, part)
			if err != nil {
				return err
			}
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8StatsBlock(seq, hdr, row, sb, child, dq, img, cdef, segState, stats, childEdges[i]); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		for i, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := decodeKeyframeMain8StatsLeaf(seq, hdr, row, sb, child, dq, img, cdef, segState, stats, childEdges[i]); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := decodeKeyframeMain8StatsLeaf(seq, hdr, row, sb, g, dq, img, cdef, segState, stats, edge); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain8StatsLeaf(seq, hdr, row, sb, child, dq, img, cdef, segState, stats, childEdges[i]); err != nil {
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

func decodeKeyframeMain8StatsLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, stats *firstKeyStats, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	haveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	haveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	segID := uint8(0)
	segPred := false
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, haveTop, haveLeft, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	_ = skipMode
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, haveTop, haveLeft, true, skip, row.Tile.Tile.Entropy)
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
	if err := readDeltaQAndLFAtSuperBlockStart(seq, hdr, row.Tile, g, skip); err != nil {
		return err
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, haveTop, haveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	syntax.Geometry = g
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	stats.blocks++
	if skip {
		stats.skips++
	}
	stats.yModes[syntax.YMode]++
	stats.uvModes[syntax.UVMode]++
	stats.tx[syntax.TX]++
	stats.uvTx[syntax.UVTX]++
	if syntax.YMode == IntraPredFilter {
		stats.filterIntra++
	}
	if syntax.PalYSize != 0 {
		stats.paletteY++
	}
	if syntax.PalUVSize != 0 {
		stats.paletteUV++
	}
	if syntax.UVMode == IntraPredCFL {
		stats.cfl++
	}
	if err := ReconstructParsedIntraLumaBlockAt8WithSkipEdges(seq, hdr, row.Tile.Tile.CDF, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), g.Start4X, g.Start4Y, skip, row.Tile.Tile.Entropy, edge.topHasRight, edge.leftHasBottom); err != nil {
		return err
	}
	chromaEdge := edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling())
	if err := reconstructParsedIntraChromaBlockAt8WithSkipEdges(seq, hdr, row.Tile.Tile.Coef, row.Tile.CurrentDQ, above, &row.Left, syntax, img.Y, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), img.Cb, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), img.Cr, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), row.Tile.Tile.Entropy, skip, chromaEdge.topHasRight, chromaEdge.leftHasBottom); err != nil {
		return err
	}
	return ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax)
}

func logSortedMap[T comparable](t *testing.T, label string, m map[T]int, name func(T) string) {
	type kv[T comparable] struct {
		key   T
		value int
	}
	items := make([]kv[T], 0, len(m))
	for k, v := range m {
		items = append(items, kv[T]{key: k, value: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].value == items[j].value {
			return name(items[i].key) < name(items[j].key)
		}
		return items[i].value > items[j].value
	})
	for _, item := range items {
		t.Logf("%s: %s=%d", label, name(item.key), item.value)
	}
}

func intraModeName(mode IntraPredMode) string {
	if mode == IntraPredFilter {
		return "FILTER"
	}
	switch mode {
	case IntraPredDC:
		return "DC"
	case IntraPredVertical:
		return "VERT"
	case IntraPredHorizontal:
		return "HOR"
	case IntraPredDiagDownLeft:
		return "D45"
	case IntraPredDiagDownRight:
		return "D135"
	case IntraPredVertRight:
		return "D113"
	case IntraPredHorDown:
		return "D157"
	case IntraPredHorUp:
		return "D203"
	case IntraPredVertLeft:
		return "D67"
	case IntraPredSmooth:
		return "SMOOTH"
	case IntraPredSmoothVertical:
		return "SMOOTH_V"
	case IntraPredSmoothHorizontal:
		return "SMOOTH_H"
	case IntraPredPaeth:
		return "PAETH"
	case IntraPredCFL:
		return "CFL"
	default:
		return "UNKNOWN"
	}
}

func txName(tx TxfmSize) string {
	switch tx {
	case TX4X4:
		return "4x4"
	case TX8X8:
		return "8x8"
	case TX16X16:
		return "16x16"
	case TX32X32:
		return "32x32"
	case TX64X64:
		return "64x64"
	case RTX4X8:
		return "4x8"
	case RTX8X4:
		return "8x4"
	case RTX8X16:
		return "8x16"
	case RTX16X8:
		return "16x8"
	case RTX16X32:
		return "16x32"
	case RTX32X16:
		return "32x16"
	case RTX32X64:
		return "32x64"
	case RTX64X32:
		return "64x32"
	case RTX4X16:
		return "4x16"
	case RTX16X4:
		return "16x4"
	case RTX8X32:
		return "8x32"
	case RTX32X8:
		return "32x8"
	case RTX16X64:
		return "16x64"
	case RTX64X16:
		return "64x16"
	default:
		return "?"
	}
}
