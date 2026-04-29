package decoder

import (
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

const (
	angleUseEdgeFilterFlag uint16 = 1024
	angleSmoothEdgeFlag    uint16 = 512
)

var av1DrIntraDerivative = [44]int{
	0,
	1023, 0,
	547,
	372, 0, 0,
	273,
	215, 0,
	178,
	151, 0,
	132,
	116, 0,
	102, 0,
	90,
	80, 0,
	71,
	64, 0,
	57,
	51, 0,
	45, 0,
	40,
	35, 0,
	31,
	27, 0,
	23,
	19, 0,
	15, 0,
	11, 0,
	7,
	3,
}

var intraDirectionalFilterKernel = [3][5]int{
	{0, 4, 8, 4, 0},
	{0, 5, 6, 5, 0},
	{2, 4, 4, 4, 2},
}

var intraDirectionalByteScratchPool sync.Pool

func intraLumaPredictFlags(seq av1.SequenceHeader, above, left *BlockContext, syntax IntraBlockSyntax) uint16 {
	if !isDirectionalMode(syntax.YMode) {
		return 0
	}
	var flags uint16
	if seq.EnableIntraEdgeFilter {
		flags |= angleUseEdgeFilterFlag
	}
	bx4 := syntax.Geometry.Start4X & 31
	by4 := syntax.Geometry.Start4Y & 31
	if above != nil && above.Intra[bx4] != 0 && isSmoothIntraMode(IntraPredMode(above.Mode[bx4])) {
		flags |= angleSmoothEdgeFlag
	}
	if left != nil && left.Intra[by4] != 0 && isSmoothIntraMode(IntraPredMode(left.Mode[by4])) {
		flags |= angleSmoothEdgeFlag
	}
	return flags
}

func intraChromaPredictFlags(seq av1.SequenceHeader, above, left *BlockContext, syntax IntraBlockSyntax) uint16 {
	if !isDirectionalMode(syntax.UVMode) {
		return 0
	}
	var flags uint16
	if seq.EnableIntraEdgeFilter {
		flags |= angleUseEdgeFilterFlag
	}
	bx4 := syntax.Geometry.Start4X & 31
	by4 := syntax.Geometry.Start4Y & 31
	switch seq.ColorConfig.ChromaSubsampling() {
	case av1.Chroma420:
		bx4 >>= 1
		by4 >>= 1
	case av1.Chroma422:
		bx4 >>= 1
	}
	if above != nil && isSmoothIntraMode(IntraPredMode(above.UVMode[bx4])) {
		flags |= angleSmoothEdgeFlag
	}
	if left != nil && isSmoothIntraMode(IntraPredMode(left.UVMode[by4])) {
		flags |= angleSmoothEdgeFlag
	}
	return flags
}

func isSmoothIntraMode(mode IntraPredMode) bool {
	return mode == IntraPredSmooth || mode == IntraPredSmoothVertical || mode == IntraPredSmoothHorizontal
}

func directionalDerivative(angle int) int {
	if angle <= 0 {
		return 0
	}
	if angle%90 == 0 {
		return 0
	}
	idx := angle >> 1
	if idx >= 0 && idx < len(av1DrIntraDerivative) && av1DrIntraDerivative[idx] != 0 {
		return av1DrIntraDerivative[idx]
	}
	return maxIntLocal(1, int((64+angle/2)/maxIntLocal(angle, 1)))
}

func useIntraEdgeUpsample(wh, angle int, isSmooth bool) bool {
	limit := 16
	if isSmooth {
		limit >>= 1
	}
	return angle < 40 && wh <= limit
}

func intraEdgeFilterStrength(wh, angle int, isSmooth bool) int {
	if isSmooth {
		switch {
		case wh <= 8:
			if angle >= 64 {
				return 2
			}
			if angle >= 40 {
				return 1
			}
		case wh <= 16:
			if angle >= 48 {
				return 2
			}
			if angle >= 20 {
				return 1
			}
		case wh <= 24:
			if angle >= 4 {
				return 3
			}
		default:
			return 3
		}
		return 0
	}

	switch {
	case wh <= 8:
		if angle >= 56 {
			return 1
		}
	case wh <= 16:
		if angle >= 40 {
			return 1
		}
	case wh <= 24:
		if angle >= 32 {
			return 3
		}
		if angle >= 16 {
			return 2
		}
		if angle >= 8 {
			return 1
		}
	case wh <= 32:
		if angle >= 32 {
			return 3
		}
		if angle >= 4 {
			return 2
		}
		return 1
	default:
		return 3
	}
	return 0
}

func edgeSample8(src []byte, logical int) byte {
	idx := logical + 1
	if idx < 0 {
		idx = 0
	} else if idx >= len(src) {
		idx = len(src) - 1
	}
	return src[idx]
}

func edgeSample16(src []uint16, logical int) uint16 {
	idx := logical + 1
	if idx < 0 {
		idx = 0
	} else if idx >= len(src) {
		idx = len(src) - 1
	}
	return src[idx]
}

func filterDirectionalEdge8Range(src []byte, outLen, limFrom, limTo, start, from, to, strength int) []byte {
	out := make([]byte, outLen)
	return filterDirectionalEdge8RangeInto(out, src, outLen, limFrom, limTo, start, from, to, strength)
}

func filterDirectionalEdge8RangeInto(dst, src []byte, outLen, limFrom, limTo, start, from, to, strength int) []byte {
	dst = dst[:outLen]
	kernel := intraDirectionalFilterKernel[strength-1]
	for i := 0; i < minInt(outLen, limFrom); i++ {
		dst[i] = edgeSample8Range(src, start+i, from, to)
	}
	for i := minInt(outLen, limFrom); i < minInt(outLen, limTo); i++ {
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(edgeSample8Range(src, start+i-2+j, from, to)) * kernel[j]
		}
		dst[i] = clipByte((sum + 8) >> 4)
	}
	for i := maxIntLocal(0, limTo); i < outLen; i++ {
		dst[i] = edgeSample8Range(src, start+i, from, to)
	}
	return dst
}

func filterDirectionalEdge8(src []byte, outLen, strength int) []byte {
	out := make([]byte, outLen)
	filterDirectionalEdge8Into(out, src, strength)
	return out
}

func filterDirectionalEdge8Into(dst, src []byte, strength int) {
	kernel := intraDirectionalFilterKernel[strength-1]
	for i := 0; i < len(dst); i++ {
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(edgeSample8(src, i-2+j)) * kernel[j]
		}
		dst[i] = clipByte((sum + 8) >> 4)
	}
}

func filterDirectionalZ3Edge8Into(dst, edge []byte, topLeft byte, total, from, strength int) []byte {
	dst = dst[:total]
	kernel := intraDirectionalFilterKernel[strength-1]
	for base := 0; base < total; base++ {
		i := total - 1 - base
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(z3InputSample8(edge, topLeft, total, i-2+j, from)) * kernel[j]
		}
		dst[base] = clipByte((sum + 8) >> 4)
	}
	return dst
}

func filterDirectionalZ2LeftEdge8Into(dst, left []byte, topLeft byte, height, strength int) []byte {
	dst = dst[:height+1]
	dst[0] = topLeft
	kernel := intraDirectionalFilterKernel[strength-1]
	for base := 0; base < height; base++ {
		i := height - 1 - base
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(z2LeftInputSample8(left, topLeft, height, i-2+j)) * kernel[j]
		}
		dst[base+1] = clipByte((sum + 8) >> 4)
	}
	return dst
}

func filterDirectionalZ2LeftEdge16(left []uint16, topLeft uint16, height, strength int, bitDepth uint8) []uint16 {
	out := make([]uint16, height+1)
	out[0] = topLeft
	kernel := intraDirectionalFilterKernel[strength-1]
	for base := 0; base < height; base++ {
		i := height - 1 - base
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(z2LeftInputSample16(left, topLeft, height, i-2+j)) * kernel[j]
		}
		out[base+1] = clipSample((sum+8)>>4, bitDepth)
	}
	return out
}

func filterDirectionalZ3Edge16(edge []uint16, topLeft uint16, total, from, strength int, bitDepth uint8) []uint16 {
	out := make([]uint16, total)
	kernel := intraDirectionalFilterKernel[strength-1]
	for base := 0; base < total; base++ {
		i := total - 1 - base
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(z3InputSample16(edge, topLeft, total, i-2+j, from)) * kernel[j]
		}
		out[base] = clipSample((sum+8)>>4, bitDepth)
	}
	return out
}

func filterDirectionalEdge16Range(src []uint16, outLen, limFrom, limTo, start, from, to, strength int, bitDepth uint8) []uint16 {
	out := make([]uint16, outLen)
	kernel := intraDirectionalFilterKernel[strength-1]
	for i := 0; i < minInt(outLen, limFrom); i++ {
		out[i] = edgeSample16Range(src, start+i, from, to)
	}
	for i := minInt(outLen, limFrom); i < minInt(outLen, limTo); i++ {
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(edgeSample16Range(src, start+i-2+j, from, to)) * kernel[j]
		}
		out[i] = clipSample((sum+8)>>4, bitDepth)
	}
	for i := maxIntLocal(0, limTo); i < outLen; i++ {
		out[i] = edgeSample16Range(src, start+i, from, to)
	}
	return out
}

func filterDirectionalEdge16(src []uint16, outLen, strength int, bitDepth uint8) []uint16 {
	out := make([]uint16, outLen)
	kernel := intraDirectionalFilterKernel[strength-1]
	for i := 0; i < outLen; i++ {
		sum := 0
		for j := 0; j < 5; j++ {
			sum += int(edgeSample16(src, i-2+j)) * kernel[j]
		}
		out[i] = clipSample((sum+8)>>4, bitDepth)
	}
	return out
}

func upsampleDirectionalEdge8Range(src []byte, halfLen, start, from, to int) []byte {
	out := make([]byte, halfLen*2-1)
	return upsampleDirectionalEdge8RangeInto(out, src, halfLen, start, from, to)
}

func upsampleDirectionalEdge8RangeInto(dst, src []byte, halfLen, start, from, to int) []byte {
	dst = dst[:halfLen*2-1]
	for i := 0; i < halfLen-1; i++ {
		logical := start + i
		dst[i*2] = edgeSample8Range(src, logical, from, to)
		sum := -int(edgeSample8Range(src, logical-1, from, to)) +
			9*int(edgeSample8Range(src, logical, from, to)) +
			9*int(edgeSample8Range(src, logical+1, from, to)) -
			int(edgeSample8Range(src, logical+2, from, to))
		dst[i*2+1] = clipByte((sum + 8) >> 4)
	}
	dst[(halfLen-1)*2] = edgeSample8Range(src, start+halfLen-1, from, to)
	return dst
}

func upsampleDirectionalEdge8(src []byte, edgeLen int) []byte {
	out := make([]byte, edgeLen*2-1)
	return upsampleDirectionalEdge8Into(out, src, edgeLen)
}

func upsampleDirectionalEdge8Into(dst, src []byte, edgeLen int) []byte {
	dst = dst[:edgeLen*2-1]
	for i := 0; i < edgeLen-1; i++ {
		dst[i*2] = edgeSample8(src, i)
		sum := -int(edgeSample8(src, i-1)) +
			9*int(edgeSample8(src, i)) +
			9*int(edgeSample8(src, i+1)) -
			int(edgeSample8(src, i+2))
		dst[i*2+1] = clipByte((sum + 8) >> 4)
	}
	dst[(edgeLen-1)*2] = edgeSample8(src, edgeLen-1)
	return dst
}

func upsampleDirectionalZ3Edge8Into(dst, edge []byte, topLeft byte, total, from int) []byte {
	dst = dst[:total*2-1]
	for base := range dst {
		revIdx := len(dst) - 1 - base
		i := revIdx >> 1
		if revIdx&1 == 0 {
			dst[base] = z3InputSample8(edge, topLeft, total, i, from)
			continue
		}
		sum := -int(z3InputSample8(edge, topLeft, total, i-1, from)) +
			9*int(z3InputSample8(edge, topLeft, total, i, from)) +
			9*int(z3InputSample8(edge, topLeft, total, i+1, from)) -
			int(z3InputSample8(edge, topLeft, total, i+2, from))
		dst[base] = clipByte((sum + 8) >> 4)
	}
	return dst
}

func upsampleDirectionalZ3Edge16(edge []uint16, topLeft uint16, total, from int, bitDepth uint8) []uint16 {
	out := make([]uint16, total*2-1)
	for base := range out {
		revIdx := len(out) - 1 - base
		i := revIdx >> 1
		if revIdx&1 == 0 {
			out[base] = z3InputSample16(edge, topLeft, total, i, from)
			continue
		}
		sum := -int(z3InputSample16(edge, topLeft, total, i-1, from)) +
			9*int(z3InputSample16(edge, topLeft, total, i, from)) +
			9*int(z3InputSample16(edge, topLeft, total, i+1, from)) -
			int(z3InputSample16(edge, topLeft, total, i+2, from))
		out[base] = clipSample((sum+8)>>4, bitDepth)
	}
	return out
}

func upsampleDirectionalEdge16Range(src []uint16, halfLen, start, from, to int, bitDepth uint8) []uint16 {
	out := make([]uint16, halfLen*2-1)
	return upsampleDirectionalEdge16RangeInto(out, src, halfLen, start, from, to, bitDepth)
}

func upsampleDirectionalEdge16RangeInto(dst, src []uint16, halfLen, start, from, to int, bitDepth uint8) []uint16 {
	dst = dst[:halfLen*2-1]
	for i := 0; i < halfLen-1; i++ {
		logical := start + i
		dst[i*2] = edgeSample16Range(src, logical, from, to)
		sum := -int(edgeSample16Range(src, logical-1, from, to)) +
			9*int(edgeSample16Range(src, logical, from, to)) +
			9*int(edgeSample16Range(src, logical+1, from, to)) -
			int(edgeSample16Range(src, logical+2, from, to))
		dst[i*2+1] = clipSample((sum+8)>>4, bitDepth)
	}
	dst[(halfLen-1)*2] = edgeSample16Range(src, start+halfLen-1, from, to)
	return dst
}

func upsampleDirectionalEdge16(src []uint16, edgeLen int, bitDepth uint8) []uint16 {
	out := make([]uint16, edgeLen*2-1)
	for i := 0; i < edgeLen-1; i++ {
		out[i*2] = edgeSample16(src, i)
		sum := -int(edgeSample16(src, i-1)) +
			9*int(edgeSample16(src, i)) +
			9*int(edgeSample16(src, i+1)) -
			int(edgeSample16(src, i+2))
		out[i*2+1] = clipSample((sum+8)>>4, bitDepth)
	}
	out[(edgeLen-1)*2] = edgeSample16(src, edgeLen-1)
	return out
}

func edgeSample8Range(src []byte, logical, from, to int) byte {
	if logical < from {
		logical = from
	} else if logical >= to {
		logical = to - 1
	}
	return edgeSample8(src, logical)
}

func edgeSample16Range(src []uint16, logical, from, to int) uint16 {
	if logical < from {
		logical = from
	} else if logical >= to {
		logical = to - 1
	}
	return edgeSample16(src, logical)
}

func z3InputSample8(edge []byte, topLeft byte, total, logical, from int) byte {
	if logical < from {
		logical = from
	} else if logical > total {
		logical = total
	}
	if logical == total {
		return topLeft
	}
	return edge[total-1-logical]
}

func z3InputSample16(edge []uint16, topLeft uint16, total, logical, from int) uint16 {
	if logical < from {
		logical = from
	} else if logical > total {
		logical = total
	}
	if logical == total {
		return topLeft
	}
	return edge[total-1-logical]
}

func z2LeftInputSample8(left []byte, topLeft byte, height, logical int) byte {
	if logical < 0 {
		logical = 0
	} else if logical > height {
		logical = height
	}
	if logical == height {
		return topLeft
	}
	return left[height-1-logical]
}

func z2LeftInputSample16(left []uint16, topLeft uint16, height, logical int) uint16 {
	if logical < 0 {
		logical = 0
	} else if logical > height {
		logical = height
	}
	if logical == height {
		return topLeft
	}
	return left[height-1-logical]
}
