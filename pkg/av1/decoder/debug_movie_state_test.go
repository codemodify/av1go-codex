//go:build av1debug

package decoder

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugMovieFirstBlockStateDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("debug only")
	}
	if os.Getenv("DEBUG_MOVIE_STATE") == "" {
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

	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	syntaxState, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	row, err := syntaxState.NewRowState(states[0].Geometry.StartSBY)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.SuperBlocks) == 0 {
		t.Fatal("no superblocks")
	}

	sb := row.SuperBlocks[0]
	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
		t.Fatal(err)
	}
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	state, err := debugAdvanceToTargetLeafCDEFBefore(dec.header, &parsed.Header, row, sb, BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}, segState, 0, 0, 16, 0)
	if err != nil {
		t.Fatal(err)
	}

	firstSyntax := state.syntax
	t.Logf("movie first leaf tx=%s uvtx=%s y=%s uv=%s palY=%d palUV=%d nextRoot=(%d,%d)-(%d,%d)",
		txName(firstSyntax.TX), txName(firstSyntax.UVTX), intraModeName(firstSyntax.YMode), intraModeName(firstSyntax.UVMode), firstSyntax.PalYSize, firstSyntax.PalUVSize,
		state.nextRoot.Start4X*4, state.nextRoot.Start4Y*4, state.nextRoot.End4X*4, state.nextRoot.End4Y*4)

	baseDec := state.dec.Clone()
	baseAbove := state.above
	baseLeft := state.left

	for _, maxTUs := range []int{5, 6, 7, 8} {
		partialDec := baseDec.Clone()
		partialAbove := baseAbove
		partialLeft := baseLeft
		if err := debugConsumeFirstBlockLumaLimit(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &partialAbove, &partialLeft, firstSyntax, partialDec, maxTUs); err != nil {
			t.Fatalf("consume first block luma limit=%d: %v", maxTUs, err)
		}
		if err := ApplyIntraBlockContext(dec.header, &parsed.Header, &partialAbove, &partialLeft, firstSyntax); err != nil {
			t.Fatal(err)
		}
		mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &partialAbove, &partialLeft, state.nextRoot, partialDec.Clone(), 16, 0)
		if err != nil {
			t.Fatalf("peek next block after luma limit=%d: %v", maxTUs, err)
		}
		nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &partialAbove, &partialLeft, state.nextRoot, segState, partialDec.Clone(), 16, 0)
		if err != nil {
			t.Fatalf("peek next block syntax after luma limit=%d: %v", maxTUs, err)
		}
		t.Logf("movie next block after luma-first-%d-tus: y=%s syntaxY=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
			maxTUs, intraModeName(mode), intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
	}

	fullDec := baseDec.Clone()
	fullAbove := baseAbove
	fullLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &fullAbove, &fullLeft, firstSyntax, fullDec); err != nil {
		t.Fatal(err)
	}
	if err := ApplyIntraBlockContext(dec.header, &parsed.Header, &fullAbove, &fullLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	mode, err := debugPeekNextLeafYMode(dec.header, &parsed.Header, row.Tile.Tile.CDF, &fullAbove, &fullLeft, state.nextRoot, fullDec.Clone(), 16, 0)
	if err != nil {
		t.Fatal(err)
	}
	nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &fullAbove, &fullLeft, state.nextRoot, segState, fullDec.Clone(), 16, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("movie next block after full luma: y=%s syntaxY=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
		intraModeName(mode), intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
}
