package decoder

import (
	"fmt"
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

type wedgeDirection uint8

const (
	wedgeHorizontal wedgeDirection = iota
	wedgeVertical
	wedgeOblique27
	wedgeOblique63
	wedgeOblique117
	wedgeOblique153
	numWedgeDirections
)

type wedgeCode struct {
	direction wedgeDirection
	xOffset   int
	yOffset   int
}

var wedgeCodebookHgtw = [16]wedgeCode{
	{wedgeOblique27, 4, 4}, {wedgeOblique63, 4, 4},
	{wedgeOblique117, 4, 4}, {wedgeOblique153, 4, 4},
	{wedgeHorizontal, 4, 2}, {wedgeHorizontal, 4, 4},
	{wedgeHorizontal, 4, 6}, {wedgeVertical, 4, 4},
	{wedgeOblique27, 4, 2}, {wedgeOblique27, 4, 6},
	{wedgeOblique153, 4, 2}, {wedgeOblique153, 4, 6},
	{wedgeOblique63, 2, 4}, {wedgeOblique63, 6, 4},
	{wedgeOblique117, 2, 4}, {wedgeOblique117, 6, 4},
}

var wedgeCodebookHltw = [16]wedgeCode{
	{wedgeOblique27, 4, 4}, {wedgeOblique63, 4, 4},
	{wedgeOblique117, 4, 4}, {wedgeOblique153, 4, 4},
	{wedgeVertical, 2, 4}, {wedgeVertical, 4, 4},
	{wedgeVertical, 6, 4}, {wedgeHorizontal, 4, 4},
	{wedgeOblique27, 4, 2}, {wedgeOblique27, 4, 6},
	{wedgeOblique153, 4, 2}, {wedgeOblique153, 4, 6},
	{wedgeOblique63, 2, 4}, {wedgeOblique63, 6, 4},
	{wedgeOblique117, 2, 4}, {wedgeOblique117, 6, 4},
}

var wedgeCodebookHeqw = [16]wedgeCode{
	{wedgeOblique27, 4, 4}, {wedgeOblique63, 4, 4},
	{wedgeOblique117, 4, 4}, {wedgeOblique153, 4, 4},
	{wedgeHorizontal, 4, 2}, {wedgeHorizontal, 4, 6},
	{wedgeVertical, 2, 4}, {wedgeVertical, 6, 4},
	{wedgeOblique27, 4, 2}, {wedgeOblique27, 4, 6},
	{wedgeOblique153, 4, 2}, {wedgeOblique153, 4, 6},
	{wedgeOblique63, 2, 4}, {wedgeOblique63, 6, 4},
	{wedgeOblique117, 2, 4}, {wedgeOblique117, 6, 4},
}

var wedgeMasterOdd = [8]uint8{1, 2, 6, 18, 37, 53, 60, 63}
var wedgeMasterEven = [8]uint8{1, 4, 11, 27, 46, 58, 62, 63}
var wedgeMasterVert = [8]uint8{0, 2, 7, 21, 43, 57, 62, 64}
var interIntraWeights1D = [32]uint8{
	60, 52, 45, 39, 34, 30, 26, 22,
	19, 17, 15, 13, 11, 10, 8, 7,
	6, 6, 5, 4, 4, 3, 3, 2,
	2, 2, 2, 1, 1, 1, 1, 1,
}

const (
	wedgeSigns32x32 uint16 = 0x7bfb
	wedgeSigns32x16 uint16 = 0x7beb
	wedgeSigns32x8  uint16 = 0x6beb
	wedgeSigns16x32 uint16 = 0x7beb
	wedgeSigns16x16 uint16 = 0x7bfb
	wedgeSigns16x8  uint16 = 0x7beb
	wedgeSigns8x32  uint16 = 0x7aeb
	wedgeSigns8x16  uint16 = 0x7beb
	wedgeSigns8x8   uint16 = 0x7bfb
)

var compoundWedgeMasters = buildCompoundWedgeMasters()

var (
	interIntraByteScratchPool   sync.Pool
	interIntraUint16ScratchPool sync.Pool
)

const (
	maxInterIntraPredictorEdge = 32
	maxInterIntraPredictorArea = maxInterIntraPredictorEdge * maxInterIntraPredictorEdge
	maxCompoundMaskArea        = maxInterIntraPredictorArea
)

func takeOrMakeByteBuffer(dst []byte, n int) []byte {
	if n <= 0 {
		return dst[:0]
	}
	if cap(dst) >= n {
		return dst[:n]
	}
	return make([]byte, n)
}

func takeOrMakeUint16Buffer(dst []uint16, n int) []uint16 {
	if n <= 0 {
		return dst[:0]
	}
	if cap(dst) >= n {
		return dst[:n]
	}
	return make([]uint16, n)
}

func clipInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func PredictMaskedCompoundInterLumaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, mask []uint8, maskStride int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if len(mask) < visibleHeight*maskStride {
		return fmt.Errorf("decoder: masked compound mask too small")
	}
	tmp0 := make([]int16, visibleWidth*visibleHeight)
	tmp1 := make([]int16, visibleWidth*visibleHeight)
	predictInterLumaPrep8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0)
	predictInterLumaPrep8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1)
	blendMaskedCompound8(dst, dstStride, x, y, visibleWidth, visibleHeight, tmp0, tmp1, visibleWidth, mask, maskStride)
	return nil
}

func PredictMaskedCompoundInterChromaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, mask []uint8, maskStride int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if len(mask) < visibleHeight*maskStride {
		return fmt.Errorf("decoder: masked compound mask too small")
	}
	tmp0 := make([]int16, visibleWidth*visibleHeight)
	tmp1 := make([]int16, visibleWidth*visibleHeight)
	predictInterChromaPrep8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0)
	predictInterChromaPrep8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1)
	blendMaskedCompound8(dst, dstStride, x, y, visibleWidth, visibleHeight, tmp0, tmp1, visibleWidth, mask, maskStride)
	return nil
}

func PredictMaskedCompoundInterLumaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, bitDepth uint8, mask []uint8, maskStride int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if len(mask) < visibleHeight*maskStride {
		return fmt.Errorf("decoder: masked compound mask too small")
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictInterLumaBlock16WithFiltersAt(tmp0, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, bitDepth, filter0); err != nil {
		return err
	}
	if err := predictInterLumaBlock16WithFiltersAt(tmp1, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, bitDepth, filter1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < visibleWidth; px++ {
			w0 := int(mr[px])
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*w0+int(tmp1[py*visibleWidth+px])*(64-w0)+32)>>6, bitDepth)
		}
	}
	return nil
}

func PredictMaskedCompoundInterChromaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, bitDepth uint8, mask []uint8, maskStride int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if len(mask) < visibleHeight*maskStride {
		return fmt.Errorf("decoder: masked compound mask too small")
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictInterChromaBlock16WithFiltersAt(tmp0, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, bitDepth, filter0); err != nil {
		return err
	}
	if err := predictInterChromaBlock16WithFiltersAt(tmp1, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, bitDepth, filter1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < visibleWidth; px++ {
			w0 := int(mr[px])
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*w0+int(tmp1[py*visibleWidth+px])*(64-w0)+32)>>6, bitDepth)
		}
	}
	return nil
}

func diffWeightedMaskValue(diff int, bitDepth uint8) uint8 {
	maskRndShift := int(bitDepth) - 5
	maskRnd := 0
	if maskRndShift >= 0 {
		maskRnd = 1 << maskRndShift
	}
	m := 38 + (((diff << 4) + maskRnd) >> bitDepth)
	if m > 64 {
		m = 64
	}
	return uint8(m)
}

func PredictDiffWeightedCompoundInterLumaBlock8WithFilters(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, sign bool) ([]uint8, error) {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	mask := make([]uint8, width*height)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return mask, nil
	}
	area := visibleWidth * visibleHeight
	var (
		tmp0 []int16
		tmp1 []int16
	)
	if area <= maxCompoundMaskArea {
		var tmpLocal [2 * maxCompoundMaskArea]int16
		tmp0 = tmpLocal[:area]
		tmp1 = tmpLocal[maxCompoundMaskArea : maxCompoundMaskArea+area]
		predictInterLumaPrep8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0)
		predictInterLumaPrep8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1)
		blendDiffWeightedCompound8(dst, dstStride, x, y, visibleWidth, visibleHeight, tmp0, tmp1, visibleWidth, sign, mask, width)
		return mask, nil
	}
	tmp0 = make([]int16, area)
	tmp1 = make([]int16, area)
	predictInterLumaPrep8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0)
	predictInterLumaPrep8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1)
	blendDiffWeightedCompound8(dst, dstStride, x, y, visibleWidth, visibleHeight, tmp0, tmp1, visibleWidth, sign, mask, width)
	return mask, nil
}

func PredictDiffWeightedCompoundInterLumaBlock16WithFilters(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, bitDepth uint8, sign bool) ([]uint8, error) {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	mask := make([]uint8, width*height)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return mask, nil
	}
	area := visibleWidth * visibleHeight
	var (
		tmp0 []uint16
		tmp1 []uint16
	)
	if area <= maxCompoundMaskArea {
		var tmpLocal [2 * maxCompoundMaskArea]uint16
		tmp0 = tmpLocal[:area]
		tmp1 = tmpLocal[maxCompoundMaskArea : maxCompoundMaskArea+area]
		if err := predictInterLumaBlock16WithFiltersAt(tmp0, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, bitDepth, filter0); err != nil {
			return nil, err
		}
		if err := predictInterLumaBlock16WithFiltersAt(tmp1, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, bitDepth, filter1); err != nil {
			return nil, err
		}
		for py := 0; py < visibleHeight; py++ {
			row := dst[(y+py)*dstStride:]
			mr := mask[py*width:]
			for px := 0; px < visibleWidth; px++ {
				w0 := diffWeightedMaskValue(absInt(int(tmp0[py*visibleWidth+px])-int(tmp1[py*visibleWidth+px])), bitDepth)
				if sign {
					w0 = 64 - w0
				}
				mr[px] = w0
				row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*int(w0)+int(tmp1[py*visibleWidth+px])*(64-int(w0))+32)>>6, bitDepth)
			}
		}
		return mask, nil
	}
	tmp0 = make([]uint16, area)
	tmp1 = make([]uint16, area)
	if err := predictInterLumaBlock16WithFiltersAt(tmp0, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, bitDepth, filter0); err != nil {
		return nil, err
	}
	if err := predictInterLumaBlock16WithFiltersAt(tmp1, visibleWidth, visibleWidth, visibleHeight, 0, 0, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, bitDepth, filter1); err != nil {
		return nil, err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*width:]
		for px := 0; px < visibleWidth; px++ {
			w0 := diffWeightedMaskValue(absInt(int(tmp0[py*visibleWidth+px])-int(tmp1[py*visibleWidth+px])), bitDepth)
			if sign {
				w0 = 64 - w0
			}
			mr[px] = w0
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*int(w0)+int(tmp1[py*visibleWidth+px])*(64-int(w0))+32)>>6, bitDepth)
		}
	}
	return mask, nil
}

func buildCompoundMaskInto(dst []uint8, width, height int, compType CompoundType, wedgeIdx uint8, maskSign bool) []uint8 {
	switch compType {
	case compoundTypeSeg:
		return nil
	case compoundTypeWedge:
		idx := int(wedgeIdx)
		effectiveSign := defaultWedgeSign(width, height, idx) != maskSign
		return buildWedgeMaskInto(dst, width, height, idx, effectiveSign)
	default:
		dst = takeOrMakeByteBuffer(dst, width*height)
		fillUint8(dst, 32)
		return dst
	}
}

func buildCompoundMask(width, height int, compType CompoundType, wedgeIdx uint8, maskSign bool) []uint8 {
	return buildCompoundMaskInto(nil, width, height, compType, wedgeIdx, maskSign)
}

func buildSegMask(width, height int, sign bool) []uint8 {
	mask := make([]uint8, width*height)
	if width >= height {
		half := width / 2
		for y := 0; y < height; y++ {
			row := mask[y*width:]
			for x := 0; x < width; x++ {
				v := uint8(0)
				if x < half {
					v = 64
				}
				if sign {
					v = 64 - v
				}
				row[x] = v
			}
		}
		return mask
	}
	half := height / 2
	for y := 0; y < height; y++ {
		row := mask[y*width:]
		v := uint8(0)
		if y < half {
			v = 64
		}
		if sign {
			v = 64 - v
		}
		for x := 0; x < width; x++ {
			row[x] = v
		}
	}
	return mask
}

func insertWedgeBorder(dst []uint8, src [8]uint8, ctr int) {
	if ctr > 4 {
		fillUint8(dst[:ctr-4], 0)
	}
	copyStart := maxIntLocal(ctr, 4) - 4
	srcStart := maxIntLocal(4-ctr, 0)
	n := minInt(64-ctr, len(src)-srcStart)
	if n > 0 {
		copy(dst[copyStart:copyStart+n], src[srcStart:srcStart+n])
	}
	if ctr < 60 {
		fillUint8(dst[ctr+4:], 64)
	}
}

func transposeWedgeMaster(dst, src []uint8) {
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dst[x*64+y] = src[y*64+x]
		}
	}
}

func hflipWedgeMaster(dst, src []uint8) {
	for y := 0; y < 64; y++ {
		row := y * 64
		for x := 0; x < 64; x++ {
			dst[row+63-x] = src[row+x]
		}
	}
}

func buildCompoundWedgeMasters() [numWedgeDirections][64 * 64]uint8 {
	var master [numWedgeDirections][64 * 64]uint8
	for y := 0; y < 64; y++ {
		insertWedgeBorder(master[wedgeVertical][y*64:(y+1)*64], wedgeMasterVert, 32)
	}
	ctr := 48
	for y := 0; y < 64; y += 2 {
		off := y * 64
		insertWedgeBorder(master[wedgeOblique63][off:off+64], wedgeMasterEven, ctr)
		insertWedgeBorder(master[wedgeOblique63][off+64:off+128], wedgeMasterOdd, ctr-1)
		ctr--
	}
	transposeWedgeMaster(master[wedgeOblique27][:], master[wedgeOblique63][:])
	transposeWedgeMaster(master[wedgeHorizontal][:], master[wedgeVertical][:])
	hflipWedgeMaster(master[wedgeOblique117][:], master[wedgeOblique63][:])
	hflipWedgeMaster(master[wedgeOblique153][:], master[wedgeOblique27][:])
	return master
}

func selectWedgeCodebook(width, height int) [16]wedgeCode {
	switch {
	case height > width:
		return wedgeCodebookHgtw
	case height < width:
		return wedgeCodebookHltw
	default:
		return wedgeCodebookHeqw
	}
}

func buildWedgeMaskInto(dst []uint8, width, height, idx int, sign bool) []uint8 {
	codebook := selectWedgeCodebook(width, height)
	if idx < 0 || idx >= len(codebook) {
		idx = 0
	}
	code := codebook[idx]
	dst = takeOrMakeByteBuffer(dst, width*height)
	xOff := clipInt(32-((width*code.xOffset)>>3), 0, 64-width)
	yOff := clipInt(32-((height*code.yOffset)>>3), 0, 64-height)
	src := compoundWedgeMasters[code.direction][:]
	for y := 0; y < height; y++ {
		dstRow := dst[y*width : (y+1)*width]
		srcRow := src[(yOff+y)*64+xOff:]
		if sign {
			for x := 0; x < width; x++ {
				dstRow[x] = 64 - srcRow[x]
			}
		} else {
			copy(dstRow, srcRow[:width])
		}
	}
	return dst
}

func buildWedgeMask(width, height, idx int, sign bool) []uint8 {
	return buildWedgeMaskInto(nil, width, height, idx, sign)
}

func resizeCompoundMaskInto(dst, mask []uint8, inW, inH, outW, outH int) []uint8 {
	if outW == inW && outH == inH {
		dst = takeOrMakeByteBuffer(dst, len(mask))
		copy(dst, mask)
		return dst
	}
	dst = takeOrMakeByteBuffer(dst, outW*outH)
	for oy := 0; oy < outH; oy++ {
		y0 := oy * inH / outH
		y1 := maxIntLocal(y0+1, (oy+1)*inH/outH)
		for ox := 0; ox < outW; ox++ {
			x0 := ox * inW / outW
			x1 := maxIntLocal(x0+1, (ox+1)*inW/outW)
			sum := 0
			count := 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					sum += int(mask[y*inW+x])
					count++
				}
			}
			dst[oy*outW+ox] = uint8((sum + count/2) / maxIntLocal(count, 1))
		}
	}
	return dst
}

func resizeCompoundMask(mask []uint8, inW, inH, outW, outH int) []uint8 {
	return resizeCompoundMaskInto(nil, mask, inW, inH, outW, outH)
}

func subsampleCompoundMaskInto(dst, mask []uint8, inW, inH int, layout av1.ChromaSubsampling, sign bool) []uint8 {
	switch layout {
	case av1.Chroma444:
		dst = takeOrMakeByteBuffer(dst, len(mask))
		copy(dst, mask)
		return dst
	case av1.Chroma422:
		outW := (inW + 1) >> 1
		dst = takeOrMakeByteBuffer(dst, outW*inH)
		bias := 1
		if sign {
			bias = 0
		}
		for y := 0; y < inH; y++ {
			inRow := mask[y*inW:]
			outRow := dst[y*outW:]
			for x := 0; x < inW; x += 2 {
				x1 := minInt(x+1, inW-1)
				outRow[x>>1] = uint8((int(inRow[x]) + int(inRow[x1]) + bias) >> 1)
			}
		}
		return dst
	case av1.Chroma420:
		outW := (inW + 1) >> 1
		outH := (inH + 1) >> 1
		dst = takeOrMakeByteBuffer(dst, outW*outH)
		bias := 2
		if sign {
			bias = 1
		}
		for y := 0; y < inH; y += 2 {
			y1 := minInt(y+1, inH-1)
			row0 := mask[y*inW:]
			row1 := mask[y1*inW:]
			outRow := dst[(y>>1)*outW:]
			for x := 0; x < inW; x += 2 {
				x1 := minInt(x+1, inW-1)
				sum := int(row0[x]) + int(row0[x1]) + int(row1[x]) + int(row1[x1])
				outRow[x>>1] = uint8((sum + bias) >> 2)
			}
		}
		return dst
	default:
		return resizeCompoundMaskInto(dst, mask, inW, inH, (inW+1)>>1, (inH+1)>>1)
	}
}

func subsampleCompoundMask(mask []uint8, inW, inH int, layout av1.ChromaSubsampling, sign bool) []uint8 {
	return subsampleCompoundMaskInto(nil, mask, inW, inH, layout, sign)
}

func buildInterIntraMaskInto(dst []uint8, width, height int, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8) []uint8 {
	if width <= 0 || height <= 0 {
		return nil
	}
	if typ == interIntraWedge {
		idx := int(wedgeIdx)
		return buildWedgeMaskInto(dst, width, height, idx, defaultWedgeSign(width, height, idx))
	}
	dst = takeOrMakeByteBuffer(dst, width*height)
	if mode == IntraPredDC {
		fillUint8(dst, 32)
		return dst
	}
	step := maxIntLocal(1, 32/maxIntLocal(width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var idx int
			switch mode {
			case IntraPredVertical:
				idx = y * step
			case IntraPredHorizontal:
				idx = x * step
			case IntraPredSmooth:
				idx = minInt(x, y) * step
			default:
				dst[y*width+x] = 32
				continue
			}
			dst[y*width+x] = interIntraWeights1D[minInt(idx, len(interIntraWeights1D)-1)]
		}
	}
	return dst
}

func buildInterIntraMask(width, height int, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8) []uint8 {
	return buildInterIntraMaskInto(nil, width, height, typ, mode, wedgeIdx)
}

func buildInterIntraChromaMaskInto(dst, lumaScratch []uint8, layout av1.ChromaSubsampling, size BlockSize, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8) []uint8 {
	chromaW, chromaH := blockChromaSampleDimensions(layout, size)
	if typ != interIntraWedge {
		return buildInterIntraMaskInto(dst, chromaW, chromaH, typ, mode, wedgeIdx)
	}
	lumaW, lumaH := blockSampleDimensions(size)
	lumaMask := buildInterIntraMaskInto(lumaScratch, lumaW, lumaH, typ, mode, wedgeIdx)
	return subsampleCompoundMaskInto(dst, lumaMask, lumaW, lumaH, layout, false)
}

func blockSampleDimensions(size BlockSize) (int, int) {
	dims := BlockDims(size)
	return int(dims.W4) * 4, int(dims.H4) * 4
}

func blockChromaSampleDimensions(layout av1.ChromaSubsampling, size BlockSize) (int, int) {
	cw4, ch4 := chromaBlockDims(layout, BlockDims(size))
	return cw4 * 4, ch4 * 4
}

func defaultWedgeSign(width, height, idx int) bool {
	if idx < 0 || idx >= 16 {
		return false
	}
	var signs uint16
	switch {
	case width == 32 && height == 32:
		signs = wedgeSigns32x32
	case width == 32 && height == 16:
		signs = wedgeSigns32x16
	case width == 32 && height == 8:
		signs = wedgeSigns32x8
	case width == 16 && height == 32:
		signs = wedgeSigns16x32
	case width == 16 && height == 16:
		signs = wedgeSigns16x16
	case width == 16 && height == 8:
		signs = wedgeSigns16x8
	case width == 8 && height == 32:
		signs = wedgeSigns8x32
	case width == 8 && height == 16:
		signs = wedgeSigns8x16
	case width == 8 && height == 8:
		signs = wedgeSigns8x8
	default:
		return false
	}
	return ((signs >> idx) & 1) != 0
}

func buildInterIntraPredictor8Into(dstBuf, topBuf, leftBuf []byte, plane []byte, stride, planeWidth, planeHeight, x, y, width, height int, mode IntraPredMode) ([]byte, int, int, error) {
	if width <= 0 || height <= 0 || x >= planeWidth || y >= planeHeight {
		return nil, 0, 0, nil
	}
	haveTop := y > 0
	haveLeft := x > 0
	var top []byte
	var left []byte
	topLeft := uint8(128)
	if haveTop {
		top = takeOrMakeByteBuffer(topBuf, width)
		available := minInt(width, planeWidth-x)
		copy(top, plane[(y-1)*stride+x:(y-1)*stride+x+available])
		fillRunUint8(top, available, width-available, top[available-1])
	}
	if haveLeft {
		left = takeOrMakeByteBuffer(leftBuf, height)
		available := minInt(height, planeHeight-y)
		for i := 0; i < available; i++ {
			left[i] = plane[(y+i)*stride+x-1]
		}
		fillRunUint8(left, available, height-available, left[available-1])
	}
	if haveTop && haveLeft {
		topLeft = plane[(y-1)*stride+x-1]
	} else if haveTop {
		topLeft = top[0]
	} else if haveLeft {
		topLeft = left[0]
	}
	dst := takeOrMakeByteBuffer(dstBuf, width*height)
	if err := PredictIntraLuma8(dst, width, width, height, mode, 0, top, left, topLeft, haveTop, haveLeft); err != nil {
		return nil, 0, 0, err
	}
	return dst, width, height, nil
}

func buildInterIntraPredictor8(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int, mode IntraPredMode) ([]byte, int, int, error) {
	return buildInterIntraPredictor8Into(nil, nil, nil, plane, stride, planeWidth, planeHeight, x, y, width, height, mode)
}

func buildInterIntraPredictor16Into(dstBuf, topBuf, leftBuf []uint16, plane []uint16, stride, planeWidth, planeHeight, x, y, width, height int, mode IntraPredMode, bitDepth uint8) ([]uint16, int, int, error) {
	if width <= 0 || height <= 0 || x >= planeWidth || y >= planeHeight {
		return nil, 0, 0, nil
	}
	haveTop := y > 0
	haveLeft := x > 0
	var top []uint16
	var left []uint16
	topLeft := uint16(1 << (bitDepth - 1))
	if haveTop {
		top = takeOrMakeUint16Buffer(topBuf, width)
		available := minInt(width, planeWidth-x)
		copy(top, plane[(y-1)*stride+x:(y-1)*stride+x+available])
		fillRunUint16(top, available, width-available, top[available-1])
	}
	if haveLeft {
		left = takeOrMakeUint16Buffer(leftBuf, height)
		available := minInt(height, planeHeight-y)
		for i := 0; i < available; i++ {
			left[i] = plane[(y+i)*stride+x-1]
		}
		fillRunUint16(left, available, height-available, left[available-1])
	}
	if haveTop && haveLeft {
		topLeft = plane[(y-1)*stride+x-1]
	} else if haveTop {
		topLeft = top[0]
	} else if haveLeft {
		topLeft = left[0]
	}
	dst := takeOrMakeUint16Buffer(dstBuf, width*height)
	if err := PredictIntraLuma16(dst, width, width, height, mode, 0, top, left, topLeft, haveTop, haveLeft, bitDepth); err != nil {
		return nil, 0, 0, err
	}
	return dst, width, height, nil
}

func buildInterIntraPredictor16(plane []uint16, stride, planeWidth, planeHeight, x, y, width, height int, mode IntraPredMode, bitDepth uint8) ([]uint16, int, int, error) {
	return buildInterIntraPredictor16Into(nil, nil, nil, plane, stride, planeWidth, planeHeight, x, y, width, height, mode, bitDepth)
}

func blendInterIntraBlock8(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	area := width * height
	scratch := takeScratch[byte](&interIntraByteScratchPool, 2*area+width+height)
	defer putScratch(&interIntraByteScratchPool, scratch)
	buf := scratch.buf
	predBuf := buf[:area]
	buf = buf[area:]
	maskBuf := buf[:area]
	buf = buf[area:]
	topBuf := buf[:width]
	leftBuf := buf[width : width+height]
	intraPred, iw, ih, err := buildInterIntraPredictor8Into(predBuf[:0], topBuf[:0], leftBuf[:0], dst, dstStride, planeWidth, planeHeight, x, y, width, height, mode)
	if err != nil || intraPred == nil {
		return err
	}
	mask := buildInterIntraMaskInto(maskBuf[:0], iw, ih, typ, mode, wedgeIdx)
	return blendInterIntraLumaBlock8WithStrides(dst, dstStride, planeWidth, planeHeight, x, y, width, height, intraPred, iw, mask, iw)
}

func blendInterIntraChromaBlock8(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, layout av1.ChromaSubsampling, size BlockSize, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	area := width * height
	lumaW, lumaH := blockSampleDimensions(size)
	lumaArea := lumaW * lumaH
	scratch := takeScratch[byte](&interIntraByteScratchPool, 2*area+width+height+lumaArea)
	defer putScratch(&interIntraByteScratchPool, scratch)
	buf := scratch.buf
	predBuf := buf[:area]
	buf = buf[area:]
	maskBuf := buf[:area]
	buf = buf[area:]
	topBuf := buf[:width]
	buf = buf[width:]
	leftBuf := buf[:height]
	buf = buf[height:]
	lumaMaskBuf := buf[:lumaArea]
	intraPred, iw, _, err := buildInterIntraPredictor8Into(predBuf[:0], topBuf[:0], leftBuf[:0], dst, dstStride, planeWidth, planeHeight, x, y, width, height, mode)
	if err != nil || intraPred == nil {
		return err
	}
	mask := buildInterIntraChromaMaskInto(maskBuf[:0], lumaMaskBuf[:0], layout, size, typ, mode, wedgeIdx)
	return blendInterIntraLumaBlock8WithStrides(dst, dstStride, planeWidth, planeHeight, x, y, width, height, intraPred, iw, mask, width)
}

func blendInterIntraBlock16(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8, bitDepth uint8) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	area := width * height
	scratch16 := takeScratch[uint16](&interIntraUint16ScratchPool, area+width+height)
	defer putScratch(&interIntraUint16ScratchPool, scratch16)
	buf16 := scratch16.buf
	predBuf := buf16[:area]
	buf16 = buf16[area:]
	topBuf := buf16[:width]
	leftBuf := buf16[width : width+height]
	maskScratch := takeScratch[byte](&interIntraByteScratchPool, area)
	defer putScratch(&interIntraByteScratchPool, maskScratch)
	intraPred, iw, ih, err := buildInterIntraPredictor16Into(predBuf[:0], topBuf[:0], leftBuf[:0], dst, dstStride, planeWidth, planeHeight, x, y, width, height, mode, bitDepth)
	if err != nil || intraPred == nil {
		return err
	}
	mask := buildInterIntraMaskInto(maskScratch.buf[:0], iw, ih, typ, mode, wedgeIdx)
	return blendInterIntraLumaBlock16WithStrides(dst, dstStride, planeWidth, planeHeight, x, y, width, height, intraPred, iw, mask, iw, bitDepth)
}

func blendInterIntraChromaBlock16(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, layout av1.ChromaSubsampling, size BlockSize, typ InterIntraType, mode IntraPredMode, wedgeIdx uint8, bitDepth uint8) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	area := width * height
	scratch16 := takeScratch[uint16](&interIntraUint16ScratchPool, area+width+height)
	defer putScratch(&interIntraUint16ScratchPool, scratch16)
	buf16 := scratch16.buf
	predBuf := buf16[:area]
	buf16 = buf16[area:]
	topBuf := buf16[:width]
	buf16 = buf16[width:]
	leftBuf := buf16[:height]
	lumaW, lumaH := blockSampleDimensions(size)
	maskScratch := takeScratch[byte](&interIntraByteScratchPool, area+lumaW*lumaH)
	defer putScratch(&interIntraByteScratchPool, maskScratch)
	maskBuf := maskScratch.buf[:area]
	lumaMaskBuf := maskScratch.buf[area : area+lumaW*lumaH]
	intraPred, iw, _, err := buildInterIntraPredictor16Into(predBuf[:0], topBuf[:0], leftBuf[:0], dst, dstStride, planeWidth, planeHeight, x, y, width, height, mode, bitDepth)
	if err != nil || intraPred == nil {
		return err
	}
	mask := buildInterIntraChromaMaskInto(maskBuf[:0], lumaMaskBuf[:0], layout, size, typ, mode, wedgeIdx)
	return blendInterIntraLumaBlock16WithStrides(dst, dstStride, planeWidth, planeHeight, x, y, width, height, intraPred, iw, mask, width, bitDepth)
}

func BlendInterIntraLumaBlock8(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, intra []byte, mask []uint8, maskStride int) error {
	visibleWidth := minInt(width, planeWidth-x)
	return blendInterIntraLumaBlock8WithStrides(dst, dstStride, planeWidth, planeHeight, x, y, width, height, intra, visibleWidth, mask, maskStride)
}

func blendInterIntraLumaBlock8WithStrides(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, intra []byte, intraStride int, mask []uint8, maskStride int) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if len(intra) < (visibleHeight-1)*intraStride+visibleWidth || len(mask) < (visibleHeight-1)*maskStride+visibleWidth {
		return fmt.Errorf("decoder: interintra buffers too small")
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		ir := intra[py*intraStride:]
		for px := 0; px < visibleWidth; px++ {
			w := int(mr[px])
			inter := int(row[x+px])
			inv := int(ir[px])
			row[x+px] = clipByte((inter*(64-w) + inv*w + 32) >> 6)
		}
	}
	return nil
}

func BlendInterIntraLumaBlock16(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, intra []uint16, mask []uint8, maskStride int, bitDepth uint8) error {
	visibleWidth := minInt(width, planeWidth-x)
	return blendInterIntraLumaBlock16WithStrides(dst, dstStride, planeWidth, planeHeight, x, y, width, height, intra, visibleWidth, mask, maskStride, bitDepth)
}

func blendInterIntraLumaBlock16WithStrides(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, intra []uint16, intraStride int, mask []uint8, maskStride int, bitDepth uint8) error {
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if len(intra) < (visibleHeight-1)*intraStride+visibleWidth || len(mask) < (visibleHeight-1)*maskStride+visibleWidth {
		return fmt.Errorf("decoder: interintra buffers too small")
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		ir := intra[py*intraStride:]
		for px := 0; px < visibleWidth; px++ {
			w := int(mr[px])
			inter := int(row[x+px])
			inv := int(ir[px])
			row[x+px] = clipSample((inter*(64-w)+inv*w+32)>>6, bitDepth)
		}
	}
	return nil
}
