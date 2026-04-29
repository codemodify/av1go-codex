//go:build av1debug

package decoder

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestDebugBBBParsedFrameMetricsSweep(t *testing.T) {
	if os.Getenv("DEBUG_BBB_PARSED_FRAME_SWEEP") == "" {
		t.Skip("debug only")
	}

	path := strings.TrimSpace(os.Getenv("DEBUG_BBB_REFERENCE_PATH"))
	if path == "" {
		path = "../../../testvideo/BigBuckBunny-AV1.mp4"
	}
	limit := debugEnvInt("DEBUG_BBB_PARSED_FRAME_SWEEP_LIMIT", 12)
	refPath := strings.TrimSpace(os.Getenv("DEBUG_BBB_REF_YUV"))
	if refPath == "" {
		refPath = "/tmp/bbb_dav1d_all12.yuv"
	}

	refs := readDebugRaw420Frames(t, refPath, 854, 480, limit)
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for i, ref := range refs {
		parsed, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatalf("NextParsedFrame(%d): EOF", i)
		}
		if err != nil {
			t.Fatalf("NextParsedFrame(%d): %v", i, err)
		}
		frame, err := dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame(%d): %v", i, err)
		}
		metrics, _ := compareFrameToReference(frame, ref)
		worstX, worstY, worstRMSE := debugWorstYBlock(frame, ref, 16)
		firstThreshold := debugEnvInt("DEBUG_BBB_FIRST_Y_THRESHOLD", 24)
		firstX, firstY, firstGot, firstWant, firstOK := debugFirstYMismatchOver(frame, ref, firstThreshold)
		if os.Getenv("DEBUG_BBB_PLANE_DETAIL") != "" {
			for _, plane := range []string{"U", "V"} {
				px, py, got, want, ok := debugFirstChromaMismatchOverAny(frame, ref, plane, firstThreshold)
				if ok {
					t.Logf("parsed=%02d first%s>%d=(%d,%d) got=%d ref=%d", i, plane, firstThreshold, px, py, got, want)
				}
			}
		}
		if firstOK {
			t.Logf("parsed=%02d show=%v offset=%d q=%d y=%.4f/%d u=%.4f/%d v=%.4f/%d worstY=(%d,%d) %.2f firstY>%d=(%d,%d) got=%d ref=%d",
				i, parsed.Header.ShowFrame, parsed.Header.FrameOffset, parsed.Header.Quantization.YAC,
				metrics.Y.RMSE, metrics.Y.MaxAbs,
				metrics.U.RMSE, metrics.U.MaxAbs,
				metrics.V.RMSE, metrics.V.MaxAbs,
				worstX, worstY, worstRMSE,
				firstThreshold,
				firstX, firstY, firstGot, firstWant,
			)
		} else {
			t.Logf("parsed=%02d show=%v offset=%d q=%d y=%.4f/%d u=%.4f/%d v=%.4f/%d worstY=(%d,%d) %.2f firstY>%d=none",
				i, parsed.Header.ShowFrame, parsed.Header.FrameOffset, parsed.Header.Quantization.YAC,
				metrics.Y.RMSE, metrics.Y.MaxAbs,
				metrics.U.RMSE, metrics.U.MaxAbs,
				metrics.V.RMSE, metrics.V.MaxAbs,
				worstX, worstY, worstRMSE,
				firstThreshold,
			)
		}
		_ = frame.Close()
	}
}

func TestDebugBBBMCCheck(t *testing.T) {
	if os.Getenv("DEBUG_BBB_MC_CHECK") == "" {
		t.Skip("debug only")
	}

	path := strings.TrimSpace(os.Getenv("DEBUG_BBB_REFERENCE_PATH"))
	if path == "" {
		path = "../../../testvideo/BigBuckBunny-AV1.mp4"
	}
	refPath := strings.TrimSpace(os.Getenv("DEBUG_BBB_REF_YUV"))
	if refPath == "" {
		refPath = "/tmp/bbb_dav1d_all200.yuv"
	}
	refs := readDebugRaw420Frames(t, refPath, 854, 480, 70)
	if len(refs) <= 68 {
		t.Fatalf("reference frames=%d, need parsed frame 68", len(refs))
	}

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()
	var frame *Frame
	for i := 0; i <= 68; i++ {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame(%d): %v", i, err)
		}
		frame, err = dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame(%d): %v", i, err)
		}
		_ = frame.Close()
	}

	blockX, blockY := 768, 224
	mv := MotionVector{Y: 0, X: 10}
	filter := InterFilterPair{interFilterRegular, interFilterSharp}
	davPred := make([]byte, 16)
	predictInterLumaBlock8WithFiltersAt(davPred, 4, 4, 4, 0, 0, 4, 4, blockX, blockY, refs[68].Y, refs[68].Width, refs[68].Width, refs[68].Height, mv, filter)
	t.Logf("dav-ref mc rows=%v", debugTraceRows8(davPred, 4, 4, 4))
	t.Logf("dav-ref src rows=%v", debugTraceRows8(refs[68].Y[blockY*refs[68].Width+blockX-2:], refs[68].Width, 10, 4))

	slot := 5
	if slot >= len(dec.pureGoRefs) || dec.pureGoRefs[slot] == nil {
		t.Fatalf("missing purego slot %d", slot)
	}
	goRef := dec.pureGoRefs[slot]
	goPred := make([]byte, 16)
	predictInterLumaBlock8WithFiltersAt(goPred, 4, 4, 4, 0, 0, 4, 4, blockX, blockY, goRef.Y, goRef.YStride, goRef.Width, goRef.Height, mv, filter)
	t.Logf("go-slot%d mc rows=%v", slot, debugTraceRows8(goPred, 4, 4, 4))
	t.Logf("go-slot%d src rows=%v", slot, debugTraceRows8(goRef.Y[blockY*goRef.YStride+blockX-2:], goRef.YStride, 10, 4))
}

func TestDebugBBBGlobalWarpCheck(t *testing.T) {
	if os.Getenv("DEBUG_BBB_GLOBAL_WARP_CHECK") == "" {
		t.Skip("debug only")
	}

	path := strings.TrimSpace(os.Getenv("DEBUG_BBB_REFERENCE_PATH"))
	if path == "" {
		path = "../../../testvideo/BigBuckBunny-AV1.mp4"
	}
	refPath := strings.TrimSpace(os.Getenv("DEBUG_BBB_REF_YUV"))
	if refPath == "" {
		refPath = "/tmp/bbb_dav1d_all200.yuv"
	}
	refs := readDebugRaw420Frames(t, refPath, 854, 480, 188)
	if len(refs) <= 187 {
		t.Fatalf("reference frames=%d, need parsed frame 187", len(refs))
	}

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	slotParsed := [8]int{-1, -1, -1, -1, -1, -1, -1, -1}
	for i := 0; i <= 186; i++ {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame(%d): %v", i, err)
		}
		frame, err := dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame(%d): %v", i, err)
		}
		_ = frame.Close()
		for slot := range slotParsed {
			if parsed.Header.RefreshFrameFlags&(1<<slot) != 0 {
				slotParsed[slot] = i
			}
		}
	}

	parsed, err := dec.NextParsedFrame()
	if err != nil {
		t.Fatalf("NextParsedFrame(187): %v", err)
	}
	const (
		blockX = 160
		blockY = 64
	)
	ref0 := int8(debugEnvInt("DEBUG_BBB_GLOBAL_REF", 3))
	slot := -1
	if ref0 >= 0 && int(ref0) < len(parsed.Header.RefIdx) {
		slot = int(parsed.Header.RefIdx[ref0])
	}
	if slot < 0 || slot >= len(dec.pureGoRefs) || dec.pureGoRefs[slot] == nil {
		t.Fatalf("missing slot for ref0=%d slot=%d refs=%v", ref0, slot, parsed.Header.RefIdx)
	}
	g := BlockGeometry{Start4X: blockX / 4, Start4Y: blockY / 4, End4X: blockX/4 + 2, End4Y: blockY/4 + 2, Size: Block8x8}
	mv, warp, ok := singleRefGlobalMotionForPlane(&parsed.Header, ref0, 0, dec.header.ColorConfig.ChromaSubsampling(), g)
	if !ok {
		t.Fatalf("singleRefGlobalMotionForPlane ref0=%d returned !ok gmv=%+v", ref0, parsed.Header.GMV[ref0])
	}
	t.Logf("parsed187 off=%d refs=%v ref0=%d slot=%d slotParsed=%v refPOC=%v purePOC=%v gmv=%+v mv=%+v fixed=%v matrix=%v abcd=%v",
		parsed.Header.FrameOffset, parsed.Header.RefIdx, ref0, slot, slotParsed, parsed.RefFramePOC, dec.pureGoRefPOC,
		parsed.Header.GMV[ref0], mv, warp.fixed, warp.matrix, warp.abcd)

	goRef := dec.pureGoRefs[slot]
	goPred := make([]byte, 8*8)
	if err := predictInterPlane8WarpAt(goPred, 8, 8, 8, 0, 0, 8, 8, blockX, blockY, goRef.Y, goRef.YStride, goRef.Width, goRef.Height, warp); err != nil {
		t.Fatalf("go slot warp: %v", err)
	}
	t.Logf("go-slot%d warp rows=%v target=%d", slot, debugTraceRows8(goPred, 8, 8, 8), goPred[2*8])
	t.Logf("go-slot%d src rows=%v", slot, debugTraceRows8(goRef.Y[blockY*goRef.YStride+blockX:], goRef.YStride, 8, 8))
	t.Logf("go-slot%d warp-src rows=%v", slot, debugTraceRows8(goRef.Y[67*goRef.YStride+162:], goRef.YStride, 8, 8))

	refParsed := slotParsed[slot]
	if refParsed < 0 || refParsed >= len(refs) {
		t.Fatalf("slot %d has invalid parsed source %d", slot, refParsed)
	}
	davPred := make([]byte, 8*8)
	if err := predictInterPlane8WarpAt(davPred, 8, 8, 8, 0, 0, 8, 8, blockX, blockY, refs[refParsed].Y, refs[refParsed].Width, refs[refParsed].Width, refs[refParsed].Height, warp); err != nil {
		t.Fatalf("dav ref warp: %v", err)
	}
	t.Logf("dav-parsed%d warp rows=%v target=%d", refParsed, debugTraceRows8(davPred, 8, 8, 8), davPred[2*8])
	t.Logf("dav-parsed%d src rows=%v", refParsed, debugTraceRows8(refs[refParsed].Y[blockY*refs[refParsed].Width+blockX:], refs[refParsed].Width, 8, 8))
	t.Logf("dav-parsed%d warp-src rows=%v", refParsed, debugTraceRows8(refs[refParsed].Y[67*refs[refParsed].Width+162:], refs[refParsed].Width, 8, 8))
}
