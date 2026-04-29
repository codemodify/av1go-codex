//go:build av1debug

package decoder

import (
	"image"
	"io"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugSPBTVFirstBadRenderedBlocks(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}

	dec, err := OpenMP4(testutil.SamplePath(t, "spbtv_sample_bipbop_av1_960x540_25fps.mp4"))
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
		{478, 0}, {479, 4}, {480, 0},
		{496, 0}, {512, 0}, {528, 0}, {544, 0},
		{560, 0}, {576, 0}, {592, 0}, {608, 0},
		{624, 0}, {640, 0}, {656, 0}, {672, 0},
	}
	if err := debugKeyframeProblemBlocks(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, segState, targets, t); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DEBUG_SPBTV_TRACE_TARGET_WRITES") != "" {
		for _, pt := range targets {
			if pt[0] >= 0 && pt[0] < img.Rect.Dx() && pt[1] >= 0 && pt[1] < img.Rect.Dy() {
				t.Logf("target-final target=(%d,%d) y=%d", pt[0], pt[1], img.Y[pt[1]*img.YStride+pt[0]])
			}
		}
	}
}
