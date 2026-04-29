package nativeplayer

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/codemodify/av1go-codex/internal/displaycolor"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
	"github.com/codemodify/av1go-codex/pkg/av1/encoder"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
	"github.com/codemodify/av1go-codex/pkg/container/mp4"
)

const DemoFrameCount = 2

const (
	decodeQueueSize        = 12
	startupPrerollFrames   = 6
	startupPrerollDuration = 180 * time.Millisecond
	statusUpdateInterval   = 250 * time.Millisecond
	minLateDropThreshold   = 30 * time.Millisecond
	maxLateDropThreshold   = 120 * time.Millisecond
)

var featuredDefaultVideos = []string{
	"AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4",
	"BigBuckBunny-AV1.mp4",
	"Sintel_1080_10s_30MB.mp4",
}

type VideoInfo struct {
	Name        string
	Path        string
	Width       int
	Height      int
	SampleCount int
	Timescale   uint32
	Duration    time.Duration
	Codec       string
	BitDepth    uint8
	Chroma      string
	Decoder     string
}

type PlaybackTarget struct {
	Path      string
	Source    string
	Generated bool
}

type playbackSession struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type PlayerState struct {
	mu      sync.Mutex
	session *playbackSession
}

type playbackItem struct {
	frame *decoder.Frame
	err   error
	eof   bool
}

type playbackTiming struct {
	index    int
	pts      time.Duration
	duration time.Duration
}

var playbackRGBAPixPool sync.Pool

func ListVideoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".mp4") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no playable mp4 files found in %s", dir)
	}
	return files, nil
}

func InspectVideo(path string) (VideoInfo, error) {
	f, err := mp4.Open(path)
	if err != nil {
		return VideoInfo{}, err
	}
	defer f.Close()

	track, err := f.AV1VideoTrack()
	if err != nil {
		return VideoInfo{}, err
	}
	raw, err := track.AV1C.MarshalBinary()
	if err != nil {
		return VideoInfo{}, err
	}
	dec, err := decoder.OpenCodecConfig(raw)
	if err != nil {
		return VideoInfo{}, err
	}
	meta := dec.Metadata()
	return VideoInfo{
		Name:        filepath.Base(path),
		Path:        path,
		Width:       track.Width,
		Height:      track.Height,
		SampleCount: len(track.Samples),
		Timescale:   track.Timescale,
		Duration:    sampleDuration(track.Duration, track.Timescale),
		Codec:       track.AV1C.CodecString(),
		BitDepth:    meta.BitDepth,
		Chroma:      chromaString(meta.Chroma),
		Decoder:     decoder.BackendDescription(),
	}, nil
}

func ResolvePlaybackTarget(input string, searchDirs []string) (PlaybackTarget, error) {
	if input != "" {
		path, source, info, err := resolvePlaybackInput(input, searchDirs)
		if err != nil {
			return PlaybackTarget{}, err
		}
		if info.IsDir() {
			files, err := ListVideoFiles(path)
			if err != nil {
				return PlaybackTarget{}, err
			}
			selected := preferredVideoFile(files)
			return PlaybackTarget{
				Path:   filepath.Join(path, selected),
				Source: fmt.Sprintf("%s (selected %s)", source, selected),
			}, nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".mp4") {
			return PlaybackTarget{}, fmt.Errorf("unsupported input %s: expected .mp4 file or directory", input)
		}
		return PlaybackTarget{
			Path:   path,
			Source: source,
		}, nil
	}

	for _, dir := range searchDirs {
		files, err := ListVideoFiles(dir)
		if err == nil && len(files) > 0 {
			selected := preferredVideoFile(files)
			return PlaybackTarget{
				Path:   filepath.Join(dir, selected),
				Source: fmt.Sprintf("directory %s (selected %s)", dir, selected),
			}, nil
		}
	}

	path, err := createDemoClip()
	if err != nil {
		return PlaybackTarget{}, err
	}
	return PlaybackTarget{
		Path:      path,
		Source:    "generated 16x16 AV1 demo clip",
		Generated: true,
	}, nil
}

func resolvePlaybackInput(input string, searchDirs []string) (path string, source string, info os.FileInfo, err error) {
	info, err = os.Stat(input)
	if err == nil {
		if info.IsDir() {
			return input, "directory " + input, info, nil
		}
		return input, "explicit file " + input, info, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", "", nil, err
	}

	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, input)
		candidateInfo, candidateErr := os.Stat(candidate)
		if candidateErr == nil {
			if candidateInfo.IsDir() {
				return candidate, fmt.Sprintf("directory %s", candidate), candidateInfo, nil
			}
			return candidate, fmt.Sprintf("search dir %s (%s)", dir, input), candidateInfo, nil
		}
		if candidateErr != nil && !errors.Is(candidateErr, os.ErrNotExist) {
			return "", "", nil, candidateErr
		}
	}

	return "", "", nil, err
}

func (s *PlayerState) Play(path string, onFrame func(*decoder.Frame, int), onStatus func(string)) error {
	if err := s.Stop(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	session := &playbackSession{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	s.mu.Lock()
	s.session = session
	s.mu.Unlock()

	items := make(chan playbackItem, decodeQueueSize)
	workerDone := make(chan struct{})

	go func() {
		defer close(workerDone)
		defer close(items)
		dec, err := decoder.OpenMP4(path)
		if err != nil {
			sendPlaybackItem(ctx, items, playbackItem{err: err})
			return
		}
		defer dec.Close()
		decodePlaybackItems(ctx, dec, items)
	}()

	go func() {
		defer close(session.done)
		defer s.clearSession(session)
		defer cancel()

		presentPlaybackItems(ctx, items, onFrame, onStatus)
		<-workerDone
	}()

	return nil
}

func (s *PlayerState) HasSession() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session != nil
}

func (s *PlayerState) Stop() error {
	s.mu.Lock()
	session := s.session
	s.session = nil
	s.mu.Unlock()

	if session == nil {
		return nil
	}
	session.cancel()
	<-session.done
	return nil
}

func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Truncate(time.Millisecond).String()
}

func preferredVideoFile(files []string) string {
	for _, preferred := range featuredDefaultVideos {
		for _, file := range files {
			if file == preferred {
				return file
			}
		}
	}
	return files[0]
}

func PlaceholderImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.RGBA{R: 28, G: 31, B: 38, A: 255}
			if ((x/24)+(y/24))%2 == 0 {
				c = color.RGBA{R: 38, G: 42, B: 50, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func ParsedFrameToPlaybackFrame(meta decoder.Metadata, parsed *decoder.ParsedFrame) *decoder.Frame {
	width := parsed.Header.Width
	height := parsed.Header.Height
	if width <= 0 {
		width = meta.Width
	}
	if height <= 0 {
		height = meta.Height
	}
	return &decoder.Frame{
		Index:    parsed.Index,
		Image:    syntaxPreviewImage(width, height, parsed),
		Width:    width,
		Height:   height,
		BitDepth: meta.BitDepth,
		Layout:   meta.Chroma,
		PTS:      parsed.PTS,
		Duration: parsed.Duration,
	}
}

func prepareFrameForDisplay(src *decoder.Frame) *decoder.Frame {
	if src == nil {
		return nil
	}
	clone := *src
	var release func()
	switch img := src.Image.(type) {
	case *image.RGBA:
		if img.Bounds().Dx() > 0 && img.Bounds().Dy() > 0 && img.Stride == img.Bounds().Dx()*4 {
			clone.Image = img
		} else {
			clone.Image, release = copyImageRGBA(src)
		}
	default:
		clone.Image, release = copyImageRGBA(src)
	}
	clone.SetCloseHook(release)

	// The player only needs a display image and timestamps. Releasing plane
	// slices here prevents the UI and channel buffers from retaining extra frame
	// storage for long HD clips.
	clone.Y = nil
	clone.U = nil
	clone.V = nil
	clone.Y16 = nil
	clone.U16 = nil
	clone.V16 = nil
	clone.YStride = 0
	clone.UStride = 0
	clone.VStride = 0
	return &clone
}

// PrepareFrameForDisplay converts a decoded frame into the same display-ready
// form used by the native player. The returned frame must be closed by the
// caller when no longer needed.
func PrepareFrameForDisplay(src *decoder.Frame) *decoder.Frame {
	return prepareFrameForDisplay(src)
}

func takePlaybackRGBA(width, height int) (*image.RGBA, func()) {
	if width <= 0 || height <= 0 {
		return nil, nil
	}
	stride := width * 4
	need := stride * height
	bufAny := playbackRGBAPixPool.Get()
	var pix []byte
	if bufAny != nil {
		pix, _ = bufAny.([]byte)
	}
	if cap(pix) < need {
		pix = make([]byte, need)
	} else {
		pix = pix[:need]
	}
	img := &image.RGBA{
		Pix:    pix,
		Stride: stride,
		Rect:   image.Rect(0, 0, width, height),
	}
	return img, func() {
		playbackRGBAPixPool.Put(pix[:0])
	}
}

func copyImageRGBA(frame *decoder.Frame) (*image.RGBA, func()) {
	width, height := frameDimensions(frame)
	if width <= 0 || height <= 0 {
		return nil, nil
	}
	rgba, release := takePlaybackRGBA(width, height)
	if rgba == nil {
		return nil, nil
	}

	if frame.Image != nil {
		switch img := frame.Image.(type) {
		case *image.YCbCr:
			copyYCbCrToRGBA(rgba, img, normalizeDisplayColorConfig(frame.Color, frame.Layout, frame.BitDepth))
			return rgba, release
		default:
			bounds := frame.Image.Bounds()
			imagedraw.Draw(rgba, rgba.Bounds(), frame.Image, bounds.Min, imagedraw.Src)
			return rgba, release
		}
	}

	if canConvertPlanar420Main16(frame, width, height) {
		convertPlanar420Main16ToRGBA(rgba, frame)
		return rgba, release
	}
	if canConvertPlanar420Main8(frame, width, height) {
		convertPlanar420Main8ToRGBA(rgba, frame)
		return rgba, release
	}
	clear(rgba.Pix)
	return rgba, release
}

func frameDimensions(frame *decoder.Frame) (int, int) {
	if frame.Width > 0 && frame.Height > 0 {
		return frame.Width, frame.Height
	}
	if frame.Image != nil {
		bounds := frame.Image.Bounds()
		return bounds.Dx(), bounds.Dy()
	}
	return 0, 0
}

func canConvertPlanar420Main8(frame *decoder.Frame, width, height int) bool {
	if len(frame.Y) == 0 || len(frame.U) == 0 || len(frame.V) == 0 {
		return false
	}
	if frame.YStride < width {
		return false
	}
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	if frame.UStride < chromaWidth || frame.VStride < chromaWidth {
		return false
	}
	if len(frame.Y) < (height-1)*frame.YStride+width {
		return false
	}
	if len(frame.U) < (chromaHeight-1)*frame.UStride+chromaWidth {
		return false
	}
	if len(frame.V) < (chromaHeight-1)*frame.VStride+chromaWidth {
		return false
	}
	layout := frame.Layout
	if layout == av1.ChromaUnknown {
		layout = frame.Color.ChromaSubsampling()
	}
	return layout == av1.Chroma420 && frame.BitDepth <= 8
}

func canConvertPlanar420Main16(frame *decoder.Frame, width, height int) bool {
	if frame.BitDepth <= 8 || frame.BitDepth > 12 {
		return false
	}
	if len(frame.Y16) == 0 || len(frame.U16) == 0 || len(frame.V16) == 0 {
		return false
	}
	if frame.YStride < width {
		return false
	}
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	if frame.UStride < chromaWidth || frame.VStride < chromaWidth {
		return false
	}
	if len(frame.Y16) < (height-1)*frame.YStride+width {
		return false
	}
	if len(frame.U16) < (chromaHeight-1)*frame.UStride+chromaWidth {
		return false
	}
	if len(frame.V16) < (chromaHeight-1)*frame.VStride+chromaWidth {
		return false
	}
	layout := frame.Layout
	if layout == av1.ChromaUnknown {
		layout = frame.Color.ChromaSubsampling()
	}
	return layout == av1.Chroma420
}

func convertPlanar420Main8ToRGBA(dst *image.RGBA, frame *decoder.Frame) {
	width, height := frameDimensions(frame)
	colorCfg := normalizeDisplayColorConfig(frame.Color, frame.Layout, frame.BitDepth)
	converter := newDisplayYUVConverter(colorCfg, width, height)
	if converter.lut != nil {
		convertPlanar420Main8ToRGBAWithLUT(dst, frame, width, height, converter.lut)
		return
	}
	for y := 0; y < height; y++ {
		yRow := frame.Y[y*frame.YStride:]
		uRow := frame.U[(y>>1)*frame.UStride:]
		vRow := frame.V[(y>>1)*frame.VStride:]
		dstRow := dst.Pix[y*dst.Stride:]
		for x := 0; x < width; x++ {
			r, g, b := converter.toRGB(int(yRow[x]), int(uRow[x>>1]), int(vRow[x>>1]))
			base := x * 4
			dstRow[base+0] = r
			dstRow[base+1] = g
			dstRow[base+2] = b
			dstRow[base+3] = 0xff
		}
	}
}

func convertPlanar420Main16ToRGBA(dst *image.RGBA, frame *decoder.Frame) {
	width, height := frameDimensions(frame)
	colorCfg := normalizeDisplayColorConfig(frame.Color, frame.Layout, frame.BitDepth)
	lut := newDisplayYUVLUT16For(colorCfg, width, height)
	convertPlanar420Main16ToRGBAWithLUT(dst, frame, width, height, lut)
}

func convertPlanar420Main8ToRGBAWithLUT(dst *image.RGBA, frame *decoder.Frame, width, height int, lut *displayYUVLUT) {
	const round = int32(1) << (displayYUVLUTShift - 1)
	chromaWidth := (width + 1) >> 1
	for y := 0; y < height; y += 2 {
		yRow0 := frame.Y[y*frame.YStride:]
		dstRow0 := dst.Pix[y*dst.Stride:]
		uRow := frame.U[(y>>1)*frame.UStride:][:chromaWidth]
		vRow := frame.V[(y>>1)*frame.VStride:][:chromaWidth]

		var yRow1 []byte
		var dstRow1 []byte
		if y+1 < height {
			yRow1 = frame.Y[(y+1)*frame.YStride:]
			dstRow1 = dst.Pix[(y+1)*dst.Stride:]
		}

		for cx := 0; cx < chromaWidth; cx++ {
			u := uRow[cx]
			v := vRow[cx]
			uToG := lut.uToG[u]
			uToB := lut.uToB[u]
			vToR := lut.vToR[v]
			vToG := lut.vToG[v]
			base := cx << 3
			x := cx << 1

			yBase0 := lut.y[yRow0[x]]
			dstRow0[base+0] = clipByte(int((yBase0 + vToR + round) >> displayYUVLUTShift))
			dstRow0[base+1] = clipByte(int((yBase0 + uToG + vToG + round) >> displayYUVLUTShift))
			dstRow0[base+2] = clipByte(int((yBase0 + uToB + round) >> displayYUVLUTShift))
			dstRow0[base+3] = 0xff

			if x+1 < width {
				yBase1 := lut.y[yRow0[x+1]]
				dstRow0[base+4] = clipByte(int((yBase1 + vToR + round) >> displayYUVLUTShift))
				dstRow0[base+5] = clipByte(int((yBase1 + uToG + vToG + round) >> displayYUVLUTShift))
				dstRow0[base+6] = clipByte(int((yBase1 + uToB + round) >> displayYUVLUTShift))
				dstRow0[base+7] = 0xff
			}

			if yRow1 == nil {
				continue
			}
			yBase2 := lut.y[yRow1[x]]
			dstRow1[base+0] = clipByte(int((yBase2 + vToR + round) >> displayYUVLUTShift))
			dstRow1[base+1] = clipByte(int((yBase2 + uToG + vToG + round) >> displayYUVLUTShift))
			dstRow1[base+2] = clipByte(int((yBase2 + uToB + round) >> displayYUVLUTShift))
			dstRow1[base+3] = 0xff

			if x+1 < width {
				yBase3 := lut.y[yRow1[x+1]]
				dstRow1[base+4] = clipByte(int((yBase3 + vToR + round) >> displayYUVLUTShift))
				dstRow1[base+5] = clipByte(int((yBase3 + uToG + vToG + round) >> displayYUVLUTShift))
				dstRow1[base+6] = clipByte(int((yBase3 + uToB + round) >> displayYUVLUTShift))
				dstRow1[base+7] = 0xff
			}
		}
	}
}

func convertPlanar420Main16ToRGBAWithLUT(dst *image.RGBA, frame *decoder.Frame, width, height int, lut *displayYUVLUT16) {
	const round = int32(1) << (displayYUVLUTShift - 1)
	chromaWidth := (width + 1) >> 1
	for y := 0; y < height; y += 2 {
		yRow0 := frame.Y16[y*frame.YStride:]
		dstRow0 := dst.Pix[y*dst.Stride:]
		uRow := frame.U16[(y>>1)*frame.UStride:][:chromaWidth]
		vRow := frame.V16[(y>>1)*frame.VStride:][:chromaWidth]

		var yRow1 []uint16
		var dstRow1 []byte
		if y+1 < height {
			yRow1 = frame.Y16[(y+1)*frame.YStride:]
			dstRow1 = dst.Pix[(y+1)*dst.Stride:]
		}

		for cx := 0; cx < chromaWidth; cx++ {
			u := displayLUTIndex16(uRow[cx], lut.maxSample)
			v := displayLUTIndex16(vRow[cx], lut.maxSample)
			uToG := lut.uToG[u]
			uToB := lut.uToB[u]
			vToR := lut.vToR[v]
			vToG := lut.vToG[v]
			base := cx << 3
			x := cx << 1

			yBase0 := lut.y[displayLUTIndex16(yRow0[x], lut.maxSample)]
			dstRow0[base+0] = clipByte(int((yBase0 + vToR + round) >> displayYUVLUTShift))
			dstRow0[base+1] = clipByte(int((yBase0 + uToG + vToG + round) >> displayYUVLUTShift))
			dstRow0[base+2] = clipByte(int((yBase0 + uToB + round) >> displayYUVLUTShift))
			dstRow0[base+3] = 0xff

			if x+1 < width {
				yBase1 := lut.y[displayLUTIndex16(yRow0[x+1], lut.maxSample)]
				dstRow0[base+4] = clipByte(int((yBase1 + vToR + round) >> displayYUVLUTShift))
				dstRow0[base+5] = clipByte(int((yBase1 + uToG + vToG + round) >> displayYUVLUTShift))
				dstRow0[base+6] = clipByte(int((yBase1 + uToB + round) >> displayYUVLUTShift))
				dstRow0[base+7] = 0xff
			}

			if yRow1 == nil {
				continue
			}
			yBase2 := lut.y[displayLUTIndex16(yRow1[x], lut.maxSample)]
			dstRow1[base+0] = clipByte(int((yBase2 + vToR + round) >> displayYUVLUTShift))
			dstRow1[base+1] = clipByte(int((yBase2 + uToG + vToG + round) >> displayYUVLUTShift))
			dstRow1[base+2] = clipByte(int((yBase2 + uToB + round) >> displayYUVLUTShift))
			dstRow1[base+3] = 0xff

			if x+1 < width {
				yBase3 := lut.y[displayLUTIndex16(yRow1[x+1], lut.maxSample)]
				dstRow1[base+4] = clipByte(int((yBase3 + vToR + round) >> displayYUVLUTShift))
				dstRow1[base+5] = clipByte(int((yBase3 + uToG + vToG + round) >> displayYUVLUTShift))
				dstRow1[base+6] = clipByte(int((yBase3 + uToB + round) >> displayYUVLUTShift))
				dstRow1[base+7] = 0xff
			}
		}
	}
}

func copyYCbCrToRGBA(dst *image.RGBA, src *image.YCbCr, colorCfg av1.ColorConfig) {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	colorCfg = normalizeDisplayColorConfig(colorCfg, chromaLayoutFromYCbCrSubsample(src.SubsampleRatio), 8)
	converter := newDisplayYUVConverter(colorCfg, width, height)
	if converter.lut != nil && src.SubsampleRatio == image.YCbCrSubsampleRatio420 {
		copyYCbCr420ToRGBAWithLUT(dst, src, bounds, width, height, converter.lut)
		return
	}
	for y := 0; y < height; y++ {
		dstRow := dst.Pix[y*dst.Stride:]
		for x := 0; x < width; x++ {
			yv, uv, vv := ycbcrAt(src, x+bounds.Min.X, y+bounds.Min.Y)
			r, g, b := converter.toRGB(yv, uv, vv)
			base := x * 4
			dstRow[base+0] = r
			dstRow[base+1] = g
			dstRow[base+2] = b
			dstRow[base+3] = 0xff
		}
	}
}

func copyYCbCr420ToRGBAWithLUT(dst *image.RGBA, src *image.YCbCr, bounds image.Rectangle, width, height int, lut *displayYUVLUT) {
	const round = int32(1) << (displayYUVLUTShift - 1)
	chromaWidth := (width + 1) >> 1
	minX := bounds.Min.X
	minY := bounds.Min.Y
	for y := 0; y < height; y += 2 {
		srcY0 := minY + y
		yRow0 := src.Y[src.YOffset(minX, srcY0):]
		dstRow0 := dst.Pix[y*dst.Stride:]
		uRow := src.Cb[src.COffset(minX, srcY0):][:chromaWidth]
		vRow := src.Cr[src.COffset(minX, srcY0):][:chromaWidth]

		var yRow1 []byte
		var dstRow1 []byte
		if y+1 < height {
			yRow1 = src.Y[src.YOffset(minX, srcY0+1):]
			dstRow1 = dst.Pix[(y+1)*dst.Stride:]
		}

		for cx := 0; cx < chromaWidth; cx++ {
			u := uRow[cx]
			v := vRow[cx]
			uToG := lut.uToG[u]
			uToB := lut.uToB[u]
			vToR := lut.vToR[v]
			vToG := lut.vToG[v]
			base := cx << 3
			x := cx << 1

			yBase0 := lut.y[yRow0[x]]
			dstRow0[base+0] = clipByte(int((yBase0 + vToR + round) >> displayYUVLUTShift))
			dstRow0[base+1] = clipByte(int((yBase0 + uToG + vToG + round) >> displayYUVLUTShift))
			dstRow0[base+2] = clipByte(int((yBase0 + uToB + round) >> displayYUVLUTShift))
			dstRow0[base+3] = 0xff

			if x+1 < width {
				yBase1 := lut.y[yRow0[x+1]]
				dstRow0[base+4] = clipByte(int((yBase1 + vToR + round) >> displayYUVLUTShift))
				dstRow0[base+5] = clipByte(int((yBase1 + uToG + vToG + round) >> displayYUVLUTShift))
				dstRow0[base+6] = clipByte(int((yBase1 + uToB + round) >> displayYUVLUTShift))
				dstRow0[base+7] = 0xff
			}

			if yRow1 == nil {
				continue
			}
			yBase2 := lut.y[yRow1[x]]
			dstRow1[base+0] = clipByte(int((yBase2 + vToR + round) >> displayYUVLUTShift))
			dstRow1[base+1] = clipByte(int((yBase2 + uToG + vToG + round) >> displayYUVLUTShift))
			dstRow1[base+2] = clipByte(int((yBase2 + uToB + round) >> displayYUVLUTShift))
			dstRow1[base+3] = 0xff

			if x+1 < width {
				yBase3 := lut.y[yRow1[x+1]]
				dstRow1[base+4] = clipByte(int((yBase3 + vToR + round) >> displayYUVLUTShift))
				dstRow1[base+5] = clipByte(int((yBase3 + uToG + vToG + round) >> displayYUVLUTShift))
				dstRow1[base+6] = clipByte(int((yBase3 + uToB + round) >> displayYUVLUTShift))
				dstRow1[base+7] = 0xff
			}
		}
	}
}

func normalizeDisplayColorConfig(colorCfg av1.ColorConfig, layout av1.ChromaSubsampling, bitDepth uint8) av1.ColorConfig {
	if bitDepth == 0 {
		bitDepth = colorCfg.BitDepth
	}
	if bitDepth == 0 {
		bitDepth = 8
	}
	colorCfg.BitDepth = bitDepth
	switch layout {
	case av1.Chroma400:
		colorCfg.Monochrome = true
	case av1.Chroma420:
		colorCfg.Monochrome = false
		colorCfg.SubsamplingX = true
		colorCfg.SubsamplingY = true
	case av1.Chroma422:
		colorCfg.Monochrome = false
		colorCfg.SubsamplingX = true
		colorCfg.SubsamplingY = false
	case av1.Chroma444:
		colorCfg.Monochrome = false
		colorCfg.SubsamplingX = false
		colorCfg.SubsamplingY = false
	}
	return colorCfg
}

func chromaLayoutFromYCbCrSubsample(r image.YCbCrSubsampleRatio) av1.ChromaSubsampling {
	switch r {
	case image.YCbCrSubsampleRatio422:
		return av1.Chroma422
	case image.YCbCrSubsampleRatio444:
		return av1.Chroma444
	case image.YCbCrSubsampleRatio440:
		return av1.ChromaUnknown
	default:
		return av1.Chroma420
	}
}

func ycbcrAt(img *image.YCbCr, x, y int) (int, int, int) {
	yi := img.YOffset(x, y)
	ci := img.COffset(x, y)
	return int(img.Y[yi]), int(img.Cb[ci]), int(img.Cr[ci])
}

type displayYUVConverter struct {
	passthrough bool
	lut         *displayYUVLUT
	colorCfg    av1.ColorConfig
}

type displayYUVLUT struct {
	y    [256]int32
	uToG [256]int32
	uToB [256]int32
	vToR [256]int32
	vToG [256]int32
}

type displayYUVLUT16 struct {
	y         []int32
	uToG      []int32
	uToB      []int32
	vToR      []int32
	vToG      []int32
	maxSample int
}

const displayYUVLUTShift = 16

var (
	displayYUVLUT601Limited  = newDisplayYUVLUT(false, 1.4020, -0.344136, -0.714136, 1.7720)
	displayYUVLUT601Full     = newDisplayYUVLUT(true, 1.4020, -0.344136, -0.714136, 1.7720)
	displayYUVLUT709Limited  = newDisplayYUVLUT(false, 1.5748, -0.187324, -0.468124, 1.8556)
	displayYUVLUT709Full     = newDisplayYUVLUT(true, 1.5748, -0.187324, -0.468124, 1.8556)
	displayYUVLUT2020Limited = newDisplayYUVLUT(false, 1.4746, -0.164553, -0.571353, 1.8814)
	displayYUVLUT2020Full    = newDisplayYUVLUT(true, 1.4746, -0.164553, -0.571353, 1.8814)
)

func roundDisplayLUT(v float64) int32 {
	if v >= 0 {
		return int32(v + 0.5)
	}
	return int32(v - 0.5)
}

func newDisplayYUVLUT(fullRange bool, rCr, gCb, gCr, bCb float64) *displayYUVLUT {
	lut := &displayYUVLUT{}
	scale := float64(int32(1) << displayYUVLUTShift)
	for i := 0; i < 256; i++ {
		yf := float64(i)
		uf := float64(i)
		vf := float64(i)
		var yNorm, cb, cr float64
		if fullRange {
			yNorm = yf / 255.0
			cb = uf/255.0 - 0.5
			cr = vf/255.0 - 0.5
		} else {
			yNorm = (yf - 16.0) / 219.0
			cb = (uf - 128.0) / 224.0
			cr = (vf - 128.0) / 224.0
		}
		if yNorm < 0 {
			yNorm = 0
		}
		lut.y[i] = roundDisplayLUT(yNorm * 255.0 * scale)
		lut.uToG[i] = roundDisplayLUT(gCb * cb * 255.0 * scale)
		lut.uToB[i] = roundDisplayLUT(bCb * cb * 255.0 * scale)
		lut.vToR[i] = roundDisplayLUT(rCr * cr * 255.0 * scale)
		lut.vToG[i] = roundDisplayLUT(gCr * cr * 255.0 * scale)
	}
	return lut
}

func newDisplayYUVLUT16(bitDepth uint8, fullRange bool, rCr, gCb, gCr, bCb float64) *displayYUVLUT16 {
	if bitDepth < 8 {
		bitDepth = 8
	}
	if bitDepth > 12 {
		bitDepth = 12
	}
	entries := 1 << bitDepth
	lut := &displayYUVLUT16{
		y:         make([]int32, entries),
		uToG:      make([]int32, entries),
		uToB:      make([]int32, entries),
		vToR:      make([]int32, entries),
		vToG:      make([]int32, entries),
		maxSample: entries - 1,
	}
	scale := float64(int32(1) << displayYUVLUTShift)
	sampleScaleShift := max(int(bitDepth)-8, 0)
	sampleScale := 1
	if sampleScaleShift > 0 {
		sampleScale = sampleScale << sampleScaleShift
	}
	scaleInt := float64(sampleScale)
	maxValue := float64(entries - 1)
	for i := 0; i < entries; i++ {
		yf := float64(i)
		uf := float64(i)
		vf := float64(i)
		var yNorm, cb, cr float64
		if fullRange {
			yNorm = yf / maxValue
			cb = uf/maxValue - 0.5
			cr = vf/maxValue - 0.5
		} else {
			yNorm = (yf - 16.0*scaleInt) / (219.0 * scaleInt)
			cb = (uf - 128.0*scaleInt) / (224.0 * scaleInt)
			cr = (vf - 128.0*scaleInt) / (224.0 * scaleInt)
		}
		if yNorm < 0 {
			yNorm = 0
		}
		lut.y[i] = roundDisplayLUT(yNorm * 255.0 * scale)
		lut.uToG[i] = roundDisplayLUT(gCb * cb * 255.0 * scale)
		lut.uToB[i] = roundDisplayLUT(bCb * cb * 255.0 * scale)
		lut.vToR[i] = roundDisplayLUT(rCr * cr * 255.0 * scale)
		lut.vToG[i] = roundDisplayLUT(gCr * cr * 255.0 * scale)
	}
	return lut
}

func displayYUVLUTFor(colorCfg av1.ColorConfig, width, height int) *displayYUVLUT {
	switch previewMatrix(colorCfg, width, height) {
	case 601:
		if colorCfg.FullRange {
			return displayYUVLUT601Full
		}
		return displayYUVLUT601Limited
	case 2020:
		if colorCfg.FullRange {
			return displayYUVLUT2020Full
		}
		return displayYUVLUT2020Limited
	default:
		if colorCfg.FullRange {
			return displayYUVLUT709Full
		}
		return displayYUVLUT709Limited
	}
}

func newDisplayYUVLUT16For(colorCfg av1.ColorConfig, width, height int) *displayYUVLUT16 {
	switch previewMatrix(colorCfg, width, height) {
	case 601:
		return newDisplayYUVLUT16(colorCfg.BitDepth, colorCfg.FullRange, 1.4020, -0.344136, -0.714136, 1.7720)
	case 2020:
		return newDisplayYUVLUT16(colorCfg.BitDepth, colorCfg.FullRange, 1.4746, -0.164553, -0.571353, 1.8814)
	default:
		return newDisplayYUVLUT16(colorCfg.BitDepth, colorCfg.FullRange, 1.5748, -0.187324, -0.468124, 1.8556)
	}
}

func displayLUTIndex16(v uint16, maxSample int) int {
	if int(v) > maxSample {
		return maxSample
	}
	return int(v)
}

func newDisplayYUVConverter(colorCfg av1.ColorConfig, width, height int) displayYUVConverter {
	if colorCfg.MatrixCoefficients == 0 && colorCfg.ChromaSubsampling() == av1.Chroma444 && !colorCfg.Monochrome {
		return displayYUVConverter{passthrough: true, colorCfg: colorCfg}
	}
	if displaycolor.CanUseSimple8BitLUT(colorCfg) {
		return displayYUVConverter{lut: displayYUVLUTFor(colorCfg, width, height), colorCfg: colorCfg}
	}
	return displayYUVConverter{colorCfg: colorCfg}
}

func (c displayYUVConverter) toRGB(yv, uv, vv int) (uint8, uint8, uint8) {
	if c.passthrough {
		return displaycolor.YUVToDisplayRGB(c.colorCfg, yv, uv, vv)
	}
	if c.lut != nil {
		const round = int32(1) << (displayYUVLUTShift - 1)
		yBase := c.lut.y[yv]
		r := int((yBase + c.lut.vToR[vv] + round) >> displayYUVLUTShift)
		g := int((yBase + c.lut.uToG[uv] + c.lut.vToG[vv] + round) >> displayYUVLUTShift)
		b := int((yBase + c.lut.uToB[uv] + round) >> displayYUVLUTShift)
		return clipByte(r), clipByte(g), clipByte(b)
	}
	return displaycolor.YUVToDisplayRGB(c.colorCfg, yv, uv, vv)
}

func yuvToDisplayRGB(colorCfg av1.ColorConfig, width, height int, yv, uv, vv int) (uint8, uint8, uint8) {
	_ = width
	_ = height
	return displaycolor.YUVToDisplayRGB(colorCfg, yv, uv, vv)
}

func previewMatrix(colorCfg av1.ColorConfig, width, height int) int {
	_ = width
	_ = height
	return displaycolor.PreviewMatrix(colorCfg)
}

func clipByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func (s *PlayerState) clearSession(session *playbackSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == session {
		s.session = nil
	}
}

func createDemoClip() (string, error) {
	dir, err := os.MkdirTemp("", "av1go-nativeplayer-*")
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "demo-black16.mp4")
	out, err := os.Create(path)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	defer out.Close()

	enc, err := encoder.New(encoder.Config{
		Width:     16,
		Height:    16,
		Timescale: 1000,
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}

	frames := make(chan *image.YCbCr, DemoFrameCount)
	for i := 0; i < DemoFrameCount; i++ {
		frames <- blackYCbCr16()
	}
	close(frames)

	if err := enc.EncodeAllIntraMP4(frames, out); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return path, nil
}

func blackYCbCr16() *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio420)
	for i := range img.Cb {
		img.Cb[i] = 128
		img.Cr[i] = 128
	}
	return img
}

func chromaString(v interface{ String() string }) string {
	if v == nil {
		return "unknown"
	}
	return v.String()
}

func sampleDuration(value uint64, timescale uint32) time.Duration {
	if timescale == 0 {
		return 0
	}
	return time.Duration(float64(value) / float64(timescale) * float64(time.Second))
}

func syntaxPreviewImage(width, height int, parsed *decoder.ParsedFrame) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	accent := frameAccent(parsed)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			base := uint8(22)
			if ((x/24)+(y/24))%2 == 0 {
				base = 34
			}
			c := color.RGBA{
				R: clampColor(int(base) + int(accent.R)/4),
				G: clampColor(int(base) + int(accent.G)/4),
				B: clampColor(int(base) + int(accent.B)/4),
				A: 255,
			}
			if !parsed.Header.ShowFrame && !parsed.Header.ShowExistingFrame {
				c.R /= 2
				c.G /= 2
				c.B /= 2
			}
			img.SetRGBA(x, y, c)
		}
	}

	border := accent
	for x := 0; x < width; x++ {
		img.SetRGBA(x, 0, border)
		img.SetRGBA(x, height-1, border)
	}
	for y := 0; y < height; y++ {
		img.SetRGBA(0, y, border)
		img.SetRGBA(width-1, y, border)
	}
	drawTileGrid(img, parsed)
	return img
}

func drawTileGrid(img *image.RGBA, parsed *decoder.ParsedFrame) {
	cols := int(parsed.Header.Tiling.Cols)
	rows := int(parsed.Header.Tiling.Rows)
	if cols <= 1 && rows <= 1 {
		return
	}
	bounds := img.Bounds()
	grid := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	for col := 1; col < cols; col++ {
		x := bounds.Min.X + col*bounds.Dx()/cols
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			img.SetRGBA(x, y, grid)
		}
	}
	for row := 1; row < rows; row++ {
		y := bounds.Min.Y + row*bounds.Dy()/rows
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetRGBA(x, y, grid)
		}
	}
}

func frameAccent(parsed *decoder.ParsedFrame) color.RGBA {
	switch parsed.Header.FrameType {
	case obu.FrameTypeKey:
		return color.RGBA{R: 52, G: 142, B: 255, A: 255}
	case obu.FrameTypeInter:
		return color.RGBA{R: 72, G: 198, B: 116, A: 255}
	case obu.FrameTypeIntra:
		return color.RGBA{R: 255, G: 170, B: 64, A: 255}
	default:
		return color.RGBA{R: 220, G: 84, B: 132, A: 255}
	}
}

func frameTypeString(v obu.FrameType) string {
	switch v {
	case obu.FrameTypeKey:
		return "key"
	case obu.FrameTypeInter:
		return "inter"
	case obu.FrameTypeIntra:
		return "intra"
	case obu.FrameTypeSwitch:
		return "switch"
	default:
		return fmt.Sprintf("unknown(%d)", v)
	}
}

func clampColor(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func decodePlaybackItems(ctx context.Context, dec *decoder.Decoder, items chan<- playbackItem) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		frame, err := dec.NextFrame()
		if err != nil {
			item := playbackItem{}
			switch {
			case errors.Is(err, io.EOF):
				item.eof = true
			default:
				item.err = err
			}
			sendPlaybackItem(ctx, items, item)
			return
		}
		prepared := prepareFrameForDisplay(frame)
		if !sendPlaybackItem(ctx, items, playbackItem{frame: prepared}) {
			closePlaybackFrame(prepared)
			return
		}
	}
}

func buildPlaybackTimeline(track *mp4.Track) []playbackTiming {
	if track == nil || len(track.Samples) == 0 {
		return nil
	}
	timeline := make([]playbackTiming, 0, len(track.Samples))
	for _, sample := range track.Samples {
		timeline = append(timeline, playbackTiming{
			index:    sample.Index,
			pts:      sampleDuration(uint64(sample.PTS), track.Timescale),
			duration: sampleDuration(uint64(sample.Duration), track.Timescale),
		})
	}
	sort.SliceStable(timeline, func(i, j int) bool {
		if timeline[i].pts == timeline[j].pts {
			return timeline[i].index < timeline[j].index
		}
		return timeline[i].pts < timeline[j].pts
	})
	return timeline
}

func playbackTimingForIndex(timeline []playbackTiming, frameIndex int) playbackTiming {
	if frameIndex >= 0 && frameIndex < len(timeline) {
		return timeline[frameIndex]
	}
	if len(timeline) == 0 {
		return playbackTiming{index: frameIndex}
	}
	last := timeline[len(timeline)-1]
	pts := last.pts + last.duration*time.Duration(frameIndex-len(timeline)+1)
	return playbackTiming{
		index:    frameIndex,
		pts:      pts,
		duration: last.duration,
	}
}

func sendPlaybackItem(ctx context.Context, items chan<- playbackItem, item playbackItem) bool {
	select {
	case <-ctx.Done():
		return false
	case items <- item:
		return true
	}
}

func presentPlaybackItems(ctx context.Context, items <-chan playbackItem, onFrame func(*decoder.Frame, int), onStatus func(string)) {
	buffer := make([]playbackItem, 0, decodeQueueSize)
	defer func() {
		closePlaybackItems(buffer)
	}()
	sourceClosed := false
	presented := 0
	dropped := 0
	underruns := 0
	var (
		basePTS      time.Duration
		anchor       time.Time
		started      bool
		lastStatus   string
		lastStatusAt time.Time
		delayTimer   *time.Timer
	)
	defer stopPlaybackTimer(&delayTimer)

	maybeStatus := func(text string, force bool) {
		now := time.Now()
		if !force {
			if text == lastStatus {
				return
			}
			if !lastStatusAt.IsZero() && now.Sub(lastStatusAt) < statusUpdateInterval {
				return
			}
		}
		onStatus(text)
		lastStatus = text
		lastStatusAt = now
	}

	maybeStatus("Buffering playback.", true)

	for {
		if !started {
			ok := fillStartupBuffer(ctx, items, &buffer, &sourceClosed)
			if !ok {
				return
			}
			if len(buffer) == 0 {
				if sourceClosed {
					maybeStatus("Playback finished.", true)
				}
				return
			}
			if buffer[0].err != nil {
				maybeStatus(fmt.Sprintf("Decode error: %v", buffer[0].err), true)
				return
			}
			if buffer[0].eof {
				maybeStatus("Playback finished.", true)
				return
			}
			basePTS = buffer[0].frame.PTS
			anchor = time.Now()
			started = true
			maybeStatus(playbackStatus(len(buffer)-1, dropped, underruns), true)
		}

		drainPlaybackItems(items, &buffer, &sourceClosed)
		if len(buffer) == 0 {
			if sourceClosed {
				maybeStatus("Playback finished.", true)
				return
			}
			started = false
			underruns++
			maybeStatus(bufferingStatus(dropped, underruns), true)
			continue
		}

		item := buffer[0]
		if item.err != nil {
			maybeStatus(fmt.Sprintf("Decode error: %v", item.err), true)
			return
		}
		if item.eof {
			maybeStatus("Playback finished.", true)
			return
		}

		delay := presentationDelay(anchor, basePTS, item.frame.PTS)
		if shouldDropLateFrame(delay, item.frame.Duration, len(buffer)-1) {
			dropped++
			closePlaybackFrame(item.frame)
			buffer = buffer[1:]
			maybeStatus(playbackStatus(len(buffer), dropped, underruns), false)
			continue
		}
		if delay > 0 {
			if !waitForPlaybackDelay(ctx, &delayTimer, delay) {
				return
			}
		}

		presented++
		onFrame(item.frame, presented)
		buffer = buffer[1:]
		maybeStatus(playbackStatus(len(buffer), dropped, underruns), false)
	}
}

func closePlaybackFrame(frame *decoder.Frame) {
	if frame == nil {
		return
	}
	_ = frame.Close()
}

func closePlaybackItems(items []playbackItem) {
	for i := range items {
		closePlaybackFrame(items[i].frame)
		items[i].frame = nil
	}
}

func fillStartupBuffer(ctx context.Context, items <-chan playbackItem, buffer *[]playbackItem, sourceClosed *bool) bool {
	if len(*buffer) == 0 {
		ok := receivePlaybackItem(ctx, items, buffer, sourceClosed)
		if !ok {
			return false
		}
	}
	if len(*buffer) == 0 {
		return true
	}
	if (*buffer)[0].err != nil || (*buffer)[0].eof {
		return true
	}

	timer := time.NewTimer(startupPrerollDuration)
	defer timer.Stop()

	for len(*buffer) < startupPrerollFrames && bufferedPlaybackSpan(*buffer) < startupPrerollDuration {
		if *sourceClosed {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case item, ok := <-items:
			if !ok {
				*sourceClosed = true
				return true
			}
			*buffer = append(*buffer, item)
			if item.err != nil || item.eof {
				*sourceClosed = true
				return true
			}
		case <-timer.C:
			return true
		}
	}
	return true
}

func receivePlaybackItem(ctx context.Context, items <-chan playbackItem, buffer *[]playbackItem, sourceClosed *bool) bool {
	select {
	case <-ctx.Done():
		return false
	case item, ok := <-items:
		if !ok {
			*sourceClosed = true
			return true
		}
		*buffer = append(*buffer, item)
		if item.err != nil || item.eof {
			*sourceClosed = true
		}
		return true
	}
}

func drainPlaybackItems(items <-chan playbackItem, buffer *[]playbackItem, sourceClosed *bool) {
	for !*sourceClosed && len(*buffer) < decodeQueueSize {
		select {
		case item, ok := <-items:
			if !ok {
				*sourceClosed = true
				return
			}
			*buffer = append(*buffer, item)
			if item.err != nil || item.eof {
				*sourceClosed = true
				return
			}
		default:
			return
		}
	}
}

func bufferedPlaybackSpan(items []playbackItem) time.Duration {
	if len(items) < 2 {
		if len(items) == 1 && items[0].frame != nil {
			return items[0].frame.Duration
		}
		return 0
	}

	first := -1
	last := -1
	for i, item := range items {
		if item.frame == nil {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 || last < 0 {
		return 0
	}
	span := items[last].frame.PTS - items[first].frame.PTS + items[last].frame.Duration
	if span < 0 {
		return 0
	}
	return span
}

func presentationDelay(anchor time.Time, basePTS, framePTS time.Duration) time.Duration {
	return framePTS - basePTS - time.Since(anchor)
}

func waitForPlaybackDelay(ctx context.Context, timer **time.Timer, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	if *timer == nil {
		*timer = time.NewTimer(delay)
	} else {
		resetPlaybackTimer(*timer, delay)
	}
	select {
	case <-ctx.Done():
		stopPlaybackTimer(timer)
		return false
	case <-(*timer).C:
		return true
	}
}

func stopPlaybackTimer(timer **time.Timer) {
	if timer == nil || *timer == nil {
		return
	}
	if !(*timer).Stop() {
		select {
		case <-(*timer).C:
		default:
		}
	}
	*timer = nil
}

func resetPlaybackTimer(timer *time.Timer, delay time.Duration) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func shouldDropLateFrame(delay, frameDuration time.Duration, bufferedAhead int) bool {
	if bufferedAhead <= 0 || delay >= 0 {
		return false
	}
	return -delay > lateDropThreshold(frameDuration)
}

func lateDropThreshold(frameDuration time.Duration) time.Duration {
	if frameDuration <= 0 {
		return minLateDropThreshold
	}
	threshold := frameDuration / 2
	if threshold < minLateDropThreshold {
		return minLateDropThreshold
	}
	if threshold > maxLateDropThreshold {
		return maxLateDropThreshold
	}
	return threshold
}

func bufferingStatus(dropped, underruns int) string {
	switch {
	case underruns > 0 && dropped > 0:
		return fmt.Sprintf("Buffering playback. Rebuffer %d. Dropped %d late frames.", underruns, dropped)
	case underruns > 0:
		return fmt.Sprintf("Buffering playback. Rebuffer %d.", underruns)
	case dropped > 0:
		return fmt.Sprintf("Buffering playback. Dropped %d late frames.", dropped)
	default:
		return "Buffering playback."
	}
}

func playbackStatus(buffered, dropped, underruns int) string {
	switch {
	case dropped > 0 && underruns > 0:
		return fmt.Sprintf("Playing decoded frames. Buffered %d. Dropped %d late frames. Rebuffered %d times.", buffered, dropped, underruns)
	case dropped > 0:
		return fmt.Sprintf("Playing decoded frames. Buffered %d. Dropped %d late frames.", buffered, dropped)
	case underruns > 0:
		return fmt.Sprintf("Playing decoded frames. Buffered %d. Rebuffered %d times.", buffered, underruns)
	default:
		return fmt.Sprintf("Playing decoded frames. Buffered %d.", buffered)
	}
}
