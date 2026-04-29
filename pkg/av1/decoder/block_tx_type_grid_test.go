package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestBlockTxTypeForChromaTUUsesOwningOddLumaColumn(t *testing.T) {
	luma := BlockGeometry{Start4X: 3, Start4Y: 62, End4X: 4, End4Y: 64}
	chroma := chromaGeometryForBlock(av1.Chroma420, luma)
	grid := make([]TxType, luma.Width4()*luma.Height4())
	setBlockTxTypeRange(grid, luma, luma, TxTypeVDCT)

	got := blockTxTypeForChromaTU(grid, luma, chroma, chroma, av1.Chroma420)
	if got != TxTypeVDCT {
		t.Fatalf("blockTxTypeForChromaTU = %d, want %d", got, TxTypeVDCT)
	}
}

func TestBlockTxTypeForChromaTUUsesChromaOffsetWithinLumaBlock(t *testing.T) {
	luma := BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 4, End4Y: 4}
	chroma := chromaGeometryForBlock(av1.Chroma420, luma)
	grid := make([]TxType, luma.Width4()*luma.Height4())
	setBlockTxTypeRange(grid, luma, BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4}, TxTypeHADST)

	tu := BlockGeometry{Start4X: chroma.Start4X + 1, Start4Y: chroma.Start4Y + 1, End4X: chroma.End4X, End4Y: chroma.End4Y}
	got := blockTxTypeForChromaTU(grid, luma, chroma, tu, av1.Chroma420)
	if got != TxTypeHADST {
		t.Fatalf("blockTxTypeForChromaTU = %d, want %d", got, TxTypeHADST)
	}
}
