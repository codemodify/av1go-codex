package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func floorDivMod16(v int) (int, int) {
	q := v / 16
	r := v % 16
	if r < 0 {
		q--
		r += 16
	}
	return q, r
}

func PredictInterChromaBlock8(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector) error {
	return PredictInterChromaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, InterFilterPair{interFilterRegular, interFilterRegular})
}

func PredictInterChromaBlock8WithFilter(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filterMode uint8) error {
	return PredictInterChromaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, InterFilterPair{filterMode, filterMode})
}

func PredictInterChromaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if err := validateInterPredictBlock8Args(dst, dstStride, planeWidth, planeHeight, refPlane, refStride, refWidth, refHeight); err != nil {
		if refWidth <= 0 || refHeight <= 0 {
			return fmt.Errorf("decoder: invalid chroma reference plane %dx%d", refWidth, refHeight)
		}
		return err
	}
	predictInterChromaBlock8WithFiltersUnchecked(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, filter)
	return nil
}

func predictInterChromaBlock8WithFiltersUnchecked(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) {
	predictInterChromaBlock8WithFiltersAt(dst, dstStride, planeWidth, planeHeight, x, y, width, height, x, y, refPlane, refStride, refWidth, refHeight, mv, filter)
}

func predictInterChromaBlock8WithFiltersAt(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height, sampleX, sampleY int, refPlane []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair) {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return
	}
	baseX16 := sampleX*16 + mv.X
	baseY16 := sampleY*16 + mv.Y
	startX, fx := floorDivMod16(baseX16)
	startY, fy := floorDivMod16(baseY16)
	if filter[0] == interFilterBilinear && filter[1] == interFilterBilinear {
		if startX >= 0 && startY >= 0 && startX+visibleWidth+1 <= refWidth && startY+visibleHeight+1 <= refHeight {
			_ = predictInterBlock8BilinearInterior(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, startX, startY, fx, fy, 16)
			return
		}
		_ = predictInterChromaBlock8Bilinear(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, fx, fy)
		return
	}
	useSmallH := width <= 4
	useSmallV := height <= 4
	hf := inter8TapCoeffs(filter[1], fx, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(filter[0], fy, ternaryInt(useSmallV, 4, 8))
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

func predictInterChromaBlock8Bilinear(dst []byte, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []byte, refStride, refWidth, refHeight int, startX, startY, fx, fy int) error {
	predictInterBlock8BilinearClamped(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, fx, fy, 16)
	return nil
}

func ReconstructInterChromaCoefficients8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, lossless bool) error {
	return ReconstructInterChromaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, lossless)
}

func ReconstructInterChromaCoefficients8WithTxType(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, lossless bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	if x < 0 || y < 0 {
		return fmt.Errorf("decoder: negative inter chroma reconstruction origin")
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
		return fmt.Errorf("decoder: inverse inter chroma transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	writeback.commit()
	return nil
}

func PredictInterChromaBlock16(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8) error {
	return PredictInterChromaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, bitDepth, InterFilterPair{interFilterRegular, interFilterRegular})
}

func PredictInterChromaBlock16WithFilter(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8, filterMode uint8) error {
	return PredictInterChromaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, mv, bitDepth, InterFilterPair{filterMode, filterMode})
}

func PredictInterChromaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8, filter InterFilterPair) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if refWidth <= 0 || refHeight <= 0 {
		return fmt.Errorf("decoder: invalid chroma reference plane %dx%d", refWidth, refHeight)
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
	return predictInterChromaBlock16WithFiltersAt(dst, dstStride, planeWidth, planeHeight, x, y, width, height, x, y, refPlane, refStride, refWidth, refHeight, mv, bitDepth, filter)
}

func predictInterChromaBlock16WithFiltersAt(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height, sampleX, sampleY int, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, bitDepth uint8, filter InterFilterPair) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	baseX16 := sampleX*16 + mv.X
	baseY16 := sampleY*16 + mv.Y
	startX, fx := floorDivMod16(baseX16)
	startY, fy := floorDivMod16(baseY16)
	if filter[0] == interFilterBilinear && filter[1] == interFilterBilinear {
		return predictInterChromaBlock16Bilinear(dst, dstStride, x, y, visibleWidth, visibleHeight, refPlane, refStride, refWidth, refHeight, startX, startY, fx, fy, bitDepth)
	}
	useSmallH := width <= 4
	useSmallV := height <= 4
	hf := inter8TapCoeffs(filter[1], fx, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(filter[0], fy, ternaryInt(useSmallV, 4, 8))
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

func predictInterChromaBlock16Bilinear(dst []uint16, dstStride, x, y, visibleWidth, visibleHeight int, refPlane []uint16, refStride, refWidth, refHeight int, startX, startY, fx, fy int, bitDepth uint8) error {
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
			top := (16-fx)*int(p00) + fx*int(p10)
			bottom := (16-fx)*int(p01) + fx*int(p11)
			row[px] = clipSample((top*(16-fy)+bottom*fy+128)>>8, bitDepth)
		}
	}
	return nil
}

func ReconstructInterChromaCoefficients16(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeffs []int32, bitDepth uint8, lossless bool) error {
	return ReconstructInterChromaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, coeffs, bitDepth, lossless)
}

func ReconstructInterChromaCoefficients16WithTxType(plane []uint16, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8, lossless bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	if x < 0 || y < 0 {
		return fmt.Errorf("decoder: negative inter chroma reconstruction origin")
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
		return fmt.Errorf("decoder: inverse inter chroma transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	commit()
	return nil
}

func interChromaTXForBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, bs BlockSize, segID uint8) TxfmSize {
	tx := MaxTXSizeForBlock(bs, seq.ColorConfig.ChromaSubsampling())
	if hdr != nil && int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
		tx = TX4X4
	}
	return tx
}

func buildChromaTransformUnits(layout av1.ChromaSubsampling, g BlockGeometry, tx TxfmSize) []BlockGeometry {
	return buildChromaTransformUnitsInto(nil, layout, g, tx)
}

func buildChromaTransformUnitsInto(dst []BlockGeometry, layout av1.ChromaSubsampling, g BlockGeometry, tx TxfmSize) []BlockGeometry {
	chroma := chromaGeometryForBlock(layout, g)
	info := TxfmInfoFor(tx)
	countX, countY := transformGridCounts(chroma, tx)
	need := countX * countY
	var out []BlockGeometry
	if cap(dst) < need {
		out = make([]BlockGeometry, 0, need)
	} else {
		out = dst[:0]
	}
	for offY4 := 0; offY4 < chroma.Height4(); offY4 += int(info.H4) {
		for offX4 := 0; offX4 < chroma.Width4(); offX4 += int(info.W4) {
			out = append(out, BlockGeometry{
				Level:   chroma.Level,
				Size:    chroma.Size,
				Start4X: chroma.Start4X + offX4,
				Start4Y: chroma.Start4Y + offY4,
				End4X:   min(chroma.Start4X+offX4+int(info.W4), chroma.End4X),
				End4Y:   min(chroma.Start4Y+offY4+int(info.H4), chroma.End4Y),
			})
		}
	}
	return out
}
