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

func TestDebugSPBTVPipelineStages(t *testing.T) {
	if os.Getenv("DEBUG_FIRST_KEY_BLOCKS") == "" {
		t.Skip("debug only")
	}
	ref, err := os.ReadFile("/tmp/spbtv_frame0_ref.yuv")
	if err != nil {
		t.Skipf("reference frame unavailable: %v", err)
	}

	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
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
	if os.Getenv("DEBUG_SPBTV_DISABLE_CDF_UPDATE") != "" {
		parsed.Header.DisableCDFUpdate = true
		t.Logf("forcing DisableCDFUpdate for spbtv debug run")
	}

	img := image.NewYCbCr(image.Rect(0, 0, parsed.Header.Width, parsed.Header.Height), image.YCbCrSubsampleRatio420)
	fillUint8(img.Cb, 128)
	fillUint8(img.Cr, 128)

	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	cdef := NewCDEFIndexMap(parsed.Header.Width, parsed.Header.Height)
	edges := newLoopFilterEdgeMaps(parsed.Header.Width, parsed.Header.Height, dec.header.ColorConfig.ChromaSubsampling())
	restoration := NewRestorationState(dec.header, &parsed.Header)
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	if err := decodeKeyframeMain8DCLumaWithCDEFOrder(dec.header, &parsed.Header, &parsed.TileGroup, dq, img, cdef, edges, restoration, segState, true); err != nil {
		t.Fatal(err)
	}

	logSPBTVFirstMismatch(t, "raw", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVFirstChromaMismatch(t, "raw", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVStagePoints(t, "raw", img)
	logSPBTVChromaStagePoints(t, "raw", img, ref, parsed.Header.Width, parsed.Header.Height)
	applySimpleLoopFilter420_8MaskedWithEdges(img, &parsed.Header, edges, cdef)
	logSPBTVFirstMismatch(t, "loop", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVFirstChromaMismatch(t, "loop", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVStagePoints(t, "loop", img)
	logSPBTVChromaStagePoints(t, "loop", img, ref, parsed.Header.Width, parsed.Header.Height)
	applyCDEF420_8(img, &parsed.Header, cdef)
	logSPBTVFirstMismatch(t, "cdef", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVFirstChromaMismatch(t, "cdef", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVStagePoints(t, "cdef", img)
	logSPBTVChromaStagePoints(t, "cdef", img, ref, parsed.Header.Width, parsed.Header.Height)
	applyRestoration420_8(img, &parsed.Header, restoration)
	logSPBTVFirstMismatch(t, "restoration", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVFirstChromaMismatch(t, "restoration", img, ref, parsed.Header.Width, parsed.Header.Height)
	logSPBTVStagePoints(t, "restoration", img)
	logSPBTVChromaStagePoints(t, "restoration", img, ref, parsed.Header.Width, parsed.Header.Height)
}

func logSPBTVStagePoints(t *testing.T, label string, img *image.YCbCr) {
	t.Helper()
	points := [][2]int{{478, 0}, {480, 0}, {496, 0}, {504, 0}, {512, 0}, {512, 8}, {640, 0}, {640, 8}}
	for _, pt := range points {
		x, y := pt[0], pt[1]
		t.Logf("%s (%d,%d)=%d", label, x, y, img.Y[y*img.YStride+x])
	}
}

func logSPBTVChromaStagePoints(t *testing.T, label string, img *image.YCbCr, ref []byte, width, height int) {
	t.Helper()
	cw := (width + 1) >> 1
	ch := (height + 1) >> 1
	ySize := width * height
	uOff := ySize
	vOff := ySize + cw*ch
	if len(ref) < vOff+cw*ch {
		return
	}
	points := [][2]int{{480, 0}, {496, 0}, {512, 0}, {512, 8}, {640, 0}, {640, 8}, {768, 0}, {944, 528}}
	for _, pt := range points {
		x, y := pt[0]>>1, pt[1]>>1
		if x < 0 || x >= cw || y < 0 || y >= ch {
			continue
		}
		gotU := img.Cb[y*img.CStride+x]
		gotV := img.Cr[y*img.CStride+x]
		refU := ref[uOff+y*cw+x]
		refV := ref[vOff+y*cw+x]
		t.Logf("%s uv@luma(%d,%d) got=(%d,%d) ref=(%d,%d)", label, pt[0], pt[1], gotU, gotV, refU, refV)
	}
}

func logSPBTVFirstMismatch(t *testing.T, label string, img *image.YCbCr, ref []byte, width, height int) {
	t.Helper()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			got := img.Y[y*img.YStride+x]
			want := ref[y*width+x]
			if got != want {
				t.Logf("%s first mismatch (%d,%d) ours=%d ref=%d", label, x, y, got, want)
				return
			}
		}
	}
	t.Logf("%s first mismatch none", label)
}

func logSPBTVFirstChromaMismatch(t *testing.T, label string, img *image.YCbCr, ref []byte, width, height int) {
	t.Helper()
	cw := (width + 1) >> 1
	ch := (height + 1) >> 1
	ySize := width * height
	uOff := ySize
	vOff := ySize + cw*ch
	if len(ref) < vOff+cw*ch {
		return
	}
	foundU := false
	for y := 0; y < ch && !foundU; y++ {
		for x := 0; x < cw; x++ {
			gotU := img.Cb[y*img.CStride+x]
			refU := ref[uOff+y*cw+x]
			if gotU != refU {
				t.Logf("%s first U mismatch chroma(%d,%d) luma(%d,%d) ours=%d ref=%d", label, x, y, x*2, y*2, gotU, refU)
				foundU = true
				break
			}
		}
	}
	if !foundU {
		t.Logf("%s first U mismatch none", label)
	}
	foundV := false
	for y := 0; y < ch && !foundV; y++ {
		for x := 0; x < cw; x++ {
			gotV := img.Cr[y*img.CStride+x]
			refV := ref[vOff+y*cw+x]
			if gotV != refV {
				t.Logf("%s first V mismatch chroma(%d,%d) luma(%d,%d) ours=%d ref=%d", label, x, y, x*2, y*2, gotV, refV)
				foundV = true
				break
			}
		}
	}
	if !foundV {
		t.Logf("%s first V mismatch none", label)
	}
}
