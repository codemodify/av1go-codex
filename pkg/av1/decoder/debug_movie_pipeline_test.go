package decoder

import (
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugMoviePipelineComparison(t *testing.T) {
	if os.Getenv("DEBUG_MOVIE_PIPELINE") == "" {
		t.Skip("debug only")
	}

	ref, err := os.ReadFile("/tmp/movie_ref.yuv")
	if err != nil {
		t.Skipf("reference frame unavailable: %v", err)
	}

	path := filepath.Join("..", "..", "..", "testvideo", "AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	ySize := parsed.Header.Width * parsed.Header.Height
	uvW := (parsed.Header.Width + 1) >> 1
	uvH := (parsed.Header.Height + 1) >> 1
	uvSize := uvW * uvH
	if len(ref) < ySize+2*uvSize {
		t.Fatalf("reference frame too small: len=%d want>=%d", len(ref), ySize+2*uvSize)
	}
	refY := ref[:ySize]
	refU := ref[ySize : ySize+uvSize]
	refV := ref[ySize+uvSize : ySize+2*uvSize]
	t.Logf("quant yac=%d ydc=%d udc=%d uac=%d vdc=%d vac=%d segEnabled=%v qidx0=%d lossless0=%v allLossless=%v reducedTxtp=%v",
		parsed.Header.Quantization.YAC,
		parsed.Header.Quantization.YDCDelta,
		parsed.Header.Quantization.UDCDelta,
		parsed.Header.Quantization.UACDelta,
		parsed.Header.Quantization.VDCDelta,
		parsed.Header.Quantization.VACDelta,
		parsed.Header.Segmentation.Enabled,
		parsed.Header.Segmentation.QIndex[0],
		parsed.Header.Segmentation.Lossless[0],
		parsed.Header.AllLossless,
		parsed.Header.ReducedTxtpSet,
	)
	if os.Getenv("DEBUG_MOVIE_DISABLE_CDF_UPDATE") != "" {
		parsed.Header.DisableCDFUpdate = true
		t.Logf("forcing DisableCDFUpdate for debug run")
	}

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	cdefBeforeSyntax := os.Getenv("DEBUG_MOVIE_CDEF_AFTER") == ""
	if err := decodeKeyframeMain8DCLumaWithCDEFOrder(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, restoration, segState, cdefBeforeSyntax); err != nil {
		t.Fatal(err)
	}

	logMoviePipelineMetrics(t, "raw", img, refY, refU, refV)
	logMovieEnvPatch(t, "raw", img.Y, img.YStride, refY, parsed.Header.Width)
	logMovieTargetWHTVariants(t, refY, img.YStride)
	applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
	logMoviePipelineMetrics(t, "loop", img, refY, refU, refV)
	applyCDEF420_8(img, &parsed.Header, cdef)
	logMoviePipelineMetrics(t, "cdef", img, refY, refU, refV)
	applyRestoration420_8(img, &parsed.Header, restoration)
	logMoviePipelineMetrics(t, "restoration", img, refY, refU, refV)
}

func logMoviePipelineMetrics(t *testing.T, label string, img *image.YCbCr, refY, refU, refV []byte) {
	t.Helper()
	t.Logf("%s Y rmse=%.4f U rmse=%.4f V rmse=%.4f", label, debugPlaneRMSE(img.Y, refY), debugPlaneRMSE(img.Cb, refU), debugPlaneRMSE(img.Cr, refV))
	logMovieMismatchSummary(t, label, img.Y, img.YStride, refY, img.Rect.Dx(), img.Rect.Dy())
	logMoviePatch(t, label, "ours", img.Y, img.YStride, 828, 28, 12, 8)
	logMoviePatch(t, label, "ref", refY, img.YStride, 828, 28, 12, 8)
}

func logMovieMismatchSummary(t *testing.T, label string, plane []byte, stride int, ref []byte, width, height int) {
	t.Helper()
	if width <= 0 || height <= 0 {
		return
	}
	firstX, firstY := -1, -1
	firstGot, firstWant := 0, 0
	maxX, maxY := 0, 0
	maxGot, maxWant := 0, 0
	maxDiff := -1
	for y := 0; y < height; y++ {
		rowBase := y * stride
		refBase := y * width
		for x := 0; x < width; x++ {
			got := int(plane[rowBase+x])
			want := int(ref[refBase+x])
			diff := got - want
			if diff < 0 {
				diff = -diff
			}
			if diff != 0 && firstX < 0 {
				firstX, firstY = x, y
				firstGot, firstWant = got, want
			}
			if diff > maxDiff {
				maxDiff = diff
				maxX, maxY = x, y
				maxGot, maxWant = got, want
			}
		}
	}
	if firstX < 0 {
		t.Logf("%s mismatch first=none max=0", label)
		return
	}
	t.Logf("%s mismatch first=(%d,%d) tu=(%d,%d) got=%d want=%d max=(%d,%d) tu=(%d,%d) got=%d want=%d abs=%d",
		label,
		firstX, firstY, firstX&^3, firstY&^3, firstGot, firstWant,
		maxX, maxY, maxX&^3, maxY&^3, maxGot, maxWant, maxDiff,
	)
	logMovieBlockPatch(t, label, "first-ours", plane, stride, firstX&^3, firstY&^3)
	logMovieBlockPatch(t, label, "first-ref", ref, width, firstX&^3, firstY&^3)
	if maxX != firstX || maxY != firstY {
		logMovieBlockPatch(t, label, "max-ours", plane, stride, maxX&^3, maxY&^3)
		logMovieBlockPatch(t, label, "max-ref", ref, width, maxX&^3, maxY&^3)
	}
}

func logMovieBlockPatch(t *testing.T, label, which string, plane []byte, stride, x, y int) {
	t.Helper()
	if stride <= 0 || len(plane) == 0 || x < 0 || y < 0 {
		return
	}
	height := len(plane) / stride
	if height <= 0 || y >= height || x >= stride {
		return
	}
	w := minInt(4, stride-x)
	h := minInt(4, height-y)
	for py := 0; py < h; py++ {
		start := (y+py)*stride + x
		row := plane[start : start+w]
		t.Logf("%s %s block y=%d x=%d: %v", label, which, y+py, x, row)
	}
}

func TestLogMovieBlockPatchClampsAtFrameEdge(t *testing.T) {
	plane := make([]byte, 3*5)
	logMovieBlockPatch(t, "debug", "edge", plane, 5, 3, 1)
}

func logMoviePatch(t *testing.T, label, which string, plane []byte, stride, x, y, w, h int) {
	t.Helper()
	for py := 0; py < h; py++ {
		row := plane[(y+py)*stride+x : (y+py)*stride+x+w]
		t.Logf("%s %s patch y=%d: %v", label, which, y+py, row)
	}
}

func logMovieEnvPatch(t *testing.T, label string, plane []byte, stride int, ref []byte, refStride int) {
	t.Helper()
	spec := strings.TrimSpace(os.Getenv("DEBUG_MOVIE_PATCH"))
	if spec == "" {
		return
	}
	parts := strings.Split(spec, ",")
	if len(parts) != 4 {
		t.Logf("invalid DEBUG_MOVIE_PATCH=%q", spec)
		return
	}
	x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	w, errW := strconv.Atoi(strings.TrimSpace(parts[2]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[3]))
	if errX != nil || errY != nil || errW != nil || errH != nil || w <= 0 || h <= 0 {
		t.Logf("invalid DEBUG_MOVIE_PATCH=%q", spec)
		return
	}
	logMoviePatch(t, label, "env-ours", plane, stride, x, y, w, h)
	logMoviePatch(t, label, "env-ref", ref, refStride, x, y, w, h)
}

func logMovieTargetWHTVariants(t *testing.T, refY []byte, stride int) {
	t.Helper()
	logMovieBlockWHTVariants(t, "target836", refY, stride, 836, 32, []int32{4, 100, 24, 12, -16, -4, 0, -8, 24, -12, 0, 4, 0, 8, 0, 0})
	logMovieBlockWHTVariants(t, "target900", refY, stride, 900, 0, []int32{0, 76, 4, 0, 20, 60, -16, 0, -4, -28, 0, 4, -4, 0, 12, 0})
}

func logMovieBlockWHTVariants(t *testing.T, label string, refY []byte, stride, x, y int, coeffs []int32) {
	t.Helper()
	ref := make([]byte, 16)
	for py := 0; py < 4; py++ {
		copy(ref[py*4:py*4+4], refY[(y+py)*stride+x:(y+py)*stride+x+4])
	}
	for _, transpose := range []bool{false, true} {
		for shift := 0; shift <= 4; shift++ {
			block := simulateMovieTargetWHT(coeffs, transpose, shift)
			mse := 0.0
			for i := range block {
				d := int(block[i]) - int(ref[i])
				mse += float64(d * d)
			}
			mse /= 16
			t.Logf("%s wht transpose=%v shift=%d mse=%.2f block=%v", label, transpose, shift, mse, [][]byte{
				append([]byte(nil), block[0:4]...),
				append([]byte(nil), block[4:8]...),
				append([]byte(nil), block[8:12]...),
				append([]byte(nil), block[12:16]...),
			})
		}
	}
}

func simulateMovieTargetWHT(coeffs []int32, transpose bool, shift int) []byte {
	tmp := make([]int32, 16)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src := y*4 + x
			if transpose {
				src = y + x*4
			}
			tmp[y*4+x] = coeffs[src] >> shift
		}
		invWHT4_1D(tmp[y*4:y*4+4], 1)
	}
	for x := 0; x < 4; x++ {
		invWHT4_1D(tmp[x:], 4)
	}
	out := make([]byte, 16)
	for i := range out {
		out[i] = clipByte(16 + int(tmp[i]))
	}
	return out
}
