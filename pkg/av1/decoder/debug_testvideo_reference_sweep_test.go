//go:build av1debug

package decoder

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

func TestDebugTestvideoReferenceSweep(t *testing.T) {
	if os.Getenv("DEBUG_TESTVIDEO_REFERENCE_SWEEP") == "" {
		t.Skip("debug only")
	}

	root := strings.TrimSpace(os.Getenv("DEBUG_TESTVIDEO_REFERENCE_ROOT"))
	if root == "" {
		root = filepath.Join("..", "..", "..", "testvideo")
	}
	match := strings.TrimSpace(os.Getenv("DEBUG_TESTVIDEO_REFERENCE_MATCH"))
	limit := debugEnvInt("DEBUG_TESTVIDEO_REFERENCE_LIMIT", 3)
	if limit <= 0 {
		t.Fatalf("DEBUG_TESTVIDEO_REFERENCE_LIMIT must be positive, got %d", limit)
	}
	failOnMismatch := os.Getenv("DEBUG_TESTVIDEO_REFERENCE_FAIL") != ""
	maxRMSE := debugEnvFloat("DEBUG_TESTVIDEO_REFERENCE_MAX_RMSE", 3.0)
	maxAbs := debugEnvInt("DEBUG_TESTVIDEO_REFERENCE_MAX_ABS", 24)

	paths, err := filepath.Glob(filepath.Join(root, "*.mp4"))
	if err != nil {
		t.Fatalf("glob testvideo mp4s: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("no mp4 files under %s", root)
	}

	for _, path := range paths {
		name := filepath.Base(path)
		if match != "" && !strings.Contains(name, match) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			refs := debugDecodeReferenceFrames420(t, path, limit)
			dec, err := OpenMP4(path)
			if err != nil {
				t.Fatalf("OpenMP4: %v", err)
			}
			defer dec.Close()

			for i, ref := range refs {
				frame, err := dec.NextFrame()
				if err == io.EOF {
					t.Fatalf("NextFrame(%d): EOF", i)
				}
				if err != nil {
					t.Fatalf("NextFrame(%d): %v", i, err)
				}
				metrics, detail := compareFrameToReference(frame, ref)
				firstX, firstY, firstGot, firstWant, firstOK := debugFirstYMismatchOverAny(frame, ref, maxAbs)
				if os.Getenv("DEBUG_TESTVIDEO_REFERENCE_PLANE_DETAIL") != "" {
					for _, plane := range []string{"U", "V"} {
						px, py, got, want, ok := debugFirstChromaMismatchOverAny(frame, ref, plane, maxAbs)
						if ok {
							t.Logf("frame=%02d first%s>%d=(%d,%d) got=%d ref=%d", i, plane, maxAbs, px, py, got, want)
						}
					}
				}
				if firstOK {
					t.Logf("frame=%02d %s y=%.4f/%d u=%.4f/%d v=%.4f/%d firstY>%d=(%d,%d) got=%d ref=%d",
						i, detail,
						metrics.Y.RMSE, metrics.Y.MaxAbs,
						metrics.U.RMSE, metrics.U.MaxAbs,
						metrics.V.RMSE, metrics.V.MaxAbs,
						maxAbs, firstX, firstY, firstGot, firstWant)
				} else {
					t.Logf("frame=%02d %s y=%.4f/%d u=%.4f/%d v=%.4f/%d firstY>%d=none",
						i, detail,
						metrics.Y.RMSE, metrics.Y.MaxAbs,
						metrics.U.RMSE, metrics.U.MaxAbs,
						metrics.V.RMSE, metrics.V.MaxAbs,
						maxAbs)
				}
				if failOnMismatch {
					debugAssertPlaneMetrics(t, "Y", metrics.Y, maxRMSE, maxAbs)
					debugAssertPlaneMetrics(t, "U", metrics.U, maxRMSE, maxAbs)
					debugAssertPlaneMetrics(t, "V", metrics.V, maxRMSE, maxAbs)
				}
				_ = frame.Close()
			}
		})
	}
}

func TestDebugTestvideoHeaderSweep(t *testing.T) {
	if os.Getenv("DEBUG_TESTVIDEO_HEADER_SWEEP") == "" {
		t.Skip("debug only")
	}
	root := strings.TrimSpace(os.Getenv("DEBUG_TESTVIDEO_REFERENCE_ROOT"))
	if root == "" {
		root = filepath.Join("..", "..", "..", "testvideo")
	}
	match := strings.TrimSpace(os.Getenv("DEBUG_TESTVIDEO_REFERENCE_MATCH"))
	limit := debugEnvInt("DEBUG_TESTVIDEO_HEADER_LIMIT", 6)

	paths, err := filepath.Glob(filepath.Join(root, "*.mp4"))
	if err != nil {
		t.Fatalf("glob testvideo mp4s: %v", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		name := filepath.Base(path)
		if match != "" && !strings.Contains(name, match) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dec, err := OpenMP4(path)
			if err != nil {
				t.Fatalf("OpenMP4: %v", err)
			}
			defer dec.Close()
			t.Logf("meta bitDepth=%d chroma=%v size=%dx%d codec=%s", dec.metadata.BitDepth, dec.metadata.Chroma, dec.metadata.Width, dec.metadata.Height, dec.metadata.CodecString)
			for i := 0; i < limit; i++ {
				parsed, err := dec.NextParsedFrame()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("NextParsedFrame(%d): %v", i, err)
				}
				t.Logf("parsed=%02d sample=%d key=%v show=%v showExisting=%v type=%d off=%d width=%d height=%d q=%d deltaQ=%v/%d deltaLF=%v/%d/%v allLossless=%v intrabc=%v reducedTx=%v filmGrain=%v disableCDF=%v refreshCtx=%v refresh=%08b primary=%d refs=%v tiles=%d",
					i,
					parsed.SampleIndex,
					parsed.IsKeyframe,
					parsed.Header.ShowFrame,
					parsed.Header.ShowExistingFrame,
					parsed.Header.FrameType,
					parsed.Header.FrameOffset,
					parsed.Header.Width,
					parsed.Header.Height,
					parsed.Header.Quantization.YAC,
					parsed.Header.Delta.QPresent,
					parsed.Header.Delta.QResLog2,
					parsed.Header.Delta.LFPresent,
					parsed.Header.Delta.LFResLog2,
					parsed.Header.Delta.LFMulti,
					parsed.Header.AllLossless,
					parsed.Header.AllowIntrabc,
					parsed.Header.ReducedTxtpSet,
					parsed.Header.FilmGrainPresent,
					parsed.Header.DisableCDFUpdate,
					parsed.Header.RefreshContext,
					parsed.Header.RefreshFrameFlags,
					parsed.Header.PrimaryRefFrame,
					parsed.Header.RefIdx,
					len(parsed.TileGroup.Tiles),
				)
				tileLens := make([]int, len(parsed.TileGroup.Tiles))
				for ti := range parsed.TileGroup.Tiles {
					tileLens[ti] = len(parsed.TileGroup.Tiles[ti].Data)
				}
				t.Logf("tiling uniform=%v cols=%d rows=%d log2=%d/%d sizeBytes=%d update=%d colStart=%v rowStart=%v tileGroup=%d..%d tileLens=%v",
					parsed.Header.Tiling.Uniform,
					parsed.Header.Tiling.Cols,
					parsed.Header.Tiling.Rows,
					parsed.Header.Tiling.Log2Cols,
					parsed.Header.Tiling.Log2Rows,
					parsed.Header.Tiling.SizeBytes,
					parsed.Header.Tiling.Update,
					parsed.Header.Tiling.ColStartSB[:parsed.Header.Tiling.Cols+1],
					parsed.Header.Tiling.RowStartSB[:parsed.Header.Tiling.Rows+1],
					parsed.TileGroup.Start,
					parsed.TileGroup.End,
					tileLens,
				)
				if parsed.Header.ShowExistingFrame || len(parsed.TileGroup.Tiles) == 0 {
					continue
				}
				states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
				if err != nil {
					t.Fatalf("BuildTileStates: %v", err)
				}
				for _, state := range states {
					g := state.Geometry
					t.Logf("tile index=%d row=%d col=%d sbX=%d..%d sbY=%d..%d px=(%d,%d)..(%d,%d) data=%d",
						g.Index,
						g.Row,
						g.Col,
						g.StartSBX,
						g.EndSBX,
						g.StartSBY,
						g.EndSBY,
						g.StartX,
						g.StartY,
						g.EndX,
						g.EndY,
						len(state.Data),
					)
				}
				CloseTileStates(states)
			}
		})
	}
}

func debugDecodeReferenceFrames420(t *testing.T, path string, limit int) []testutil.RawFrame420 {
	t.Helper()
	testutil.RequireTool(t, "ffmpeg")

	probe := testutil.ProbeFile(t, path)
	stream := testutil.StreamByType(t, probe, "video")
	pixFmt, bitDepth, err := debugReference420PixFmt(stream.PixFmt)
	if err != nil {
		t.Fatalf("reference decode setup for %s: %v", path, err)
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
		t.Fatalf("ffmpeg reference decode failed: %v\n%s", err, strings.TrimSpace(stderr.String()))
	}

	width := stream.Width
	height := stream.Height
	if width <= 0 || height <= 0 {
		t.Fatalf("invalid probed dimensions %dx%d", width, height)
	}
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	lumaSamples := width * height
	chromaSamples := chromaWidth * chromaHeight

	frameBytes := lumaSamples + 2*chromaSamples
	if bitDepth > 8 {
		frameBytes *= 2
	}
	if stdout.Len()%frameBytes != 0 {
		t.Fatalf("ffmpeg raw output size %d is not a multiple of frame size %d", stdout.Len(), frameBytes)
	}

	raw := stdout.Bytes()
	frameCount := stdout.Len() / frameBytes
	out := make([]testutil.RawFrame420, 0, frameCount)
	for i := 0; i < frameCount; i++ {
		frame, err := debugDecodeRawFrame420(raw[i*frameBytes:(i+1)*frameBytes], width, height, bitDepth)
		if err != nil {
			t.Fatalf("decode raw reference frame %d: %v", i, err)
		}
		out = append(out, frame)
	}
	return out
}

func debugReference420PixFmt(pixFmt string) (string, uint8, error) {
	switch strings.TrimSpace(pixFmt) {
	case "yuv420p":
		return "yuv420p", 8, nil
	case "yuv420p10le":
		return "yuv420p10le", 10, nil
	default:
		return "", 0, fmt.Errorf("unsupported reference pixel format %q", pixFmt)
	}
}

func debugDecodeRawFrame420(raw []byte, width, height int, bitDepth uint8) (testutil.RawFrame420, error) {
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	lumaSamples := width * height
	chromaSamples := chromaWidth * chromaHeight
	frame := testutil.RawFrame420{
		Width:    width,
		Height:   height,
		BitDepth: bitDepth,
	}
	if bitDepth > 8 {
		wantBytes := (lumaSamples + 2*chromaSamples) * 2
		if len(raw) != wantBytes {
			return testutil.RawFrame420{}, fmt.Errorf("10-bit raw frame len=%d want=%d", len(raw), wantBytes)
		}
		values := make([]uint16, lumaSamples+2*chromaSamples)
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
		return testutil.RawFrame420{}, fmt.Errorf("8-bit raw frame len=%d want=%d", len(raw), wantBytes)
	}
	frame.Y = append([]byte(nil), raw[:lumaSamples]...)
	frame.U = append([]byte(nil), raw[lumaSamples:lumaSamples+chromaSamples]...)
	frame.V = append([]byte(nil), raw[lumaSamples+chromaSamples:]...)
	return frame, nil
}

func debugEnvFloat(name string, fallback float64) float64 {
	spec := strings.TrimSpace(os.Getenv(name))
	if spec == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(spec, 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func debugAssertPlaneMetrics(t *testing.T, plane string, got testutil.PlaneMetrics, maxRMSE float64, maxAbs int) {
	t.Helper()
	if got.RMSE > maxRMSE || got.MaxAbs > maxAbs {
		t.Fatalf("%s metrics: rmse=%.4f mean_abs=%.4f max_abs=%d (limits rmse<=%.4f max_abs<=%d)",
			plane, got.RMSE, got.MeanAbs, got.MaxAbs, maxRMSE, maxAbs)
	}
}

func debugFirstYMismatchOverAny(frame *Frame, ref testutil.RawFrame420, threshold int) (int, int, int, int, bool) {
	if ref.BitDepth > 8 {
		got := flattenPlane16(frame.Y16, frame.YStride, ref.Width, ref.Height)
		for y := 0; y < ref.Height; y++ {
			for x := 0; x < ref.Width; x++ {
				idx := y*ref.Width + x
				diff := int(got[idx]) - int(ref.Y16[idx])
				if diff < 0 {
					diff = -diff
				}
				if diff > threshold {
					return x, y, int(got[idx]), int(ref.Y16[idx]), true
				}
			}
		}
		return 0, 0, 0, 0, false
	}
	x, y, got, want, ok := debugFirstYMismatchOver(frame, ref, threshold)
	return x, y, int(got), int(want), ok
}

func debugFirstChromaMismatchOverAny(frame *Frame, ref testutil.RawFrame420, plane string, threshold int) (int, int, int, int, bool) {
	chromaWidth := (ref.Width + 1) >> 1
	chromaHeight := (ref.Height + 1) >> 1
	if ref.BitDepth > 8 {
		var got []uint16
		var want []uint16
		switch plane {
		case "U":
			got = flattenPlane16(frame.U16, frame.UStride, chromaWidth, chromaHeight)
			want = ref.U16
		case "V":
			got = flattenPlane16(frame.V16, frame.VStride, chromaWidth, chromaHeight)
			want = ref.V16
		default:
			return 0, 0, 0, 0, false
		}
		for y := 0; y < chromaHeight; y++ {
			for x := 0; x < chromaWidth; x++ {
				idx := y*chromaWidth + x
				diff := int(got[idx]) - int(want[idx])
				if diff < 0 {
					diff = -diff
				}
				if diff > threshold {
					return x, y, int(got[idx]), int(want[idx]), true
				}
			}
		}
		return 0, 0, 0, 0, false
	}

	var got []byte
	var want []byte
	switch plane {
	case "U":
		got = flattenPlane8(frame.U, frame.UStride, chromaWidth, chromaHeight)
		want = ref.U
	case "V":
		got = flattenPlane8(frame.V, frame.VStride, chromaWidth, chromaHeight)
		want = ref.V
	default:
		return 0, 0, 0, 0, false
	}
	for y := 0; y < chromaHeight; y++ {
		for x := 0; x < chromaWidth; x++ {
			idx := y*chromaWidth + x
			diff := int(got[idx]) - int(want[idx])
			if diff < 0 {
				diff = -diff
			}
			if diff > threshold {
				return x, y, int(got[idx]), int(want[idx]), true
			}
		}
	}
	return 0, 0, 0, 0, false
}
