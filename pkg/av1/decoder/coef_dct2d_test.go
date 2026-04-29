package decoder

import "testing"

type orderedEntropyStep struct {
	kind  string
	value uint32
}

type recordingCoefficientEntropy struct {
	t          *testing.T
	adaptCDF0  []uint16
	symbolCDF0 []uint16
	symbolVals []uint32
	adaptVals  []uint32
	equiVals   []uint32
}

func (r *recordingCoefficientEntropy) DecodeBoolEqui() uint32 {
	r.t.Helper()
	if len(r.equiVals) == 0 {
		return 0
	}
	v := r.equiVals[0]
	r.equiVals = r.equiVals[1:]
	return v
}

func (r *recordingCoefficientEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	r.t.Helper()
	if len(cdf) != 0 {
		r.adaptCDF0 = append(r.adaptCDF0, cdf[0])
	}
	if len(r.adaptVals) == 0 {
		return 0
	}
	v := r.adaptVals[0]
	r.adaptVals = r.adaptVals[1:]
	return v
}

func (r *recordingCoefficientEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	r.t.Helper()
	if len(cdf) != 0 {
		r.symbolCDF0 = append(r.symbolCDF0, cdf[0])
	}
	if len(r.symbolVals) == 0 {
		return 0
	}
	v := r.symbolVals[0]
	r.symbolVals = r.symbolVals[1:]
	return v
}

func (r *recordingCoefficientEntropy) DecodeHiTok(cdf []uint16) uint32 {
	r.t.Helper()
	return 0
}

type orderedCoefficientEntropy struct {
	t     *testing.T
	steps []orderedEntropyStep
}

func (o *orderedCoefficientEntropy) pop(kind string) uint32 {
	o.t.Helper()
	if len(o.steps) == 0 {
		o.t.Fatalf("unexpected %s call with no remaining scripted steps", kind)
	}
	step := o.steps[0]
	o.steps = o.steps[1:]
	if step.kind != kind {
		o.t.Fatalf("unexpected decoder call %s, want %s", kind, step.kind)
	}
	return step.value
}

func (o *orderedCoefficientEntropy) DecodeBoolEqui() uint32 {
	return o.pop("equi")
}

func (o *orderedCoefficientEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	return o.pop("adapt")
}

func (o *orderedCoefficientEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	return o.pop("symbol")
}

func (o *orderedCoefficientEntropy) DecodeHiTok(cdf []uint16) uint32 {
	return o.pop("hitok")
}

type mutatingCoefficientEntropy struct{}

func (m mutatingCoefficientEntropy) DecodeBoolEqui() uint32 { return 0 }

func (m mutatingCoefficientEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	if len(cdf) != 0 {
		cdf[0]++
	}
	return 0
}

func (m mutatingCoefficientEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	if len(cdf) != 0 {
		cdf[0]++
	}
	return 0
}

func (m mutatingCoefficientEntropy) DecodeHiTok(cdf []uint16) uint32 {
	if len(cdf) != 0 {
		cdf[0]++
	}
	return 0
}

type mutatingCountCoefficientEntropy struct{}

func (m mutatingCountCoefficientEntropy) DecodeBoolEqui() uint32 { return 0 }

func (m mutatingCountCoefficientEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	if len(cdf) != 0 {
		cdf[0]++
		cdf[len(cdf)-1]++
	}
	return 0
}

func (m mutatingCountCoefficientEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	if len(cdf) != 0 {
		cdf[0]++
		cdf[len(cdf)-1]++
	}
	return 0
}

func (m mutatingCountCoefficientEntropy) DecodeHiTok(cdf []uint16) uint32 {
	if len(cdf) != 0 {
		cdf[0]++
		cdf[len(cdf)-1]++
	}
	return 0
}

func TestCoeffDecodeSymbolAdaptCanDisableCDFUpdatesPerFamily(t *testing.T) {
	t.Setenv("DEBUG_COEF_DISABLE_CDF_UPDATE", "base")
	cdf := []uint16{10, 0}
	coeffDecodeSymbolAdapt(mutatingCoefficientEntropy{}, cdf, 1, "base")
	if got, want := cdf[0], uint16(10); got != want {
		t.Fatalf("base family cdf[0] = %d, want %d", got, want)
	}

	coeffDecodeSymbolAdapt(mutatingCoefficientEntropy{}, cdf, 1, "br")
	if got, want := cdf[0], uint16(11); got != want {
		t.Fatalf("other family cdf[0] = %d, want %d", got, want)
	}
}

func TestCoeffDecodeBoolAdaptSupportsGlobalCDFUpdateDisable(t *testing.T) {
	t.Setenv("DEBUG_COEF_DISABLE_CDF_UPDATE", "all")
	cdf := []uint16{10, 0}
	coeffDecodeBoolAdapt(mutatingCoefficientEntropy{}, cdf, "dc-sign")
	if got, want := cdf[0], uint16(10); got != want {
		t.Fatalf("global disable cdf[0] = %d, want %d", got, want)
	}
}

func TestCoeffDecodeSymbolAdaptCanFreezeCountPerFamily(t *testing.T) {
	t.Setenv("DEBUG_COEF_FREEZE_COUNT", "base")
	cdf := []uint16{10, 20, 3}
	coeffDecodeSymbolAdapt(mutatingCountCoefficientEntropy{}, cdf, 2, "base")
	if got, want := cdf[0], uint16(11); got != want {
		t.Fatalf("base threshold cdf[0] = %d, want %d", got, want)
	}
	if got, want := cdf[2], uint16(3); got != want {
		t.Fatalf("base count cdf[2] = %d, want %d", got, want)
	}

	coeffDecodeSymbolAdapt(mutatingCountCoefficientEntropy{}, cdf, 2, "br")
	if got, want := cdf[2], uint16(4); got != want {
		t.Fatalf("other family count cdf[2] = %d, want %d", got, want)
	}
}

func TestCoeffDecodeHiTokCanFreezeCountPerFamily(t *testing.T) {
	t.Setenv("DEBUG_COEF_FREEZE_COUNT", "br")
	cdf := []uint16{10, 20, 30, 4}
	coeffDecodeHiTok(mutatingCountCoefficientEntropy{}, cdf, "br")
	if got, want := cdf[0], uint16(11); got != want {
		t.Fatalf("br threshold cdf[0] = %d, want %d", got, want)
	}
	if got, want := cdf[3], uint16(4); got != want {
		t.Fatalf("br count cdf[3] = %d, want %d", got, want)
	}
}

func TestDecodeLumaCoefficientsDCT2DOneAC(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedCoefficientEntropy{
		adapt:  []uint32{0, 0},
		symbol: []uint32{1, 0, 0},
		equi:   []uint32{0},
	}

	coeffs, resCtx, skip, err := DecodeLumaCoefficientsDCT2D(Block4x4, BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}, TX4X4, 0, NewDefaultCoefCDF(0), DequantTables{{{0, 8}}}, &above, &left, dec)
	if err != nil {
		t.Fatalf("DecodeLumaCoefficientsDCT2D: %v", err)
	}
	if skip {
		t.Fatal("unexpected skip")
	}
	acIdx, err := CoeffIndexFromScan(TX4X4, uint32(ScanOrder(TX4X4)[1]))
	if err != nil {
		t.Fatalf("CoeffIndexFromScan: %v", err)
	}
	if coeffs[acIdx] != 8 {
		t.Fatalf("AC coefficient = %d, want 8", coeffs[acIdx])
	}
	if resCtx != 0x41 {
		t.Fatalf("resCtx = %d, want 0x41", resCtx)
	}
}

func TestDecodeLumaCoefficientsDCT2DDefersACGolombUntilSignPass(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	dec := &orderedCoefficientEntropy{
		t: t,
		steps: []orderedEntropyStep{
			{kind: "adapt", value: 0},
			{kind: "symbol", value: 1},
			{kind: "symbol", value: 2},
			{kind: "hitok", value: 15},
			{kind: "symbol", value: 0},
			{kind: "equi", value: 0},
			{kind: "equi", value: 0},
			{kind: "equi", value: 1},
			{kind: "equi", value: 0},
		},
	}

	coeffs, _, skip, err := DecodeLumaCoefficientsDCT2D(Block4x4, BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}, TX4X4, 0, NewDefaultCoefCDF(0), DequantTables{{{0, 8}}}, &above, &left, dec)
	if err != nil {
		t.Fatalf("DecodeLumaCoefficientsDCT2D: %v", err)
	}
	if skip {
		t.Fatal("unexpected skip")
	}
	acIdx, err := CoeffIndexFromScan(TX4X4, uint32(ScanOrder(TX4X4)[1]))
	if err != nil {
		t.Fatalf("CoeffIndexFromScan: %v", err)
	}
	if coeffs[acIdx] != 128 {
		t.Fatalf("AC coefficient = %d, want 128", coeffs[acIdx])
	}
	if len(dec.steps) != 0 {
		t.Fatalf("unused scripted steps remain: %d", len(dec.steps))
	}
}

func TestDecodeLumaCoefficientsDCT2DReadsACSignsInForwardScanOrder(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	dec := &orderedCoefficientEntropy{
		t: t,
		steps: []orderedEntropyStep{
			{kind: "adapt", value: 0},  // skip = false
			{kind: "symbol", value: 2}, // eob bin -> indexed eob=2 after hi bit
			{kind: "adapt", value: 0},  // eob hi bit
			{kind: "symbol", value: 0}, // eob coeff tok = 1 at scan[2]
			{kind: "symbol", value: 1}, // next AC tok = 1 at scan[1]
			{kind: "symbol", value: 0}, // dc tok = 0
			{kind: "equi", value: 0},   // sign for scan[1] coefficient
			{kind: "equi", value: 1},   // sign for scan[2] coefficient
		},
	}

	coeffs, _, skip, err := DecodeLumaCoefficientsDCT2D(Block4x4, BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}, TX4X4, 0, NewDefaultCoefCDF(0), DequantTables{{{0, 8}}}, &above, &left, dec)
	if err != nil {
		t.Fatalf("DecodeLumaCoefficientsDCT2D: %v", err)
	}
	if skip {
		t.Fatal("unexpected skip")
	}

	eobACIdx, err := CoeffIndexFromScan(TX4X4, uint32(ScanOrder(TX4X4)[2]))
	if err != nil {
		t.Fatalf("CoeffIndexFromScan eob AC: %v", err)
	}
	firstACIdx, err := CoeffIndexFromScan(TX4X4, uint32(ScanOrder(TX4X4)[1]))
	if err != nil {
		t.Fatalf("CoeffIndexFromScan first AC: %v", err)
	}

	if got := coeffs[eobACIdx]; got != -8 {
		t.Fatalf("eob AC coefficient = %d, want -8", got)
	}
	if got := coeffs[firstACIdx]; got != 8 {
		t.Fatalf("first AC coefficient = %d, want 8", got)
	}
	if len(dec.steps) != 0 {
		t.Fatalf("unused scripted steps remain: %d", len(dec.steps))
	}
}

func TestHiTokenContextOffsetHalfMagnitudeBuckets(t *testing.T) {
	cases := []struct {
		mag  uint32
		want int
	}{
		{mag: 0, want: 0},
		{mag: 1, want: 1},
		{mag: 2, want: 1},
		{mag: 5, want: 3},
		{mag: 6, want: 3},
		{mag: 9, want: 5},
		{mag: 10, want: 5},
		{mag: 24, want: 6},
		{mag: 25, want: 6},
	}
	for _, tc := range cases {
		if got := hiTokenContextOffset(tc.mag); got != tc.want {
			t.Fatalf("hiTokenContextOffset(%d) = %d, want %d", tc.mag, got, tc.want)
		}
	}
}

func TestEOBHiBitCtxDefaultsToTransformContext(t *testing.T) {
	if got, want := eobHiBitCtx(TxfmInfoFor(TX4X4)), 0; got != want {
		t.Fatalf("eobHiBitCtx(TX4X4) = %d, want %d", got, want)
	}
	if got, want := eobHiBitCtx(TxfmInfoFor(TX32X32)), int(TxfmInfoFor(TX32X32).Ctx); got != want {
		t.Fatalf("eobHiBitCtx(TX32X32) = %d, want %d", got, want)
	}
}

func TestEOBHiBitCtxHonorsDebugOverride(t *testing.T) {
	t.Setenv("DEBUG_EOB_HI_CTX", "9")
	if got, want := eobHiBitCtx(TxfmInfoFor(TX4X4)), 4; got != want {
		t.Fatalf("eobHiBitCtx(debug override) = %d, want %d", got, want)
	}
}

func TestEOBHiBitCtxSupportsNamedStrategies(t *testing.T) {
	t.Setenv("DEBUG_EOB_HI_CTX", "tx2d-1")
	if got, want := eobHiBitCtx(TxfmInfoFor(TX8X8)), 1; got != want {
		t.Fatalf("eobHiBitCtx(tx2d-1) = %d, want %d", got, want)
	}
}

func TestEOBBaseCtxDefaultsToSpecThresholds(t *testing.T) {
	info := TxfmInfoFor(TX8X8)
	for _, tc := range []struct {
		eob  int
		want int
	}{
		{eob: 1, want: 1},
		{eob: 8, want: 1},
		{eob: 9, want: 2},
		{eob: 16, want: 2},
		{eob: 17, want: 3},
		{eob: 25, want: 3},
	} {
		if got := eobBaseCtx(info, tc.eob); got != tc.want {
			t.Fatalf("eobBaseCtx(TX8X8, %d) = %d, want %d", tc.eob, got, tc.want)
		}
	}
}

func TestEOBBaseCtxHonorsDebugOverride(t *testing.T) {
	t.Setenv("DEBUG_EOB_BASE_CTX", "9")
	if got, want := eobBaseCtx(TxfmInfoFor(TX8X8), 25), 3; got != want {
		t.Fatalf("eobBaseCtx(debug override) = %d, want %d", got, want)
	}
}

func TestEOBBaseCtxSupportsLegacyNamedOverride(t *testing.T) {
	t.Setenv("DEBUG_EOB_BASE_CTX", "legacy")
	if got, want := eobBaseCtx(TxfmInfoFor(TX8X8), 17), 3; got != want {
		t.Fatalf("eobBaseCtx(legacy) = %d, want %d", got, want)
	}
}

func TestEOBBaseCtxLegacyOverrideCanBeLimitedToTXShape(t *testing.T) {
	t.Setenv("DEBUG_EOB_BASE_CTX", "legacy")
	t.Setenv("DEBUG_EOB_BASE_CTX_TX", "8x8")
	if got, want := eobBaseCtx(TxfmInfoFor(TX8X8), 17), 3; got != want {
		t.Fatalf("eobBaseCtx(legacy 8x8) = %d, want %d", got, want)
	}
	if got, want := eobBaseCtx(TxfmInfoFor(TX16X16), 65), 2; got != want {
		t.Fatalf("eobBaseCtx(legacy 8x8 filtered 16x16) = %d, want %d", got, want)
	}
}

func TestDebugEOBBaseTargetCtxScopesOverrideToMatchedBlock(t *testing.T) {
	info := TxfmInfoFor(TX8X8)
	eob := 17
	matched := BlockGeometry{Start4X: 4, Start4Y: 8}
	other := BlockGeometry{Start4X: 8, Start4Y: 8}

	t.Setenv("DEBUG_EOB_BASE_CTX", "legacy")
	t.Setenv("DEBUG_EOB_BASE_CTX_TARGET", "16,32")
	t.Setenv("DEBUG_EOB_BASE_CTX_TARGET_CHROMA", "luma")

	if got, want := debugEOBBaseTargetCtx(matched, false, 0, info, eob), 3; got != want {
		t.Fatalf("debugEOBBaseTargetCtx(matched) = %d, want %d", got, want)
	}
	if got, want := debugEOBBaseTargetCtx(other, false, 0, info, eob), 2; got != want {
		t.Fatalf("debugEOBBaseTargetCtx(other) = %d, want %d", got, want)
	}
	if got, want := debugEOBBaseTargetCtx(matched, true, 1, info, eob), 2; got != want {
		t.Fatalf("debugEOBBaseTargetCtx(chroma filtered) = %d, want %d", got, want)
	}
}

func TestBaseTokCtxHonorsDebugOverride(t *testing.T) {
	t.Setenv("DEBUG_BASETOK_CTX", "99")
	if got, want := baseTokCtx(7), 40; got != want {
		t.Fatalf("baseTokCtx(debug override) = %d, want %d", got, want)
	}
}

func TestBRTokCtxHonorsDebugOverride(t *testing.T) {
	t.Setenv("DEBUG_BRTOK_CTX", "99")
	if got, want := brTokCtx(7), 20; got != want {
		t.Fatalf("brTokCtx(debug override) = %d, want %d", got, want)
	}
}

func TestCoeffCDFRowHonorsFamilyOverride(t *testing.T) {
	t.Setenv("DEBUG_COEF_BASE_ROW", "9")
	if got, want := coeffCDFRow(1, "base", 4), 4; got != want {
		t.Fatalf("coeffCDFRow(base override) = %d, want %d", got, want)
	}
	if got, want := coeffCDFRow(2, "br", 3), 2; got != want {
		t.Fatalf("coeffCDFRow(br default) = %d, want %d", got, want)
	}
}

func TestCoeffCDFRowSupportsBRContextMap(t *testing.T) {
	t.Setenv("DEBUG_COEF_BR_ROW_MAP", "2:0,3:1")
	if got, want := coeffCDFRowForCoef(nil, 3, "br", 3), 1; got != want {
		t.Fatalf("coeffCDFRow(br ctx map) = %d, want %d", got, want)
	}
	if got, want := coeffCDFRowForCoef(nil, 1, "br", 3), 1; got != want {
		t.Fatalf("coeffCDFRow(br unmapped ctx) = %d, want %d", got, want)
	}
}

func TestCoeffCDFRowPrefersExplicitBROverrideOverMap(t *testing.T) {
	t.Setenv("DEBUG_COEF_BR_ROW", "2")
	t.Setenv("DEBUG_COEF_BR_ROW_MAP", "3:1")
	if got, want := coeffCDFRowForCoef(nil, 3, "br", 3), 2; got != want {
		t.Fatalf("coeffCDFRow(br explicit override) = %d, want %d", got, want)
	}
}

func TestCoeffCDFRowSupportsBRQCatSpecificMap(t *testing.T) {
	t.Setenv("DEBUG_COEF_BR_ROW_MAP", "2/3:1,3/3:2")
	qcat2 := NewDefaultCoefCDFForQCat(2)
	qcat3 := NewDefaultCoefCDFForQCat(3)
	if got, want := coeffCDFRowForCoef(qcat2, 3, "br", 3), 1; got != want {
		t.Fatalf("coeffCDFRow(br qcat2 map) = %d, want %d", got, want)
	}
	if got, want := coeffCDFRowForCoef(qcat3, 3, "br", 3), 2; got != want {
		t.Fatalf("coeffCDFRow(br qcat3 map) = %d, want %d", got, want)
	}
}

func TestCoeffBRCDFRowForOrderSupportsScopedMap(t *testing.T) {
	t.Setenv("DEBUG_COEF_BR_ROW_MAP", "2/3:1")
	t.Setenv("DEBUG_COEF_BR_ROW_MAP_ORDERS", "628-633")
	qcat2 := NewDefaultCoefCDFForQCat(2)
	if got, want := coeffBRCDFRowForOrder(qcat2, 3, 633, 3), 1; got != want {
		t.Fatalf("coeffBRCDFRowForOrder(mapped) = %d, want %d", got, want)
	}
	if got, want := coeffBRCDFRowForOrder(qcat2, 3, 627, 3), 3; got != want {
		t.Fatalf("coeffBRCDFRowForOrder(outside scope) = %d, want %d", got, want)
	}
}

func TestCoeffBRCDFRowForOrderPrefersExplicitOverride(t *testing.T) {
	t.Setenv("DEBUG_COEF_BR_ROW", "2")
	t.Setenv("DEBUG_COEF_BR_ROW_MAP", "2/3:1")
	t.Setenv("DEBUG_COEF_BR_ROW_MAP_ORDERS", "628-633")
	qcat2 := NewDefaultCoefCDFForQCat(2)
	if got, want := coeffBRCDFRowForOrder(qcat2, 3, 627, 3), 2; got != want {
		t.Fatalf("coeffBRCDFRowForOrder(explicit override) = %d, want %d", got, want)
	}
}

func TestDebugCoeffTraceOrderSupportsSinglesAndRanges(t *testing.T) {
	t.Setenv("DEBUG_COEF_TRACE_ORDERS", "630,633-628,700")
	for order, want := range map[int]bool{
		627: false,
		628: true,
		629: true,
		630: true,
		631: true,
		632: true,
		633: true,
		634: false,
		700: true,
	} {
		if got := debugCoeffTraceOrder(order); got != want {
			t.Fatalf("debugCoeffTraceOrder(%d) = %v, want %v", order, got, want)
		}
	}
}

func TestDebugCoeffSkipTargetOverrideHelpers(t *testing.T) {
	t.Setenv("DEBUG_COEF_SKIP_TARGET", "32,0")
	t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "1")
	t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", "2")
	if !debugCoeffSkipTargetMatches(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, true, 2) {
		t.Fatal("expected chroma target match")
	}
	if debugCoeffSkipTargetMatches(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, false, 0) {
		t.Fatal("unexpected luma target match")
	}
	if debugCoeffSkipTargetMatches(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, true, 1) {
		t.Fatal("unexpected chroma plane 1 target match")
	}
	if got, want := debugCoeffSkipOverrideValue("DEBUG_COEF_SKIP_ROW_TARGET", 2, 4), 2; got != want {
		t.Fatalf("default override value = %d, want %d", got, want)
	}
	t.Setenv("DEBUG_COEF_SKIP_ROW_TARGET", "9")
	if got, want := debugCoeffSkipOverrideValue("DEBUG_COEF_SKIP_ROW_TARGET", 2, 4), 4; got != want {
		t.Fatalf("clamped override value = %d, want %d", got, want)
	}
}

func TestDebugCoeffSkipDisableCDFUpdateTargetMatches(t *testing.T) {
	t.Setenv("DEBUG_COEF_SKIP_TARGET", "32,0")
	t.Setenv("DEBUG_COEF_SKIP_TARGET_CHROMA", "1")
	t.Setenv("DEBUG_COEF_SKIP_TARGET_PLANES", "2")
	t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "1")
	if !debugCoeffSkipDisableCDFUpdateTargetMatches(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, true, 2) {
		t.Fatal("expected targeted skip-disable match")
	}
	if debugCoeffSkipDisableCDFUpdateTargetMatches(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, true, 1) {
		t.Fatal("unexpected plane-1 skip-disable match")
	}
	t.Setenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET", "off")
	if debugCoeffSkipDisableCDFUpdateTargetMatches(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, true, 2) {
		t.Fatal("unexpected disabled skip-disable match")
	}
}

func TestDecodeLumaCoefficientsUsesBaseRowOverride(t *testing.T) {
	t.Setenv("DEBUG_COEF_BASE_ROW", "4")

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	coef := NewDefaultCoefCDF(0)
	for row := range coef.BaseTok {
		coef.BaseTok[row][0][0][0] = uint16(100 + row)
	}

	dec := &recordingCoefficientEntropy{
		t:          t,
		adaptVals:  []uint32{0},
		symbolVals: []uint32{1, 0, 0},
	}

	if _, _, _, err := DecodeLumaCoefficientsDCT2D(Block4x4, BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}, TX4X4, 0, coef, DequantTables{{{0, 8}}}, &above, &left, dec); err != nil {
		t.Fatalf("DecodeLumaCoefficientsDCT2D: %v", err)
	}
	if len(dec.symbolCDF0) < 3 {
		t.Fatalf("symbol cdf trace len = %d, want at least 3", len(dec.symbolCDF0))
	}
	if got, want := dec.symbolCDF0[2], uint16(104); got != want {
		t.Fatalf("dc base cdf[0] = %d, want %d from overridden row", got, want)
	}
}

func TestEncodedLoCtxMatchesGetLoCtx(t *testing.T) {
	type tc struct {
		name    string
		tx      TxfmSize
		txClass TxClass
		rc      int
		fillRaw func([]uint8, int, int)
	}
	tests := []tc{
		{
			name:    "2d",
			tx:      TX4X4,
			txClass: TxClass2D,
			rc:      6,
			fillRaw: func(levels []uint8, x, y int) {
				_, _, stride := coeffContextXY(TX4X4, 6)
				levels[(x+0)*stride+(y+1)] = 1
				levels[(x+1)*stride+y] = 2
				levels[(x+2)*stride+y] = 4
				levels[(x+0)*stride+(y+2)] = 5
				levels[(x+1)*stride+(y+1)] = 3
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y, stride := coeffContextXY(tc.tx, tc.rc)
			rawLevels := make([]uint8, coeffLevelsBufferSize(tc.tx))
			tc.fillRaw(rawLevels, x, y)
			encodedLevels := make([]uint8, len(rawLevels))
			for i, tok := range rawLevels {
				encodedLevels[i] = encodeCoeffContextLevel(tok)
			}

			axisVal := x
			if tc.txClass == TxClass2D {
				axisVal = x | y
			} else if tc.txClass == TxClassV {
				axisVal = y
			}
			got, _ := encodedLoCtx(encodedLevels[coeffContextIndex(tc.tx, tc.rc):], tc.txClass, &generatedLoContextOffsets[loContextOffsetGroup(tc.tx)], x, y, axisVal, stride)
			want := GetLoCtx(rawLevels, tc.txClass, tc.tx, tc.rc)
			if got != want {
				t.Fatalf("encodedLoCtx() = %d, want %d", got, want)
			}
		})
	}
}

func TestEncodedHiCtxMatchesGetBRCtx(t *testing.T) {
	type tc struct {
		name    string
		tx      TxfmSize
		txClass TxClass
		rc      int
		fillRaw func([]uint8, int, int)
	}
	tests := []tc{
		{
			name:    "2d",
			tx:      TX4X4,
			txClass: TxClass2D,
			rc:      6,
			fillRaw: func(levels []uint8, x, y int) {
				_, _, stride := coeffContextXY(TX4X4, 6)
				levels[(x+0)*stride+(y+1)] = 1
				levels[(x+1)*stride+y] = 2
				levels[(x+1)*stride+(y+1)] = 3
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y, stride := coeffContextXY(tc.tx, tc.rc)
			rawLevels := make([]uint8, coeffLevelsBufferSize(tc.tx))
			tc.fillRaw(rawLevels, x, y)
			encodedLevels := make([]uint8, len(rawLevels))
			for i, tok := range rawLevels {
				encodedLevels[i] = encodeCoeffContextLevel(tok)
			}

			axisVal := x
			if tc.txClass == TxClass2D {
				axisVal = x | y
			} else if tc.txClass == TxClassV {
				axisVal = y
			}
			_, hiMag := encodedLoCtx(encodedLevels[coeffContextIndex(tc.tx, tc.rc):], tc.txClass, &generatedLoContextOffsets[loContextOffsetGroup(tc.tx)], x, y, axisVal, stride)
			got := encodedHiCtx(tc.txClass, x, y, axisVal, hiMag)
			want := GetBRCtx(rawLevels, tc.txClass, tc.tx, tc.rc)
			if got != want {
				t.Fatalf("encodedHiCtx() = %d, want %d", got, want)
			}
		})
	}
}

func TestEncodedLoCtxFastMatchesGetLoCtxOnCompact2DLayout(t *testing.T) {
	type tc struct {
		name   string
		tx     TxfmSize
		rc     int
		tokens map[int]uint8
	}
	tests := []tc{
		{
			name:   "tx4x4",
			tx:     TX4X4,
			rc:     6,
			tokens: map[int]uint8{10: 1, 9: 2, 14: 4, 11: 5, 13: 3},
		},
		{
			name:   "tx8x8-trace-like",
			tx:     TX8X8,
			rc:     5,
			tokens: map[int]uint8{6: 1},
		},
		{
			name:   "tx8x8-mixed",
			tx:     TX8X8,
			rc:     9,
			tokens: map[int]uint8{10: 2, 17: 1, 25: 3, 11: 4, 18: 5},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawLevels := make([]uint8, coeffLevelsBufferSize(tc.tx))
			compact := make([]uint8, coeffDecodeMetaCache[tc.tx][TxClass2D].levelsBufSize)
			for rc, tok := range tc.tokens {
				rawLevels[coeffContextIndex(tc.tx, rc)] = tok
				compact[rc] = encodeCoeffContextLevel(tok)
			}

			x, y, _ := coeffContextXY(tc.tx, tc.rc)
			info := TxfmInfoFor(tc.tx)
			loBase := int(generatedLoContextOffsets[loContextOffsetGroup(tc.tx)][minInt(y, 4)][minInt(x, 4)])
			got, _ := encodedLoCtxFast(compact[tc.rc:], true, loBase, 4<<minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH)))
			want := GetLoCtx(rawLevels, TxClass2D, tc.tx, tc.rc)
			if got != want {
				t.Fatalf("encodedLoCtxFast() = %d, want %d", got, want)
			}
		})
	}
}

func TestEncodedHiCtxMatchesGetBRCtxOnCompact2DLayout(t *testing.T) {
	type tc struct {
		name   string
		tx     TxfmSize
		rc     int
		tokens map[int]uint8
	}
	tests := []tc{
		{
			name:   "tx4x4",
			tx:     TX4X4,
			rc:     6,
			tokens: map[int]uint8{10: 1, 9: 2, 13: 3},
		},
		{
			name:   "tx8x8-trace-like",
			tx:     TX8X8,
			rc:     5,
			tokens: map[int]uint8{6: 1, 13: 2, 14: 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawLevels := make([]uint8, coeffLevelsBufferSize(tc.tx))
			compact := make([]uint8, coeffDecodeMetaCache[tc.tx][TxClass2D].levelsBufSize)
			for rc, tok := range tc.tokens {
				rawLevels[coeffContextIndex(tc.tx, rc)] = tok
				compact[rc] = encodeCoeffContextLevel(tok)
			}

			x, y, _ := coeffContextXY(tc.tx, tc.rc)
			info := TxfmInfoFor(tc.tx)
			loBase := int(generatedLoContextOffsets[loContextOffsetGroup(tc.tx)][minInt(y, 4)][minInt(x, 4)])
			_, hiMag := encodedLoCtxFast(compact[tc.rc:], true, loBase, 4<<minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH)))
			got := encodedHiCtx(TxClass2D, x, y, x|y, hiMag)
			want := GetBRCtx(rawLevels, TxClass2D, tc.tx, tc.rc)
			if got != want {
				t.Fatalf("encodedHiCtx() = %d, want %d", got, want)
			}
		})
	}
}

func TestCoeffDecodeMetaCacheMatchesHelpers(t *testing.T) {
	for tx := TxfmSize(0); tx < numRectTxfmSizes; tx++ {
		scan := ScanOrder(tx)
		if len(scan) == 0 {
			continue
		}
		scanWidth, scanHeight := ScanCoeffArea(tx)
		orderCount := scanWidth * scanHeight
		loCtxOffsets := &generatedLoContextOffsets[loContextOffsetGroup(tx)]
		for txClass := TxClass2D; txClass <= TxClassV; txClass++ {
			meta := coeffDecodeMetaCache[tx][txClass]
			if len(meta.orders) != orderCount {
				t.Fatalf("len(coeffDecodeMetaCache[%d][%d]) = %d, want %d", tx, txClass, len(meta.orders), orderCount)
			}
			for order := 0; order < orderCount; order++ {
				stride, levelIdx, axisVal, rcI, _ := coeffDecodeLayout(txClass, tx, order, scan)
				x := axisXForClass(txClass, tx, order, scan)
				y := axisYForClass(txClass, tx, order, scan)
				coeffIdx := int(invalidCoeffIndex)
				if idx, err := CoeffIndexFromScan(tx, rcI); err == nil {
					coeffIdx = idx
				}
				wantLoBase := 0
				wantHiBase := 7
				if txClass == TxClass2D {
					wantLoBase = int(loCtxOffsets[minInt(y, 4)][minInt(x, 4)])
					if (x | y) > 1 {
						wantHiBase = 14
					}
				} else {
					wantLoBase = 26 + minInt(axisVal, 2)*5
					if axisVal > 0 {
						wantHiBase = 14
					}
				}
				if meta.stride != stride {
					t.Fatalf("meta.stride tx=%d class=%d = %d, want %d", tx, txClass, meta.stride, stride)
				}
				got := meta.orders[order]
				if int(got.levelIdx) != levelIdx {
					t.Fatalf("levelIdx tx=%d class=%d order=%d = %d, want %d", tx, txClass, order, got.levelIdx, levelIdx)
				}
				if int(got.coeffIdx) != coeffIdx {
					t.Fatalf("coeffIdx tx=%d class=%d order=%d = %d, want %d", tx, txClass, order, got.coeffIdx, coeffIdx)
				}
				if uint32(got.rc) != rcI {
					t.Fatalf("rc tx=%d class=%d order=%d = %d, want %d", tx, txClass, order, got.rc, rcI)
				}
				if int(got.loCtxBase) != wantLoBase {
					t.Fatalf("loCtxBase tx=%d class=%d order=%d = %d, want %d", tx, txClass, order, got.loCtxBase, wantLoBase)
				}
				if int(got.hiBase) != wantHiBase {
					t.Fatalf("hiBase tx=%d class=%d order=%d = %d, want %d", tx, txClass, order, got.hiBase, wantHiBase)
				}
			}
		}
	}
}

func TestCoeffDecodeLayoutUsesCappedScanGeometryForLargeTransforms(t *testing.T) {
	tx := RTX16X64
	scan := ScanOrder(tx)
	order := 66
	rc := int(scan[order])
	if rc != 352 {
		t.Fatalf("ScanOrder(%d)[%d] = %d, want 352", tx, order, rc)
	}

	stride, levelIdx, axisVal, rcI, _ := coeffDecodeLayout(TxClass2D, tx, order, scan)
	if stride != 32 {
		t.Fatalf("stride = %d, want 32", stride)
	}
	if levelIdx != rc {
		t.Fatalf("levelIdx = %d, want %d", levelIdx, rc)
	}
	if rcI != uint32(rc) {
		t.Fatalf("rcI = %d, want %d", rcI, rc)
	}
	if axisVal != 11 {
		t.Fatalf("axisVal = %d, want 11", axisVal)
	}

	x := axisXForClass(TxClass2D, tx, order, scan)
	y := axisYForClass(TxClass2D, tx, order, scan)
	if x != 11 || y != 0 {
		t.Fatalf("axis = (%d,%d), want (11,0)", x, y)
	}
	if got := coeffDecodeMetaCache[tx][TxClass2D].orders[order].loCtxBase; got != 11 {
		t.Fatalf("loCtxBase = %d, want 11", got)
	}
}
