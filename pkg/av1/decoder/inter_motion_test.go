package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestReadInterMotionModeDefaultsToTranslationWhenIneligible(t *testing.T) {
	hdr := &obu.FrameHeader{SwitchableMotion: true}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{Size: Block4x4, Start4X: 0, Start4Y: 0, End4X: 1, End4Y: 1},
		Size:     Block4x4,
		Ref0:     0,
		Mode:     InterPredNearest,
	}
	mode, err := ReadInterMotionMode(hdr, cdf, &above, &left, nil, syntax.Geometry, syntax, false, false, fullIntraEdgeAvailability, &scriptedIntraEntropy{})
	if err != nil {
		t.Fatalf("ReadInterMotionMode: %v", err)
	}
	if mode != interMotionTranslation {
		t.Fatalf("motion mode = %d, want translation", mode)
	}
}

func TestReadInterMotionModeReadsOBMCBool(t *testing.T) {
	hdr := &obu.FrameHeader{
		SwitchableMotion: true,
		ForceIntegerMV:   true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{adapt: []uint32{1}}
	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{Size: Block16x16, Start4X: 2, Start4Y: 2, End4X: 6, End4Y: 6},
		Size:     Block16x16,
		Ref0:     0,
		Mode:     InterPredNearest,
	}
	mode, err := ReadInterMotionMode(hdr, cdf, &above, &left, nil, syntax.Geometry, syntax, true, true, fullIntraEdgeAvailability, dec)
	if err != nil {
		t.Fatalf("ReadInterMotionMode: %v", err)
	}
	if mode != interMotionOBMC {
		t.Fatalf("motion mode = %d, want obmc", mode)
	}
}

func TestReadInterMotionModeReadsForSkippedInterBlock(t *testing.T) {
	hdr := &obu.FrameHeader{
		SwitchableMotion: true,
		ForceIntegerMV:   true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{adapt: []uint32{1}}
	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{Size: Block16x16, Start4X: 2, Start4Y: 2, End4X: 6, End4Y: 6},
		Size:     Block16x16,
		Skip:     true,
		Ref0:     0,
		Mode:     InterPredNearest,
	}
	mode, err := ReadInterMotionMode(hdr, cdf, &above, &left, nil, syntax.Geometry, syntax, true, true, fullIntraEdgeAvailability, dec)
	if err != nil {
		t.Fatalf("ReadInterMotionMode: %v", err)
	}
	if mode != interMotionOBMC {
		t.Fatalf("motion mode = %d, want obmc", mode)
	}
}

func TestReadInterMotionModeReadsWarpSymbolWhenAllowed(t *testing.T) {
	hdr := &obu.FrameHeader{
		SwitchableMotion: true,
		WarpMotion:       true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	mvTile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 16, End4Y: 16})
	mvTile.SplatSingleRef(
		BlockGeometry{Size: Block8x8, Start4X: 2, Start4Y: 1, End4X: 4, End4Y: 3},
		0, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair(),
	)
	dec := &scriptedIntraEntropy{symbols: []uint32{uint32(interMotionWarp)}}
	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{Size: Block16x16, Start4X: 2, Start4Y: 2, End4X: 6, End4Y: 6},
		Size:     Block16x16,
		Ref0:     0,
		Mode:     InterPredNearest,
	}
	mode, err := ReadInterMotionMode(hdr, cdf, &above, &left, mvTile, syntax.Geometry, syntax, true, true, fullIntraEdgeAvailability, dec)
	if err != nil {
		t.Fatalf("ReadInterMotionMode: %v", err)
	}
	if mode != interMotionWarp {
		t.Fatalf("motion mode = %d, want warp", mode)
	}
}

func TestReadInterMotionModeIgnoresInterIntraNeighborForLocalWarp(t *testing.T) {
	hdr := &obu.FrameHeader{
		SwitchableMotion: true,
		WarpMotion:       true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	mvTile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 16, End4Y: 16})
	mvTile.SplatSingleRefWithInterIntra(
		BlockGeometry{Size: Block16x8, Start4X: 4, Start4Y: 2, End4X: 8, End4Y: 4},
		0, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair(), true,
	)
	dec := &scriptedIntraEntropy{adapt: []uint32{1}, symbols: []uint32{uint32(interMotionWarp)}}
	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{Size: Block16x16, Start4X: 4, Start4Y: 4, End4X: 8, End4Y: 8},
		Size:     Block16x16,
		Ref0:     0,
		Mode:     InterPredNearest,
	}
	mode, err := ReadInterMotionMode(hdr, cdf, &above, &left, mvTile, syntax.Geometry, syntax, true, true, fullIntraEdgeAvailability, dec)
	if err != nil {
		t.Fatalf("ReadInterMotionMode: %v", err)
	}
	if mode != interMotionOBMC {
		t.Fatalf("motion mode = %d, want obmc", mode)
	}
	if len(dec.symbols) != 1 {
		t.Fatalf("DecodeSymbolAdapt was consumed for an inter-intra neighbor")
	}
}

func TestReadInterMotionModeScansSub8x8NeighborsForWarpEligibility(t *testing.T) {
	hdr := &obu.FrameHeader{
		SwitchableMotion: true,
		WarpMotion:       true,
	}
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	syntax := InterBlockSyntax{
		Geometry: BlockGeometry{Size: Block8x8, Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4},
		Size:     Block8x8,
		Ref0:     1,
		Mode:     InterPredNearest,
	}

	tests := []struct {
		name  string
		setup func(*SpatialMVTile)
	}{
		{
			name: "left edge lower 8x4",
			setup: func(tile *SpatialMVTile) {
				tile.SplatSingleRef(BlockGeometry{Size: Block8x4, Start4X: 0, Start4Y: 2, End4X: 2, End4Y: 3}, 6, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
				tile.SplatSingleRef(BlockGeometry{Size: Block8x4, Start4X: 0, Start4Y: 3, End4X: 2, End4Y: 4}, 1, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
			},
		},
		{
			name: "top edge right 4x8",
			setup: func(tile *SpatialMVTile) {
				tile.SplatSingleRef(BlockGeometry{Size: Block4x8, Start4X: 2, Start4Y: 0, End4X: 3, End4Y: 2}, 6, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
				tile.SplatSingleRef(BlockGeometry{Size: Block4x8, Start4X: 3, Start4Y: 0, End4X: 4, End4Y: 2}, 1, 0, MotionVector{}, InterPredNearest, defaultInterFilterPair())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mvTile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
			tt.setup(mvTile)
			dec := &scriptedIntraEntropy{
				symbols: []uint32{uint32(interMotionOBMC)},
				adapt:   []uint32{0},
			}
			mode, err := ReadInterMotionMode(hdr, cdf, &above, &left, mvTile, syntax.Geometry, syntax, true, true, fullIntraEdgeAvailability, dec)
			if err != nil {
				t.Fatalf("ReadInterMotionMode: %v", err)
			}
			if mode != interMotionOBMC {
				t.Fatalf("motion mode = %d, want obmc from motion-mode symbol", mode)
			}
			if len(dec.symbols) != 0 {
				t.Fatalf("DecodeSymbolAdapt was not consumed for warp-eligible sub8x8 neighbor")
			}
			if len(dec.adapt) != 1 {
				t.Fatalf("DecodeBoolAdapt was consumed; warp eligibility should use motion-mode symbols")
			}
		})
	}
}
