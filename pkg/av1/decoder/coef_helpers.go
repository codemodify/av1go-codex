package decoder

import "github.com/codemodify/av1go-codex/pkg/av1"

type TxClass uint8

const (
	TxClass2D TxClass = iota
	TxClassH
	TxClassV
)

type TxType uint8

const (
	TxTypeDCTDCT TxType = iota
	TxTypeADSTDCT
	TxTypeDCTADST
	TxTypeADSTADST
	TxTypeFlipADSTDCT
	TxTypeDCTFlipADST
	TxTypeFlipADSTFlipADST
	TxTypeADSTFlipADST
	TxTypeFlipADSTADST
	TxTypeIdentity
	TxTypeVDCT
	TxTypeHDCT
	TxTypeVADST
	TxTypeHADST
	TxTypeVFlipADST
	TxTypeHFlipADST
	TxTypeWHTWHT

	numTxTypesPlusLL
)

var generatedSkipContext = [5][5]uint8{
	{1, 2, 2, 2, 3},
	{2, 4, 4, 4, 5},
	{2, 4, 4, 4, 5},
	{2, 4, 4, 4, 5},
	{3, 5, 5, 5, 6},
}

var generatedLoContextOffsets = [3][5][5]uint8{
	{
		{0, 1, 6, 6, 21},
		{1, 6, 6, 21, 21},
		{6, 6, 21, 21, 21},
		{6, 21, 21, 21, 21},
		{21, 21, 21, 21, 21},
	},
	{
		{0, 16, 6, 6, 21},
		{16, 16, 6, 21, 21},
		{16, 16, 21, 21, 21},
		{16, 16, 21, 21, 21},
		{16, 16, 21, 21, 21},
	},
	{
		{0, 11, 11, 11, 11},
		{11, 11, 11, 11, 11},
		{6, 6, 21, 21, 21},
		{6, 21, 21, 21, 21},
		{21, 21, 21, 21, 21},
	},
}

var generatedTXTypeClass = [numTxTypesPlusLL]TxClass{
	TxTypeDCTDCT:           TxClass2D,
	TxTypeADSTDCT:          TxClass2D,
	TxTypeDCTADST:          TxClass2D,
	TxTypeADSTADST:         TxClass2D,
	TxTypeFlipADSTDCT:      TxClass2D,
	TxTypeDCTFlipADST:      TxClass2D,
	TxTypeFlipADSTFlipADST: TxClass2D,
	TxTypeADSTFlipADST:     TxClass2D,
	TxTypeFlipADSTADST:     TxClass2D,
	TxTypeIdentity:         TxClass2D,
	TxTypeVDCT:             TxClassV,
	TxTypeHDCT:             TxClassH,
	TxTypeVADST:            TxClassV,
	TxTypeHADST:            TxClassH,
	TxTypeVFlipADST:        TxClassV,
	TxTypeHFlipADST:        TxClassH,
	TxTypeWHTWHT:           TxClass2D,
}

func decodeGolomb(next func() uint32) uint32 {
	var length uint32
	value := uint32(1)
	for next() == 0 && length < 32 {
		length++
	}
	for length > 0 {
		length--
		value = (value << 1) | next()
	}
	return value - 1
}

func GetSkipCtx(maxTX TxfmInfo, bs BlockSize, above, left []uint8, chroma bool, layout av1.ChromaSubsampling) int {
	return GetSkipCtxWithNeighbors(maxTX, bs, above, left, chroma, layout, true, true)
}

func GetSkipCtxWithNeighbors(maxTX TxfmInfo, bs BlockSize, above, left []uint8, chroma bool, layout av1.ChromaSubsampling, haveTop, haveLeft bool) int {
	width4 := 1 << maxTX.LW
	height4 := 1 << maxTX.LH
	if chroma {
		ssVer := layout == av1.Chroma420
		ssHor := layout != av1.Chroma444
		bw := int(blockDimensions[bs].Log2W4)
		bh := int(blockDimensions[bs].Log2H4)
		if ssHor && bw > 0 {
			bw--
		}
		if ssVer && bh > 0 {
			bh--
		}
		notOneBlk := bw > int(maxTX.LW) || bh > int(maxTX.LH)
		ca := anyNonDefaultResidual(above, width4)
		cl := anyNonDefaultResidual(left, height4)
		return 7 + boolToInt(notOneBlk)*3 + boolToInt(ca) + boolToInt(cl)
	}

	if int(blockDimensions[bs].Log2W4) == int(maxTX.LW) && int(blockDimensions[bs].Log2H4) == int(maxTX.LH) {
		return 0
	}
	la := minInt(orResidualMagnitude(above, width4), 4)
	ll := minInt(orResidualMagnitude(left, height4), 4)
	return int(generatedSkipContext[la][ll])
}

func GetDCSignCtx(tx TxfmSize, above, left []uint8) int {
	info := TxfmInfoFor(tx)
	s := sumDCSigns(above, 1<<info.LW) + sumDCSigns(left, 1<<info.LH)
	return boolToInt(s != 0) + boolToInt(s > 0)
}

func GetLoCtx(levels []uint8, txClass TxClass, tx TxfmSize, rc int) int {
	x, y, stride := coeffContextXY(tx, rc)
	sum := 0
	for _, off := range loCtxOffsets(txClass) {
		sum += minInt(int(coeffContextLevel(levels, stride, x+off[0], y+off[1])), 3)
	}
	ctx := minInt((sum+1)>>1, 4)
	if txClass == TxClass2D {
		if rc == 0 {
			return 0
		}
		offset := generatedLoContextOffsets[loContextOffsetGroup(tx)][minInt(y, 4)][minInt(x, 4)]
		return int(offset) + ctx
	}
	axis := x
	if txClass == TxClassV {
		axis = y
	}
	return 26 + minInt(axis, 2)*5 + ctx
}

func GetBRCtx(levels []uint8, txClass TxClass, tx TxfmSize, rc int) int {
	x, y, stride := coeffContextXY(tx, rc)
	sum := uint32(0)
	for _, off := range brCtxOffsets(txClass) {
		sum += uint32(encodeCoeffContextLevel(coeffContextLevel(levels, stride, x+off[0], y+off[1])))
	}
	mag := hiTokenContextOffset(sum & 0x3f)
	if rc == 0 {
		return mag
	}
	switch txClass {
	case TxClass2D:
		if y < 2 && x < 2 {
			return mag + 7
		}
	case TxClassH:
		if x == 0 {
			return mag + 7
		}
	case TxClassV:
		if y == 0 {
			return mag + 7
		}
	}
	return mag + 14
}

func encodeCoeffContextLevel(tok uint8) uint8 {
	if tok <= 3 {
		return tok * 0x41
	}
	return tok + (3 << 6)
}

func loContextOffsetGroup(tx TxfmSize) int {
	info := TxfmInfoFor(tx)
	switch {
	case info.W4 == info.H4:
		return 0
	case info.W4 > info.H4:
		return 1
	default:
		return 2
	}
}

func coeffContextXY(tx TxfmSize, rc int) (x, y, stride int) {
	info := TxfmInfoFor(tx)
	height := 4 * minInt(int(info.H4), 8)
	if height <= 0 {
		return 0, 0, 0
	}
	x = rc >> (2 + minInt(int(info.LH), 3))
	y = rc & (height - 1)
	stride = height + 4
	return x, y, stride
}

func coeffLevelsBufferSize(tx TxfmSize) int {
	info := TxfmInfoFor(tx)
	width := 4 * minInt(int(info.W4), 8)
	_, _, stride := coeffContextXY(tx, 0)
	return (width + 4) * stride
}

func coeffContextIndex(tx TxfmSize, rc int) int {
	x, y, stride := coeffContextXY(tx, rc)
	return x*stride + y
}

func coeffContextLevel(levels []uint8, stride, x, y int) uint8 {
	if x < 0 || y < 0 || stride <= 0 {
		return 0
	}
	idx := x*stride + y
	if idx < 0 || idx >= len(levels) {
		return 0
	}
	return levels[idx]
}

func loCtxOffsets(txClass TxClass) [][2]int {
	switch txClass {
	case TxClassH:
		return [][2]int{{1, 0}, {0, 1}, {2, 0}, {3, 0}, {4, 0}}
	case TxClassV:
		return [][2]int{{1, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}}
	default:
		return [][2]int{{0, 1}, {1, 0}, {2, 0}, {0, 2}, {1, 1}}
	}
}

func brCtxOffsets(txClass TxClass) [][2]int {
	switch txClass {
	case TxClassH:
		return [][2]int{{1, 0}, {0, 1}, {2, 0}}
	case TxClassV:
		return [][2]int{{1, 0}, {0, 1}, {0, 2}}
	default:
		return [][2]int{{0, 1}, {1, 0}, {1, 1}}
	}
}

func anyNonDefaultResidual(v []uint8, n int) bool {
	for i := 0; i < n && i < len(v); i++ {
		if v[i] != 0x40 {
			return true
		}
	}
	return false
}

func orResidualMagnitude(v []uint8, n int) int {
	mask := 0
	for i := 0; i < n && i < len(v); i++ {
		mask |= int(v[i] & 0x3f)
	}
	return mask
}

func sumDCSigns(v []uint8, n int) int {
	total := 0
	for i := 0; i < n && i < len(v); i++ {
		total += int(v[i] >> 6)
	}
	return total - n
}
