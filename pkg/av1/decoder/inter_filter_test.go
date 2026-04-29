package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestGetFilterCtxUsesMatchingReferenceNeighbors(t *testing.T) {
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Ref[0][3] = 1
	above.Filter[0][3] = interFilterSharp
	left.Ref[0][2] = 1
	left.Filter[0][2] = interFilterSharp

	ctx := GetFilterCtx(&above, &left, 0, 0, 1, 2, 3)
	if ctx != int(interFilterSharp) {
		t.Fatalf("GetFilterCtx = %d, want %d", ctx, interFilterSharp)
	}

	left.Filter[0][2] = interFilterSmooth
	ctx = GetFilterCtx(&above, &left, 0, 0, 1, 2, 3)
	if ctx != int(switchableFilterUnset) {
		t.Fatalf("GetFilterCtx mismatch = %d, want %d", ctx, switchableFilterUnset)
	}
}

func TestReadInterFilterPairSwitchableDual(t *testing.T) {
	seq := av1.SequenceHeader{EnableDualFilter: true}
	hdr := &obu.FrameHeader{SubpelFilterMode: interFilterSwitchable}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Ref[0][0] = 0
	left.Ref[0][0] = 0
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(interFilterSmooth), uint32(interFilterSharp)},
	}

	filter, err := ReadInterFilterPair(seq, hdr, cdf, &above, &left, BlockGeometry{Start4X: 0, Start4Y: 0}, 0, false, MotionVector{X: 1}, dec)
	if err != nil {
		t.Fatalf("ReadInterFilterPair: %v", err)
	}
	if filter[0] != interFilterSmooth || filter[1] != interFilterSharp {
		t.Fatalf("filter = %v, want [%d %d]", filter, interFilterSmooth, interFilterSharp)
	}
}

func TestDefaultInterFilterCDFMatchesDav1d(t *testing.T) {
	if got, want := generatedInterFilterCDF[1][0], ([3]uint16{858, 44, 0}); got != want {
		t.Fatalf("generatedInterFilterCDF[1][0] = %v, want %v", got, want)
	}
}

func TestReadInterFilterPairForSingleReadsGlobalTranslation(t *testing.T) {
	seq := av1.SequenceHeader{EnableDualFilter: true}
	hdr := &obu.FrameHeader{SubpelFilterMode: interFilterSwitchable}
	hdr.GMV[0] = obu.WarpedMotionParams{Type: 1}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(interFilterSmooth), uint32(interFilterSharp)},
	}

	filter, err := ReadInterFilterPairForSingle(seq, hdr, cdf, &above, &left, BlockGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   8,
	}, 0, InterPredGlobal, dec)
	if err != nil {
		t.Fatalf("ReadInterFilterPairForSingle: %v", err)
	}
	if filter[0] != interFilterSmooth || filter[1] != interFilterSharp {
		t.Fatalf("filter = %v, want [%d %d]", filter, interFilterSmooth, interFilterSharp)
	}
}

func TestReadInterFilterPairForSingleSkipsLargeAffineGlobal(t *testing.T) {
	seq := av1.SequenceHeader{EnableDualFilter: true}
	hdr := &obu.FrameHeader{SubpelFilterMode: interFilterSwitchable}
	hdr.GMV[0] = obu.WarpedMotionParams{Type: 2}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		symbols: []uint32{uint32(interFilterSmooth), uint32(interFilterSharp)},
	}

	filter, err := ReadInterFilterPairForSingle(seq, hdr, cdf, &above, &left, BlockGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   8,
	}, 0, InterPredGlobal, dec)
	if err != nil {
		t.Fatalf("ReadInterFilterPairForSingle: %v", err)
	}
	if filter[0] != interFilterRegular || filter[1] != interFilterRegular {
		t.Fatalf("filter = %v, want regular/default", filter)
	}
	if len(dec.symbols) != 2 {
		t.Fatalf("symbols consumed for ineligible global filter, remaining=%d", len(dec.symbols))
	}
}

func TestPredictInterLumaBlock8WithFiltersChangesOutput(t *testing.T) {
	ref := make([]byte, 8*8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			ref[y*8+x] = uint8(x*19 + y*11)
		}
	}
	regular := make([]byte, 8*8)
	filtered := make([]byte, 8*8)

	mv := MotionVector{X: 3, Y: 5}
	if err := PredictInterLumaBlock8WithFilters(regular, 8, 8, 8, 0, 0, 8, 8, ref, 8, 8, 8, mv, InterFilterPair{interFilterRegular, interFilterRegular}); err != nil {
		t.Fatalf("regular predict: %v", err)
	}
	if err := PredictInterLumaBlock8WithFilters(filtered, 8, 8, 8, 0, 0, 8, 8, ref, 8, 8, 8, mv, InterFilterPair{interFilterSmooth, interFilterSharp}); err != nil {
		t.Fatalf("filtered predict: %v", err)
	}
	if string(regular) == string(filtered) {
		t.Fatalf("filtered prediction did not change output")
	}
}

func TestPredictInterLumaBlock8SmallSharpUsesRegularSmallFilter(t *testing.T) {
	ref := make([]byte, 16*8)
	rows := [][]byte{
		{149, 148, 148, 148, 146, 169, 214, 235, 244, 244},
		{148, 148, 147, 148, 147, 169, 213, 235, 244, 244},
		{147, 148, 147, 148, 147, 169, 213, 236, 245, 245},
		{147, 147, 147, 148, 147, 169, 213, 236, 245, 245},
	}
	for y, row := range rows {
		copy(ref[y*16+2:], row)
	}

	got := make([]byte, 4*4)
	predictInterLumaBlock8WithFiltersAt(got, 4, 4, 4, 0, 0, 4, 4, 4, 0, ref, 16, 16, 8, MotionVector{X: 10}, InterFilterPair{interFilterRegular, interFilterSharp})
	want := []byte{
		146, 148, 180, 223,
		147, 149, 180, 222,
		147, 149, 180, 222,
		147, 149, 180, 222,
	}
	if string(got) != string(want) {
		t.Fatalf("small sharp prediction = %v, want %v", debugTraceRows8(got, 4, 4, 4), debugTraceRows8(want, 4, 4, 4))
	}
}
