package decoder

import (
	"fmt"
	"image"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugReferencePipelineGeneratedMain8(t *testing.T) {
	if os.Getenv("DEBUG_REFERENCE_PIPELINE") == "" {
		t.Skip("debug only")
	}

	opts := testutil.AV1FixtureOptions{
		Name:     "debug-reference-main8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   6,
		BitDepth: 8,
	}
	if strings.TrimSpace(os.Getenv("DEBUG_REFERENCE_BIT_DEPTH")) == "10" {
		opts = testutil.AV1FixtureOptions{
			Name:     "debug-reference-main10.mp4",
			Width:    128,
			Height:   72,
			FPS:      4,
			Frames:   6,
			BitDepth: 10,
		}
	}
	path := strings.TrimSpace(os.Getenv("DEBUG_REFERENCE_INPUT"))
	if path == "" {
		fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), opts)
		path = fixture.Path
	}
	refFrames := testutil.DecodeReferenceFrames420(t, path, 1)
	if len(refFrames) != 1 {
		t.Fatalf("reference frames=%d, want 1", len(refFrames))
	}
	ref := refFrames[0]

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
	if os.Getenv("DEBUG_REFERENCE_DISABLE_CDF_UPDATE") != "" {
		parsed.Header.DisableCDFUpdate = true
		t.Logf("forcing DisableCDFUpdate for debug run")
	}
	if os.Getenv("DEBUG_REFERENCE_DISABLE_DELTA_Q") != "" {
		parsed.Header.Delta.QPresent = false
		t.Logf("forcing Delta.QPresent=false for debug run")
	}
	t.Logf("header frame_type=%d txfm_mode=%d reduced_txtp=%v allow_screen_tools=%v allow_intrabc=%v all_lossless=%v seg_enabled=%v delta_q=%v delta_lf=%v yac=%d qm=%v qmy=%d qmu=%d qmv=%d",
		parsed.Header.FrameType,
		parsed.Header.TxfmMode,
		parsed.Header.ReducedTxtpSet,
		parsed.Header.AllowScreenTools,
		parsed.Header.AllowIntrabc,
		parsed.Header.AllLossless,
		parsed.Header.Segmentation.Enabled,
		parsed.Header.Delta.QPresent,
		parsed.Header.Delta.LFPresent,
		parsed.Header.Quantization.YAC,
		parsed.Header.Quantization.QM,
		parsed.Header.Quantization.QMY,
		parsed.Header.Quantization.QMU,
		parsed.Header.Quantization.QMV,
	)

	if ref.BitDepth > 8 {
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
			t.Fatal("Main10 debug pipeline does not support intraBC fixtures")
		}
		if err := decodeKeyframeMain10(dec.header, &parsed.Header, &parsed.TileGroup, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, cdef, restoration, segState); err != nil {
			t.Fatalf("decodeKeyframeMain10: %v", err)
		}
		logReferencePipelineMetrics16(t, "raw", yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, ref)
		applySimpleLoopFilter420_16Masked(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, dec.header.ColorConfig.BitDepth, cdef)
		logReferencePipelineMetrics16(t, "loop", yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, ref)
		applyCDEF420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, dec.header.ColorConfig.BitDepth, cdef)
		logReferencePipelineMetrics16(t, "cdef", yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, ref)
		applyRestoration420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, restoration, dec.header.ColorConfig.BitDepth)
		logReferencePipelineMetrics16(t, "restoration", yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, ref)
	} else {
		img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
		fillUint8(img.Cb, 128)
		fillUint8(img.Cr, 128)
		dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
		cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
		edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
		restoration := NewRestorationState(dec.header, &parsed.Header)
		segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
		cdefBeforeSyntax := os.Getenv("DEBUG_REFERENCE_CDEF_AFTER") == ""

		if err := decodeKeyframeMain8DCLumaWithCDEFOrder(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, restoration, segState, cdefBeforeSyntax); err != nil {
			t.Fatalf("decodeKeyframeMain8DCLumaWithCDEFOrder: %v", err)
		}
		logReferencePipelineMetrics(t, "raw", img, ref)
		applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
		logReferencePipelineMetrics(t, "loop", img, ref)
		applyCDEF420_8(img, &parsed.Header, cdef)
		logReferencePipelineMetrics(t, "cdef", img, ref)
		applyRestoration420_8(img, &parsed.Header, restoration)
		logReferencePipelineMetrics(t, "restoration", img, ref)
	}

	if os.Getenv("DEBUG_REFERENCE_FIRST_BLOCK") != "" {
		debugTraceReferenceLeaf(t, dec, parsed, ref, 0, 0, 32, 0, "first leaf")
	}
	if spec := strings.TrimSpace(os.Getenv("DEBUG_REFERENCE_TARGET_BLOCK")); spec != "" {
		targetX, targetY, err := parseDebugReferenceTargetBlock(spec)
		if err != nil {
			t.Fatalf("parse DEBUG_REFERENCE_TARGET_BLOCK: %v", err)
		}
		nextX := minInt(targetX+4, parsed.Header.Width-1)
		debugTraceReferenceLeaf(t, dec, parsed, ref, targetX, targetY, nextX, targetY, "target leaf")
	}
}

func TestDebugReferencePipelineGeneratedMain8VariantDiff(t *testing.T) {
	if os.Getenv("DEBUG_REFERENCE_VARIANT_DIFF") == "" {
		t.Skip("debug only")
	}

	opts := testutil.AV1FixtureOptions{
		Name:     "debug-reference-variant-diff.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   6,
		BitDepth: 8,
	}
	if strings.TrimSpace(os.Getenv("DEBUG_REFERENCE_BIT_DEPTH")) == "10" {
		opts = testutil.AV1FixtureOptions{
			Name:     "debug-reference-variant-diff-main10.mp4",
			Width:    128,
			Height:   72,
			FPS:      4,
			Frames:   6,
			BitDepth: 10,
		}
	}
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), opts)
	variantEnv := strings.TrimSpace(os.Getenv("DEBUG_REFERENCE_VARIANT_ENV"))
	if variantEnv == "" {
		variantEnv = "DEBUG_COEF_BR_ROW"
	}
	variantValue := strings.TrimSpace(os.Getenv("DEBUG_REFERENCE_VARIANT_VALUE"))
	if variantValue == "" {
		variantValue = "1"
	}
	label := fmt.Sprintf("variant-%s=%s", variantEnv, variantValue)

	if opts.BitDepth > 8 {
		baseStages := debugDecodeReferenceVariantStages10(t, fixture.Path, variantEnv, variantValue, false)
		variantStages := debugDecodeReferenceVariantStages10(t, fixture.Path, variantEnv, variantValue, true)
		for i := range baseStages {
			stage := baseStages[i]
			logMovieMismatchSummary16(t, label+"-"+stage.label, variantStages[i].plane, variantStages[i].stride, stage.plane, stage.width, stage.height)
		}
		return
	}

	decode := func(applyVariant bool) *Frame {
		restore := debugReferenceEnvOverride(variantEnv, "")
		if applyVariant {
			restore = debugReferenceEnvOverride(variantEnv, variantValue)
		}
		defer restore()
		dec, err := OpenMP4(fixture.Path)
		if err != nil {
			t.Fatalf("OpenMP4(%v): %v", applyVariant, err)
		}
		defer dec.Close()
		frame, err := dec.NextFrame()
		if err == io.EOF {
			t.Fatal("no frame decoded")
		}
		if err != nil {
			t.Fatalf("NextFrame(%v): %v", applyVariant, err)
		}
		return frame
	}

	base := decode(false)
	defer base.Close()
	variant := decode(true)
	defer variant.Close()

	if base.BitDepth > 8 {
		baseY := flattenPlane16(base.Y16, base.YStride, base.Width, base.Height)
		logMovieMismatchSummary16(t, label, variant.Y16, variant.YStride, baseY, base.Width, base.Height)
		return
	}
	baseY := flattenPlane8(base.Y, base.YStride, base.Width, base.Height)
	logMovieMismatchSummary(t, label, variant.Y, variant.YStride, baseY, base.Width, base.Height)
}

type debugReferenceStage16 struct {
	label  string
	plane  []uint16
	stride int
	width  int
	height int
}

func debugDecodeReferenceVariantStages10(t *testing.T, path, variantEnv, variantValue string, applyVariant bool) []debugReferenceStage16 {
	t.Helper()
	restore := debugReferenceEnvOverride(variantEnv, "")
	if applyVariant {
		restore = debugReferenceEnvOverride(variantEnv, variantValue)
	}
	defer restore()

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4(%v): %v", applyVariant, err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatalf("NextParsedFrame(%v): %v", applyVariant, err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	width := parsed.Header.Width
	height := parsed.Header.Height
	yPlane := make([]uint16, width*height)
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	uPlane := make([]uint16, chromaWidth*chromaHeight)
	vPlane := make([]uint16, chromaWidth*chromaHeight)
	fillUint16(uPlane, 1<<(dec.header.ColorConfig.BitDepth-1))
	fillUint16(vPlane, 1<<(dec.header.ColorConfig.BitDepth-1))

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	if parsed.Header.AllowIntrabc {
		t.Fatal("debugDecodeReferenceVariantStages10 does not support intraBC fixtures")
	}
	if err := decodeKeyframeMain10(dec.header, &parsed.Header, &parsed.TileGroup, dq, yPlane, width, uPlane, chromaWidth, vPlane, chromaWidth, cdef, restoration, segState); err != nil {
		t.Fatalf("decodeKeyframeMain10(%v): %v", applyVariant, err)
	}

	stages := []debugReferenceStage16{{
		label:  "raw",
		plane:  append([]uint16(nil), yPlane...),
		stride: width,
		width:  width,
		height: height,
	}}
	applySimpleLoopFilter420_16Masked(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, dec.header.ColorConfig.BitDepth, cdef)
	stages = append(stages, debugReferenceStage16{
		label:  "loop",
		plane:  append([]uint16(nil), yPlane...),
		stride: width,
		width:  width,
		height: height,
	})
	applyCDEF420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, dec.header.ColorConfig.BitDepth, cdef)
	stages = append(stages, debugReferenceStage16{
		label:  "cdef",
		plane:  append([]uint16(nil), yPlane...),
		stride: width,
		width:  width,
		height: height,
	})
	applyRestoration420_16(yPlane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, &parsed.Header, restoration, dec.header.ColorConfig.BitDepth)
	stages = append(stages, debugReferenceStage16{
		label:  "restoration",
		plane:  append([]uint16(nil), yPlane...),
		stride: width,
		width:  width,
		height: height,
	})
	return stages
}

func debugReferenceEnvOverride(name, value string) func() {
	prev, ok := os.LookupEnv(name)
	if value == "" {
		_ = os.Unsetenv(name)
	} else {
		_ = os.Setenv(name, value)
	}
	return func() {
		if ok {
			_ = os.Setenv(name, prev)
			return
		}
		_ = os.Unsetenv(name)
	}
}

func parseDebugReferenceTargetBlock(spec string) (int, int, error) {
	parts := strings.Split(spec, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("want x,y, got %q", spec)
	}
	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func debugTraceReferenceLeaf(t *testing.T, dec *Decoder, parsed *ParsedFrame, ref testutil.RawFrame420, targetX, targetY, nextX, nextY int, label string) {
	t.Helper()
	t.Logf("tracing %s at (%d,%d)", label, targetX, targetY)
	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	var sb SuperBlockGeometry
	stateIndex := -1
	found := false
	for i := range states {
		for _, cur := range states[i].Geometry.SuperBlocks {
			if targetX >= cur.Start4X*4 && targetX < cur.End4X*4 && targetY >= cur.Start4Y*4 && targetY < cur.End4Y*4 {
				sb = cur
				stateIndex = i
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("target superblock not found for %s at (%d,%d)", label, targetX, targetY)
	}
	syntaxState, err := NewTileSyntaxState(dec.header, &parsed.Header, states[stateIndex], 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}
	traceSegState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	traceRestoration := NewRestorationState(dec.header, &parsed.Header)
	traceRestRefs := newTileRestorationRefs()
	for sbRow := states[stateIndex].Geometry.StartSBY; sbRow < sb.SBY; sbRow++ {
		priorRow, err := syntaxState.NewRowState(sbRow)
		if err != nil {
			t.Fatalf("NewRowState(%d): %v", sbRow, err)
		}
		for _, priorSB := range priorRow.SuperBlocks {
			if os.Getenv("DEBUG_SKIP_REST_PARSE") == "" {
				if err := parseRestorationUnitsForSuperBlock(&parsed.Header, priorRow.Tile.Tile.CDF, priorRow.Tile.Tile.Entropy, priorSB, traceRestoration, &traceRestRefs); err != nil {
					t.Fatalf("parseRestorationUnitsForSuperBlock(row=%d sb=%d): %v", sbRow, priorSB.Index, err)
				}
			}
			if err := debugConsumeBlockCDEFBeforeWithReader(dec.header, &parsed.Header, priorRow, priorSB, BlockGeometry{
				Level:   RootBlockLevel(dec.header),
				Size:    RootBlockSize(dec.header),
				Start4X: priorSB.Start4X,
				Start4Y: priorSB.Start4Y,
				End4X:   priorSB.End4X,
				End4Y:   priorSB.End4Y,
			}, traceSegState, nil); err != nil {
				t.Fatalf("debugConsumeBlockCDEFBeforeWithReader(row=%d sb=%d): %v", sbRow, priorSB.Index, err)
			}
		}
	}
	row, err := syntaxState.NewRowState(sb.SBY)
	if err != nil {
		t.Fatalf("NewRowState: %v", err)
	}
	for _, priorSB := range row.SuperBlocks {
		if priorSB.Index == sb.Index {
			break
		}
		if os.Getenv("DEBUG_SKIP_REST_PARSE") == "" {
			if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, priorSB, traceRestoration, &traceRestRefs); err != nil {
				t.Fatalf("parseRestorationUnitsForSuperBlock(row=%d sb=%d): %v", sb.SBY, priorSB.Index, err)
			}
		}
		if err := debugConsumeBlockCDEFBeforeWithReader(dec.header, &parsed.Header, row, priorSB, BlockGeometry{
			Level:   RootBlockLevel(dec.header),
			Size:    RootBlockSize(dec.header),
			Start4X: priorSB.Start4X,
			Start4Y: priorSB.Start4Y,
			End4X:   priorSB.End4X,
			End4Y:   priorSB.End4Y,
		}, traceSegState, nil); err != nil {
			t.Fatalf("debugConsumeBlockCDEFBeforeWithReader(%d): %v", priorSB.Index, err)
		}
	}
	if os.Getenv("DEBUG_SKIP_REST_PARSE") == "" {
		if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, traceRestoration, &traceRestRefs); err != nil {
			t.Fatalf("parseRestorationUnitsForSuperBlock(target sb=%d): %v", sb.Index, err)
		}
	}
	var (
		targetPreSyntaxCDF      *ModeCDF
		targetPreSyntaxDec      *msac.Context
		targetPreSyntaxAbove    BlockContext
		targetPreSyntaxLeft     BlockContext
		targetPreSyntaxGeom     BlockGeometry
		targetPreSyntaxHaveTop  bool
		targetPreSyntaxHaveLeft bool
		targetPreSyntaxSegID    uint8
	)
	captureSyntax := func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, entropy intraEntropyDecoder) (IntraBlockSyntax, error) {
		if targetPreSyntaxCDF == nil && cdf != nil {
			targetPreSyntaxCDF = cdf.Clone()
		}
		if targetPreSyntaxDec == nil {
			switch ent := entropy.(type) {
			case *msac.Context:
				targetPreSyntaxDec = ent.Clone()
			case msacEntropyDecoder:
				if ent.ctx != nil {
					targetPreSyntaxDec = ent.ctx.Clone()
				}
			}
		}
		if above != nil {
			targetPreSyntaxAbove = *above
		}
		if left != nil {
			targetPreSyntaxLeft = *left
		}
		targetPreSyntaxGeom = g
		targetPreSyntaxHaveTop = haveTop
		targetPreSyntaxHaveLeft = haveLeft
		targetPreSyntaxSegID = segID
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, entropy)
	}
	state, err := debugAdvanceToTargetLeafCDEFBeforeWithReader(dec.header, &parsed.Header, row, sb, BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}, traceSegState, targetX, targetY, nextX, nextY, captureSyntax)
	if err != nil {
		t.Fatalf("debugAdvanceToTargetLeafCDEFBefore: %v", err)
	}
	t.Logf("%s actual syntax xy=(%d,%d) size=%s y=%s uv=%s palY=%d palUV=%d tx=%s uvtx=%s",
		label,
		state.syntax.Geometry.Start4X*4,
		state.syntax.Geometry.Start4Y*4,
		blockName(state.syntax.Size),
		intraModeName(state.syntax.YMode),
		intraModeName(state.syntax.UVMode),
		state.syntax.PalYSize,
		state.syntax.PalUVSize,
		txName(state.syntax.TX),
		txName(state.syntax.UVTX),
	)
	blockX := state.syntax.Geometry.Start4X * 4
	blockY := state.syntax.Geometry.Start4Y * 4
	blockGeomW := minInt(state.syntax.Geometry.Width4()*4, parsed.Header.Width-blockX)
	blockGeomH := minInt(state.syntax.Geometry.Height4()*4, parsed.Header.Height-blockY)
	bx4 := state.syntax.Geometry.Start4X & 31
	by4 := state.syntax.Geometry.Start4Y & 31
	if state.syntax.PalYSize != 0 {
		if targetPreSyntaxCDF != nil && targetPreSyntaxDec != nil {
			debugTraceTargetClonedIntraSyntax(t, dec.header, &parsed.Header, targetPreSyntaxCDF, &targetPreSyntaxAbove, &targetPreSyntaxLeft, targetPreSyntaxGeom, state.syntax.Geometry, targetPreSyntaxHaveTop, targetPreSyntaxHaveLeft, targetPreSyntaxSegID, targetPreSyntaxDec.Clone())
			debugTraceTargetPaletteDecision(t, dec.header, &parsed.Header, targetPreSyntaxCDF, &targetPreSyntaxAbove, &targetPreSyntaxLeft, targetPreSyntaxGeom, state.syntax.Geometry, targetPreSyntaxHaveTop, targetPreSyntaxHaveLeft, targetPreSyntaxSegID, targetPreSyntaxDec.Clone())
		}
		cache, nCache := paletteCache(&state.above, &state.left, 0, bx4, by4)
		head := minInt(16, len(state.syntax.PalYIdx))
		t.Logf("%s palY cache=%v values=%v idx0=%v", label, append([]uint16(nil), cache[:nCache]...), state.syntax.PalY[:state.syntax.PalYSize], append([]uint8(nil), state.syntax.PalYIdx[:head]...))
		if ref.BitDepth > 8 {
			base := make([]uint16, blockGeomW*blockGeomH)
			if err := fillPaletteBlock16(base, blockGeomW, blockGeomW, blockGeomH, state.syntax.PalY, state.syntax.PalYIdx, state.syntax.Geometry.Width4()*4, 0, 0, ref.BitDepth); err != nil {
				t.Fatalf("fillPaletteBlock16: %v", err)
			}
			t.Logf("%s palY base rows=%v", label, debugBlockRows16(base, blockGeomW, 0, 0, minInt(4, blockGeomW), minInt(4, blockGeomH)))
			t.Logf("%s palY ref rows=%v", label, debugBlockRows16(ref.Y16, ref.Width, blockX, blockY, minInt(4, blockGeomW), minInt(4, blockGeomH)))
		} else {
			base := make([]byte, blockGeomW*blockGeomH)
			if err := fillPaletteBlock8(base, blockGeomW, blockGeomW, blockGeomH, state.syntax.PalY, state.syntax.PalYIdx, state.syntax.Geometry.Width4()*4, 0, 0); err != nil {
				t.Fatalf("fillPaletteBlock8: %v", err)
			}
			t.Logf("%s palY base rows=%v", label, debugBlockRows(base, blockGeomW, 0, 0, minInt(4, blockGeomW), minInt(4, blockGeomH)))
			t.Logf("%s palY ref rows=%v", label, debugBlockRows(ref.Y, ref.Width, blockX, blockY, minInt(4, blockGeomW), minInt(4, blockGeomH)))
		}
	}
	if state.syntax.PalUVSize != 0 {
		cache, nCache := paletteCache(&state.above, &state.left, 1, bx4, by4)
		head := minInt(16, len(state.syntax.PalUVIdx))
		t.Logf("%s palUV cache=%v palU=%v palV=%v idx0=%v", label, append([]uint16(nil), cache[:nCache]...), state.syntax.PalU[:state.syntax.PalUVSize], state.syntax.PalV[:state.syntax.PalUVSize], append([]uint8(nil), state.syntax.PalUVIdx[:head]...))
	}
	txInfo := TxfmInfoFor(state.syntax.TX)
	blockW := minInt(int(txInfo.W4)*4, parsed.Header.Width-blockX)
	blockH := minInt(int(txInfo.H4)*4, parsed.Header.Height-blockY)
	tuHaveTop := state.syntax.Geometry.Start4Y > 0
	tuHaveLeft := state.syntax.Geometry.Start4X > 0
	skipCtx := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, state.above.LCoef[bx4:], state.left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
	// DecodeBoolAdapt mutates the skip CDF, so keep the probe isolated from the
	// replay that we compare against the live decode path.
	probeCoefCDF := state.coef.Clone()
	if probeCoefCDF == nil {
		probeCoefCDF = NewDefaultCoefCDF(uint8(state.qidx))
	}
	cur, rng, cnt, pos := state.dec.DebugState()
	t.Logf("%s first-tu pre state=(%d,%d,%d,%d)", label, cur, rng, cnt, pos)
	firstTUBit := state.dec.Clone().DecodeBoolAdapt(probeCoefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0
	t.Logf("%s first-tu skip_ctx=%d haveTop=%v haveLeft=%v qidx=%d skip=%v", label, skipCtx, tuHaveTop, tuHaveLeft, state.qidx, firstTUBit)
	if firstTUBit {
		return
	}
	coefCDF := state.coef.Clone()
	if coefCDF == nil {
		coefCDF = NewDefaultCoefCDF(uint8(state.qidx))
	}

	modeCDF := NewDefaultModeCDF()
	if state.mode != nil {
		modeCDF = state.mode.Clone()
	}
	decClone := state.dec.Clone()
	if decClone.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
		t.Fatal("unexpected skip in actual replay")
	}
	cur, rng, cnt, pos = decClone.DebugState()
	t.Logf("%s first-tu after-skip state=(%d,%d,%d,%d)", label, cur, rng, cnt, pos)
	yTxType, err := ReadIntraTxType(&parsed.Header, modeCDF, state.syntax, state.syntax.TX, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: decClone})
	if err != nil {
		t.Fatalf("ReadIntraTxType: %v", err)
	}
	cur, rng, cnt, pos = decClone.DebugState()
	t.Logf("%s first-tu after-txtype=%d state=(%d,%d,%d,%d)", label, yTxType, cur, rng, cnt, pos)
	tu := childTXGeometry(state.syntax.Geometry, state.syntax.TX, 0, 0)
	aboveClone := state.above
	leftClone := state.left
	coeffs, _, _, err := DecodeLumaCoefficientsNoSkip(state.syntax.Size, tu, state.syntax.TX, yTxType, state.syntax.Segment, coefCDF, state.dq, &aboveClone, &leftClone, msacCoefficientDecoder{ctx: decClone})
	if err != nil {
		t.Fatalf("DecodeLumaCoefficientsNoSkip: %v", err)
	}
	t.Logf("%s first-tu txtype=%d dc=%d nonzero=%d maxAbs=%d coeffs=%v", label, yTxType, coeffDC(coeffs), coeffNonZero(coeffs), coeffMaxAbs(coeffs), append([]int32(nil), coeffs[:minInt(len(coeffs), 16)]...))
	if len(ref.Y) == 0 {
		t.Logf("%s skipping 8-bit replay metrics for %dbit reference trace", label, ref.BitDepth)
		return
	}
	predictFlags := intraLumaPredictFlags(dec.header, &state.above, &state.left, state.syntax)
	qidxVariants := []int{state.qidx}
	if headerQ := int(parsed.Header.Quantization.YAC); headerQ != state.qidx {
		qidxVariants = append(qidxVariants, headerQ)
	}
	if state.qidx != 255 {
		qidxVariants = append(qidxVariants, 255)
	}
	for _, qidx := range qidxVariants {
		altDQ := InitDequantTables(dec.header, &parsed.Header, qidx)
		modeCDFAlt := NewDefaultModeCDF()
		if state.mode != nil {
			modeCDFAlt = state.mode.Clone()
		}
		coefCDFAlt := NewDefaultCoefCDF(uint8(qidx))
		decAlt := state.dec.Clone()
		if decAlt.DecodeBoolAdapt(coefCDFAlt.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
			t.Fatalf("unexpected skip in forced-qidx=%d replay", qidx)
		}
		yTxTypeAlt, err := ReadIntraTxType(&parsed.Header, modeCDFAlt, state.syntax, state.syntax.TX, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: decAlt})
		if err != nil {
			t.Fatalf("ReadIntraTxType forced qidx=%d: %v", qidx, err)
		}
		aboveAlt := state.above
		leftAlt := state.left
		coeffsAlt, _, _, err := DecodeLumaCoefficientsNoSkip(state.syntax.Size, tu, state.syntax.TX, yTxTypeAlt, state.syntax.Segment, coefCDFAlt, altDQ, &aboveAlt, &leftAlt, msacCoefficientDecoder{ctx: decAlt})
		if err != nil {
			t.Fatalf("DecodeLumaCoefficientsNoSkip forced qidx=%d: %v", qidx, err)
		}
		scratch := make([]byte, parsed.Header.Width*parsed.Header.Height)
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			parsed.Header.Width,
			parsed.Header.Width,
			parsed.Header.Height,
			blockX,
			blockY,
			state.syntax.TX,
			yTxTypeAlt,
			state.syntax.YMode,
			state.syntax.YAngle,
			coeffsAlt,
			parsed.Header.Segmentation.Lossless[state.syntax.Segment],
			predictFlags,
			tuHaveTop,
			tuHaveLeft,
			true,
			true,
		); err != nil {
			t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges forced qidx=%d: %v", qidx, err)
		}
		mse := compareBlockToRefY(scratch, ref.Y, parsed.Header.Width, blockX, blockY, blockW, blockH)
		t.Logf("%s first-tu replay forced-qidx=%d dc=%d nonzero=%d maxAbs=%d mse=%.2f rows=%v", label, qidx, coeffDC(coeffsAlt), coeffNonZero(coeffsAlt), coeffMaxAbs(coeffsAlt), mse, debugBlockRows(scratch, parsed.Header.Width, blockX, blockY, minInt(4, blockW), minInt(4, blockH)))
	}
	{
		coefCDFAlt := NewDefaultCoefCDF(uint8(state.qidx))
		decAlt := state.dec.Clone()
		if decAlt.DecodeBoolAdapt(coefCDFAlt.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
			t.Fatal("unexpected skip in forced-dctdct-no-symbol replay")
		}
		aboveAlt := state.above
		leftAlt := state.left
		coeffsAlt, _, _, err := DecodeLumaCoefficientsNoSkip(state.syntax.Size, tu, state.syntax.TX, TxTypeDCTDCT, state.syntax.Segment, coefCDFAlt, state.dq, &aboveAlt, &leftAlt, msacCoefficientDecoder{ctx: decAlt})
		if err != nil {
			t.Fatalf("DecodeLumaCoefficientsNoSkip forced dctdct no-symbol: %v", err)
		}
		scratch := make([]byte, parsed.Header.Width*parsed.Header.Height)
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			parsed.Header.Width,
			parsed.Header.Width,
			parsed.Header.Height,
			blockX,
			blockY,
			state.syntax.TX,
			TxTypeDCTDCT,
			state.syntax.YMode,
			state.syntax.YAngle,
			coeffsAlt,
			parsed.Header.Segmentation.Lossless[state.syntax.Segment],
			predictFlags,
			tuHaveTop,
			tuHaveLeft,
			true,
			true,
		); err != nil {
			t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges forced dctdct no-symbol: %v", err)
		}
		mse := compareBlockToRefY(scratch, ref.Y, parsed.Header.Width, blockX, blockY, blockW, blockH)
		t.Logf("%s first-tu forced-dctdct-no-symbol dc=%d nonzero=%d maxAbs=%d coeffs=%v mse=%.2f rows=%v", label, coeffDC(coeffsAlt), coeffNonZero(coeffsAlt), coeffMaxAbs(coeffsAlt), append([]int32(nil), coeffsAlt[:minInt(len(coeffsAlt), 16)]...), mse, debugBlockRows(scratch, parsed.Header.Width, blockX, blockY, minInt(4, blockW), minInt(4, blockH)))
	}
	for _, variant := range []struct {
		label  string
		coeffs []int32
	}{
		{label: "predict-only", coeffs: make([]int32, len(coeffs))},
		{label: "dc-only", coeffs: func() []int32 {
			out := make([]int32, len(coeffs))
			copy(out, coeffs)
			for i := 1; i < len(out); i++ {
				out[i] = 0
			}
			return out
		}()},
		{label: "manual-neg-dc-q1", coeffs: func() []int32 {
			out := make([]int32, len(coeffs))
			out[0] = -int32(state.dq[state.syntax.Segment][0][0])
			return out
		}()},
		{label: "manual-pos-dc-q1", coeffs: func() []int32 {
			out := make([]int32, len(coeffs))
			out[0] = int32(state.dq[state.syntax.Segment][0][0])
			return out
		}()},
		{label: "manual-neg-scan1-q1", coeffs: func() []int32 {
			out := make([]int32, len(coeffs))
			idx, err := CoeffIndexFromScan(state.syntax.TX, uint32(ScanOrder(state.syntax.TX)[1]))
			if err != nil {
				t.Fatalf("CoeffIndexFromScan(scan1): %v", err)
			}
			out[idx] = -int32(state.dq[state.syntax.Segment][0][1])
			return out
		}()},
		{label: "manual-pos-scan1-q1", coeffs: func() []int32 {
			out := make([]int32, len(coeffs))
			idx, err := CoeffIndexFromScan(state.syntax.TX, uint32(ScanOrder(state.syntax.TX)[1]))
			if err != nil {
				t.Fatalf("CoeffIndexFromScan(scan1): %v", err)
			}
			out[idx] = int32(state.dq[state.syntax.Segment][0][1])
			return out
		}()},
		{label: "manual-neg-scan2-q1", coeffs: func() []int32 {
			out := make([]int32, len(coeffs))
			idx, err := CoeffIndexFromScan(state.syntax.TX, uint32(ScanOrder(state.syntax.TX)[2]))
			if err != nil {
				t.Fatalf("CoeffIndexFromScan(scan2): %v", err)
			}
			out[idx] = -int32(state.dq[state.syntax.Segment][0][1])
			return out
		}()},
	} {
		scratch := make([]byte, parsed.Header.Width*parsed.Header.Height)
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			parsed.Header.Width,
			parsed.Header.Width,
			parsed.Header.Height,
			blockX,
			blockY,
			state.syntax.TX,
			yTxType,
			state.syntax.YMode,
			state.syntax.YAngle,
			variant.coeffs,
			parsed.Header.Segmentation.Lossless[state.syntax.Segment],
			predictFlags,
			tuHaveTop,
			tuHaveLeft,
			true,
			true,
		); err != nil {
			t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(%s): %v", variant.label, err)
		}
		mse := compareBlockToRefY(scratch, ref.Y, parsed.Header.Width, blockX, blockY, blockW, blockH)
		t.Logf("%s first-tu replay %s mse=%.2f rows=%v", label, variant.label, mse, debugBlockRows(scratch, parsed.Header.Width, blockX, blockY, minInt(4, blockW), minInt(4, blockH)))
	}
	for _, forced := range debugReplayAltTxTypes(state.syntax.TX) {
		scratch := make([]byte, parsed.Header.Width*parsed.Header.Height)
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			parsed.Header.Width,
			parsed.Header.Width,
			parsed.Header.Height,
			blockX,
			blockY,
			state.syntax.TX,
			forced,
			state.syntax.YMode,
			state.syntax.YAngle,
			coeffs,
			parsed.Header.Segmentation.Lossless[state.syntax.Segment],
			predictFlags,
			tuHaveTop,
			tuHaveLeft,
			true,
			true,
		); err != nil {
			t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(%v): %v", forced, err)
		}
		mse := compareBlockToRefY(scratch, ref.Y, parsed.Header.Width, blockX, blockY, blockW, blockH)
		t.Logf("%s first-tu replay forced-txtype=%d mse=%.2f rows=%v", label, forced, mse, debugBlockRows(scratch, parsed.Header.Width, blockX, blockY, minInt(4, blockW), minInt(4, blockH)))
	}
	for _, forcedMode := range []IntraPredMode{
		IntraPredDC,
		IntraPredVertical,
		IntraPredHorizontal,
		IntraPredSmooth,
		IntraPredSmoothVertical,
		IntraPredSmoothHorizontal,
		IntraPredPaeth,
	} {
		forcedSyntax := state.syntax
		forcedSyntax.YMode = forcedMode
		forcedSyntax.YAngle = 0
		forcedPredictFlags := intraLumaPredictFlags(dec.header, &state.above, &state.left, forcedSyntax)
		scratch := make([]byte, parsed.Header.Width*parsed.Header.Height)
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			parsed.Header.Width,
			parsed.Header.Width,
			parsed.Header.Height,
			blockX,
			blockY,
			state.syntax.TX,
			yTxType,
			forcedMode,
			0,
			coeffs,
			parsed.Header.Segmentation.Lossless[state.syntax.Segment],
			forcedPredictFlags,
			tuHaveTop,
			tuHaveLeft,
			true,
			true,
		); err != nil {
			t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(mode=%v): %v", forcedMode, err)
		}
		mse := compareBlockToRefY(scratch, ref.Y, parsed.Header.Width, blockX, blockY, blockW, blockH)
		t.Logf("%s first-tu replay forced-ymode=%s mse=%.2f rows=%v", label, intraModeName(forcedMode), mse, debugBlockRows(scratch, parsed.Header.Width, blockX, blockY, minInt(4, blockW), minInt(4, blockH)))
	}
}

func debugConsumeRestorationThroughSuperBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, syntaxState *TileSyntaxState, target SuperBlockGeometry) error {
	if hdr == nil || syntaxState == nil {
		return nil
	}
	restoration := NewRestorationState(seq, hdr)
	restRefs := newTileRestorationRefs()
	for _, sb := range syntaxState.Tile.Geometry.SuperBlocks {
		if err := parseRestorationUnitsForSuperBlock(hdr, syntaxState.Tile.CDF, syntaxState.Tile.Entropy, sb, restoration, &restRefs); err != nil {
			return err
		}
		if sb.Index == target.Index {
			return nil
		}
	}
	return fmt.Errorf("decoder: target superblock %d not found in tile restoration traversal", target.Index)
}

func debugBlockRows(plane []byte, stride, x, y, width, height int) []string {
	rows := make([]string, 0, height)
	for row := 0; row < height; row++ {
		start := (y+row)*stride + x
		rows = append(rows, fmt.Sprintf("%v", plane[start:start+width]))
	}
	return rows
}

func debugBlockRows16(plane []uint16, stride, x, y, width, height int) []string {
	rows := make([]string, 0, height)
	for row := 0; row < height; row++ {
		start := (y+row)*stride + x
		rows = append(rows, fmt.Sprintf("%v", plane[start:start+width]))
	}
	return rows
}

func logReferencePipelineMetrics(t *testing.T, label string, img *image.YCbCr, ref testutil.RawFrame420) {
	t.Helper()
	if len(ref.Y) == 0 {
		t.Logf("%s metrics skipped for %dbit reference", label, ref.BitDepth)
		return
	}
	chromaWidth := (ref.Width + 1) >> 1
	chromaHeight := (ref.Height + 1) >> 1
	gotY := flattenPlane8(img.Y, img.YStride, ref.Width, ref.Height)
	gotU := flattenPlane8(img.Cb, img.CStride, chromaWidth, chromaHeight)
	gotV := flattenPlane8(img.Cr, img.CStride, chromaWidth, chromaHeight)
	y := testutil.ComparePlane8(gotY, ref.Y)
	u := testutil.ComparePlane8(gotU, ref.U)
	v := testutil.ComparePlane8(gotV, ref.V)
	t.Logf("%s Y rmse=%.4f mean_abs=%.4f max_abs=%d", label, y.RMSE, y.MeanAbs, y.MaxAbs)
	t.Logf("%s U rmse=%.4f mean_abs=%.4f max_abs=%d", label, u.RMSE, u.MeanAbs, u.MaxAbs)
	t.Logf("%s V rmse=%.4f mean_abs=%.4f max_abs=%d", label, v.RMSE, v.MeanAbs, v.MaxAbs)
	logMovieMismatchSummary(t, label, img.Y, img.YStride, ref.Y, ref.Width, ref.Height)
	logMovieMismatchSummary(t, label+" U", img.Cb, img.CStride, ref.U, chromaWidth, chromaHeight)
	logMovieMismatchSummary(t, label+" V", img.Cr, img.CStride, ref.V, chromaWidth, chromaHeight)
}

func logReferencePipelineMetrics16(t *testing.T, label string, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, ref testutil.RawFrame420) {
	t.Helper()
	if len(ref.Y16) == 0 {
		t.Logf("%s metrics skipped for %dbit reference", label, ref.BitDepth)
		return
	}
	chromaWidth := (ref.Width + 1) >> 1
	chromaHeight := (ref.Height + 1) >> 1
	gotY := flattenPlane16(yPlane, yStride, ref.Width, ref.Height)
	gotU := flattenPlane16(uPlane, uStride, chromaWidth, chromaHeight)
	gotV := flattenPlane16(vPlane, vStride, chromaWidth, chromaHeight)
	y := testutil.ComparePlane16(gotY, ref.Y16)
	u := testutil.ComparePlane16(gotU, ref.U16)
	v := testutil.ComparePlane16(gotV, ref.V16)
	t.Logf("%s Y rmse=%.4f mean_abs=%.4f max_abs=%d", label, y.RMSE, y.MeanAbs, y.MaxAbs)
	t.Logf("%s U rmse=%.4f mean_abs=%.4f max_abs=%d", label, u.RMSE, u.MeanAbs, u.MaxAbs)
	t.Logf("%s V rmse=%.4f mean_abs=%.4f max_abs=%d", label, v.RMSE, v.MeanAbs, v.MaxAbs)
	logMovieMismatchSummary16(t, label, yPlane, yStride, ref.Y16, ref.Width, ref.Height)
	logMovieMismatchSummary16(t, label+" U", uPlane, uStride, ref.U16, chromaWidth, chromaHeight)
	logMovieMismatchSummary16(t, label+" V", vPlane, vStride, ref.V16, chromaWidth, chromaHeight)
}

func logMovieMismatchSummary16(t *testing.T, label string, plane []uint16, stride int, ref []uint16, width, height int) {
	t.Helper()
	if width <= 0 || height <= 0 {
		return
	}
	firstX, firstY := -1, -1
	firstGot, firstWant := 0, 0
	maxX, maxY := 0, 0
	maxGot, maxWant := 0, 0
	maxDiff := -1
	for y := 0; y < height; y++ {
		rowBase := y * stride
		refBase := y * width
		for x := 0; x < width; x++ {
			got := int(plane[rowBase+x])
			want := int(ref[refBase+x])
			diff := got - want
			if diff < 0 {
				diff = -diff
			}
			if diff != 0 && firstX < 0 {
				firstX, firstY = x, y
				firstGot, firstWant = got, want
			}
			if diff > maxDiff {
				maxDiff = diff
				maxX, maxY = x, y
				maxGot, maxWant = got, want
			}
		}
	}
	if firstX < 0 {
		t.Logf("%s mismatch first=none max=0", label)
		return
	}
	t.Logf("%s mismatch first=(%d,%d) tu=(%d,%d) got=%d want=%d max=(%d,%d) tu=(%d,%d) got=%d want=%d abs=%d",
		label,
		firstX, firstY, firstX&^3, firstY&^3, firstGot, firstWant,
		maxX, maxY, maxX&^3, maxY&^3, maxGot, maxWant, maxDiff,
	)
	logMovieBlockPatch16(t, label, "first-ours", plane, stride, firstX&^3, firstY&^3)
	logMovieBlockPatch16(t, label, "first-ref", ref, width, firstX&^3, firstY&^3)
	if maxX != firstX || maxY != firstY {
		logMovieBlockPatch16(t, label, "max-ours", plane, stride, maxX&^3, maxY&^3)
		logMovieBlockPatch16(t, label, "max-ref", ref, width, maxX&^3, maxY&^3)
	}
	for _, threshold := range []int{24, 64, 128, 256} {
		x, y, got, want, ok := firstMismatchOverThreshold16(plane, stride, ref, width, height, threshold)
		if !ok {
			t.Logf("%s first abs>%d=none", label, threshold)
			continue
		}
		t.Logf("%s first abs>%d=(%d,%d) tu=(%d,%d) got=%d want=%d",
			label, threshold, x, y, x&^3, y&^3, got, want)
		logMovieBlockPatch16(t, label, fmt.Sprintf("abs>%d-ours", threshold), plane, stride, x&^3, y&^3)
		logMovieBlockPatch16(t, label, fmt.Sprintf("abs>%d-ref", threshold), ref, width, x&^3, y&^3)
	}
}

func firstMismatchOverThreshold16(plane []uint16, stride int, ref []uint16, width, height, threshold int) (int, int, int, int, bool) {
	if width <= 0 || height <= 0 || stride <= 0 {
		return 0, 0, 0, 0, false
	}
	for y := 0; y < height; y++ {
		rowBase := y * stride
		refBase := y * width
		for x := 0; x < width; x++ {
			got := int(plane[rowBase+x])
			want := int(ref[refBase+x])
			diff := got - want
			if diff < 0 {
				diff = -diff
			}
			if diff > threshold {
				return x, y, got, want, true
			}
		}
	}
	return 0, 0, 0, 0, false
}

func logMovieBlockPatch16(t *testing.T, label, which string, plane []uint16, stride, x, y int) {
	t.Helper()
	if stride <= 0 || len(plane) == 0 || x < 0 || y < 0 {
		return
	}
	height := len(plane) / stride
	if height <= 0 || y >= height || x >= stride {
		return
	}
	w := minInt(4, stride-x)
	h := minInt(4, height-y)
	for py := 0; py < h; py++ {
		start := (y+py)*stride + x
		row := plane[start : start+w]
		t.Logf("%s %s block y=%d x=%d: %v", label, which, y+py, x, row)
	}
}
