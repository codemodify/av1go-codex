package decoder

import "testing"

func TestScanOrder4x4MatchesDav1d(t *testing.T) {
	got := ScanOrder(TX4X4)
	want := []uint16{0, 4, 1, 2, 5, 8, 12, 9, 6, 3, 7, 10, 13, 14, 11, 15}
	if len(got) != len(want) {
		t.Fatalf("len(ScanOrder(TX4X4)) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ScanOrder(TX4X4)[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestLastNonzeroColFromEOBMonotonic(t *testing.T) {
	for tx := TxfmSize(0); tx < numRectTxfmSizes; tx++ {
		scan := ScanOrder(tx)
		last := LastNonzeroColFromEOB(tx)
		if len(scan) == 0 {
			t.Fatalf("ScanOrder(%d) is empty", tx)
		}
		if len(last) != len(scan) {
			t.Fatalf("len(LastNonzeroColFromEOB(%d)) = %d, want %d", tx, len(last), len(scan))
		}
		for i := 1; i < len(last); i++ {
			if last[i] < last[i-1] {
				t.Fatalf("LastNonzeroColFromEOB(%d) is not monotonic at %d: %d < %d", tx, i, last[i], last[i-1])
			}
		}
	}
}

func TestLastNonzeroColFromEOB4x4MatchesDav1d(t *testing.T) {
	got := LastNonzeroColFromEOB(TX4X4)
	want := []uint8{0, 0, 1, 2, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 3}
	if len(got) != len(want) {
		t.Fatalf("len(LastNonzeroColFromEOB(TX4X4)) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LastNonzeroColFromEOB(TX4X4)[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestCoeffIndexFromScan64x64MapsIntoTopLeft32x32(t *testing.T) {
	idx, err := CoeffIndexFromScan(TX64X64, 33)
	if err != nil {
		t.Fatalf("CoeffIndexFromScan(TX64X64, 33): %v", err)
	}
	if idx != 65 {
		t.Fatalf("CoeffIndexFromScan(TX64X64, 33) = %d, want 65", idx)
	}
}
