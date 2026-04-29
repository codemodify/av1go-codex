package encoder_test

import (
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestSequenceHeaderSupportClassification(t *testing.T) {
	supported := av1.SequenceHeader{
		Profile: av1.ProfileMain,
		ColorConfig: av1.ColorConfig{
			BitDepth:     8,
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	if !supported.SupportedMain8Bit420() {
		t.Fatalf("supported 8-bit 4:2:0 header rejected")
	}

	tenBit := supported
	tenBit.ColorConfig.BitDepth = 10
	if tenBit.SupportedMain8Bit420() {
		t.Fatalf("10-bit header accepted")
	}
	if !tenBit.SupportedMain10Bit420() {
		t.Fatalf("supported 10-bit 4:2:0 header rejected")
	}

	rgb := supported
	rgb.ColorConfig.SubsamplingX = false
	rgb.ColorConfig.SubsamplingY = false
	if rgb.SupportedMain8Bit420() {
		t.Fatalf("4:4:4 header accepted")
	}
	if rgb.SupportedMain10Bit420() {
		t.Fatalf("4:4:4 10-bit header accepted")
	}
}

func TestV1EncoderFixturePolicy(t *testing.T) {
	main8 := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "main8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})
	main10 := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   4,
		BitDepth: 10,
	})

	main8Probe := testutil.ProbeFile(t, main8.Path)
	main8Video := testutil.StreamByType(t, main8Probe, "video")
	if main8Video.CodecName != "av1" {
		t.Fatalf("codec = %s, want av1", main8Video.CodecName)
	}
	if main8Video.PixFmt != "yuv420p" {
		t.Fatalf("pix_fmt = %s, want yuv420p", main8Video.PixFmt)
	}

	main10Probe := testutil.ProbeFile(t, main10.Path)
	main10Video := testutil.StreamByType(t, main10Probe, "video")
	if main10Video.CodecName != "av1" {
		t.Fatalf("codec = %s, want av1", main10Video.CodecName)
	}
	if main10Video.PixFmt != "yuv420p10le" {
		t.Fatalf("pix_fmt = %s, want yuv420p10le", main10Video.PixFmt)
	}
}
