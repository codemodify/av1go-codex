package decoder

import (
	"fmt"
	"math"
	"sync"
)

var intraEdgeScratchPool sync.Pool
var intraTargetByteScratchPool sync.Pool
var smoothWeightsCache sync.Map

type intraTargetWriteback8 struct {
	plane         []byte
	dst           []byte
	planeStride   int
	dstStride     int
	x             int
	y             int
	visibleWidth  int
	visibleHeight int
	dstScratch    *scratchBuffer[byte]
	topScratch    *scratchBuffer[byte]
	leftScratch   *scratchBuffer[byte]
}

func (w intraTargetWriteback8) commit() {
	if len(w.plane) == 0 || len(w.dst) == 0 || w.visibleWidth <= 0 || w.visibleHeight <= 0 {
		return
	}
	for row := 0; row < w.visibleHeight; row++ {
		copy(
			w.plane[(w.y+row)*w.planeStride+w.x:(w.y+row)*w.planeStride+w.x+w.visibleWidth],
			w.dst[row*w.dstStride:row*w.dstStride+w.visibleWidth],
		)
	}
}

func (w intraTargetWriteback8) release() {
	putScratch(&intraTargetByteScratchPool, w.dstScratch)
	putScratch(&intraTargetByteScratchPool, w.topScratch)
	putScratch(&intraTargetByteScratchPool, w.leftScratch)
}

func ReconstructIntraLumaBlock8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, mode IntraPredMode, angle int8, coeff int32) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTarget(plane, stride, planeWidth, planeHeight, x, y, width, height)
	if err != nil {
		return err
	}
	defer writeback.release()
	if err := PredictIntraLuma8WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, 0); err != nil {
		return err
	}
	if err := InverseAddDCTDCTDC8(dst, dstStride, tx, coeff); err != nil {
		return err
	}
	writeback.commit()
	return nil
}

func PredictIntraLuma8(dst []byte, stride, width, height int, mode IntraPredMode, angle int8, top, left []byte, topLeft uint8, haveTop, haveLeft bool) error {
	return PredictIntraLuma8WithFlags(dst, stride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, 0)
}

func PredictIntraLuma8WithFlags(dst []byte, stride, width, height int, mode IntraPredMode, angle int8, top, left []byte, topLeft uint8, haveTop, haveLeft bool, flags uint16) error {
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
	edgeScratch, topExt, topWithTL, leftExt, leftWithTL := extendIntraEdges(width, height, top, left, topLeft, haveTop, haveLeft)
	defer putZeroScratch(&intraEdgeScratchPool, edgeScratch)
	if effMode == intraPredZ2 && flags&angleUseEdgeFilterFlag != 0 && width+height >= 24 {
		smoothed := ((int(leftWithTL[1])+int(topWithTL[1]))*5 + int(topWithTL[0])*6 + 8) >> 4
		topWithTL[0] = clipByte(smoothed)
		leftWithTL[0] = topWithTL[0]
		topLeft = topWithTL[0]
	}

	switch effMode {
	case IntraPredDC:
		return PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft)
	case IntraPredFilter:
		return PredictFilterIntra8(dst, stride, width, height, angle, top, left, topLeft, haveTop, haveLeft)
	case IntraPredVertical:
		for y := 0; y < height; y++ {
			copy(dst[y*stride:y*stride+width], topExt[:width])
		}
		return nil
	case IntraPredHorizontal:
		for y := 0; y < height; y++ {
			fillBlock8(dst[y*stride:], stride, width, 1, leftExt[y])
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
				row[x] = uint8((pred + 256) >> 9)
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
				row[x] = uint8((pred + 128) >> 8)
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
				row[x] = uint8((pred + 128) >> 8)
			}
		}
		return nil
	case intraPredZ1:
		if err := predictDirectionalZ1(dst, stride, width, height, actualAngle, topExt, topLeft, flags); err == nil {
			return nil
		}
		return PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft)
	case intraPredZ2:
		if err := predictDirectionalZ2(dst, stride, width, height, actualAngle, topWithTL, leftWithTL, flags); err == nil {
			return nil
		}
		return PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft)
	case intraPredZ3:
		if err := predictDirectionalZ3(dst, stride, width, height, actualAngle, leftExt, topLeft, flags); err == nil {
			return nil
		}
		return PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft)
	default:
		return PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft)
	}
}

const (
	intraPredZ1 IntraPredMode = 64 + iota
	intraPredZ2
	intraPredZ3
)

func effectiveIntraMode(mode IntraPredMode, angle int8, haveTop, haveLeft bool) (IntraPredMode, int) {
	if isDirectionalMode(mode) {
		actual := directionalAngle(mode, angle)
		switch {
		case actual <= 90:
			if actual < 90 && haveTop {
				return intraPredZ1, actual
			}
			return IntraPredVertical, actual
		case actual < 180:
			return intraPredZ2, actual
		default:
			if actual > 180 && haveLeft {
				return intraPredZ3, actual
			}
			return IntraPredHorizontal, actual
		}
	}
	if mode == IntraPredDC {
		return IntraPredDC, 0
	}
	if mode == IntraPredPaeth {
		switch {
		case haveTop && haveLeft:
			return IntraPredPaeth, 0
		case haveTop:
			return IntraPredVertical, 0
		case haveLeft:
			return IntraPredHorizontal, 0
		default:
			return IntraPredDC, 0
		}
	}
	return mode, 0
}

func isDirectionalMode(mode IntraPredMode) bool {
	return mode >= IntraPredVertical && mode <= IntraPredVertLeft
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var av1DirectionalBaseAngle = [8]int{90, 180, 45, 135, 113, 157, 203, 67}

func directionalAngle(mode IntraPredMode, delta int8) int {
	if mode < IntraPredVertical || mode > IntraPredVertLeft {
		return 0
	}
	base := av1DirectionalBaseAngle[int(mode-IntraPredVertical)]
	return base + 3*int(delta)
}

func extendIntraEdges(width, height int, top, left []byte, topLeft uint8, haveTop, haveLeft bool) (*scratchBuffer[byte], []byte, []byte, []byte, []byte) {
	const mid = uint8(128)
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
	scratch := takeScratch[byte](&intraEdgeScratchPool, 2*(edgeLen+1))
	buf := scratch.buf
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
		fillRunUint8(topExt, n, len(topExt)-n, top[n-1])
	}
	if haveLeft && len(left) != 0 {
		n := minInt(len(left), len(leftExt))
		copy(leftExt, left[:n])
		fillRunUint8(leftExt, n, len(leftExt)-n, left[n-1])
	}

	topWithTL[0] = topLeft
	leftWithTL[0] = topLeft
	return scratch, topExt, topWithTL, leftExt, leftWithTL
}

func smoothWeightsFor(n int) []uint8 {
	switch n {
	case 4:
		return smoothWeights4[:]
	case 8:
		return smoothWeights8[:]
	case 16:
		return smoothWeights16[:]
	case 32:
		return smoothWeights32[:]
	case 64:
		return smoothWeights64[:]
	default:
		if cached, ok := smoothWeightsCache.Load(n); ok {
			return cached.([]uint8)
		}
		out := make([]uint8, n)
		for i := range out {
			out[i] = uint8(maxIntLocal(8, int(math.Round(256.0*float64(n-i)/float64(n+1)))))
		}
		if cached, loaded := smoothWeightsCache.LoadOrStore(n, out); loaded {
			return cached.([]uint8)
		}
		return out
	}
}

var (
	smoothWeights4  = [...]uint8{255, 149, 85, 64}
	smoothWeights8  = [...]uint8{255, 197, 146, 105, 73, 50, 37, 32}
	smoothWeights16 = [...]uint8{
		255, 225, 196, 170, 145, 123, 102, 84,
		68, 54, 43, 33, 26, 20, 17, 16,
	}
	smoothWeights32 = [...]uint8{
		255, 240, 225, 210, 196, 182, 169, 157,
		145, 133, 122, 111, 101, 92, 83, 74,
		66, 59, 52, 45, 39, 34, 29, 25,
		21, 17, 14, 12, 10, 9, 8, 8,
	}
	smoothWeights64 = [...]uint8{
		255, 248, 240, 233, 225, 218, 210, 203,
		196, 189, 182, 176, 169, 163, 156, 150,
		144, 138, 133, 127, 121, 116, 111, 106,
		101, 96, 91, 86, 82, 77, 73, 69,
		65, 61, 57, 54, 50, 47, 44, 41,
		38, 35, 32, 29, 27, 25, 22, 20,
		18, 16, 15, 13, 12, 10, 9, 8,
		7, 6, 6, 5, 5, 4, 4, 4,
	}
)

func predictDirectionalZ1(dst []byte, stride, width, height, angle int, top []byte, topLeft uint8, flags uint16) error {
	if angle <= 0 || angle >= 90 {
		return fmt.Errorf("decoder: invalid z1 angle %d", angle)
	}
	dx := directionalDerivative(angle)
	isSmooth := flags&angleSmoothEdgeFlag != 0
	enableEdgeFilter := flags&angleUseEdgeFilterFlag != 0
	topLen := width + minInt(width, height)
	edge := top[:minInt(topLen, len(top))]
	baseInc := 1
	var topTLScratch *scratchBuffer[byte]
	var edgeScratch *scratchBuffer[byte]
	if enableEdgeFilter {
		if useIntraEdgeUpsample(width+height, 90-angle, isSmooth) {
			topTLScratch = takeScratch[byte](&intraDirectionalByteScratchPool, topLen+1)
			topTL := topTLScratch.buf[:topLen+1]
			clear(topTL)
			topTL[0] = topLeft
			copy(topTL[1:], top[:minInt(topLen, len(top))])
			edgeScratch = takeScratch[byte](&intraDirectionalByteScratchPool, (width+height)*2-1)
			edge = upsampleDirectionalEdge8Into(edgeScratch.buf[:(width+height)*2-1], topTL, width+height)
			dx <<= 1
			baseInc = 2
		} else if strength := intraEdgeFilterStrength(width+height, 90-angle, isSmooth); strength > 0 {
			topTLScratch = takeScratch[byte](&intraDirectionalByteScratchPool, topLen+1)
			topTL := topTLScratch.buf[:topLen+1]
			clear(topTL)
			topTL[0] = topLeft
			copy(topTL[1:], top[:minInt(topLen, len(top))])
			edgeScratch = takeScratch[byte](&intraDirectionalByteScratchPool, width+height)
			edge = edgeScratch.buf[:width+height]
			filterDirectionalEdge8Into(edge, topTL, strength)
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
				row[x] = uint8((v + 32) >> 6)
			} else {
				fillBlock8(row[x:], stride, width-x, 1, edge[maxBaseX])
				break
			}
			base += baseInc
		}
	}
	putScratch(&intraDirectionalByteScratchPool, edgeScratch)
	putScratch(&intraDirectionalByteScratchPool, topTLScratch)
	return nil
}

func predictDirectionalZ2(dst []byte, stride, width, height, angle int, topWithTL, leftWithTL []byte, flags uint16) error {
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
	var topRefScratch *scratchBuffer[byte]
	var leftRefScratch *scratchBuffer[byte]
	if enableEdgeFilter {
		if useIntraEdgeUpsample(width+height, angle-90, isSmooth) {
			topRefScratch = takeScratch[byte](&intraDirectionalByteScratchPool, len(topRef)*2-1)
			topRef = upsampleDirectionalEdge8RangeInto(topRefScratch.buf[:len(topRef)*2-1], topRef, len(topRef), -1, -1, len(topRef)-1)
			dx <<= 1
			upsampleAbove = true
		} else if strength := intraEdgeFilterStrength(width+height, angle-90, isSmooth); strength > 0 {
			topRefScratch = takeScratch[byte](&intraDirectionalByteScratchPool, width+1)
			topRef = topRefScratch.buf[:width+1]
			topRef[0] = topWithTL[0]
			filterDirectionalEdge8RangeInto(topRef[1:], topWithTL, width, 0, width, 0, -1, width, strength)
		}
		if useIntraEdgeUpsample(width+height, 180-angle, isSmooth) {
			leftRefScratch = takeScratch[byte](&intraDirectionalByteScratchPool, len(leftRef)*2-1)
			leftRef = upsampleDirectionalEdge8RangeInto(leftRefScratch.buf[:len(leftRef)*2-1], leftRef, len(leftRef), -1, -1, len(leftRef)-1)
			dy <<= 1
			upsampleLeft = true
		} else if strength := intraEdgeFilterStrength(width+height, 180-angle, isSmooth); strength > 0 {
			leftRefScratch = takeScratch[byte](&intraDirectionalByteScratchPool, height+1)
			leftRef = filterDirectionalZ2LeftEdge8Into(leftRefScratch.buf[:height+1], leftWithTL[1:], leftWithTL[0], height, strength)
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
				row[x] = uint8((v + 32) >> 6)
			} else {
				baseY := ypos >> 6
				fracY := ypos & 0x3e
				idx := minInt(maxIntLocal(0, baseY+leftBase), len(leftRef)-2)
				v := int(leftRef[idx])*(64-fracY) + int(leftRef[idx+1])*fracY
				row[x] = uint8((v + 32) >> 6)
			}
			baseX += baseIncX
			ypos -= dy
		}
		xpos -= dx
	}
	putScratch(&intraDirectionalByteScratchPool, leftRefScratch)
	putScratch(&intraDirectionalByteScratchPool, topRefScratch)
	return nil
}

func predictDirectionalZ3(dst []byte, stride, width, height, angle int, left []byte, topLeft uint8, flags uint16) error {
	if angle <= 180 || angle >= 270 {
		return fmt.Errorf("decoder: invalid z3 angle %d", angle)
	}
	dy := directionalDerivative(270 - angle)
	isSmooth := flags&angleSmoothEdgeFlag != 0
	enableEdgeFilter := flags&angleUseEdgeFilterFlag != 0
	leftLen := height + minInt(width, height)
	edge := left[:minInt(leftLen, len(left))]
	if len(edge) < leftLen {
		fixedScratch := takeScratch[byte](&intraDirectionalByteScratchPool, leftLen)
		fixed := fixedScratch.buf[:leftLen]
		copy(fixed, edge)
		if len(edge) > 0 {
			fillRunUint8(fixed, len(edge), leftLen-len(edge), edge[len(edge)-1])
		}
		defer putScratch(&intraDirectionalByteScratchPool, fixedScratch)
		edge = fixed
	}
	var bottomLeftScratch *scratchBuffer[byte]
	if len(edge) > height && height > 0 && edge[height-1] != 0 {
		tailZero := true
		for i := height; i < len(edge); i++ {
			if edge[i] != 0 {
				tailZero = false
				break
			}
		}
		if tailZero {
			bottomLeftScratch = takeScratch[byte](&intraDirectionalByteScratchPool, len(edge))
			fixed := bottomLeftScratch.buf[:len(edge)]
			copy(fixed, edge)
			fillRunUint8(fixed, height, len(fixed)-height, fixed[height-1])
			edge = fixed
		}
	}
	baseInc := 1
	var leftTLScratch *scratchBuffer[byte]
	var edgeScratch *scratchBuffer[byte]
	if enableEdgeFilter {
		total := width + height
		from := maxIntLocal(width-height, 0)
		if useIntraEdgeUpsample(width+height, angle-180, isSmooth) {
			edgeScratch = takeScratch[byte](&intraDirectionalByteScratchPool, (width+height)*2-1)
			edge = upsampleDirectionalZ3Edge8Into(edgeScratch.buf[:(width+height)*2-1], edge, topLeft, total, from)
			dy <<= 1
			baseInc = 2
		} else if strength := intraEdgeFilterStrength(width+height, angle-180, isSmooth); strength > 0 {
			edgeScratch = takeScratch[byte](&intraDirectionalByteScratchPool, width+height)
			edge = filterDirectionalZ3Edge8Into(edgeScratch.buf[:width+height], edge, topLeft, total, from, strength)
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
				dst[y*stride+x] = uint8((v + 32) >> 6)
			} else {
				for ; y < height; y++ {
					dst[y*stride+x] = edge[maxBaseY]
				}
				break
			}
			base += baseInc
		}
	}
	putScratch(&intraDirectionalByteScratchPool, edgeScratch)
	putScratch(&intraDirectionalByteScratchPool, leftTLScratch)
	putScratch(&intraDirectionalByteScratchPool, bottomLeftScratch)
	return nil
}

func prepareIntraLumaTarget(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int) ([]byte, int, []byte, []byte, uint8, bool, bool, intraTargetWriteback8, error) {
	return prepareIntraLumaTargetEdges(plane, stride, planeWidth, planeHeight, x, y, width, height, true, true)
}

func prepareIntraLumaTargetEdges(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int, topHasRight, leftHasBottom bool) ([]byte, int, []byte, []byte, uint8, bool, bool, intraTargetWriteback8, error) {
	return prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, y > 0, x > 0, topHasRight, leftHasBottom)
}

func prepareIntraLumaTargetEdgesWithNeighbors(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int, haveTop, haveLeft, topHasRight, leftHasBottom bool) ([]byte, int, []byte, []byte, uint8, bool, bool, intraTargetWriteback8, error) {
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight {
		return nil, 0, nil, nil, 0, false, false, intraTargetWriteback8{}, fmt.Errorf("decoder: block (%d,%d %dx%d) out of plane bounds %dx%d", x, y, width, height, planeWidth, planeHeight)
	}
	if stride < planeWidth {
		return nil, 0, nil, nil, 0, false, false, intraTargetWriteback8{}, fmt.Errorf("decoder: plane stride %d < width %d", stride, planeWidth)
	}
	if len(plane) < (planeHeight-1)*stride+planeWidth {
		return nil, 0, nil, nil, 0, false, false, intraTargetWriteback8{}, fmt.Errorf("decoder: plane buffer too small")
	}

	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil, 0, nil, nil, 0, false, false, intraTargetWriteback8{}, fmt.Errorf("decoder: block (%d,%d %dx%d) has no visible pixels in %dx%d plane", x, y, width, height, planeWidth, planeHeight)
	}

	haveTop = haveTop && y > 0
	haveLeft = haveLeft && x > 0
	edgeLen := width + height
	var top []byte
	var left []byte
	topLeft := uint8(128)
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
			leftScratch := takeScratch[byte](&intraTargetByteScratchPool, leftAvail)
			left = leftScratch.buf[:leftAvail]
			for i := 0; i < leftAvail; i++ {
				left[i] = plane[(y+i)*stride+x-1]
			}
			return dst, stride, top, left, topLeftValue(top, left, topLeft, haveTop, haveLeft), haveTop, haveLeft, intraTargetWriteback8{leftScratch: leftScratch}, nil
		}
		return dst, stride, top, left, topLeftValue(top, left, topLeft, haveTop, haveLeft), haveTop, haveLeft, intraTargetWriteback8{}, nil
	}

	dstScratch := takeScratch[byte](&intraTargetByteScratchPool, width*height)
	dst := dstScratch.buf[:width*height]
	writeback := intraTargetWriteback8{
		plane:         plane,
		dst:           dst,
		planeStride:   stride,
		dstStride:     width,
		x:             x,
		y:             y,
		visibleWidth:  visibleWidth,
		visibleHeight: visibleHeight,
		dstScratch:    dstScratch,
	}
	if haveTop {
		topLimit := width
		if topHasRight {
			topLimit = edgeLen
		}
		topScratch := takeScratch[byte](&intraTargetByteScratchPool, topLimit)
		top = topScratch.buf[:topLimit]
		topAvail := minInt(topLimit, planeWidth-x)
		copy(top, plane[(y-1)*stride+x:(y-1)*stride+x+topAvail])
		fillRunUint8(top, topAvail, topLimit-topAvail, top[topAvail-1])
		writeback.topScratch = topScratch
		if !haveLeft {
			topLeft = top[0]
		}
	}
	if haveLeft {
		leftLimit := height
		if leftHasBottom {
			leftLimit = edgeLen
		}
		leftScratch := takeScratch[byte](&intraTargetByteScratchPool, leftLimit)
		left = leftScratch.buf[:leftLimit]
		leftAvail := minInt(leftLimit, planeHeight-y)
		for i := 0; i < leftAvail; i++ {
			left[i] = plane[(y+i)*stride+x-1]
		}
		fillRunUint8(left, leftAvail, leftLimit-leftAvail, left[leftAvail-1])
		writeback.leftScratch = leftScratch
		if !haveTop {
			topLeft = left[0]
		}
	}
	return dst, width, top, left, topLeft, haveTop, haveLeft, writeback, nil
}

func topLeftValue(top, left []byte, topLeft uint8, haveTop, haveLeft bool) uint8 {
	if haveTop && !haveLeft {
		return top[0]
	}
	if haveLeft && !haveTop {
		return left[0]
	}
	return topLeft
}

func extendedTopEdge(plane []byte, stride, planeWidth, x, y, width int) []byte {
	edge := make([]byte, width)
	available := minInt(width, planeWidth-x)
	copy(edge, plane[(y-1)*stride+x:(y-1)*stride+x+available])
	fillRunUint8(edge, available, width-available, edge[available-1])
	return edge
}

func extendedLeftEdge(plane []byte, stride, planeHeight, x, y, height int) []byte {
	edge := make([]byte, height)
	available := minInt(height, planeHeight-y)
	for i := 0; i < available; i++ {
		edge[i] = plane[(y+i)*stride+x-1]
	}
	fillRunUint8(edge, available, height-available, edge[available-1])
	return edge
}
