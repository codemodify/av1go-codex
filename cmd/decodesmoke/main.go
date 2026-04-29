package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/codemodify/av1go-codex/internal/nativeplayer"
	"github.com/codemodify/av1go-codex/pkg/av1/decoder"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "decodesmoke: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("decodesmoke", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		dir       = fs.String("dir", "testvideo", "directory containing AV1 MP4 files")
		input     = fs.String("input", "", "single MP4 file to decode instead of scanning -dir")
		limit     = fs.Int("limit", 0, "maximum shown frames per file; 0 decodes until EOF")
		display   = fs.Bool("display", true, "also run the native player display conversion")
		failFast  = fs.Bool("fail-fast", true, "stop at the first decode failure")
		nameMatch = fs.String("match", "", "optional filename substring filter")
		progress  = fs.Int("progress", 0, "print progress every N decoded frames; 0 disables progress")
		cpuProf   = fs.String("cpuprofile", "", "write a Go CPU profile while decoding")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}

	files, err := smokeFiles(*input, *dir, *nameMatch)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no MP4 files matched")
	}
	stopProfile, err := startCPUProfile(*cpuProf)
	if err != nil {
		return err
	}
	defer stopProfile()

	failures := 0
	for _, path := range files {
		start := time.Now()
		info, err := nativeplayer.InspectVideo(path)
		if err != nil {
			failures++
			fmt.Fprintf(stdout, "FAIL inspect %s: %v\n", path, err)
			if *failFast {
				return err
			}
			continue
		}
		frames, err := decodeFile(path, *limit, *display, *progress, stdout)
		elapsed := time.Since(start).Truncate(time.Millisecond)
		if err != nil {
			failures++
			fmt.Fprintf(stdout, "FAIL decode %s frames=%d size=%dx%d samples=%d codec=%s elapsed=%s: %v\n",
				path, frames, info.Width, info.Height, info.SampleCount, info.Codec, elapsed, err)
			if *failFast {
				return err
			}
			continue
		}
		fmt.Fprintf(stdout, "PASS %s frames=%d size=%dx%d samples=%d codec=%s elapsed=%s\n",
			path, frames, info.Width, info.Height, info.SampleCount, info.Codec, elapsed)
	}
	if failures > 0 {
		return fmt.Errorf("%d file(s) failed", failures)
	}
	return nil
}

func startCPUProfile(path string) (func(), error) {
	if strings.TrimSpace(path) == "" {
		return func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}, nil
}

func smokeFiles(input, dir, nameMatch string) ([]string, error) {
	if strings.TrimSpace(input) != "" {
		return []string{input}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".mp4") {
			continue
		}
		if nameMatch != "" && !strings.Contains(entry.Name(), nameMatch) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func decodeFile(path string, limit int, display bool, progressEvery int, stdout io.Writer) (int, error) {
	dec, err := decoder.OpenMP4(path)
	if err != nil {
		return 0, err
	}
	defer dec.Close()

	frames := 0
	for limit == 0 || frames < limit {
		frame, err := dec.NextFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return frames, nil
			}
			return frames, err
		}
		if display {
			prepared := nativeplayer.PrepareFrameForDisplay(frame)
			if prepared == nil || prepared.Image == nil {
				_ = frame.Close()
				if prepared != nil {
					_ = prepared.Close()
				}
				return frames, fmt.Errorf("frame %d produced no display image", frames)
			}
			if err := prepared.Close(); err != nil {
				_ = frame.Close()
				return frames, err
			}
		}
		if err := frame.Close(); err != nil {
			return frames, err
		}
		frames++
		if progressEvery > 0 && frames%progressEvery == 0 {
			fmt.Fprintf(stdout, "PROGRESS %s frames=%d\n", path, frames)
		}
	}
	return frames, nil
}
