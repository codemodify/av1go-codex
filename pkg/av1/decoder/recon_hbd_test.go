package decoder

import "testing"

func TestPredictIntraLuma16UnsupportedModeFallsBackToDC(t *testing.T) {
	dst := make([]uint16, 4)
	top := []uint16{100, 200}
	left := []uint16{300, 400}
	if err := PredictIntraLuma16(dst, 2, 2, 2, IntraPredMode(255), 5, top, left, 0, true, true, 10); err != nil {
		t.Fatalf("PredictIntraLuma16 fallback: %v", err)
	}
	if dst[0] == 0 && dst[1] == 0 && dst[2] == 0 && dst[3] == 0 {
		t.Fatal("expected fallback predictor to write samples")
	}
}

func TestPredictIntraLuma16Z2WalksDownLeftEdge(t *testing.T) {
	top := []uint16{40, 80, 120, 160, 200, 240, 280, 320}
	left := []uint16{800, 840, 880, 920, 960, 1000, 1004, 1008}
	dst := make([]uint16, 8*8)

	if err := PredictIntraLuma16(dst, 8, 8, 8, IntraPredVertRight, 0, top, left, 400, true, true, 10); err != nil {
		t.Fatalf("PredictIntraLuma16: %v", err)
	}

	if got, want := dst[3*8], uint16(825); got != want {
		t.Fatalf("Z2 left-projected sample = %d, want %d", got, want)
	}
}

func TestInverseAddDCTDCT16RectangularMatchesDCPath(t *testing.T) {
	tx := RTX8X16
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	dcOnly := make([]int32, width*height)
	dcOnly[0] = 64

	got := make([]uint16, width*height)
	want := make([]uint16, width*height)
	if err := InverseAddDCTDCT16(got, width, tx, dcOnly, 10); err != nil {
		t.Fatalf("InverseAddDCTDCT16(%d): %v", tx, err)
	}
	if err := InverseAddDCTDCTDC16(want, width, tx, 64, 10); err != nil {
		t.Fatalf("InverseAddDCTDCTDC16(%d): %v", tx, err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}
