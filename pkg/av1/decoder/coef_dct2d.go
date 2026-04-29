package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

var (
	coeffTokenScratchPool  sync.Pool
	coeffLevelsScratchPool sync.Pool
)

var debugMovieTraceCoeffEnabled = os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != ""

func debugCoeffDisableCDFUpdate(family string) bool {
	spec := strings.TrimSpace(os.Getenv("DEBUG_COEF_DISABLE_CDF_UPDATE"))
	if spec == "" {
		return false
	}
	family = strings.ToLower(family)
	for _, item := range strings.Split(spec, ",") {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "", "0":
			continue
		case "1", "*", "all", family:
			return true
		}
	}
	return false
}

func debugCoeffFreezeCount(family string) bool {
	spec := strings.TrimSpace(os.Getenv("DEBUG_COEF_FREEZE_COUNT"))
	if spec == "" {
		return false
	}
	family = strings.ToLower(family)
	for _, item := range strings.Split(spec, ",") {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "", "0":
			continue
		case "1", "*", "all", family:
			return true
		}
	}
	return false
}

func coeffDecodeBoolAdapt(dec coefficientEntropyDecoder, cdf []uint16, family string) uint32 {
	if debugCoeffDisableCDFUpdate(family) {
		return dec.DecodeBoolAdapt(append([]uint16(nil), cdf...))
	}
	if debugCoeffFreezeCount(family) {
		clone := append([]uint16(nil), cdf...)
		bit := dec.DecodeBoolAdapt(clone)
		if len(cdf) != 0 {
			cdf[0] = clone[0]
		}
		return bit
	}
	return dec.DecodeBoolAdapt(cdf)
}

func coeffDecodeSymbolAdapt(dec coefficientEntropyDecoder, cdf []uint16, n int, family string) uint32 {
	if debugCoeffDisableCDFUpdate(family) {
		return dec.DecodeSymbolAdapt(append([]uint16(nil), cdf...), n)
	}
	if debugCoeffFreezeCount(family) {
		clone := append([]uint16(nil), cdf...)
		tok := dec.DecodeSymbolAdapt(clone, n)
		copy(cdf[:minInt(len(cdf), n)], clone[:minInt(len(clone), n)])
		return tok
	}
	return dec.DecodeSymbolAdapt(cdf, n)
}

func coeffDecodeHiTok(dec coefficientEntropyDecoder, cdf []uint16, family string) uint32 {
	if debugCoeffDisableCDFUpdate(family) {
		return dec.DecodeHiTok(append([]uint16(nil), cdf...))
	}
	if debugCoeffFreezeCount(family) {
		clone := append([]uint16(nil), cdf...)
		tok := dec.DecodeHiTok(clone)
		if len(cdf) != 0 {
			copy(cdf[:len(cdf)-1], clone[:len(clone)-1])
		}
		return tok
	}
	return dec.DecodeHiTok(cdf)
}

func debugCoeffTraceEventLimit() int {
	spec := strings.TrimSpace(os.Getenv("DEBUG_COEF_TRACE_EVENT_LIMIT"))
	if spec == "" {
		return 0
	}
	limit, err := strconv.Atoi(spec)
	if err != nil || limit <= 0 {
		return 0
	}
	return limit
}

func debugCoeffOrderMatchesSpec(order int, spec string) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return false
	}
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "-") {
			bounds := strings.SplitN(item, "-", 2)
			if len(bounds) != 2 {
				continue
			}
			start, errStart := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, errEnd := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if errStart != nil || errEnd != nil {
				continue
			}
			if start > end {
				start, end = end, start
			}
			if order >= start && order <= end {
				return true
			}
			continue
		}
		match, err := strconv.Atoi(item)
		if err != nil {
			continue
		}
		if order == match {
			return true
		}
	}
	return false
}

func debugCoeffTraceOrder(order int) bool {
	return debugCoeffOrderMatchesSpec(order, os.Getenv("DEBUG_COEF_TRACE_ORDERS"))
}

func debugCoeffSkipPlaneMatches(dqPlane int) bool {
	return debugCoeffPlaneMatches("DEBUG_COEF_SKIP_TARGET_PLANES", dqPlane)
}

func debugCoeffPlaneMatches(envName string, dqPlane int) bool {
	spec := strings.TrimSpace(os.Getenv(envName))
	if spec == "" {
		return true
	}
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		plane, err := strconv.Atoi(item)
		if err == nil && plane == dqPlane {
			return true
		}
	}
	return false
}

func debugCoeffSkipTargetMatches(g BlockGeometry, chroma bool, dqPlane int) bool {
	return debugCoeffTargetMatches("DEBUG_COEF_SKIP_TARGET", "DEBUG_COEF_SKIP_TARGET_CHROMA", "DEBUG_COEF_SKIP_TARGET_PLANES", g, chroma, dqPlane)
}

func debugCoeffSkipDisableCDFUpdateTargetMatches(g BlockGeometry, chroma bool, dqPlane int) bool {
	spec := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_COEF_SKIP_DISABLE_CDF_UPDATE_TARGET")))
	switch spec {
	case "", "0", "false", "no", "off":
		return false
	}
	return debugCoeffSkipTargetMatches(g, chroma, dqPlane)
}

func debugCoeffTargetMatches(targetEnv, chromaEnv, planesEnv string, g BlockGeometry, chroma bool, dqPlane int) bool {
	spec := strings.TrimSpace(os.Getenv(targetEnv))
	if spec == "" {
		return false
	}
	if !debugCoeffPlaneMatches(planesEnv, dqPlane) {
		return false
	}
	if only := strings.TrimSpace(os.Getenv(chromaEnv)); only != "" {
		switch strings.ToLower(only) {
		case "1", "true", "yes", "chroma":
			if !chroma {
				return false
			}
		case "0", "false", "no", "luma":
			if chroma {
				return false
			}
		}
	}
	parts := strings.Split(spec, ",")
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

func debugEOBBaseTargetCtx(g BlockGeometry, chroma bool, dqPlane int, info TxfmInfo, eob int) int {
	if strings.TrimSpace(os.Getenv("DEBUG_EOB_BASE_CTX_TARGET")) == "" {
		return eobBaseCtx(info, eob)
	}
	if !debugCoeffTargetMatches("DEBUG_EOB_BASE_CTX_TARGET", "DEBUG_EOB_BASE_CTX_TARGET_CHROMA", "DEBUG_EOB_BASE_CTX_TARGET_PLANES", g, chroma, dqPlane) {
		return 2
	}
	return eobBaseCtx(info, eob)
}

func debugCoeffSkipOverrideValue(envName string, current, max int) int {
	spec := strings.TrimSpace(os.Getenv(envName))
	if spec == "" {
		return current
	}
	if value, err := strconv.Atoi(spec); err == nil {
		return clampInt(value, 0, max)
	}
	return current
}

func coeffCDFRow(ctx int, family string, max int) int {
	var envName string
	switch family {
	case "skip":
		envName = "DEBUG_COEF_SKIP_ROW"
	case "eob-base":
		envName = "DEBUG_COEF_EOB_BASE_ROW"
	case "base":
		envName = "DEBUG_COEF_BASE_ROW"
	case "br":
		envName = "DEBUG_COEF_BR_ROW"
	default:
		return clampInt(ctx, 0, max)
	}
	if override := strings.TrimSpace(os.Getenv(envName)); override != "" {
		if idx, err := strconv.Atoi(override); err == nil {
			return clampInt(idx, 0, max)
		}
	}
	return clampInt(ctx, 0, max)
}

func coeffCDFRowForCoef(coef *CoefCDF, ctx int, family string, max int) int {
	row := coeffCDFRow(ctx, family, max)
	if family != "br" || strings.TrimSpace(os.Getenv("DEBUG_COEF_BR_ROW")) != "" {
		return row
	}
	qcat := -1
	if coef != nil {
		qcat = coef.QCat
	}
	if mapped, ok := coeffCDFRowMap(qcat, ctx, max, strings.TrimSpace(os.Getenv("DEBUG_COEF_BR_ROW_MAP"))); ok {
		return mapped
	}
	return row
}

func coeffBRCDFRowForOrder(coef *CoefCDF, ctx, order, max int) int {
	row := coeffCDFRow(ctx, "br", max)
	if strings.TrimSpace(os.Getenv("DEBUG_COEF_BR_ROW")) != "" {
		return row
	}
	orderSpec := strings.TrimSpace(os.Getenv("DEBUG_COEF_BR_ROW_MAP_ORDERS"))
	if orderSpec != "" && !debugCoeffOrderMatchesSpec(order, orderSpec) {
		return row
	}
	qcat := -1
	if coef != nil {
		qcat = coef.QCat
	}
	if mapped, ok := coeffCDFRowMap(qcat, ctx, max, strings.TrimSpace(os.Getenv("DEBUG_COEF_BR_ROW_MAP"))); ok {
		return mapped
	}
	return row
}

func coeffCDFRowMap(qcat, ctx, max int, spec string) (int, bool) {
	if spec == "" {
		return 0, false
	}
	fallback := -1
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		match := strings.TrimSpace(parts[0])
		rowVal, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		if strings.Contains(match, "/") {
			matchParts := strings.SplitN(match, "/", 2)
			if len(matchParts) != 2 {
				continue
			}
			matchQCat, errQCat := strconv.Atoi(strings.TrimSpace(matchParts[0]))
			matchCtx, errCtx := strconv.Atoi(strings.TrimSpace(matchParts[1]))
			if errQCat != nil || errCtx != nil {
				continue
			}
			if matchQCat == qcat && matchCtx == ctx {
				return clampInt(rowVal, 0, max), true
			}
			continue
		}
		matchCtx, err := strconv.Atoi(match)
		if err != nil || matchCtx != ctx {
			continue
		}
		if fallback < 0 {
			fallback = rowVal
		}
	}
	if fallback >= 0 {
		return clampInt(fallback, 0, max), true
	}
	return 0, false
}

func eobTx2DCtx(info TxfmInfo) int {
	slw := minInt(int(info.LW), int(TxfmInfoFor(TX32X32).LW))
	slh := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	return slw + slh
}

func eobHiBitCtx(info TxfmInfo) int {
	if override := strings.TrimSpace(os.Getenv("DEBUG_EOB_HI_CTX")); override != "" {
		if idx, err := strconv.Atoi(override); err == nil {
			return clampInt(idx, 0, 4)
		}
		switch strings.ToLower(override) {
		case "info", "ctx":
			return clampInt(int(info.Ctx), 0, 4)
		case "tx2d":
			return clampInt(eobTx2DCtx(info), 0, 4)
		case "tx2d-1", "tx2dminus1":
			return clampInt(eobTx2DCtx(info)-1, 0, 4)
		}
	}
	return clampInt(int(info.Ctx), 0, 4)
}

func eobBaseCtx(info TxfmInfo, eob int) int {
	tx2dCtx := eobTx2DCtx(info)
	spec := clampInt(1+boolToInt(eob > (2<<tx2dCtx))+boolToInt(eob > (4<<tx2dCtx)), 0, 3)
	if override := strings.TrimSpace(os.Getenv("DEBUG_EOB_BASE_CTX")); override != "" {
		if idx, err := strconv.Atoi(override); err == nil {
			return clampInt(idx, 0, 3)
		}
		switch strings.ToLower(override) {
		case "legacy", "area":
			if !debugEOBBaseCtxTxMatch(info) {
				return 2
			}
			return legacyEOBBaseCtx(info, eob)
		case "stable":
			return 2
		case "spec", "dav1d", "default":
			return spec
		}
	}
	return spec
}

func debugEOBBaseCtxTxMatch(info TxfmInfo) bool {
	spec := strings.TrimSpace(os.Getenv("DEBUG_EOB_BASE_CTX_TX"))
	if spec == "" {
		return true
	}
	shape := strconv.Itoa(int(info.W4)*4) + "x" + strconv.Itoa(int(info.H4)*4)
	for _, item := range strings.Split(spec, ",") {
		if strings.EqualFold(strings.TrimSpace(item), shape) {
			return true
		}
	}
	return false
}

func legacyEOBBaseCtx(info TxfmInfo, eob int) int {
	sw := minInt(int(info.W4), int(TxfmInfoFor(TX32X32).W4))
	sh := minInt(int(info.H4), int(TxfmInfoFor(TX32X32).H4))
	return clampInt(1+boolToInt(eob > sw*sh*2)+boolToInt(eob > sw*sh*4), 0, 3)
}

func baseTokCtx(ctx int) int {
	if override := strings.TrimSpace(os.Getenv("DEBUG_BASETOK_CTX")); override != "" {
		if idx, err := strconv.Atoi(override); err == nil {
			return clampInt(idx, 0, 40)
		}
	}
	return clampInt(ctx, 0, 40)
}

func brTokCtx(ctx int) int {
	if override := strings.TrimSpace(os.Getenv("DEBUG_BRTOK_CTX")); override != "" {
		if idx, err := strconv.Atoi(override); err == nil {
			return clampInt(idx, 0, 20)
		}
	}
	return clampInt(ctx, 0, 20)
}

type coeffOrderMeta struct {
	levelIdx  uint16
	coeffIdx  uint16
	rc        uint16
	loCtxBase uint8
	hiBase    uint8
}

type coeffDecodeMeta struct {
	stride        int
	levelsBufSize int
	is2D          bool
	orders        []coeffOrderMeta
}

const invalidCoeffIndex = ^uint16(0)

var coeffDecodeMetaCache = buildCoeffDecodeMetaCache()

func DecodeLumaCoefficientsDCT2D(bs BlockSize, g BlockGeometry, tx TxfmSize, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) ([]int32, uint8, bool, error) {
	return DecodeLumaCoefficients(bs, g, tx, TxTypeDCTDCT, segID, coef, dq, above, left, dec)
}

func DecodeLumaCoefficients(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) ([]int32, uint8, bool, error) {
	return DecodeLumaCoefficientsWithBitDepth(bs, g, tx, txType, segID, coef, dq, above, left, dec, 8)
}

func DecodeLumaCoefficientsWithBitDepth(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) ([]int32, uint8, bool, error) {
	coeffs := make([]int32, coeffBufferLen(tx))
	resCtx, skip, err := DecodeLumaCoefficientsIntoWithBitDepth(coeffs, bs, g, tx, txType, segID, coef, dq, above, left, dec, bitDepth)
	if err != nil {
		return nil, 0, false, err
	}
	return coeffs, resCtx, skip, nil
}

func DecodeLumaCoefficientsInto(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) (uint8, bool, error) {
	return DecodeLumaCoefficientsIntoWithBitDepth(coeffs, bs, g, tx, txType, segID, coef, dq, above, left, dec, 8)
}

func DecodeLumaCoefficientsIntoWithBitDepth(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) (uint8, bool, error) {
	if coef == nil {
		return 0, false, fmt.Errorf("decoder: coefficient cdf required")
	}
	if above == nil || left == nil {
		return 0, false, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return 0, false, fmt.Errorf("decoder: entropy decoder required")
	}
	if int(segID) >= len(dq) {
		return 0, false, fmt.Errorf("decoder: invalid segment id %d", segID)
	}
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31

	return decodeCoefficientsInto(coeffs, bs, g, tx, txType, segID, 0, false, av1.Chroma420, coef, dq, above.LCoef[bx4:], left.LCoef[by4:], dec, true, bitDepth)
}

func DecodeLumaCoefficientsNoSkip(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) ([]int32, uint8, bool, error) {
	return DecodeLumaCoefficientsNoSkipWithBitDepth(bs, g, tx, txType, segID, coef, dq, above, left, dec, 8)
}

func DecodeLumaCoefficientsNoSkipWithBitDepth(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) ([]int32, uint8, bool, error) {
	coeffs := make([]int32, coeffBufferLen(tx))
	resCtx, skip, err := DecodeLumaCoefficientsNoSkipIntoWithBitDepth(coeffs, bs, g, tx, txType, segID, coef, dq, above, left, dec, bitDepth)
	if err != nil {
		return nil, 0, false, err
	}
	return coeffs, resCtx, skip, nil
}

func DecodeLumaCoefficientsNoSkipInto(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) (uint8, bool, error) {
	return DecodeLumaCoefficientsNoSkipIntoWithBitDepth(coeffs, bs, g, tx, txType, segID, coef, dq, above, left, dec, 8)
}

func DecodeLumaCoefficientsNoSkipIntoWithBitDepth(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) (uint8, bool, error) {
	if coef == nil {
		return 0, false, fmt.Errorf("decoder: coefficient cdf required")
	}
	if above == nil || left == nil {
		return 0, false, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return 0, false, fmt.Errorf("decoder: entropy decoder required")
	}
	if int(segID) >= len(dq) {
		return 0, false, fmt.Errorf("decoder: invalid segment id %d", segID)
	}
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31

	return decodeCoefficientsInto(coeffs, bs, g, tx, txType, segID, 0, false, av1.Chroma420, coef, dq, above.LCoef[bx4:], left.LCoef[by4:], dec, false, bitDepth)
}

func decodeCoefficients(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, dqPlane int, chroma bool, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, aboveCoef, leftCoef []uint8, dec coefficientEntropyDecoder, readSkip bool) ([]int32, uint8, bool, error) {
	coeffs := make([]int32, coeffBufferLen(tx))
	resCtx, skip, err := decodeCoefficientsInto(coeffs, bs, g, tx, txType, segID, dqPlane, chroma, layout, coef, dq, aboveCoef, leftCoef, dec, readSkip, 8)
	if err != nil {
		return nil, 0, false, err
	}
	return coeffs, resCtx, skip, nil
}

func decodeCoefficientsInto(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, dqPlane int, chroma bool, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, aboveCoef, leftCoef []uint8, dec coefficientEntropyDecoder, readSkip bool, bitDepth uint8) (uint8, bool, error) {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	scan := ScanOrder(tx)
	scanWidth, scanHeight := ScanCoeffArea(tx)
	orderCount := scanWidth * scanHeight
	if len(scan) == 0 {
		return 0, false, fmt.Errorf("decoder: missing scan order for tx %d", tx)
	}
	if len(scan) < orderCount {
		return 0, false, fmt.Errorf("decoder: short scan order for tx %d: have %d, need %d", tx, len(scan), orderCount)
	}
	if len(coeffs) < width*height {
		return 0, false, fmt.Errorf("decoder: coefficient buffer too small for %dx%d tx", width, height)
	}
	coeffs = coeffs[:width*height]
	traceTarget := debugMovieTraceCoeffEnabled && debugTraceMovieBlock(g.Start4X*4, g.Start4Y*4, width, height)
	traceEventLimit := 0
	traceEventCount := 0
	traceEvent := func(format string, args ...interface{}) {}
	if traceTarget {
		traceEventLimit = debugCoeffTraceEventLimit()
		if traceEventLimit > 0 {
			traceEvent = func(format string, args ...interface{}) {
				if traceEventCount >= traceEventLimit {
					return
				}
				fmt.Fprintf(os.Stderr, format, args...)
				traceEventCount++
			}
		}
	}
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace coeff xy=(%d,%d) above=%v left=%v txType=%d readSkip=%v\n",
			g.Start4X*4, g.Start4Y*4,
			append([]uint8(nil), aboveCoef[:minInt(len(aboveCoef), 4)]...), append([]uint8(nil), leftCoef[:minInt(len(leftCoef), 4)]...), txType, readSkip)
	}
	if readSkip {
		skipCtx := GetSkipCtx(info, bs, aboveCoef, leftCoef, chroma, layout)
		skipRow := coeffCDFRow(int(info.Ctx), "skip", len(coef.Skip)-1)
		if debugCoeffSkipTargetMatches(g, chroma, dqPlane) {
			skipRow = debugCoeffSkipOverrideValue("DEBUG_COEF_SKIP_ROW_TARGET", skipRow, len(coef.Skip)-1)
			skipCtx = debugCoeffSkipOverrideValue("DEBUG_COEF_SKIP_CTX_TARGET", skipCtx, len(coef.Skip[skipRow])-1)
		}
		if traceTarget {
			fmt.Fprintf(os.Stderr, "trace coeff readSkip skipCtx=%d skipRow=%d\n", skipCtx, skipRow)
		}
		skipCDF := coef.Skip[skipRow][skipCtx][:]
		if debugCoeffSkipDisableCDFUpdateTargetMatches(g, chroma, dqPlane) {
			skipCDF = append([]uint16(nil), skipCDF...)
		}
		if coeffDecodeBoolAdapt(dec, skipCDF, "skip") != 0 {
			return 0x40, true, nil
		}
	}

	txClass := generatedTXTypeClass[txType]
	if traceTarget {
		traceEOBCoeffCount(info, coef, dec, chroma, txClass != TxClass2D)
	}
	eob, err := decodeEOBCoeffCount(info, coef, dec, chroma, txClass != TxClass2D)
	if err != nil {
		return 0, false, err
	}
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace coeff post eob=%d txClass=%d scan0=%v\n", eob, txClass, append([]uint16(nil), scan[:minInt(len(scan), 16)]...))
	}
	if eob >= orderCount {
		return 0, false, fmt.Errorf("decoder: eob=%d exceeds coded scan size %d", eob, orderCount)
	}

	chromaIdx := boolToInt(chroma)
	eobRow := coeffCDFRowForCoef(coef, int(info.Ctx), "eob-base", len(coef.EOBBaseTok)-1)
	defaultBRRow := coeffCDFRowForCoef(coef, minInt(int(info.Ctx), 3), "br", len(coef.BRTok)-1)
	baseRow := coeffCDFRowForCoef(coef, int(info.Ctx), "base", len(coef.BaseTok)-1)
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace coeff rows qcat=%d baseRow=%d brRow=%d eobRow=%d\n", coef.QCat, baseRow, defaultBRRow, eobRow)
	}
	eobCDF := &coef.EOBBaseTok[eobRow][chromaIdx]
	loCDF := &coef.BaseTok[baseRow][chromaIdx]
	hiCDFForOrder := func(order int) (int, *[21][4]uint16) {
		row := coeffBRCDFRowForOrder(coef, minInt(int(info.Ctx), 3), order, len(coef.BRTok)-1)
		return row, &coef.BRTok[row][chromaIdx]
	}
	meta := coeffDecodeMetaCache[tx][txClass]

	var dcTok uint32
	var levelsByRC []uint8
	var levelsByRCScratch *scratchBuffer[uint8]
	if eob != 0 {
		levelsByRCScratch = takeScratch[uint8](&coeffTokenScratchPool, len(scan))
		defer putZeroScratch(&coeffTokenScratchPool, levelsByRCScratch)
		levelsByRC = levelsByRCScratch.buf
		ctx := debugEOBBaseTargetCtx(g, chroma, dqPlane, info, eob)
		if traceTarget {
			fmt.Fprintf(os.Stderr, "trace coeff eob-base current-ctx=%d eob=%d\n", ctx, eob)
			if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
				for altCtx := 0; altCtx < len(*eobCDF); altCtx++ {
					altDec := cloneable.Clone()
					altCDF := append([]uint16(nil), (*eobCDF)[altCtx][:]...)
					altTok := altDec.DecodeSymbolAdapt(altCDF, 2)
					fmt.Fprintf(os.Stderr, "trace coeff eob-base alt-ctx=%d tok=%d\n", altCtx, altTok)
				}
			}
		}
		eobTok := coeffDecodeSymbolAdapt(dec, (*eobCDF)[ctx][:], 2, "eob-base")
		tok := eobTok + 1
		orderMeta := meta.orders[eob]
		stride := meta.stride
		levelIdx := int(orderMeta.levelIdx)
		rcFirstU := uint32(orderMeta.rc)
		levelsScratch := takeScratch[uint8](&coeffLevelsScratchPool, meta.levelsBufSize)
		defer putZeroScratch(&coeffLevelsScratchPool, levelsScratch)
		levels := levelsScratch.buf
		rcFirst := int(rcFirstU)
		levelTok := uint8(tok * 0x41)
		if eobTok == 2 {
			hiCtx := int(orderMeta.hiBase)
			hiRow, hiCDF := hiCDFForOrder(eob)
			if traceTarget {
				fmt.Fprintf(os.Stderr, "trace coeff eob-hi current-hictx=%d rc=%d row=%d\n", hiCtx, rcFirst, hiRow)
				if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
					for altHiCtx := 0; altHiCtx < len(hiCDF); altHiCtx++ {
						altDec := cloneable.Clone()
						altCDF := append([]uint16(nil), (*hiCDF)[altHiCtx][:]...)
						altTok := altDec.DecodeHiTok(altCDF)
						fmt.Fprintf(os.Stderr, "trace coeff eob-hi alt-hictx=%d tok=%d\n", altHiCtx, altTok)
					}
				}
			}
			tok = coeffDecodeHiTok(dec, (*hiCDF)[brTokCtx(hiCtx)][:], "br")
			levelTok = uint8(tok + (3 << 6))
			traceEvent("trace coeff event kind=eob-hi order=%d rc=%d loCtx=%d brRow=%d hiCtx=%d tok=%d\n", eob, rcFirst, ctx, hiRow, hiCtx, tok)
		} else {
			traceEvent("trace coeff event kind=eob-base order=%d rc=%d loCtx=%d tok=%d\n", eob, rcFirst, ctx, tok)
		}
		levelsByRC[rcFirst] = uint8(tok)
		levels[levelIdx] = levelTok

		for i := eob - 1; i > 0; i-- {
			orderMeta := meta.orders[i]
			levelIdx := int(orderMeta.levelIdx)
			rcI := int(orderMeta.rc)
			ctx, hiMag := encodedLoCtxFast(levels[levelIdx:], meta.is2D, int(orderMeta.loCtxBase), stride)
			traceProbe := traceTarget && (i == eob-1 || rcI == 4 || i == 1)
			if traceProbe {
				fmt.Fprintf(os.Stderr, "trace coeff ac-first current-loctx=%d rc=%d\n", ctx, rcI)
			}
			ctx = baseTokCtx(ctx)
			if traceProbe {
				if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
					for altCtx := 0; altCtx < len(loCDF); altCtx++ {
						altDec := cloneable.Clone()
						altCDF := append([]uint16(nil), (*loCDF)[altCtx][:]...)
						altTok := altDec.DecodeSymbolAdapt(altCDF, 3)
						fmt.Fprintf(os.Stderr, "trace coeff ac-first alt-loctx=%d tok=%d\n", altCtx, altTok)
					}
				}
			}
			baseTok := coeffDecodeSymbolAdapt(dec, (*loCDF)[ctx][:], 3, "base")
			tok = baseTok
			levelTok = uint8(tok * 0x41)
			hiCtxLogged := -1
			hiRowLogged := -1
			if tok == 3 {
				hiCtx := int(orderMeta.hiBase) + hiTokenContextOffset(hiMag&0x3f)
				hiRow, hiCDF := hiCDFForOrder(i)
				if traceProbe {
					fmt.Fprintf(os.Stderr, "trace coeff ac-first current-hictx=%d row=%d\n", hiCtx, hiRow)
				}
				hiCtx = brTokCtx(hiCtx)
				hiCtxLogged = hiCtx
				hiRowLogged = hiRow
				if traceProbe {
					if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
						for altHiCtx := 0; altHiCtx < len(hiCDF); altHiCtx++ {
							altDec := cloneable.Clone()
							altCDF := append([]uint16(nil), (*hiCDF)[altHiCtx][:]...)
							altTok := altDec.DecodeHiTok(altCDF)
							fmt.Fprintf(os.Stderr, "trace coeff ac-first alt-hictx=%d tok=%d\n", altHiCtx, altTok)
						}
					}
				}
				tok = coeffDecodeHiTok(dec, (*hiCDF)[hiCtx][:], "br")
				levelTok = uint8(tok + (3 << 6))
			}
			if baseTok != 0 || tok != 0 || debugCoeffTraceOrder(i) {
				traceEvent("trace coeff event kind=ac order=%d rc=%d loCtx=%d baseTok=%d brRow=%d hiCtx=%d tok=%d\n", i, rcI, ctx, baseTok, hiRowLogged, hiCtxLogged, tok)
			}
			levels[levelIdx] = levelTok
			if tok != 0 {
				levelsByRC[rcI] = uint8(tok)
			}
		}

		if txClass == TxClass2D {
			if traceTarget {
				if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
					for altCtx := 0; altCtx < len(loCDF); altCtx++ {
						altDec := cloneable.Clone()
						altCDF := append([]uint16(nil), (*loCDF)[altCtx][:]...)
						altTok := altDec.DecodeSymbolAdapt(altCDF, 3)
						fmt.Fprintf(os.Stderr, "trace coeff dc alt-loctx=%d tok=%d\n", altCtx, altTok)
					}
				}
			}
			dcBaseTok := coeffDecodeSymbolAdapt(dec, (*loCDF)[baseTokCtx(0)][:], 3, "base")
			dcTok = dcBaseTok
			hiCtxLogged := -1
			hiRowLogged := -1
			if dcTok == 3 {
				hiMag := uint32(levels[1]) + uint32(levels[stride]) + uint32(levels[stride+1])
				dcHiCtx := hiTokenContextOffset(hiMag & 0x3f)
				hiRow, hiCDF := hiCDFForOrder(0)
				if traceTarget {
					fmt.Fprintf(os.Stderr, "trace coeff dc-hi current-hictx=%d row=%d\n", dcHiCtx, hiRow)
					if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
						for altHiCtx := 0; altHiCtx < len(hiCDF); altHiCtx++ {
							altDec := cloneable.Clone()
							altCDF := append([]uint16(nil), (*hiCDF)[altHiCtx][:]...)
							altTok := altDec.DecodeHiTok(altCDF)
							fmt.Fprintf(os.Stderr, "trace coeff dc-hi alt-hictx=%d tok=%d\n", altHiCtx, altTok)
						}
					}
				}
				dcTok = coeffDecodeHiTok(dec, (*hiCDF)[brTokCtx(dcHiCtx)][:], "br")
				hiCtxLogged = brTokCtx(dcHiCtx)
				hiRowLogged = hiRow
			}
			traceEvent("trace coeff event kind=dc loCtx=%d baseTok=%d brRow=%d hiCtx=%d tok=%d\n", baseTokCtx(0), dcBaseTok, hiRowLogged, hiCtxLogged, dcTok)
		} else {
			ctx, hiMag := encodedLoCtxFast(levels, false, int(meta.orders[0].loCtxBase), stride)
			dcBaseTok := coeffDecodeSymbolAdapt(dec, (*loCDF)[baseTokCtx(ctx)][:], 3, "base")
			dcTok = dcBaseTok
			hiCtxLogged := -1
			hiRowLogged := -1
			if dcTok == 3 {
				hiRow, hiCDF := hiCDFForOrder(0)
				hiCtxLogged = brTokCtx(hiTokenContextOffset(hiMag & 0x3f))
				dcTok = coeffDecodeHiTok(dec, (*hiCDF)[hiCtxLogged][:], "br")
				hiRowLogged = hiRow
			}
			traceEvent("trace coeff event kind=dc loCtx=%d baseTok=%d brRow=%d hiCtx=%d tok=%d\n", baseTokCtx(ctx), dcBaseTok, hiRowLogged, hiCtxLogged, dcTok)
		}
	} else {
		tokBR := coeffDecodeSymbolAdapt(dec, (*eobCDF)[0][:], 2, "eob-base")
		dcTok = tokBR + 1
		if tokBR == 2 {
			hiRow, hiCDF := hiCDFForOrder(0)
			hiCtx := brTokCtx(0)
			dcTok = coeffDecodeHiTok(dec, (*hiCDF)[hiCtx][:], "br")
			traceEvent("trace coeff event kind=dc-eob baseTok=%d brRow=%d hiCtx=%d tok=%d\n", tokBR, hiRow, hiCtx, dcTok)
		} else {
			traceEvent("trace coeff event kind=dc-eob baseTok=%d brRow=%d hiCtx=%d tok=%d\n", tokBR, -1, -1, dcTok)
		}
	}

	dqShift := maxIntLocal(0, int(info.Ctx)-2)
	cfMax := coeffAbsMaxForBitDepth(bitDepth)
	culLevel := uint32(0)
	dcSignLevel := uint8(1 << 6)
	dcSignCDF := &coef.DCSign[chromaIdx]

	if dcTok != 0 {
		dcSignCtx := GetDCSignCtx(tx, aboveCoef, leftCoef)
		if traceTarget {
			if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
				for altSignCtx := 0; altSignCtx < len(*dcSignCDF); altSignCtx++ {
					altDec := cloneable.Clone()
					altCDF := append([]uint16(nil), (*dcSignCDF)[altSignCtx][:]...)
					altBit := altDec.DecodeBoolAdapt(altCDF)
					fmt.Fprintf(os.Stderr, "trace coeff dc-sign alt-ctx=%d bit=%d\n", altSignCtx, altBit)
				}
			}
		}
		neg := coeffDecodeBoolAdapt(dec, (*dcSignCDF)[dcSignCtx][:], "dc-sign") != 0
		if traceTarget {
			fmt.Fprintf(os.Stderr, "trace coeff dcTok=%d dcSignCtx=%d neg=%v dqdc=%d dqShift=%d\n", dcTok, dcSignCtx, neg, dq[segID][dqPlane][0], dqShift)
		}
		tok := int(dcTok)
		if tok == 15 {
			tok = int(decodeGolomb(dec.DecodeBoolEqui)) + 15
		}
		dc := (int(dq[segID][dqPlane][0]) * tok) >> dqShift
		if neg {
			dc = minInt(dc, cfMax+1)
			dc = -dc
			dcSignLevel = 0
		} else {
			dc = minInt(dc, cfMax)
			dcSignLevel = 2 << 6
		}
		dcIdx := int(meta.orders[0].coeffIdx)
		if meta.orders[0].coeffIdx == invalidCoeffIndex {
			dcIdx, err = CoeffIndexFromScan(tx, uint32(meta.orders[0].rc))
			if err != nil {
				return 0, false, err
			}
		}
		coeffs[dcIdx] = int32(dc)
		culLevel = uint32(tok)
	}

	if eob != 0 {
		acDQ := int(dq[segID][dqPlane][1])
		for i := 1; i <= eob; i++ {
			orderMeta := meta.orders[i]
			rcI := uint32(orderMeta.rc)
			tok := int(levelsByRC[rcI])
			if tok == 0 {
				continue
			}
			sign := dec.DecodeBoolEqui() != 0
			if tok == 15 {
				tok = int(decodeGolomb(dec.DecodeBoolEqui)) + 15
			}
			if traceTarget {
				fmt.Fprintf(os.Stderr, "trace coeff ac order=%d rc=%d tok=%d sign=%v dqa=%d\n", i, rcI, tok, sign, acDQ)
			}
			ac := (acDQ * tok) >> dqShift
			if sign {
				ac = minInt(ac, cfMax+1)
				ac = -ac
			} else {
				ac = minInt(ac, cfMax)
			}
			coeffIdx := int(orderMeta.coeffIdx)
			if orderMeta.coeffIdx == invalidCoeffIndex {
				coeffIdx, err = CoeffIndexFromScan(tx, rcI)
				if err != nil {
					return 0, false, err
				}
			}
			coeffs[coeffIdx] = int32(ac)
			culLevel += uint32(tok)
		}
	}

	return uint8(minInt(int(culLevel), 63)) | dcSignLevel, false, nil
}

func coeffAbsMaxForBitDepth(bitDepth uint8) int {
	if bitDepth < 8 {
		bitDepth = 8
	}
	return (1 << (int(bitDepth) + 7)) - 1
}

func buildCoeffDecodeMetaCache() [numRectTxfmSizes][3]coeffDecodeMeta {
	var out [numRectTxfmSizes][3]coeffDecodeMeta
	for tx := TxfmSize(0); tx < numRectTxfmSizes; tx++ {
		scan := ScanOrder(tx)
		if len(scan) == 0 {
			continue
		}
		scanWidth, scanHeight := ScanCoeffArea(tx)
		for txClass := TxClass2D; txClass <= TxClassV; txClass++ {
			stride, _, _, _, _ := coeffDecodeLayout(txClass, tx, 0, scan)
			orderCount := scanWidth * scanHeight
			meta := coeffDecodeMeta{
				stride:        stride,
				levelsBufSize: levelsBufferSize(txClass, scanWidth, scanHeight, stride),
				is2D:          txClass == TxClass2D,
				orders:        make([]coeffOrderMeta, orderCount),
			}
			loCtxOffsets := &generatedLoContextOffsets[loContextOffsetGroup(tx)]
			for order := 0; order < orderCount; order++ {
				stride, levelIdx, axisVal, rcI, _ := coeffDecodeLayout(txClass, tx, order, scan)
				_ = stride
				x := axisXForClass(txClass, tx, order, scan)
				y := axisYForClass(txClass, tx, order, scan)
				coeffIdx := int(invalidCoeffIndex)
				if idx, err := CoeffIndexFromScan(tx, rcI); err == nil {
					coeffIdx = idx
				}
				loBase := 0
				hiBase := 7
				if txClass == TxClass2D {
					loBase = int(loCtxOffsets[minInt(y, 4)][minInt(x, 4)])
					if (x | y) > 1 {
						hiBase = 14
					}
				} else {
					loBase = 26 + minInt(axisVal, 2)*5
					if axisVal > 0 {
						hiBase = 14
					}
				}
				meta.orders[order] = coeffOrderMeta{
					levelIdx:  uint16(levelIdx),
					coeffIdx:  uint16(coeffIdx),
					rc:        uint16(rcI),
					loCtxBase: uint8(loBase),
					hiBase:    uint8(hiBase),
				}
			}
			out[tx][txClass] = meta
		}
	}
	return out
}

func coeffDecodeLayout(txClass TxClass, tx TxfmSize, order int, scan []uint16) (stride, levelIdx, axisVal int, rcI, rc uint32) {
	info := TxfmInfoFor(tx)
	slw := minInt(int(info.LW), int(TxfmInfoFor(TX32X32).LW))
	slh := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	switch txClass {
	case TxClass2D:
		stride = 4 << slh
		rc = uint32(scan[order])
		shift := slh + 2
		mask := (4 << slh) - 1
		x := int(rc) >> shift
		y := int(rc) & mask
		return stride, int(rc), x | y, rc, rc
	case TxClassH:
		stride = 16
		shift := slh + 2
		mask := (4 << slh) - 1
		x := order & mask
		y := order >> shift
		rc = uint32(order)
		return stride, x*stride + y, y, rc, rc
	case TxClassV:
		stride = 16
		shift := slw + 2
		shift2 := slh + 2
		mask := (4 << slw) - 1
		x := order & mask
		y := order >> shift
		rc = uint32((x << shift2) | y)
		return stride, x*stride + y, y, rc, rc
	default:
		return 0, 0, 0, 0, 0
	}
}

func axisXForClass(txClass TxClass, tx TxfmSize, order int, scan []uint16) int {
	info := TxfmInfoFor(tx)
	slw := minInt(int(info.LW), int(TxfmInfoFor(TX32X32).LW))
	slh := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	switch txClass {
	case TxClass2D:
		rc := uint32(scan[order])
		return int(rc) >> (slh + 2)
	case TxClassH, TxClassV:
		mask := (4 << slh) - 1
		if txClass == TxClassV {
			mask = (4 << slw) - 1
		}
		return order & mask
	default:
		return 0
	}
}

func axisYForClass(txClass TxClass, tx TxfmSize, order int, scan []uint16) int {
	info := TxfmInfoFor(tx)
	slw := minInt(int(info.LW), int(TxfmInfoFor(TX32X32).LW))
	slh := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	switch txClass {
	case TxClass2D:
		rc := uint32(scan[order])
		return int(rc) & ((4 << slh) - 1)
	case TxClassH:
		return order >> (slh + 2)
	case TxClassV:
		return order >> (slw + 2)
	default:
		return 0
	}
}

func levelsBufferSize(txClass TxClass, scanWidth, scanHeight, stride int) int {
	switch txClass {
	case TxClass2D:
		return stride * (scanWidth + 2)
	case TxClassH:
		return stride * (scanHeight + 2)
	case TxClassV:
		return stride * (scanWidth + 2)
	default:
		return stride * (scanWidth + 2)
	}
}

func decodeEOBCoeffCount(info TxfmInfo, coef *CoefCDF, dec coefficientEntropyDecoder, chroma bool, is1D bool) (int, error) {
	tx2dCtx := eobTx2DCtx(info)
	c := boolToInt(chroma)
	is1 := boolToInt(is1D)

	eobBin := 0
	switch tx2dCtx {
	case 0:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin16[c][is1][:], 4, "eob-bin"))
	case 1:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin32[c][is1][:], 5, "eob-bin"))
	case 2:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin64[c][is1][:], 6, "eob-bin"))
	case 3:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin128[c][is1][:], 7, "eob-bin"))
	case 4:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin256[c][is1][:], 8, "eob-bin"))
	case 5:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin512[c][:], 9, "eob-bin"))
	case 6:
		eobBin = int(coeffDecodeSymbolAdapt(dec, coef.EOBBin1024[c][:], 10, "eob-bin"))
	default:
		return 0, fmt.Errorf("decoder: unsupported tx2d context %d", tx2dCtx)
	}
	if eobBin > 1 {
		bin := eobBin - 2
		hiBit := coeffDecodeBoolAdapt(dec, coef.EOBHiBit[eobHiBitCtx(info)][c][bin][:], "eob-hi")
		return ((int(hiBit) | 2) << bin) | int(decodeEquiBits(dec, bin)), nil
	}
	return eobBin, nil
}

func traceEOBCoeffCount(info TxfmInfo, coef *CoefCDF, dec coefficientEntropyDecoder, chroma bool, is1D bool) {
	cloneable, ok := dec.(interface{ Clone() *msac.Context })
	if !ok {
		return
	}
	altDec := cloneable.Clone()
	if altDec == nil {
		return
	}

	tx2dCtx := eobTx2DCtx(info)
	c := boolToInt(chroma)
	is1 := boolToInt(is1D)

	var (
		binCDF []uint16
		n      int
	)
	switch tx2dCtx {
	case 0:
		binCDF = append([]uint16(nil), coef.EOBBin16[c][is1][:]...)
		n = 4
	case 1:
		binCDF = append([]uint16(nil), coef.EOBBin32[c][is1][:]...)
		n = 5
	case 2:
		binCDF = append([]uint16(nil), coef.EOBBin64[c][is1][:]...)
		n = 6
	case 3:
		binCDF = append([]uint16(nil), coef.EOBBin128[c][is1][:]...)
		n = 7
	case 4:
		binCDF = append([]uint16(nil), coef.EOBBin256[c][is1][:]...)
		n = 8
	case 5:
		binCDF = append([]uint16(nil), coef.EOBBin512[c][:]...)
		n = 9
	case 6:
		binCDF = append([]uint16(nil), coef.EOBBin1024[c][:]...)
		n = 10
	default:
		return
	}

	if state, ok := dec.(interface {
		DebugState() (uint32, uint32, int, int)
	}); ok {
		cur, rng, cnt, pos := state.DebugState()
		fmt.Fprintf(os.Stderr, "trace coeff eob-bin-cdf tx2d=%d chroma=%d is1d=%d n=%d cdf=%v state=[%d %d %d %d]\n", tx2dCtx, c, is1, n, binCDF[:minInt(len(binCDF), n+1)], cur, rng, cnt, pos)
	} else {
		fmt.Fprintf(os.Stderr, "trace coeff eob-bin-cdf tx2d=%d chroma=%d is1d=%d n=%d cdf=%v\n", tx2dCtx, c, is1, n, binCDF[:minInt(len(binCDF), n+1)])
	}
	eobBin := int(altDec.DecodeSymbolAdapt(binCDF, n))
	if eobBin <= 1 {
		fmt.Fprintf(os.Stderr, "trace coeff eob-components bin=%d final=%d\n", eobBin, eobBin)
		return
	}

	hiCDF := append([]uint16(nil), coef.EOBHiBit[eobHiBitCtx(info)][c][eobBin-2][:]...)
	hiBit := altDec.DecodeBoolAdapt(hiCDF)
	var equi uint32
	for i := 0; i < eobBin-2; i++ {
		equi = (equi << 1) | altDec.DecodeBoolEqui()
	}
	final := ((int(hiBit) | 2) << (eobBin - 2)) | int(equi)
	fmt.Fprintf(os.Stderr, "trace coeff eob-components bin=%d hiBit=%d equi=%d final=%d\n", eobBin, hiBit, equi, final)
}

func decodeEquiBits(dec coefficientEntropyDecoder, n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		v = (v << 1) | dec.DecodeBoolEqui()
	}
	return v
}

func encodedLoCtx(levels []uint8, txClass TxClass, loCtxOffsets *[5][5]uint8, x, y, axisVal, stride int) (ctx int, hiMag uint32) {
	mag := uint32(coeffContextLevel(levels, stride, 0, 1)) + uint32(coeffContextLevel(levels, stride, 1, 0))
	if txClass == TxClass2D {
		mag += uint32(coeffContextLevel(levels, stride, 1, 1))
		hiMag = mag
		mag += uint32(coeffContextLevel(levels, stride, 0, 2)) + uint32(coeffContextLevel(levels, stride, 2, 0))
		return int(loCtxOffsets[minInt(y, 4)][minInt(x, 4)]) + minInt(int((mag+64)>>7), 4), hiMag
	}
	mag += uint32(coeffContextLevel(levels, stride, 0, 2))
	hiMag = mag
	mag += uint32(coeffContextLevel(levels, stride, 0, 3)) + uint32(coeffContextLevel(levels, stride, 0, 4))
	return 26 + minInt(axisVal, 2)*5 + minInt(int((mag+64)>>7), 4), hiMag
}

func encodedLoCtxFast(levels []uint8, is2D bool, loCtxBase, stride int) (ctx int, hiMag uint32) {
	mag := uint32(coeffContextLevel(levels, stride, 0, 1)) + uint32(coeffContextLevel(levels, stride, 1, 0))
	if is2D {
		mag += uint32(coeffContextLevel(levels, stride, 1, 1))
		hiMag = mag
		mag += uint32(coeffContextLevel(levels, stride, 0, 2)) + uint32(coeffContextLevel(levels, stride, 2, 0))
		return loCtxBase + minInt(int((mag+64)>>7), 4), hiMag
	}
	mag += uint32(coeffContextLevel(levels, stride, 0, 2))
	hiMag = mag
	mag += uint32(coeffContextLevel(levels, stride, 0, 3)) + uint32(coeffContextLevel(levels, stride, 0, 4))
	return loCtxBase + minInt(int((mag+64)>>7), 4), hiMag
}

func encodedHiCtx(txClass TxClass, x, y, axisVal int, hiMag uint32) int {
	pos := axisVal
	if txClass == TxClass2D {
		pos = x | y
	}
	base := 7
	limit := 0
	if txClass == TxClass2D {
		limit = 1
	}
	if pos > limit {
		base = 14
	}
	return base + hiTokenContextOffset(hiMag&0x3f)
}

func hiTokenContextOffset(mag uint32) int {
	return minInt(int((mag+1)>>1), 6)
}
