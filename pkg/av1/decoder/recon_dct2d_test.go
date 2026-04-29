package decoder

import "testing"

func TestInverseAddDCTDCT8MatchesDCPath(t *testing.T) {
	for _, tx := range []TxfmSize{TX4X4, TX8X8, TX16X16, TX64X64} {
		info := TxfmInfoFor(tx)
		n := int(info.W4) * 4
		dcOnly := make([]int32, n*n)
		dcOnly[0] = 64

		got := make([]byte, n*n)
		want := make([]byte, n*n)
		if err := InverseAddDCTDCT8(got, n, tx, dcOnly); err != nil {
			t.Fatalf("InverseAddDCTDCT8(%d): %v", tx, err)
		}
		if err := InverseAddDCTDCTDC8(want, n, tx, 64); err != nil {
			t.Fatalf("InverseAddDCTDCTDC8(%d): %v", tx, err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("tx %d sample %d = %d, want %d", tx, i, got[i], want[i])
			}
		}
	}
}

func TestInverseAddDCTDCT8WithACProducesVariation(t *testing.T) {
	coeffs := make([]int32, 16)
	coeffs[0] = 64
	acIdx, err := CoeffIndexFromScan(TX4X4, uint32(ScanOrder(TX4X4)[1]))
	if err != nil {
		t.Fatalf("CoeffIndexFromScan: %v", err)
	}
	coeffs[acIdx] = 32
	dst := make([]byte, 16)
	if err := InverseAddDCTDCT8(dst, 4, TX4X4, coeffs); err != nil {
		t.Fatalf("InverseAddDCTDCT8: %v", err)
	}
	allSame := true
	for i := 1; i < len(dst); i++ {
		if dst[i] != dst[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("expected AC coefficient to produce non-uniform output")
	}
}

func TestInverseAddDCTDCT8RectangularWithACProducesVariation(t *testing.T) {
	info := TxfmInfoFor(RTX8X32)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	coeffs := make([]int32, width*height)
	coeffs[0] = 64
	acIdx, err := CoeffIndexFromScan(RTX8X32, uint32(ScanOrder(RTX8X32)[10]))
	if err != nil {
		t.Fatalf("CoeffIndexFromScan: %v", err)
	}
	coeffs[acIdx] = 4096
	dst := make([]byte, width*height)
	if err := InverseAddDCTDCT8(dst, width, RTX8X32, coeffs); err != nil {
		t.Fatalf("InverseAddDCTDCT8: %v", err)
	}
	allSame := true
	for i := 1; i < len(dst); i++ {
		if dst[i] != dst[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("expected AC coefficient to produce non-uniform output")
	}
}

func TestInverseAddDCTDCT8RectangularMatchesDCPath(t *testing.T) {
	tx := RTX8X32
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	dcOnly := make([]int32, width*height)
	dcOnly[0] = 64

	got := make([]byte, width*height)
	want := make([]byte, width*height)
	if err := InverseAddDCTDCT8(got, width, tx, dcOnly); err != nil {
		t.Fatalf("InverseAddDCTDCT8(%d): %v", tx, err)
	}
	if err := InverseAddDCTDCTDC8(want, width, tx, 64); err != nil {
		t.Fatalf("InverseAddDCTDCTDC8(%d): %v", tx, err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestReconstructIntraLumaCoefficients8LosslessWHTDoesNotFlattenDetail(t *testing.T) {
	const stride = 12
	plane := make([]byte, stride*12)
	plane[3*stride+3] = 1
	for i, v := range []byte{0, 0, 0, 0, 50, 80, 86, 83} {
		plane[3*stride+4+i] = v
	}
	for i := 0; i < 8; i++ {
		plane[(4+i)*stride+3] = 1
	}
	coeffs := []int32{
		960, -372, -120, -148,
		-252, -76, 168, -48,
		-148, 120, 0, 68,
		-128, -32, 72, -16,
	}

	if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
		plane,
		stride,
		stride,
		12,
		4,
		4,
		TX4X4,
		TxTypeWHTWHT,
		IntraPredDiagDownLeft,
		0,
		coeffs,
		true,
		angleUseEdgeFilterFlag,
		true,
		true,
		true,
		true,
	); err != nil {
		t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges: %v", err)
	}

	want := [][]byte{
		{3, 6, 33, 116},
		{11, 45, 131, 189},
		{24, 124, 187, 189},
		{92, 169, 191, 145},
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got := plane[(4+y)*stride+4+x]; got != want[y][x] {
				t.Fatalf("sample (%d,%d) = %d, want %d", x, y, got, want[y][x])
			}
		}
	}
}

func TestReconstructIntraLumaCoefficients8LosslessWHTAppliesBitmovinDetail(t *testing.T) {
	const stride = 16
	const x = 4
	const y = 4
	plane := make([]byte, stride*10)
	plane[(y-1)*stride+x-1] = 17
	copy(plane[(y-1)*stride+x:], []byte{17, 17, 17, 17, 17, 17, 9, 212})
	for row, v := range []byte{17, 17, 17, 17} {
		plane[(y+row)*stride+x-1] = v
	}
	coeffs := []int32{
		1416, -180, -72, -180,
		-1456, 104, 164, 104,
		260, 52, -88, 52,
		-236, 36, -24, 36,
	}

	if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
		plane,
		stride,
		stride,
		10,
		x,
		y,
		TX4X4,
		TxTypeWHTWHT,
		IntraPredVertLeft,
		0,
		coeffs,
		true,
		angleUseEdgeFilterFlag,
		true,
		true,
		true,
		true,
	); err != nil {
		t.Fatalf("ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges: %v", err)
	}

	want := [][]byte{
		{17, 17, 17, 13},
		{5, 0, 0, 46},
		{117, 176, 176, 193},
		{175, 246, 246, 243},
	}
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			if got := plane[(y+row)*stride+x+col]; got != want[row][col] {
				t.Fatalf("sample (%d,%d) = %d, want %d", col, row, got, want[row][col])
			}
		}
	}
}
