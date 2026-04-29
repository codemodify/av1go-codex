package decoder

import (
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func debugPaletteSlotValues(entry [32][8]uint16, size [32]uint8, idx int) []uint16 {
	if idx < 0 || idx >= len(size) {
		return nil
	}
	n := int(size[idx])
	if n <= 0 {
		return nil
	}
	return append([]uint16(nil), entry[idx][:n]...)
}

func debugReadIntraBlockSyntaxWithPaletteContextMutation(
	mutate func(above, left *BlockContext, bx4, by4 int),
) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if mutate != nil {
			mutate(above, left, g.Start4X&31, g.Start4Y&31)
		}
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	}
}

func TestDebugGeneratedMain10Call4PaletteContextProvenance(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_CONTEXT") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-context.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	captured := make(map[int]debugTargetLeafAdvanceState, 5)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, state debugTargetLeafAdvanceState) {
		if callIdx > 4 {
			return
		}
		captured[callIdx] = state
	})
	call4, ok := captured[4]
	if !ok {
		t.Fatal("did not capture call 4")
	}
	bx4 := call4.syntax.Geometry.Start4X & 31
	by4 := call4.syntax.Geometry.Start4Y & 31
	cache, nCache := paletteCache(&call4.entryAbove, &call4.entryLeft, 0, bx4, by4)
	t.Logf("call4 entry cache=%v bx4=%d by4=%d aboveSize=%d abovePal=%v leftSize=%d leftPal=%v",
		append([]uint16(nil), cache[:nCache]...),
		bx4,
		by4,
		call4.entryAbove.PalSz[bx4],
		debugPaletteSlotValues(call4.entryAbove.Pal[0], call4.entryAbove.PalSz, bx4),
		call4.entryLeft.PalSz[by4],
		debugPaletteSlotValues(call4.entryLeft.Pal[0], call4.entryLeft.PalSz, by4),
	)

	for call := 0; call <= 4; call++ {
		state, ok := captured[call]
		if !ok {
			t.Fatalf("did not capture call %d", call)
		}
		t.Logf("call=%d syntax=%s", call, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		t.Logf("call=%d entry above[%d]=size:%d pal:%v left[%d]=size:%d pal:%v",
			call,
			bx4,
			state.entryAbove.PalSz[bx4],
			debugPaletteSlotValues(state.entryAbove.Pal[0], state.entryAbove.PalSz, bx4),
			by4,
			state.entryLeft.PalSz[by4],
			debugPaletteSlotValues(state.entryLeft.Pal[0], state.entryLeft.PalSz, by4),
		)
		t.Logf("call=%d post  above[%d]=size:%d pal:%v left[%d]=size:%d pal:%v",
			call,
			bx4,
			state.postAbove.PalSz[bx4],
			debugPaletteSlotValues(state.postAbove.Pal[0], state.postAbove.PalSz, bx4),
			by4,
			state.postLeft.PalSz[by4],
			debugPaletteSlotValues(state.postLeft.Pal[0], state.postLeft.PalSz, by4),
		)
	}
}

func TestDebugGeneratedMain10Call4PaletteContextVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_CONTEXT_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-context-variants.mp4",
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
		{name: "clear_above", read: debugReadIntraBlockSyntaxWithPaletteContextMutation(func(above, left *BlockContext, bx4, by4 int) {
			above.PalSz[bx4] = 0
			above.Pal[0][bx4] = [8]uint16{}
		})},
		{name: "clear_left", read: debugReadIntraBlockSyntaxWithPaletteContextMutation(func(above, left *BlockContext, bx4, by4 int) {
			left.PalSz[by4] = 0
			left.Pal[0][by4] = [8]uint16{}
		})},
		{name: "clear_both", read: debugReadIntraBlockSyntaxWithPaletteContextMutation(func(above, left *BlockContext, bx4, by4 int) {
			above.PalSz[bx4] = 0
			above.Pal[0][bx4] = [8]uint16{}
			left.PalSz[by4] = 0
			left.Pal[0][by4] = [8]uint16{}
		})},
		{name: "copy_above_from_left", read: debugReadIntraBlockSyntaxWithPaletteContextMutation(func(above, left *BlockContext, bx4, by4 int) {
			above.PalSz[bx4] = left.PalSz[by4]
			above.Pal[0][bx4] = left.Pal[0][by4]
		})},
		{name: "copy_left_from_above", read: debugReadIntraBlockSyntaxWithPaletteContextMutation(func(above, left *BlockContext, bx4, by4 int) {
			left.PalSz[by4] = above.PalSz[bx4]
			left.Pal[0][by4] = above.Pal[0][bx4]
		})},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		run := debugGeneratedMain10SequenceRunForCalls(t, fixture.Path, ref, 5)
		if overrides != nil {
			run = func() debugGeneratedMain10SequenceRun {
				captured := make(map[int]debugTargetLeafAdvanceState, 6)
				yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, overrides, func(callIdx int, state debugTargetLeafAdvanceState) {
					if callIdx > 5 {
						return
					}
					captured[callIdx] = state
				})
				out := debugGeneratedMain10SequenceRun{
					yRMSE:  testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
					uRMSE:  testutil.ComparePlane16(uPlane, ref.U16).RMSE,
					vRMSE:  testutil.ComparePlane16(vPlane, ref.V16).RMSE,
					leaves: make(map[int]debugGeneratedMain10SequenceLeafScore, len(captured)),
				}
				for call, state := range captured {
					out.leaves[call] = debugGeneratedMain10SequenceLeafScoreForState(state, yPlane, yStride, uPlane, uStride, vPlane, vStride, ref, call)
				}
				return out
			}()
		}
		call4 := run.leaves[4]
		call5 := run.leaves[5]
		cache, nCache := paletteCache(&call4.state.entryAbove, &call4.state.entryLeft, 0, call4.state.syntax.Geometry.Start4X&31, call4.state.syntax.Geometry.Start4Y&31)
		call4OracleBaseMSE := -1.0
		if call4.state.syntax.PalYSize != 0 {
			blockX := call4.state.syntax.Geometry.Start4X * 4
			blockY := call4.state.syntax.Geometry.Start4Y * 4
			blockW := call4.state.syntax.Geometry.Width4() * 4
			blockH := call4.state.syntax.Geometry.Height4() * 4
			refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
			oraclePal := fitPaletteValuesToRef16(call4.state.syntax.PalYIdx, int(call4.state.syntax.PalYSize), blockW, blockH, refBlock, 10)
			call4OracleBaseMSE = paletteBaseMSE16(oraclePal, int(call4.state.syntax.PalYSize), call4.state.syntax.PalYIdx, blockW, blockH, refBlock)
		}
		t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
		t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4))
		t.Logf("%s call4 entry cache=%v aboveSize=%d abovePal=%v leftSize=%d leftPal=%v",
			tc.name,
			append([]uint16(nil), cache[:nCache]...),
			call4.state.entryAbove.PalSz[call4.state.syntax.Geometry.Start4X&31],
			debugPaletteSlotValues(call4.state.entryAbove.Pal[0], call4.state.entryAbove.PalSz, call4.state.syntax.Geometry.Start4X&31),
			call4.state.entryLeft.PalSz[call4.state.syntax.Geometry.Start4Y&31],
			debugPaletteSlotValues(call4.state.entryLeft.Pal[0], call4.state.entryLeft.PalSz, call4.state.syntax.Geometry.Start4Y&31),
		)
		t.Logf("%s call4 oracle_base_mse=%.2f", tc.name, call4OracleBaseMSE)
		t.Logf("%s call5 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call5))
	}
}

func TestDebugGeneratedMain10Call4PaletteValueVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_VALUE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-value-variants.mp4",
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
		{name: "local_no_palette_cache", read: debugReadIntraBlockSyntaxNoPaletteCache},
		{name: "force_cache_mask_0", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(0)},
		{name: "force_cache_mask_1", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(1)},
		{name: "force_cache_mask_2", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(2)},
		{name: "force_cache_mask_3", read: debugReadIntraBlockSyntaxForcePaletteCacheMask(3)},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		run := func() debugGeneratedMain10SequenceRun {
			captured := make(map[int]debugTargetLeafAdvanceState, 6)
			yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, overrides, func(callIdx int, state debugTargetLeafAdvanceState) {
				if callIdx > 5 {
					return
				}
				captured[callIdx] = state
			})
			out := debugGeneratedMain10SequenceRun{
				yRMSE:  testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
				uRMSE:  testutil.ComparePlane16(uPlane, ref.U16).RMSE,
				vRMSE:  testutil.ComparePlane16(vPlane, ref.V16).RMSE,
				leaves: make(map[int]debugGeneratedMain10SequenceLeafScore, len(captured)),
			}
			for call, state := range captured {
				out.leaves[call] = debugGeneratedMain10SequenceLeafScoreForState(state, yPlane, yStride, uPlane, uStride, vPlane, vStride, ref, call)
			}
			return out
		}()
		call4 := run.leaves[4]
		call5 := run.leaves[5]
		cache, nCache := paletteCache(&call4.state.entryAbove, &call4.state.entryLeft, 0, call4.state.syntax.Geometry.Start4X&31, call4.state.syntax.Geometry.Start4Y&31)
		t.Logf("%s frame_rmse y=%.4f u=%.4f v=%.4f", tc.name, run.yRMSE, run.uRMSE, run.vRMSE)
		t.Logf("%s call4 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call4))
		t.Logf("%s call4 entry cache=%v aboveSize=%d abovePal=%v leftSize=%d leftPal=%v",
			tc.name,
			append([]uint16(nil), cache[:nCache]...),
			call4.state.entryAbove.PalSz[call4.state.syntax.Geometry.Start4X&31],
			debugPaletteSlotValues(call4.state.entryAbove.Pal[0], call4.state.entryAbove.PalSz, call4.state.syntax.Geometry.Start4X&31),
			call4.state.entryLeft.PalSz[call4.state.syntax.Geometry.Start4Y&31],
			debugPaletteSlotValues(call4.state.entryLeft.Pal[0], call4.state.entryLeft.PalSz, call4.state.syntax.Geometry.Start4Y&31),
		)
		t.Logf("%s call5 %s", tc.name, debugGeneratedMain10SequenceLeafSummary(call5))
	}
}
