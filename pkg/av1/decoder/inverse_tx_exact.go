package decoder

import (
	"fmt"
	"sync"
)

var exactInverseShift = [numRectTxfmSizes]int8{
	TX4X4:    0,
	TX8X8:    1,
	TX16X16:  2,
	TX32X32:  2,
	TX64X64:  2,
	RTX4X8:   0,
	RTX8X4:   0,
	RTX8X16:  1,
	RTX16X8:  1,
	RTX16X32: 1,
	RTX32X16: 1,
	RTX32X64: 1,
	RTX64X32: 1,
	RTX4X16:  1,
	RTX16X4:  1,
	RTX8X32:  2,
	RTX32X8:  2,
	RTX16X64: 2,
	RTX64X16: 2,
}

var exactInverseSupported = [numRectTxfmSizes]bool{
	TX4X4:    true,
	TX8X8:    true,
	TX16X16:  true,
	TX32X32:  true,
	TX64X64:  true,
	RTX4X8:   true,
	RTX8X4:   true,
	RTX8X16:  true,
	RTX16X8:  true,
	RTX16X32: true,
	RTX32X16: true,
	RTX32X64: true,
	RTX64X32: true,
	RTX4X16:  true,
	RTX16X4:  true,
	RTX8X32:  true,
	RTX32X8:  true,
	RTX16X64: true,
	RTX64X16: true,
}

var exactInverseScratchPool sync.Pool

func inverseAddExact8(dst []byte, stride int, tx TxfmSize, txType TxType, coeffs []int32) (bool, error) {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	valuesScratch := takeScratch[int](&exactInverseScratchPool, width*height)
	defer putScratch(&exactInverseScratchPool, valuesScratch)
	values := valuesScratch.buf

	width, height, ok, err := inverseExactMatrix(values, tx, txType, coeffs, exactClipRanges8())
	if !ok || err != nil {
		return ok, err
	}
	if stride < width {
		return true, fmt.Errorf("decoder: inverse transform stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return true, fmt.Errorf("decoder: inverse transform buffer too small")
	}
	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		base := y * width
		for x := 0; x < width; x++ {
			row[x] = clipByte(int(row[x]) + ((values[base+x] + 8) >> 4))
		}
	}
	return true, nil
}

func inverseAddExact16(dst []uint16, stride int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8) (bool, error) {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	valuesScratch := takeScratch[int](&exactInverseScratchPool, width*height)
	defer putScratch(&exactInverseScratchPool, valuesScratch)
	values := valuesScratch.buf

	width, height, ok, err := inverseExactMatrix(values, tx, txType, coeffs, exactClipRangesHBD(bitDepth))
	if !ok || err != nil {
		return ok, err
	}
	if stride < width {
		return true, fmt.Errorf("decoder: inverse transform stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return true, fmt.Errorf("decoder: inverse transform buffer too small")
	}
	for y := 0; y < height; y++ {
		row := dst[y*stride:]
		base := y * width
		for x := 0; x < width; x++ {
			row[x] = clipSample(int(row[x])+((values[base+x]+8)>>4), bitDepth)
		}
	}
	return true, nil
}

type exactClipRanges struct {
	rowMin int
	rowMax int
	colMin int
	colMax int
}

func exactClipRanges8() exactClipRanges {
	return exactClipRanges{
		rowMin: -(1 << 15),
		rowMax: (1 << 15) - 1,
		colMin: -(1 << 15),
		colMax: (1 << 15) - 1,
	}
}

func exactClipRangesHBD(bitDepth uint8) exactClipRanges {
	bitDepthMax := (1 << bitDepth) - 1
	rowMin := -((bitDepthMax + 1) << 7)
	colMin := -((bitDepthMax + 1) << 5)
	return exactClipRanges{
		rowMin: rowMin,
		rowMax: -rowMin - 1,
		colMin: colMin,
		colMax: -colMin - 1,
	}
}

func inverseExactMatrix(tmp []int, tx TxfmSize, txType TxType, coeffs []int32, clips exactClipRanges) (int, int, bool, error) {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return 0, 0, false, fmt.Errorf("decoder: coefficient buffer too small")
	}
	if len(tmp) < width*height {
		return 0, 0, false, fmt.Errorf("decoder: inverse scratch buffer too small")
	}
	if !exactInverseSupported[tx] {
		return 0, 0, false, nil
	}
	shift := int(exactInverseShift[tx])
	rowKind, colKind, err := tx1DTypesFor(txType)
	if err != nil {
		return 0, 0, false, err
	}
	if !supportsExact1D(rowKind, width) || !supportsExact1D(colKind, height) {
		return 0, 0, false, nil
	}
	rowNeedsClip := rowKind == tx1DKindIdentity
	colNeedsClip := colKind == tx1DKindIdentity

	var rowInBuf [64]int
	var rowOutBuf [64]int
	rowIn := rowInBuf[:width]
	rowOut := rowOutBuf[:width]
	rect2 := width*2 == height || height*2 == width
	for y := 0; y < height; y++ {
		base := y * width
		for x := 0; x < width; x++ {
			v := int(coeffs[base+x])
			if rect2 {
				v = (v*181 + 128) >> 8
			}
			rowIn[x] = v
		}
		if err := exact1D(rowKind, width, rowIn, rowOut, clips.rowMin, clips.rowMax); err != nil {
			return 0, 0, false, err
		}
		if rowNeedsClip {
			for x := 0; x < width; x++ {
				rowOut[x] = clipInt(rowOut[x], clips.rowMin, clips.rowMax)
			}
		}
		copy(tmp[base:base+width], rowOut)
	}

	if shift > 0 {
		rnd := (1 << shift) >> 1
		for i := range tmp {
			tmp[i] = clipInt((tmp[i]+rnd)>>shift, clips.colMin, clips.colMax)
		}
	}

	var colInBuf [64]int
	var colOutBuf [64]int
	colIn := colInBuf[:height]
	colOut := colOutBuf[:height]
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			colIn[y] = tmp[y*width+x]
		}
		if err := exact1D(colKind, height, colIn, colOut, clips.colMin, clips.colMax); err != nil {
			return 0, 0, false, err
		}
		if colNeedsClip {
			for y := 0; y < height; y++ {
				tmp[y*width+x] = clipInt(colOut[y], clips.colMin, clips.colMax)
			}
			continue
		}
		for y := 0; y < height; y++ {
			tmp[y*width+x] = colOut[y]
		}
	}
	return width, height, true, nil
}

func supportsExact1D(kind tx1DKind, n int) bool {
	switch kind {
	case tx1DKindDCT:
		return n == 4 || n == 8 || n == 16 || n == 32 || n == 64
	case tx1DKindADST, tx1DKindFlipADST:
		return n == 4 || n == 8 || n == 16
	case tx1DKindIdentity:
		return n == 4 || n == 8 || n == 16 || n == 32
	default:
		return false
	}
}

func exact1D(kind tx1DKind, n int, in []int, out []int, min, max int) error {
	switch kind {
	case tx1DKindDCT:
		switch n {
		case 4:
			exactInvDCT4(in, out, min, max)
		case 8:
			exactInvDCT8(in, out, min, max)
		case 16:
			exactInvDCT16(in, out, min, max)
		case 32:
			exactInvDCT32(in, out, min, max)
		case 64:
			exactInvDCT64(in, out, min, max)
		default:
			return fmt.Errorf("decoder: exact dct inverse does not support size %d", n)
		}
	case tx1DKindADST:
		switch n {
		case 4:
			exactInvADST4(in, out, min, max)
		case 8:
			exactInvADST8(in, out, min, max)
		case 16:
			exactInvADST16(in, out, min, max)
		default:
			return fmt.Errorf("decoder: exact adst inverse does not support size %d", n)
		}
	case tx1DKindFlipADST:
		var tmpBuf [32]int
		tmp := tmpBuf[:n]
		if err := exact1D(tx1DKindADST, n, in, tmp, min, max); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			out[i] = tmp[n-1-i]
		}
	case tx1DKindIdentity:
		switch n {
		case 4:
			exactInvIdentity4(in, out)
		case 8:
			exactInvIdentity8(in, out)
		case 16:
			exactInvIdentity16(in, out)
		case 32:
			exactInvIdentity32(in, out)
		default:
			return fmt.Errorf("decoder: exact identity inverse does not support size %d", n)
		}
	default:
		return fmt.Errorf("decoder: unsupported exact tx kind %d", kind)
	}
	return nil
}

func exactInvDCT4(in []int, out []int, min, max int) {
	in0, in1, in2, in3 := in[0], in[1], in[2], in[3]
	t0 := ((in0+in2)*181 + 128) >> 8
	t1 := ((in0-in2)*181 + 128) >> 8
	t2 := ((in1*1567 - in3*(3784-4096) + 2048) >> 12) - in3
	t3 := ((in1*(3784-4096) + in3*1567 + 2048) >> 12) + in1
	out[0] = clipInt(t0+t3, min, max)
	out[1] = clipInt(t1+t2, min, max)
	out[2] = clipInt(t1-t2, min, max)
	out[3] = clipInt(t0-t3, min, max)
}

func exactInvDCT8(in []int, out []int, min, max int) {
	var even [4]int
	exactInvDCT4([]int{in[0], in[2], in[4], in[6]}, even[:], min, max)

	in1, in3, in5, in7 := in[1], in[3], in[5], in[7]
	t4a := ((in1*799 - in7*(4017-4096) + 2048) >> 12) - in7
	t5a := (in5*1703 - in3*1138 + 1024) >> 11
	t6a := (in5*1138 + in3*1703 + 1024) >> 11
	t7a := ((in1*(4017-4096) + in7*799 + 2048) >> 12) + in1

	t4 := clipInt(t4a+t5a, min, max)
	t5a = clipInt(t4a-t5a, min, max)
	t7 := clipInt(t7a+t6a, min, max)
	t6a = clipInt(t7a-t6a, min, max)

	t5 := ((t6a-t5a)*181 + 128) >> 8
	t6 := ((t6a+t5a)*181 + 128) >> 8

	out[0] = clipInt(even[0]+t7, min, max)
	out[1] = clipInt(even[1]+t6, min, max)
	out[2] = clipInt(even[2]+t5, min, max)
	out[3] = clipInt(even[3]+t4, min, max)
	out[4] = clipInt(even[3]-t4, min, max)
	out[5] = clipInt(even[2]-t5, min, max)
	out[6] = clipInt(even[1]-t6, min, max)
	out[7] = clipInt(even[0]-t7, min, max)
}

func exactInvDCT16(in []int, out []int, min, max int) {
	var even [8]int
	exactInvDCT8([]int{in[0], in[2], in[4], in[6], in[8], in[10], in[12], in[14]}, even[:], min, max)

	in1, in3, in5, in7 := in[1], in[3], in[5], in[7]
	in9, in11, in13, in15 := in[9], in[11], in[13], in[15]

	t8a := ((in1*401 - in15*(4076-4096) + 2048) >> 12) - in15
	t9a := (in9*1583 - in7*1299 + 1024) >> 11
	t10a := ((in5*1931 - in11*(3612-4096) + 2048) >> 12) - in11
	t11a := ((in13*(3920-4096) - in3*1189 + 2048) >> 12) + in13
	t12a := ((in13*1189 + in3*(3920-4096) + 2048) >> 12) + in3
	t13a := ((in5*(3612-4096) + in11*1931 + 2048) >> 12) + in5
	t14a := (in9*1299 + in7*1583 + 1024) >> 11
	t15a := ((in1*(4076-4096) + in15*401 + 2048) >> 12) + in1

	t8 := clipInt(t8a+t9a, min, max)
	t9 := clipInt(t8a-t9a, min, max)
	t10 := clipInt(t11a-t10a, min, max)
	t11 := clipInt(t11a+t10a, min, max)
	t12 := clipInt(t12a+t13a, min, max)
	t13 := clipInt(t12a-t13a, min, max)
	t14 := clipInt(t15a-t14a, min, max)
	t15 := clipInt(t15a+t14a, min, max)

	t9a = ((t14*1567 - t9*(3784-4096) + 2048) >> 12) - t9
	t14a = ((t14*(3784-4096) + t9*1567 + 2048) >> 12) + t14
	t10a = ((-(t13*(3784-4096) + t10*1567) + 2048) >> 12) - t13
	t13a = ((t13*1567 - t10*(3784-4096) + 2048) >> 12) - t10

	t8a = clipInt(t8+t11, min, max)
	t9 = clipInt(t9a+t10a, min, max)
	t10 = clipInt(t9a-t10a, min, max)
	t11a = clipInt(t8-t11, min, max)
	t12a = clipInt(t15-t12, min, max)
	t13 = clipInt(t14a-t13a, min, max)
	t14 = clipInt(t14a+t13a, min, max)
	t15a = clipInt(t15+t12, min, max)

	t10a = ((t13-t10)*181 + 128) >> 8
	t13a = ((t13+t10)*181 + 128) >> 8
	t11 = ((t12a-t11a)*181 + 128) >> 8
	t12 = ((t12a+t11a)*181 + 128) >> 8

	out[0] = clipInt(even[0]+t15a, min, max)
	out[1] = clipInt(even[1]+t14, min, max)
	out[2] = clipInt(even[2]+t13a, min, max)
	out[3] = clipInt(even[3]+t12, min, max)
	out[4] = clipInt(even[4]+t11, min, max)
	out[5] = clipInt(even[5]+t10a, min, max)
	out[6] = clipInt(even[6]+t9, min, max)
	out[7] = clipInt(even[7]+t8a, min, max)
	out[8] = clipInt(even[7]-t8a, min, max)
	out[9] = clipInt(even[6]-t9, min, max)
	out[10] = clipInt(even[5]-t10a, min, max)
	out[11] = clipInt(even[4]-t11, min, max)
	out[12] = clipInt(even[3]-t12, min, max)
	out[13] = clipInt(even[2]-t13a, min, max)
	out[14] = clipInt(even[1]-t14, min, max)
	out[15] = clipInt(even[0]-t15a, min, max)
}

func exactInvDCT32(in []int, out []int, min, max int) {
	var evenIn [16]int
	for i := range evenIn {
		evenIn[i] = in[i*2]
	}
	var even [16]int
	exactInvDCT16(evenIn[:], even[:], min, max)

	in1, in3, in5, in7 := in[1], in[3], in[5], in[7]
	in9, in11, in13, in15 := in[9], in[11], in[13], in[15]
	in17, in19 := in[17], in[19]
	in21, in23 := in[21], in[23]
	in25, in27 := in[25], in[27]
	in29, in31 := in[29], in[31]

	t16a := ((in1*201 - in31*(4091-4096) + 2048) >> 12) - in31
	t17a := ((in17*(3035-4096) - in15*2751 + 2048) >> 12) + in17
	t18a := ((in9*1751 - in23*(3703-4096) + 2048) >> 12) - in23
	t19a := ((in25*(3857-4096) - in7*1380 + 2048) >> 12) + in25
	t20a := ((in5*995 - in27*(3973-4096) + 2048) >> 12) - in27
	t21a := ((in21*(3513-4096) - in11*2106 + 2048) >> 12) + in21
	t22a := (in13*1220 - in19*1645 + 1024) >> 11
	t23a := ((in29*(4052-4096) - in3*601 + 2048) >> 12) + in29
	t24a := ((in29*601 + in3*(4052-4096) + 2048) >> 12) + in3
	t25a := (in13*1645 + in19*1220 + 1024) >> 11
	t26a := ((in21*2106 + in11*(3513-4096) + 2048) >> 12) + in11
	t27a := ((in5*(3973-4096) + in27*995 + 2048) >> 12) + in5
	t28a := ((in25*1380 + in7*(3857-4096) + 2048) >> 12) + in7
	t29a := ((in9*(3703-4096) + in23*1751 + 2048) >> 12) + in9
	t30a := ((in17*2751 + in15*(3035-4096) + 2048) >> 12) + in15
	t31a := ((in1*(4091-4096) + in31*201 + 2048) >> 12) + in1

	t16 := clipInt(t16a+t17a, min, max)
	t17 := clipInt(t16a-t17a, min, max)
	t18 := clipInt(t19a-t18a, min, max)
	t19 := clipInt(t19a+t18a, min, max)
	t20 := clipInt(t20a+t21a, min, max)
	t21 := clipInt(t20a-t21a, min, max)
	t22 := clipInt(t23a-t22a, min, max)
	t23 := clipInt(t23a+t22a, min, max)
	t24 := clipInt(t24a+t25a, min, max)
	t25 := clipInt(t24a-t25a, min, max)
	t26 := clipInt(t27a-t26a, min, max)
	t27 := clipInt(t27a+t26a, min, max)
	t28 := clipInt(t28a+t29a, min, max)
	t29 := clipInt(t28a-t29a, min, max)
	t30 := clipInt(t31a-t30a, min, max)
	t31 := clipInt(t31a+t30a, min, max)

	t17a = ((t30*799 - t17*(4017-4096) + 2048) >> 12) - t17
	t30a = ((t30*(4017-4096) + t17*799 + 2048) >> 12) + t30
	t18a = ((-(t29*(4017-4096) + t18*799) + 2048) >> 12) - t29
	t29a = ((t29*799 - t18*(4017-4096) + 2048) >> 12) - t18
	t21a = (t26*1703 - t21*1138 + 1024) >> 11
	t26a = (t26*1138 + t21*1703 + 1024) >> 11
	t22a = (-(t25*1138 + t22*1703) + 1024) >> 11
	t25a = (t25*1703 - t22*1138 + 1024) >> 11

	t16a = clipInt(t16+t19, min, max)
	t17 = clipInt(t17a+t18a, min, max)
	t18 = clipInt(t17a-t18a, min, max)
	t19a = clipInt(t16-t19, min, max)
	t20a = clipInt(t23-t20, min, max)
	t21 = clipInt(t22a-t21a, min, max)
	t22 = clipInt(t22a+t21a, min, max)
	t23a = clipInt(t23+t20, min, max)
	t24a = clipInt(t24+t27, min, max)
	t25 = clipInt(t25a+t26a, min, max)
	t26 = clipInt(t25a-t26a, min, max)
	t27a = clipInt(t24-t27, min, max)
	t28a = clipInt(t31-t28, min, max)
	t29 = clipInt(t30a-t29a, min, max)
	t30 = clipInt(t30a+t29a, min, max)
	t31a = clipInt(t31+t28, min, max)

	t18a = ((t29*1567 - t18*(3784-4096) + 2048) >> 12) - t18
	t29a = ((t29*(3784-4096) + t18*1567 + 2048) >> 12) + t29
	t19 = ((t28a*1567 - t19a*(3784-4096) + 2048) >> 12) - t19a
	t28 = ((t28a*(3784-4096) + t19a*1567 + 2048) >> 12) + t28a
	t20 = ((-(t27a*(3784-4096) + t20a*1567) + 2048) >> 12) - t27a
	t27 = ((t27a*1567 - t20a*(3784-4096) + 2048) >> 12) - t20a
	t21a = ((-(t26*(3784-4096) + t21*1567) + 2048) >> 12) - t26
	t26a = ((t26*1567 - t21*(3784-4096) + 2048) >> 12) - t21

	t16 = clipInt(t16a+t23a, min, max)
	t17a = clipInt(t17+t22, min, max)
	t18 = clipInt(t18a+t21a, min, max)
	t19a = clipInt(t19+t20, min, max)
	t20a = clipInt(t19-t20, min, max)
	t21 = clipInt(t18a-t21a, min, max)
	t22a = clipInt(t17-t22, min, max)
	t23 = clipInt(t16a-t23a, min, max)
	t24 = clipInt(t31a-t24a, min, max)
	t25a = clipInt(t30-t25, min, max)
	t26 = clipInt(t29a-t26a, min, max)
	t27a = clipInt(t28-t27, min, max)
	t28a = clipInt(t28+t27, min, max)
	t29 = clipInt(t29a+t26a, min, max)
	t30a = clipInt(t30+t25, min, max)
	t31 = clipInt(t31a+t24a, min, max)

	t20 = ((t27a-t20a)*181 + 128) >> 8
	t27 = ((t27a+t20a)*181 + 128) >> 8
	t21a = ((t26-t21)*181 + 128) >> 8
	t26a = ((t26+t21)*181 + 128) >> 8
	t22 = ((t25a-t22a)*181 + 128) >> 8
	t25 = ((t25a+t22a)*181 + 128) >> 8
	t23a = ((t24-t23)*181 + 128) >> 8
	t24a = ((t24+t23)*181 + 128) >> 8

	out[0] = clipInt(even[0]+t31, min, max)
	out[1] = clipInt(even[1]+t30a, min, max)
	out[2] = clipInt(even[2]+t29, min, max)
	out[3] = clipInt(even[3]+t28a, min, max)
	out[4] = clipInt(even[4]+t27, min, max)
	out[5] = clipInt(even[5]+t26a, min, max)
	out[6] = clipInt(even[6]+t25, min, max)
	out[7] = clipInt(even[7]+t24a, min, max)
	out[8] = clipInt(even[8]+t23a, min, max)
	out[9] = clipInt(even[9]+t22, min, max)
	out[10] = clipInt(even[10]+t21a, min, max)
	out[11] = clipInt(even[11]+t20, min, max)
	out[12] = clipInt(even[12]+t19a, min, max)
	out[13] = clipInt(even[13]+t18, min, max)
	out[14] = clipInt(even[14]+t17a, min, max)
	out[15] = clipInt(even[15]+t16, min, max)
	out[16] = clipInt(even[15]-t16, min, max)
	out[17] = clipInt(even[14]-t17a, min, max)
	out[18] = clipInt(even[13]-t18, min, max)
	out[19] = clipInt(even[12]-t19a, min, max)
	out[20] = clipInt(even[11]-t20, min, max)
	out[21] = clipInt(even[10]-t21a, min, max)
	out[22] = clipInt(even[9]-t22, min, max)
	out[23] = clipInt(even[8]-t23a, min, max)
	out[24] = clipInt(even[7]-t24a, min, max)
	out[25] = clipInt(even[6]-t25, min, max)
	out[26] = clipInt(even[5]-t26a, min, max)
	out[27] = clipInt(even[4]-t27, min, max)
	out[28] = clipInt(even[3]-t28a, min, max)
	out[29] = clipInt(even[2]-t29, min, max)
	out[30] = clipInt(even[1]-t30a, min, max)
	out[31] = clipInt(even[0]-t31, min, max)
}

func exactInvDCT4Tx64(in []int, out []int, min, max int) {
	in0, in1 := in[0], in[1]
	t0 := (in0*181 + 128) >> 8
	t1 := t0
	t2 := (in1*1567 + 2048) >> 12
	t3 := (in1*3784 + 2048) >> 12
	out[0] = clipInt(t0+t3, min, max)
	out[1] = clipInt(t1+t2, min, max)
	out[2] = clipInt(t1-t2, min, max)
	out[3] = clipInt(t0-t3, min, max)
}

func exactInvDCT8Tx64(in []int, out []int, min, max int) {
	var even [4]int
	exactInvDCT4Tx64([]int{in[0], in[2], in[4], in[6]}, even[:], min, max)

	in1, in3 := in[1], in[3]
	t4a := (in1*799 + 2048) >> 12
	t5a := (in3*-2276 + 2048) >> 12
	t6a := (in3*3406 + 2048) >> 12
	t7a := (in1*4017 + 2048) >> 12

	t4 := clipInt(t4a+t5a, min, max)
	t5a = clipInt(t4a-t5a, min, max)
	t7 := clipInt(t7a+t6a, min, max)
	t6a = clipInt(t7a-t6a, min, max)

	t5 := ((t6a - t5a) * 181) + 128
	t5 >>= 8
	t6 := ((t6a + t5a) * 181) + 128
	t6 >>= 8

	out[0] = clipInt(even[0]+t7, min, max)
	out[1] = clipInt(even[1]+t6, min, max)
	out[2] = clipInt(even[2]+t5, min, max)
	out[3] = clipInt(even[3]+t4, min, max)
	out[4] = clipInt(even[3]-t4, min, max)
	out[5] = clipInt(even[2]-t5, min, max)
	out[6] = clipInt(even[1]-t6, min, max)
	out[7] = clipInt(even[0]-t7, min, max)
}

func exactInvDCT16Tx64(in []int, out []int, min, max int) {
	var evenIn [8]int
	for i := range evenIn {
		evenIn[i] = in[i*2]
	}
	var even [8]int
	exactInvDCT8Tx64(evenIn[:], even[:], min, max)

	in1, in3, in5, in7 := in[1], in[3], in[5], in[7]

	t8a := (in1*401 + 2048) >> 12
	t9a := (in7*-2598 + 2048) >> 12
	t10a := (in5*1931 + 2048) >> 12
	t11a := (in3*-1189 + 2048) >> 12
	t12a := (in3*3920 + 2048) >> 12
	t13a := (in5*3612 + 2048) >> 12
	t14a := (in7*3166 + 2048) >> 12
	t15a := (in1*4076 + 2048) >> 12

	t8 := clipInt(t8a+t9a, min, max)
	t9 := clipInt(t8a-t9a, min, max)
	t10 := clipInt(t11a-t10a, min, max)
	t11 := clipInt(t11a+t10a, min, max)
	t12 := clipInt(t12a+t13a, min, max)
	t13 := clipInt(t12a-t13a, min, max)
	t14 := clipInt(t15a-t14a, min, max)
	t15 := clipInt(t15a+t14a, min, max)

	t9a = ((t14*1567 - t9*(3784-4096) + 2048) >> 12) - t9
	t14a = ((t14*(3784-4096) + t9*1567 + 2048) >> 12) + t14
	t10a = ((-(t13*(3784-4096) + t10*1567) + 2048) >> 12) - t13
	t13a = ((t13*1567 - t10*(3784-4096) + 2048) >> 12) - t10

	t8a = clipInt(t8+t11, min, max)
	t9 = clipInt(t9a+t10a, min, max)
	t10 = clipInt(t9a-t10a, min, max)
	t11a = clipInt(t8-t11, min, max)
	t12a = clipInt(t15-t12, min, max)
	t13 = clipInt(t14a-t13a, min, max)
	t14 = clipInt(t14a+t13a, min, max)
	t15a = clipInt(t15+t12, min, max)

	t10a = ((t13-t10)*181 + 128) >> 8
	t13a = ((t13+t10)*181 + 128) >> 8
	t11 = ((t12a-t11a)*181 + 128) >> 8
	t12 = ((t12a+t11a)*181 + 128) >> 8

	out[0] = clipInt(even[0]+t15a, min, max)
	out[1] = clipInt(even[1]+t14, min, max)
	out[2] = clipInt(even[2]+t13a, min, max)
	out[3] = clipInt(even[3]+t12, min, max)
	out[4] = clipInt(even[4]+t11, min, max)
	out[5] = clipInt(even[5]+t10a, min, max)
	out[6] = clipInt(even[6]+t9, min, max)
	out[7] = clipInt(even[7]+t8a, min, max)
	out[8] = clipInt(even[7]-t8a, min, max)
	out[9] = clipInt(even[6]-t9, min, max)
	out[10] = clipInt(even[5]-t10a, min, max)
	out[11] = clipInt(even[4]-t11, min, max)
	out[12] = clipInt(even[3]-t12, min, max)
	out[13] = clipInt(even[2]-t13a, min, max)
	out[14] = clipInt(even[1]-t14, min, max)
	out[15] = clipInt(even[0]-t15a, min, max)
}

func exactInvDCT32Tx64(in []int, out []int, min, max int) {
	var evenIn [16]int
	for i := range evenIn {
		evenIn[i] = in[i*2]
	}
	var even [16]int
	exactInvDCT16Tx64(evenIn[:], even[:], min, max)

	in1, in3, in5, in7 := in[1], in[3], in[5], in[7]
	in9, in11, in13, in15 := in[9], in[11], in[13], in[15]

	t16a := (in1*201 + 2048) >> 12
	t17a := (in15*-2751 + 2048) >> 12
	t18a := (in9*1751 + 2048) >> 12
	t19a := (in7*-1380 + 2048) >> 12
	t20a := (in5*995 + 2048) >> 12
	t21a := (in11*-2106 + 2048) >> 12
	t22a := (in13*2440 + 2048) >> 12
	t23a := (in3*-601 + 2048) >> 12
	t24a := (in3*4052 + 2048) >> 12
	t25a := (in13*3290 + 2048) >> 12
	t26a := (in11*3513 + 2048) >> 12
	t27a := (in5*3973 + 2048) >> 12
	t28a := (in7*3857 + 2048) >> 12
	t29a := (in9*3703 + 2048) >> 12
	t30a := (in15*3035 + 2048) >> 12
	t31a := (in1*4091 + 2048) >> 12

	t16 := clipInt(t16a+t17a, min, max)
	t17 := clipInt(t16a-t17a, min, max)
	t18 := clipInt(t19a-t18a, min, max)
	t19 := clipInt(t19a+t18a, min, max)
	t20 := clipInt(t20a+t21a, min, max)
	t21 := clipInt(t20a-t21a, min, max)
	t22 := clipInt(t23a-t22a, min, max)
	t23 := clipInt(t23a+t22a, min, max)
	t24 := clipInt(t24a+t25a, min, max)
	t25 := clipInt(t24a-t25a, min, max)
	t26 := clipInt(t27a-t26a, min, max)
	t27 := clipInt(t27a+t26a, min, max)
	t28 := clipInt(t28a+t29a, min, max)
	t29 := clipInt(t28a-t29a, min, max)
	t30 := clipInt(t31a-t30a, min, max)
	t31 := clipInt(t31a+t30a, min, max)

	t17a = ((t30*799 - t17*(4017-4096) + 2048) >> 12) - t17
	t30a = ((t30*(4017-4096) + t17*799 + 2048) >> 12) + t30
	t18a = ((-(t29*(4017-4096) + t18*799) + 2048) >> 12) - t29
	t29a = ((t29*799 - t18*(4017-4096) + 2048) >> 12) - t18
	t21a = (t26*1703 - t21*1138 + 1024) >> 11
	t26a = (t26*1138 + t21*1703 + 1024) >> 11
	t22a = (-(t25*1138 + t22*1703) + 1024) >> 11
	t25a = (t25*1703 - t22*1138 + 1024) >> 11

	t16a = clipInt(t16+t19, min, max)
	t17 = clipInt(t17a+t18a, min, max)
	t18 = clipInt(t17a-t18a, min, max)
	t19a = clipInt(t16-t19, min, max)
	t20a = clipInt(t23-t20, min, max)
	t21 = clipInt(t22a-t21a, min, max)
	t22 = clipInt(t22a+t21a, min, max)
	t23a = clipInt(t23+t20, min, max)
	t24a = clipInt(t24+t27, min, max)
	t25 = clipInt(t25a+t26a, min, max)
	t26 = clipInt(t25a-t26a, min, max)
	t27a = clipInt(t24-t27, min, max)
	t28a = clipInt(t31-t28, min, max)
	t29 = clipInt(t30a-t29a, min, max)
	t30 = clipInt(t30a+t29a, min, max)
	t31a = clipInt(t31+t28, min, max)

	t18a = ((t29*1567 - t18*(3784-4096) + 2048) >> 12) - t18
	t29a = ((t29*(3784-4096) + t18*1567 + 2048) >> 12) + t29
	t19 = ((t28a*1567 - t19a*(3784-4096) + 2048) >> 12) - t19a
	t28 = ((t28a*(3784-4096) + t19a*1567 + 2048) >> 12) + t28a
	t20 = ((-(t27a*(3784-4096) + t20a*1567) + 2048) >> 12) - t27a
	t27 = ((t27a*1567 - t20a*(3784-4096) + 2048) >> 12) - t20a
	t21a = ((-(t26*(3784-4096) + t21*1567) + 2048) >> 12) - t26
	t26a = ((t26*1567 - t21*(3784-4096) + 2048) >> 12) - t21

	t16 = clipInt(t16a+t23a, min, max)
	t17a = clipInt(t17+t22, min, max)
	t18 = clipInt(t18a+t21a, min, max)
	t19a = clipInt(t19+t20, min, max)
	t20a = clipInt(t19-t20, min, max)
	t21 = clipInt(t18a-t21a, min, max)
	t22a = clipInt(t17-t22, min, max)
	t23 = clipInt(t16a-t23a, min, max)
	t24 = clipInt(t31a-t24a, min, max)
	t25a = clipInt(t30-t25, min, max)
	t26 = clipInt(t29a-t26a, min, max)
	t27a = clipInt(t28-t27, min, max)
	t28a = clipInt(t28+t27, min, max)
	t29 = clipInt(t29a+t26a, min, max)
	t30a = clipInt(t30+t25, min, max)
	t31 = clipInt(t31a+t24a, min, max)

	t20 = ((t27a-t20a)*181 + 128) >> 8
	t27 = ((t27a+t20a)*181 + 128) >> 8
	t21a = ((t26-t21)*181 + 128) >> 8
	t26a = ((t26+t21)*181 + 128) >> 8
	t22 = ((t25a-t22a)*181 + 128) >> 8
	t25 = ((t25a+t22a)*181 + 128) >> 8
	t23a = ((t24-t23)*181 + 128) >> 8
	t24a = ((t24+t23)*181 + 128) >> 8

	out[0] = clipInt(even[0]+t31, min, max)
	out[1] = clipInt(even[1]+t30a, min, max)
	out[2] = clipInt(even[2]+t29, min, max)
	out[3] = clipInt(even[3]+t28a, min, max)
	out[4] = clipInt(even[4]+t27, min, max)
	out[5] = clipInt(even[5]+t26a, min, max)
	out[6] = clipInt(even[6]+t25, min, max)
	out[7] = clipInt(even[7]+t24a, min, max)
	out[8] = clipInt(even[8]+t23a, min, max)
	out[9] = clipInt(even[9]+t22, min, max)
	out[10] = clipInt(even[10]+t21a, min, max)
	out[11] = clipInt(even[11]+t20, min, max)
	out[12] = clipInt(even[12]+t19a, min, max)
	out[13] = clipInt(even[13]+t18, min, max)
	out[14] = clipInt(even[14]+t17a, min, max)
	out[15] = clipInt(even[15]+t16, min, max)
	out[16] = clipInt(even[15]-t16, min, max)
	out[17] = clipInt(even[14]-t17a, min, max)
	out[18] = clipInt(even[13]-t18, min, max)
	out[19] = clipInt(even[12]-t19a, min, max)
	out[20] = clipInt(even[11]-t20, min, max)
	out[21] = clipInt(even[10]-t21a, min, max)
	out[22] = clipInt(even[9]-t22, min, max)
	out[23] = clipInt(even[8]-t23a, min, max)
	out[24] = clipInt(even[7]-t24a, min, max)
	out[25] = clipInt(even[6]-t25, min, max)
	out[26] = clipInt(even[5]-t26a, min, max)
	out[27] = clipInt(even[4]-t27, min, max)
	out[28] = clipInt(even[3]-t28a, min, max)
	out[29] = clipInt(even[2]-t29, min, max)
	out[30] = clipInt(even[1]-t30a, min, max)
	out[31] = clipInt(even[0]-t31, min, max)
}

func exactInvDCT64(in []int, out []int, min, max int) {
	var evenIn [32]int
	for i := range evenIn {
		evenIn[i] = in[i*2]
	}
	var even [32]int
	exactInvDCT32Tx64(evenIn[:], even[:], min, max)

	in1, in3, in5, in7 := in[1], in[3], in[5], in[7]
	in9, in11, in13, in15 := in[9], in[11], in[13], in[15]
	in17, in19, in21, in23 := in[17], in[19], in[21], in[23]
	in25, in27, in29, in31 := in[25], in[27], in[29], in[31]

	t32a := (in1*101 + 2048) >> 12
	t33a := (in31*-2824 + 2048) >> 12
	t34a := (in17*1660 + 2048) >> 12
	t35a := (in15*-1474 + 2048) >> 12
	t36a := (in9*897 + 2048) >> 12
	t37a := (in23*-2191 + 2048) >> 12
	t38a := (in25*2359 + 2048) >> 12
	t39a := (in7*-700 + 2048) >> 12
	t40a := (in5*501 + 2048) >> 12
	t41a := (in27*-2520 + 2048) >> 12
	t42a := (in21*2019 + 2048) >> 12
	t43a := (in11*-1092 + 2048) >> 12
	t44a := (in13*1285 + 2048) >> 12
	t45a := (in19*-1842 + 2048) >> 12
	t46a := (in29*2675 + 2048) >> 12
	t47a := (in3*-301 + 2048) >> 12
	t48a := (in3*4085 + 2048) >> 12
	t49a := (in29*3102 + 2048) >> 12
	t50a := (in19*3659 + 2048) >> 12
	t51a := (in13*3889 + 2048) >> 12
	t52a := (in11*3948 + 2048) >> 12
	t53a := (in21*3564 + 2048) >> 12
	t54a := (in27*3229 + 2048) >> 12
	t55a := (in5*4065 + 2048) >> 12
	t56a := (in7*4036 + 2048) >> 12
	t57a := (in25*3349 + 2048) >> 12
	t58a := (in23*3461 + 2048) >> 12
	t59a := (in9*3996 + 2048) >> 12
	t60a := (in15*3822 + 2048) >> 12
	t61a := (in17*3745 + 2048) >> 12
	t62a := (in31*2967 + 2048) >> 12
	t63a := (in1*4095 + 2048) >> 12

	t32 := clipInt(t32a+t33a, min, max)
	t33 := clipInt(t32a-t33a, min, max)
	t34 := clipInt(t35a-t34a, min, max)
	t35 := clipInt(t35a+t34a, min, max)
	t36 := clipInt(t36a+t37a, min, max)
	t37 := clipInt(t36a-t37a, min, max)
	t38 := clipInt(t39a-t38a, min, max)
	t39 := clipInt(t39a+t38a, min, max)
	t40 := clipInt(t40a+t41a, min, max)
	t41 := clipInt(t40a-t41a, min, max)
	t42 := clipInt(t43a-t42a, min, max)
	t43 := clipInt(t43a+t42a, min, max)
	t44 := clipInt(t44a+t45a, min, max)
	t45 := clipInt(t44a-t45a, min, max)
	t46 := clipInt(t47a-t46a, min, max)
	t47 := clipInt(t47a+t46a, min, max)
	t48 := clipInt(t48a+t49a, min, max)
	t49 := clipInt(t48a-t49a, min, max)
	t50 := clipInt(t51a-t50a, min, max)
	t51 := clipInt(t51a+t50a, min, max)
	t52 := clipInt(t52a+t53a, min, max)
	t53 := clipInt(t52a-t53a, min, max)
	t54 := clipInt(t55a-t54a, min, max)
	t55 := clipInt(t55a+t54a, min, max)
	t56 := clipInt(t56a+t57a, min, max)
	t57 := clipInt(t56a-t57a, min, max)
	t58 := clipInt(t59a-t58a, min, max)
	t59 := clipInt(t59a+t58a, min, max)
	t60 := clipInt(t60a+t61a, min, max)
	t61 := clipInt(t60a-t61a, min, max)
	t62 := clipInt(t63a-t62a, min, max)
	t63 := clipInt(t63a+t62a, min, max)

	t33a = ((t33*(4096-4076) + t62*401 + 2048) >> 12) - t33
	t34a = ((t34*-401 + t61*(4096-4076) + 2048) >> 12) - t61
	t37a = (t37*-1299 + t58*1583 + 1024) >> 11
	t38a = (t38*-1583 + t57*-1299 + 1024) >> 11
	t41a = ((t41*(4096-3612) + t54*1931 + 2048) >> 12) - t41
	t42a = ((t42*-1931 + t53*(4096-3612) + 2048) >> 12) - t53
	t45a = ((t45*-1189 + t50*(3920-4096) + 2048) >> 12) + t50
	t46a = ((t46*(4096-3920) + t49*-1189 + 2048) >> 12) - t46
	t49a = ((t46*-1189 + t49*(3920-4096) + 2048) >> 12) + t49
	t50a = ((t45*(3920-4096) + t50*1189 + 2048) >> 12) + t45
	t53a = ((t42*(4096-3612) + t53*1931 + 2048) >> 12) - t42
	t54a = ((t41*1931 + t54*(3612-4096) + 2048) >> 12) + t54
	t57a = (t38*-1299 + t57*1583 + 1024) >> 11
	t58a = (t37*1583 + t58*1299 + 1024) >> 11
	t61a = ((t34*(4096-4076) + t61*401 + 2048) >> 12) - t34
	t62a = ((t33*401 + t62*(4076-4096) + 2048) >> 12) + t62

	t32a = clipInt(t32+t35, min, max)
	t33 = clipInt(t33a+t34a, min, max)
	t34 = clipInt(t33a-t34a, min, max)
	t35a = clipInt(t32-t35, min, max)
	t36a = clipInt(t39-t36, min, max)
	t37 = clipInt(t38a-t37a, min, max)
	t38 = clipInt(t38a+t37a, min, max)
	t39a = clipInt(t39+t36, min, max)
	t40a = clipInt(t40+t43, min, max)
	t41 = clipInt(t41a+t42a, min, max)
	t42 = clipInt(t41a-t42a, min, max)
	t43a = clipInt(t40-t43, min, max)
	t44a = clipInt(t47-t44, min, max)
	t45 = clipInt(t46a-t45a, min, max)
	t46 = clipInt(t46a+t45a, min, max)
	t47a = clipInt(t47+t44, min, max)
	t48a = clipInt(t48+t51, min, max)
	t49 = clipInt(t49a+t50a, min, max)
	t50 = clipInt(t49a-t50a, min, max)
	t51a = clipInt(t48-t51, min, max)
	t52a = clipInt(t55-t52, min, max)
	t53 = clipInt(t54a-t53a, min, max)
	t54 = clipInt(t54a+t53a, min, max)
	t55a = clipInt(t55+t52, min, max)
	t56a = clipInt(t56+t59, min, max)
	t57 = clipInt(t57a+t58a, min, max)
	t58 = clipInt(t57a-t58a, min, max)
	t59a = clipInt(t56-t59, min, max)
	t60a = clipInt(t63-t60, min, max)
	t61 = clipInt(t62a-t61a, min, max)
	t62 = clipInt(t62a+t61a, min, max)
	t63a = clipInt(t63+t60, min, max)

	t34a = ((t34*(4096-4017) + t61*799 + 2048) >> 12) - t34
	t35 = ((t35a*(4096-4017) + t60a*799 + 2048) >> 12) - t35a
	t36 = ((t36a*-799 + t59a*(4096-4017) + 2048) >> 12) - t59a
	t37a = ((t37*-799 + t58*(4096-4017) + 2048) >> 12) - t58
	t42a = (t42*-1138 + t53*1703 + 1024) >> 11
	t43 = (t43a*-1138 + t52a*1703 + 1024) >> 11
	t44 = (t44a*-1703 + t51a*-1138 + 1024) >> 11
	t45a = (t45*-1703 + t50*-1138 + 1024) >> 11
	t50a = (t45*-1138 + t50*1703 + 1024) >> 11
	t51 = (t44a*-1138 + t51a*1703 + 1024) >> 11
	t52 = (t43a*1703 + t52a*1138 + 1024) >> 11
	t53a = (t42*1703 + t53*1138 + 1024) >> 11
	t58a = ((t37*(4096-4017) + t58*799 + 2048) >> 12) - t37
	t59 = ((t36a*(4096-4017) + t59a*799 + 2048) >> 12) - t36a
	t60 = ((t35a*799 + t60a*(4017-4096) + 2048) >> 12) + t60a
	t61a = ((t34*799 + t61*(4017-4096) + 2048) >> 12) + t61

	t32 = clipInt(t32a+t39a, min, max)
	t33a = clipInt(t33+t38, min, max)
	t34 = clipInt(t34a+t37a, min, max)
	t35a = clipInt(t35+t36, min, max)
	t36a = clipInt(t35-t36, min, max)
	t37 = clipInt(t34a-t37a, min, max)
	t38a = clipInt(t33-t38, min, max)
	t39 = clipInt(t32a-t39a, min, max)
	t40 = clipInt(t47a-t40a, min, max)
	t41a = clipInt(t46-t41, min, max)
	t42 = clipInt(t45a-t42a, min, max)
	t43a = clipInt(t44-t43, min, max)
	t44a = clipInt(t44+t43, min, max)
	t45 = clipInt(t45a+t42a, min, max)
	t46a = clipInt(t46+t41, min, max)
	t47 = clipInt(t47a+t40a, min, max)
	t48 = clipInt(t48a+t55a, min, max)
	t49a = clipInt(t49+t54, min, max)
	t50 = clipInt(t50a+t53a, min, max)
	t51a = clipInt(t51+t52, min, max)
	t52a = clipInt(t51-t52, min, max)
	t53 = clipInt(t50a-t53a, min, max)
	t54a = clipInt(t49-t54, min, max)
	t55 = clipInt(t48a-t55a, min, max)
	t56 = clipInt(t63a-t56a, min, max)
	t57a = clipInt(t62-t57, min, max)
	t58 = clipInt(t61a-t58a, min, max)
	t59a = clipInt(t60-t59, min, max)
	t60a = clipInt(t60+t59, min, max)
	t61 = clipInt(t61a+t58a, min, max)
	t62a = clipInt(t62+t57, min, max)
	t63 = clipInt(t63a+t56a, min, max)

	t36 = ((t36a*(4096-3784) + t59a*1567 + 2048) >> 12) - t36a
	t37a = ((t37*(4096-3784) + t58*1567 + 2048) >> 12) - t37
	t38 = ((t38a*(4096-3784) + t57a*1567 + 2048) >> 12) - t38a
	t39a = ((t39*(4096-3784) + t56*1567 + 2048) >> 12) - t39
	t40a = ((t40*-1567 + t55*(4096-3784) + 2048) >> 12) - t55
	t41 = ((t41a*-1567 + t54a*(4096-3784) + 2048) >> 12) - t54a
	t42a = ((t42*-1567 + t53*(4096-3784) + 2048) >> 12) - t53
	t43 = ((t43a*-1567 + t52a*(4096-3784) + 2048) >> 12) - t52a
	t52 = ((t43a*(4096-3784) + t52a*1567 + 2048) >> 12) - t43a
	t53a = ((t42*(4096-3784) + t53*1567 + 2048) >> 12) - t42
	t54 = ((t41a*(4096-3784) + t54a*1567 + 2048) >> 12) - t41a
	t55a = ((t40*(4096-3784) + t55*1567 + 2048) >> 12) - t40
	t56a = ((t39*1567 + t56*(3784-4096) + 2048) >> 12) + t56
	t57 = ((t38a*1567 + t57a*(3784-4096) + 2048) >> 12) + t57a
	t58a = ((t37*1567 + t58*(3784-4096) + 2048) >> 12) + t58
	t59 = ((t36a*1567 + t59a*(3784-4096) + 2048) >> 12) + t59a

	t32a = clipInt(t32+t47, min, max)
	t33 = clipInt(t33a+t46a, min, max)
	t34a = clipInt(t34+t45, min, max)
	t35 = clipInt(t35a+t44a, min, max)
	t36a = clipInt(t36+t43, min, max)
	t37 = clipInt(t37a+t42a, min, max)
	t38a = clipInt(t38+t41, min, max)
	t39 = clipInt(t39a+t40a, min, max)
	t40 = clipInt(t39a-t40a, min, max)
	t41a = clipInt(t38-t41, min, max)
	t42 = clipInt(t37a-t42a, min, max)
	t43a = clipInt(t36-t43, min, max)
	t44 = clipInt(t35a-t44a, min, max)
	t45a = clipInt(t34-t45, min, max)
	t46 = clipInt(t33a-t46a, min, max)
	t47a = clipInt(t32-t47, min, max)
	t48a = clipInt(t63-t48, min, max)
	t49 = clipInt(t62a-t49a, min, max)
	t50a = clipInt(t61-t50, min, max)
	t51 = clipInt(t60a-t51a, min, max)
	t52a = clipInt(t59-t52, min, max)
	t53 = clipInt(t58a-t53a, min, max)
	t54a = clipInt(t57-t54, min, max)
	t55 = clipInt(t56a-t55a, min, max)
	t56 = clipInt(t56a+t55a, min, max)
	t57a = clipInt(t57+t54, min, max)
	t58 = clipInt(t58a+t53a, min, max)
	t59a = clipInt(t59+t52, min, max)
	t60 = clipInt(t60a+t51a, min, max)
	t61a = clipInt(t61+t50, min, max)
	t62 = clipInt(t62a+t49a, min, max)
	t63a = clipInt(t63+t48, min, max)

	t40a = ((t55-t40)*181 + 128) >> 8
	t41 = ((t54a-t41a)*181 + 128) >> 8
	t42a = ((t53-t42)*181 + 128) >> 8
	t43 = ((t52a-t43a)*181 + 128) >> 8
	t44a = ((t51-t44)*181 + 128) >> 8
	t45 = ((t50a-t45a)*181 + 128) >> 8
	t46a = ((t49-t46)*181 + 128) >> 8
	t47 = ((t48a-t47a)*181 + 128) >> 8
	t48 = ((t47a+t48a)*181 + 128) >> 8
	t49a = ((t46+t49)*181 + 128) >> 8
	t50 = ((t45a+t50a)*181 + 128) >> 8
	t51a = ((t44+t51)*181 + 128) >> 8
	t52 = ((t43a+t52a)*181 + 128) >> 8
	t53a = ((t42+t53)*181 + 128) >> 8
	t54 = ((t41a+t54a)*181 + 128) >> 8
	t55a = ((t40+t55)*181 + 128) >> 8

	out[0] = clipInt(even[0]+t63a, min, max)
	out[1] = clipInt(even[1]+t62, min, max)
	out[2] = clipInt(even[2]+t61a, min, max)
	out[3] = clipInt(even[3]+t60, min, max)
	out[4] = clipInt(even[4]+t59a, min, max)
	out[5] = clipInt(even[5]+t58, min, max)
	out[6] = clipInt(even[6]+t57a, min, max)
	out[7] = clipInt(even[7]+t56, min, max)
	out[8] = clipInt(even[8]+t55a, min, max)
	out[9] = clipInt(even[9]+t54, min, max)
	out[10] = clipInt(even[10]+t53a, min, max)
	out[11] = clipInt(even[11]+t52, min, max)
	out[12] = clipInt(even[12]+t51a, min, max)
	out[13] = clipInt(even[13]+t50, min, max)
	out[14] = clipInt(even[14]+t49a, min, max)
	out[15] = clipInt(even[15]+t48, min, max)
	out[16] = clipInt(even[16]+t47, min, max)
	out[17] = clipInt(even[17]+t46a, min, max)
	out[18] = clipInt(even[18]+t45, min, max)
	out[19] = clipInt(even[19]+t44a, min, max)
	out[20] = clipInt(even[20]+t43, min, max)
	out[21] = clipInt(even[21]+t42a, min, max)
	out[22] = clipInt(even[22]+t41, min, max)
	out[23] = clipInt(even[23]+t40a, min, max)
	out[24] = clipInt(even[24]+t39, min, max)
	out[25] = clipInt(even[25]+t38a, min, max)
	out[26] = clipInt(even[26]+t37, min, max)
	out[27] = clipInt(even[27]+t36a, min, max)
	out[28] = clipInt(even[28]+t35, min, max)
	out[29] = clipInt(even[29]+t34a, min, max)
	out[30] = clipInt(even[30]+t33, min, max)
	out[31] = clipInt(even[31]+t32a, min, max)
	out[32] = clipInt(even[31]-t32a, min, max)
	out[33] = clipInt(even[30]-t33, min, max)
	out[34] = clipInt(even[29]-t34a, min, max)
	out[35] = clipInt(even[28]-t35, min, max)
	out[36] = clipInt(even[27]-t36a, min, max)
	out[37] = clipInt(even[26]-t37, min, max)
	out[38] = clipInt(even[25]-t38a, min, max)
	out[39] = clipInt(even[24]-t39, min, max)
	out[40] = clipInt(even[23]-t40a, min, max)
	out[41] = clipInt(even[22]-t41, min, max)
	out[42] = clipInt(even[21]-t42a, min, max)
	out[43] = clipInt(even[20]-t43, min, max)
	out[44] = clipInt(even[19]-t44a, min, max)
	out[45] = clipInt(even[18]-t45, min, max)
	out[46] = clipInt(even[17]-t46a, min, max)
	out[47] = clipInt(even[16]-t47, min, max)
	out[48] = clipInt(even[15]-t48, min, max)
	out[49] = clipInt(even[14]-t49a, min, max)
	out[50] = clipInt(even[13]-t50, min, max)
	out[51] = clipInt(even[12]-t51a, min, max)
	out[52] = clipInt(even[11]-t52, min, max)
	out[53] = clipInt(even[10]-t53a, min, max)
	out[54] = clipInt(even[9]-t54, min, max)
	out[55] = clipInt(even[8]-t55a, min, max)
	out[56] = clipInt(even[7]-t56, min, max)
	out[57] = clipInt(even[6]-t57a, min, max)
	out[58] = clipInt(even[5]-t58, min, max)
	out[59] = clipInt(even[4]-t59a, min, max)
	out[60] = clipInt(even[3]-t60, min, max)
	out[61] = clipInt(even[2]-t61a, min, max)
	out[62] = clipInt(even[1]-t62, min, max)
	out[63] = clipInt(even[0]-t63a, min, max)
}

func exactInvADST4(in []int, out []int, min, max int) {
	in0, in1, in2, in3 := in[0], in[1], in[2], in[3]
	out[0] = ((1321*in0 + (3803-4096)*in2 + (2482-4096)*in3 + (3344-4096)*in1 + 2048) >> 12) + in2 + in3 + in1
	out[1] = (((2482-4096)*in0 - 1321*in2 - (3803-4096)*in3 + (3344-4096)*in1 + 2048) >> 12) + in0 - in3 + in1
	out[2] = (209*(in0-in2+in3) + 128) >> 8
	out[3] = (((3803-4096)*in0 + (2482-4096)*in2 - 1321*in3 - (3344-4096)*in1 + 2048) >> 12) + in0 + in2 - in1
	for i := 0; i < 4; i++ {
		out[i] = clipInt(out[i], min, max)
	}
}

func exactInvADST8(in []int, out []int, min, max int) {
	in0, in1, in2, in3 := in[0], in[1], in[2], in[3]
	in4, in5, in6, in7 := in[4], in[5], in[6], in[7]

	t0a := (((4076 - 4096) * in7) + 401*in0 + 2048) >> 12
	t0a += in7
	t1a := ((401*in7 - (4076-4096)*in0 + 2048) >> 12) - in0
	t2a := (((3612 - 4096) * in5) + 1931*in2 + 2048) >> 12
	t2a += in5
	t3a := ((1931*in5 - (3612-4096)*in2 + 2048) >> 12) - in2
	t4a := (1299*in3 + 1583*in4 + 1024) >> 11
	t5a := (1583*in3 - 1299*in4 + 1024) >> 11
	t6a := ((1189*in1 + (3920-4096)*in6 + 2048) >> 12) + in6
	t7a := (((3920-4096)*in1 - 1189*in6 + 2048) >> 12) + in1

	t0 := clipInt(t0a+t4a, min, max)
	t1 := clipInt(t1a+t5a, min, max)
	t2 := clipInt(t2a+t6a, min, max)
	t3 := clipInt(t3a+t7a, min, max)
	t4 := clipInt(t0a-t4a, min, max)
	t5 := clipInt(t1a-t5a, min, max)
	t6 := clipInt(t2a-t6a, min, max)
	t7 := clipInt(t3a-t7a, min, max)

	t4a = (((3784 - 4096) * t4) + 1567*t5 + 2048) >> 12
	t4a += t4
	t5a = ((1567*t4 - (3784-4096)*t5 + 2048) >> 12) - t5
	t6a = (((3784 - 4096) * t7) - 1567*t6 + 2048) >> 12
	t6a += t7
	t7a = ((1567*t7 + (3784-4096)*t6 + 2048) >> 12) + t6

	out[0] = clipInt(t0+t2, min, max)
	out[7] = -clipInt(t1+t3, min, max)
	t2 = clipInt(t0-t2, min, max)
	t3 = clipInt(t1-t3, min, max)
	out[1] = -clipInt(t4a+t6a, min, max)
	out[6] = clipInt(t5a+t7a, min, max)
	t6 = clipInt(t4a-t6a, min, max)
	t7 = clipInt(t5a-t7a, min, max)

	out[3] = -(((t2+t3)*181 + 128) >> 8)
	out[4] = ((t2-t3)*181 + 128) >> 8
	out[2] = ((t6+t7)*181 + 128) >> 8
	out[5] = -(((t6-t7)*181 + 128) >> 8)
	for i := 0; i < 8; i++ {
		out[i] = clipInt(out[i], min, max)
	}
}

func exactInvADST16(in []int, out []int, min, max int) {
	in0, in1, in2, in3 := in[0], in[1], in[2], in[3]
	in4, in5, in6, in7 := in[4], in[5], in[6], in[7]
	in8, in9, in10, in11 := in[8], in[9], in[10], in[11]
	in12, in13, in14, in15 := in[12], in[13], in[14], in[15]

	t0 := ((in15*(4091-4096) + in0*201 + 2048) >> 12) + in15
	t1 := ((in15*201 - in0*(4091-4096) + 2048) >> 12) - in0
	t2 := ((in13*(3973-4096) + in2*995 + 2048) >> 12) + in13
	t3 := ((in13*995 - in2*(3973-4096) + 2048) >> 12) - in2
	t4 := ((in11*(3703-4096) + in4*1751 + 2048) >> 12) + in11
	t5 := ((in11*1751 - in4*(3703-4096) + 2048) >> 12) - in4
	t6 := (in9*1645 + in6*1220 + 1024) >> 11
	t7 := (in9*1220 - in6*1645 + 1024) >> 11
	t8 := ((in7*2751 + in8*(3035-4096) + 2048) >> 12) + in8
	t9 := ((in7*(3035-4096) - in8*2751 + 2048) >> 12) + in7
	t10 := ((in5*2106 + in10*(3513-4096) + 2048) >> 12) + in10
	t11 := ((in5*(3513-4096) - in10*2106 + 2048) >> 12) + in5
	t12 := ((in3*1380 + in12*(3857-4096) + 2048) >> 12) + in12
	t13 := ((in3*(3857-4096) - in12*1380 + 2048) >> 12) + in3
	t14 := ((in1*601 + in14*(4052-4096) + 2048) >> 12) + in14
	t15 := ((in1*(4052-4096) - in14*601 + 2048) >> 12) + in1

	t0a := t0 + t8
	t1a := t1 + t9
	t2a := t2 + t10
	t3a := t3 + t11
	t4a := t4 + t12
	t5a := t5 + t13
	t6a := t6 + t14
	t7a := t7 + t15
	t8a := t0 - t8
	t9a := t1 - t9
	t10a := t2 - t10
	t11a := t3 - t11
	t12a := t4 - t12
	t13a := t5 - t13
	t14a := t6 - t14
	t15a := t7 - t15

	t8 = (((4017 - 4096) * t8a) + 799*t9a + 2048) >> 12
	t8 += t8a
	t9 = ((799*t8a - (4017-4096)*t9a + 2048) >> 12) - t9a
	t10 = ((2276*t10a + (3406-4096)*t11a + 2048) >> 12) + t11a
	t11 = (((3406-4096)*t10a - 2276*t11a + 2048) >> 12) + t10a
	t12 = (((4017 - 4096) * t13a) - 799*t12a + 2048) >> 12
	t12 += t13a
	t13 = ((799*t13a + (4017-4096)*t12a + 2048) >> 12) + t12a
	t14 = ((2276*t15a - (3406-4096)*t14a + 2048) >> 12) - t14a
	t15 = (((3406-4096)*t15a + 2276*t14a + 2048) >> 12) + t15a

	t0 = clipInt(t0a+t4a, min, max)
	t1 = clipInt(t1a+t5a, min, max)
	t2 = clipInt(t2a+t6a, min, max)
	t3 = clipInt(t3a+t7a, min, max)
	t4 = clipInt(t0a-t4a, min, max)
	t5 = clipInt(t1a-t5a, min, max)
	t6 = clipInt(t2a-t6a, min, max)
	t7 = clipInt(t3a-t7a, min, max)
	t8a = clipInt(t8+t12, min, max)
	t9a = clipInt(t9+t13, min, max)
	t10a = clipInt(t10+t14, min, max)
	t11a = clipInt(t11+t15, min, max)
	t12a = clipInt(t8-t12, min, max)
	t13a = clipInt(t9-t13, min, max)
	t14a = clipInt(t10-t14, min, max)
	t15a = clipInt(t11-t15, min, max)

	t4a = (((3784 - 4096) * t4) + 1567*t5 + 2048) >> 12
	t4a += t4
	t5a = ((1567*t4 - (3784-4096)*t5 + 2048) >> 12) - t5
	t6a = (((3784 - 4096) * t7) - 1567*t6 + 2048) >> 12
	t6a += t7
	t7a = ((1567*t7 + (3784-4096)*t6 + 2048) >> 12) + t6
	t12 = (((3784 - 4096) * t12a) + 1567*t13a + 2048) >> 12
	t12 += t12a
	t13 = ((1567*t12a - (3784-4096)*t13a + 2048) >> 12) - t13a
	t14 = (((3784 - 4096) * t15a) - 1567*t14a + 2048) >> 12
	t14 += t15a
	t15 = ((1567*t15a + (3784-4096)*t14a + 2048) >> 12) + t14a

	out[0] = clipInt(t0+t2, min, max)
	out[15] = -clipInt(t1+t3, min, max)
	t2a = clipInt(t0-t2, min, max)
	t3a = clipInt(t1-t3, min, max)
	out[3] = -clipInt(t4a+t6a, min, max)
	out[12] = clipInt(t5a+t7a, min, max)
	t6 = clipInt(t4a-t6a, min, max)
	t7 = clipInt(t5a-t7a, min, max)
	out[1] = -clipInt(t8a+t10a, min, max)
	out[14] = clipInt(t9a+t11a, min, max)
	t10 = clipInt(t8a-t10a, min, max)
	t11 = clipInt(t9a-t11a, min, max)
	out[2] = clipInt(t12+t14, min, max)
	out[13] = -clipInt(t13+t15, min, max)
	t14a = clipInt(t12-t14, min, max)
	t15a = clipInt(t13-t15, min, max)

	out[7] = -(((t2a+t3a)*181 + 128) >> 8)
	out[8] = ((t2a-t3a)*181 + 128) >> 8
	out[4] = ((t6+t7)*181 + 128) >> 8
	out[11] = -(((t6-t7)*181 + 128) >> 8)
	out[6] = ((t10+t11)*181 + 128) >> 8
	out[9] = -(((t10-t11)*181 + 128) >> 8)
	out[5] = -(((t14a+t15a)*181 + 128) >> 8)
	out[10] = ((t14a-t15a)*181 + 128) >> 8
	for i := 0; i < 16; i++ {
		out[i] = clipInt(out[i], min, max)
	}
}

func exactInvIdentity4(in []int, out []int) {
	for i := 0; i < 4; i++ {
		v := in[i]
		out[i] = v + ((v*1697 + 2048) >> 12)
	}
}

func exactInvIdentity8(in []int, out []int) {
	for i := 0; i < 8; i++ {
		out[i] = in[i] * 2
	}
}

func exactInvIdentity16(in []int, out []int) {
	for i := 0; i < 16; i++ {
		v := in[i]
		out[i] = 2*v + ((v*1697 + 1024) >> 11)
	}
}

func exactInvIdentity32(in []int, out []int) {
	for i := 0; i < 32; i++ {
		out[i] = in[i] * 4
	}
}
