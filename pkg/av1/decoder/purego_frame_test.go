package decoder

import (
	"image"
	"testing"
	"time"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDecodePureGoFrameFallsBackToPreviousFrameForInter(t *testing.T) {
	last := newPureGoTestFrame(8, 8, 17)
	d := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		},
		lastPureGoFrame: last,
	}

	hdr := obu.FrameHeader{
		FrameType: obu.FrameTypeInter,
		ShowFrame: true,
		Width:     8,
		Height:    8,
	}
	for i := range hdr.RefIdx {
		hdr.RefIdx[i] = -1
	}

	frame, err := d.decodePureGoFrame(&ParsedFrame{
		PTS:      40 * time.Millisecond,
		Duration: 40 * time.Millisecond,
		Header:   hdr,
	})
	if err != nil {
		t.Fatalf("decodePureGoFrame: %v", err)
	}
	if frame.PTS != 40*time.Millisecond {
		t.Fatalf("PTS = %s, want 40ms", frame.PTS)
	}
	if len(frame.Y) != len(last.Y) || frame.Y[7] != last.Y[7] {
		t.Fatal("expected cloned luma plane")
	}
	if &frame.Y[0] == &d.lastPureGoFrame.Y[0] {
		t.Fatal("expected cloned frame buffers")
	}
}

func TestDecodePureGoFrameShowExistingUsesReferenceSlot(t *testing.T) {
	d := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		},
		lastPureGoFrame: newPureGoTestFrame(8, 8, 11),
	}
	d.pureGoRefs[3] = newPureGoTestFrame(8, 8, 73)
	d.pureGoMVRefs[3] = NewFrameMVField(8, 8)
	d.pureGoMVRefs[3].Blocks[0] = SpatialMVBlock{Valid: true, MV: [2]MotionVector{{X: 4, Y: -2}, {}}}
	d.pureGoSegRefs[3] = NewSegmentationMap(8, 8)
	d.pureGoSegRefs[3].Data[0] = 5

	frame, err := d.decodePureGoFrame(&ParsedFrame{
		PTS:      80 * time.Millisecond,
		Duration: 40 * time.Millisecond,
		Header: obu.FrameHeader{
			ShowExistingFrame: true,
			ExistingFrameIdx:  3,
		},
	})
	if err != nil {
		t.Fatalf("decodePureGoFrame: %v", err)
	}
	if got := frame.Y[0]; got != 73 {
		t.Fatalf("frame.Y[0] = %d, want 73", got)
	}
	if frame.PTS != 80*time.Millisecond {
		t.Fatalf("PTS = %s, want 80ms", frame.PTS)
	}
	if &frame.Y[0] == &d.pureGoRefs[3].Y[0] {
		t.Fatal("expected returned frame buffers to stay cloned from stored reference")
	}
	if d.lastPureGoFrame != d.pureGoRefs[3] {
		t.Fatal("expected last pure-Go frame cache to reuse stored reference snapshot")
	}
	if d.lastPureGoMVField != d.pureGoMVRefs[3] {
		t.Fatal("expected last MV field cache to reuse stored reference snapshot")
	}
	if d.currentSegField != d.pureGoSegRefs[3] {
		t.Fatal("expected current segmentation map to reuse stored reference snapshot")
	}
}

func TestDecodePureGoFrameInterUsesPrimaryRefAndRefreshesSlot(t *testing.T) {
	d := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		},
		lastPureGoFrame: newPureGoTestFrame(8, 8, 9),
	}
	d.pureGoRefs[1] = newPureGoTestFrame(8, 8, 21)
	d.pureGoRefs[2] = newPureGoTestFrame(8, 8, 55)

	hdr := obu.FrameHeader{
		FrameType:         obu.FrameTypeInter,
		ShowFrame:         true,
		Width:             8,
		Height:            8,
		PrimaryRefFrame:   0,
		RefreshFrameFlags: 1 << 5,
	}
	for i := range hdr.RefIdx {
		hdr.RefIdx[i] = -1
	}
	hdr.RefIdx[0] = 2
	hdr.RefIdx[1] = 1

	frame, err := d.decodePureGoFrame(&ParsedFrame{
		PTS:      120 * time.Millisecond,
		Duration: 40 * time.Millisecond,
		Header:   hdr,
	})
	if err != nil {
		t.Fatalf("decodePureGoFrame: %v", err)
	}
	if got := frame.Y[0]; got != 55 {
		t.Fatalf("frame.Y[0] = %d, want 55", got)
	}
	if d.pureGoRefs[5] == nil {
		t.Fatal("expected refresh slot 5 to be populated")
	}
	if got := d.pureGoRefs[5].Y[0]; got != 55 {
		t.Fatalf("refresh slot Y[0] = %d, want 55", got)
	}
	if &d.pureGoRefs[5].Y[0] == &d.pureGoRefs[2].Y[0] {
		t.Fatal("expected refresh slot to clone frame buffers")
	}
}

func TestApplyPureGoRefreshSharesSingleSnapshotAcrossSlots(t *testing.T) {
	d := &Decoder{}
	frame := newPureGoTestFrame(8, 8, 41)
	field := NewFrameMVField(8, 8)
	field.Blocks[0] = SpatialMVBlock{Valid: true, MV: [2]MotionVector{{X: 3, Y: 7}, {}}}
	seg := NewSegmentationMap(8, 8)
	seg.Data[0] = 6

	d.snapshotPureGoState((1<<1)|(1<<4), frame, field, seg, 19, nil)

	if d.lastPureGoFrame == nil || d.lastPureGoMVField == nil {
		t.Fatal("expected last-frame caches to be populated")
	}
	if d.pureGoRefs[1] != d.pureGoRefs[4] {
		t.Fatal("expected refreshed frame slots to reuse one immutable snapshot")
	}
	if d.pureGoMVRefs[1] != d.pureGoMVRefs[4] {
		t.Fatal("expected refreshed MV slots to reuse one immutable snapshot")
	}
	if d.pureGoSegRefs[1] != d.pureGoSegRefs[4] {
		t.Fatal("expected refreshed segmentation slots to reuse one immutable snapshot")
	}
	if &d.pureGoRefs[1].Y[0] == &frame.Y[0] {
		t.Fatal("expected refreshed frame snapshot to stay isolated from decode output")
	}
	if d.pureGoMVRefs[1] != field || d.lastPureGoMVField != field {
		t.Fatal("expected refreshed MV slots to reuse the immutable current MV field")
	}
	if d.pureGoSegRefs[1] != seg {
		t.Fatal("expected refreshed segmentation slots to reuse the immutable segmentation map")
	}
}

func TestDecoderCloseReleasesPureGoState(t *testing.T) {
	d := &Decoder{}
	frame := cloneFrameForReference(newPureGoTestFrame(8, 8, 17))
	field := NewFrameMVField(8, 8)
	seg := NewSegmentationMap(8, 8)
	d.lastPureGoFrame = frame
	d.pureGoRefs[2] = frame
	d.currentMVField = field
	d.lastPureGoMVField = field
	d.pureGoMVRefs[2] = field
	d.currentSegField = seg
	d.pureGoSegRefs[2] = seg

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if d.lastPureGoFrame != nil || d.pureGoRefs[2] != nil {
		t.Fatal("expected pure-Go frame refs to be cleared on close")
	}
	if d.currentMVField != nil || d.lastPureGoMVField != nil || d.pureGoMVRefs[2] != nil {
		t.Fatal("expected pure-Go MV refs to be cleared on close")
	}
	if d.currentSegField != nil || d.pureGoSegRefs[2] != nil {
		t.Fatal("expected pure-Go segmentation refs to be cleared on close")
	}
}

func TestResolvePureGoBaseReferenceFallsBackToBlank8Bit(t *testing.T) {
	d := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		},
	}
	hdr := &obu.FrameHeader{Width: 8, Height: 8}

	frame := d.resolvePureGoBaseReference(hdr)
	if frame == nil {
		t.Fatal("expected fallback frame")
	}
	if frame.Width != 8 || frame.Height != 8 {
		t.Fatalf("fallback dimensions = %dx%d, want 8x8", frame.Width, frame.Height)
	}
	if len(frame.Y) != 64 {
		t.Fatalf("fallback luma len = %d, want 64", len(frame.Y))
	}
	for i, v := range frame.U {
		if v != 128 {
			t.Fatalf("fallback U[%d] = %d, want 128", i, v)
		}
	}
}

func TestResolvePureGoBaseReferenceFallsBackToBlank10Bit(t *testing.T) {
	d := &Decoder{
		header: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 10, SubsamplingX: true, SubsamplingY: true},
		},
	}
	hdr := &obu.FrameHeader{Width: 8, Height: 8}

	frame := d.resolvePureGoBaseReference(hdr)
	if frame == nil {
		t.Fatal("expected fallback frame")
	}
	if len(frame.Y16) != 64 {
		t.Fatalf("fallback luma len = %d, want 64", len(frame.Y16))
	}
	for i, v := range frame.U16 {
		if v != 512 {
			t.Fatalf("fallback U16[%d] = %d, want 512", i, v)
		}
	}
}

func newPureGoTestFrame(width, height int, y byte) *Frame {
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	for i := range img.Y {
		img.Y[i] = y
	}
	for i := range img.Cb {
		img.Cb[i] = 128
		img.Cr[i] = 128
	}
	return &Frame{
		Image:    img,
		Width:    width,
		Height:   height,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Y:        append([]byte(nil), img.Y...),
		U:        append([]byte(nil), img.Cb...),
		V:        append([]byte(nil), img.Cr...),
		YStride:  img.YStride,
		UStride:  img.CStride,
		VStride:  img.CStride,
	}
}
