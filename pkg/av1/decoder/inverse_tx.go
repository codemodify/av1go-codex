package decoder

import (
	"fmt"
	"math"
	"os"
)

var (
	adstBasisCache     = map[int][]float64{}
	identityBasisCache = map[int][]float64{}
)

func InverseAddResidualTx8(dst []byte, stride int, tx TxfmSize, txType TxType, coeffs []int32, lossless bool) error {
	if lossless {
		if tx != TX4X4 {
			return fmt.Errorf("decoder: lossless inverse transform only supports tx4x4, got %d", tx)
		}
		return InverseAddWHTWHT4X48(dst, stride, coeffs)
	}
	if txType == TxTypeWHTWHT {
		if tx != TX4X4 {
			return fmt.Errorf("decoder: wht inverse transform only supports tx4x4, got %d", tx)
		}
		return InverseAddWHTWHT4X48(dst, stride, coeffs)
	}
	if debugForceDisableExactInverse() {
		if txType == TxTypeDCTDCT {
			return InverseAddDCTDCT8(dst, stride, tx, coeffs)
		}
		return inverseAddGeneric8(dst, stride, tx, txType, coeffs)
	}
	if ok, err := inverseAddExact8(dst, stride, tx, txType, coeffs); ok || err != nil {
		return err
	}
	if txType == TxTypeDCTDCT {
		return InverseAddDCTDCT8(dst, stride, tx, coeffs)
	}
	return inverseAddGeneric8(dst, stride, tx, txType, coeffs)
}

func InverseAddResidualTx16(dst []uint16, stride int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8, lossless bool) error {
	if lossless {
		if tx != TX4X4 {
			return fmt.Errorf("decoder: lossless inverse transform only supports tx4x4, got %d", tx)
		}
		return InverseAddWHTWHT4X416(dst, stride, coeffs, bitDepth)
	}
	if txType == TxTypeWHTWHT {
		if tx != TX4X4 {
			return fmt.Errorf("decoder: wht inverse transform only supports tx4x4, got %d", tx)
		}
		return InverseAddWHTWHT4X416(dst, stride, coeffs, bitDepth)
	}
	if debugForceDisableExactInverse() {
		if txType == TxTypeDCTDCT {
			return InverseAddDCTDCT16(dst, stride, tx, coeffs, bitDepth)
		}
		return inverseAddGeneric16(dst, stride, tx, txType, coeffs, bitDepth)
	}
	if ok, err := inverseAddExact16(dst, stride, tx, txType, coeffs, bitDepth); ok || err != nil {
		return err
	}
	if txType == TxTypeDCTDCT {
		return InverseAddDCTDCT16(dst, stride, tx, coeffs, bitDepth)
	}
	return inverseAddGeneric16(dst, stride, tx, txType, coeffs, bitDepth)
}

func debugForceDisableExactInverse() bool {
	return os.Getenv("DEBUG_FORCE_DISABLE_EXACT_INVERSE") != ""
}

func inverseAddGeneric8(dst []byte, stride int, tx TxfmSize, txType TxType, coeffs []int32) error {
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

	rowKind, colKind, err := tx1DTypesFor(txType)
	if err != nil {
		return err
	}
	rowBasis, err := txBasis(rowKind, width)
	if err != nil {
		return err
	}
	colBasis, err := txBasis(colKind, height)
	if err != nil {
		return err
	}

	tmp := make([]float64, height*width)
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
			row[x] = clipByte(int(row[x]) + int(math.Round(sum/8.0)))
		}
	}
	return nil
}

func inverseAddGeneric16(dst []uint16, stride int, tx TxfmSize, txType TxType, coeffs []int32, bitDepth uint8) error {
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

	rowKind, colKind, err := tx1DTypesFor(txType)
	if err != nil {
		return err
	}
	rowBasis, err := txBasis(rowKind, width)
	if err != nil {
		return err
	}
	colBasis, err := txBasis(colKind, height)
	if err != nil {
		return err
	}

	tmp := make([]float64, height*width)
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
			row[x] = clipSample(int(row[x])+int(math.Round(sum/8.0)), bitDepth)
		}
	}
	return nil
}

func txBasis(kind tx1DKind, n int) ([]float64, error) {
	switch kind {
	case tx1DKindDCT:
		return idctBasis(n), nil
	case tx1DKindADST:
		return iadstBasis(n)
	case tx1DKindFlipADST:
		adst, err := iadstBasis(n)
		if err != nil {
			return nil, err
		}
		out := make([]float64, len(adst))
		for k := 0; k < n; k++ {
			for x := 0; x < n; x++ {
				out[k*n+x] = adst[k*n+(n-1-x)]
			}
		}
		return out, nil
	case tx1DKindIdentity:
		return identityBasis(n)
	default:
		return nil, fmt.Errorf("decoder: unsupported tx 1d kind %d", kind)
	}
}

func iadstBasis(n int) ([]float64, error) {
	if basis, ok := adstBasisCache[n]; ok {
		return basis, nil
	}
	switch n {
	case 4, 8, 16:
	default:
		return nil, fmt.Errorf("decoder: iadst basis does not support size %d", n)
	}
	basis := make([]float64, n*n)
	scale := math.Sqrt(2.0 / float64(2*n+1))
	for k := 0; k < n; k++ {
		for x := 0; x < n; x++ {
			basis[k*n+x] = scale * math.Sin((math.Pi*float64(2*x+1)*float64(k+1))/float64(2*n+1))
		}
	}
	adstBasisCache[n] = basis
	return basis, nil
}

func identityBasis(n int) ([]float64, error) {
	if basis, ok := identityBasisCache[n]; ok {
		return basis, nil
	}
	scale, ok := map[int]float64{
		4:  math.Sqrt2,
		8:  2.0,
		16: 2.0 * math.Sqrt2,
		32: 4.0,
	}[n]
	if !ok {
		return nil, fmt.Errorf("decoder: identity basis does not support size %d", n)
	}
	basis := make([]float64, n*n)
	for i := 0; i < n; i++ {
		basis[i*n+i] = scale
	}
	identityBasisCache[n] = basis
	return basis, nil
}
