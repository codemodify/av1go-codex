package decoder

import "testing"

func TestDefaultCoefQCat(t *testing.T) {
	tests := []struct {
		qidx uint8
		want int
	}{
		{0, 0},
		{20, 0},
		{21, 1},
		{60, 1},
		{61, 2},
		{120, 2},
		{121, 3},
		{255, 3},
	}
	for _, tt := range tests {
		if got := DefaultCoefQCat(tt.qidx); got != tt.want {
			t.Fatalf("DefaultCoefQCat(%d) = %d, want %d", tt.qidx, got, tt.want)
		}
	}
}

func TestNewDefaultCoefCDFSeedsExpectedValues(t *testing.T) {
	cdf0 := NewDefaultCoefCDF(0)
	if got, want := cdf0.QCat, 0; got != want {
		t.Fatalf("qcat0 metadata = %d, want %d", got, want)
	}
	if got, want := cdf0.Skip[0][0][0], uint16(32768-31849); got != want {
		t.Fatalf("qcat0 Skip[0][0][0] = %d, want %d", got, want)
	}
	if got, want := cdf0.EOBBin16[0][0][0], uint16(32768-840); got != want {
		t.Fatalf("qcat0 EOBBin16[0][0][0] = %d, want %d", got, want)
	}
	if got, want := cdf0.EOBBaseTok[0][0][0][0], uint16(32768-17837); got != want {
		t.Fatalf("qcat0 EOBBaseTok[0][0][0][0] = %d, want %d", got, want)
	}

	cdf3 := NewDefaultCoefCDF(200)
	if got, want := cdf3.QCat, 3; got != want {
		t.Fatalf("qcat3 metadata = %d, want %d", got, want)
	}
	if got, want := cdf3.DCSign[1][2][0], uint16(32768-17280); got != want {
		t.Fatalf("qcat3 DCSign[1][2][0] = %d, want %d", got, want)
	}
}
