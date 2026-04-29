package decoder

import "testing"

func TestInverseAddWHTWHT4X48DCOnly(t *testing.T) {
	dst := make([]byte, 16)
	coeffs := make([]int32, 16)
	coeffs[0] = 16
	if err := InverseAddWHTWHT4X48(dst, 4, coeffs); err != nil {
		t.Fatalf("InverseAddWHTWHT4X48: %v", err)
	}
	for i, v := range dst {
		if v != 1 {
			t.Fatalf("dst[%d] = %d, want 1", i, v)
		}
	}
}
