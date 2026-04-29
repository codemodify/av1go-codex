package decoder

import (
	"io"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugGeneratedMain10PaletteCDFVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_CDF_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-cdf-variants.mp4",
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
	parsed := openGeneratedMain10ParsedKeyframe(t, fixture.Path)

	variants := []struct {
		name string
		read debugIntraSyntaxReader
	}{
		{
			name: "current",
			read: nil,
		},
		{
			name: "default_pal_y",
			read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true}),
		},
		{
			name: "default_pal_size",
			read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true}),
		},
		{
			name: "default_pal_uv",
			read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palUV: true}),
		},
		{
			name: "default_palette_flags",
			read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true}),
		},
		{
			name: "default_color_map",
			read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{colorMap: true}),
		},
		{
			name: "default_palette_all",
			read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true}),
		},
	}

	for _, variant := range variants {
		state, calls := advanceGeneratedMain10TargetWithReader(t, fixture.Path, variant.read)
		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

		baseMSE := -1.0
		fullMSE := -1.0
		txLabel := "n/a"
		nonSkipped := false
		if state.syntax.PalYSize != 0 {
			baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
			if err != nil {
				t.Fatalf("%s cloneNthNonSkippedTUCoefficientInput: %v", variant.name, err)
			}
			if ok {
				decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
				fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
				txLabel = "txType=" + itoaDebugInt(int(txType))
				nonSkipped = true
			}
		}

		t.Logf("%s calls=%d syntax=%s", variant.name, calls, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		if state.syntax.PalYSize != 0 {
			t.Logf("%s palette base_mse=%.2f full_mse=%.2f first_non_skipped=%v tx=%s pal=%v idx0=%v",
				variant.name,
				baseMSE,
				fullMSE,
				nonSkipped,
				txLabel,
				state.syntax.PalY[:state.syntax.PalYSize],
				append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
			)
		}
	}
}

func TestDebugGeneratedMain10PaletteCDFPerCall(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_CDF_PER_CALL") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-cdf-per-call.mp4",
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
	parsed := openGeneratedMain10ParsedKeyframe(t, fixture.Path)

	entries, baselineState, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) == 0 {
		t.Fatal("no target entries")
	}
	targetCall := len(entries) - 1
	prevCall := targetCall - 1

	variants := []struct {
		name   string
		call   int
		reset  debugPaletteCDFReset
		active bool
	}{
		{name: "current"},
		{name: "target_default_pal_y", call: targetCall, reset: debugPaletteCDFReset{palY: true}, active: true},
		{name: "target_default_pal_size", call: targetCall, reset: debugPaletteCDFReset{palSize: true}, active: true},
		{name: "target_default_pal_uv", call: targetCall, reset: debugPaletteCDFReset{palUV: true}, active: true},
		{name: "target_default_palette_flags", call: targetCall, reset: debugPaletteCDFReset{palY: true, palSize: true, palUV: true}, active: true},
		{name: "target_default_color_map", call: targetCall, reset: debugPaletteCDFReset{colorMap: true}, active: true},
	}
	if prevCall >= 0 {
		variants = append(variants,
			struct {
				name   string
				call   int
				reset  debugPaletteCDFReset
				active bool
			}{name: "prev_default_pal_y", call: prevCall, reset: debugPaletteCDFReset{palY: true}, active: true},
			struct {
				name   string
				call   int
				reset  debugPaletteCDFReset
				active bool
			}{name: "prev_default_pal_size", call: prevCall, reset: debugPaletteCDFReset{palSize: true}, active: true},
			struct {
				name   string
				call   int
				reset  debugPaletteCDFReset
				active bool
			}{name: "prev_default_palette_flags", call: prevCall, reset: debugPaletteCDFReset{palY: true, palSize: true, palUV: true}, active: true},
		)
	}

	for _, variant := range variants {
		var (
			state debugTargetLeafAdvanceState
			calls int
		)
		overrides := map[int]debugIntraSyntaxReader(nil)
		if variant.active {
			overrides = map[int]debugIntraSyntaxReader{
				variant.call: debugPaletteCDFOverrideReader(variant.reset),
			}
			_, state, calls = traceGeneratedMain10TargetLeaves(t, fixture.Path, overrides)
		} else {
			state = baselineState
			calls = len(entries)
		}

		yPlane, _, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, overrides)
		yMetrics := testutil.ComparePlane16(yPlane, ref.Y16)

		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

		baseMSE := -1.0
		fullMSE := -1.0
		txLabel := "n/a"
		nonSkipped := false
		if state.syntax.PalYSize != 0 {
			baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
			if err != nil {
				t.Fatalf("%s cloneNthNonSkippedTUCoefficientInput: %v", variant.name, err)
			}
			if ok {
				decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
				fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
				txLabel = "txType=" + itoaDebugInt(int(txType))
				nonSkipped = true
			}
		}

		if variant.active {
			callLabel := "call=" + itoaDebugInt(variant.call)
			if variant.call >= 0 && variant.call < len(entries) {
				callLabel += " " + formatLeafTraceEntry(entries[variant.call])
			}
			t.Logf("%s %s calls=%d syntax=%s", variant.name, callLabel, calls, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		} else {
			t.Logf("%s calls=%d syntax=%s", variant.name, calls, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		}
		t.Logf("%s whole_frame_y_rmse=%.4f", variant.name, yMetrics.RMSE)
		if state.syntax.PalYSize != 0 {
			t.Logf("%s palette base_mse=%.2f full_mse=%.2f first_non_skipped=%v tx=%s pal=%v idx0=%v",
				variant.name,
				baseMSE,
				fullMSE,
				nonSkipped,
				txLabel,
				state.syntax.PalY[:state.syntax.PalYSize],
				append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
			)
		}
	}
}

func TestDebugGeneratedMain10PaletteSizeResetSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_SIZE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-size-sweep.mp4",
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
	parsed := openGeneratedMain10ParsedKeyframe(t, fixture.Path)
	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) == 0 {
		t.Fatal("no target entries")
	}
	targetCall := len(entries) - 1

	baseline := debugScoreGeneratedMain10TargetPaletteWithOverrides(t, fixture.Path, parsed, ref, nil)
	t.Logf("baseline target_call=%d y_rmse=%.4f base_mse=%.2f full_mse=%.2f syntax=%s",
		targetCall, baseline.yRMSE, baseline.baseMSE, baseline.fullMSE, formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)))

	for call := 0; call <= targetCall; call++ {
		overrides := map[int]debugIntraSyntaxReader{
			call: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true}),
		}
		score := debugScoreGeneratedMain10TargetPaletteWithOverrides(t, fixture.Path, parsed, ref, overrides)
		if debugTargetPaletteScoreMatches(baseline, score) {
			continue
		}
		t.Logf("call=%d %s", call, formatLeafTraceEntry(entries[call]))
		t.Logf("call=%d y_rmse=%.4f base_mse=%.2f full_mse=%.2f syntax=%s pal=%v idx0=%v",
			call,
			score.yRMSE,
			score.baseMSE,
			score.fullMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			score.state.syntax.PalY[:score.state.syntax.PalYSize],
			append([]uint8(nil), score.state.syntax.PalYIdx[:minInt(len(score.state.syntax.PalYIdx), 16)]...),
		)
	}
}

func TestDebugGeneratedMain10PaletteSizeRestoreAfterCallOne(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_SIZE_RESTORE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-size-restore.mp4",
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
	parsed := openGeneratedMain10ParsedKeyframe(t, fixture.Path)
	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) <= 1 {
		t.Fatalf("need at least two entries, got %d", len(entries))
	}

	baseline := debugScoreGeneratedMain10TargetPaletteWithOverrides(t, fixture.Path, parsed, ref, nil)
	t.Logf("baseline call1=%s", formatLeafTraceEntry(entries[1]))
	t.Logf("baseline y_rmse=%.4f base_mse=%.2f full_mse=%.2f syntax=%s",
		baseline.yRMSE, baseline.baseMSE, baseline.fullMSE, formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)))

	variants := []struct {
		name string
		read debugIntraSyntaxReader
	}{
		{
			name: "force_default_row_y_ctx4_after_call1",
			read: func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
				syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
				cdf.PalSize[0][4] = defaultModeCDFValue.PalSize[0][4]
				return syntax, err
			},
		},
		{
			name: "force_default_all_palsize_after_call1",
			read: func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
				syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
				cdf.PalSize = defaultModeCDFValue.PalSize
				return syntax, err
			},
		},
		{
			name: "restore_row_y_ctx4_after_call1",
			read: func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
				saved := cdf.PalSize[0][4]
				syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
				cdf.PalSize[0][4] = saved
				return syntax, err
			},
		},
		{
			name: "restore_all_palsize_after_call1",
			read: func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
				saved := cdf.PalSize
				syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
				cdf.PalSize = saved
				return syntax, err
			},
		},
	}

	for _, variant := range variants {
		score := debugScoreGeneratedMain10TargetPaletteWithOverrides(t, fixture.Path, parsed, ref, map[int]debugIntraSyntaxReader{1: variant.read})
		t.Logf("%s y_rmse=%.4f base_mse=%.2f full_mse=%.2f syntax=%s pal=%v idx0=%v",
			variant.name,
			score.yRMSE,
			score.baseMSE,
			score.fullMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			score.state.syntax.PalY[:score.state.syntax.PalYSize],
			append([]uint8(nil), score.state.syntax.PalYIdx[:minInt(len(score.state.syntax.PalYIdx), 16)]...),
		)
	}
}

func TestDebugGeneratedMain10PaletteSizeRowTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_SIZE_ROW_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-size-row-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	perCall := map[int]debugIntraSyntaxReader{}
	for _, callIdx := range []int{0, 1, 32} {
		callIdx := callIdx
		perCall[callIdx] = func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
			before := cdf.PalSize[0][4]
			syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
			after := cdf.PalSize[0][4]
			t.Logf("call=%d geom=(%d,%d)-(%d,%d) size=%s sizeCtx=%d y=%s uv=%s palY=%d palUV=%d row_before=%v row_after=%v",
				callIdx,
				g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
				blockName(g.Size),
				paletteSizeContext(g),
				intraModeName(syntax.YMode),
				intraModeName(syntax.UVMode),
				syntax.PalYSize,
				syntax.PalUVSize,
				before,
				after,
			)
			return syntax, err
		}
	}
	traceGeneratedMain10TargetLeaves(t, fixture.Path, perCall)
}

type debugPaletteCDFReset struct {
	palY     bool
	palSize  bool
	palUV    bool
	colorMap bool
}

type debugTargetPaletteScore struct {
	state      debugTargetLeafAdvanceState
	calls      int
	yRMSE      float64
	baseMSE    float64
	fullMSE    float64
	txLabel    string
	nonSkipped bool
}

func debugScoreGeneratedMain10TargetPaletteWithOverrides(t *testing.T, path string, parsed *ParsedFrame, ref testutil.RawFrame420, overrides map[int]debugIntraSyntaxReader) debugTargetPaletteScore {
	t.Helper()

	_, state, calls := traceGeneratedMain10TargetLeaves(t, path, overrides)
	yPlane, _, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, path, overrides)
	yMetrics := testutil.ComparePlane16(yPlane, ref.Y16)

	blockW := state.syntax.Geometry.Width4() * 4
	blockH := state.syntax.Geometry.Height4() * 4
	blockX := state.syntax.Geometry.Start4X * 4
	blockY := state.syntax.Geometry.Start4Y * 4
	refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

	score := debugTargetPaletteScore{
		state:   state,
		calls:   calls,
		yRMSE:   yMetrics.RMSE,
		baseMSE: -1.0,
		fullMSE: -1.0,
		txLabel: "n/a",
	}
	if state.syntax.PalYSize == 0 {
		return score
	}

	score.baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
	prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
	if err != nil {
		t.Fatalf("cloneNthNonSkippedTUCoefficientInput: %v", err)
	}
	if !ok {
		return score
	}

	decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
	score.fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
	score.txLabel = "txType=" + itoaDebugInt(int(txType))
	score.nonSkipped = true
	return score
}

func debugTargetPaletteScoreMatches(a, b debugTargetPaletteScore) bool {
	if a.state.syntax.Geometry != b.state.syntax.Geometry {
		return false
	}
	if a.state.syntax.Skip != b.state.syntax.Skip ||
		a.state.syntax.YMode != b.state.syntax.YMode ||
		a.state.syntax.UVMode != b.state.syntax.UVMode ||
		a.state.syntax.TX != b.state.syntax.TX ||
		a.state.syntax.UVTX != b.state.syntax.UVTX ||
		a.state.syntax.PalYSize != b.state.syntax.PalYSize ||
		a.state.syntax.PalUVSize != b.state.syntax.PalUVSize {
		return false
	}
	if a.baseMSE != b.baseMSE || a.fullMSE != b.fullMSE {
		return false
	}
	head := minInt(len(a.state.syntax.PalYIdx), len(b.state.syntax.PalYIdx))
	for i := 0; i < head; i++ {
		if a.state.syntax.PalYIdx[i] != b.state.syntax.PalYIdx[i] {
			return false
		}
	}
	for i := uint8(0); i < a.state.syntax.PalYSize; i++ {
		if a.state.syntax.PalY[i] != b.state.syntax.PalY[i] {
			return false
		}
	}
	return true
}

func debugApplyPaletteCDFReset(cdf *ModeCDF, reset debugPaletteCDFReset) {
	if cdf == nil {
		return
	}
	if reset.palY {
		cdf.PalY = defaultModeCDFValue.PalY
	}
	if reset.palSize {
		cdf.PalSize = defaultModeCDFValue.PalSize
	}
	if reset.palUV {
		cdf.PalUV = defaultModeCDFValue.PalUV
	}
	if reset.colorMap {
		cdf.ColorMap = defaultModeCDFValue.ColorMap
	}
}

func debugPaletteCDFOverrideReader(reset debugPaletteCDFReset) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		debugApplyPaletteCDFReset(cdf, reset)
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	}
}

func itoaDebugInt(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func openGeneratedMain10ParsedKeyframe(t *testing.T, path string) *ParsedFrame {
	t.Helper()

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			return pf
		}
	}
}
