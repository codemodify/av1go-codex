package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func fillBlockBoundarySamples16(plane []uint16, stride, x, y, width, height int, topValue, leftValue uint16) {
	if y > 0 {
		for i := 0; i < width; i++ {
			plane[(y-1)*stride+x+i] = topValue
		}
	}
	if x > 0 {
		for i := 0; i < height; i++ {
			plane[(y+i)*stride+x-1] = leftValue
		}
	}
}

func requireBlockFilled16(t *testing.T, plane []uint16, stride, x, y, width, height int, want uint16) {
	t.Helper()
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			if got := plane[(y+row)*stride+x+col]; got != want {
				t.Fatalf("plane[%d,%d] = %d, want %d", x+col, y+row, got, want)
			}
		}
	}
}

func TestDecodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighborsHonorsExplicitMissingNeighbors(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			Monochrome: true,
			BitDepth:   10,
		},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(0)

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	const stride = 16
	plane := make([]uint16, stride*16)
	fillBlockBoundarySamples16(plane, stride, 8, 8, 4, 4, 80, 900)

	dec := &scriptedKeyIntraEntropy{
		scriptedIntraEntropy: scriptedIntraEntropy{
			symbols: []uint32{uint32(IntraPredDC)},
		},
		scriptedCoefficientEntropy: scriptedCoefficientEntropy{
			adapt: []uint32{1},
		},
	}

	_, err := DecodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighbors(
		seq,
		hdr,
		modeCDF,
		coefCDF,
		DequantTables{},
		&above,
		&left,
		BlockGeometry{
			Size:    Block4x4,
			Start4X: 2,
			Start4Y: 2,
			End4X:   3,
			End4Y:   3,
		},
		0,
		plane,
		stride,
		16,
		16,
		2,
		2,
		10,
		dec,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("DecodeAndReconstructKeyIntraDCLumaBlockAt16WithNeighbors: %v", err)
	}

	requireBlockFilled16(t, plane, stride, 8, 8, 4, 4, 512)
}

func TestReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdgesHonorsExplicitMissingNeighborsWhenNotSkipped(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			Monochrome: true,
			BitDepth:   10,
		},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(0)

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	const stride = 16
	plane := make([]uint16, stride*16)
	fillBlockBoundarySamples16(plane, stride, 8, 8, 4, 4, 96, 840)

	syntax := IntraBlockSyntax{
		Geometry: BlockGeometry{
			Size:    Block4x4,
			Start4X: 2,
			Start4Y: 2,
			End4X:   3,
			End4Y:   3,
		},
		Size:    Block4x4,
		Segment: 0,
		YMode:   IntraPredDC,
		UVMode:  IntraPredDC,
		TX:      TX4X4,
		UVTX:    TX4X4,
	}
	dec := &scriptedKeyIntraEntropy{
		scriptedCoefficientEntropy: scriptedCoefficientEntropy{
			adapt: []uint32{1},
		},
	}

	if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
		seq,
		hdr,
		modeCDF,
		coefCDF,
		DequantTables{},
		&above,
		&left,
		syntax,
		plane,
		stride,
		16,
		16,
		2,
		2,
		10,
		false,
		dec,
		false,
		false,
		true,
		true,
	); err != nil {
		t.Fatalf("ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges: %v", err)
	}

	requireBlockFilled16(t, plane, stride, 8, 8, 4, 4, 512)
}

func TestReconstructParsedIntraChromaBlockAt16WithSkipNeighborEdgesHonorsExplicitMissingNeighbors(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			BitDepth:     10,
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}

	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)

	luma := make([]uint16, 16*16)
	const chromaStride = 8
	uPlane := make([]uint16, chromaStride*8)
	vPlane := make([]uint16, chromaStride*8)
	fillBlockBoundarySamples16(uPlane, chromaStride, 4, 4, 4, 4, 120, 820)
	fillBlockBoundarySamples16(vPlane, chromaStride, 4, 4, 4, 4, 160, 900)

	syntax := IntraBlockSyntax{
		Geometry: BlockGeometry{
			Size:    Block8x8,
			Start4X: 2,
			Start4Y: 2,
			End4X:   4,
			End4Y:   4,
		},
		Size:    Block8x8,
		Segment: 0,
		YMode:   IntraPredDC,
		UVMode:  IntraPredDC,
		TX:      TX4X4,
		UVTX:    TX4X4,
	}

	if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(
		seq,
		hdr,
		nil,
		DequantTables{},
		&above,
		&left,
		syntax,
		luma,
		16,
		16,
		16,
		uPlane,
		chromaStride,
		8,
		8,
		vPlane,
		chromaStride,
		8,
		8,
		10,
		true,
		nil,
		false,
		false,
		true,
		true,
	); err != nil {
		t.Fatalf("reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges: %v", err)
	}

	requireBlockFilled16(t, uPlane, chromaStride, 4, 4, 4, 4, 512)
	requireBlockFilled16(t, vPlane, chromaStride, 4, 4, 4, 4, 512)
}
