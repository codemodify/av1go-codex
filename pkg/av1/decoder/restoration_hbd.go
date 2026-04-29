package decoder

import "github.com/codemodify/av1go-codex/pkg/av1/obu"

func applyRestoration420_16(yPlane []uint16, yStride, width, height int, uPlane []uint16, uStride, chromaWidth, chromaHeight int, vPlane []uint16, vStride int, hdr *obu.FrameHeader, state *RestorationState, bitDepth uint8) {
	if hdr == nil || state == nil {
		return
	}
	srcY := append([]uint16(nil), yPlane...)
	srcU := append([]uint16(nil), uPlane...)
	srcV := append([]uint16(nil), vPlane...)
	applyRestorationPlane16(yPlane, srcY, yStride, width, height, &state.Planes[0], bitDepth)
	applyRestorationPlane16(uPlane, srcU, uStride, chromaWidth, chromaHeight, &state.Planes[1], bitDepth)
	applyRestorationPlane16(vPlane, srcV, vStride, chromaWidth, chromaHeight, &state.Planes[2], bitDepth)
}

func applyRestorationPlane16(dst, src []uint16, stride, width, height int, plane *RestorationPlane, bitDepth uint8) {
	if plane == nil || !plane.Enabled || len(plane.Units) == 0 || stride <= 0 || width <= 0 || height <= 0 {
		return
	}
	for unitY := 0; unitY < plane.UnitsY; unitY++ {
		for unitX := 0; unitX < plane.UnitsX; unitX++ {
			unit := plane.Units[plane.index(unitX, unitY)]
			if !unit.Parsed || unit.Type == restorationTypeNone {
				continue
			}
			x0 := unitX * plane.UnitSize
			y0 := unitY * plane.UnitSize
			x1 := minInt(width, x0+plane.UnitSize)
			y1 := minInt(height, y0+plane.UnitSize)
			if x1 <= x0 || y1 <= y0 {
				continue
			}
			if unit.Type == restorationTypeWiener {
				applyWienerRestorationUnit16(dst, src, stride, width, height, x0, y0, x1, y1, unit, bitDepth)
				continue
			}
			if unit.Type >= restorationTypeSGRProj {
				sgrIdx := int(unit.Type - restorationTypeSGRProj)
				if sgrIdx < len(restoreSGRParams) {
					applySGRRestorationUnit16(dst, src, stride, width, height, x0, y0, x1, y1, unit, restoreSGRParams[sgrIdx], bitDepth)
				}
			}
		}
	}
}

func applyWienerRestorationUnit16(dst, src []uint16, stride, width, height, x0, y0, x1, y1 int, unit RestorationUnit, bitDepth uint8) {
	unitW := x1 - x0
	unitH := y1 - y0
	if unitW <= 0 || unitH <= 0 {
		return
	}
	tmp := make([]int, unitW*unitH)
	fh := restorationWienerHorizontal16(unit)
	fv := restorationWienerVertical(unit)
	roundBitsH := 3
	roundBitsV := 11
	if bitDepth == 12 {
		roundBitsH += 2
		roundBitsV -= 2
	}
	roundingOffH := 1 << (roundBitsH - 1)
	clipLimit := 1 << (int(bitDepth) + 1 + 7 - roundBitsH)
	roundingOffV := 1 << (roundBitsV - 1)
	roundOffset := 1 << (int(bitDepth) + roundBitsV - 1)

	for y := 0; y < unitH; y++ {
		for x := 0; x < unitW; x++ {
			px := x0 + x
			py := y0 + y
			sum := 1 << (int(bitDepth) + 6)
			for k := -3; k <= 3; k++ {
				sum += int(sampleRestorationPlane16(src, stride, width, height, px+k, py)) * fh[k+3]
			}
			v := (sum + roundingOffH) >> roundBitsH
			if v < 0 {
				v = 0
			} else if v >= clipLimit {
				v = clipLimit - 1
			}
			tmp[y*unitW+x] = v
		}
	}
	for y := 0; y < unitH; y++ {
		row := dst[(y0+y)*stride:]
		for x := 0; x < unitW; x++ {
			sum := -roundOffset
			for k := -3; k <= 3; k++ {
				yy := minInt(unitH-1, maxIntLocal(0, y+k))
				sum += tmp[yy*unitW+x] * fv[k+3]
			}
			row[x0+x] = clipSample((sum+roundingOffV)>>roundBitsV, bitDepth)
		}
	}
}

func restorationWienerHorizontal16(unit RestorationUnit) [7]int {
	a := int(unit.FilterH[0])
	b := int(unit.FilterH[1])
	c := int(unit.FilterH[2])
	return [7]int{a, b, c, 128 - 2*(a+b+c), c, b, a}
}

func applySGRRestorationUnit16(dst, src []uint16, stride, width, height, x0, y0, x1, y1 int, unit RestorationUnit, params [2]uint16, bitDepth uint8) {
	w0 := int(unit.SGRWeights[0])
	w1 := 128 - (int(unit.SGRWeights[0]) + int(unit.SGRWeights[1]))
	for py := y0; py < y1; py++ {
		row := dst[py*stride:]
		for px := x0; px < x1; px++ {
			srcV := int(src[py*stride+px])
			delta := 0
			if params[0] != 0 {
				filt5 := sgrFilteredPixel16(src, stride, width, height, px, py, 2, int(params[0]), bitDepth)
				delta += w0 * (filt5 - srcV)
			}
			if params[1] != 0 {
				filt3 := sgrFilteredPixel16(src, stride, width, height, px, py, 1, int(params[1]), bitDepth)
				delta += w1 * (filt3 - srcV)
			}
			row[px] = clipSample(srcV+((delta+64)>>7), bitDepth)
		}
	}
}

func sgrFilteredPixel16(src []uint16, stride, width, height, x, y, radius, strength int, bitDepth uint8) int {
	n := 2*radius + 1
	count := n * n
	shift := maxIntLocal(int(bitDepth)-8, 0)
	sum := 0
	scaledSum := 0
	scaledSumSq := 0
	for yy := -radius; yy <= radius; yy++ {
		sy := minInt(height-1, maxIntLocal(0, y+yy))
		row := src[sy*stride:]
		for xx := -radius; xx <= radius; xx++ {
			sx := minInt(width-1, maxIntLocal(0, x+xx))
			v := int(row[sx])
			sum += v
			vs := v >> shift
			scaledSum += vs
			scaledSumSq += vs * vs
		}
	}
	p := count*scaledSumSq - scaledSum*scaledSum
	if p < 0 {
		p = 0
	}
	z := (p*strength + (1 << 19)) >> 20
	if z > 255 {
		z = 255
	}
	xByX := 255
	if z > 0 {
		xByX = 256 / z
		if xByX > 255 {
			xByX = 255
		}
	}
	srcV := int(src[y*stride+x])
	mean := (sum + count/2) / count
	return srcV + ((xByX*(mean-srcV) + 128) >> 8)
}

func sampleRestorationPlane16(src []uint16, stride, width, height, x, y int) uint16 {
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
	return src[y*stride+x]
}
