package decoder

import (
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

type debugGeneratedMain10LeafScore struct {
	call      int
	state     debugTargetLeafAdvanceState
	got       bool
	yRMSE     float64
	blockMSE  float64
	uBlockMSE float64
	vBlockMSE float64
	baseMSE   float64
	fullMSE   float64
}

func debugGeneratedMain10LeafTargetScore(t *testing.T, path string, ref testutil.RawFrame420, call int, overrides map[int]debugIntraSyntaxReader) debugGeneratedMain10LeafScore {
	t.Helper()

	var (
		state debugTargetLeafAdvanceState
		got   bool
	)
	yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != call {
			return
		}
		state = captured
		got = true
	})
	if !got {
		t.Fatalf("did not capture call %d state", call)
	}

	score := debugGeneratedMain10LeafScore{
		call:      call,
		state:     state,
		got:       true,
		yRMSE:     testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
		blockMSE:  -1.0,
		uBlockMSE: -1.0,
		vBlockMSE: -1.0,
		baseMSE:   -1.0,
		fullMSE:   -1.0,
	}

	blockX := state.syntax.Geometry.Start4X * 4
	blockY := state.syntax.Geometry.Start4Y * 4
	blockW := state.syntax.Geometry.Width4() * 4
	blockH := state.syntax.Geometry.Height4() * 4
	block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
	score.blockMSE = compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

	chromaGeom := chromaGeometryForBlock(av1.Chroma420, state.syntax.Geometry)
	if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
		chromaX := chromaGeom.Start4X * 4
		chromaY := chromaGeom.Start4Y * 4
		chromaW := chromaGeom.Width4() * 4
		chromaH := chromaGeom.Height4() * 4
		chromaStride := (ref.Width + 1) >> 1
		uBlock := extractBlock16(uPlane, uStride, chromaX, chromaY, chromaW, chromaH)
		vBlock := extractBlock16(vPlane, vStride, chromaX, chromaY, chromaW, chromaH)
		score.uBlockMSE = compareBlockToRefY16(uBlock, chromaW, ref.U16, chromaStride, chromaX, chromaY, chromaW, chromaH)
		score.vBlockMSE = compareBlockToRefY16(vBlock, chromaW, ref.V16, chromaStride, chromaX, chromaY, chromaW, chromaH)
	}

	if state.syntax.PalYSize == 0 {
		return score
	}

	refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	score.baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
	parsed := openGeneratedMain10ParsedKeyframe(t, path)
	prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
	if err != nil {
		t.Fatalf("call %d cloneNthNonSkippedTUCoefficientInput: %v", call, err)
	}
	if !ok {
		score.fullMSE = score.baseMSE
		return score
	}
	decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
	score.fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
	return score
}

func TestDebugGeneratedMain10Call5VariantSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL5_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call5-variants.mp4",
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

	type variant struct {
		name string
		read debugIntraSyntaxReader
	}
	variants := []variant{
		{name: "current"},
		{name: "bounded_y_delta_bits", read: debugReadIntraBlockSyntaxWithCustomYPalette(debugReadPalettePlaneSyntaxBoundedYDeltaBits)},
		{name: "skip_cfl_alpha", read: debugReadIntraBlockSyntaxSkipCFLAlpha},
		{name: "force_cfl_v_neg1", read: debugReadIntraBlockSyntaxOverrideCFLAlpha(0, -1)},
		{name: "force_cfl_v_zero", read: debugReadIntraBlockSyntaxOverrideCFLAlpha(0, 0)},
		{name: "cfl_after_luma_palette", read: debugReadIntraBlockSyntaxCFLAfterLumaPalette},
		{name: "cfl_after_luma_indices", read: debugReadIntraBlockSyntaxCFLAfterLumaIndices},
		{name: "palette_before_uv_mode", read: debugReadIntraBlockSyntaxPaletteBeforeUVMode},
		{name: "force_cache_mask_0", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(0)},
		{name: "force_cache_mask_1", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(1)},
		{name: "force_cache_mask_2", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(2)},
		{name: "no_cfl", read: debugReadIntraBlockSyntaxNoCFL},
		{name: "local_no_palette_cache", read: debugReadIntraBlockSyntaxNoPaletteCache},
		{name: "default_pal_size", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true})},
		{name: "default_pal_y", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true})},
		{name: "default_palette_all", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true})},
		{name: "default_color_map", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{colorMap: true})},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{5: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 5, overrides)
		cache, nCache := paletteCache(&score.state.above, &score.state.left, 0, score.state.syntax.Geometry.Start4X&31, score.state.syntax.Geometry.Start4Y&31)
		cacheVals := append([]uint16(nil), cache[:nCache]...)
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f syntax=%s cfl=%v cache=%v pal=%v idx0=%v",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			score.state.syntax.CFLAlpha,
			cacheVals,
			score.state.syntax.PalY[:score.state.syntax.PalYSize],
			append([]uint8(nil), score.state.syntax.PalYIdx[:minInt(len(score.state.syntax.PalYIdx), 16)]...),
		)
	}
}

func TestDebugGeneratedMain10Call4SingleLeafVariantSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_SINGLE_LEAF") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-single-leaf.mp4",
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

	type variant struct {
		name string
		read debugIntraSyntaxReader
	}
	variants := []variant{
		{name: "current"},
		{name: "skip_cfl_alpha", read: debugReadIntraBlockSyntaxSkipCFLAlpha},
		{name: "force_cfl_v_neg1", read: debugReadIntraBlockSyntaxOverrideCFLAlpha(0, -1)},
		{name: "force_cfl_v_zero", read: debugReadIntraBlockSyntaxOverrideCFLAlpha(0, 0)},
		{name: "cfl_after_luma_palette", read: debugReadIntraBlockSyntaxCFLAfterLumaPalette},
		{name: "cfl_after_luma_indices", read: debugReadIntraBlockSyntaxCFLAfterLumaIndices},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 4, overrides)
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v syntax=%s",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			score.state.syntax.CFLAlpha,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
		)
	}
}

func TestDebugGeneratedMain10Call5PaletteCDFPerCall(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL5_PALETTE_CDF_PER_CALL") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call5-palette-cdf-per-call.mp4",
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

	baseline := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 5, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f syntax=%s cfl=%v",
		baseline.yRMSE,
		baseline.blockMSE,
		baseline.uBlockMSE,
		baseline.vBlockMSE,
		baseline.baseMSE,
		baseline.fullMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
		baseline.state.syntax.CFLAlpha,
	)

	type variant struct {
		name string
		call int
		read debugIntraSyntaxReader
	}
	variants := []variant{
		{name: "call4_default_pal_y", call: 4, read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true})},
		{name: "call4_default_pal_size", call: 4, read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true})},
		{name: "call4_default_palette_all", call: 4, read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true})},
		{name: "call4_default_color_map", call: 4, read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{colorMap: true})},
		{name: "call4_default_cfl_sign", call: 4, read: debugCFLCDFOverrideReader(debugCFLCDFReset{sign: true})},
		{name: "call4_default_cfl_alpha", call: 4, read: debugCFLCDFOverrideReader(debugCFLCDFReset{alpha: true})},
		{name: "call4_default_cfl_all", call: 4, read: debugCFLCDFOverrideReader(debugCFLCDFReset{sign: true, alpha: true})},
	}

	for _, tc := range variants {
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 5, map[int]debugIntraSyntaxReader{
			tc.call: tc.read,
		})
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f syntax=%s cfl=%v",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			score.state.syntax.CFLAlpha,
		)
	}
}
