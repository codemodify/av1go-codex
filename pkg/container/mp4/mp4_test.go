package mp4

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

func TestOpenAV1VideoTrack(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "track.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	f, err := Open(fixture.Path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	track, err := f.AV1VideoTrack()
	if err != nil {
		t.Fatalf("AV1VideoTrack: %v", err)
	}

	if got, want := track.Width, fixture.Width; got != want {
		t.Fatalf("Width = %d, want %d", got, want)
	}
	if got, want := track.Height, fixture.Height; got != want {
		t.Fatalf("Height = %d, want %d", got, want)
	}
	if track.Timescale == 0 {
		t.Fatal("expected non-zero timescale")
	}
	if got, want := len(track.Samples), fixture.Frames; got != want {
		t.Fatalf("sample count = %d, want %d", got, want)
	}
	if !track.Samples[0].Keyframe {
		t.Fatalf("first sample should be a keyframe")
	}
	if got := len(track.AV1C.Raw); got == 0 {
		t.Fatalf("expected av1C raw bytes")
	}
	if got, want := track.AV1C.BitDepth(), uint8(fixture.BitDepth); got != want {
		t.Fatalf("BitDepth = %d, want %d", got, want)
	}
	if got := track.AV1C.CodecString(); got == "" {
		t.Fatalf("expected codec string")
	}

	sample, err := track.ReadSample(0)
	if err != nil {
		t.Fatalf("ReadSample: %v", err)
	}
	if len(sample) != track.Samples[0].Size {
		t.Fatalf("sample bytes = %d, want %d", len(sample), track.Samples[0].Size)
	}
}

func TestWriteVideoRoundTrip(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "roundtrip-src.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	f, err := Open(fixture.Path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	track, err := f.AV1VideoTrack()
	if err != nil {
		t.Fatalf("AV1VideoTrack: %v", err)
	}

	samples := make([]SampleInput, 0, 3)
	for i := 0; i < 3; i++ {
		data, err := track.ReadSample(i)
		if err != nil {
			t.Fatalf("ReadSample(%d): %v", i, err)
		}
		s := track.Samples[i]
		samples = append(samples, SampleInput{
			Data:              data,
			Duration:          s.Duration,
			CompositionOffset: s.CompositionOffset,
			Keyframe:          s.Keyframe,
		})
	}

	outPath := filepath.Join(t.TempDir(), "roundtrip.mp4")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cfg := VideoConfig{
		Width:     track.Width,
		Height:    track.Height,
		Timescale: track.Timescale,
		AV1C:      track.AV1C.Clone(),
	}
	if err := WriteVideo(outFile, cfg, samples); err != nil {
		_ = outFile.Close()
		t.Fatalf("WriteVideo: %v", err)
	}
	if err := outFile.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rt, err := Open(outPath)
	if err != nil {
		t.Fatalf("roundtrip Open: %v", err)
	}
	defer rt.Close()
	rtTrack, err := rt.AV1VideoTrack()
	if err != nil {
		t.Fatalf("roundtrip AV1VideoTrack: %v", err)
	}
	if got, want := rtTrack.Width, track.Width; got != want {
		t.Fatalf("roundtrip width = %d, want %d", got, want)
	}
	if got, want := rtTrack.Height, track.Height; got != want {
		t.Fatalf("roundtrip height = %d, want %d", got, want)
	}
	if got, want := len(rtTrack.Samples), len(samples); got != want {
		t.Fatalf("roundtrip sample count = %d, want %d", got, want)
	}
	rtSample, err := rtTrack.ReadSample(0)
	if err != nil {
		t.Fatalf("roundtrip ReadSample: %v", err)
	}
	if len(rtSample) != len(samples[0].Data) {
		t.Fatalf("roundtrip sample size = %d, want %d", len(rtSample), len(samples[0].Data))
	}
}
