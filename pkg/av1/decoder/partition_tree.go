package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

type BlockGeometry struct {
	Level   BlockLevel
	Size    BlockSize
	Start4X int
	Start4Y int
	End4X   int
	End4Y   int
}

func (g BlockGeometry) Width4() int  { return g.End4X - g.Start4X }
func (g BlockGeometry) Height4() int { return g.End4Y - g.Start4Y }

type PartitionDecisionFunc func(BlockGeometry) (BlockPartition, error)
type BlockVisitFunc func(BlockGeometry) error

func WalkSuperBlockPartitions(seq av1.SequenceHeader, sb SuperBlockGeometry, decide PartitionDecisionFunc, visit BlockVisitFunc) error {
	root := BlockGeometry{
		Level:   RootBlockLevel(seq),
		Size:    RootBlockSize(seq),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return walkPartitionTree(root, decide, visit)
}

func walkPartitionTree(root BlockGeometry, decide PartitionDecisionFunc, visit BlockVisitFunc) error {
	if decide == nil || visit == nil {
		return fmt.Errorf("decoder: partition walker requires callbacks")
	}
	part, err := decide(root)
	if err != nil {
		return err
	}
	var childrenBuf [4]BlockGeometry
	children, split, err := partitionChildrenInto(childrenBuf[:0], root, part)
	if err != nil {
		return err
	}
	if !split {
		return visit(root)
	}
	for _, child := range children {
		if err := walkPartitionTree(child, decide, visit); err != nil {
			return err
		}
	}
	return nil
}

func partitionChildren(root BlockGeometry, part BlockPartition) ([]BlockGeometry, bool, error) {
	return partitionChildrenInto(nil, root, part)
}

func partitionChildrenInto(dst []BlockGeometry, root BlockGeometry, part BlockPartition) ([]BlockGeometry, bool, error) {
	dims := BlockDims(root.Size)
	w4 := int(dims.W4)
	h4 := int(dims.H4)

	switch part {
	case PartitionNone:
		return dst[:0], false, nil
	case PartitionSplit:
		if root.Level >= BlockLevel8x8 {
			if root.Size == Block4x4 {
				return dst[:0], false, nil
			}
			return splitChildrenInto(dst, root), true, nil
		}
		return splitChildrenInto(dst, root), true, nil
	case PartitionH:
		childSize := PartitionChildSizes(root.Level, part)[0]
		out := ensureBlockGeometryLen(dst, 2)
		out[0] = childGeometry(root, childSize, 0, 0)
		out[1] = childGeometry(root, childSize, 0, h4/2)
		return out, true, nil
	case PartitionV:
		childSize := PartitionChildSizes(root.Level, part)[0]
		out := ensureBlockGeometryLen(dst, 2)
		out[0] = childGeometry(root, childSize, 0, 0)
		out[1] = childGeometry(root, childSize, w4/2, 0)
		return out, true, nil
	case PartitionTTopSplit:
		sizes := PartitionChildSizes(root.Level, part)
		out := ensureBlockGeometryLen(dst, 3)
		out[0] = childGeometry(root, sizes[0], 0, 0)
		out[1] = childGeometry(root, sizes[0], w4/2, 0)
		out[2] = childGeometry(root, sizes[1], 0, h4/2)
		return out, true, nil
	case PartitionTBottomSplit:
		sizes := PartitionChildSizes(root.Level, part)
		out := ensureBlockGeometryLen(dst, 3)
		out[0] = childGeometry(root, sizes[0], 0, 0)
		out[1] = childGeometry(root, sizes[1], 0, h4/2)
		out[2] = childGeometry(root, sizes[1], w4/2, h4/2)
		return out, true, nil
	case PartitionTLeftSplit:
		sizes := PartitionChildSizes(root.Level, part)
		out := ensureBlockGeometryLen(dst, 3)
		out[0] = childGeometry(root, sizes[0], 0, 0)
		out[1] = childGeometry(root, sizes[0], 0, h4/2)
		out[2] = childGeometry(root, sizes[1], w4/2, 0)
		return out, true, nil
	case PartitionTRightSplit:
		sizes := PartitionChildSizes(root.Level, part)
		out := ensureBlockGeometryLen(dst, 3)
		out[0] = childGeometry(root, sizes[0], 0, 0)
		out[1] = childGeometry(root, sizes[1], w4/2, 0)
		out[2] = childGeometry(root, sizes[1], w4/2, h4/2)
		return out, true, nil
	case PartitionH4:
		childSize := PartitionChildSizes(root.Level, part)[0]
		step := h4 / 4
		out := ensureBlockGeometryLen(dst, 4)
		out[0] = childGeometry(root, childSize, 0, 0*step)
		out[1] = childGeometry(root, childSize, 0, 1*step)
		out[2] = childGeometry(root, childSize, 0, 2*step)
		out[3] = childGeometry(root, childSize, 0, 3*step)
		return out, true, nil
	case PartitionV4:
		childSize := PartitionChildSizes(root.Level, part)[0]
		step := w4 / 4
		out := ensureBlockGeometryLen(dst, 4)
		out[0] = childGeometry(root, childSize, 0*step, 0)
		out[1] = childGeometry(root, childSize, 1*step, 0)
		out[2] = childGeometry(root, childSize, 2*step, 0)
		out[3] = childGeometry(root, childSize, 3*step, 0)
		return out, true, nil
	default:
		return dst[:0], false, fmt.Errorf("decoder: unsupported partition %d", part)
	}
}

func splitChildren(root BlockGeometry) []BlockGeometry {
	return splitChildrenInto(nil, root)
}

func splitChildrenInto(dst []BlockGeometry, root BlockGeometry) []BlockGeometry {
	childSize := splitChildSize(root.Level)
	dims := BlockDims(root.Size)
	w4 := int(dims.W4) / 2
	h4 := int(dims.H4) / 2
	out := ensureBlockGeometryLen(dst, 4)
	out[0] = childGeometry(root, childSize, 0, 0)
	out[1] = childGeometry(root, childSize, w4, 0)
	out[2] = childGeometry(root, childSize, 0, h4)
	out[3] = childGeometry(root, childSize, w4, h4)
	return out
}

func ensureBlockGeometryLen(dst []BlockGeometry, n int) []BlockGeometry {
	if cap(dst) < n {
		return make([]BlockGeometry, n)
	}
	return dst[:n]
}

func splitChildSize(level BlockLevel) BlockSize {
	switch level {
	case BlockLevel128x128:
		return Block64x64
	case BlockLevel64x64:
		return Block32x32
	case BlockLevel32x32:
		return Block16x16
	case BlockLevel16x16:
		return Block8x8
	default:
		return Block4x4
	}
}

func childGeometry(root BlockGeometry, size BlockSize, dx4, dy4 int) BlockGeometry {
	dims := BlockDims(size)
	start4X := min(root.Start4X+dx4, root.End4X)
	start4Y := min(root.Start4Y+dy4, root.End4Y)
	end4X := min(root.Start4X+dx4+int(dims.W4), root.End4X)
	end4Y := min(root.Start4Y+dy4+int(dims.H4), root.End4Y)
	if end4X < start4X {
		end4X = start4X
	}
	if end4Y < start4Y {
		end4Y = start4Y
	}
	return BlockGeometry{
		Level:   childLevel(size),
		Size:    size,
		Start4X: start4X,
		Start4Y: start4Y,
		End4X:   end4X,
		End4Y:   end4Y,
	}
}

func childLevel(size BlockSize) BlockLevel {
	switch size {
	case Block128x128:
		return BlockLevel128x128
	case Block64x64, Block128x64, Block64x128, Block64x32, Block64x16, Block32x64, Block16x64:
		return BlockLevel64x64
	case Block32x32, Block32x16, Block32x8, Block16x32, Block8x32:
		return BlockLevel32x32
	case Block16x16, Block16x8, Block16x4, Block8x16, Block4x16:
		return BlockLevel16x16
	default:
		return BlockLevel8x8
	}
}
