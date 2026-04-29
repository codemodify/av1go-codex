package decoder

import "testing"

func TestInverseExactMatrixSupportSplit(t *testing.T) {
	coeffs := make([]int32, 16*16)
	coeffs[0] = 64

	clips := exactClipRanges8()
	scratch := make([]int, len(coeffs))

	if _, _, ok, err := inverseExactMatrix(scratch, TX16X16, TxTypeDCTDCT, coeffs, clips); err != nil {
		t.Fatalf("inverseExactMatrix(DCT_DCT) error = %v", err)
	} else if !ok {
		t.Fatal("inverseExactMatrix(DCT_DCT) did not use exact path")
	}

	if _, _, ok, err := inverseExactMatrix(scratch, TX16X16, TxTypeIdentity, coeffs, clips); err != nil {
		t.Fatalf("inverseExactMatrix(Identity) error = %v", err)
	} else if !ok {
		t.Fatal("inverseExactMatrix(Identity) did not use exact path")
	}

	if _, _, ok, err := inverseExactMatrix(scratch, TX16X16, TxTypeADSTADST, coeffs, clips); err != nil {
		t.Fatalf("inverseExactMatrix(ADST_ADST) error = %v", err)
	} else if !ok {
		t.Fatal("inverseExactMatrix(ADST_ADST) did not use exact path")
	}
}

func TestInverseExactMatrixSupportsLargeDCT64(t *testing.T) {
	for _, tx := range []TxfmSize{TX64X64, RTX16X64, RTX32X64, RTX64X16, RTX64X32} {
		info := TxfmInfoFor(tx)
		coeffs := make([]int32, int(info.W4)*4*int(info.H4)*4)
		coeffs[0] = 64
		coeffs[1] = -16

		scratch := make([]int, len(coeffs))
		if _, _, ok, err := inverseExactMatrix(scratch, tx, TxTypeDCTDCT, coeffs, exactClipRangesHBD(10)); err != nil {
			t.Fatalf("inverseExactMatrix(%d) error = %v", tx, err)
		} else if !ok {
			t.Fatalf("inverseExactMatrix(%d) did not use exact path", tx)
		}
	}
}

func TestExactClipRangesHBD(t *testing.T) {
	clips := exactClipRangesHBD(10)
	if clips.rowMin != -131072 || clips.rowMax != 131071 {
		t.Fatalf("row clip mismatch: %+v", clips)
	}
	if clips.colMin != -32768 || clips.colMax != 32767 {
		t.Fatalf("col clip mismatch: %+v", clips)
	}
}

func TestTxBasisFlipADSTReversesOutputAxis(t *testing.T) {
	adst, err := txBasis(tx1DKindADST, 4)
	if err != nil {
		t.Fatalf("txBasis(ADST): %v", err)
	}
	flip, err := txBasis(tx1DKindFlipADST, 4)
	if err != nil {
		t.Fatalf("txBasis(FlipADST): %v", err)
	}
	for k := 0; k < 4; k++ {
		for x := 0; x < 4; x++ {
			got := flip[k*4+x]
			want := adst[k*4+(3-x)]
			if got != want {
				t.Fatalf("flip basis mismatch at k=%d x=%d: got=%f want=%f", k, x, got, want)
			}
		}
	}
}

func TestTx1DTypesAxisMapping(t *testing.T) {
	tests := []struct {
		txType       TxType
		wantWidth1D  tx1DKind
		wantHeight1D tx1DKind
	}{
		{TxTypeADSTDCT, tx1DKindDCT, tx1DKindADST},
		{TxTypeDCTADST, tx1DKindADST, tx1DKindDCT},
		{TxTypeFlipADSTDCT, tx1DKindDCT, tx1DKindFlipADST},
		{TxTypeDCTFlipADST, tx1DKindFlipADST, tx1DKindDCT},
		{TxTypeADSTFlipADST, tx1DKindFlipADST, tx1DKindADST},
		{TxTypeFlipADSTADST, tx1DKindADST, tx1DKindFlipADST},
		{TxTypeVDCT, tx1DKindIdentity, tx1DKindDCT},
		{TxTypeHDCT, tx1DKindDCT, tx1DKindIdentity},
		{TxTypeVADST, tx1DKindIdentity, tx1DKindADST},
		{TxTypeHADST, tx1DKindADST, tx1DKindIdentity},
		{TxTypeVFlipADST, tx1DKindIdentity, tx1DKindFlipADST},
		{TxTypeHFlipADST, tx1DKindFlipADST, tx1DKindIdentity},
	}
	for _, tt := range tests {
		gotWidth, gotHeight, err := tx1DTypesFor(tt.txType)
		if err != nil {
			t.Fatalf("tx1DTypesFor(%d) error = %v", tt.txType, err)
		}
		if gotWidth != tt.wantWidth1D || gotHeight != tt.wantHeight1D {
			t.Fatalf("tx1DTypesFor(%d) = (%d,%d), want (%d,%d)", tt.txType, gotWidth, gotHeight, tt.wantWidth1D, tt.wantHeight1D)
		}
	}
}
