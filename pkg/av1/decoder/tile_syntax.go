package decoder

import (
	"fmt"
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type TileSyntaxState struct {
	Tile         TileState
	Above        []BlockContext
	aboveScratch *scratchBuffer[BlockContext]
	rowWidthSB   int
	Keyframe     bool
	Pass         int
	LastQIdx     int
	LastLF       [4]int8
	ReadDeltas   bool
	CurrentDQ    DequantTables
}

type TileRowSyntaxState struct {
	Tile        *TileSyntaxState
	SBRow       int
	Left        BlockContext
	SuperBlocks []SuperBlockGeometry
}

var tileBlockContextScratchPool sync.Pool

func NewTileSyntaxState(seq av1.SequenceHeader, hdr *obu.FrameHeader, tile TileState, pass int) (*TileSyntaxState, error) {
	if hdr == nil {
		return nil, fmt.Errorf("decoder: frame header required")
	}

	start, end := tileContextColumnRange(seq, tile.Geometry)
	if end < start {
		return nil, fmt.Errorf("decoder: invalid tile context range [%d,%d)", start, end)
	}
	aboveScratch := takeScratch[BlockContext](&tileBlockContextScratchPool, end-start)

	state := &TileSyntaxState{
		Tile:         tile,
		Above:        aboveScratch.buf[:end-start],
		aboveScratch: aboveScratch,
		rowWidthSB:   maxIntLocal(0, tile.Geometry.EndSBX-tile.Geometry.StartSBX),
		Keyframe:     isKeyOrIntraFrame(hdr.FrameType),
		Pass:         pass,
		LastQIdx:     int(hdr.Quantization.YAC),
	}
	if state.Tile.Coef == nil {
		state.Tile.SetActiveCoefQIndex(state.LastQIdx)
	}
	state.CurrentDQ = InitDequantTables(seq, hdr, state.LastQIdx)
	for i := range state.Above {
		state.Above[i].Reset(state.Keyframe, pass)
	}
	return state, nil
}

func (s *TileSyntaxState) NewRowState(sbRow int) (*TileRowSyntaxState, error) {
	row := &TileRowSyntaxState{}
	if err := s.ResetRowState(row, sbRow); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TileSyntaxState) ResetRowState(row *TileRowSyntaxState, sbRow int) error {
	if s == nil {
		return fmt.Errorf("decoder: tile syntax state required")
	}
	if row == nil {
		return fmt.Errorf("decoder: tile row syntax state required")
	}
	if sbRow < s.Tile.Geometry.StartSBY || sbRow >= s.Tile.Geometry.EndSBY {
		return fmt.Errorf("decoder: superblock row %d out of tile bounds [%d,%d)", sbRow, s.Tile.Geometry.StartSBY, s.Tile.Geometry.EndSBY)
	}

	row.Tile = s
	row.SBRow = sbRow
	row.Left.Reset(s.Keyframe, s.Pass)
	row.SuperBlocks = s.superBlocksForRow(sbRow)
	return nil
}

func (s *TileSyntaxState) superBlocksForRow(sbRow int) []SuperBlockGeometry {
	if s == nil || s.rowWidthSB == 0 {
		return nil
	}
	rowIndex := sbRow - s.Tile.Geometry.StartSBY
	start := rowIndex * s.rowWidthSB
	end := start + s.rowWidthSB
	superBlocks := s.Tile.Geometry.SuperBlocks
	if start < 0 {
		start = 0
	}
	if start > len(superBlocks) {
		start = len(superBlocks)
	}
	if end < start {
		end = start
	}
	if end > len(superBlocks) {
		end = len(superBlocks)
	}
	return superBlocks[start:end]
}

func (s *TileSyntaxState) Close() {
	if s == nil || s.aboveScratch == nil {
		return
	}
	putScratch(&tileBlockContextScratchPool, s.aboveScratch)
	s.aboveScratch = nil
	s.Above = nil
}

func (s *TileSyntaxState) ResetSuperBlockDeltas(hdr *obu.FrameHeader) {
	if s == nil || hdr == nil {
		return
	}
	s.ReadDeltas = hdr.Delta.QPresent
}

func (s *TileSyntaxState) AboveContextIndex(seq av1.SequenceHeader, sbx int) int {
	start, _ := tileContextColumnRange(seq, s.Tile.Geometry)
	if seq.Use128x128SuperBlock {
		return sbx - start
	}
	return (sbx >> 1) - start
}

func tileContextColumnRange(seq av1.SequenceHeader, tile TileGeometry) (int, int) {
	if seq.Use128x128SuperBlock {
		return tile.StartSBX, tile.EndSBX
	}
	return tile.StartSBX >> 1, (tile.EndSBX + 1) >> 1
}

func isKeyOrIntraFrame(ft obu.FrameType) bool {
	return ft == obu.FrameTypeKey || ft == obu.FrameTypeIntra
}
