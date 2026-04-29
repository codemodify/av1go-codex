package decoder

const compoundIntermediateBits8 = 4

func floorDivMod16Subpel(v int) (int, int) {
	q := v / 16
	r := v % 16
	if r < 0 {
		q--
		r += 16
	}
	return q, r
}

func predictInterLumaPrep8(dst []int16, dstStride, width, height, sampleX, sampleY int, ref []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) {
	baseX16 := sampleX*16 + mv.X*2
	baseY16 := sampleY*16 + mv.Y*2
	startX, fx := floorDivMod16Subpel(baseX16)
	startY, fy := floorDivMod16Subpel(baseY16)
	if filter[0] == interFilterBilinear && filter[1] == interFilterBilinear {
		predictInterPrep8Bilinear(dst, dstStride, width, height, startX, startY, fx, fy, ref, refStride, refWidth, refHeight)
		return
	}
	predictInterPrep8Tap(dst, dstStride, width, height, startX, startY, fx, fy, ref, refStride, refWidth, refHeight, filter)
}

func predictInterChromaPrep8(dst []int16, dstStride, width, height, sampleX, sampleY int, ref []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) {
	baseX16 := sampleX*16 + mv.X
	baseY16 := sampleY*16 + mv.Y
	startX, fx := floorDivMod16Subpel(baseX16)
	startY, fy := floorDivMod16Subpel(baseY16)
	if filter[0] == interFilterBilinear && filter[1] == interFilterBilinear {
		predictInterPrep8Bilinear(dst, dstStride, width, height, startX, startY, fx, fy, ref, refStride, refWidth, refHeight)
		return
	}
	predictInterPrep8Tap(dst, dstStride, width, height, startX, startY, fx, fy, ref, refStride, refWidth, refHeight, filter)
}

func predictInterPrep8Bilinear(dst []int16, dstStride, width, height, startX, startY, fx, fy int, ref []byte, refStride, refWidth, refHeight int) {
	for py := 0; py < height; py++ {
		row := dst[py*dstStride:]
		sy0 := clampInt(startY+py, 0, refHeight-1)
		sy1 := clampInt(startY+py+1, 0, refHeight-1)
		for px := 0; px < width; px++ {
			sx0 := clampInt(startX+px, 0, refWidth-1)
			if fx == 0 && fy == 0 {
				row[px] = int16(int(ref[sy0*refStride+sx0]) << compoundIntermediateBits8)
				continue
			}
			sx1 := clampInt(startX+px+1, 0, refWidth-1)
			p00 := int(ref[sy0*refStride+sx0])
			p10 := int(ref[sy0*refStride+sx1])
			if fy == 0 {
				row[px] = int16((16-fx)*p00 + fx*p10)
				continue
			}
			p01 := int(ref[sy1*refStride+sx0])
			if fx == 0 {
				row[px] = int16((16-fy)*p00 + fy*p01)
				continue
			}
			p11 := int(ref[sy1*refStride+sx1])
			top := (16-fx)*p00 + fx*p10
			bottom := (16-fx)*p01 + fx*p11
			row[px] = int16((top*(16-fy) + bottom*fy + 8) >> 4)
		}
	}
}

func predictInterPrep8Tap(dst []int16, dstStride, width, height, startX, startY, fx, fy int, ref []byte, refStride, refWidth, refHeight int, filter InterFilterPair) {
	useSmallH := width <= 4
	useSmallV := height <= 4
	hf := inter8TapCoeffs(filter[1], fx, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(filter[0], fy, ternaryInt(useSmallV, 4, 8))
	switch {
	case hf != nil && vf != nil:
		for py := 0; py < height; py++ {
			row := dst[py*dstStride:]
			for px := 0; px < width; px++ {
				var mid [8]int
				for ky := 0; ky < 8; ky++ {
					sy := clampInt(startY+py+ky-3, 0, refHeight-1)
					sum := 0
					for kx := 0; kx < 8; kx++ {
						sx := clampInt(startX+px+kx-3, 0, refWidth-1)
						sum += hf[kx] * int(ref[sy*refStride+sx])
					}
					mid[ky] = (sum + 2) >> 2
				}
				sum := 0
				for ky := 0; ky < 8; ky++ {
					sum += vf[ky] * mid[ky]
				}
				row[px] = int16((sum + 32) >> 6)
			}
		}
	case hf != nil:
		for py := 0; py < height; py++ {
			row := dst[py*dstStride:]
			sy := clampInt(startY+py, 0, refHeight-1)
			for px := 0; px < width; px++ {
				sum := 0
				for kx := 0; kx < 8; kx++ {
					sx := clampInt(startX+px+kx-3, 0, refWidth-1)
					sum += hf[kx] * int(ref[sy*refStride+sx])
				}
				row[px] = int16((sum + 2) >> 2)
			}
		}
	case vf != nil:
		for py := 0; py < height; py++ {
			row := dst[py*dstStride:]
			for px := 0; px < width; px++ {
				sx := clampInt(startX+px, 0, refWidth-1)
				sum := 0
				for ky := 0; ky < 8; ky++ {
					sy := clampInt(startY+py+ky-3, 0, refHeight-1)
					sum += vf[ky] * int(ref[sy*refStride+sx])
				}
				row[px] = int16((sum + 2) >> 2)
			}
		}
	default:
		for py := 0; py < height; py++ {
			row := dst[py*dstStride:]
			sy := clampInt(startY+py, 0, refHeight-1)
			for px := 0; px < width; px++ {
				sx := clampInt(startX+px, 0, refWidth-1)
				row[px] = int16(int(ref[sy*refStride+sx]) << compoundIntermediateBits8)
			}
		}
	}
}

func blendWeightedCompound8(dst []byte, dstStride, x, y, width, height int, tmp0, tmp1 []int16, tmpStride, weight int) {
	for py := 0; py < height; py++ {
		row := dst[(y+py)*dstStride+x:]
		r0 := tmp0[py*tmpStride:]
		r1 := tmp1[py*tmpStride:]
		for px := 0; px < width; px++ {
			row[px] = clipByte((int(r0[px])*weight + int(r1[px])*(16-weight) + 128) >> 8)
		}
	}
}

func blendMaskedCompound8(dst []byte, dstStride, x, y, width, height int, tmp0, tmp1 []int16, tmpStride int, mask []uint8, maskStride int) {
	for py := 0; py < height; py++ {
		row := dst[(y+py)*dstStride+x:]
		r0 := tmp0[py*tmpStride:]
		r1 := tmp1[py*tmpStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < width; px++ {
			w0 := int(mr[px])
			row[px] = clipByte((int(r0[px])*w0 + int(r1[px])*(64-w0) + 512) >> 10)
		}
	}
}

func blendDiffWeightedCompound8(dst []byte, dstStride, x, y, width, height int, tmp0, tmp1 []int16, tmpStride int, sign bool, mask []uint8, maskStride int) {
	for py := 0; py < height; py++ {
		row := dst[(y+py)*dstStride+x:]
		r0 := tmp0[py*tmpStride:]
		r1 := tmp1[py*tmpStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < width; px++ {
			diff := int(r0[px]) - int(r1[px])
			m := 38 + ((absInt(diff) + 8) >> 8)
			if m > 64 {
				m = 64
			}
			w0 := m
			if sign {
				w0 = 64 - w0
			}
			mr[px] = uint8(w0)
			row[px] = clipByte((int(r0[px])*w0 + int(r1[px])*(64-w0) + 512) >> 10)
		}
	}
}
