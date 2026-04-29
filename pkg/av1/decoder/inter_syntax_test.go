package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestReadInterBlockSyntaxReadsIntraCodedInterBlock(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeInter,
		TxfmMode:  txModeSwitchable,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // skip
			0, // is_inter flag => intra-coded
			0, // filter intra disabled
		},
		symbols: []uint32{
			uint32(IntraPredDC),
			uint32(IntraPredDC),
			1, // tx depth => TX8X8 for 16x16
		},
	}

	syntax, err := ReadInterBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, 0, dec)
	if err != nil {
		t.Fatalf("ReadInterBlockSyntax: %v", err)
	}
	if syntax.Skip {
		t.Fatal("expected non-skip block")
	}
	if !syntax.Intra {
		t.Fatal("expected intra-coded inter block")
	}
	if syntax.IntraBlk == nil {
		t.Fatal("expected intra block syntax")
	}
	if syntax.IntraBlk.YMode != IntraPredDC {
		t.Fatalf("YMode = %d, want DC", syntax.IntraBlk.YMode)
	}
	if syntax.IntraBlk.TX != TX8X8 {
		t.Fatalf("TX = %d, want TX8X8", syntax.IntraBlk.TX)
	}
}

func TestReadInterBlockSyntaxReadsSkipMode(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{
		FrameType:       obu.FrameTypeInter,
		SkipModeEnabled: true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.SkipMode[0] = 1
	left.SkipMode[0] = 1
	dec := &scriptedIntraEntropy{
		adapt: []uint32{1},
	}

	syntax, err := ReadInterBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, 0, dec)
	if err != nil {
		t.Fatalf("ReadInterBlockSyntax: %v", err)
	}
	if !syntax.SkipMode {
		t.Fatal("expected skip_mode block")
	}
	if !syntax.Skip {
		t.Fatal("skip_mode block should imply skip")
	}
	if syntax.Intra {
		t.Fatal("skip_mode block should be inter-coded")
	}
	if syntax.Ref0 != -1 || syntax.Ref1 != -1 {
		t.Fatalf("default refs = (%d,%d), want (-1,-1) before ref syntax", syntax.Ref0, syntax.Ref1)
	}
}

func TestReadInterBlockSyntaxReadsIntraFlagForPlainSkip(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{
		FrameType: obu.FrameTypeInter,
		TxfmMode:  txModeSwitchable,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			1, // skip
			0, // is_inter flag => intra-coded
		},
		symbols: []uint32{
			uint32(IntraPredDC),
			uint32(IntraPredDC),
			1, // tx depth => TX8X8 for 16x16
		},
	}

	syntax, err := ReadInterBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, 0, dec)
	if err != nil {
		t.Fatalf("ReadInterBlockSyntax: %v", err)
	}
	if !syntax.Skip {
		t.Fatal("expected plain skip block")
	}
	if syntax.SkipMode {
		t.Fatal("did not expect skip_mode")
	}
	if !syntax.Intra {
		t.Fatal("plain skip must still read is_inter and allow intra-coded blocks")
	}
	if len(dec.adapt) != 0 {
		t.Fatalf("unconsumed adaptive bools = %d, want 0", len(dec.adapt))
	}
}

func TestReadInterBlockPreludeWithNeighborsUsesExplicitTileBoundaryFlags(t *testing.T) {
	seq := av1.SequenceHeader{}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeInter}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Intra[0] = 1
	left.Intra[0] = 1
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // skip = false
			1, // is_inter flag => inter-coded
		},
	}

	_, err := ReadInterBlockPreludeWithNeighbors(seq, hdr, cdf, &above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}, true, true, 0, dec)
	if err != nil {
		t.Fatalf("ReadInterBlockPreludeWithNeighbors: %v", err)
	}
	if len(dec.adaptCDF0) < 2 {
		t.Fatalf("recorded adapt cdfs = %d, want at least 2", len(dec.adaptCDF0))
	}
	if got, want := dec.adaptCDF0[1], cdf.Intra[3][0]; got != want {
		t.Fatalf("intra cdf[0] = %d, want context-3 value %d when explicit neighbors are present", got, want)
	}
}

func TestApplyInterBlockContextForInterBlock(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeInter}
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)

	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{
			Size:    Block16x16,
			Start4X: 4,
			Start4Y: 8,
			End4X:   8,
			End4Y:   12,
		},
		Size:     Block16x16,
		SkipMode: false,
		Skip:     true,
		Intra:    false,
	}

	if err := ApplyInterBlockContext(seq, hdr, &above, &left, syntax); err != nil {
		t.Fatalf("ApplyInterBlockContext: %v", err)
	}
	for i := 4; i < 8; i++ {
		if got := above.Intra[i]; got != 0 {
			t.Fatalf("above.Intra[%d] = %d, want 0", i, got)
		}
		if got := above.Skip[i]; got != 1 {
			t.Fatalf("above.Skip[%d] = %d, want 1", i, got)
		}
		if got := above.Mode[i]; got != nearestMVMode {
			t.Fatalf("above.Mode[%d] = %d, want nearest mv mode", i, got)
		}
		if got := above.Ref[1][i]; got != -1 {
			t.Fatalf("above.Ref[1][%d] = %d, want -1 for single-ref context", i, got)
		}
	}
	for i := 8; i < 12; i++ {
		if got := left.Intra[i]; got != 0 {
			t.Fatalf("left.Intra[%d] = %d, want 0", i, got)
		}
		if got := left.Skip[i]; got != 1 {
			t.Fatalf("left.Skip[%d] = %d, want 1", i, got)
		}
		if got := left.Mode[i]; got != nearestMVMode {
			t.Fatalf("left.Mode[%d] = %d, want nearest mv mode", i, got)
		}
		if got := left.Ref[1][i]; got != -1 {
			t.Fatalf("left.Ref[1][%d] = %d, want -1 for single-ref context", i, got)
		}
	}
}
