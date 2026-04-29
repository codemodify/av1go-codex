package decoder

import (
	"fmt"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugPaletteIndexTraceStep struct {
	x     int
	y     int
	ctx   int
	tok   int
	order [8]uint8
}

type debugDeferredCFLSigns struct {
	signU int
	signV int
}

func debugReadPaletteIndicesTrace(cdf *ModeCDF, dec intraEntropyDecoder, palSize, plane, width, height int) ([]uint8, []debugPaletteIndexTraceStep, error) {
	if cdf == nil || dec == nil {
		return nil, nil, fmt.Errorf("decoder: palette indices require decoder state")
	}
	if palSize < 2 || palSize > 8 {
		return nil, nil, fmt.Errorf("decoder: invalid palette size %d", palSize)
	}
	if width <= 0 || height <= 0 {
		return nil, nil, fmt.Errorf("decoder: invalid palette dimensions %dx%d", width, height)
	}

	idx := make([]uint8, width*height)
	idx[0] = uint8(dec.DecodeUniform(uint32(palSize)))
	trace := make([]debugPaletteIndexTraceStep, 0, width*height-1)
	for diag := 1; diag < width+height-1; diag++ {
		startX, endX, stepX := paletteDiagonalXOrder(diag, width, height)
		for x := startX; ; x += stepX {
			y := diag - x
			order, ctx := paletteOrderForCell(idx, width, x, y, palSize)
			color := int(dec.DecodeSymbolAdapt(cdf.ColorMap[plane][palSize-2][ctx][:], palSize-1))
			if color < 0 || color >= palSize {
				return nil, nil, fmt.Errorf("decoder: palette color index %d out of range for size %d", color, palSize)
			}
			idx[y*width+x] = order[color]
			trace = append(trace, debugPaletteIndexTraceStep{
				x:     x,
				y:     y,
				ctx:   ctx,
				tok:   color,
				order: order,
			})
			if x == endX {
				break
			}
		}
	}
	return idx, trace, nil
}

func debugReadCFLSignsOnly(syntax *IntraBlockSyntax, cdf *ModeCDF, dec intraEntropyDecoder) (debugDeferredCFLSigns, error) {
	if syntax == nil || cdf == nil || dec == nil || syntax.UVMode != IntraPredCFL {
		return debugDeferredCFLSigns{}, nil
	}
	signTok := int(dec.DecodeSymbolAdapt(cdf.CFLSign[:], 7))
	sign := signTok + 1
	signU, signV := decodeCFLSigns(sign)
	return debugDeferredCFLSigns{signU: signU, signV: signV}, nil
}

func debugReadCFLAlphaWithDeferredSigns(syntax *IntraBlockSyntax, cdf *ModeCDF, dec intraEntropyDecoder, signs debugDeferredCFLSigns) error {
	if syntax == nil || cdf == nil || dec == nil || syntax.UVMode != IntraPredCFL {
		return nil
	}
	if signs.signU != 0 {
		ctx := boolToInt(signs.signU == 2)*3 + signs.signV
		alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
		if signs.signU == 1 {
			alpha = -alpha
		}
		syntax.CFLAlpha[0] = int8(alpha)
	}
	if signs.signV != 0 {
		ctx := boolToInt(signs.signV == 2)*3 + signs.signU
		alpha := int(dec.DecodeSymbolAdapt(cdf.CFLAlpha[ctx][:], 15)) + 1
		if signs.signV == 1 {
			alpha = -alpha
		}
		syntax.CFLAlpha[1] = int8(alpha)
	}
	return nil
}

func debugTraceCall4PaletteIndexReader(t *testing.T, label, mode string) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if t == nil || hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: debug call4 palette index reader requires full state")
		}

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
		syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
		if needsIntraAngleDelta(syntax.YMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
			if err != nil {
				return syntax, err
			}
			syntax.YAngle = angle
		}
		if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
			syntax.YMode = IntraPredDC
			syntax.YAngle = 0
		}

		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
			cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
			lastSymbol := int(numIntraPredModes - 1)
			if cflAllowed {
				lastSymbol++
			}
			syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
			if needsIntraAngleDelta(syntax.UVMode, g.Size) {
				angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
				if err != nil {
					return syntax, err
				}
				syntax.UVAngle = angle
			}
		}

		if paletteAllowedForBlock(hdr, g) && syntax.YMode == IntraPredDC {
			sizeCtx := paletteSizeContext(g)
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
			}
		}

		var deferredSigns debugDeferredCFLSigns
		switch mode {
		case "actual_default":
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
		case "after_luma_indices":
			// Delay CFL until after luma palette indices for comparison.
		case "sign_before_alpha_after_indices":
			signs, err := debugReadCFLSignsOnly(&syntax, cdf, dec)
			if err != nil {
				return syntax, err
			}
			deferredSigns = signs
		default:
			return syntax, fmt.Errorf("unknown trace mode %q", mode)
		}

		if syntax.PalYSize != 0 {
			indices, trace, err := debugReadPaletteIndicesTrace(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalYIdx = indices
			head := minInt(16, len(indices))
			t.Logf("%s pal=%v idx0=%v cfl=%v", label, syntax.PalY[:syntax.PalYSize], indices[:head], syntax.CFLAlpha)
			for i, step := range trace {
				if i >= 24 {
					t.Logf("%s ... truncated after %d steps", label, i)
					break
				}
				t.Logf("%s step=%d xy=(%d,%d) ctx=%d tok=%d order=%v out=%d",
					label, i, step.x, step.y, step.ctx, step.tok, step.order[:int(syntax.PalYSize)], indices[step.y*g.Width4()*4+step.x])
			}
		}

		if mode == "after_luma_indices" {
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
			t.Logf("%s post_idx_cfl=%v", label, syntax.CFLAlpha)
		}
		if mode == "sign_before_alpha_after_indices" {
			if err := debugReadCFLAlphaWithDeferredSigns(&syntax, cdf, dec, deferredSigns); err != nil {
				return syntax, err
			}
			t.Logf("%s post_idx_cfl=%v", label, syntax.CFLAlpha)
		}

		if hdr.Segmentation.Lossless[segID] {
			syntax.TX = TX4X4
			syntax.UVTX = TX4X4
			return syntax, nil
		}
		if hdr.TxfmMode == txModeSwitchable {
			info := TxfmInfoFor(syntax.TX)
			if info.Max > 0 {
				txCtx := GetTxCtx(above, left, info, by4, bx4)
				txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
				for txDepth > 0 {
					syntax.TX = TxfmInfoFor(syntax.TX).Sub
					txDepth--
				}
			}
		}
		return syntax, nil
	}
}

func TestDebugGeneratedMain10Call4PaletteIndexTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_INDEX_TRACE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-index-trace.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	variants := []struct {
		name string
		mode string
	}{
		{name: "actual_default", mode: "actual_default"},
		{name: "sign_before_alpha_after_indices", mode: "sign_before_alpha_after_indices"},
		{name: "after_luma_indices", mode: "after_luma_indices"},
	}

	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			var (
				state debugTargetLeafAdvanceState
				got   bool
			)
			debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, fixture.Path, map[int]debugIntraSyntaxReader{
				4: debugTraceCall4PaletteIndexReader(t, tc.name, tc.mode),
			}, func(callIdx int, captured debugTargetLeafAdvanceState) {
				if callIdx != 4 {
					return
				}
				state = captured
				got = true
			})
			if !got {
				t.Fatalf("%s did not capture call4", tc.name)
			}
			head := minInt(16, len(state.syntax.PalYIdx))
			t.Logf("%s final pal=%v idx0=%v cfl=%v", tc.name, state.syntax.PalY[:state.syntax.PalYSize], state.syntax.PalYIdx[:head], state.syntax.CFLAlpha)
		})
	}
}

func TestDebugGeneratedMain10Call4SplitCFLSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_SPLIT_CFL_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-split-cfl-sweep.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})
	refFrames := testutil.DecodeReferenceFrames420(t, fixture.Path, 1)
	if len(refFrames) != 1 {
		t.Fatalf("reference frames=%d, want 1", len(refFrames))
	}
	ref := refFrames[0]

	variants := []struct {
		name string
		read debugIntraSyntaxReader
	}{
		{name: "current"},
		{name: "sign_before_alpha_after_indices", read: debugTraceCall4PaletteIndexReader(t, "split_sign_alpha", "sign_before_alpha_after_indices")},
		{name: "after_luma_indices", read: debugTraceCall4PaletteIndexReader(t, "after_indices", "after_luma_indices")},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 4, overrides)
		head := minInt(16, len(score.state.syntax.PalYIdx))
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v idx0=%v",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			score.state.syntax.CFLAlpha,
			score.state.syntax.PalYIdx[:head],
		)
	}
}

func debugReadIntraBlockSyntaxCFLPlacementVariant(mode string) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: syntax reader requires decoder state")
		}

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
		syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, above, left, g, by4, bx4), int(numIntraPredModes-1)))
		if needsIntraAngleDelta(syntax.YMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
			if err != nil {
				return syntax, err
			}
			syntax.YAngle = angle
		}
		if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
			syntax.YMode = IntraPredDC
			syntax.YAngle = 0
		}

		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
			cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
			lastSymbol := int(numIntraPredModes - 1)
			if cflAllowed {
				lastSymbol++
			}
			syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
			if needsIntraAngleDelta(syntax.UVMode, g.Size) {
				angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
				if err != nil {
					return syntax, err
				}
				syntax.UVAngle = angle
			}
		}

		var deferredSigns debugDeferredCFLSigns
		switch mode {
		case "spec_before_palette":
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
		case "sign_before_palette_alpha_after_palette", "sign_before_palette_alpha_after_indices":
			signs, err := debugReadCFLSignsOnly(&syntax, cdf, dec)
			if err != nil {
				return syntax, err
			}
			deferredSigns = signs
		case "current_default", "sign_after_palette_alpha_after_indices", "after_luma_indices":
			// handled later
		default:
			return syntax, fmt.Errorf("unknown cfl placement variant %q", mode)
		}

		if paletteAllowedForBlock(hdr, g) {
			sizeCtx := paletteSizeContext(g)
			if syntax.YMode == IntraPredDC {
				palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
				if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
					size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
					if err != nil {
						return syntax, err
					}
					syntax.PalYSize = size
					syntax.PalY = values
				}
			}
			if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
				palCtx := boolToInt(syntax.PalYSize > 0)
				if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
					chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
					if chromaGeom.Width4() <= 0 || chromaGeom.Height4() <= 0 {
						return syntax, nil
					}
					size, palU, palV, err := readPaletteUVSyntax(seq.ColorConfig.BitDepth, cdf, above, left, sizeCtx, bx4, by4, dec)
					if err != nil {
						return syntax, err
					}
					syntax.PalUVSize = size
					syntax.PalU = palU
					syntax.PalV = palV
				}
			}
		}

		switch mode {
		case "current_default":
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
		case "sign_before_palette_alpha_after_palette":
			if err := debugReadCFLAlphaWithDeferredSigns(&syntax, cdf, dec, deferredSigns); err != nil {
				return syntax, err
			}
		case "sign_after_palette_alpha_after_indices":
			signs, err := debugReadCFLSignsOnly(&syntax, cdf, dec)
			if err != nil {
				return syntax, err
			}
			deferredSigns = signs
		}

		if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
			if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
				syntax.YMode = IntraPredFilter
				syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
			}
		}

		if syntax.PalYSize != 0 {
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, g.Width4()*4, g.Height4()*4)
			if err != nil {
				return syntax, err
			}
			syntax.PalYIdx = indices
		}

		switch mode {
		case "sign_before_palette_alpha_after_indices", "sign_after_palette_alpha_after_indices":
			if err := debugReadCFLAlphaWithDeferredSigns(&syntax, cdf, dec, deferredSigns); err != nil {
				return syntax, err
			}
		case "after_luma_indices":
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
		}

		if syntax.PalUVSize != 0 {
			chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
			if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
				indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, chromaGeom.Width4()*4, chromaGeom.Height4()*4)
				if err != nil {
					return syntax, err
				}
				syntax.PalUVIdx = indices
			}
		}
		if hdr.Segmentation.Lossless[segID] {
			syntax.TX = TX4X4
			syntax.UVTX = TX4X4
			return syntax, nil
		}
		if hdr.TxfmMode == txModeSwitchable {
			info := TxfmInfoFor(syntax.TX)
			if info.Max > 0 {
				txCtx := GetTxCtx(above, left, info, by4, bx4)
				txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
				for txDepth > 0 {
					syntax.TX = TxfmInfoFor(syntax.TX).Sub
					txDepth--
				}
			}
		}
		return syntax, nil
	}
}

func TestDebugGeneratedMain10Call4CFLPlacementSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_CFL_PLACEMENT_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-cfl-placement-sweep.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})
	refFrames := testutil.DecodeReferenceFrames420(t, fixture.Path, 1)
	if len(refFrames) != 1 {
		t.Fatalf("reference frames=%d, want 1", len(refFrames))
	}
	ref := refFrames[0]

	variants := []struct {
		name string
		read debugIntraSyntaxReader
	}{
		{name: "current"},
		{name: "spec_before_palette", read: debugReadIntraBlockSyntaxCFLPlacementVariant("spec_before_palette")},
		{name: "sign_before_palette_alpha_after_palette", read: debugReadIntraBlockSyntaxCFLPlacementVariant("sign_before_palette_alpha_after_palette")},
		{name: "sign_before_palette_alpha_after_indices", read: debugReadIntraBlockSyntaxCFLPlacementVariant("sign_before_palette_alpha_after_indices")},
		{name: "sign_after_palette_alpha_after_indices", read: debugReadIntraBlockSyntaxCFLPlacementVariant("sign_after_palette_alpha_after_indices")},
		{name: "after_luma_indices", read: debugReadIntraBlockSyntaxCFLPlacementVariant("after_luma_indices")},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 4, overrides)
		head := minInt(16, len(score.state.syntax.PalYIdx))
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v idx0=%v",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			score.state.syntax.CFLAlpha,
			score.state.syntax.PalYIdx[:head],
		)
	}
}

func TestDebugGeneratedMain10Call4ColorMapStateSweep(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_COLORMAP_STATE_SWEEP") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-colormap-state-sweep.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})
	refFrames := testutil.DecodeReferenceFrames420(t, fixture.Path, 1)
	if len(refFrames) != 1 {
		t.Fatalf("reference frames=%d, want 1", len(refFrames))
	}
	ref := refFrames[0]

	variants := []struct {
		name string
		read debugIntraSyntaxReader
	}{
		{name: "current"},
		{name: "default_pal_y", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true})},
		{name: "default_pal_size", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palSize: true})},
		{name: "default_color_map", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{colorMap: true})},
		{name: "default_palette_all", read: debugPaletteCDFOverrideReader(debugPaletteCDFReset{palY: true, palSize: true, palUV: true, colorMap: true})},
		{name: "default_cfl_sign", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLSign = defaultModeCDFValue.CFLSign
			return clone
		})},
		{name: "default_cfl_alpha", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			return clone
		})},
		{name: "default_cfl_only", read: debugModeCDFCloneReader(func(base *ModeCDF) *ModeCDF {
			clone := base.Clone()
			clone.CFLSign = defaultModeCDFValue.CFLSign
			clone.CFLAlpha = defaultModeCDFValue.CFLAlpha
			return clone
		})},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 4, overrides)
		head := minInt(16, len(score.state.syntax.PalYIdx))
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v idx0=%v",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			score.state.syntax.CFLAlpha,
			score.state.syntax.PalYIdx[:head],
		)
	}
}
