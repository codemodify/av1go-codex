package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func TestWalkSuperBlockPartitionsSplitToFour4x4Leaves(t *testing.T) {
	sb := SuperBlockGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   32,
		End4Y:   32,
	}
	var leaves []BlockGeometry
	err := WalkSuperBlockPartitions(av1.SequenceHeader{Use128x128SuperBlock: true}, sb, func(g BlockGeometry) (BlockPartition, error) {
		if g.Size == Block8x8 {
			return PartitionSplit, nil
		}
		return PartitionSplit, nil
	}, func(g BlockGeometry) error {
		leaves = append(leaves, g)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkSuperBlockPartitions: %v", err)
	}
	if len(leaves) != 1024 {
		t.Fatalf("leaf count = %d, want 1024", len(leaves))
	}
	for _, leaf := range leaves {
		if leaf.Size != Block4x4 {
			t.Fatalf("leaf size = %d, want %d", leaf.Size, Block4x4)
		}
		if leaf.Width4() != 1 || leaf.Height4() != 1 {
			t.Fatalf("leaf dims = %dx%d, want 1x1", leaf.Width4(), leaf.Height4())
		}
	}
}

func TestPartitionChildrenTTopSplit(t *testing.T) {
	root := BlockGeometry{
		Level:   BlockLevel64x64,
		Size:    Block64x64,
		Start4X: 0,
		Start4Y: 0,
		End4X:   16,
		End4Y:   16,
	}
	children, split, err := partitionChildren(root, PartitionTTopSplit)
	if err != nil {
		t.Fatalf("partitionChildren: %v", err)
	}
	if !split {
		t.Fatal("expected split")
	}
	if len(children) != 3 {
		t.Fatalf("child count = %d, want 3", len(children))
	}
	if children[0].Size != Block32x32 || children[1].Size != Block32x32 || children[2].Size != Block64x32 {
		t.Fatalf("unexpected child sizes: %d %d %d", children[0].Size, children[1].Size, children[2].Size)
	}
	if children[2].Start4Y != 8 || children[2].End4Y != 16 {
		t.Fatalf("bottom child y = %d..%d, want 8..16", children[2].Start4Y, children[2].End4Y)
	}
}

func TestPartitionChildrenH4(t *testing.T) {
	root := BlockGeometry{
		Level:   BlockLevel16x16,
		Size:    Block16x16,
		Start4X: 0,
		Start4Y: 0,
		End4X:   4,
		End4Y:   4,
	}
	children, split, err := partitionChildren(root, PartitionH4)
	if err != nil {
		t.Fatalf("partitionChildren: %v", err)
	}
	if !split {
		t.Fatal("expected split")
	}
	if len(children) != 4 {
		t.Fatalf("child count = %d, want 4", len(children))
	}
	for i, child := range children {
		if child.Size != Block16x4 {
			t.Fatalf("child %d size = %d, want %d", i, child.Size, Block16x4)
		}
		if child.Height4() != 1 {
			t.Fatalf("child %d height4 = %d, want 1", i, child.Height4())
		}
	}
}

func TestSplitChildrenUsesNominalOffsetsForClippedBottomEdge(t *testing.T) {
	root := BlockGeometry{
		Level:   BlockLevel32x32,
		Size:    Block32x32,
		Start4X: 24,
		Start4Y: 17,
		End4X:   32,
		End4Y:   18,
	}
	children := splitChildren(root)
	if len(children) != 4 {
		t.Fatalf("child count = %d, want 4", len(children))
	}
	if children[0].Width4() != 4 || children[0].Height4() != 1 {
		t.Fatalf("top-left child dims = %dx%d, want 4x1", children[0].Width4(), children[0].Height4())
	}
	if children[1].Width4() != 4 || children[1].Height4() != 1 {
		t.Fatalf("top-right child dims = %dx%d, want 4x1", children[1].Width4(), children[1].Height4())
	}
	for i := 2; i < 4; i++ {
		if children[i].Height4() != 0 {
			t.Fatalf("bottom child %d height4 = %d, want 0", i, children[i].Height4())
		}
		if children[i].Start4Y != root.End4Y || children[i].End4Y != root.End4Y {
			t.Fatalf("bottom child %d y = %d..%d, want %d..%d", i, children[i].Start4Y, children[i].End4Y, root.End4Y, root.End4Y)
		}
	}
}
