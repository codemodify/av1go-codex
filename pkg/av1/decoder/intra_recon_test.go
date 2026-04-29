package decoder

import "testing"

func TestDirectionalDerivativeUsesAV1Table(t *testing.T) {
	tests := map[int]int{
		3:  1023,
		6:  547,
		14: 273,
		23: 151,
		45: 64,
		67: 27,
		84: 7,
		87: 3,
		90: 0,
	}
	for angle, want := range tests {
		if got := directionalDerivative(angle); got != want {
			t.Fatalf("directionalDerivative(%d) = %d, want %d", angle, got, want)
		}
	}
}

func TestPredictIntraLuma8Paeth(t *testing.T) {
	dst := make([]byte, 4*4)
	top := []byte{10, 20, 30, 40}
	left := []byte{50, 60, 70, 80}
	if err := PredictIntraLuma8(dst, 4, 4, 4, IntraPredPaeth, 0, top, left, 15, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8: %v", err)
	}
	if dst[0] != 50 {
		t.Fatalf("dst[0] = %d, want 50", dst[0])
	}
	if dst[3] != 50 {
		t.Fatalf("dst[3] = %d, want 50", dst[3])
	}
}

func TestPredictIntraLuma8Vertical(t *testing.T) {
	dst := make([]byte, 4*4)
	top := []byte{11, 22, 33, 44}
	if err := PredictIntraLuma8(dst, 4, 4, 4, IntraPredVertical, 0, top, nil, 0, true, false); err != nil {
		t.Fatalf("PredictIntraLuma8: %v", err)
	}
	for y := 0; y < 4; y++ {
		for x, want := range top {
			if got := dst[y*4+x]; got != want {
				t.Fatalf("dst[%d,%d] = %d, want %d", x, y, got, want)
			}
		}
	}
}

func TestPredictIntraLuma8SmoothUsesLastInBlockEdges(t *testing.T) {
	dst := make([]byte, 4*4)
	top := []byte{53, 26, 15, 16, 16, 16, 16, 16}
	left := []byte{163, 127, 65, 35, 23, 16, 15, 15}

	if err := PredictIntraLuma8(dst, 4, 4, 4, IntraPredSmooth, 0, top, left, 81, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8: %v", err)
	}

	want := []byte{
		108, 64, 40, 34,
		86, 55, 38, 34,
		53, 38, 30, 28,
		37, 30, 26, 26,
	}
	if string(dst) != string(want) {
		t.Fatalf("smooth prediction = %v, want %v", dst, want)
	}
}

func TestPredictIntraLuma8Z3ExtendsZeroBottomLeftTail(t *testing.T) {
	dst := make([]byte, 4*4)
	top := []byte{16, 19, 16, 37, 110, 158, 99, 33}
	left := []byte{14, 32, 97, 116, 0, 0, 0, 0}

	if err := PredictIntraLuma8WithFlags(dst, 4, 4, 4, IntraPredHorUp, 0, top, left, 16, true, true, angleUseEdgeFilterFlag); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags: %v", err)
	}

	want := []byte{
		18, 28, 49, 76,
		59, 87, 104, 113,
		109, 114, 117, 117,
		117, 116, 116, 116,
	}
	if string(dst) != string(want) {
		t.Fatalf("Z3 prediction = %v, want %v", dst, want)
	}
}

func TestEffectiveIntraModeUsesVerticalWithoutTop(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredDiagDownLeft, 0, false, true)
	if mode != IntraPredVertical {
		t.Fatalf("mode = %v, want vertical fallback", mode)
	}
	if angle != directionalAngle(IntraPredDiagDownLeft, 0) {
		t.Fatalf("angle = %d, want %d", angle, directionalAngle(IntraPredDiagDownLeft, 0))
	}
}

func TestEffectiveIntraModeUsesHorizontalWithoutLeft(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredHorUp, 0, true, false)
	if mode != IntraPredHorizontal {
		t.Fatalf("mode = %v, want horizontal fallback", mode)
	}
	if angle != directionalAngle(IntraPredHorUp, 0) {
		t.Fatalf("angle = %d, want %d", angle, directionalAngle(IntraPredHorUp, 0))
	}
}

func TestEffectiveIntraModeKeepsSmoothWithoutTop(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredSmooth, 0, false, true)
	if mode != IntraPredSmooth {
		t.Fatalf("mode = %v, want smooth", mode)
	}
	if angle != 0 {
		t.Fatalf("angle = %d, want 0", angle)
	}
}

func TestEffectiveIntraModeKeepsSmoothWithoutLeft(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredSmooth, 0, true, false)
	if mode != IntraPredSmooth {
		t.Fatalf("mode = %v, want smooth", mode)
	}
	if angle != 0 {
		t.Fatalf("angle = %d, want 0", angle)
	}
}

func TestEffectiveIntraModeZ2KeepsZ2WithoutTop(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredDiagDownRight, 0, false, true)
	if mode != intraPredZ2 {
		t.Fatalf("mode = %v, want z2", mode)
	}
	if angle != directionalAngle(IntraPredDiagDownRight, 0) {
		t.Fatalf("angle = %d, want %d", angle, directionalAngle(IntraPredDiagDownRight, 0))
	}
}

func TestEffectiveIntraModeZ2KeepsZ2WithoutLeft(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredDiagDownRight, 0, true, false)
	if mode != intraPredZ2 {
		t.Fatalf("mode = %v, want z2", mode)
	}
	if angle != directionalAngle(IntraPredDiagDownRight, 0) {
		t.Fatalf("angle = %d, want %d", angle, directionalAngle(IntraPredDiagDownRight, 0))
	}
}

func TestEffectiveIntraModeDirectionalKeepsZ2WithoutNeighbors(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredVertRight, 0, false, false)
	if mode != intraPredZ2 {
		t.Fatalf("mode = %v, want z2", mode)
	}
	if angle != directionalAngle(IntraPredVertRight, 0) {
		t.Fatalf("angle = %d, want %d", angle, directionalAngle(IntraPredVertRight, 0))
	}
}

func TestEffectiveIntraModeMapsPaethWithoutTopToHorizontal(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredPaeth, 0, false, true)
	if mode != IntraPredHorizontal {
		t.Fatalf("mode = %v, want horizontal fallback", mode)
	}
	if angle != 0 {
		t.Fatalf("angle = %d, want 0", angle)
	}
}

func TestEffectiveIntraModeMapsPaethWithoutLeftToVertical(t *testing.T) {
	mode, angle := effectiveIntraMode(IntraPredPaeth, 0, true, false)
	if mode != IntraPredVertical {
		t.Fatalf("mode = %v, want vertical fallback", mode)
	}
	if angle != 0 {
		t.Fatalf("angle = %d, want 0", angle)
	}
}

func TestExtendIntraEdgesUsesAV1MissingNeighborFill(t *testing.T) {
	edgeScratch, topExt, topWithTL, leftExt, leftWithTL := extendIntraEdges(4, 4, nil, nil, 0, false, false)
	defer putZeroScratch(&intraEdgeScratchPool, edgeScratch)
	for i, got := range topExt[:4] {
		if got != 127 {
			t.Fatalf("topExt[%d] = %d, want 127", i, got)
		}
	}
	for i, got := range leftExt[:4] {
		if got != 129 {
			t.Fatalf("leftExt[%d] = %d, want 129", i, got)
		}
	}
	if topWithTL[0] != 128 || leftWithTL[0] != 128 {
		t.Fatalf("topLeft = %d/%d, want 128/128", topWithTL[0], leftWithTL[0])
	}
}

func TestPredictIntraLuma8DirectionalHorizontalAngle(t *testing.T) {
	dst := make([]byte, 4*4)
	top := []byte{20, 30, 40, 50}
	left := []byte{60, 70, 80, 90}
	if err := PredictIntraLuma8(dst, 4, 4, 4, IntraPredHorizontal, -1, top, left, 10, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8: %v", err)
	}
	allSame := true
	for i := 1; i < len(dst); i++ {
		if dst[i] != dst[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("expected directional predictor to vary across the block")
	}
}

func TestPredictIntraLuma8WithEdgeFilterFlagsChangesDirectionalOutput(t *testing.T) {
	top := []byte{20, 30, 45, 70, 90, 110, 140, 180}
	left := []byte{25, 40, 60, 85, 100, 120, 150, 190}
	plain := make([]byte, 8*8)
	filtered := make([]byte, 8*8)
	if err := PredictIntraLuma8WithFlags(plain, 8, 8, 8, IntraPredVertLeft, 0, top, left, 12, true, true, 0); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags plain: %v", err)
	}
	if err := PredictIntraLuma8WithFlags(filtered, 8, 8, 8, IntraPredVertLeft, 0, top, left, 12, true, true, angleUseEdgeFilterFlag); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags filtered: %v", err)
	}
	same := true
	for i := range plain {
		if plain[i] != filtered[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("expected edge-filtered directional predictor to differ from plain predictor")
	}
}

func TestUpsampleDirectionalEdge8RangePreservesTopLeftAtLogicalMinusOne(t *testing.T) {
	src := []byte{10, 20, 30, 40}
	got := upsampleDirectionalEdge8Range(src, 4, -1, -1, 3)
	wantEven := []byte{10, 20, 30, 40}
	for i, want := range wantEven {
		if got[i*2] != want {
			t.Fatalf("got[%d] = %d, want %d", i*2, got[i*2], want)
		}
	}
}

func TestPredictIntraLuma8Z2DependsOnTopLeftWhenProjectingAboveTop(t *testing.T) {
	top := []byte{90, 100, 110, 120}
	left := []byte{20, 30, 40, 50}
	a := make([]byte, 16)
	b := make([]byte, 16)

	if err := PredictIntraLuma8(a, 4, 4, 4, IntraPredHorizontal, -1, top, left, 10, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8(a): %v", err)
	}
	if err := PredictIntraLuma8(b, 4, 4, 4, IntraPredHorizontal, -1, top, left, 200, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8(b): %v", err)
	}
	if a[0] == b[0] {
		t.Fatalf("expected top-left sample to affect first Z2 pixel, got %d == %d", a[0], b[0])
	}
}

func TestPredictIntraLuma8Z2UpsampleStartsAtTopLeft(t *testing.T) {
	top := []byte{51, 51, 114, 239}
	left := []byte{51, 51, 51, 51, 51, 51, 51, 51}
	dst := make([]byte, 4*8)

	if err := PredictIntraLuma8WithFlags(dst, 4, 4, 8, IntraPredVertRight, 1, top, left, 51, true, true, angleUseEdgeFilterFlag); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags: %v", err)
	}

	want := []byte{
		51, 47, 76, 182,
		51, 51, 53, 118,
		51, 51, 47, 79,
		51, 51, 51, 54,
		51, 51, 51, 48,
		51, 51, 51, 50,
		51, 51, 51, 51,
		51, 51, 51, 51,
	}
	if string(dst) != string(want) {
		t.Fatalf("Z2 upsample prediction = %v, want %v", dst, want)
	}
}

func TestPredictIntraLuma8Z2WalksDownLeftEdge(t *testing.T) {
	top := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	left := []byte{200, 210, 220, 230, 240, 250, 251, 252}
	dst := make([]byte, 8*8)

	if err := PredictIntraLuma8(dst, 8, 8, 8, IntraPredVertRight, 0, top, left, 100, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8: %v", err)
	}

	if got, want := dst[3*8], byte(206); got != want {
		t.Fatalf("Z2 left-projected sample = %d, want %d", got, want)
	}
}

func TestPredictIntraLuma8Z2SmoothsTopLeftWhenEdgeFilterEnabled(t *testing.T) {
	top := []byte{100, 110, 120, 130}
	left := []byte{20, 30, 40, 50, 60, 70, 80, 90}
	plain := make([]byte, 4*8)
	filtered := make([]byte, 4*8)

	if err := PredictIntraLuma8WithFlags(plain, 4, 4, 8, IntraPredHorizontal, -1, top, left, 60, true, true, 0); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags plain: %v", err)
	}
	if err := PredictIntraLuma8WithFlags(filtered, 4, 4, 8, IntraPredHorizontal, -1, top, left, 60, true, true, angleUseEdgeFilterFlag); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags filtered: %v", err)
	}
	if plain[0] == filtered[0] {
		t.Fatalf("expected edge-filtered Z2 top-left smoothing to change the first pixel, got %d", plain[0])
	}
}

func TestPredictIntraLuma8Z2FilterIgnoresExtendedEdges(t *testing.T) {
	topA := make([]byte, 64)
	topB := make([]byte, 64)
	leftA := make([]byte, 64)
	leftB := make([]byte, 64)
	for i := 0; i < 32; i++ {
		topA[i] = byte(40 + i*3)
		topB[i] = topA[i]
		leftA[i] = byte(220 - i*4)
		leftB[i] = leftA[i]
	}
	for i := 32; i < 64; i++ {
		topA[i] = 0
		topB[i] = 255
		leftA[i] = 255
		leftB[i] = 0
	}

	a := make([]byte, 32*32)
	b := make([]byte, 32*32)
	if err := PredictIntraLuma8WithFlags(a, 32, 32, 32, IntraPredHorDown, 3, topA, leftA, 98, true, true, angleUseEdgeFilterFlag); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags(a): %v", err)
	}
	if err := PredictIntraLuma8WithFlags(b, 32, 32, 32, IntraPredHorDown, 3, topB, leftB, 98, true, true, angleUseEdgeFilterFlag); err != nil {
		t.Fatalf("PredictIntraLuma8WithFlags(b): %v", err)
	}
	if string(a) != string(b) {
		t.Fatal("Z2 edge filtering should not depend on top-right or bottom-left extension samples")
	}
}

func TestPredictIntraLuma8UnsupportedModeFallsBackToDC(t *testing.T) {
	dst := make([]byte, 4)
	top := []byte{10, 20}
	left := []byte{30, 40}
	if err := PredictIntraLuma8(dst, 2, 2, 2, IntraPredMode(255), 5, top, left, 0, true, true); err != nil {
		t.Fatalf("PredictIntraLuma8 fallback: %v", err)
	}
	if dst[0] == 0 && dst[1] == 0 && dst[2] == 0 && dst[3] == 0 {
		t.Fatal("expected fallback predictor to write samples")
	}
}

func TestReconstructIntraLumaBlock8Rectangular(t *testing.T) {
	tx := RTX8X4
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	stride := 16
	plane := make([]byte, stride*12)
	for i := 0; i < width; i++ {
		plane[3*stride+4+i] = byte(15 + 4*i)
	}
	for i := 0; i < height; i++ {
		plane[(4+i)*stride+3] = byte(80 + 6*i)
	}

	want := append([]byte(nil), plane...)
	dst := want[4*stride+4:]
	left := make([]byte, height)
	for i := 0; i < height; i++ {
		left[i] = want[(4+i)*stride+3]
	}
	if err := PredictIntraLuma8(dst, stride, width, height, IntraPredDC, 0, want[3*stride+4:], left, want[3*stride+3], true, true); err != nil {
		t.Fatalf("PredictIntraLuma8: %v", err)
	}
	if err := InverseAddDCTDCTDC8(dst, stride, tx, 64); err != nil {
		t.Fatalf("InverseAddDCTDCTDC8: %v", err)
	}

	if err := ReconstructIntraLumaBlock8(plane, stride, 16, 12, 4, 4, tx, IntraPredDC, 0, 64); err != nil {
		t.Fatalf("ReconstructIntraLumaBlock8(rectangular): %v", err)
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

func TestPrepareIntraLumaTargetEdgesWithNeighborsSuppressesTileBoundarySamples(t *testing.T) {
	plane := []byte{
		10, 20, 30, 40,
		50, 60, 70, 80,
		90, 100, 110, 120,
		130, 140, 150, 160,
	}
	_, _, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, 4, 4, 4, 2, 1, 2, 2, false, false, true, true)
	if err != nil {
		t.Fatalf("prepareIntraLumaTargetEdgesWithNeighbors: %v", err)
	}
	defer writeback.release()
	if haveTop {
		t.Fatal("expected top edge to be suppressed")
	}
	if haveLeft {
		t.Fatal("expected left edge to be suppressed")
	}
	if len(top) != 0 {
		t.Fatalf("top edge len = %d, want 0", len(top))
	}
	if len(left) != 0 {
		t.Fatalf("left edge len = %d, want 0", len(left))
	}
	if topLeft != 128 {
		t.Fatalf("topLeft = %d, want 128 fallback", topLeft)
	}
}
