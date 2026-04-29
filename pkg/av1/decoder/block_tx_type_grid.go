package decoder

import "github.com/codemodify/av1go-codex/pkg/av1"

const maxBlockTxTypeEntries = 32 * 32

func blockTxTypeGridIndex(g BlockGeometry, x4, y4 int) int {
	return (y4-g.Start4Y)*g.Width4() + (x4 - g.Start4X)
}

func setBlockTxTypeRange(grid []TxType, block, tu BlockGeometry, txType TxType) {
	for y4 := tu.Start4Y; y4 < tu.End4Y; y4++ {
		row := (y4 - block.Start4Y) * block.Width4()
		for x4 := tu.Start4X; x4 < tu.End4X; x4++ {
			grid[row+x4-block.Start4X] = txType
		}
	}
}

func blockTxTypeAt(grid []TxType, block BlockGeometry, x4, y4 int) TxType {
	if x4 < block.Start4X || x4 >= block.End4X || y4 < block.Start4Y || y4 >= block.End4Y {
		return TxTypeDCTDCT
	}
	return grid[blockTxTypeGridIndex(block, x4, y4)]
}

func blockTxTypeForChromaTU(grid []TxType, lumaBlock, chromaBlock, chromaTU BlockGeometry, layout av1.ChromaSubsampling) TxType {
	ssHor := 0
	ssVer := 0
	if layout != av1.Chroma444 {
		ssHor = 1
	}
	if layout == av1.Chroma420 {
		ssVer = 1
	}
	x4 := lumaBlock.Start4X + ((chromaTU.Start4X - chromaBlock.Start4X) << ssHor)
	y4 := lumaBlock.Start4Y + ((chromaTU.Start4Y - chromaBlock.Start4Y) << ssVer)
	return blockTxTypeAt(grid, lumaBlock, x4, y4)
}
