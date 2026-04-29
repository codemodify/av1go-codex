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

func TestDebugMovieFirstFrameProblemBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("debug only")
	}
	if os.Getenv("DEBUG_MOVIE_BLOCKS") == "" {
		t.Skip("debug only")
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

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	targets := [][2]int{
		{0, 0}, {4, 0}, {8, 0}, {16, 0}, {32, 0}, {64, 0},
		{0, 4}, {4, 4}, {8, 4}, {16, 4}, {32, 4}, {64, 4},
		{0, 16}, {16, 16}, {32, 16}, {64, 16},
		{0, 32}, {16, 32}, {32, 32}, {64, 32},
	}
	if err := debugKeyframeProblemBlocks(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, segState, targets, t); err != nil {
		t.Fatal(err)
	}
}
