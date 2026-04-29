package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestCDFStateFromTileStateResetsSymbolCounts(t *testing.T) {
	mode := NewDefaultModeCDF()
	mode.SkipMode[1] = [2]uint16{1234, 32}
	mode.Partition[BlockLevel128x128][2][7] = 29
	mode.Partition[BlockLevel8x8][3][3] = 31
	mode.Partition[BlockLevel64x64][0][9] = 32
	mode.UVMode[0][IntraPredPaeth][12] = 17
	mode.UVMode[1][IntraPredPaeth][13] = 18
	mode.PalSize[1][6][6] = 19
	mode.ColorMap[1][3][4][4] = 20
	mode.TXSize[0][2][1] = 21
	mode.TXSize[2][1][2] = 22
	mode.RestoreSwitch[2] = 23
	mv := NewDefaultMVCDF()
	mv.Joint = [4]uint16{1000, 2000, 3000, 32}
	coef := NewDefaultCoefCDFForQCat(0)
	coef.Skip[0][0] = [2]uint16{2345, 32}
	coef.BaseTok[0][0][0] = [4]uint16{100, 200, 300, 32}

	state := cdfStateFromTileState(&TileState{
		CDF:     mode,
		MV:      mv,
		CoefByQ: [4]*CoefCDF{coef},
	})
	if state == nil {
		t.Fatal("cdfStateFromTileState returned nil")
	}

	if got, want := state.Mode.SkipMode[1][0], uint16(1234); got != want {
		t.Fatalf("skip mode cdf value = %d, want %d", got, want)
	}
	if got := state.Mode.SkipMode[1][1]; got != 0 {
		t.Fatalf("skip mode count = %d, want 0", got)
	}
	if got := state.Mode.Partition[BlockLevel128x128][2][7]; got != 0 {
		t.Fatalf("128x128 partition count = %d, want 0", got)
	}
	if got := state.Mode.Partition[BlockLevel8x8][3][3]; got != 0 {
		t.Fatalf("8x8 partition count = %d, want 0", got)
	}
	if got := state.Mode.Partition[BlockLevel64x64][0][9]; got != 0 {
		t.Fatalf("partition count = %d, want 0", got)
	}
	if got := state.Mode.UVMode[0][IntraPredPaeth][12]; got != 0 {
		t.Fatalf("no-CFL UV mode count = %d, want 0", got)
	}
	if got := state.Mode.UVMode[1][IntraPredPaeth][13]; got != 0 {
		t.Fatalf("CFL UV mode count = %d, want 0", got)
	}
	if got := state.Mode.PalSize[1][6][6]; got != 0 {
		t.Fatalf("palette size count = %d, want 0", got)
	}
	if got := state.Mode.ColorMap[1][3][4][4]; got != 0 {
		t.Fatalf("color map count = %d, want 0", got)
	}
	if got := state.Mode.TXSize[0][2][1]; got != 0 {
		t.Fatalf("tx size depth-1 count = %d, want 0", got)
	}
	if got := state.Mode.TXSize[2][1][2]; got != 0 {
		t.Fatalf("tx size depth-2 count = %d, want 0", got)
	}
	if got := state.Mode.RestoreSwitch[2]; got != 0 {
		t.Fatalf("restore switch count = %d, want 0", got)
	}
	if got := state.MV.Joint[3]; got != 0 {
		t.Fatalf("mv joint count = %d, want 0", got)
	}
	if got := state.CoefByQ[0].Skip[0][0][1]; got != 0 {
		t.Fatalf("coef skip count = %d, want 0", got)
	}
	if got := state.CoefByQ[0].BaseTok[0][0][0][3]; got != 0 {
		t.Fatalf("coef base token count = %d, want 0", got)
	}

	if got := mode.SkipMode[1][1]; got != 32 {
		t.Fatalf("source skip mode count mutated to %d, want 32", got)
	}
	if got := mode.UVMode[0][IntraPredPaeth][12]; got != 17 {
		t.Fatalf("source no-CFL UV mode count mutated to %d, want 17", got)
	}
	if got := coef.BaseTok[0][0][0][3]; got != 32 {
		t.Fatalf("source coef count mutated to %d, want 32", got)
	}
}

func TestSnapshotPureGoCDFStateRefreshContextFalseUsesInputContext(t *testing.T) {
	sourceMode := NewDefaultModeCDF()
	sourceMode.Partition[BlockLevel64x64][0][0] = 1111
	currentMode := NewDefaultModeCDF()
	currentMode.Partition[BlockLevel64x64][0][0] = 2222
	oldMode := NewDefaultModeCDF()
	oldMode.Partition[BlockLevel64x64][0][0] = 3333

	d := &Decoder{
		currentCDFState: &FrameCDFState{Mode: currentMode},
	}
	d.pureGoCDFRefs[1] = &FrameCDFState{Mode: oldMode}
	d.pureGoCDFRefs[2] = &FrameCDFState{Mode: sourceMode}

	hdr := &obu.FrameHeader{
		PrimaryRefFrame:   4,
		RefreshContext:    false,
		RefreshFrameFlags: 1 << 1,
		RefIdx:            [7]int8{-1, -1, -1, -1, 2, -1, -1},
	}
	d.snapshotPureGoCDFState(hdr.RefreshFrameFlags, hdr)

	got := d.pureGoCDFRefs[1]
	if got == nil || got.Mode == nil {
		t.Fatal("refreshed CDF state is nil")
	}
	if got.Mode.Partition[BlockLevel64x64][0][0] != 1111 {
		t.Fatalf("refreshed CDF used wrong context: got %d, want input 1111", got.Mode.Partition[BlockLevel64x64][0][0])
	}
	sourceMode.Partition[BlockLevel64x64][0][0] = 4444
	if got.Mode.Partition[BlockLevel64x64][0][0] != 1111 {
		t.Fatal("refreshed CDF state aliases input context")
	}
}

func TestSnapshotPureGoCDFStateRefreshContextFalseClearsToDefaultContext(t *testing.T) {
	oldMode := NewDefaultModeCDF()
	oldMode.Partition[BlockLevel64x64][0][0] = 3333
	d := &Decoder{}
	d.pureGoCDFRefs[1] = &FrameCDFState{Mode: oldMode}

	hdr := &obu.FrameHeader{
		PrimaryRefFrame:   7,
		RefreshContext:    false,
		RefreshFrameFlags: 1 << 1,
		RefIdx:            [7]int8{-1, -1, -1, -1, -1, -1, -1},
	}
	d.snapshotPureGoCDFState(hdr.RefreshFrameFlags, hdr)

	if d.pureGoCDFRefs[1] != nil {
		t.Fatalf("refreshed CDF state = %#v, want default nil context", d.pureGoCDFRefs[1])
	}
}
