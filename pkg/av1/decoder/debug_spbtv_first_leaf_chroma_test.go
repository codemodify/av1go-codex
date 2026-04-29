//go:build av1debug

package decoder

import (
	"errors"
	"image"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugSPBTVFirstBlocksAgainstReference(t *testing.T) {
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

	frame, err := dec.NextFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame == nil {
		t.Fatal("expected decoded frame")
	}

	ySize := frame.Width * frame.Height
	uvWidth := frame.Width / 2
	uvHeight := frame.Height / 2
	uvSize := uvWidth * uvHeight
	if len(ref) < ySize+2*uvSize {
		t.Fatalf("reference frame too small: len=%d want>=%d", len(ref), ySize+2*uvSize)
	}
	refY := ref[:ySize]
	refU := ref[ySize : ySize+uvSize]
	refV := ref[ySize+uvSize : ySize+2*uvSize]

	t.Logf("first leaf Y mse=%.2f U mse=%.2f V mse=%.2f",
		compareBlockToRefY(frame.Y, refY, frame.YStride, 0, 0, 32, 32),
		compareBlockToRefY(frame.U, refU, frame.UStride, 0, 0, 16, 16),
		compareBlockToRefY(frame.V, refV, frame.VStride, 0, 0, 16, 16),
	)
	t.Logf("second block Y mse=%.2f U mse=%.2f V mse=%.2f",
		compareBlockToRefY(frame.Y, refY, frame.YStride, 32, 0, 32, 16),
		compareBlockToRefY(frame.U, refU, frame.UStride, 16, 0, 16, 8),
		compareBlockToRefY(frame.V, refV, frame.VStride, 16, 0, 16, 8),
	)
}

func TestDebugSPBTVFirstBlocksCDEFOrderComparison(t *testing.T) {
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

	targetParsed := debugTraceEnvInt("DEBUG_SPBTV_TARGET_PARSED", -1)
	var parsed *ParsedFrame
	for i := 0; ; i++ {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if targetParsed >= 0 {
			if i != targetParsed {
				continue
			}
			parsed = pf
			break
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
		t.Logf("cdefBefore=%v first leaf Y mse=%.2f U mse=%.2f V mse=%.2f",
			cdefBefore,
			compareBlockToRefY(img.Y, refY, img.YStride, 0, 0, 32, 32),
			compareBlockToRefY(img.Cb, refU, img.CStride, 0, 0, 16, 16),
			compareBlockToRefY(img.Cr, refV, img.CStride, 0, 0, 16, 16),
		)
		t.Logf("cdefBefore=%v second block Y mse=%.2f U mse=%.2f V mse=%.2f",
			cdefBefore,
			compareBlockToRefY(img.Y, refY, img.YStride, 32, 0, 32, 16),
			compareBlockToRefY(img.Cb, refU, img.CStride, 16, 0, 16, 8),
			compareBlockToRefY(img.Cr, refV, img.CStride, 16, 0, 16, 8),
		)
	}
}

func TestDebugSPBTVFirstLeafChromaOrderNextSyntaxCDEFBefore(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}

	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	targetParsed := debugTraceEnvInt("DEBUG_SPBTV_TARGET_PARSED", -1)
	var parsed *ParsedFrame
	for i := 0; ; i++ {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if targetParsed >= 0 {
			if i != targetParsed {
				continue
			}
			parsed = pf
			break
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	targetState := func() (*TileRowSyntaxState, *SegmentationState, debugTargetLeafAdvanceState) {
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
		state, err := debugAdvanceToTargetLeafCDEFBefore(dec.header, &parsed.Header, row, sb, root, segState, 0, 0, 32, 0)
		if err != nil {
			t.Fatal(err)
		}
		return row, segState, state
	}

	for _, tc := range []struct {
		name   string
		swapUV bool
	}{
		{name: "current", swapUV: false},
		{name: "swapped", swapUV: true},
	} {
		row, segState, state := targetState()
		firstSyntax := state.syntax
		if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &state.above, &state.left, firstSyntax, state.dec); err != nil {
			t.Fatalf("%s luma consume: %v", tc.name, err)
		}
		if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &state.above, &state.left, firstSyntax, state.dec, tc.swapUV); err != nil {
			t.Fatalf("%s chroma consume: %v", tc.name, err)
		}
		if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &state.above, &state.left, firstSyntax); err != nil {
			t.Fatalf("%s apply context: %v", tc.name, err)
		}
		cur, rng, cnt, pos := state.dec.DebugState()
		nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &state.above, &state.left, state.nextRoot, segState, state.dec.Clone(), 32, 0)
		if err != nil {
			t.Fatalf("%s next syntax: %v", tc.name, err)
		}
		t.Logf("%s first leaf y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s",
			tc.name,
			intraModeName(firstSyntax.YMode),
			intraModeName(firstSyntax.UVMode),
			firstSyntax.PalYSize,
			firstSyntax.PalUVSize,
			txName(firstSyntax.TX),
			txName(firstSyntax.UVTX),
		)
		t.Logf("%s next block y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s skip=%v",
			tc.name,
			intraModeName(nextSyntax.YMode),
			intraModeName(nextSyntax.UVMode),
			nextSyntax.PalYSize,
			nextSyntax.PalUVSize,
			txName(nextSyntax.TX),
			txName(nextSyntax.UVTX),
			nextSkip,
		)
		t.Logf("%s post-first-block state current=%d rng=%d cnt=%d pos=%d",
			tc.name, cur, rng, cnt, pos,
		)
	}

	row, segState, state := targetState()
	firstSyntax := state.syntax
	yWidth := parsed.Header.Width
	yHeight := parsed.Header.Height
	uvWidth := (parsed.Header.Width + 1) >> 1
	uvHeight := (parsed.Header.Height + 1) >> 1
	yPlane := make([]byte, yWidth*yHeight)
	uPlane := make([]byte, uvWidth*uvHeight)
	vPlane := make([]byte, uvWidth*uvHeight)
	if err := ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &state.above, &state.left, firstSyntax, yPlane, yWidth, yWidth, yHeight, firstSyntax.Geometry.Start4X, firstSyntax.Geometry.Start4Y, firstSyntax.Skip, state.dec, false, false, true, true); err != nil {
		t.Fatalf("production luma consume: %v", err)
	}
	if err := reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &state.above, &state.left, firstSyntax, yPlane, yWidth, yWidth, yHeight, uPlane, uvWidth, uvWidth, uvHeight, vPlane, uvWidth, uvWidth, uvHeight, state.dec, firstSyntax.Skip, false, false, true, true); err != nil {
		t.Fatalf("production chroma consume: %v", err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &state.above, &state.left, firstSyntax); err != nil {
		t.Fatalf("production apply context: %v", err)
	}
	cur, rng, cnt, pos := state.dec.DebugState()
	nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &state.above, &state.left, state.nextRoot, segState, state.dec.Clone(), 32, 0)
	if err != nil {
		t.Fatalf("production next syntax: %v", err)
	}
	t.Logf("production first leaf y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s",
		intraModeName(firstSyntax.YMode),
		intraModeName(firstSyntax.UVMode),
		firstSyntax.PalYSize,
		firstSyntax.PalUVSize,
		txName(firstSyntax.TX),
		txName(firstSyntax.UVTX),
	)
	t.Logf("production next block y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s skip=%v",
		intraModeName(nextSyntax.YMode),
		intraModeName(nextSyntax.UVMode),
		nextSyntax.PalYSize,
		nextSyntax.PalUVSize,
		txName(nextSyntax.TX),
		txName(nextSyntax.UVTX),
		nextSkip,
	)
	t.Logf("production post-first-block state current=%d rng=%d cnt=%d pos=%d",
		cur, rng, cnt, pos,
	)
}

func TestDebugSPBTVFirstBlocksFilterDelta(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}

	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	targetParsed := debugTraceEnvInt("DEBUG_SPBTV_TARGET_PARSED", -1)
	var parsed *ParsedFrame
	for i := 0; ; i++ {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if targetParsed >= 0 {
			if i != targetParsed {
				continue
			}
			parsed = pf
			break
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
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	if err := decodeKeyframeMain8DCLumaWithCDEFOrder(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, restoration, segState, true); err != nil {
		t.Fatal(err)
	}

	preY := append([]byte(nil), img.Y...)
	preU := append([]byte(nil), img.Cb...)
	preV := append([]byte(nil), img.Cr...)

	applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
	applyCDEF420_8(img, &parsed.Header, cdef)
	applyRestoration420_8(img, &parsed.Header, restoration)

	t.Logf("first leaf filter delta Y mse=%.2f U mse=%.2f V mse=%.2f",
		compareBlockToRefY(img.Y, preY, img.YStride, 0, 0, 32, 32),
		compareBlockToRefY(img.Cb, preU, img.CStride, 0, 0, 16, 16),
		compareBlockToRefY(img.Cr, preV, img.CStride, 0, 0, 16, 16),
	)
	t.Logf("second block filter delta Y mse=%.2f U mse=%.2f V mse=%.2f",
		compareBlockToRefY(img.Y, preY, img.YStride, 32, 0, 32, 16),
		compareBlockToRefY(img.Cb, preU, img.CStride, 16, 0, 16, 8),
		compareBlockToRefY(img.Cr, preV, img.CStride, 16, 0, 16, 8),
	)
}

func TestDebugSPBTVManualPipelineMatchesNextFrame(t *testing.T) {
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

	manual := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(manual.Cb, 128)
	fillUint8(manual.Cr, 128)
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	if err := decodeKeyframeMain8DCLumaWithCDEFOrder(dec.header, &parsed.Header, &parsed.TileGroup, dq, manual, cdef, edges, restoration, segState, true); err != nil {
		t.Fatal(err)
	}
	applySimpleLoopFilter420_8MaskedWithEdges(manual, &parsed.Header, edges, cdef)
	applyCDEF420_8(manual, &parsed.Header, cdef)
	applyRestoration420_8(manual, &parsed.Header, restoration)

	dec2, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec2.Close()
	frame, err := dec2.NextFrame()
	if err != nil {
		t.Fatal(err)
	}
	if frame == nil {
		t.Fatal("expected decoded frame")
	}

	t.Logf("manual-vs-next first leaf Y mse=%.2f U mse=%.2f V mse=%.2f",
		compareBlockToRefY(manual.Y, frame.Y, manual.YStride, 0, 0, 32, 32),
		compareBlockToRefY(manual.Cb, frame.U, manual.CStride, 0, 0, 16, 16),
		compareBlockToRefY(manual.Cr, frame.V, manual.CStride, 0, 0, 16, 16),
	)
	t.Logf("manual-vs-next second block Y mse=%.2f U mse=%.2f V mse=%.2f",
		compareBlockToRefY(manual.Y, frame.Y, manual.YStride, 32, 0, 32, 16),
		compareBlockToRefY(manual.Cb, frame.U, manual.CStride, 16, 0, 16, 8),
		compareBlockToRefY(manual.Cr, frame.V, manual.CStride, 16, 0, 16, 8),
	)
}

func TestDebugSPBTVLiveFirstLeavesCDEFBefore(t *testing.T) {
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

	if err := debugTraceLiveKeyframeFirstLeaves(t, dec, parsed, 3, true); err != nil {
		t.Fatal(err)
	}
}

func TestDebugSPBTVSecondLeafCDFVariantsCDEFBefore(t *testing.T) {
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

	if err := debugTraceLiveKeyframeFirstLeavesWithOptions(t, dec, parsed, 2, true, true); err != nil {
		t.Fatal(err)
	}
}

func TestDebugSPBTVPartitionTraceTopLeftCDEFBefore(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}

	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	targetParsed := debugTraceEnvInt("DEBUG_SPBTV_TARGET_PARSED", -1)
	var parsed *ParsedFrame
	for i := 0; ; i++ {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if targetParsed >= 0 {
			if i != targetParsed {
				continue
			}
			parsed = pf
			break
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	tracer := &debugLiveLeafTracer{
		t:                          t,
		maxLeaves:                  debugTraceEnvInt("DEBUG_PARTITION_TRACE_MAX_LEAVES", 12),
		tracePartitionTopLeft:      true,
		traceSecondLeafCDFVariants: os.Getenv("DEBUG_PARTITION_TRACE_SECOND_LEAF_CDF_VARIANTS") != "",
		partitionTraceMinX:         debugTraceEnvInt("DEBUG_PARTITION_TRACE_MIN_X", 0),
		partitionTraceMaxX:         debugTraceEnvInt("DEBUG_PARTITION_TRACE_MAX_X", 64),
		partitionTraceMinY:         debugTraceEnvInt("DEBUG_PARTITION_TRACE_MIN_Y", 0),
		partitionTraceMaxY:         debugTraceEnvInt("DEBUG_PARTITION_TRACE_MAX_Y", 32),
		limitLeavesToTraceWindow:   os.Getenv("DEBUG_PARTITION_TRACE_LIMIT_LEAVES_TO_WINDOW") != "",
	}
	if err := debugTraceLiveKeyframeFirstLeavesWithTracer(t, dec, parsed, true, tracer); err != nil {
		t.Fatal(err)
	}
}

var errDebugStopLiveLeafTrace = errors.New("debug stop after requested live leaves")

type debugLiveLeafTracer struct {
	t                          *testing.T
	maxLeaves                  int
	count                      int
	refY                       []byte
	refU                       []byte
	refV                       []byte
	traceSecondLeafCDFVariants bool
	tracePartitionTopLeft      bool
	partitionTraceMinX         int
	partitionTraceMaxX         int
	partitionTraceMinY         int
	partitionTraceMaxY         int
	limitLeavesToTraceWindow   bool
}

func debugTraceEnvInt(name string, fallback int) int {
	if value := os.Getenv(name); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func (tracer *debugLiveLeafTracer) inTraceWindow(x, y int) bool {
	if tracer == nil {
		return true
	}
	if x < tracer.partitionTraceMinX || y < tracer.partitionTraceMinY {
		return false
	}
	if tracer.partitionTraceMaxX > 0 && x >= tracer.partitionTraceMaxX {
		return false
	}
	if tracer.partitionTraceMaxY > 0 && y >= tracer.partitionTraceMaxY {
		return false
	}
	return true
}

func debugTraceLiveKeyframeFirstLeaves(t *testing.T, dec *Decoder, parsed *ParsedFrame, maxLeaves int, cdefBeforeSyntax bool) error {
	return debugTraceLiveKeyframeFirstLeavesWithOptions(t, dec, parsed, maxLeaves, cdefBeforeSyntax, false)
}

func debugTraceLiveKeyframeFirstLeavesWithOptions(t *testing.T, dec *Decoder, parsed *ParsedFrame, maxLeaves int, cdefBeforeSyntax bool, traceSecondLeafCDFVariants bool) error {
	tracer := &debugLiveLeafTracer{
		t:                          t,
		maxLeaves:                  maxLeaves,
		traceSecondLeafCDFVariants: traceSecondLeafCDFVariants,
	}
	return debugTraceLiveKeyframeFirstLeavesWithTracer(t, dec, parsed, cdefBeforeSyntax, tracer)
}

func debugTraceLiveKeyframeFirstLeavesWithTracer(t *testing.T, dec *Decoder, parsed *ParsedFrame, cdefBeforeSyntax bool, tracer *debugLiveLeafTracer) error {
	t.Helper()
	if dec == nil || parsed == nil {
		return io.ErrUnexpectedEOF
	}
	if tracer == nil {
		tracer = &debugLiveLeafTracer{}
	}
	if tracer.t == nil {
		tracer.t = t
	}

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	restoration := NewRestorationState(dec.header, &parsed.Header)
	_ = edges

	if ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv"); err == nil {
		ySize := parsed.Header.Width * parsed.Header.Height
		uvW := (parsed.Header.Width + 1) >> 1
		uvH := (parsed.Header.Height + 1) >> 1
		uvSize := uvW * uvH
		if len(ref) >= ySize+2*uvSize {
			tracer.refY = ref[:ySize]
			tracer.refU = ref[ySize : ySize+uvSize]
			tracer.refV = ref[ySize+uvSize : ySize+2*uvSize]
		}
	}

	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	if len(states) == 0 {
		return io.ErrUnexpectedEOF
	}

	for _, tile := range states {
		syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, tile, 0)
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
				if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
					return err
				}
				root := BlockGeometry{
					Level:   RootBlockLevel(dec.header),
					Size:    RootBlockSize(dec.header),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}
				err = debugTraceLiveKeyframeLeavesBlock(dec.header, &parsed.Header, &row, sb, root, img, cdef, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root), cdefBeforeSyntax, tracer)
				if errors.Is(err, errDebugStopLiveLeafTrace) {
					return nil
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func debugTraceLiveKeyframeLeavesBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability, cdefBeforeSyntax bool, tracer *debugLiveLeafTracer) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	tracePartition := false
	partCtx := 0
	partCDF := []uint16(nil)
	var partPreCur, partPreRng uint32
	partPreCnt, partPrePos := 0, 0
	partRawAbove, partRawLeft := uint8(0), uint8(0)
	if tracer != nil && tracer.tracePartitionTopLeft && tracer.inTraceWindow(g.Start4X*4, g.Start4Y*4) {
		bx8 := local.Start4X >> 1
		by8 := local.Start4Y >> 1
		partCtx = GetPartitionCtx(above, &row.Left, local.Level, by8, bx8)
		partCDF = append([]uint16(nil), row.Tile.Tile.CDF.PartitionCDF(local.Level, partCtx)...)
		partPreCur, partPreRng, partPreCnt, partPrePos = row.Tile.Tile.Entropy.DebugState()
		partRawAbove = above.Partition[bx8]
		partRawLeft = row.Left.Partition[by8]
		tracePartition = true
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if tracePartition {
		partPostCur, partPostRng, partPostCnt, partPostPos := row.Tile.Tile.Entropy.DebugState()
		tracer.t.Logf("part block=(%d,%d)-(%d,%d) size=%s level=%d ctx=%d rawAbove=%#02x rawLeft=%#02x pre=(%d,%d,%d,%d) part=%s post=(%d,%d,%d,%d) cdf=%v",
			g.Start4X*4,
			g.Start4Y*4,
			g.End4X*4,
			g.End4Y*4,
			blockName(g.Size),
			g.Level,
			partCtx,
			partRawAbove,
			partRawLeft,
			partPreCur,
			partPreRng,
			partPreCnt,
			partPrePos,
			debugPartitionName(part),
			partPostCur,
			partPostRng,
			partPostCnt,
			partPostPos,
			partCDF,
		)
	}
	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for i, child := range splitChildren(g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugTraceLiveKeyframeLeavesBlock(seq, hdr, row, sb, child, img, cdef, segState, childEdges[i], cdefBeforeSyntax, tracer); err != nil {
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
				if err := debugTraceLiveKeyframeLeavesBlock(seq, hdr, row, sb, child, img, cdef, segState, childEdges[i], cdefBeforeSyntax, tracer); err != nil {
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
			if err := debugTraceLiveKeyframeLeavesLeaf(seq, hdr, row, sb, child, img, cdef, segState, childEdges[i], cdefBeforeSyntax, tracer); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugTraceLiveKeyframeLeavesLeaf(seq, hdr, row, sb, g, img, cdef, segState, edge, cdefBeforeSyntax, tracer); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugTraceLiveKeyframeLeavesLeaf(seq, hdr, row, sb, child, img, cdef, segState, childEdges[i], cdefBeforeSyntax, tracer); err != nil {
					return err
				}
			}
		}
	}
	if part != PartitionSplit || g.Level == BlockLevel8x8 {
		if err := ApplyPartitionContext(above, &row.Left, local, part); err != nil {
			return err
		}
		if tracePartition {
			bx8 := local.Start4X >> 1
			by8 := local.Start4Y >> 1
			tracer.t.Logf("part apply block=(%d,%d)-(%d,%d) part=%s above[%d]=%#02x left[%d]=%#02x",
				g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, debugPartitionName(part),
				bx8, above.Partition[bx8], by8, row.Left.Partition[by8])
		}
	}
	return nil
}

func debugTraceLiveKeyframeLeavesLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, img *image.YCbCr, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability, cdefBeforeSyntax bool, tracer *debugLiveLeafTracer) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
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
	if cdefBeforeSyntax {
		if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
			return err
		}
		cdef.markNoSkip(g, skip)
	}
	if err := readDeltaQAndLF(seq, hdr, row.Tile, g, skip); err != nil {
		return err
	}
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	preCur, preRng, preCnt, prePos := row.Tile.Tile.Entropy.DebugState()
	cache, nCache := paletteCache(above, &row.Left, 0, bx4, by4)
	topMode := IntraPredMode(above.Mode[bx4])
	leftMode := IntraPredMode(row.Left.Mode[by4])
	if tracer != nil && tracer.traceSecondLeafCDFVariants && tracer.count == 1 && cdefBeforeSyntax {
		debugTraceSecondLeafCDFVariants(seq, hdr, row, local, g, img, above, &row.Left, edge, ctxHaveTop, ctxHaveLeft, reconHaveTop, reconHaveLeft, segID, segPred, skipMode, skip, tracer)
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, ctxHaveTop, ctxHaveLeft, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	syntax.Geometry = g
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	synCur, synRng, synCnt, synPos := row.Tile.Tile.Entropy.DebugState()
	if !cdefBeforeSyntax {
		if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, skip, row.Tile.Tile.Entropy); err != nil {
			return err
		}
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
			reconHaveTop,
			reconHaveLeft,
			edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling()).topHasRight,
			edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling()).leftHasBottom,
		); err != nil {
			return err
		}
	}
	postCur, postRng, postCnt, postPos := row.Tile.Tile.Entropy.DebugState()
	syntax.PreserveTXContext = !skip
	if err := ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax); err != nil {
		return err
	}

	x := g.Start4X * 4
	y := g.Start4Y * 4
	w := g.End4X*4 - x
	h := g.End4Y*4 - y
	traceLeaf := tracer == nil || !tracer.limitLeavesToTraceWindow || tracer.inTraceWindow(x, y)
	if traceLeaf {
		tracer.count++
		tracer.t.Logf("live leaf #%d block=(%d,%d)-(%d,%d) size=%s skip=%v qidx=%d topMode=%s leftMode=%s topCtx=%d leftCtx=%d cache=%v pre=(%d,%d,%d,%d) syntax=y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s afterSyntax=(%d,%d,%d,%d) post=(%d,%d,%d,%d)",
			tracer.count,
			x, y, g.End4X*4, g.End4Y*4,
			blockName(g.Size),
			skip,
			row.Tile.LastQIdx,
			intraModeName(topMode),
			intraModeName(leftMode),
			generatedIntraModeContext[above.Mode[bx4]],
			generatedIntraModeContext[row.Left.Mode[by4]],
			append([]uint16(nil), cache[:nCache]...),
			preCur, preRng, preCnt, prePos,
			intraModeName(syntax.YMode),
			intraModeName(syntax.UVMode),
			syntax.PalYSize,
			syntax.PalUVSize,
			txName(syntax.TX),
			txName(syntax.UVTX),
			synCur, synRng, synCnt, synPos,
			postCur, postRng, postCnt, postPos,
		)
		if syntax.PalYSize != 0 {
			head := minInt(16, len(syntax.PalYIdx))
			tracer.t.Logf("live leaf #%d palY=%v idx0=%v", tracer.count, syntax.PalY[:syntax.PalYSize], syntax.PalYIdx[:head])
		}
		if syntax.PalUVSize != 0 {
			head := minInt(16, len(syntax.PalUVIdx))
			tracer.t.Logf("live leaf #%d palU=%v palV=%v idx0=%v", tracer.count, syntax.PalU[:syntax.PalUVSize], syntax.PalV[:syntax.PalUVSize], syntax.PalUVIdx[:head])
		}
		if len(tracer.refY) >= parsedPlaneWidth(img)*parsedPlaneHeight(img) {
			visibleW := minInt(w, parsedPlaneWidth(img)-x)
			visibleH := minInt(h, parsedPlaneHeight(img)-y)
			tracer.t.Logf("live leaf #%d Y mse full=%.2f top=%.2f bottom=%.2f",
				tracer.count,
				compareBlockToRefY(img.Y, tracer.refY, img.YStride, x, y, visibleW, visibleH),
				compareBlockToRefY(img.Y, tracer.refY, img.YStride, x, y, visibleW, minInt(visibleH, 16)),
				compareBlockToRefY(img.Y, tracer.refY, img.YStride, x, y+maxIntLocal(0, visibleH-16), visibleW, minInt(visibleH, 16)),
			)
			tracer.t.Logf("live leaf #%d refY values=%v", tracer.count, debugBlockValueSet(tracer.refY, parsedPlaneWidth(img), x, y, visibleW, visibleH, 12))
		}
		if len(tracer.refU) >= parsedChromaPlaneWidth(img)*parsedChromaPlaneHeight(img) && len(tracer.refV) >= parsedChromaPlaneWidth(img)*parsedChromaPlaneHeight(img) {
			cx := x >> 1
			cy := y >> 1
			cw := minInt((w+1)>>1, parsedChromaPlaneWidth(img)-cx)
			ch := minInt((h+1)>>1, parsedChromaPlaneHeight(img)-cy)
			tracer.t.Logf("live leaf #%d U mse=%.2f V mse=%.2f",
				tracer.count,
				compareBlockToRefY(img.Cb, tracer.refU, img.CStride, cx, cy, cw, ch),
				compareBlockToRefY(img.Cr, tracer.refV, img.CStride, cx, cy, cw, ch),
			)
		}
		if tracer.maxLeaves > 0 && tracer.count >= tracer.maxLeaves {
			return errDebugStopLiveLeafTrace
		}
	}
	return nil
}

func debugTraceSecondLeafCDFVariants(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, local, g BlockGeometry, img *image.YCbCr, above, left *BlockContext, edge intraEdgeAvailability, ctxHaveTop, ctxHaveLeft, reconHaveTop, reconHaveLeft bool, segID uint8, segPred, skipMode, skip bool, tracer *debugLiveLeafTracer) {
	if tracer == nil || tracer.t == nil || hdr == nil || row == nil || img == nil || above == nil || left == nil || row.Tile.Tile.Entropy == nil || row.Tile.Tile.CDF == nil || row.Tile.Tile.Coef == nil {
		return
	}

	variants := []struct {
		name  string
		build func(*ModeCDF) *ModeCDF
	}{
		{
			name: "current",
			build: func(base *ModeCDF) *ModeCDF {
				return base.Clone()
			},
		},
		{
			name: "default_all",
			build: func(base *ModeCDF) *ModeCDF {
				return NewDefaultModeCDF()
			},
		},
		{
			name: "default_y_mode",
			build: func(base *ModeCDF) *ModeCDF {
				clone := base.Clone()
				clone.YMode = defaultModeCDFValue.YMode
				clone.KFYMode = defaultModeCDFValue.KFYMode
				return clone
			},
		},
		{
			name: "default_uv_mode",
			build: func(base *ModeCDF) *ModeCDF {
				clone := base.Clone()
				clone.UVMode = defaultModeCDFValue.UVMode
				clone.CFLSign = defaultModeCDFValue.CFLSign
				clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
				clone.AngleDelta = defaultModeCDFValue.AngleDelta
				return clone
			},
		},
		{
			name: "default_filter_intra",
			build: func(base *ModeCDF) *ModeCDF {
				clone := base.Clone()
				clone.UseFilterIntra = defaultModeCDFValue.UseFilterIntra
				clone.FilterIntra = defaultModeCDFValue.FilterIntra
				return clone
			},
		},
		{
			name: "default_palette_flags",
			build: func(base *ModeCDF) *ModeCDF {
				clone := base.Clone()
				clone.PalY = defaultModeCDFValue.PalY
				clone.PalSize = defaultModeCDFValue.PalSize
				clone.PalUV = defaultModeCDFValue.PalUV
				return clone
			},
		},
		{
			name: "default_color_map",
			build: func(base *ModeCDF) *ModeCDF {
				clone := base.Clone()
				clone.ColorMap = defaultModeCDFValue.ColorMap
				return clone
			},
		},
	}

	x := g.Start4X * 4
	y := g.Start4Y * 4
	w := g.End4X*4 - x
	h := g.End4Y*4 - y
	preDec := row.Tile.Tile.Entropy.Clone()
	for _, variant := range variants {
		modeCDF := variant.build(row.Tile.Tile.CDF)
		coefCDF := row.Tile.Tile.Coef.Clone()
		aboveClone := *above
		leftClone := *left
		decClone := preDec.Clone()
		imgClone := debugCloneYCbCr(img)
		syntax, err := ReadIntraBlockSyntax(seq, hdr, modeCDF, &aboveClone, &leftClone, local, ctxHaveTop, ctxHaveLeft, segID, msacEntropyDecoder{ctx: decClone})
		if err != nil {
			tracer.t.Logf("leaf2 cdf_variant=%s syntax err=%v", variant.name, err)
			continue
		}
		syntax.Geometry = g
		syntax.SegPred = segPred
		syntax.SkipMode = skipMode
		syntax.Skip = skip
		synCur, synRng, synCnt, synPos := decClone.DebugState()
		if !skip && shouldUseDav1dIntraCoeffOrder(seq, syntax) {
			if _, err := reconstructParsedIntraBlockAt8Dav1dOrder(
				seq,
				hdr,
				modeCDF,
				coefCDF,
				row.Tile.CurrentDQ,
				&aboveClone,
				&leftClone,
				syntax,
				imgClone.Y,
				imgClone.YStride,
				parsedPlaneWidth(imgClone),
				parsedPlaneHeight(imgClone),
				imgClone.Cb,
				imgClone.CStride,
				parsedChromaPlaneWidth(imgClone),
				parsedChromaPlaneHeight(imgClone),
				imgClone.Cr,
				imgClone.CStride,
				parsedChromaPlaneWidth(imgClone),
				parsedChromaPlaneHeight(imgClone),
				decClone,
				reconHaveTop,
				reconHaveLeft,
				edge,
			); err != nil {
				tracer.t.Logf("leaf2 cdf_variant=%s dav1d-order err=%v", variant.name, err)
				continue
			}
		} else {
			if err := ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(
				seq,
				hdr,
				modeCDF,
				coefCDF,
				row.Tile.CurrentDQ,
				&aboveClone,
				&leftClone,
				syntax,
				imgClone.Y,
				imgClone.YStride,
				parsedPlaneWidth(imgClone),
				parsedPlaneHeight(imgClone),
				g.Start4X,
				g.Start4Y,
				skip,
				decClone,
				reconHaveTop,
				reconHaveLeft,
				edge.topHasRight,
				edge.leftHasBottom,
			); err != nil {
				tracer.t.Logf("leaf2 cdf_variant=%s luma err=%v", variant.name, err)
				continue
			}
			if err := reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(
				seq,
				hdr,
				coefCDF,
				row.Tile.CurrentDQ,
				&aboveClone,
				&leftClone,
				syntax,
				imgClone.Y,
				imgClone.YStride,
				parsedPlaneWidth(imgClone),
				parsedPlaneHeight(imgClone),
				imgClone.Cb,
				imgClone.CStride,
				parsedChromaPlaneWidth(imgClone),
				parsedChromaPlaneHeight(imgClone),
				imgClone.Cr,
				imgClone.CStride,
				parsedChromaPlaneWidth(imgClone),
				parsedChromaPlaneHeight(imgClone),
				decClone,
				skip,
				reconHaveTop,
				reconHaveLeft,
				edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling()).topHasRight,
				edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling()).leftHasBottom,
			); err != nil {
				tracer.t.Logf("leaf2 cdf_variant=%s chroma err=%v", variant.name, err)
				continue
			}
		}
		postCur, postRng, postCnt, postPos := decClone.DebugState()
		yMSE := 0.0
		uMSE := 0.0
		vMSE := 0.0
		if len(tracer.refY) >= parsedPlaneWidth(imgClone)*parsedPlaneHeight(imgClone) {
			yMSE = compareBlockToRefY(imgClone.Y, tracer.refY, imgClone.YStride, x, y, w, h)
		}
		if len(tracer.refU) >= parsedChromaPlaneWidth(imgClone)*parsedChromaPlaneHeight(imgClone) && len(tracer.refV) >= parsedChromaPlaneWidth(imgClone)*parsedChromaPlaneHeight(imgClone) {
			cx := x >> 1
			cy := y >> 1
			cw := (w + 1) >> 1
			ch := (h + 1) >> 1
			uMSE = compareBlockToRefY(imgClone.Cb, tracer.refU, imgClone.CStride, cx, cy, cw, ch)
			vMSE = compareBlockToRefY(imgClone.Cr, tracer.refV, imgClone.CStride, cx, cy, cw, ch)
		}
		tracer.t.Logf("leaf2 cdf_variant=%s syntax=y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s afterSyntax=(%d,%d,%d,%d) post=(%d,%d,%d,%d) mse=Y %.2f U %.2f V %.2f",
			variant.name,
			intraModeName(syntax.YMode),
			intraModeName(syntax.UVMode),
			syntax.PalYSize,
			syntax.PalUVSize,
			txName(syntax.TX),
			txName(syntax.UVTX),
			synCur, synRng, synCnt, synPos,
			postCur, postRng, postCnt, postPos,
			yMSE, uMSE, vMSE,
		)
		if syntax.PalYSize != 0 {
			head := minInt(16, len(syntax.PalYIdx))
			tracer.t.Logf("leaf2 cdf_variant=%s palY=%v idx0=%v", variant.name, syntax.PalY[:syntax.PalYSize], syntax.PalYIdx[:head])
		}
		if syntax.PalUVSize != 0 {
			head := minInt(16, len(syntax.PalUVIdx))
			tracer.t.Logf("leaf2 cdf_variant=%s palU=%v palV=%v idx0=%v", variant.name, syntax.PalU[:syntax.PalUVSize], syntax.PalV[:syntax.PalUVSize], syntax.PalUVIdx[:head])
		}
	}
}

func debugCloneYCbCr(src *image.YCbCr) *image.YCbCr {
	if src == nil {
		return nil
	}
	clone := image.NewYCbCr(src.Rect, src.SubsampleRatio)
	copy(clone.Y, src.Y)
	copy(clone.Cb, src.Cb)
	copy(clone.Cr, src.Cr)
	return clone
}

func debugPartitionName(part BlockPartition) string {
	switch part {
	case PartitionNone:
		return "none"
	case PartitionH:
		return "h"
	case PartitionV:
		return "v"
	case PartitionSplit:
		return "split"
	case PartitionTTopSplit:
		return "t_top"
	case PartitionTBottomSplit:
		return "t_bottom"
	case PartitionTLeftSplit:
		return "t_left"
	case PartitionTRightSplit:
		return "t_right"
	case PartitionH4:
		return "h4"
	case PartitionV4:
		return "v4"
	default:
		return "unknown"
	}
}

func debugBlockValueSet(plane []byte, stride, x, y, width, height, limit int) []uint16 {
	if limit <= 0 {
		return nil
	}
	seen := make(map[byte]struct{}, limit)
	out := make([]uint16, 0, limit)
	for py := 0; py < height && y+py >= 0 && y+py < len(plane)/stride; py++ {
		row := plane[(y+py)*stride:]
		for px := 0; px < width && x+px >= 0 && x+px < stride; px++ {
			v := row[x+px]
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, uint16(v))
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}
