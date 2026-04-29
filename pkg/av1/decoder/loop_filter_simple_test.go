package decoder

import "testing"

func TestWeakFilterPair8SmoothsBoundary(t *testing.T) {
	buf := []byte{80, 82, 84, 120, 122, 124}
	weakFilterPair8(buf, 1, 2, 3, 4, 12)
	if buf[2] >= 120 {
		t.Fatalf("left edge sample was not filtered: %v", buf)
	}
	if buf[3] <= 84 {
		t.Fatalf("right edge sample was not filtered: %v", buf)
	}
}

func TestLoopFilterEdge8WidePreservesFlatEdge(t *testing.T) {
	buf := make([]byte, 14)
	for i := range buf {
		buf[i] = 214
	}
	loopFilterEdge8(buf, 6, 1, calcLoopFilterParams(32, 0, 8), 16)
	for i, got := range buf {
		if got != 214 {
			t.Fatalf("sample %d=%d, want flat edge preserved", i, got)
		}
	}
}

func TestShouldFilterVerticalUsesNoSkipMask(t *testing.T) {
	mask := NewCDEFIndexMap(32, 32)
	if shouldFilterVertical(mask, nil, 8, 0, 0, 0) {
		t.Fatalf("unexpected filter without noskip blocks")
	}
	mask.markNoSkip(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2}, false)
	if !shouldFilterVertical(mask, nil, 8, 0, 0, 0) {
		t.Fatalf("expected filter across coded block boundary")
	}
}

func TestShouldFilterHorizontalUsesNoSkipMask(t *testing.T) {
	mask := NewCDEFIndexMap(32, 32)
	if shouldFilterHorizontal(mask, nil, 0, 8, 0, 0) {
		t.Fatalf("unexpected filter without noskip blocks")
	}
	mask.markNoSkip(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2}, false)
	if !shouldFilterHorizontal(mask, nil, 0, 8, 0, 0) {
		t.Fatalf("expected filter across coded block boundary")
	}
}

func TestCalcLoopFilterParamsSharpnessReducesThresholds(t *testing.T) {
	soft := calcLoopFilterParams(32, 0, 8)
	sharp := calcLoopFilterParams(32, 7, 8)
	if sharp.I >= soft.I {
		t.Fatalf("sharp inner threshold=%d, want less than %d", sharp.I, soft.I)
	}
	if sharp.E >= soft.E {
		t.Fatalf("sharp edge threshold=%d, want less than %d", sharp.E, soft.E)
	}
}

func TestChooseLoopFilterWidthPrefersChroma6AndLuma8(t *testing.T) {
	if got := chooseLoopFilterWidth(true, 24, 4, 4); got != 6 {
		t.Fatalf("chroma width=%d, want 6", got)
	}
	if got := chooseLoopFilterWidth(false, 24, 5, 5); got != 8 {
		t.Fatalf("luma width=%d, want 8", got)
	}
}
