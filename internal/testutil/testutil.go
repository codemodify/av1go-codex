package testutil

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

type ProbeResult struct {
	Streams []ProbeStream `json:"streams"`
	Format  ProbeFormat   `json:"format"`
}

type ProbeStream struct {
	Index        int    `json:"index"`
	CodecName    string `json:"codec_name"`
	CodecType    string `json:"codec_type"`
	Profile      string `json:"profile"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	PixFmt       string `json:"pix_fmt"`
	AvgFrameRate string `json:"avg_frame_rate"`
	TimeBase     string `json:"time_base"`
	NbFrames     string `json:"nb_frames"`
}

type ProbeFormat struct {
	Filename   string `json:"filename"`
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	Size       string `json:"size"`
	BitRate    string `json:"bit_rate"`
	Tags       struct {
		MajorBrand       string `json:"major_brand"`
		MinorVersion     string `json:"minor_version"`
		CompatibleBrands string `json:"compatible_brands"`
		Encoder          string `json:"encoder"`
	} `json:"tags"`
}

type ProbeFrame struct {
	KeyFrame                int    `json:"key_frame"`
	PictType                string `json:"pict_type"`
	CodedPictureNumber      int    `json:"coded_picture_number"`
	BestEffortTimestampTime string `json:"best_effort_timestamp_time"`
}

func RepoRoot(tb testing.TB) string {
	tb.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("testutil: unable to determine repo root")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func RepoPath(tb testing.TB, elems ...string) string {
	tb.Helper()
	parts := append([]string{RepoRoot(tb)}, elems...)
	return filepath.Join(parts...)
}

func AssetPath(tb testing.TB, name string) string {
	tb.Helper()
	path := RepoPath(tb, "testvideo", name)
	RequireFile(tb, path)
	return path
}

func RequireCommand(tb testing.TB, name string) string {
	tb.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		tb.Skipf("%s not available: %v", name, err)
	}
	return path
}

func RunJSON(tb testing.TB, name string, args ...string) []byte {
	tb.Helper()

	cmdPath := RequireCommand(tb, name)
	cmd := exec.Command(cmdPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return out
}

func ProbeFile(tb testing.TB, path string) ProbeResult {
	tb.Helper()

	out := RunJSON(tb, "ffprobe",
		"-v", "error",
		"-show_streams",
		"-show_format",
		"-of", "json",
		path,
	)

	var probe ProbeResult
	if err := json.Unmarshal(out, &probe); err != nil {
		tb.Fatalf("decode ffprobe output for %s: %v\n%s", path, err, out)
	}
	return probe
}

func ProbeFrames(tb testing.TB, path string, limit int) []ProbeFrame {
	tb.Helper()

	out := RunJSON(tb, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_frames",
		"-show_entries", "frame=key_frame,pict_type,coded_picture_number,best_effort_timestamp_time",
		"-of", "json",
		path,
	)

	var raw struct {
		Frames []ProbeFrame `json:"frames"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		tb.Fatalf("decode ffprobe frame output for %s: %v\n%s", path, err, out)
	}
	if limit > 0 && len(raw.Frames) > limit {
		raw.Frames = raw.Frames[:limit]
	}
	return raw.Frames
}

func StreamByType(tb testing.TB, probe ProbeResult, codecType string) ProbeStream {
	tb.Helper()

	for _, s := range probe.Streams {
		if s.CodecType == codecType {
			return s
		}
	}
	tb.Fatalf("missing %s stream in %s", codecType, probe.Format.Filename)
	panic("unreachable")
}

func MustParseFloat64(tb testing.TB, s string) float64 {
	tb.Helper()
	v, err := parseFloat64(s)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

func parseFloat64(s string) (float64, error) {
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return 0, err
	}
	return v, nil
}
