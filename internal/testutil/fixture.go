package testutil

import (
	"fmt"
	"path/filepath"
	"testing"
)

type AV1FixtureOptions struct {
	Name     string
	Width    int
	Height   int
	FPS      int
	Frames   int
	BitDepth int
}

type AV1Fixture struct {
	Path     string
	Width    int
	Height   int
	FPS      int
	Frames   int
	BitDepth int
}

func CreateAV1MP4Fixture(tb testing.TB, dir string, opts AV1FixtureOptions) AV1Fixture {
	tb.Helper()

	RequireTool(tb, "ffmpeg")

	if dir == "" {
		tb.Fatalf("fixture directory must not be empty")
	}

	if opts.Width <= 0 {
		opts.Width = 160
	}
	if opts.Height <= 0 {
		opts.Height = 90
	}
	if opts.FPS <= 0 {
		opts.FPS = 5
	}
	if opts.Frames <= 0 {
		opts.Frames = 5
	}
	if opts.BitDepth == 0 {
		opts.BitDepth = 8
	}
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("fixture-%dbit.mp4", opts.BitDepth)
	}

	pixFmt := "yuv420p"
	if opts.BitDepth > 8 {
		pixFmt = "yuv420p10le"
	}

	path := filepath.Join(dir, opts.Name)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=size=%dx%d:rate=%d", opts.Width, opts.Height, opts.FPS),
		"-frames:v", fmt.Sprintf("%d", opts.Frames),
		"-pix_fmt", pixFmt,
		"-c:v", "libaom-av1",
		"-cpu-used", "8",
		"-crf", "42",
		"-b:v", "0",
		path,
	}

	cmd := Command("ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("ffmpeg fixture generation failed: %v\n%s", err, string(out))
	}

	return AV1Fixture{
		Path:     path,
		Width:    opts.Width,
		Height:   opts.Height,
		FPS:      opts.FPS,
		Frames:   opts.Frames,
		BitDepth: opts.BitDepth,
	}
}
