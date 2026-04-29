package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestInitDequantTablesNoSegmentation(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 8},
	}
	hdr := &obu.FrameHeader{}
	hdr.Quantization.YDCDelta = 1
	hdr.Quantization.UDCDelta = -1
	hdr.Quantization.UACDelta = 2
	hdr.Quantization.VDCDelta = 3
	hdr.Quantization.VACDelta = -2

	dq := InitDequantTables(seq, hdr, 80)
	if got, want := dq[0][0][0], generatedDQTable[0][81][0]; got != want {
		t.Fatalf("Y dc = %d, want %d", got, want)
	}
	if got, want := dq[0][0][1], generatedDQTable[0][80][1]; got != want {
		t.Fatalf("Y ac = %d, want %d", got, want)
	}
	if got, want := dq[0][1][0], generatedDQTable[0][79][0]; got != want {
		t.Fatalf("U dc = %d, want %d", got, want)
	}
	if got, want := dq[0][2][1], generatedDQTable[0][78][1]; got != want {
		t.Fatalf("V ac = %d, want %d", got, want)
	}
	if dq[7] != dq[0] {
		t.Fatal("expected replicated dequant tables when segmentation is disabled")
	}
}

func TestInitDequantTablesSegmentationDeltaQ(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 10},
	}
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.Enabled = true
	hdr.Segmentation.Data[3].DeltaQ = 5

	dq := InitDequantTables(seq, hdr, 20)
	if got, want := dq[3][0][1], generatedDQTable[1][25][1]; got != want {
		t.Fatalf("seg3 Y ac = %d, want %d", got, want)
	}
	if got, want := dq[0][0][1], generatedDQTable[1][20][1]; got != want {
		t.Fatalf("seg0 Y ac = %d, want %d", got, want)
	}
}
