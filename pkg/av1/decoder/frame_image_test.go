package decoder

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/codemodify/av1go-codex/internal/displaycolor"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestBuildFrameFromPlanar16CreatesPreview(t *testing.T) {
	y := []uint16{
		0, 256, 512, 768,
		1023, 900, 700, 400,
	}
	u := []uint16{512, 768}
	v := []uint16{256, 1023}
	cfg := previewTestConfig(10, av1.Chroma420)

	frame, err := buildFrameFromPlanar16(4, 2, cfg, 33*time.Millisecond, 40*time.Millisecond, y, 4, u, 2, v, 2)
	if err != nil {
		t.Fatalf("buildFrameFromPlanar16: %v", err)
	}
	if frame.Image == nil {
		t.Fatal("expected preview image")
	}
	if got, want := frame.Image.Bounds(), image.Rect(0, 0, 4, 2); got != want {
		t.Fatalf("image bounds = %v, want %v", got, want)
	}
	preview, ok := frame.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("preview image type = %T, want *image.RGBA", frame.Image)
	}
	if got := rgbaAt(preview, 0, 0); got == (color.RGBA{}) {
		t.Fatalf("preview pixel = %v, want non-zero rgba", got)
	}
	if got, want := rgbaAt(preview, 3, 0), rgbaAt(preview, 0, 0); got == want {
		t.Fatalf("expected preview variation, pixel(3,0) = %v, pixel(0,0) = %v", got, want)
	}
	if len(frame.Y16) != len(y) || len(frame.U16) != len(u) || len(frame.V16) != len(v) {
		t.Fatal("expected stored 16-bit planes")
	}
}

func TestCloneFramePreservesHighBitDepthPlanes(t *testing.T) {
	frame, err := buildFrameFromPlanar16(
		4,
		2,
		previewTestConfig(10, av1.Chroma420),
		10*time.Millisecond,
		40*time.Millisecond,
		[]uint16{1, 2, 3, 4, 5, 6, 7, 8},
		4,
		[]uint16{400, 500},
		2,
		[]uint16{600, 700},
		2,
	)
	if err != nil {
		t.Fatalf("buildFrameFromPlanar16: %v", err)
	}

	clone := cloneFrame(frame)
	if clone == nil {
		t.Fatal("expected cloned frame")
	}
	if len(clone.Y16) != len(frame.Y16) || len(clone.U16) != len(frame.U16) || len(clone.V16) != len(frame.V16) {
		t.Fatal("expected cloned 16-bit planes")
	}
	if &clone.Y16[0] == &frame.Y16[0] {
		t.Fatal("expected deep-copied luma plane")
	}
	if clone.Image != nil {
		t.Fatal("expected high-bit-depth reference clone to avoid eager preview image")
	}
}

func TestBuildFrameFromPlanar16NoPreviewStoresPlanesOnly(t *testing.T) {
	frame, err := buildFrameFromPlanar16NoPreview(
		4,
		2,
		previewTestConfig(10, av1.Chroma420),
		10*time.Millisecond,
		40*time.Millisecond,
		[]uint16{1, 2, 3, 4, 5, 6, 7, 8},
		4,
		[]uint16{400, 500},
		2,
		[]uint16{600, 700},
		2,
	)
	if err != nil {
		t.Fatalf("buildFrameFromPlanar16NoPreview: %v", err)
	}
	if frame.Image != nil {
		t.Fatal("expected no eager preview image")
	}
	if len(frame.Y16) != 8 || len(frame.U16) != 2 || len(frame.V16) != 2 {
		t.Fatal("expected stored high-bit-depth planes")
	}
}

func TestCloneFramePreservesYCbCrImageFor8BitPlanar(t *testing.T) {
	img := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio420)
	for i := range img.Y {
		img.Y[i] = byte(10 + i)
	}
	for i := range img.Cb {
		img.Cb[i] = byte(100 + i)
		img.Cr[i] = byte(150 + i)
	}
	frame := buildFrameFromYCbCr(img, previewTestConfig(8, av1.Chroma420), 0, 0)

	clone := cloneFrame(frame)
	if clone == nil {
		t.Fatal("expected cloned frame")
	}
	ycbcr, ok := clone.Image.(*image.YCbCr)
	if !ok {
		t.Fatalf("clone image type = %T, want *image.YCbCr", clone.Image)
	}
	if got, want := ycbcr.Rect, image.Rect(0, 0, 4, 2); got != want {
		t.Fatalf("clone bounds = %v, want %v", got, want)
	}
	if &clone.Y[0] == &frame.Y[0] {
		t.Fatal("expected deep-copied luma plane")
	}
	if &ycbcr.Y[0] != &clone.Y[0] || &ycbcr.Cb[0] != &clone.U[0] || &ycbcr.Cr[0] != &clone.V[0] {
		t.Fatal("expected cloned image to reuse cloned planar buffers")
	}
}

func TestFrameCloseReleasesScratchBackedPlanarBuffers(t *testing.T) {
	frame := takeReferenceFrame420(4, 2, previewTestConfig(8, av1.Chroma420), 0, 0)
	if frame == nil || frame.scratch8 == nil {
		t.Fatal("expected pooled frame scratch")
	}
	if err := frame.Close(); err != nil {
		t.Fatalf("Frame.Close: %v", err)
	}
	if frame.scratch8 != nil {
		t.Fatal("expected Close to release frame scratch")
	}
	if frame.Image != nil || len(frame.Y) != 0 || len(frame.U) != 0 || len(frame.V) != 0 {
		t.Fatal("expected Close to clear planar image views")
	}
	if err := frame.Close(); err != nil {
		t.Fatalf("Frame.Close second call: %v", err)
	}
}

func TestCopyReferenceFrame420FillsMissingPixelsFromNeutralBase(t *testing.T) {
	frame := takeReferenceFrame420(4, 2, previewTestConfig(8, av1.Chroma420), 0, 0)
	t.Cleanup(func() { _ = frame.Close() })

	copyReferenceFrame420(frame, &Frame{
		Width:    2,
		Height:   2,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Y:        []byte{1, 2, 3, 4},
		U:        []byte{5},
		V:        []byte{6},
	})

	if got, want := frame.Y[:6], []byte{1, 2, 3, 4, 0, 0}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] || got[4] != want[4] || got[5] != want[5] {
		t.Fatalf("copied luma prefix = %v, want %v", got, want)
	}
	for i := 4; i < len(frame.Y); i++ {
		if frame.Y[i] != 0 {
			t.Fatalf("expected zero-filled luma tail, Y[%d]=%d", i, frame.Y[i])
		}
	}
	if got, want := frame.U[0], byte(5); got != want {
		t.Fatalf("copied U[0] = %d, want %d", got, want)
	}
	if got, want := frame.V[0], byte(6); got != want {
		t.Fatalf("copied V[0] = %d, want %d", got, want)
	}
	for i := 1; i < len(frame.U); i++ {
		if frame.U[i] != 128 {
			t.Fatalf("expected neutral-filled U[%d] = 128, got %d", i, frame.U[i])
		}
		if frame.V[i] != 128 {
			t.Fatalf("expected neutral-filled V[%d] = 128, got %d", i, frame.V[i])
		}
	}
}

func TestBuildFrameFromPlanar16Creates422Preview(t *testing.T) {
	frame, err := buildFrameFromPlanar16(
		4,
		2,
		previewTestConfig(10, av1.Chroma422),
		0,
		0,
		[]uint16{0, 256, 512, 768, 1023, 900, 700, 400},
		4,
		[]uint16{512, 768, 256, 1023},
		2,
		[]uint16{300, 900, 700, 100},
		2,
	)
	if err != nil {
		t.Fatalf("buildFrameFromPlanar16: %v", err)
	}
	preview, ok := frame.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("preview image type = %T, want *image.RGBA", frame.Image)
	}
	if got, want := rgbaAt(preview, 1, 0), rgbaAt(preview, 3, 1); got == want {
		t.Fatalf("expected 422 preview pixels to differ, got %v", got)
	}
}

func TestBuildFrameFromPlanar16Creates444Preview(t *testing.T) {
	cfg := previewTestConfig(10, av1.Chroma444)
	cfg.MatrixCoefficients = 0
	frame, err := buildFrameFromPlanar16(
		2,
		2,
		cfg,
		0,
		0,
		[]uint16{100, 200, 300, 400},
		2,
		[]uint16{0, 256, 512, 1023},
		2,
		[]uint16{1023, 512, 256, 0},
		2,
	)
	if err != nil {
		t.Fatalf("buildFrameFromPlanar16: %v", err)
	}
	preview, ok := frame.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("preview image type = %T, want *image.RGBA", frame.Image)
	}
	if got, want := rgbaAt(preview, 0, 0), (color.RGBA{
		R: downshiftTo8Bit(100, 10),
		G: downshiftTo8Bit(0, 10),
		B: downshiftTo8Bit(1023, 10),
		A: 0xff,
	}); got != want {
		t.Fatalf("preview pixel = %v, want %v", got, want)
	}
}

func TestBuildFrameFromPlanar16UsesSharedHDRDisplayTransform(t *testing.T) {
	cfg := previewTestConfig(10, av1.Chroma420)
	cfg.MatrixCoefficients = 9
	cfg.ColorPrimaries = 9
	cfg.TransferCharacteristics = 16

	y := []uint16{
		840, 768,
		640, 512,
	}
	u := []uint16{320}
	v := []uint16{896}
	frame, err := buildFrameFromPlanar16(2, 2, cfg, 0, 0, y, 2, u, 1, v, 1)
	if err != nil {
		t.Fatalf("buildFrameFromPlanar16: %v", err)
	}
	preview, ok := frame.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("preview image type = %T, want *image.RGBA", frame.Image)
	}

	wantR, wantG, wantB := displaycolor.YUVToDisplayRGB(cfg, int(y[0]), int(u[0]), int(v[0]))
	if got, want := rgbaAt(preview, 0, 0), (color.RGBA{R: wantR, G: wantG, B: wantB, A: 0xff}); got != want {
		t.Fatalf("preview pixel = %v, want %v", got, want)
	}
}

func previewTestConfig(bitDepth uint8, layout av1.ChromaSubsampling) av1.ColorConfig {
	cfg := av1.ColorConfig{
		BitDepth:           bitDepth,
		MatrixCoefficients: 1,
	}
	switch layout {
	case av1.Chroma420:
		cfg.SubsamplingX = true
		cfg.SubsamplingY = true
	case av1.Chroma422:
		cfg.SubsamplingX = true
	case av1.Chroma444:
		cfg.SubsamplingX = false
		cfg.SubsamplingY = false
	}
	return cfg
}

func rgbaAt(img *image.RGBA, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}
