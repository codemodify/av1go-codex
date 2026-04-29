package decoder

import (
	"fmt"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugGeneratedMain10PrefixReplay struct {
	skipMode    bool
	skip        bool
	preMSA      [4]int
	preQIdx     int
	postMSA     [4]int
	postQIdx    int
	syntax      IntraBlockSyntax
}

func debugReplayGeneratedMain10LeafPrefix(seq av1.SequenceHeader, hdr *obu.FrameHeader, live debugTargetLeafAdvanceState, consumeCDEF, consumeDelta bool) (debugGeneratedMain10PrefixReplay, error) {
	if hdr == nil {
		return debugGeneratedMain10PrefixReplay{}, fmt.Errorf("decoder: frame header required")
	}
	if live.entryMode == nil || live.entryCoef == nil || live.entryDec == nil {
		return debugGeneratedMain10PrefixReplay{}, fmt.Errorf("decoder: missing entry replay state")
	}
	above := live.entryAbove
	left := live.entryLeft
	cdf := live.entryMode.Clone()
	coef := live.entryCoef.Clone()
	dec := live.entryDec.Clone()
	cdef := debugCloneCDEFIndexMap(live.entryCDEF)
	if cdef == nil {
		cdef = NewCDEFIndexMap(hdr.Width, hdr.Height)
	}
	tile := TileState{
		Entropy: dec,
		CDF:     cdf,
		Coef:    coef,
	}
	qcat := DefaultCoefQCat(uint8(clipQIndexRuntime(live.entryQIdx)))
	tile.CoefByQ[qcat] = coef
	ts := &TileSyntaxState{
		Tile:       tile,
		LastQIdx:   live.entryQIdx,
		LastLF:     live.entryLastLF,
		ReadDeltas: live.entryReadDeltas,
		CurrentDQ:  live.entryDQ,
	}

	haveTop := live.syntax.Geometry.Start4Y > 0
	haveLeft := live.syntax.Geometry.Start4X > 0
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, cdf, &above, &left, live.entryLocal, msacEntropyDecoder{ctx: dec})
	if err != nil {
		return debugGeneratedMain10PrefixReplay{}, err
	}
	if consumeCDEF {
		if err := cdef.ensureBlockIndex(live.syntax.Geometry, hdr.CDEF.NBits, skip, msacEntropyDecoder{ctx: dec}); err != nil {
			return debugGeneratedMain10PrefixReplay{}, err
		}
	}
	if consumeDelta {
		if err := readDeltaQAndLFAtSuperBlockStart(seq, hdr, ts, live.syntax.Geometry, skip); err != nil {
			return debugGeneratedMain10PrefixReplay{}, err
		}
	}

	replay := debugGeneratedMain10PrefixReplay{
		skipMode: skipMode,
		skip:     skip,
		preMSA:   debugGeneratedMain10MSACState(dec),
		preQIdx:  ts.LastQIdx,
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, cdf, &above, &left, live.entryLocal, haveTop, haveLeft, live.syntax.Segment, msacEntropyDecoder{ctx: dec})
	if err != nil {
		return debugGeneratedMain10PrefixReplay{}, err
	}
	syntax.Geometry = live.syntax.Geometry
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	replay.postMSA = debugGeneratedMain10MSACState(dec)
	replay.postQIdx = ts.LastQIdx
	replay.syntax = syntax
	return replay, nil
}

func TestDebugGeneratedMain10Call4PrefixReplay(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PREFIX_REPLAY") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-prefix-replay.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	var live debugTargetLeafAdvanceState
	var got bool
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 4 {
			return
		}
		live = captured
		got = true
	})
	if !got {
		t.Fatal("did not capture call 4 state")
	}
	if live.preSyntaxDec == nil {
		t.Fatal("call 4 missing pre-syntax state")
	}

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()
	var parsed *ParsedFrame
	for {
		pf, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			parsed = pf
			break
		}
	}
	if parsed == nil {
		t.Fatal("no keyframe found")
	}
	if parsed.Header.Segmentation.Enabled {
		t.Fatal("debug prefix replay assumes generated fixture has segmentation disabled")
	}

	livePreMSA := debugGeneratedMain10MSACState(live.preSyntaxDec)
	t.Logf("live call4 preSyntax msa=%v qidx=%d syntax=%s cfl=%v idx0=%v",
		livePreMSA,
		live.preSyntaxQIdx,
		formatTargetSyntaxSnapshot(snapshotTargetSyntax(live)),
		live.syntax.CFLAlpha,
		append([]uint8(nil), live.syntax.PalYIdx[:minInt(len(live.syntax.PalYIdx), 16)]...),
	)

	variants := []struct {
		name         string
		consumeCDEF  bool
		consumeDelta bool
	}{
		{name: "current", consumeCDEF: true, consumeDelta: true},
		{name: "no_cdef", consumeCDEF: false, consumeDelta: true},
		{name: "no_delta", consumeCDEF: true, consumeDelta: false},
		{name: "no_cdef_no_delta", consumeCDEF: false, consumeDelta: false},
	}

	for _, tc := range variants {
		replay, err := debugReplayGeneratedMain10LeafPrefix(dec.header, &parsed.Header, live, tc.consumeCDEF, tc.consumeDelta)
		if err != nil {
			t.Fatalf("%s replay: %v", tc.name, err)
		}
		idx0 := append([]uint8(nil), replay.syntax.PalYIdx[:minInt(len(replay.syntax.PalYIdx), 16)]...)
		t.Logf("%s pre=%v pre_qidx=%d post=%v post_qidx=%d syntax=%s cfl=%v idx0=%v",
			tc.name,
			replay.preMSA,
			replay.preQIdx,
			replay.postMSA,
			replay.postQIdx,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(debugTargetLeafAdvanceState{syntax: replay.syntax})),
			replay.syntax.CFLAlpha,
			idx0,
		)
		if tc.name == "current" {
			if replay.preMSA != livePreMSA {
				t.Fatalf("current pre-syntax msa=%v want %v", replay.preMSA, livePreMSA)
			}
			if replay.preQIdx != live.preSyntaxQIdx {
				t.Fatalf("current pre-syntax qidx=%d want %d", replay.preQIdx, live.preSyntaxQIdx)
			}
			if snapshotTargetSyntax(debugTargetLeafAdvanceState{syntax: replay.syntax}) != snapshotTargetSyntax(live) {
				t.Fatalf("current syntax=%s want %s",
					formatTargetSyntaxSnapshot(snapshotTargetSyntax(debugTargetLeafAdvanceState{syntax: replay.syntax})),
					formatTargetSyntaxSnapshot(snapshotTargetSyntax(live)),
				)
			}
		}
	}
}
