package decoder

import (
	"fmt"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugCFLCDFReset struct {
	sign  bool
	alpha bool
}

type debugGeneratedMain10Call4Score struct {
	state     debugTargetLeafAdvanceState
	got       bool
	yRMSE     float64
	blockMSE  float64
	uBlockMSE float64
	vBlockMSE float64
	baseMSE   float64
}

func debugApplyCFLCDFReset(cdf *ModeCDF, reset debugCFLCDFReset) {
	if cdf == nil {
		return
	}
	if reset.sign {
		cdf.CFLSign = defaultModeCDFValue.CFLSign
	}
	if reset.alpha {
		cdf.CFLAlpha = defaultModeCDFValue.CFLAlpha
	}
}

func debugCFLCDFOverrideReader(reset debugCFLCDFReset) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		debugApplyCFLCDFReset(cdf, reset)
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	}
}

func debugUVModeLabel(mode IntraPredMode) string {
	if mode == IntraPredCFL {
		return "CFL"
	}
	if mode == IntraPredFilter {
		return "FILTER"
	}
	return intraModeName(mode)
}

func debugGeneratedMain10Call4TargetScore(t *testing.T, path string, ref testutil.RawFrame420, overrides map[int]debugIntraSyntaxReader) debugGeneratedMain10Call4Score {
	t.Helper()

	var (
		state debugTargetLeafAdvanceState
		got   bool
	)
	yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 4 {
			return
		}
		state = captured
		got = true
	})
	if !got {
		t.Fatal("did not capture call 4 state")
	}

	score := debugGeneratedMain10Call4Score{
		state:     state,
		got:       true,
		yRMSE:     testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
		blockMSE:  -1.0,
		uBlockMSE: -1.0,
		vBlockMSE: -1.0,
		baseMSE:   -1.0,
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

	if state.syntax.PalYSize != 0 {
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		score.baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
	}
	return score
}

func debugCFLCDFSliceString(vals []uint16) string {
	if len(vals) == 0 {
		return "[]"
	}
	return fmt.Sprintf("%v", append([]uint16(nil), vals...))
}

func debugTraceCFLCDFReader(t *testing.T, call int) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		before := cdf.Clone()
		syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
		if err != nil {
			return syntax, err
		}
		t.Logf(
			"call=%d geom=(%d,%d)-(%d,%d) y=%s uv=%s palY=%d cflSign before=%s after=%s",
			call,
			g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
			intraModeName(syntax.YMode), debugUVModeLabel(syntax.UVMode), syntax.PalYSize,
			debugCFLCDFSliceString(before.CFLSign[:]),
			debugCFLCDFSliceString(cdf.CFLSign[:]),
		)
		if syntax.UVMode != IntraPredCFL {
			t.Logf("call=%d no CFL alpha rows consumed", call)
			return syntax, nil
		}

		signU := 0
		if syntax.CFLAlpha[0] < 0 {
			signU = 1
		} else if syntax.CFLAlpha[0] > 0 {
			signU = 2
		}
		signV := 0
		if syntax.CFLAlpha[1] < 0 {
			signV = 1
		} else if syntax.CFLAlpha[1] > 0 {
			signV = 2
		}
		if signU != 0 {
			ctxU := cflAlphaContext(0, signU, signV)
			t.Logf("call=%d alphaU ctx=%d before=%s after=%s alpha=%d", call, ctxU, debugCFLCDFSliceString(before.CFLAlpha[ctxU][:]), debugCFLCDFSliceString(cdf.CFLAlpha[ctxU][:]), syntax.CFLAlpha[0])
		}
		if signV != 0 {
			ctxV := cflAlphaContext(1, signV, signU)
			t.Logf("call=%d alphaV ctx=%d before=%s after=%s alpha=%d", call, ctxV, debugCFLCDFSliceString(before.CFLAlpha[ctxV][:]), debugCFLCDFSliceString(cdf.CFLAlpha[ctxV][:]), syntax.CFLAlpha[1])
		}
		return syntax, nil
	}
}

func TestDebugGeneratedMain10Call4CFLTargetSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_CFL_TARGET_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-cfl-target-sweep.mp4",
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

	t.Setenv("DEBUG_CFL_TARGET", "0,48")

	t.Log("alphaV ctx sweep")
	for ctx := 0; ctx < 6; ctx++ {
		t.Setenv("DEBUG_CFL_ALPHA_CTX_TARGET", "")
		t.Setenv("DEBUG_CFL_ALPHA_CTX_U_TARGET", "")
		t.Setenv("DEBUG_CFL_ALPHA_CTX_V_TARGET", itoaDebugInt(ctx))
		t.Setenv("DEBUG_CFL_SIGN_TOKEN_TARGET", "")
		score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
		t.Logf("alphaV_ctx=%d y_rmse=%.4f block_mse=%.2f base_mse=%.2f v_block_mse=%.2f cfl=%v syntax=%s",
			ctx, score.yRMSE, score.blockMSE, score.baseMSE, score.vBlockMSE, score.state.syntax.CFLAlpha,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
		)
	}

	t.Log("sign token sweep")
	for signTok := 0; signTok <= 6; signTok++ {
		t.Setenv("DEBUG_CFL_ALPHA_CTX_TARGET", "")
		t.Setenv("DEBUG_CFL_ALPHA_CTX_U_TARGET", "")
		t.Setenv("DEBUG_CFL_ALPHA_CTX_V_TARGET", "")
		t.Setenv("DEBUG_CFL_SIGN_TOKEN_TARGET", itoaDebugInt(signTok))
		score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
		t.Logf("signTok=%d y_rmse=%.4f block_mse=%.2f base_mse=%.2f v_block_mse=%.2f cfl=%v syntax=%s",
			signTok, score.yRMSE, score.blockMSE, score.baseMSE, score.vBlockMSE, score.state.syntax.CFLAlpha,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
		)
	}
}

func TestDebugGeneratedMain10EarlyCFLCDFTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_EARLY_CFL_CDF_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-early-cfl-cdf-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})
	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) <= 4 {
		t.Fatalf("need at least 5 traced entries, got %d", len(entries))
	}

	overrides := make(map[int]debugIntraSyntaxReader, 5)
	for call := 0; call <= 4; call++ {
		overrides[call] = debugTraceCFLCDFReader(t, call)
	}
	debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, overrides)
}

func TestDebugGeneratedMain10Call4CFLCDFPerCall(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_CFL_CDF_PER_CALL") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-cfl-cdf-per-call.mp4",
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
	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) <= 4 {
		t.Fatalf("need at least 5 traced entries, got %d", len(entries))
	}

	baseline := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f syntax=%s",
		baseline.yRMSE, baseline.blockMSE, baseline.uBlockMSE, baseline.vBlockMSE, baseline.baseMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
	)

	type variant struct {
		name  string
		reset debugCFLCDFReset
	}
	variants := []variant{
		{name: "reset_cfl_sign", reset: debugCFLCDFReset{sign: true}},
		{name: "reset_cfl_alpha", reset: debugCFLCDFReset{alpha: true}},
		{name: "reset_cfl_all", reset: debugCFLCDFReset{sign: true, alpha: true}},
	}

	for _, variant := range variants {
		firstImprovingCall := -1
		for call := 0; call < 4; call++ {
			entry := entries[call]
			if entry.Syntax.UVMode != IntraPredCFL {
				continue
			}
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, map[int]debugIntraSyntaxReader{
				call: debugCFLCDFOverrideReader(variant.reset),
			})
			improved := score.blockMSE < baseline.blockMSE || score.baseMSE < baseline.baseMSE || score.yRMSE < baseline.yRMSE
			if improved && firstImprovingCall < 0 {
				firstImprovingCall = call
			}
			t.Logf("%s call=%d %s -> y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f syntax=%s",
				variant.name,
				call,
				formatLeafTraceEntry(entry),
				score.yRMSE,
				score.blockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		}
		t.Logf("%s first_improving_call=%d", variant.name, firstImprovingCall)
	}
}

func TestDebugGeneratedMain10Call4PaletteCDFPerCall(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_CDF_PER_CALL") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-cdf-per-call.mp4",
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
	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) <= 4 {
		t.Fatalf("need at least 5 traced entries, got %d", len(entries))
	}

	baseline := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f syntax=%s",
		baseline.yRMSE, baseline.blockMSE, baseline.uBlockMSE, baseline.vBlockMSE, baseline.baseMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
	)

	type variant struct {
		name  string
		reset debugPaletteCDFReset
	}
	variants := []variant{
		{name: "reset_pal_y", reset: debugPaletteCDFReset{palY: true}},
		{name: "reset_pal_size", reset: debugPaletteCDFReset{palSize: true}},
		{name: "reset_palette_all", reset: debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true}},
	}

	for _, variant := range variants {
		firstImprovingCall := -1
		for call := 0; call < 4; call++ {
			entry := entries[call]
			if entry.Syntax.PalYSize == 0 {
				continue
			}
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, map[int]debugIntraSyntaxReader{
				call: debugPaletteCDFOverrideReader(variant.reset),
			})
			sameGeometry := score.state.syntax.Geometry == baseline.state.syntax.Geometry
			improved := sameGeometry && (score.blockMSE < baseline.blockMSE || score.baseMSE < baseline.baseMSE || score.yRMSE < baseline.yRMSE)
			if improved && firstImprovingCall < 0 {
				firstImprovingCall = call
			}
			t.Logf("%s call=%d same_geom=%v %s -> y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f syntax=%s",
				variant.name,
				call,
				sameGeometry,
				formatLeafTraceEntry(entry),
				score.yRMSE,
				score.blockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		}
		t.Logf("%s first_improving_call=%d", variant.name, firstImprovingCall)
	}
}
