package decoder

const (
	dcPredMode                 = 0
	nearestMVMode              = 0
	switchableFilterUnset      = 3
	txSize64x64           int8 = 4
)

type BlockContext struct {
	Mode      [32]uint8
	LCoef     [32]uint8
	CCoef     [2][32]uint8
	Pal       [3][32][8]uint16
	SegPred   [32]uint8
	Skip      [32]uint8
	SkipMode  [32]uint8
	Intra     [32]uint8
	CompType  [32]uint8
	Ref       [2][32]int8
	Filter    [2][32]uint8
	TxIntra   [32]int8
	Tx        [32]int8
	Partition [16]uint8
	UVMode    [32]uint8
	PalSz     [32]uint8
	UVPalSz   [32]uint8
}

func (c *BlockContext) Reset(keyframe bool, pass int) {
	fillUint8(c.Intra[:], boolToByte(keyframe))
	fillUint8(c.UVMode[:], dcPredMode)
	if keyframe {
		fillUint8(c.Mode[:], dcPredMode)
	}

	if pass == 2 {
		return
	}

	fillUint8(c.Partition[:], 0)
	fillUint8(c.Skip[:], 0)
	fillUint8(c.SkipMode[:], 0)
	fillUint8(c.SegPred[:], 0)
	fillUint8(c.PalSz[:], 0)
	fillUint8(c.UVPalSz[:], 0)
	fillUint8(c.LCoef[:], 0x40)
	fillUint8(c.CCoef[0][:], 0x40)
	fillUint8(c.CCoef[1][:], 0x40)
	fillUint8(c.CompType[:], 0)
	fillUint8(c.Filter[0][:], switchableFilterUnset)
	fillUint8(c.Filter[1][:], switchableFilterUnset)
	fillInt8(c.TxIntra[:], -1)
	fillInt8(c.Tx[:], txSize64x64)

	if !keyframe {
		fillInt8(c.Ref[0][:], -1)
		fillInt8(c.Ref[1][:], -1)
		fillUint8(c.Mode[:], nearestMVMode)
	}
}

func GetIntraCtx(above, left *BlockContext, yb4, xb4 int, haveTop, haveLeft bool) int {
	if haveLeft {
		if haveTop {
			ctx := int(left.Intra[yb4]) + int(above.Intra[xb4])
			if ctx == 2 {
				return 3
			}
			return ctx
		}
		return int(left.Intra[yb4]) * 2
	}
	if haveTop {
		return int(above.Intra[xb4]) * 2
	}
	return 0
}

func GetPartitionCtx(above, left *BlockContext, level BlockLevel, yb8, xb8 int) int {
	shift := 4 - int(level)
	return int((above.Partition[xb8]>>shift)&1) + int(((left.Partition[yb8]>>shift)&1)<<1)
}

func GetTxCtx(above, left *BlockContext, maxTX TxfmInfo, yb4, xb4 int) int {
	return boolToInt(left.TxIntra[yb4] >= int8(maxTX.LH)) + boolToInt(above.TxIntra[xb4] >= int8(maxTX.LW))
}

func GetInterTxCtx(above, left *BlockContext, maxTX TxfmInfo, yb4, xb4 int) int {
	return boolToInt(left.Tx[yb4] < int8(maxTX.LH)) + boolToInt(above.Tx[xb4] < int8(maxTX.LW))
}

func GatherLeftPartitionProb(in []uint16, level BlockLevel) uint32 {
	out := int(in[PartitionH-1]) - int(in[PartitionH])
	out += int(in[PartitionSplit-1]) - int(in[PartitionTLeftSplit])
	if level != BlockLevel128x128 {
		out += int(in[PartitionH4-1]) - int(in[PartitionH4])
	}
	return uint32(out)
}

func GatherTopPartitionProb(in []uint16, level BlockLevel) uint32 {
	out := int(in[PartitionV-1]) - int(in[PartitionTTopSplit])
	out += int(in[PartitionTLeftSplit-1])
	if level != BlockLevel128x128 {
		out += int(in[PartitionV4-1]) - int(in[PartitionTRightSplit])
	}
	return uint32(out)
}

func boolToByte(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func fillUint8(dst []uint8, v uint8) {
	for i := range dst {
		dst[i] = v
	}
}

func fillInt8(dst []int8, v int8) {
	for i := range dst {
		dst[i] = v
	}
}
