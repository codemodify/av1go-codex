package decoder

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestDebugCompareCurrentAndDav1dLiteralCoeffProfiles(t *testing.T) {
	if os.Getenv("DEBUG_COMPARE_DAV1D_COEF") == "" {
		t.Skip("debug only")
	}

	cases := []struct {
		name             string
		fixture          testutil.AV1FixtureOptions
		targetX, targetY int
		nextX, nextY     int
	}{
		{
			name:    "generated-main8-first-leaf",
			fixture: testutil.AV1FixtureOptions{Name: "dav1d-compare-main8.mp4", Width: 160, Height: 90, FPS: 5, Frames: 6, BitDepth: 8},
			targetX: 0, targetY: 0, nextX: 32, nextY: 0,
		},
		{
			name:    "generated-main10-residual-leaf",
			fixture: testutil.AV1FixtureOptions{Name: "dav1d-compare-main10.mp4", Width: 128, Height: 72, FPS: 4, Frames: 6, BitDepth: 10},
			targetX: 100, targetY: 24, nextX: 104, nextY: 24,
		},
		{
			name:    "generated-main10-palette-target",
			fixture: testutil.AV1FixtureOptions{Name: "dav1d-compare-main10-palette.mp4", Width: 128, Height: 72, FPS: 4, Frames: 6, BitDepth: 10},
			targetX: 64, targetY: 0, nextX: 68, nextY: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), tc.fixture)
			dec, err := OpenMP4(fixture.Path)
			if err != nil {
				t.Fatalf("OpenMP4: %v", err)
			}
			defer dec.Close()

			parsed, state := generatedKeyframeLeafState(t, dec, tc.targetX, tc.targetY, tc.nextX, tc.nextY)
			prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
			if err != nil {
				t.Fatalf("cloneNthNonSkippedTUCoefficientInput: %v", err)
			}
			if !ok {
				t.Logf("no non-skipped TU: qidx=%d syntax=%v", state.qidx, state.syntax.TX)
				return
			}
			current, currentCDF, currentDec, txType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffCurrentProfile)
			dav1d, dav1dCDF, dav1dDec, dav1dTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffDav1dProfile)

			var diffs []string
			if txType != dav1dTxType {
				diffs = append(diffs, fmt.Sprintf("txType current=%d dav1d=%d", txType, dav1dTxType))
			}
			if current.skip != dav1d.skip {
				diffs = append(diffs, fmt.Sprintf("skip current=%v dav1d=%v", current.skip, dav1d.skip))
			}
			if current.resCtx != dav1d.resCtx {
				diffs = append(diffs, fmt.Sprintf("resCtx current=%#x dav1d=%#x", current.resCtx, dav1d.resCtx))
			}
			if !reflect.DeepEqual(current.coeffs, dav1d.coeffs) {
				diffs = append(diffs, fmt.Sprintf("coeffs current=%v dav1d=%v", summarizeNonZeroCoeffs(current.coeffs), summarizeNonZeroCoeffs(dav1d.coeffs)))
			}
			if idx, cur, ref, ok := firstLiteralTraceDiff(current.trace, dav1d.trace); ok {
				diffs = append(diffs, fmt.Sprintf("trace[%d] current=%s dav1d=%s", idx, formatLiteralTraceStep(cur), formatLiteralTraceStep(ref)))
			}
			if !reflect.DeepEqual(currentCDF, dav1dCDF) {
				diffs = append(diffs, "coef CDF state differs")
			}
			cur0, cur1, cur2, cur3 := currentDec.DebugState()
			dav0, dav1, dav2, dav3 := dav1dDec.DebugState()
			if cur0 != dav0 || cur1 != dav1 || cur2 != dav2 || cur3 != dav3 {
				diffs = append(diffs, fmt.Sprintf("msac current=(%d,%d,%d,%d) dav1d=(%d,%d,%d,%d)", cur0, cur1, cur2, cur3, dav0, dav1, dav2, dav3))
			}

			if len(diffs) == 0 {
				t.Logf("no difference: qidx=%d tu=%d nonSkipped=%d geom=(%d,%d)-(%d,%d) tx=%d syntax=%v",
					state.qidx, prepared.tuIndex, prepared.nonSkippedIndex,
					prepared.tu.Start4X*4, prepared.tu.Start4Y*4, prepared.tu.End4X*4, prepared.tu.End4Y*4,
					txType, state.syntax.TX)
				return
			}

			t.Fatalf("qidx=%d tu=%d nonSkipped=%d geom=(%d,%d)-(%d,%d) tx=%d syntax=%v diffs=%v",
				state.qidx, prepared.tuIndex, prepared.nonSkippedIndex,
				prepared.tu.Start4X*4, prepared.tu.Start4Y*4, prepared.tu.End4X*4, prepared.tu.End4Y*4,
				txType, state.syntax.TX, diffs)
		})
	}
}

func TestDebugCompareCurrentAndDav1dLiteralCoeffProfilesOnGeneratedMain10Call3Chroma(t *testing.T) {
	if os.Getenv("DEBUG_COMPARE_DAV1D_COEF_CHROMA") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "dav1d-compare-main10-call3-chroma.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) < 5 {
		t.Fatalf("need at least 5 traced leaves, got %d", len(entries))
	}
	call3Entry := entries[3]
	call4Entry := entries[4]

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	parsed, state := generatedKeyframeLeafState(t, dec,
		call3Entry.Geometry.Start4X*4, call3Entry.Geometry.Start4Y*4,
		call4Entry.Geometry.Start4X*4, call4Entry.Geometry.Start4Y*4,
	)
	seq := dec.header
	dec.Close()

	modeCDF := state.mode.Clone()
	baseCoef := state.coef.Clone()
	if baseCoef == nil {
		baseCoef = NewDefaultCoefCDF(uint8(state.qidx))
	}
	baseAbove := state.above
	baseLeft := state.left
	baseDec := state.dec.Clone()
	if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, baseCoef, state.dq, &baseAbove, &baseLeft, state.syntax, debugMSACCombinedDecoder{ctx: baseDec}); err != nil {
		t.Fatalf("debugConsumeFirstBlockLuma: %v", err)
	}

	currentCDF := baseCoef.Clone()
	dav1dCDF := baseCoef.Clone()
	currentAbove := baseAbove
	currentLeft := baseLeft
	dav1dAbove := baseAbove
	dav1dLeft := baseLeft
	currentDec := baseDec.Clone()
	dav1dDec := baseDec.Clone()

	layout := seq.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, state.syntax.Segment, state.syntax.UVTX, state.syntax.UVMode)
	units := buildChromaTransformUnits(layout, state.syntax.Geometry, state.syntax.UVTX)
	for plane := 1; plane <= 2; plane++ {
		for tuIndex, tu := range units {
			current, currentResCtx, currentSkip, currentTrace, err := decodeChromaCoefficientsLiteralWithProfile(state.syntax.Size, tu, state.syntax.UVTX, uvTxType, state.syntax.Segment, plane, layout, currentCDF, state.dq, &currentAbove, &currentLeft, msacCoefficientDecoder{ctx: currentDec}, literalCoeffCurrentProfile)
			if err != nil {
				t.Fatalf("current plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			dav1d, dav1dResCtx, dav1dSkip, dav1dTrace, err := decodeChromaCoefficientsLiteralWithProfile(state.syntax.Size, tu, state.syntax.UVTX, uvTxType, state.syntax.Segment, plane, layout, dav1dCDF, state.dq, &dav1dAbove, &dav1dLeft, msacCoefficientDecoder{ctx: dav1dDec}, literalCoeffDav1dProfile)
			if err != nil {
				t.Fatalf("dav1d plane=%d tu=%d: %v", plane, tuIndex, err)
			}

			var diffs []string
			if currentSkip != dav1dSkip {
				diffs = append(diffs, fmt.Sprintf("skip current=%v dav1d=%v", currentSkip, dav1dSkip))
			}
			if currentResCtx != dav1dResCtx {
				diffs = append(diffs, fmt.Sprintf("resCtx current=%#x dav1d=%#x", currentResCtx, dav1dResCtx))
			}
			if !reflect.DeepEqual(current, dav1d) {
				diffs = append(diffs, fmt.Sprintf("coeffs current=%v dav1d=%v", summarizeNonZeroCoeffs(current), summarizeNonZeroCoeffs(dav1d)))
			}
			if idx, cur, ref, ok := firstLiteralTraceDiff(currentTrace, dav1dTrace); ok {
				diffs = append(diffs, fmt.Sprintf("trace[%d] current=%s dav1d=%s", idx, formatLiteralTraceStep(cur), formatLiteralTraceStep(ref)))
			}
			if len(diffs) != 0 {
				t.Fatalf("plane=%d tu=%d geom=(%d,%d)-(%d,%d) diffs=%v", plane, tuIndex, tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4, diffs)
			}
			if err := ApplyChromaResidualContext(&currentAbove, &currentLeft, plane, tu, state.syntax.UVTX, currentResCtx); err != nil {
				t.Fatalf("ApplyChromaResidualContext current plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			if err := ApplyChromaResidualContext(&dav1dAbove, &dav1dLeft, plane, tu, state.syntax.UVTX, dav1dResCtx); err != nil {
				t.Fatalf("ApplyChromaResidualContext dav1d plane=%d tu=%d: %v", plane, tuIndex, err)
			}
		}
	}

	t.Logf("no current-vs-dav1d chroma coefficient differences after call3 luma: geom=(%d,%d)-(%d,%d) uvtx=%d uvtxtype=%d", state.syntax.Geometry.Start4X*4, state.syntax.Geometry.Start4Y*4, state.syntax.Geometry.End4X*4, state.syntax.Geometry.End4Y*4, state.syntax.UVTX, uvTxType)
}

func TestDebugCompareCurrentAndDav1dLiteralCoeffProfilesOnGeneratedMain10Call3Luma(t *testing.T) {
	if os.Getenv("DEBUG_COMPARE_DAV1D_COEF_CALL3_LUMA") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "dav1d-compare-main10-call3-luma.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) < 5 {
		t.Fatalf("need at least 5 traced leaves, got %d", len(entries))
	}
	call3Entry := entries[3]
	call4Entry := entries[4]

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	parsed, state := generatedKeyframeLeafState(t, dec,
		call3Entry.Geometry.Start4X*4, call3Entry.Geometry.Start4Y*4,
		call4Entry.Geometry.Start4X*4, call4Entry.Geometry.Start4Y*4,
	)
	dec.Close()

	currentMode := state.mode.Clone()
	dav1dMode := state.mode.Clone()
	currentCoef := state.coef.Clone()
	dav1dCoef := state.coef.Clone()
	if currentCoef == nil {
		currentCoef = NewDefaultCoefCDF(uint8(state.qidx))
	}
	if dav1dCoef == nil {
		dav1dCoef = NewDefaultCoefCDF(uint8(state.qidx))
	}
	currentAbove := state.above
	currentLeft := state.left
	dav1dAbove := state.above
	dav1dLeft := state.left
	currentDec := state.dec.Clone()
	dav1dDec := state.dec.Clone()

	currentUnits, err := ReadIntraTransformUnitsInto(nil, &parsed.Header, currentMode, &currentAbove, &currentLeft, state.syntax.Geometry, state.syntax.TX, state.syntax.Segment, msacEntropyDecoder{ctx: currentDec})
	if err != nil {
		t.Fatalf("ReadIntraTransformUnitsInto current: %v", err)
	}
	dav1dUnits, err := ReadIntraTransformUnitsInto(nil, &parsed.Header, dav1dMode, &dav1dAbove, &dav1dLeft, state.syntax.Geometry, state.syntax.TX, state.syntax.Segment, msacEntropyDecoder{ctx: dav1dDec})
	if err != nil {
		t.Fatalf("ReadIntraTransformUnitsInto dav1d: %v", err)
	}
	if !reflect.DeepEqual(currentUnits, dav1dUnits) {
		t.Fatalf("transform units mismatch current=%v dav1d=%v", currentUnits, dav1dUnits)
	}

	foundAny := false
	nonSkippedSeen := 0
	for tuIndex, unit := range currentUnits {
		tu := unit.Global
		tx := unit.TX
		tuHaveTop := state.haveTop || tu.Start4Y > state.syntax.Geometry.Start4Y
		tuHaveLeft := state.haveLeft || tu.Start4X > state.syntax.Geometry.Start4X
		txInfo := TxfmInfoFor(tx)
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		currentSkipCtx := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, currentAbove.LCoef[bx4:], currentLeft.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		dav1dSkipCtx := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, dav1dAbove.LCoef[bx4:], dav1dLeft.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		if currentSkipCtx != dav1dSkipCtx {
			t.Fatalf("skip ctx mismatch at TU %d current=%d dav1d=%d", tuIndex, currentSkipCtx, dav1dSkipCtx)
		}
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(currentCoef.Skip)-1)
		currentSkip := msacCoefficientDecoder{ctx: currentDec}.DecodeBoolAdapt(currentCoef.Skip[skipRow][currentSkipCtx][:]) != 0
		dav1dSkip := msacCoefficientDecoder{ctx: dav1dDec}.DecodeBoolAdapt(dav1dCoef.Skip[skipRow][dav1dSkipCtx][:]) != 0
		if currentSkip != dav1dSkip {
			t.Fatalf("skip mismatch at TU %d current=%v dav1d=%v", tuIndex, currentSkip, dav1dSkip)
		}
		if currentSkip {
			if err := ApplyLumaResidualContext(&currentAbove, &currentLeft, tu, tx, 0x40); err != nil {
				t.Fatalf("ApplyLumaResidualContext current skip TU %d: %v", tuIndex, err)
			}
			if err := ApplyLumaResidualContext(&dav1dAbove, &dav1dLeft, tu, tx, 0x40); err != nil {
				t.Fatalf("ApplyLumaResidualContext dav1d skip TU %d: %v", tuIndex, err)
			}
			continue
		}

		foundAny = true
		currentTxType, err := ReadIntraTxType(&parsed.Header, currentMode, state.syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: currentDec})
		if err != nil {
			t.Fatalf("ReadIntraTxType current TU %d: %v", tuIndex, err)
		}
		dav1dTxType, err := ReadIntraTxType(&parsed.Header, dav1dMode, state.syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: dav1dDec})
		if err != nil {
			t.Fatalf("ReadIntraTxType dav1d TU %d: %v", tuIndex, err)
		}

		currentCoeffs, currentResCtx, currentNoSkipSkip, currentTrace, err := decodeLumaCoefficientsLiteralNoSkipWithProfile(state.syntax.Size, tu, tx, currentTxType, state.syntax.Segment, currentCoef, state.dq, &currentAbove, &currentLeft, msacCoefficientDecoder{ctx: currentDec}, literalCoeffCurrentProfile)
		if err != nil {
			t.Fatalf("decodeLumaCoefficientsLiteralNoSkip current TU %d: %v", tuIndex, err)
		}
		dav1dCoeffs, dav1dResCtx, dav1dNoSkipSkip, dav1dTrace, err := decodeLumaCoefficientsLiteralNoSkipWithProfile(state.syntax.Size, tu, tx, dav1dTxType, state.syntax.Segment, dav1dCoef, state.dq, &dav1dAbove, &dav1dLeft, msacCoefficientDecoder{ctx: dav1dDec}, literalCoeffDav1dProfile)
		if err != nil {
			t.Fatalf("decodeLumaCoefficientsLiteralNoSkip dav1d TU %d: %v", tuIndex, err)
		}

		var diffs []string
		if currentTxType != dav1dTxType {
			diffs = append(diffs, fmt.Sprintf("txType current=%d dav1d=%d", currentTxType, dav1dTxType))
		}
		if currentNoSkipSkip != dav1dNoSkipSkip {
			diffs = append(diffs, fmt.Sprintf("skip current=%v dav1d=%v", currentNoSkipSkip, dav1dNoSkipSkip))
		}
		if currentResCtx != dav1dResCtx {
			diffs = append(diffs, fmt.Sprintf("resCtx current=%#x dav1d=%#x", currentResCtx, dav1dResCtx))
		}
		if !reflect.DeepEqual(currentCoeffs, dav1dCoeffs) {
			diffs = append(diffs, fmt.Sprintf("coeffs current=%v dav1d=%v", summarizeNonZeroCoeffs(currentCoeffs), summarizeNonZeroCoeffs(dav1dCoeffs)))
		}
		if idx, cur, ref, ok := firstLiteralTraceDiff(currentTrace, dav1dTrace); ok {
			diffs = append(diffs, fmt.Sprintf("trace[%d] current=%s dav1d=%s", idx, formatLiteralTraceStep(cur), formatLiteralTraceStep(ref)))
		}
		if !reflect.DeepEqual(currentCoef, dav1dCoef) {
			diffs = append(diffs, "coef CDF state differs")
		}
		cur0, cur1, cur2, cur3 := currentDec.DebugState()
		dav0, dav1, dav2, dav3 := dav1dDec.DebugState()
		if cur0 != dav0 || cur1 != dav1 || cur2 != dav2 || cur3 != dav3 {
			diffs = append(diffs, fmt.Sprintf("msac current=(%d,%d,%d,%d) dav1d=(%d,%d,%d,%d)", cur0, cur1, cur2, cur3, dav0, dav1, dav2, dav3))
		}
		if len(diffs) != 0 {
			t.Fatalf("nonSkipped=%d tu=%d geom=(%d,%d)-(%d,%d) diffs=%v",
				nonSkippedSeen, tuIndex, tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4, diffs)
		}

		if err := ApplyLumaResidualContext(&currentAbove, &currentLeft, tu, tx, currentResCtx); err != nil {
			t.Fatalf("ApplyLumaResidualContext current TU %d: %v", tuIndex, err)
		}
		if err := ApplyLumaResidualContext(&dav1dAbove, &dav1dLeft, tu, tx, dav1dResCtx); err != nil {
			t.Fatalf("ApplyLumaResidualContext dav1d TU %d: %v", tuIndex, err)
		}
		nonSkippedSeen++
	}

	if !foundAny {
		t.Fatal("expected at least one non-skipped call3 luma TU")
	}
}

func firstLiteralTraceDiff(current, dav1d []literalCoeffTraceStep) (int, literalCoeffTraceStep, literalCoeffTraceStep, bool) {
	n := minInt(len(current), len(dav1d))
	for i := 0; i < n; i++ {
		if current[i] != dav1d[i] {
			return i, current[i], dav1d[i], true
		}
	}
	if len(current) != len(dav1d) {
		var cur literalCoeffTraceStep
		var ref literalCoeffTraceStep
		if len(current) > n {
			cur = current[n]
		}
		if len(dav1d) > n {
			ref = dav1d[n]
		}
		return n, cur, ref, true
	}
	return 0, literalCoeffTraceStep{}, literalCoeffTraceStep{}, false
}

func formatLiteralTraceStep(step literalCoeffTraceStep) string {
	return fmt.Sprintf("%s(order=%d rc=%d ctx=%d tok=%d)", step.label, step.order, step.rc, step.ctx, step.tok)
}
