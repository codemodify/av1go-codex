package decoder

import "fmt"

func PredictDCLuma8(dst []byte, stride, width, height int, top, left []byte, haveTop, haveLeft bool) error {
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
		dc = 128
	}

	fillBlock8(dst, stride, width, height, uint8(dc))
	return nil
}

func InverseAddDCTDCTDC8(dst []byte, stride int, tx TxfmSize, coeff int32) error {
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
		return inverseAddDCTDCT8Dense(dst, stride, tx, coeffs)
	}
	shift, ok := dctDCTSquareShift[tx]
	if !ok {
		coeffs := make([]int32, width*height)
		coeffs[0] = coeff
		return inverseAddDCTDCT8Dense(dst, stride, tx, coeffs)
	}
	dc := int(coeff)
	dc = (dc*181 + 128) >> 8
	dc = (dc + ((1 << shift) >> 1)) >> shift
	dc = (dc*181 + 128 + 2048) >> 12

	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			row[x] = clipByte(int(row[x]) + dc)
		}
	}
	return nil
}

var dctDCTSquareShift = map[TxfmSize]int{
	TX4X4:   0,
	TX8X8:   1,
	TX16X16: 2,
	TX32X32: 2,
	TX64X64: 2,
}

func fillBlock8(dst []byte, stride, width, height int, v uint8) {
	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		for x := 0; x < width; x++ {
			row[x] = v
		}
	}
}

func clipByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func ctzPow2(v int) int {
	n := 0
	for v > 1 && (v&1) == 0 {
		n++
		v >>= 1
	}
	return n
}
