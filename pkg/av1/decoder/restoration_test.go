package decoder

import (
	"image"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedRestorationEntropy struct {
	adapt   []uint32
	symbols []uint32
	subexp  []int
	bools   []uint32
}

func (s *scriptedRestorationEntropy) DecodeBoolAdapt([]uint16) uint32 {
	if len(s.adapt) == 0 {
		return 0
	}
	v := s.adapt[0]
	s.adapt = s.adapt[1:]
	return v
}

func (s *scriptedRestorationEntropy) DecodeSymbolAdapt([]uint16, int) uint32 {
	if len(s.symbols) == 0 {
		return 0
	}
	v := s.symbols[0]
	s.symbols = s.symbols[1:]
	return v
}

func (s *scriptedRestorationEntropy) DecodeSubexp(int, int, uint) int {
	if len(s.subexp) == 0 {
		return 0
	}
	v := s.subexp[0]
	s.subexp = s.subexp[1:]
	return v
}

func (s *scriptedRestorationEntropy) DecodeBools(uint) uint32 {
	if len(s.bools) == 0 {
		return 0
	}
	v := s.bools[0]
	s.bools = s.bools[1:]
	return v
}

func TestNewRestorationStateSpbtvGeometry(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			BitDepth:     8,
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	hdr := &obu.FrameHeader{
		Width:         960,
		UpscaledWidth: 960,
		Height:        540,
		Restoration: obu.Restoration{
			Type:     [3]uint8{restorationTypeSGRProj, restorationTypeSGRProj, restorationTypeSGRProj},
			UnitSize: [2]uint8{8, 8},
		},
	}
	state := NewRestorationState(seq, hdr)
	if state == nil {
		t.Fatal("NewRestorationState returned nil")
	}
	if got, want := state.Planes[0].UnitsX, 4; got != want {
		t.Fatalf("luma UnitsX=%d want %d", got, want)
	}
	if got, want := state.Planes[0].UnitsY, 2; got != want {
		t.Fatalf("luma UnitsY=%d want %d", got, want)
	}
	if got, want := state.Planes[1].UnitsX, 2; got != want {
		t.Fatalf("chroma UnitsX=%d want %d", got, want)
	}
	if got, want := state.Planes[1].UnitsY, 1; got != want {
		t.Fatalf("chroma UnitsY=%d want %d", got, want)
	}
}

func TestReadRestorationUnitSwitchableSGR(t *testing.T) {
	cdf := NewDefaultModeCDF()
	dec := &scriptedRestorationEntropy{
		symbols: []uint32{2},
		bools:   []uint32{0},
		subexp:  []int{12},
	}
	ref := defaultRestorationUnit()
	unit, err := readRestorationUnit(cdf, dec, 0, restorationTypeSwitchable, ref)
	if err != nil {
		t.Fatalf("readRestorationUnit: %v", err)
	}
	if got, want := unit.Type, uint8(restorationTypeSGRProj); got != want {
		t.Fatalf("unit.Type=%d want %d", got, want)
	}
	if got, want := unit.SGRWeights[0], int8(12-96); got != want {
		t.Fatalf("unit.SGRWeights[0]=%d want %d", got, want)
	}
}

func TestApplyRestorationPlane8WienerChangesPixels(t *testing.T) {
	img := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio420)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if x < 8 {
				img.Y[y*img.YStride+x] = 32
			} else {
				img.Y[y*img.YStride+x] = 224
			}
		}
	}
	before := append([]byte(nil), img.Y...)
	state := &RestorationState{
		Planes: [3]RestorationPlane{
			{
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
			},
		},
	}
	applyRestorationPlane8(img.Y, before, img.YStride, 16, 16, &state.Planes[0])
	if img.Y[7] == before[7] && img.Y[8] == before[8] {
		t.Fatal("wiener restoration did not change the edge pixels")
	}
}
