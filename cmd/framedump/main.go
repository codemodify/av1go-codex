package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/internal/nativeplayer"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "framedump: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("framedump", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		input  = fs.String("input", "", "input mp4 path, sample filename, or directory")
		frames = fs.String("frames", "0,1,10,30", "comma-separated shown-frame indices to dump")
		outDir = fs.String("out", "/tmp/av1go-frames", "output directory for png files")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	target, err := nativeplayer.ResolvePlaybackTarget(*input, []string{"testvideo", "."})
	if err != nil {
		return err
	}
	frameList, err := parseFrameList(*frames)
	if err != nil {
		return err
	}
	if len(frameList) == 0 {
		return errors.New("no frame indices requested")
	}

	absOutDir, err := filepath.Abs(*outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absOutDir, 0o755); err != nil {
		return err
	}

	dec, err := decoder.OpenMP4(target.Path)
	if err != nil {
		return err
	}
	defer dec.Close()

	baseName := strings.TrimSuffix(filepath.Base(target.Path), filepath.Ext(target.Path))
	fmt.Fprintf(stdout, "input: %s\n", target.Path)
	fmt.Fprintf(stdout, "output: %s\n", absOutDir)

	nextTarget := 0
	for shown := 0; nextTarget < len(frameList); shown++ {
		frame, err := dec.NextFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("reached EOF after shown frame %d while waiting for frame %d", shown-1, frameList[nextTarget])
			}
			return err
		}

		if shown != frameList[nextTarget] {
			if closeErr := frame.Close(); closeErr != nil {
				return closeErr
			}
			continue
		}

		prepared := nativeplayer.PrepareFrameForDisplay(frame)
		if closeErr := frame.Close(); closeErr != nil {
			if prepared != nil {
				_ = prepared.Close()
			}
			return closeErr
		}
		if prepared == nil || prepared.Image == nil {
			if prepared != nil {
				_ = prepared.Close()
			}
			return fmt.Errorf("frame %d had no display image", shown)
		}

		outPath := filepath.Join(absOutDir, fmt.Sprintf("%s-frame-%04d.png", baseName, shown))
		if err := writePNG(outPath, prepared.Image); err != nil {
			_ = prepared.Close()
			return err
		}
		if closeErr := prepared.Close(); closeErr != nil {
			return closeErr
		}

		fmt.Fprintf(stdout, "wrote frame %d -> %s\n", shown, outPath)
		nextTarget++
	}

	return nil
}

func parseFrameList(spec string) ([]int, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("empty frame list")
	}
	parts := strings.Split(spec, ",")
	frames := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid frame list %q", spec)
		}
		v, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("parse frame index %q: %w", part, err)
		}
		if v < 0 {
			return nil, fmt.Errorf("frame index %d must be non-negative", v)
		}
		if _, ok := seen[v]; ok {
			continue
		}
		if len(frames) > 0 && v < frames[len(frames)-1] {
			return nil, fmt.Errorf("frame list must be non-decreasing: %q", spec)
		}
		seen[v] = struct{}{}
		frames = append(frames, v)
	}
	return frames, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
