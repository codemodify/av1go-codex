package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestReadIntraTransformUnitsSwitchableUsesFixedRootTX(t *testing.T) {
	hdr := &obu.FrameHeader{TxfmMode: txModeSwitchable}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{1},
	}

	units, err := ReadIntraTransformUnits(hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, TX16X16, 0, dec)
	if err != nil {
		t.Fatalf("ReadIntraTransformUnits: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("len(units) = %d, want 1", len(units))
	}
	if units[0].TX != TX16X16 {
		t.Fatalf("units[0].TX = %d, want TX16X16", units[0].TX)
	}
	if got := units[0].Global; got.Start4X != 0 || got.Start4Y != 0 || got.End4X != 4 || got.End4Y != 4 {
		t.Fatalf("units[0].Global = %+v, want one 16x16 TU", got)
	}
	if len(dec.adaptCDF0) != 0 {
		t.Fatalf("switchable intra consumed %d TXPart symbols, want 0", len(dec.adaptCDF0))
	}
}

func TestReadIntraTransformUnitsSwitchable8x8DoesNotSplitTo4x4(t *testing.T) {
	hdr := &obu.FrameHeader{TxfmMode: txModeSwitchable}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{1},
	}

	units, err := ReadIntraTransformUnits(hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, TX8X8, 0, dec)
	if err != nil {
		t.Fatalf("ReadIntraTransformUnits: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("len(units) = %d, want 1", len(units))
	}
	if units[0].TX != TX8X8 {
		t.Fatalf("units[0].TX = %d, want TX8X8", units[0].TX)
	}
	if got := units[0].Global; got.Start4X != 0 || got.Start4Y != 0 || got.End4X != 2 || got.End4Y != 2 {
		t.Fatalf("units[0].Global = %+v, want one 8x8 TU", got)
	}
	if len(dec.adaptCDF0) != 0 {
		t.Fatalf("switchable 8x8 intra consumed %d TXPart symbols, want 0", len(dec.adaptCDF0))
	}
}

func TestReadIntraTransformUnitsLargestModeUsesFixedUnits(t *testing.T) {
	hdr := &obu.FrameHeader{TxfmMode: txModeLargest}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	units, err := ReadIntraTransformUnits(hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, TX8X8, 0, nil)
	if err != nil {
		t.Fatalf("ReadIntraTransformUnits: %v", err)
	}
	if len(units) != 4 {
		t.Fatalf("len(units) = %d, want 4", len(units))
	}
	for i, unit := range units {
		if unit.TX != TX8X8 {
			t.Fatalf("units[%d].TX = %d, want TX8X8", i, unit.TX)
		}
	}
}

func TestReadIntraTransformUnitsSwitchableSmallerTXUsesFixedGrid(t *testing.T) {
	hdr := &obu.FrameHeader{TxfmMode: txModeSwitchable}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	units, err := ReadIntraTransformUnits(hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block32x32,
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   8,
	}, TX8X8, 0, &scriptedIntraEntropy{})
	if err != nil {
		t.Fatalf("ReadIntraTransformUnits: %v", err)
	}
	if len(units) != 16 {
		t.Fatalf("len(units) = %d, want 16", len(units))
	}
	want := [][4]int{
		{0, 0, 2, 2},
		{2, 0, 4, 2},
		{4, 0, 6, 2},
		{6, 0, 8, 2},
		{0, 2, 2, 4},
		{2, 2, 4, 4},
		{4, 2, 6, 4},
		{6, 2, 8, 4},
		{0, 4, 2, 6},
		{2, 4, 4, 6},
		{4, 4, 6, 6},
		{6, 4, 8, 6},
		{0, 6, 2, 8},
		{2, 6, 4, 8},
		{4, 6, 6, 8},
		{6, 6, 8, 8},
	}
	for i, unit := range units {
		if unit.TX != TX8X8 {
			t.Fatalf("units[%d].TX = %d, want TX8X8", i, unit.TX)
		}
		got := [4]int{unit.Global.Start4X, unit.Global.Start4Y, unit.Global.End4X, unit.Global.End4Y}
		if got != want[i] {
			t.Fatalf("units[%d].Global = %v, want %v", i, got, want[i])
		}
	}
}
