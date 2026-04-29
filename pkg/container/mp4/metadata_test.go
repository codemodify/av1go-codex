package mp4_test

import (
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

func TestGeneratedAV1MP4Metadata(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "metadata.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	probe := testutil.ProbeFile(t, fixture.Path)
	if got := len(probe.Streams); got < 1 {
		t.Fatalf("stream count = %d, want at least 1", got)
	}

	video := testutil.StreamByType(t, probe, "video")
	if video.CodecName != "av1" {
		t.Fatalf("codec = %s, want av1", video.CodecName)
	}
	if !strings.EqualFold(video.Profile, "Main") {
		t.Fatalf("profile = %s, want Main", video.Profile)
	}
	if video.Width != fixture.Width || video.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", video.Width, video.Height, fixture.Width, fixture.Height)
	}
	if video.PixFmt != "yuv420p" {
		t.Fatalf("pix_fmt = %s, want yuv420p", video.PixFmt)
	}
}
