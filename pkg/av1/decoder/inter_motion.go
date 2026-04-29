package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type InterMotionMode uint8

const (
	interMotionTranslation InterMotionMode = iota
	interMotionOBMC
	interMotionWarp
)

func ReadInterMotionMode(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, mvTile *SpatialMVTile, g BlockGeometry, syntax InterBlockSyntax, haveTop, haveLeft bool, edge intraEdgeAvailability, dec intraEntropyDecoder) (InterMotionMode, error) {
	if hdr == nil {
		return interMotionTranslation, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil || above == nil || left == nil || dec == nil {
		return interMotionTranslation, fmt.Errorf("decoder: motion mode decode requires cdf, contexts, and entropy decoder")
	}
	if !hdr.SwitchableMotion || syntax.SkipMode || syntax.Intra || g.Width4() < 2 || g.Height4() < 2 {
		return interMotionTranslation, nil
	}
	if syntax.Mode == InterPredGlobal && !hdr.ForceIntegerMV && int(syntax.Ref0) >= 0 && int(syntax.Ref0) < len(hdr.GMV) && hdr.GMV[syntax.Ref0].Type > 1 {
		return interMotionTranslation, nil
	}
	if !hasOBMCNeighbors(above, left, g, haveTop, haveLeft) {
		return interMotionTranslation, nil
	}
	if allowLocalWarp(hdr, mvTile, g, syntax.Ref0, haveTop, haveLeft, edge) {
		return InterMotionMode(dec.DecodeSymbolAdapt(cdf.MotionMode[syntax.Size][:], 2)), nil
	}
	if dec.DecodeBoolAdapt(cdf.OBMC[syntax.Size][:]) != 0 {
		return interMotionOBMC, nil
	}
	return interMotionTranslation, nil
}

func hasOBMCNeighbors(above, left *BlockContext, g BlockGeometry, haveTop, haveLeft bool) bool {
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	if haveLeft && findOddZero(left.Intra[:], by4+1, g.Height4()>>1) {
		return true
	}
	if haveTop && findOddZero(above.Intra[:], bx4+1, g.Width4()>>1) {
		return true
	}
	return false
}

func findOddZero(buf []uint8, start, count int) bool {
	for n := 0; n < count; n++ {
		idx := start + n*2
		if idx < 0 || idx >= len(buf) {
			break
		}
		if buf[idx] == 0 {
			return true
		}
	}
	return false
}

func allowLocalWarp(hdr *obu.FrameHeader, mvTile *SpatialMVTile, g BlockGeometry, ref0 int8, haveTop, haveLeft bool, edge intraEdgeAvailability) bool {
	if hdr == nil || mvTile == nil || hdr.ForceIntegerMV || !hdr.WarpMotion || ref0 < 0 {
		return false
	}
	if hasMatchingRefTop(mvTile, g, ref0, haveTop) || hasMatchingRefLeft(mvTile, g, ref0, haveLeft) {
		return true
	}
	if haveTop && haveLeft && matchingSingleRefBlock(mvTile.block(g.Start4X-1, g.Start4Y-1), ref0) {
		return true
	}
	if haveTop && edge.topHasRight && maxIntLocal(g.Width4(), g.Height4()) < 32 && matchingSingleRefBlock(mvTile.block(g.End4X, g.Start4Y-1), ref0) {
		return true
	}
	return false
}

func hasMatchingRefTop(mvTile *SpatialMVTile, g BlockGeometry, ref0 int8, haveTop bool) bool {
	if !haveTop || mvTile == nil {
		return false
	}
	for x4 := 0; x4 < g.Width4(); {
		b := mvTile.block(g.Start4X+x4, g.Start4Y-1)
		if matchingSingleRefBlock(b, ref0) {
			return true
		}
		x4 += blockStep4(b, true)
	}
	return false
}

func hasMatchingRefLeft(mvTile *SpatialMVTile, g BlockGeometry, ref0 int8, haveLeft bool) bool {
	if !haveLeft || mvTile == nil {
		return false
	}
	for y4 := 0; y4 < g.Height4(); {
		b := mvTile.block(g.Start4X-1, g.Start4Y+y4)
		if matchingSingleRefBlock(b, ref0) {
			return true
		}
		y4 += blockStep4(b, false)
	}
	return false
}

func matchingSingleRefBlock(b *SpatialMVBlock, ref0 int8) bool {
	return b != nil && b.Valid && !b.Intra && b.Ref[0] == ref0 && b.Ref[1] == -1
}

func overlapStep4(b *SpatialMVBlock, horizontal bool) int {
	if b == nil {
		return 1
	}
	dims := BlockDims(b.Size)
	step := int(dims.W4)
	if !horizontal {
		step = int(dims.H4)
	}
	return maxIntLocal(1, minInt(16, maxIntLocal(2, step)))
}

func blockStep4(b *SpatialMVBlock, horizontal bool) int {
	if b == nil {
		return 1
	}
	dims := BlockDims(b.Size)
	step := int(dims.W4)
	if !horizontal {
		step = int(dims.H4)
	}
	return maxIntLocal(1, step)
}
