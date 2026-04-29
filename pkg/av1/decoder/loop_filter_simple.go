package decoder

import (
	"image"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type loopFilterParams struct {
	E int
	I int
	H int
	F int
}

func applySimpleLoopFilter420_8(img *image.YCbCr, hdr *obu.FrameHeader) {
	applySimpleLoopFilter420_8Masked(img, hdr, nil)
}

func applySimpleLoopFilter420_8Masked(img *image.YCbCr, hdr *obu.FrameHeader, mask *CDEFIndexMap) {
	applyLoopFilter420_8MaskedWithLevels(img, hdr, nil, nil, mask)
}

func applySimpleLoopFilter420_8MaskedWithEdges(img *image.YCbCr, hdr *obu.FrameHeader, edges *loopFilterEdgeMaps, mask *CDEFIndexMap) {
	applyLoopFilter420_8MaskedWithLevels(img, hdr, nil, edges, mask)
}

func applyAdaptiveLoopFilter420_8Masked(img *image.YCbCr, hdr *obu.FrameHeader, field *SpatialMVTile, seg *SegmentationMap, edges *loopFilterEdgeMaps, mask *CDEFIndexMap) {
	levels := buildLoopFilterLevelMap(hdr, field, seg)
	defer putLoopFilterLevelMap(levels)
	applyLoopFilter420_8MaskedWithLevels(img, hdr, levels, edges, mask)
}

func applyLoopFilter420_8MaskedWithLevels(img *image.YCbCr, hdr *obu.FrameHeader, levels *loopFilterLevelMap, edges *loopFilterEdgeMaps, mask *CDEFIndexMap) {
	if img == nil || hdr == nil {
		return
	}
	if lvl := int(hdr.LoopFilter.LevelY[0]); lvl > 0 {
		deblockPlaneVertical8(img.Y, img.YStride, img.Rect.Dx(), img.Rect.Dy(), lvl, hdr.LoopFilter.Sharpness, false, mask, 0, 0, levels, 0, ternaryEdges(edges, 0))
	}
	if lvl := int(hdr.LoopFilter.LevelY[1]); lvl > 0 {
		deblockPlaneHorizontal8(img.Y, img.YStride, img.Rect.Dx(), img.Rect.Dy(), lvl, hdr.LoopFilter.Sharpness, false, mask, 0, 0, levels, 1, ternaryEdges(edges, 0))
	}
	if lvl := int(hdr.LoopFilter.LevelU); lvl > 0 {
		cw := parsedChromaPlaneWidth(img)
		ch := parsedChromaPlaneHeight(img)
		deblockPlaneVertical8(img.Cb, img.CStride, cw, ch, lvl, hdr.LoopFilter.Sharpness, true, mask, 1, 1, levels, 2, ternaryEdges(edges, 1))
		deblockPlaneHorizontal8(img.Cb, img.CStride, cw, ch, lvl, hdr.LoopFilter.Sharpness, true, mask, 1, 1, levels, 2, ternaryEdges(edges, 1))
	}
	if lvl := int(hdr.LoopFilter.LevelV); lvl > 0 {
		cw := parsedChromaPlaneWidth(img)
		ch := parsedChromaPlaneHeight(img)
		deblockPlaneVertical8(img.Cr, img.CStride, cw, ch, lvl, hdr.LoopFilter.Sharpness, true, mask, 1, 1, levels, 3, ternaryEdges(edges, 2))
		deblockPlaneHorizontal8(img.Cr, img.CStride, cw, ch, lvl, hdr.LoopFilter.Sharpness, true, mask, 1, 1, levels, 3, ternaryEdges(edges, 2))
	}
}

func deblockPlaneVertical8(plane []byte, stride, width, height, level int, sharpness uint8, chroma bool, mask *CDEFIndexMap, ssX, ssY int, levels *loopFilterLevelMap, levelPlane int, edgeMap *loopFilterPlaneEdges) {
	if stride <= 0 || width < 4 || height <= 0 {
		return
	}
	params := calcLoopFilterParams(level, sharpness, 8)
	var paramCache [64]loopFilterParams
	for y := 0; y < height; y++ {
		row := plane[y*stride:]
		for x := 4; x < width; x += 4 {
			edgeLevel := level
			if levels != nil {
				edgeLevel = levels.verticalLevel(levelPlane, x, y, ssX, ssY)
				if edgeLevel == 0 {
					continue
				}
				params = paramCache[edgeLevel]
				if params.E == 0 {
					params = calcLoopFilterParams(edgeLevel, sharpness, 8)
					paramCache[edgeLevel] = params
				}
			}
			wd := chooseLoopFilterWidth(chroma, edgeLevel, x, width-x)
			if wd == 0 || !hasLoopFilterSupport(wd, x, width-x) {
				continue
			}
			if !shouldFilterVertical(mask, edgeMap, x, y, ssX, ssY) {
				continue
			}
			loopFilterEdge8(row, x-1, 1, params, wd)
		}
	}
}

func deblockPlaneHorizontal8(plane []byte, stride, width, height, level int, sharpness uint8, chroma bool, mask *CDEFIndexMap, ssX, ssY int, levels *loopFilterLevelMap, levelPlane int, edgeMap *loopFilterPlaneEdges) {
	if stride <= 0 || width <= 0 || height < 4 {
		return
	}
	params := calcLoopFilterParams(level, sharpness, 8)
	var paramCache [64]loopFilterParams
	for y := 4; y < height; y += 4 {
		for x := 0; x < width; x++ {
			edgeLevel := level
			if levels != nil {
				edgeLevel = levels.horizontalLevel(levelPlane, x, y, ssX, ssY)
				if edgeLevel == 0 {
					continue
				}
				params = paramCache[edgeLevel]
				if params.E == 0 {
					params = calcLoopFilterParams(edgeLevel, sharpness, 8)
					paramCache[edgeLevel] = params
				}
			}
			wd := chooseLoopFilterWidth(chroma, edgeLevel, y, height-y)
			if wd == 0 || !hasLoopFilterSupport(wd, y, height-y) {
				continue
			}
			if !shouldFilterHorizontal(mask, edgeMap, x, y, ssX, ssY) {
				continue
			}
			loopFilterEdge8(plane, (y-1)*stride+x, stride, params, wd)
		}
	}
}

func calcLoopFilterParams(level int, sharpness uint8, bitDepth uint8) loopFilterParams {
	if level <= 0 {
		return loopFilterParams{}
	}
	limit := level
	if sharpness > 0 {
		limit >>= int((sharpness + 3) >> 2)
		limit = minInt(limit, maxIntLocal(0, 9-int(sharpness)))
	}
	limit = maxIntLocal(limit, 1)
	scale := 1 << maxIntLocal(int(bitDepth)-8, 0)
	return loopFilterParams{
		E: (2*(level+2) + limit) * scale,
		I: limit * scale,
		H: (level >> 4) * scale,
		F: scale,
	}
}

func chooseLoopFilterWidth(chroma bool, level, before, after int) int {
	if chroma {
		if before >= 3 && after >= 3 {
			return 6
		}
		if before >= 2 && after >= 2 {
			return 4
		}
		return 0
	}
	if level >= 32 && before >= 7 && after >= 7 {
		return 16
	}
	if before >= 4 && after >= 4 {
		return 8
	}
	if before >= 2 && after >= 2 {
		return 4
	}
	return 0
}

func hasLoopFilterSupport(wd, before, after int) bool {
	switch wd {
	case 16:
		return before >= 7 && after >= 7
	case 8:
		return before >= 4 && after >= 4
	case 6:
		return before >= 3 && after >= 3
	case 4:
		return before >= 2 && after >= 2
	default:
		return false
	}
}

func loopFilterEdge8(buf []byte, p0Idx, step int, params loopFilterParams, wd int) {
	if params.E == 0 || params.I == 0 || wd < 4 {
		return
	}
	q0Idx := p0Idx + step
	p1 := int(buf[p0Idx-step])
	p0 := int(buf[p0Idx])
	q0 := int(buf[q0Idx])
	q1 := int(buf[q0Idx+step])

	fm := absInt(p1-p0) <= params.I &&
		absInt(q1-q0) <= params.I &&
		absInt(p0-q0)*2+(absInt(p1-q1)>>1) <= params.E

	p2, q2 := 0, 0
	if wd > 4 {
		p2 = int(buf[p0Idx-2*step])
		q2 = int(buf[q0Idx+2*step])
		fm = fm && absInt(p2-p1) <= params.I && absInt(q2-q1) <= params.I
	}
	p3, q3 := 0, 0
	if wd > 6 {
		p3 = int(buf[p0Idx-3*step])
		q3 = int(buf[q0Idx+3*step])
		fm = fm && absInt(p3-p2) <= params.I && absInt(q3-q2) <= params.I
	}
	if !fm {
		return
	}

	flat8in := false
	if wd >= 6 {
		flat8in = absInt(p2-p0) <= params.F &&
			absInt(p1-p0) <= params.F &&
			absInt(q1-q0) <= params.F &&
			absInt(q2-q0) <= params.F
	}
	if wd >= 8 {
		flat8in = flat8in &&
			absInt(p3-p0) <= params.F &&
			absInt(q3-q0) <= params.F
	}
	if wd >= 16 {
		p4 := int(buf[p0Idx-4*step])
		p5 := int(buf[p0Idx-5*step])
		p6 := int(buf[p0Idx-6*step])
		q4 := int(buf[q0Idx+4*step])
		q5 := int(buf[q0Idx+5*step])
		q6 := int(buf[q0Idx+6*step])
		flat8out := absInt(p6-p0) <= params.F &&
			absInt(p5-p0) <= params.F &&
			absInt(p4-p0) <= params.F &&
			absInt(q4-q0) <= params.F &&
			absInt(q5-q0) <= params.F &&
			absInt(q6-q0) <= params.F
		if flat8out && flat8in {
			buf[p0Idx-5*step] = clipByte((p6 + p6 + p6 + p6 + p6 + 2*p6 + 2*p5 + 2*p4 + p3 + p2 + p1 + p0 + q0 + 8) >> 4)
			buf[p0Idx-4*step] = clipByte((p6 + p6 + p6 + p6 + p6 + 2*p5 + 2*p4 + 2*p3 + p2 + p1 + p0 + q0 + q1 + 8) >> 4)
			buf[p0Idx-3*step] = clipByte((p6 + p6 + p6 + p6 + p5 + 2*p4 + 2*p3 + 2*p2 + p1 + p0 + q0 + q1 + q2 + 8) >> 4)
			buf[p0Idx-2*step] = clipByte((p6 + p6 + p6 + p5 + p4 + 2*p3 + 2*p2 + 2*p1 + p0 + q0 + q1 + q2 + q3 + 8) >> 4)
			buf[p0Idx-1*step] = clipByte((p6 + p6 + p5 + p4 + p3 + 2*p2 + 2*p1 + 2*p0 + q0 + q1 + q2 + q3 + q4 + 8) >> 4)
			buf[p0Idx] = clipByte((p6 + p5 + p4 + p3 + p2 + 2*p1 + 2*p0 + 2*q0 + q1 + q2 + q3 + q4 + q5 + 8) >> 4)
			buf[q0Idx] = clipByte((p5 + p4 + p3 + p2 + p1 + 2*p0 + 2*q0 + 2*q1 + q2 + q3 + q4 + q5 + q6 + 8) >> 4)
			buf[q0Idx+step] = clipByte((p4 + p3 + p2 + p1 + p0 + 2*q0 + 2*q1 + 2*q2 + q3 + q4 + q5 + q6 + q6 + 8) >> 4)
			buf[q0Idx+2*step] = clipByte((p3 + p2 + p1 + p0 + q0 + 2*q1 + 2*q2 + 2*q3 + q4 + q5 + q6 + q6 + q6 + 8) >> 4)
			buf[q0Idx+3*step] = clipByte((p2 + p1 + p0 + q0 + q1 + 2*q2 + 2*q3 + 2*q4 + q5 + q6 + q6 + q6 + q6 + 8) >> 4)
			buf[q0Idx+4*step] = clipByte((p1 + p0 + q0 + q1 + q2 + 2*q3 + 2*q4 + 2*q5 + q6 + q6 + q6 + q6 + q6 + 8) >> 4)
			buf[q0Idx+5*step] = clipByte((p0 + q0 + q1 + q2 + q3 + 2*q4 + 2*q5 + 2*q6 + q6 + q6 + q6 + q6 + q6 + 8) >> 4)
			return
		}
	}
	if wd >= 8 && flat8in {
		buf[p0Idx-2*step] = clipByte((p3 + p3 + p3 + 2*p2 + p1 + p0 + q0 + 4) >> 3)
		buf[p0Idx-step] = clipByte((p3 + p3 + p2 + 2*p1 + p0 + q0 + q1 + 4) >> 3)
		buf[p0Idx] = clipByte((p3 + p2 + p1 + 2*p0 + q0 + q1 + q2 + 4) >> 3)
		buf[q0Idx] = clipByte((p2 + p1 + p0 + 2*q0 + q1 + q2 + q3 + 4) >> 3)
		buf[q0Idx+step] = clipByte((p1 + p0 + q0 + 2*q1 + q2 + q3 + q3 + 4) >> 3)
		buf[q0Idx+2*step] = clipByte((p0 + q0 + q1 + 2*q2 + q3 + q3 + q3 + 4) >> 3)
		return
	}
	if wd == 6 && flat8in {
		buf[p0Idx-step] = clipByte((p2 + 2*p2 + 2*p1 + 2*p0 + q0 + 4) >> 3)
		buf[p0Idx] = clipByte((p2 + 2*p1 + 2*p0 + 2*q0 + q1 + 4) >> 3)
		buf[q0Idx] = clipByte((p1 + 2*p0 + 2*q0 + 2*q1 + q2 + 4) >> 3)
		buf[q0Idx+step] = clipByte((p0 + 2*q0 + 2*q1 + 2*q2 + q2 + 4) >> 3)
		return
	}

	diffMin := -128 * params.F
	diffMax := 128*params.F - 1
	hev := absInt(p1-p0) > params.H || absInt(q1-q0) > params.H
	if hev {
		f := clampLoopInt(p1-q1, diffMin, diffMax)
		f = clampLoopInt(3*(q0-p0)+f, diffMin, diffMax)
		f1 := minInt(f+4, diffMax) >> 3
		f2 := minInt(f+3, diffMax) >> 3
		buf[p0Idx] = clipByte(p0 + f2)
		buf[q0Idx] = clipByte(q0 - f1)
		return
	}
	f := clampLoopInt(3*(q0-p0), diffMin, diffMax)
	f1 := minInt(f+4, diffMax) >> 3
	f2 := minInt(f+3, diffMax) >> 3
	buf[p0Idx] = clipByte(p0 + f2)
	buf[q0Idx] = clipByte(q0 - f1)
	f = (f1 + 1) >> 1
	buf[p0Idx-step] = clipByte(p1 + f)
	buf[q0Idx+step] = clipByte(q1 - f)
}

func simpleLoopFilterThreshold(level int) int {
	if level <= 0 {
		return 0
	}
	return 4 + level/6
}

func shouldFilterVertical(mask *CDEFIndexMap, edgeMap *loopFilterPlaneEdges, x, y, ssX, ssY int) bool {
	if edgeMap != nil {
		return edgeMap.hasVertical(x>>2, y>>2)
	}
	if mask == nil || mask.MaskW == 0 || mask.MaskH == 0 {
		return true
	}
	lumaX := x << ssX
	lumaY := y << ssY
	y8 := minInt(mask.MaskH-1, maxIntLocal(0, lumaY>>3))
	left8 := maxIntLocal(0, (lumaX-1)>>3)
	right8 := minInt(mask.MaskW-1, lumaX>>3)
	return mask.hasNoSkip(left8, y8) || mask.hasNoSkip(right8, y8)
}

func shouldFilterHorizontal(mask *CDEFIndexMap, edgeMap *loopFilterPlaneEdges, x, y, ssX, ssY int) bool {
	if edgeMap != nil {
		return edgeMap.hasHorizontal(x>>2, y>>2)
	}
	if mask == nil || mask.MaskW == 0 || mask.MaskH == 0 {
		return true
	}
	lumaX := x << ssX
	lumaY := y << ssY
	x8 := minInt(mask.MaskW-1, maxIntLocal(0, lumaX>>3))
	top8 := maxIntLocal(0, (lumaY-1)>>3)
	bottom8 := minInt(mask.MaskH-1, lumaY>>3)
	return mask.hasNoSkip(x8, top8) || mask.hasNoSkip(x8, bottom8)
}

func ternaryEdges(edges *loopFilterEdgeMaps, plane int) *loopFilterPlaneEdges {
	if edges == nil {
		return nil
	}
	switch plane {
	case 0:
		return edges.Y
	case 1:
		return edges.U
	case 2:
		return edges.V
	default:
		return nil
	}
}

func weakFilterPair8(buf []byte, p1Idx, p0Idx, q0Idx, q1Idx, thr int) {
	p1 := int(buf[p1Idx])
	p0 := int(buf[p0Idx])
	q0 := int(buf[q0Idx])
	q1 := int(buf[q1Idx])
	if absInt(p0-q0)*2+absInt(p1-q1)/2 > thr*3 {
		return
	}
	if absInt(p1-p0) > thr || absInt(q1-q0) > thr {
		return
	}
	delta := clampLoopInt((((q0-p0)*3 + (p1 - q1) + 4) >> 3), -thr, thr)
	buf[p0Idx] = clipByte(p0 + delta)
	buf[q0Idx] = clipByte(q0 - delta)
	if thr < 6 {
		return
	}
	delta1 := clampLoopInt((p1+((p0+q0+1)>>1)-2*p1)>>1, -(thr >> 1), thr>>1)
	delta2 := clampLoopInt((q1+((p0+q0+1)>>1)-2*q1)>>1, -(thr >> 1), thr>>1)
	buf[p1Idx] = clipByte(p1 + delta1)
	buf[q1Idx] = clipByte(q1 + delta2)
}

func clampLoopInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
