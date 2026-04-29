//go:build av1debug

package decoder

import (
	"image"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugSPBTVTargetBlock(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}

	dec, err := OpenMP4(debugSPBTVPath())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	targetParsed := debugEnvInt("DEBUG_SPBTV_TARGET_PARSED", -1)
	var parsed *ParsedFrame
	for i := 0; ; i++ {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if targetParsed >= 0 {
			if i != targetParsed {
				continue
			}
			parsed = pf
			break
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

	targets := [][2]int{{48, 8}}
	if spec := strings.TrimSpace(os.Getenv("DEBUG_SPBTV_TARGET_BLOCK")); spec != "" {
		parsedTargets, err := parseDebugMovieTargets(spec)
		if err != nil {
			t.Fatal(err)
		}
		targets = parsedTargets
	}

	if err := debugKeyframeProblemBlocks(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, segState, targets, t); err != nil {
		t.Fatal(err)
	}
}
