package decoder

import (
	"image"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestPredictInterLumaBlock8ClipsVisibleArea(t *testing.T) {
	dst := make([]byte, 8*8)
	ref := make([]byte, 8*8)
	for i := range ref {
		ref[i] = 77
	}
	if err := PredictInterLumaBlock8(dst, 8, 8, 8, 6, 6, 4, 4, ref, 8, 8, 8, MotionVector{}); err != nil {
		t.Fatalf("PredictInterLumaBlock8: %v", err)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := dst[y*8+x]
			if x >= 6 && y >= 6 {
				if got != 77 {
					t.Fatalf("dst[%d,%d] = %d, want 77", x, y, got)
				}
			} else if got != 0 {
				t.Fatalf("dst[%d,%d] = %d, want 0 outside visible area", x, y, got)
			}
		}
	}
}

func TestPredictInterChroma420Sub8x8FallbackUsesAlignedChromaBlock(t *testing.T) {
	dec := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
		},
	}
	img := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio420)
	ref := &Frame{
		Width:   16,
		Height:  16,
		U:       make([]byte, 8*8),
		V:       make([]byte, 8*8),
		UStride: 8,
		VStride: 8,
	}
	for i := range img.Cb {
		img.Cb[i] = 0
		img.Cr[i] = 0
		ref.U[i] = byte(64 + i)
		ref.V[i] = byte(96 + i)
	}

	g := BlockGeometry{Start4X: 1, Start4Y: 1, End4X: 2, End4Y: 2, Size: Block4x4}
	syntax := InterBlockSyntax{
		Geometry: g,
		Size:     Block4x4,
		Ref0:     0,
		MV:       MotionVector{},
		Filter:   defaultInterFilterPair(),
	}
	mvTile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 4, End4Y: 4})
	if err := dec.predictInterChroma420Block8(&ParsedFrame{}, &BlockContext{}, &BlockContext{}, mvTile, g, syntax, img, ref); err != nil {
		t.Fatalf("predictInterChroma420Block8: %v", err)
	}

	if got, want := img.Cr[1*img.CStride+3], ref.V[1*ref.VStride+3]; got != want {
		t.Fatalf("top chroma quadrant = %d, want aligned fallback copy %d", got, want)
	}
	if got, want := img.Cr[3*img.CStride+3], ref.V[3*ref.VStride+3]; got != want {
		t.Fatalf("current chroma quadrant = %d, want aligned fallback copy %d", got, want)
	}
}
