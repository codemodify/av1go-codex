package displaycolor

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestCanUseSimple8BitLUTRejectsAdvancedDisplayTransforms(t *testing.T) {
	sdr := av1.ColorConfig{
		BitDepth:           8,
		SubsamplingX:       true,
		SubsamplingY:       true,
		MatrixCoefficients: 1,
	}
	if !CanUseSimple8BitLUT(sdr) {
		t.Fatal("expected simple 8-bit SDR config to use LUT")
	}

	hdr := sdr
	hdr.MatrixCoefficients = 9
	hdr.ColorPrimaries = primariesBT2020
	hdr.TransferCharacteristics = transferST2084
	if CanUseSimple8BitLUT(hdr) {
		t.Fatal("expected BT.2020 PQ config to bypass simple LUT")
	}

	identity444 := av1.ColorConfig{
		BitDepth:           8,
		SubsamplingX:       false,
		SubsamplingY:       false,
		MatrixCoefficients: 0,
	}
	if CanUseSimple8BitLUT(identity444) {
		t.Fatal("expected identity 4:4:4 config to bypass YUV LUT")
	}
}

func TestYUVToDisplayRGBAppliesHDRPreviewTransform(t *testing.T) {
	cfg := av1.ColorConfig{
		BitDepth:                10,
		SubsamplingX:            true,
		SubsamplingY:            true,
		MatrixCoefficients:      9,
		ColorPrimaries:          primariesBT2020,
		TransferCharacteristics: transferST2084,
	}
	yv, uv, vv := 840, 320, 896

	rawR, rawG, rawB := yuvToRGBSignal(cfg, yv, uv, vv)
	raw8 := [3]uint8{floatToByte(clamp01(rawR)), floatToByte(clamp01(rawG)), floatToByte(clamp01(rawB))}
	got := [3]uint8{}
	got[0], got[1], got[2] = YUVToDisplayRGB(cfg, yv, uv, vv)

	if got == raw8 {
		t.Fatalf("HDR preview transform left signal unchanged: got %v", got)
	}
	if got[0] == got[1] && got[1] == got[2] {
		t.Fatalf("expected transformed HDR sample to retain visible color separation, got %v", got)
	}
}
