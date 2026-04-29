package decoder

import (
	"fmt"
	"math"
	"os"
	"sync"
)

var inverseDenseFloatScratchPool sync.Pool

func ReconstructIntraLumaCoefficients8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, mode IntraPredMode, angle int8, coeffs []int32, lossless bool) error {
	return ReconstructIntraLumaCoefficients8WithTxType(plane, stride, planeWidth, planeHeight, x, y, tx, TxTypeDCTDCT, mode, angle, coeffs, lossless)
}

func ReconstructIntraLumaCoefficients8WithTxType(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, lossless bool) error {
	return ReconstructIntraLumaCoefficients8WithTxTypeFlags(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, coeffs, lossless, 0)
}

func ReconstructIntraLumaCoefficients8WithTxTypeFlags(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, lossless bool, flags uint16) error {
	return ReconstructIntraLumaCoefficients8WithTxTypeFlagsEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, coeffs, lossless, flags, true, true)
}

func ReconstructIntraLumaCoefficients8WithTxTypeFlagsEdges(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, lossless bool, flags uint16, topHasRight, leftHasBottom bool) error {
	return ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, x, y, tx, txType, mode, angle, coeffs, lossless, flags, y > 0, x > 0, topHasRight, leftHasBottom)
}

func ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, mode IntraPredMode, angle int8, coeffs []int32, lossless bool, flags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	defer writeback.release()
	traceTarget := debugTraceMovieBlock(x, y, width, height)
	if err := PredictIntraLuma8WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, flags); err != nil {
		return err
	}
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace target xy=(%d,%d) before_residual txType=%d lossless=%v mode=%d angle=%d topLeft=%d haveTop=%v haveLeft=%v top=%v left=%v pred=%v coeffs=%v\n",
			x, y, txType, lossless, mode, angle, topLeft, haveTop, haveLeft, top, left,
			[][]byte{
				append([]byte(nil), dst[0:4]...),
				append([]byte(nil), dst[dstStride:dstStride+4]...),
				append([]byte(nil), dst[2*dstStride:2*dstStride+4]...),
				append([]byte(nil), dst[3*dstStride:3*dstStride+4]...),
			},
			append([]int32(nil), coeffs[:16]...),
		)
	}
	if err := InverseAddResidualTx8(dst, dstStride, tx, txType, coeffs, lossless); err != nil {
		return fmt.Errorf("decoder: inverse intra transform add at (%d,%d) tx=%d failed: %w", x, y, tx, err)
	}
	if traceTarget {
		fmt.Fprintf(os.Stderr, "trace target xy=(%d,%d) after_residual out=%v\n",
			x, y,
			debugTraceRows8(dst, dstStride, width, height),
		)
	}
	writeback.commit()
	return nil
}

func PredictIntraLumaBlock8WithFlagsNeighborEdges(plane []byte, stride, planeWidth, planeHeight, x, y, width, height int, mode IntraPredMode, angle int8, flags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	dst, dstStride, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, x, y, width, height, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	defer writeback.release()
	if err := PredictIntraLuma8WithFlags(dst, dstStride, width, height, mode, angle, top, left, topLeft, haveTop, haveLeft, flags); err != nil {
		return err
	}
	writeback.commit()
	return nil
}

func InverseAddResidualTx8At(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, txType TxType, coeffs []int32, lossless bool) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if len(coeffs) < width*height {
		return fmt.Errorf("decoder: coefficient buffer too small for %dx%d tx", width, height)
	}
	if x >= planeWidth || y >= planeHeight {
		return nil
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if visibleWidth == width && visibleHeight == height {
		return InverseAddResidualTx8(plane[y*stride+x:], stride, tx, txType, coeffs, lossless)
	}
	scratch := make([]byte, width*height)
	for row := 0; row < visibleHeight; row++ {
		copy(scratch[row*width:row*width+visibleWidth], plane[(y+row)*stride+x:(y+row)*stride+x+visibleWidth])
	}
	if err := InverseAddResidualTx8(scratch, width, tx, txType, coeffs, lossless); err != nil {
		return err
	}
	for row := 0; row < visibleHeight; row++ {
		copy(plane[(y+row)*stride+x:(y+row)*stride+x+visibleWidth], scratch[row*width:row*width+visibleWidth])
	}
	return nil
}

func InverseAddResidual8(dst []byte, stride int, tx TxfmSize, coeffs []int32, lossless bool) error {
	return InverseAddResidualTx8(dst, stride, tx, TxTypeDCTDCT, coeffs, lossless)
}

func debugTraceRows8(dst []byte, stride, width, height int) [][]byte {
	if stride <= 0 || width <= 0 || height <= 0 || len(dst) == 0 {
		return nil
	}
	rows := []int{0, 1, 2, 3}
	if height > 8 {
		rows = append(rows, height-4, height-3, height-2, height-1)
	}
	out := make([][]byte, 0, len(rows))
	for _, row := range rows {
		if row < 0 || row >= height {
			continue
		}
		start := row * stride
		w := minInt(width, 8)
		if start+w > len(dst) {
			continue
		}
		out = append(out, append([]byte(nil), dst[start:start+w]...))
	}
	return out
}

func inverseAddDCTDCT8Dense(dst []byte, stride int, tx TxfmSize, coeffs []int32) error {
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

	if ok, err := inverseAddExact8(dst, stride, tx, TxTypeDCTDCT, coeffs); ok || err != nil {
		return err
	}

	rowBasis := idctBasis(width)
	colBasis := idctBasis(height)
	tmpScratch := takeScratch[float64](&inverseDenseFloatScratchPool, height*width)
	defer putScratch(&inverseDenseFloatScratchPool, tmpScratch)
	tmp := tmpScratch.buf
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
			row[x] = clipByte(int(row[x]) + roundFloatToInt(sum*0.125))
		}
	}
	return nil
}

func InverseAddDCTDCT8(dst []byte, stride int, tx TxfmSize, coeffs []int32) error {
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
			return InverseAddDCTDCTDC8(dst, stride, tx, coeff)
		}
	}
	return inverseAddDCTDCT8Dense(dst, stride, tx, coeffs)
}

func idctBasis(n int) []float64 {
	if basis, ok := idctBasisCache[n]; ok {
		return basis
	}
	basis := make([]float64, n*n)
	scale := 2.0 / float64(n)
	for u := 0; u < n; u++ {
		alpha := 1.0
		if u == 0 {
			alpha = 1.0 / math.Sqrt2
		}
		for x := 0; x < n; x++ {
			basis[u*n+x] = scale * alpha * math.Cos((math.Pi*float64(2*x+1)*float64(u))/(2.0*float64(n)))
		}
	}
	idctBasisCache[n] = basis
	return basis
}

var idctBasisCache = map[int][]float64{}

func coeffsDConly(coeffs []int32, n int) (bool, int32) {
	if len(coeffs) < n {
		return false, 0
	}
	for i := 1; i < n; i++ {
		if coeffs[i] != 0 {
			return false, 0
		}
	}
	return true, coeffs[0]
}
