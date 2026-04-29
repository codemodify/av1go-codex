package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestSupportedMain8Bit420FixtureMetadata(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "metadata8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	probe := testutil.ProbeAV1VideoStream(t, fixture.Path)
	header, err := testutil.SequenceHeaderFromProbe(t, probe)
	if err != nil {
		t.Fatalf("parse codec config: %v", err)
	}

	if probe.CodecName != "av1" {
		t.Fatalf("codec_name = %q, want av1", probe.CodecName)
	}
	if probe.Width != fixture.Width || probe.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", probe.Width, probe.Height, fixture.Width, fixture.Height)
	}
	if header.Profile != av1.ProfileMain {
		t.Fatalf("profile = %v, want Main", header.Profile)
	}
	if header.ColorConfig.BitDepth != 8 {
		t.Fatalf("bit_depth = %d, want 8", header.ColorConfig.BitDepth)
	}
	if !header.SupportedMain8Bit420() {
		t.Fatalf("header should be supported: %+v", header)
	}
}

func TestTenBitFixtureMetadata(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "metadata10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   4,
		BitDepth: 10,
	})

	probe := testutil.ProbeAV1VideoStream(t, fixture.Path)
	header, err := testutil.SequenceHeaderFromProbe(t, probe)
	if err != nil {
		t.Fatalf("parse codec config: %v", err)
	}

	if header.ColorConfig.BitDepth != 10 {
		t.Fatalf("bit_depth = %d, want 10", header.ColorConfig.BitDepth)
	}
	if header.SupportedMain8Bit420() {
		t.Fatalf("10-bit fixture should not be reported as main 8-bit 4:2:0")
	}
	if !header.SupportedMain10Bit420() {
		t.Fatalf("10-bit fixture should be reported as main 10-bit 4:2:0")
	}
}

func TestCodecConfigRejectsMalformedExtradata(t *testing.T) {
	_, err := testutil.ParseCodecConfigFromAV1C(t, []byte{0x00, 0x04})
	if err == nil {
		t.Fatal("expected malformed codec config to fail")
	}

	_, err = testutil.ParseCodecConfigFromAV1C(t, []byte{0x81})
	if err == nil {
		t.Fatal("expected truncated codec config to fail")
	}
}
