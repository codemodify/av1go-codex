package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestChromaCoeffContextOffsetsUseSubsampledLumaModulo(t *testing.T) {
	g := BlockGeometry{Start4X: 88, Start4Y: 32}
	x, y := chromaCoeffContextOffsets(av1.Chroma420, g)
	if x != 8 || y != 0 {
		t.Fatalf("4:2:0 offsets=(%d,%d), want (8,0)", x, y)
	}

	x, y = chromaCoeffContextOffsets(av1.Chroma444, g)
	if x != 24 || y != 0 {
		t.Fatalf("4:4:4 offsets=(%d,%d), want (24,0)", x, y)
	}
}
