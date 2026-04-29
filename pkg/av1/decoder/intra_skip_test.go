package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func fillBlockBoundarySamples8(plane []byte, stride, x, y, width, height int, topValue, leftValue byte) {
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

func requireBlockFilled8(t *testing.T, plane []byte, stride, x, y, width, height int, want byte) {
	t.Helper()
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			if got := plane[(y+row)*stride+x+col]; got != want {
				t.Fatalf("plane[%d,%d] = %d, want %d", x+col, y+row, got, want)
			}
		}
	}
}

func TestDecodeAndReconstructKeyIntraDCLumaBlockAt8EdgesWithNeighborsHonorsExplicitMissingNeighbors(t *testing.T) {
	seq := av1.SequenceHeader{ColorConfig: av1.ColorConfig{Monochrome: true}}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(0)

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	const stride = 16
	plane := make([]byte, stride*16)
	fillBlockBoundarySamples8(plane, stride, 8, 8, 4, 4, 24, 216)

	dec := &scriptedKeyIntraEntropy{
		scriptedIntraEntropy: scriptedIntraEntropy{
			symbols: []uint32{uint32(IntraPredDC)},
		},
		scriptedCoefficientEntropy: scriptedCoefficientEntropy{
			adapt: []uint32{1},
		},
	}

	_, _, err := decodeAndReconstructKeyIntraDCLumaBlockAt8EdgesWithNeighbors(
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
		dec,
		false,
		false,
		true,
		true,
	)
	if err != nil {
		t.Fatalf("decodeAndReconstructKeyIntraDCLumaBlockAt8EdgesWithNeighbors: %v", err)
	}

	requireBlockFilled8(t, plane, stride, 8, 8, 4, 4, 128)
}

func TestReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdgesHonorsExplicitMissingNeighborsWhenNotSkipped(t *testing.T) {
	seq := av1.SequenceHeader{ColorConfig: av1.ColorConfig{Monochrome: true}}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}
	modeCDF := NewDefaultModeCDF()
	coefCDF := NewDefaultCoefCDF(0)

	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	const stride = 16
	plane := make([]byte, stride*16)
	fillBlockBoundarySamples8(plane, stride, 8, 8, 4, 4, 32, 200)

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

	if err := ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges(
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
		false,
		dec,
		false,
		false,
		true,
		true,
	); err != nil {
		t.Fatalf("ReconstructParsedIntraLumaBlockAt8WithSkipNeighborEdges: %v", err)
	}

	requireBlockFilled8(t, plane, stride, 8, 8, 4, 4, 128)
}

func TestReconstructParsedIntraChromaBlockAt8WithSkipNeighborEdgesHonorsExplicitMissingNeighbors(t *testing.T) {
	seq := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	hdr := &obu.FrameHeader{FrameType: obu.FrameTypeKey}

	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)

	luma := make([]byte, 16*16)
	const chromaStride = 8
	uPlane := make([]byte, chromaStride*8)
	vPlane := make([]byte, chromaStride*8)
	fillBlockBoundarySamples8(uPlane, chromaStride, 4, 4, 4, 4, 40, 180)
	fillBlockBoundarySamples8(vPlane, chromaStride, 4, 4, 4, 4, 72, 220)

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

	if err := reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges(
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
		nil,
		true,
		false,
		false,
		true,
		true,
	); err != nil {
		t.Fatalf("reconstructParsedIntraChromaBlockAt8WithSkipNeighborEdges: %v", err)
	}

	requireBlockFilled8(t, uPlane, chromaStride, 4, 4, 4, 4, 128)
	requireBlockFilled8(t, vPlane, chromaStride, 4, 4, 4, 4, 128)
}
