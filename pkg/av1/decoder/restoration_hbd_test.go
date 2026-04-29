package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestNewRestorationStateAllows10Bit420(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			BitDepth:     10,
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	hdr := &obu.FrameHeader{
		Width:         1920,
		UpscaledWidth: 1920,
		Height:        1080,
		Restoration: obu.Restoration{
			Type:     [3]uint8{restorationTypeSGRProj, restorationTypeSGRProj, restorationTypeSGRProj},
			UnitSize: [2]uint8{8, 8},
		},
	}
	state := NewRestorationState(seq, hdr)
	if state == nil {
		t.Fatal("NewRestorationState returned nil for 10-bit 4:2:0")
	}
	if got, want := state.Planes[0].UnitsX, 8; got != want {
		t.Fatalf("luma UnitsX=%d want %d", got, want)
	}
	if got, want := state.Planes[1].UnitsY, 2; got != want {
		t.Fatalf("chroma UnitsY=%d want %d", got, want)
	}
}

func TestApplyRestorationPlane16WienerChangesPixels(t *testing.T) {
	plane := make([]uint16, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if x < 8 {
				plane[y*16+x] = 128
			} else {
				plane[y*16+x] = 896
			}
		}
	}
	before := append([]uint16(nil), plane...)
	state := &RestorationPlane{
		Enabled:      true,
		UnitSizeLog2: 4,
		UnitSize:     16,
		Width:        16,
		Height:       16,
		UnitsX:       1,
		UnitsY:       1,
		Units: []RestorationUnit{{
			Type:    restorationTypeWiener,
			Parsed:  true,
			FilterH: [3]int8{3, -7, 15},
			FilterV: [3]int8{3, -7, 15},
		}},
	}
	applyRestorationPlane16(plane, before, 16, 16, 16, state, 10)
	if plane[7] == before[7] && plane[8] == before[8] {
		t.Fatal("16-bit wiener restoration did not change the edge pixels")
	}
}

func TestApplyRestoration42016NoopOnNilState(t *testing.T) {
	yPlane := make([]uint16, 8*8)
	uPlane := make([]uint16, 4*4)
	vPlane := make([]uint16, 4*4)
	beforeY := append([]uint16(nil), yPlane...)
	applyRestoration420_16(yPlane, 8, 8, 8, uPlane, 4, 4, 4, vPlane, 4, &obu.FrameHeader{}, nil, 10)
	for i := range yPlane {
		if yPlane[i] != beforeY[i] {
			t.Fatalf("nil restoration state changed luma pixel %d", i)
		}
	}
}
