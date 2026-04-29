package decoder

import (
	"bytes"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedKeyIntraEntropy struct {
	scriptedIntraEntropy
	scriptedCoefficientEntropy
}

func (s *scriptedKeyIntraEntropy) DecodeBool(prob uint32) uint32 {
	return s.scriptedIntraEntropy.DecodeBool(prob)
}

func (s *scriptedKeyIntraEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	if len(s.scriptedIntraEntropy.adapt) != 0 {
		return s.scriptedIntraEntropy.DecodeBoolAdapt(cdf)
	}
	return s.scriptedCoefficientEntropy.DecodeBoolAdapt(cdf)
}

func (s *scriptedKeyIntraEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	if len(s.scriptedIntraEntropy.symbols) != 0 {
		return s.scriptedIntraEntropy.DecodeSymbolAdapt(cdf, n)
	}
	return s.scriptedCoefficientEntropy.DecodeSymbolAdapt(cdf, n)
}

func (s *scriptedKeyIntraEntropy) DecodeBoolEqui() uint32 {
	if len(s.scriptedIntraEntropy.equi) != 0 {
		return s.scriptedIntraEntropy.DecodeBoolEqui()
	}
	return s.scriptedCoefficientEntropy.DecodeBoolEqui()
}

func (s *scriptedKeyIntraEntropy) DecodeUniform(n uint32) int {
	return s.scriptedIntraEntropy.DecodeUniform(n)
}

func (s *scriptedKeyIntraEntropy) DecodeBools(n uint) uint32 {
	return s.scriptedIntraEntropy.DecodeBools(n)
}

func (s *scriptedKeyIntraEntropy) DecodeHiTok(cdf []uint16) uint32 {
	return s.scriptedCoefficientEntropy.DecodeHiTok(cdf)
}

func TestDecodeAndReconstructKeyIntraDCLumaBlock8(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{Monochrome: true},
	}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeKey,
	}
	hdr.Quantization.YAC = 8
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(hdr.Quantization.YAC)
	dq := InitDequantTables(seq, hdr, int(hdr.Quantization.YAC))

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	plane := make([]byte, 8*8)

	dec := &scriptedKeyIntraEntropy{
		scriptedIntraEntropy: scriptedIntraEntropy{
			symbols: []uint32{uint32(IntraPredDC)},
			adapt:   []uint32{},
		},
		scriptedCoefficientEntropy: scriptedCoefficientEntropy{
			adapt:  []uint32{0, 0},
			symbol: []uint32{0, 0},
		},
	}

	syntax, err := DecodeAndReconstructKeyIntraDCLumaBlock8(seq, hdr, modeCDF, coefCDF, dq, &above, &left, BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}, 0, plane, 8, 8, 8, dec)
	if err != nil {
		t.Fatalf("DecodeAndReconstructKeyIntraDCLumaBlock8: %v", err)
	}
	if syntax.YMode != IntraPredDC {
		t.Fatalf("YMode = %d, want DC", syntax.YMode)
	}

	wantDC := expectedDCOnlyPixel(128, int(dq[0][0][0]), 0)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got := plane[y*8+x]; got != wantDC {
				t.Fatalf("plane[%d,%d] = %d, want %d", x, y, got, wantDC)
			}
		}
	}
	if got := above.Mode[0]; got != uint8(IntraPredDC) {
		t.Fatalf("above.Mode[0] = %d, want DC", got)
	}
	if got := above.LCoef[0]; got == 0x40 {
		t.Fatalf("above.LCoef[0] was not updated")
	}
}

func TestDecodeAndReconstructKeyIntraDCLumaBlock8SupportsUniformSubdividedTX(t *testing.T) {
	seq := av1.SequenceHeader{ColorConfig: av1.ColorConfig{Monochrome: true}}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey, TxfmMode: txModeSwitchable}
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(0)
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	dec := &scriptedKeyIntraEntropy{
		scriptedIntraEntropy: scriptedIntraEntropy{
			symbols: []uint32{uint32(IntraPredDC), 1},
		},
		scriptedCoefficientEntropy: scriptedCoefficientEntropy{
			adapt: []uint32{1, 1, 1, 1},
		},
	}

	_, err := DecodeAndReconstructKeyIntraDCLumaBlock8(seq, hdr, modeCDF, coefCDF, DequantTables{}, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, 0, make([]byte, 16*16), 16, 16, 16, dec)
	if err != nil {
		t.Fatalf("DecodeAndReconstructKeyIntraDCLumaBlock8: %v", err)
	}
}

func TestDecodeAndReconstructKeyIntraDCLumaBlock8SkipDoesNotConsumeTxType(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{Monochrome: true},
	}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeKey,
		TxfmMode:  txModeLargest,
	}
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(0)

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	dec := &scriptedKeyIntraEntropy{
		scriptedIntraEntropy: scriptedIntraEntropy{
			symbols: []uint32{uint32(IntraPredDC)},
		},
		scriptedCoefficientEntropy: scriptedCoefficientEntropy{
			adapt:  []uint32{1},
			symbol: []uint32{99},
		},
	}

	_, err := DecodeAndReconstructKeyIntraDCLumaBlock8(seq, hdr, modeCDF, coefCDF, DequantTables{}, &above, &left, BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}, 0, make([]byte, 4*4), 4, 4, 4, dec)
	if err != nil {
		t.Fatalf("DecodeAndReconstructKeyIntraDCLumaBlock8: %v", err)
	}
	if got := len(dec.scriptedCoefficientEntropy.symbol); got != 1 {
		t.Fatalf("coefficient tx-type symbol stream consumed on skip, remaining=%d want 1", got)
	}
}

func TestShouldUsePaletteLumaPredictor8PrefersIntraWhenPaletteBreaksLeftEdge(t *testing.T) {
	planeWidth, planeHeight := 9, 8
	stride := planeWidth
	plane := make([]byte, stride*planeHeight)
	for y := 0; y < planeHeight; y++ {
		plane[y*stride] = 100
	}
	var syntax IntraBlockSyntax
	syntax.Geometry = BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}
	syntax.YMode = IntraPredDC
	syntax.PalYSize = 2
	syntax.PalY[0] = 0
	syntax.PalY[1] = 200
	syntax.PalYIdx = make([]uint8, 8*8)
	for i := range syntax.PalYIdx {
		syntax.PalYIdx[i] = uint8(i & 1)
	}

	got, err := shouldUsePaletteLumaPredictor8(plane, stride, planeWidth, planeHeight, 1, 0, 8, 8, syntax, 0, false, true, true, true)
	if err != nil {
		t.Fatalf("shouldUsePaletteLumaPredictor8: %v", err)
	}
	if got {
		t.Fatalf("shouldUsePaletteLumaPredictor8 = true, want false")
	}
}

func TestIntraBoundaryProfileCost8PrefersEdgeGradients(t *testing.T) {
	top := []byte{10, 20, 30, 40}
	palette := []byte{10, 10, 10, 10}
	intra := []byte{10, 20, 30, 40}
	if got, want := intraBoundaryProfileCost8(palette, 4, 4, 1, top, nil, true, false), 120; got != want {
		t.Fatalf("palette profile cost = %d, want %d", got, want)
	}
	if got := intraBoundaryProfileCost8(intra, 4, 4, 1, top, nil, true, false); got != 0 {
		t.Fatalf("intra profile cost = %d, want 0", got)
	}
}

func TestPaletteHasDuplicateValues(t *testing.T) {
	if !paletteHasDuplicateValues([8]uint16{10, 20, 20}, 3) {
		t.Fatalf("paletteHasDuplicateValues = false, want true")
	}
	if paletteHasDuplicateValues([8]uint16{10, 20, 30}, 3) {
		t.Fatalf("paletteHasDuplicateValues = true, want false")
	}
}

func TestShouldUsePaletteLumaPredictor8KeepsPaletteWithoutEdges(t *testing.T) {
	var syntax IntraBlockSyntax
	syntax.Geometry = BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}
	syntax.PalYSize = 2
	syntax.PalYIdx = make([]uint8, 8*8)

	got, err := shouldUsePaletteLumaPredictor8(make([]byte, 8*8), 8, 8, 8, 0, 0, 8, 8, syntax, 0, false, false, true, true)
	if err != nil {
		t.Fatalf("shouldUsePaletteLumaPredictor8: %v", err)
	}
	if !got {
		t.Fatalf("shouldUsePaletteLumaPredictor8 = false, want true")
	}
}

func expectedDCOnlyPixel(base, coeff, shift int) uint8 {
	dc := coeff
	dc = (dc*181 + 128) >> 8
	dc = (dc + ((1 << shift) >> 1)) >> shift
	dc = (dc*181 + 128 + 2048) >> 12
	return clipByte(base + dc)
}

func TestShouldSearchNonPaletteLumaFallback(t *testing.T) {
	if !shouldSearchNonPaletteLumaFallback(IntraBlockSyntax{YMode: IntraPredSmooth}, true, false, 128, 128) {
		t.Fatalf("shouldSearchNonPaletteLumaFallback = false, want true for large one-edge smooth block")
	}
	if shouldSearchNonPaletteLumaFallback(IntraBlockSyntax{YMode: IntraPredDC}, true, false, 128, 128) {
		t.Fatalf("shouldSearchNonPaletteLumaFallback = true, want false for DC block")
	}
	if shouldSearchNonPaletteLumaFallback(IntraBlockSyntax{YMode: IntraPredSmooth}, true, true, 128, 128) {
		t.Fatalf("shouldSearchNonPaletteLumaFallback = true, want false when both edges are present")
	}
	if shouldSearchNonPaletteLumaFallback(IntraBlockSyntax{YMode: IntraPredSmooth}, true, false, 16, 16) {
		t.Fatalf("shouldSearchNonPaletteLumaFallback = true, want false for small block")
	}
	if shouldSearchNonPaletteLumaFallback(IntraBlockSyntax{YMode: IntraPredSmooth, PalYSize: 2}, true, false, 128, 128) {
		t.Fatalf("shouldSearchNonPaletteLumaFallback = true, want false for palette block")
	}
}

func TestReconstructNonPaletteCandidateLumaBlock8ReplaysTUsSequentially(t *testing.T) {
	syntax := IntraBlockSyntax{
		Size:   Block8x8,
		TX:     TX4X4,
		YMode:  IntraPredSmooth,
		UVMode: IntraPredDC,
		Geometry: BlockGeometry{
			Size:    Block8x8,
			Start4X: 0,
			Start4Y: 0,
			End4X:   2,
			End4Y:   2,
		},
	}

	firstTU := make([]int32, 16)
	firstTU[0] = 16
	tus := []decodedLumaTU8{
		{offX4: 0, offY4: 0, tx: TX4X4, txType: TxTypeWHTWHT, coeffs: firstTU},
		{offX4: 1, offY4: 0, tx: TX4X4, txType: TxTypeWHTWHT, coeffs: make([]int32, 16)},
		{offX4: 0, offY4: 1, tx: TX4X4, txType: TxTypeWHTWHT, coeffs: make([]int32, 16)},
		{offX4: 1, offY4: 1, tx: TX4X4, txType: TxTypeWHTWHT, coeffs: make([]int32, 16)},
	}
	left := []byte{20, 40, 60, 80, 100, 120, 140, 160}

	got, err := reconstructNonPaletteCandidateLumaBlock8(8, 8, syntax, IntraPredSmooth, 0, nil, left, left[0], false, true, true, true, true, tus)
	if err != nil {
		t.Fatalf("reconstructNonPaletteCandidateLumaBlock8: %v", err)
	}

	want := make([]byte, 8*8)
	edgeLen := 8 + 8
	scratchStride := edgeLen + 1
	scratchHeight := edgeLen + 1
	scratch := make([]byte, scratchStride*scratchHeight)
	fillExtendedLeftEdge8(scratch, scratchStride, left)
	scratch[0] = left[0]
	for _, tu := range tus {
		tuEdge := intraTUNodeEdgeAvailability(2, 2, tu.offX4, tu.offY4, tu.tx, intraEdgeAvailability{
			topHasRight:   true,
			leftHasBottom: true,
		})
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			scratchStride,
			scratchStride,
			scratchHeight,
			1+tu.offX4*4,
			1+tu.offY4*4,
			tu.tx,
			tu.txType,
			IntraPredSmooth,
			0,
			tu.coeffs,
			true,
			0,
			tu.offY4 > 0,
			true,
			tuEdge.topHasRight,
			tuEdge.leftHasBottom,
		); err != nil {
			t.Fatalf("sequential replay: %v", err)
		}
	}
	for row := 0; row < 8; row++ {
		copy(want[row*8:(row+1)*8], scratch[(row+1)*scratchStride+1:(row+1)*scratchStride+1+8])
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("candidate replay mismatch\n got=%v\nwant=%v", got, want)
	}

	naive := make([]byte, 8*8)
	if err := PredictIntraLuma8WithFlags(naive, 8, 8, 8, IntraPredSmooth, 0, nil, left, left[0], false, true, 0); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags: %v", err)
	}
	if err := InverseAddResidualTx8At(naive, 8, 8, 8, 0, 0, TX4X4, TxTypeWHTWHT, firstTU, true); err != nil {
		t.Fatalf("InverseAddResidualTx8At: %v", err)
	}
	if bytes.Equal(got, naive) {
		t.Fatalf("candidate replay collapsed to naive whole-block predictor")
	}
}
