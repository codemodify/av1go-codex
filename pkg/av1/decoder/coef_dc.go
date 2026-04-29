package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/internal/msac"
)

type coefficientEntropyDecoder interface {
	DecodeBoolEqui() uint32
	DecodeBoolAdapt([]uint16) uint32
	DecodeSymbolAdapt([]uint16, int) uint32
	DecodeHiTok([]uint16) uint32
}

type msacCoefficientDecoder struct {
	ctx *msac.Context
}

func (d msacCoefficientDecoder) DecodeBoolEqui() uint32 {
	return d.ctx.DecodeBoolEqui()
}

func (d msacCoefficientDecoder) DecodeBoolAdapt(cdf []uint16) uint32 {
	return d.ctx.DecodeBoolAdapt(cdf)
}

func (d msacCoefficientDecoder) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	return d.ctx.DecodeSymbolAdapt(cdf, n)
}

func (d msacCoefficientDecoder) DecodeHiTok(cdf []uint16) uint32 {
	return d.ctx.DecodeHiTok(cdf)
}

func DecodeLumaDCOnlyCoefficient(bs BlockSize, g BlockGeometry, tx TxfmSize, segID uint8, coef *CoefCDF, dq DequantTables, above, left *BlockContext, dec coefficientEntropyDecoder) (int32, uint8, bool, error) {
	if coef == nil {
		return 0, 0, false, fmt.Errorf("decoder: coefficient cdf required")
	}
	if above == nil || left == nil {
		return 0, 0, false, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return 0, 0, false, fmt.Errorf("decoder: entropy decoder required")
	}
	if int(segID) >= len(dq) {
		return 0, 0, false, fmt.Errorf("decoder: invalid segment id %d", segID)
	}

	info := TxfmInfoFor(tx)
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	skipCtx := GetSkipCtx(info, bs, above.LCoef[bx4:], left.LCoef[by4:], false, 0)
	if dec.DecodeBoolAdapt(coef.Skip[info.Ctx][skipCtx][:]) != 0 {
		return 0, 0x40, true, nil
	}

	eob, err := decodeEOBDCOnly(info, coef, dec)
	if err != nil {
		return 0, 0, false, err
	}
	if eob != 0 {
		return 0, 0, false, fmt.Errorf("decoder: dc-only helper received eob=%d with AC coefficients", eob)
	}

	dcTok, err := decodeDCTokenDCOnly(info, coef, dec)
	if err != nil {
		return 0, 0, false, err
	}
	dcSignCtx := GetDCSignCtx(tx, above.LCoef[bx4:], left.LCoef[by4:])
	neg := dec.DecodeBoolAdapt(coef.DCSign[0][dcSignCtx][:]) != 0

	value := int(dq[segID][0][0])
	if dcTok == 15 {
		dcTok = int(decodeGolomb(dec.DecodeBoolEqui)) + 15
	}
	dqShift := maxIntLocal(0, int(info.Ctx)-2)
	value = (value * dcTok) >> dqShift
	cfMax := (1 << 15) - 1
	if neg {
		value = min(value, cfMax+1)
		value = -value
	} else {
		value = min(value, cfMax)
	}

	culLevel := min(dcTok, 63)
	resCtx := uint8(culLevel)
	if !neg {
		resCtx |= 2 << 6
	}
	return int32(value), resCtx, false, nil
}

func ApplyLumaResidualContext(above, left *BlockContext, g BlockGeometry, tx TxfmSize, resCtx uint8) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	info := TxfmInfoFor(tx)
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	fillRunUint8(above.LCoef[:], bx4, minInt(int(info.W4), g.Width4()), resCtx)
	fillRunUint8(left.LCoef[:], by4, minInt(int(info.H4), g.Height4()), resCtx)
	return nil
}

func decodeEOBDCOnly(info TxfmInfo, coef *CoefCDF, dec coefficientEntropyDecoder) (int, error) {
	slw := minInt(int(info.LW), int(TxfmInfoFor(TX32X32).LW))
	slh := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	tx2dCtx := slw + slh

	switch tx2dCtx {
	case 0:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin16[0][0][:], 4)), nil
	case 1:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin32[0][0][:], 5)), nil
	case 2:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin64[0][0][:], 6)), nil
	case 3:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin128[0][0][:], 7)), nil
	case 4:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin256[0][0][:], 8)), nil
	case 5:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin512[0][:], 9)), nil
	case 6:
		return int(dec.DecodeSymbolAdapt(coef.EOBBin1024[0][:], 10)), nil
	default:
		return 0, fmt.Errorf("decoder: unsupported tx2d context %d", tx2dCtx)
	}
}

func decodeDCTokenDCOnly(info TxfmInfo, coef *CoefCDF, dec coefficientEntropyDecoder) (int, error) {
	eobRow := coeffCDFRowForCoef(coef, int(info.Ctx), "eob-base", len(coef.EOBBaseTok)-1)
	tokBR := int(dec.DecodeSymbolAdapt(coef.EOBBaseTok[eobRow][0][0][:], 2))
	if tokBR < 2 {
		return tokBR + 1, nil
	}
	brRow := coeffBRCDFRowForOrder(coef, minInt(int(info.Ctx), 3), 0, len(coef.BRTok)-1)
	return int(dec.DecodeHiTok(coef.BRTok[brRow][0][0][:])), nil
}
