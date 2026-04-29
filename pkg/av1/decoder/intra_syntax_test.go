package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedIntraEntropy struct {
	symbols   []uint32
	bools     []uint32
	adapt     []uint32
	equi      []uint32
	bits      []uint32
	uniform   []int
	cdf0      []uint16
	adaptCDF0 []uint16
}

func (s *scriptedIntraEntropy) DecodeBool(prob uint32) uint32 {
	if len(s.bools) == 0 {
		return 0
	}
	v := s.bools[0]
	s.bools = s.bools[1:]
	return v
}

func (s *scriptedIntraEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	if len(cdf) != 0 {
		s.adaptCDF0 = append(s.adaptCDF0, cdf[0])
	}
	if len(s.adapt) == 0 {
		return 0
	}
	v := s.adapt[0]
	s.adapt = s.adapt[1:]
	return v
}

func (s *scriptedIntraEntropy) DecodeBoolEqui() uint32 {
	if len(s.equi) == 0 {
		return 0
	}
	v := s.equi[0]
	s.equi = s.equi[1:]
	return v
}

func (s *scriptedIntraEntropy) DecodeUniform(n uint32) int {
	if len(s.uniform) == 0 {
		return 0
	}
	v := s.uniform[0]
	s.uniform = s.uniform[1:]
	return v
}

func (s *scriptedIntraEntropy) DecodeBools(n uint) uint32 {
	if len(s.bits) == 0 {
		return 0
	}
	v := s.bits[0]
	s.bits = s.bits[1:]
	return v
}

func (s *scriptedIntraEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	if len(cdf) != 0 {
		s.cdf0 = append(s.cdf0, cdf[0])
	}
	if len(s.symbols) == 0 {
		return 0
	}
	v := s.symbols[0]
	s.symbols = s.symbols[1:]
	return v
}

func TestReadKeyIntraBlockSyntaxDC(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeKey,
		TxfmMode:  txModeSwitchable,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{
			uint32(IntraPredDC),
			uint32(IntraPredDC),
			1,
		},
		adapt: []uint32{0},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if syntax.YMode != IntraPredDC {
		t.Fatalf("YMode = %d, want DC", syntax.YMode)
	}
	if syntax.UVMode != IntraPredDC {
		t.Fatalf("UVMode = %d, want DC", syntax.UVMode)
	}
	if syntax.TX != TX8X8 {
		t.Fatalf("TX = %d, want TX8X8", syntax.TX)
	}
	if syntax.UVTX != TX8X8 {
		t.Fatalf("UVTX = %d, want TX8X8 for 4:2:0 chroma", syntax.UVTX)
	}
}

func TestReadKeyIntraBlockSyntaxReadsDirectionalMode(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(IntraPredVertical)},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size: Block8x8,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if syntax.YMode != IntraPredVertical {
		t.Fatalf("YMode = %d, want vertical", syntax.YMode)
	}
}

func TestReadIntraBlockSyntaxInterFrameUsesYModeSizeContext(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeInter}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Mode[0] = uint8(IntraPredPaeth)
	left.Mode[0] = uint8(IntraPredSmooth)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(IntraPredDC)},
	}

	_, err := ReadIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadIntraBlockSyntax: %v", err)
	}
	if len(dec.cdf0) == 0 {
		t.Fatal("expected entropy decoder to observe at least one cdf")
	}
	if got, want := dec.cdf0[0], cdf.YMode[generatedYModeSizeContext[Block16x16]][0]; got != want {
		t.Fatalf("first ymode cdf entry = %d, want inter-frame ymode size-context value %d", got, want)
	}
}

func TestCFLAllowedForBlockDebugOverride(t *testing.T) {
	if got := cflAllowedForBlock(nil, Block32x32); !got {
		t.Fatal("cflAllowedForBlock(Block32x32) = false, want true by default")
	}

	t.Setenv("DEBUG_FORCE_CFL_ALLOWED", "off")
	if got := cflAllowedForBlock(nil, Block32x32); got {
		t.Fatal("cflAllowedForBlock(Block32x32) = true with override off, want false")
	}

	t.Setenv("DEBUG_FORCE_CFL_ALLOWED", "on")
	if got := cflAllowedForBlock(nil, Block64x64); !got {
		t.Fatal("cflAllowedForBlock(Block64x64) = false with override on, want true")
	}
}

func TestCFLAllowedForSegmentLosslessUsesChromaResidualSize(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.Lossless[1] = true

	if got := cflAllowedForSegment(seq, hdr, Block4x8, 1); !got {
		t.Fatal("cflAllowedForSegment(Block4x8, seg=1) = false, want true for 4:2:0 lossless chroma 4x4")
	}
	if got := cflAllowedForSegment(seq, hdr, Block8x8, 1); !got {
		t.Fatal("cflAllowedForSegment(Block8x8, seg=1) = false, want true for 4:2:0 lossless chroma 4x4")
	}
	if got := cflAllowedForSegment(seq, hdr, Block16x16, 1); got {
		t.Fatal("cflAllowedForSegment(Block16x16, seg=1) = true, want false for 4:2:0 lossless chroma 8x8")
	}
	if got := cflAllowedForSegment(seq, hdr, Block8x8, 0); !got {
		t.Fatal("cflAllowedForSegment(Block8x8, seg=0) = false, want lossy/default path to remain allowed")
	}
}

func TestReadKeyIntraBlockSyntaxReadsCFLAlpha(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{
			uint32(IntraPredDC),
			uint32(IntraPredCFL),
			0,
			2,
		},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if syntax.UVMode != IntraPredCFL {
		t.Fatalf("UVMode = %d, want CFL", syntax.UVMode)
	}
	if syntax.CFLAlpha[0] != 0 || syntax.CFLAlpha[1] != -3 {
		t.Fatalf("CFLAlpha = %v, want [0 -3]", syntax.CFLAlpha)
	}
}

func TestShouldDelayCFLUntilAfterLumaPaletteDefaultIsSpecOrder(t *testing.T) {
	seq10 := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 10, SubsamplingX: true, SubsamplingY: true},
	}
	seq8 := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{AllowScreenTools: true}
	g := BlockGeometry{Size: Block16x16, Start4X: 0, Start4Y: 0, End4X: 4, End4Y: 4}

	if shouldDelayCFLUntilAfterLumaPalette(seq10, hdr, g, IntraBlockSyntax{YMode: IntraPredDC, UVMode: IntraPredCFL}) {
		t.Fatal("shouldDelayCFLUntilAfterLumaPalette(10-bit DC+CFL palette block) = true, want false")
	}
	if shouldDelayCFLUntilAfterLumaPalette(seq8, hdr, g, IntraBlockSyntax{YMode: IntraPredDC, UVMode: IntraPredCFL}) {
		t.Fatal("shouldDelayCFLUntilAfterLumaPalette(8-bit DC+CFL palette block) = true, want false")
	}
	if shouldDelayCFLUntilAfterLumaPalette(seq10, hdr, g, IntraBlockSyntax{YMode: IntraPredVertical, UVMode: IntraPredCFL}) {
		t.Fatal("shouldDelayCFLUntilAfterLumaPalette(non-DC luma) = true, want false")
	}
	if shouldDelayCFLUntilAfterLumaPalette(seq10, hdr, g, IntraBlockSyntax{YMode: IntraPredDC, UVMode: IntraPredSmooth}) {
		t.Fatal("shouldDelayCFLUntilAfterLumaPalette(non-CFL chroma) = true, want false")
	}
	if shouldDelayCFLUntilAfterLumaPalette(seq10, hdr, BlockGeometry{Size: Block4x4, Start4X: 0, Start4Y: 0, End4X: 1, End4Y: 1}, IntraBlockSyntax{YMode: IntraPredDC, UVMode: IntraPredCFL}) {
		t.Fatal("shouldDelayCFLUntilAfterLumaPalette(non-palette-capable block) = true, want false")
	}
}

func TestDebugYModeOverrideForBlock(t *testing.T) {
	g := BlockGeometry{Start4X: 0, Start4Y: 12, End4X: 4, End4Y: 16}
	t.Setenv("DEBUG_YMODE_TARGET", "0,48")
	t.Setenv("DEBUG_YMODE_FORCE_TARGET", "9")
	if mode, ok := debugYModeOverrideForBlock(&g); !ok || mode != IntraPredSmooth {
		t.Fatalf("debugYModeOverrideForBlock() = (%d,%v), want (%d,true)", mode, ok, IntraPredSmooth)
	}
	t.Setenv("DEBUG_YMODE_FORCE_TARGET", "")
	if _, ok := debugYModeOverrideForBlock(&g); ok {
		t.Fatal("unexpected override with empty mode env")
	}
}

func TestDebugYModeContextOverrideForBlock(t *testing.T) {
	g := BlockGeometry{Start4X: 0, Start4Y: 12, End4X: 4, End4Y: 16}
	t.Setenv("DEBUG_YMODE_TARGET", "0,48")
	t.Setenv("DEBUG_YMODE_TOPCTX_TARGET", "3")
	t.Setenv("DEBUG_YMODE_LEFTCTX_TARGET", "1")
	top, left, ok := debugYModeContextOverrideForBlock(&g)
	if !ok || top != 3 || left != 1 {
		t.Fatalf("debugYModeContextOverrideForBlock() = (%d,%d,%v), want (3,1,true)", top, left, ok)
	}
	t.Setenv("DEBUG_YMODE_LEFTCTX_TARGET", "")
	if _, _, ok := debugYModeContextOverrideForBlock(&g); ok {
		t.Fatal("unexpected context override with incomplete env")
	}
}

func TestDecodeCFLSigns(t *testing.T) {
	cases := []struct {
		sign  int
		wantU int
		wantV int
	}{
		{sign: 1, wantU: 0, wantV: 1},
		{sign: 2, wantU: 0, wantV: 2},
		{sign: 3, wantU: 1, wantV: 0},
		{sign: 4, wantU: 1, wantV: 1},
		{sign: 5, wantU: 1, wantV: 2},
		{sign: 6, wantU: 2, wantV: 0},
		{sign: 7, wantU: 2, wantV: 1},
		{sign: 8, wantU: 2, wantV: 2},
	}
	for _, tc := range cases {
		gotU, gotV := decodeCFLSigns(tc.sign)
		if gotU != tc.wantU || gotV != tc.wantV {
			t.Fatalf("decodeCFLSigns(%d) = (%d,%d), want (%d,%d)", tc.sign, gotU, gotV, tc.wantU, tc.wantV)
		}
	}
}

func TestIntraPredSpecialModesRemainDistinct(t *testing.T) {
	if IntraPredCFL == IntraPredFilter {
		t.Fatal("IntraPredCFL and IntraPredFilter must remain distinct")
	}
}

func TestCFLAlphaContextDebugOverride(t *testing.T) {
	if got := cflAlphaContext(1, 1, 0); got != 0 {
		t.Fatalf("default cflAlphaContext(V, neg, zero) = %d, want 0", got)
	}

	t.Setenv("DEBUG_CFL_ALPHA_CTX_V", "4")
	if got := cflAlphaContext(1, 1, 0); got != 4 {
		t.Fatalf("cflAlphaContext(V, neg, zero) with V override = %d, want 4", got)
	}

	t.Setenv("DEBUG_CFL_ALPHA_CTX_V", "")
	t.Setenv("DEBUG_CFL_ALPHA_CTX", "2")
	if got := cflAlphaContext(0, 2, 1); got != 2 {
		t.Fatalf("cflAlphaContext(U, pos, neg) with global override = %d, want 2", got)
	}
}

func TestCFLAlphaContextTargetedDebugOverride(t *testing.T) {
	g := BlockGeometry{Start4X: 0, Start4Y: 12}
	other := BlockGeometry{Start4X: 4, Start4Y: 12}
	t.Setenv("DEBUG_CFL_TARGET", "0,48")
	t.Setenv("DEBUG_CFL_ALPHA_CTX_V_TARGET", "4")

	if got := cflAlphaContextForBlock(&g, 1, 1, 0); got != 4 {
		t.Fatalf("targeted cflAlphaContextForBlock(V) = %d, want 4", got)
	}
	if got := cflAlphaContextForBlock(&other, 1, 1, 0); got != 0 {
		t.Fatalf("non-targeted cflAlphaContextForBlock(V) = %d, want 0", got)
	}
}

func TestDebugCFLReadOrder(t *testing.T) {
	if got := debugCFLReadOrder(); got != "immediate" {
		t.Fatalf("default debugCFLReadOrder = %q, want immediate", got)
	}

	t.Setenv("DEBUG_CFL_READ_ORDER", "after_luma_palette")
	if got := debugCFLReadOrder(); got != "after_luma_palette" {
		t.Fatalf("after_luma_palette debugCFLReadOrder = %q", got)
	}

	t.Setenv("DEBUG_CFL_READ_ORDER", "after_luma_indices")
	if got := debugCFLReadOrder(); got != "after_luma_indices" {
		t.Fatalf("after_luma_indices debugCFLReadOrder = %q", got)
	}

	t.Setenv("DEBUG_CFL_READ_ORDER", "skip")
	if got := debugCFLReadOrder(); got != "skip" {
		t.Fatalf("skip debugCFLReadOrder = %q", got)
	}

	t.Setenv("DEBUG_CFL_READ_ORDER", "skip_palette_10bit")
	if got := debugCFLReadOrder(); got != "skip_palette_10bit" {
		t.Fatalf("skip_palette_10bit debugCFLReadOrder = %q", got)
	}
}

func TestDebugCFLReadOrderTargetOverride(t *testing.T) {
	g := BlockGeometry{Start4X: 0, Start4Y: 12}
	other := BlockGeometry{Start4X: 4, Start4Y: 12}
	t.Setenv("DEBUG_CFL_TARGET", "0,48")
	t.Setenv("DEBUG_CFL_READ_ORDER_TARGET", "after_luma_palette")

	if got := debugCFLReadOrderForBlock(&g); got != "after_luma_palette" {
		t.Fatalf("debugCFLReadOrderForBlock(target) = %q, want after_luma_palette", got)
	}
	if got := debugCFLReadOrderForBlock(&other); got != "immediate" {
		t.Fatalf("debugCFLReadOrderForBlock(non-target) = %q, want immediate", got)
	}
}

func TestDebugCFLSignTokenOverride(t *testing.T) {
	if _, ok := debugCFLSignTokenOverride(); ok {
		t.Fatal("default debugCFLSignTokenOverride unexpectedly set")
	}

	t.Setenv("DEBUG_CFL_SIGN_TOKEN", "5")
	if got, ok := debugCFLSignTokenOverride(); !ok || got != 5 {
		t.Fatalf("debugCFLSignTokenOverride = (%d,%v), want (5,true)", got, ok)
	}
}

func TestDebugCFLSignTokenTargetOverride(t *testing.T) {
	g := BlockGeometry{Start4X: 0, Start4Y: 12}
	other := BlockGeometry{Start4X: 4, Start4Y: 12}
	t.Setenv("DEBUG_CFL_TARGET", "0,48")
	t.Setenv("DEBUG_CFL_SIGN_TOKEN_TARGET", "6")

	if got, ok := debugCFLSignTokenOverrideForBlock(&g); !ok || got != 6 {
		t.Fatalf("debugCFLSignTokenOverrideForBlock(target) = (%d,%v), want (6,true)", got, ok)
	}
	if _, ok := debugCFLSignTokenOverrideForBlock(&other); ok {
		t.Fatal("debugCFLSignTokenOverrideForBlock(non-target) unexpectedly set")
	}
}

func TestDebugDisableCFLCDFUpdate(t *testing.T) {
	if debugDisableCFLCDFUpdate("sign") || debugDisableCFLCDFUpdate("alpha") {
		t.Fatal("default debugDisableCFLCDFUpdate unexpectedly enabled")
	}

	t.Setenv("DEBUG_DISABLE_CFL_CDF_UPDATE", "sign")
	if !debugDisableCFLCDFUpdate("sign") || debugDisableCFLCDFUpdate("alpha") {
		t.Fatal("sign-only disable did not match expected families")
	}

	t.Setenv("DEBUG_DISABLE_CFL_CDF_UPDATE", "alpha")
	if debugDisableCFLCDFUpdate("sign") || !debugDisableCFLCDFUpdate("alpha") {
		t.Fatal("alpha-only disable did not match expected families")
	}

	t.Setenv("DEBUG_DISABLE_CFL_CDF_UPDATE", "both")
	if !debugDisableCFLCDFUpdate("sign") || !debugDisableCFLCDFUpdate("alpha") {
		t.Fatal("both disable did not enable both families")
	}
}

func TestReadKeyIntraBlockSyntaxReadsFilterIntra(t *testing.T) {
	seq := av1.SequenceHeader{
		EnableFilterIntra: true,
		ColorConfig:       av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(IntraPredDC), uint32(IntraPredDC), 2},
		adapt:   []uint32{1},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if syntax.YMode != IntraPredFilter {
		t.Fatalf("YMode = %d, want filter intra", syntax.YMode)
	}
	if syntax.YAngle != 2 {
		t.Fatalf("YAngle = %d, want 2", syntax.YAngle)
	}
}

func TestReadKeyIntraBlockSyntaxDoesNotReadFilterIntraForNonDC(t *testing.T) {
	seq := av1.SequenceHeader{
		EnableFilterIntra: true,
		ColorConfig:       av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(IntraPredVertical), uint32(IntraPredDC)},
		adapt:   []uint32{1},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if syntax.YMode != IntraPredVertical {
		t.Fatalf("YMode = %d, want vertical", syntax.YMode)
	}
	if got := len(dec.adapt); got != 1 {
		t.Fatalf("filter intra flag was consumed for non-DC mode, remaining adapt entries = %d, want 1", got)
	}
}

func TestReadKeyIntraBlockSyntaxReadsPalette(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{
		FrameType:        obu.FrameTypeKey,
		AllowScreenTools: true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{
			uint32(IntraPredDC),
			uint32(IntraPredDC),
			0,
		},
		adapt:   []uint32{1, 0},
		bits:    []uint32{10, 0, 3},
		uniform: []int{1},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, false, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if got, want := syntax.PalYSize, uint8(2); got != want {
		t.Fatalf("PalYSize = %d, want %d", got, want)
	}
	if got, want := syntax.PalY[0], uint16(10); got != want {
		t.Fatalf("PalY[0] = %d, want %d", got, want)
	}
	if got, want := syntax.PalY[1], uint16(14); got != want {
		t.Fatalf("PalY[1] = %d, want %d", got, want)
	}
	if len(syntax.PalYIdx) != 64 {
		t.Fatalf("len(PalYIdx) = %d, want 64", len(syntax.PalYIdx))
	}
	if got, want := syntax.PalYIdx[0], uint8(1); got != want {
		t.Fatalf("PalYIdx[0] = %d, want %d", got, want)
	}
}

func TestReadKeyIntraBlockSyntaxUsesVisiblePaletteIndexSizeAtFrameEdge(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{
		FrameType:        obu.FrameTypeKey,
		AllowScreenTools: true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{
			uint32(IntraPredDC),
			uint32(IntraPredDC),
			0,
		},
		adapt:   []uint32{1, 0},
		bits:    []uint32{10, 0, 3},
		uniform: []int{1},
	}

	syntax, err := ReadKeyIntraBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block8x32,
		Start4X: 224,
		Start4Y: 128,
		End4X:   226,
		End4Y:   135,
	}, true, false, 0, dec)
	if err != nil {
		t.Fatalf("ReadKeyIntraBlockSyntax: %v", err)
	}
	if got, want := len(syntax.PalYIdx), 8*28; got != want {
		t.Fatalf("len(PalYIdx) = %d, want visible 8x28=%d", got, want)
	}
}

func TestApplyIntraBlockContext(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	syntax := IntraBlockSyntax{
		Geometry: BlockGeometry{
			Size:    Block16x16,
			Start4X: 4,
			Start4Y: 8,
			End4X:   8,
			End4Y:   12,
		},
		Size:      Block16x16,
		YMode:     IntraPredDC,
		UVMode:    IntraPredDC,
		SkipMode:  true,
		Skip:      true,
		PalYSize:  2,
		PalUVSize: 2,
		PalY:      [8]uint16{12, 33},
		PalU:      [8]uint16{90, 120},
		PalV:      [8]uint16{100, 110},
		TX:        TX8X8,
		UVTX:      TX4X4,
	}

	if err := ApplyIntraBlockContext(seq, hdr, &above, &left, syntax); err != nil {
		t.Fatalf("ApplyIntraBlockContext: %v", err)
	}
	for i := 4; i < 8; i++ {
		if got := above.Mode[i]; got != uint8(IntraPredDC) {
			t.Fatalf("above.Mode[%d] = %d, want DC", i, got)
		}
		if got := above.TxIntra[i]; got != 1 {
			t.Fatalf("above.TxIntra[%d] = %d, want 1", i, got)
		}
		if got := above.PalSz[i]; got != 2 {
			t.Fatalf("above.PalSz[%d] = %d, want 2", i, got)
		}
		if got := above.SkipMode[i]; got != 1 {
			t.Fatalf("above.SkipMode[%d] = %d, want 1", i, got)
		}
		if got := above.Skip[i]; got != 1 {
			t.Fatalf("above.Skip[%d] = %d, want 1", i, got)
		}
	}
	for i := 8; i < 12; i++ {
		if got := left.Mode[i]; got != uint8(IntraPredDC) {
			t.Fatalf("left.Mode[%d] = %d, want DC", i, got)
		}
		if got := left.TxIntra[i]; got != 1 {
			t.Fatalf("left.TxIntra[%d] = %d, want 1", i, got)
		}
		if got := left.PalSz[i]; got != 2 {
			t.Fatalf("left.PalSz[%d] = %d, want 2", i, got)
		}
		if got := left.SkipMode[i]; got != 1 {
			t.Fatalf("left.SkipMode[%d] = %d, want 1", i, got)
		}
		if got := left.Skip[i]; got != 1 {
			t.Fatalf("left.Skip[%d] = %d, want 1", i, got)
		}
	}
	for i := 2; i < 4; i++ {
		if got := above.UVMode[i]; got != uint8(IntraPredDC) {
			t.Fatalf("above.UVMode[%d] = %d, want DC", i, got)
		}
	}
	for i := 4; i < 6; i++ {
		if got := left.UVMode[i]; got != uint8(IntraPredDC) {
			t.Fatalf("left.UVMode[%d] = %d, want DC", i, got)
		}
	}
	for i := 4; i < 8; i++ {
		if got := above.UVPalSz[i]; got != 2 {
			t.Fatalf("above.UVPalSz[%d] = %d, want 2", i, got)
		}
		if got := above.Pal[0][i][1]; got != 33 {
			t.Fatalf("above.Pal[0][%d][1] = %d, want 33", i, got)
		}
		if got := above.Pal[1][i][0]; got != 90 {
			t.Fatalf("above.Pal[1][%d][0] = %d, want 90", i, got)
		}
		if got := above.Pal[2][i][0]; got != 100 {
			t.Fatalf("above.Pal[2][%d][0] = %d, want 100", i, got)
		}
	}
}

func TestApplyIntraBlockContextFilterIntraStoresDCMode(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	syntax := IntraBlockSyntax{
		Geometry: BlockGeometry{
			Size:    Block32x32,
			Start4X: 4,
			Start4Y: 8,
			End4X:   12,
			End4Y:   16,
		},
		Size:   Block32x32,
		YMode:  IntraPredFilter,
		YAngle: 3,
		UVMode: IntraPredDC,
		TX:     TX8X8,
		UVTX:   TX16X16,
	}

	if err := ApplyIntraBlockContext(seq, hdr, &above, &left, syntax); err != nil {
		t.Fatalf("ApplyIntraBlockContext: %v", err)
	}
	for i := 4; i < 12; i++ {
		if got := above.Mode[i]; got != uint8(IntraPredDC) {
			t.Fatalf("above.Mode[%d] = %d, want DC for filter intra context", i, got)
		}
	}
	for i := 8; i < 16; i++ {
		if got := left.Mode[i]; got != uint8(IntraPredDC) {
			t.Fatalf("left.Mode[%d] = %d, want DC for filter intra context", i, got)
		}
	}
}

func TestApplyIntraBlockContextPreservesExistingTXContext(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	for i := 4; i < 8; i++ {
		above.TxIntra[i] = 0
		above.Tx[i] = 0
	}
	for i := 8; i < 12; i++ {
		left.TxIntra[i] = 0
		left.Tx[i] = 0
	}

	syntax := IntraBlockSyntax{
		Geometry: BlockGeometry{
			Size:    Block16x16,
			Start4X: 4,
			Start4Y: 8,
			End4X:   8,
			End4Y:   12,
		},
		Size:              Block16x16,
		YMode:             IntraPredDC,
		UVMode:            IntraPredDC,
		TX:                TX8X8,
		UVTX:              TX4X4,
		PreserveTXContext: true,
	}

	if err := ApplyIntraBlockContext(seq, hdr, &above, &left, syntax); err != nil {
		t.Fatalf("ApplyIntraBlockContext: %v", err)
	}
	for i := 4; i < 8; i++ {
		if got := above.TxIntra[i]; got != 0 {
			t.Fatalf("above.TxIntra[%d] = %d, want preserved 0", i, got)
		}
		if got := above.Tx[i]; got != 0 {
			t.Fatalf("above.Tx[%d] = %d, want preserved 0", i, got)
		}
	}
	for i := 8; i < 12; i++ {
		if got := left.TxIntra[i]; got != 0 {
			t.Fatalf("left.TxIntra[%d] = %d, want preserved 0", i, got)
		}
		if got := left.Tx[i]; got != 0 {
			t.Fatalf("left.Tx[%d] = %d, want preserved 0", i, got)
		}
	}
}

func TestHasChromaPlane(t *testing.T) {
	mono := av1.SequenceHeader{ColorConfig: av1.ColorConfig{Monochrome: true}}
	if hasChromaPlane(mono, Block16x16, 0, 0) {
		t.Fatal("monochrome sequence unexpectedly reports chroma")
	}
	color := av1.SequenceHeader{ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true}}
	if !hasChromaPlane(color, Block16x16, 0, 0) {
		t.Fatal("4:2:0 sequence unexpectedly reports no chroma")
	}
	if hasChromaPlane(color, Block4x4, 1, 0) {
		t.Fatal("top-right 4x4 in 4:2:0 unexpectedly reports chroma")
	}
	if hasChromaPlane(color, Block4x4, 0, 1) {
		t.Fatal("bottom-left 4x4 in 4:2:0 unexpectedly reports chroma")
	}
	if !hasChromaPlane(color, Block4x4, 1, 1) {
		t.Fatal("bottom-right 4x4 in 4:2:0 unexpectedly reports no chroma")
	}
}
