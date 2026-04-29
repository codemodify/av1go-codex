package decoder

import (
	"fmt"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugGeneratedMain10Call3And4Score struct {
	call3     debugTargetLeafAdvanceState
	call4     debugTargetLeafAdvanceState
	got3      bool
	got4      bool
	yRMSE     float64
	call3Row  int
	call3Ctx  int
	call3Prob uint16
	call3Cnt  uint16
	call3Bit  bool
	call4MSE  float64
	call4Base float64
}

type debugGeneratedMain10Call0LocalScore struct {
	call0     debugTargetLeafAdvanceState
	call1     debugTargetLeafAdvanceState
	got0      bool
	got1      bool
	yRMSE     float64
	yBlockMSE float64
	uBlockMSE float64
	vBlockMSE float64
	baseMSE   float64
	fullMSE   float64
}

func debugGeneratedMain10Call0LocalTargetScore(t *testing.T, path string, ref testutil.RawFrame420, overrides map[int]debugIntraSyntaxReader) debugGeneratedMain10Call0LocalScore {
	t.Helper()

	var (
		call0 debugTargetLeafAdvanceState
		call1 debugTargetLeafAdvanceState
		got0  bool
		got1  bool
	)
	yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 0:
			call0 = captured
			got0 = true
		case 1:
			call1 = captured
			got1 = true
		}
	})
	if !got0 || !got1 {
		t.Fatalf("captured call0=%v call1=%v", got0, got1)
	}

	score := debugGeneratedMain10Call0LocalScore{
		call0: call0,
		call1: call1,
		got0:  true,
		got1:  true,
		yRMSE: testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
	}

	blockX := call0.syntax.Geometry.Start4X * 4
	blockY := call0.syntax.Geometry.Start4Y * 4
	blockW := call0.syntax.Geometry.Width4() * 4
	blockH := call0.syntax.Geometry.Height4() * 4
	yBlock := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
	score.yBlockMSE = compareBlockToRefY16(yBlock, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

	chromaGeom := chromaGeometryForBlock(av1.Chroma420, call0.syntax.Geometry)
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
	} else {
		score.uBlockMSE = -1
		score.vBlockMSE = -1
	}

	if call0.syntax.PalYSize == 0 {
		score.baseMSE = -1
		score.fullMSE = -1
		return score
	}

	parsed := openGeneratedMain10ParsedKeyframe(t, path)
	refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	score.baseMSE = paletteBaseMSE16(call0.syntax.PalY, int(call0.syntax.PalYSize), call0.syntax.PalYIdx, blockW, blockH, refBlock)
	prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, call0, 0)
	if err != nil {
		t.Fatalf("cloneNthNonSkippedTUCoefficientInput: %v", err)
	}
	if !ok {
		score.fullMSE = score.baseMSE
		return score
	}
	decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, call0, prepared.Clone())
	score.fullMSE = paletteFullMSE16(t, call0.syntax, call0.syntax.PalY, call0.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[call0.syntax.Segment], refBlock, blockW, blockH)
	return score
}

func debugGeneratedMain10Call3And4TargetScore(t *testing.T, path string, ref testutil.RawFrame420, overrides map[int]debugIntraSyntaxReader) debugGeneratedMain10Call3And4Score {
	t.Helper()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	seq := dec.header
	dec.Close()
	parsed := openGeneratedMain10ParsedKeyframe(t, path)

	score := debugGeneratedMain10Call3And4Score{
		call3: call3,
		call4: call4,
		got3:  true,
		got4:  true,
		yRMSE: testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
	}

	mode3 := call3.mode.Clone()
	coef3 := call3.coef.Clone()
	above3 := call3.above
	left3 := call3.left
	dec3 := call3.dec.Clone()
	if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, mode3, coef3, call3.dq, &above3, &left3, call3.syntax, dec3); err != nil {
		t.Fatalf("consume call3 luma: %v", err)
	}
	layout := seq.ColorConfig.ChromaSubsampling()
	units := buildChromaTransformUnits(layout, call3.syntax.Geometry, call3.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected call3 chroma transform units")
	}
	tu := units[0]
	txInfo := TxfmInfoFor(call3.syntax.UVTX)
	bx4 := tu.Start4X & 31
	by4 := tu.Start4Y & 31
	score.call3Row = coeffCDFRow(int(txInfo.Ctx), "skip", len(coef3.Skip)-1)
	score.call3Ctx = GetSkipCtx(txInfo, call3.syntax.Size, above3.CCoef[0][bx4:], left3.CCoef[0][by4:], true, layout)
	entry := coef3.Skip[score.call3Row][score.call3Ctx]
	score.call3Prob = entry[0]
	score.call3Cnt = entry[1]
	probe := entry
	score.call3Bit = dec3.Clone().DecodeBoolAdapt(probe[:]) != 0

	blockX := call4.syntax.Geometry.Start4X * 4
	blockY := call4.syntax.Geometry.Start4Y * 4
	blockW := call4.syntax.Geometry.Width4() * 4
	blockH := call4.syntax.Geometry.Height4() * 4
	block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
	score.call4MSE = compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	if call4.syntax.PalYSize != 0 {
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		score.call4Base = paletteBaseMSE16(call4.syntax.PalY, int(call4.syntax.PalYSize), call4.syntax.PalYIdx, blockW, blockH, refBlock)
	} else {
		score.call4Base = -1
	}
	return score
}

func TestDebugGeneratedMain10PaletteInputs(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_INPUTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-inputs.mp4",
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

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	parsed, state := generatedKeyframeLeafState(t, dec, 64, 0, 68, 0)
	if state.syntax.PalYSize == 0 {
		t.Fatal("expected palette target syntax")
	}
	prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
	if err != nil {
		t.Fatalf("cloneNthNonSkippedTUCoefficientInput: %v", err)
	}
	if !ok {
		t.Fatal("expected at least one non-skipped TU in target block")
	}
	current, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())

	blockW := state.syntax.Geometry.Width4() * 4
	blockH := state.syntax.Geometry.Height4() * 4
	blockX := state.syntax.Geometry.Start4X * 4
	blockY := state.syntax.Geometry.Start4Y * 4
	refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	cache, nCache := paletteCache(&state.above, &state.left, 0, state.syntax.Geometry.Start4X&31, state.syntax.Geometry.Start4Y&31)
	cacheVals := append([]uint16(nil), cache[:nCache]...)
	unionVals := uniqueSortedUint16(append(cacheVals, state.syntax.PalY[:state.syntax.PalYSize]...))
	t.Logf("target geom=(%d,%d)-(%d,%d) pal=%v cache=%v idx0=%v",
		blockX, blockY, blockX+blockW, blockY+blockH,
		state.syntax.PalY[:state.syntax.PalYSize],
		cacheVals,
		append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
	)

	type idxVariant struct {
		name string
		idx  []uint8
	}
	variants := []idxVariant{
		{name: "current", idx: append([]uint8(nil), state.syntax.PalYIdx...)},
		{name: "reverse_diag", idx: paletteIndicesReverseDiagonals(state.syntax.PalYIdx, blockW, blockH)},
		{name: "diag_to_row_major", idx: paletteIndicesDiagonalToRowMajor(state.syntax.PalYIdx, blockW, blockH)},
	}

	lossless := parsed.Header.Segmentation.Lossless[state.syntax.Segment]
	for _, variant := range variants {
		if len(variant.idx) != len(state.syntax.PalYIdx) {
			t.Logf("%s idx variant unavailable", variant.name)
			continue
		}
		currentBaseMSE := paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), variant.idx, blockW, blockH, refBlock)
		currentFullMSE := paletteFullMSE16(t, state.syntax, state.syntax.PalY, variant.idx, txType, current.coeffs, ref.BitDepth, lossless, refBlock, blockW, blockH)
		oraclePal := fitPaletteValuesToRef16(variant.idx, int(state.syntax.PalYSize), blockW, blockH, refBlock, ref.BitDepth)
		oracleBaseMSE := paletteBaseMSE16(oraclePal, int(state.syntax.PalYSize), variant.idx, blockW, blockH, refBlock)
		bestUnionPal, bestUnionBaseMSE := bestPaletteAssignmentFromSet16(variant.idx, int(state.syntax.PalYSize), blockW, blockH, refBlock, unionVals)
		bestUnionFullMSE := paletteFullMSE16(t, state.syntax, bestUnionPal, variant.idx, txType, current.coeffs, ref.BitDepth, lossless, refBlock, blockW, blockH)

		t.Logf("%s current base_mse=%.2f full_mse=%.2f pal=%v", variant.name, currentBaseMSE, currentFullMSE, state.syntax.PalY[:state.syntax.PalYSize])
		t.Logf("%s oracle  base_mse=%.2f pal=%v", variant.name, oracleBaseMSE, oraclePal[:state.syntax.PalYSize])
		t.Logf("%s union   base_mse=%.2f full_mse=%.2f pal=%v values=%v", variant.name, bestUnionBaseMSE, bestUnionFullMSE, bestUnionPal[:state.syntax.PalYSize], unionVals)
	}
}

func TestDebugGeneratedMain10EarlyPaletteBlocks(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_EARLY_PALETTE_BLOCKS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-early-palette-blocks.mp4",
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

	type target struct {
		name  string
		x     int
		y     int
		nextX int
		nextY int
	}
	targets := []target{
		{name: "call0_block", x: 0, y: 0, nextX: 32, nextY: 0},
		{name: "call1_block", x: 32, y: 0, nextX: 64, nextY: 0},
		{name: "call2_block", x: 0, y: 32, nextX: 16, nextY: 32},
	}

	for _, tc := range targets {
		dec, err := OpenMP4(fixture.Path)
		if err != nil {
			t.Fatalf("%s OpenMP4: %v", tc.name, err)
		}
		parsed, state := generatedKeyframeLeafState(t, dec, tc.x, tc.y, tc.nextX, tc.nextY)
		dec.Close()

		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		baseMSE := -1.0
		fullMSE := -1.0
		txLabel := "n/a"
		if state.syntax.PalYSize != 0 {
			baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
			if err != nil {
				t.Fatalf("%s cloneNthNonSkippedTUCoefficientInput: %v", tc.name, err)
			}
			if ok {
				decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
				fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
				txLabel = "txType=" + itoaDebugInt(int(txType))
			}
		}

		t.Logf("%s geom=(%d,%d)-(%d,%d) syntax=%s", tc.name, blockX, blockY, blockX+blockW, blockY+blockH, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		if state.syntax.PalYSize != 0 {
			t.Logf("%s palette base_mse=%.2f full_mse=%.2f tx=%s pal=%v idx0=%v",
				tc.name,
				baseMSE,
				fullMSE,
				txLabel,
				state.syntax.PalY[:state.syntax.PalYSize],
				append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
			)
		}
	}
}

func TestDebugGeneratedMain10PaletteBearingLeafSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_LEAF_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-leaf-sweep.mp4",
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
	if len(entries) < 2 {
		t.Fatalf("need at least two traced leaves, got %d", len(entries))
	}
	yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, nil)

	for i := 0; i < len(entries)-1; i++ {
		entry := entries[i]
		if entry.Syntax.PalYSize == 0 {
			continue
		}

		next := entries[i+1]
		targetX := entry.Geometry.Start4X * 4
		targetY := entry.Geometry.Start4Y * 4
		nextX := next.Geometry.Start4X * 4
		nextY := next.Geometry.Start4Y * 4

		dec, err := OpenMP4(fixture.Path)
		if err != nil {
			t.Fatalf("call %d OpenMP4: %v", i, err)
		}
		parsed, state := generatedKeyframeLeafState(t, dec, targetX, targetY, nextX, nextY)
		dec.Close()

		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		baseMSE := paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
		fullMSE := -1.0
		txLabel := "n/a"
		if prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0); err != nil {
			t.Fatalf("call %d cloneNthNonSkippedTUCoefficientInput: %v", i, err)
		} else if ok {
			decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
			fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
			txLabel = "txType=" + itoaDebugInt(int(txType))
		}
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		blockMSE := compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

		t.Logf("call=%d %s", i, formatLeafTraceEntry(entry))
		t.Logf("call=%d palette base_mse=%.2f full_mse=%.2f block_mse=%.2f tx=%s pal=%v idx0=%v",
			i,
			baseMSE,
			fullMSE,
			blockMSE,
			txLabel,
			state.syntax.PalY[:state.syntax.PalYSize],
			append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
		)
	}
}

func TestDebugGeneratedMain10EarlyLeafBlockMSE(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_EARLY_LEAF_BLOCK_MSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-early-leaf-block-mse.mp4",
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
	yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, nil)

	for i := 0; i < len(entries) && i <= 10; i++ {
		entry := entries[i]
		blockX := entry.Geometry.Start4X * 4
		blockY := entry.Geometry.Start4Y * 4
		blockW := entry.Geometry.Width4() * 4
		blockH := entry.Geometry.Height4() * 4
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		blockMSE := compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		t.Logf("call=%d block_mse=%.2f %s", i, blockMSE, formatLeafTraceEntry(entry))
	}
}

func TestDebugGeneratedMain10Call3ToCall4EntropyBoundary(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CALL4_BOUNDARY") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-call4-boundary.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) <= 5 {
		t.Fatalf("need at least 6 traced leaves, got %d", len(entries))
	}
	call3Entry := entries[3]
	call4Entry := entries[4]
	call5Entry := entries[5]

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4 call3: %v", err)
	}
	parsed, call3 := generatedKeyframeLeafState(t, dec,
		call3Entry.Geometry.Start4X*4, call3Entry.Geometry.Start4Y*4,
		call4Entry.Geometry.Start4X*4, call4Entry.Geometry.Start4Y*4,
	)
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	dec, err = OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4 call4: %v", err)
	}
	_, call4 := generatedKeyframeLeafState(t, dec,
		call4Entry.Geometry.Start4X*4, call4Entry.Geometry.Start4Y*4,
		call5Entry.Geometry.Start4X*4, call5Entry.Geometry.Start4Y*4,
	)
	dec.Close()

	targetX := call4Entry.Geometry.Start4X * 4
	targetY := call4Entry.Geometry.Start4Y * 4

	formatMode := func(mode IntraPredMode, chroma bool) string {
		if chroma && mode == IntraPredCFL {
			return "CFL"
		}
		if !chroma && mode == IntraPredFilter {
			return "FILTER"
		}
		return intraModeName(mode)
	}
	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, formatMode(syntax.YMode, false), formatMode(syntax.UVMode, true),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntaxAfterCDEF := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, g BlockGeometry, dec *msac.Context) (IntraBlockSyntax, bool, error) {
		leaf, err := debugDescendToPointLeaf(seq, &parsed.Header, cdf, above, left, g, dec, targetX, targetY)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		local := leaf
		segID := uint8(0)
		segPred := false
		haveTop := leaf.Start4Y > 0
		haveLeft := leaf.Start4X > 0
		msacDec := msacEntropyDecoder{ctx: dec}
		if parsed.Header.Segmentation.Enabled && parsed.Header.Segmentation.Preskip {
			segID, segPred, err = readBlockSegmentID(&parsed.Header, cdf, above, left, segState, leaf, haveTop, haveLeft, false, false, msacDec)
			if err != nil {
				return IntraBlockSyntax{}, false, err
			}
		}
		_, skip, err := ReadIntraSkipPrelude(&parsed.Header, cdf, above, left, local, msacDec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		if parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.Preskip {
			segID, segPred, err = readBlockSegmentID(&parsed.Header, cdf, above, left, segState, leaf, haveTop, haveLeft, true, skip, msacDec)
			if err != nil {
				return IntraBlockSyntax{}, false, err
			}
		}
		if segmentDataForID(&parsed.Header, segID).Skip {
			skip = true
		}
		tileSyntax := &TileSyntaxState{
			Tile: TileState{
				Entropy: dec,
				CDF:     cdf,
				Coef:    coefCDF,
			},
			LastQIdx:   call3.qidx,
			LastLF:     call3.lastLF,
			ReadDeltas: call3.readDeltas,
			CurrentDQ:  call3.dq,
		}
		tileSyntax.Tile.CoefByQ[DefaultCoefQCat(uint8(clipQIndexRuntime(call3.qidx)))] = coefCDF
		if err := readDeltaQAndLF(seq, &parsed.Header, tileSyntax, leaf, skip); err != nil {
			return IntraBlockSyntax{}, false, err
		}
		syntax, err := ReadIntraBlockSyntax(seq, &parsed.Header, tileSyntax.Tile.CDF, above, left, local, haveTop, haveLeft, segID, msacDec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		syntax.Geometry = leaf
		syntax.SegPred = segPred
		syntax.Skip = skip
		return syntax, skip, nil
	}

	t.Logf("call3 syntax=%s", formatSyntax(call3.syntax))
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error
	}
	scenarios := []scenario{
		{
			name: "actual_luma_path",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				width := parsed.Header.Width
				height := parsed.Header.Height
				plane := make([]uint16, width*height)
				chromaWidth := (width + 1) >> 1
				chromaHeight := (height + 1) >> 1
				uPlane := make([]uint16, chromaWidth*chromaHeight)
				vPlane := make([]uint16, chromaWidth*chromaHeight)
				fillUint16(uPlane, 1<<(seq.ColorConfig.BitDepth-1))
				fillUint16(vPlane, 1<<(seq.ColorConfig.BitDepth-1))
				if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
					seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax,
					plane, width, width, height, call3.syntax.Geometry.Start4X, call3.syntax.Geometry.Start4Y,
					seq.ColorConfig.BitDepth, call3.syntax.Skip, dec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
				); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "actual_full_path",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				width := parsed.Header.Width
				height := parsed.Header.Height
				plane := make([]uint16, width*height)
				chromaWidth := (width + 1) >> 1
				chromaHeight := (height + 1) >> 1
				uPlane := make([]uint16, chromaWidth*chromaHeight)
				vPlane := make([]uint16, chromaWidth*chromaHeight)
				fillUint16(uPlane, 1<<(seq.ColorConfig.BitDepth-1))
				fillUint16(vPlane, 1<<(seq.ColorConfig.BitDepth-1))
				if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
					seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax,
					plane, width, width, height, call3.syntax.Geometry.Start4X, call3.syntax.Geometry.Start4Y,
					seq.ColorConfig.BitDepth, call3.syntax.Skip, dec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
				); err != nil {
					return err
				}
				if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(
					seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax,
					plane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, chromaWidth, chromaHeight,
					seq.ColorConfig.BitDepth, call3.syntax.Skip, dec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
				); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "syntax_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_first_1_tu",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLumaLimit(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec, 1); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_u",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_v",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "full_residual",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
	}

	for _, tc := range scenarios {
		modeCDF := call3.mode.Clone()
		coefCDF := call3.coef.Clone()
		above := call3.above
		left := call3.left
		stateDec := call3.dec.Clone()
		if err := tc.apply(modeCDF, coefCDF, &above, &left, stateDec); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		nextSyntax, nextSkip, err := peekNextLeafSyntaxAfterCDEF(modeCDF, coefCDF, &above, &left, call3.nextRoot, stateDec.Clone())
		if err != nil {
			t.Fatalf("%s peek next syntax: %v", tc.name, err)
		}
		nextSyntax.Skip = nextSkip
		t.Logf("%s -> call4 %s", tc.name, formatSyntax(nextSyntax))
	}
}

func TestDebugGeneratedMain10Call3ToCall4EntropyBoundaryFromFullDecode(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CALL4_BOUNDARY_FULL") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-call4-boundary-full.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	formatMode := func(mode IntraPredMode, chroma bool) string {
		if chroma && mode == IntraPredCFL {
			return "CFL"
		}
		if !chroma && mode == IntraPredFilter {
			return "FILTER"
		}
		return intraModeName(mode)
	}
	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, formatMode(syntax.YMode, false), formatMode(syntax.UVMode, true),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntax := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context, state debugTargetLeafAdvanceState) (IntraBlockSyntax, bool, error) {
		nextState, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, state, cdf, coefCDF, above, left, dec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		return nextState.syntax, nextState.syntax.Skip, nil
	}

	t.Logf("call3 nextRoot=(%d,%d)-(%d,%d) syntax=%s",
		call3.nextRoot.Start4X*4, call3.nextRoot.Start4Y*4, call3.nextRoot.End4X*4, call3.nextRoot.End4Y*4,
		formatSyntax(call3.syntax),
	)
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error
	}
	scenarios := []scenario{
		{
			name: "actual_luma_path",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				width := parsed.Header.Width
				height := parsed.Header.Height
				plane := make([]uint16, width*height)
				chromaWidth := (width + 1) >> 1
				chromaHeight := (height + 1) >> 1
				uPlane := make([]uint16, chromaWidth*chromaHeight)
				vPlane := make([]uint16, chromaWidth*chromaHeight)
				fillUint16(uPlane, 1<<(seq.ColorConfig.BitDepth-1))
				fillUint16(vPlane, 1<<(seq.ColorConfig.BitDepth-1))
				if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
					seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax,
					plane, width, width, height, call3.syntax.Geometry.Start4X, call3.syntax.Geometry.Start4Y,
					seq.ColorConfig.BitDepth, call3.syntax.Skip, dec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
				); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "actual_full_path",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				width := parsed.Header.Width
				height := parsed.Header.Height
				plane := make([]uint16, width*height)
				chromaWidth := (width + 1) >> 1
				chromaHeight := (height + 1) >> 1
				uPlane := make([]uint16, chromaWidth*chromaHeight)
				vPlane := make([]uint16, chromaWidth*chromaHeight)
				fillUint16(uPlane, 1<<(seq.ColorConfig.BitDepth-1))
				fillUint16(vPlane, 1<<(seq.ColorConfig.BitDepth-1))
				if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
					seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax,
					plane, width, width, height, call3.syntax.Geometry.Start4X, call3.syntax.Geometry.Start4Y,
					seq.ColorConfig.BitDepth, call3.syntax.Skip, dec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
				); err != nil {
					return err
				}
				if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(
					seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax,
					plane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, chromaWidth, chromaHeight,
					seq.ColorConfig.BitDepth, call3.syntax.Skip, dec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
				); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "syntax_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_first_1_tu",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLumaLimit(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec, 1); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_u",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_v",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "full_residual",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
	}
	for _, tc := range scenarios {
		modeCDF := call3.mode.Clone()
		coefCDF := call3.coef.Clone()
		above := call3.above
		left := call3.left
		stateDec := call3.dec.Clone()
		if err := tc.apply(modeCDF, coefCDF, &above, &left, stateDec); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		nextSyntax, nextSkip, err := peekNextLeafSyntax(modeCDF, coefCDF, &above, &left, stateDec.Clone(), call3)
		if err != nil {
			t.Fatalf("%s peek next syntax: %v", tc.name, err)
		}
		nextSyntax.Skip = nextSkip
		t.Logf("%s -> call4 %s", tc.name, formatSyntax(nextSyntax))
	}
}

func TestDebugGeneratedMain10Call3UVTxTypeSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_UVTXTYPE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-uvtxtype-sweep.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	formatMode := func(mode IntraPredMode, chroma bool) string {
		if chroma && mode == IntraPredCFL {
			return "CFL"
		}
		if !chroma && mode == IntraPredFilter {
			return "FILTER"
		}
		return intraModeName(mode)
	}
	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, formatMode(syntax.YMode, false), formatMode(syntax.UVMode, true),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntax := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context, state debugTargetLeafAdvanceState) (IntraBlockSyntax, bool, error) {
		nextState, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, state, cdf, coefCDF, above, left, dec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		return nextState.syntax, nextState.syntax.Skip, nil
	}

	chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), call3.syntax.Geometry)
	txInfo := TxfmInfoFor(call3.syntax.UVTX)
	currentUVTxType := IntraUVTxType(&parsed.Header, call3.syntax.Segment, call3.syntax.UVTX, call3.syntax.UVMode)
	t.Logf("call3 syntax=%s current_uvtxtype=%d", formatSyntax(call3.syntax), currentUVTxType)
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))

	for _, uvTxType := range debugReplayAltTxTypes(call3.syntax.UVTX) {
		modeCDF := call3.mode.Clone()
		coefCDF := call3.coef.Clone()
		above := call3.above
		left := call3.left
		stateDec := call3.dec.Clone()

		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, &above, &left, call3.syntax, stateDec); err != nil {
			t.Fatalf("uvtxType=%d consume luma: %v", uvTxType, err)
		}
		if err := debugConsumeFirstBlockChromaPlanesWithGeom(seq, &parsed.Header, coefCDF, call3.dq, &above, &left, call3.syntax, chromaGeom, txInfo, uvTxType, stateDec, []int{1, 2}); err != nil {
			t.Logf("uvtxType=%d consume chroma err=%v", uvTxType, err)
			continue
		}
		if err := debugApplyIntraBlockContextReplay(seq, &parsed.Header, &above, &left, call3.syntax); err != nil {
			t.Fatalf("uvtxType=%d apply context: %v", uvTxType, err)
		}
		nextSyntax, nextSkip, err := peekNextLeafSyntax(modeCDF, coefCDF, &above, &left, stateDec.Clone(), call3)
		if err != nil {
			t.Logf("uvtxType=%d next syntax err=%v", uvTxType, err)
			continue
		}
		nextSyntax.Skip = nextSkip
		t.Logf("uvtxType=%d -> call4 %s", uvTxType, formatSyntax(nextSyntax))
	}
}

func TestDebugGeneratedMain10TxTypeModelInputSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_TXTYPE_MODEL_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-txtype-model-sweep.mp4",
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
		env  map[string]string
	}
	variants := []variant{
		{name: "baseline"},
		{name: "mode_raw", env: map[string]string{"DEBUG_INTRA_TXTYPE_MODE": "raw"}},
		{name: "mode_dc", env: map[string]string{"DEBUG_INTRA_TXTYPE_MODE": "dc"}},
		{name: "neighbors_both", env: map[string]string{"DEBUG_INTRA_TXTYPE_NEIGHBORS": "both"}},
		{name: "neighbors_none", env: map[string]string{"DEBUG_INTRA_TXTYPE_NEIGHBORS": "none"}},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"DEBUG_INTRA_TXTYPE_MODE", "DEBUG_INTRA_TXTYPE_NEIGHBORS"} {
				t.Setenv(key, "")
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s y_rmse=%.4f call4_block_mse=%.2f call4_base_mse=%.2f syntax=%s",
				tc.name,
				score.yRMSE,
				score.blockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call3TXSizeSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_TXSIZE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-txsize-sweep.mp4",
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

	variants := []struct {
		name      string
		overrides map[int]debugIntraSyntaxReader
	}{
		{name: "baseline"},
		{name: "leaf3_tx4x4", overrides: map[int]debugIntraSyntaxReader{3: debugReadIntraBlockSyntaxForceTX(TX4X4)}},
		{name: "leaf3_tx16x16", overrides: map[int]debugIntraSyntaxReader{3: debugReadIntraBlockSyntaxForceTX(TX16X16)}},
	}
	for _, tc := range variants {
		score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, tc.overrides)
		t.Logf("%s y_rmse=%.4f call4_block_mse=%.2f call4_base_mse=%.2f syntax=%s",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.baseMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
		)
	}
}

func TestDebugGeneratedMain10Call3LumaTxTypeBoundarySweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_LUMA_TXTYPE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-luma-txtype-sweep.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	formatMode := func(mode IntraPredMode, chroma bool) string {
		if chroma && mode == IntraPredCFL {
			return "CFL"
		}
		if !chroma && mode == IntraPredFilter {
			return "FILTER"
		}
		return intraModeName(mode)
	}
	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, formatMode(syntax.YMode, false), formatMode(syntax.UVMode, true),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntax := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context, state debugTargetLeafAdvanceState) (IntraBlockSyntax, bool, error) {
		nextState, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, state, cdf, coefCDF, above, left, dec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		return nextState.syntax, nextState.syntax.Skip, nil
	}

	t.Logf("call3 syntax=%s", formatSyntax(call3.syntax))
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))
	for _, tuIndex := range []int{0, 2} {
		for _, forced := range debugReplayAltTxTypes(call3.syntax.TX) {
			modeCDF := call3.mode.Clone()
			coefCDF := call3.coef.Clone()
			above := call3.above
			left := call3.left
			stateDec := call3.dec.Clone()
			if err := debugConsumeFirstBlockLumaLimitWithTXTypeOverrides(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, &above, &left, call3.syntax, stateDec, -1, map[int]TxType{tuIndex: forced}); err != nil {
				t.Fatalf("tu=%d txType=%d consume luma: %v", tuIndex, forced, err)
			}
			if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call3.dq, &above, &left, call3.syntax, stateDec, false); err != nil {
				t.Fatalf("tu=%d txType=%d consume chroma: %v", tuIndex, forced, err)
			}
			if err := debugApplyIntraBlockContextReplay(seq, &parsed.Header, &above, &left, call3.syntax); err != nil {
				t.Fatalf("tu=%d txType=%d apply context: %v", tuIndex, forced, err)
			}
			nextSyntax, nextSkip, err := peekNextLeafSyntax(modeCDF, coefCDF, &above, &left, stateDec.Clone(), call3)
			if err != nil {
				t.Fatalf("tu=%d txType=%d peek next syntax: %v", tuIndex, forced, err)
			}
			nextSyntax.Skip = nextSkip
			t.Logf("tu=%d txType=%d -> call4 %s", tuIndex, forced, formatSyntax(nextSyntax))
		}
	}
}

func TestDebugGeneratedMain10Call3ChromaBoundaryVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CHROMA_BOUNDARY_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-chroma-boundary-variants.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	formatMode := func(mode IntraPredMode, chroma bool) string {
		if chroma && mode == IntraPredCFL {
			return "CFL"
		}
		if !chroma && mode == IntraPredFilter {
			return "FILTER"
		}
		return intraModeName(mode)
	}
	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, formatMode(syntax.YMode, false), formatMode(syntax.UVMode, true),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntax := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context, state debugTargetLeafAdvanceState) (IntraBlockSyntax, bool, error) {
		nextState, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, state, cdf, coefCDF, above, left, dec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		return nextState.syntax, nextState.syntax.Skip, nil
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, call3.syntax.Segment, call3.syntax.UVTX, call3.syntax.UVMode)
	units := buildChromaTransformUnits(layout, call3.syntax.Geometry, call3.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected chroma transform units for call3")
	}

	type scenario struct {
		name            string
		planes          []int
		applyCtx        bool
		resetSkipBefore int
		resetAllBefore  int
	}
	scenarios := []scenario{
		{name: "current", planes: []int{1, 2}, applyCtx: true},
		{name: "swapped", planes: []int{2, 1}, applyCtx: true},
		{name: "no_apply_ctx", planes: []int{1, 2}, applyCtx: false},
		{name: "reset_skip_before_v", planes: []int{1, 2}, applyCtx: true, resetSkipBefore: 1},
		{name: "reset_all_coef_before_v", planes: []int{1, 2}, applyCtx: true, resetAllBefore: 1},
	}

	t.Logf("call3 syntax=%s", formatSyntax(call3.syntax))
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))
	for _, tc := range scenarios {
		modeCDF := call3.mode.Clone()
		coefCDF := call3.coef.Clone()
		above := call3.above
		left := call3.left
		stateDec := call3.dec.Clone()
		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, &above, &left, call3.syntax, stateDec); err != nil {
			t.Fatalf("%s consume luma: %v", tc.name, err)
		}
		coefAfterLuma := coefCDF.Clone()
		for step, plane := range tc.planes {
			if tc.resetAllBefore == step {
				*coefCDF = *coefAfterLuma.Clone()
			} else if tc.resetSkipBefore == step {
				coefCDF.Skip = coefAfterLuma.Skip
			}
			for tuIndex, tu := range units {
				coeffs, resCtx, skip, err := DecodeChromaCoefficients(call3.syntax.Size, tu, call3.syntax.UVTX, uvTxType, call3.syntax.Segment, plane, layout, coefCDF, call3.dq, &above, &left, stateDec)
				if err != nil {
					t.Fatalf("%s plane=%d tu=%d decode: %v", tc.name, plane, tuIndex, err)
				}
				t.Logf("%s plane=%d tu=%d skip=%v resCtx=%#x coeffs=%v", tc.name, plane, tuIndex, skip, resCtx, summarizeNonZeroCoeffs(coeffs))
				if tc.applyCtx {
					if err := ApplyChromaResidualContext(&above, &left, plane, tu, call3.syntax.UVTX, resCtx); err != nil {
						t.Fatalf("%s plane=%d tu=%d apply ctx: %v", tc.name, plane, tuIndex, err)
					}
				}
			}
		}
		if err := debugApplyIntraBlockContextReplay(seq, &parsed.Header, &above, &left, call3.syntax); err != nil {
			t.Fatalf("%s apply block context: %v", tc.name, err)
		}
		nextSyntax, nextSkip, err := peekNextLeafSyntax(modeCDF, coefCDF, &above, &left, stateDec.Clone(), call3)
		if err != nil {
			t.Fatalf("%s peek next syntax: %v", tc.name, err)
		}
		nextSyntax.Skip = nextSkip
		t.Logf("%s -> call4 %s", tc.name, formatSyntax(nextSyntax))
	}
}

func TestDebugGeneratedMain10Call3LocalChromaScore(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_LOCAL_SCORE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-local-score.mp4",
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
	yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 3 {
			return
		}
		call3 = captured
		got3 = true
	})
	if !got3 {
		t.Fatal("did not capture call3 state")
	}

	blockX := call3.syntax.Geometry.Start4X * 4
	blockY := call3.syntax.Geometry.Start4Y * 4
	blockW := call3.syntax.Geometry.Width4() * 4
	blockH := call3.syntax.Geometry.Height4() * 4
	yBlock := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
	yMSE := compareBlockToRefY16(yBlock, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

	chromaGeom := chromaGeometryForBlock(av1.Chroma420, call3.syntax.Geometry)
	chromaX := chromaGeom.Start4X * 4
	chromaY := chromaGeom.Start4Y * 4
	chromaW := chromaGeom.Width4() * 4
	chromaH := chromaGeom.Height4() * 4
	uBlock := extractBlock16(uPlane, uStride, chromaX, chromaY, chromaW, chromaH)
	vBlock := extractBlock16(vPlane, vStride, chromaX, chromaY, chromaW, chromaH)
	chromaStride := (ref.Width + 1) >> 1
	uMSE := compareBlockToRefY16(uBlock, chromaW, ref.U16, chromaStride, chromaX, chromaY, chromaW, chromaH)
	vMSE := compareBlockToRefY16(vBlock, chromaW, ref.V16, chromaStride, chromaX, chromaY, chromaW, chromaH)

	t.Logf("call3 syntax=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call3)))
	t.Logf("call3 y_block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f", yMSE, uMSE, vMSE)
}

func TestDebugGeneratedMain10Call3ChromaSkipContextSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CHROMA_SKIPCTX_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-chroma-skipctx-sweep.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	formatMode := func(mode IntraPredMode, chroma bool) string {
		if chroma && mode == IntraPredCFL {
			return "CFL"
		}
		if !chroma && mode == IntraPredFilter {
			return "FILTER"
		}
		return intraModeName(mode)
	}
	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, formatMode(syntax.YMode, false), formatMode(syntax.UVMode, true),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntax := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context, state debugTargetLeafAdvanceState) (IntraBlockSyntax, bool, error) {
		nextState, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, state, cdf, coefCDF, above, left, dec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		return nextState.syntax, nextState.syntax.Skip, nil
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, call3.syntax.Segment, call3.syntax.UVTX, call3.syntax.UVMode)
	units := buildChromaTransformUnits(layout, call3.syntax.Geometry, call3.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected chroma transform units for call3")
	}
	txInfo := TxfmInfoFor(call3.syntax.UVTX)
	synthSlices := func(ctx int) ([]uint8, []uint8, bool) {
		switch ctx {
		case 7:
			return []uint8{0x40, 0x40}, []uint8{0x40, 0x40}, true
		case 8:
			return []uint8{0x80, 0x40}, []uint8{0x40, 0x40}, true
		case 9:
			return []uint8{0x80, 0x40}, []uint8{0x80, 0x40}, true
		default:
			return nil, nil, false
		}
	}

	t.Logf("call3 syntax=%s", formatSyntax(call3.syntax))
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))
	for _, uCtx := range []int{7, 8, 9} {
		for _, vCtx := range []int{7, 8, 9} {
			modeCDF := call3.mode.Clone()
			coefCDF := call3.coef.Clone()
			above := call3.above
			left := call3.left
			stateDec := call3.dec.Clone()
			if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, &above, &left, call3.syntax, stateDec); err != nil {
				t.Fatalf("uCtx=%d vCtx=%d consume luma: %v", uCtx, vCtx, err)
			}
			ctxByPlane := map[int]int{1: uCtx, 2: vCtx}
			for _, plane := range []int{1, 2} {
				ctx := ctxByPlane[plane]
				aboveCoef, leftCoef, ok := synthSlices(ctx)
				if !ok {
					t.Fatalf("unsupported synthetic ctx %d", ctx)
				}
				for tuIndex, tu := range units {
					coeffs := make([]int32, coeffBufferLen(call3.syntax.UVTX))
					resCtx, skip, err := decodeCoefficientsInto(coeffs, call3.syntax.Size, tu, call3.syntax.UVTX, uvTxType, call3.syntax.Segment, plane, true, layout, coefCDF, call3.dq, aboveCoef[:minInt(len(aboveCoef), int(txInfo.W4))], leftCoef[:minInt(len(leftCoef), int(txInfo.H4))], stateDec, true, seq.ColorConfig.BitDepth)
					if err != nil {
						t.Fatalf("uCtx=%d vCtx=%d plane=%d tu=%d decode: %v", uCtx, vCtx, plane, tuIndex, err)
					}
					t.Logf("uCtx=%d vCtx=%d plane=%d tu=%d skip=%v resCtx=%#x coeffs=%v", uCtx, vCtx, plane, tuIndex, skip, resCtx, summarizeNonZeroCoeffs(coeffs))
					if err := ApplyChromaResidualContext(&above, &left, plane, tu, call3.syntax.UVTX, resCtx); err != nil {
						t.Fatalf("uCtx=%d vCtx=%d plane=%d tu=%d apply ctx: %v", uCtx, vCtx, plane, tuIndex, err)
					}
				}
			}
			if err := debugApplyIntraBlockContextReplay(seq, &parsed.Header, &above, &left, call3.syntax); err != nil {
				t.Fatalf("uCtx=%d vCtx=%d apply block context: %v", uCtx, vCtx, err)
			}
			nextSyntax, nextSkip, err := peekNextLeafSyntax(modeCDF, coefCDF, &above, &left, stateDec.Clone(), call3)
			if err != nil {
				t.Fatalf("uCtx=%d vCtx=%d peek next syntax: %v", uCtx, vCtx, err)
			}
			nextSyntax.Skip = nextSkip
			t.Logf("uCtx=%d vCtx=%d -> call4 %s", uCtx, vCtx, formatSyntax(nextSyntax))
		}
	}
}

func TestDebugGeneratedMain10Call3ChromaSkipPolicyRMSE(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_SKIP_POLICY_RMSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-skip-policy-rmse.mp4",
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

	baseline := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
		baseline.yRMSE,
		baseline.blockMSE,
		baseline.uBlockMSE,
		baseline.vBlockMSE,
		baseline.baseMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
	)

	variants := []struct {
		name   string
		policy string
	}{
		{name: "reset_all", policy: "reset_all"},
		{name: "reset_all_prob", policy: "reset_all_prob"},
		{name: "reset_all_count", policy: "reset_all_count"},
		{name: "reset_entry", policy: "reset_entry"},
		{name: "reset_entry_prob", policy: "reset_entry_prob"},
		{name: "reset_entry_count", policy: "reset_entry_count"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "16,32")
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", tc.policy)
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
				tc.name,
				score.yRMSE,
				score.blockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call3ChromaSkipEntryTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_SKIP_ENTRY_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-skip-entry-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

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

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	dec.Close()
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}

	modeCDF := call3.mode.Clone()
	coefCDF := call3.coef.Clone()
	above := call3.above
	left := call3.left
	stateDec := call3.dec.Clone()
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, modeCDF, coefCDF, call3.dq, &above, &left, call3.syntax, stateDec); err != nil {
		t.Fatalf("consume luma: %v", err)
	}
	coefAfterLuma := coefCDF.Clone()
	layout := dec.header.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, call3.syntax.Segment, call3.syntax.UVTX, call3.syntax.UVMode)
	units := buildChromaTransformUnits(layout, call3.syntax.Geometry, call3.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected chroma transform units for call3")
	}
	txInfo := TxfmInfoFor(call3.syntax.UVTX)
	tu := units[0]
	skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coefCDF.Skip)-1)

	logEntry := func(label string, plane int) int {
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtx(txInfo, call3.syntax.Size, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], true, layout)
		entry := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("%s plane=%d skipRow=%d skipCtx=%d cdf=[%d %d]", label, plane, skipRow, skipCtx, entry[0], entry[1])
		return skipCtx
	}

	logEntry("before_u", 1)
	uCoeffs, uResCtx, uSkip, err := DecodeChromaCoefficients(call3.syntax.Size, tu, call3.syntax.UVTX, uvTxType, call3.syntax.Segment, 1, layout, coefCDF, call3.dq, &above, &left, stateDec)
	if err != nil {
		t.Fatalf("decode U: %v", err)
	}
	t.Logf("after_u_decode skip=%v resCtx=%#x coeffs=%v", uSkip, uResCtx, summarizeNonZeroCoeffs(uCoeffs))
	if err := ApplyChromaResidualContext(&above, &left, 1, tu, call3.syntax.UVTX, uResCtx); err != nil {
		t.Fatalf("apply U residual context: %v", err)
	}
	logEntry("after_u", 1)
	logEntry("before_v", 2)

	debugCopySkipCDFEntry(coefCDF, coefAfterLuma, skipRow, logEntry("baseline_v_entry", 2), true, false)
	logEntry("before_v_prob_reset", 2)
}

func TestDebugGeneratedMain10Call3ChromaSkipSourceSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_SKIP_SOURCE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-skip-source-sweep.mp4",
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

	baseline := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
		baseline.yRMSE,
		baseline.blockMSE,
		baseline.uBlockMSE,
		baseline.vBlockMSE,
		baseline.baseMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
	)

	variants := []struct {
		name   string
		source string
	}{
		{name: "row1_ctx8_prob", source: "1,8"},
		{name: "row1_ctx9_prob", source: "1,9"},
		{name: "row0_ctx7_prob", source: "0,7"},
		{name: "row2_ctx7_prob", source: "2,7"},
		{name: "row0_ctx8_prob", source: "0,8"},
		{name: "row2_ctx8_prob", source: "2,8"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "16,32")
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", "copy_source_prob")
			t.Setenv("DEBUG_CHROMA_SKIP_SOURCE", tc.source)
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s source=%s y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
				tc.name,
				tc.source,
				score.yRMSE,
				score.blockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call3ChromaSkipSourceBothPlanesSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_SKIP_SOURCE_BOTH_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-skip-source-both-sweep.mp4",
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

	baseline := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
		baseline.yRMSE,
		baseline.blockMSE,
		baseline.uBlockMSE,
		baseline.vBlockMSE,
		baseline.baseMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
	)

	variants := []struct {
		name   string
		source string
	}{
		{name: "row1_ctx8_prob", source: "1,8"},
		{name: "row1_ctx9_prob", source: "1,9"},
		{name: "row0_ctx7_prob", source: "0,7"},
		{name: "row2_ctx7_prob", source: "2,7"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "16,32")
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", "copy_source_prob")
			t.Setenv("DEBUG_CHROMA_SKIP_SOURCE", tc.source)
			t.Setenv("DEBUG_CHROMA_SKIP_PLANES", "1,2")
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s source=%s y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
				tc.name,
				tc.source,
				score.yRMSE,
				score.blockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call3ChromaSkipQCatSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_SKIP_QCAT_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-skip-qcat-sweep.mp4",
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

	baseline := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
		baseline.yRMSE,
		baseline.blockMSE,
		baseline.uBlockMSE,
		baseline.vBlockMSE,
		baseline.baseMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.state)),
	)

	for _, qcat := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("qcat_%d", qcat), func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "16,32")
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", "copy_qcat_prob")
			t.Setenv("DEBUG_CHROMA_SKIP_SOURCE_QCAT", itoaDebugInt(qcat))
			t.Setenv("DEBUG_CHROMA_SKIP_PLANES", "1,2")
			score := debugGeneratedMain10Call4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("qcat=%d y_rmse=%.4f block_mse=%.2f u_mse=%.2f v_mse=%.2f base_mse=%.2f syntax=%s",
				qcat,
				score.yRMSE,
				score.blockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call2ToCall3USkipBoundary(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL2_TO_CALL3_U_SKIP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call2-to-call3-u-skip.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call2 debugTargetLeafAdvanceState
		call3 debugTargetLeafAdvanceState
		got2  bool
		got3  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 2:
			call2 = captured
			got2 = true
		case 3:
			call3 = captured
			got3 = true
		}
	})
	if !got2 || !got3 {
		t.Fatalf("captured call2=%v call3=%v", got2, got3)
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	uvUnits := buildChromaTransformUnits(layout, call3.syntax.Geometry, call3.syntax.UVTX)
	if len(uvUnits) == 0 {
		t.Fatal("expected call3 chroma transform units")
	}
	uvTU := uvUnits[0]
	txInfo := TxfmInfoFor(call3.syntax.UVTX)

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error
	}
	scenarios := []scenario{
		{
			name: "syntax_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_plus_u",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call2.dq, above, left, call2.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_plus_v",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call2.dq, above, left, call2.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "full_residual",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call2.dq, above, left, call2.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
	}

	t.Logf("call2 syntax=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call2)))
	t.Logf("call3 live=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call3)))
	for _, tc := range scenarios {
		modeCDF := call2.mode.Clone()
		coefCDF := call2.coef.Clone()
		above := call2.above
		left := call2.left
		stateDec := call2.dec.Clone()
		if err := tc.apply(modeCDF, coefCDF, &above, &left, stateDec); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		next, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, call2, modeCDF, coefCDF, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("%s resume next leaf: %v", tc.name, err)
		}

		mode3 := next.mode.Clone()
		coef3 := next.coef.Clone()
		above3 := next.above
		left3 := next.left
		dec3 := next.dec.Clone()
		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, mode3, coef3, next.dq, &above3, &left3, next.syntax, dec3); err != nil {
			t.Fatalf("%s consume call3 luma: %v", tc.name, err)
		}
		bx4 := uvTU.Start4X & 31
		by4 := uvTU.Start4Y & 31
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coef3.Skip)-1)
		skipCtx := GetSkipCtx(txInfo, next.syntax.Size, above3.CCoef[0][bx4:], left3.CCoef[0][by4:], true, layout)
		entry := coef3.Skip[skipRow][skipCtx]
		probe := entry
		skipBit := dec3.Clone().DecodeBoolAdapt(probe[:]) != 0
		t.Logf("%s -> call3 %s", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(next)))
		t.Logf("%s call3_u skipRow=%d skipCtx=%d cdf=[%d %d] bit=%v", tc.name, skipRow, skipCtx, entry[0], entry[1], skipBit)
	}
}

func TestDebugGeneratedMain10Call2ChromaSkipEntryTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL2_SKIP_ENTRY_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call2-skip-entry-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

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

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	dec.Close()
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}

	modeCDF := call2.mode.Clone()
	coefCDF := call2.coef.Clone()
	above := call2.above
	left := call2.left
	stateDec := call2.dec.Clone()
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, modeCDF, coefCDF, call2.dq, &above, &left, call2.syntax, stateDec); err != nil {
		t.Fatalf("consume luma: %v", err)
	}
	layout := dec.header.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, call2.syntax.Segment, call2.syntax.UVTX, call2.syntax.UVMode)
	units := buildChromaTransformUnits(layout, call2.syntax.Geometry, call2.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected chroma transform units for call2")
	}
	tu := units[0]
	txInfo := TxfmInfoFor(call2.syntax.UVTX)
	skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coefCDF.Skip)-1)

	for _, plane := range []int{1, 2} {
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtx(txInfo, call2.syntax.Size, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], true, layout)
		before := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("plane=%d before skipRow=%d skipCtx=%d cdf=[%d %d]", plane, skipRow, skipCtx, before[0], before[1])
		coeffs, resCtx, skip, err := DecodeChromaCoefficients(call2.syntax.Size, tu, call2.syntax.UVTX, uvTxType, call2.syntax.Segment, plane, layout, coefCDF, call2.dq, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("plane=%d decode: %v", plane, err)
		}
		after := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("plane=%d after skip=%v resCtx=%#x coeffs=%v cdf=[%d %d]", plane, skip, resCtx, summarizeNonZeroCoeffs(coeffs), after[0], after[1])
		if err := ApplyChromaResidualContext(&above, &left, plane, tu, call2.syntax.UVTX, resCtx); err != nil {
			t.Fatalf("plane=%d apply ctx: %v", plane, err)
		}
	}
}

func TestDebugGeneratedMain10Call2VariantSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL2_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call2-variants.mp4",
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
		{name: "cfl_after_luma_palette", read: debugReadIntraBlockSyntaxCFLAfterLumaPalette},
		{name: "cfl_after_luma_indices", read: debugReadIntraBlockSyntaxCFLAfterLumaIndices},
		{name: "no_cfl", read: debugReadIntraBlockSyntaxNoCFL},
		{name: "default_uv_mode_only", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode = defaultModeCDFValue.UVMode
			return clone
		})},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{2: tc.read}
		}
		score := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, overrides)
		t.Logf("%s y_rmse=%.4f call3=%s call3_u_skip row=%d ctx=%d cdf=[%d %d] bit=%v call4_mse=%.2f call4_base=%.2f call4=%s",
			tc.name,
			score.yRMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call3)),
			score.call3Row,
			score.call3Ctx,
			score.call3Prob,
			score.call3Cnt,
			score.call3Bit,
			score.call4MSE,
			score.call4Base,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call4)),
		)
	}
}

func TestDebugGeneratedMain10Call2LumaBoundarySweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL2_LUMA_BOUNDARY") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call2-luma-boundary.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

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

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error
	}
	scenarios := []scenario{
		{
			name: "syntax_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_first_1_tu",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLumaLimit(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec, 1); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_first_2_tu",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLumaLimit(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec, 2); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_first_3_tu",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLumaLimit(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec, 3); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "luma_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
		{
			name: "full_residual",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call2.dq, above, left, call2.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call2.dq, above, left, call2.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call2.syntax)
			},
		},
	}

	t.Logf("call2 syntax=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call2)))
	for _, tc := range scenarios {
		modeCDF := call2.mode.Clone()
		coefCDF := call2.coef.Clone()
		above := call2.above
		left := call2.left
		stateDec := call2.dec.Clone()
		if err := tc.apply(modeCDF, coefCDF, &above, &left, stateDec); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		next, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, call2, modeCDF, coefCDF, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("%s resume next leaf: %v", tc.name, err)
		}

		mode3 := next.mode.Clone()
		coef3 := next.coef.Clone()
		above3 := next.above
		left3 := next.left
		dec3 := next.dec.Clone()
		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, mode3, coef3, next.dq, &above3, &left3, next.syntax, dec3); err != nil {
			t.Fatalf("%s consume call3 luma: %v", tc.name, err)
		}
		layout := seq.ColorConfig.ChromaSubsampling()
		units := buildChromaTransformUnits(layout, next.syntax.Geometry, next.syntax.UVTX)
		if len(units) == 0 {
			t.Fatalf("%s expected call3 chroma transform units", tc.name)
		}
		tu := units[0]
		txInfo := TxfmInfoFor(next.syntax.UVTX)
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coef3.Skip)-1)
		skipCtx := GetSkipCtx(txInfo, next.syntax.Size, above3.CCoef[0][bx4:], left3.CCoef[0][by4:], true, layout)
		entry := coef3.Skip[skipRow][skipCtx]
		probe := entry
		skipBit := dec3.Clone().DecodeBoolAdapt(probe[:]) != 0
		t.Logf("%s -> call3 %s", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(next)))
		t.Logf("%s call3_u skipRow=%d skipCtx=%d cdf=[%d %d] bit=%v", tc.name, skipRow, skipCtx, entry[0], entry[1], skipBit)
	}
}

func TestDebugGeneratedMain10Call2SkipPolicyRMSE(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL2_SKIP_POLICY_RMSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call2-skip-policy-rmse.mp4",
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

	baseline := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f call3=%s call3_u_skip row=%d ctx=%d cdf=[%d %d] bit=%v call4_mse=%.2f call4_base=%.2f call4=%s",
		baseline.yRMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.call3)),
		baseline.call3Row,
		baseline.call3Ctx,
		baseline.call3Prob,
		baseline.call3Cnt,
		baseline.call3Bit,
		baseline.call4MSE,
		baseline.call4Base,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.call4)),
	)

	type variant struct {
		name   string
		planes string
		policy string
	}
	variants := []variant{
		{name: "u_reset_prob", planes: "1", policy: "reset_entry_prob"},
		{name: "v_reset_prob", planes: "2", policy: "reset_entry_prob"},
		{name: "uv_reset_prob", planes: "1,2", policy: "reset_entry_prob"},
		{name: "u_reset_count", planes: "1", policy: "reset_entry_count"},
		{name: "v_reset_count", planes: "2", policy: "reset_entry_count"},
		{name: "uv_reset_count", planes: "1,2", policy: "reset_entry_count"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "0,32")
			t.Setenv("DEBUG_CHROMA_SKIP_PLANES", tc.planes)
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", tc.policy)
			score := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s planes=%s policy=%s y_rmse=%.4f call3=%s call3_u_skip row=%d ctx=%d cdf=[%d %d] bit=%v call4_mse=%.2f call4_base=%.2f call4=%s",
				tc.name,
				tc.planes,
				tc.policy,
				score.yRMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call3)),
				score.call3Row,
				score.call3Ctx,
				score.call3Prob,
				score.call3Cnt,
				score.call3Bit,
				score.call4MSE,
				score.call4Base,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call4)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call1ToCall2USkipBoundary(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL1_TO_CALL2_U_SKIP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call1-to-call2-u-skip.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call1 debugTargetLeafAdvanceState
		call2 debugTargetLeafAdvanceState
		got1  bool
		got2  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 1:
			call1 = captured
			got1 = true
		case 2:
			call2 = captured
			got2 = true
		}
	})
	if !got1 || !got2 {
		t.Fatalf("captured call1=%v call2=%v", got1, got2)
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	uvUnits := buildChromaTransformUnits(layout, call2.syntax.Geometry, call2.syntax.UVTX)
	if len(uvUnits) == 0 {
		t.Fatal("expected call2 chroma transform units")
	}
	uvTU := uvUnits[0]
	txInfo := TxfmInfoFor(call2.syntax.UVTX)

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error
	}
	scenarios := []scenario{
		{
			name: "syntax_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "luma_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "luma_plus_u",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call1.dq, above, left, call1.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "luma_plus_u_no_ctx",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanesNoResidualContext(seq, &parsed.Header, coefCDF, call1.dq, above, left, call1.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "luma_plus_v",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call1.dq, above, left, call1.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "luma_plus_v_no_ctx",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanesNoResidualContext(seq, &parsed.Header, coefCDF, call1.dq, above, left, call1.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "full_residual",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call1.dq, above, left, call1.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
		{
			name: "full_residual_no_ctx",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call1.dq, above, left, call1.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanesNoResidualContext(seq, &parsed.Header, coefCDF, call1.dq, above, left, call1.syntax, dec, []int{1, 2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call1.syntax)
			},
		},
	}

	t.Logf("call1 syntax=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call1)))
	t.Logf("call2 live=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call2)))
	for _, tc := range scenarios {
		modeCDF := call1.mode.Clone()
		coefCDF := call1.coef.Clone()
		above := call1.above
		left := call1.left
		stateDec := call1.dec.Clone()
		if err := tc.apply(modeCDF, coefCDF, &above, &left, stateDec); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		next, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, call1, modeCDF, coefCDF, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("%s resume next leaf: %v", tc.name, err)
		}

		mode2 := next.mode.Clone()
		coef2 := next.coef.Clone()
		above2 := next.above
		left2 := next.left
		dec2 := next.dec.Clone()
		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, mode2, coef2, next.dq, &above2, &left2, next.syntax, dec2); err != nil {
			t.Fatalf("%s consume call2 luma: %v", tc.name, err)
		}
		bx4 := uvTU.Start4X & 31
		by4 := uvTU.Start4Y & 31
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coef2.Skip)-1)
		skipCtx := GetSkipCtx(txInfo, next.syntax.Size, above2.CCoef[0][bx4:], left2.CCoef[0][by4:], true, layout)
		entry := coef2.Skip[skipRow][skipCtx]
		probe := entry
		skipBit := dec2.Clone().DecodeBoolAdapt(probe[:]) != 0
		t.Logf("%s -> call2 %s", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(next)))
		t.Logf("%s call2_u skipRow=%d skipCtx=%d cdf=[%d %d] bit=%v", tc.name, skipRow, skipCtx, entry[0], entry[1], skipBit)
	}
}

func TestDebugGeneratedMain10Call1ChromaSkipEntryTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL1_SKIP_ENTRY_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call1-skip-entry-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

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

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	dec.Close()
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}

	modeCDF := call1.mode.Clone()
	coefCDF := call1.coef.Clone()
	above := call1.above
	left := call1.left
	stateDec := call1.dec.Clone()
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, modeCDF, coefCDF, call1.dq, &above, &left, call1.syntax, stateDec); err != nil {
		t.Fatalf("consume luma: %v", err)
	}
	layout := dec.header.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, call1.syntax.Segment, call1.syntax.UVTX, call1.syntax.UVMode)
	units := buildChromaTransformUnits(layout, call1.syntax.Geometry, call1.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected chroma transform units for call1")
	}
	tu := units[0]
	txInfo := TxfmInfoFor(call1.syntax.UVTX)
	skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coefCDF.Skip)-1)

	for _, plane := range []int{1, 2} {
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtx(txInfo, call1.syntax.Size, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], true, layout)
		before := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("plane=%d before skipRow=%d skipCtx=%d cdf=[%d %d]", plane, skipRow, skipCtx, before[0], before[1])
		coeffs, resCtx, skip, err := DecodeChromaCoefficients(call1.syntax.Size, tu, call1.syntax.UVTX, uvTxType, call1.syntax.Segment, plane, layout, coefCDF, call1.dq, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("plane=%d decode: %v", plane, err)
		}
		after := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("plane=%d after skip=%v resCtx=%#x coeffs=%v cdf=[%d %d]", plane, skip, resCtx, summarizeNonZeroCoeffs(coeffs), after[0], after[1])
		if err := ApplyChromaResidualContext(&above, &left, plane, tu, call1.syntax.UVTX, resCtx); err != nil {
			t.Fatalf("plane=%d apply ctx: %v", plane, err)
		}
	}
}

func TestDebugGeneratedMain10Call1SkipPolicyRMSE(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL1_SKIP_POLICY_RMSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call1-skip-policy-rmse.mp4",
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

	baseline := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f call3=%s call3_u_skip row=%d ctx=%d cdf=[%d %d] bit=%v call4_mse=%.2f call4_base=%.2f call4=%s",
		baseline.yRMSE,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.call3)),
		baseline.call3Row,
		baseline.call3Ctx,
		baseline.call3Prob,
		baseline.call3Cnt,
		baseline.call3Bit,
		baseline.call4MSE,
		baseline.call4Base,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.call4)),
	)

	type variant struct {
		name   string
		planes string
		policy string
	}
	variants := []variant{
		{name: "u_reset_prob", planes: "1", policy: "reset_entry_prob"},
		{name: "v_reset_prob", planes: "2", policy: "reset_entry_prob"},
		{name: "uv_reset_prob", planes: "1,2", policy: "reset_entry_prob"},
		{name: "u_reset_count", planes: "1", policy: "reset_entry_count"},
		{name: "v_reset_count", planes: "2", policy: "reset_entry_count"},
		{name: "uv_reset_count", planes: "1,2", policy: "reset_entry_count"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "32,0")
			t.Setenv("DEBUG_CHROMA_SKIP_PLANES", tc.planes)
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", tc.policy)
			score := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s planes=%s policy=%s y_rmse=%.4f call3=%s call3_u_skip row=%d ctx=%d cdf=[%d %d] bit=%v call4_mse=%.2f call4_base=%.2f call4=%s",
				tc.name,
				tc.planes,
				tc.policy,
				score.yRMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call3)),
				score.call3Row,
				score.call3Ctx,
				score.call3Prob,
				score.call3Cnt,
				score.call3Bit,
				score.call4MSE,
				score.call4Base,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call4)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call1SkipSourceSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL1_SKIP_SOURCE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call1-skip-source-sweep.mp4",
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

	baseline := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f call4_mse=%.2f call4_base=%.2f call4=%s",
		baseline.yRMSE,
		baseline.call4MSE,
		baseline.call4Base,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.call4)),
	)

	type source struct {
		name   string
		planes string
		row    int
		ctx    int
	}
	sources := []source{
		{name: "u_row1_ctx7", planes: "1", row: 1, ctx: 7},
		{name: "v_row1_ctx7", planes: "2", row: 1, ctx: 7},
		{name: "uv_row1_ctx7", planes: "1,2", row: 1, ctx: 7},
		{name: "u_row2_ctx6", planes: "1", row: 2, ctx: 6},
		{name: "v_row2_ctx6", planes: "2", row: 2, ctx: 6},
		{name: "uv_row2_ctx6", planes: "1,2", row: 2, ctx: 6},
		{name: "u_row2_ctx8", planes: "1", row: 2, ctx: 8},
		{name: "v_row2_ctx8", planes: "2", row: 2, ctx: 8},
		{name: "uv_row2_ctx8", planes: "1,2", row: 2, ctx: 8},
	}

	for _, tc := range sources {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "32,0")
			t.Setenv("DEBUG_CHROMA_SKIP_PLANES", tc.planes)
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", "copy_source_prob")
			t.Setenv("DEBUG_CHROMA_SKIP_SOURCE", fmt.Sprintf("%d,%d", tc.row, tc.ctx))
			score := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s planes=%s src=(%d,%d) y_rmse=%.4f call4_mse=%.2f call4_base=%.2f call4=%s",
				tc.name,
				tc.planes,
				tc.row,
				tc.ctx,
				score.yRMSE,
				score.call4MSE,
				score.call4Base,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call4)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call1SkipQCatSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL1_SKIP_QCAT_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call1-skip-qcat-sweep.mp4",
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

	baseline := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
	t.Logf("baseline y_rmse=%.4f call4_mse=%.2f call4_base=%.2f call4=%s",
		baseline.yRMSE,
		baseline.call4MSE,
		baseline.call4Base,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(baseline.call4)),
	)

	for _, qcat := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("qcat_%d", qcat), func(t *testing.T) {
			t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "32,0")
			t.Setenv("DEBUG_CHROMA_SKIP_PLANES", "1,2")
			t.Setenv("DEBUG_CHROMA_SKIP_POLICY", "copy_qcat_prob")
			t.Setenv("DEBUG_CHROMA_SKIP_SOURCE_QCAT", itoaDebugInt(qcat))
			score := debugGeneratedMain10Call3And4TargetScore(t, fixture.Path, ref, nil)
			t.Logf("qcat=%d y_rmse=%.4f call4_mse=%.2f call4_base=%.2f call4=%s",
				qcat,
				score.yRMSE,
				score.call4MSE,
				score.call4Base,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call4)),
			)
		})
	}
}

func TestDebugGeneratedMain10CallQIdxTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL_QIDX_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call-qidx-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx > 8 {
			return
		}
		qcat := DefaultCoefQCat(uint8(clipQIndexRuntime(captured.qidx)))
		t.Logf("call=%d qidx=%d qcat=%d syntax=%s", callIdx, captured.qidx, qcat, formatTargetSyntaxSnapshot(snapshotTargetSyntax(captured)))
	})
}

func TestDebugGeneratedMain10Call0VariantSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL0_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call0-variants.mp4",
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

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}
	seq := dec.header
	dec.Close()

	type variant struct {
		name string
		read debugIntraSyntaxReader
	}
	variants := []variant{
		{name: "current"},
		{name: "bounded_y_delta_bits", read: debugReadIntraBlockSyntaxWithCustomYPalette(debugReadPalettePlaneSyntaxBoundedYDeltaBits)},
		{name: "local_no_palette_cache", read: debugReadIntraBlockSyntaxNoPaletteCache},
		{name: "cfl_after_luma_palette", read: debugReadIntraBlockSyntaxCFLAfterLumaPalette},
		{name: "cfl_after_luma_indices", read: debugReadIntraBlockSyntaxCFLAfterLumaIndices},
		{name: "no_cfl", read: debugReadIntraBlockSyntaxNoCFL},
		{name: "default_uv_mode_only", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode = defaultModeCDFValue.UVMode
			return clone
		})},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{0: tc.read}
		}
		var (
			call1 debugTargetLeafAdvanceState
			call4 debugTargetLeafAdvanceState
			got1  bool
			got4  bool
		)
		yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, overrides, func(callIdx int, captured debugTargetLeafAdvanceState) {
			switch callIdx {
			case 1:
				call1 = captured
				got1 = true
			case 4:
				call4 = captured
				got4 = true
			}
		})
		if !got1 || !got4 {
			t.Fatalf("%s captured call1=%v call4=%v", tc.name, got1, got4)
		}

		layout := seq.ColorConfig.ChromaSubsampling()
		units := buildChromaTransformUnits(layout, call1.syntax.Geometry, call1.syntax.UVTX)
		if len(units) == 0 {
			t.Fatalf("%s expected call1 chroma transform units", tc.name)
		}
		tu := units[0]
		txInfo := TxfmInfoFor(call1.syntax.UVTX)
		mode1 := call1.mode.Clone()
		coef1 := call1.coef.Clone()
		above1 := call1.above
		left1 := call1.left
		dec1 := call1.dec.Clone()
		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, mode1, coef1, call1.dq, &above1, &left1, call1.syntax, dec1); err != nil {
			t.Fatalf("%s consume call1 luma: %v", tc.name, err)
		}
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coef1.Skip)-1)
		skipCtx := GetSkipCtx(txInfo, call1.syntax.Size, above1.CCoef[0][bx4:], left1.CCoef[0][by4:], true, layout)
		entry := coef1.Skip[skipRow][skipCtx]
		probe := entry
		skipBit := dec1.Clone().DecodeBoolAdapt(probe[:]) != 0

		blockX := call4.syntax.Geometry.Start4X * 4
		blockY := call4.syntax.Geometry.Start4Y * 4
		blockW := call4.syntax.Geometry.Width4() * 4
		blockH := call4.syntax.Geometry.Height4() * 4
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		call4MSE := compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)

		t.Logf("%s y_rmse=%.4f call1=%s call1_u skipRow=%d skipCtx=%d cdf=[%d %d] bit=%v call4_mse=%.2f call4=%s",
			tc.name,
			testutil.ComparePlane16(yPlane, ref.Y16).RMSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(call1)),
			skipRow,
			skipCtx,
			entry[0],
			entry[1],
			skipBit,
			call4MSE,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(call4)),
		)
	}
}

func TestDebugGeneratedMain10Call0SkipRowLocalScore(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL0_SKIP_ROW_LOCAL_SCORE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call0-skip-row-local-score.mp4",
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
		name   string
		planes string
	}
	variants := []variant{
		{name: "baseline"},
		{name: "plane2_row0", planes: "2"},
		{name: "planes12_row0", planes: "1,2"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			if tc.planes != "" {
				t.Setenv("DEBUG_COEF_SKIP_TARGET", "0,0")
				t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "1")
				t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", tc.planes)
				t.Setenv("DEBUG_COEF_SKIP_ROW_TARGET", "0")
			}
			score := debugGeneratedMain10Call0LocalTargetScore(t, fixture.Path, ref, nil)
			t.Logf("%s y_rmse=%.4f y_block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f call0=%s call1=%s",
				tc.name,
				score.yRMSE,
				score.yBlockMSE,
				score.uBlockMSE,
				score.vBlockMSE,
				score.baseMSE,
				score.fullMSE,
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call0)),
				formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.call1)),
			)
		})
	}
}

func TestDebugGeneratedMain10Call0ToCall1USkipBoundary(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL0_TO_CALL1_U_SKIP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call0-to-call1-u-skip.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call0 debugTargetLeafAdvanceState
		call1 debugTargetLeafAdvanceState
		got0  bool
		got1  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 0:
			call0 = captured
			got0 = true
		case 1:
			call1 = captured
			got1 = true
		}
	})
	if !got0 || !got1 {
		t.Fatalf("captured call0=%v call1=%v", got0, got1)
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	uvUnits := buildChromaTransformUnits(layout, call1.syntax.Geometry, call1.syntax.UVTX)
	if len(uvUnits) == 0 {
		t.Fatal("expected call1 chroma transform units")
	}
	uvTU := uvUnits[0]
	txInfo := TxfmInfoFor(call1.syntax.UVTX)

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error
	}
	scenarios := []scenario{
		{
			name: "syntax_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "luma_only",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "luma_plus_u",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call0.dq, above, left, call0.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "luma_plus_u_no_ctx",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanesNoResidualContext(seq, &parsed.Header, coefCDF, call0.dq, above, left, call0.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "luma_plus_v",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call0.dq, above, left, call0.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "luma_plus_v_no_ctx",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanesNoResidualContext(seq, &parsed.Header, coefCDF, call0.dq, above, left, call0.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "full_residual",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call0.dq, above, left, call0.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
		{
			name: "full_residual_no_ctx",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call0.dq, above, left, call0.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanesNoResidualContext(seq, &parsed.Header, coefCDF, call0.dq, above, left, call0.syntax, dec, []int{1, 2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call0.syntax)
			},
		},
	}

	t.Logf("call0 syntax=%s qidx=%d qcat=%d", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call0)), call0.qidx, DefaultCoefQCat(uint8(clipQIndexRuntime(call0.qidx))))
	t.Logf("call1 live=%s qidx=%d qcat=%d", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call1)), call1.qidx, DefaultCoefQCat(uint8(clipQIndexRuntime(call1.qidx))))
	for _, tc := range scenarios {
		modeCDF := call0.mode.Clone()
		coefCDF := call0.coef.Clone()
		above := call0.above
		left := call0.left
		stateDec := call0.dec.Clone()
		if err := tc.apply(modeCDF, coefCDF, &above, &left, stateDec); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		next, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, call0, modeCDF, coefCDF, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("%s resume next leaf: %v", tc.name, err)
		}

		mode1 := next.mode.Clone()
		coef1 := next.coef.Clone()
		above1 := next.above
		left1 := next.left
		dec1 := next.dec.Clone()
		if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, mode1, coef1, next.dq, &above1, &left1, next.syntax, dec1); err != nil {
			t.Fatalf("%s consume call1 luma: %v", tc.name, err)
		}
		bx4 := uvTU.Start4X & 31
		by4 := uvTU.Start4Y & 31
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coef1.Skip)-1)
		skipCtx := GetSkipCtx(txInfo, next.syntax.Size, above1.CCoef[0][bx4:], left1.CCoef[0][by4:], true, layout)
		entry := coef1.Skip[skipRow][skipCtx]
		probe := entry
		skipBit := dec1.Clone().DecodeBoolAdapt(probe[:]) != 0
		t.Logf("%s -> call1 qidx=%d qcat=%d %s", tc.name, next.qidx, DefaultCoefQCat(uint8(clipQIndexRuntime(next.qidx))), formatTargetSyntaxSnapshot(snapshotTargetSyntax(next)))
		t.Logf("%s call1_u skipRow=%d skipCtx=%d cdf=[%d %d] bit=%v", tc.name, skipRow, skipCtx, entry[0], entry[1], skipBit)
	}
}

func TestDebugGeneratedMain10Call0ChromaSkipEntryTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL0_SKIP_ENTRY_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call0-skip-entry-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	var (
		call0 debugTargetLeafAdvanceState
		got0  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 0 {
			return
		}
		call0 = captured
		got0 = true
	})
	if !got0 {
		t.Fatal("did not capture call0 state")
	}

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	dec.Close()
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}

	modeCDF := call0.mode.Clone()
	coefCDF := call0.coef.Clone()
	above := call0.above
	left := call0.left
	stateDec := call0.dec.Clone()
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, modeCDF, coefCDF, call0.dq, &above, &left, call0.syntax, stateDec); err != nil {
		t.Fatalf("consume luma: %v", err)
	}
	layout := dec.header.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, call0.syntax.Segment, call0.syntax.UVTX, call0.syntax.UVMode)
	units := buildChromaTransformUnits(layout, call0.syntax.Geometry, call0.syntax.UVTX)
	if len(units) == 0 {
		t.Fatal("expected chroma transform units for call0")
	}
	tu := units[0]
	txInfo := TxfmInfoFor(call0.syntax.UVTX)
	skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coefCDF.Skip)-1)

	for _, plane := range []int{1, 2} {
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtx(txInfo, call0.syntax.Size, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], true, layout)
		before := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("plane=%d before skipRow=%d skipCtx=%d cdf=[%d %d]", plane, skipRow, skipCtx, before[0], before[1])
		coeffs, resCtx, skip, err := DecodeChromaCoefficients(call0.syntax.Size, tu, call0.syntax.UVTX, uvTxType, call0.syntax.Segment, plane, layout, coefCDF, call0.dq, &above, &left, stateDec)
		if err != nil {
			t.Fatalf("plane=%d decode: %v", plane, err)
		}
		after := coefCDF.Skip[skipRow][skipCtx]
		t.Logf("plane=%d after skip=%v resCtx=%#x coeffs=%v cdf=[%d %d]", plane, skip, resCtx, summarizeNonZeroCoeffs(coeffs), after[0], after[1])
		if err := ApplyChromaResidualContext(&above, &left, plane, tu, call0.syntax.UVTX, resCtx); err != nil {
			t.Fatalf("plane=%d apply ctx: %v", plane, err)
		}
	}
}

func TestDebugGeneratedMain10Call3ReplayMatchesLivePostLeafState(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_POST_REPLAY") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-post-replay.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

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
		t.Fatal("did not capture call 3 state")
	}
	if call3.postMode == nil || call3.postCoef == nil || call3.postDec == nil {
		t.Fatal("call 3 post state missing")
	}

	dec, err := OpenMP4(fixture.Path)
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}

	modeCDF := call3.mode.Clone()
	coefCDF := call3.coef.Clone()
	above := call3.above
	left := call3.left
	stateDec := call3.dec.Clone()

	width := parsed.Header.Width
	height := parsed.Header.Height
	plane := make([]uint16, width*height)
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	uPlane := make([]uint16, chromaWidth*chromaHeight)
	vPlane := make([]uint16, chromaWidth*chromaHeight)
	fillUint16(uPlane, 1<<(dec.header.ColorConfig.BitDepth-1))
	fillUint16(vPlane, 1<<(dec.header.ColorConfig.BitDepth-1))

	if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
		dec.header, &parsed.Header, modeCDF, coefCDF, call3.dq, &above, &left, call3.syntax,
		plane, width, width, height, call3.syntax.Geometry.Start4X, call3.syntax.Geometry.Start4Y,
		dec.header.ColorConfig.BitDepth, call3.syntax.Skip, stateDec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
	); err != nil {
		t.Fatalf("ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges: %v", err)
	}
	if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(
		dec.header, &parsed.Header, coefCDF, call3.dq, &above, &left, call3.syntax,
		plane, width, width, height, uPlane, chromaWidth, chromaWidth, chromaHeight, vPlane, chromaWidth, chromaWidth, chromaHeight,
		dec.header.ColorConfig.BitDepth, call3.syntax.Skip, stateDec, call3.haveTop, call3.haveLeft, call3.edge.topHasRight, call3.edge.leftHasBottom,
	); err != nil {
		t.Fatalf("reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges: %v", err)
	}
	if err := debugApplyIntraBlockContextReplay(dec.header, &parsed.Header, &above, &left, call3.syntax); err != nil {
		t.Fatalf("debugApplyIntraBlockContextReplay: %v", err)
	}

	gotCur, gotRng, gotCnt, gotPos := stateDec.DebugState()
	wantCur, wantRng, wantCnt, wantPos := call3.postDec.DebugState()
	sameMSAC := gotCur == wantCur && gotRng == wantRng && gotCnt == wantCnt && gotPos == wantPos
	sameAbove := above == call3.postAbove
	sameLeft := left == call3.postLeft
	sameMode := *modeCDF == *call3.postMode
	sameCoef := *coefCDF == *call3.postCoef

	t.Logf("call3 syntax=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(call3)))
	t.Logf("replay_vs_live same_msac=%v same_above=%v same_left=%v same_mode=%v same_coef=%v", sameMSAC, sameAbove, sameLeft, sameMode, sameCoef)
	t.Logf("replay msac current=%d rng=%d cnt=%d pos=%d", gotCur, gotRng, gotCnt, gotPos)
	t.Logf("live   msac current=%d rng=%d cnt=%d pos=%d", wantCur, wantRng, wantCnt, wantPos)
	if !sameMSAC || !sameAbove || !sameLeft || !sameMode || !sameCoef {
		t.Fatalf("call3 replay does not match live post-leaf state")
	}
}

func TestDebugGeneratedMain10Call3ResumeMatchesLiveCall4Entry(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CALL4_ENTRY") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-call4-entry.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}
	if call3.postMode == nil || call3.postCoef == nil || call3.postDec == nil {
		t.Fatal("call3 post state missing")
	}
	if call4.entryMode == nil || call4.entryCoef == nil || call4.entryDec == nil {
		t.Fatal("call4 entry state missing")
	}

	dec, err := OpenMP4(fixture.Path)
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
	if parsed == nil {
		t.Fatal("missing parsed keyframe")
	}

	modeCDF := call3.postMode.Clone()
	coefCDF := call3.postCoef.Clone()
	above := call3.postAbove
	left := call3.postLeft
	stateDec := call3.postDec.Clone()
	entry, err := debugResumeNextLeafEntry(dec.header, &parsed.Header, call3, modeCDF, coefCDF, &above, &left, stateDec)
	if err != nil {
		t.Fatalf("debugResumeNextLeafEntry: %v", err)
	}

	gotCur, gotRng, gotCnt, gotPos := entry.entryDec.DebugState()
	wantCur, wantRng, wantCnt, wantPos := call4.entryDec.DebugState()
	sameMSAC := gotCur == wantCur && gotRng == wantRng && gotCnt == wantCnt && gotPos == wantPos
	sameGeometry := entry.syntax.Geometry == call4.syntax.Geometry
	sameLocal := entry.entryLocal == call4.entryLocal
	sameAbove := entry.entryAbove == call4.entryAbove
	sameLeft := entry.entryLeft == call4.entryLeft
	sameMode := *entry.entryMode == *call4.entryMode
	sameCoef := *entry.entryCoef == *call4.entryCoef

	t.Logf("reconstructed entry geom=(%d,%d)-(%d,%d)", entry.syntax.Geometry.Start4X*4, entry.syntax.Geometry.Start4Y*4, entry.syntax.Geometry.End4X*4, entry.syntax.Geometry.End4Y*4)
	t.Logf("live          geom=(%d,%d)-(%d,%d)", call4.syntax.Geometry.Start4X*4, call4.syntax.Geometry.Start4Y*4, call4.syntax.Geometry.End4X*4, call4.syntax.Geometry.End4Y*4)
	t.Logf("entry_vs_live same_msac=%v same_geometry=%v same_local=%v same_above=%v same_left=%v same_mode=%v same_coef=%v", sameMSAC, sameGeometry, sameLocal, sameAbove, sameLeft, sameMode, sameCoef)
	t.Logf("entry msac current=%d rng=%d cnt=%d pos=%d", gotCur, gotRng, gotCnt, gotPos)
	t.Logf("live  msac current=%d rng=%d cnt=%d pos=%d", wantCur, wantRng, wantCnt, wantPos)
	for i, frame := range call3.traversal {
		children := make([]string, 0, frame.ChildCount)
		for j := 0; j < frame.ChildCount; j++ {
			child := frame.Children[j]
			children = append(children, fmt.Sprintf("(%d,%d)-(%d,%d)", child.Start4X*4, child.Start4Y*4, child.End4X*4, child.End4Y*4))
		}
		t.Logf("call3 stack[%d] geom=(%d,%d)-(%d,%d) part=%d child=%d/%d local=(%d,%d)-(%d,%d) children=%v",
			i,
			frame.Geometry.Start4X*4, frame.Geometry.Start4Y*4, frame.Geometry.End4X*4, frame.Geometry.End4Y*4,
			frame.Part, frame.ChildIndex, frame.ChildCount,
			frame.Local.Start4X*4, frame.Local.Start4Y*4, frame.Local.End4X*4, frame.Local.End4Y*4,
			children,
		)
	}
	for i := range entry.entryAbove.Partition {
		if entry.entryAbove.Partition[i] != call4.entryAbove.Partition[i] {
			t.Logf("above.Partition[%d] reconstructed=%d live=%d", i, entry.entryAbove.Partition[i], call4.entryAbove.Partition[i])
		}
	}
	if !sameMSAC || !sameGeometry || !sameLocal || !sameAbove || !sameLeft || !sameMode || !sameCoef {
		t.Fatalf("call3 resume does not match live call4 entry")
	}
}

func TestDebugGeneratedMain10PaletteBreakpointLeaves(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_BREAKPOINT_LEAVES") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-palette-breakpoint-leaves.mp4",
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
	if len(entries) < 11 {
		t.Fatalf("need at least 11 traced leaves, got %d", len(entries))
	}
	yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, nil)

	for _, call := range []int{8, 10} {
		if call+1 >= len(entries) {
			t.Fatalf("call %d missing next entry", call)
		}
		entry := entries[call]
		next := entries[call+1]
		targetX := entry.Geometry.Start4X * 4
		targetY := entry.Geometry.Start4Y * 4
		nextX := next.Geometry.Start4X * 4
		nextY := next.Geometry.Start4Y * 4

		dec, err := OpenMP4(fixture.Path)
		if err != nil {
			t.Fatalf("call %d OpenMP4: %v", call, err)
		}
		parsed, state := generatedKeyframeLeafState(t, dec, targetX, targetY, nextX, nextY)
		dec.Close()

		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		blockMSE := compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		cache, nCache := paletteCache(&state.above, &state.left, 0, state.syntax.Geometry.Start4X&31, state.syntax.Geometry.Start4Y&31)
		cacheVals := append([]uint16(nil), cache[:nCache]...)
		unionVals := uniqueSortedUint16(append(cacheVals, state.syntax.PalY[:state.syntax.PalYSize]...))

		baseMSE := -1.0
		fullMSE := -1.0
		txLabel := "n/a"
		if state.syntax.PalYSize != 0 {
			baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			if prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0); err != nil {
				t.Fatalf("call %d cloneNthNonSkippedTUCoefficientInput: %v", call, err)
			} else if ok {
				decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
				fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
				txLabel = "txType=" + itoaDebugInt(int(txType))
			}
		}
		oraclePal := fitPaletteValuesToRef16(state.syntax.PalYIdx, int(state.syntax.PalYSize), blockW, blockH, refBlock, ref.BitDepth)
		oracleBaseMSE := paletteBaseMSE16(oraclePal, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
		bestUnionPal, bestUnionBaseMSE := bestPaletteAssignmentFromSet16(state.syntax.PalYIdx, int(state.syntax.PalYSize), blockW, blockH, refBlock, unionVals)
		reverseDiagIdx := paletteIndicesReverseDiagonals(state.syntax.PalYIdx, blockW, blockH)
		reverseDiagBaseMSE := paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), reverseDiagIdx, blockW, blockH, refBlock)
		rowMajorIdx := paletteIndicesDiagonalToRowMajor(state.syntax.PalYIdx, blockW, blockH)
		rowMajorBaseMSE := paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), rowMajorIdx, blockW, blockH, refBlock)

		t.Logf("call=%d traced=%s", call, formatLeafTraceEntry(entry))
		t.Logf("call=%d actual=%s", call, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		t.Logf("call=%d block_mse=%.2f base_mse=%.2f full_mse=%.2f tx=%s cache=%v pal=%v idx0=%v",
			call,
			blockMSE,
			baseMSE,
			fullMSE,
			txLabel,
			cacheVals,
			state.syntax.PalY[:state.syntax.PalYSize],
			append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
		)
		t.Logf("call=%d oracle_base_mse=%.2f oracle_pal=%v union_base_mse=%.2f union_pal=%v union_vals=%v",
			call,
			oracleBaseMSE,
			oraclePal[:state.syntax.PalYSize],
			bestUnionBaseMSE,
			bestUnionPal[:state.syntax.PalYSize],
			unionVals,
		)
		t.Logf("call=%d reverse_diag_base_mse=%.2f row_major_base_mse=%.2f", call, reverseDiagBaseMSE, rowMajorBaseMSE)
	}
}

func TestDebugGeneratedMain10Call8PaletteLocalCDFVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL8_PALETTE_CDF") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call8-palette-cdf.mp4",
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
	if len(entries) < 10 {
		t.Fatalf("need at least 10 traced leaves, got %d", len(entries))
	}
	entry := entries[8]
	next := entries[9]
	targetX := entry.Geometry.Start4X * 4
	targetY := entry.Geometry.Start4Y * 4
	nextX := next.Geometry.Start4X * 4
	nextY := next.Geometry.Start4Y * 4

	type variant struct {
		name string
		read debugIntraSyntaxReader
	}
	variants := []variant{
		{name: "current"},
		{name: "default_pal_y", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true})},
		{name: "default_pal_size", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true})},
		{name: "default_palette_flags", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true})},
		{name: "default_color_map", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{colorMap: true})},
		{name: "default_palette_all", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true})},
	}

	for _, tc := range variants {
		dec, err := OpenMP4(fixture.Path)
		if err != nil {
			t.Fatalf("%s OpenMP4: %v", tc.name, err)
		}
		parsed, state := generatedKeyframeLeafStateWithReader(t, dec, targetX, targetY, nextX, nextY, tc.read)
		dec.Close()

		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		baseMSE := paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
		fullMSE := -1.0
		txLabel := "n/a"
		if prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0); err != nil {
			t.Fatalf("%s cloneNthNonSkippedTUCoefficientInput: %v", tc.name, err)
		} else if ok {
			decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
			fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
			txLabel = "txType=" + itoaDebugInt(int(txType))
		}
		t.Logf("%s syntax=%s", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		t.Logf("%s base_mse=%.2f full_mse=%.2f tx=%s pal=%v idx0=%v",
			tc.name,
			baseMSE,
			fullMSE,
			txLabel,
			state.syntax.PalY[:state.syntax.PalYSize],
			append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
		)
	}
}

func TestDebugGeneratedMain10Call8PaletteTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL8_PALETTE_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call8-palette-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})
	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) < 10 {
		t.Fatalf("need at least 10 traced leaves, got %d", len(entries))
	}
	entry := entries[8]
	next := entries[9]
	targetX := entry.Geometry.Start4X * 4
	targetY := entry.Geometry.Start4Y * 4
	nextX := next.Geometry.Start4X * 4
	nextY := next.Geometry.Start4Y * 4

	type preSyntaxPaletteState struct {
		mode    *ModeCDF
		above   BlockContext
		left    BlockContext
		dec     *msac.Context
		sizeCtx int
		bx4     int
		by4     int
	}
	var captured preSyntaxPaletteState

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	_, state := generatedKeyframeLeafStateWithReader(t, dec, targetX, targetY, nextX, nextY, func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, entropy intraEntropyDecoder) (IntraBlockSyntax, error) {
		captured.mode = cdf.Clone()
		captured.above = *above
		captured.left = *left
		captured.sizeCtx = paletteSizeContext(g)
		captured.bx4 = g.Start4X & 31
		captured.by4 = g.Start4Y & 31
		switch dec := entropy.(type) {
		case msacEntropyDecoder:
			if dec.ctx == nil {
				t.Fatalf("nil msac entropy decoder")
			}
			captured.dec = dec.ctx.Clone()
		case *msac.Context:
			captured.dec = dec.Clone()
		default:
			t.Fatalf("unexpected entropy decoder type %T", entropy)
		}
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, entropy)
	})
	dec.Close()
	if captured.mode == nil || captured.dec == nil {
		t.Fatal("failed to capture pre-syntax palette state")
	}

	t.Logf("call8 actual syntax=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))

	traceDec := captured.dec.Clone()
	traceCDF := captured.mode.Clone()
	_, _, err = debugReadPalettePlaneSyntaxTrace(t, "call8 current", 10, traceCDF, &captured.above, &captured.left, 0, captured.sizeCtx, captured.bx4, captured.by4, msacEntropyDecoder{ctx: traceDec})
	if err != nil {
		t.Fatalf("call8 current trace: %v", err)
	}

	defaultPalSizeDec := captured.dec.Clone()
	defaultPalSizeCDF := captured.mode.Clone()
	defaultPalSizeCDF.PalSize = defaultModeCDFValue.PalSize
	_, _, err = debugReadPalettePlaneSyntaxTrace(t, "call8 default_palsize", 10, defaultPalSizeCDF, &captured.above, &captured.left, 0, captured.sizeCtx, captured.bx4, captured.by4, msacEntropyDecoder{ctx: defaultPalSizeDec})
	if err != nil {
		t.Fatalf("call8 default_palsize trace: %v", err)
	}

	defaultPaletteAllDec := captured.dec.Clone()
	defaultPaletteAllCDF := captured.mode.Clone()
	defaultPaletteAllCDF.PalY = defaultModeCDFValue.PalY
	defaultPaletteAllCDF.PalSize = defaultModeCDFValue.PalSize
	defaultPaletteAllCDF.PalUV = defaultModeCDFValue.PalUV
	defaultPaletteAllCDF.ColorMap = defaultModeCDFValue.ColorMap
	_, _, err = debugReadPalettePlaneSyntaxTrace(t, "call8 default_palette_all", 10, defaultPaletteAllCDF, &captured.above, &captured.left, 0, captured.sizeCtx, captured.bx4, captured.by4, msacEntropyDecoder{ctx: defaultPaletteAllDec})
	if err != nil {
		t.Fatalf("call8 default_palette_all trace: %v", err)
	}
}

func TestDebugGeneratedMain10FirstWrongLeafSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_FIRST_WRONG_LEAF") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-first-wrong-leaf.mp4",
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
	yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, nil)

	firstWrong := -1
	for i, entry := range entries {
		blockX := entry.Geometry.Start4X * 4
		blockY := entry.Geometry.Start4Y * 4
		blockW := entry.Geometry.Width4() * 4
		blockH := entry.Geometry.Height4() * 4
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		blockMSE := compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		if blockMSE == 0 {
			continue
		}
		if firstWrong < 0 {
			firstWrong = i
		}
		t.Logf("call=%d block_mse=%.2f %s", i, blockMSE, formatLeafTraceEntry(entry))
		if i >= firstWrong+4 {
			break
		}
	}
	if firstWrong < 0 {
		t.Log("no wrong leaf found before target")
		return
	}
	t.Logf("first_wrong_call=%d", firstWrong)
}

func TestDebugGeneratedMain10WorstLeafBlockMSE(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_WORST_LEAF_BLOCK_MSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-worst-leaf-block-mse.mp4",
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
	yPlane, yStride, _, _, _, _ := debugDecodeGeneratedMain10RawWithLeafReaders(t, fixture.Path, nil)

	type scoredLeaf struct {
		call  int
		mse   float64
		entry debugLeafTraceEntry
	}
	scored := make([]scoredLeaf, 0, len(entries))
	for i, entry := range entries {
		blockX := entry.Geometry.Start4X * 4
		blockY := entry.Geometry.Start4Y * 4
		blockW := entry.Geometry.Width4() * 4
		blockH := entry.Geometry.Height4() * 4
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		scored = append(scored, scoredLeaf{
			call:  i,
			mse:   compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH),
			entry: entry,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].mse > scored[j].mse
	})
	for i := 0; i < len(scored) && i < 12; i++ {
		leaf := scored[i]
		t.Logf("rank=%d call=%d block_mse=%.2f %s", i, leaf.call, leaf.mse, formatLeafTraceEntry(leaf.entry))
	}
}

func TestDebugGeneratedMain10Call4VariantSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-variants.mp4",
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
		{name: "uvmode_no_cfl_row", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode[1] = clone.UVMode[0]
			return clone
		})},
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
		{name: "default_uv_mode_only", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode = defaultModeCDFValue.UVMode
			return clone
		})},
		{name: "default_cfl_sign", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLSign = defaultModeCDFValue.CFLSign
			return clone
		})},
		{name: "default_cfl_alpha", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			return clone
		})},
		{name: "default_cfl_only", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLSign = defaultModeCDFValue.CFLSign
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			return clone
		})},
		{name: "default_uv_mode", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.UVMode = defaultModeCDFValue.UVMode
			clone.CFLSign = defaultModeCDFValue.CFLSign
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			clone.AngleDelta = defaultModeCDFValue.AngleDelta
			return clone
		})},
		{name: "local_no_palette_cache", read: debugReadIntraBlockSyntaxNoPaletteCache},
		{name: "default_pal_size", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true})},
		{name: "default_pal_y", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true})},
		{name: "default_palette_all", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true})},
		{name: "default_color_map", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{colorMap: true})},
		{name: "tx_before_palette_indices", read: debugReadIntraBlockSyntaxTxBeforePaletteIndices},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, overrides)
		if len(entries) <= 4 {
			t.Fatalf("%s traced entries=%d, want > 4", tc.name, len(entries))
		}
		entry := entries[4]
		parsed := openGeneratedMain10ParsedKeyframe(t, fixture.Path)
		var (
			state debugTargetLeafAdvanceState
			got   bool
		)
		yPlane, yStride, uPlane, uStride, vPlane, vStride := debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, overrides, func(callIdx int, captured debugTargetLeafAdvanceState) {
			if callIdx != 4 {
				return
			}
			state = captured
			got = true
		})
		if !got {
			t.Fatalf("%s did not capture call 4 state", tc.name)
		}
		blockX := state.syntax.Geometry.Start4X * 4
		blockY := state.syntax.Geometry.Start4Y * 4
		blockW := state.syntax.Geometry.Width4() * 4
		blockH := state.syntax.Geometry.Height4() * 4
		block := extractBlock16(yPlane, yStride, blockX, blockY, blockW, blockH)
		blockMSE := compareBlockToRefY16(block, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		uBlockMSE := -1.0
		vBlockMSE := -1.0
		chromaGeom := chromaGeometryForBlock(av1.Chroma420, state.syntax.Geometry)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			chromaX := chromaGeom.Start4X * 4
			chromaY := chromaGeom.Start4Y * 4
			chromaW := chromaGeom.Width4() * 4
			chromaH := chromaGeom.Height4() * 4
			chromaStride := (ref.Width + 1) >> 1
			uBlock := extractBlock16(uPlane, uStride, chromaX, chromaY, chromaW, chromaH)
			vBlock := extractBlock16(vPlane, vStride, chromaX, chromaY, chromaW, chromaH)
			uBlockMSE = compareBlockToRefY16(uBlock, chromaW, ref.U16, chromaStride, chromaX, chromaY, chromaW, chromaH)
			vBlockMSE = compareBlockToRefY16(vBlock, chromaW, ref.V16, chromaStride, chromaX, chromaY, chromaW, chromaH)
		}
		refBlock := extractBlock16(ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
		baseMSE := -1.0
		fullMSE := -1.0
		oracleBaseMSE := -1.0
		oraclePal := [8]uint16{}
		monotoneBaseMSE := -1.0
		monotonePal := [8]uint16{}
		reverseDiagBaseMSE := -1.0
		rowMajorBaseMSE := -1.0
		txLabel := "n/a"
		if state.syntax.PalYSize != 0 {
			baseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			oraclePal = fitPaletteValuesToRef16(state.syntax.PalYIdx, int(state.syntax.PalYSize), blockW, blockH, refBlock, ref.BitDepth)
			oracleBaseMSE = paletteBaseMSE16(oraclePal, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			monotonePal = fitMonotonePaletteValuesToRef16(state.syntax.PalYIdx, int(state.syntax.PalYSize), blockW, blockH, refBlock, ref.BitDepth)
			monotoneBaseMSE = paletteBaseMSE16(monotonePal, int(state.syntax.PalYSize), state.syntax.PalYIdx, blockW, blockH, refBlock)
			reverseDiagIdx := paletteIndicesReverseDiagonals(state.syntax.PalYIdx, blockW, blockH)
			rowMajorIdx := paletteIndicesDiagonalToRowMajor(state.syntax.PalYIdx, blockW, blockH)
			reverseDiagBaseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), reverseDiagIdx, blockW, blockH, refBlock)
			rowMajorBaseMSE = paletteBaseMSE16(state.syntax.PalY, int(state.syntax.PalYSize), rowMajorIdx, blockW, blockH, refBlock)
			if prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0); err != nil {
				t.Fatalf("%s cloneNthNonSkippedTUCoefficientInput: %v", tc.name, err)
			} else if ok {
				decoded, _, _, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
				fullMSE = paletteFullMSE16(t, state.syntax, state.syntax.PalY, state.syntax.PalYIdx, txType, decoded.coeffs, ref.BitDepth, parsed.Header.Segmentation.Lossless[state.syntax.Segment], refBlock, blockW, blockH)
				txLabel = "txType=" + itoaDebugInt(int(txType))
			}
		}
		cache, nCache := paletteCache(&state.above, &state.left, 0, state.syntax.Geometry.Start4X&31, state.syntax.Geometry.Start4Y&31)
		cacheVals := append([]uint16(nil), cache[:nCache]...)
		t.Logf("%s traced=%s", tc.name, formatLeafTraceEntry(entry))
		t.Logf("%s actual=%s", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		t.Logf("%s call4 block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v tx=%s cache=%v pal=%v idx0=%v",
			tc.name,
			blockMSE,
			uBlockMSE,
			vBlockMSE,
			baseMSE,
			fullMSE,
			state.syntax.CFLAlpha,
			txLabel,
			cacheVals,
			state.syntax.PalY[:state.syntax.PalYSize],
			append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
		)
		if state.syntax.PalYSize != 0 {
			t.Logf("%s call4 oracle_base_mse=%.2f oracle_pal=%v", tc.name, oracleBaseMSE, oraclePal[:state.syntax.PalYSize])
			t.Logf("%s call4 monotone_base_mse=%.2f monotone_pal=%v", tc.name, monotoneBaseMSE, monotonePal[:state.syntax.PalYSize])
			t.Logf("%s call4 reverse_diag_base_mse=%.2f row_major_base_mse=%.2f", tc.name, reverseDiagBaseMSE, rowMajorBaseMSE)
		}
	}
}

func TestDebugGeneratedMain10Call4PaletteTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) <= 4 {
		t.Fatalf("traced entries=%d, want > 4", len(entries))
	}
	t.Logf("call4 traced=%s", formatLeafTraceEntry(entries[4]))

	traceReader := func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, entropy intraEntropyDecoder) (IntraBlockSyntax, error) {
		var traceCtx *msac.Context
		switch dec := entropy.(type) {
		case msacEntropyDecoder:
			if dec.ctx == nil {
				t.Fatal("nil msac entropy decoder")
			}
			traceCtx = dec.ctx.Clone()
		case *msac.Context:
			traceCtx = dec.Clone()
		default:
			t.Fatalf("unexpected entropy decoder type %T", entropy)
		}
		debugTraceTargetClonedIntraSyntax(t, seq, hdr, cdf, above, left, g, g, haveTop, haveLeft, segID, traceCtx.Clone())
		debugTraceTargetPaletteDecision(t, seq, hdr, cdf, above, left, g, g, haveTop, haveLeft, segID, traceCtx)
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, entropy)
	}

	var (
		state debugTargetLeafAdvanceState
		got   bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, map[int]debugIntraSyntaxReader{4: traceReader}, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 4 {
			return
		}
		state = captured
		got = true
	})
	if !got {
		t.Fatal("did not capture call 4 state")
	}
	t.Logf("call4 actual=%s", formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
}

func debugTraceCall4EntropyState(dec intraEntropyDecoder) (int, int, int, int, bool) {
	switch v := dec.(type) {
	case msacEntropyDecoder:
		if v.ctx == nil {
			return 0, 0, 0, 0, false
		}
		cur, rng, cnt, pos := v.ctx.DebugState()
		return int(cur), int(rng), cnt, pos, true
	case *msac.Context:
		if v == nil {
			return 0, 0, 0, 0, false
		}
		cur, rng, cnt, pos := v.DebugState()
		return int(cur), int(rng), cnt, pos, true
	default:
		return 0, 0, 0, 0, false
	}
}

func debugTraceCall4ReadCFLAlpha(t *testing.T, label string, cdf *ModeCDF, dec intraEntropyDecoder, syntax *IntraBlockSyntax) error {
	if t == nil || cdf == nil || dec == nil || syntax == nil || syntax.UVMode != IntraPredCFL {
		return nil
	}
	cur, rng, cnt, pos, ok := debugTraceCall4EntropyState(dec)
	if ok {
		t.Logf("%s cfl before current=%d rng=%d cnt=%d pos=%d", label, cur, rng, cnt, pos)
	}
	signTok := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7))
	sign := signTok + 1
	signU := sign * 0x56 >> 8
	signV := sign - signU*3
	t.Logf("%s cfl signTok=%d signU=%d signV=%d", label, signTok, signU, signV)
	if signU != 0 {
		ctx := boolToInt(signU == 2)*3 + signV
		alphaTok := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15))
		alphaVal := alphaTok + 1
		if signU == 1 {
			alphaVal = -alphaVal
		}
		syntax.CFLAlpha[0] = int8(alphaVal)
		t.Logf("%s cfl alphaU ctx=%d tok=%d val=%d", label, ctx, alphaTok, alphaVal)
	}
	if signV != 0 {
		ctx := boolToInt(signV == 2)*3 + signU
		alphaTok := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15))
		alphaVal := alphaTok + 1
		if signV == 1 {
			alphaVal = -alphaVal
		}
		syntax.CFLAlpha[1] = int8(alphaVal)
		t.Logf("%s cfl alphaV ctx=%d tok=%d val=%d", label, ctx, alphaTok, alphaVal)
	}
	cur, rng, cnt, pos, ok = debugTraceCall4EntropyState(dec)
	if ok {
		t.Logf("%s cfl after current=%d rng=%d cnt=%d pos=%d", label, cur, rng, cnt, pos)
	}
	return nil
}

func debugTraceCall4VariantReader(t *testing.T, label, mode string) debugIntraSyntaxReader {
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
		if cur, rng, cnt, pos, ok := debugTraceCall4EntropyState(dec); ok {
			t.Logf("%s after ymode=%s current=%d rng=%d cnt=%d pos=%d", label, intraModeName(syntax.YMode), cur, rng, cnt, pos)
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
			if cur, rng, cnt, pos, ok := debugTraceCall4EntropyState(dec); ok {
				t.Logf("%s after uvmode=%s uvangle=%d current=%d rng=%d cnt=%d pos=%d", label, intraModeName(syntax.UVMode), syntax.UVAngle, cur, rng, cnt, pos)
			}
		}

		readYPalette := func(traceLabel string) error {
			if !paletteAllowedForBlock(hdr, g) || syntax.YMode != IntraPredDC {
				return nil
			}
			sizeCtx := paletteSizeContext(g)
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			usePal := dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0
			if cur, rng, cnt, pos, ok := debugTraceCall4EntropyState(dec); ok {
				t.Logf("%s palY=%v sizeCtx=%d palCtx=%d current=%d rng=%d cnt=%d pos=%d", traceLabel, usePal, sizeCtx, palCtx, cur, rng, cnt, pos)
			}
			if !usePal {
				return nil
			}
			size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
			if err != nil {
				return err
			}
			syntax.PalYSize = size
			syntax.PalY = values
			t.Logf("%s pal=%v", traceLabel, syntax.PalY[:syntax.PalYSize])
			return nil
		}
		readYIdx := func(traceLabel string) error {
			if syntax.PalYSize == 0 {
				return nil
			}
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
			if err != nil {
				return err
			}
			syntax.PalYIdx = indices
			head := minInt(16, len(indices))
			t.Logf("%s idx0=%v", traceLabel, indices[:head])
			return nil
		}

		switch mode {
		case "current":
			if err := debugTraceCall4ReadCFLAlpha(t, label, cdf, dec, &syntax); err != nil {
				return syntax, err
			}
			if err := readYPalette(label + " ypal"); err != nil {
				return syntax, err
			}
		case "skip_cfl_alpha":
			if err := readYPalette(label + " ypal"); err != nil {
				return syntax, err
			}
		case "after_luma_palette":
			if err := readYPalette(label + " ypal"); err != nil {
				return syntax, err
			}
			if err := debugTraceCall4ReadCFLAlpha(t, label, cdf, dec, &syntax); err != nil {
				return syntax, err
			}
		case "after_luma_indices":
			if err := readYPalette(label + " ypal"); err != nil {
				return syntax, err
			}
			if err := readYIdx(label + " yidx"); err != nil {
				return syntax, err
			}
			if err := debugTraceCall4ReadCFLAlpha(t, label, cdf, dec, &syntax); err != nil {
				return syntax, err
			}
		default:
			return syntax, fmt.Errorf("unknown call4 trace mode %q", mode)
		}

		if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
			if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
				syntax.YMode = IntraPredFilter
				syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
			}
		}
		if mode != "after_luma_indices" {
			if err := readYIdx(label + " yidx"); err != nil {
				return syntax, err
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

func TestDebugGeneratedMain10Call4ActualVariantTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_ACTUAL_VARIANT_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-actual-variant-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	variants := []struct {
		name string
		mode string
	}{
		{name: "current", mode: "current"},
		{name: "skip_cfl_alpha", mode: "skip_cfl_alpha"},
		{name: "after_luma_palette", mode: "after_luma_palette"},
		{name: "after_luma_indices", mode: "after_luma_indices"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			var (
				state debugTargetLeafAdvanceState
				got   bool
			)
			debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, map[int]debugIntraSyntaxReader{4: debugTraceCall4VariantReader(t, tc.name, tc.mode)}, func(callIdx int, captured debugTargetLeafAdvanceState) {
				if callIdx != 4 {
					return
				}
				state = captured
				got = true
			})
			if !got {
				t.Fatalf("%s did not capture call 4 state", tc.name)
			}
			t.Logf("%s final=%s", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(state)))
		})
	}
}

func extractBlock16(src []uint16, stride, x, y, width, height int) []uint16 {
	if width <= 0 || height <= 0 {
		return nil
	}
	out := make([]uint16, width*height)
	for row := 0; row < height; row++ {
		copy(out[row*width:(row+1)*width], src[(y+row)*stride+x:(y+row)*stride+x+width])
	}
	return out
}

func debugModeCDFCloneReader(build func(*ModeCDF) *ModeCDF) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if build == nil {
			return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
		}
		return ReadIntraBlockSyntax(seq, hdr, build(cdf), above, left, g, haveTop, haveLeft, segID, dec)
	}
}

func uniqueSortedUint16(src []uint16) []uint16 {
	if len(src) == 0 {
		return nil
	}
	seen := make(map[uint16]struct{}, len(src))
	out := make([]uint16, 0, len(src))
	for _, v := range src {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func paletteBaseMSE16(palette [8]uint16, size int, idx []uint8, width, height int, refBlock []uint16) float64 {
	if size <= 0 || width <= 0 || height <= 0 || len(idx) != width*height || len(refBlock) != width*height {
		return 0
	}
	mse := 0.0
	for i, palIdx := range idx {
		if int(palIdx) >= size {
			continue
		}
		d := int(palette[palIdx]) - int(refBlock[i])
		mse += float64(d * d)
	}
	return mse / float64(width*height)
}

func paletteFullMSE16(t *testing.T, syntax IntraBlockSyntax, palette [8]uint16, idx []uint8, txType TxType, coeffs []int32, bitDepth uint8, lossless bool, refBlock []uint16, width, height int) float64 {
	t.Helper()

	alt := syntax
	alt.PalY = palette
	alt.PalYIdx = append([]uint8(nil), idx...)
	block := reconstructPaletteLiteralBlock16(t, alt, txType, coeffs, bitDepth, lossless)
	return compareBlockToRefY16(block, width, refBlock, width, 0, 0, width, height)
}

func fitPaletteValuesToRef16(idx []uint8, size, width, height int, refBlock []uint16, bitDepth uint8) [8]uint16 {
	var (
		sum   [8]int
		count [8]int
		pal   [8]uint16
	)
	for i, palIdx := range idx {
		if int(palIdx) >= size || i >= len(refBlock) {
			continue
		}
		sum[palIdx] += int(refBlock[i])
		count[palIdx]++
	}
	for i := 0; i < size; i++ {
		if count[i] == 0 {
			continue
		}
		pal[i] = clipSample((sum[i]+count[i]/2)/count[i], bitDepth)
	}
	return pal
}

func fitMonotonePaletteValuesToRef16(idx []uint8, size, width, height int, refBlock []uint16, bitDepth uint8) [8]uint16 {
	type block struct {
		start int
		end   int
		sum   int
		count int
	}
	var (
		sum   [8]int
		count [8]int
		pal   [8]uint16
	)
	for i, palIdx := range idx {
		if int(palIdx) >= size || i >= len(refBlock) {
			continue
		}
		sum[palIdx] += int(refBlock[i])
		count[palIdx]++
	}
	blocks := make([]block, 0, size)
	for i := 0; i < size; i++ {
		blocks = append(blocks, block{
			start: i,
			end:   i,
			sum:   sum[i],
			count: count[i],
		})
		for len(blocks) >= 2 {
			n := len(blocks)
			prev := blocks[n-2]
			cur := blocks[n-1]
			prevMean := 0.0
			if prev.count > 0 {
				prevMean = float64(prev.sum) / float64(prev.count)
			}
			curMean := 0.0
			if cur.count > 0 {
				curMean = float64(cur.sum) / float64(cur.count)
			}
			if prevMean <= curMean {
				break
			}
			blocks[n-2] = block{
				start: prev.start,
				end:   cur.end,
				sum:   prev.sum + cur.sum,
				count: prev.count + cur.count,
			}
			blocks = blocks[:n-1]
		}
	}
	for _, blk := range blocks {
		value := 0
		if blk.count > 0 {
			value = (blk.sum + blk.count/2) / blk.count
		}
		clipped := clipSample(value, bitDepth)
		for i := blk.start; i <= blk.end && i < len(pal); i++ {
			pal[i] = clipped
		}
	}
	return pal
}

func bestPaletteAssignmentFromSet16(idx []uint8, size, width, height int, refBlock []uint16, values []uint16) ([8]uint16, float64) {
	bestPal := [8]uint16{}
	bestMSE := -1.0
	if size <= 0 || len(values) < size {
		return bestPal, 0
	}

	cur := [8]uint16{}
	used := make([]bool, len(values))
	var choose func(int)
	choose = func(depth int) {
		if depth == size {
			mse := paletteBaseMSE16(cur, size, idx, width, height, refBlock)
			if bestMSE < 0 || mse < bestMSE {
				bestMSE = mse
				bestPal = cur
			}
			return
		}
		for i := 0; i < len(values); i++ {
			if used[i] {
				continue
			}
			used[i] = true
			cur[depth] = values[i]
			choose(depth + 1)
			used[i] = false
		}
	}
	choose(0)
	return bestPal, bestMSE
}
