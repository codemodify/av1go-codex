package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestMaxTXSizeForBlock420(t *testing.T) {
	if got, want := MaxTXSizeForBlock(Block128x128, av1.Chroma420), TX32X32; got != want {
		t.Fatalf("MaxTXSizeForBlock(128x128,420) = %d, want %d", got, want)
	}
	if got, want := MaxTXSizeForBlock(Block32x16, av1.Chroma420), RTX16X8; got != want {
		t.Fatalf("MaxTXSizeForBlock(32x16,420) = %d, want %d", got, want)
	}
	if got, want := MaxTXSizeForBlock(Block8x8, av1.Chroma420), TX4X4; got != want {
		t.Fatalf("MaxTXSizeForBlock(8x8,420) = %d, want %d", got, want)
	}
}

func TestTxfmInfoFor(t *testing.T) {
	info := TxfmInfoFor(RTX16X8)
	if info.W4 != 4 || info.H4 != 2 || info.Sub != TX8X8 {
		t.Fatalf("RTX16X8 info = %+v", info)
	}
}
