package decoder

import "github.com/codemodify/av1go-codex/pkg/av1"

type TxfmSize uint8

const (
	TX4X4 TxfmSize = iota
	TX8X8
	TX16X16
	TX32X32
	TX64X64
	RTX4X8
	RTX8X4
	RTX8X16
	RTX16X8
	RTX16X32
	RTX32X16
	RTX32X64
	RTX64X32
	RTX4X16
	RTX16X4
	RTX8X32
	RTX32X8
	RTX16X64
	RTX64X16

	numRectTxfmSizes
)

type TxfmInfo struct {
	W4  uint8
	H4  uint8
	LW  uint8
	LH  uint8
	Min uint8
	Max uint8
	Sub TxfmSize
	Ctx uint8
}

var txfmInfo = [numRectTxfmSizes]TxfmInfo{
	TX4X4:    {W4: 1, H4: 1, LW: 0, LH: 0, Min: 0, Max: 0, Ctx: 0},
	TX8X8:    {W4: 2, H4: 2, LW: 1, LH: 1, Min: 1, Max: 1, Sub: TX4X4, Ctx: 1},
	TX16X16:  {W4: 4, H4: 4, LW: 2, LH: 2, Min: 2, Max: 2, Sub: TX8X8, Ctx: 2},
	TX32X32:  {W4: 8, H4: 8, LW: 3, LH: 3, Min: 3, Max: 3, Sub: TX16X16, Ctx: 3},
	TX64X64:  {W4: 16, H4: 16, LW: 4, LH: 4, Min: 4, Max: 4, Sub: TX32X32, Ctx: 4},
	RTX4X8:   {W4: 1, H4: 2, LW: 0, LH: 1, Min: 0, Max: 1, Sub: TX4X4, Ctx: 1},
	RTX8X4:   {W4: 2, H4: 1, LW: 1, LH: 0, Min: 0, Max: 1, Sub: TX4X4, Ctx: 1},
	RTX8X16:  {W4: 2, H4: 4, LW: 1, LH: 2, Min: 1, Max: 2, Sub: TX8X8, Ctx: 2},
	RTX16X8:  {W4: 4, H4: 2, LW: 2, LH: 1, Min: 1, Max: 2, Sub: TX8X8, Ctx: 2},
	RTX16X32: {W4: 4, H4: 8, LW: 2, LH: 3, Min: 2, Max: 3, Sub: TX16X16, Ctx: 3},
	RTX32X16: {W4: 8, H4: 4, LW: 3, LH: 2, Min: 2, Max: 3, Sub: TX16X16, Ctx: 3},
	RTX32X64: {W4: 8, H4: 16, LW: 3, LH: 4, Min: 3, Max: 4, Sub: TX32X32, Ctx: 4},
	RTX64X32: {W4: 16, H4: 8, LW: 4, LH: 3, Min: 3, Max: 4, Sub: TX32X32, Ctx: 4},
	RTX4X16:  {W4: 1, H4: 4, LW: 0, LH: 2, Min: 0, Max: 2, Sub: RTX4X8, Ctx: 1},
	RTX16X4:  {W4: 4, H4: 1, LW: 2, LH: 0, Min: 0, Max: 2, Sub: RTX8X4, Ctx: 1},
	RTX8X32:  {W4: 2, H4: 8, LW: 1, LH: 3, Min: 1, Max: 3, Sub: RTX8X16, Ctx: 2},
	RTX32X8:  {W4: 8, H4: 2, LW: 3, LH: 1, Min: 1, Max: 3, Sub: RTX16X8, Ctx: 2},
	RTX16X64: {W4: 4, H4: 16, LW: 2, LH: 4, Min: 2, Max: 4, Sub: RTX16X32, Ctx: 3},
	RTX64X16: {W4: 16, H4: 4, LW: 4, LH: 2, Min: 2, Max: 4, Sub: RTX32X16, Ctx: 3},
}

func TxfmInfoFor(tx TxfmSize) TxfmInfo {
	return txfmInfo[tx]
}

func MaxTXSizeForBlock(bs BlockSize, chroma av1.ChromaSubsampling) TxfmSize {
	layout := 0
	switch chroma {
	case av1.Chroma420:
		layout = 1
	case av1.Chroma422:
		layout = 2
	case av1.Chroma444:
		layout = 3
	}
	return TxfmSize(generatedMaxTXSizeForBlock[bs][layout])
}
