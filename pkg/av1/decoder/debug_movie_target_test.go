//go:build av1debug

package decoder

import (
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugMovieTargetBlock900x0(t *testing.T) {
	if testing.Short() {
		t.Skip("debug only")
	}
	if os.Getenv("DEBUG_MOVIE_TARGET") == "" {
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
	targets := [][2]int{{836, 32}}
	if spec := strings.TrimSpace(os.Getenv("DEBUG_MOVIE_TARGET_BLOCK")); spec != "" {
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

func parseDebugMovieTargets(spec string) ([][2]int, error) {
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	targets := make([][2]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		parts := strings.Split(field, ",")
		if len(parts) != 2 {
			return nil, io.ErrUnexpectedEOF
		}
		x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, err
		}
		y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, err
		}
		targets = append(targets, [2]int{x, y})
	}
	if len(targets) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return targets, nil
}
