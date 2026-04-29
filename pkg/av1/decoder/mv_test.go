package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestNewDefaultMVCDFSeedsExpectedValues(t *testing.T) {
	cdf := NewDefaultMVCDF()
	if got, want := cdf.Joint[0], uint16(32768-4096); got != want {
		t.Fatalf("Joint[0] = %d, want %d", got, want)
	}
	if got, want := cdf.Comp[0].Classes[0], uint16(32768-28672); got != want {
		t.Fatalf("Comp[0].Classes[0] = %d, want %d", got, want)
	}
	if got, want := cdf.Comp[1].Class0FP[1][0], uint16(32768-12288); got != want {
		t.Fatalf("Comp[1].Class0FP[1][0] = %d, want %d", got, want)
	}
}

func TestReadMVComponentDiffIntegerPrecision(t *testing.T) {
	cdf := NewDefaultMVCDF()
	dec := &scriptedIntraEntropy{
		adapt:   []uint32{0, 0},
		symbols: []uint32{0},
	}

	diff, err := ReadMVComponentDiff(&cdf.Comp[0], -1, dec)
	if err != nil {
		t.Fatalf("ReadMVComponentDiff: %v", err)
	}
	if diff != 8 {
		t.Fatalf("diff = %d, want 8", diff)
	}
}

func TestReadMVComponentDiffLowPrecisionConsumesFractionalPart(t *testing.T) {
	cdf := NewDefaultMVCDF()
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // sign
			1, // class0 high bit
		},
		symbols: []uint32{
			0, // class0
			2, // fractional part
		},
	}

	diff, err := ReadMVComponentDiff(&cdf.Comp[0], 0, dec)
	if err != nil {
		t.Fatalf("ReadMVComponentDiff: %v", err)
	}
	if diff != 14 {
		t.Fatalf("diff = %d, want 14", diff)
	}
}

func TestReadMVComponentDiffHighPrecisionNegative(t *testing.T) {
	cdf := NewDefaultMVCDF()
	dec := &scriptedIntraEntropy{
		adapt:   []uint32{1, 1, 1},
		symbols: []uint32{0, 2},
	}

	diff, err := ReadMVComponentDiff(&cdf.Comp[0], 1, dec)
	if err != nil {
		t.Fatalf("ReadMVComponentDiff: %v", err)
	}
	if diff != -14 {
		t.Fatalf("diff = %d, want -14", diff)
	}
}

func TestMVResidualPrecisionDistinguishesLowPrecisionFromForceInteger(t *testing.T) {
	if got := mvResidualPrecision(&obu.FrameHeader{ForceIntegerMV: true}); got != -1 {
		t.Fatalf("force-integer mv precision=%d, want -1", got)
	}
	if got := mvResidualPrecision(&obu.FrameHeader{HighPrecisionMV: false}); got != 0 {
		t.Fatalf("low mv precision=%d, want 0", got)
	}
	if got := mvResidualPrecision(&obu.FrameHeader{HighPrecisionMV: true}); got != 1 {
		t.Fatalf("high mv precision=%d, want 1", got)
	}
}

func TestReadMVResidualHV(t *testing.T) {
	cdf := NewDefaultMVCDF()
	dec := &scriptedIntraEntropy{
		symbols: []uint32{
			uint32(MVJointHV), // joint
			0,                 // y class
			0,                 // x class
		},
		adapt: []uint32{
			0, 0, // y sign, y class0
			1, 0, // x sign, x class0
		},
	}

	mv, err := ReadMVResidual(cdf, MotionVector{Y: 3, X: -2}, -1, dec)
	if err != nil {
		t.Fatalf("ReadMVResidual: %v", err)
	}
	if mv.Y != 11 || mv.X != -10 {
		t.Fatalf("mv = {%d %d}, want {11 -10}", mv.Y, mv.X)
	}
}

func TestFixMVPrecisionLowPrecisionRoundsTowardZeroToEven(t *testing.T) {
	hdr := &obu.FrameHeader{HighPrecisionMV: false}
	mv := fixMVPrecision(hdr, MotionVector{Y: -3, X: 5})
	if mv != (MotionVector{Y: -2, X: 4}) {
		t.Fatalf("mv=%+v, want {Y:-2 X:4}", mv)
	}
}

func TestFixMVPrecisionForceIntegerRoundsToEighths(t *testing.T) {
	hdr := &obu.FrameHeader{ForceIntegerMV: true}
	mv := fixMVPrecision(hdr, MotionVector{Y: -5, X: 5})
	if mv != (MotionVector{Y: -8, X: 8}) {
		t.Fatalf("mv=%+v, want {Y:-8 X:8}", mv)
	}
}

func TestActiveRefMVCandidateCountIgnoresPadding(t *testing.T) {
	cands := []RefMVCandidate{
		{MV: MotionVector{X: 4}, Weight: 12},
		{MV: MotionVector{}, Weight: 0},
	}
	if got := activeRefMVCandidateCount(cands); got != 1 {
		t.Fatalf("activeRefMVCandidateCount=%d, want 1", got)
	}
}

func TestNormalizeSingleRefMVUsesDrlSpecificPrecisionRules(t *testing.T) {
	hdr := &obu.FrameHeader{HighPrecisionMV: false}
	mv := MotionVector{Y: -3, X: 5}
	if got := normalizeSingleRefMV(hdr, InterPredNearest, drlNearest, 4, mv, MotionVector{Y: 12, X: 10}); got != (MotionVector{Y: -2, X: 4}) {
		t.Fatalf("nearest mv=%+v, want {Y:-2 X:4}", got)
	}
	if got := normalizeSingleRefMV(hdr, InterPredNear, drlNearer, 4, mv, MotionVector{Y: 12, X: 10}); got != (MotionVector{Y: -2, X: 4}) {
		t.Fatalf("nearer mv=%+v, want {Y:-2 X:4}", got)
	}
	if got := normalizeSingleRefMV(hdr, InterPredNear, drlNear, 4, mv, MotionVector{Y: 12, X: 10}); got != mv {
		t.Fatalf("near mv=%+v, want %+v", got, mv)
	}
	if got := normalizeSingleRefMV(hdr, InterPredNew, drlNearest, 4, mv, MotionVector{Y: 12, X: 10}); got != mv {
		t.Fatalf("newmv mv=%+v, want %+v", got, mv)
	}
	if got := normalizeSingleRefMV(hdr, InterPredNew, drlNearest, 1, mv, MotionVector{Y: 12, X: 10}); got != (MotionVector{Y: -2, X: 4}) {
		t.Fatalf("newmv fallback mv=%+v, want {Y:-2 X:4}", got)
	}
}

func TestNormalizeSingleRefMVUsesGlobalMotionForGlobalMode(t *testing.T) {
	hdr := &obu.FrameHeader{HighPrecisionMV: false}
	got := normalizeSingleRefMV(hdr, InterPredGlobal, drlNearest, 4, MotionVector{Y: -3, X: 5}, MotionVector{Y: 12, X: 10})
	if got != (MotionVector{Y: 12, X: 10}) {
		t.Fatalf("mv=%+v, want {Y:12 X:10}", got)
	}
}

func TestNormalizeCompoundBaseMVPrecisionRules(t *testing.T) {
	hdr := &obu.FrameHeader{HighPrecisionMV: false}
	mv := MotionVector{Y: -3, X: 5}
	if got := normalizeCompoundBaseMV(hdr, InterPredNearest, mv, MotionVector{Y: 12, X: 10}); got != (MotionVector{Y: -2, X: 4}) {
		t.Fatalf("nearest mv=%+v, want {Y:-2 X:4}", got)
	}
	if got := normalizeCompoundBaseMV(hdr, InterPredNear, mv, MotionVector{Y: 12, X: 10}); got != (MotionVector{Y: -2, X: 4}) {
		t.Fatalf("near mv=%+v, want {Y:-2 X:4}", got)
	}
	if got := normalizeCompoundBaseMV(hdr, InterPredNew, mv, MotionVector{Y: 12, X: 10}); got != mv {
		t.Fatalf("new mv=%+v, want %+v", got, mv)
	}
}

func TestMVResidualPrecisionMatchesCurrentDecoderPolicy(t *testing.T) {
	if got := mvResidualPrecision(&obu.FrameHeader{}); got != 0 {
		t.Fatalf("default mv prec=%d, want 0", got)
	}
	if got := mvResidualPrecision(&obu.FrameHeader{HighPrecisionMV: true}); got != 1 {
		t.Fatalf("high precision mv prec=%d, want 1", got)
	}
	if got := mvResidualPrecision(&obu.FrameHeader{ForceIntegerMV: true, HighPrecisionMV: true}); got != -1 {
		t.Fatalf("force integer mv prec=%d, want -1", got)
	}
}

func TestApplyTemporalGlobalMVContextUsesTemporalDistance(t *testing.T) {
	hdr := &obu.FrameHeader{UseRefFrameMvs: true, Width: 32, Height: 32}
	hdr.GMV[0] = obu.WarpedMotionParams{
		Type: 2,
		Matrix: [6]int32{
			1 << 16, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}
	g := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	mvCtx := (3 << 4) | (1 << 3) | 2
	got := applyTemporalGlobalMVContext(mvCtx, []RefMVCandidate{{MV: MotionVector{X: 8, Y: 0}}}, hdr, 0, g)
	if got&(1<<3) != 0 {
		t.Fatalf("globalmv bit = 1, want 0 when temporal mv matches gmv")
	}
	got = applyTemporalGlobalMVContext(mvCtx, []RefMVCandidate{{MV: MotionVector{X: 40, Y: 0}}}, hdr, 0, g)
	if got&(1<<3) == 0 {
		t.Fatalf("globalmv bit = 0, want 1 when temporal mv differs from gmv")
	}
}

func TestTemporalGlobalMVContextUsesTopLeftProjectedBlockOnly(t *testing.T) {
	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 2
	parsed.Header.UseRefFrameMvs = true
	parsed.RefFramePOC[0] = 1
	hdr := &parsed.Header
	g := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 6, End4Y: 6, Size: Block16x16}
	m := newTemporalMVMap(64, 64)

	m.setBlock8((g.Start4X>>1)+1, g.Start4Y>>1, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	if _, ok := m.projectedGlobalMVContext(seq, parsed, 0, hdr, g); ok {
		t.Fatal("globalmv context updated from non-top-left temporal block")
	}

	m.setBlock8(g.Start4X>>1, g.Start4Y>>1, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	got, ok := m.projectedGlobalMVContext(seq, parsed, 0, hdr, g)
	if !ok {
		t.Fatal("globalmv context not updated from top-left temporal block")
	}
	if got != 0 {
		t.Fatalf("globalmv context=%d, want 0 for temporal mv matching identity global mv", got)
	}
}
