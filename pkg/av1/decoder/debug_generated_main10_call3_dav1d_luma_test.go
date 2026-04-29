package decoder

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func debugConsumeFirstBlockLumaWithLiteralProfile(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, dec keyIntraBlockEntropyDecoder, profile literalCoeffProfile) error {
	g := syntax.Geometry
	units, err := ReadIntraTransformUnitsInto(nil, hdr, modeCDF, above, left, g, syntax.TX, syntax.Segment, dec)
	if err != nil {
		return err
	}
	for _, unit := range units {
		tu := unit.Global
		tx := unit.TX
		txInfo := TxfmInfoFor(tx)
		offX4 := tu.Start4X - g.Start4X
		offY4 := tu.Start4Y - g.Start4Y
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		tuHaveTop := g.Start4Y > 0 || offY4 > 0
		tuHaveLeft := g.Start4X > 0 || offX4 > 0
		skipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		if dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
			if err := ApplyLumaResidualContext(above, left, tu, tx, 0x40); err != nil {
				return err
			}
			continue
		}
		yTxType, err := ReadIntraTxType(hdr, modeCDF, syntax, tx, tuHaveTop, tuHaveLeft, dec)
		if err != nil {
			return err
		}
		_, resCtx, _, _, err := decodeLumaCoefficientsLiteralNoSkipWithProfile(syntax.Size, tu, tx, yTxType, syntax.Segment, coefCDF, dq, above, left, dec, profile)
		if err != nil {
			return err
		}
		if err := ApplyLumaResidualContext(above, left, tu, tx, resCtx); err != nil {
			return err
		}
	}
	return nil
}

func TestDebugGeneratedMain10Call3Dav1dLumaBoundary(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL3_DAV1D_LUMA_BOUNDARY") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call3-dav1d-luma-boundary.mp4",
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

	formatSyntax := func(syntax IntraBlockSyntax) string {
		head := minInt(len(syntax.PalYIdx), 16)
		return fmt.Sprintf("geom=(%d,%d)-(%d,%d) skip=%v y=%s uv=%s tx=%s uvtx=%s palY=%v idxY=%v",
			syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4,
			syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
			syntax.Skip, intraModeName(syntax.YMode), intraModeName(syntax.UVMode),
			txName(syntax.TX), txName(syntax.UVTX),
			syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...))
	}
	peekNextLeafSyntax := func(cdf *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context, state debugTargetLeafAdvanceState) (IntraBlockSyntax, bool, error) {
		nextState, err := debugResumeNextLeafCDEFBefore(seq, &parsed.Header, segState, state, cdf, coefCDF, above, left, dec)
		if err != nil {
			return IntraBlockSyntax{}, false, err
		}
		return nextState.syntax, nextState.syntax.Skip, nil
	}

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
		{
			name: "luma_only_dav1d_profile",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLumaWithLiteralProfile(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec, literalCoeffDav1dProfile); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_u_dav1d_profile",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLumaWithLiteralProfile(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec, literalCoeffDav1dProfile); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{1}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "luma_plus_v_dav1d_profile",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLumaWithLiteralProfile(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec, literalCoeffDav1dProfile); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChromaPlanes(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, []int{2}); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
		{
			name: "full_residual_dav1d_luma",
			apply: func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error {
				if err := debugConsumeFirstBlockLumaWithLiteralProfile(seq, &parsed.Header, modeCDF, coefCDF, call3.dq, above, left, call3.syntax, dec, literalCoeffDav1dProfile); err != nil {
					return err
				}
				if err := debugConsumeFirstBlockChroma(seq, &parsed.Header, coefCDF, call3.dq, above, left, call3.syntax, dec, false); err != nil {
					return err
				}
				return debugApplyIntraBlockContextReplay(seq, &parsed.Header, above, left, call3.syntax)
			},
		},
	}

	t.Logf("call3 syntax=%s", formatSyntax(call3.syntax))
	t.Logf("call4 actual=%s", formatSyntax(call4.syntax))
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
		nextSyntax, nextSkip, err := peekNextLeafSyntax(modeCDF, coefCDF, &above, &left, stateDec.Clone(), call3)
		if err != nil {
			t.Fatalf("%s peek next syntax: %v", tc.name, err)
		}
		nextSyntax.Skip = nextSkip
		cur, rng, cnt, pos := stateDec.DebugState()
		t.Logf("%s -> call4 %s msac=(%d,%d,%d,%d)", tc.name, formatSyntax(nextSyntax), cur, rng, cnt, pos)
	}
}
