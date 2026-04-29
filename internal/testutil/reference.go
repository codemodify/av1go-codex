package testutil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"testing"
)

type RawFrame420 struct {
	Width    int
	Height   int
	BitDepth uint8
	Y        []byte
	U        []byte
	V        []byte
	Y16      []uint16
	U16      []uint16
	V16      []uint16
}

type PlaneMetrics struct {
	RMSE    float64
	MeanAbs float64
	MaxAbs  int
}

type FrameMetrics420 struct {
	Y PlaneMetrics
	U PlaneMetrics
	V PlaneMetrics
}

func DecodeReferenceFrames420(tb testing.TB, path string, limit int) []RawFrame420 {
	tb.Helper()
	RequireTool(tb, "ffmpeg")
	RequireFile(tb, path)

	if limit <= 0 {
		tb.Fatalf("DecodeReferenceFrames420 limit must be positive, got %d", limit)
	}

	probe := ProbeFile(tb, path)
	stream := StreamByType(tb, probe, "video")
	pixFmt, bitDepth, err := reference420PixFmt(stream.PixFmt)
	if err != nil {
		tb.Fatalf("reference decode setup for %s: %v", path, err)
	}

	args := []string{
		"-v", "error",
		"-i", path,
		"-map", "0:v:0",
		"-frames:v", fmt.Sprintf("%d", limit),
		"-pix_fmt", pixFmt,
		"-f", "rawvideo",
		"-",
	}
	cmd := exec.Command("ffmpeg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tb.Fatalf("ffmpeg reference decode failed: %v\n%s", err, strings.TrimSpace(stderr.String()))
	}

	width := stream.Width
	height := stream.Height
	if width <= 0 || height <= 0 {
		tb.Fatalf("invalid probed dimensions %dx%d", width, height)
	}
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	lumaSamples := width * height
	chromaSamples := chromaWidth * chromaHeight

	var frameBytes int
	if bitDepth > 8 {
		frameBytes = (lumaSamples + 2*chromaSamples) * 2
	} else {
		frameBytes = lumaSamples + 2*chromaSamples
	}

	if stdout.Len()%frameBytes != 0 {
		tb.Fatalf("ffmpeg raw output size %d is not a multiple of frame size %d", stdout.Len(), frameBytes)
	}

	frameCount := stdout.Len() / frameBytes
	frames := make([]RawFrame420, 0, frameCount)
	raw := stdout.Bytes()
	for i := 0; i < frameCount; i++ {
		start := i * frameBytes
		end := start + frameBytes
		frame, err := decodeRawFrame420(raw[start:end], width, height, bitDepth)
		if err != nil {
			tb.Fatalf("decode raw reference frame %d: %v", i, err)
		}
		frames = append(frames, frame)
	}
	return frames
}

func reference420PixFmt(pixFmt string) (string, uint8, error) {
	switch strings.TrimSpace(pixFmt) {
	case "yuv420p":
		return "yuv420p", 8, nil
	case "yuv420p10le":
		return "yuv420p10le", 10, nil
	default:
		return "", 0, fmt.Errorf("unsupported reference pixel format %q", pixFmt)
	}
}

func decodeRawFrame420(raw []byte, width, height int, bitDepth uint8) (RawFrame420, error) {
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	lumaSamples := width * height
	chromaSamples := chromaWidth * chromaHeight

	frame := RawFrame420{
		Width:    width,
		Height:   height,
		BitDepth: bitDepth,
	}
	if bitDepth > 8 {
		wantBytes := (lumaSamples + 2*chromaSamples) * 2
		if len(raw) != wantBytes {
			return RawFrame420{}, fmt.Errorf("10-bit raw frame len=%d want=%d", len(raw), wantBytes)
		}
		values := make([]uint16, (lumaSamples + 2*chromaSamples))
		for i := range values {
			values[i] = binary.LittleEndian.Uint16(raw[i*2:])
		}
		frame.Y16 = append([]uint16(nil), values[:lumaSamples]...)
		frame.U16 = append([]uint16(nil), values[lumaSamples:lumaSamples+chromaSamples]...)
		frame.V16 = append([]uint16(nil), values[lumaSamples+chromaSamples:]...)
		return frame, nil
	}

	wantBytes := lumaSamples + 2*chromaSamples
	if len(raw) != wantBytes {
		return RawFrame420{}, fmt.Errorf("8-bit raw frame len=%d want=%d", len(raw), wantBytes)
	}
	frame.Y = append([]byte(nil), raw[:lumaSamples]...)
	frame.U = append([]byte(nil), raw[lumaSamples:lumaSamples+chromaSamples]...)
	frame.V = append([]byte(nil), raw[lumaSamples+chromaSamples:]...)
	return frame, nil
}

func ComparePlane8(got, want []byte) PlaneMetrics {
	if len(got) != len(want) {
		panic(fmt.Sprintf("ComparePlane8 length mismatch: got=%d want=%d", len(got), len(want)))
	}
	if len(got) == 0 {
		return PlaneMetrics{}
	}
	var sumAbs float64
	var sumSq float64
	maxAbs := 0
	for i := range got {
		diff := int(got[i]) - int(want[i])
		abs := diff
		if abs < 0 {
			abs = -abs
		}
		if abs > maxAbs {
			maxAbs = abs
		}
		sumAbs += float64(abs)
		sumSq += float64(diff * diff)
	}
	n := float64(len(got))
	return PlaneMetrics{
		RMSE:    math.Sqrt(sumSq / n),
		MeanAbs: sumAbs / n,
		MaxAbs:  maxAbs,
	}
}

func ComparePlane16(got, want []uint16) PlaneMetrics {
	if len(got) != len(want) {
		panic(fmt.Sprintf("ComparePlane16 length mismatch: got=%d want=%d", len(got), len(want)))
	}
	if len(got) == 0 {
		return PlaneMetrics{}
	}
	var sumAbs float64
	var sumSq float64
	maxAbs := 0
	for i := range got {
		diff := int(got[i]) - int(want[i])
		abs := diff
		if abs < 0 {
			abs = -abs
		}
		if abs > maxAbs {
			maxAbs = abs
		}
		sumAbs += float64(abs)
		sumSq += float64(diff * diff)
	}
	n := float64(len(got))
	return PlaneMetrics{
		RMSE:    math.Sqrt(sumSq / n),
		MeanAbs: sumAbs / n,
		MaxAbs:  maxAbs,
	}
}
