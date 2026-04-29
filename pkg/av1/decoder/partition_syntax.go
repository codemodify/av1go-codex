package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type partitionEntropyDecoder interface {
	DecodeBool(uint32) uint32
	DecodeSymbolAdapt([]uint16, int) uint32
}

type msacPartitionDecoder struct {
	ctx *msac.Context
}

func (d msacPartitionDecoder) DecodeBool(prob uint32) uint32 {
	return d.ctx.DecodeBool(prob)
}

func (d msacPartitionDecoder) DecodeSymbolAdapt(cdf []uint16, n int) uint32 {
	return d.ctx.DecodeSymbolAdapt(cdf, n)
}

func DecodePartitionDecision(seq av1.SequenceHeader, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, g BlockGeometry, dec partitionEntropyDecoder) (BlockPartition, error) {
	if hdr == nil {
		return 0, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return 0, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return 0, fmt.Errorf("decoder: block contexts required")
	}
	if dec == nil {
		return 0, fmt.Errorf("decoder: entropy decoder required")
	}

	dims := BlockDims(g.Size)
	halfW4 := int(dims.W4) >> 1
	halfH4 := int(dims.H4) >> 1
	haveHSplit := g.Width4() > halfW4
	haveVSplit := g.Height4() > halfH4
	if !haveHSplit && !haveVSplit {
		if g.Level >= BlockLevel8x8 {
			return PartitionNone, nil
		}
		return PartitionSplit, nil
	}
	if g.Level == BlockLevel8x8 {
		switch {
		case haveHSplit && haveVSplit:
		case haveHSplit:
			return PartitionH, nil
		case haveVSplit:
			return PartitionV, nil
		}
	}

	bx8 := g.Start4X >> 1
	by8 := g.Start4Y >> 1
	ctx := GetPartitionCtx(above, left, g.Level, by8, bx8)
	pcdf := cdf.PartitionCDF(g.Level, ctx)

	switch {
	case haveHSplit && haveVSplit:
		lastSymbol := int(PartitionTypeCount(g.Level))
		part := BlockPartition(dec.DecodeSymbolAdapt(pcdf, lastSymbol))
		if int(part) > lastSymbol {
			return 0, fmt.Errorf("decoder: invalid partition %d for level %d", part, g.Level)
		}
		return part, nil
	case haveHSplit:
		prob := GatherTopPartitionProb(pcdf, g.Level)
		if prob > 32768 {
			return 0, fmt.Errorf("decoder: invalid top partition probability %d for level %d pcdf=%v", prob, g.Level, pcdf)
		}
		if dec.DecodeBool(prob) != 0 {
			return PartitionSplit, nil
		}
		return PartitionH, nil
	default:
		prob := GatherLeftPartitionProb(pcdf, g.Level)
		if prob > 32768 {
			return 0, fmt.Errorf("decoder: invalid left partition probability %d for level %d pcdf=%v", prob, g.Level, pcdf)
		}
		if dec.DecodeBool(prob) != 0 {
			return PartitionSplit, nil
		}
		return PartitionV, nil
	}
}

func ApplyPartitionContext(above, left *BlockContext, g BlockGeometry, part BlockPartition) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	xv := AlignPartitionContext(0, g.Level, part)
	yv := AlignPartitionContext(1, g.Level, part)
	if xv < 0 || yv < 0 {
		return fmt.Errorf("decoder: partition %d does not update context at level %d", part, g.Level)
	}

	dims := BlockDims(g.Size)
	runX := int(dims.W4) >> 1
	runY := int(dims.H4) >> 1
	fillRunUint8(above.Partition[:], g.Start4X>>1, minInt(runX, len(above.Partition)-(g.Start4X>>1)), uint8(xv))
	fillRunUint8(left.Partition[:], g.Start4Y>>1, minInt(runY, len(left.Partition)-(g.Start4Y>>1)), uint8(yv))
	return nil
}
