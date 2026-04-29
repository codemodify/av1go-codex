package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestClipQIndexRuntimeClampsDeltaQToSpecRange(t *testing.T) {
	if got := clipQIndexRuntime(-4); got != 1 {
		t.Fatalf("clipQIndexRuntime(-4) = %d, want 1", got)
	}
	if got := clipQIndexRuntime(0); got != 1 {
		t.Fatalf("clipQIndexRuntime(0) = %d, want 1", got)
	}
	if got := clipQIndexRuntime(17); got != 17 {
		t.Fatalf("clipQIndexRuntime(17) = %d, want 17", got)
	}
	if got := clipQIndexRuntime(999); got != 255 {
		t.Fatalf("clipQIndexRuntime(999) = %d, want 255", got)
	}
}

func TestTileSyntaxStateResetSuperBlockDeltas(t *testing.T) {
	state := &TileSyntaxState{}
	hdr := &obu.FrameHeader{}

	state.ResetSuperBlockDeltas(hdr)
	if state.ReadDeltas {
		t.Fatal("ReadDeltas = true, want false when delta_q is disabled")
	}

	hdr.Delta.QPresent = true
	state.ResetSuperBlockDeltas(hdr)
	if !state.ReadDeltas {
		t.Fatal("ReadDeltas = false, want true when delta_q is enabled")
	}
}

func TestDebugDeltaQTargetQIndexHonorsOverride(t *testing.T) {
	if got := debugDeltaQTargetQIndex(121); got != 121 {
		t.Fatalf("debugDeltaQTargetQIndex(121) = %d, want 121 without override", got)
	}

	t.Setenv("DEBUG_DELTA_Q_FORCE_QINDEX", "140")
	if got := debugDeltaQTargetQIndex(121); got != 140 {
		t.Fatalf("debugDeltaQTargetQIndex(121) = %d, want 140 with override", got)
	}

	t.Setenv("DEBUG_DELTA_Q_FORCE_QINDEX", "999")
	if got := debugDeltaQTargetQIndex(121); got != 255 {
		t.Fatalf("debugDeltaQTargetQIndex(121) = %d, want 255 with clipped override", got)
	}
}

func TestDecodeDeltaMagnitudeUsesSpecEscapeByDefault(t *testing.T) {
	dec := &scriptedIntraEntropy{symbols: []uint32{2}}
	if got := decodeDeltaMagnitude(generatedDeltaQCDF[:], dec); got != 2 {
		t.Fatalf("decodeDeltaMagnitude(direct) = %d, want 2", got)
	}

	dec = &scriptedIntraEntropy{
		symbols: []uint32{3},
		bits:    []uint32{2, 5},
	}
	if got := decodeDeltaMagnitude(generatedDeltaQCDF[:], dec); got != 14 {
		t.Fatalf("decodeDeltaMagnitude(spec escape) = %d, want 14", got)
	}
}

func TestDecodeDeltaMagnitudeCanUseLegacyEscapeForDebugging(t *testing.T) {
	t.Setenv("DEBUG_DELTA_Q_LEGACY_ESCAPE", "1")

	dec := &scriptedIntraEntropy{
		symbols: []uint32{3},
		bits:    []uint32{5},
	}
	if got := decodeDeltaMagnitude(generatedDeltaQCDF[:], dec); got != 6 {
		t.Fatalf("decodeDeltaMagnitude(legacy escape) = %d, want 6", got)
	}
}

func TestReadDeltaQAndLFSkipsEntireSuperBlockWithoutChangingQIndex(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{}
	hdr.Delta.QPresent = true
	hdr.Quantization.YAC = 73

	state := &TileSyntaxState{
		Tile: TileState{
			Entropy: msac.New([]byte{0x00}, true),
			CDF:     NewDefaultModeCDF(),
		},
		LastQIdx:   int(hdr.Quantization.YAC),
		ReadDeltas: true,
	}
	state.CurrentDQ = InitDequantTables(seq, hdr, state.LastQIdx)
	wantDQ := state.CurrentDQ

	if err := readDeltaQAndLF(seq, hdr, state, BlockGeometry{Size: RootBlockSize(seq)}, true); err != nil {
		t.Fatalf("readDeltaQAndLF: %v", err)
	}
	if state.ReadDeltas {
		t.Fatal("ReadDeltas = true, want false after consuming the superblock delta window")
	}
	if state.LastQIdx != int(hdr.Quantization.YAC) {
		t.Fatalf("LastQIdx = %d, want %d", state.LastQIdx, hdr.Quantization.YAC)
	}
	if state.CurrentDQ != wantDQ {
		t.Fatal("CurrentDQ changed for a fully skipped superblock")
	}
}

func TestReadDeltaQAndLFConsumesSkippedTopLeftSubBlock(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{}
	hdr.Delta.QPresent = true
	hdr.Quantization.YAC = 73

	state := &TileSyntaxState{
		Tile: TileState{
			Entropy: msac.New([]byte{0x00}, true),
			CDF:     NewDefaultModeCDF(),
		},
		LastQIdx:   int(hdr.Quantization.YAC),
		ReadDeltas: true,
	}
	state.CurrentDQ = InitDequantTables(seq, hdr, state.LastQIdx)
	wantDQ := state.CurrentDQ

	if err := readDeltaQAndLF(seq, hdr, state, BlockGeometry{Size: Block32x32}, true); err != nil {
		t.Fatalf("readDeltaQAndLF: %v", err)
	}
	if state.ReadDeltas {
		t.Fatal("ReadDeltas = true, want false after skipped top-left sub-block")
	}
	if state.LastQIdx != int(hdr.Quantization.YAC) {
		t.Fatalf("LastQIdx = %d, want %d", state.LastQIdx, hdr.Quantization.YAC)
	}
	if state.CurrentDQ != wantDQ {
		t.Fatal("CurrentDQ changed for skipped top-left sub-block with zero delta")
	}
}

func TestApplyDeltaQStateKeepsCoefCDFOnDeltaQByDefault(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{}
	hdr.Quantization.YAC = 73

	newState := func() *TileSyntaxState {
		state := &TileSyntaxState{
			Tile:     TileState{},
			LastQIdx: int(hdr.Quantization.YAC),
		}
		state.CurrentDQ = InitDequantTables(seq, hdr, state.LastQIdx)
		state.Tile.SetActiveCoefQIndex(state.LastQIdx)
		return state
	}

	t.Run("skip dequant update", func(t *testing.T) {
		t.Setenv("DEBUG_DELTA_Q_SKIP_DEQUANT_UPDATE", "1")
		state := newState()
		wantDQ := state.CurrentDQ
		beforeCoef := state.Tile.Coef

		applyDeltaQState(seq, hdr, state, 121, debugDeltaQUpdateBehavior())

		if state.LastQIdx != 121 {
			t.Fatalf("LastQIdx = %d, want 121", state.LastQIdx)
		}
		if state.CurrentDQ != wantDQ {
			t.Fatal("CurrentDQ changed when dequant update was disabled")
		}
		if state.Tile.Coef != beforeCoef {
			t.Fatal("active coefficient qcat changed for delta-q")
		}
	})

	t.Run("update dequant without coefficient qcat", func(t *testing.T) {
		state := newState()
		wantDQ := InitDequantTables(seq, hdr, 121)
		beforeCoef := state.Tile.Coef

		applyDeltaQState(seq, hdr, state, 121, debugDeltaQUpdateBehavior())

		if state.LastQIdx != 121 {
			t.Fatalf("LastQIdx = %d, want 121", state.LastQIdx)
		}
		if state.CurrentDQ != wantDQ {
			t.Fatal("CurrentDQ did not update when dequant update was enabled")
		}
		if state.Tile.Coef != beforeCoef {
			t.Fatal("active coefficient qcat changed for delta-q")
		}
	})

	t.Run("debug applies legacy coefficient qcat update", func(t *testing.T) {
		t.Setenv("DEBUG_DELTA_Q_APPLY_COEF_Q_UPDATE", "1")
		state := newState()
		wantDQ := InitDequantTables(seq, hdr, 121)
		beforeCoef := state.Tile.Coef
		wantQCat := DefaultCoefQCat(121)

		applyDeltaQState(seq, hdr, state, 121, debugDeltaQUpdateBehavior())

		if state.LastQIdx != 121 {
			t.Fatalf("LastQIdx = %d, want 121", state.LastQIdx)
		}
		if state.CurrentDQ != wantDQ {
			t.Fatal("CurrentDQ did not update when legacy coefficient qcat update was enabled")
		}
		if state.Tile.Coef == beforeCoef {
			t.Fatal("active coefficient qcat did not change with legacy debug update enabled")
		}
		if state.Tile.Coef.QCat != wantQCat {
			t.Fatalf("active coefficient qcat = %d, want %d", state.Tile.Coef.QCat, wantQCat)
		}
	})
}
