package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestFindSingleRefCandidatesUsesGlobalMotionForGlobalNeighbor(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	neighbor := BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8}
	tile.SplatSingleRef(neighbor, 0, 0, MotionVector{X: 999, Y: 777}, InterPredGlobal, defaultInterFilterPair())

	hdr := &obu.FrameHeader{Width: 32, Height: 32}
	hdr.GMV[0] = obu.WarpedMotionParams{
		Type: 2,
		Matrix: [6]int32{
			1 << 16, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}

	cands, _ := tile.FindSingleRefCandidates(hdr, target, 0)
	if len(cands) == 0 {
		t.Fatal("expected refmv candidates")
	}
	if got, want := cands[0].MV, (MotionVector{X: 8, Y: 0}); got != want {
		t.Fatalf("first candidate=%+v, want global %+v", got, want)
	}
	_, ctx := tile.FindSingleRefCandidates(hdr, target, 0)
	if got, want := (ctx>>3)&1, 0; got != want {
		t.Fatalf("globalmv_ctx=%d, want %d", got, want)
	}
}

func TestFindSingleRefCandidatesCountsCompoundNewMVForContext(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 12, End4Y: 12})
	target := BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8, Size: Block16x16}
	above := BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 8, End4Y: 4, Size: Block16x16}
	left := BlockGeometry{Start4X: 0, Start4Y: 4, End4X: 4, End4Y: 8, Size: Block16x16}

	tile.SplatCompoundRef(above, 0, 2, 0, 2,
		MotionVector{}, MotionVector{X: 64},
		InterPredNearest, InterPredNew, defaultInterFilterPair())
	tile.SplatSingleRef(left, 0, 0, MotionVector{X: -128}, InterPredNearest, defaultInterFilterPair())

	_, ctx := tile.FindSingleRefCandidates(&obu.FrameHeader{}, target, 0)
	if got, want := ctx&7, 4; got != want {
		t.Fatalf("newmv_ctx=%d, want %d from compound neighbor block-level NEWMV flag", got, want)
	}
}

func TestSpatialMVTileViewWritesThroughAndRespectsGeometry(t *testing.T) {
	field := NewFrameMVField(32, 32)
	view := field.tileView(TileGeometry{Start4X: 2, Start4Y: 1, End4X: 6, End4Y: 5})
	g := BlockGeometry{Start4X: 2, Start4Y: 1, End4X: 4, End4Y: 3, Size: Block8x8}

	view.SplatSingleRef(g, 0, 0, MotionVector{X: 6, Y: -4}, InterPredNearest, defaultInterFilterPair())

	if block := field.block(2, 1); block == nil || !block.Valid || block.MV[0] != (MotionVector{X: 6, Y: -4}) {
		t.Fatalf("frame field block = %+v, want written-through single-ref mv", block)
	}
	if block := view.block(1, 1); block != nil {
		t.Fatalf("view block outside tile = %+v, want nil", block)
	}
}

func TestFrameMVFieldUsesAV1MiGridPadding(t *testing.T) {
	field := NewFrameMVField(960, 540)
	if got, want := field.Geometry.End4X, 240; got != want {
		t.Fatalf("End4X=%d, want %d", got, want)
	}
	if got, want := field.Geometry.End4Y, 136; got != want {
		t.Fatalf("End4Y=%d, want padded AV1 mi-grid height %d", got, want)
	}
	field.SplatSingleRef(
		BlockGeometry{Start4X: 238, Start4Y: 134, End4X: 240, End4Y: 136, Size: Block8x8},
		0, 0, MotionVector{X: 6, Y: -4}, InterPredNearest, defaultInterFilterPair(),
	)
	if block := field.block(239, 135); block == nil || !block.Valid {
		t.Fatalf("bottom padded mv block = %+v, want valid block", block)
	}
	if block := field.block(239, 136); block != nil {
		t.Fatalf("block beyond padded mi-grid = %+v, want nil", block)
	}

	scratch := takeFrameMVField(960, 540)
	t.Cleanup(func() { putSpatialMVTileScratch(scratch) })
	if got, want := scratch.Geometry.End4Y, 136; got != want {
		t.Fatalf("scratch End4Y=%d, want %d", got, want)
	}
}

func TestBuildTemporalMVMapSamplesLowerRight4x4Cell(t *testing.T) {
	field := NewFrameMVField(32, 32)
	field.SlotPOC[0] = 1
	field.SplatSingleRef(
		BlockGeometry{Start4X: 3, Start4Y: 2, End4X: 4, End4Y: 3, Size: Block4x4},
		0, 0, MotionVector{X: 10, Y: 10}, InterPredNew, defaultInterFilterPair(),
	)
	field.SplatSingleRef(
		BlockGeometry{Start4X: 3, Start4Y: 3, End4X: 4, End4Y: 4, Size: Block4x4},
		0, 0, MotionVector{X: 20, Y: 30}, InterPredNew, defaultInterFilterPair(),
	)

	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.Width = 32
	parsed.Header.Height = 32
	parsed.Header.FrameOffset = 2
	parsed.RefFramePOC[0] = 1

	m := buildTemporalMVMap(seq, parsed, field)
	if m == nil {
		t.Fatal("buildTemporalMVMap returned nil")
	}
	if got, want := m.block8(1, 1).MV, (MotionVector{X: 20, Y: 30}); got != want {
		t.Fatalf("saved temporal mv=%+v, want lower-right %+v", got, want)
	}
}

func TestFindTemporalCandidatesForSlotProjectedProjectsByOrderHintRatio(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	tile.FramePOC = 18
	tile.SlotPOC[3] = 14
	tile.SplatSingleRef(
		BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8},
		0, 3, MotionVector{X: 8}, InterPredNearest, defaultInterFilterPair(),
	)

	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.Header.HighPrecisionMV = true
	parsed.RefFramePOC[0] = 10

	cands := tile.FindTemporalCandidatesForSlotProjected(seq, parsed, 0, 3, &parsed.Header, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8})
	if len(cands) == 0 {
		t.Fatal("expected temporal candidates")
	}
	if got, want := cands[0].MV, (MotionVector{X: 20}); got != want {
		t.Fatalf("projected mv=%+v, want %+v", got, want)
	}
}

func TestFindTemporalCandidatesForSlotProjectedHonors8x8Boundary(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 64, End4Y: 128})
	target := BlockGeometry{Start4X: 14, Start4Y: 118, End4X: 16, End4Y: 120, Size: Block8x8}
	tile.SplatSingleRef(target, 0, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 12, Start4Y: 120, End4X: 14, End4Y: 122, Size: Block8x8}, 0, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 16, Start4Y: 118, End4X: 18, End4Y: 120, Size: Block8x8}, 0, 0, MotionVector{X: -24}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 16, Start4Y: 120, End4X: 18, End4Y: 122, Size: Block8x8}, 0, 0, MotionVector{X: -24}, InterPredNearest, defaultInterFilterPair())

	cands := tile.FindTemporalCandidatesForSlotProjected(av1.SequenceHeader{}, &ParsedFrame{}, 0, 0, &obu.FrameHeader{}, target)
	if len(cands) != 1 {
		t.Fatalf("len(cands)=%d, want only in-tile temporal samples: %+v", len(cands), cands)
	}
	if cands[0].MV != (MotionVector{}) {
		t.Fatalf("candidate MV=%+v, want right-edge samples excluded", cands[0].MV)
	}
}

func TestTemporalMVMapProjectedCandidatesHonor8x8Boundary(t *testing.T) {
	m := newTemporalMVMap(512, 1024)
	target := BlockGeometry{Start4X: 14, Start4Y: 118, End4X: 16, End4Y: 120, Size: Block8x8}
	m.setBlock8(7, 59, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	m.setBlock8(6, 60, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	m.setBlock8(8, 59, TemporalMVBlock{MV: MotionVector{X: -24}, Ref: 1})
	m.setBlock8(8, 60, TemporalMVBlock{MV: MotionVector{X: -24}, Ref: 1})

	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.Header.HighPrecisionMV = true
	parsed.RefFramePOC[0] = 19
	cands := m.findCandidatesProjectedInto(nil, seq, parsed, 0, &parsed.Header, target)
	if len(cands) != 1 {
		t.Fatalf("len(cands)=%d, want right-edge temporal samples excluded: %+v", len(cands), cands)
	}
	if cands[0].MV != (MotionVector{}) {
		t.Fatalf("candidate MV=%+v, want only in-region zero MV", cands[0].MV)
	}
}

func TestTemporalMVMapProjectedCompoundCandidatesProjectBothRefs(t *testing.T) {
	m := newTemporalMVMap(64, 64)
	m.setBlock8(1, 1, TemporalMVBlock{MV: MotionVector{X: 32}, Ref: 4})

	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.Header.HighPrecisionMV = true
	parsed.RefFramePOC[0] = 18
	parsed.RefFramePOC[6] = 24

	cands := m.findCompoundCandidatesProjectedInto(nil, seq, parsed, 0, 6, &parsed.Header, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8})
	if len(cands) != 1 {
		t.Fatalf("len(cands)=%d, want one projected compound candidate: %+v", len(cands), cands)
	}
	if got, want := cands[0].MV[0], (MotionVector{X: 16}); got != want {
		t.Fatalf("first ref projected MV=%+v, want %+v", got, want)
	}
	if got, want := cands[0].MV[1], (MotionVector{X: -32}); got != want {
		t.Fatalf("second ref projected MV=%+v, want %+v", got, want)
	}
}

func TestTemporalMVMapProjectedCompoundCandidatesHonor8x8Boundary(t *testing.T) {
	m := newTemporalMVMap(512, 1024)
	target := BlockGeometry{Start4X: 14, Start4Y: 118, End4X: 16, End4Y: 120, Size: Block8x8}
	m.setBlock8(7, 59, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	m.setBlock8(6, 60, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	m.setBlock8(8, 59, TemporalMVBlock{MV: MotionVector{X: -24}, Ref: 1})
	m.setBlock8(8, 60, TemporalMVBlock{MV: MotionVector{X: -24}, Ref: 1})

	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.Header.HighPrecisionMV = true
	parsed.RefFramePOC[0] = 19
	parsed.RefFramePOC[6] = 18
	cands := m.findCompoundCandidatesProjectedInto(nil, seq, parsed, 0, 6, &parsed.Header, target)
	if len(cands) != 1 {
		t.Fatalf("len(cands)=%d, want right-edge temporal samples excluded: %+v", len(cands), cands)
	}
	if cands[0].MV != [2]MotionVector{} {
		t.Fatalf("candidate MV=%+v, want only in-region zero MV pair", cands[0].MV)
	}
}

func TestProjectTemporalMVMapWindowDoesNotProjectAcrossSBRowStart(t *testing.T) {
	src := newTemporalMVMap(128, 512)
	src.setBlock8(8, 31, TemporalMVBlock{MV: MotionVector{Y: 64}, Ref: 1})
	src.setBlock8(9, 32, TemporalMVBlock{MV: MotionVector{}, Ref: 1})
	var refs [7]*TemporalMVMap
	refs[4] = src
	out := newTemporalMVMap(128, 512)
	selected := []temporalMFMVRef{{
		ref:     4,
		ref2cur: 1,
		ref2ref: [7]int{1},
	}}

	projectTemporalMVMapWindow(nil, refs, out, selected, 0, out.Width8, 32, 48)
	if got := out.block8(8, 32); got.Ref != 0 {
		t.Fatalf("projected block at row-window start=%+v, want source from previous row ignored", got)
	}
	if got := out.block8(9, 32); got.Ref != 1 {
		t.Fatalf("same-window projected block=%+v, want valid projection", got)
	}
}

func TestAugmentSingleRefCandidatesTemporalPreservesNearestSection(t *testing.T) {
	base := []RefMVCandidate{
		{MV: MotionVector{X: 4}, Weight: 644},
		{MV: MotionVector{X: 2}, Weight: 640},
		{MV: MotionVector{X: 10}, Weight: 6},
		{MV: MotionVector{X: 12}, Weight: 4},
	}
	temporal := []RefMVCandidate{
		{MV: MotionVector{X: 8}, Weight: 2},
		{MV: MotionVector{X: 10}, Weight: 2},
	}
	got := AugmentSingleRefCandidatesTemporal(base, temporal)
	if len(got) < 5 {
		t.Fatalf("len(got)=%d, want at least 5", len(got))
	}
	if got[0].MV.X != 4 || got[1].MV.X != 2 {
		t.Fatalf("nearest section reordered: got[0]=%+v got[1]=%+v", got[0].MV, got[1].MV)
	}
	if got[2].MV.X != 10 || got[2].Weight != 8 {
		t.Fatalf("secondary head = %+v weight=%d, want X=10 weight=8", got[2].MV, got[2].Weight)
	}
	if got[4].MV.X != 8 {
		t.Fatalf("temporal candidate ordering mismatch: got[4]=%+v", got[4].MV)
	}
}

func TestAugmentSingleRefCandidatesTemporalResortsBoostedNearestSection(t *testing.T) {
	base := []RefMVCandidate{
		{MV: MotionVector{X: -8, Y: -72}, Weight: 736},
		{MV: MotionVector{}, Weight: 732},
	}
	temporal := []RefMVCandidate{
		{MV: MotionVector{}, Weight: 32},
	}
	got := AugmentSingleRefCandidatesTemporal(base, temporal)
	if got[0].MV != (MotionVector{}) || got[0].Weight != 764 {
		t.Fatalf("got first=%+v weight=%d, want boosted zero MV first with weight 764", got[0].MV, got[0].Weight)
	}
	if got[1].MV != (MotionVector{X: -8, Y: -72}) || got[1].Weight != 736 {
		t.Fatalf("got second=%+v weight=%d, want displaced spatial MV", got[1].MV, got[1].Weight)
	}
}

func TestTemporalSupportBreaksEqualWeightNearestTie(t *testing.T) {
	cands := []RefMVCandidate{
		{MV: MotionVector{X: 2, Y: -35}, Weight: 740},
		{MV: MotionVector{}, Weight: 740},
	}
	temporal := []RefMVCandidate{
		{MV: MotionVector{X: 1}, Weight: 32},
	}

	preferTemporalSupportedRefMVTies(cands, temporal)

	if got := cands[0].MV; got != (MotionVector{}) {
		t.Fatalf("first candidate=%+v, want temporal-supported zero MV", got)
	}
	if got := cands[1].MV; got != (MotionVector{X: 2, Y: -35}) {
		t.Fatalf("second candidate=%+v, want displaced spatial MV", got)
	}
}

func TestTemporalSupportDoesNotOverrideHigherWeightCandidate(t *testing.T) {
	cands := []RefMVCandidate{
		{MV: MotionVector{X: 18, Y: -10}, Weight: 776},
		{MV: MotionVector{}, Weight: 736},
	}
	temporal := []RefMVCandidate{
		{MV: MotionVector{}, Weight: 32},
	}

	preferTemporalSupportedRefMVTies(cands, temporal)

	if got := cands[0].MV; got != (MotionVector{X: 18, Y: -10}) {
		t.Fatalf("first candidate=%+v, want higher-weight candidate preserved", got)
	}
}

func TestAugmentCompoundCandidatesTemporalPrecedesEqualWeightSecondary(t *testing.T) {
	secondary := CompoundRefMVCandidate{
		MV:     [2]MotionVector{{Y: 64}, {}},
		Weight: 16,
		Flags:  refMVFlagNew,
	}
	temporal := CompoundRefMVCandidate{
		MV:     [2]MotionVector{{}, {}},
		Weight: 16,
	}
	cands := AugmentCompoundCandidatesTemporal([]CompoundRefMVCandidate{secondary}, []CompoundRefMVCandidate{temporal})
	if got := cands[0].MV; got != temporal.MV {
		t.Fatalf("first candidate=%+v, want temporal before equal-weight secondary", got)
	}
	if got := cands[1].MV; got != secondary.MV {
		t.Fatalf("second candidate=%+v, want secondary after temporal", got)
	}
}

func TestSortCompoundCandidateSectionPreservesEqualWeightOrder(t *testing.T) {
	cands := []CompoundRefMVCandidate{
		{MV: [2]MotionVector{{}, {X: 256}}, Weight: 664, Flags: refMVFlagNew},
		{MV: [2]MotionVector{{Y: -64}, {}}, Weight: 664},
		{MV: [2]MotionVector{{}, {X: 128}}, Weight: 644, Flags: refMVFlagNew},
	}
	sortCompoundCandidateSection(cands)
	if got := cands[0].MV; got != ([2]MotionVector{{}, {X: 256}}) {
		t.Fatalf("first candidate=%+v, want equal-weight insertion order preserved", got)
	}
	if got := cands[1].MV; got != ([2]MotionVector{{Y: -64}, {}}) {
		t.Fatalf("second candidate=%+v, want equal-weight insertion order preserved", got)
	}
}

func TestFindCompoundRefCandidatesMergesTemporalBeforeSecondarySort(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 12, End4Y: 8})
	target := BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 6, Size: Block16x8}
	zero := [2]MotionVector{{}, {}}
	near := [2]MotionVector{{Y: -1}, {Y: 1}}

	tile.SplatCompoundRef(BlockGeometry{Start4X: 4, Start4Y: 2, End4X: 8, End4Y: 4, Size: Block16x8},
		0, 6, 0, 1, zero[0], zero[1], InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 4, End4X: 4, End4Y: 6, Size: Block16x8},
		0, 6, 0, 1, near[0], near[1], InterPredNew, InterPredNew, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block16x8},
		0, 6, 0, 1, near[0], near[1], InterPredNew, InterPredNew, defaultInterFilterPair())

	cands, _ := tile.findCompoundRefCandidatesBaseWithTemporalInto(nil, av1.SequenceHeader{}, nil, &obu.FrameHeader{Width: 48, Height: 32, UseRefFrameMvs: true}, target, 0, 6, []CompoundRefMVCandidate{
		{MV: zero, Weight: 4},
	})
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want at least 2", len(cands))
	}
	if cands[0].MV != zero {
		t.Fatalf("first candidate=%+v, want temporal-supported zero before secondary tie; candidates=%+v", cands[0], cands)
	}
	if cands[1].MV != near {
		t.Fatalf("second candidate=%+v, want secondary candidate %+v; candidates=%+v", cands[1], near, cands)
	}
	if cands[0].Weight != cands[1].Weight {
		t.Fatalf("weights=%d/%d, want temporal/secondary tie", cands[0].Weight, cands[1].Weight)
	}
}

func TestFindCompoundRefCandidatesUsesNominalWidthForRightEdgeWeights(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 14, End4Y: 16})
	target := BlockGeometry{Start4X: 8, Start4Y: 8, End4X: 14, End4Y: 16, Size: Block32x32}
	zero := [2]MotionVector{{}, {}}
	left := [2]MotionVector{{X: -2}, {}}

	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8, Size: Block32x32},
		0, 4, 0, 4, zero[0], zero[1], InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 8, Start4Y: 0, End4X: 14, End4Y: 8, Size: Block32x32},
		0, 4, 0, 4, zero[0], zero[1], InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 8, End4X: 8, End4Y: 16, Size: Block32x32},
		0, 4, 0, 4, left[0], left[1], InterPredNew, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.FindCompoundRefCandidates(nil, target, 0, 4)
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want at least 2", len(cands))
	}
	if cands[0].MV != zero {
		t.Fatalf("first candidate=%+v, want nominal-width top candidate %+v; candidates=%+v", cands[0], zero, cands)
	}
	if cands[0].Weight != 692 || cands[1].Weight != 688 {
		t.Fatalf("weights=%d/%d, want nominal-width weights 692/688; candidates=%+v", cands[0].Weight, cands[1].Weight, cands)
	}
}

func TestCompoundExtendedCandidatesPreserveDuplicateSameRefMVs(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 12, End4Y: 12})
	target := BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8, Size: Block16x16}
	tile.SplatCompoundRef(BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 8, End4Y: 4, Size: Block16x16},
		0, 6, 0, 3, MotionVector{}, MotionVector{X: 128}, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 4, End4X: 4, End4Y: 8, Size: Block16x16},
		0, 6, 0, 3, MotionVector{}, MotionVector{X: 256}, InterPredNearest, InterPredNearest, defaultInterFilterPair())

	parsed := &ParsedFrame{}
	cands := augmentCompoundCandidatesExtended(
		[]CompoundRefMVCandidate{{MV: [2]MotionVector{{}, {}}, Weight: 22}},
		tile, av1.SequenceHeader{}, parsed, &obu.FrameHeader{Width: 64, Height: 64}, target, 0, 1,
	)
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want extended candidate", len(cands))
	}
	if got, want := cands[1].MV, ([2]MotionVector{{}, {X: 128}}); got != want {
		t.Fatalf("extended candidate=%+v, want duplicate same-ref MV to preserve first ref %+v", got, want)
	}
}

func TestCompoundExtendedCandidatesPreserveDuplicateSlotsWithoutBasePair(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 12, End4Y: 12})
	target := BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8, Size: Block16x16}
	mv := MotionVector{X: -24, Y: -2}
	tile.SplatSingleRef(BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 8, End4Y: 4, Size: Block16x16},
		0, 0, mv, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 4, End4X: 4, End4Y: 8, Size: Block16x16},
		0, 0, mv, InterPredNearest, defaultInterFilterPair())

	parsed := &ParsedFrame{}
	cands := augmentCompoundCandidatesExtended(nil, tile, av1.SequenceHeader{}, parsed, &obu.FrameHeader{Width: 64, Height: 64}, target, 0, 6)
	if len(cands) != 2 {
		t.Fatalf("len(cands)=%d, want two duplicate extended slots: %+v", len(cands), cands)
	}
	want := [2]MotionVector{mv, mv}
	if cands[0].MV != want || cands[1].MV != want {
		t.Fatalf("extended slots=%+v/%+v, want duplicate %+v", cands[0].MV, cands[1].MV, want)
	}
}

func TestAugmentCompoundCandidatesExtendedRunsAfterTemporal(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 64, End4Y: 128})
	target := BlockGeometry{Start4X: 16, Start4Y: 64, End4X: 20, End4Y: 80, Size: Block16x64}
	tile.SplatCompoundRef(BlockGeometry{Start4X: 16, Start4Y: 60, End4X: 20, End4Y: 64, Size: Block16x16},
		0, 1, 0, 1, MotionVector{X: -128}, MotionVector{X: -128}, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 12, Start4Y: 64, End4X: 16, End4Y: 80, Size: Block16x64},
		0, 1, 0, 1, MotionVector{X: 64}, MotionVector{X: 64}, InterPredNearest, InterPredNearest, defaultInterFilterPair())
	parsed := &ParsedFrame{}
	parsed.Header.Width = 256
	parsed.Header.Height = 512

	got := augmentCompoundCandidatesExtended([]CompoundRefMVCandidate{
		{MV: [2]MotionVector{{}, {}}, Weight: 16},
	}, tile, av1.SequenceHeader{}, parsed, &parsed.Header, target, 0, 1)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want temporal plus one extended candidate: %+v", len(got), got)
	}
	if got[0].MV != [2]MotionVector{} || got[0].Weight != 16 {
		t.Fatalf("temporal candidate changed: %+v", got[0])
	}
	want := [2]MotionVector{{X: -128}, {X: -128}}
	if got[1].MV != want {
		t.Fatalf("extended candidate=%+v, want first extended %+v", got[1].MV, want)
	}
}

func TestFindSingleRefCandidatesPreservesRawSpatialMVs(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 0, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block16x8}
	leftAbove := BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2, Size: Block8x8}
	rightAbove := BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8}
	tile.SplatSingleRef(leftAbove, 0, 0, MotionVector{X: 5, Y: -3}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(rightAbove, 0, 0, MotionVector{X: 4, Y: -2}, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.FindSingleRefCandidates(&obu.FrameHeader{HighPrecisionMV: false}, target, 0)
	if got := activeRefMVCandidateCount(cands); got != 2 {
		t.Fatalf("activeRefMVCandidateCount=%d, want 2", got)
	}
	if got, want := cands[0].MV, (MotionVector{X: 5, Y: -3}); got != want {
		t.Fatalf("first candidate=%+v, want %+v", got, want)
	}
	if got, want := cands[1].MV, (MotionVector{X: 4, Y: -2}); got != want {
		t.Fatalf("second candidate=%+v, want %+v", got, want)
	}
	if got, want := cands[0].Weight, cands[1].Weight; got != want {
		t.Fatalf("weights=%d/%d, want equal unmerged spatial contributions", got, want)
	}
}

func TestFindSingleRefCandidatesKeepsForceIntegerSpatialSeparateFromTemporal(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 0, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block16x8}
	above := BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block16x8}
	tile.SplatSingleRef(above, 0, 0, MotionVector{X: 50}, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.findSingleRefCandidatesWithEdgeAndTemporalInto(nil, &obu.FrameHeader{ForceIntegerMV: true, UseRefFrameMvs: true}, target, 0, fullIntraEdgeAvailability, []RefMVCandidate{
		{MV: MotionVector{X: 48}, Weight: 4},
	})
	if got := activeRefMVCandidateCount(cands); got != 2 {
		t.Fatalf("activeRefMVCandidateCount=%d, want 2; candidates=%+v", got, cands)
	}
	if got, want := cands[0].MV, (MotionVector{X: 50}); got != want {
		t.Fatalf("spatial candidate=%+v, want raw %+v", got, want)
	}
	if got, want := cands[1].MV, (MotionVector{X: 48}); got != want {
		t.Fatalf("temporal candidate=%+v, want distinct %+v", got, want)
	}
}

func TestFindSingleRefCandidatesKeepsNearestTieInsertionOrder(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 12, End4Y: 12})
	target := BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8, Size: Block16x16}
	above := BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 8, End4Y: 4, Size: Block16x16}
	left := BlockGeometry{Start4X: 0, Start4Y: 4, End4X: 4, End4Y: 8, Size: Block16x16}
	tile.SplatSingleRef(above, 0, 0, MotionVector{X: 50}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(left, 0, 0, MotionVector{X: 48}, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.findSingleRefCandidatesWithEdgeAndTemporalInto(nil, &obu.FrameHeader{UseRefFrameMvs: true}, target, 0, fullIntraEdgeAvailability, []RefMVCandidate{
		{MV: MotionVector{X: 46}, Weight: 2},
	})
	if got, want := activeRefMVCandidateCount(cands), 3; got != want {
		t.Fatalf("activeRefMVCandidateCount=%d, want %d; candidates=%+v", got, want, cands)
	}
	if got, want := cands[0].MV, (MotionVector{X: 50}); got != want {
		t.Fatalf("first nearest candidate=%+v, want top candidate %+v", got, want)
	}
	if got, want := cands[1].MV, (MotionVector{X: 48}); got != want {
		t.Fatalf("second nearest candidate=%+v, want left candidate %+v", got, want)
	}
	if cands[0].Weight != cands[1].Weight {
		t.Fatalf("nearest weights=%d/%d, want equal tie", cands[0].Weight, cands[1].Weight)
	}
}

func TestFindSingleRefCandidatesUsesFullBlockDimsForLargeBlocks(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 96, End4Y: 96})
	target := BlockGeometry{Start4X: 32, Start4Y: 32, End4X: 64, End4Y: 64, Size: Block128x128}
	topMV := MotionVector{X: 144, Y: 2}
	leftMV := MotionVector{X: 104, Y: -2}
	tile.SplatSingleRef(BlockGeometry{Start4X: 32, Start4Y: 0, End4X: 64, End4Y: 32, Size: Block128x128},
		0, 0, topMV, InterPredNew, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 32, End4X: 32, End4Y: 64, Size: Block128x128},
		0, 0, leftMV, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 32, End4Y: 32, Size: Block128x128},
		0, 0, leftMV, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 64, Start4Y: 16, End4X: 80, End4Y: 32, Size: Block64x64},
		0, 0, topMV, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.FindSingleRefCandidatesWithEdge(&obu.FrameHeader{HighPrecisionMV: false}, target, 0, fullIntraEdgeAvailability)
	if got, want := activeRefMVCandidateCount(cands), 2; got != want {
		t.Fatalf("activeRefMVCandidateCount=%d, want %d; candidates=%+v", got, want, cands)
	}
	if got, want := cands[0].MV, leftMV; got != want {
		t.Fatalf("cand0=%+v, want left/top-left candidate %+v; candidates=%+v", got, want, cands)
	}
	if got, want := cands[1].MV, topMV; got != want {
		t.Fatalf("cand1=%+v, want top candidate %+v; candidates=%+v", got, want, cands)
	}
	if cands[0].Weight != 836 || cands[1].Weight != 832 {
		t.Fatalf("weights=%d/%d, want full-block weights 836/832; candidates=%+v", cands[0].Weight, cands[1].Weight, cands)
	}
}

func TestFindCompoundRefCandidatesUsesFullBlockDimsForLargeBlocks(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 96, End4Y: 96})
	target := BlockGeometry{Start4X: 32, Start4Y: 32, End4X: 64, End4Y: 64, Size: Block128x128}
	top := [2]MotionVector{{X: 144, Y: 2}, {X: -20, Y: 4}}
	left := [2]MotionVector{{X: 104, Y: -2}, {X: -24, Y: 8}}
	tile.SplatCompoundRef(BlockGeometry{Start4X: 32, Start4Y: 0, End4X: 64, End4Y: 32, Size: Block128x128},
		0, 1, 0, 1, top[0], top[1], InterPredNew, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 32, End4X: 32, End4Y: 64, Size: Block128x128},
		0, 1, 0, 1, left[0], left[1], InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 32, End4Y: 32, Size: Block128x128},
		0, 1, 0, 1, left[0], left[1], InterPredNearest, InterPredNearest, defaultInterFilterPair())
	tile.SplatCompoundRef(BlockGeometry{Start4X: 64, Start4Y: 16, End4X: 80, End4Y: 32, Size: Block64x64},
		0, 1, 0, 1, top[0], top[1], InterPredNearest, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.FindCompoundRefCandidates(&obu.FrameHeader{HighPrecisionMV: false, Width: 384, Height: 384}, target, 0, 1)
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want at least 2; candidates=%+v", len(cands), cands)
	}
	if got, want := cands[0].MV, left; got != want {
		t.Fatalf("cand0=%+v, want left/top-left candidate %+v; candidates=%+v", got, want, cands)
	}
	if got, want := cands[1].MV, top; got != want {
		t.Fatalf("cand1=%+v, want top candidate %+v; candidates=%+v", got, want, cands)
	}
	if cands[0].Weight != 836 || cands[1].Weight != 832 {
		t.Fatalf("weights=%d/%d, want full-block weights 836/832; candidates=%+v", cands[0].Weight, cands[1].Weight, cands)
	}
}

func TestFindSingleRefCandidatesParsedAppendsSameSignNeighborRef(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	neighbor := BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8}
	tile.SplatSingleRef(neighbor, 2, 0, MotionVector{X: 8, Y: -4}, InterPredNearest, defaultInterFilterPair())

	seq := av1.SequenceHeader{
		EnableOrderHint:     true,
		OrderHintBitsMinus1: 7,
	}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 20
	parsed.RefFramePOC[1] = 12
	parsed.RefFramePOC[2] = 10

	cands, _ := tile.FindSingleRefCandidatesParsed(seq, parsed, &parsed.Header, target, 1)
	if got := activeRefMVCandidateCount(cands); got == 0 {
		t.Fatal("expected parsed single-ref candidates")
	}
	found := false
	for _, cand := range cands {
		if cand.Weight > 0 && cand.MV == (MotionVector{X: 8, Y: -4}) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected same-sign neighbor-ref candidate in %+v", cands)
	}
}

func TestSingleRefExtendedCandidatesKeepMultipleNeighborRefs(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 80, End4Y: 80})
	target := BlockGeometry{Start4X: 50, Start4Y: 56, End4X: 52, End4Y: 58, Size: Block8x8}
	above := BlockGeometry{Start4X: 50, Start4Y: 54, End4X: 52, End4Y: 56, Size: Block8x8}
	tile.SplatCompoundRef(
		above,
		0,
		6,
		0,
		6,
		MotionVector{Y: 2, X: 8},
		MotionVector{Y: -1, X: -8},
		InterPredNearest,
		InterPredNearest,
		defaultInterFilterPair(),
	)

	seq := av1.SequenceHeader{EnableOrderHint: true, OrderHintBitsMinus1: 7}
	parsed := &ParsedFrame{}
	parsed.Header.FrameOffset = 28
	parsed.RefFramePOC[0] = 25
	parsed.RefFramePOC[1] = 27
	parsed.RefFramePOC[6] = 32

	cands := augmentSingleRefCandidatesExtended([]RefMVCandidate{
		{MV: MotionVector{}, Weight: 8},
	}, tile, seq, parsed, &parsed.Header, target, 1)

	if got, want := activeRefMVCandidateCount(cands), 3; got != want {
		t.Fatalf("activeRefMVCandidateCount=%d, want %d; candidates=%+v", got, want, cands)
	}
	if got, want := cands[1].MV, (MotionVector{Y: 2, X: 8}); got != want {
		t.Fatalf("extended candidate 1=%+v, want %+v; candidates=%+v", got, want, cands)
	}
	if got, want := cands[2].MV, (MotionVector{Y: 1, X: 8}); got != want {
		t.Fatalf("extended candidate 2=%+v, want %+v; candidates=%+v", got, want, cands)
	}
}

func TestFindSingleRefCandidatesCountsCompoundNewFlagForContext(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	neighbor := BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8}
	tile.SplatCompoundRef(
		neighbor,
		0,
		1,
		0,
		1,
		MotionVector{X: 4, Y: 2},
		MotionVector{X: 12, Y: -4},
		InterPredNearest,
		InterPredNew,
		defaultInterFilterPair(),
	)

	_, ctx := tile.FindSingleRefCandidates(&obu.FrameHeader{HighPrecisionMV: false}, target, 0)
	if got, want := ctx&7, 2; got != want {
		t.Fatalf("newmv_ctx=%d, want %d", got, want)
	}
}

func TestFindSingleRefCandidatesAppliesTemporalBeforeNearestSort(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 32, End4Y: 32})
	target := BlockGeometry{Start4X: 0, Start4Y: 8, End4X: 16, End4Y: 24, Size: Block64x64}
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 4, End4X: 4, End4Y: 8, Size: Block16x16}, 0, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 12, End4Y: 8, Size: Block32x16}, 0, 0, MotionVector{X: -248}, InterPredNew, defaultInterFilterPair())

	cands, _ := tile.findSingleRefCandidatesWithEdgeAndTemporalInto(nil, &obu.FrameHeader{HighPrecisionMV: false, UseRefFrameMvs: true}, target, 0, fullIntraEdgeAvailability, []RefMVCandidate{
		{MV: MotionVector{}, Weight: 16},
	})
	if len(cands) < 2 {
		t.Fatalf("candidates=%v, want at least 2", cands)
	}
	if got := cands[0].MV; got != (MotionVector{}) {
		t.Fatalf("cand0=%+v, want zero MV; candidates=%+v", got, cands)
	}
	if got := cands[1].MV; got != (MotionVector{X: -248}) {
		t.Fatalf("cand1=%+v, want temporal tie to preserve dav1d order", got)
	}
	if cands[0].Weight != cands[1].Weight {
		t.Fatalf("weights=%d/%d, want tie", cands[0].Weight, cands[1].Weight)
	}
}

func TestAppendOrAccumulateRefMVCandidateCapsAtEight(t *testing.T) {
	cands := make([]RefMVCandidate, maxRefMVStackSize)
	for i := range cands {
		cands[i] = RefMVCandidate{MV: MotionVector{X: i + 1}, Weight: 1}
	}
	cands = appendOrAccumulateRefMVCandidate(cands, RefMVCandidate{MV: MotionVector{X: 99}, Weight: 4})
	if len(cands) != maxRefMVStackSize {
		t.Fatalf("len(cands)=%d, want cap %d", len(cands), maxRefMVStackSize)
	}
	cands = appendOrAccumulateRefMVCandidate(cands, RefMVCandidate{MV: MotionVector{X: 3}, Weight: 4})
	if got, want := cands[2].Weight, 5; got != want {
		t.Fatalf("accumulated weight=%d, want %d", got, want)
	}
}

func TestAddSpatialCandidateCapsAtEightButUpdatesContext(t *testing.T) {
	cands := make([]RefMVCandidate, maxRefMVStackSize)
	for i := range cands {
		cands[i] = RefMVCandidate{MV: MotionVector{X: i + 1}, Weight: 1}
	}
	block := &SpatialMVBlock{
		Valid: true,
		Ref:   [2]int8{0, -1},
		MV:    [2]MotionVector{{X: 99}, {}},
		Flags: refMVFlagNew,
	}
	var haveNew, haveRef int
	(&SpatialMVTile{}).addSpatialCandidate(nil, &cands, 4, block, 0, MotionVector{}, false, &haveNew, &haveRef)
	if len(cands) != maxRefMVStackSize {
		t.Fatalf("len(cands)=%d, want cap %d", len(cands), maxRefMVStackSize)
	}
	if haveNew != 1 || haveRef != 1 {
		t.Fatalf("context flags haveNew=%d haveRef=%d, want 1/1", haveNew, haveRef)
	}
}

func TestFindSingleRefCandidatesCountsTopRightNewFlagForContext(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	topRight := BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 6, End4Y: 2, Size: Block8x8}
	tile.SplatSingleRef(topRight, 0, 0, MotionVector{X: 4}, InterPredNew, defaultInterFilterPair())

	_, ctx := tile.FindSingleRefCandidatesWithEdge(&obu.FrameHeader{HighPrecisionMV: false}, target, 0, fullIntraEdgeAvailability)
	if got, want := ctx&7, 2; got != want {
		t.Fatalf("newmv_ctx=%d, want %d", got, want)
	}
}

func TestFindSingleRefCandidatesPadsWithGlobalMotion(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	hdr := &obu.FrameHeader{Width: 32, Height: 32}
	hdr.GMV[0] = obu.WarpedMotionParams{
		Type: 2,
		Matrix: [6]int32{
			1 << 16, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}

	cands, _ := tile.FindSingleRefCandidates(hdr, target, 0)
	if len(cands) < 2 {
		t.Fatalf("len(cands)=%d, want >=2", len(cands))
	}
	if got, want := cands[0].MV, (MotionVector{X: 8, Y: 0}); got != want {
		t.Fatalf("first padded candidate=%+v, want %+v", got, want)
	}
	if got, want := cands[1].MV, (MotionVector{X: 8, Y: 0}); got != want {
		t.Fatalf("second padded candidate=%+v, want %+v", got, want)
	}
}

func TestFindSingleRefCandidatesWithEdgeGatesTopRightCandidate(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	topRight := BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 6, End4Y: 2, Size: Block8x8}
	tile.SplatSingleRef(topRight, 0, 0, MotionVector{X: 24, Y: -8}, InterPredNearest, defaultInterFilterPair())

	cands, _ := tile.FindSingleRefCandidatesWithEdge(&obu.FrameHeader{HighPrecisionMV: false}, target, 0, intraEdgeAvailability{
		topHasRight:   false,
		leftHasBottom: true,
	})
	if got, want := cands[0].MV, (MotionVector{}); got != want {
		t.Fatalf("first candidate=%+v, want gated top-right to be ignored", got)
	}

	cands, _ = tile.FindSingleRefCandidatesWithEdge(&obu.FrameHeader{HighPrecisionMV: false}, target, 0, fullIntraEdgeAvailability)
	if got, want := cands[0].MV, (MotionVector{X: 24, Y: -8}); got != want {
		t.Fatalf("first candidate=%+v, want %+v when top-right is available", got, want)
	}
}
