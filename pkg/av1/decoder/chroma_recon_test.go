package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestPredictCFL8AppliesLumaDerivedDelta(t *testing.T) {
	luma := []byte{
		10, 20, 30, 40,
		50, 60, 70, 80,
		90, 100, 110, 120,
		130, 140, 150, 160,
	}
	dst := make([]byte, 4)
	top := []byte{128, 128}
	left := []byte{128, 128}
	if err := predictCFL8(dst, 2, 2, 2, top, left, true, true, 8, luma, 4, 4, 4, 0, 0, av1.Chroma420); err != nil {
		t.Fatalf("predictCFL8: %v", err)
	}
	if dst[0] == 128 && dst[1] == 128 && dst[2] == 128 && dst[3] == 128 {
		t.Fatal("expected CFL to modify the DC predictor")
	}
}

func TestPredictCFL16AppliesLumaDerivedDelta(t *testing.T) {
	luma := []uint16{
		100, 200, 300, 400,
		500, 600, 700, 800,
		900, 1000, 900, 800,
		700, 600, 500, 400,
	}
	dst := make([]uint16, 4)
	top := []uint16{512, 512}
	left := []uint16{512, 512}
	if err := predictCFL16(dst, 2, 2, 2, top, left, true, true, -12, luma, 4, 4, 4, 0, 0, 10, av1.Chroma420); err != nil {
		t.Fatalf("predictCFL16: %v", err)
	}
	if dst[0] == 512 && dst[1] == 512 && dst[2] == 512 && dst[3] == 512 {
		t.Fatal("expected high-bit-depth CFL to modify the DC predictor")
	}
}

func TestPredictCFL8Supports422(t *testing.T) {
	luma := []byte{
		10, 30, 50, 70,
		20, 40, 60, 80,
	}
	dst := make([]byte, 4)
	top := []byte{100, 100}
	left := []byte{100, 100}
	if err := predictCFL8(dst, 2, 2, 2, top, left, true, true, 8, luma, 4, 4, 2, 0, 0, av1.Chroma422); err != nil {
		t.Fatalf("predictCFL8(422): %v", err)
	}
	if dst[0] == 100 && dst[1] == 100 && dst[2] == 100 && dst[3] == 100 {
		t.Fatal("expected 4:2:2 CFL to modify the DC predictor")
	}
}

func TestPredictCFL8Supports444(t *testing.T) {
	luma := []byte{
		10, 40,
		90, 140,
	}
	dst := make([]byte, 4)
	top := []byte{110, 110}
	left := []byte{110, 110}
	if err := predictCFL8(dst, 2, 2, 2, top, left, true, true, -8, luma, 2, 2, 2, 0, 0, av1.Chroma444); err != nil {
		t.Fatalf("predictCFL8(444): %v", err)
	}
	if dst[0] == 110 && dst[1] == 110 && dst[2] == 110 && dst[3] == 110 {
		t.Fatal("expected 4:4:4 CFL to modify the DC predictor")
	}
}

func TestChromaSampleRectForBlock420Sub8x8(t *testing.T) {
	x, y, w, h := chromaSampleRectForBlock(av1.Chroma420, BlockGeometry{
		Start4X: 1,
		Start4Y: 2,
		End4X:   2,
		End4Y:   4,
	})
	if x != 2 || y != 4 || w != 2 || h != 4 {
		t.Fatalf("rect = (%d,%d %dx%d), want (2,4 2x4)", x, y, w, h)
	}
}

func TestChromaSampleRectForBlock422OddOrigin(t *testing.T) {
	x, y, w, h := chromaSampleRectForBlock(av1.Chroma422, BlockGeometry{
		Start4X: 1,
		Start4Y: 3,
		End4X:   2,
		End4Y:   5,
	})
	if x != 2 || y != 12 || w != 2 || h != 8 {
		t.Fatalf("rect = (%d,%d %dx%d), want (2,12 2x8)", x, y, w, h)
	}
}

func TestDebugChromaSkipPolicyForBlock(t *testing.T) {
	t.Setenv("DEBUG_CHROMA_SKIP_TARGET", "16,32")
	t.Setenv("DEBUG_CHROMA_SKIP_POLICY", "entry_prob")

	got := debugChromaSkipPolicyForBlock(BlockGeometry{
		Start4X: 4,
		Start4Y: 8,
		End4X:   8,
		End4Y:   12,
	}, 2)
	if got != debugChromaSkipPolicyResetEntryProb {
		t.Fatalf("policy = %d, want %d", got, debugChromaSkipPolicyResetEntryProb)
	}
	if got := debugChromaSkipPolicyForBlock(BlockGeometry{
		Start4X: 4,
		Start4Y: 8,
		End4X:   8,
		End4Y:   12,
	}, 1); got != debugChromaSkipPolicyNone {
		t.Fatalf("plane 1 policy = %d, want none", got)
	}
	t.Setenv("DEBUG_CHROMA_SKIP_PLANES", "1,2")
	if got := debugChromaSkipPolicyForBlock(BlockGeometry{
		Start4X: 4,
		Start4Y: 8,
		End4X:   8,
		End4Y:   12,
	}, 1); got != debugChromaSkipPolicyResetEntryProb {
		t.Fatalf("plane 1 opted-in policy = %d, want %d", got, debugChromaSkipPolicyResetEntryProb)
	}
	if got := debugChromaSkipPolicyForBlock(BlockGeometry{
		Start4X: 0,
		Start4Y: 8,
		End4X:   4,
		End4Y:   12,
	}, 2); got != debugChromaSkipPolicyNone {
		t.Fatalf("non-target policy = %d, want none", got)
	}
}

func TestDebugSwapChromaPlaneOrderForBlock(t *testing.T) {
	t.Setenv("DEBUG_SWAP_CHROMA_PLANE_ORDER_TARGET", "32,0")
	if !debugSwapChromaPlaneOrderForBlock(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}) {
		t.Fatal("expected targeted swap to be enabled")
	}
	if debugSwapChromaPlaneOrderForBlock(BlockGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   8,
	}) {
		t.Fatal("unexpected swap on non-target block")
	}
}

func TestDebugResetChromaCoefBeforePlane2ForBlock(t *testing.T) {
	t.Setenv("DEBUG_RESET_CHROMA_COEF_BEFORE_PLANE2_TARGET", "0,0")
	if !debugResetChromaCoefBeforePlane2ForBlock(BlockGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   8,
	}) {
		t.Fatal("expected targeted reset to be enabled")
	}
	if debugResetChromaCoefBeforePlane2ForBlock(BlockGeometry{
		Start4X: 8,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}) {
		t.Fatal("unexpected reset on non-target block")
	}
}

func TestDebugApplyChromaSkipPolicyEntryProbAndCount(t *testing.T) {
	baseline := NewDefaultCoefCDFForQCat(0)
	current := baseline.Clone()
	current.Skip[1][7][0] = baseline.Skip[1][7][0] + 11
	current.Skip[1][7][1] = baseline.Skip[1][7][1] + 3

	debugApplyChromaSkipPolicy(debugChromaSkipPolicyResetEntryProb, current, baseline, 1, 7)
	if got, want := current.Skip[1][7][0], baseline.Skip[1][7][0]; got != want {
		t.Fatalf("entry prob = %d, want %d", got, want)
	}
	if got, want := current.Skip[1][7][1], baseline.Skip[1][7][1]+3; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}

	debugApplyChromaSkipPolicy(debugChromaSkipPolicyResetEntryCount, current, baseline, 1, 7)
	if got, want := current.Skip[1][7][1], baseline.Skip[1][7][1]; got != want {
		t.Fatalf("entry count reset = %d, want %d", got, want)
	}
}

func TestDebugApplyChromaSkipPolicyAllProb(t *testing.T) {
	baseline := NewDefaultCoefCDFForQCat(0)
	current := baseline.Clone()
	current.Skip[0][0][0]++
	current.Skip[2][9][0] += 5
	current.Skip[2][9][1] += 2

	debugApplyChromaSkipPolicy(debugChromaSkipPolicyResetAllProb, current, baseline, 2, 9)
	if got, want := current.Skip[0][0][0], baseline.Skip[0][0][0]; got != want {
		t.Fatalf("Skip[0][0] prob = %d, want %d", got, want)
	}
	if got, want := current.Skip[2][9][0], baseline.Skip[2][9][0]; got != want {
		t.Fatalf("Skip[2][9] prob = %d, want %d", got, want)
	}
	if got, want := current.Skip[2][9][1], baseline.Skip[2][9][1]+2; got != want {
		t.Fatalf("Skip[2][9] count = %d, want %d", got, want)
	}
}

func TestDebugApplyChromaSkipPolicyCopySourceProb(t *testing.T) {
	t.Setenv("DEBUG_CHROMA_SKIP_SOURCE", "1,8")
	current := NewDefaultCoefCDFForQCat(0)
	current.Skip[1][7][0] = 30000
	current.Skip[1][7][1] = 9
	current.Skip[1][8][0] = 12345
	current.Skip[1][8][1] = 4

	debugApplyChromaSkipPolicy(debugChromaSkipPolicyCopySourceProb, current, nil, 1, 7)
	if got, want := current.Skip[1][7][0], uint16(12345); got != want {
		t.Fatalf("entry prob = %d, want %d", got, want)
	}
	if got, want := current.Skip[1][7][1], uint16(9); got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
}

func TestDebugApplyChromaSkipPolicyCopyQCatProb(t *testing.T) {
	t.Setenv("DEBUG_CHROMA_SKIP_SOURCE_QCAT", "1")
	current := NewDefaultCoefCDFForQCat(3)
	want := NewDefaultCoefCDFForQCat(1)
	current.Skip[1][7][0] = 9999

	debugApplyChromaSkipPolicy(debugChromaSkipPolicyCopyQCatProb, current, nil, 1, 7)
	if got, wantProb := current.Skip[1][7][0], want.Skip[1][7][0]; got != wantProb {
		t.Fatalf("entry prob = %d, want %d", got, wantProb)
	}
}
