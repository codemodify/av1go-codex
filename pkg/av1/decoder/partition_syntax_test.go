package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedPartitionEntropy struct {
	bools   []uint32
	symbols []uint32
}

func (s *scriptedPartitionEntropy) DecodeBool(prob uint32) uint32 {
	if len(s.bools) == 0 {
		return 0
	}
	v := s.bools[0]
	s.bools = s.bools[1:]
	return v
}

func (s *scriptedPartitionEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	if len(s.symbols) == 0 {
		return 0
	}
	v := s.symbols[0]
	s.symbols = s.symbols[1:]
	return v
}

func TestDecodePartitionDecisionBothAxes(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	dec := &scriptedPartitionEntropy{symbols: []uint32{uint32(PartitionSplit)}}

	part, err := DecodePartitionDecision(av1.SequenceHeader{}, &obu.FrameHeader{}, cdf, &above, &left, BlockGeometry{
		Level:   BlockLevel64x64,
		Size:    Block64x64,
		Start4X: 0,
		Start4Y: 0,
		End4X:   16,
		End4Y:   16,
	}, dec)
	if err != nil {
		t.Fatalf("DecodePartitionDecision: %v", err)
	}
	if part != PartitionSplit {
		t.Fatalf("partition = %d, want split", part)
	}
}

func TestDecodePartitionDecisionAcceptsHighestSymbol(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	dec := &scriptedPartitionEntropy{symbols: []uint32{uint32(PartitionV4)}}

	part, err := DecodePartitionDecision(av1.SequenceHeader{}, &obu.FrameHeader{}, cdf, &above, &left, BlockGeometry{
		Level:   BlockLevel32x32,
		Size:    Block32x32,
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   8,
	}, dec)
	if err != nil {
		t.Fatalf("DecodePartitionDecision: %v", err)
	}
	if part != PartitionV4 {
		t.Fatalf("partition = %d, want V4", part)
	}
}

func TestDecodePartitionDecisionHorizontalEdgeUsesBool(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	dec := &scriptedPartitionEntropy{bools: []uint32{0}}

	part, err := DecodePartitionDecision(av1.SequenceHeader{}, &obu.FrameHeader{}, cdf, &above, &left, BlockGeometry{
		Level:   BlockLevel64x64,
		Size:    Block64x64,
		Start4X: 0,
		Start4Y: 0,
		End4X:   16,
		End4Y:   8,
	}, dec)
	if err != nil {
		t.Fatalf("DecodePartitionDecision: %v", err)
	}
	if part != PartitionH {
		t.Fatalf("partition = %d, want H", part)
	}
}

func TestDecodePartitionDecisionVerticalEdgeUsesBool(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	dec := &scriptedPartitionEntropy{bools: []uint32{1}}

	part, err := DecodePartitionDecision(av1.SequenceHeader{}, &obu.FrameHeader{}, cdf, &above, &left, BlockGeometry{
		Level:   BlockLevel64x64,
		Size:    Block64x64,
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   16,
	}, dec)
	if err != nil {
		t.Fatalf("DecodePartitionDecision: %v", err)
	}
	if part != PartitionSplit {
		t.Fatalf("partition = %d, want split", part)
	}
}

func TestDecodePartitionDecisionVerticalEdge422UsesBool(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	dec := &scriptedPartitionEntropy{bools: []uint32{0}}
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			SubsamplingX: true,
			SubsamplingY: false,
		},
	}

	part, err := DecodePartitionDecision(seq, &obu.FrameHeader{}, cdf, &above, &left, BlockGeometry{
		Level:   BlockLevel64x64,
		Size:    Block64x64,
		Start4X: 0,
		Start4Y: 0,
		End4X:   8,
		End4Y:   16,
	}, dec)
	if err != nil {
		t.Fatalf("DecodePartitionDecision: %v", err)
	}
	if part != PartitionV {
		t.Fatalf("partition = %d, want V", part)
	}
}

func TestDecodePartitionDecision8x8HorizontalEdgeIsForced(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext

	part, err := DecodePartitionDecision(av1.SequenceHeader{}, &obu.FrameHeader{}, cdf, &above, &left, BlockGeometry{
		Level:   BlockLevel8x8,
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   1,
	}, &scriptedPartitionEntropy{})
	if err != nil {
		t.Fatalf("DecodePartitionDecision: %v", err)
	}
	if part != PartitionH {
		t.Fatalf("partition = %d, want H", part)
	}
}

func TestApplyPartitionContext(t *testing.T) {
	var above, left BlockContext
	err := ApplyPartitionContext(&above, &left, BlockGeometry{
		Level:   BlockLevel64x64,
		Size:    Block64x64,
		Start4X: 4,
		Start4Y: 6,
		End4X:   20,
		End4Y:   22,
	}, PartitionH)
	if err != nil {
		t.Fatalf("ApplyPartitionContext: %v", err)
	}
	for i := 2; i < 10; i++ {
		if got := above.Partition[i]; got != uint8(AlignPartitionContext(0, BlockLevel64x64, PartitionH)) {
			t.Fatalf("above.Partition[%d] = %d", i, got)
		}
		if got := left.Partition[i+1]; got != uint8(AlignPartitionContext(1, BlockLevel64x64, PartitionH)) {
			t.Fatalf("left.Partition[%d] = %d", i+1, got)
		}
	}
}
