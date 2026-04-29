package decoder

import (
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedCoefficientEntropy struct {
	equi   []uint32
	adapt  []uint32
	symbol []uint32
	hitok  []uint32
}

func (s *scriptedCoefficientEntropy) DecodeBoolEqui() uint32 {
	if len(s.equi) == 0 {
		return 0
	}
	v := s.equi[0]
	s.equi = s.equi[1:]
	return v
}

func (s *scriptedCoefficientEntropy) DecodeBoolAdapt(cdf []uint16) uint32 {
	if len(s.adapt) == 0 {
		return 0
	}
	v := s.adapt[0]
	s.adapt = s.adapt[1:]
	return v
}

func (s *scriptedCoefficientEntropy) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	if len(s.symbol) == 0 {
		return 0
	}
	v := s.symbol[0]
	s.symbol = s.symbol[1:]
	return v
}

func (s *scriptedCoefficientEntropy) DecodeHiTok(cdf []uint16) uint32 {
	if len(s.hitok) == 0 {
		return 3
	}
	v := s.hitok[0]
	s.hitok = s.hitok[1:]
	return v
}

func TestDecodeLumaDCOnlyCoefficientSkip(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedCoefficientEntropy{adapt: []uint32{1}}

	dc, resCtx, skip, err := DecodeLumaDCOnlyCoefficient(Block8x8, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, TX4X4, 0, NewDefaultCoefCDF(0), DequantTables{}, &above, &left, dec)
	if err != nil {
		t.Fatalf("DecodeLumaDCOnlyCoefficient: %v", err)
	}
	if !skip {
		t.Fatal("expected skip")
	}
	if dc != 0 {
		t.Fatalf("dc = %d, want 0", dc)
	}
	if resCtx != 0x40 {
		t.Fatalf("resCtx = %d, want 0x40", resCtx)
	}
}

func TestDecodeLumaDCOnlyCoefficientPositive(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	hdr := &obu.FrameHeader{}
	hdr.Quantization.YAC = 8
	dq := InitDequantTables(av1.SequenceHeader{ColorConfig: av1.ColorConfig{BitDepth: 8}}, hdr, int(hdr.Quantization.YAC))
	dec := &scriptedCoefficientEntropy{
		adapt:  []uint32{0, 0},
		symbol: []uint32{0, 0},
	}

	dc, resCtx, skip, err := DecodeLumaDCOnlyCoefficient(Block8x8, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, TX4X4, 0, NewDefaultCoefCDF(hdr.Quantization.YAC), dq, &above, &left, dec)
	if err != nil {
		t.Fatalf("DecodeLumaDCOnlyCoefficient: %v", err)
	}
	if skip {
		t.Fatal("unexpected skip")
	}
	if want := int32(dq[0][0][0]); dc != want {
		t.Fatalf("dc = %d, want %d", dc, want)
	}
	if resCtx != 129 {
		t.Fatalf("resCtx = %d, want 129", resCtx)
	}
}

func TestDecodeLumaDCOnlyCoefficientNegative(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	hdr := &obu.FrameHeader{}
	hdr.Quantization.YAC = 8
	dq := InitDequantTables(av1.SequenceHeader{ColorConfig: av1.ColorConfig{BitDepth: 8}}, hdr, int(hdr.Quantization.YAC))
	dec := &scriptedCoefficientEntropy{
		adapt:  []uint32{0, 1},
		symbol: []uint32{0, 0},
	}

	dc, resCtx, _, err := DecodeLumaDCOnlyCoefficient(Block8x8, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, TX4X4, 0, NewDefaultCoefCDF(hdr.Quantization.YAC), dq, &above, &left, dec)
	if err != nil {
		t.Fatalf("DecodeLumaDCOnlyCoefficient: %v", err)
	}
	if want := -int32(dq[0][0][0]); dc != want {
		t.Fatalf("dc = %d, want %d", dc, want)
	}
	if resCtx != 1 {
		t.Fatalf("resCtx = %d, want 1", resCtx)
	}
}

func TestDecodeLumaDCOnlyCoefficientRejectsAC(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	dec := &scriptedCoefficientEntropy{
		adapt:  []uint32{0},
		symbol: []uint32{1},
	}

	_, _, _, err := DecodeLumaDCOnlyCoefficient(Block8x8, BlockGeometry{
		Size:    Block8x8,
		Start4X: 0,
		Start4Y: 0,
		End4X:   2,
		End4Y:   2,
	}, TX4X4, 0, NewDefaultCoefCDF(0), DequantTables{}, &above, &left, dec)
	if err == nil || !strings.Contains(err.Error(), "AC coefficients") {
		t.Fatalf("expected AC unsupported error, got %v", err)
	}
}

func TestApplyLumaResidualContext(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)
	err := ApplyLumaResidualContext(&above, &left, BlockGeometry{
		Size:    Block16x16,
		Start4X: 4,
		Start4Y: 8,
		End4X:   8,
		End4Y:   12,
	}, TX8X8, 129)
	if err != nil {
		t.Fatalf("ApplyLumaResidualContext: %v", err)
	}
	for i := 4; i < 6; i++ {
		if got := above.LCoef[i]; got != 129 {
			t.Fatalf("above.LCoef[%d] = %d, want 129", i, got)
		}
	}
	for i := 8; i < 10; i++ {
		if got := left.LCoef[i]; got != 129 {
			t.Fatalf("left.LCoef[%d] = %d, want 129", i, got)
		}
	}
}
