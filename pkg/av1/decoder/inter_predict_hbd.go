package decoder

import "fmt"

func PredictInterLumaBlock16(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8) error {
	return PredictInterLumaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, bitDepth, InterFilterPair{interFilterRegular, interFilterRegular})
}

func PredictInterLumaBlock16WithFilter(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8, filterMode uint8) error {
	return PredictInterLumaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, bitDepth, InterFilterPair{filterMode, filterMode})
}

func PredictInterLumaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8, filter InterFilterPair) error {
	if width <= 0 || height <= 0 {
		return nil
	}
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
	return predictInterLumaBlock16WithFiltersAt(dst, dstStride, planeWidth, planeHeight, x, y, width, height, x, y, refPlane, refStride, refWidth, refHeight, mv, bitDepth, filter)
}

func predictInterLumaBlock16WithFiltersAt(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height, sampleX, sampleY int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8, filter InterFilterPair) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	baseX8 := sampleX*8 + mv.X
	baseY8 := sampleY*8 + mv.Y
	startX, fx := floorDivMod8(baseX8)
	startY, fy := floorDivMod8(baseY8)
	if filter[0] == interFilterBilinear && filter[1] == interFilterBilinear {
		return predictInterLumaBlock16Bilinear(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, fx, fy, bitDepth)
	}
	useSmallH := width <= 4
	useSmallV := height <= 4
	hf := inter8TapCoeffs(filter[1], fx<<1, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(filter[0], fy<<1, ternaryInt(useSmallV, 4, 8))
	switch {
	case hf == nil && vf == nil:
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := clampInt(startY+py, 0, refHeight-1)
			if startX >= 0 && startX+visibleWidth <= refWidth {
				copy(row[:visibleWidth], refPlane[sy*refStride+startX:sy*refStride+startX+visibleWidth])
				continue
			}
			for px := 0; px < visibleWidth; px++ {
				row[px] = refSample16(refPlane, refStride, refWidth, refHeight, startX+px, sy)
			}
		}
	case hf != nil && vf != nil:
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := startY + py
			for px := 0; px < visibleWidth; px++ {
				sx := startX + px
				var mid [8]int
				for tapY := 0; tapY < 8; tapY++ {
					sum := 0
					sampleY := sy + tapY - 3
					for tapX := 0; tapX < 8; tapX++ {
						sum += hf[tapX] * int(refSample16(refPlane, refStride, refWidth, refHeight, sx+tapX-3, sampleY))
					}
					mid[tapY] = (sum + 2) >> 2
				}
				sum := 0
				for tapY := 0; tapY < 8; tapY++ {
					sum += vf[tapY] * mid[tapY]
				}
				row[px] = clipSample((sum+512)>>10, bitDepth)
			}
		}
	case hf != nil:
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := startY + py
			for px := 0; px < visibleWidth; px++ {
				sx := startX + px
				sum := 0
				for tapX := 0; tapX < 8; tapX++ {
					sum += hf[tapX] * int(refSample16(refPlane, refStride, refWidth, refHeight, sx+tapX-3, sy))
				}
				row[px] = clipSample((sum+34)>>6, bitDepth)
			}
		}
	default:
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride+x:]
			sy := startY + py
			for px := 0; px < visibleWidth; px++ {
				sx := startX + px
				sum := 0
				for tapY := 0; tapY < 8; tapY++ {
					sum += vf[tapY] * int(refSample16(refPlane, refStride, refWidth, refHeight, sx, sy+tapY-3))
				}
				row[px] = clipSample((sum+32)>>6, bitDepth)
			}
		}
	}
	return nil
}

func predictInterLumaBlock16Bilinear(dst []uint16, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []uint16, refStride, refWidth, refHeight int, startX, startY, fx, fy int, bitDepth uint8) error {
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride+x:]
		sy := startY + py
		for px := 0; px < visibleWidth; px++ {
			sx := startX + px
			p00 := refSample16(refPlane, refStride, refWidth, refHeight, sx, sy)
			if fx == 0 && fy == 0 {
				row[px] = p00
				continue
			}
			p10 := refSample16(refPlane, refStride, refWidth, refHeight, sx+1, sy)
			p01 := refSample16(refPlane, refStride, refWidth, refHeight, sx, sy+1)
			p11 := refSample16(refPlane, refStride, refWidth, refHeight, sx+1, sy+1)
			top := (8-fx)*int(p00) + fx*int(p10)
			bottom := (8-fx)*int(p01) + fx*int(p11)
			row[px] = clipSample((top*(8-fy)+bottom*fy+32)>>6, bitDepth)
		}
	}
	return nil
}

func ReconstructInterLumaCoefficients16(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, bitDepth uint8, lossless bool) error {
	return ReconstructInterLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, bitDepth, lossless)
}

func ReconstructInterLumaCoefficients16WithTxType(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8, lossless bool) error {
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
	dst, dstStride, commit, err := prepareInterLumaTarget16(plane, stride, planeWidth, planeHeight, x, y, width, height)
	if err != nil {
		return err
	}
	if err := InverseAddResidualTx16(dst, dstStride, tx, txType, coeffs, bitDepth, lossless); err != nil {
		return fmt.Errorf("decoder: inverse inter transform add at (%d,%d) tx=%d failed (dst_stride=%d dst_len=%d): %w", x, y, tx, dstStride, len(dst), err)
	}
	commit()
	return nil
}

func refSample16(plane []uint16, stride, width, height, x, y int) uint16 {
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

func prepareInterLumaTarget16(plane []uint16, stride, planeWidth, planeHeight, x, y, width, height int) ([]uint16, int, func(), error) {
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight {
		return nil, 0, nil, fmt.Errorf("decoder: inter block (%d,%d %dx%d) out of plane bounds %dx%d", x, y, width, height, planeWidth, planeHeight)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth == width && visibleHeight == height {
		return plane[y*stride+x:], stride, func() {}, nil
	}
	dst := make([]uint16, width*height)
	for row := 0; row < visibleHeight; row++ {
		copy(dst[row*width:row*width+visibleWidth], plane[(y+row)*stride+x:(y+row)*stride+x+visibleWidth])
	}
	commit := func() {
		for row := 0; row < visibleHeight; row++ {
			copy(plane[(y+row)*stride+x:(y+row)*stride+x+visibleWidth], dst[row*width:row*width+visibleWidth])
		}
	}
	return dst, width, commit, nil
}
