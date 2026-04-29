package main

import (
	"image"
	"sync"
	"time"

	player "github.com/codemodify/av1go-codex/internal/nativeplayer"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
)

type videoInfo = player.VideoInfo

type playerState struct {
	videoDir string

	mu     sync.Mutex
	status string
	inner  player.PlayerState
}

type frameEvent struct {
	frame      *decoder.Frame
	frameCount int
}

func listVideoFiles(dir string) ([]string, error) {
	return player.ListVideoFiles(dir)
}

func inspectVideo(path string) (videoInfo, error) {
	return player.InspectVideo(path)
}

func (s *playerState) play(path string, onFrame func(*decoder.Frame, int), onStatus func(string)) error {
	return s.inner.Play(path, onFrame, onStatus)
}

func (s *playerState) hasSession() bool {
	return s.inner.HasSession()
}

func (s *playerState) stop() error {
	return s.inner.Stop()
}

func formatDuration(d time.Duration) string {
	return player.FormatDuration(d)
}

func placeholderImage(width, height int) image.Image {
	return player.PlaceholderImage(width, height)
}

func parsedFrameToPlaybackFrame(meta decoder.Metadata, parsed *decoder.ParsedFrame) *decoder.Frame {
	return player.ParsedFrameToPlaybackFrame(meta, parsed)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
