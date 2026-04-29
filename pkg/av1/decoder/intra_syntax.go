package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type IntraPredMode uint8

const (
	IntraPredDC IntraPredMode = iota
	IntraPredVertical
	IntraPredHorizontal
	IntraPredDiagDownLeft
	IntraPredDiagDownRight
	IntraPredVertRight
	IntraPredHorDown
	IntraPredHorUp
	IntraPredVertLeft
	IntraPredSmooth
	IntraPredSmoothVertical
	IntraPredSmoothHorizontal
	IntraPredPaeth

	numIntraPredModes

	IntraPredCFL    IntraPredMode = numIntraPredModes
	IntraPredFilter IntraPredMode = numIntraPredModes + 1
)

const (
	txModeLargest    = 1
	txModeSwitchable = 2
)

const cflAllowedMask = (1 << Block32x32) |
	(1 << Block32x16) |
	(1 << Block32x8) |
	(1 << Block16x32) |
	(1 << Block16x16) |
	(1 << Block16x8) |
	(1 << Block16x4) |
	(1 << Block8x32) |
	(1 << Block8x16) |
	(1 << Block8x8) |
	(1 << Block8x4) |
	(1 << Block4x16) |
	(1 << Block4x8) |
	(1 << Block4x4)

type intraEntropyDecoder interface {
	DecodeBool(uint32) uint32
	DecodeBoolEqui() uint32
	DecodeBoolAdapt([]uint16) uint32
	DecodeUniform(uint32) int
	DecodeBools(uint) uint32
	DecodeSymbolAdapt([]uint16, int) uint32
}

type msacEntropyDecoder struct {
	ctx *msac.Context
}

func (d msacEntropyDecoder) DecodeBool(prob uint32) uint32 {
	return d.ctx.DecodeBool(prob)
}

func (d msacEntropyDecoder) DecodeBoolEqui() uint32 {
	return d.ctx.DecodeBoolEqui()
}

func (d msacEntropyDecoder) DecodeBoolAdapt(cdf []uint16) uint32 {
	return d.ctx.DecodeBoolAdapt(cdf)
}

func (d msacEntropyDecoder) DecodeUniform(n uint32) int {
	return d.ctx.DecodeUniform(n)
}

func (d msacEntropyDecoder) DecodeBools(n uint) uint32 {
	return d.ctx.DecodeBools(n)
}

func (d msacEntropyDecoder) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	return d.ctx.DecodeSymbolAdapt(cdf, n)
}

func (d msacEntropyDecoder) DebugState() (uint32, uint32, int, int) {
	if d.ctx == nil {
		return 0, 0, 0, 0
	}
	return d.ctx.DebugState()
}

type IntraBlockSyntax struct {
	Geometry          BlockGeometry
	Size              BlockSize
	Segment           uint8
	SegPred           bool
	SkipMode          bool
	Skip              bool
	PreserveTXContext bool
	YMode             IntraPredMode
	UVMode            IntraPredMode
	YAngle            int8
	UVAngle           int8
	CFLAlpha          [2]int8
	PalYSize          uint8
	PalUVSize         uint8
	PalY              [8]uint16
	PalU              [8]uint16
	PalV              [8]uint16
	PalYIdx           []uint8
	PalUVIdx          []uint8
	TX                TxfmSize
	UVTX              TxfmSize
}

func decodeCFLSigns(sign int) (int, int) {
	signU := sign * 0x56 >> 8
	signV := sign - signU*3
	return signU, signV
}

func cflAlphaContext(plane, signSelf, signOther int) int {
	return cflAlphaContextForBlock(nil, plane, signSelf, signOther)
}

func cflAlphaContextForBlock(g *BlockGeometry, plane, signSelf, signOther int) int {
	ctx := boolToInt(signSelf == 2)*3 + signOther
	if override, ok := debugCFLAlphaContextOverrideForBlock(g, plane); ok {
		return override
	}
	return ctx
}

func readCFLAlphaSyntax(syntax *IntraBlockSyntax, cdf *ModeCDF, dec intraEntropyDecoder) error {
	if syntax == nil || cdf == nil || dec == nil || syntax.UVMode != IntraPredCFL {
		return nil
	}
	signTok := int(decodeCFLSymbolAdapt(dec, cdf.CFLSign[:], 7, "sign"))
	if override, ok := debugCFLSignTokenOverrideForBlock(&syntax.Geometry); ok {
		signTok = override
	}
	sign := signTok + 1
	signU, signV := decodeCFLSigns(sign)
	if signU != 0 {
		ctx := cflAlphaContextForBlock(&syntax.Geometry, 0, signU, signV)
		alpha := int(decodeCFLSymbolAdapt(dec, cdf.CFLAlpha[ctx][:], 15, "alpha")) + 1
		if signU == 1 {
			alpha = -alpha
		}
		syntax.CFLAlpha[0] = int8(alpha)
	}
	if signV != 0 {
		ctx := cflAlphaContextForBlock(&syntax.Geometry, 1, signV, signU)
		alpha := int(decodeCFLSymbolAdapt(dec, cdf.CFLAlpha[ctx][:], 15, "alpha")) + 1
		if signV == 1 {
			alpha = -alpha
		}
		syntax.CFLAlpha[1] = int8(alpha)
	}
	return nil
}

func debugCFLReadOrder() string {
	return debugCFLReadOrderForBlock(nil)
}

func debugCFLReadOrderFromValue(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "immediate", "default":
		return "immediate"
	case "after_luma_palette":
		return "after_luma_palette"
	case "after_luma_indices":
		return "after_luma_indices"
	case "skip_palette_10bit":
		return "skip_palette_10bit"
	case "skip", "off":
		return "skip"
	default:
		return "immediate"
	}
}

func debugCFLReadOrderForBlock(g *BlockGeometry) string {
	if debugCFLTargetMatches(g) {
		if raw := strings.TrimSpace(os.Getenv("DEBUG_CFL_READ_ORDER_TARGET")); raw != "" {
			return debugCFLReadOrderFromValue(raw)
		}
	}
	return debugCFLReadOrderFromValue(os.Getenv("DEBUG_CFL_READ_ORDER"))
}

// AV1 consumes CFL alpha immediately after CFL uvmode in the default syntax
// order. Non-default CFL placement is kept behind DEBUG_CFL_READ_ORDER for
// differential experiments only.
func shouldDelayCFLUntilAfterLumaPalette(seq av1.SequenceHeader, hdr *obu.FrameHeader, g BlockGeometry, syntax IntraBlockSyntax) bool {
	return false
}

func debugCFLSignTokenOverride() (int, bool) {
	return debugCFLSignTokenOverrideForBlock(nil)
}

func debugCFLTargetMatches(g *BlockGeometry) bool {
	raw := strings.TrimSpace(os.Getenv("DEBUG_CFL_TARGET"))
	if raw == "" || g == nil {
		return false
	}
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return false
	}
	wantX, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	wantY, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return false
	}
	return g.Start4X*4 == wantX && g.Start4Y*4 == wantY
}

func debugTargetMatchesEnv(g *BlockGeometry, envName string) bool {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" || g == nil {
		return false
	}
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return false
	}
	wantX, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	wantY, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return false
	}
	return g.Start4X*4 == wantX && g.Start4Y*4 == wantY
}

func debugContextTraceTarget() (int, int, bool) {
	raw := strings.TrimSpace(os.Getenv("DEBUG_CONTEXT_TRACE_TARGET"))
	if raw == "" {
		return 0, 0, false
	}
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return 0, 0, false
	}
	x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errX != nil || errY != nil {
		return 0, 0, false
	}
	return x / 4, y / 4, true
}

func absIntLocal(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func debugYModeOverrideForBlock(g *BlockGeometry) (IntraPredMode, bool) {
	if !debugTargetMatchesEnv(g, "DEBUG_YMODE_TARGET") {
		return 0, false
	}
	raw := strings.TrimSpace(os.Getenv("DEBUG_YMODE_FORCE_TARGET"))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 || v > int(IntraPredFilter) {
		return 0, false
	}
	return IntraPredMode(v), true
}

func debugYModeContextOverrideForBlock(g *BlockGeometry) (int, int, bool) {
	if !debugTargetMatchesEnv(g, "DEBUG_YMODE_TARGET") {
		return 0, 0, false
	}
	rawTop := strings.TrimSpace(os.Getenv("DEBUG_YMODE_TOPCTX_TARGET"))
	rawLeft := strings.TrimSpace(os.Getenv("DEBUG_YMODE_LEFTCTX_TARGET"))
	if rawTop == "" || rawLeft == "" {
		return 0, 0, false
	}
	top, errTop := strconv.Atoi(rawTop)
	left, errLeft := strconv.Atoi(rawLeft)
	if errTop != nil || errLeft != nil || top < 0 || top >= 5 || left < 0 || left >= 5 {
		return 0, 0, false
	}
	return top, left, true
}

func debugCFLSignTokenOverrideFromEnv(envName string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 || v > 7 {
		return 0, false
	}
	return v, true
}

func debugCFLSignTokenOverrideForBlock(g *BlockGeometry) (int, bool) {
	if debugCFLTargetMatches(g) {
		if v, ok := debugCFLSignTokenOverrideFromEnv("DEBUG_CFL_SIGN_TOKEN_TARGET"); ok {
			return v, true
		}
	}
	return debugCFLSignTokenOverrideFromEnv("DEBUG_CFL_SIGN_TOKEN")
}

func debugCFLAlphaContextOverrideFromEnv(keys ...string) (int, bool) {
	for _, key := range keys {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 || v >= 6 {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func debugCFLAlphaContextOverrideForBlock(g *BlockGeometry, plane int) (int, bool) {
	if debugCFLTargetMatches(g) {
		keys := []string{"DEBUG_CFL_ALPHA_CTX_TARGET", "DEBUG_CFL_ALPHA_CTX_U_TARGET", "DEBUG_CFL_ALPHA_CTX_V_TARGET"}
		switch plane {
		case 0:
			keys = append([]string{"DEBUG_CFL_ALPHA_CTX_U_TARGET"}, keys...)
		case 1:
			keys = append([]string{"DEBUG_CFL_ALPHA_CTX_V_TARGET"}, keys...)
		}
		if v, ok := debugCFLAlphaContextOverrideFromEnv(keys...); ok {
			return v, true
		}
	}
	return debugCFLAlphaContextOverride(plane)
}

func debugCFLAlphaContextOverride(plane int) (int, bool) {
	keys := []string{"DEBUG_CFL_ALPHA_CTX", "DEBUG_CFL_ALPHA_CTX_U", "DEBUG_CFL_ALPHA_CTX_V"}
	switch plane {
	case 0:
		keys = append([]string{"DEBUG_CFL_ALPHA_CTX_U"}, keys...)
	case 1:
		keys = append([]string{"DEBUG_CFL_ALPHA_CTX_V"}, keys...)
	}
	return debugCFLAlphaContextOverrideFromEnv(keys...)
}

func debugDisableCFLCDFUpdate(kind string) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_DISABLE_CFL_CDF_UPDATE")))
	if raw == "" || raw == "0" || raw == "off" || raw == "false" {
		return false
	}
	if raw == "1" || raw == "on" || raw == "true" || raw == "both" || raw == "all" {
		return true
	}
	switch kind {
	case "sign":
		return raw == "sign"
	case "alpha":
		return raw == "alpha"
	default:
		return false
	}
}

func decodeCFLSymbolAdapt(dec intraEntropyDecoder, cdf []uint16, n int, kind string) uint32 {
	if dec == nil {
		return 0
	}
	if debugDisableCFLCDFUpdate(kind) {
		return dec.DecodeSymbolAdapt(append([]uint16(nil), cdf...), n)
	}
	return dec.DecodeSymbolAdapt(cdf, n)
}

func debugTraceIntraSyntaxState(enabled bool, dec intraEntropyDecoder, g BlockGeometry, label string, format string, args ...any) {
	if !enabled {
		return
	}
	state, ok := dec.(interface {
		DebugState() (uint32, uint32, int, int)
	})
	if !ok {
		return
	}
	cur, rng, cnt, pos := state.DebugState()
	fmt.Fprintf(os.Stderr, "trace syntax state xy=(%d,%d) local=(%d,%d) size=%d %s state=(%d,%d,%d,%d)",
		g.Start4X*4, g.Start4Y*4, g.Start4X&31, g.Start4Y&31, g.Size, label, cur, rng, cnt, pos)
	if format != "" {
		fmt.Fprint(os.Stderr, " ")
		fmt.Fprintf(os.Stderr, format, args...)
	}
	fmt.Fprintln(os.Stderr)
}

func debugCDFHead(cdf []uint16, n int) []uint16 {
	if n > len(cdf) {
		n = len(cdf)
	}
	if n < 0 {
		n = 0
	}
	head := make([]uint16, n)
	copy(head, cdf[:n])
	return head
}

func ReadIntraBlockSyntax(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, fmt.Errorf("decoder: entropy decoder required")
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
	traceSyntaxState := debugTraceMovieBlock(g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4)
	yModeCDF := intraYModeCDF(hdr, cdf, above, left, g, by4, bx4)
	debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "entry",
		"haveTop=%t haveLeft=%t seg=%d cflOrder=%s yCDF=%v aboveMode=%d leftMode=%d",
		haveTop, haveLeft, segID, cflOrder, debugCDFHead(yModeCDF, 8), above.Mode[bx4], left.Mode[by4])
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(yModeCDF, int(numIntraPredModes-1)))
	debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-ymode",
		"ymode=%d yCDF=%v", syntax.YMode, debugCDFHead(yModeCDF, 8))
	if override, ok := debugYModeOverrideForBlock(&g); ok {
		syntax.YMode = override
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-ymode-override", "ymode=%d", syntax.YMode)
	}
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			return syntax, err
		}
		syntax.YAngle = angle
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-yangle", "yangle=%d", syntax.YAngle)
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-ymode-sanitize", "ymode=%d", syntax.YMode)
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, segID)
		lastSymbol := int(numIntraPredModes - 1)
		if cflAllowed {
			lastSymbol++
		}
		uvModeCDF := cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:]
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "pre-uvmode",
			"cflAllowed=%t lastSymbol=%d uvCDF=%v", cflAllowed, lastSymbol, debugCDFHead(uvModeCDF, 10))
		syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(uvModeCDF, lastSymbol))
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-uvmode",
			"uvmode=%d uvCDF=%v", syntax.UVMode, debugCDFHead(uvModeCDF, 10))
		deferCFLForLumaPalette := cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)
		cflCanUsePaletteProbe := cflOrder == "skip_palette_10bit" && seq.ColorConfig.BitDepth > 8 && paletteAllowedForBlock(hdr, g) && syntax.YMode == IntraPredDC
		if (cflOrder == "immediate" && !deferCFLForLumaPalette) || (cflOrder == "skip_palette_10bit" && !cflCanUsePaletteProbe) {
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-cfl-alpha",
				"uvmode=%d cflAlpha=%v", syntax.UVMode, syntax.CFLAlpha)
		}
		if needsIntraAngleDelta(syntax.UVMode, g.Size) {
			angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
			if err != nil {
				return syntax, err
			}
			syntax.UVAngle = angle
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-uvangle", "uvangle=%d", syntax.UVAngle)
		}
	}

	if paletteAllowedForBlock(hdr, g) {
		sizeCtx := paletteSizeContext(g)
		tracePaletteSyntax := traceSyntaxState
		if tracePaletteSyntax {
			cache, nCache := paletteCache(above, left, 0, bx4, by4)
			fmt.Fprintf(os.Stderr, "trace palette syntax xy=(%d,%d) local=(%d,%d) size=%d ymode=%d uv=%d aboveSz=%d above=%v leftSz=%d left=%v cache=%v\n",
				g.Start4X*4, g.Start4Y*4, bx4, by4, g.Size, syntax.YMode, syntax.UVMode,
				above.PalSz[bx4], above.Pal[0][bx4][:above.PalSz[bx4]],
				left.PalSz[by4], left.Pal[0][by4][:left.PalSz[by4]],
				cache[:nCache])
		}
		if syntax.YMode == IntraPredDC {
			palCtx := boolToInt(above.PalSz[bx4] > 0) + boolToInt(left.PalSz[by4] > 0)
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "pre-y-pal-bit",
				"sizeCtx=%d palCtx=%d palYCDF=%v", sizeCtx, palCtx, debugCDFHead(cdf.PalY[sizeCtx][palCtx][:], 3))
			palBit := dec.DecodeBoolAdapt(cdf.PalY[sizeCtx][palCtx][:])
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-y-pal-bit",
				"palBit=%d palYCDF=%v", palBit, debugCDFHead(cdf.PalY[sizeCtx][palCtx][:], 3))
			if tracePaletteSyntax {
				fmt.Fprintf(os.Stderr, "trace palette syntax y-pal-bit=%d sizeCtx=%d palCtx=%d\n", palBit, sizeCtx, palCtx)
			}
			if palBit != 0 {
				size, values, err := readPalettePlaneSyntax(seq.ColorConfig.BitDepth, cdf, above, left, 0, sizeCtx, bx4, by4, dec)
				if err != nil {
					return syntax, err
				}
				syntax.PalYSize = size
				syntax.PalY = values
				debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-y-pal-values",
					"palYSize=%d palY=%v palSizeCDF=%v", syntax.PalYSize, syntax.PalY[:syntax.PalYSize], debugCDFHead(cdf.PalSize[0][sizeCtx][:], 8))
				if tracePaletteSyntax {
					fmt.Fprintf(os.Stderr, "trace palette syntax y-pal size=%d values=%v\n", syntax.PalYSize, syntax.PalY[:syntax.PalYSize])
				}
			}
		}
		if cflOrder == "after_luma_palette" || (cflOrder == "immediate" && shouldDelayCFLUntilAfterLumaPalette(seq, hdr, g, syntax)) {
			if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
				return syntax, err
			}
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-cfl-alpha-after-palette",
				"uvmode=%d cflAlpha=%v", syntax.UVMode, syntax.CFLAlpha)
		}
		if cflOrder == "skip_palette_10bit" {
			if syntax.PalYSize == 0 {
				if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
					return syntax, err
				}
				debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-cfl-alpha-skip-palette",
					"uvmode=%d cflAlpha=%v", syntax.UVMode, syntax.CFLAlpha)
			}
		}
		if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) && syntax.UVMode == IntraPredDC {
			palCtx := boolToInt(syntax.PalYSize > 0)
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "pre-uv-pal-bit",
				"palCtx=%d palUVCDF=%v", palCtx, debugCDFHead(cdf.PalUV[palCtx][:], 3))
			if dec.DecodeBoolAdapt(cdf.PalUV[palCtx][:]) != 0 {
				debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-uv-pal-bit", "palBit=1")
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
				debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-uv-pal-values",
					"palUVSize=%d palU=%v palV=%v", syntax.PalUVSize, syntax.PalU[:syntax.PalUVSize], syntax.PalV[:syntax.PalUVSize])
			} else {
				debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-uv-pal-bit", "palBit=0")
			}
		}
	}

	if seq.EnableFilterIntra && syntax.YMode == IntraPredDC && syntax.PalYSize == 0 && maxIntLocal(int(BlockDims(g.Size).Log2W4), int(BlockDims(g.Size).Log2H4)) <= 3 {
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "pre-filter-intra",
			"useFilterCDF=%v", debugCDFHead(cdf.UseFilterIntra[g.Size][:], 3))
		if dec.DecodeBoolAdapt(cdf.UseFilterIntra[g.Size][:]) != 0 {
			syntax.YMode = IntraPredFilter
			syntax.YAngle = int8(dec.DecodeSymbolAdapt(cdf.FilterIntra[:], 4))
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-filter-intra",
				"enabled=1 filter=%d filterCDF=%v", syntax.YAngle, debugCDFHead(cdf.FilterIntra[:], 6))
		} else {
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-filter-intra", "enabled=0")
		}
	}

	if syntax.PalYSize != 0 {
		indexWidth, indexHeight := lumaPaletteIndexDimensions(g)
		indices, err := readPaletteIndices(cdf, dec, int(syntax.PalYSize), 0, indexWidth, indexHeight)
		if err != nil {
			return syntax, err
		}
		syntax.PalYIdx = indices
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-y-pal-indices",
			"palYIdxLen=%d", len(syntax.PalYIdx))
	}
	if cflOrder == "after_luma_indices" {
		if err := readCFLAlphaSyntax(&syntax, cdf, dec); err != nil {
			return syntax, err
		}
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-cfl-alpha-after-indices",
			"uvmode=%d cflAlpha=%v", syntax.UVMode, syntax.CFLAlpha)
	}
	if syntax.PalUVSize != 0 {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		if chromaGeom.Width4() > 0 && chromaGeom.Height4() > 0 {
			indexWidth, indexHeight := chromaPaletteIndexDimensions(seq.ColorConfig.ChromaSubsampling(), g)
			indices, err := readPaletteIndices(cdf, dec, int(syntax.PalUVSize), 1, indexWidth, indexHeight)
			if err != nil {
				return syntax, err
			}
			syntax.PalUVIdx = indices
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-uv-pal-indices",
				"palUVIdxLen=%d", len(syntax.PalUVIdx))
		}
	}
	if hdr.Segmentation.Lossless[segID] {
		syntax.TX = TX4X4
		syntax.UVTX = TX4X4
		debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "lossless-return",
			"tx=%d uvtx=%d", syntax.TX, syntax.UVTX)
		return syntax, nil
	}

	if hdr.TxfmMode == txModeSwitchable {
		info := TxfmInfoFor(syntax.TX)
		if info.Max > 0 {
			txCtx := GetTxCtx(above, left, info, by4, bx4)
			txCDF := cdf.TXSize[info.Max-1][txCtx][:]
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "pre-tx-size",
				"tx=%d txCtx=%d txCDF=%v", syntax.TX, txCtx, debugCDFHead(txCDF, 4))
			txDepth := dec.DecodeSymbolAdapt(cdf.TXSize[info.Max-1][txCtx][:], minInt(int(info.Max), 2))
			debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "post-tx-size",
				"txDepth=%d txCDF=%v", txDepth, debugCDFHead(txCDF, 4))
			for txDepth > 0 {
				syntax.TX = TxfmInfoFor(syntax.TX).Sub
				txDepth--
			}
		}
	}
	debugTraceIntraSyntaxState(traceSyntaxState, dec, g, "return",
		"ymode=%d uv=%d tx=%d uvtx=%d palY=%d palUV=%d", syntax.YMode, syntax.UVMode, syntax.TX, syntax.UVTX, syntax.PalYSize, syntax.PalUVSize)
	return syntax, nil
}

func ReadKeyIntraBlockSyntax(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool, segID uint8, dec intraEntropyDecoder) (IntraBlockSyntax, error) {
	return ReadIntraBlockSyntax(seq, hdr, cdf, above, left, g, haveTop, haveLeft, segID, dec)
}

func intraYModeCDF(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, by4, bx4 int) []uint16 {
	if hdr != nil && isInterOrSwitchFrameType(hdr.FrameType) {
		return cdf.YMode[generatedYModeSizeContext[g.Size]][:]
	}
	if topMode, leftMode, ok := debugYModeContextOverrideForBlock(&g); ok {
		return cdf.KFYMode[topMode][leftMode][:]
	}
	topMode := generatedIntraModeContext[above.Mode[bx4]]
	leftMode := generatedIntraModeContext[left.Mode[by4]]
	return cdf.KFYMode[topMode][leftMode][:]
}

func supportsLumaIntraPredictor(mode IntraPredMode, angle int8) bool {
	switch mode {
	case IntraPredDC, IntraPredVertical, IntraPredHorizontal, IntraPredDiagDownLeft, IntraPredDiagDownRight,
		IntraPredVertRight, IntraPredHorDown, IntraPredHorUp, IntraPredVertLeft,
		IntraPredSmooth, IntraPredSmoothVertical, IntraPredSmoothHorizontal, IntraPredPaeth, IntraPredFilter:
		_ = angle
		return true
	default:
		return false
	}
}

func needsIntraAngleDelta(mode IntraPredMode, bs BlockSize) bool {
	dims := BlockDims(bs)
	return dims.Log2W4+dims.Log2H4 >= 2 && mode >= IntraPredVertical && mode <= IntraPredVertLeft
}

func readIntraAngleDelta(cdf *ModeCDF, dec intraEntropyDecoder, mode IntraPredMode) (int8, error) {
	if cdf == nil || dec == nil {
		return 0, fmt.Errorf("decoder: angle delta requires entropy state")
	}
	if mode < IntraPredVertical || mode > IntraPredVertLeft {
		return 0, nil
	}
	angle := dec.DecodeSymbolAdapt(cdf.AngleDelta[mode-IntraPredVertical][:], 6)
	return int8(angle) - 3, nil
}

func ApplyIntraBlockContext(seq av1.SequenceHeader, hdr *obu.FrameHeader, above, left *BlockContext, syntax IntraBlockSyntax) error {
	if hdr == nil {
		return fmt.Errorf("decoder: frame header required")
	}
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}

	dims := BlockDims(syntax.Size)
	bx4 := syntax.Geometry.Start4X & 31
	by4 := syntax.Geometry.Start4Y & 31
	txInfo := TxfmInfoFor(syntax.TX)
	yModeNoFilt := uint8(intraModeForContext(syntax.YMode))
	if targetX4, targetY4, ok := debugContextTraceTarget(); ok {
		coversAbove := syntax.Geometry.Start4X <= targetX4 && targetX4 < syntax.Geometry.End4X && syntax.Geometry.End4Y <= targetY4
		coversLeft := syntax.Geometry.Start4Y <= targetY4 && targetY4 < syntax.Geometry.End4Y && syntax.Geometry.End4X <= targetX4
		startsNear := absIntLocal(syntax.Geometry.Start4X-targetX4) <= 8 && absIntLocal(syntax.Geometry.Start4Y-targetY4) <= 8
		if coversAbove || coversLeft || startsNear {
			fmt.Fprintf(os.Stderr, "trace context apply block=(%d,%d)-(%d,%d) local=(%d,%d) size=%d mode=%d ctxMode=%d uv=%d tx=%d palY=%d palYVals=%v coversAbove=%t coversLeft=%t aboveBeforeMode=%d aboveBeforePalSz=%d leftBeforeMode=%d leftBeforePalSz=%d\n",
				syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4, syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
				bx4, by4, syntax.Size, syntax.YMode, yModeNoFilt, syntax.UVMode, syntax.TX,
				syntax.PalYSize, syntax.PalY[:syntax.PalYSize], coversAbove, coversLeft,
				above.Mode[bx4], above.PalSz[bx4], left.Mode[by4], left.PalSz[by4])
		}
	}

	if !syntax.PreserveTXContext {
		fillRunInt8(above.TxIntra[:], bx4, 1<<dims.Log2W4, int8(txInfo.LW))
		fillRunInt8(left.TxIntra[:], by4, 1<<dims.Log2H4, int8(txInfo.LH))
		fillRunInt8(above.Tx[:], bx4, 1<<dims.Log2W4, int8(txInfo.LW))
		fillRunInt8(left.Tx[:], by4, 1<<dims.Log2H4, int8(txInfo.LH))
	}
	fillRunUint8(above.Mode[:], bx4, 1<<dims.Log2W4, yModeNoFilt)
	fillRunUint8(left.Mode[:], by4, 1<<dims.Log2H4, yModeNoFilt)
	fillRunUint8(above.PalSz[:], bx4, 1<<dims.Log2W4, syntax.PalYSize)
	fillRunUint8(left.PalSz[:], by4, 1<<dims.Log2H4, syntax.PalYSize)
	setPaletteRun(&above.Pal[0], bx4, 1<<dims.Log2W4, syntax.PalY, int(syntax.PalYSize))
	setPaletteRun(&left.Pal[0], by4, 1<<dims.Log2H4, syntax.PalY, int(syntax.PalYSize))
	fillRunUint8(above.SegPred[:], bx4, 1<<dims.Log2W4, boolToByte(syntax.SegPred))
	fillRunUint8(left.SegPred[:], by4, 1<<dims.Log2H4, boolToByte(syntax.SegPred))
	fillRunUint8(above.SkipMode[:], bx4, 1<<dims.Log2W4, boolToByte(syntax.SkipMode))
	fillRunUint8(left.SkipMode[:], by4, 1<<dims.Log2H4, boolToByte(syntax.SkipMode))
	fillRunUint8(above.Intra[:], bx4, 1<<dims.Log2W4, 1)
	fillRunUint8(left.Intra[:], by4, 1<<dims.Log2H4, 1)
	fillRunUint8(above.Skip[:], bx4, 1<<dims.Log2W4, boolToByte(syntax.Skip))
	fillRunUint8(left.Skip[:], by4, 1<<dims.Log2H4, boolToByte(syntax.Skip))

	if isInterOrSwitchFrameType(hdr.FrameType) {
		fillRunUint8(above.CompType[:], bx4, 1<<dims.Log2W4, 0)
		fillRunUint8(left.CompType[:], by4, 1<<dims.Log2H4, 0)
		fillRunInt8(above.Ref[0][:], bx4, 1<<dims.Log2W4, -1)
		fillRunInt8(left.Ref[0][:], by4, 1<<dims.Log2H4, -1)
		fillRunInt8(above.Ref[1][:], bx4, 1<<dims.Log2W4, -1)
		fillRunInt8(left.Ref[1][:], by4, 1<<dims.Log2H4, -1)
		fillRunUint8(above.Filter[0][:], bx4, 1<<dims.Log2W4, switchableFilterUnset)
		fillRunUint8(left.Filter[0][:], by4, 1<<dims.Log2H4, switchableFilterUnset)
		fillRunUint8(above.Filter[1][:], bx4, 1<<dims.Log2W4, switchableFilterUnset)
		fillRunUint8(left.Filter[1][:], by4, 1<<dims.Log2H4, switchableFilterUnset)
	}

	if hasChromaPlane(seq, syntax.Size, syntax.Geometry.Start4X, syntax.Geometry.Start4Y) {
		cw4, ch4 := chromaBlockDims(seq.ColorConfig.ChromaSubsampling(), dims)
		cbx4 := bx4
		cby4 := by4
		if seq.ColorConfig.ChromaSubsampling() != av1.Chroma444 {
			cbx4 >>= 1
		}
		if seq.ColorConfig.ChromaSubsampling() == av1.Chroma420 {
			cby4 >>= 1
		}
		fillRunUint8(above.UVMode[:], cbx4, cw4, uint8(syntax.UVMode))
		fillRunUint8(left.UVMode[:], cby4, ch4, uint8(syntax.UVMode))
	}
	fillRunUint8(above.UVPalSz[:], bx4, 1<<dims.Log2W4, syntax.PalUVSize)
	fillRunUint8(left.UVPalSz[:], by4, 1<<dims.Log2H4, syntax.PalUVSize)
	setPaletteRun(&above.Pal[1], bx4, 1<<dims.Log2W4, syntax.PalU, int(syntax.PalUVSize))
	setPaletteRun(&left.Pal[1], by4, 1<<dims.Log2H4, syntax.PalU, int(syntax.PalUVSize))
	setPaletteRun(&above.Pal[2], bx4, 1<<dims.Log2W4, syntax.PalV, int(syntax.PalUVSize))
	setPaletteRun(&left.Pal[2], by4, 1<<dims.Log2H4, syntax.PalV, int(syntax.PalUVSize))

	return nil
}

func MaxLumaTXSizeForBlock(bs BlockSize) TxfmSize {
	return TxfmSize(generatedMaxTXSizeForBlock[bs][0])
}

func hasChromaPlane(seq av1.SequenceHeader, bs BlockSize, bx4, by4 int) bool {
	layout := seq.ColorConfig.ChromaSubsampling()
	if layout == av1.Chroma400 {
		return false
	}
	dims := BlockDims(bs)
	ssVer := layout == av1.Chroma420
	ssHor := layout != av1.Chroma444
	hasHor := int(dims.W4) > boolToInt(ssHor) || (bx4&1) != 0
	hasVer := int(dims.H4) > boolToInt(ssVer) || (by4&1) != 0
	return hasHor && hasVer
}

func chromaBlockDims(layout av1.ChromaSubsampling, dims BlockDimensions) (int, int) {
	cw4 := int(dims.W4)
	ch4 := int(dims.H4)
	if layout != av1.Chroma444 && cw4 > 0 {
		cw4 = (cw4 + 1) >> 1
	}
	if layout == av1.Chroma420 && ch4 > 0 {
		ch4 = (ch4 + 1) >> 1
	}
	return cw4, ch4
}

func cflAllowedForSegment(seq av1.SequenceHeader, hdr *obu.FrameHeader, bs BlockSize, segID uint8) bool {
	if override := strings.TrimSpace(os.Getenv("DEBUG_FORCE_CFL_ALLOWED")); override != "" {
		switch strings.ToLower(override) {
		case "0", "false", "no", "off":
			return false
		case "1", "true", "yes", "on":
			return true
		}
	}
	if hdr != nil && int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
		dims := BlockDims(bs)
		cw4, ch4 := chromaBlockDims(seq.ColorConfig.ChromaSubsampling(), dims)
		return cw4 == 1 && ch4 == 1
	}
	return (cflAllowedMask & (1 << bs)) != 0
}

func cflAllowedForBlock(hdr *obu.FrameHeader, bs BlockSize) bool {
	return cflAllowedForSegment(av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}, hdr, bs, 0)
}

func fillRunUint8(dst []uint8, off, n int, v uint8) {
	if off >= len(dst) || n <= 0 {
		return
	}
	if off+n > len(dst) {
		n = len(dst) - off
	}
	for i := 0; i < n; i++ {
		dst[off+i] = v
	}
}

func fillRunInt8(dst []int8, off, n int, v int8) {
	if off >= len(dst) || n <= 0 {
		return
	}
	if off+n > len(dst) {
		n = len(dst) - off
	}
	for i := 0; i < n; i++ {
		dst[off+i] = v
	}
}

func isInterOrSwitchFrameType(ft obu.FrameType) bool {
	return ft == obu.FrameTypeInter || ft == obu.FrameTypeSwitch
}

func maxIntLocal(a, b int) int {
	if a > b {
		return a
	}
	return b
}
