package decoder

import (
	"image"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestBlendOBMCHorizontal8(t *testing.T) {
	dst := make([]byte, 16)
	tmp := make([]byte, 16)
	for i := range dst {
		dst[i] = 64
		tmp[i] = 192
	}
	blendOBMCHorizontal8(dst, 4, tmp, 4, 4)
	if dst[0] <= 64 {
		t.Fatalf("top row did not blend: %d", dst[0])
	}
	if dst[3*4] != 64 {
		t.Fatalf("last row should remain unchanged, got %d", dst[12])
	}
}

func TestBlendOBMCVertical8(t *testing.T) {
	dst := make([]byte, 16)
	tmp := make([]byte, 16)
	for i := range dst {
		dst[i] = 64
		tmp[i] = 192
	}
	blendOBMCVertical8(dst, 4, tmp, 4, 4)
	if dst[0] <= 64 {
		t.Fatalf("left column did not blend: %d", dst[0])
	}
	if dst[3] != 64 {
		t.Fatalf("last column should remain unchanged, got %d", dst[3])
	}
}

func TestApplyInterOBMCChroma8BlendsTopNeighbor(t *testing.T) {
	d := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		},
	}
	ref := &Frame{
		Width:   32,
		Height:  32,
		Layout:  av1.Chroma420,
		U:       make([]byte, 16*16),
		V:       make([]byte, 16*16),
		UStride: 16,
		VStride: 16,
	}
	for i := range ref.U {
		ref.U[i] = 192
		ref.V[i] = 208
	}
	d.pureGoRefs[0] = ref

	img := image.NewYCbCr(image.Rect(0, 0, 32, 32), image.YCbCrSubsampleRatio420)
	for i := range img.Cb {
		img.Cb[i] = 64
		img.Cr[i] = 80
	}

	mvTile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	mvTile.SplatSingleRef(
		BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 6, End4Y: 2, Size: Block16x8},
		0, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair(),
	)

	hdr := obu.FrameHeader{Width: 32, Height: 32}
	for i := range hdr.RefIdx {
		hdr.RefIdx[i] = -1
	}
	hdr.RefIdx[0] = 0

	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 6, End4Y: 6, Size: Block16x16}
	if err := d.applyInterOBMCChroma8(&ParsedFrame{Header: hdr}, mvTile, target, img); err != nil {
		t.Fatalf("applyInterOBMCChroma8: %v", err)
	}

	chroma := chromaGeometryForBlock(av1.Chroma420, target)
	cx := chroma.Start4X * 4
	cy := chroma.Start4Y * 4
	topU := img.Cb[cy*img.CStride+cx]
	lastU := img.Cb[(cy+3)*img.CStride+cx]
	topV := img.Cr[cy*img.CStride+cx]
	lastV := img.Cr[(cy+3)*img.CStride+cx]
	if topU <= 64 {
		t.Fatalf("top U pixel = %d, want blended above 64", topU)
	}
	if lastU != 64 {
		t.Fatalf("last U overlap row = %d, want unchanged 64", lastU)
	}
	if topV <= 80 {
		t.Fatalf("top V pixel = %d, want blended above 80", topV)
	}
	if lastV != 80 {
		t.Fatalf("last V overlap row = %d, want unchanged 80", lastV)
	}
}
