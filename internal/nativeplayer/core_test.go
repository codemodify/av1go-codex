package nativeplayer

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codemodify/av1go-codex/internal/displaycolor"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestListVideoFiles(t *testing.T) {
	dir := t.TempDir()
	testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{Name: "sample.mp4"})

	files, err := ListVideoFiles(dir)
	if err != nil {
		t.Fatalf("ListVideoFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 mp4 file, got %d", len(files))
	}
}

func TestListVideoFilesSkipsBrokenSymlinks(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.mp4")
	if err := os.Symlink(filepath.Join(dir, "missing.mp4"), broken); err != nil {
		t.Skipf("Symlink: %v", err)
	}
	testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{Name: "sample.mp4"})

	files, err := ListVideoFiles(dir)
	if err != nil {
		t.Fatalf("ListVideoFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "sample.mp4" {
		t.Fatalf("files = %v, want [sample.mp4]", files)
	}
}

func TestInspectVideo(t *testing.T) {
	dir := t.TempDir()
	fixture := testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{
		Name:     "sample.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	info, err := InspectVideo(filepath.Join(dir, "sample.mp4"))
	if err != nil {
		t.Fatalf("InspectVideo: %v", err)
	}
	if info.Width != fixture.Width || info.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", info.Width, info.Height, fixture.Width, fixture.Height)
	}
	if info.Codec == "" {
		t.Fatal("expected codec string")
	}
	if info.SampleCount != fixture.Frames {
		t.Fatalf("sample count = %d, want %d", info.SampleCount, fixture.Frames)
	}
	if info.Decoder == "" {
		t.Fatal("expected decoder description")
	}
}

func TestPlayerStateStopWithoutSession(t *testing.T) {
	var state PlayerState
	if err := state.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestParsedFrameToPlaybackFrame(t *testing.T) {
	parsed := &decoder.ParsedFrame{
		Index:    3,
		PTS:      40,
		Duration: 20,
		Header: obu.FrameHeader{
			FrameType: obu.FrameTypeInter,
			ShowFrame: true,
			Width:     160,
			Height:    90,
			Tiling: obu.Tiling{
				Cols: 2,
				Rows: 2,
			},
		},
	}

	frame := ParsedFrameToPlaybackFrame(decoder.Metadata{
		Width:    160,
		Height:   90,
		BitDepth: 8,
		Chroma:   av1.Chroma420,
	}, parsed)
	if frame == nil || frame.Image == nil {
		t.Fatal("expected playback frame image")
	}
	if got, want := frame.Image.Bounds(), image.Rect(0, 0, 160, 90); got != want {
		t.Fatalf("image bounds = %v, want %v", got, want)
	}
	if frame.Layout != av1.Chroma420 {
		t.Fatalf("layout = %v, want %v", frame.Layout, av1.Chroma420)
	}
}

func TestResolvePlaybackTargetExplicitFile(t *testing.T) {
	dir := t.TempDir()
	fixture := testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{Name: "sample.mp4"})

	target, err := ResolvePlaybackTarget(fixture.Path, nil)
	if err != nil {
		t.Fatalf("ResolvePlaybackTarget(file): %v", err)
	}
	if target.Path != fixture.Path {
		t.Fatalf("target path = %s, want %s", target.Path, fixture.Path)
	}
	if target.Generated {
		t.Fatal("explicit file should not be marked generated")
	}
}

func TestResolvePlaybackTargetSearchesByFilenameInSearchDirs(t *testing.T) {
	videoDir := t.TempDir()
	fixture := testutil.CreateAV1MP4Fixture(t, videoDir, testutil.AV1FixtureOptions{
		Name: "spbtv_sample_bipbop_av1_960x540_25fps.mp4",
	})

	target, err := ResolvePlaybackTarget(filepath.Base(fixture.Path), []string{videoDir})
	if err != nil {
		t.Fatalf("ResolvePlaybackTarget(search by filename): %v", err)
	}
	if target.Path != fixture.Path {
		t.Fatalf("target path = %s, want %s", target.Path, fixture.Path)
	}
	if !strings.Contains(target.Source, videoDir) {
		t.Fatalf("target source = %q, want search dir %s", target.Source, videoDir)
	}
}

func TestResolvePlaybackTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{Name: "b.mp4"})
	testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{Name: "a.mp4"})

	target, err := ResolvePlaybackTarget(dir, nil)
	if err != nil {
		t.Fatalf("ResolvePlaybackTarget(dir): %v", err)
	}
	if got, want := filepath.Base(target.Path), "a.mp4"; got != want {
		t.Fatalf("selected file = %s, want %s", got, want)
	}
}

func TestResolvePlaybackTargetDirectoryPrefersFeaturedMovie(t *testing.T) {
	dir := t.TempDir()
	testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{Name: "z-last.mp4"})
	testutil.CreateAV1MP4Fixture(t, dir, testutil.AV1FixtureOptions{
		Name: "AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4",
	})

	target, err := ResolvePlaybackTarget(dir, nil)
	if err != nil {
		t.Fatalf("ResolvePlaybackTarget(dir featured): %v", err)
	}
	if got, want := filepath.Base(target.Path), "AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4"; got != want {
		t.Fatalf("selected file = %s, want %s", got, want)
	}
}

func TestResolvePlaybackTargetGeneratedDemo(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(prev)

	target, err := ResolvePlaybackTarget("", []string{"testvideo", "."})
	if err != nil {
		t.Fatalf("ResolvePlaybackTarget(demo): %v", err)
	}
	defer os.RemoveAll(filepath.Dir(target.Path))

	if !target.Generated {
		t.Fatal("expected generated demo target")
	}
	if !strings.HasSuffix(target.Path, ".mp4") {
		t.Fatalf("generated path = %s, want .mp4 suffix", target.Path)
	}
	info, err := InspectVideo(target.Path)
	if err != nil {
		t.Fatalf("InspectVideo(generated): %v", err)
	}
	if info.SampleCount != DemoFrameCount {
		t.Fatalf("sample count = %d, want %d", info.SampleCount, DemoFrameCount)
	}
}

func TestResolvePlaybackTargetFallsBackToDemoWhenSearchDirsContainOnlyUnavailableSamples(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "missing.mp4"), filepath.Join(dir, "broken.mp4")); err != nil {
		t.Skipf("Symlink: %v", err)
	}

	target, err := ResolvePlaybackTarget("", []string{dir})
	if err != nil {
		t.Fatalf("ResolvePlaybackTarget(fallback demo): %v", err)
	}
	defer os.RemoveAll(filepath.Dir(target.Path))

	if !target.Generated {
		t.Fatalf("target = %+v, want generated demo clip", target)
	}
}

func TestBufferedPlaybackSpan(t *testing.T) {
	items := []playbackItem{
		{frame: &decoder.Frame{PTS: 0, Duration: 40 * time.Millisecond}},
		{frame: &decoder.Frame{PTS: 40 * time.Millisecond, Duration: 40 * time.Millisecond}},
		{frame: &decoder.Frame{PTS: 80 * time.Millisecond, Duration: 40 * time.Millisecond}},
	}
	if got, want := bufferedPlaybackSpan(items), 120*time.Millisecond; got != want {
		t.Fatalf("bufferedPlaybackSpan = %s, want %s", got, want)
	}
}

func TestPrepareFrameForDisplayConvertsPlanar420ToRGBA(t *testing.T) {
	src := &decoder.Frame{
		Width:    2,
		Height:   2,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Color: av1.ColorConfig{
			BitDepth:           8,
			SubsamplingX:       true,
			SubsamplingY:       true,
			MatrixCoefficients: 1,
		},
		Y:       []byte{16, 235, 81, 145},
		U:       []byte{128},
		V:       []byte{128},
		YStride: 2,
		UStride: 1,
		VStride: 1,
	}

	prepared := prepareFrameForDisplay(src)
	if prepared == nil {
		t.Fatal("expected prepared frame")
	}
	rgba, ok := prepared.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("prepared image type = %T, want *image.RGBA", prepared.Image)
	}
	if got, want := rgba.Bounds(), image.Rect(0, 0, 2, 2); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
	if len(prepared.Y) != 0 || len(prepared.U) != 0 || len(prepared.V) != 0 {
		t.Fatal("expected prepared frame planes to be released")
	}
	if len(src.Y) == 0 || len(src.U) == 0 || len(src.V) == 0 {
		t.Fatal("expected source frame planes to remain intact")
	}
	if got := rgba.RGBAAt(1, 0); got.A != 0xff {
		t.Fatalf("pixel alpha = %d, want 255", got.A)
	}
}

func TestPrepareFrameForDisplayConvertsPlanar420HighBitDepthToRGBA(t *testing.T) {
	src := &decoder.Frame{
		Width:    4,
		Height:   2,
		BitDepth: 10,
		Layout:   av1.Chroma420,
		Color: av1.ColorConfig{
			BitDepth:           10,
			SubsamplingX:       true,
			SubsamplingY:       true,
			MatrixCoefficients: 1,
		},
		Y16:     []uint16{64, 256, 512, 940, 128, 384, 640, 800},
		U16:     []uint16{512, 384},
		V16:     []uint16{512, 768},
		YStride: 4,
		UStride: 2,
		VStride: 2,
	}

	prepared := prepareFrameForDisplay(src)
	if prepared == nil {
		t.Fatal("expected prepared frame")
	}
	t.Cleanup(func() { _ = prepared.Close() })
	rgba, ok := prepared.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("prepared image type = %T, want *image.RGBA", prepared.Image)
	}
	if got, want := rgba.Bounds(), image.Rect(0, 0, 4, 2); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
	if len(prepared.Y16) != 0 || len(prepared.U16) != 0 || len(prepared.V16) != 0 {
		t.Fatal("expected prepared frame high-bit-depth planes to be released")
	}
	if len(src.Y16) == 0 || len(src.U16) == 0 || len(src.V16) == 0 {
		t.Fatal("expected source frame planes to remain intact")
	}
	if got := rgba.RGBAAt(3, 0); got.A != 0xff {
		t.Fatalf("pixel alpha = %d, want 255", got.A)
	}
	if got, want := rgba.RGBAAt(0, 0), rgba.RGBAAt(3, 0); got == want {
		t.Fatalf("expected high-bit-depth conversion variation, pixel(0,0) = %v, pixel(3,0) = %v", got, want)
	}
}

func TestPrepareFrameForDisplayUsesDecodedImageAsSource(t *testing.T) {
	img := image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Y[img.YOffset(x, y)] = uint8(16 + y*40 + x*10)
		}
	}
	for i := range img.Cb {
		img.Cb[i] = uint8(90 + i*20)
		img.Cr[i] = uint8(160 - i*15)
	}

	src := &decoder.Frame{
		Width:    4,
		Height:   4,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Color: av1.ColorConfig{
			BitDepth:           8,
			SubsamplingX:       true,
			SubsamplingY:       true,
			MatrixCoefficients: 1,
		},
		Image:   img,
		Y:       img.Y,
		U:       img.Cb,
		V:       img.Cr,
		YStride: img.YStride,
		UStride: img.CStride,
		VStride: img.CStride,
	}

	prepared := prepareFrameForDisplay(src)
	got, ok := prepared.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("prepared image type = %T, want *image.RGBA", prepared.Image)
	}

	want := image.NewRGBA(image.Rect(0, 0, 4, 4))
	copyYCbCrToRGBA(want, img, src.Color)

	if string(got.Pix) != string(want.Pix) {
		t.Fatal("prepared image pixels do not match decoded image copy")
	}
}

func TestDebugSPBTVFirstFramePlaneAndDisplayMetrics(t *testing.T) {
	if os.Getenv("DEBUG_SPBTV_FRAME_METRICS") == "" {
		t.Skip("debug only")
	}

	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		t.Skipf("reference frame unavailable: %v", err)
	}

	path := filepath.Join("..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := decoder.OpenMP4(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	frame, err := dec.NextFrame()
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Close()

	width, height := frame.Width, frame.Height
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	ySize := width * height
	uvSize := chromaWidth * chromaHeight
	if len(ref) < ySize+2*uvSize {
		t.Fatalf("reference frame too small: len=%d want>=%d", len(ref), ySize+2*uvSize)
	}
	refY := ref[:ySize]
	refU := ref[ySize : ySize+uvSize]
	refV := ref[ySize+uvSize : ySize+2*uvSize]

	gotY := flattenDebugPlane8(frame.Y, frame.YStride, width, height)
	gotU := flattenDebugPlane8(frame.U, frame.UStride, chromaWidth, chromaHeight)
	gotV := flattenDebugPlane8(frame.V, frame.VStride, chromaWidth, chromaHeight)
	yMetrics := testutil.ComparePlane8(gotY, refY)
	uMetrics := testutil.ComparePlane8(gotU, refU)
	vMetrics := testutil.ComparePlane8(gotV, refV)
	t.Logf("raw planes Y rmse=%.4f mean_abs=%.4f max_abs=%d", yMetrics.RMSE, yMetrics.MeanAbs, yMetrics.MaxAbs)
	t.Logf("raw planes U rmse=%.4f mean_abs=%.4f max_abs=%d", uMetrics.RMSE, uMetrics.MeanAbs, uMetrics.MaxAbs)
	t.Logf("raw planes V rmse=%.4f mean_abs=%.4f max_abs=%d", vMetrics.RMSE, vMetrics.MeanAbs, vMetrics.MaxAbs)
	logFirstDebugPlaneMismatch(t, "Y", gotY, refY, width, height)
	logFirstDebugPlaneMismatch(t, "U", gotU, refU, chromaWidth, chromaHeight)
	logFirstDebugPlaneMismatch(t, "V", gotV, refV, chromaWidth, chromaHeight)
	logFirstDebugPlaneMismatchOver(t, "Y", gotY, refY, width, height, 24)
	logFirstDebugPlaneMismatchOver(t, "U", gotU, refU, chromaWidth, chromaHeight, 24)
	logFirstDebugPlaneMismatchOver(t, "V", gotV, refV, chromaWidth, chromaHeight, 24)
	logWorstDebugPlaneBlock(t, "Y", gotY, refY, width, height, 16)
	logWorstDebugPlaneBlock(t, "U", gotU, refU, chromaWidth, chromaHeight, 8)
	logWorstDebugPlaneBlock(t, "V", gotV, refV, chromaWidth, chromaHeight, 8)

	prepared := prepareFrameForDisplay(frame)
	if prepared == nil {
		t.Fatal("prepareFrameForDisplay returned nil")
	}
	defer prepared.Close()
	rgba, ok := prepared.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("prepared image type = %T, want *image.RGBA", prepared.Image)
	}

	colorCfg := normalizeDisplayColorConfig(frame.Color, frame.Layout, frame.BitDepth)
	converter := newDisplayYUVConverter(colorCfg, width, height)
	points := [][2]int{
		{0, 0},
		{32, 0},
		{64, 0},
		{80, 0},
		{95, 0},
		{96, 0},
		{112, 0},
		{128, 0},
		{384, 0},
		{400, 0},
		{448, 0},
		{464, 0},
		{480, 0},
		{496, 32},
		{512, 0},
		{640, 0},
	}
	for _, pt := range points {
		x, y := pt[0], pt[1]
		cx, cy := x>>1, y>>1
		gotRGBA := rgba.RGBAAt(x, y)
		gotExpected := debugRGBFromYUV(converter, int(gotY[y*width+x]), int(gotU[cy*chromaWidth+cx]), int(gotV[cy*chromaWidth+cx]))
		refExpected := debugRGBFromYUV(converter, int(refY[y*width+x]), int(refU[cy*chromaWidth+cx]), int(refV[cy*chromaWidth+cx]))
		t.Logf("point (%d,%d) got_yuv=(%d,%d,%d) ref_yuv=(%d,%d,%d) got_rgba=(%d,%d,%d) got_expected=(%d,%d,%d) ref_expected=(%d,%d,%d)",
			x, y,
			gotY[y*width+x], gotU[cy*chromaWidth+cx], gotV[cy*chromaWidth+cx],
			refY[y*width+x], refU[cy*chromaWidth+cx], refV[cy*chromaWidth+cx],
			gotRGBA.R, gotRGBA.G, gotRGBA.B,
			gotExpected.R, gotExpected.G, gotExpected.B,
			refExpected.R, refExpected.G, refExpected.B,
		)
	}

	blockSize := 16
	type blockScore struct {
		x, y int
		rmse float64
	}
	worst := blockScore{rmse: -1}
	for by := 0; by < height; by += blockSize {
		for bx := 0; bx < width; bx += blockSize {
			bw := minDebugInt(blockSize, width-bx)
			bh := minDebugInt(blockSize, height-by)
			rmse := debugRGBBlockRMSE(rgba, refY, refU, refV, chromaWidth, converter, bx, by, bw, bh)
			if rmse > worst.rmse {
				worst = blockScore{x: bx, y: by, rmse: rmse}
			}
		}
	}
	t.Logf("worst display-vs-refYUV block (%d,%d)-(%d,%d) rmse=%.2f",
		worst.x, worst.y, worst.x+blockSize, worst.y+blockSize, worst.rmse)
}

func flattenDebugPlane8(src []byte, stride, width, height int) []byte {
	if width == 0 || height == 0 {
		return nil
	}
	if stride < width {
		panic("flattenDebugPlane8: stride smaller than width")
	}
	if len(src) < (height-1)*stride+width {
		panic("flattenDebugPlane8: short source plane")
	}
	out := make([]byte, width*height)
	for y := 0; y < height; y++ {
		copy(out[y*width:(y+1)*width], src[y*stride:y*stride+width])
	}
	return out
}

func debugRGBFromYUV(converter displayYUVConverter, y, u, v int) color.RGBA {
	r, g, b := converter.toRGB(y, u, v)
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}

func debugRGBBlockRMSE(rgba *image.RGBA, refY, refU, refV []byte, chromaWidth int, converter displayYUVConverter, x, y, width, height int) float64 {
	bounds := rgba.Bounds()
	sumSq := 0.0
	samples := 0
	for row := 0; row < height; row++ {
		py := y + row
		for col := 0; col < width; col++ {
			px := x + col
			got := rgba.RGBAAt(px+bounds.Min.X, py+bounds.Min.Y)
			ref := debugRGBFromYUV(converter, int(refY[py*bounds.Dx()+px]), int(refU[(py>>1)*chromaWidth+(px>>1)]), int(refV[(py>>1)*chromaWidth+(px>>1)]))
			dr := int(got.R) - int(ref.R)
			dg := int(got.G) - int(ref.G)
			db := int(got.B) - int(ref.B)
			sumSq += float64(dr*dr + dg*dg + db*db)
			samples += 3
		}
	}
	if samples == 0 {
		return 0
	}
	return math.Sqrt(sumSq / float64(samples))
}

func minDebugInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func logWorstDebugPlaneBlock(t *testing.T, label string, got, ref []byte, width, height, blockSize int) {
	t.Helper()
	worstX, worstY := 0, 0
	worstRMSE := -1.0
	for y := 0; y < height; y += blockSize {
		for x := 0; x < width; x += blockSize {
			bw := minDebugInt(blockSize, width-x)
			bh := minDebugInt(blockSize, height-y)
			rmse := debugPlaneBlockRMSE(got, ref, width, x, y, bw, bh)
			if rmse > worstRMSE {
				worstRMSE = rmse
				worstX = x
				worstY = y
			}
		}
	}
	t.Logf("worst %s block (%d,%d)-(%d,%d) rmse=%.2f",
		label, worstX, worstY, worstX+blockSize, worstY+blockSize, worstRMSE)
}

func logFirstDebugPlaneMismatch(t *testing.T, label string, got, ref []byte, width, height int) {
	t.Helper()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if got[idx] != ref[idx] {
				t.Logf("first %s mismatch (%d,%d) got=%d ref=%d", label, x, y, got[idx], ref[idx])
				return
			}
		}
	}
	t.Logf("first %s mismatch none", label)
}

func logFirstDebugPlaneMismatchOver(t *testing.T, label string, got, ref []byte, width, height, threshold int) {
	t.Helper()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			diff := int(got[idx]) - int(ref[idx])
			if diff < 0 {
				diff = -diff
			}
			if diff > threshold {
				t.Logf("first %s mismatch abs>%d (%d,%d) got=%d ref=%d", label, threshold, x, y, got[idx], ref[idx])
				return
			}
		}
	}
	t.Logf("first %s mismatch abs>%d none", label, threshold)
}

func debugPlaneBlockRMSE(got, ref []byte, stride, x, y, width, height int) float64 {
	sumSq := 0.0
	n := 0
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			idx := (y+row)*stride + x + col
			diff := int(got[idx]) - int(ref[idx])
			sumSq += float64(diff * diff)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sumSq / float64(n))
}

func TestNewDisplayYUVConverterUsesLUTOnlyForSimpleSDR(t *testing.T) {
	sdr := av1.ColorConfig{
		BitDepth:           8,
		SubsamplingX:       true,
		SubsamplingY:       true,
		MatrixCoefficients: 1,
	}
	if converter := newDisplayYUVConverter(sdr, 1280, 720); converter.lut == nil {
		t.Fatal("expected simple SDR converter to use LUT")
	}

	hdr := sdr
	hdr.MatrixCoefficients = 9
	hdr.ColorPrimaries = 9
	hdr.TransferCharacteristics = 16
	converter := newDisplayYUVConverter(hdr, 1280, 720)
	if converter.lut != nil {
		t.Fatal("expected BT.2020 PQ converter to bypass LUT")
	}
	gotR, gotG, gotB := converter.toRGB(196, 88, 220)
	wantR, wantG, wantB := displaycolor.YUVToDisplayRGB(hdr, 196, 88, 220)
	if gotR != wantR || gotG != wantG || gotB != wantB {
		t.Fatalf("converter RGB = (%d,%d,%d), want (%d,%d,%d)", gotR, gotG, gotB, wantR, wantG, wantB)
	}
}

func TestPreviewMatrixDefaultsUnknownTo709(t *testing.T) {
	if got := previewMatrix(av1.ColorConfig{}, 960, 540); got != 709 {
		t.Fatalf("previewMatrix(unknown, 960x540) = %d, want 709", got)
	}
}

func TestLateDropThresholdClampsReasonably(t *testing.T) {
	if got, want := lateDropThreshold(0), minLateDropThreshold; got != want {
		t.Fatalf("lateDropThreshold(0) = %s, want %s", got, want)
	}
	if got, want := lateDropThreshold(10*time.Millisecond), minLateDropThreshold; got != want {
		t.Fatalf("lateDropThreshold(10ms) = %s, want %s", got, want)
	}
	if got, want := lateDropThreshold(500*time.Millisecond), maxLateDropThreshold; got != want {
		t.Fatalf("lateDropThreshold(500ms) = %s, want %s", got, want)
	}
}

func TestShouldDropLateFrame(t *testing.T) {
	if shouldDropLateFrame(-20*time.Millisecond, 40*time.Millisecond, 2) {
		t.Fatal("expected modest lateness to keep frame")
	}
	if !shouldDropLateFrame(-80*time.Millisecond, 40*time.Millisecond, 2) {
		t.Fatal("expected sufficiently late frame to drop")
	}
	if shouldDropLateFrame(-80*time.Millisecond, 40*time.Millisecond, 0) {
		t.Fatal("expected last buffered frame to be preserved")
	}
}

func TestDecodePlaybackItemsMatchesPureGoFirstFrame(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:   "purego-match.mp4",
		Width:  318,
		Height: 178,
		FPS:    7,
		Frames: 3,
	})
	path := fixture.Path

	expectedDecoder, err := decoder.OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4(expected): %v", err)
	}
	defer expectedDecoder.Close()

	wantFrame, err := expectedDecoder.NextFrame()
	if err != nil {
		t.Fatalf("NextFrame(expected): %v", err)
	}
	want, ok := prepareFrameForDisplay(wantFrame).Image.(*image.RGBA)
	if !ok {
		t.Fatalf("expected frame image type = %T, want *image.RGBA", wantFrame.Image)
	}

	playbackDecoder, err := decoder.OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4(playback): %v", err)
	}
	defer playbackDecoder.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	items := make(chan playbackItem, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(items)
		decodePlaybackItems(ctx, playbackDecoder, items)
	}()

	item, ok := <-items
	if !ok {
		t.Fatal("expected first playback item")
	}
	if item.err != nil {
		t.Fatalf("first playback item error: %v", item.err)
	}
	if item.eof || item.frame == nil {
		t.Fatal("expected first playback frame")
	}

	got, ok := item.frame.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("first playback image type = %T, want *image.RGBA", item.frame.Image)
	}
	if got.Bounds() != want.Bounds() {
		t.Fatalf("first playback bounds = %v, want %v", got.Bounds(), want.Bounds())
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("first playback frame pixels do not match pure-Go decoder output")
	}

	cancel()
	<-done
}

func TestPlayerStatePlayFeaturedSampleHeadless(t *testing.T) {
	const sampleName = "AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4"
	testPlayerStatePlayHeadless(t, testutil.SamplePath(t, sampleName), "featured sample")
}

func TestPlayerStatePlaySPBTVHeadless(t *testing.T) {
	const sampleName = "spbtv_sample_bipbop_av1_960x540_25fps.mp4"
	testPlayerStatePlayHeadless(t, testutil.SamplePath(t, sampleName), "spbtv sample")
}

func testPlayerStatePlayHeadless(t *testing.T, path string, label string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("%s unavailable: %v", label, err)
	}

	var (
		mu         sync.Mutex
		frameCount int
		lastFrame  *decoder.Frame
		lastStatus string
		playErr    error
	)
	frameReady := make(chan struct{}, 1)
	statusReady := make(chan struct{}, 1)

	state := &PlayerState{}
	if err := state.Play(path, func(frame *decoder.Frame, presented int) {
		mu.Lock()
		defer mu.Unlock()
		if frame == nil || frame.Image == nil {
			if playErr == nil {
				playErr = errors.New("received nil playback frame")
			}
		} else {
			frameCount = presented
			lastFrame = frame
		}
		select {
		case frameReady <- struct{}{}:
		default:
		}
	}, func(status string) {
		mu.Lock()
		defer mu.Unlock()
		lastStatus = status
		if strings.HasPrefix(status, "Decode error:") && playErr == nil {
			playErr = errors.New(status)
		}
		select {
		case statusReady <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("Play(%s): %v", label, err)
	}
	defer state.Stop()

	deadline := time.After(15 * time.Second)
	for {
		mu.Lock()
		err := playErr
		presented := frameCount
		frame := lastFrame
		status := lastStatus
		mu.Unlock()

		if err != nil {
			t.Fatalf("playback error: %v", err)
		}
		if presented >= 3 {
			if frame == nil || frame.Image == nil {
				t.Fatal("expected decoded playback frame")
			}
			if status == "" {
				t.Fatal("expected playback status")
			}
			if err := state.Stop(); err != nil {
				t.Fatalf("Stop(%s): %v", label, err)
			}
			return
		}

		select {
		case <-frameReady:
		case <-statusReady:
		case <-deadline:
			t.Fatalf("timed out waiting for playback frames; presented=%d status=%q", presented, status)
		}
	}
}
