package encoder

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
	"github.com/codemodify/av1go-codex/pkg/container/mp4"
)

func TestNewAndRemux(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "remux-src.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	f, err := mp4.Open(fixture.Path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	track, err := f.AV1VideoTrack()
	if err != nil {
		t.Fatalf("AV1VideoTrack: %v", err)
	}

	enc, err := New(Config{
		Width:     track.Width,
		Height:    track.Height,
		Timescale: track.Timescale,
		AV1C:      track.AV1C.Clone(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "remux.mp4")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer outFile.Close()

	samples := make([]mp4.SampleInput, 0, 3)
	for i := 0; i < 3; i++ {
		data, err := track.ReadSample(i)
		if err != nil {
			t.Fatalf("ReadSample(%d): %v", i, err)
		}
		s := track.Samples[i]
		samples = append(samples, mp4.SampleInput{
			Data:              data,
			Duration:          s.Duration,
			CompositionOffset: s.CompositionOffset,
			Keyframe:          s.Keyframe,
		})
	}

	if err := enc.RemuxMP4(outFile, samples); err != nil {
		t.Fatalf("RemuxMP4: %v", err)
	}

	rt, err := mp4.Open(outPath)
	if err != nil {
		t.Fatalf("roundtrip Open: %v", err)
	}
	defer rt.Close()

	rtTrack, err := rt.AV1VideoTrack()
	if err != nil {
		t.Fatalf("roundtrip AV1VideoTrack: %v", err)
	}
	if got, want := len(rtTrack.Samples), len(samples); got != want {
		t.Fatalf("sample count = %d, want %d", got, want)
	}
}

func TestNewDefaultsAV1C(t *testing.T) {
	enc, err := New(Config{Width: 16, Height: 16})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := enc.cfg.AV1C.Version, uint8(1); got != want {
		t.Fatalf("AV1C.Version = %d, want %d", got, want)
	}
	if !enc.cfg.AV1C.ChromaSubsamplingX || !enc.cfg.AV1C.ChromaSubsamplingY {
		t.Fatalf("default AV1C subsampling = (%v,%v), want 4:2:0", enc.cfg.AV1C.ChromaSubsamplingX, enc.cfg.AV1C.ChromaSubsamplingY)
	}
}

func TestEncodeFrameValidatesInput(t *testing.T) {
	enc, err := New(Config{Width: 16, Height: 16})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := enc.EncodeFrame(nil); err != ErrNilImage {
		t.Fatalf("EncodeFrame(nil) error = %v, want %v", err, ErrNilImage)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if _, err := enc.EncodeFrame(img); err != ErrFrameDimensions {
		t.Fatalf("EncodeFrame(dim mismatch) error = %v, want %v", err, ErrFrameDimensions)
	}
	img = image.NewRGBA(image.Rect(0, 0, 16, 16))
	if data, err := enc.EncodeFrame(img); err != nil {
		t.Fatalf("EncodeFrame(black) error = %v", err)
	} else if !bytes.Equal(data, black16x16Sample) {
		t.Fatalf("EncodeFrame(black) sample mismatch")
	}
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if _, err := enc.EncodeFrame(img); err != ErrBitstreamUnavailable {
		t.Fatalf("EncodeFrame(non-black) error = %v, want %v", err, ErrBitstreamUnavailable)
	}
}

func TestEncodeAllIntraMP4ValidatesInput(t *testing.T) {
	enc, err := New(Config{Width: 16, Height: 16})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := enc.EncodeAllIntraMP4(nil, ioDiscard{}); err != ErrNilFrameStream {
		t.Fatalf("EncodeAllIntraMP4(nil stream) error = %v, want %v", err, ErrNilFrameStream)
	}
	ch := make(chan *image.YCbCr)
	close(ch)
	if err := enc.EncodeAllIntraMP4(ch, ioDiscard{}); err != ErrEmptyFrameStream {
		t.Fatalf("EncodeAllIntraMP4(empty stream) error = %v, want %v", err, ErrEmptyFrameStream)
	}
	ch = make(chan *image.YCbCr, 1)
	ch <- nil
	close(ch)
	if err := enc.EncodeAllIntraMP4(ch, ioDiscard{}); err != ErrNilImage {
		t.Fatalf("EncodeAllIntraMP4(nil frame) error = %v, want %v", err, ErrNilImage)
	}
	ch = make(chan *image.YCbCr, 1)
	ch <- image.NewYCbCr(image.Rect(0, 0, 8, 8), image.YCbCrSubsampleRatio420)
	close(ch)
	if err := enc.EncodeAllIntraMP4(ch, ioDiscard{}); err != ErrFrameDimensions {
		t.Fatalf("EncodeAllIntraMP4(dim mismatch) error = %v, want %v", err, ErrFrameDimensions)
	}
	ch = make(chan *image.YCbCr, 1)
	ch <- blackYCbCr16()
	close(ch)
	if err := enc.EncodeAllIntraMP4(ch, nil); err != ErrNilWriter {
		t.Fatalf("EncodeAllIntraMP4(nil writer) error = %v, want %v", err, ErrNilWriter)
	}
}

func TestEncodeAllIntraMP4WritesPlayableBlackTrack(t *testing.T) {
	enc, err := New(Config{Width: 16, Height: 16, Timescale: 1000})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "black16.mp4")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer outFile.Close()

	ch := make(chan *image.YCbCr, 2)
	ch <- blackYCbCr16()
	ch <- blackYCbCr16()
	close(ch)
	if err := enc.EncodeAllIntraMP4(ch, outFile); err != nil {
		t.Fatalf("EncodeAllIntraMP4: %v", err)
	}

	f, err := mp4.Open(outPath)
	if err != nil {
		t.Fatalf("mp4.Open: %v", err)
	}
	defer f.Close()
	track, err := f.AV1VideoTrack()
	if err != nil {
		t.Fatalf("AV1VideoTrack: %v", err)
	}
	if got, want := len(track.Samples), 2; got != want {
		t.Fatalf("sample count = %d, want %d", got, want)
	}
	raw, err := track.AV1C.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if !bytes.Equal(raw, black16x16AV1C) {
		t.Fatalf("av1c mismatch")
	}

	dec, err := decoder.OpenMP4(outPath)
	if err != nil {
		t.Fatalf("decoder.OpenMP4: %v", err)
	}
	frame, err := dec.NextFrame()
	if err != nil {
		t.Fatalf("NextFrame: %v", err)
	}
	if frame.Width != 16 || frame.Height != 16 {
		t.Fatalf("frame dims = %dx%d, want 16x16", frame.Width, frame.Height)
	}
	if frame.BitDepth != 8 {
		t.Fatalf("frame bit depth = %d, want 8", frame.BitDepth)
	}
}

func TestEncodedTemplateParsesAsFrameOBU(t *testing.T) {
	obus, err := obu.Split(black16x16Sample)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(obus) == 0 {
		t.Fatal("no obu units")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func blackYCbCr16() *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio420)
	for i := range img.Cb {
		img.Cb[i] = 128
		img.Cr[i] = 128
	}
	return img
}
