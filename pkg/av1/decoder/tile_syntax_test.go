package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestNewTileSyntaxState128x128(t *testing.T) {
	seq := av1.SequenceHeader{Use128x128SuperBlock: true}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeKey,
		Width:     960,
		Height:    540,
		Tiling: obu.Tiling{
			Cols: 1,
			Rows: 1,
			ColStartSB: [65]uint16{
				0, 8,
			},
			RowStartSB: [65]uint16{
				0, 5,
			},
		},
	}
	states, err := BuildTileStates(seq, hdr, &obu.TileGroup{
		Tiles: []obu.Tile{{Index: 0, Data: []byte{0xff}}},
	})
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}

	syntax, err := NewTileSyntaxState(seq, hdr, states[0], 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}
	if got, want := len(syntax.Above), 8; got != want {
		t.Fatalf("len(Above) = %d, want %d", got, want)
	}
	for i := range syntax.Above {
		if syntax.Above[i].Intra[0] != 1 {
			t.Fatalf("Above[%d] not reset as keyframe context", i)
		}
	}
}

func TestNewTileSyntaxState64x64Uses128ColumnGrouping(t *testing.T) {
	seq := av1.SequenceHeader{Use128x128SuperBlock: false}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeInter,
		Width:     256,
		Height:    128,
		Tiling: obu.Tiling{
			Cols: 1,
			Rows: 1,
			ColStartSB: [65]uint16{
				0, 4,
			},
			RowStartSB: [65]uint16{
				0, 2,
			},
		},
	}
	states, err := BuildTileStates(seq, hdr, &obu.TileGroup{
		Tiles: []obu.Tile{{Index: 0, Data: []byte{0xff}}},
	})
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}

	syntax, err := NewTileSyntaxState(seq, hdr, states[0], 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}
	if got, want := len(syntax.Above), 2; got != want {
		t.Fatalf("len(Above) = %d, want %d", got, want)
	}
	if got := syntax.AboveContextIndex(seq, 3); got != 1 {
		t.Fatalf("AboveContextIndex(3) = %d, want 1", got)
	}
}

func TestTileRowSyntaxStateResetsLeftContext(t *testing.T) {
	seq := av1.SequenceHeader{Use128x128SuperBlock: true}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeKey,
		Width:     128,
		Height:    256,
		Tiling: obu.Tiling{
			Cols: 1,
			Rows: 1,
			ColStartSB: [65]uint16{
				0, 1,
			},
			RowStartSB: [65]uint16{
				0, 2,
			},
		},
	}
	states, err := BuildTileStates(seq, hdr, &obu.TileGroup{
		Tiles: []obu.Tile{{Index: 0, Data: []byte{0xff}}},
	})
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	syntax, err := NewTileSyntaxState(seq, hdr, states[0], 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}

	row, err := syntax.NewRowState(1)
	if err != nil {
		t.Fatalf("NewRowState: %v", err)
	}
	if got, want := len(row.SuperBlocks), 1; got != want {
		t.Fatalf("len(SuperBlocks) = %d, want %d", got, want)
	}
	if row.Left.Intra[0] != 1 {
		t.Fatal("left context was not reset for keyframe row")
	}
}

func TestContextForBlockPreserves128PhaseFor64SuperBlockRows(t *testing.T) {
	seq := av1.SequenceHeader{Use128x128SuperBlock: false}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeKey,
		Width:     256,
		Height:    192,
		Tiling: obu.Tiling{
			Cols: 1,
			Rows: 1,
			ColStartSB: [65]uint16{
				0, 4,
			},
			RowStartSB: [65]uint16{
				0, 3,
			},
		},
	}
	states, err := BuildTileStates(seq, hdr, &obu.TileGroup{
		Tiles: []obu.Tile{{Index: 0, Data: []byte{0xff}}},
	})
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	t.Cleanup(func() { CloseTileStates(states) })
	syntax, err := NewTileSyntaxState(seq, hdr, states[0], 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}
	t.Cleanup(syntax.Close)

	row, err := syntax.NewRowState(1)
	if err != nil {
		t.Fatalf("NewRowState: %v", err)
	}
	g := BlockGeometry{
		Level:   BlockLevel16x16,
		Size:    Block32x16,
		Start4X: 40,
		End4X:   48,
		Start4Y: 16,
		End4Y:   20,
	}
	_, local, err := contextForBlock(seq, row, row.SuperBlocks[2], g)
	if err != nil {
		t.Fatalf("contextForBlock: %v", err)
	}
	if local.Start4X != 8 || local.End4X != 16 {
		t.Fatalf("local X = (%d,%d), want (8,16)", local.Start4X, local.End4X)
	}
	if local.Start4Y != 16 || local.End4Y != 20 {
		t.Fatalf("local Y = (%d,%d), want (16,20)", local.Start4Y, local.End4Y)
	}

	row, err = syntax.NewRowState(2)
	if err != nil {
		t.Fatalf("NewRowState row 2: %v", err)
	}
	g.Start4Y = 32
	g.End4Y = 36
	_, local, err = contextForBlock(seq, row, row.SuperBlocks[2], g)
	if err != nil {
		t.Fatalf("contextForBlock row 2: %v", err)
	}
	if local.Start4Y != 0 || local.End4Y != 4 {
		t.Fatalf("local row 2 Y = (%d,%d), want (0,4)", local.Start4Y, local.End4Y)
	}
}
