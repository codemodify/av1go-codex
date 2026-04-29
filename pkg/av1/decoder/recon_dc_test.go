package decoder

import "testing"

func TestPredictDCLuma8(t *testing.T) {
	top := []byte{10, 20, 30, 40}
	left := []byte{50, 60, 70, 80}

	tests := []struct {
		name     string
		haveTop  bool
		haveLeft bool
		want     uint8
	}{
		{name: "both", haveTop: true, haveLeft: true, want: 45},
		{name: "top", haveTop: true, haveLeft: false, want: 25},
		{name: "left", haveTop: false, haveLeft: true, want: 65},
		{name: "none", haveTop: false, haveLeft: false, want: 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := make([]byte, 16)
			if err := PredictDCLuma8(dst, 4, 4, 4, top, left, tt.haveTop, tt.haveLeft); err != nil {
				t.Fatalf("PredictDCLuma8() error = %v", err)
			}
			for i, v := range dst {
				if v != tt.want {
					t.Fatalf("dst[%d] = %d, want %d", i, v, tt.want)
				}
			}
		})
	}
}

func TestInverseAddDCTDCTDC8(t *testing.T) {
	tests := []struct {
		name  string
		tx    TxfmSize
		coeff int32
		want  uint8
	}{
		{name: "4x4", tx: TX4X4, coeff: 64, want: 2},
		{name: "8x8", tx: TX8X8, coeff: 64, want: 1},
		{name: "16x16", tx: TX16X16, coeff: 64, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := TxfmInfoFor(tt.tx)
			width := int(info.W4) * 4
			height := int(info.H4) * 4
			dst := make([]byte, width*height)
			if err := InverseAddDCTDCTDC8(dst, width, tt.tx, tt.coeff); err != nil {
				t.Fatalf("InverseAddDCTDCTDC8() error = %v", err)
			}
			for i, v := range dst {
				if v != tt.want {
					t.Fatalf("dst[%d] = %d, want %d", i, v, tt.want)
				}
			}
		})
	}
}

func TestInverseAddDCTDCTDC8Clips(t *testing.T) {
	dst := []byte{255, 254, 1, 0}
	if err := InverseAddDCTDCTDC8(dst, 2, TX4X4, 4096); err == nil {
		t.Fatal("expected buffer size error for short 4x4 destination")
	}

	full := make([]byte, 16)
	for i := range full {
		full[i] = 254
	}
	if err := InverseAddDCTDCTDC8(full, 4, TX4X4, 4096); err != nil {
		t.Fatalf("InverseAddDCTDCTDC8() error = %v", err)
	}
	for i, v := range full {
		if v != 255 {
			t.Fatalf("full[%d] = %d, want 255", i, v)
		}
	}
}
