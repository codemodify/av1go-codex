package decoder

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type debugPaletteLiteralEntry struct {
	bitDepth uint8
	cdf      *ModeCDF
	above    BlockContext
	left     BlockContext
	dec      *msac.Context
	sizeCtx  int
	bx4      int
	by4      int
}

type debugPaletteIndexLiteralEntry struct {
	cdf     *ModeCDF
	dec     *msac.Context
	palSize int
	width   int
	height  int
}

func debugCloneMSACFromIntraEntropy(dec intraEntropyDecoder) (*msac.Context, error) {
	switch d := dec.(type) {
	case msacEntropyDecoder:
		if d.ctx == nil {
			return nil, fmt.Errorf("decoder: nil msac context")
		}
		return d.ctx.Clone(), nil
	case debugMSACCombinedDecoder:
		if d.ctx == nil {
			return nil, fmt.Errorf("decoder: nil combined msac context")
		}
		return d.ctx.Clone(), nil
	case *msac.Context:
		return d.Clone(), nil
	default:
		return nil, fmt.Errorf("decoder: unsupported entropy decoder type %T", dec)
	}
}

func debugReadPalettePlaneSyntaxDav1dLiteral(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return 0, [8]uint16{}, fmt.Errorf("decoder: palette plane syntax requires decoder state")
	}
	if bitDepth == 0 {
		bitDepth = 8
	}

	palSize := int(dec.DecodeSymbolAdapt(cdf.PalSize[plane][sizeCtx][:], 6)) + 2
	cache, nCache := paletteCache(above, left, plane, bx4, by4)
	usedCache := [8]uint16{}
	nUsed := 0
	for i := 0; i < nCache && nUsed < palSize; i++ {
		if dec.DecodeBoolEqui() != 0 {
			usedCache[nUsed] = cache[i]
			nUsed++
		}
	}

	var pal [8]uint16
	if nUsed < palSize {
		bpc := int(bitDepth)
		next := nUsed
		prev := int(dec.DecodeBools(uint(bpc)))
		pal[next] = uint16(prev)
		next++

		if next < palSize {
			bits := bpc - 3 + int(dec.DecodeBools(2))
			maxValue := (1 << bpc) - 1
			minStep := 0
			if plane == 0 {
				minStep = 1
			}
			for next < palSize {
				delta := int(dec.DecodeBools(uint(bits)))
				prev = minInt(prev+delta+minStep, maxValue)
				pal[next] = uint16(prev)
				next++
				if prev+minStep >= maxValue {
					for next < palSize {
						pal[next] = uint16(maxValue)
						next++
					}
					break
				}
				bits = minInt(bits, 1+ulog2(maxValue-prev-minStep))
			}
		}

		merged := [8]uint16{}
		a := 0
		b := nUsed
		for i := 0; i < palSize; i++ {
			if a < nUsed && (b >= palSize || usedCache[a] <= pal[b]) {
				merged[i] = usedCache[a]
				a++
			} else {
				merged[i] = pal[b]
				b++
			}
		}
		pal = merged
	} else {
		copy(pal[:], usedCache[:nUsed])
	}

	return uint8(palSize), pal, nil
}

func debugReadPaletteIndicesDav1dLiteral(cdf *ModeCDF, dec intraEntropyDecoder, palSize, plane, width, height int) ([]uint8, error) {
	if cdf == nil || dec == nil {
		return nil, fmt.Errorf("decoder: palette indices require decoder state")
	}
	if palSize < 2 || palSize > 8 {
		return nil, fmt.Errorf("decoder: invalid palette size %d", palSize)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("decoder: invalid palette dimensions %dx%d", width, height)
	}

	idx := make([]uint8, width*height)
	idx[0] = uint8(dec.DecodeUniform(uint32(palSize)))
	for diag := 1; diag < width+height-1; diag++ {
		startX := minInt(diag, width-1)
		endX := maxIntLocal(0, diag-height+1)
		for x := startX; ; x-- {
			y := diag - x
			order, ctx := paletteOrderForCell(idx, width, x, y, palSize)
			color := int(dec.DecodeSymbolAdapt(cdf.ColorMap[plane][palSize-2][ctx][:], palSize-1))
			if color < 0 || color >= palSize {
				return nil, fmt.Errorf("decoder: palette color index %d out of range for size %d", color, palSize)
			}
			idx[y*width+x] = order[color]
			if x == endX {
				break
			}
		}
	}
	return idx, nil
}

func debugCaptureGeneratedMain10CallPaletteValueEntry(t *testing.T, path string, callIdx int) debugPaletteLiteralEntry {
	t.Helper()

	var (
		entry debugPaletteLiteralEntry
		got   bool
	)
	reader := func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: full state required")
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
		cflOrder := debugCFLReadOrderForBlock(&g)

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
			deferCFLForLumaPalette := cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)
			cflCanUsePaletteProbe := cflOrder == "skip_palette_10bit" && seq.ColorConfig.BitDepth > 8 && paletteAllowedForBlock(hdr, g) && syntax.YMode == IntraPredDC
			if (cflOrder == "immediate" && !deferCFLForLumaPalette) || (cflOrder == "skip_palette_10bit" && !cflCanUsePaletteProbe) {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
				}
			}
			if needsIntraAngleDelta(syntax.UVMode, g.Size) {
				angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
				if err != nil {
					return syntax, err
				}
				syntax.UVAngle = angle
			}
		}

		if paletteAllowedForBlock(hdr, g) {
			sizeCtx := paletteSizeContext(g)
			if syntax.YMode == IntraPredDC {
				palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
				if dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:]) != 0 {
					if msacClone, err := debugCloneMSACFromIntraEntropy(dec); err != nil {
						return syntax, err
					} else if !got {
						entry = debugPaletteLiteralEntry{
							bitDepth: seq.ColorConfig.BitDepth,
							cdf:      cdf.Clone(),
							above:    *above,
							left:     *left,
							dec:      msacClone,
							sizeCtx:  sizeCtx,
							bx4:      bx4,
							by4:      by4,
						}
						got = true
					}
					size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
					if err != nil {
						return syntax, err
					}
					syntax.PalYSize = size
					syntax.PalY = values
				}
			}
			if cflOrder == "after_luma_palette" || (cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)) {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
				}
			}
			if cflOrder == "skip_palette_10bit" && syntax.PalYSize == 0 {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
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
		if cflOrder == "after_luma_indices" {
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

	overrides := map[int]debugIntraSyntaxReader{callIdx: reader}
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, nil)
	if !got {
		t.Fatalf("did not capture palette value entry for call %d", callIdx)
	}
	return entry
}

func debugCaptureGeneratedMain10CallPaletteIndexEntry(t *testing.T, path string, callIdx int) debugPaletteIndexLiteralEntry {
	t.Helper()

	var (
		entry debugPaletteIndexLiteralEntry
		got   bool
	)
	reader := func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: full state required")
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
		cflOrder := debugCFLReadOrderForBlock(&g)

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
			deferCFLForLumaPalette := cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)
			cflCanUsePaletteProbe := cflOrder == "skip_palette_10bit" && seq.ColorConfig.BitDepth > 8 && paletteAllowedForBlock(hdr, g) && syntax.YMode == IntraPredDC
			if (cflOrder == "immediate" && !deferCFLForLumaPalette) || (cflOrder == "skip_palette_10bit" && !cflCanUsePaletteProbe) {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
				}
			}
			if needsIntraAngleDelta(syntax.UVMode, g.Size) {
				angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
				if err != nil {
					return syntax, err
				}
				syntax.UVAngle = angle
			}
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
			if cflOrder == "after_luma_palette" || (cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)) {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
				}
			}
			if cflOrder == "skip_palette_10bit" && syntax.PalYSize == 0 {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
				}
			}
			if syntax.PalYSize != 0 {
				if msacClone, err := debugCloneMSACFromIntraEntropy(dec); err != nil {
					return syntax, err
				} else if !got {
					entry = debugPaletteIndexLiteralEntry{
						cdf:     cdf.Clone(),
						dec:     msacClone,
						palSize: int(syntax.PalYSize),
						width:   g.Width4() * 4,
						height:  g.Height4() * 4,
					}
					got = true
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
		if cflOrder == "after_luma_indices" {
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
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

	overrides := map[int]debugIntraSyntaxReader{callIdx: reader}
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, overrides, nil)
	if !got {
		t.Fatalf("did not capture palette index entry for call %d", callIdx)
	}
	return entry
}

func debugReadCFLAlphaPlane(syntax *IntraBlockSyntax, cdf *ModeCDF, dec intraEntropyDecoder, signSelf, signOther, plane int) error {
	if syntax == nil || cdf == nil || dec == nil || syntax.UVMode != IntraPredCFL || signSelf == 0 {
		return nil
	}
	ctx := cflAlphaContextForBlock(&syntax.Geometry, plane, signSelf, signOther)
	alpha := int(decodeCFLSymbolAdapt(dec, cdf.CFLAlpha[ctx][:], 15, "alpha")) + 1
	if signSelf == 1 {
		alpha = -alpha
	}
	syntax.CFLAlpha[plane] = int8(alpha)
	return nil
}

func debugReadIntraBlockSyntaxCFLVAfterLumaIndices(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: full state required")
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

	deferredSignV := 0
	deferredSignU := 0
	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
		if syntax.UVMode == IntraPredCFL {
			signTok := int(decodeCFLSymbolAdapt(dec, cdf.CFLSign[:], 7, "sign"))
			sign := signTok + 1
			signU, signV := decodeCFLSigns(sign)
			deferredSignU = signU
			deferredSignV = signV
			if err := debugReadCFLAlphaPlane(&syntax, cdf, dec, signU, signV, 0); err != nil {
				return syntax, err
			}
		}
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
		}
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
	if err := debugReadCFLAlphaPlane(&syntax, cdf, dec, deferredSignV, deferredSignU, 1); err != nil {
		return syntax, err
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

func debugReadIntraBlockSyntaxCFLAlphaPlanesAfterLumaIndices(deferU, deferV bool) debugIntraSyntaxReader {
	return func(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
		if hdr == nil || cdf == nil || above == nil || left == nil || dec == nil {
			return IntraBlockSyntax{}, fmt.Errorf("decoder: full state required")
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

		signU := 0
		signV := 0
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

			if syntax.UVMode == IntraPredCFL {
				signTok := int(decodeCFLSymbolAdapt(dec, cdf.CFLSign[:], 7, "sign"))
				sign := signTok + 1
				signU, signV = decodeCFLSigns(sign)
				if !deferU {
					if err := debugReadCFLAlphaPlane(&syntax, cdf, dec, signU, signV, 0); err != nil {
						return syntax, err
					}
				}
				if !deferV {
					if err := debugReadCFLAlphaPlane(&syntax, cdf, dec, signV, signU, 1); err != nil {
						return syntax, err
					}
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
		if deferU {
			if err := debugReadCFLAlphaPlane(&syntax, cdf, dec, signU, signV, 0); err != nil {
				return syntax, err
			}
		}
		if deferV {
			if err := debugReadCFLAlphaPlane(&syntax, cdf, dec, signV, signU, 1); err != nil {
				return syntax, err
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

func TestDebugGeneratedMain10Call4PaletteLiteralMatchesDav1d(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_LITERAL_COMPARE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-literal-compare.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entry := debugCaptureGeneratedMain10CallPaletteValueEntry(t, fixture.Path, 4)

	currentCDF := entry.cdf.Clone()
	currentDec := entry.dec.Clone()
	currentSize, currentPal, err := readPalettePlaneSyntax(entry.bitDepth, currentCDF, &entry.above, &entry.left, 0, entry.sizeCtx, entry.bx4, entry.by4, msacEntropyDecoder{ctx: currentDec})
	if err != nil {
		t.Fatalf("current readPalettePlaneSyntax: %v", err)
	}
	dav1dCDF := entry.cdf.Clone()
	dav1dDec := entry.dec.Clone()
	dav1dSize, dav1dPal, err := debugReadPalettePlaneSyntaxDav1dLiteral(entry.bitDepth, dav1dCDF, &entry.above, &entry.left, 0, entry.sizeCtx, entry.bx4, entry.by4, msacEntropyDecoder{ctx: dav1dDec})
	if err != nil {
		t.Fatalf("dav1d literal readPalettePlaneSyntax: %v", err)
	}

	curMSA := debugGeneratedMain10MSACState(currentDec)
	davMSA := debugGeneratedMain10MSACState(dav1dDec)
	t.Logf("call4 current size=%d pal=%v msa=%v", currentSize, currentPal[:currentSize], curMSA)
	t.Logf("call4 dav1d   size=%d pal=%v msa=%v", dav1dSize, dav1dPal[:dav1dSize], davMSA)

	if currentSize != dav1dSize {
		t.Fatalf("pal size current=%d dav1d=%d", currentSize, dav1dSize)
	}
	if currentPal != dav1dPal {
		t.Fatalf("pal current=%v dav1d=%v", currentPal[:currentSize], dav1dPal[:dav1dSize])
	}
	if curMSA != davMSA {
		t.Fatalf("msac current=%v dav1d=%v", curMSA, davMSA)
	}
	if !reflect.DeepEqual(currentCDF.PalSize, dav1dCDF.PalSize) {
		t.Fatal("PalSize CDF state differs after palette value decode")
	}
}

func TestDebugGeneratedMain10Call4PaletteIndicesLiteralMatchDav1d(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_PALETTE_INDEX_LITERAL_COMPARE") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-palette-index-literal-compare.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   6,
		BitDepth: 10,
	})

	entry := debugCaptureGeneratedMain10CallPaletteIndexEntry(t, fixture.Path, 4)

	currentCDF := entry.cdf.Clone()
	currentDec := entry.dec.Clone()
	currentIdx, err := readPaletteIndices(currentCDF, msacEntropyDecoder{ctx: currentDec}, entry.palSize, 0, entry.width, entry.height)
	if err != nil {
		t.Fatalf("current readPaletteIndices: %v", err)
	}
	dav1dCDF := entry.cdf.Clone()
	dav1dDec := entry.dec.Clone()
	dav1dIdx, err := debugReadPaletteIndicesDav1dLiteral(dav1dCDF, msacEntropyDecoder{ctx: dav1dDec}, entry.palSize, 0, entry.width, entry.height)
	if err != nil {
		t.Fatalf("dav1d literal readPaletteIndices: %v", err)
	}

	curMSA := debugGeneratedMain10MSACState(currentDec)
	davMSA := debugGeneratedMain10MSACState(dav1dDec)
	t.Logf("call4 current idx0=%v msa=%v", append([]uint8(nil), currentIdx[:minInt(len(currentIdx), 16)]...), curMSA)
	t.Logf("call4 dav1d   idx0=%v msa=%v", append([]uint8(nil), dav1dIdx[:minInt(len(dav1dIdx), 16)]...), davMSA)

	if !reflect.DeepEqual(currentIdx, dav1dIdx) {
		t.Fatalf("idx current=%v dav1d=%v", append([]uint8(nil), currentIdx[:minInt(len(currentIdx), 16)]...), append([]uint8(nil), dav1dIdx[:minInt(len(dav1dIdx), 16)]...))
	}
	if curMSA != davMSA {
		t.Fatalf("msac current=%v dav1d=%v", curMSA, davMSA)
	}
	if !reflect.DeepEqual(currentCDF.ColorMap, dav1dCDF.ColorMap) {
		t.Fatal("ColorMap CDF state differs after palette index decode")
	}
}

func TestDebugGeneratedMain10Call4CFLVAfterIndicesVariant(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_CFL_V_AFTER_INDICES") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-cfl-v-after-indices.mp4",
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
		{name: "call4_v_after_indices", read: debugReadIntraBlockSyntaxCFLVAfterLumaIndices},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 4, overrides)
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v syntax=%s",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			score.state.syntax.CFLAlpha,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
		)
	}
}

func TestDebugGeneratedMain10Call4CFLAlphaPlaneAfterIndicesVariants(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_CALL4_CFL_ALPHA_PLANE_AFTER_INDICES") == "" {
		t.Skip("debug only")
	}

	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "debug-main10-call4-cfl-alpha-plane-after-indices.mp4",
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
		{name: "u_after_indices", read: debugReadIntraBlockSyntaxCFLAlphaPlanesAfterLumaIndices(true, false)},
		{name: "v_after_indices", read: debugReadIntraBlockSyntaxCFLAlphaPlanesAfterLumaIndices(false, true)},
		{name: "both_after_indices", read: debugReadIntraBlockSyntaxCFLAlphaPlanesAfterLumaIndices(true, true)},
	}

	for _, tc := range variants {
		var overrides map[int]debugIntraSyntaxReader
		if tc.read != nil {
			overrides = map[int]debugIntraSyntaxReader{4: tc.read}
		}
		score := debugGeneratedMain10LeafTargetScore(t, fixture.Path, ref, 4, overrides)
		t.Logf("%s y_rmse=%.4f block_mse=%.2f u_block_mse=%.2f v_block_mse=%.2f base_mse=%.2f full_mse=%.2f cfl=%v syntax=%s",
			tc.name,
			score.yRMSE,
			score.blockMSE,
			score.uBlockMSE,
			score.vBlockMSE,
			score.baseMSE,
			score.fullMSE,
			score.state.syntax.CFLAlpha,
			formatTargetSyntaxSnapshot(snapshotTargetSyntax(score.state)),
		)
	}
}
