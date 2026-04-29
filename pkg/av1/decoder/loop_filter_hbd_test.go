package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestSimpleLoopFilterThresholdHBDScalesBitDepth(t *testing.T) {
	if got, want := simpleLoopFilterThresholdHBD(24, 8), simpleLoopFilterThreshold(24); got != want {
		t.Fatalf("8-bit threshold=%d want %d", got, want)
	}
	if got, want := simpleLoopFilterThresholdHBD(24, 10), simpleLoopFilterThreshold(24)<<2; got != want {
		t.Fatalf("10-bit threshold=%d want %d", got, want)
	}
}

func TestWeakFilterPair16AdjustsEdge(t *testing.T) {
	row := []uint16{480, 492, 500, 516, 524, 536, 548}
	beforeP := row[2]
	beforeQ := row[3]
	weakFilterPair16(row, 1, 2, 3, 4, 64, 10)
	if row[2] == beforeP && row[3] == beforeQ {
		t.Fatal("weakFilterPair16 did not modify edge samples")
	}
}

func TestLoopFilterEdge16WidePreservesFlatEdge(t *testing.T) {
	buf := make([]uint16, 14)
	for i := range buf {
		buf[i] = 856
	}
	loopFilterEdge16(buf, 6, 1, calcLoopFilterParams(32, 0, 10), 16, 10)
	for i, got := range buf {
		if got != 856 {
			t.Fatalf("sample %d=%d, want flat edge preserved", i, got)
		}
	}
}

func TestApplySimpleLoopFilter42016MaskedUsesMask(t *testing.T) {
	yPlane := make([]uint16, 8*8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x < 4 {
				yPlane[y*8+x] = 500
			} else {
				yPlane[y*8+x] = 516
			}
		}
	}
	mask := NewCDEFIndexMap(8, 8)
	mask.markNoSkip(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2}, false)
	hdr := &obu.FrameHeader{}
	hdr.LoopFilter.LevelY[0] = 24
	applySimpleLoopFilter420_16Masked(yPlane, 8, 8, 8, nil, 0, 0, 0, nil, 0, hdr, 10, mask)
	if yPlane[3] == 500 && yPlane[4] == 516 {
		t.Fatal("masked 16-bit loop filter did not touch the marked edge")
	}
}
