package decoder

import (
	"io"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestSpatialMVTileSplatIntraBCProducesCandidates(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{
		Start4X: 0,
		End4X:   32,
		Start4Y: 0,
		End4Y:   32,
	})
	block := BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}
	want := MotionVector{Y: -64, X: -128}
	tile.SplatIntraBC(block, want)

	cands, _ := tile.FindSingleRefCandidates(nil, BlockGeometry{
		Size:    Block8x8,
		Start4X: 2,
		Start4Y: 0,
		End4X:   4,
		End4Y:   2,
	}, 0)
	if len(cands) == 0 {
		t.Fatal("expected intrabc candidates")
	}
	if got := cands[0].MV; got != want {
		t.Fatalf("candidate MV = %+v, want %+v", got, want)
	}
}

func TestClipIntraBCMotionVectorMovesSourceOutsideCurrentSuperBlock(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig:           av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		Use128x128SuperBlock: true,
	}
	tile := TileGeometry{Start4X: 0, End4X: 64, Start4Y: 0, End4Y: 64}
	sb := SuperBlockGeometry{StartX: 0, StartY: 64}
	g := BlockGeometry{
		Size:    Block16x16,
		Start4X: 4,
		Start4Y: 20,
		End4X:   8,
		End4Y:   24,
	}

	mv, err := clipIntraBCMotionVector(seq, tile, sb, g, MotionVector{}, true)
	if err != nil {
		t.Fatalf("clipIntraBCMotionVector: %v", err)
	}
	srcLeft := g.Start4X*4 + (mv.X >> 3)
	srcTop := g.Start4Y*4 + (mv.Y >> 3)
	srcRight := srcLeft + g.Width4()*4
	srcBottom := srcTop + g.Height4()*4
	sbRight := sb.StartX + SuperBlockSizePx(seq)
	sbBottom := sb.StartY + SuperBlockSizePx(seq)
	if srcLeft < sbRight && srcTop < sbBottom && srcRight > sb.StartX && srcBottom > sb.StartY {
		t.Fatalf("clipped source still overlaps current superblock: [%d,%d)-[%d,%d)", srcLeft, srcTop, srcRight, srcBottom)
	}
}

func TestResolveIntraBCMotionVectorFallsBackToAlternateReference(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
	}
	tile := TileGeometry{Start4X: 0, End4X: 64, Start4Y: 0, End4Y: 64}
	sb := SuperBlockGeometry{StartX: 0, StartY: 64}
	g := BlockGeometry{
		Size:    Block16x16,
		Start4X: 8,
		Start4Y: 20,
		End4X:   12,
		End4Y:   24,
	}

	cands := []RefMVCandidate{
		{MV: MotionVector{}},
		{MV: MotionVector{Y: -512, X: 0}},
	}
	mv, err := resolveIntraBCMotionVector(seq, tile, sb, g, MotionVector{}, MotionVector{}, cands, true)
	if err != nil {
		t.Fatalf("resolveIntraBCMotionVector: %v", err)
	}
	srcLeft := g.Start4X*4 + (mv.X >> 3)
	srcTop := g.Start4Y*4 + (mv.Y >> 3)
	srcRight := srcLeft + g.Width4()*4
	srcBottom := srcTop + g.Height4()*4
	sbRight := sb.StartX + SuperBlockSizePx(seq)
	sbBottom := sb.StartY + SuperBlockSizePx(seq)
	if srcLeft < sbRight && srcTop < sbBottom && srcRight > sb.StartX && srcBottom > sb.StartY {
		t.Fatalf("resolved MV still overlaps current superblock: mv=%+v src=[%d,%d)-[%d,%d)", mv, srcLeft, srcTop, srcRight, srcBottom)
	}
}

func TestResolveIntraBCMotionVectorFallsBackToSafeTilePosition(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig:           av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		Use128x128SuperBlock: true,
	}
	tile := TileGeometry{Start4X: 0, End4X: 480, Start4Y: 0, End4Y: 214}
	sb := SuperBlockGeometry{StartX: 0, StartY: 0}
	g := BlockGeometry{
		Size:    Block4x4,
		Start4X: 13,
		Start4Y: 12,
		End4X:   14,
		End4Y:   13,
	}

	mv, err := resolveIntraBCMotionVector(seq, tile, sb, g, MotionVector{}, MotionVector{Y: -32, X: -3424}, nil, true)
	if err != nil {
		t.Fatalf("resolveIntraBCMotionVector: %v", err)
	}
	srcLeft := g.Start4X*4 + (mv.X >> 3)
	srcTop := g.Start4Y*4 + (mv.Y >> 3)
	srcRight := srcLeft + g.Width4()*4
	srcBottom := srcTop + g.Height4()*4
	decodedBeforeCurrent := srcBottom <= g.Start4Y*4 || srcRight <= g.Start4X*4
	if !decodedBeforeCurrent {
		t.Fatalf("fallback intrabc source is not in a previously decoded region: mv=%+v src=[%d,%d)-[%d,%d)", mv, srcLeft, srcTop, srcRight, srcBottom)
	}
}

func TestResolveIntraBCMotionVectorFallsBackWithinCurrentSuperBlock(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig:           av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		Use128x128SuperBlock: true,
	}
	tile := TileGeometry{Start4X: 0, End4X: 480, Start4Y: 0, End4Y: 214}
	sb := SuperBlockGeometry{StartX: 0, StartY: 0}
	g := BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 14,
		End4X:   2,
		End4Y:   16,
	}

	mv, err := resolveIntraBCMotionVector(seq, tile, sb, g, MotionVector{}, MotionVector{Y: 8, X: -3080}, nil, true)
	if err != nil {
		t.Fatalf("resolveIntraBCMotionVector: %v", err)
	}
	srcLeft := g.Start4X*4 + (mv.X >> 3)
	srcTop := g.Start4Y*4 + (mv.Y >> 3)
	srcRight := srcLeft + g.Width4()*4
	srcBottom := srcTop + g.Height4()*4
	decodedBeforeCurrent := srcBottom <= g.Start4Y*4 || srcRight <= g.Start4X*4
	if !decodedBeforeCurrent {
		t.Fatalf("fallback intrabc source is not in a previously decoded region: mv=%+v src=[%d,%d)-[%d,%d)", mv, srcLeft, srcTop, srcRight, srcBottom)
	}
}

func TestDecodeFirstAllowIntrabcFrameFromCorpus(t *testing.T) {
	path := testutil.SamplePath(t, "AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4")
	testutil.RequireFile(t, path)

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			if err == io.EOF {
				t.Skip("no allow_intrabc frame found in corpus sample")
			}
			t.Fatalf("NextParsedFrame: %v", err)
		}
		frame, err := dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame parsed=%d allow_intrabc=%v: %v", parsed.Index, parsed.Header.AllowIntrabc, err)
		}
		if !parsed.Header.AllowIntrabc {
			continue
		}
		if frame == nil || frame.Image == nil {
			t.Fatal("expected decoded frame image")
		}
		if len(frame.Y) == 0 || len(frame.U) == 0 || len(frame.V) == 0 {
			t.Fatal("expected decoded YUV planes")
		}
		return
	}
}

func TestDecodeAllowIntrabcFrameThroughInterDispatch(t *testing.T) {
	path := testutil.SamplePath(t, "AV1-Video-Sample-The-next-gen-Codec-in-Bitmovins-HTML5-Video-Pla.mp4")
	testutil.RequireFile(t, path)

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			if err == io.EOF {
				t.Skip("no allow_intrabc frame found in corpus sample")
			}
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if !parsed.Header.AllowIntrabc {
			continue
		}
		parsed.Header.FrameType = obu.FrameTypeInter
		frame, err := dec.decodePureGoFrame(parsed)
		if err != nil {
			t.Fatalf("decodePureGoFrame mutated inter allow_intrabc parsed=%d: %v", parsed.Index, err)
		}
		if frame == nil {
			t.Fatal("expected decoded frame")
		}
		if frame.Image == nil && len(frame.Y) == 0 && len(frame.Y16) == 0 {
			t.Fatal("expected decoded pixel planes")
		}
		return
	}
}

func TestReadScreenContentBlockSyntaxSegmentSkip(t *testing.T) {
	seq := av1.SequenceHeader{ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true}}
	hdr := &obu.FrameHeader{AllowIntrabc: true}
	hdr.Segmentation.Enabled = true
	hdr.Segmentation.Data[3].Ref = -1
	hdr.Segmentation.Data[3].Skip = true
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)

	syntax, err := ReadScreenContentBlockSyntax(seq, hdr, cdf, &above, &left, BlockGeometry{Size: Block16x16}, 3, &scriptedIntraEntropy{})
	if err != nil {
		t.Fatalf("ReadScreenContentBlockSyntax: %v", err)
	}
	if !syntax.Skip {
		t.Fatal("expected segment skip feature to force skip")
	}
}

func TestApplyIntraBCContextPreservesSegPred(t *testing.T) {
	seq := av1.SequenceHeader{ColorConfig: av1.ColorConfig{SubsamplingX: true, SubsamplingY: true}}
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	g := BlockGeometry{Size: Block16x16, Start4X: 4, Start4Y: 6}

	if err := ApplyIntraBCContext(seq, &above, &left, g, true, true); err != nil {
		t.Fatalf("ApplyIntraBCContext: %v", err)
	}
	if above.SegPred[g.Start4X&31] != 1 {
		t.Fatal("expected above segpred to be set")
	}
	if left.SegPred[g.Start4Y&31] != 1 {
		t.Fatal("expected left segpred to be set")
	}
}
