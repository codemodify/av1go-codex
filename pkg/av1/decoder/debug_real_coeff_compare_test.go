package decoder

import (
	"os"
	"reflect"
	"testing"
)

func TestDebugCompareRealFirstTUCoefficientsToLiteral(t *testing.T) {
	if os.Getenv("DEBUG_COMPARE_REAL_FIRST_TU_COEF") == "" {
		t.Skip("debug only")
	}
	path := os.Getenv("DEBUG_REAL_FIRST_TU_INPUT")
	if path == "" {
		t.Fatal("DEBUG_REAL_FIRST_TU_INPUT is required")
	}

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	parsed, state := generatedKeyframeLeafState(t, dec, 0, 0, 16, 0)
	prepared, ok, err := cloneNthNonSkippedTUCoefficientInput(parsed, state, 0)
	if err != nil {
		t.Fatalf("cloneNthNonSkippedTUCoefficientInput: %v", err)
	}
	if !ok {
		t.Fatal("first target leaf has no non-skipped TU")
	}

	current, currentCDF, currentDec, txType := decodePreparedTUCoefficientsCurrent(t, state, prepared.Clone())
	literal, literalCDF, literalDec, literalTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffCurrentProfile)
	dav1d, dav1dCDF, dav1dDec, dav1dTxType := decodePreparedTUCoefficientsLiteralWithProfile(t, state, prepared.Clone(), literalCoeffDav1dProfile)

	t.Logf("state qidx=%d syntax=(%d,%d)-(%d,%d) size=%s tx=%s", state.qidx,
		state.syntax.Geometry.Start4X*4, state.syntax.Geometry.Start4Y*4,
		state.syntax.Geometry.End4X*4, state.syntax.Geometry.End4Y*4,
		blockName(state.syntax.Size), txName(state.syntax.TX))
	t.Logf("current txType=%d skip=%v resCtx=%#x coeffs=%v dec=%v", txType, current.skip, current.resCtx, summarizeNonZeroCoeffs(current.coeffs), debugGeneratedMain10MSACState(currentDec))
	t.Logf("literal txType=%d skip=%v resCtx=%#x coeffs=%v dec=%v trace=%v cdfEqual=%v", literalTxType, literal.skip, literal.resCtx, summarizeNonZeroCoeffs(literal.coeffs), debugGeneratedMain10MSACState(literalDec), literal.trace, reflect.DeepEqual(currentCDF, literalCDF))
	t.Logf("dav1d-profile txType=%d skip=%v resCtx=%#x coeffs=%v dec=%v trace=%v cdfEqual=%v", dav1dTxType, dav1d.skip, dav1d.resCtx, summarizeNonZeroCoeffs(dav1d.coeffs), debugGeneratedMain10MSACState(dav1dDec), dav1d.trace, reflect.DeepEqual(currentCDF, dav1dCDF))
}
