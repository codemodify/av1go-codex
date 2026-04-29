package mp4_test

import (
	"math"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

func TestGeneratedFrameCadence(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "cadence.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	frames := testutil.ProbeFrames(t, fixture.Path, fixture.Frames)
	if len(frames) < 3 {
		t.Fatalf("got %d frames, want at least 3", len(frames))
	}

	if frames[0].KeyFrame != 1 || frames[0].PictType != "I" {
		t.Fatalf("first frame = key=%d type=%s, want key=1 type=I", frames[0].KeyFrame, frames[0].PictType)
	}
	wantStep := 1.0 / float64(fixture.FPS)
	for i := 1; i < len(frames); i++ {
		prev := testutil.MustParseFloat64(t, frames[i-1].BestEffortTimestampTime)
		got := testutil.MustParseFloat64(t, frames[i].BestEffortTimestampTime) - prev
		if math.Abs(got-wantStep) > 0.01 {
			t.Fatalf("frame %d step = %.6f, want %.6f", i, got, wantStep)
		}
	}
}
