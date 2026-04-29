package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestIntraTUNodeEdgeAvailabilityUsesInternalTransformNeighbors(t *testing.T) {
	blockWidth4 := 8
	blockHeight4 := 8
	tx := TX16X16
	blockEdge := intraEdgeAvailability{
		topHasRight:   false,
		leftHasBottom: false,
	}

	tests := []struct {
		name string
		x4   int
		y4   int
		want intraEdgeAvailability
	}{
		{
			name: "top left TU can see current block top and left edge extensions",
			x4:   0,
			y4:   0,
			want: intraEdgeAvailability{topHasRight: true, leftHasBottom: true},
		},
		{
			name: "top right TU cannot see internal left-bottom",
			x4:   4,
			y4:   0,
			want: intraEdgeAvailability{topHasRight: false, leftHasBottom: false},
		},
		{
			name: "bottom left TU keeps internal top-right",
			x4:   0,
			y4:   4,
			want: intraEdgeAvailability{topHasRight: true, leftHasBottom: false},
		},
		{
			name: "bottom right TU has no internal extensions",
			x4:   4,
			y4:   4,
			want: intraEdgeAvailability{topHasRight: false, leftHasBottom: false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, tc.x4, tc.y4, tx, blockEdge)
			if got != tc.want {
				t.Fatalf("intraTUNodeEdgeAvailability(%d,%d) = %+v, want %+v", tc.x4, tc.y4, got, tc.want)
			}
		})
	}
}

func TestIntraTUNodeEdgeAvailabilityOnlyUsesExternalTopRightOnTopRow(t *testing.T) {
	blockEdge := intraEdgeAvailability{
		topHasRight:   true,
		leftHasBottom: true,
	}

	if got := intraTUNodeEdgeAvailability(2, 2, 1, 1, TX4X4, blockEdge); got.topHasRight {
		t.Fatalf("bottom-right TU unexpectedly inherited external top-right: %+v", got)
	}
	if got := intraTUNodeEdgeAvailability(2, 2, 1, 1, TX4X4, blockEdge); got.leftHasBottom {
		t.Fatalf("bottom-right TU unexpectedly inherited external left-bottom: %+v", got)
	}
	if got := intraTUNodeEdgeAvailability(2, 2, 1, 0, TX4X4, blockEdge); !got.topHasRight {
		t.Fatalf("top-right TU should inherit external top-right: %+v", got)
	}
	if got := intraTUNodeEdgeAvailability(2, 2, 1, 0, TX4X4, blockEdge); got.leftHasBottom {
		t.Fatalf("top-right TU unexpectedly inherited left-bottom: %+v", got)
	}
	if got := intraTUNodeEdgeAvailability(2, 2, 0, 1, TX4X4, blockEdge); !got.leftHasBottom {
		t.Fatalf("bottom-left TU should inherit external left-bottom: %+v", got)
	}
}

func TestRootIntraEdgeAvailabilityForBlockUsesTileBounds(t *testing.T) {
	tile := TileGeometry{Start4X: 32, End4X: 64, Start4Y: 0, End4Y: 64}

	leftTopRoot := BlockGeometry{Start4X: 32, End4X: 64, Start4Y: 0, End4Y: 32}
	if got := rootIntraEdgeAvailabilityForBlock(tile, leftTopRoot); got != intraEdgeAvailabilityWithAll(false, false) {
		t.Fatalf("rootIntraEdgeAvailabilityForBlock(leftTopRoot) = %+v", got)
	}

	lastRoot := BlockGeometry{Start4X: 32, End4X: 64, Start4Y: 32, End4Y: 64}
	if got := rootIntraEdgeAvailabilityForBlock(tile, lastRoot); got != intraEdgeAvailabilityWithAll(false, false) {
		t.Fatalf("rootIntraEdgeAvailabilityForBlock(lastRoot) = %+v", got)
	}

	interiorRoot := BlockGeometry{Start4X: 16, End4X: 32, Start4Y: 16, End4Y: 32}
	fullTile := TileGeometry{Start4X: 0, End4X: 64, Start4Y: 0, End4Y: 64}
	if got := rootIntraEdgeAvailabilityForBlock(fullTile, interiorRoot); got != intraEdgeAvailabilityWithAll(true, false) {
		t.Fatalf("rootIntraEdgeAvailabilityForBlock(interiorRoot) = %+v", got)
	}
}

func TestPartitionSplitBottomLeftGetsDecodedTopRightSibling(t *testing.T) {
	children, ok := partitionIntraEdgeChildren(BlockLevel32x32, PartitionSplit, intraEdgeAvailability{})
	if !ok {
		t.Fatal("partitionIntraEdgeChildren returned ok=false")
	}
	if !children[2].topHasRight {
		t.Fatalf("bottom-left split child should see decoded top-right sibling: %+v", children[2])
	}
}

func TestPartition8x8HPreservesChroma420TopRightForBottomChild(t *testing.T) {
	parent := intraEdgeAvailabilityWithAll(true, false)
	children, ok := partitionIntraEdgeChildren(BlockLevel8x8, PartitionH, parent)
	if !ok {
		t.Fatal("partitionIntraEdgeChildren returned ok=false")
	}
	lumaEdge := children[1]
	if lumaEdge.topHasRight {
		t.Fatalf("bottom 8x4 luma child should not keep I444 top-right: %+v", lumaEdge)
	}
	chromaEdge := children[1].forChromaLayout(av1.Chroma420)
	if !chromaEdge.topHasRight {
		t.Fatalf("bottom 8x4 4:2:0 chroma child should keep I420 top-right: %+v", children[1])
	}
}

func TestTileIntraNeighborAvailabilityUsesTileOrigins(t *testing.T) {
	tile := TileGeometry{Start4X: 32, End4X: 64, Start4Y: 16, End4Y: 64}

	if haveTop, haveLeft := tileIntraNeighborAvailability(tile, BlockGeometry{Start4X: 32, Start4Y: 16}); haveTop || haveLeft {
		t.Fatalf("tileIntraNeighborAvailability(root) = (%v,%v), want (false,false)", haveTop, haveLeft)
	}
	if haveTop, haveLeft := tileIntraNeighborAvailability(tile, BlockGeometry{Start4X: 36, Start4Y: 16}); haveTop || !haveLeft {
		t.Fatalf("tileIntraNeighborAvailability(top edge) = (%v,%v), want (false,true)", haveTop, haveLeft)
	}
	if haveTop, haveLeft := tileIntraNeighborAvailability(tile, BlockGeometry{Start4X: 32, Start4Y: 20}); !haveTop || haveLeft {
		t.Fatalf("tileIntraNeighborAvailability(left edge) = (%v,%v), want (true,false)", haveTop, haveLeft)
	}
	if haveTop, haveLeft := tileIntraNeighborAvailability(tile, BlockGeometry{Start4X: 36, Start4Y: 20}); !haveTop || !haveLeft {
		t.Fatalf("tileIntraNeighborAvailability(interior) = (%v,%v), want (true,true)", haveTop, haveLeft)
	}
}

func TestFrameIntraNeighborAvailabilityCrossesTileOrigins(t *testing.T) {
	g := BlockGeometry{Start4X: 36, Start4Y: 20}
	if haveTop, haveLeft := frameIntraNeighborAvailability(g); !haveTop || !haveLeft {
		t.Fatalf("frameIntraNeighborAvailability(interior frame block) = (%v,%v), want (true,true)", haveTop, haveLeft)
	}

	g = BlockGeometry{Start4X: 0, Start4Y: 20}
	if haveTop, haveLeft := frameIntraNeighborAvailability(g); !haveTop || haveLeft {
		t.Fatalf("frameIntraNeighborAvailability(frame-left block) = (%v,%v), want (true,false)", haveTop, haveLeft)
	}
}
