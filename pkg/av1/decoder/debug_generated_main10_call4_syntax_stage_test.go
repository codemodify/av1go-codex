package decoder

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugIntraSyntaxStage struct {
	name  string
	value string
	msa   [4]int
}

func debugStage(name, value string, dec *msac.Context) debugIntraSyntaxStage {
	return debugIntraSyntaxStage{name: name, value: value, msa: debugGeneratedMain10MSACState(dec)}
}

func debugTraceIntraSyntaxStages(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, segID uint8, dec *msac.Context) (IntraBlockSyntax, []debugIntraSyntaxStage, error) {
	if hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
		return IntraBlockSyntax{}, nil, fmt.Errorf("decoder: syntax stage trace requires non-nil state")
	}

	entropy := msacEntropyDecoder{ctx: dec}
	stages := make([]debugIntraSyntaxStage, 0, 8)
	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  segID,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}

	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	cflOrder := debugCFLReadOrderForBlock(&g)
	syntax.YMode = IntraPredMode(entropy.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, entropy, syntax.YMode)
		if err != nil {
			return syntax, stages, err
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}
	stages = append(stages, debugStage("ymode", fmt.Sprintf("%s angle=%d", intraModeName(syntax.YMode), syntax.YAngle), dec))

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(entropy.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		deferCFLForLumaPalette := cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)
		cflCanUsePaletteProbe := cflOrder == "skip_palette_10bit" && seq.ColorConfig.BitDepth > 8 && paletteAllowedForBlock(hdr, g) && syntax.YMode == IntraPredDC
		if (cflOrder == "immediate" && !deferCFLForLumaPalette) || (cflOrder == "skip_palette_10bit" && !cflCanUsePaletteProbe) {
			if err := readCFLAlphaSyntax(&syntax, cdf, entropy); err != nil {
				return syntax, stages, err
			}
		}
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, entropy, syntax.UVMode)
			if err != nil {
				return syntax, stages, err
			}
			syntax.UVAngle = angle
		}
		stages = append(stages, debugStage("uvmode", fmt.Sprintf("%s angle=%d cfl=%v", intraModeName(syntax.UVMode), syntax.UVAngle, syntax.CFLAlpha), dec))
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			palY := entropy.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0
			stages = append(stages, debugStage("paly_flag", fmt.Sprintf("%v sizeCtx=%d palCtx=%d", palY, sizeCtx, palCtx), dec))
			if palY {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, entropy)
				if err != nil {
					return syntax, stages, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
				stages = append(stages, debugStage("paly_values", fmt.Sprintf("size=%d values=%v", size, values[:size]), dec))
			}
		}
		if cflOrder == "after_luma_palette" || (cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)) {
			if err := readCFLAlphaSyntax(&syntax, cdf, entropy); err != nil {
				return syntax, stages, err
			}
			stages = append(stages, debugStage("cfl_after_palette", fmt.Sprintf("%v", syntax.CFLAlpha), dec))
		}
		if cflOrder == "skip_palette_10bit" && syntax.PalYSize == 0 {
			if err := readCFLAlphaSyntax(&syntax, cdf, entropy); err != nil {
				return syntax, stages, err
			}
			stages = append(stages, debugStage("cfl_no_palette", fmt.Sprintf("%v", syntax.CFLAlpha), dec))
		}
	}

	if syntax.PalYSize != 0 {
		indices, _, err := debugReadPaletteIndicesTrace(cdf, entropy, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
		if err != nil {
			return syntax, stages, err
		}
		syntax.PalYIdx = indices
		stages = append(stages, debugStage("paly_idx", fmt.Sprintf("idx0=%v", append([]uint8(nil), indices[:minInt(len(indices), 16)]...)), dec))
	}
	if cflOrder == "after_luma_indices" {
		if err := readCFLAlphaSyntax(&syntax, cdf, entropy); err != nil {
			return syntax, stages, err
		}
		stages = append(stages, debugStage("cfl_after_indices", fmt.Sprintf("%v", syntax.CFLAlpha), dec))
	}

	return syntax, stages, nil
}

func debugTraceNextLeafSyntaxStages(seq av1.SequenceHeader, hdr *obu.FrameHeader, segState *SegmentationState, base debugTargetLeafAdvanceState, modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec *msac.Context) (IntraBlockSyntax, []debugIntraSyntaxStage, error) {
	entry, err := debugResumeNextLeafEntry(seq, hdr, base, modeCDF, coefCDF, above, left, dec)
	if err != nil {
		return IntraBlockSyntax{}, nil, err
	}
	leaf := entry.syntax.Geometry
	local := entry.entryLocal
	segID := uint8(0)
	segPred := false
	haveTop := leaf.Start4Y > 0
	haveLeft := leaf.Start4X > 0
	msacDec := msacEntropyDecoder{ctx: dec}
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, modeCDF, above, left, segState, leaf, haveTop, haveLeft, false, false, msacDec)
		if err != nil {
			return IntraBlockSyntax{}, nil, err
		}
	}
	skipMode, skip, err := ReadIntraSkipPrelude(hdr, modeCDF, above, left, local, msacDec)
	if err != nil {
		return IntraBlockSyntax{}, nil, err
	}
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, modeCDF, above, left, segState, leaf, haveTop, haveLeft, true, skip, msacDec)
		if err != nil {
			return IntraBlockSyntax{}, nil, err
		}
	}
	if segmentDataForID(hdr, segID).Skip {
		skip = true
	}
	cdef := debugCloneCDEFIndexMap(base.postCDEF)
	if cdef == nil {
		cdef = debugCloneCDEFIndexMap(base.cdef)
	}
	if cdef == nil {
		cdef = NewCDEFIndexMap(hdr.Width, hdr.Height)
	}
	if err := cdef.ensureBlockIndex(leaf, hdr.CDEF.NBits, skip, msacDec); err != nil {
		return IntraBlockSyntax{}, nil, err
	}
	tileSyntax := &TileSyntaxState{
		Tile: TileState{
			Entropy: dec,
			CDF:     modeCDF,
			Coef:    coefCDF,
		},
		LastQIdx:   base.qidx,
		LastLF:     base.lastLF,
		ReadDeltas: base.readDeltas,
		CurrentDQ:  base.dq,
	}
	tileSyntax.Tile.CoefByQ[DefaultCoefQCat(uint8(clipQIndexRuntime(base.qidx)))] = coefCDF
	if err := readDeltaQAndLF(seq, hdr, tileSyntax, leaf, skip); err != nil {
		return IntraBlockSyntax{}, nil, err
	}
	syntax, stages, err := debugTraceIntraSyntaxStages(seq, hdr, tileSyntax.Tile.CDF, above, left, local, segID, dec)
	if err != nil {
		return IntraBlockSyntax{}, nil, err
	}
	syntax.Geometry = leaf
	syntax.SegPred = segPred
	syntax.SkipMode = skipMode
	syntax.Skip = skip
	return syntax, stages, nil
}

func TestDebugGeneratedMain10Call4SyntaxStagesFromCall3Variants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_SYNTAX_STAGES") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-syntax-stages.mp4",
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

	var call3 debugTargetLeafAdvanceState
	var got3 bool
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, nil, func(callIdx int, captured debugTargetLeafAdvanceState) {
		if callIdx != 3 {
			return
		}
		call3 = captured
		got3 = true
	})
	if !got3 {
		t.Fatal("did not capture call3 state")
	}

	type scenario struct {
		name  string
		apply func(modeCDF *ModeCDF, coefCDF *CoefCDF, above, left *BlockContext, dec keyIntraBlockEntropyDecoder) error
	}
	scenarios := []scenario{
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

	type traced struct {
		syntax IntraBlockSyntax
		stages []debugIntraSyntaxStage
	}
	results := make(map[string]traced, len(scenarios))
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
		syntax, stages, err := debugTraceNextLeafSyntaxStages(seq, &parsed.Header, segState, call3, modeCDF, coefCDF, &above, &left, stateDec.Clone())
		if err != nil {
			t.Fatalf("%s trace next syntax: %v", tc.name, err)
		}
		results[tc.name] = traced{syntax: syntax, stages: stages}
		t.Logf("%s final=%s cfl=%v", tc.name, formatTargetSyntaxSnapshot(snapshotTargetSyntax(debugTargetLeafAdvanceState{syntax: syntax})), syntax.CFLAlpha)
		for i, stage := range stages {
			t.Logf("%s stage[%d] %s value=%s msa=%v", tc.name, i, stage.name, stage.value, stage.msa)
		}
	}

	a := results["luma_plus_u_current"]
	b := results["full_residual_current"]
	firstDiff := -1
	limit := len(a.stages)
	if len(b.stages) < limit {
		limit = len(b.stages)
	}
	for i := 0; i < limit; i++ {
		if !reflect.DeepEqual(a.stages[i], b.stages[i]) {
			firstDiff = i
			break
		}
	}
	t.Logf("first_diff_stage=%d", firstDiff)
	if firstDiff >= 0 {
		t.Logf("luma_plus_u_current diff stage=%+v", a.stages[firstDiff])
		t.Logf("full_residual_current diff stage=%+v", b.stages[firstDiff])
	}
}
