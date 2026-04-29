//go:build av1debug

package decoder

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

func TestDebugSPBTVFrameMetricsSweep(t *testing.T) {
	if os.Getenv("DEBUG_SPBTV_FRAME_SWEEP") == "" {
		t.Skip("debug only")
	}

	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	limit := debugEnvInt("DEBUG_SPBTV_FRAME_SWEEP_LIMIT", 40)
	refPath := strings.TrimSpace(os.Getenv("DEBUG_SPBTV_REF_YUV"))
	if refPath == "" {
		refPath = "/tmp/spbtv_ref_40.yuv"
	}

	refs := readDebugRaw420Frames(t, refPath, 960, 540, limit)
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
		metrics, _ := compareFrameToReference(frame, ref)
		worstX, worstY, worstRMSE := debugWorstYBlock(frame, ref, 16)
		firstX, firstY, firstGot, firstWant, firstOK := debugFirstYMismatchOver(frame, ref, 24)
		if firstOK {
			t.Logf("frame=%02d y=%.4f/%d u=%.4f/%d v=%.4f/%d worstY=(%d,%d) %.2f firstY>24=(%d,%d) got=%d ref=%d",
				i,
				metrics.Y.RMSE, metrics.Y.MaxAbs,
				metrics.U.RMSE, metrics.U.MaxAbs,
				metrics.V.RMSE, metrics.V.MaxAbs,
				worstX, worstY, worstRMSE,
				firstX, firstY, firstGot, firstWant,
			)
		} else {
			t.Logf("frame=%02d y=%.4f/%d u=%.4f/%d v=%.4f/%d worstY=(%d,%d) %.2f firstY>24=none",
				i,
				metrics.Y.RMSE, metrics.Y.MaxAbs,
				metrics.U.RMSE, metrics.U.MaxAbs,
				metrics.V.RMSE, metrics.V.MaxAbs,
				worstX, worstY, worstRMSE,
			)
		}
		_ = frame.Close()
	}
}

func TestDebugSPBTVParsedFrameMetricsSweep(t *testing.T) {
	if os.Getenv("DEBUG_SPBTV_PARSED_FRAME_SWEEP") == "" {
		t.Skip("debug only")
	}

	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	limit := debugEnvInt("DEBUG_SPBTV_PARSED_FRAME_SWEEP_LIMIT", 4)
	refPath := strings.TrimSpace(os.Getenv("DEBUG_SPBTV_REF_YUV"))
	if refPath == "" {
		refPath = "/tmp/spbtv_ref_all3.yuv"
	}

	refs := readDebugRaw420Frames(t, refPath, 960, 540, limit)
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for i, ref := range refs {
		parsed, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatalf("NextParsedFrame(%d): EOF", i)
		}
		if err != nil {
			t.Fatalf("NextParsedFrame(%d): %v", i, err)
		}
		frame, err := dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame(%d): %v", i, err)
		}
		metrics, _ := compareFrameToReference(frame, ref)
		worstX, worstY, worstRMSE := debugWorstYBlock(frame, ref, 16)
		firstX, firstY, firstGot, firstWant, firstOK := debugFirstYMismatchOver(frame, ref, 24)
		if firstOK {
			t.Logf("parsed=%02d show=%v offset=%d q=%d initCoefQCat=%d y=%.4f/%d u=%.4f/%d v=%.4f/%d worstY=(%d,%d) %.2f firstY>24=(%d,%d) got=%d ref=%d",
				i, parsed.Header.ShowFrame, parsed.Header.FrameOffset, parsed.Header.Quantization.YAC, parsed.InitialCoefQCat,
				metrics.Y.RMSE, metrics.Y.MaxAbs,
				metrics.U.RMSE, metrics.U.MaxAbs,
				metrics.V.RMSE, metrics.V.MaxAbs,
				worstX, worstY, worstRMSE,
				firstX, firstY, firstGot, firstWant,
			)
		} else {
			t.Logf("parsed=%02d show=%v offset=%d q=%d initCoefQCat=%d y=%.4f/%d u=%.4f/%d v=%.4f/%d worstY=(%d,%d) %.2f firstY>24=none",
				i, parsed.Header.ShowFrame, parsed.Header.FrameOffset, parsed.Header.Quantization.YAC, parsed.InitialCoefQCat,
				metrics.Y.RMSE, metrics.Y.MaxAbs,
				metrics.U.RMSE, metrics.U.MaxAbs,
				metrics.V.RMSE, metrics.V.MaxAbs,
				worstX, worstY, worstRMSE,
			)
		}
		_ = frame.Close()
	}
}

func TestDebugSPBTVHeaderTrace(t *testing.T) {
	if os.Getenv("DEBUG_SPBTV_HEADER_TRACE") == "" {
		t.Skip("debug only")
	}

	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	limit := debugEnvInt("DEBUG_SPBTV_HEADER_TRACE_LIMIT", 12)
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for i := 0; i < limit; i++ {
		parsed, err := dec.NextParsedFrame()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("NextParsedFrame(%d): %v", i, err)
		}
		h := parsed.Header
		t.Logf("parsed=%02d type=%d show=%v existing=%v offset=%d refresh=%08b primary=%d refs=%v refPOC=%v q=%d qcat=%d initCoefQCat=%d deltaQ=%v skipMode=%v useRefMVs=%v forceInt=%v highPrec=%v filterMode=%d switchableMotion=%v warp=%v size=%dx%d upscaled=%d superres=%v/%d",
			i, h.FrameType, h.ShowFrame, h.ShowExistingFrame, h.FrameOffset, h.RefreshFrameFlags, h.PrimaryRefFrame,
			h.RefIdx, parsed.RefFramePOC, h.Quantization.YAC, DefaultCoefQCat(h.Quantization.YAC), parsed.InitialCoefQCat, h.Delta.QPresent,
			h.SkipModeEnabled, h.UseRefFrameMvs, h.ForceIntegerMV, h.HighPrecisionMV,
			h.SubpelFilterMode, h.SwitchableMotion, h.WarpMotion, h.Width, h.Height, h.UpscaledWidth,
			h.SuperResEnabled, h.SuperResDenominator)
	}
}

func readDebugRaw420Frames(t *testing.T, path string, width, height, limit int) []testutil.RawFrame420 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read reference raw %s: %v", path, err)
	}
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	ySize := width * height
	uvSize := chromaWidth * chromaHeight
	frameSize := ySize + 2*uvSize
	if frameSize == 0 || len(raw)%frameSize != 0 {
		t.Fatalf("raw reference len=%d is not a multiple of frame size %d", len(raw), frameSize)
	}
	count := len(raw) / frameSize
	if limit > 0 && count > limit {
		count = limit
	}
	frames := make([]testutil.RawFrame420, 0, count)
	for i := 0; i < count; i++ {
		start := i * frameSize
		yStart := start
		uStart := yStart + ySize
		vStart := uStart + uvSize
		frames = append(frames, testutil.RawFrame420{
			Width:    width,
			Height:   height,
			BitDepth: 8,
			Y:        append([]byte(nil), raw[yStart:uStart]...),
			U:        append([]byte(nil), raw[uStart:vStart]...),
			V:        append([]byte(nil), raw[vStart:vStart+uvSize]...),
		})
	}
	return frames
}

func debugEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func debugWorstYBlock(frame *Frame, ref testutil.RawFrame420, blockSize int) (int, int, float64) {
	got := flattenPlane8(frame.Y, frame.YStride, ref.Width, ref.Height)
	worstX, worstY := 0, 0
	worstRMSE := -1.0
	for y := 0; y < ref.Height; y += blockSize {
		for x := 0; x < ref.Width; x += blockSize {
			w := blockSize
			if x+w > ref.Width {
				w = ref.Width - x
			}
			h := blockSize
			if y+h > ref.Height {
				h = ref.Height - y
			}
			rmse := debugRawYBlockRMSE(got, ref.Y, ref.Width, x, y, w, h)
			if rmse > worstRMSE {
				worstX, worstY, worstRMSE = x, y, rmse
			}
		}
	}
	return worstX, worstY, worstRMSE
}

func debugFirstYMismatchOver(frame *Frame, ref testutil.RawFrame420, threshold int) (int, int, byte, byte, bool) {
	got := flattenPlane8(frame.Y, frame.YStride, ref.Width, ref.Height)
	for y := 0; y < ref.Height; y++ {
		for x := 0; x < ref.Width; x++ {
			idx := y*ref.Width + x
			diff := int(got[idx]) - int(ref.Y[idx])
			if diff < 0 {
				diff = -diff
			}
			if diff > threshold {
				return x, y, got[idx], ref.Y[idx], true
			}
		}
	}
	return 0, 0, 0, 0, false
}

func debugRawYBlockRMSE(got, ref []byte, stride, x, y, width, height int) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	sum := 0
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			idx := (y+row)*stride + x + col
			diff := int(got[idx]) - int(ref[idx])
			sum += diff * diff
		}
	}
	return sqrtDebugFloat(float64(sum) / float64(width*height))
}

func sqrtDebugFloat(v float64) float64 {
	// Avoid importing math into this debug file's hot path output helpers.
	z := v
	if z == 0 {
		return 0
	}
	x := z
	for i := 0; i < 8; i++ {
		x = 0.5 * (x + z/x)
	}
	return x
}

func TestDebugSqrtFloatSanity(t *testing.T) {
	if got := fmt.Sprintf("%.2f", sqrtDebugFloat(9)); got != "3.00" {
		t.Fatalf("sqrtDebugFloat(9) = %s", got)
	}
}
