//go:build av1debug

package decoder

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugMovieNeighborStateDrift(t *testing.T) {
	if os.Getenv("DEBUG_MOVIE_NEIGHBOR_STATE") == "" {
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
	var sb SuperBlockGeometry
	found := false
	for _, cur := range row.SuperBlocks {
		if 832 >= cur.Start4X*4 && 832 < cur.End4X*4 && 32 >= cur.Start4Y*4 && 32 < cur.End4Y*4 {
			sb = cur
			found = true
			break
		}
	}
	if !found {
		t.Fatal("target superblock not found")
	}

	restoration := NewRestorationState(dec.header, &parsed.Header)
	restRefs := newTileRestorationRefs()
	if err := parseRestorationUnitsForSuperBlock(&parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Entropy, sb, restoration, &restRefs); err != nil {
		t.Fatal(err)
	}
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	root := BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	state, err := debugAdvanceToTargetLeafCDEFBefore(dec.header, &parsed.Header, row, sb, root, segState, 832, 32, 836, 32)
	if err != nil {
		t.Fatal(err)
	}

	firstSyntax := state.syntax
	t.Logf("target block syntax y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d nextRoot=(%d,%d)-(%d,%d)",
		intraModeName(firstSyntax.YMode), intraModeName(firstSyntax.UVMode), txName(firstSyntax.TX), txName(firstSyntax.UVTX), firstSyntax.Skip, firstSyntax.PalYSize, firstSyntax.PalUVSize,
		state.nextRoot.Start4X*4, state.nextRoot.Start4Y*4, state.nextRoot.End4X*4, state.nextRoot.End4Y*4)

	baseDec := state.dec.Clone()
	baseAbove := state.above
	baseLeft := state.left

	lumaOnlyDec := baseDec.Clone()
	lumaOnlyAbove := baseAbove
	lumaOnlyLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &lumaOnlyAbove, &lumaOnlyLeft, firstSyntax, lumaOnlyDec); err != nil {
		t.Fatal(err)
	}
	if err := ApplyIntraBlockContext(dec.header, &parsed.Header, &lumaOnlyAbove, &lumaOnlyLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	nextSyntax, nextSkip, err := debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &lumaOnlyAbove, &lumaOnlyLeft, state.nextRoot, segState, lumaOnlyDec.Clone(), 836, 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after luma-only: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
		intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)

	currentDec := baseDec.Clone()
	currentAbove := baseAbove
	currentLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &currentAbove, &currentLeft, firstSyntax, currentDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &currentAbove, &currentLeft, firstSyntax, currentDec, false); err != nil {
		t.Fatal(err)
	}
	if err := ApplyIntraBlockContext(dec.header, &parsed.Header, &currentAbove, &currentLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &currentAbove, &currentLeft, state.nextRoot, segState, currentDec.Clone(), 836, 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after current chroma order: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
		intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)

	swappedDec := baseDec.Clone()
	swappedAbove := baseAbove
	swappedLeft := baseLeft
	if err := debugConsumeFirstBlockLuma(dec.header, &parsed.Header, row.Tile.Tile.CDF, row.Tile.Tile.Coef, state.dq, &swappedAbove, &swappedLeft, firstSyntax, swappedDec); err != nil {
		t.Fatal(err)
	}
	if err := debugConsumeFirstBlockChroma(dec.header, &parsed.Header, row.Tile.Tile.Coef, state.dq, &swappedAbove, &swappedLeft, firstSyntax, swappedDec, true); err != nil {
		t.Fatal(err)
	}
	if err := ApplyIntraBlockContext(dec.header, &parsed.Header, &swappedAbove, &swappedLeft, firstSyntax); err != nil {
		t.Fatal(err)
	}
	nextSyntax, nextSkip, err = debugPeekNextLeafSyntaxCDEFBefore(dec.header, &parsed.Header, row.Tile.Tile.CDF, &swappedAbove, &swappedLeft, state.nextRoot, segState, swappedDec.Clone(), 836, 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("next block after swapped chroma order: y=%s uv=%s tx=%s uvtx=%s skip=%v palY=%d palUV=%d",
		intraModeName(nextSyntax.YMode), intraModeName(nextSyntax.UVMode), txName(nextSyntax.TX), txName(nextSyntax.UVTX), nextSkip, nextSyntax.PalYSize, nextSyntax.PalUVSize)
}
