package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func paletteAllowedForBlock(hdr *obu.FrameHeader, g BlockGeometry) bool {
	if hdr == nil || !hdr.AllowScreenTools {
		return false
	}
	if g.Width4() <= 0 || g.Height4() <= 0 {
		return false
	}
	dims := BlockDims(g.Size)
	return maxIntLocal(int(dims.W4), int(dims.H4)) <= 16 && int(dims.W4)+int(dims.H4) >= 4
}

func paletteSizeContext(g BlockGeometry) int {
	dims := BlockDims(g.Size)
	ctx := int(dims.Log2W4) + int(dims.Log2H4) - 2
	if ctx < 0 {
		return 0
	}
	return ctx
}

func readPalettePlaneSyntax(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, plane, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, error) {
	if cdf == nil || above == nil || left == nil || dec == nil {
		return 0, [8]uint16{}, fmt.Errorf("decoder: palette plane syntax requires decoder state")
	}
	if bitDepth == 0 {
		bitDepth = 8
	}

	palSize := int(dec.DecodeSymbolAdapt(cdf.PalSize[plane][sizeCtx][:], 6)) + 2
	cache, nCache := paletteCache(above, left, plane, bx4, by4)
	cache, nCache = debugApplyPaletteCacheMode(cache, nCache)
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
			minStep := 1
			if plane != 0 {
				minStep = 0
			}
			if plane == 0 {
				bits = maxIntLocal(1, bits+debugPaletteYBitsAdjust())
				if override, ok := debugPaletteYMinStepOverride(); ok {
					minStep = override
				}
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

func debugPaletteYBitsAdjust() int {
	raw := strings.TrimSpace(os.Getenv("DEBUG_PALETTE_Y_BITS_ADJUST"))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return v
}

func debugPaletteYMinStepOverride() (int, bool) {
	raw := strings.TrimSpace(os.Getenv("DEBUG_PALETTE_Y_MIN_STEP"))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

func debugApplyPaletteCacheMode(cache [16]uint16, nCache int) ([16]uint16, int) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_PALETTE_CACHE_MODE"))) {
	case "", "0", "off", "default":
		return cache, nCache
	case "disable", "disabled", "none", "no_cache":
		return cache, 0
	case "reverse", "reverse_diag":
		for i, j := 0, nCache-1; i < j; i, j = i+1, j-1 {
			cache[i], cache[j] = cache[j], cache[i]
		}
		return cache, nCache
	default:
		return cache, nCache
	}
}

func readPaletteUVSyntax(bitDepth uint8, cdf *ModeCDF, above, left *BlockContext, sizeCtx, bx4, by4 int, dec intraEntropyDecoder) (uint8, [8]uint16, [8]uint16, error) {
	if bitDepth == 0 {
		bitDepth = 8
	}
	palSize, palU, err := readPalettePlaneSyntax(bitDepth, cdf, above, left, 1, sizeCtx, bx4, by4, dec)
	if err != nil {
		return 0, [8]uint16{}, [8]uint16{}, err
	}

	var palV [8]uint16
	bpc := int(bitDepth)
	maxValue := (1 << bpc) - 1
	if dec.DecodeBoolEqui() != 0 {
		bits := bpc - 4 + int(dec.DecodeBools(2))
		prev := int(dec.DecodeBools(uint(bpc)))
		palV[0] = uint16(prev)
		for i := 1; i < int(palSize); i++ {
			delta := int(dec.DecodeBools(uint(bits)))
			if delta != 0 && dec.DecodeBoolEqui() != 0 {
				delta = -delta
			}
			prev = (prev + delta) & maxValue
			palV[i] = uint16(prev)
		}
	} else {
		for i := 0; i < int(palSize); i++ {
			palV[i] = uint16(dec.DecodeBools(uint(bpc)))
		}
	}

	return palSize, palU, palV, nil
}

func readPaletteIndices(cdf *ModeCDF, dec intraEntropyDecoder, palSize, plane, width, height int) ([]uint8, error) {
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
	traceLimit := debugPaletteIndexTraceLimit()
	debugTracePaletteIndexStep(dec, traceLimit, 0, plane, 0, 0, 0, 0, [8]uint8{}, int(idx[0]))
	traceStep := 1
	for diag := 1; diag < width+height-1; diag++ {
		startX, endX, stepX := paletteDiagonalXOrder(diag, width, height)
		nCells := absInt(startX-endX) + 1
		orders := make([][8]uint8, 0, nCells)
		ctxs := make([]int, 0, nCells)
		xs := make([]int, 0, nCells)
		ys := make([]int, 0, nCells)
		for x := startX; ; x += stepX {
			y := diag - x
			order, ctx := paletteOrderForCell(idx, width, x, y, palSize)
			orders = append(orders, order)
			ctxs = append(ctxs, ctx)
			xs = append(xs, x)
			ys = append(ys, y)
			if x == endX {
				break
			}
		}
		for i, x := range xs {
			y := ys[i]
			order := orders[i]
			ctx := ctxs[i]
			color := int(dec.DecodeSymbolAdapt(cdf.ColorMap[plane][palSize-2][ctx][:], palSize-1))
			if color < 0 || color >= palSize {
				return nil, fmt.Errorf("decoder: palette color index %d out of range for size %d", color, palSize)
			}
			idx[y*width+x] = order[color]
			debugTracePaletteIndexStep(dec, traceLimit, traceStep, plane, diag, x, y, ctx, order, color)
			traceStep++
			if x == endX {
				break
			}
		}
	}
	return idx, nil
}

func debugPaletteIndexTraceLimit() int {
	raw := strings.TrimSpace(os.Getenv("DEBUG_PALETTE_INDEX_TRACE_LIMIT"))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func debugTracePaletteIndexStep(dec intraEntropyDecoder, limit, step, plane, diag, x, y, ctx int, order [8]uint8, tok int) {
	if limit <= 0 || step >= limit {
		return
	}
	state, ok := dec.(interface {
		DebugState() (uint32, uint32, int, int)
	})
	if !ok {
		return
	}
	cur, rng, cnt, pos := state.DebugState()
	fmt.Fprintf(os.Stderr, "trace palette-index step=%d plane=%d diag=%d xy=(%d,%d) ctx=%d tok=%d order=%v state=(%d,%d,%d,%d)\n",
		step, plane, diag, x, y, ctx, tok, order, cur, rng, cnt, pos)
}

func lumaPaletteIndexDimensions(g BlockGeometry) (int, int) {
	w4, h4 := paletteVisibleDims4(g)
	return w4 * 4, h4 * 4
}

func chromaPaletteIndexDimensions(layout av1.ChromaSubsampling, g BlockGeometry) (int, int) {
	w4, h4 := paletteVisibleDims4(g)
	cw4, ch4 := chromaBlockDims(layout, BlockDimensions{W4: uint8(w4), H4: uint8(h4)})
	return cw4 * 4, ch4 * 4
}

func paletteVisibleDims4(g BlockGeometry) (int, int) {
	w4 := g.Width4()
	h4 := g.Height4()
	if w4 > 0 && h4 > 0 {
		return w4, h4
	}
	dims := BlockDims(g.Size)
	return int(dims.W4), int(dims.H4)
}

func paletteDiagonalXOrder(diag, width, height int) (start, end, step int) {
	first := minInt(diag, width-1)
	last := maxIntLocal(0, diag-height+1)
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DEBUG_PALETTE_INDEX_ORDER")), "reverse_diag") {
		return last, first, 1
	}
	return first, last, -1
}

func paletteOrderForCell(pal []uint8, stride, x, y, palSize int) ([8]uint8, int) {
	var order [8]uint8
	for i := range order {
		order[i] = uint8(i)
	}

	haveLeft := x > 0
	haveTop := y > 0
	var left, top, topLeft uint8
	if haveLeft {
		left = pal[y*stride+x-1]
	}
	if haveTop {
		top = pal[(y-1)*stride+x]
	}
	if haveLeft && haveTop {
		topLeft = pal[(y-1)*stride+x-1]
	}

	var (
		ctx   int
		seen  uint32
		oIdx  int
		added = func(v uint8) {
			if int(v) >= len(order) || oIdx >= len(order) {
				return
			}
			mask := uint32(1) << v
			order[oIdx] = v
			oIdx++
			seen |= mask
		}
	)

	switch {
	case !haveLeft:
		ctx = 0
		added(top)
	case !haveTop:
		ctx = 0
		added(left)
	case top == left && top == topLeft:
		ctx = 4
		added(top)
	case top == left:
		ctx = 3
		added(top)
		added(topLeft)
	case top == topLeft || left == topLeft:
		ctx = 2
		added(topLeft)
		if top == topLeft {
			added(left)
		} else {
			added(top)
		}
	default:
		ctx = 1
		if top < left {
			added(top)
			added(left)
		} else {
			added(left)
			added(top)
		}
		added(topLeft)
	}

	for i := range order {
		mask := uint32(1) << uint8(i)
		if seen&mask == 0 {
			added(uint8(i))
		}
	}
	return order, ctx
}

func paletteCache(above, left *BlockContext, plane, bx4, by4 int) ([16]uint16, int) {
	var cache [16]uint16
	if above == nil || left == nil {
		return cache, 0
	}

	leftSize := int(left.PalSz[by4])
	aboveSize := 0
	if plane != 0 {
		leftSize = int(left.UVPalSz[by4])
	}
	// dav1d zeroes the above palette cache at every SB64 boundary, even when the
	// current sequence uses 128x128 root superblocks. Local by4 is relative to
	// the current superblock row, so rows 0 and 16 both start new SB64 regions.
	if by4&15 != 0 {
		aboveSize = int(above.PalSz[bx4])
		if plane != 0 {
			aboveSize = int(above.UVPalSz[bx4])
		}
	}

	l := left.Pal[plane][by4][:]
	a := above.Pal[plane][bx4][:]
	n := 0
	appendUnique := func(v uint16) {
		if n == 0 || cache[n-1] != v {
			cache[n] = v
			n++
		}
	}
	for leftSize > 0 && aboveSize > 0 {
		if l[0] < a[0] {
			appendUnique(l[0])
			l = l[1:]
			leftSize--
			continue
		}
		if a[0] == l[0] {
			l = l[1:]
			leftSize--
		}
		appendUnique(a[0])
		a = a[1:]
		aboveSize--
	}
	for leftSize > 0 {
		appendUnique(l[0])
		l = l[1:]
		leftSize--
	}
	for aboveSize > 0 {
		appendUnique(a[0])
		a = a[1:]
		aboveSize--
	}
	return cache, n
}

func setPaletteRun(dst *[32][8]uint16, off, n int, values [8]uint16, size int) {
	if dst == nil || n <= 0 || off >= len(dst) {
		return
	}
	if off+n > len(dst) {
		n = len(dst) - off
	}
	for i := 0; i < n; i++ {
		for j := 0; j < len(dst[off+i]); j++ {
			if j < size {
				dst[off+i][j] = values[j]
			} else {
				dst[off+i][j] = 0
			}
		}
	}
}

func ReconstructPaletteLumaCoefficients8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, lossless bool, syntax IntraBlockSyntax, blockX, blockY int) error {
	return ReconstructPaletteLumaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, lossless, syntax, blockX, blockY)
}

func ReconstructPaletteLumaCoefficients8WithTxType(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, lossless bool, syntax IntraBlockSyntax, blockX, blockY int) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	traceTarget := debugTraceMovieBlock(x, y, width, height)
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, _, _, _, _, _, writeback, err := prepareIntraLumaTarget(plane, stride, planeWidth, planeHeight, x, y, width, height)
	if err != nil {
		return err
	}
	defer writeback.release()
	if traceTarget {
		head := minInt(16, len(syntax.PalYIdx))
		fmt.Fprintf(os.Stderr, "trace palette8 xy=(%d,%d) tx=%d txtype=%d pal=%v idx0=%v coeffs0=%v\n",
			x, y, tx, txType, syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...), append([]int32(nil), coeffs[:minInt(len(coeffs), 16)]...))
	}
	indexStride, _ := lumaPaletteIndexDimensions(syntax.Geometry)
	if err := fillPaletteBlock8(dst, dstStride, width, height, syntax.PalY, syntax.PalYIdx, indexStride, x-blockX, y-blockY); err != nil {
		return err
	}
	if traceTarget {
		debugTracePaletteRows8("trace palette8 base", dst, dstStride, minInt(width, 4), minInt(height, 4))
	}
	if err := InverseAddResidualTx8(dst, dstStride, tx, txType, coeffs, lossless); err != nil {
		return fmt.Errorf("decoder: inverse palette transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	if traceTarget {
		debugTracePaletteRows8("trace palette8 out", dst, dstStride, minInt(width, 4), minInt(height, 4))
	}
	writeback.commit()
	return nil
}

func ReconstructPaletteLumaCoefficients16(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, bitDepth uint8, lossless bool, syntax IntraBlockSyntax, blockX, blockY int) error {
	return ReconstructPaletteLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, bitDepth, lossless, syntax, blockX, blockY)
}

func ReconstructPaletteLumaCoefficients16WithTxType(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8, lossless bool, syntax IntraBlockSyntax, blockX, blockY int) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	traceTarget := debugTraceMovieBlock(x, y, width, height)
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, _, _, _, _, _, commit, err := prepareIntraLumaTarget16(plane, stride, planeWidth, planeHeight, x, y, width, height, bitDepth)
	if err != nil {
		return err
	}
	if traceTarget {
		head := minInt(16, len(syntax.PalYIdx))
		fmt.Fprintf(os.Stderr, "trace palette16 xy=(%d,%d) tx=%d txtype=%d pal=%v idx0=%v coeffs0=%v\n",
			x, y, tx, txType, syntax.PalY[:syntax.PalYSize], append([]uint8(nil), syntax.PalYIdx[:head]...), append([]int32(nil), coeffs[:minInt(len(coeffs), 16)]...))
	}
	indexStride, _ := lumaPaletteIndexDimensions(syntax.Geometry)
	if err := fillPaletteBlock16(dst, dstStride, width, height, syntax.PalY, syntax.PalYIdx, indexStride, x-blockX, y-blockY, bitDepth); err != nil {
		return err
	}
	if traceTarget {
		debugTracePaletteRows16("trace palette16 base", dst, dstStride, minInt(width, 4), minInt(height, 4))
	}
	if err := InverseAddResidualTx16(dst, dstStride, tx, txType, coeffs, bitDepth, lossless); err != nil {
		return fmt.Errorf("decoder: inverse palette transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	if traceTarget {
		debugTracePaletteRows16("trace palette16 out", dst, dstStride, minInt(width, 4), minInt(height, 4))
	}
	commit()
	return nil
}

func ReconstructPaletteChromaCoefficients8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, lossless bool, palette [8]uint16, indices []uint8, indexStride, blockX, blockY int) error {
	return ReconstructPaletteChromaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, lossless, palette, indices, indexStride, blockX, blockY)
}

func ReconstructPaletteChromaCoefficients8WithTxType(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, lossless bool, palette [8]uint16, indices []uint8, indexStride, blockX, blockY int) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d uv tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, _, _, _, _, _, writeback, err := prepareIntraLumaTarget(plane, stride, planeWidth, planeHeight, x, y, width, height)
	if err != nil {
		return err
	}
	defer writeback.release()
	if err := fillPaletteBlock8(dst, dstStride, width, height, palette, indices, indexStride, x-blockX, y-blockY); err != nil {
		return err
	}
	if err := InverseAddResidualTx8(dst, dstStride, tx, txType, coeffs, lossless); err != nil {
		return fmt.Errorf("decoder: inverse palette chroma transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	writeback.commit()
	return nil
}

func ReconstructPaletteChromaCoefficients16(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, bitDepth uint8, lossless bool, palette [8]uint16, indices []uint8, indexStride, blockX, blockY int) error {
	return ReconstructPaletteChromaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, bitDepth, lossless, palette, indices, indexStride, blockX, blockY)
}

func ReconstructPaletteChromaCoefficients16WithTxType(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8, lossless bool, palette [8]uint16, indices []uint8, indexStride, blockX, blockY int) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d uv tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, _, _, _, _, _, commit, err := prepareIntraLumaTarget16(plane, stride, planeWidth, planeHeight, x, y, width, height, bitDepth)
	if err != nil {
		return err
	}
	if err := fillPaletteBlock16(dst, dstStride, width, height, palette, indices, indexStride, x-blockX, y-blockY, bitDepth); err != nil {
		return err
	}
	if err := InverseAddResidualTx16(dst, dstStride, tx, txType, coeffs, bitDepth, lossless); err != nil {
		return fmt.Errorf("decoder: inverse palette chroma transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	commit()
	return nil
}

func fillPaletteBlock8(dst []byte, stride, width, height int, palette [8]uint16, indices []uint8, indexStride, srcX, srcY int) error {
	if indexStride <= 0 {
		return fmt.Errorf("decoder: invalid palette stride %d", indexStride)
	}
	if srcX < 0 || srcY < 0 {
		return fmt.Errorf("decoder: invalid palette source offset (%d,%d)", srcX, srcY)
	}
	if len(indices) < indexStride {
		return fmt.Errorf("decoder: palette index buffer too small")
	}
	indexHeight := len(indices) / indexStride
	if indexHeight == 0 {
		return fmt.Errorf("decoder: palette index buffer too small")
	}
	for y := 0; y < height; y++ {
		dstRow := dst[y*stride:]
		sy := minInt(srcY+y, indexHeight-1)
		rowStart := sy * indexStride
		idxRow := indices[rowStart : rowStart+indexStride]
		for x := 0; x < width; x++ {
			sx := minInt(srcX+x, indexStride-1)
			idx := idxRow[sx]
			if idx >= 8 {
				return fmt.Errorf("decoder: palette index %d out of range", idx)
			}
			dstRow[x] = clipByte(int(palette[idx]))
		}
	}
	return nil
}

func fillPaletteBlock16(dst []uint16, stride, width, height int, palette [8]uint16, indices []uint8, indexStride, srcX, srcY int, bitDepth uint8) error {
	if indexStride <= 0 {
		return fmt.Errorf("decoder: invalid palette stride %d", indexStride)
	}
	if srcX < 0 || srcY < 0 {
		return fmt.Errorf("decoder: invalid palette source offset (%d,%d)", srcX, srcY)
	}
	if len(indices) < indexStride {
		return fmt.Errorf("decoder: palette index buffer too small")
	}
	indexHeight := len(indices) / indexStride
	if indexHeight == 0 {
		return fmt.Errorf("decoder: palette index buffer too small")
	}
	for y := 0; y < height; y++ {
		dstRow := dst[y*stride:]
		sy := minInt(srcY+y, indexHeight-1)
		rowStart := sy * indexStride
		idxRow := indices[rowStart : rowStart+indexStride]
		for x := 0; x < width; x++ {
			sx := minInt(srcX+x, indexStride-1)
			idx := idxRow[sx]
			if idx >= 8 {
				return fmt.Errorf("decoder: palette index %d out of range", idx)
			}
			dstRow[x] = clipSample(int(palette[idx]), bitDepth)
		}
	}
	return nil
}

func debugTracePaletteRows16(label string, plane []uint16, stride, width, height int) {
	if stride <= 0 || len(plane) == 0 || width <= 0 || height <= 0 {
		return
	}
	for row := 0; row < height; row++ {
		start := row * stride
		fmt.Fprintf(os.Stderr, "%s row=%d values=%v\n", label, row, plane[start:start+width])
	}
}

func debugTracePaletteRows8(label string, plane []byte, stride, width, height int) {
	if stride <= 0 || len(plane) == 0 || width <= 0 || height <= 0 {
		return
	}
	for row := 0; row < height; row++ {
		start := row * stride
		fmt.Fprintf(os.Stderr, "%s row=%d values=%v\n", label, row, plane[start:start+width])
	}
}

func ulog2(v int) int {
	if v <= 0 {
		return 0
	}
	n := 0
	for v > 1 {
		v >>= 1
		n++
	}
	return n
}
