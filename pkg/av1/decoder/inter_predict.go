package decoder

import (
	"fmt"
	"sync"
)

var interTargetByteScratchPool sync.Pool

const interClampedIndexBufLen = 136

func validateInterPredictBlock8Args(dst []byte, dstStride, planeWidth, planeHeight int, refPlane []byte, refStride, refWidth, refHeight int) error {
	if refWidth <= 0 || refHeight <= 0 {
		return fmt.Errorf("decoder: invalid reference plane %dx%d", refWidth, refHeight)
	}
	if dstStride < planeWidth {
		return fmt.Errorf("decoder: destination stride %d < width %d", dstStride, planeWidth)
	}
	if len(dst) < (planeHeight-1)*dstStride+planeWidth {
		return fmt.Errorf("decoder: destination plane buffer too small")
	}
	if refStride < refWidth {
		return fmt.Errorf("decoder: reference stride %d < width %d", refStride, refWidth)
	}
	if len(refPlane) < (refHeight-1)*refStride+refWidth {
		return fmt.Errorf("decoder: reference plane buffer too small")
	}
	return nil
}

type interTargetWriteback8 struct {
	plane         []byte
	dst           []byte
	planeStride   int
	dstStride     int
	x             int
	y             int
	visibleWidth  int
	visibleHeight int
	dstScratch    *scratchBuffer[byte]
}

func (w interTargetWriteback8) commit() {
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

func (w interTargetWriteback8) release() {
	putScratch(&interTargetByteScratchPool, w.dstScratch)
}

func PredictInterLumaBlock8(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector) error {
	return PredictInterLumaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, InterFilterPair{interFilterRegular, interFilterRegular})
}

func PredictInterLumaBlock8WithFilter(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filterMode uint8) error {
	return PredictInterLumaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, InterFilterPair{filterMode, filterMode})
}

func PredictInterLumaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if err := validateInterPredictBlock8Args(dst, dstStride, planeWidth, planeHeight, refPlane, refStride, refWidth, refHeight); err != nil {
		return err
	}
	predictInterLumaBlock8WithFiltersUnchecked(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, filter)
	return nil
}

func predictInterLumaBlock8WithFiltersUnchecked(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) {
	predictInterLumaBlock8WithFiltersAt(dst, dstStride, planeWidth, planeHeight, x, y, width, height, x, y, refPlane, refStride, refWidth, refHeight, mv, filter)
}

func predictInterLumaBlock8WithFiltersAt(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height, sampleX, sampleY int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return
	}
	baseX8 := sampleX*8 + mv.X
	baseY8 := sampleY*8 + mv.Y
	startX, fx := floorDivMod8(baseX8)
	startY, fy := floorDivMod8(baseY8)
	if filter[0] == interFilterBilinear && filter[1] == interFilterBilinear {
		if startX >= 0 && startY >= 0 && startX+visibleWidth+1 <= refWidth && startY+visibleHeight+1 <= refHeight {
			_ = predictInterBlock8BilinearInterior(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, startX, startY, fx, fy, 8)
			return
		}
		_ = predictInterLumaBlock8Bilinear(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, fx, fy)
		return
	}
	useSmallH := width <= 4
	useSmallV := height <= 4
	hf := inter8TapCoeffs(filter[1], fx<<1, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(filter[0], fy<<1, ternaryInt(useSmallV, 4, 8))
	hInterior := startX >= 3 && startX+visibleWidth+4 <= refWidth
	vInterior := startY >= 3 && startY+visibleHeight+4 <= refHeight
	switch {
	case hf == nil && vf == nil:
		var xBuf [interClampedIndexBufLen]int
		var xIdx []int
		if startX < 0 || startX+visibleWidth > refWidth {
			xIdx = precomputeClampedIndices(&xBuf, startX, visibleWidth, refWidth-1)
		}
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := clampInt(startY+py, 0, refHeight-1)
			if startX >= 0 && startX+visibleWidth <= refWidth {
				copy(row[:visibleWidth], refPlane[sy*refStride+startX:sy*refStride+startX+visibleWidth])
				continue
			}
			src := refPlane[sy*refStride:]
			for px := 0; px < visibleWidth; px++ {
				row[px] = src[xIdx[px]]
			}
		}
	case hf != nil && vf != nil:
		if hInterior && vInterior {
			predictInterBlock8HFVFInterior(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, startX, startY, hf, vf)
			return
		}
		predictInterBlock8HFVFClamped(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, hf, vf)
	case hf != nil:
		if hInterior && startY >= 0 && startY+visibleHeight <= refHeight {
			predictInterBlock8HFInterior(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, startX, startY, hf)
			return
		}
		predictInterBlock8HFClamped(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, hf)
	default:
		if vInterior && startX >= 0 && startX+visibleWidth <= refWidth {
			predictInterBlock8VFInterior(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, startX, startY, vf)
			return
		}
		predictInterBlock8VFClamped(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, vf)
	}
}

func interFracShift(frac int) uint {
	switch frac {
	case 8:
		return 3
	case 16:
		return 4
	default:
		shift := uint(0)
		for (1 << shift) < frac {
			shift++
		}
		return shift
	}
}

func predictInterBlock8BilinearInterior(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, startX, startY, fx, fy, frac int) error {
	shift1D := interFracShift(frac)
	if fx == 0 && fy == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			src := refPlane[(startY+py)*refStride+startX:]
			copy(row[:visibleWidth], src[:visibleWidth])
		}
		return nil
	}
	if fy == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			src := refPlane[(startY+py)*refStride+startX:]
			for px := 0; px < visibleWidth; px++ {
				row[px] = uint8(((frac-fx)*int(src[px]) + fx*int(src[px+1]) + (frac >> 1)) >> shift1D)
			}
		}
		return nil
	}
	if fx == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			src0 := refPlane[(startY+py)*refStride+startX:]
			src1 := refPlane[(startY+py+1)*refStride+startX:]
			for px := 0; px < visibleWidth; px++ {
				row[px] = uint8(((frac-fy)*int(src0[px]) + fy*int(src1[px]) + (frac >> 1)) >> shift1D)
			}
		}
		return nil
	}
	shift2D := shift1D << 1
	round := 1 << (shift2D - 1)
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		src0 := refPlane[(startY+py)*refStride+startX:]
		src1 := refPlane[(startY+py+1)*refStride+startX:]
		for px := 0; px < visibleWidth; px++ {
			top := (frac-fx)*int(src0[px]) + fx*int(src0[px+1])
			bottom := (frac-fx)*int(src1[px]) + fx*int(src1[px+1])
			row[px] = uint8((top*(frac-fy) + bottom*fy + round) >> shift2D)
		}
	}
	return nil
}

func predictInterBlock8HFVFInterior(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, startX, startY int, hf, vf *[8]int) {
	h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
	h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
	v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
	v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
	if h0 == 0 && h7 == 0 && v0 == 0 && v7 == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := startY + py
			_, r1 := refPlane[(sy-3)*refStride+startX-3:], refPlane[(sy-2)*refStride+startX-3:]
			r2 := refPlane[(sy-1)*refStride+startX-3:]
			r3 := refPlane[sy*refStride+startX-3:]
			r4 := refPlane[(sy+1)*refStride+startX-3:]
			r5 := refPlane[(sy+2)*refStride+startX-3:]
			r6, _ := refPlane[(sy+3)*refStride+startX-3:], refPlane[(sy+4)*refStride+startX-3:]
			for px := 0; px < visibleWidth; px++ {
				m1 := (h1*int(r1[px+1]) + h2*int(r1[px+2]) + h3*int(r1[px+3]) + h4*int(r1[px+4]) + h5*int(r1[px+5]) + h6*int(r1[px+6]) + 2) >> 2
				m2 := (h1*int(r2[px+1]) + h2*int(r2[px+2]) + h3*int(r2[px+3]) + h4*int(r2[px+4]) + h5*int(r2[px+5]) + h6*int(r2[px+6]) + 2) >> 2
				m3 := (h1*int(r3[px+1]) + h2*int(r3[px+2]) + h3*int(r3[px+3]) + h4*int(r3[px+4]) + h5*int(r3[px+5]) + h6*int(r3[px+6]) + 2) >> 2
				m4 := (h1*int(r4[px+1]) + h2*int(r4[px+2]) + h3*int(r4[px+3]) + h4*int(r4[px+4]) + h5*int(r4[px+5]) + h6*int(r4[px+6]) + 2) >> 2
				m5 := (h1*int(r5[px+1]) + h2*int(r5[px+2]) + h3*int(r5[px+3]) + h4*int(r5[px+4]) + h5*int(r5[px+5]) + h6*int(r5[px+6]) + 2) >> 2
				m6 := (h1*int(r6[px+1]) + h2*int(r6[px+2]) + h3*int(r6[px+3]) + h4*int(r6[px+4]) + h5*int(r6[px+5]) + h6*int(r6[px+6]) + 2) >> 2
				sum := v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6
				row[px] = clipByte((sum + 512) >> 10)
			}
		}
		return
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		sy := startY + py
		r0 := refPlane[(sy-3)*refStride+startX-3:]
		r1 := refPlane[(sy-2)*refStride+startX-3:]
		r2 := refPlane[(sy-1)*refStride+startX-3:]
		r3 := refPlane[sy*refStride+startX-3:]
		r4 := refPlane[(sy+1)*refStride+startX-3:]
		r5 := refPlane[(sy+2)*refStride+startX-3:]
		r6 := refPlane[(sy+3)*refStride+startX-3:]
		r7 := refPlane[(sy+4)*refStride+startX-3:]
		for px := 0; px < visibleWidth; px++ {
			m0 := (h0*int(r0[px+0]) + h1*int(r0[px+1]) + h2*int(r0[px+2]) + h3*int(r0[px+3]) + h4*int(r0[px+4]) + h5*int(r0[px+5]) + h6*int(r0[px+6]) + h7*int(r0[px+7]) + 2) >> 2
			m1 := (h0*int(r1[px+0]) + h1*int(r1[px+1]) + h2*int(r1[px+2]) + h3*int(r1[px+3]) + h4*int(r1[px+4]) + h5*int(r1[px+5]) + h6*int(r1[px+6]) + h7*int(r1[px+7]) + 2) >> 2
			m2 := (h0*int(r2[px+0]) + h1*int(r2[px+1]) + h2*int(r2[px+2]) + h3*int(r2[px+3]) + h4*int(r2[px+4]) + h5*int(r2[px+5]) + h6*int(r2[px+6]) + h7*int(r2[px+7]) + 2) >> 2
			m3 := (h0*int(r3[px+0]) + h1*int(r3[px+1]) + h2*int(r3[px+2]) + h3*int(r3[px+3]) + h4*int(r3[px+4]) + h5*int(r3[px+5]) + h6*int(r3[px+6]) + h7*int(r3[px+7]) + 2) >> 2
			m4 := (h0*int(r4[px+0]) + h1*int(r4[px+1]) + h2*int(r4[px+2]) + h3*int(r4[px+3]) + h4*int(r4[px+4]) + h5*int(r4[px+5]) + h6*int(r4[px+6]) + h7*int(r4[px+7]) + 2) >> 2
			m5 := (h0*int(r5[px+0]) + h1*int(r5[px+1]) + h2*int(r5[px+2]) + h3*int(r5[px+3]) + h4*int(r5[px+4]) + h5*int(r5[px+5]) + h6*int(r5[px+6]) + h7*int(r5[px+7]) + 2) >> 2
			m6 := (h0*int(r6[px+0]) + h1*int(r6[px+1]) + h2*int(r6[px+2]) + h3*int(r6[px+3]) + h4*int(r6[px+4]) + h5*int(r6[px+5]) + h6*int(r6[px+6]) + h7*int(r6[px+7]) + 2) >> 2
			m7 := (h0*int(r7[px+0]) + h1*int(r7[px+1]) + h2*int(r7[px+2]) + h3*int(r7[px+3]) + h4*int(r7[px+4]) + h5*int(r7[px+5]) + h6*int(r7[px+6]) + h7*int(r7[px+7]) + 2) >> 2
			sum := v0*m0 + v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6 + v7*m7
			row[px] = clipByte((sum + 512) >> 10)
		}
	}
}

func predictInterBlock8HFInterior(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, startX, startY int, hf *[8]int) {
	h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
	h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
	if h0 == 0 && h7 == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			src := refPlane[(startY+py)*refStride+startX-3:]
			for px := 0; px < visibleWidth; px++ {
				taps := src[px:]
				sum := h1*int(taps[1]) + h2*int(taps[2]) + h3*int(taps[3]) + h4*int(taps[4]) + h5*int(taps[5]) + h6*int(taps[6])
				row[px] = clipByte((sum + 34) >> 6)
			}
		}
		return
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		src := refPlane[(startY+py)*refStride+startX-3:]
		for px := 0; px < visibleWidth; px++ {
			taps := src[px:]
			sum := h0*int(taps[0]) + h1*int(taps[1]) + h2*int(taps[2]) + h3*int(taps[3]) + h4*int(taps[4]) + h5*int(taps[5]) + h6*int(taps[6]) + h7*int(taps[7])
			row[px] = clipByte((sum + 34) >> 6)
		}
	}
}

func predictInterBlock8VFInterior(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, startX, startY int, vf *[8]int) {
	v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
	v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
	if v0 == 0 && v7 == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := startY + py
			r1 := refPlane[(sy-2)*refStride+startX:]
			r2 := refPlane[(sy-1)*refStride+startX:]
			r3 := refPlane[sy*refStride+startX:]
			r4 := refPlane[(sy+1)*refStride+startX:]
			r5 := refPlane[(sy+2)*refStride+startX:]
			r6 := refPlane[(sy+3)*refStride+startX:]
			for px := 0; px < visibleWidth; px++ {
				sum := v1*int(r1[px]) +
					v2*int(r2[px]) +
					v3*int(r3[px]) +
					v4*int(r4[px]) +
					v5*int(r5[px]) +
					v6*int(r6[px])
				row[px] = clipByte((sum + 32) >> 6)
			}
		}
		return
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		sy := startY + py
		r0 := refPlane[(sy-3)*refStride+startX:]
		r1 := refPlane[(sy-2)*refStride+startX:]
		r2 := refPlane[(sy-1)*refStride+startX:]
		r3 := refPlane[sy*refStride+startX:]
		r4 := refPlane[(sy+1)*refStride+startX:]
		r5 := refPlane[(sy+2)*refStride+startX:]
		r6 := refPlane[(sy+3)*refStride+startX:]
		r7 := refPlane[(sy+4)*refStride+startX:]
		for px := 0; px < visibleWidth; px++ {
			sum := v0*int(r0[px]) +
				v1*int(r1[px]) +
				v2*int(r2[px]) +
				v3*int(r3[px]) +
				v4*int(r4[px]) +
				v5*int(r5[px]) +
				v6*int(r6[px]) +
				v7*int(r7[px])
			row[px] = clipByte((sum + 32) >> 6)
		}
	}
}

func precomputeClampedIndices(dst *[interClampedIndexBufLen]int, start, count, max int) []int {
	xIdx := dst[:count]
	for i := 0; i < count; i++ {
		x := start + i
		if x < 0 {
			x = 0
		} else if x > max {
			x = max
		}
		xIdx[i] = x
	}
	return xIdx
}

func fillClampedRows8(rows *[8][]byte, plane []byte, stride, maxY, centerY int) {
	for i := range rows {
		y := centerY + i - 3
		if y < 0 {
			y = 0
		} else if y > maxY {
			y = maxY
		}
		rows[i] = plane[y*stride:]
	}
}

func predictInterBlock8HFVFClamped(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, refWidth, refHeight, startX, startY int, hf, vf *[8]int) {
	h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
	h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
	v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
	v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
	maxY := refHeight - 1
	xBase := startX - 3
	xDirect := xBase >= 0 && startX+visibleWidth+4 <= refWidth
	var xBuf [interClampedIndexBufLen]int
	var xIdx []int
	if !xDirect {
		xIdx = precomputeClampedIndices(&xBuf, xBase, visibleWidth+7, refWidth-1)
	}
	var rows [8][]byte
	if h0 == 0 && h7 == 0 && v0 == 0 && v7 == 0 {
		for py := 0; py < visibleHeight; py++ {
			fillClampedRows8(&rows, refPlane, refStride, maxY, startY+py)
			_, r1, r2, r3 := rows[0], rows[1], rows[2], rows[3]
			r4, r5, r6, _ := rows[4], rows[5], rows[6], rows[7]
			row := dst[(y+py)*dstStride+x:]
			if xDirect {
				for px := 0; px < visibleWidth; px++ {
					xp := xBase + px
					m1 := (h1*int(r1[xp+1]) + h2*int(r1[xp+2]) + h3*int(r1[xp+3]) + h4*int(r1[xp+4]) + h5*int(r1[xp+5]) + h6*int(r1[xp+6]) + 2) >> 2
					m2 := (h1*int(r2[xp+1]) + h2*int(r2[xp+2]) + h3*int(r2[xp+3]) + h4*int(r2[xp+4]) + h5*int(r2[xp+5]) + h6*int(r2[xp+6]) + 2) >> 2
					m3 := (h1*int(r3[xp+1]) + h2*int(r3[xp+2]) + h3*int(r3[xp+3]) + h4*int(r3[xp+4]) + h5*int(r3[xp+5]) + h6*int(r3[xp+6]) + 2) >> 2
					m4 := (h1*int(r4[xp+1]) + h2*int(r4[xp+2]) + h3*int(r4[xp+3]) + h4*int(r4[xp+4]) + h5*int(r4[xp+5]) + h6*int(r4[xp+6]) + 2) >> 2
					m5 := (h1*int(r5[xp+1]) + h2*int(r5[xp+2]) + h3*int(r5[xp+3]) + h4*int(r5[xp+4]) + h5*int(r5[xp+5]) + h6*int(r5[xp+6]) + 2) >> 2
					m6 := (h1*int(r6[xp+1]) + h2*int(r6[xp+2]) + h3*int(r6[xp+3]) + h4*int(r6[xp+4]) + h5*int(r6[xp+5]) + h6*int(r6[xp+6]) + 2) >> 2
					sum := v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6
					row[px] = clipByte((sum + 512) >> 10)
				}
				continue
			}
			for px := 0; px < visibleWidth; px++ {
				m1 := (h1*int(r1[xIdx[px+1]]) + h2*int(r1[xIdx[px+2]]) + h3*int(r1[xIdx[px+3]]) + h4*int(r1[xIdx[px+4]]) + h5*int(r1[xIdx[px+5]]) + h6*int(r1[xIdx[px+6]]) + 2) >> 2
				m2 := (h1*int(r2[xIdx[px+1]]) + h2*int(r2[xIdx[px+2]]) + h3*int(r2[xIdx[px+3]]) + h4*int(r2[xIdx[px+4]]) + h5*int(r2[xIdx[px+5]]) + h6*int(r2[xIdx[px+6]]) + 2) >> 2
				m3 := (h1*int(r3[xIdx[px+1]]) + h2*int(r3[xIdx[px+2]]) + h3*int(r3[xIdx[px+3]]) + h4*int(r3[xIdx[px+4]]) + h5*int(r3[xIdx[px+5]]) + h6*int(r3[xIdx[px+6]]) + 2) >> 2
				m4 := (h1*int(r4[xIdx[px+1]]) + h2*int(r4[xIdx[px+2]]) + h3*int(r4[xIdx[px+3]]) + h4*int(r4[xIdx[px+4]]) + h5*int(r4[xIdx[px+5]]) + h6*int(r4[xIdx[px+6]]) + 2) >> 2
				m5 := (h1*int(r5[xIdx[px+1]]) + h2*int(r5[xIdx[px+2]]) + h3*int(r5[xIdx[px+3]]) + h4*int(r5[xIdx[px+4]]) + h5*int(r5[xIdx[px+5]]) + h6*int(r5[xIdx[px+6]]) + 2) >> 2
				m6 := (h1*int(r6[xIdx[px+1]]) + h2*int(r6[xIdx[px+2]]) + h3*int(r6[xIdx[px+3]]) + h4*int(r6[xIdx[px+4]]) + h5*int(r6[xIdx[px+5]]) + h6*int(r6[xIdx[px+6]]) + 2) >> 2
				sum := v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6
				row[px] = clipByte((sum + 512) >> 10)
			}
		}
		return
	}
	for py := 0; py < visibleHeight; py++ {
		fillClampedRows8(&rows, refPlane, refStride, maxY, startY+py)
		r0, r1, r2, r3 := rows[0], rows[1], rows[2], rows[3]
		r4, r5, r6, r7 := rows[4], rows[5], rows[6], rows[7]
		row := dst[(y+py)*dstStride+x:]
		if xDirect {
			for px := 0; px < visibleWidth; px++ {
				xp := xBase + px
				m0 := (h0*int(r0[xp+0]) + h1*int(r0[xp+1]) + h2*int(r0[xp+2]) + h3*int(r0[xp+3]) + h4*int(r0[xp+4]) + h5*int(r0[xp+5]) + h6*int(r0[xp+6]) + h7*int(r0[xp+7]) + 2) >> 2
				m1 := (h0*int(r1[xp+0]) + h1*int(r1[xp+1]) + h2*int(r1[xp+2]) + h3*int(r1[xp+3]) + h4*int(r1[xp+4]) + h5*int(r1[xp+5]) + h6*int(r1[xp+6]) + h7*int(r1[xp+7]) + 2) >> 2
				m2 := (h0*int(r2[xp+0]) + h1*int(r2[xp+1]) + h2*int(r2[xp+2]) + h3*int(r2[xp+3]) + h4*int(r2[xp+4]) + h5*int(r2[xp+5]) + h6*int(r2[xp+6]) + h7*int(r2[xp+7]) + 2) >> 2
				m3 := (h0*int(r3[xp+0]) + h1*int(r3[xp+1]) + h2*int(r3[xp+2]) + h3*int(r3[xp+3]) + h4*int(r3[xp+4]) + h5*int(r3[xp+5]) + h6*int(r3[xp+6]) + h7*int(r3[xp+7]) + 2) >> 2
				m4 := (h0*int(r4[xp+0]) + h1*int(r4[xp+1]) + h2*int(r4[xp+2]) + h3*int(r4[xp+3]) + h4*int(r4[xp+4]) + h5*int(r4[xp+5]) + h6*int(r4[xp+6]) + h7*int(r4[xp+7]) + 2) >> 2
				m5 := (h0*int(r5[xp+0]) + h1*int(r5[xp+1]) + h2*int(r5[xp+2]) + h3*int(r5[xp+3]) + h4*int(r5[xp+4]) + h5*int(r5[xp+5]) + h6*int(r5[xp+6]) + h7*int(r5[xp+7]) + 2) >> 2
				m6 := (h0*int(r6[xp+0]) + h1*int(r6[xp+1]) + h2*int(r6[xp+2]) + h3*int(r6[xp+3]) + h4*int(r6[xp+4]) + h5*int(r6[xp+5]) + h6*int(r6[xp+6]) + h7*int(r6[xp+7]) + 2) >> 2
				m7 := (h0*int(r7[xp+0]) + h1*int(r7[xp+1]) + h2*int(r7[xp+2]) + h3*int(r7[xp+3]) + h4*int(r7[xp+4]) + h5*int(r7[xp+5]) + h6*int(r7[xp+6]) + h7*int(r7[xp+7]) + 2) >> 2
				sum := v0*m0 + v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6 + v7*m7
				row[px] = clipByte((sum + 512) >> 10)
			}
			continue
		}
		for px := 0; px < visibleWidth; px++ {
			m0 := (h0*int(r0[xIdx[px+0]]) + h1*int(r0[xIdx[px+1]]) + h2*int(r0[xIdx[px+2]]) + h3*int(r0[xIdx[px+3]]) + h4*int(r0[xIdx[px+4]]) + h5*int(r0[xIdx[px+5]]) + h6*int(r0[xIdx[px+6]]) + h7*int(r0[xIdx[px+7]]) + 2) >> 2
			m1 := (h0*int(r1[xIdx[px+0]]) + h1*int(r1[xIdx[px+1]]) + h2*int(r1[xIdx[px+2]]) + h3*int(r1[xIdx[px+3]]) + h4*int(r1[xIdx[px+4]]) + h5*int(r1[xIdx[px+5]]) + h6*int(r1[xIdx[px+6]]) + h7*int(r1[xIdx[px+7]]) + 2) >> 2
			m2 := (h0*int(r2[xIdx[px+0]]) + h1*int(r2[xIdx[px+1]]) + h2*int(r2[xIdx[px+2]]) + h3*int(r2[xIdx[px+3]]) + h4*int(r2[xIdx[px+4]]) + h5*int(r2[xIdx[px+5]]) + h6*int(r2[xIdx[px+6]]) + h7*int(r2[xIdx[px+7]]) + 2) >> 2
			m3 := (h0*int(r3[xIdx[px+0]]) + h1*int(r3[xIdx[px+1]]) + h2*int(r3[xIdx[px+2]]) + h3*int(r3[xIdx[px+3]]) + h4*int(r3[xIdx[px+4]]) + h5*int(r3[xIdx[px+5]]) + h6*int(r3[xIdx[px+6]]) + h7*int(r3[xIdx[px+7]]) + 2) >> 2
			m4 := (h0*int(r4[xIdx[px+0]]) + h1*int(r4[xIdx[px+1]]) + h2*int(r4[xIdx[px+2]]) + h3*int(r4[xIdx[px+3]]) + h4*int(r4[xIdx[px+4]]) + h5*int(r4[xIdx[px+5]]) + h6*int(r4[xIdx[px+6]]) + h7*int(r4[xIdx[px+7]]) + 2) >> 2
			m5 := (h0*int(r5[xIdx[px+0]]) + h1*int(r5[xIdx[px+1]]) + h2*int(r5[xIdx[px+2]]) + h3*int(r5[xIdx[px+3]]) + h4*int(r5[xIdx[px+4]]) + h5*int(r5[xIdx[px+5]]) + h6*int(r5[xIdx[px+6]]) + h7*int(r5[xIdx[px+7]]) + 2) >> 2
			m6 := (h0*int(r6[xIdx[px+0]]) + h1*int(r6[xIdx[px+1]]) + h2*int(r6[xIdx[px+2]]) + h3*int(r6[xIdx[px+3]]) + h4*int(r6[xIdx[px+4]]) + h5*int(r6[xIdx[px+5]]) + h6*int(r6[xIdx[px+6]]) + h7*int(r6[xIdx[px+7]]) + 2) >> 2
			m7 := (h0*int(r7[xIdx[px+0]]) + h1*int(r7[xIdx[px+1]]) + h2*int(r7[xIdx[px+2]]) + h3*int(r7[xIdx[px+3]]) + h4*int(r7[xIdx[px+4]]) + h5*int(r7[xIdx[px+5]]) + h6*int(r7[xIdx[px+6]]) + h7*int(r7[xIdx[px+7]]) + 2) >> 2
			sum := v0*m0 + v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6 + v7*m7
			row[px] = clipByte((sum + 512) >> 10)
		}
	}
}

func predictInterBlock8HFClamped(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, refWidth, refHeight, startX, startY int, hf *[8]int) {
	h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
	h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
	maxY := refHeight - 1
	xBase := startX - 3
	xDirect := xBase >= 0 && startX+visibleWidth+4 <= refWidth
	var xBuf [interClampedIndexBufLen]int
	var xIdx []int
	if !xDirect {
		xIdx = precomputeClampedIndices(&xBuf, xBase, visibleWidth+7, refWidth-1)
	}
	if h0 == 0 && h7 == 0 {
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := startY + py
			if sy < 0 {
				sy = 0
			} else if sy > maxY {
				sy = maxY
			}
			src := refPlane[sy*refStride:]
			if xDirect {
				for px := 0; px < visibleWidth; px++ {
					xp := xBase + px
					sum := h1*int(src[xp+1]) + h2*int(src[xp+2]) + h3*int(src[xp+3]) + h4*int(src[xp+4]) + h5*int(src[xp+5]) + h6*int(src[xp+6])
					row[px] = clipByte((sum + 34) >> 6)
				}
				continue
			}
			for px := 0; px < visibleWidth; px++ {
				sum := h1*int(src[xIdx[px+1]]) + h2*int(src[xIdx[px+2]]) + h3*int(src[xIdx[px+3]]) + h4*int(src[xIdx[px+4]]) + h5*int(src[xIdx[px+5]]) + h6*int(src[xIdx[px+6]])
				row[px] = clipByte((sum + 34) >> 6)
			}
		}
		return
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		sy := startY + py
		if sy < 0 {
			sy = 0
		} else if sy > maxY {
			sy = maxY
		}
		src := refPlane[sy*refStride:]
		if xDirect {
			for px := 0; px < visibleWidth; px++ {
				xp := xBase + px
				sum := h0*int(src[xp+0]) + h1*int(src[xp+1]) + h2*int(src[xp+2]) + h3*int(src[xp+3]) + h4*int(src[xp+4]) + h5*int(src[xp+5]) + h6*int(src[xp+6]) + h7*int(src[xp+7])
				row[px] = clipByte((sum + 34) >> 6)
			}
			continue
		}
		for px := 0; px < visibleWidth; px++ {
			sum := h0*int(src[xIdx[px+0]]) + h1*int(src[xIdx[px+1]]) + h2*int(src[xIdx[px+2]]) + h3*int(src[xIdx[px+3]]) + h4*int(src[xIdx[px+4]]) + h5*int(src[xIdx[px+5]]) + h6*int(src[xIdx[px+6]]) + h7*int(src[xIdx[px+7]])
			row[px] = clipByte((sum + 34) >> 6)
		}
	}
}

func predictInterBlock8VFClamped(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, refWidth, refHeight, startX, startY int, vf *[8]int) {
	v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
	v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
	maxY := refHeight - 1
	xDirect := startX >= 0 && startX+visibleWidth <= refWidth
	var xBuf [interClampedIndexBufLen]int
	var xIdx []int
	if !xDirect {
		xIdx = precomputeClampedIndices(&xBuf, startX, visibleWidth, refWidth-1)
	}
	var rows [8][]byte
	if v0 == 0 && v7 == 0 {
		for py := 0; py < visibleHeight; py++ {
			fillClampedRows8(&rows, refPlane, refStride, maxY, startY+py)
			_, r1, r2, r3 := rows[0], rows[1], rows[2], rows[3]
			r4, r5, r6, _ := rows[4], rows[5], rows[6], rows[7]
			row := dst[(y+py)*dstStride+x:]
			if xDirect {
				for px := 0; px < visibleWidth; px++ {
					xp := startX + px
					sum := v1*int(r1[xp]) + v2*int(r2[xp]) + v3*int(r3[xp]) + v4*int(r4[xp]) + v5*int(r5[xp]) + v6*int(r6[xp])
					row[px] = clipByte((sum + 32) >> 6)
				}
				continue
			}
			for px := 0; px < visibleWidth; px++ {
				xp := xIdx[px]
				sum := v1*int(r1[xp]) + v2*int(r2[xp]) + v3*int(r3[xp]) + v4*int(r4[xp]) + v5*int(r5[xp]) + v6*int(r6[xp])
				row[px] = clipByte((sum + 32) >> 6)
			}
		}
		return
	}
	for py := 0; py < visibleHeight; py++ {
		fillClampedRows8(&rows, refPlane, refStride, maxY, startY+py)
		r0, r1, r2, r3 := rows[0], rows[1], rows[2], rows[3]
		r4, r5, r6, r7 := rows[4], rows[5], rows[6], rows[7]
		row := dst[(y+py)*dstStride+x:]
		if xDirect {
			for px := 0; px < visibleWidth; px++ {
				xp := startX + px
				sum := v0*int(r0[xp]) + v1*int(r1[xp]) + v2*int(r2[xp]) + v3*int(r3[xp]) + v4*int(r4[xp]) + v5*int(r5[xp]) + v6*int(r6[xp]) + v7*int(r7[xp])
				row[px] = clipByte((sum + 32) >> 6)
			}
			continue
		}
		for px := 0; px < visibleWidth; px++ {
			xp := xIdx[px]
			sum := v0*int(r0[xp]) + v1*int(r1[xp]) + v2*int(r2[xp]) + v3*int(r3[xp]) + v4*int(r4[xp]) + v5*int(r5[xp]) + v6*int(r6[xp]) + v7*int(r7[xp])
			row[px] = clipByte((sum + 32) >> 6)
		}
	}
}

func predictInterBlock8BilinearClamped(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, refWidth, refHeight int, startX, startY, fx, fy, frac int) {
	shift1D := interFracShift(frac)
	shift2D := shift1D << 1
	round2D := 1 << (shift2D - 1)
	maxY := refHeight - 1
	xDirect := startX >= 0 && startX+visibleWidth+1 <= refWidth
	var xBuf [interClampedIndexBufLen]int
	var xIdx []int
	if !xDirect {
		xIdx = precomputeClampedIndices(&xBuf, startX, visibleWidth+1, refWidth-1)
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		sy0 := startY + py
		if sy0 < 0 {
			sy0 = 0
		} else if sy0 > maxY {
			sy0 = maxY
		}
		src0 := refPlane[sy0*refStride:]
		var src1 []byte
		if fy != 0 {
			sy1 := startY + py + 1
			if sy1 < 0 {
				sy1 = 0
			} else if sy1 > maxY {
				sy1 = maxY
			}
			src1 = refPlane[sy1*refStride:]
		}
		if xDirect {
			for px := 0; px < visibleWidth; px++ {
				p00 := src0[startX+px]
				if fx == 0 && fy == 0 {
					row[px] = p00
					continue
				}
				p10 := src0[startX+px+1]
				if fy == 0 {
					row[px] = uint8(((frac-fx)*int(p00) + fx*int(p10) + (frac >> 1)) >> shift1D)
					continue
				}
				p01 := src1[startX+px]
				if fx == 0 {
					row[px] = uint8(((frac-fy)*int(p00) + fy*int(p01) + (frac >> 1)) >> shift1D)
					continue
				}
				p11 := src1[startX+px+1]
				top := (frac-fx)*int(p00) + fx*int(p10)
				bottom := (frac-fx)*int(p01) + fx*int(p11)
				row[px] = uint8((top*(frac-fy) + bottom*fy + round2D) >> shift2D)
			}
			continue
		}
		for px := 0; px < visibleWidth; px++ {
			p00 := src0[xIdx[px]]
			if fx == 0 && fy == 0 {
				row[px] = p00
				continue
			}
			p10 := src0[xIdx[px+1]]
			if fy == 0 {
				row[px] = uint8(((frac-fx)*int(p00) + fx*int(p10) + (frac >> 1)) >> shift1D)
				continue
			}
			p01 := src1[xIdx[px]]
			if fx == 0 {
				row[px] = uint8(((frac-fy)*int(p00) + fy*int(p01) + (frac >> 1)) >> shift1D)
				continue
			}
			p11 := src1[xIdx[px+1]]
			top := (frac-fx)*int(p00) + fx*int(p10)
			bottom := (frac-fx)*int(p01) + fx*int(p11)
			row[px] = uint8((top*(frac-fy) + bottom*fy + round2D) >> shift2D)
		}
	}
}

func predictInterLumaBlock8Bilinear(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, refWidth, refHeight int, startX, startY, fx, fy int) error {
	predictInterBlock8BilinearClamped(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, fx, fy, 8)
	return nil
}

func ReconstructInterLumaCoefficients8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, lossless bool) error {
	return ReconstructInterLumaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, lossless)
}

func ReconstructInterLumaCoefficients8WithTxType(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, lossless bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	if x < 0 || y < 0 {
		return fmt.Errorf("decoder: negative inter reconstruction origin")
	}
	if stride < planeWidth {
		return fmt.Errorf("decoder: plane stride %d < width %d", stride, planeWidth)
	}
	if len(plane) < (planeHeight-1)*stride+planeWidth {
		return fmt.Errorf("decoder: plane buffer too small")
	}
	if len(coeffs) < int(info.W4)*4*int(info.H4)*4 {
		return fmt.Errorf("decoder: coefficient buffer too small")
	}
	dst, dstStride, writeback, err := prepareInterLumaTarget(plane, stride, planeWidth, planeHeight, x, y, width, height)
	if err != nil {
		return err
	}
	defer writeback.release()
	if err := InverseAddResidualTx8(dst, dstStride, tx, txType, coeffs, lossless); err != nil {
		return fmt.Errorf("decoder: inverse inter transform add at (%d,%d) tx=%d failed (dst_stride=%d dst_len=%d): %w", x, y, tx, dstStride, len(dst), err)
	}
	writeback.commit()
	return nil
}

func floorDivMod8(v int) (int, int) {
	q := v / 8
	r := v % 8
	if r < 0 {
		q--
		r += 8
	}
	return q, r
}

func refSample8(plane []byte, stride, width, height, x, y int) uint8 {
	if x < 0 {
		x = 0
	} else if x >= width {
		x = width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= height {
		y = height - 1
	}
	return plane[y*stride+x]
}

func prepareInterLumaTarget(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int) ([]byte, int, interTargetWriteback8, error) {
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight {
		return nil, 0, interTargetWriteback8{}, fmt.Errorf("decoder: inter block (%d,%d %dx%d) out of plane bounds %dx%d", x, y, width, height, planeWidth, planeHeight)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth == width && visibleHeight == height {
		return plane[y*stride+x:], stride, interTargetWriteback8{}, nil
	}
	dstScratch := takeScratch[byte](&interTargetByteScratchPool, width*height)
	dst := dstScratch.buf[:width*height]
	clear(dst)
	for row := 0; row < visibleHeight; row++ {
		copy(dst[row*width:row*width+visibleWidth], plane[(y+row)*stride+x:(y+row)*stride+x+visibleWidth])
	}
	return dst, width, interTargetWriteback8{
		plane:         plane,
		dst:           dst,
		planeStride:   stride,
		dstStride:     width,
		x:             x,
		y:             y,
		visibleWidth:  visibleWidth,
		visibleHeight: visibleHeight,
		dstScratch:    dstScratch,
	}, nil
}
