package decoder

import (
	"fmt"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

type debugGeneratedMain10SequenceLeafScore struct {
	call     int
	state    debugTargetLeafAdvanceState
	got      bool
	entryMSA [4]int
	postMSA  [4]int
	yBlock   float64
	uBlock   float64
	vBlock   float64
	baseMSE  float64
}

func (s debugGeneratedMain10SequenceLeafScore) totalBlockMSE() float64 {
	total := 0.0
	if s.yBlock >= 0 {
		total += s.yBlock
	}
	if s.uBlock >= 0 {
		total += s.uBlock
	}
	if s.vBlock >= 0 {
		total += s.vBlock
	}
	return total
}

type debugGeneratedMain10SequenceRun struct {
	yRMSE  float64
	uRMSE  float64
	vRMSE  float64
	leaves map[int]debugGeneratedMain10SequenceLeafScore
}

func debugGeneratedMain10MSACState(ctx *msac.Context) [4]int {
	if ctx == nil {
		return [4]int{-1, -1, -1, -1}
	}
	cur, rng, cnt, pos := ctx.DebugState()
	return [4]int{int(cur), int(rng), cnt, pos}
}

func debugGeneratedMain10SequenceLeafScoreForState(state debugTargetLeafAdvanceState, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, ref testutil.RawFrame420, call int) debugGeneratedMain10SequenceLeafScore {
	score := debugGeneratedMain10SequenceLeafScore{
		call:     call,
		state:    state,
		got:      true,
		entryMSA: debugGeneratedMain10MSACState(state.entryDec),
		postMSA:  debugGeneratedMain10MSACState(state.postDec),
		yBlock:   -1.0,
		uBlock:   -1.0,
		vBlock:   -1.0,
		baseMSE:  -1.0,
	}

	blockX := state.syntax.Geometry.Start4X * 4
	blockY := state.syntax.Geometry.Start4Y * 4
	blockW := state.syntax.Geometry.Width4() * 4
	blockH := state.syntax.Geometry.Height4() * 4
	if blockW > 0 && blockH > 0 {
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		score.yBlock = compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	}

	chromaGeom := chromaGeometryForBlock(av1.Chroma420, state.syntax.Geometry)
	if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
		chromaX := chromaGeom.Start4X * 4
		chromaY := chromaGeom.Start4Y * 4
		chromaW := chromaGeom.Width4() * 4
		chromaH := chromaGeom.Height4() * 4
		chromaStride := (ref.Width + 1) >> 1
		uBlock := extractBlock16(uPlane, uStride, chromaX, chromaY, chromaW, chromaH)
		vBlock := extractBlock16(vPlane, vStride, chromaX, chromaY, chromaW, chromaH)
		score.uBlock = compareBlockToRefY16(uBlock, chromaW, ref.U16, chromaStride, chromaX, chromaY, chromaW, chromaH)
		score.vBlock = compareBlockToRefY16(vBlock, chromaW, ref.V16, chromaStride, chromaX, chromaY, chromaW, chromaH)
	}

	if state.syntax.PalYSize != 0 {
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		score.baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
	}

	return score
}

func debugGeneratedMain10SequenceRunForCalls(t *testing.T, path string, ref testutil.RawFrame420, maxCall int) debugGeneratedMain10SequenceRun {
	t.Helper()

	captured := make(map[int]debugTargetLeafAdvanceState, maxCall+1)
	yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, nil, func(callIdx int, state debugTargetLeafAdvanceState) {
		if callIdx > maxCall {
			return
		}
		captured[callIdx] = state
	})

	run := debugGeneratedMain10SequenceRun{
		yRMSE:  testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
		uRMSE:  testutil.ComparePlane16(uPlane, ref.U16).RMSE,
		vRMSE:  testutil.ComparePlane16(vPlane, ref.V16).RMSE,
		leaves: make(map[int]debugGeneratedMain10SequenceLeafScore, len(captured)),
	}
	for call, state := range captured {
		run.leaves[call] = debugGeneratedMain10SequenceLeafScoreForState(state, yPlane, yStride, uPlane, uStride, vPlane, vStride, ref, call)
	}
	return run
}

func debugGeneratedMain10SequenceLeafUsesCFLOrPalette(score debugGeneratedMain10SequenceLeafScore) bool {
	return score.state.syntax.UVMode == IntraPredCFL || score.state.syntax.PalYSize != 0
}

func debugGeneratedMain10SequenceLeafDiverges(a, b debugGeneratedMain10SequenceLeafScore) bool {
	if a.got != b.got {
		return true
	}
	if !a.got {
		return false
	}
	if snapshotTargetSyntax(a.state) != snapshotTargetSyntax(b.state) {
		return true
	}
	if a.state.syntax.CFLAlpha != b.state.syntax.CFLAlpha {
		return true
	}
	if a.entryMSA != b.entryMSA || a.postMSA != b.postMSA {
		return true
	}
	return false
}

func debugGeneratedMain10SequenceLeafSummary(score debugGeneratedMain10SequenceLeafScore) string {
	if !score.got {
		return "missing"
	}
	snap := snapshotTargetSyntax(score.state)
	return fmt.Sprintf("%s cfl=%v entry=%v post=%v y_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f",
		formatTargetSyntaxSnapshot(snap),
		score.state.syntax.CFLAlpha,
		score.entryMSA,
		score.postMSA,
		score.yBlock,
		score.uBlock,
		score.vBlock,
		score.baseMSE,
	)
}

func TestDebugGeneratedMain10CFLSequenceCompare(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CFL_SEQUENCE_COMPARE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-cfl-sequence-compare.mp4",
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

	const maxCall = 10
	targetCalls := []int{4, 8, 10}

	t.Setenv("DEBUG_CFL_READ_ORDER", "")
	baseline := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, maxCall)

	t.Setenv("DEBUG_CFL_READ_ORDER", "after_luma_palette")
	afterLumaPalette := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, maxCall)

	t.Logf("baseline rmse y=%.4f u=%.4f v=%.4f", baseline.yRMSE, baseline.uRMSE, baseline.vRMSE)
	t.Logf("after_luma_palette rmse y=%.4f u=%.4f v=%.4f", afterLumaPalette.yRMSE, afterLumaPalette.uRMSE, afterLumaPalette.vRMSE)

	for _, call := range targetCalls {
		t.Logf("call=%d baseline %s", call, debugGeneratedMain10SequenceLeafSummary(baseline.leaves[call]))
		t.Logf("call=%d after_luma_palette %s", call, debugGeneratedMain10SequenceLeafSummary(afterLumaPalette.leaves[call]))
	}

	firstDivergingCall := -1
	firstImprovingCall := -1
	firstImprovingCFLOrPaletteCall := -1
	for call := 0; call <= maxCall; call++ {
		base := baseline.leaves[call]
		after := afterLumaPalette.leaves[call]
		if !debugGeneratedMain10SequenceLeafDiverges(base, after) {
			continue
		}
		if firstDivergingCall < 0 {
			firstDivergingCall = call
		}
		if after.totalBlockMSE() < base.totalBlockMSE() && firstImprovingCall < 0 {
			firstImprovingCall = call
		}
		if after.totalBlockMSE() < base.totalBlockMSE() && debugGeneratedMain10SequenceLeafUsesCFLOrPalette(base) && firstImprovingCFLOrPaletteCall < 0 {
			firstImprovingCFLOrPaletteCall = call
		}
		t.Logf("diff call=%d baseline=%s", call, debugGeneratedMain10SequenceLeafSummary(base))
		t.Logf("diff call=%d after_luma_palette=%s", call, debugGeneratedMain10SequenceLeafSummary(after))
	}

	t.Logf("first_diverging_call=%d", firstDivergingCall)
	t.Logf("first_improving_call=%d", firstImprovingCall)
	t.Logf("first_improving_cfl_or_palette_call=%d", firstImprovingCFLOrPaletteCall)
}
