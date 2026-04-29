package decoder

import "testing"

func TestReconstructDCLumaBlock8(t *testing.T) {
	plane := make([]byte, 8*8)
	for i := 0; i < 4; i++ {
		plane[1*8+4+i] = byte(10 * (i + 1))
	}
	for i := 0; i < 4; i++ {
		plane[(2+i)*8+3] = byte(50 + 10*i)
	}

	if err := ReconstructDCLumaBlock8(plane, 8, 8, 8, 4, 2, TX4X4, 64); err != nil {
		t.Fatalf("ReconstructDCLumaBlock8() error = %v", err)
	}

	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got, want := plane[(2+y)*8+4+x], uint8(47); got != want {
				t.Fatalf("plane[%d,%d] = %d, want %d", 4+x, 2+y, got, want)
			}
		}
	}
}

func TestReconstructDCLumaBlock8Bounds(t *testing.T) {
	plane := make([]byte, 16)
	if err := ReconstructDCLumaBlock8(plane, 4, 4, 4, 2, 2, TX4X4, 0); err == nil {
		t.Fatal("expected bounds error")
	}
}

func TestReconstructDCLumaBlock8Rectangular(t *testing.T) {
	tx := RTX8X4
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	stride := 16
	plane := make([]byte, stride*12)
	for i := 0; i < width; i++ {
		plane[3*stride+4+i] = byte(10 + 3*i)
	}
	for i := 0; i < height; i++ {
		plane[(4+i)*stride+3] = byte(70 + 5*i)
	}

	want := append([]byte(nil), plane...)
	left := make([]byte, height)
	for i := 0; i < height; i++ {
		left[i] = want[(4+i)*stride+3]
	}
	if err := PredictDCLuma8(want[4*stride+4:], stride, width, height, want[3*stride+4:], left, true, true); err != nil {
		t.Fatalf("PredictDCLuma8: %v", err)
	}
	coeffs := make([]int32, width*height)
	coeffs[0] = 64
	if err := InverseAddDCTDCT8(want[4*stride+4:], stride, tx, coeffs); err != nil {
		t.Fatalf("InverseAddDCTDCT8: %v", err)
	}

	if err := ReconstructDCLumaBlock8(plane, stride, 16, 12, 4, 4, tx, 64); err != nil {
		t.Fatalf("ReconstructDCLumaBlock8(rectangular) error = %v", err)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			got := plane[(4+y)*stride+4+x]
			wantPx := want[(4+y)*stride+4+x]
			if got != wantPx {
				t.Fatalf("plane[%d,%d] = %d, want %d", 4+x, 4+y, got, wantPx)
			}
		}
	}
}
