//go:build av1debug

package decoder

import (
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugMovieProblemBlockReplay(t *testing.T) {
	if os.Getenv("DEBUG_MOVIE_BLOCK_REPLAY") == "" {
		t.Skip("debug only")
	}

	restoreRef, err := swapDebugReference("/tmp/movie_ref.yuv", "/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := restoreRef(); err != nil {
			t.Fatal(err)
		}
	}()

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

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	targets := [][2]int{
		{984, 0},
		{952, 72},
		{932, 64},
	}
	if spec := os.Getenv("DEBUG_MOVIE_BLOCK_REPLAY_TARGETS"); spec != "" {
		targets, err = parseDebugMovieTargets(spec)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := debugKeyframeProblemBlocks(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, segState, targets, t); err != nil {
		t.Fatal(err)
	}
}

func swapDebugReference(srcPath, dstPath string) (func() error, error) {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}

	orig, origErr := os.ReadFile(dstPath)
	origExists := origErr == nil
	if origErr != nil && !os.IsNotExist(origErr) {
		return nil, origErr
	}

	if err := os.WriteFile(dstPath, src, 0o644); err != nil {
		return nil, err
	}

	return func() error {
		if origExists {
			return os.WriteFile(dstPath, orig, 0o644)
		}
		if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}, nil
}
