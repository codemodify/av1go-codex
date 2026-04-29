package nativeplayer

import (
	"image"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
)

func BenchmarkPrepareFrameForDisplayPlanar420(b *testing.B) {
	const (
		width  = 1280
		height = 720
	)
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	frame := &decoder.Frame{
		Width:    width,
		Height:   height,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Color: av1.ColorConfig{
			BitDepth:           8,
			SubsamplingX:       true,
			SubsamplingY:       true,
			MatrixCoefficients: 1,
		},
		Y:       make([]byte, width*height),
		U:       make([]byte, chromaWidth*chromaHeight),
		V:       make([]byte, chromaWidth*chromaHeight),
		YStride: width,
		UStride: chromaWidth,
		VStride: chromaWidth,
	}
	for i := range frame.Y {
		frame.Y[i] = byte(i)
	}
	for i := range frame.U {
		frame.U[i] = byte(128 + i%17)
		frame.V[i] = byte(128 - i%13)
	}

	b.ReportAllocs()
	b.SetBytes(int64(width * height))
	for i := 0; i < b.N; i++ {
		prepared := prepareFrameForDisplay(frame)
		if prepared == nil || prepared.Image == nil {
			b.Fatal("prepareFrameForDisplay returned nil image")
		}
		if err := prepared.Close(); err != nil {
			b.Fatalf("prepared.Close: %v", err)
		}
	}
}

func BenchmarkPrepareFrameForDisplayYCbCr420(b *testing.B) {
	const (
		width  = 1280
		height = 720
	)
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	frame := &decoder.Frame{
		Width:    width,
		Height:   height,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Color: av1.ColorConfig{
			BitDepth:           8,
			SubsamplingX:       true,
			SubsamplingY:       true,
			MatrixCoefficients: 1,
		},
		Image:   img,
		Y:       img.Y,
		U:       img.Cb,
		V:       img.Cr,
		YStride: img.YStride,
		UStride: img.CStride,
		VStride: img.CStride,
	}
	for i := range img.Y {
		img.Y[i] = byte(i)
	}
	for i := range img.Cb {
		img.Cb[i] = byte(128 + i%17)
		img.Cr[i] = byte(128 - i%13)
	}

	b.ReportAllocs()
	b.SetBytes(int64(width * height))
	for i := 0; i < b.N; i++ {
		prepared := prepareFrameForDisplay(frame)
		if prepared == nil || prepared.Image == nil {
			b.Fatal("prepareFrameForDisplay returned nil image")
		}
		if err := prepared.Close(); err != nil {
			b.Fatalf("prepared.Close: %v", err)
		}
	}
}
