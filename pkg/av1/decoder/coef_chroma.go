package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func DecodeChromaCoefficientsDCT2D(bs BlockSize, g BlockGeometry, tx TxfmSize, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) ([]int32, uint8, bool, error) {
	return DecodeChromaCoefficients(bs, g, tx, TxTypeDCTDCT, segID, plane, layout, coef, dq, above, left, dec)
}

func DecodeChromaCoefficientsDCT2DWithBitDepth(bs BlockSize, g BlockGeometry, tx TxfmSize, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) ([]int32, uint8, bool, error) {
	return DecodeChromaCoefficientsWithBitDepth(bs, g, tx, TxTypeDCTDCT, segID, plane, layout, coef, dq, above, left, dec, bitDepth)
}

func DecodeChromaCoefficients(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) ([]int32, uint8, bool, error) {
	return DecodeChromaCoefficientsWithBitDepth(bs, g, tx, txType, segID, plane, layout, coef, dq, above, left, dec, 8)
}

func DecodeChromaCoefficientsWithBitDepth(bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) ([]int32, uint8, bool, error) {
	coeffs := make([]int32, coeffBufferLen(tx))
	resCtx, skip, err := DecodeChromaCoefficientsIntoWithBitDepth(coeffs, bs, g, tx, txType, segID, plane, layout, coef, dq, above, left, dec, bitDepth)
	if err != nil {
		return nil, 0, false, err
	}
	return coeffs, resCtx, skip, nil
}

func DecodeChromaCoefficientsInto(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) (uint8, bool, error) {
	return DecodeChromaCoefficientsIntoWithBitDepth(coeffs, bs, g, tx, txType, segID, plane, layout, coef, dq, above, left, dec, 8)
}

func DecodeChromaCoefficientsIntoWithBitDepth(coeffs []int32, bs BlockSize, g BlockGeometry, tx TxfmSize, txType TxType, segID uint8, plane int, layout av1.ChromaSubsampling, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder, bitDepth uint8) (uint8, bool, error) {
	if coef == nil {
		return 0, false, fmt.Errorf("decoder: coefficient cdf required")
	}
	if above == nil || left == nil {
		return 0, false, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return 0, false, fmt.Errorf("decoder: entropy decoder required")
	}
	if int(segID) >= len(dq) {
		return 0, false, fmt.Errorf("decoder: invalid segment id %d", segID)
	}
	if plane < 1 || plane > 2 {
		return 0, false, fmt.Errorf("decoder: invalid chroma plane %d", plane)
	}
	bx4, by4 := chromaCoeffContextOffsets(layout, g)
	return decodeCoefficientsInto(coeffs, bs, g, tx, txType, segID, plane, true, layout, coef, dq, above.CCoef[plane-1][bx4:], left.CCoef[plane-1][by4:], dec, true, bitDepth)
}

func ApplyChromaResidualContext(above, left *BlockContext, plane int, g BlockGeometry, tx TxfmSize, resCtx uint8) error {
	return ApplyChromaResidualContextWithLayout(above, left, plane, av1.Chroma420, g, tx, resCtx)
}

func ApplyChromaResidualContextWithLayout(above, left *BlockContext, plane int, layout av1.ChromaSubsampling, g BlockGeometry, tx TxfmSize, resCtx uint8) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	if plane < 1 || plane > 2 {
		return fmt.Errorf("decoder: invalid chroma plane %d", plane)
	}
	info := TxfmInfoFor(tx)
	bx4, by4 := chromaCoeffContextOffsets(layout, g)
	fillRunUint8(above.CCoef[plane-1][:], bx4, minInt(int(info.W4), g.Width4()), resCtx)
	fillRunUint8(left.CCoef[plane-1][:], by4, minInt(int(info.H4), g.Height4()), resCtx)
	return nil
}

func chromaCoeffContextOffsets(layout av1.ChromaSubsampling, g BlockGeometry) (int, int) {
	ssX, ssY := chromaSubsamplingShifts(layout)
	return chromaCoeffContextOffset(g.Start4X, ssX), chromaCoeffContextOffset(g.Start4Y, ssY)
}

func chromaCoeffContextOffset(pos4, subsamplingShift int) int {
	if subsamplingShift <= 0 {
		return pos4 & 31
	}
	return pos4 & ((32 >> subsamplingShift) - 1)
}
