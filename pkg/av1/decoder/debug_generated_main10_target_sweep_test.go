package decoder

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugTargetSyntaxSnapshot struct {
	Geometry   BlockGeometry
	Skip       bool
	YMode      IntraPredMode
	UVMode     IntraPredMode
	TX         TxfmSize
	UVTX       TxfmSize
	PalYSize   uint8
	PalUVSize  uint8
	PalY       [8]uint16
	PalU       [8]uint16
	PalV       [8]uint16
	PalYIdx16  [16]uint8
	PalUVIdx16 [16]uint8
}

type debugLeafTraceEntry struct {
	Call       int
	Geometry   BlockGeometry
	HaveTop    bool
	HaveLeft   bool
	SegID      uint8
	HaveChroma bool
	CFLAllowed bool
	Syntax     debugTargetSyntaxSnapshot
}

func snapshotTargetSyntax(state debugTargetLeafAdvanceState) debugTargetSyntaxSnapshot {
	var snap debugTargetSyntaxSnapshot
	snap.Geometry = state.syntax.Geometry
	snap.Skip = state.syntax.Skip
	snap.YMode = state.syntax.YMode
	snap.UVMode = state.syntax.UVMode
	snap.TX = state.syntax.TX
	snap.UVTX = state.syntax.UVTX
	snap.PalYSize = state.syntax.PalYSize
	snap.PalUVSize = state.syntax.PalUVSize
	snap.PalY = state.syntax.PalY
	snap.PalU = state.syntax.PalU
	snap.PalV = state.syntax.PalV
	copy(snap.PalYIdx16[:], state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), len(snap.PalYIdx16))])
	copy(snap.PalUVIdx16[:], state.syntax.PalUVIdx[:minInt(len(state.syntax.PalUVIdx), len(snap.PalUVIdx16))])
	return snap
}

func formatTargetSyntaxSnapshot(snap debugTargetSyntaxSnapshot) string {
	return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v palUV=%v idxUV=%v",
		snap.Geometry.Start4X*4, snap.Geometry.Start4Y*4, snap.Geometry.End4X*4, snap.Geometry.End4Y*4,
		snap.Skip, intraModeName(snap.YMode), intraModeName(snap.UVMode), txName(snap.TX), txName(snap.UVTX),
		snap.PalY[:snap.PalYSize], snap.PalYIdx16[:], snap.PalU[:snap.PalUVSize], snap.PalUVIdx16[:])
}

func formatLeafTraceEntry(entry debugLeafTraceEntry) string {
	return fmt.Sprintf("call=%d geom=(%d,%d)-(%d,%d) size=%s top=%v left=%v seg=%d chroma=%v cfl=%v y=%s uv=%s tx=%s uvtx=%s palY=%d palUV=%d",
		entry.Call,
		entry.Geometry.Start4X*4, entry.Geometry.Start4Y*4, entry.Geometry.End4X*4, entry.Geometry.End4Y*4,
		blockName(entry.Geometry.Size),
		entry.HaveTop, entry.HaveLeft, entry.SegID,
		entry.HaveChroma, entry.CFLAllowed,
		intraModeName(entry.Syntax.YMode), intraModeName(entry.Syntax.UVMode),
		txName(entry.Syntax.TX), txName(entry.Syntax.UVTX),
		entry.Syntax.PalYSize, entry.Syntax.PalUVSize)
}

func advanceGeneratedMain10TargetWithReader(t *testing.T, path string, readSyntax debugIntraSyntaxReader) (debugTargetLeafAdvanceState, int) {
	t.Helper()

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}

	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no tile states")
	}
	syntaxState, err := NewTileSyntaxState(dec.header, &parsed.Header, states[0], 0)
	if err != nil {
		t.Fatalf("NewTileSyntaxState: %v", err)
	}

	targetX, targetY := 64, 0
	nextX, nextY := 68, 0
	var sb SuperBlockGeometry
	found := false
	for _, cur := range states[0].Geometry.SuperBlocks {
		if targetX >= cur.Start4X*4 && targetX < cur.End4X*4 && targetY >= cur.Start4Y*4 && targetY < cur.End4Y*4 {
			sb = cur
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("target superblock not found for (%d,%d)", targetX, targetY)
	}
	if err := debugConsumeRestorationThroughSuperBlock(dec.header, &parsed.Header, syntaxState, sb); err != nil {
		t.Fatalf("debugConsumeRestorationThroughSuperBlock: %v", err)
	}
	row, err := syntaxState.NewRowState(sb.SBY)
	if err != nil {
		t.Fatalf("NewRowState: %v", err)
	}
	traceSegState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	callCount := 0
	countingReader := func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		callCount++
		if readSyntax != nil {
			return readSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
		}
		return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
	}
	for _, priorSB := range row.SuperBlocks {
		if priorSB.Index == sb.Index {
			break
		}
		if err := debugConsumeBlockCDEFBeforeWithReader(dec.header, &parsed.Header, row, priorSB, BlockGeometry{
			Level:   RootBlockLevel(dec.header),
			Size:    RootBlockSize(dec.header),
			Start4X: priorSB.Start4X,
			Start4Y: priorSB.Start4Y,
			End4X:   priorSB.End4X,
			End4Y:   priorSB.End4Y,
		}, traceSegState, countingReader); err != nil {
			t.Fatalf("debugConsumeBlockCDEFBeforeWithReader(%d): %v", priorSB.Index, err)
		}
	}

	state, err := debugAdvanceToTargetLeafCDEFBeforeWithGlobalReader(dec.header, &parsed.Header, row, sb, BlockGeometry{
		Level:   RootBlockLevel(dec.header),
		Size:    RootBlockSize(dec.header),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}, traceSegState, targetX, targetY, nextX, nextY, countingReader)
	if err != nil {
		t.Fatalf("debugAdvanceToTargetLeafCDEFBeforeWithReader: %v", err)
	}
	return state, callCount
}

func traceGeneratedMain10TargetLeaves(t *testing.T, path string, perCall map[int]debugIntraSyntaxReader) ([]debugLeafTraceEntry, debugTargetLeafAdvanceState, int) {
	t.Helper()

	entries := make([]debugLeafTraceEntry, 0, 40)
	state, calls := advanceGeneratedMain10TargetWithReader(t, path, func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		callIdx := len(entries)
		reader := ReadIntraBlockSyntax
		if alt, ok := perCall[callIdx]; ok && alt != nil {
			reader = alt
		}
		syntax, err := reader(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
		if err != nil {
			return syntax, err
		}
		entry := debugLeafTraceEntry{
			Call:       callIdx,
			Geometry:   g,
			HaveTop:    haveTop,
			HaveLeft:   haveLeft,
			SegID:      segID,
			HaveChroma: hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y),
			CFLAllowed: hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && cflAllowedForSegment(seq, hdr, g.Size, segID),
			Syntax: debugTargetSyntaxSnapshot{
				Geometry:  g,
				YMode:     syntax.YMode,
				UVMode:    syntax.UVMode,
				TX:        syntax.TX,
				UVTX:      syntax.UVTX,
				PalYSize:  syntax.PalYSize,
				PalUVSize: syntax.PalUVSize,
				PalY:      syntax.PalY,
				PalU:      syntax.PalU,
				PalV:      syntax.PalV,
			},
		}
		copy(entry.Syntax.PalYIdx16[:], syntax.PalYIdx[:minInt(len(syntax.PalYIdx), len(entry.Syntax.PalYIdx16))])
		copy(entry.Syntax.PalUVIdx16[:], syntax.PalUVIdx[:minInt(len(syntax.PalUVIdx), len(entry.Syntax.PalUVIdx16))])
		entries = append(entries, entry)
		return syntax, nil
	})
	return entries, state, calls
}

func debugAdvanceToTargetLeafCDEFBeforeWithGlobalReader(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, segState *SegmentationState, targetX, targetY, nextX, nextY int, readSyntax debugIntraSyntaxReader) (debugTargetLeafAdvanceState, error) {
	if g.Start4X == sb.Start4X && g.Start4Y == sb.Start4Y && g.End4X == sb.End4X && g.End4Y == sb.End4Y {
		row.Tile.ResetSuperBlockDeltas(hdr)
	}
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	children, split, err := partitionChildren(g, part)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if !split {
		return debugAdvanceLeafSyntaxCDEFBeforeWithReader(seq, hdr, row, sb, g, segState, readSyntax)
	}

	targetIdx := -1
	nextIdx := -1
	for i, child := range children {
		if child.Width4() == 0 || child.Height4() == 0 {
			continue
		}
		if targetX >= child.Start4X*4 && targetX < child.End4X*4 && targetY >= child.Start4Y*4 && targetY < child.End4Y*4 {
			targetIdx = i
		}
		if nextX >= child.Start4X*4 && nextX < child.End4X*4 && nextY >= child.Start4Y*4 && nextY < child.End4Y*4 {
			nextIdx = i
		}
	}
	if targetIdx < 0 {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: no partition child covers target (%d,%d)", targetX, targetY)
	}
	if nextIdx < 0 {
		nextIdx = targetIdx
	}
	if nextIdx < targetIdx {
		return debugTargetLeafAdvanceState{}, fmt.Errorf("decoder: next target (%d,%d) precedes target (%d,%d) in traversal", nextX, nextY, targetX, targetY)
	}
	for i := 0; i < targetIdx; i++ {
		if children[i].Width4() == 0 || children[i].Height4() == 0 {
			continue
		}
		if err := debugConsumeBlockCDEFBeforeWithReader(seq, hdr, row, sb, children[i], segState, readSyntax); err != nil {
			return debugTargetLeafAdvanceState{}, err
		}
	}

	state, err := debugAdvanceToTargetLeafCDEFBeforeWithGlobalReader(seq, hdr, row, sb, children[targetIdx], segState, targetX, targetY, nextX, nextY, readSyntax)
	if err != nil {
		return debugTargetLeafAdvanceState{}, err
	}
	if targetIdx != nextIdx {
		state.nextRoot = children[nextIdx]
	}
	return state, nil
}

func TestDebugGeneratedMain10TargetDependencySweep(t *testing.T) {
	if os.Getenv("DEBUG_TARGET_DEP_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-target-dependency-main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	actualState, actualCalls := advanceGeneratedMain10TargetWithReader(t, fixture.Path, nil)
	actual := snapshotTargetSyntax(actualState)
	targetLeafCall := actualCalls - 1
	t.Logf("actual target leaf call=%d %s", targetLeafCall, formatTargetSyntaxSnapshot(actual))

	variants := []struct {
		name string
		read debugIntraSyntaxReader
	}{
		{name: "no_cfl", read: debugReadIntraBlockSyntaxNoCFL},
		{name: "tx_before_palette_indices", read: debugReadIntraBlockSyntaxTxBeforePaletteIndices},
	}

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			currentCall := 0
			targetOverrideState, _ := advanceGeneratedMain10TargetWithReader(t, fixture.Path, func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
				callIdx := currentCall
				currentCall++
				if callIdx == targetLeafCall {
					return variant.read(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
				}
				return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
			})
			targetOverride := snapshotTargetSyntax(targetOverrideState)
			t.Logf("variant %s on actual target leaf: %s", variant.name, formatTargetSyntaxSnapshot(targetOverride))

			changed := 0
			for leafCall := 0; leafCall < targetLeafCall; leafCall++ {
				currentCall := 0
				state, calls := advanceGeneratedMain10TargetWithReader(t, fixture.Path, func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
					callIdx := currentCall
					currentCall++
					if callIdx == leafCall {
						return variant.read(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
					}
					return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
				})
				if calls != actualCalls {
					changed++
					t.Logf("leaf %d changed target traversal count: got %d want %d", leafCall, calls, actualCalls)
					continue
				}
				snap := snapshotTargetSyntax(state)
				if reflect.DeepEqual(snap, actual) {
					continue
				}
				changed++
				t.Logf("leaf %d changed target: %s", leafCall, formatTargetSyntaxSnapshot(snap))
			}
			t.Logf("variant %s changed target on %d/%d earlier leaves", variant.name, changed, targetLeafCall)
		})
	}
}

func TestDebugGeneratedMain10TargetLeafTrace(t *testing.T) {
	if os.Getenv("DEBUG_TARGET_LEAF_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-target-leaf-trace-main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	current, targetState, calls := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	targetLeafCall := calls - 1
	t.Logf("target leaf call=%d %s", targetLeafCall, formatTargetSyntaxSnapshot(snapshotTargetSyntax(targetState)))
	for _, idx := range []int{27, 28, 29, 30, 31, 32} {
		if idx >= 0 && idx < len(current) {
			t.Logf("current %s", formatLeafTraceEntry(current[idx]))
		}
	}

	noCFL, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, map[int]debugIntraSyntaxReader{
		31: debugReadIntraBlockSyntaxNoCFL,
	})
	if len(noCFL) > 31 {
		t.Logf("leaf31 no_cfl %s", formatLeafTraceEntry(noCFL[31]))
	}

	txBefore, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, map[int]debugIntraSyntaxReader{
		31: debugReadIntraBlockSyntaxTxBeforePaletteIndices,
	})
	if len(txBefore) > 31 {
		t.Logf("leaf31 tx_before_palette_indices %s", formatLeafTraceEntry(txBefore[31]))
	}
}

func TestDebugGeneratedMain10EarlyLeafTrace(t *testing.T) {
	if os.Getenv("DEBUG_EARLY_LEAF_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-early-leaf-trace-main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	current, _, _ := traceGeneratedMain10TargetLeaves(t, fixture.Path, nil)
	for idx := 0; idx < len(current) && idx <= 10; idx++ {
		t.Logf("current %s", formatLeafTraceEntry(current[idx]))
	}
}
