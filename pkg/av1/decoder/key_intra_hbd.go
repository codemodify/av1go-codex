package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func DecodeAndReconstructKeyIntraDCLumaBlockAt16(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder) (IntraBlockSyntax, error) {
	return DecodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighbors(seq, hdr, modeCDF, coefCDF, dq, above, left, g, segID, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, dec, g.Start4Y > 0, g.Start4X > 0)
}

func DecodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighbors(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft bool) (IntraBlockSyntax, error) {
	syntax, _, err := decodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighbors(seq, hdr, modeCDF, coefCDF, dq, above, left, g, segID, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, dec, haveTop, haveLeft)
	return syntax, err
}

func decodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighbors(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft bool) (IntraBlockSyntax, bool, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, false, fmt.Errorf("decoder: frame header required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, false, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax, err := ReadIntraBlockSyntax(seq, hdr, modeCDF, above, left, g, haveTop, haveLeft, segID, dec)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	hasResidual, err := reconstructParsedIntraLumaBlockAt16WithResidual(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, dec, haveTop, haveLeft, true, true)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	syntax.PreserveTXContext = !syntax.Skip
	if err := ApplyIntraBlockContext(seq, hdr, above, left, syntax); err != nil {
		return IntraBlockSyntax{}, false, err
	}
	return syntax, hasResidual, nil
}

func ReconstructParsedIntraLumaBlockAt16(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder) error {
	return ReconstructParsedIntraLumaBlockAt16NeighborEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, true, true)
}

func ReconstructParsedIntraLumaBlockAt16NeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	_, err := reconstructParsedIntraLumaBlockAt16WithResidual(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, bitDepth, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	return err
}

func reconstructParsedIntraLumaBlockAt16WithResidual(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []uint16, stride, planeWidth, planeHeight, planeX4, planeY4 int, bitDepth uint8, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) (bool, error) {
	if hdr == nil {
		return false, fmt.Errorf("decoder: frame header required")
	}
	if modeCDF == nil {
		return false, fmt.Errorf("decoder: mode cdf required")
	}
	if dec == nil {
		return false, fmt.Errorf("decoder: entropy decoder required")
	}
	g := syntax.Geometry
	blockX := planeX4 * 4
	blockY := planeY4 * 4
	predictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	hasResidual := false
	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	unitsScratch := takeTransformUnitScratch(g)
	defer putTransformUnitScratch(unitsScratch)
	units, err := ReadIntraTransformUnitsInto(unitsScratch.buf[:0], hdr, modeCDF, above, left, g, syntax.TX, syntax.Segment, dec)
	if err != nil {
		return false, err
	}

	for _, unit := range units {
		tu := unit.Global
		tx := unit.TX
		txInfo := TxfmInfoFor(tx)
		offX4 := tu.Start4X - g.Start4X
		offY4 := tu.Start4Y - g.Start4Y
		tuHaveTop := haveTop || offY4 > 0
		tuHaveLeft := haveLeft || offX4 > 0
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		var (
			yTxType TxType
			coeffs  []int32
			resCtx  uint8
			skip    bool
			err     error
		)
		if dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:]) != 0 {
			skip = true
			resCtx = 0x40
			coeffs = make([]int32, int(txInfo.W4)*4*int(txInfo.H4)*4)
			if int(syntax.Segment) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[syntax.Segment] {
				yTxType = TxTypeWHTWHT
			} else {
				yTxType = TxTypeDCTDCT
			}
		} else {
			yTxType, err = ReadIntraTxType(hdr, modeCDF, syntax, tx, tuHaveTop, tuHaveLeft, dec)
			if err != nil {
				return false, err
			}
			coeffs, resCtx, skip, err = DecodeLumaCoefficientsNoSkipWithBitDepth(syntax.Size, tu, tx, yTxType, syntax.Segment, coefCDF, dq, above, left, dec, bitDepth)
			if err != nil {
				return false, err
			}
		}
		hasResidual = hasResidual || !skip
		px := (planeX4 + offX4) * 4
		py := (planeY4 + offY4) * 4
		if syntax.PalYSize != 0 {
			err = ReconstructPaletteLumaCoefficients16WithTxType(plane, stride, planeWidth, planeHeight, px, py, tx, yTxType, coeffs, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], syntax, blockX, blockY)
		} else {
			tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, tx, intraEdgeAvailability{
				topHasRight:   topHasRight,
				leftHasBottom: leftHasBottom,
			})
			err = ReconstructIntraLumaCoefficients16WithTxTypeFlagsNeighborEdges(plane, stride, planeWidth, planeHeight, px, py, tx, yTxType, syntax.YMode, syntax.YAngle, coeffs, bitDepth, hdr.Segmentation.Lossless[syntax.Segment], predictFlags, haveTop || offY4 > 0, haveLeft || offX4 > 0, tuEdge.topHasRight, tuEdge.leftHasBottom)
		}
		if err != nil {
			return false, err
		}
		if err := ApplyLumaResidualContext(above, left, tu, tx, resCtx); err != nil {
			return false, err
		}
	}
	return hasResidual, nil
}
