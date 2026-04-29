package decoder

import (
	"io"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugGeneratedMain10Call4PreludeSummary struct {
	entryMSA     [4]int
	leaf         BlockGeometry
	skipModeCtx  int
	skipMode     bool
	skipCtx      int
	skip         bool
	aboveSkip    uint8
	leftSkip     uint8
	aboveSkipMode uint8
	leftSkipMode  uint8
}

func debugPeekGeneratedMain10LeafSkipPrelude(seq av1.SequenceHeader, hdr *obu.FrameHeader, segState *SegmentationState, state debugTargetLeafAdvanceState, modeCDF *ModeCDF, above, left *BlockContext, dec *msac.Context, targetX, targetY int) (debugGeneratedMain10Call4PreludeSummary, error) {
	leaf, err := debugDescendToPointLeaf(seq, hdr, modeCDF, above, left, state.nextRoot, dec, targetX, targetY)
	if err != nil {
		return debugGeneratedMain10Call4PreludeSummary{}, err
	}
	local := leaf
	haveTop := leaf.Start4Y > 0
	haveLeft := leaf.Start4X > 0
	msacDec := msacEntropyDecoder{ctx: dec}
	segID := uint8(0)
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, _, err = readBlockSegmentID(hdr, modeCDF, above, left, segState, leaf, haveTop, haveLeft, false, false, msacDec)
		if err != nil {
			return debugGeneratedMain10Call4PreludeSummary{}, err
		}
	}
	bx4 := local.Start4X & 31
	by4 := local.Start4Y & 31
	summary := debugGeneratedMain10Call4PreludeSummary{
		entryMSA:      debugGeneratedMain10MSACState(dec),
		leaf:          leaf,
		skipModeCtx:   int(above.SkipMode[bx4]) + int(left.SkipMode[by4]),
		skipCtx:       int(above.Skip[bx4]) + int(left.Skip[by4]),
		aboveSkip:     above.Skip[bx4],
		leftSkip:      left.Skip[by4],
		aboveSkipMode: above.SkipMode[bx4],
		leftSkipMode:  left.SkipMode[by4],
	}
	if hdr.SkipModeEnabled && minInt(leaf.Width4(), leaf.Height4()) > 1 {
		summary.skipMode = msacDec.DecodeBoolAdapt(modeCDF.SkipMode[summary.skipModeCtx][:]) != 0
		if summary.skipMode {
			summary.skip = true
			return summary, nil
		}
	}
	summary.skip, err = func() (bool, error) {
		skip := msacDec.DecodeBoolAdapt(modeCDF.Skip[summary.skipCtx][:]) != 0
		if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
			segID, _, err = readBlockSegmentID(hdr, modeCDF, above, left, segState, leaf, haveTop, haveLeft, true, skip, msacDec)
			if err != nil {
				return false, err
			}
		}
		if segmentDataForID(hdr, segID).Skip {
			skip = true
		}
		return skip, nil
	}()
	if err != nil {
		return debugGeneratedMain10Call4PreludeSummary{}, err
	}
	return summary, nil
}

func TestDebugGeneratedMain10Call4PreludeFromCall3Variants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PRELUDE_VARIANTS") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-prelude-variants.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
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
	seq := dec.header
	segState := NewSegmentationState(parsed.Header.Width, parsed.Header.Height, dec.choosePureGoSegmentationRef(&parsed.Header), parsed.Header.Segmentation.Enabled && !parsed.Header.Segmentation.UpdateMap)
	dec.Close()

	var (
		call3 debugTargetLeafAdvanceState
		call4 debugTargetLeafAdvanceState
		got3  bool
		got4  bool
	)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		switch callIdx {
		case 3:
			call3 = captured
			got3 = true
		case 4:
			call4 = captured
			got4 = true
		}
	})
	if !got3 || !got4 {
		t.Fatalf("captured call3=%v call4=%v", got3, got4)
	}

	targetX := call4.syntax.Geometry.Start4X * 4
	targetY := call4.syntax.Geometry.Start4Y * 4
	livePrelude, err := debugPeekGeneratedMain10LeafSkipPrelude(seq, &parsed.Header, segState, call3, call3.postMode.Clone(), &call3.postAbove, &call3.postLeft, call3.postDec.Clone(), targetX, targetY)
	if err != nil {
		t.Fatalf("live prelude: %v", err)
	}
	t.Logf("live call4 prelude leaf=(%d,%d)-(%d,%d) entry=%v skipModeCtx=%d skipCtx=%d aboveSkipMode=%d leftSkipMode=%d aboveSkip=%d leftSkip=%d skipMode=%v skip=%v",
		livePrelude.leaf.Start4X*4, livePrelude.leaf.Start4Y*4, livePrelude.leaf.End4X*4, livePrelude.leaf.End4Y*4,
		livePrelude.entryMSA,
		livePrelude.skipModeCtx,
		livePrelude.skipCtx,
		livePrelude.aboveSkipMode,
		livePrelude.leftSkipMode,
		livePrelude.aboveSkip,
		livePrelude.leftSkip,
		livePrelude.skipMode,
		livePrelude.skip,
	)

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error
	}
	scenarios := []scenario{
		{
			name: "luma_only_current",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_u_current",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_v_current",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "full_residual_current",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLuma(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
	}

	for _, tc := range scenarios {
		modeCDF := call3.mode.Clone()
		coefCDF := call3.coef.Clone()
		above := call3.above
		left := call3.left
		stateDec := call3.dec.Clone()
		combined := debugMSACCombinedDecoder{ctx: stateDec}
		if err := tc.apply(modeCDF, coefCDF, &above, &left, combined); err != nil {
			t.Fatalf("%s apply: %v", tc.name, err)
		}
		summary, err := debugPeekGeneratedMain10LeafSkipPrelude(seq, &parsed.Header, segState, call3, modeCDF, &above, &left, stateDec.Clone(), targetX, targetY)
		if err != nil {
			t.Fatalf("%s prelude: %v", tc.name, err)
		}
		t.Logf("%s entry=%v skipModeCtx=%d skipCtx=%d aboveSkipMode=%d leftSkipMode=%d aboveSkip=%d leftSkip=%d skipMode=%v skip=%v",
			tc.name,
			summary.entryMSA,
			summary.skipModeCtx,
			summary.skipCtx,
			summary.aboveSkipMode,
			summary.leftSkipMode,
			summary.aboveSkip,
			summary.leftSkip,
			summary.skipMode,
			summary.skip,
		)
	}
}
