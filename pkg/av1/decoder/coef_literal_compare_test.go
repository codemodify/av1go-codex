package decoder

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type literalCoeffTraceStep struct {
	label string
	order int
	rc    int
	ctx   int
	tok   uint32
}

type literalCoeffResult struct {
	coeffs []int32
	resCtx uint8
	skip   bool
	trace  []literalCoeffTraceStep
}

type preparedTUCoefficientInput struct {
	coefCDF         *CoefCDF
	dec             *msac.Context
	txType          TxType
	tu              BlockGeometry
	above           BlockContext
	left            BlockContext
	tuIndex         int
	nonSkippedIndex int
}

func (p preparedTUCoefficientInput) Clone() preparedTUCoefficientInput {
	clone := p
	if p.coefCDF != nil {
		clone.coefCDF = p.coefCDF.Clone()
	}
	if p.dec != nil {
		clone.dec = p.dec.Clone()
	}
	return clone
}

type literalCoeffProfile struct {
	name        string
	eobHiBitCtx func(TxfmInfo) int
	eobBaseCtx  func(TxfmInfo, int) int
}

var (
	literalCoeffCurrentProfile = literalCoeffProfile{
		name:        "current",
		eobHiBitCtx: eobHiBitCtx,
		eobBaseCtx:  eobBaseCtx,
	}
	literalCoeffDav1dProfile = literalCoeffProfile{
		name:        "dav1d",
		eobHiBitCtx: func(info TxfmInfo) int { return int(info.Ctx) },
		eobBaseCtx:  literalDav1dEOBBaseCtx,
	}
)

func literalDav1dEOBBaseCtx(info TxfmInfo, eob int) int {
	tx2dCtx := eobTx2DCtx(info)
	return clampInt(1+boolToInt(eob > (2<<tx2dCtx))+boolToInt(eob > (4<<tx2dCtx)), 0, 3)
}

func TestDecodeLumaCoefficientsNoSkipMatchesLiteralParserOnGeneratedFirstNonSkippedTU(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-compare-main8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   6,
		BitDepth: 8,
	})

	state, prepared, ok := firstGeneratedMain8NonSkippedTUInput(t, fixture.Path)
	if !ok {
		t.Skip("generated main8 fixture did not expose a non-skipped luma residual TU in the scanned keyframe leaves")
	}

	current, currentCDF, currentDec, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
	literal, literalCDF, literalDec, literalTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffCurrentProfile)
	if txType != literalTxType {
		t.Fatalf("tx type mismatch: current=%d literal=%d", txType, literalTxType)
	}
	if current.skip != literal.skip {
		t.Fatalf("skip mismatch: current=%v literal=%v", current.skip, literal.skip)
	}
	if current.resCtx != literal.resCtx {
		t.Fatalf("res ctx mismatch: current=%#x literal=%#x literal trace=%v", current.resCtx, literal.resCtx, literal.trace)
	}
	if !reflect.DeepEqual(current.coeffs, literal.coeffs) {
		t.Fatalf("coeff mismatch:\ncurrent=%v\nliteral=%v\nliteral trace=%v", summarizeNonZeroCoeffs(current.coeffs), summarizeNonZeroCoeffs(literal.coeffs), literal.trace)
	}
	cur0, cur1, cur2, cur3 := currentDec.DebugState()
	lit0, lit1, lit2, lit3 := literalDec.DebugState()
	if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
		t.Fatalf("decoder state mismatch: current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d) literal trace=%v", cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3, literal.trace)
	}
	if !reflect.DeepEqual(currentCDF, literalCDF) {
		t.Fatalf("coef cdf mismatch after first TU parse; literal trace=%v", literal.trace)
	}
}

func TestDecodeLumaCoefficientsNoSkipMatchesLiteralParserOnGeneratedMain10ResidualTU(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-compare-main10-residual-tu.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entries, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	if len(entries) < 2 {
		t.Fatalf("need at least 2 traced leaves, got %d", len(entries))
	}

	var (
		parsed   *ParsedFrame
		state    debugTargetLeafAdvanceState
		prepared preparedTUCoefficientInput
		ok       bool
	)
	found := false
	for i := 0; i+1 < len(entries); i++ {
		entry := entries[i]
		if entry.Syntax.PalYSize != 0 {
			continue
		}
		dec, err := OpenMP4(fixture.Path)
		if err != nil {
			t.Fatalf("OpenMP4(call=%d): %v", i, err)
		}
		parsed, state = generatedKeyframeLeafState(t, dec,
			entry.Geometry.Start4X*4, entry.Geometry.Start4Y*4,
			entries[i+1].Geometry.Start4X*4, entries[i+1].Geometry.Start4Y*4,
		)
		dec.Close()
		prepared, ok, err = cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
		if err != nil {
			t.Fatalf("cloneNthNonSkippedTUCoefficientInput(call=%d): %v", i, err)
		}
		if ok {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one non-skipped luma residual TU in traced generated Main10 leaves")
	}
	current, currentCDF, currentDec, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
	literal, literalCDF, literalDec, literalTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffCurrentProfile)
	if txType != literalTxType {
		t.Fatalf("tx type mismatch at TU %d: current=%d literal=%d", prepared.tuIndex, txType, literalTxType)
	}
	if current.skip != literal.skip {
		t.Fatalf("skip mismatch at TU %d: current=%v literal=%v", prepared.tuIndex, current.skip, literal.skip)
	}
	if current.resCtx != literal.resCtx {
		t.Fatalf("res ctx mismatch at TU %d: current=%#x literal=%#x literal trace=%v", prepared.tuIndex, current.resCtx, literal.resCtx, literal.trace)
	}
	if !reflect.DeepEqual(current.coeffs, literal.coeffs) {
		t.Fatalf("coeff mismatch at TU %d:\ncurrent=%v\nliteral=%v\nliteral trace=%v", prepared.tuIndex, summarizeNonZeroCoeffs(current.coeffs), summarizeNonZeroCoeffs(literal.coeffs), literal.trace)
	}
	cur0, cur1, cur2, cur3 := currentDec.DebugState()
	lit0, lit1, lit2, lit3 := literalDec.DebugState()
	if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
		t.Fatalf("decoder state mismatch at TU %d: current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d) literal trace=%v", prepared.tuIndex, cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3, literal.trace)
	}
	if !reflect.DeepEqual(currentCDF, literalCDF) {
		t.Fatalf("coef cdf mismatch at TU %d after parse; literal trace=%v", prepared.tuIndex, literal.trace)
	}
}

func TestDecodeLumaCoefficientsNoSkipMatchesLiteralParserOnGeneratedMain10PaletteTU(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-compare-main10-palette-tu.mp4",
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
	defer dec.Close()

	parsed, state := generatedKeyframeLeafState(t, dec, 64, 0, 68, 0)
	prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
	if err != nil {
		t.Fatalf("cloneNthNonSkippedTUCoefficientInput: %v", err)
	}
	if !ok {
		t.Fatal("expected at least one non-skipped TU in palette target block")
	}
	current, currentCDF, currentDec, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
	literal, literalCDF, literalDec, literalTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffCurrentProfile)
	if txType != literalTxType {
		t.Fatalf("tx type mismatch at TU %d: current=%d literal=%d", prepared.tuIndex, txType, literalTxType)
	}
	if current.skip != literal.skip {
		t.Fatalf("skip mismatch at TU %d: current=%v literal=%v", prepared.tuIndex, current.skip, literal.skip)
	}
	if current.resCtx != literal.resCtx {
		t.Fatalf("res ctx mismatch at TU %d: current=%#x literal=%#x literal trace=%v", prepared.tuIndex, current.resCtx, literal.resCtx, literal.trace)
	}
	if !reflect.DeepEqual(current.coeffs, literal.coeffs) {
		t.Fatalf("coeff mismatch at TU %d:\ncurrent=%v\nliteral=%v\nliteral trace=%v", prepared.tuIndex, summarizeNonZeroCoeffs(current.coeffs), summarizeNonZeroCoeffs(literal.coeffs), literal.trace)
	}
	cur0, cur1, cur2, cur3 := currentDec.DebugState()
	lit0, lit1, lit2, lit3 := literalDec.DebugState()
	if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
		t.Fatalf("decoder state mismatch at TU %d: current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d) literal trace=%v", prepared.tuIndex, cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3, literal.trace)
	}
	if !reflect.DeepEqual(currentCDF, literalCDF) {
		t.Fatalf("coef cdf mismatch at TU %d after parse; literal trace=%v", prepared.tuIndex, literal.trace)
	}
}

func TestDecodeGeneratedMain10Call3ChromaMatchesLiteralParser(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-compare-main10-call3-chroma.mp4",
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
	literalCDF := baseCoef.Clone()
	currentAbove := baseAbove
	currentLeft := baseLeft
	literalAbove := baseAbove
	literalLeft := baseLeft
	currentDec := baseDec.Clone()
	literalDec := baseDec.Clone()

	layout := seq.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, state.syntax.Segment, state.syntax.UVTX, state.syntax.UVMode)
	units := buildChromaTransformUnits(layout, state.syntax.Geometry, state.syntax.UVTX)
	for plane := 1; plane <= 2; plane++ {
		for tuIndex, tu := range units {
			current, currentResCtx, currentSkip, err := DecodeChromaCoefficients(state.syntax.Size, tu, state.syntax.UVTX, uvTxType, state.syntax.Segment, plane, layout, currentCDF, state.dq, &currentAbove, &currentLeft, msacCoefficientDecoder{ctx: currentDec})
			if err != nil {
				t.Fatalf("DecodeChromaCoefficients plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			literal, literalResCtx, literalSkip, literalTrace, err := decodeChromaCoefficientsLiteralWithProfile(state.syntax.Size, tu, state.syntax.UVTX, uvTxType, state.syntax.Segment, plane, layout, literalCDF, state.dq, &literalAbove, &literalLeft, msacCoefficientDecoder{ctx: literalDec}, literalCoeffCurrentProfile)
			if err != nil {
				t.Fatalf("decodeChromaCoefficientsLiteral plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			if currentSkip != literalSkip {
				t.Fatalf("skip mismatch plane=%d tu=%d current=%v literal=%v trace=%v", plane, tuIndex, currentSkip, literalSkip, literalTrace)
			}
			if currentResCtx != literalResCtx {
				t.Fatalf("res ctx mismatch plane=%d tu=%d current=%#x literal=%#x trace=%v", plane, tuIndex, currentResCtx, literalResCtx, literalTrace)
			}
			if !reflect.DeepEqual(current, literal) {
				t.Fatalf("coeff mismatch plane=%d tu=%d current=%v literal=%v trace=%v", plane, tuIndex, summarizeNonZeroCoeffs(current), summarizeNonZeroCoeffs(literal), literalTrace)
			}
			cur0, cur1, cur2, cur3 := currentDec.DebugState()
			lit0, lit1, lit2, lit3 := literalDec.DebugState()
			if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
				t.Fatalf("decoder state mismatch plane=%d tu=%d current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d) trace=%v", plane, tuIndex, cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3, literalTrace)
			}
			if err := ApplyChromaResidualContext(&currentAbove, &currentLeft, plane, tu, state.syntax.UVTX, currentResCtx); err != nil {
				t.Fatalf("ApplyChromaResidualContext current plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			if err := ApplyChromaResidualContext(&literalAbove, &literalLeft, plane, tu, state.syntax.UVTX, literalResCtx); err != nil {
				t.Fatalf("ApplyChromaResidualContext literal plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			if currentAbove != literalAbove || currentLeft != literalLeft {
				t.Fatalf("block context mismatch after chroma plane=%d tu=%d trace=%v", plane, tuIndex, literalTrace)
			}
			if !reflect.DeepEqual(currentCDF, literalCDF) {
				t.Fatalf("coef cdf mismatch after chroma plane=%d tu=%d trace=%v", plane, tuIndex, literalTrace)
			}
		}
	}

	cur0, cur1, cur2, cur3 := currentDec.DebugState()
	lit0, lit1, lit2, lit3 := literalDec.DebugState()
	if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
		t.Fatalf("final decoder state mismatch current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d)", cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3)
	}
	if currentAbove != literalAbove || currentLeft != literalLeft {
		t.Fatal("final block context mismatch after chroma replay")
	}
	if !reflect.DeepEqual(currentCDF, literalCDF) {
		t.Fatal("final coef cdf mismatch after chroma replay")
	}
}

func TestDecodeGeneratedMain10Call3LumaMatchesLiteralParser(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-compare-main10-call3-luma.mp4",
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
	literalMode := state.mode.Clone()
	currentCoef := state.coef.Clone()
	literalCoef := state.coef.Clone()
	if currentCoef == nil {
		currentCoef = NewDefaultCoefCDF(uint8(state.qidx))
	}
	if literalCoef == nil {
		literalCoef = NewDefaultCoefCDF(uint8(state.qidx))
	}
	currentAbove := state.above
	currentLeft := state.left
	literalAbove := state.above
	literalLeft := state.left
	currentDec := state.dec.Clone()
	literalDec := state.dec.Clone()

	currentUnits, err := ReadIntraTransformUnitsInto(nil, &parsed.Header, currentMode, &currentAbove, &currentLeft, state.syntax.Geometry, state.syntax.TX, state.syntax.Segment, msacEntropyDecoder{ctx: currentDec})
	if err != nil {
		t.Fatalf("ReadIntraTransformUnitsInto current: %v", err)
	}
	literalUnits, err := ReadIntraTransformUnitsInto(nil, &parsed.Header, literalMode, &literalAbove, &literalLeft, state.syntax.Geometry, state.syntax.TX, state.syntax.Segment, msacEntropyDecoder{ctx: literalDec})
	if err != nil {
		t.Fatalf("ReadIntraTransformUnitsInto literal: %v", err)
	}
	if !reflect.DeepEqual(currentUnits, literalUnits) {
		t.Fatalf("transform units mismatch current=%v literal=%v", currentUnits, literalUnits)
	}
	if !reflect.DeepEqual(currentMode, literalMode) {
		t.Fatal("mode cdf mismatch after transform-unit read")
	}
	cur0, cur1, cur2, cur3 := currentDec.DebugState()
	lit0, lit1, lit2, lit3 := literalDec.DebugState()
	if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
		t.Fatalf("decoder state mismatch after transform-unit read current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d)", cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3)
	}

	for tuIndex, unit := range currentUnits {
		tu := unit.Global
		tx := unit.TX
		tuHaveTop := state.haveTop || tu.Start4Y > state.syntax.Geometry.Start4Y
		tuHaveLeft := state.haveLeft || tu.Start4X > state.syntax.Geometry.Start4X
		txInfo := TxfmInfoFor(tx)
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtxCurrent := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, currentAbove.LCoef[bx4:], currentLeft.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		skipCtxLiteral := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, literalAbove.LCoef[bx4:], literalLeft.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		if skipCtxCurrent != skipCtxLiteral {
			t.Fatalf("skip ctx mismatch at TU %d current=%d literal=%d", tuIndex, skipCtxCurrent, skipCtxLiteral)
		}
		skipRowCurrent := coeffCDFRow(int(txInfo.Ctx), "skip", len(currentCoef.Skip)-1)
		skipRowLiteral := coeffCDFRow(int(txInfo.Ctx), "skip", len(literalCoef.Skip)-1)
		if skipRowCurrent != skipRowLiteral {
			t.Fatalf("skip row mismatch at TU %d current=%d literal=%d", tuIndex, skipRowCurrent, skipRowLiteral)
		}

		currentSkip := msacCoefficientDecoder{ctx: currentDec}.DecodeBoolAdapt(currentCoef.Skip[skipRowCurrent][skipCtxCurrent][:]) != 0
		literalSkip := msacCoefficientDecoder{ctx: literalDec}.DecodeBoolAdapt(literalCoef.Skip[skipRowLiteral][skipCtxLiteral][:]) != 0
		if currentSkip != literalSkip {
			t.Fatalf("skip mismatch at TU %d current=%v literal=%v", tuIndex, currentSkip, literalSkip)
		}
		cur0, cur1, cur2, cur3 = currentDec.DebugState()
		lit0, lit1, lit2, lit3 = literalDec.DebugState()
		if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
			t.Fatalf("decoder state mismatch after skip bit TU %d current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d)", tuIndex, cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3)
		}
		if !reflect.DeepEqual(currentCoef, literalCoef) {
			t.Fatalf("coef cdf mismatch after skip bit at TU %d", tuIndex)
		}

		if currentSkip {
			if err := ApplyLumaResidualContext(&currentAbove, &currentLeft, tu, tx, 0x40); err != nil {
				t.Fatalf("ApplyLumaResidualContext current skip TU %d: %v", tuIndex, err)
			}
			if err := ApplyLumaResidualContext(&literalAbove, &literalLeft, tu, tx, 0x40); err != nil {
				t.Fatalf("ApplyLumaResidualContext literal skip TU %d: %v", tuIndex, err)
			}
			if currentAbove != literalAbove || currentLeft != literalLeft {
				t.Fatalf("block context mismatch after skip TU %d", tuIndex)
			}
			continue
		}

		currentTxType, err := ReadIntraTxType(&parsed.Header, currentMode, state.syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: currentDec})
		if err != nil {
			t.Fatalf("ReadIntraTxType current TU %d: %v", tuIndex, err)
		}
		literalTxType, err := ReadIntraTxType(&parsed.Header, literalMode, state.syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: literalDec})
		if err != nil {
			t.Fatalf("ReadIntraTxType literal TU %d: %v", tuIndex, err)
		}
		if currentTxType != literalTxType {
			t.Fatalf("tx type mismatch at TU %d current=%d literal=%d", tuIndex, currentTxType, literalTxType)
		}

		currentCoeffs, currentResCtx, currentSkipNoSkip, err := DecodeLumaCoefficientsNoSkip(state.syntax.Size, tu, tx, currentTxType, state.syntax.Segment, currentCoef, state.dq, &currentAbove, &currentLeft, msacCoefficientDecoder{ctx: currentDec})
		if err != nil {
			t.Fatalf("DecodeLumaCoefficientsNoSkip current TU %d: %v", tuIndex, err)
		}
		literalCoeffs, literalResCtx, literalSkipNoSkip, literalTrace, err := decodeLumaCoefficientsLiteralNoSkipWithProfile(state.syntax.Size, tu, tx, literalTxType, state.syntax.Segment, literalCoef, state.dq, &literalAbove, &literalLeft, msacCoefficientDecoder{ctx: literalDec}, literalCoeffCurrentProfile)
		if err != nil {
			t.Fatalf("decodeLumaCoefficientsLiteralNoSkip literal TU %d: %v", tuIndex, err)
		}
		if currentSkipNoSkip != literalSkipNoSkip {
			t.Fatalf("no-skip decode skip mismatch at TU %d current=%v literal=%v trace=%v", tuIndex, currentSkipNoSkip, literalSkipNoSkip, literalTrace)
		}
		if currentResCtx != literalResCtx {
			t.Fatalf("res ctx mismatch at TU %d current=%#x literal=%#x trace=%v", tuIndex, currentResCtx, literalResCtx, literalTrace)
		}
		if !reflect.DeepEqual(currentCoeffs, literalCoeffs) {
			t.Fatalf("coeff mismatch at TU %d current=%v literal=%v trace=%v", tuIndex, summarizeNonZeroCoeffs(currentCoeffs), summarizeNonZeroCoeffs(literalCoeffs), literalTrace)
		}
		cur0, cur1, cur2, cur3 = currentDec.DebugState()
		lit0, lit1, lit2, lit3 = literalDec.DebugState()
		if cur0 != lit0 || cur1 != lit1 || cur2 != lit2 || cur3 != lit3 {
			t.Fatalf("decoder state mismatch after coeff decode TU %d current=(%d,%d,%d,%d) literal=(%d,%d,%d,%d) trace=%v", tuIndex, cur0, cur1, cur2, cur3, lit0, lit1, lit2, lit3, literalTrace)
		}
		if !reflect.DeepEqual(currentMode, literalMode) {
			t.Fatalf("mode cdf mismatch after tx type TU %d", tuIndex)
		}
		if !reflect.DeepEqual(currentCoef, literalCoef) {
			t.Fatalf("coef cdf mismatch after coeff decode TU %d trace=%v", tuIndex, literalTrace)
		}
		if err := ApplyLumaResidualContext(&currentAbove, &currentLeft, tu, tx, currentResCtx); err != nil {
			t.Fatalf("ApplyLumaResidualContext current TU %d: %v", tuIndex, err)
		}
		if err := ApplyLumaResidualContext(&literalAbove, &literalLeft, tu, tx, literalResCtx); err != nil {
			t.Fatalf("ApplyLumaResidualContext literal TU %d: %v", tuIndex, err)
		}
		if currentAbove != literalAbove || currentLeft != literalLeft {
			t.Fatalf("block context mismatch after TU %d trace=%v", tuIndex, literalTrace)
		}
	}
}

func TestDebugGeneratedMain10Call3LumaTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_LUMA_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-trace-main10-call3-luma.mp4",
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

	modeCDF := state.mode.Clone()
	coefCDF := state.coef.Clone()
	if coefCDF == nil {
		coefCDF = NewDefaultCoefCDF(uint8(state.qidx))
	}
	above := state.above
	left := state.left
	traceDec := state.dec.Clone()

	units, err := ReadIntraTransformUnitsInto(nil, &parsed.Header, modeCDF, &above, &left, state.syntax.Geometry, state.syntax.TX, state.syntax.Segment, msacEntropyDecoder{ctx: traceDec})
	if err != nil {
		t.Fatalf("ReadIntraTransformUnitsInto: %v", err)
	}
	t.Logf("call3 syntax geom=(%d,%d)-(%d,%d) ymode=%s tx=%s units=%d",
		state.syntax.Geometry.Start4X*4, state.syntax.Geometry.Start4Y*4, state.syntax.Geometry.End4X*4, state.syntax.Geometry.End4Y*4,
		intraModeName(state.syntax.YMode), txName(state.syntax.TX), len(units))
	for tuIndex, unit := range units {
		tu := unit.Global
		tx := unit.TX
		tuHaveTop := state.haveTop || tu.Start4Y > state.syntax.Geometry.Start4Y
		tuHaveLeft := state.haveLeft || tu.Start4X > state.syntax.Geometry.Start4X
		txInfo := TxfmInfoFor(tx)
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		skipRow := coeffCDFRow(int(txInfo.Ctx), "skip", len(coefCDF.Skip)-1)
		skip := msacCoefficientDecoder{ctx: traceDec}.DecodeBoolAdapt(coefCDF.Skip[skipRow][skipCtx][:]) != 0
		if skip {
			cur0, cur1, cur2, cur3 := traceDec.DebugState()
			t.Logf("tu=%d geom=(%d,%d)-(%d,%d) skip=true skipRow=%d skipCtx=%d state=(%d,%d,%d,%d)",
				tuIndex, tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4, skipRow, skipCtx, cur0, cur1, cur2, cur3)
			if err := ApplyLumaResidualContext(&above, &left, tu, tx, 0x40); err != nil {
				t.Fatalf("ApplyLumaResidualContext skip TU %d: %v", tuIndex, err)
			}
			continue
		}

		yTxType, err := ReadIntraTxType(&parsed.Header, modeCDF, state.syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: traceDec})
		if err != nil {
			t.Fatalf("ReadIntraTxType TU %d: %v", tuIndex, err)
		}
		coeffs, resCtx, skipNoSkip, trace, err := decodeLumaCoefficientsLiteralNoSkipWithProfile(state.syntax.Size, tu, tx, yTxType, state.syntax.Segment, coefCDF, state.dq, &above, &left, msacCoefficientDecoder{ctx: traceDec}, literalCoeffCurrentProfile)
		if err != nil {
			t.Fatalf("decodeLumaCoefficientsLiteralNoSkip TU %d: %v", tuIndex, err)
		}
		cur0, cur1, cur2, cur3 := traceDec.DebugState()
		t.Logf("tu=%d geom=(%d,%d)-(%d,%d) skip=false skipRow=%d skipCtx=%d txType=%d noSkipSkip=%v resCtx=%#x coeffs=%v state=(%d,%d,%d,%d) trace=%v",
			tuIndex, tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4, skipRow, skipCtx, yTxType, skipNoSkip, resCtx,
			summarizeNonZeroCoeffs(coeffs), cur0, cur1, cur2, cur3, trace)
		if err := ApplyLumaResidualContext(&above, &left, tu, tx, resCtx); err != nil {
			t.Fatalf("ApplyLumaResidualContext TU %d: %v", tuIndex, err)
		}
	}
}

func TestDebugGeneratedMain10Call3ChromaTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_CHROMA_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-trace-main10-call3-chroma.mp4",
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
	coefCDF := state.coef.Clone()
	if coefCDF == nil {
		coefCDF = NewDefaultCoefCDF(uint8(state.qidx))
	}
	above := state.above
	left := state.left
	traceDec := state.dec.Clone()
	if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, state.dq, &above, &left, state.syntax, debugMSACCombinedDecoder{ctx: traceDec}); err != nil {
		t.Fatalf("debugConsumeFirstBlockLuma: %v", err)
	}

	layout := seq.ColorConfig.ChromaSubsampling()
	uvTxType := IntraUVTxType(&parsed.Header, state.syntax.Segment, state.syntax.UVTX, state.syntax.UVMode)
	units := buildChromaTransformUnits(layout, state.syntax.Geometry, state.syntax.UVTX)
	t.Logf("call3 syntax geom=(%d,%d)-(%d,%d) uvmode=%s uvtx=%s uvtxtype=%d units=%d",
		state.syntax.Geometry.Start4X*4, state.syntax.Geometry.Start4Y*4, state.syntax.Geometry.End4X*4, state.syntax.Geometry.End4Y*4,
		intraModeName(state.syntax.UVMode), txName(state.syntax.UVTX), uvTxType, len(units))
	for plane := 1; plane <= 2; plane++ {
		for tuIndex, tu := range units {
			coeffs, resCtx, skip, trace, err := decodeChromaCoefficientsLiteralWithProfile(state.syntax.Size, tu, state.syntax.UVTX, uvTxType, state.syntax.Segment, plane, layout, coefCDF, state.dq, &above, &left, msacCoefficientDecoder{ctx: traceDec}, literalCoeffCurrentProfile)
			if err != nil {
				t.Fatalf("decodeChromaCoefficientsLiteral plane=%d tu=%d: %v", plane, tuIndex, err)
			}
			cur0, cur1, cur2, cur3 := traceDec.DebugState()
			t.Logf("plane=%d tu=%d geom=(%d,%d)-(%d,%d) skip=%v resCtx=%#x coeffs=%v state=(%d,%d,%d,%d) trace=%v",
				plane, tuIndex,
				tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4,
				skip, resCtx, summarizeNonZeroCoeffs(coeffs), cur0, cur1, cur2, cur3, trace)
			if err := ApplyChromaResidualContext(&above, &left, plane, tu, state.syntax.UVTX, resCtx); err != nil {
				t.Fatalf("ApplyChromaResidualContext plane=%d tu=%d: %v", plane, tuIndex, err)
			}
		}
	}
}

func TestDebugGeneratedMain10PaletteTargetDav1dProfileBlockMSE(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_PALETTE_BLOCK_MSE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "literal-compare-main10-palette-mse.mp4",
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
		t.Fatal("expected at least one non-skipped TU in palette target block")
	}
	current, _, _, txType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffCurrentProfile)
	dav1d, _, _, dav1dTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffDav1dProfile)
	if txType != dav1dTxType {
		t.Fatalf("tx type mismatch: current=%d dav1d=%d", txType, dav1dTxType)
	}

	lossless := parsed.Header.Segmentation.Lossless[state.syntax.Segment]
	currentBlock := reconstructPaletteLiteralBlock16(t, state.syntax, txType, current.coeffs, ref.BitDepth, lossless)
	dav1dBlock := reconstructPaletteLiteralBlock16(t, state.syntax, dav1dTxType, dav1d.coeffs, ref.BitDepth, lossless)
	blockX := state.syntax.Geometry.Start4X * 4
	blockY := state.syntax.Geometry.Start4Y * 4
	blockW := state.syntax.Geometry.Width4() * 4
	blockH := state.syntax.Geometry.Height4() * 4
	currentMSE := compareBlockToRefY16(currentBlock, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	dav1dMSE := compareBlockToRefY16(dav1dBlock, blockW, ref.Y16, ref.Width, blockX, blockY, blockW, blockH)
	t.Logf("current MSE=%.2f dav1d MSE=%.2f current=%v dav1d=%v ref=%v",
		currentMSE,
		dav1dMSE,
		debugBlockRows16(currentBlock, blockW, 0, 0, minInt(4, blockW), minInt(4, blockH)),
		debugBlockRows16(dav1dBlock, blockW, 0, 0, minInt(4, blockW), minInt(4, blockH)),
		debugBlockRows16(ref.Y16, ref.Width, blockX, blockY, minInt(4, blockW), minInt(4, blockH)),
	)
}

func reconstructPaletteLiteralBlock16(t *testing.T, syntax IntraBlockSyntax, txType TxType, coeffs []int32, bitDepth uint8, lossless bool) []uint16 {
	t.Helper()

	blockW := syntax.Geometry.Width4() * 4
	blockH := syntax.Geometry.Height4() * 4
	plane := make([]uint16, blockW*blockH)
	if err := ReconstructPaletteLumaCoefficients16WithTxType(plane, blockW, blockW, blockH, 0, 0, syntax.TX, txType, coeffs, bitDepth, lossless, syntax, 0, 0); err != nil {
		t.Fatalf("ReconstructPaletteLumaCoefficients16WithTxType: %v", err)
	}
	return plane
}

func compareBlockToRefY16(ours []uint16, ourStride int, ref []uint16, refStride, x, y, width, height int) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	mse := 0.0
	count := 0
	for row := 0; row < height; row++ {
		ourRow := ours[row*ourStride:]
		refRow := ref[(y+row)*refStride+x:]
		for col := 0; col < width; col++ {
			d := int(ourRow[col]) - int(refRow[col])
			mse += float64(d * d)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return mse / float64(count)
}

func firstGeneratedMain8NonSkippedTUInput(t *testing.T, path string) (debugTargetLeafAdvanceState, preparedTUCoefficientInput, bool) {
	t.Helper()

	for y := 0; y < 90; y += 16 {
		for x := 0; x < 160; x += 16 {
			dec, err := OpenMP4(path)
			if err != nil {
				t.Fatalf("OpenMP4: %v", err)
			}
			parsed, state := generatedKeyframeLeafState(t, dec, x, y, x, y)
			dec.Close()

			prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
			if err != nil {
				t.Fatalf("cloneNthNonSkippedTUCoefficientInput target=(%d,%d): %v", x, y, err)
			}
			if ok {
				return state, prepared, true
			}
		}
	}
	return debugTargetLeafAdvanceState{}, preparedTUCoefficientInput{}, false
}

func firstGeneratedKeyframeLeafState(t *testing.T, dec *Decoder) (*ParsedFrame, debugTargetLeafAdvanceState) {
	t.Helper()
	return generatedKeyframeLeafState(t, dec, 0, 0, 32, 0)
}

func generatedKeyframeLeafState(t *testing.T, dec *Decoder, targetX, targetY, nextX, nextY int) (*ParsedFrame, debugTargetLeafAdvanceState) {
	return generatedKeyframeLeafStateWithReader(t, dec, targetX, targetY, nextX, nextY, nil)
}

func generatedKeyframeLeafStateWithReader(t *testing.T, dec *Decoder, targetX, targetY, nextX, nextY int, readSyntax debugIntraSyntaxReader) (*ParsedFrame, debugTargetLeafAdvanceState) {
	t.Helper()

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

	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	var (
		targetTile TileState
		targetSB   SuperBlockGeometry
		foundSB    bool
	)
	for _, tile := range states {
		for _, sb := range tile.Geometry.SuperBlocks {
			if targetX >= sb.Start4X*4 && targetX < sb.End4X*4 && targetY >= sb.Start4Y*4 && targetY < sb.End4Y*4 {
				targetTile = tile
				targetSB = sb
				foundSB = true
				break
			}
		}
		if foundSB {
			break
		}
	}
	if !foundSB {
		t.Fatalf("no superblock covers target (%d,%d)", targetX, targetY)
	}

	syntaxState, err := NewTileSyntaxState(dec.header, &parsed.Header, targetTile, 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}
	if err := debugConsumeRestorationThroughSuperBlock(dec.header, &parsed.Header, syntaxState, targetSB); err != nil {
		t.Fatalf("debugConsumeRestorationThroughSuperBlock: %v", err)
	}
	row, err := syntaxState.NewRowState(targetSB.SBY)
	if err != nil {
		t.Fatalf("NewRowState: %v", err)
	}

	sb := targetSB
	foundRowSB := false
	for _, rowSB := range row.SuperBlocks {
		if rowSB.Index == targetSB.Index {
			sb = rowSB
			foundRowSB = true
			break
		}
	}
	if !foundRowSB {
		t.Fatalf("row state missing target superblock %d", targetSB.Index)
	}

	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	for _, priorSB := range row.SuperBlocks {
		if priorSB.Index == sb.Index {
			break
		}
		if err := debugConsumeBlockCDEFBeforeWithReader(dec.header, &parsed.Header, row, priorSB, BlockGeometry{
			Level:   RootBlockLevel(dec.header),
			Size:    RootBlockSize(dec.header),
			Start4X: priorSB.Start4X,
			Start4Y: priorSB.Start4Y,
			End4X:   priorSB.End4X,
			End4Y:   priorSB.End4Y,
		}, segState, nil); err != nil {
			t.Fatalf("debugConsumeBlockCDEFBeforeWithReader(%d): %v", priorSB.Index, err)
		}
	}
	root := BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	var state debugTargetLeafAdvanceState
	if readSyntax != nil {
		state, err = debugAdvanceToTargetLeafCDEFBeforeWithReader(dec.header, &parsed.Header, row, sb, BlockGeometry{
			Level:   root.Level,
			Size:    root.Size,
			Start4X: root.Start4X,
			Start4Y: root.Start4Y,
			End4X:   root.End4X,
			End4Y:   root.End4Y,
		}, segState, targetX, targetY, nextX, nextY, readSyntax)
	} else {
		state, err = debugAdvanceToTargetLeafCDEFBefore(dec.header, &parsed.Header, row, sb, root, segState, targetX, targetY, nextX, nextY)
	}
	if err != nil {
		t.Fatalf("debugAdvanceToTargetLeafCDEFBefore: %v", err)
	}
	return parsed, state
}

func decodeGeneratedFirstTUCoefficientsCurrent(t *testing.T, parsed *ParsedFrame, state debugTargetLeafAdvanceState) (literalCoeffResult, *CoefCDF, *msac.Context, TxType) {
	t.Helper()

	coefCDF, decClone, txType, tu := cloneFirstTUDecodeInputs(t, parsed, state)
	return decodePreparedTUCoefficientsCurrent(t, state, preparedTUCoefficientInput{
		coefCDF: coefCDF,
		dec:     decClone,
		txType:  txType,
		tu:      tu,
		above:   state.above,
		left:    state.left,
	})
}

func decodePreparedTUCoefficientsCurrent(t *testing.T, state debugTargetLeafAdvanceState, prepared preparedTUCoefficientInput) (literalCoeffResult, *CoefCDF, *msac.Context, TxType) {
	t.Helper()

	coefCDF := prepared.coefCDF
	decClone := prepared.dec
	txType := prepared.txType
	tu := prepared.tu
	aboveClone := prepared.above
	leftClone := prepared.left
	coeffs, resCtx, skip, err := DecodeLumaCoefficientsNoSkip(state.syntax.Size, tu, state.syntax.TX, txType, state.syntax.Segment, coefCDF, state.dq, &aboveClone, &leftClone, msacCoefficientDecoder{ctx: decClone})
	if err != nil {
		t.Fatalf("DecodeLumaCoefficientsNoSkip: %v", err)
	}
	return literalCoeffResult{
		coeffs: coeffs,
		resCtx: resCtx,
		skip:   skip,
	}, coefCDF, decClone, txType
}

func decodeGeneratedFirstTUCoefficientsLiteral(t *testing.T, parsed *ParsedFrame, state debugTargetLeafAdvanceState) (literalCoeffResult, *CoefCDF, *msac.Context, TxType) {
	return decodeGeneratedFirstTUCoefficientsLiteralWithProfile(t, parsed, state, literalCoeffCurrentProfile)
}

func decodeGeneratedFirstTUCoefficientsLiteralWithProfile(t *testing.T, parsed *ParsedFrame, state debugTargetLeafAdvanceState, profile literalCoeffProfile) (literalCoeffResult, *CoefCDF, *msac.Context, TxType) {
	t.Helper()

	coefCDF, decClone, txType, tu := cloneFirstTUDecodeInputs(t, parsed, state)
	return decodePreparedTUCoefficientsLiteralWithProfile(t, state, preparedTUCoefficientInput{
		coefCDF: coefCDF,
		dec:     decClone,
		txType:  txType,
		tu:      tu,
		above:   state.above,
		left:    state.left,
	}, profile)
}

func tryDecodeGeneratedFirstTUCoefficientsLiteralWithProfile(t *testing.T, parsed *ParsedFrame, state debugTargetLeafAdvanceState, profile literalCoeffProfile) (literalCoeffResult, *CoefCDF, *msac.Context, TxType, bool) {
	t.Helper()

	coefCDF, decClone, txType, tu, skipped, err := cloneFirstTUDecodeInputsMaybeSkip(parsed, state)
	if err != nil {
		t.Fatalf("cloneFirstTUDecodeInputsMaybeSkip: %v", err)
	}
	if skipped {
		return literalCoeffResult{skip: true}, coefCDF, decClone, txType, false
	}
	result, outCDF, outDec, outTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, preparedTUCoefficientInput{
		coefCDF: coefCDF,
		dec:     decClone,
		txType:  txType,
		tu:      tu,
		above:   state.above,
		left:    state.left,
	}, profile)
	return result, outCDF, outDec, outTxType, true
}

func decodePreparedTUCoefficientsLiteralWithProfile(t *testing.T, state debugTargetLeafAdvanceState, prepared preparedTUCoefficientInput, profile literalCoeffProfile) (literalCoeffResult, *CoefCDF, *msac.Context, TxType) {
	t.Helper()

	coefCDF := prepared.coefCDF
	decClone := prepared.dec
	txType := prepared.txType
	tu := prepared.tu
	aboveClone := prepared.above
	leftClone := prepared.left
	coeffs, resCtx, skip, trace, err := decodeLumaCoefficientsLiteralNoSkipWithProfile(state.syntax.Size, tu, state.syntax.TX, txType, state.syntax.Segment, coefCDF, state.dq, &aboveClone, &leftClone, msacCoefficientDecoder{ctx: decClone}, profile)
	if err != nil {
		t.Fatalf("decodeLumaCoefficientsLiteralNoSkip(%s): %v", profile.name, err)
	}
	return literalCoeffResult{
		coeffs: coeffs,
		resCtx: resCtx,
		skip:   skip,
		trace:  trace,
	}, coefCDF, decClone, txType
}

func cloneNthNonSkippedTUCoefficientInput(parsed *ParsedFrame, state debugTargetLeafAdvanceState, targetNonSkipped int) (preparedTUCoefficientInput, bool, error) {
	if parsed == nil {
		return preparedTUCoefficientInput{}, false, fmt.Errorf("decoder: parsed frame required")
	}
	if targetNonSkipped < 0 {
		return preparedTUCoefficientInput{}, false, fmt.Errorf("decoder: invalid non-skipped TU index %d", targetNonSkipped)
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
	aboveClone := state.above
	leftClone := state.left
	tx := state.syntax.TX
	txInfo := TxfmInfoFor(tx)
	units := buildPlaneTransformUnits(state.syntax.Geometry, tx)

	nonSkippedSeen := 0
	for tuIndex, tu := range units {
		offX4 := tu.Start4X - state.syntax.Geometry.Start4X
		offY4 := tu.Start4Y - state.syntax.Geometry.Start4Y
		tuHaveTop := state.syntax.Geometry.Start4Y > 0 || offY4 > 0
		tuHaveLeft := state.syntax.Geometry.Start4X > 0 || offX4 > 0
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, aboveClone.LCoef[bx4:], leftClone.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		if decClone.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
			if err := ApplyLumaResidualContext(&aboveClone, &leftClone, tu, tx, 0x40); err != nil {
				return preparedTUCoefficientInput{}, false, err
			}
			continue
		}
		txType, err := ReadIntraTxType(&parsed.Header, modeCDF, state.syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: decClone})
		if err != nil {
			return preparedTUCoefficientInput{}, false, fmt.Errorf("ReadIntraTxType(TU %d): %w", tuIndex, err)
		}
		if nonSkippedSeen == targetNonSkipped {
			return preparedTUCoefficientInput{
				coefCDF:         coefCDF,
				dec:             decClone,
				txType:          txType,
				tu:              tu,
				above:           aboveClone,
				left:            leftClone,
				tuIndex:         tuIndex,
				nonSkippedIndex: nonSkippedSeen,
			}, true, nil
		}
		coeffs := make([]int32, coeffBufferLen(tx))
		resCtx, skip, err := DecodeLumaCoefficientsNoSkipInto(coeffs, state.syntax.Size, tu, tx, txType, state.syntax.Segment, coefCDF, state.dq, &aboveClone, &leftClone, msacCoefficientDecoder{ctx: decClone})
		if err != nil {
			return preparedTUCoefficientInput{}, false, fmt.Errorf("DecodeLumaCoefficientsNoSkipInto(TU %d): %w", tuIndex, err)
		}
		if skip {
			return preparedTUCoefficientInput{}, false, fmt.Errorf("decoder: unexpected skip after explicit skip bit clear at TU %d", tuIndex)
		}
		if err := ApplyLumaResidualContext(&aboveClone, &leftClone, tu, tx, resCtx); err != nil {
			return preparedTUCoefficientInput{}, false, err
		}
		nonSkippedSeen++
	}

	return preparedTUCoefficientInput{}, false, nil
}

func cloneFirstTUDecodeInputs(t *testing.T, parsed *ParsedFrame, state debugTargetLeafAdvanceState) (*CoefCDF, *msac.Context, TxType, BlockGeometry) {
	t.Helper()

	coefCDF, decClone, txType, tu, skipped, err := cloneFirstTUDecodeInputsMaybeSkip(parsed, state)
	if err != nil {
		t.Fatalf("cloneFirstTUDecodeInputsMaybeSkip: %v", err)
	}
	if skipped {
		t.Fatal("unexpected skip in first TU replay")
	}
	return coefCDF, decClone, txType, tu
}

func cloneFirstTUDecodeInputsMaybeSkip(parsed *ParsedFrame, state debugTargetLeafAdvanceState) (*CoefCDF, *msac.Context, TxType, BlockGeometry, bool, error) {
	txInfo := TxfmInfoFor(state.syntax.TX)
	bx4 := state.syntax.Geometry.Start4X & 31
	by4 := state.syntax.Geometry.Start4Y & 31
	tuHaveTop := state.syntax.Geometry.Start4Y > 0
	tuHaveLeft := state.syntax.Geometry.Start4X > 0
	skipCtx := GetSkipCtxWithNeighbors(txInfo, state.syntax.Size, state.above.LCoef[bx4:], state.left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)

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
		return coefCDF, decClone, 0, BlockGeometry{}, true, nil
	}
	txType, err := ReadIntraTxType(&parsed.Header, modeCDF, state.syntax, state.syntax.TX, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: decClone})
	if err != nil {
		return nil, nil, 0, BlockGeometry{}, false, fmt.Errorf("ReadIntraTxType: %w", err)
	}
	return coefCDF, decClone, txType, childTXGeometry(state.syntax.Geometry, state.syntax.TX, 0, 0), false, nil
}

func decodeLumaCoefficientsLiteralNoSkip(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) ([]int32, uint8, bool, []literalCoeffTraceStep, error) {
	return decodeLumaCoefficientsLiteralNoSkipWithProfile(bs, g, tx, txType, segID, coef, dq, above, left, dec, literalCoeffCurrentProfile)
}

func decodeLumaCoefficientsLiteralNoSkipWithProfile(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, profile literalCoeffProfile) ([]int32, uint8, bool, []literalCoeffTraceStep, error) {
	return decodeCoefficientsLiteralWithProfile(bs, g, tx, txType, segID, 0, false, av1.Chroma420, coef, dq, above.LCoef[g.Start4X&31:], left.LCoef[g.Start4Y&31:], dec, false, profile)
}

func decodeChromaCoefficientsLiteralWithProfile(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, profile literalCoeffProfile) ([]int32, uint8, bool, []literalCoeffTraceStep, error) {
	if plane < 1 || plane > 2 {
		return nil, 0, false, nil, fmt.Errorf("decoder: invalid chroma plane %d", plane)
	}
	bx4, by4 := chromaCoeffContextOffsets(layout, g)
	return decodeCoefficientsLiteralWithProfile(bs, g, tx, txType, segID, plane, true, layout, coef, dq, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], dec, true, profile)
}

func decodeCoefficientsLiteralWithProfile(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, dqPlane int, chroma bool, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, aboveCoef, leftCoef []uint8, dec coefficientEntropyDecoder, readSkip bool, profile literalCoeffProfile) ([]int32, uint8, bool, []literalCoeffTraceStep, error) {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	scan := ScanOrder(tx)
	scanWidth, scanHeight := ScanCoeffArea(tx)
	orderCount := scanWidth * scanHeight
	txClass := generatedTXTypeClass[txType]
	coeffs := make([]int32, coeffBufferLen(tx))
	if len(coeffs) < width*height {
		return nil, 0, false, nil, fmt.Errorf("coefficient buffer too small for %dx%d tx", width, height)
	}
	coeffs = coeffs[:width*height]

	trace := make([]literalCoeffTraceStep, 0, orderCount+6)
	if readSkip {
		skipCtx := GetSkipCtx(info, bs, aboveCoef, leftCoef, chroma, layout)
		skipRow := coeffCDFRow(int(info.Ctx), "skip", len(coef.Skip)-1)
		skipTok := dec.DecodeBoolAdapt(coef.Skip[skipRow][skipCtx][:])
		trace = append(trace, literalCoeffTraceStep{label: "skip", order: -1, rc: -1, ctx: skipCtx, tok: skipTok})
		if skipTok != 0 {
			return coeffs, 0x40, true, trace, nil
		}
	}

	eob, eobTrace, err := decodeEOBCoeffCountWithProfile(info, coef, dec, chroma, txClass != TxClass2D, profile)
	if err != nil {
		return nil, 0, false, nil, err
	}
	if eob >= orderCount {
		return nil, 0, false, nil, fmt.Errorf("decoder: eob=%d exceeds coded scan size %d", eob, orderCount)
	}

	trace = append(trace, eobTrace...)
	chromaIdx := boolToInt(chroma)
	loRow := coeffCDFRowForCoef(coef, int(info.Ctx), "base", len(coef.BaseTok)-1)
	eobRow := coeffCDFRowForCoef(coef, int(info.Ctx), "eob-base", len(coef.EOBBaseTok)-1)
	brRow := coeffCDFRowForCoef(coef, minInt(int(info.Ctx), 3), "br", len(coef.BRTok)-1)
	loCDF := &coef.BaseTok[loRow][chromaIdx]
	eobCDF := &coef.EOBBaseTok[eobRow][chromaIdx]
	hiCDF := &coef.BRTok[brRow][chromaIdx]
	rawLevels := make([]uint8, coeffLevelsBufferSize(tx))
	levelsByRC := make([]uint8, orderCount)

	var dcTok uint32
	if eob != 0 {
		eobRC := literalCoeffRCForOrder(txClass, tx, eob, scan)
		eobCtx := profile.eobBaseCtx(info, eob)
		eobTok := dec.DecodeSymbolAdapt((*eobCDF)[eobCtx][:], 2)
		trace = append(trace, literalCoeffTraceStep{label: "eob-base", order: eob, rc: eobRC, ctx: eobCtx, tok: eobTok})
		tok := eobTok + 1
		if eobTok == 2 {
			hiCtx := literalCoeffHiCtxForRC(txClass, tx, eobRC)
			tok = dec.DecodeHiTok((*hiCDF)[hiCtx][:])
			trace = append(trace, literalCoeffTraceStep{label: "eob-hi", order: eob, rc: eobRC, ctx: hiCtx, tok: tok})
		}
		levelsByRC[eobRC] = uint8(tok)
		rawLevels[coeffContextIndex(tx, eobRC)] = uint8(tok)

		for i := eob - 1; i > 0; i-- {
			rc := literalCoeffRCForOrder(txClass, tx, i, scan)
			ctx := GetLoCtx(rawLevels, txClass, tx, rc)
			tok = dec.DecodeSymbolAdapt((*loCDF)[ctx][:], 3)
			trace = append(trace, literalCoeffTraceStep{label: "base", order: i, rc: rc, ctx: ctx, tok: tok})
			if tok == 3 {
				hiCtx := GetBRCtx(rawLevels, txClass, tx, rc)
				tok = dec.DecodeHiTok((*hiCDF)[hiCtx][:])
				trace = append(trace, literalCoeffTraceStep{label: "br", order: i, rc: rc, ctx: hiCtx, tok: tok})
			}
			rawLevels[coeffContextIndex(tx, rc)] = uint8(tok)
			if tok != 0 {
				levelsByRC[rc] = uint8(tok)
			}
		}

		dcCtx := GetLoCtx(rawLevels, txClass, tx, 0)
		dcTok = dec.DecodeSymbolAdapt((*loCDF)[dcCtx][:], 3)
		trace = append(trace, literalCoeffTraceStep{label: "dc-base", order: 0, rc: 0, ctx: dcCtx, tok: dcTok})
		if dcTok == 3 {
			hiCtx := GetBRCtx(rawLevels, txClass, tx, 0)
			dcTok = dec.DecodeHiTok((*hiCDF)[hiCtx][:])
			trace = append(trace, literalCoeffTraceStep{label: "dc-br", order: 0, rc: 0, ctx: hiCtx, tok: dcTok})
		}
	} else {
		eobTok := dec.DecodeSymbolAdapt((*eobCDF)[0][:], 2)
		trace = append(trace, literalCoeffTraceStep{label: "eob-base", order: 0, rc: 0, ctx: 0, tok: eobTok})
		dcTok = eobTok + 1
		if eobTok == 2 {
			dcTok = dec.DecodeHiTok((*hiCDF)[0][:])
			trace = append(trace, literalCoeffTraceStep{label: "dc-br", order: 0, rc: 0, ctx: 0, tok: dcTok})
		}
	}

	dqShift := maxIntLocal(0, int(info.Ctx)-2)
	cfMax := (1 << 15) - 1
	culLevel := uint32(0)
	dcSignLevel := uint8(1 << 6)
	if dcTok != 0 {
		dcSignCtx := GetDCSignCtx(tx, aboveCoef, leftCoef)
		neg := dec.DecodeBoolAdapt(coef.DCSign[chromaIdx][dcSignCtx][:]) != 0
		trace = append(trace, literalCoeffTraceStep{label: "dc-sign", order: 0, rc: 0, ctx: dcSignCtx, tok: boolToUint32(neg)})
		tok := int(dcTok)
		if tok == 15 {
			tok = int(decodeGolomb(dec.DecodeBoolEqui)) + 15
		}
		dc := (int(dq[segID][dqPlane][0]) * tok) >> dqShift
		if neg {
			dc = minInt(dc, cfMax+1)
			dc = -dc
			dcSignLevel = 0
		} else {
			dc = minInt(dc, cfMax)
			dcSignLevel = 2 << 6
		}
		coeffs[0] = int32(dc)
		culLevel = uint32(tok)
	}

	if eob != 0 {
		acDQ := int(dq[segID][dqPlane][1])
		for i := 1; i <= eob; i++ {
			rc := literalCoeffRCForOrder(txClass, tx, i, scan)
			tok := int(levelsByRC[rc])
			if tok == 0 {
				continue
			}
			sign := dec.DecodeBoolEqui() != 0
			if tok == 15 {
				tok = int(decodeGolomb(dec.DecodeBoolEqui)) + 15
			}
			ac := (acDQ * tok) >> dqShift
			if sign {
				ac = minInt(ac, cfMax+1)
				ac = -ac
			} else {
				ac = minInt(ac, cfMax)
			}
			coeffIdx, err := CoeffIndexFromScan(tx, uint32(rc))
			if err != nil {
				return nil, 0, false, trace, err
			}
			coeffs[coeffIdx] = int32(ac)
			culLevel += uint32(tok)
		}
	}

	return coeffs, uint8(minInt(int(culLevel), 63)) | dcSignLevel, false, trace, nil
}

func decodeEOBCoeffCountWithProfile(info TxfmInfo, coef *CoefCDF, dec coefficientEntropyDecoder, chroma bool, is1D bool, profile literalCoeffProfile) (int, []literalCoeffTraceStep, error) {
	tx2dCtx := eobTx2DCtx(info)
	c := boolToInt(chroma)
	is1 := boolToInt(is1D)

	trace := make([]literalCoeffTraceStep, 0, 3)
	eobBin := 0
	switch tx2dCtx {
	case 0:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin16[c][is1][:], 4))
	case 1:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin32[c][is1][:], 5))
	case 2:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin64[c][is1][:], 6))
	case 3:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin128[c][is1][:], 7))
	case 4:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin256[c][is1][:], 8))
	case 5:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin512[c][:], 9))
	case 6:
		eobBin = int(dec.DecodeSymbolAdapt(coef.EOBBin1024[c][:], 10))
	default:
		return 0, nil, fmt.Errorf("decoder: unsupported tx2d context %d", tx2dCtx)
	}
	trace = append(trace, literalCoeffTraceStep{label: "eob-bin", order: -1, rc: -1, ctx: tx2dCtx, tok: uint32(eobBin)})
	if eobBin > 1 {
		bin := eobBin - 2
		hiCtx := profile.eobHiBitCtx(info)
		hiBit := dec.DecodeBoolAdapt(coef.EOBHiBit[hiCtx][c][bin][:])
		trace = append(trace, literalCoeffTraceStep{label: "eob-hi-bit", order: -1, rc: -1, ctx: hiCtx, tok: hiBit})
		equi := decodeEquiBits(dec, bin)
		trace = append(trace, literalCoeffTraceStep{label: "eob-equi", order: -1, rc: -1, ctx: bin, tok: equi})
		return ((int(hiBit) | 2) << bin) | int(equi), trace, nil
	}
	return eobBin, trace, nil
}

func literalCoeffRCForOrder(txClass TxClass, tx TxfmSize, order int, scan []uint16) int {
	_, _, _, rc, _ := coeffDecodeLayout(txClass, tx, order, scan)
	return int(rc)
}

func literalCoeffHiCtxForRC(txClass TxClass, tx TxfmSize, rc int) int {
	if GetBRCtx(make([]uint8, coeffLevelsBufferSize(tx)), txClass, tx, rc) < 7 {
		return 0
	}
	x, y, _ := coeffContextXY(tx, rc)
	switch txClass {
	case TxClass2D:
		if x < 2 && y < 2 {
			return 7
		}
	case TxClassH:
		if x == 0 {
			return 7
		}
	case TxClassV:
		if y == 0 {
			return 7
		}
	default:
		return 0
	}
	if txClass == TxClass2D && x < 2 && y < 2 {
		return 7
	}
	return 14
}

func summarizeNonZeroCoeffs(coeffs []int32) []string {
	out := make([]string, 0, 8)
	for i, v := range coeffs {
		if v == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%d=%d", i, v))
		if len(out) == 8 {
			break
		}
	}
	if len(out) == 0 {
		return []string{"all-zero"}
	}
	return out
}

func boolToUint32(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}
