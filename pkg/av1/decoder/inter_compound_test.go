package decoder

import (
	"math"
	"reflect"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedCompoundEntropy struct {
	bools []uint32
	syms  []uint32
	equis []uint32
}

func (s *scriptedCompoundEntropy) DecodeBoolAdapt([]uint16) uint32 {
	if len(s.bools) == 0 {
		return 0
	}
	v := s.bools[0]
	s.bools = s.bools[1:]
	return v
}

func (s *scriptedCompoundEntropy) DecodeBool(uint32) uint32 {
	return s.DecodeBoolEqui()
}

func (s *scriptedCompoundEntropy) DecodeUniform(uint32) int {
	if len(s.syms) == 0 {
		return 0
	}
	v := int(s.syms[0])
	s.syms = s.syms[1:]
	return v
}

func (s *scriptedCompoundEntropy) DecodeBools(uint) uint32 {
	return s.DecodeBoolEqui()
}

func (s *scriptedCompoundEntropy) DecodeSymbolAdapt([]uint16, int) uint32 {
	if len(s.syms) == 0 {
		return 0
	}
	v := s.syms[0]
	s.syms = s.syms[1:]
	return v
}

func (s *scriptedCompoundEntropy) DecodeBoolEqui() uint32 {
	if len(s.equis) == 0 {
		return 0
	}
	v := s.equis[0]
	s.equis = s.equis[1:]
	return v
}

func TestGeneratedCompoundInterPredModesMatchAV1SymbolOrder(t *testing.T) {
	want := [8][2]InterPredMode{
		{InterPredNearest, InterPredNearest},
		{InterPredNear, InterPredNear},
		{InterPredNearest, InterPredNew},
		{InterPredNew, InterPredNearest},
		{InterPredNear, InterPredNew},
		{InterPredNew, InterPredNear},
		{InterPredGlobal, InterPredGlobal},
		{InterPredNew, InterPredNew},
	}
	if generatedCompInterPredModes != want {
		t.Fatalf("generatedCompInterPredModes=%v, want AV1 order %v", generatedCompInterPredModes, want)
	}
}

func TestPredictCompoundInterLumaBlock8WithFiltersAveragesRefs(t *testing.T) {
	ref0 := make([]byte, 8*8)
	ref1 := make([]byte, 8*8)
	for i := range ref0 {
		ref0[i] = 32
		ref1[i] = 96
	}
	dst := make([]byte, 8*8)
	if err := PredictCompoundInterLumaBlock8WithFilters(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair()); err != nil {
		t.Fatalf("PredictCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for i, v := range dst {
		if v != 64 {
			t.Fatalf("dst[%d]=%d, want 64", i, v)
		}
	}
}

func TestPredictCompoundInterLumaBlock16WithFiltersAveragesRefs(t *testing.T) {
	ref0 := make([]uint16, 8*8)
	ref1 := make([]uint16, 8*8)
	for i := range ref0 {
		ref0[i] = 256
		ref1[i] = 512
	}
	dst := make([]uint16, 8*8)
	if err := PredictCompoundInterLumaBlock16WithFilters(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), 10); err != nil {
		t.Fatalf("PredictCompoundInterLumaBlock16WithFilters: %v", err)
	}
	for i, v := range dst {
		if v != 384 {
			t.Fatalf("dst[%d]=%d, want 384", i, v)
		}
	}
}

func TestPredictWeightedCompoundInterLumaBlock8WithFiltersUsesWeight(t *testing.T) {
	ref0 := make([]byte, 8*8)
	ref1 := make([]byte, 8*8)
	for i := range ref0 {
		ref0[i] = 32
		ref1[i] = 96
	}
	dst := make([]byte, 8*8)
	if err := PredictWeightedCompoundInterLumaBlock8WithFilters(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), 12); err != nil {
		t.Fatalf("PredictWeightedCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for i, v := range dst {
		if v != 48 {
			t.Fatalf("dst[%d]=%d, want 48", i, v)
		}
	}
}

func TestPredictWeightedCompoundInterLumaBlock8WithFiltersSamplesAtBlockOrigin(t *testing.T) {
	const (
		planeW = 32
		planeH = 24
		x      = 9
		y      = 7
		bw     = 5
		bh     = 4
		weight = 10
	)
	ref0 := make([]byte, planeW*planeH)
	ref1 := make([]byte, planeW*planeH)
	for py := 0; py < planeH; py++ {
		for px := 0; px < planeW; px++ {
			ref0[py*planeW+px] = byte(7 + px*3 + py*5)
			ref1[py*planeW+px] = byte(41 + px*2 + py*4)
		}
	}
	dst := make([]byte, planeW*planeH)
	if err := PredictWeightedCompoundInterLumaBlock8WithFilters(dst, planeW, planeW, planeH, x, y, bw, bh, ref0, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), ref1, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), weight); err != nil {
		t.Fatalf("PredictWeightedCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for py := 0; py < bh; py++ {
		for px := 0; px < bw; px++ {
			idx := (y+py)*planeW + x + px
			want := byte((int(ref0[idx])*weight + int(ref1[idx])*(16-weight) + 8) >> 4)
			if got := dst[idx]; got != want {
				t.Fatalf("dst(%d,%d)=%d, want %d", x+px, y+py, got, want)
			}
		}
	}
}

func TestPredictWeightedCompoundInterLumaBlock16WithFiltersSamplesAtBlockOrigin(t *testing.T) {
	const (
		planeW   = 32
		planeH   = 24
		x        = 11
		y        = 5
		bw       = 4
		bh       = 6
		weight   = 6
		bitDepth = 10
	)
	ref0 := make([]uint16, planeW*planeH)
	ref1 := make([]uint16, planeW*planeH)
	for py := 0; py < planeH; py++ {
		for px := 0; px < planeW; px++ {
			ref0[py*planeW+px] = uint16(100 + px*5 + py*7)
			ref1[py*planeW+px] = uint16(300 + px*3 + py*4)
		}
	}
	dst := make([]uint16, planeW*planeH)
	if err := PredictWeightedCompoundInterLumaBlock16WithFilters(dst, planeW, planeW, planeH, x, y, bw, bh, ref0, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), ref1, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), bitDepth, weight); err != nil {
		t.Fatalf("PredictWeightedCompoundInterLumaBlock16WithFilters: %v", err)
	}
	for py := 0; py < bh; py++ {
		for px := 0; px < bw; px++ {
			idx := (y+py)*planeW + x + px
			want := uint16((int(ref0[idx])*weight + int(ref1[idx])*(16-weight) + 8) >> 4)
			if got := dst[idx]; got != want {
				t.Fatalf("dst(%d,%d)=%d, want %d", x+px, y+py, got, want)
			}
		}
	}
}

func TestSelectSkipModeCompoundRefsUsesHeaderRefs(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.SkipModeRefs[0] = 1
	hdr.SkipModeRefs[1] = 5
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 4, End4Y: 4})
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2, Size: Block8x8}, 1, 3, MotionVector{X: 2}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8}, 5, 6, MotionVector{Y: -2}, InterPredNearest, defaultInterFilterPair())
	d := &Decoder{}
	ref0, mv0, ref1, mv1, err := d.selectSkipModeCompoundRefs(nil, hdr, tile, nil, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}, fullIntraEdgeAvailability)
	if err != nil {
		t.Fatalf("selectSkipModeCompoundRefs: %v", err)
	}
	if ref0 != 1 || ref1 != 5 {
		t.Fatalf("refs=(%d,%d), want (1,5)", ref0, ref1)
	}
	if mv0 != (MotionVector{X: 2}) {
		t.Fatalf("mv0=%+v, want %+v", mv0, MotionVector{X: 2})
	}
	if mv1 != (MotionVector{Y: -2}) {
		t.Fatalf("mv1=%+v, want %+v", mv1, MotionVector{Y: -2})
	}
}

func TestJointCompoundWeightUsesRefPOCDistance(t *testing.T) {
	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		EnableJntComp:       true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.RefFramePOC[1] = 10
	parsed.RefFramePOC[5] = 18
	weight, ok := jointCompoundWeight(seq, parsed, 1, 5)
	if !ok {
		t.Fatal("jointCompoundWeight returned disabled")
	}
	if weight == 8 {
		t.Fatalf("weight=%d, want non-average joint weight", weight)
	}
}

func TestReadCompoundRefSelectionBidir(t *testing.T) {
	cdf := NewDefaultModeCDF()
	above := &BlockContext{}
	left := &BlockContext{}
	dec := &scriptedCompoundEntropy{
		bools: []uint32{1, 0, 1, 0, 1},
	}
	ref0, ref1, err := ReadCompoundRefSelection(cdf, above, left, 0, 0, false, false, dec)
	if err != nil {
		t.Fatalf("ReadCompoundRefSelection: %v", err)
	}
	if ref0 != 1 || ref1 != 5 {
		t.Fatalf("refs=(%d,%d), want (1,5)", ref0, ref1)
	}
}

func TestReadCompoundTypeWedge(t *testing.T) {
	seq := av1.SequenceHeader{
		EnableMaskedCompound: true,
	}
	cdf := NewDefaultModeCDF()
	above := &BlockContext{}
	left := &BlockContext{}
	dec := &scriptedCompoundEntropy{
		bools: []uint32{1, 0},
		syms:  []uint32{3},
		equis: []uint32{1},
	}
	compType, sign, wedgeIdx, err := ReadCompoundType(seq, nil, cdf, above, left, 0, 0, BlockGeometry{Size: Block16x16}, 1, 5, dec)
	if err != nil {
		t.Fatalf("ReadCompoundType: %v", err)
	}
	if compType != compoundTypeWedge {
		t.Fatalf("compType=%d, want %d", compType, compoundTypeWedge)
	}
	if !sign {
		t.Fatal("maskSign=false, want true")
	}
	if wedgeIdx != 3 {
		t.Fatalf("wedgeIdx=%d, want 3", wedgeIdx)
	}
}

func TestReadInterIntraSyntaxWedge(t *testing.T) {
	seq := av1.SequenceHeader{
		EnableInterIntraCompound: true,
	}
	cdf := NewDefaultModeCDF()
	dec := &scriptedCompoundEntropy{
		bools: []uint32{1, 1},
		syms:  []uint32{2, 4},
	}
	typ, mode, wedgeIdx, err := ReadInterIntraSyntax(seq, cdf, BlockGeometry{Size: Block8x8}, dec)
	if err != nil {
		t.Fatalf("ReadInterIntraSyntax: %v", err)
	}
	if typ != interIntraWedge {
		t.Fatalf("typ=%d, want %d", typ, interIntraWedge)
	}
	if mode != IntraPredHorizontal {
		t.Fatalf("mode=%d, want %d", mode, IntraPredHorizontal)
	}
	if wedgeIdx != 4 {
		t.Fatalf("wedgeIdx=%d, want 4", wedgeIdx)
	}
}

func TestPredictMaskedCompoundInterLumaBlock8WithFiltersUsesMask(t *testing.T) {
	ref0 := make([]byte, 8*8)
	ref1 := make([]byte, 8*8)
	fillUint8(ref0, 32)
	fillUint8(ref1, 96)
	mask := make([]uint8, 8*8)
	fillUint8(mask, 64)
	dst := make([]byte, 8*8)
	if err := PredictMaskedCompoundInterLumaBlock8WithFilters(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), mask, 8); err != nil {
		t.Fatalf("PredictMaskedCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for i, v := range dst {
		if v != 32 {
			t.Fatalf("dst[%d]=%d, want 32", i, v)
		}
	}
}

func TestPredictMaskedCompoundInterLumaBlock8WithFiltersSamplesAtBlockOrigin(t *testing.T) {
	const (
		planeW = 24
		planeH = 20
		x      = 6
		y      = 8
		bw     = 7
		bh     = 5
	)
	ref0 := make([]byte, planeW*planeH)
	ref1 := make([]byte, planeW*planeH)
	for py := 0; py < planeH; py++ {
		for px := 0; px < planeW; px++ {
			ref0[py*planeW+px] = byte(3 + px*4 + py*5)
			ref1[py*planeW+px] = byte(80 + px*2 + py*3)
		}
	}
	mask := make([]uint8, bw*bh)
	fillUint8(mask, 64)
	dst := make([]byte, planeW*planeH)
	if err := PredictMaskedCompoundInterLumaBlock8WithFilters(dst, planeW, planeW, planeH, x, y, bw, bh, ref0, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), ref1, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), mask, bw); err != nil {
		t.Fatalf("PredictMaskedCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for py := 0; py < bh; py++ {
		for px := 0; px < bw; px++ {
			idx := (y+py)*planeW + x + px
			if got, want := dst[idx], ref0[idx]; got != want {
				t.Fatalf("dst(%d,%d)=%d, want %d", x+px, y+py, got, want)
			}
		}
	}
}

func TestGlobalMotionWarpForPlaneTranslation(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.GMV[1] = obu.WarpedMotionParams{
		Type: 1,
		Matrix: [6]int32{
			1 << 16, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}
	warp, ok := globalMotionWarpForPlane(hdr, 1, 0, av1.Chroma420)
	if !ok {
		t.Fatal("globalMotionWarpForPlane returned disabled")
	}
	x, y := warp.mapPoint(3, 5)
	if math.Abs(x-4) > 1e-6 || math.Abs(y-5) > 1e-6 {
		t.Fatalf("mapPoint=(%.4f,%.4f), want (4,5)", x, y)
	}
}

func TestPredictWeightedCompoundInterLumaBlock8MotionUsesGlobalWarp(t *testing.T) {
	ref0 := make([]byte, 8*8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			ref0[y*8+x] = uint8(x)
		}
	}
	ref1 := make([]byte, 8*8)
	fillUint8(ref1, 0)
	hdr := &obu.FrameHeader{}
	hdr.GMV[2] = obu.WarpedMotionParams{
		Type: 1,
		Matrix: [6]int32{
			1 << 16, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}
	warp, ok := globalMotionWarpForPlane(hdr, 2, 0, av1.Chroma420)
	if !ok {
		t.Fatal("globalMotionWarpForPlane returned disabled")
	}
	dst := make([]byte, 8*8)
	if err := PredictWeightedCompoundInterLumaBlock8Motion(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), true, warp, ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), false, affineWarp{}, 16); err != nil {
		t.Fatalf("PredictWeightedCompoundInterLumaBlock8Motion: %v", err)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 7; x++ {
			if got := dst[y*8+x]; got != uint8(x+1) {
				t.Fatalf("dst[%d,%d]=%d, want %d", x, y, got, x+1)
			}
		}
		if got := dst[y*8+7]; got != 7 {
			t.Fatalf("dst[%d,%d]=%d, want 7", 7, y, got)
		}
	}
}

func TestPredictWeightedCompoundInterLumaBlock8MotionWarpSamplesAtBlockOrigin(t *testing.T) {
	const (
		planeW = 24
		planeH = 20
		x      = 7
		y      = 6
		bw     = 5
		bh     = 4
	)
	ref0 := make([]byte, planeW*planeH)
	for py := 0; py < planeH; py++ {
		for px := 0; px < planeW; px++ {
			ref0[py*planeW+px] = byte(11 + px*3 + py*4)
		}
	}
	ref1 := make([]byte, planeW*planeH)
	warp := affineWarp{a00: 1, a11: 1}
	dst := make([]byte, planeW*planeH)
	if err := PredictWeightedCompoundInterLumaBlock8Motion(dst, planeW, planeW, planeH, x, y, bw, bh, ref0, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), true, warp, ref1, planeW, planeW, planeH, MotionVector{}, defaultInterFilterPair(), false, affineWarp{}, 16); err != nil {
		t.Fatalf("PredictWeightedCompoundInterLumaBlock8Motion: %v", err)
	}
	for py := 0; py < bh; py++ {
		for px := 0; px < bw; px++ {
			idx := (y+py)*planeW + x + px
			if got, want := dst[idx], ref0[idx]; got != want {
				t.Fatalf("dst(%d,%d)=%d, want %d", x+px, y+py, got, want)
			}
		}
	}
}

func TestFindCompoundRefCandidatesPairsNeighborRefs(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	tile.SplatCompoundRef(
		BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2, Size: Block8x8},
		1, 5, 3, 6,
		MotionVector{X: 4}, MotionVector{Y: -4},
		InterPredNearest, InterPredNearest,
		defaultInterFilterPair(),
	)
	cands, _ := tile.FindCompoundRefCandidates(nil, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}, 1, 5)
	if len(cands) == 0 || cands[0].Weight == 0 {
		t.Fatal("expected real compound candidates")
	}
	mv0, mv1, err := SelectCompoundRefMVCandidate(cands, 0)
	if err != nil {
		t.Fatalf("SelectCompoundRefMVCandidate: %v", err)
	}
	if mv0 != (MotionVector{X: 4}) || mv1 != (MotionVector{Y: -4}) {
		t.Fatalf("mvs=(%+v,%+v), want (%+v,%+v)", mv0, mv1, MotionVector{X: 4}, MotionVector{Y: -4})
	}
}

func TestFindCompoundRefCandidatesParsedBuildsExtendedPairFromSingleRefs(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	tile.SplatSingleRef(
		BlockGeometry{Start4X: 2, Start4Y: 1, End4X: 4, End4Y: 2, Size: Block8x8},
		1, 3, MotionVector{X: 6}, InterPredNearest, defaultInterFilterPair(),
	)
	tile.SplatSingleRef(
		BlockGeometry{Start4X: 1, Start4Y: 2, End4X: 2, End4Y: 4, Size: Block8x8},
		5, 6, MotionVector{Y: -8}, InterPredNearest, defaultInterFilterPair(),
	)
	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.RefFramePOC[1] = 10
	parsed.RefFramePOC[5] = 30
	cands, _ := tile.FindCompoundRefCandidatesParsed(seq, parsed, nil, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}, 1, 5)
	if len(cands) == 0 {
		t.Fatal("expected extended compound candidates")
	}
	if cands[0].MV[0] != (MotionVector{X: 6}) {
		t.Fatalf("mv0=%+v, want %+v", cands[0].MV[0], MotionVector{X: 6})
	}
	if cands[0].MV[1] != (MotionVector{Y: -8}) {
		t.Fatalf("mv1=%+v, want %+v", cands[0].MV[1], MotionVector{Y: -8})
	}
	if cands[0].Weight != 2 {
		t.Fatalf("weight=%d, want 2", cands[0].Weight)
	}
}

func TestFindCompoundRefCandidatesParsedFallsBackToGlobalMotion(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	tile.SplatSingleRef(
		BlockGeometry{Start4X: 2, Start4Y: 1, End4X: 4, End4Y: 2, Size: Block8x8},
		1, 3, MotionVector{X: 6}, InterPredNearest, defaultInterFilterPair(),
	)
	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.Width = 32
	parsed.Header.Height = 32
	parsed.Header.FrameOffset = 20
	parsed.RefFramePOC[1] = 10
	parsed.RefFramePOC[5] = 30
	parsed.Header.GMV[5] = obu.WarpedMotionParams{
		Type: 1,
		Matrix: [6]int32{
			1 << 16, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}
	cands, _ := tile.FindCompoundRefCandidatesParsed(seq, parsed, &parsed.Header, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}, 1, 5)
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want >= 2", len(cands))
	}
	if cands[0].MV[0] != (MotionVector{X: 6}) {
		t.Fatalf("mv0=%+v, want %+v", cands[0].MV[0], MotionVector{X: 6})
	}
	if cands[1].MV[1] != (MotionVector{Y: 8}) {
		t.Fatalf("second mv1=%+v, want %+v", cands[1].MV[1], MotionVector{Y: 8})
	}
}

func TestFindCompoundRefCandidatesKeepsNearestSectionAheadOfSecondary(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 16, End4Y: 16})
	nearMV0 := MotionVector{X: 4}
	nearMV1 := MotionVector{Y: 4}
	secMV0 := MotionVector{X: -8}
	secMV1 := MotionVector{Y: -8}
	tile.SplatCompoundRef(BlockGeometry{Start4X: 4, Start4Y: 3, End4X: 8, End4Y: 4, Size: Block16x16}, 1, 5, 1, 5, nearMV0, nearMV1, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 3, Start4Y: 4, End4X: 4, End4Y: 8, Size: Block16x16}, 1, 5, 1, 5, nearMV0, nearMV1, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 5, Start4Y: 1, End4X: 9, End4Y: 3, Size: Block16x16}, 1, 5, 1, 5, secMV0, secMV1, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 1, Start4Y: 5, End4X: 3, End4Y: 9, Size: Block16x16}, 1, 5, 1, 5, secMV0, secMV1, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	cands, _ := tile.FindCompoundRefCandidates(nil, BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8, Size: Block16x16}, 1, 5)
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want >= 2", len(cands))
	}
	if cands[0].MV[0] != nearMV0 || cands[0].MV[1] != nearMV1 {
		t.Fatalf("first candidate=(%+v,%+v), want near (%+v,%+v)", cands[0].MV[0], cands[0].MV[1], nearMV0, nearMV1)
	}
}

func TestBuildInterIntraMaskWedgeIsNonUniform(t *testing.T) {
	mask := buildInterIntraMask(8, 8, interIntraWedge, IntraPredDC, 3)
	if len(mask) != 64 {
		t.Fatalf("mask len=%d, want 64", len(mask))
	}
	first := mask[0]
	allSame := true
	for _, v := range mask[1:] {
		if v != first {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("expected non-uniform wedge mask")
	}
}

func TestBuildInterIntraMaskWedgeUsesDefaultSign(t *testing.T) {
	mask := buildInterIntraMask(32, 32, interIntraWedge, IntraPredSmooth, 8)
	if got := mask[8*32+20]; got != 2 {
		t.Fatalf("mask[20,8]=%d, want default-signed value 2", got)
	}
	raw := buildWedgeMask(32, 32, 8, false)
	if got := raw[8*32+20]; got != 62 {
		t.Fatalf("raw mask[20,8]=%d, want 62 before default sign", got)
	}
}

func TestBuildWedgeMaskSignInvertsValues(t *testing.T) {
	mask0 := buildWedgeMask(16, 16, 4, false)
	mask1 := buildWedgeMask(16, 16, 4, true)
	if len(mask0) != len(mask1) {
		t.Fatalf("len(mask0)=%d len(mask1)=%d", len(mask0), len(mask1))
	}
	for i := range mask0 {
		if int(mask0[i])+int(mask1[i]) != 64 {
			t.Fatalf("mask[%d]=%d inv=%d, want sum 64", i, mask0[i], mask1[i])
		}
	}
}

func TestBuildCompoundWedgeMaskCombinesDefaultAndBitstreamSign(t *testing.T) {
	raw := buildWedgeMask(16, 16, 6, false)
	got := buildCompoundMask(16, 16, compoundTypeWedge, 6, true)
	if len(got) != len(raw) {
		t.Fatalf("mask len=%d, want %d", len(got), len(raw))
	}
	for i := range raw {
		if got[i] != raw[i] {
			t.Fatalf("mask[%d]=%d, want raw %d", i, got[i], raw[i])
		}
	}
}

func TestBuildInterIntraMaskVerticalUsesAV1Weights(t *testing.T) {
	mask := buildInterIntraMask(8, 8, interIntraBlend, IntraPredVertical, 0)
	if got := mask[0]; got != 60 {
		t.Fatalf("top weight=%d, want 60", got)
	}
	if got := mask[7*8]; got != 1 {
		t.Fatalf("bottom weight=%d, want 1", got)
	}
	for x := 1; x < 8; x++ {
		if mask[x] != mask[0] {
			t.Fatalf("top row varies at x=%d: %d vs %d", x, mask[x], mask[0])
		}
	}
}

func TestBuildInterIntraMaskSmoothUsesMinAxisWeights(t *testing.T) {
	mask := buildInterIntraMask(8, 8, interIntraBlend, IntraPredSmooth, 0)
	if got := mask[0]; got != 60 {
		t.Fatalf("top-left weight=%d, want 60", got)
	}
	if got := mask[7]; got != 60 {
		t.Fatalf("top-right weight=%d, want 60", got)
	}
	if got := mask[7*8]; got != 60 {
		t.Fatalf("bottom-left weight=%d, want 60", got)
	}
	if got := mask[7*8+7]; got != 1 {
		t.Fatalf("bottom-right weight=%d, want 1", got)
	}
}

func TestBuildInterIntraChromaWedgeMaskSubsamplesLumaMask(t *testing.T) {
	got := buildInterIntraChromaMaskInto(nil, nil, av1.Chroma420, Block8x8, interIntraWedge, IntraPredHorizontal, 0)
	luma := buildInterIntraMask(8, 8, interIntraWedge, IntraPredHorizontal, 0)
	want := make([]uint8, 4*4)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			srcY := y * 2
			srcX := x * 2
			sum := int(luma[srcY*8+srcX]) +
				int(luma[srcY*8+srcX+1]) +
				int(luma[(srcY+1)*8+srcX]) +
				int(luma[(srcY+1)*8+srcX+1])
			want[y*4+x] = uint8((sum + 2) >> 2)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chroma wedge mask=%v, want subsampled luma mask %v", got, want)
	}

	direct := buildInterIntraMask(4, 4, interIntraWedge, IntraPredHorizontal, 0)
	if reflect.DeepEqual(got, direct) {
		t.Fatalf("chroma wedge mask unexpectedly matches direct 4x4 wedge mask %v", direct)
	}
}

func TestBuildInterIntraMaskIntoReusesDestination(t *testing.T) {
	var buf [64]uint8
	mask := buildInterIntraMaskInto(buf[:0], 8, 8, interIntraBlend, IntraPredDC, 0)
	if len(mask) != 64 {
		t.Fatalf("len(mask)=%d, want 64", len(mask))
	}
	if &mask[0] != &buf[0] {
		t.Fatal("expected buildInterIntraMaskInto to reuse destination buffer")
	}
	for i, v := range mask {
		if v != 32 {
			t.Fatalf("mask[%d]=%d, want 32", i, v)
		}
	}
}

func TestBuildInterIntraPredictor8IntoReusesDestination(t *testing.T) {
	plane := make([]byte, 9*9)
	for y := 0; y < 9; y++ {
		for x := 0; x < 9; x++ {
			plane[y*9+x] = byte(16 + x + y)
		}
	}
	var dst [64]byte
	var top [8]byte
	var left [8]byte
	got, width, height, err := buildInterIntraPredictor8Into(dst[:0], top[:0], left[:0], plane, 9, 9, 9, 1, 1, 8, 8, IntraPredDC)
	if err != nil {
		t.Fatalf("buildInterIntraPredictor8Into: %v", err)
	}
	if width != 8 || height != 8 {
		t.Fatalf("size=%dx%d, want 8x8", width, height)
	}
	if len(got) != 64 {
		t.Fatalf("len(got)=%d, want 64", len(got))
	}
	if &got[0] != &dst[0] {
		t.Fatal("expected buildInterIntraPredictor8Into to reuse destination buffer")
	}
}

func TestBlendInterIntraWedgeUsesNominalBlockAtRightEdge(t *testing.T) {
	const (
		stride      = 64
		planeWidth  = 54
		planeHeight = 40
		x           = 32
		y           = 4
		width       = 32
		height      = 32
	)
	plane := make([]byte, stride*planeHeight)
	fillUint8(plane, 255)
	for py := 0; py < height; py++ {
		row := plane[(y+py)*stride:]
		for px := 0; px < planeWidth-x; px++ {
			row[x+px] = 179
		}
	}
	if err := blendInterIntraBlock8(plane, stride, planeWidth, planeHeight, x, y, width, height, interIntraWedge, IntraPredSmooth, 12); err != nil {
		t.Fatalf("blendInterIntraBlock8: %v", err)
	}
	if got := plane[y*stride+x+20]; got != 179 {
		t.Fatalf("right-edge wedge sample=%d, want preserved inter prediction 179", got)
	}
}

func TestPredictDiffWeightedCompoundInterLumaBlock8WithFiltersUsesDifferenceMask(t *testing.T) {
	ref0 := make([]byte, 8*8)
	ref1 := make([]byte, 8*8)
	fillUint8(ref0, 32)
	fillUint8(ref1, 96)
	dst := make([]byte, 8*8)
	mask, err := PredictDiffWeightedCompoundInterLumaBlock8WithFilters(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), false)
	if err != nil {
		t.Fatalf("PredictDiffWeightedCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for i, v := range mask {
		if v != 42 {
			t.Fatalf("mask[%d]=%d, want 42", i, v)
		}
	}
	for i, v := range dst {
		if v != 54 {
			t.Fatalf("dst[%d]=%d, want 54", i, v)
		}
	}
}

func TestPredictDiffWeightedCompoundInterLumaBlock8WithFiltersSignFlipsBias(t *testing.T) {
	ref0 := make([]byte, 8*8)
	ref1 := make([]byte, 8*8)
	fillUint8(ref0, 32)
	fillUint8(ref1, 96)
	dst := make([]byte, 8*8)
	mask, err := PredictDiffWeightedCompoundInterLumaBlock8WithFilters(dst, 8, 8, 8, 0, 0, 8, 8, ref0, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), ref1, 8, 8, 8, MotionVector{}, defaultInterFilterPair(), true)
	if err != nil {
		t.Fatalf("PredictDiffWeightedCompoundInterLumaBlock8WithFilters: %v", err)
	}
	for i, v := range mask {
		if v != 22 {
			t.Fatalf("mask[%d]=%d, want 22", i, v)
		}
	}
	for i, v := range dst {
		if v != 74 {
			t.Fatalf("dst[%d]=%d, want 74", i, v)
		}
	}
}

func TestSubsampleCompoundMask422UsesSignAwareRounding(t *testing.T) {
	mask := []uint8{
		10, 21, 30, 41,
		50, 61, 7, 10,
	}
	got := subsampleCompoundMask(mask, 4, 2, av1.Chroma422, false)
	want := []uint8{
		16, 36,
		56, 9,
	}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%d, want %d", i, got[i], want[i])
		}
	}
	gotSign := subsampleCompoundMask(mask, 4, 2, av1.Chroma422, true)
	wantSign := []uint8{
		15, 35,
		55, 8,
	}
	wantSign[0] = (10 + 21) >> 1
	wantSign[1] = (30 + 41) >> 1
	wantSign[2] = (50 + 61) >> 1
	wantSign[3] = (7 + 10) >> 1
	for i := range wantSign {
		if gotSign[i] != wantSign[i] {
			t.Fatalf("sign got[%d]=%d, want %d", i, gotSign[i], wantSign[i])
		}
	}
}

func TestSubsampleCompoundMask420UsesSignAwareRounding(t *testing.T) {
	mask := []uint8{
		10, 21, 30, 41,
		50, 61, 70, 81,
		1, 3, 3, 5,
		5, 7, 7, 9,
	}
	got := subsampleCompoundMask(mask, 4, 4, av1.Chroma420, false)
	want := []uint8{
		36, 56,
		4, 6,
	}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%d, want %d", i, got[i], want[i])
		}
	}
	gotSign := subsampleCompoundMask(mask, 4, 4, av1.Chroma420, true)
	wantSign := []uint8{
		(10 + 21 + 50 + 61 + 1) >> 2,
		(30 + 41 + 70 + 81 + 1) >> 2,
		(1 + 3 + 5 + 7 + 1) >> 2,
		(3 + 5 + 7 + 9 + 1) >> 2,
	}
	for i := range wantSign {
		if gotSign[i] != uint8(wantSign[i]) {
			t.Fatalf("sign got[%d]=%d, want %d", i, gotSign[i], wantSign[i])
		}
	}
}
