package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestLoopFilterPlaneEdgesMarkBlock(t *testing.T) {
	edges := newLoopFilterPlaneEdges(32, 32)
	edges.markBlock(BlockGeometry{Start4X: 2, Start4Y: 1, End4X: 6, End4Y: 5})
	if !edges.hasVertical(2, 1) || !edges.hasVertical(6, 4) {
		t.Fatal("expected vertical block boundaries")
	}
	if !edges.hasHorizontal(2, 1) || !edges.hasHorizontal(5, 5) {
		t.Fatal("expected horizontal block boundaries")
	}
}

func TestMarkInterTransformEdgesUsesTransformUnits(t *testing.T) {
	edges := newLoopFilterEdgeMaps(32, 32, av1.Chroma420)
	lumaUnits := []TransformUnit{
		{Global: BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 4}},
		{Global: BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 4}},
	}
	chromaUnits := []BlockGeometry{
		{Start4X: 0, Start4Y: 0, End4X: 1, End4Y: 2},
		{Start4X: 1, Start4Y: 0, End4X: 2, End4Y: 2},
	}
	markInterTransformEdges(edges, transformUnitBlocks(lumaUnits), chromaUnits)
	if !edges.Y.hasVertical(2, 0) {
		t.Fatal("expected luma TU boundary")
	}
	if !edges.U.hasVertical(1, 0) {
		t.Fatal("expected chroma TU boundary")
	}
}

func TestShouldFilterVerticalPrefersEdgeMap(t *testing.T) {
	mask := NewCDEFIndexMap(32, 32)
	mask.markNoSkip(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2}, false)
	edges := newLoopFilterPlaneEdges(32, 32)
	if shouldFilterVertical(mask, edges, 8, 0, 0, 0) {
		t.Fatal("unexpected filter without edge-map boundary")
	}
	edges.markVertical(2, 0, 2)
	if !shouldFilterVertical(mask, edges, 8, 0, 0, 0) {
		t.Fatal("expected filter with edge-map boundary")
	}
}
