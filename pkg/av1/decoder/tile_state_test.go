package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestRootBlockLevelAndSize(t *testing.T) {
	if got := RootBlockLevel(av1.SequenceHeader{Use128x128SuperBlock: true}); got != BlockLevel128x128 {
		t.Fatalf("RootBlockLevel(128) = %d, want %d", got, BlockLevel128x128)
	}
	if got := RootBlockLevel(av1.SequenceHeader{}); got != BlockLevel64x64 {
		t.Fatalf("RootBlockLevel(64) = %d, want %d", got, BlockLevel64x64)
	}
	if got := RootBlockSize(av1.SequenceHeader{Use128x128SuperBlock: true}); got != Block128x128 {
		t.Fatalf("RootBlockSize(128) = %d, want %d", got, Block128x128)
	}
	if got := RootBlockSize(av1.SequenceHeader{}); got != Block64x64 {
		t.Fatalf("RootBlockSize(64) = %d, want %d", got, Block64x64)
	}
}

func TestBuildTileStatesSyntheticSpbtvGeometry(t *testing.T) {
	seq := av1.SequenceHeader{Use128x128SuperBlock: true}
	hdr := &obu.FrameHeader{
		Width:         960,
		UpscaledWidth: 960,
		Height:        540,
		Tiling: obu.Tiling{
			Cols:      8,
			Rows:      1,
			SizeBytes: 1,
			ColStartSB: [65]uint16{
				0, 1, 2, 3, 4, 5, 6, 7, 8,
			},
			RowStartSB: [65]uint16{
				0, 5,
			},
		},
	}
	group := &obu.TileGroup{}
	for i := 0; i < 8; i++ {
		group.Tiles = append(group.Tiles, obu.Tile{
			Index: i,
			Data:  []byte{0xff},
		})
	}

	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	t.Cleanup(func() { CloseTileStates(states) })
	if len(states) != 8 {
		t.Fatalf("tile count = %d, want 8", len(states))
	}
	if states[0].CDF == nil {
		t.Fatal("first tile CDF is nil")
	}
	if states[0].Coef == nil {
		t.Fatal("first tile coefficient CDF is nil")
	}
	if states[0].CoefByQ[0] == nil {
		t.Fatal("first tile active coefficient qcat state is nil")
	}
	if states[0].MV == nil {
		t.Fatal("first tile mv CDF is nil")
	}
	states[0].CDF.Skip[0][0] = 1
	if got := states[1].CDF.Skip[0][0]; got == 1 {
		t.Fatal("tile CDF state is shared across tiles")
	}
	states[0].Coef.Skip[0][0][0] = 1
	if got := states[1].Coef.Skip[0][0][0]; got == 1 {
		t.Fatal("tile coefficient CDF state is shared across tiles")
	}
	states[0].SetActiveCoefQIndex(121)
	states[1].SetActiveCoefQIndex(121)
	states[0].CoefByQ[3].Skip[0][0][0] = 2
	if got := states[1].CoefByQ[3].Skip[0][0][0]; got == 2 {
		t.Fatal("tile coefficient qcat state is shared across tiles")
	}
	states[0].MV.Joint[0] = 1
	if got := states[1].MV.Joint[0]; got == 1 {
		t.Fatal("tile mv CDF state is shared across tiles")
	}

	first := states[0].Geometry
	if first.StartX != 0 || first.EndX != 128 || first.StartY != 0 || first.EndY != 540 {
		t.Fatalf("first tile bounds = (%d,%d)-(%d,%d), want (0,0)-(128,540)", first.StartX, first.StartY, first.EndX, first.EndY)
	}
	if len(first.SuperBlocks) != 5 {
		t.Fatalf("first tile superblocks = %d, want 5", len(first.SuperBlocks))
	}
	if got, want := first.End4Y, 136; got != want {
		t.Fatalf("first tile mi-grid End4Y = %d, want AV1 8x8-aligned value %d", got, want)
	}

	last := states[7].Geometry
	if last.StartX != 896 || last.EndX != 960 {
		t.Fatalf("last tile x bounds = (%d,%d), want (896,960)", last.StartX, last.EndX)
	}
	if got, want := last.SuperBlocks[4].Height4, 8; got != want {
		t.Fatalf("last tile final sb height4 = %d, want nominal 8x8-aligned %d", got, want)
	}
	if got, want := last.SuperBlocks[4].HeightPx, 28; got != want {
		t.Fatalf("last tile final sb height = %d, want %d", got, want)
	}
}

func TestBuildTileStatesRejectsEmptyTileGroup(t *testing.T) {
	_, err := BuildTileStates(av1.SequenceHeader{}, &obu.FrameHeader{}, &obu.TileGroup{})
	if err == nil {
		t.Fatal("expected error for empty tile group")
	}
}

func TestBuildTileStatesWithInitialCoefQCatUsesReferenceBucket(t *testing.T) {
	hdr := &obu.FrameHeader{
		Width:         64,
		UpscaledWidth: 64,
		Height:        64,
		Quantization:  obu.Quantization{YAC: 160},
		Tiling: obu.Tiling{
			Cols:       1,
			Rows:       1,
			ColStartSB: [65]uint16{0, 1},
			RowStartSB: [65]uint16{0, 1},
		},
	}
	group := &obu.TileGroup{Tiles: []obu.Tile{{Data: []byte{0xff}}}}

	states, err := BuildTileStatesWithInitialCoefQCat(av1.SequenceHeader{}, hdr, group, 2)
	if err != nil {
		t.Fatalf("BuildTileStatesWithInitialCoefQCat: %v", err)
	}
	t.Cleanup(func() { CloseTileStates(states) })

	if got, want := states[0].Coef.QCat, 2; got != want {
		t.Fatalf("active coefficient qcat = %d, want %d", got, want)
	}
	if got, want := states[0].Coef.Skip[1][0][0], uint16(32768-31957); got != want {
		t.Fatalf("skip cdf seed = %d, want %d", got, want)
	}
}

func TestBuildTileStatesHonorsDebugDisableCDFUpdate(t *testing.T) {
	t.Setenv("DEBUG_FORCE_DISABLE_CDF_UPDATE", "1")

	states, err := BuildTileStates(av1.SequenceHeader{}, &obu.FrameHeader{
		Width:  64,
		Height: 64,
		Tiling: obu.Tiling{
			Cols:      1,
			Rows:      1,
			SizeBytes: 1,
			ColStartSB: [65]uint16{
				0, 1,
			},
			RowStartSB: [65]uint16{
				0, 1,
			},
		},
	}, &obu.TileGroup{
		Tiles: []obu.Tile{{Index: 0, Data: []byte{0xff}}},
	})
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	t.Cleanup(func() { CloseTileStates(states) })

	cdf := []uint16{16384, 0}
	if bit := states[0].Entropy.DecodeBoolAdapt(cdf); bit != 1 {
		t.Fatalf("DecodeBoolAdapt bit = %d, want 1", bit)
	}
	if got, want := cdf[0], uint16(16384); got != want {
		t.Fatalf("cdf[0] = %d, want %d when debug disable is forced", got, want)
	}
	if got, want := cdf[1], uint16(0); got != want {
		t.Fatalf("cdf[1] = %d, want %d when debug disable is forced", got, want)
	}
}

func TestCloseTileStatesReleasesPooledState(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{
		Width:  64,
		Height: 64,
		Tiling: obu.Tiling{
			Cols:      1,
			Rows:      1,
			SizeBytes: 1,
			ColStartSB: [65]uint16{
				0, 1,
			},
			RowStartSB: [65]uint16{
				0, 1,
			},
		},
	}
	states, err := BuildTileStates(seq, hdr, &obu.TileGroup{
		Tiles: []obu.Tile{{Index: 0, Data: []byte{0xff}}},
	})
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("tile count = %d, want 1", len(states))
	}
	if states[0].CDF == nil || states[0].MV == nil || states[0].Coef == nil {
		t.Fatal("expected populated tile state before close")
	}
	if len(states[0].Geometry.SuperBlocks) == 0 {
		t.Fatal("expected populated geometry before close")
	}

	CloseTileStates(states)

	if states[0].CDF != nil || states[0].MV != nil || states[0].Coef != nil {
		t.Fatal("tile state CDF resources not cleared by close")
	}
	if states[0].superBlocksScratch != nil {
		t.Fatal("tile superblock scratch not released by close")
	}
	if states[0].Entropy != nil || states[0].Data != nil {
		t.Fatal("tile transient decode state not cleared by close")
	}
	if states[0].Geometry.SuperBlocks != nil {
		t.Fatal("tile geometry superblocks not cleared by close")
	}
}

func TestTileStateSetActiveCoefQIndexSelectsQCatBucket(t *testing.T) {
	state := TileState{}
	for qcat := range state.CoefByQ {
		state.CoefByQ[qcat] = NewDefaultCoefCDFForQCat(qcat)
		state.CoefByQ[qcat].Skip[0][0][0] = uint16(qcat + 1)
	}

	state.SetActiveCoefQIndex(10)
	if got := state.Coef.Skip[0][0][0]; got != 1 {
		t.Fatalf("qidx=10 active skip cdf = %d, want 1", got)
	}
	state.SetActiveCoefQIndex(40)
	if got := state.Coef.Skip[0][0][0]; got != 2 {
		t.Fatalf("qidx=40 active skip cdf = %d, want 2", got)
	}
	state.SetActiveCoefQIndex(115)
	if got := state.Coef.Skip[0][0][0]; got != 3 {
		t.Fatalf("qidx=115 active skip cdf = %d, want 3", got)
	}
	state.SetActiveCoefQIndex(255)
	if got := state.Coef.Skip[0][0][0]; got != 4 {
		t.Fatalf("qidx=255 active skip cdf = %d, want 4", got)
	}
}

func TestTileStateSetActiveCoefQIndexHonorsDebugOverride(t *testing.T) {
	t.Setenv("DEBUG_FORCE_COEF_QCAT", "1")
	state := TileState{}
	for qcat := range state.CoefByQ {
		state.CoefByQ[qcat] = NewDefaultCoefCDFForQCat(qcat)
		state.CoefByQ[qcat].Skip[0][0][0] = uint16(qcat + 1)
	}

	state.SetActiveCoefQIndex(255)
	if got := state.Coef.Skip[0][0][0]; got != 2 {
		t.Fatalf("forced qcat active skip cdf = %d, want 2", got)
	}
}
