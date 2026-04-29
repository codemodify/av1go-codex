package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedSegEntropy struct {
	bools   []uint32
	symbols []uint32
}

func (s *scriptedSegEntropy) DecodeBool(uint32) uint32 { return 0 }
func (s *scriptedSegEntropy) DecodeBoolEqui() uint32   { return 0 }
func (s *scriptedSegEntropy) DecodeUniform(uint32) int { return 0 }
func (s *scriptedSegEntropy) DecodeBools(uint) uint32  { return 0 }

func (s *scriptedSegEntropy) DecodeBoolAdapt([]uint16) uint32 {
	if len(s.bools) == 0 {
		return 0
	}
	v := s.bools[0]
	s.bools = s.bools[1:]
	return v
}

func (s *scriptedSegEntropy) DecodeSymbolAdapt([]uint16, int) uint32 {
	if len(s.symbols) == 0 {
		return 0
	}
	v := s.symbols[0]
	s.symbols = s.symbols[1:]
	return v
}

func TestNegDeinterleave(t *testing.T) {
	if got := negDeinterleave(0, 3, 8); got != 3 {
		t.Fatalf("diff=0 got %d want 3", got)
	}
	if got := negDeinterleave(1, 3, 8); got != 4 {
		t.Fatalf("diff=1 got %d want 4", got)
	}
	if got := negDeinterleave(2, 3, 8); got != 2 {
		t.Fatalf("diff=2 got %d want 2", got)
	}
}

func TestSegmentationMapUsesAV1MiGridPadding(t *testing.T) {
	m := NewSegmentationMap(960, 540)
	if got, want := m.Width4, 240; got != want {
		t.Fatalf("Width4=%d, want %d", got, want)
	}
	if got, want := m.Height4, 136; got != want {
		t.Fatalf("Height4=%d, want padded AV1 mi-grid height %d", got, want)
	}
	g := BlockGeometry{Start4X: 238, Start4Y: 134, End4X: 240, End4Y: 136}
	m.FillBlock(g, 5)
	if got := m.At(239, 135); got != 5 {
		t.Fatalf("bottom padded segment=%d, want 5", got)
	}
	if got := m.At(239, 136); got != 0 {
		t.Fatalf("segment past padded mi-grid=%d, want 0", got)
	}

	scratch := takeSegmentationMap(960, 540)
	t.Cleanup(func() { putSegmentationMap(scratch) })
	if got, want := scratch.Height4, 136; got != want {
		t.Fatalf("scratch Height4=%d, want %d", got, want)
	}
}

func TestReadBlockSegmentIDPreskipTemporalUsesPrevMap(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.Enabled = true
	hdr.Segmentation.UpdateMap = true
	hdr.Segmentation.Temporal = true
	hdr.Segmentation.Preskip = true
	hdr.Segmentation.LastActiveSegID = 3

	prev := NewSegmentationMap(32, 32)
	g := BlockGeometry{Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8}
	prev.FillBlock(g, 3)
	state := NewSegmentationState(32, 32, prev, false)
	above := &BlockContext{}
	left := &BlockContext{}
	dec := &scriptedSegEntropy{bools: []uint32{1}}

	segID, segPred, err := readBlockSegmentID(hdr, NewDefaultModeCDF(), above, left, state, g, true, true, false, false, dec)
	if err != nil {
		t.Fatalf("readBlockSegmentID: %v", err)
	}
	if !segPred {
		t.Fatal("expected temporal predicted segment")
	}
	if segID != 3 {
		t.Fatalf("segID got %d want 3", segID)
	}
	if got := state.Current.MinBlock(g); got != 3 {
		t.Fatalf("current seg map got %d want 3", got)
	}
}

func TestReadBlockSegmentIDPostskipSkipUsesPredictedCurrentID(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.Enabled = true
	hdr.Segmentation.UpdateMap = true
	hdr.Segmentation.Preskip = false
	hdr.Segmentation.LastActiveSegID = 4

	state := NewSegmentationState(64, 64, nil, false)
	state.Current.FillBlock(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2}, 2)
	state.Current.FillBlock(BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2}, 2)
	state.Current.FillBlock(BlockGeometry{Start4X: 0, Start4Y: 2, End4X: 2, End4Y: 4}, 2)
	g := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4}
	above := &BlockContext{}
	left := &BlockContext{}

	segID, segPred, err := readBlockSegmentID(hdr, NewDefaultModeCDF(), above, left, state, g, true, true, true, true, &scriptedSegEntropy{})
	if err != nil {
		t.Fatalf("readBlockSegmentID: %v", err)
	}
	if segPred {
		t.Fatal("did not expect temporal predictor")
	}
	if segID != 2 {
		t.Fatalf("segID got %d want 2", segID)
	}
}

func TestReadInterBlockPreludeWithSegmentSkipFeature(t *testing.T) {
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeInter}
	hdr.Segmentation.Enabled = true
	hdr.Segmentation.Data[2].Ref = -1
	hdr.Segmentation.Data[2].Skip = true
	cdf := NewDefaultModeCDF()
	above := &BlockContext{}
	left := &BlockContext{}
	above.Reset(false, 0)
	left.Reset(false, 0)

	seq := av1.SequenceHeader{ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true}}
	syntax, err := ReadInterBlockPreludeWithNeighbors(seq, hdr, cdf, above, left, BlockGeometry{Size: Block16x16}, false, false, 2, &scriptedSegEntropy{})
	if err != nil {
		t.Fatalf("ReadInterBlockPreludeWithNeighbors: %v", err)
	}
	if !syntax.Skip {
		t.Fatal("expected segment skip feature to force skip")
	}
}
