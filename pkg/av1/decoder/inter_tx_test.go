package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestReadInterTransformUnitsLosslessSplitsTo4x4Grid(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.Lossless[0] = true
	above := BlockContext{}
	left := BlockContext{}

	units, err := ReadInterTransformUnits(hdr, NewDefaultModeCDF(), &above, &left, BlockGeometry{
		Level:   BlockLevel16x16,
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, BlockGeometry{
		Level:   BlockLevel16x16,
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, 0, false, &scriptedIntraEntropy{})
	if err != nil {
		t.Fatalf("ReadInterTransformUnits: %v", err)
	}
	if got, want := len(units), 16; got != want {
		t.Fatalf("len(units) = %d, want %d", got, want)
	}
	for i, tu := range units {
		if tu.TX != TX4X4 {
			t.Fatalf("units[%d].TX = %d, want TX4X4", i, tu.TX)
		}
	}
}

func TestReadInterTransformUnitsTilesLargeInterBlockByMaxTX(t *testing.T) {
	hdr := &obu.FrameHeader{TxfmMode: txModeSwitchable}
	above := BlockContext{}
	left := BlockContext{}
	dec := &scriptedIntraEntropy{}

	local := BlockGeometry{
		Level:   BlockLevel128x128,
		Size:    Block64x128,
		Start4X: 0,
		Start4Y: 0,
		End4X:   16,
		End4Y:   32,
	}
	global := BlockGeometry{
		Level:   BlockLevel128x128,
		Size:    Block64x128,
		Start4X: 144,
		Start4Y: 0,
		End4X:   160,
		End4Y:   32,
	}

	units, err := ReadInterTransformUnits(hdr, NewDefaultModeCDF(), &above, &left, local, global, 0, false, dec)
	if err != nil {
		t.Fatalf("ReadInterTransformUnits: %v", err)
	}
	if got, want := len(units), 2; got != want {
		t.Fatalf("len(units) = %d, want %d", got, want)
	}
	if got, want := len(dec.adaptCDF0), 2; got != want {
		t.Fatalf("tx split reads = %d, want %d", got, want)
	}
	want := []TransformUnit{
		{
			Local:  BlockGeometry{Level: BlockLevel128x128, Size: Block64x128, Start4X: 0, Start4Y: 0, End4X: 16, End4Y: 16},
			Global: BlockGeometry{Level: BlockLevel128x128, Size: Block64x128, Start4X: 144, Start4Y: 0, End4X: 160, End4Y: 16},
			TX:     TX64X64,
		},
		{
			Local:  BlockGeometry{Level: BlockLevel128x128, Size: Block64x128, Start4X: 0, Start4Y: 16, End4X: 16, End4Y: 32},
			Global: BlockGeometry{Level: BlockLevel128x128, Size: Block64x128, Start4X: 144, Start4Y: 16, End4X: 160, End4Y: 32},
			TX:     TX64X64,
		},
	}
	for i := range want {
		if units[i] != want[i] {
			t.Fatalf("units[%d] = %+v, want %+v", i, units[i], want[i])
		}
	}
}

func TestReadInterTransformUnitsTiles128x128ByMaxTX(t *testing.T) {
	hdr := &obu.FrameHeader{TxfmMode: txModeSwitchable}
	above := BlockContext{}
	left := BlockContext{}
	dec := &scriptedIntraEntropy{}

	g := BlockGeometry{
		Level:   BlockLevel128x128,
		Size:    Block128x128,
		Start4X: 0,
		Start4Y: 0,
		End4X:   32,
		End4Y:   32,
	}
	units, err := ReadInterTransformUnits(hdr, NewDefaultModeCDF(), &above, &left, g, g, 0, false, dec)
	if err != nil {
		t.Fatalf("ReadInterTransformUnits: %v", err)
	}
	if got, want := len(units), 4; got != want {
		t.Fatalf("len(units) = %d, want %d", got, want)
	}
	if got, want := len(dec.adaptCDF0), 4; got != want {
		t.Fatalf("tx split reads = %d, want %d", got, want)
	}
	for i, tu := range units {
		if tu.TX != TX64X64 {
			t.Fatalf("units[%d].TX = %d, want TX64X64", i, tu.TX)
		}
		if gotW, gotH := tu.Local.Width4(), tu.Local.Height4(); gotW != 16 || gotH != 16 {
			t.Fatalf("units[%d] local size = %dx%d, want 16x16", i, gotW, gotH)
		}
	}
}
