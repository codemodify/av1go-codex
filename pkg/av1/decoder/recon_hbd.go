package decoder

import (
	"fmt"
	"math"
	"os"
)

func ReconstructIntraLumaCoefficients16(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, mode IntraPredMode, angle int8, coeffs []int32, bitDepth uint8, lossless bool) error {
	return ReconstructIntraLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, mode, angle, coeffs, bitDepth, lossless)
}

func ReconstructIntraLumaCoefficients16WithTxType(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, bitDepth uint8, lossless bool) error {
	return ReconstructIntraLumaCoefficients16WithTxTypeFlags(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, coeffs, bitDepth, lossless, 0)
}

func ReconstructIntraLumaCoefficients16WithTxTypeFlags(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, bitDepth uint8, lossless bool, flags uint16) error {
	return ReconstructIntraLumaCoefficients16WithTxTypeFlagsEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, coeffs, bitDepth, lossless, flags, true, true)
}

func ReconstructIntraLumaCoefficients16WithTxTypeFlagsEdges(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, bitDepth uint8, lossless bool, flags uint16, topHasRight, leftHasBottom bool) error {
	return ReconstructIntraLumaCoefficients16WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, coeffs, bitDepth, lossless, flags, y > 0, x > 0, topHasRight, leftHasBottom)
}

func ReconstructIntraLumaCoefficients16WithTxTypeFlagsNeighborEdges(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, bitDepth uint8, lossless bool, flags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, commit, err := prepareIntraLumaTarget16EdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, bitDepth, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	traceTarget := debugTraceMovieBlock(x, y, width, height)
	if err := PredictIntraLuma16WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, bitDepth, flags); err != nil {
		return err
	}
	if traceTarget {
		traceCols := minInt(width, 32)
		traceRows := minInt(height, 16)
		predRows := make([][]uint16, traceRows)
		for row := 0; row < traceRows; row++ {
			predRows[row] = append([]uint16(nil), dst[row*dstStride:row*dstStride+traceCols]...)
		}
		fmt.Fprintf(os.Stderr, "trace target16 xy=(%d,%d) before_residual txType=%d lossless=%v mode=%d angle=%d topLeft=%d haveTop=%v haveLeft=%v top=%v left=%v pred=%v coeffs=%v\n",
			x, y, txType, lossless, mode, angle, topLeft, haveTop, haveLeft,
			append([]uint16(nil), top[:minInt(len(top), traceCols)]...),
			append([]uint16(nil), left[:minInt(len(left), traceRows)]...),
			predRows,
			append([]int32(nil), coeffs[:minInt(len(coeffs), 32)]...),
		)
	}
	if err := InverseAddResidualTx16(dst, dstStride, tx, txType, coeffs, bitDepth, lossless); err != nil {
		return fmt.Errorf("decoder: inverse intra transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	if traceTarget {
		traceCols := minInt(width, 32)
		traceRows := minInt(height, 16)
		outRows := make([][]uint16, traceRows)
		for row := 0; row < traceRows; row++ {
			outRows[row] = append([]uint16(nil), dst[row*dstStride:row*dstStride+traceCols]...)
		}
		fmt.Fprintf(os.Stderr, "trace target16 xy=(%d,%d) after_residual out=%v\n",
			x, y,
			outRows,
		)
	}
	commit()
	return nil
}

func InverseAddResidual16(dst []uint16, stride int, tx TxfmSize, coeffs []int32, bitDepth uint8, lossless bool) error {
	return InverseAddResidualTx16(dst, stride, tx, TxTypeDCTDCT, coeffs, bitDepth, lossless)
}

func InverseAddDCTDCT16(dst []uint16, stride int, tx TxfmSize, coeffs []int32, bitDepth uint8) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small")
	}
	if stride < width {
		return fmt.Errorf("decoder: inverse transform stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: inverse transform buffer too small")
	}

	if info.W4 == info.H4 {
		n := width
		if dconly, coeff := coeffsDConly(coeffs, n*n); dconly {
			return InverseAddDCTDCTDC16(dst, stride, tx, coeff, bitDepth)
		}
	}

	return inverseAddDCTDCT16Dense(dst, stride, tx, coeffs, bitDepth)
}

func inverseAddDCTDCT16Dense(dst []uint16, stride int, tx TxfmSize, coeffs []int32, bitDepth uint8) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small")
	}
	if stride < width {
		return fmt.Errorf("decoder: inverse transform stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: inverse transform buffer too small")
	}

	if ok, err := inverseAddExact16(dst, stride, tx, TxTypeDCTDCT, coeffs, bitDepth); ok || err != nil {
		return err
	}

	rowBasis := idctBasis(width)
	colBasis := idctBasis(height)
	tmp := make([]float64, height*width)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sum := 0.0
			for u := 0; u < width; u++ {
				sum += float64(coeffs[y*width+u]) * rowBasis[u*width+x]
			}
			tmp[y*width+x] = sum
		}
	}

	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			sum := 0.0
			for v := 0; v < height; v++ {
				sum += tmp[v*width+x] * colBasis[v*height+y]
			}
			row[x] = clipSample(int(row[x])+int(math.Round(sum/8.0)), bitDepth)
		}
	}
	return nil
}

func InverseAddDCTDCTDC16(dst []uint16, stride int, tx TxfmSize, coeff int32, bitDepth uint8) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if stride < width {
		return fmt.Errorf("decoder: inverse transform stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: inverse transform buffer too small")
	}

	if info.W4 != info.H4 {
		coeffs := make([]int32, width*height)
		coeffs[0] = coeff
		return inverseAddDCTDCT16Dense(dst, stride, tx, coeffs, bitDepth)
	}
	shift, ok := dctDCTSquareShift[tx]
	if !ok {
		coeffs := make([]int32, width*height)
		coeffs[0] = coeff
		return inverseAddDCTDCT16Dense(dst, stride, tx, coeffs, bitDepth)
	}
	dc := int(coeff)
	dc = (dc*181 + 128) >> 8
	dc = (dc + ((1 << shift) >> 1)) >> shift
	dc = (dc*181 + 128 + 2048) >> 12

	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			row[x] = clipSample(int(row[x])+dc, bitDepth)
		}
	}
	return nil
}

func InverseAddWHTWHT4X416(dst []uint16, stride int, coeffs []int32, bitDepth uint8) error {
	if stride < 4 {
		return fmt.Errorf("decoder: inverse wht stride %d < width 4", stride)
	}
	if len(dst) < 3*stride+4 {
		return fmt.Errorf("decoder: inverse wht buffer too small")
	}
	if len(coeffs) < 16 {
		return fmt.Errorf("decoder: inverse wht coefficient buffer too small")
	}

	var tmp [16]int32
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			tmp[y*4+x] = coeffs[y*4+x] >> 2
		}
		invWHT4_1D(tmp[y*4:y*4+4], 1)
	}
	for x := 0; x < 4; x++ {
		invWHT4_1D(tmp[x:], 4)
	}
	for y := 0; y < 4; y++ {
		row := dst[y*stride:]
		for x := 0; x < 4; x++ {
			row[x] = clipSample(int(row[x])+int(tmp[y*4+x]), bitDepth)
		}
	}
	return nil
}

func PredictDCLuma16(dst []uint16, stride, width, height int, top, left []uint16, haveTop, haveLeft bool, bitDepth uint8) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("decoder: invalid dc predictor dimensions %dx%d", width, height)
	}
	if stride < width {
		return fmt.Errorf("decoder: dc predictor stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: dc predictor buffer too small")
	}
	if haveTop && len(top) < width {
		return fmt.Errorf("decoder: dc predictor top edge too small")
	}
	if haveLeft && len(left) < height {
		return fmt.Errorf("decoder: dc predictor left edge too small")
	}

	var dc int
	switch {
	case haveTop && haveLeft:
		sum := (width + height) >> 1
		for i := 0; i < width; i++ {
			sum += int(top[i])
		}
		for i := 0; i < height; i++ {
			sum += int(left[i])
		}
		dc = sum >> ctzPow2(width+height)
		if width != height {
			multiplier := 0x5556
			if width > height*2 || height > width*2 {
				multiplier = 0x3334
			}
			dc = (dc * multiplier) >> 16
		}
	case haveTop:
		sum := width >> 1
		for i := 0; i < width; i++ {
			sum += int(top[i])
		}
		dc = sum >> ctzPow2(width)
	case haveLeft:
		sum := height >> 1
		for i := 0; i < height; i++ {
			sum += int(left[i])
		}
		dc = sum >> ctzPow2(height)
	default:
		dc = 1 << (bitDepth - 1)
	}

	fillBlock16(dst, stride, width, height, uint16(dc))
	return nil
}

func PredictIntraLuma16(dst []uint16, stride, width, height int, mode IntraPredMode, angle int8, top, left []uint16, topLeft uint16, haveTop, haveLeft bool, bitDepth uint8) error {
	return PredictIntraLuma16WithFlags(dst, stride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, bitDepth, 0)
}

func PredictIntraLuma16WithFlags(dst []uint16, stride, width, height int, mode IntraPredMode, angle int8, top, left []uint16, topLeft uint16, haveTop, haveLeft bool, bitDepth uint8, flags uint16) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("decoder: invalid intra predictor dimensions %dx%d", width, height)
	}
	if stride < width {
		return fmt.Errorf("decoder: intra predictor stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: intra predictor buffer too small")
	}
	if haveTop && len(top) < width {
		return fmt.Errorf("decoder: top edge too small")
	}
	if haveLeft && len(left) < height {
		return fmt.Errorf("decoder: left edge too small")
	}
	if angle != 0 {
		if !isDirectionalMode(mode) && mode != IntraPredFilter {
			angle = 0
		}
	}

	effMode, actualAngle := effectiveIntraMode(mode, angle, haveTop, haveLeft)
	topExt, topWithTL, leftExt, leftWithTL := extendIntraEdges16(width, height, top, left, topLeft, haveTop, haveLeft, bitDepth)
	if effMode == intraPredZ2 && flags&angleUseEdgeFilterFlag != 0 && width+height >= 24 {
		smoothed := ((int(leftWithTL[1])+int(topWithTL[1]))*5 + int(topWithTL[0])*6 + 8) >> 4
		topWithTL[0] = clipSample(smoothed, bitDepth)
		leftWithTL[0] = topWithTL[0]
		topLeft = topWithTL[0]
	}

	switch effMode {
	case IntraPredDC:
		return PredictDCLuma16(dst, stride, width, height, top, left, haveTop, haveLeft, bitDepth)
	case IntraPredFilter:
		return PredictFilterIntra16(dst, stride, width, height, angle, top, left, topLeft, haveTop, haveLeft, bitDepth)
	case IntraPredVertical:
		for y := 0; y < height; y++ {
			copy(dst[y*stride:y*stride+width], topExt[:width])
		}
		return nil
	case IntraPredHorizontal:
		for y := 0; y < height; y++ {
			fillBlock16(dst[y*stride:], stride, width, 1, leftExt[y])
		}
		return nil
	case IntraPredPaeth:
		for y := 0; y < height; y++ {
			lv := leftExt[y]
			row := dst[y*stride:]
			for x := 0; x < width; x++ {
				tv := topExt[x]
				base := int(lv) + int(tv) - int(topLeft)
				ldiff := absInt(int(lv) - base)
				tdiff := absInt(int(tv) - base)
				tldiff := absInt(int(topLeft) - base)
				switch {
				case ldiff <= tdiff && ldiff <= tldiff:
					row[x] = lv
				case tdiff <= tldiff:
					row[x] = tv
				default:
					row[x] = topLeft
				}
			}
		}
		return nil
	case IntraPredSmooth:
		weightsH := smoothWeightsFor(width)
		weightsV := smoothWeightsFor(height)
		right := topExt[width-1]
		bottom := leftExt[height-1]
		for y := 0; y < height; y++ {
			row := dst[y*stride:]
			for x := 0; x < width; x++ {
				pred := int(weightsV[y])*int(topExt[x]) + (256-int(weightsV[y]))*int(bottom) +
					int(weightsH[x])*int(leftExt[y]) + (256-int(weightsH[x]))*int(right)
				row[x] = clipSample((pred+256)>>9, bitDepth)
			}
		}
		return nil
	case IntraPredSmoothVertical:
		weightsV := smoothWeightsFor(height)
		bottom := leftExt[height-1]
		for y := 0; y < height; y++ {
			row := dst[y*stride:]
			for x := 0; x < width; x++ {
				pred := int(weightsV[y])*int(topExt[x]) + (256-int(weightsV[y]))*int(bottom)
				row[x] = clipSample((pred+128)>>8, bitDepth)
			}
		}
		return nil
	case IntraPredSmoothHorizontal:
		weightsH := smoothWeightsFor(width)
		right := topExt[width-1]
		for y := 0; y < height; y++ {
			row := dst[y*stride:]
			for x := 0; x < width; x++ {
				pred := int(weightsH[x])*int(leftExt[y]) + (256-int(weightsH[x]))*int(right)
				row[x] = clipSample((pred+128)>>8, bitDepth)
			}
		}
		return nil
	case intraPredZ1:
		if err := predictDirectionalZ1_16(dst, stride, width, height, actualAngle, topExt, topLeft, bitDepth, flags); err == nil {
			return nil
		}
		return PredictDCLuma16(dst, stride, width, height, top, left, haveTop, haveLeft, bitDepth)
	case intraPredZ2:
		if err := predictDirectionalZ2_16(dst, stride, width, height, actualAngle, topWithTL, leftWithTL, bitDepth, flags); err == nil {
			return nil
		}
		return PredictDCLuma16(dst, stride, width, height, top, left, haveTop, haveLeft, bitDepth)
	case intraPredZ3:
		if err := predictDirectionalZ3_16(dst, stride, width, height, actualAngle, leftExt, topLeft, bitDepth, flags); err == nil {
			return nil
		}
		return PredictDCLuma16(dst, stride, width, height, top, left, haveTop, haveLeft, bitDepth)
	default:
		return PredictDCLuma16(dst, stride, width, height, top, left, haveTop, haveLeft, bitDepth)
	}
}

func predictDirectionalZ1_16(dst []uint16, stride, width, height, angle int, top []uint16, topLeft uint16, bitDepth uint8, flags uint16) error {
	if angle <= 0 || angle >= 90 {
		return fmt.Errorf("decoder: invalid z1 angle %d", angle)
	}
	dx := directionalDerivative(angle)
	isSmooth := flags&angleSmoothEdgeFlag != 0
	enableEdgeFilter := flags&angleUseEdgeFilterFlag != 0
	topLen := width + minInt(width, height)
	topTL := make([]uint16, topLen+1)
	topTL[0] = topLeft
	copy(topTL[1:], top[:minInt(topLen, len(top))])
	edge := top[:minInt(topLen, len(top))]
	baseInc := 1
	if enableEdgeFilter {
		if useIntraEdgeUpsample(width+height, 90-angle, isSmooth) {
			edge = upsampleDirectionalEdge16(topTL, width+height, bitDepth)
			dx <<= 1
			baseInc = 2
		} else if strength := intraEdgeFilterStrength(width+height, 90-angle, isSmooth); strength > 0 {
			edge = filterDirectionalEdge16(topTL, width+height, strength, bitDepth)
		}
	}
	maxBaseX := len(edge) - 1
	for y := 0; y < height; y++ {
		xpos := dx * (y + 1)
		frac := xpos & 0x3e
		base := xpos >> 6
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			if base < maxBaseX {
				v := int(edge[base])*(64-frac) + int(edge[base+1])*frac
				row[x] = clipSample((v+32)>>6, bitDepth)
			} else {
				fillBlock16(row[x:], stride, width-x, 1, edge[maxBaseX])
				break
			}
			base += baseInc
		}
	}
	return nil
}

func predictDirectionalZ2_16(dst []uint16, stride, width, height, angle int, topWithTL, leftWithTL []uint16, bitDepth uint8, flags uint16) error {
	if angle <= 90 || angle >= 180 {
		return fmt.Errorf("decoder: invalid z2 angle %d", angle)
	}
	dy := directionalDerivative(angle - 90)
	dx := directionalDerivative(180 - angle)
	isSmooth := flags&angleSmoothEdgeFlag != 0
	enableEdgeFilter := flags&angleUseEdgeFilterFlag != 0
	topRef := topWithTL[:minInt(width+1, len(topWithTL))]
	leftRef := leftWithTL[:minInt(height+1, len(leftWithTL))]
	upsampleAbove := false
	upsampleLeft := false
	if enableEdgeFilter {
		if useIntraEdgeUpsample(width+height, angle-90, isSmooth) {
			topRef = upsampleDirectionalEdge16Range(topRef, len(topRef), -1, -1, len(topRef)-1, bitDepth)
			dx <<= 1
			upsampleAbove = true
		} else if strength := intraEdgeFilterStrength(width+height, angle-90, isSmooth); strength > 0 {
			filtered := filterDirectionalEdge16Range(topWithTL, width, 0, width, 0, -1, width, strength, bitDepth)
			topRef = make([]uint16, width+1)
			topRef[0] = topWithTL[0]
			copy(topRef[1:], filtered)
		}
		if useIntraEdgeUpsample(width+height, 180-angle, isSmooth) {
			leftRef = upsampleDirectionalEdge16Range(leftRef, len(leftRef), -1, -1, len(leftRef)-1, bitDepth)
			dy <<= 1
			upsampleLeft = true
		} else if strength := intraEdgeFilterStrength(width+height, 180-angle, isSmooth); strength > 0 {
			leftRef = filterDirectionalZ2LeftEdge16(leftWithTL[1:], leftWithTL[0], height, strength, bitDepth)
		}
	}
	baseIncX := 1 + boolToInt(upsampleAbove)
	leftBase := 1 + boolToInt(upsampleLeft)
	yShift := 6 + boolToInt(upsampleLeft)
	xpos := ((1 + boolToInt(upsampleAbove)) << 6) - dx
	for y := 0; y < height; y++ {
		baseX := xpos >> 6
		fracX := xpos & 0x3e
		ypos := (y << yShift) - dy
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			if baseX >= 0 {
				idx := minInt(baseX, len(topRef)-2)
				v := int(topRef[idx])*(64-fracX) + int(topRef[idx+1])*fracX
				row[x] = clipSample((v+32)>>6, bitDepth)
			} else {
				baseY := ypos >> 6
				fracY := ypos & 0x3e
				idx := minInt(maxIntLocal(0, baseY+leftBase), len(leftRef)-2)
				v := int(leftRef[idx])*(64-fracY) + int(leftRef[idx+1])*fracY
				row[x] = clipSample((v+32)>>6, bitDepth)
			}
			baseX += baseIncX
			ypos -= dy
		}
		xpos -= dx
	}
	return nil
}

func predictDirectionalZ3_16(dst []uint16, stride, width, height, angle int, left []uint16, topLeft uint16, bitDepth uint8, flags uint16) error {
	if angle <= 180 || angle >= 270 {
		return fmt.Errorf("decoder: invalid z3 angle %d", angle)
	}
	dy := directionalDerivative(270 - angle)
	isSmooth := flags&angleSmoothEdgeFlag != 0
	enableEdgeFilter := flags&angleUseEdgeFilterFlag != 0
	leftLen := height + minInt(width, height)
	edge := left[:minInt(leftLen, len(left))]
	if len(edge) < leftLen {
		fixed := make([]uint16, leftLen)
		copy(fixed, edge)
		if len(edge) > 0 {
			fillRunUint16(fixed, len(edge), leftLen-len(edge), edge[len(edge)-1])
		}
		edge = fixed
	}
	if len(edge) > height && height > 0 && edge[height-1] != 0 {
		tailZero := true
		for i := height; i < len(edge); i++ {
			if edge[i] != 0 {
				tailZero = false
				break
			}
		}
		if tailZero {
			fixed := make([]uint16, len(edge))
			copy(fixed, edge)
			fillRunUint16(fixed, height, len(fixed)-height, fixed[height-1])
			edge = fixed
		}
	}
	baseInc := 1
	if enableEdgeFilter {
		total := width + height
		from := maxIntLocal(width-height, 0)
		if useIntraEdgeUpsample(width+height, angle-180, isSmooth) {
			edge = upsampleDirectionalZ3Edge16(edge, topLeft, total, from, bitDepth)
			dy <<= 1
			baseInc = 2
		} else if strength := intraEdgeFilterStrength(width+height, angle-180, isSmooth); strength > 0 {
			edge = filterDirectionalZ3Edge16(edge, topLeft, total, from, strength, bitDepth)
		}
	}
	maxBaseY := len(edge) - 1
	for x := 0; x < width; x++ {
		ypos := dy * (x + 1)
		frac := ypos & 0x3e
		base := ypos >> 6
		for y := 0; y < height; y++ {
			if base < maxBaseY {
				v := int(edge[base])*(64-frac) + int(edge[base+1])*frac
				dst[y*stride+x] = clipSample((v+32)>>6, bitDepth)
			} else {
				for ; y < height; y++ {
					dst[y*stride+x] = edge[maxBaseY]
				}
				break
			}
			base += baseInc
		}
	}
	return nil
}

func prepareIntraLumaTarget16(plane []uint16, stride, planeWidth, planeHeight, x, y, width, height int, bitDepth uint8) ([]uint16, int, []uint16, []uint16, uint16, bool, bool, func(), error) {
	return prepareIntraLumaTarget16Edges(plane, stride, planeWidth, planeHeight, x, y, width, height, bitDepth, true, true)
}

func prepareIntraLumaTarget16Edges(plane []uint16, stride, planeWidth, planeHeight, x, y, width, height int, bitDepth uint8, topHasRight, leftHasBottom bool) ([]uint16, int, []uint16, []uint16, uint16, bool, bool, func(), error) {
	return prepareIntraLumaTarget16EdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, bitDepth, y > 0, x > 0, topHasRight, leftHasBottom)
}

func prepareIntraLumaTarget16EdgesWithNeighbors(plane []uint16, stride, planeWidth, planeHeight, x, y, width, height int, bitDepth uint8, haveTop, haveLeft, topHasRight, leftHasBottom bool) ([]uint16, int, []uint16, []uint16, uint16, bool, bool, func(), error) {
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight {
		return nil, 0, nil, nil, 0, false, false, nil, fmt.Errorf("decoder: block (%d,%d %dx%d) out of plane bounds %dx%d", x, y, width, height, planeWidth, planeHeight)
	}
	if stride < planeWidth {
		return nil, 0, nil, nil, 0, false, false, nil, fmt.Errorf("decoder: plane stride %d < width %d", stride, planeWidth)
	}
	if len(plane) < (planeHeight-1)*stride+planeWidth {
		return nil, 0, nil, nil, 0, false, false, nil, fmt.Errorf("decoder: plane buffer too small")
	}

	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil, 0, nil, nil, 0, false, false, nil, fmt.Errorf("decoder: block (%d,%d %dx%d) has no visible pixels in %dx%d plane", x, y, width, height, planeWidth, planeHeight)
	}

	haveTop = haveTop && y > 0
	haveLeft = haveLeft && x > 0
	edgeLen := width + height
	var top []uint16
	var left []uint16
	topLeft := uint16(1 << (bitDepth - 1))
	if haveTop && haveLeft {
		topLeft = plane[(y-1)*stride+x-1]
	}

	if visibleWidth == width && visibleHeight == height {
		dst := plane[y*stride+x:]
		if haveTop {
			topLimit := width
			if topHasRight {
				topLimit = edgeLen
			}
			topAvail := minInt(topLimit, planeWidth-x)
			top = plane[(y-1)*stride+x : (y-1)*stride+x+topAvail]
		}
		if haveLeft {
			leftLimit := height
			if leftHasBottom {
				leftLimit = edgeLen
			}
			leftAvail := minInt(leftLimit, planeHeight-y)
			left = make([]uint16, leftAvail)
			for i := 0; i < leftAvail; i++ {
				left[i] = plane[(y+i)*stride+x-1]
			}
		}
		if haveTop && !haveLeft {
			topLeft = top[0]
		} else if haveLeft && !haveTop {
			topLeft = left[0]
		}
		return dst, stride, top, left, topLeft, haveTop, haveLeft, func() {}, nil
	}

	dst := make([]uint16, width*height)
	if haveTop {
		topLimit := width
		if topHasRight {
			topLimit = edgeLen
		}
		top = extendedTopEdge16(plane, stride, planeWidth, x, y, topLimit)
		if !haveLeft {
			topLeft = top[0]
		}
	}
	if haveLeft {
		leftLimit := height
		if leftHasBottom {
			leftLimit = edgeLen
		}
		left = extendedLeftEdge16(plane, stride, planeHeight, x, y, leftLimit)
		if !haveTop {
			topLeft = left[0]
		}
	}
	commit := func() {
		for row := 0; row < visibleHeight; row++ {
			copy(plane[(y+row)*stride+x:(y+row)*stride+x+visibleWidth], dst[row*width:row*width+visibleWidth])
		}
	}
	return dst, width, top, left, topLeft, haveTop, haveLeft, commit, nil
}

func extendIntraEdges16(width, height int, top, left []uint16, topLeft uint16, haveTop, haveLeft bool, bitDepth uint8) ([]uint16, []uint16, []uint16, []uint16) {
	mid := uint16(1 << (bitDepth - 1))
	topFill := mid - 1
	leftFill := mid + 1
	if haveLeft && len(left) != 0 {
		topFill = left[0]
	}
	if haveTop && len(top) != 0 {
		leftFill = top[0]
	}
	switch {
	case haveTop && !haveLeft && len(top) != 0:
		topLeft = top[0]
	case haveLeft && !haveTop && len(left) != 0:
		topLeft = left[0]
	case !haveTop && !haveLeft:
		topLeft = mid
	}

	edgeLen := width + height + 1
	buf := make([]uint16, 2*(edgeLen+1))
	topWithTL := buf[:edgeLen+1]
	leftWithTL := buf[edgeLen+1:]
	topExt := topWithTL[1:]
	leftExt := leftWithTL[1:]
	for i := range topExt {
		topExt[i] = topFill
	}
	for i := range leftExt {
		leftExt[i] = leftFill
	}
	if haveTop && len(top) != 0 {
		n := minInt(len(top), len(topExt))
		copy(topExt, top[:n])
		fillRunUint16(topExt, n, len(topExt)-n, top[n-1])
	}
	if haveLeft && len(left) != 0 {
		n := minInt(len(left), len(leftExt))
		copy(leftExt, left[:n])
		fillRunUint16(leftExt, n, len(leftExt)-n, left[n-1])
	}

	topWithTL[0] = topLeft
	leftWithTL[0] = topLeft
	return topExt, topWithTL, leftExt, leftWithTL
}

func extendedTopEdge16(plane []uint16, stride, planeWidth, x, y, width int) []uint16 {
	edge := make([]uint16, width)
	available := minInt(width, planeWidth-x)
	copy(edge, plane[(y-1)*stride+x:(y-1)*stride+x+available])
	fillRunUint16(edge, available, width-available, edge[available-1])
	return edge
}

func extendedLeftEdge16(plane []uint16, stride, planeHeight, x, y, height int) []uint16 {
	edge := make([]uint16, height)
	available := minInt(height, planeHeight-y)
	for i := 0; i < available; i++ {
		edge[i] = plane[(y+i)*stride+x-1]
	}
	fillRunUint16(edge, available, height-available, edge[available-1])
	return edge
}

func fillBlock16(dst []uint16, stride, width, height int, v uint16) {
	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			row[x] = v
		}
	}
}

func fillRunUint16(dst []uint16, start, count int, v uint16) {
	for i := 0; i < count && start+i < len(dst); i++ {
		dst[start+i] = v
	}
}

func clipSample(v int, bitDepth uint8) uint16 {
	if v < 0 {
		return 0
	}
	max := (1 << bitDepth) - 1
	if v > max {
		return uint16(max)
	}
	return uint16(v)
}
