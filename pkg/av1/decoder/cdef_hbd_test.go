package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestCDEFFindDir16VerticalEdge(t *testing.T) {
	src := make([]uint16, 8*8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x < 4 {
				src[y*8+x] = 128
			} else {
				src[y*8+x] = 896
			}
		}
	}
	dir, variance := cdefFindDir16(src, 8, 0, 0, 10)
	if variance == 0 {
		t.Fatal("variance = 0, want non-zero")
	}
	if dir != 6 {
		t.Fatalf("dir = %d, want 6 for a vertical edge", dir)
	}
}

func TestCDEFStrengths16ScaleBitDepth(t *testing.T) {
	pri, sec := cdefStrengths16(63, 10)
	if pri != 60 {
		t.Fatalf("pri = %d, want 60", pri)
	}
	if sec != 16 {
		t.Fatalf("sec = %d, want 16", sec)
	}
}

func TestApplyCDEF42016NoopWithZeroStrength(t *testing.T) {
	yPlane := make([]uint16, 8*8)
	for i := range yPlane {
		yPlane[i] = uint16(256 + i)
	}
	uPlane := make([]uint16, 4*4)
	vPlane := make([]uint16, 4*4)
	before := append([]uint16(nil), yPlane...)
	mask := NewCDEFIndexMap(8, 8)
	mask.set(0, 0, 0)
	mask.markNoSkip(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2}, false)
	hdr := &obu.FrameHeader{}
	hdr.CDEF.Damping = 3

	applyCDEF420_16(yPlane, 8, 8, 8, uPlane, 4, 4, 4, vPlane, 4, hdr, 10, mask)
	for i := range yPlane {
		if yPlane[i] != before[i] {
			t.Fatalf("pixel %d changed with zero cdef strength", i)
		}
	}
}
