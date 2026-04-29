package decoder

import (
	"fmt"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestDebugGeneratedMain10Call3ChromaStateVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CHROMA_STATE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-chroma-state-variants.mp4",
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

	var (
		call3 debugTargetLeafAdvanceState
		got3  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 3 {
			return
		}
		call3 = captured
		got3 = true
	})
	if !got3 {
		t.Fatal("did not capture call3 state")
	}
	target := itoaDebugInt(call3.syntax.Geometry.Start4X*4) + "," + itoaDebugInt(call3.syntax.Geometry.Start4Y*4)

	type variant struct {
		name       string
		swapUV     bool
		resetPlane bool
	}
	variants := []variant{
		{name: "current"},
		{name: "swap_uv_order", swapUV: true},
		{name: "reset_before_plane2", resetPlane: true},
		{name: "swap_and_reset", swapUV: true, resetPlane: true},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_SWAP_CHROMA_PLANE_ORDER_TARGET", "")
			t.Setenv("DEBUG_RESET_CHROMA_COEF_BEFORE_PLANE2_TARGET", "")
			if tc.swapUV {
				t.Setenv("DEBUG_SWAP_CHROMA_PLANE_ORDER_TARGET", target)
			}
			if tc.resetPlane {
				t.Setenv("DEBUG_RESET_CHROMA_COEF_BEFORE_PLANE2_TARGET", target)
			}

			run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 4)
			call3Score := run.leaves[3]
			call4Score := run.leaves[4]
			t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
			t.Logf("%s call3 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call3Score))
			t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4Score))
		})
	}
}

func TestDebugGeneratedMain10Call3TargetedSkipUpdateVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_TARGETED_SKIP_UPDATE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-targeted-skip-update-variants.mp4",
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

	var (
		call3 debugTargetLeafAdvanceState
		got3  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 3 {
			return
		}
		call3 = captured
		got3 = true
	})
	if !got3 {
		t.Fatal("did not capture call3 state")
	}
	chromaGeom := chromaGeometryForBlock(av1.Chroma420, call3.syntax.Geometry)
	target := fmt.Sprintf("%d,%d", chromaGeom.Start4X*4, chromaGeom.Start4Y*4)

	type variant struct {
		name   string
		planes string
	}
	variants := []variant{
		{name: "current"},
		{name: "disable_skip_update_plane1", planes: "1"},
		{name: "disable_skip_update_plane2", planes: "2"},
		{name: "disable_skip_update_both", planes: "1,2"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_COEF_SKIP_TARGET", "")
			t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "")
			t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", "")
			t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "")
			if tc.planes != "" {
				t.Setenv("DEBUG_COEF_SKIP_TARGET", target)
				t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "1")
				t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", tc.planes)
				t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "1")
			}

			run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 4)
			call3Score := run.leaves[3]
			call4Score := run.leaves[4]
			t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
			t.Logf("%s call3 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call3Score))
			t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4Score))
		})
	}
}

func TestDebugGeneratedMain10Call2TargetedSkipUpdateVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL2_TARGETED_SKIP_UPDATE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call2-targeted-skip-update-variants.mp4",
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

	var (
		call2 debugTargetLeafAdvanceState
		got2  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 2 {
			return
		}
		call2 = captured
		got2 = true
	})
	if !got2 {
		t.Fatal("did not capture call2 state")
	}
	chromaGeom := chromaGeometryForBlock(av1.Chroma420, call2.syntax.Geometry)
	target := fmt.Sprintf("%d,%d", chromaGeom.Start4X*4, chromaGeom.Start4Y*4)

	type variant struct {
		name   string
		planes string
	}
	variants := []variant{
		{name: "current"},
		{name: "disable_skip_update_plane1", planes: "1"},
		{name: "disable_skip_update_plane2", planes: "2"},
		{name: "disable_skip_update_both", planes: "1,2"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_COEF_SKIP_TARGET", "")
			t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "")
			t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", "")
			t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "")
			if tc.planes != "" {
				t.Setenv("DEBUG_COEF_SKIP_TARGET", target)
				t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "1")
				t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", tc.planes)
				t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "1")
			}

			run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 4)
			call2Score := run.leaves[2]
			call3Score := run.leaves[3]
			call4Score := run.leaves[4]
			t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
			t.Logf("%s call2 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call2Score))
			t.Logf("%s call3 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call3Score))
			t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4Score))
		})
	}
}

func TestDebugGeneratedMain10Call1TargetedSkipUpdateVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL1_TARGETED_SKIP_UPDATE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call1-targeted-skip-update-variants.mp4",
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

	var (
		call1 debugTargetLeafAdvanceState
		got1  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 1 {
			return
		}
		call1 = captured
		got1 = true
	})
	if !got1 {
		t.Fatal("did not capture call1 state")
	}
	chromaGeom := chromaGeometryForBlock(av1.Chroma420, call1.syntax.Geometry)
	target := fmt.Sprintf("%d,%d", chromaGeom.Start4X*4, chromaGeom.Start4Y*4)

	type variant struct {
		name   string
		planes string
	}
	variants := []variant{
		{name: "current"},
		{name: "disable_skip_update_plane1", planes: "1"},
		{name: "disable_skip_update_plane2", planes: "2"},
		{name: "disable_skip_update_both", planes: "1,2"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_COEF_SKIP_TARGET", "")
			t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "")
			t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", "")
			t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "")
			if tc.planes != "" {
				t.Setenv("DEBUG_COEF_SKIP_TARGET", target)
				t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "1")
				t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", tc.planes)
				t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "1")
			}

			run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 4)
			call1Score := run.leaves[1]
			call2Score := run.leaves[2]
			call3Score := run.leaves[3]
			call4Score := run.leaves[4]
			t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
			t.Logf("%s call1 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call1Score))
			t.Logf("%s call2 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call2Score))
			t.Logf("%s call3 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call3Score))
			t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4Score))
		})
	}
}

func TestDebugGeneratedMain10Call4ForcedYModeVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_FORCED_YMODE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-forced-ymode-variants.mp4",
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

	var (
		call4 debugTargetLeafAdvanceState
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 4 {
			return
		}
		call4 = captured
		got4 = true
	})
	if !got4 {
		t.Fatal("did not capture call4 state")
	}
	target := fmt.Sprintf("%d,%d", call4.syntax.Geometry.Start4X*4, call4.syntax.Geometry.Start4Y*4)

	type variant struct {
		name string
		mode IntraPredMode
		set  bool
	}
	variants := []variant{
		{name: "current"},
		{name: "force_dc", mode: IntraPredDC, set: true},
		{name: "force_vertical", mode: IntraPredVertical, set: true},
		{name: "force_smooth", mode: IntraPredSmooth, set: true},
		{name: "force_paeth", mode: IntraPredPaeth, set: true},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_YMODE_TARGET", "")
			t.Setenv("DEBUG_YMODE_FORCE_TARGET", "")
			if tc.set {
				t.Setenv("DEBUG_YMODE_TARGET", target)
				t.Setenv("DEBUG_YMODE_FORCE_TARGET", itoaDebugInt(int(tc.mode)))
			}

			run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 5)
			call4Score := run.leaves[4]
			call5Score := run.leaves[5]
			t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
			t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4Score))
			t.Logf("%s call5 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call5Score))
		})
	}
}

func TestDebugGeneratedMain10Call4YModeContextSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_YMODE_CONTEXT_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-ymode-context-sweep.mp4",
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

	var (
		call4 debugTargetLeafAdvanceState
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 4 {
			return
		}
		call4 = captured
		got4 = true
	})
	if !got4 {
		t.Fatal("did not capture call4 state")
	}
	target := fmt.Sprintf("%d,%d", call4.syntax.Geometry.Start4X*4, call4.syntax.Geometry.Start4Y*4)

	bestY := 1e30
	bestLabel := ""
	for topCtx := 0; topCtx < 5; topCtx++ {
		for leftCtx := 0; leftCtx < 5; leftCtx++ {
			t.Setenv("DEBUG_YMODE_TARGET", target)
			t.Setenv("DEBUG_YMODE_FORCE_TARGET", "")
			t.Setenv("DEBUG_YMODE_TOPCTX_TARGET", itoaDebugInt(topCtx))
			t.Setenv("DEBUG_YMODE_LEFTCTX_TARGET", itoaDebugInt(leftCtx))

			run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 5)
			call4Score := run.leaves[4]
			call5Score := run.leaves[5]
			t.Logf("topCtx=%d leftCtx=%d frame_rmse y=%.4f u=%.4f v=%.4f call4=%s call5=%s",
				topCtx, leftCtx, run.yRMSE, run.uRMSE, run.vRMSE,
				debugGeneratedMain10SequenceLeafSummary(call4Score),
				debugGeneratedMain10SequenceLeafSummary(call5Score),
			)
			if run.yRMSE < bestY {
				bestY = run.yRMSE
				bestLabel = fmt.Sprintf("topCtx=%d leftCtx=%d", topCtx, leftCtx)
			}
		}
	}
	t.Logf("best y_rmse=%.4f at %s", bestY, bestLabel)
}
