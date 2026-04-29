package decoder

import "testing"

func TestCDEFEnsureBlockIndexDoesNotReadWhenTopLeftCellAlreadySet(t *testing.T) {
	m := NewCDEFIndexMap(128, 64)
	m.set(0, 0, 2)
	dec := &scriptedIntraEntropy{bits: []uint32{3}}

	if err := m.ensureBlockIndex(BlockGeometry{
		Size:    Block64x64,
		Start4X: 8,
		Start4Y: 0,
		End4X:   24,
		End4Y:   16,
	}, 2, false, dec); err != nil {
		t.Fatalf("ensureBlockIndex: %v", err)
	}
	if len(dec.bits) != 1 {
		t.Fatalf("DecodeBools was consumed for an already-set top-left cell")
	}
	if got := m.at(1, 0); got != -1 {
		t.Fatalf("neighboring CDEF cell = %d, want unset until its own top-left read", got)
	}
}

func TestCDEFEnsureBlockIndexSetsSpecBlockFootprintFromTopLeftCell(t *testing.T) {
	m := NewCDEFIndexMap(128, 128)
	dec := &scriptedIntraEntropy{bits: []uint32{1}}

	if err := m.ensureBlockIndex(BlockGeometry{
		Size:    Block128x128,
		Start4X: 0,
		Start4Y: 0,
		End4X:   32,
		End4Y:   32,
	}, 2, false, dec); err != nil {
		t.Fatalf("ensureBlockIndex: %v", err)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if got := m.at(x, y); got != 1 {
				t.Fatalf("CDEF cell (%d,%d) = %d, want 1", x, y, got)
			}
		}
	}
	if len(dec.bits) != 0 {
		t.Fatalf("DecodeBools was not consumed for unset top-left cell")
	}
}
