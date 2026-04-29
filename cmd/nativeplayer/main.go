package main

import (
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	player "github.com/codemodify/av1go-codex/internal/nativeplayer"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
)

const (
	initialWindowWidth  = 1320
	initialWindowHeight = 860
	minWindowWidth      = 960
	minWindowHeight     = 640
	defaultSidebarWidth = 360
	padding             = 16
	lineHeight          = 18
	defaultVideoDir     = "testvideo"
)

type playbackTarget = player.PlaybackTarget

type playerApp struct {
	state           *playerState
	videoDir        string
	files           []string
	selected        int
	scrollOffset    int
	info            videoInfo
	status          string
	frameText       string
	ptsText         string
	playbackPos     time.Duration
	playing         bool
	preview         *ebiten.Image
	previewScratch  *image.RGBA
	frameCh         chan frameEvent
	statusCh        chan string
	lastWindowTitle string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	input := ""
	if len(os.Args) > 1 {
		input = os.Args[1]
	}

	target, err := resolvePlaybackTarget(input, []string{defaultVideoDir, "."})
	if err != nil {
		return err
	}
	if target.Generated {
		defer os.RemoveAll(filepath.Dir(target.Path))
	}

	videoDir, files, selected, err := collectPlayerFiles(target)
	if err != nil {
		return err
	}

	state := &playerState{
		videoDir: videoDir,
		status:   "Ready.",
	}
	app, err := newPlayerApp(state, videoDir, files, selected)
	if err != nil {
		return err
	}
	defer app.state.stop()
	defer app.closePreview()

	ebiten.SetWindowSize(initialWindowWidth, initialWindowHeight)
	ebiten.SetTPS(ebiten.SyncWithFPS)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(app.windowTitle())
	return ebiten.RunGame(app)
}

func collectPlayerFiles(target playbackTarget) (string, []string, int, error) {
	videoDir := filepath.Dir(target.Path)
	files, err := listVideoFiles(videoDir)
	if err != nil {
		return "", nil, 0, err
	}
	selected := 0
	name := filepath.Base(target.Path)
	for i, file := range files {
		if file == name {
			selected = i
			break
		}
	}
	return videoDir, files, selected, nil
}

func resolvePlaybackTarget(input string, searchDirs []string) (playbackTarget, error) {
	return player.ResolvePlaybackTarget(input, searchDirs)
}

func newPlayerApp(state *playerState, videoDir string, files []string, selected int) (*playerApp, error) {
	app := &playerApp{
		state:     state,
		videoDir:  videoDir,
		files:     files,
		status:    state.status,
		frameText: "-",
		ptsText:   "-",
		frameCh:   make(chan frameEvent, 1),
		statusCh:  make(chan string, 8),
	}
	app.setPreview(placeholderImage(960, 540))
	if len(files) > 0 {
		if selected < 0 || selected >= len(files) {
			selected = 0
		}
		if err := app.selectFile(selected); err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (a *playerApp) Update() error {
	width, height := ebiten.WindowSize()
	width = maxInt(width, minWindowWidth)
	height = maxInt(height, minWindowHeight)
	layout := selectionLayout(width, height)
	a.ensureSelectionVisible(layout.visibleRows)

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		_ = a.state.stop()
		return ebiten.Termination
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		if index, ok := a.fileIndexAt(x, y, layout); ok {
			if err := a.selectIndex(index); err != nil {
				a.setStatus(fmt.Sprintf("Select failed: %v", err))
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		a.moveSelection(1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		a.moveSelection(-1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if a.state.hasSession() {
			_ = a.state.stop()
			a.drainPendingEvents()
			a.playing = false
			a.setStatus("Playback stopped.")
		} else {
			if err := a.startSelected(); err != nil {
				a.setStatus(fmt.Sprintf("Play failed: %v", err))
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		if err := a.startSelected(); err != nil {
			a.setStatus(fmt.Sprintf("Restart failed: %v", err))
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		_ = a.state.stop()
		a.drainPendingEvents()
		a.playing = false
		a.setStatus("Playback stopped.")
	}

	for {
		select {
		case status := <-a.statusCh:
			a.handleStatus(status)
		case evt := <-a.frameCh:
			a.frameText = fmt.Sprintf("%d", evt.frameCount)
			a.playbackPos = evt.frame.PTS + evt.frame.Duration
			if a.info.Duration > 0 && a.playbackPos > a.info.Duration {
				a.playbackPos = a.info.Duration
			}
			a.ptsText = fmt.Sprintf("%s / %s", formatDuration(evt.frame.PTS), formatDuration(evt.frame.Duration))
			a.setPreview(evt.frame.Image)
			closeFrameEvent(evt)
		default:
			a.syncWindowTitle()
			return nil
		}
	}
}

func (a *playerApp) Draw(screen *ebiten.Image) {
	width := screen.Bounds().Dx()
	height := screen.Bounds().Dy()
	sidebarWidth := clampInt(maxInt(width/4, 260), 260, minInt(defaultSidebarWidth, maxInt(width-320, 260)))

	screen.Fill(color.RGBA{R: 18, G: 20, B: 26, A: 255})

	sidebar := color.RGBA{R: 27, G: 31, B: 39, A: 255}
	panel := color.RGBA{R: 23, G: 26, B: 33, A: 255}
	highlight := color.RGBA{R: 52, G: 142, B: 255, A: 255}

	ebitenutil.DrawRect(screen, 0, 0, float64(sidebarWidth), float64(height), sidebar)
	ebitenutil.DrawRect(screen, float64(sidebarWidth), 0, float64(width-sidebarWidth), float64(height), panel)

	ebitenutil.DebugPrintAt(screen, "AV1 Native Player", padding, 14)
	ebitenutil.DebugPrintAt(screen, "Controls: Click/Up/Down select, Space play/stop, R restart, S stop, Esc quit", padding, 34)

	layout := selectionLayout(width, height)
	a.ensureSelectionVisible(layout.visibleRows)

	if a.scrollOffset > 0 {
		ebitenutil.DebugPrintAt(screen, "^ more", padding, layout.listTop-18)
	}
	for row := 0; row < layout.visibleRows; row++ {
		i := a.scrollOffset + row
		if i >= len(a.files) {
			break
		}
		y := layout.listTop + row*lineHeight
		name := a.files[i]
		if i == a.selected {
			ebitenutil.DrawRect(screen, 8, float64(y-12), float64(layout.sidebarWidth-16), 18, highlight)
			ebitenutil.DebugPrintAt(screen, name, padding, y-10)
		} else {
			ebitenutil.DebugPrintAt(screen, name, padding, y-10)
		}
	}
	if a.scrollOffset+layout.visibleRows < len(a.files) {
		ebitenutil.DebugPrintAt(screen, "v more", padding, layout.listBottom+4)
	}

	infoX := sidebarWidth + padding
	infoY := 32
	lines := []string{
		"File: " + a.info.Name,
		fmt.Sprintf("Dimensions: %d x %d", a.info.Width, a.info.Height),
		fmt.Sprintf("Playback: %s / %s", formatDuration(a.playbackPos), formatDuration(a.info.Duration)),
		fmt.Sprintf("Samples: %d, timescale %d", a.info.SampleCount, a.info.Timescale),
		fmt.Sprintf("Codec: %s, %d-bit, %s", a.info.Codec, a.info.BitDepth, a.info.Chroma),
		"Decoder: " + a.info.Decoder,
		"State: " + playbackStateLabel(a.playing),
		"Frame: " + a.frameText,
		"PTS: " + a.ptsText,
		fmt.Sprintf("Render: %.0f FPS / %.0f TPS", ebiten.ActualFPS(), ebiten.ActualTPS()),
		"Status: " + a.status,
	}
	ebitenutil.DebugPrintAt(screen, strings.Join(lines, "\n"), infoX, infoY-14)

	previewX := sidebarWidth + padding
	previewY := 220
	previewW := maxInt(1, width-sidebarWidth-padding*2)
	previewH := maxInt(1, height-previewY-padding)
	a.drawProgressBar(screen, previewX, previewY-24, previewW)
	ebitenutil.DrawRect(screen, float64(previewX), float64(previewY), float64(previewW), float64(previewH), color.RGBA{R: 14, G: 16, B: 20, A: 255})
	a.drawPreview(screen, previewX, previewY, previewW, previewH)
}

func (a *playerApp) Layout(outsideWidth, outsideHeight int) (int, int) {
	return maxInt(outsideWidth, minWindowWidth), maxInt(outsideHeight, minWindowHeight)
}

func (a *playerApp) moveSelection(delta int) {
	if len(a.files) == 0 {
		return
	}
	next := clampInt(a.selected+delta, 0, len(a.files)-1)
	if next == a.selected {
		return
	}
	if err := a.selectIndex(next); err != nil {
		a.setStatus(fmt.Sprintf("Select failed: %v", err))
	}
}

func (a *playerApp) selectIndex(index int) error {
	if index < 0 || index >= len(a.files) {
		return fmt.Errorf("invalid file index %d", index)
	}
	if index == a.selected {
		return nil
	}
	_ = a.state.stop()
	a.drainPendingEvents()
	a.playing = false
	return a.selectFile(index)
}

func (a *playerApp) selectFile(index int) error {
	if index < 0 || index >= len(a.files) {
		return fmt.Errorf("invalid file index %d", index)
	}
	info, err := inspectVideo(filepath.Join(a.videoDir, a.files[index]))
	if err != nil {
		return err
	}
	a.selected = index
	a.info = info
	a.frameText = "-"
	a.ptsText = "-"
	a.playbackPos = 0
	width := info.Width
	height := info.Height
	if width <= 0 {
		width = 960
	}
	if height <= 0 {
		height = 540
	}
	a.setPreview(placeholderImage(width, height))
	a.setStatus(fmt.Sprintf("Loaded metadata for %s", info.Name))
	return nil
}

func (a *playerApp) startSelected() error {
	if a.info.Path == "" {
		return fmt.Errorf("no file selected")
	}
	a.frameText = "-"
	a.ptsText = "-"
	a.playbackPos = 0
	a.drainPendingEvents()
	a.setStatus(fmt.Sprintf("Opening %s", a.info.Name))
	if err := a.state.play(a.info.Path, func(frame *decoder.Frame, frameCount int) {
		pushLatestFrame(a.frameCh, frameEvent{frame: frame, frameCount: frameCount})
	}, func(text string) {
		pushLatestStatus(a.statusCh, text)
	}); err != nil {
		a.playing = false
		return err
	}
	a.playing = true
	return nil
}

type selectionMetrics struct {
	sidebarWidth int
	listTop      int
	listBottom   int
	visibleRows  int
}

func selectionLayout(width, height int) selectionMetrics {
	sidebarWidth := clampInt(maxInt(width/4, 260), 260, minInt(defaultSidebarWidth, maxInt(width-320, 260)))
	listTop := 84
	listBottom := height - 220
	return selectionMetrics{
		sidebarWidth: sidebarWidth,
		listTop:      listTop,
		listBottom:   listBottom,
		visibleRows:  maxInt(1, (listBottom-listTop)/lineHeight),
	}
}

func (a *playerApp) fileIndexAt(x, y int, layout selectionMetrics) (int, bool) {
	if x < 8 || x >= layout.sidebarWidth-8 {
		return 0, false
	}
	for row := 0; row < layout.visibleRows; row++ {
		index := a.scrollOffset + row
		if index >= len(a.files) {
			break
		}
		rowY := layout.listTop + row*lineHeight
		if y >= rowY-12 && y < rowY+6 {
			return index, true
		}
	}
	return 0, false
}

func (a *playerApp) handleStatus(text string) {
	switch {
	case strings.HasPrefix(text, "Buffering playback."),
		strings.HasPrefix(text, "Playing decoded frames."):
		a.playing = true
	case text == "Playback finished.":
		a.playing = false
		if a.info.Duration > 0 {
			a.playbackPos = a.info.Duration
		}
	case playbackTerminalStatus(text):
		a.playing = false
	}
	a.setStatus(text)
}

func (a *playerApp) setStatus(text string) {
	a.status = text
	a.state.mu.Lock()
	a.state.status = text
	a.state.mu.Unlock()
}

func (a *playerApp) setPreview(img image.Image) {
	if img == nil {
		return
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return
	}
	if a.preview == nil || a.preview.Bounds().Dx() != width || a.preview.Bounds().Dy() != height {
		if a.preview != nil {
			a.preview.Deallocate()
		}
		a.preview = ebiten.NewImageWithOptions(image.Rect(0, 0, width, height), &ebiten.NewImageOptions{Unmanaged: true})
		a.previewScratch = image.NewRGBA(image.Rect(0, 0, width, height))
	}
	if rgba, ok := img.(*image.RGBA); ok && rgba.Bounds().Dx() == width && rgba.Bounds().Dy() == height && rgba.Stride == width*4 {
		a.preview.WritePixels(rgba.Pix)
		return
	}
	imagedraw.Draw(a.previewScratch, a.previewScratch.Bounds(), img, bounds.Min, imagedraw.Src)
	a.preview.WritePixels(a.previewScratch.Pix)
}

func (a *playerApp) closePreview() {
	if a.preview != nil {
		a.preview.Deallocate()
		a.preview = nil
	}
	a.previewScratch = nil
}

func (a *playerApp) drawPreview(screen *ebiten.Image, x, y, w, h int) {
	if a.preview == nil {
		return
	}
	iw, ih := a.preview.Bounds().Dx(), a.preview.Bounds().Dy()
	if iw == 0 || ih == 0 {
		return
	}
	scale := minFloat(float64(w)/float64(iw), float64(h)/float64(ih))
	op := &ebiten.DrawImageOptions{}
	if math.Abs(scale-math.Round(scale)) > 0.001 {
		op.Filter = ebiten.FilterLinear
	}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(
		float64(x)+(float64(w)-float64(iw)*scale)/2,
		float64(y)+(float64(h)-float64(ih)*scale)/2,
	)
	screen.DrawImage(a.preview, op)
}

func (a *playerApp) drawProgressBar(screen *ebiten.Image, x, y, width int) {
	const barHeight = 10
	if width <= 0 {
		return
	}
	ebitenutil.DrawRect(screen, float64(x), float64(y), float64(width), barHeight, color.RGBA{R: 35, G: 40, B: 50, A: 255})
	progress := playbackProgress(a.playbackPos, a.info.Duration)
	if progress <= 0 {
		return
	}
	ebitenutil.DrawRect(screen, float64(x), float64(y), float64(width)*progress, barHeight, color.RGBA{R: 52, G: 142, B: 255, A: 255})
}

func (a *playerApp) ensureSelectionVisible(visibleRows int) {
	if visibleRows <= 0 {
		visibleRows = 1
	}
	if a.selected < a.scrollOffset {
		a.scrollOffset = a.selected
	}
	if a.selected >= a.scrollOffset+visibleRows {
		a.scrollOffset = a.selected - visibleRows + 1
	}
	maxOffset := maxInt(len(a.files)-visibleRows, 0)
	a.scrollOffset = clampInt(a.scrollOffset, 0, maxOffset)
}

func (a *playerApp) drainPendingEvents() {
	for {
		select {
		case evt := <-a.frameCh:
			closeFrameEvent(evt)
		case <-a.statusCh:
		default:
			return
		}
	}
}

func (a *playerApp) windowTitle() string {
	name := a.info.Name
	if name == "" {
		name = "AV1 Native Player"
	}
	return fmt.Sprintf("%s | %s", name, a.status)
}

func (a *playerApp) syncWindowTitle() {
	title := a.windowTitle()
	if title == a.lastWindowTitle {
		return
	}
	ebiten.SetWindowTitle(title)
	a.lastWindowTitle = title
}

func playbackStateLabel(playing bool) string {
	if playing {
		return "playing"
	}
	return "stopped"
}

func playbackTerminalStatus(status string) bool {
	return status == "Playback finished." ||
		status == "Parser-only playback finished." ||
		status == "Playback stopped." ||
		strings.HasPrefix(status, "Decode error:")
}

func playbackProgress(position, total time.Duration) float64 {
	if total <= 0 || position <= 0 {
		return 0
	}
	progress := float64(position) / float64(total)
	if progress > 1 {
		return 1
	}
	return progress
}

func pushLatestFrame(ch chan frameEvent, evt frameEvent) {
	select {
	case ch <- evt:
	default:
		select {
		case dropped := <-ch:
			closeFrameEvent(dropped)
		default:
		}
		ch <- evt
	}
}

func closeFrameEvent(evt frameEvent) {
	if evt.frame == nil {
		return
	}
	_ = evt.frame.Close()
}

func pushLatestStatus(ch chan string, status string) {
	select {
	case ch <- status:
	default:
		select {
		case <-ch:
		default:
		}
		ch <- status
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
