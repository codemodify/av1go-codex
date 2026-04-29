package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestCalcBlockLoopFilterLevelUsesModeRefAndSegDelta(t *testing.T) {
	lf := obu.LoopFilter{
		ModeRefDeltaEnable: true,
		ModeRefDeltas: obu.ModeRefDeltas{
			Ref:  [8]int8{1, 2, 3, 4, 5, 6, 7, 8},
			Mode: [2]int8{3, 5},
		},
	}
	if got := calcBlockLoopFilterLevel(20, 5, 0, false, lf, false); got != 26 {
		t.Fatalf("intra level=%d, want 26", got)
	}
	if got := calcBlockLoopFilterLevel(20, 5, 3, false, lf, false); got != 34 {
		t.Fatalf("inter non-global level=%d, want 34", got)
	}
	if got := calcBlockLoopFilterLevel(20, 5, 3, true, lf, false); got != 32 {
		t.Fatalf("inter global level=%d, want 32", got)
	}
}

func TestBuildLoopFilterLevelMapUsesCompoundGlobalFlags(t *testing.T) {
	hdr := &obu.FrameHeader{
		Width:  16,
		Height: 16,
		LoopFilter: obu.LoopFilter{
			LevelY:             [2]uint8{20, 20},
			ModeRefDeltaEnable: true,
			ModeRefDeltas: obu.ModeRefDeltas{
				Ref:  [8]int8{0, 0, 0, 4, 0, 0, 0, 0},
				Mode: [2]int8{1, 6},
			},
		},
	}
	field := NewFrameMVField(16, 16)
	field.SplatCompoundRef(
		BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2, Size: Block8x8},
		2, 5, 2, 5,
		MotionVector{X: 4}, MotionVector{Y: -4},
		InterPredGlobal, InterPredNearest,
		defaultInterFilterPair(),
	)
	field.SplatCompoundRef(
		BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8},
		2, 5, 2, 5,
		MotionVector{X: 4}, MotionVector{Y: -4},
		InterPredGlobal, InterPredGlobal,
		defaultInterFilterPair(),
	)
	levels := buildLoopFilterLevelMap(hdr, field, NewSegmentationMap(16, 16))
	if levels == nil {
		t.Fatal("expected loop-filter levels")
	}
	if got := levels.levelAt(0, 0, 0); got != 30 {
		t.Fatalf("mixed compound level=%d, want 30", got)
	}
	if got := levels.levelAt(0, 2, 0); got != 25 {
		t.Fatalf("global compound level=%d, want 25", got)
	}
}

func TestLoopFilterLevelMapVerticalLevelUsesMaxAdjacentCells(t *testing.T) {
	levels := newLoopFilterLevelMap(16, 16)
	levels.Level[0][0] = 12
	levels.Level[0][1] = 28
	if got := levels.verticalLevel(0, 4, 0, 0, 0); got != 28 {
		t.Fatalf("vertical level=%d, want 28", got)
	}
}

