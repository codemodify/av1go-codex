package mp4_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

func TestReferenceMP4VideoOnlyRemuxBaseline(t *testing.T) {
	ffmpeg := testutil.RequireCommand(t, "ffmpeg")
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "remux-src.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "remuxed.mp4")

	cmd := testutil.Command(ffmpeg,
		"-v", "error",
		"-i", fixture.Path,
		"-map", "0:v:0",
		"-c", "copy",
		"-movflags", "+faststart",
		out,
	)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg remux failed: %v\n%s", err, outBytes)
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("missing remux output: %v", err)
	}

	probe := testutil.ProbeFile(t, out)
	if got := len(probe.Streams); got != 1 {
		t.Fatalf("stream count = %d, want 1", got)
	}
	video := testutil.StreamByType(t, probe, "video")
	if video.CodecName != "av1" {
		t.Fatalf("codec = %s, want av1", video.CodecName)
	}
	if video.Width != fixture.Width || video.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", video.Width, video.Height, fixture.Width, fixture.Height)
	}
}
