package decoder

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

type referenceThresholds struct {
	YRMSE   float64
	URMSE   float64
	VRMSE   float64
	YMaxAbs int
	UMaxAbs int
	VMaxAbs int
}

func TestDecodeGeneratedMain8FramesMatchReference(t *testing.T) {
	requireReferenceCompare(t)

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "reference-main8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   6,
		BitDepth: 8,
	})

	assertDecoderMatchesReference(t, fixture.Path, 4, referenceThresholds{
		YRMSE:   1.0,
		URMSE:   1.0,
		VRMSE:   1.0,
		YMaxAbs: 6,
		UMaxAbs: 6,
		VMaxAbs: 6,
	})
}

func TestDecodeGeneratedMain10FramesMatchReference(t *testing.T) {
	requireReferenceCompare(t)

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "reference-main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	assertDecoderMatchesReference(t, fixture.Path, 4, referenceThresholds{
		YRMSE:   2.0,
		URMSE:   2.0,
		VRMSE:   2.0,
		YMaxAbs: 24,
		UMaxAbs: 24,
		VMaxAbs: 24,
	})
}

func TestDecodeSPBTVFramesMatchReference(t *testing.T) {
	requireReferenceCompare(t)

	path := testutil.SamplePath(t, "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	testutil.RequireFile(t, path)

	assertDecoderMatchesReference(t, path, 4, referenceThresholds{
		YRMSE:   3.0,
		URMSE:   3.0,
		VRMSE:   3.0,
		YMaxAbs: 24,
		UMaxAbs: 24,
		VMaxAbs: 24,
	})
}

func TestDecodeBigBuckBunnyFramesMatchReference(t *testing.T) {
	requireReferenceCompare(t)

	path := testutil.SamplePath(t, "BigBuckBunny-AV1.mp4")
	if override := os.Getenv("DEBUG_BBB_REFERENCE_PATH"); override != "" {
		path = override
	}
	testutil.RequireFile(t, path)

	assertDecoderMatchesReference(t, path, 120, referenceThresholds{
		YRMSE:   3.0,
		URMSE:   3.0,
		VRMSE:   3.0,
		YMaxAbs: 24,
		UMaxAbs: 24,
		VMaxAbs: 24,
	})
}

func TestDebugBigBuckBunnyFrame0Mismatch(t *testing.T) {
	if os.Getenv("DEBUG_BBB_MISMATCH") == "" {
		t.Skip("debug only")
	}

	path := os.Getenv("DEBUG_BBB_REFERENCE_PATH")
	if path == "" {
		path = testutil.SamplePath(t, "BigBuckBunny-AV1.mp4")
	}
	testutil.RequireFile(t, path)

	frameIndex := 0
	if spec := os.Getenv("DEBUG_BBB_FRAME"); spec != "" {
		v, err := strconv.Atoi(spec)
		if err != nil || v < 0 {
			t.Fatalf("invalid DEBUG_BBB_FRAME=%q", spec)
		}
		frameIndex = v
	}
	refFrames := testutil.DecodeReferenceFrames420(t, path, frameIndex+1)
	if len(refFrames) <= frameIndex {
		t.Fatalf("reference frames=%d, want at least %d", len(refFrames), frameIndex+1)
	}
	ref := refFrames[frameIndex]
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()
	var frame *Frame
	for i := 0; i <= frameIndex; i++ {
		frame, err = dec.NextFrame()
		if err != nil {
			t.Fatalf("NextFrame(%d): %v", i, err)
		}
	}
	metrics, _ := compareFrameToReference(frame, ref)
	t.Logf("frame%d decoded_index=%d metrics Y rmse=%.4f mean=%.4f max=%d U rmse=%.4f max=%d V rmse=%.4f max=%d",
		frameIndex, frame.Index,
		metrics.Y.RMSE, metrics.Y.MeanAbs, metrics.Y.MaxAbs,
		metrics.U.RMSE, metrics.U.MaxAbs,
		metrics.V.RMSE, metrics.V.MaxAbs)
	gotY := flattenPlane8(frame.Y, frame.YStride, ref.Width, ref.Height)
	logPlaneMismatchSummary(t, "Y", gotY, ref.Y, ref.Width, ref.Height, 16, 10)
	chromaWidth := (ref.Width + 1) >> 1
	chromaHeight := (ref.Height + 1) >> 1
	gotU := flattenPlane8(frame.U, frame.UStride, chromaWidth, chromaHeight)
	gotV := flattenPlane8(frame.V, frame.VStride, chromaWidth, chromaHeight)
	logPlaneMismatchSummary(t, "U", gotU, ref.U, chromaWidth, chromaHeight, 8, 6)
	logPlaneMismatchSummary(t, "V", gotV, ref.V, chromaWidth, chromaHeight, 8, 6)
}

func TestDebugBigBuckBunnyHeaderTrace(t *testing.T) {
	if os.Getenv("DEBUG_BBB_HEADER_TRACE") == "" {
		t.Skip("debug only")
	}

	path := os.Getenv("DEBUG_BBB_REFERENCE_PATH")
	if path == "" {
		path = testutil.SamplePath(t, "BigBuckBunny-AV1.mp4")
	}
	testutil.RequireFile(t, path)

	limit := debugBBBEnvInt("DEBUG_BBB_HEADER_TRACE_LIMIT", 12)
	x := debugBBBEnvInt("DEBUG_BBB_TRACE_X", 131)
	y := debugBBBEnvInt("DEBUG_BBB_TRACE_Y", 402)
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	shown := 0
	for i := 0; i < limit; i++ {
		parsed, err := dec.NextParsedFrame()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("NextParsedFrame(%d): %v", i, err)
		}
		frame, err := dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame(%d): %v", i, err)
		}
		target := -1
		if y >= 0 && y < frame.Height && x >= 0 && x < frame.Width && frame.BitDepth <= 8 {
			target = int(frame.Y[y*frame.YStride+x])
		}
		h := parsed.Header
		t.Logf("parsed=%02d sample=%d pts=%s dts=%s type=%d show=%v existing=%v existingIdx=%d showNo=%d target=%d offset=%d refresh=%08b primary=%d refs=%v refPOC=%v purePOC=%v slotTarget=%v",
			parsed.Index, parsed.SampleIndex, parsed.PTS, parsed.DTS, h.FrameType, h.ShowFrame, h.ShowExistingFrame,
			h.ExistingFrameIdx, shown, target, h.FrameOffset, h.RefreshFrameFlags, h.PrimaryRefFrame, h.RefIdx,
			parsed.RefFramePOC, dec.pureGoRefPOC, debugPureGoSlotTargets(dec, x, y))
		if h.ShowFrame || h.ShowExistingFrame {
			shown++
		}
		_ = frame.Close()
	}
}

func debugPureGoSlotTargets(d *Decoder, x, y int) [8]int {
	var out [8]int
	for i := range out {
		out[i] = -1
		ref := d.pureGoRefs[i]
		if ref == nil || ref.BitDepth > 8 || x < 0 || y < 0 || x >= ref.Width || y >= ref.Height {
			continue
		}
		out[i] = int(ref.Y[y*ref.YStride+x])
	}
	return out
}

func debugBBBEnvInt(name string, fallback int) int {
	spec := os.Getenv(name)
	if spec == "" {
		return fallback
	}
	v, err := strconv.Atoi(spec)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func assertDecoderMatchesReference(t *testing.T, path string, limit int, th referenceThresholds) {
	t.Helper()

	refFrames := testutil.DecodeReferenceFrames420(t, path, limit)
	if len(refFrames) == 0 {
		t.Fatal("expected at least one reference frame")
	}

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for i, ref := range refFrames {
		frame, err := dec.NextFrame()
		if err == io.EOF {
			t.Fatalf("NextFrame(%d): unexpected EOF after %d/%d frames", i, i, len(refFrames))
		}
		if err != nil {
			t.Fatalf("NextFrame(%d): %v", i, err)
		}
		metrics, detail := compareFrameToReference(frame, ref)
		assertPlaneMetrics(t, fmt.Sprintf("frame %d Y (%s)", i, detail), metrics.Y, th.YRMSE, th.YMaxAbs)
		assertPlaneMetrics(t, fmt.Sprintf("frame %d U (%s)", i, detail), metrics.U, th.URMSE, th.UMaxAbs)
		assertPlaneMetrics(t, fmt.Sprintf("frame %d V (%s)", i, detail), metrics.V, th.VRMSE, th.VMaxAbs)
	}
}

func compareFrameToReference(frame *Frame, ref testutil.RawFrame420) (testutil.FrameMetrics420, string) {
	if frame == nil {
		panic("compareFrameToReference: nil frame")
	}
	if frame.Width != 0 && frame.Width != ref.Width {
		panic(fmt.Sprintf("compareFrameToReference width mismatch: got=%d want=%d", frame.Width, ref.Width))
	}
	if frame.Height != 0 && frame.Height != ref.Height {
		panic(fmt.Sprintf("compareFrameToReference height mismatch: got=%d want=%d", frame.Height, ref.Height))
	}

	chromaWidth := (ref.Width + 1) >> 1
	chromaHeight := (ref.Height + 1) >> 1
	if ref.BitDepth > 8 {
		gotY := flattenPlane16(frame.Y16, frame.YStride, ref.Width, ref.Height)
		gotU := flattenPlane16(frame.U16, frame.UStride, chromaWidth, chromaHeight)
		gotV := flattenPlane16(frame.V16, frame.VStride, chromaWidth, chromaHeight)
		return testutil.FrameMetrics420{
			Y: testutil.ComparePlane16(gotY, ref.Y16),
			U: testutil.ComparePlane16(gotU, ref.U16),
			V: testutil.ComparePlane16(gotV, ref.V16),
		}, fmt.Sprintf("%dbit", ref.BitDepth)
	}

	gotY := flattenPlane8(frame.Y, frame.YStride, ref.Width, ref.Height)
	gotU := flattenPlane8(frame.U, frame.UStride, chromaWidth, chromaHeight)
	gotV := flattenPlane8(frame.V, frame.VStride, chromaWidth, chromaHeight)
	return testutil.FrameMetrics420{
		Y: testutil.ComparePlane8(gotY, ref.Y),
		U: testutil.ComparePlane8(gotU, ref.U),
		V: testutil.ComparePlane8(gotV, ref.V),
	}, fmt.Sprintf("%dbit", ref.BitDepth)
}

func flattenPlane8(src []byte, stride, width, height int) []byte {
	if width == 0 || height == 0 {
		return nil
	}
	if stride < width {
		panic(fmt.Sprintf("flattenPlane8 stride %d < width %d", stride, width))
	}
	if len(src) < (height-1)*stride+width {
		panic(fmt.Sprintf("flattenPlane8 short buffer len=%d stride=%d width=%d height=%d", len(src), stride, width, height))
	}
	if stride == width {
		return append([]byte(nil), src[:width*height]...)
	}
	out := make([]byte, width*height)
	for y := 0; y < height; y++ {
		copy(out[y*width:(y+1)*width], src[y*stride:y*stride+width])
	}
	return out
}

func flattenPlane16(src []uint16, stride, width, height int) []uint16 {
	if width == 0 || height == 0 {
		return nil
	}
	if stride < width {
		panic(fmt.Sprintf("flattenPlane16 stride %d < width %d", stride, width))
	}
	if len(src) < (height-1)*stride+width {
		panic(fmt.Sprintf("flattenPlane16 short buffer len=%d stride=%d width=%d height=%d", len(src), stride, width, height))
	}
	if stride == width {
		return append([]uint16(nil), src[:width*height]...)
	}
	out := make([]uint16, width*height)
	for y := 0; y < height; y++ {
		copy(out[y*width:(y+1)*width], src[y*stride:y*stride+width])
	}
	return out
}

func assertPlaneMetrics(t *testing.T, label string, got testutil.PlaneMetrics, maxRMSE float64, maxAbs int) {
	t.Helper()
	if got.RMSE > maxRMSE || got.MaxAbs > maxAbs {
		t.Fatalf("%s metrics: rmse=%.4f mean_abs=%.4f max_abs=%d (limits rmse<=%.4f max_abs<=%d)", label, got.RMSE, got.MeanAbs, got.MaxAbs, maxRMSE, maxAbs)
	}
}

type blockMismatchSummary struct {
	x, y    int
	rmse    float64
	meanAbs float64
	maxAbs  int
}

func logPlaneMismatchSummary(t *testing.T, label string, got, want []byte, width, height, blockSize, topN int) {
	t.Helper()
	firstX, firstY := -1, -1
	firstGot, firstWant := 0, 0
	maxX, maxY, maxGot, maxWant, maxAbs := 0, 0, 0, 0, -1
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			diff := int(got[idx]) - int(want[idx])
			if diff < 0 {
				diff = -diff
			}
			if diff != 0 && firstX < 0 {
				firstX, firstY = x, y
				firstGot, firstWant = int(got[idx]), int(want[idx])
			}
			if diff > maxAbs {
				maxX, maxY = x, y
				maxGot, maxWant = int(got[idx]), int(want[idx])
				maxAbs = diff
			}
		}
	}
	t.Logf("%s first=(%d,%d) got=%d want=%d max=(%d,%d) got=%d want=%d abs=%d",
		label, firstX, firstY, firstGot, firstWant, maxX, maxY, maxGot, maxWant, maxAbs)

	blocks := make([]blockMismatchSummary, 0)
	for by := 0; by < height; by += blockSize {
		bh := minInt(blockSize, height-by)
		for bx := 0; bx < width; bx += blockSize {
			bw := minInt(blockSize, width-bx)
			var sumAbs, sumSq float64
			blockMax := 0
			for y := 0; y < bh; y++ {
				for x := 0; x < bw; x++ {
					idx := (by+y)*width + bx + x
					diff := int(got[idx]) - int(want[idx])
					if diff < 0 {
						diff = -diff
					}
					if diff > blockMax {
						blockMax = diff
					}
					sumAbs += float64(diff)
					sumSq += float64(diff * diff)
				}
			}
			n := float64(bw * bh)
			blocks = append(blocks, blockMismatchSummary{
				x:       bx,
				y:       by,
				rmse:    sqrtFloat(sumSq / n),
				meanAbs: sumAbs / n,
				maxAbs:  blockMax,
			})
		}
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].rmse == blocks[j].rmse {
			return blocks[i].maxAbs > blocks[j].maxAbs
		}
		return blocks[i].rmse > blocks[j].rmse
	})
	if topN > len(blocks) {
		topN = len(blocks)
	}
	for i := 0; i < topN; i++ {
		b := blocks[i]
		t.Logf("%s block[%d] x=%d y=%d rmse=%.2f mean=%.2f max=%d", label, i, b.x, b.y, b.rmse, b.meanAbs, b.maxAbs)
	}
}

func sqrtFloat(v float64) float64 {
	x := v
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 16; i++ {
		z = 0.5 * (z + x/z)
	}
	return z
}

func requireReferenceCompare(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_REFERENCE_COMPARE") == "" {
		t.Skip("integration test: set RUN_REFERENCE_COMPARE=1 to compare decoder output against ffmpeg reference frames")
	}
}
