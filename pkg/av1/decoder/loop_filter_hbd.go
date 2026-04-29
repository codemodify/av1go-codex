package decoder

import "github.com/codemodify/av1go-codex/pkg/av1/obu"

func applySimpleLoopFilter420_16(yPlane []uint16, yStride, width, height int, uPlane []uint16, uStride, chromaWidth, chromaHeight int, vPlane []uint16, vStride int, hdr *obu.FrameHeader, bitDepth uint8) {
	applySimpleLoopFilter420_16Masked(yPlane, yStride, width, height, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, hdr, bitDepth, nil)
}

func applySimpleLoopFilter420_16Masked(yPlane []uint16, yStride, width, height int, uPlane []uint16, uStride, chromaWidth, chromaHeight int, vPlane []uint16, vStride int, hdr *obu.FrameHeader, bitDepth uint8, mask *CDEFIndexMap) {
	applyLoopFilter420_16MaskedWithLevels(yPlane, yStride, width, height, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, hdr, bitDepth, nil, nil, mask)
}

func applyAdaptiveLoopFilter420_16Masked(yPlane []uint16, yStride, width, height int, uPlane []uint16, uStride, chromaWidth, chromaHeight int, vPlane []uint16, vStride int, hdr *obu.FrameHeader, bitDepth uint8, field *SpatialMVTile, seg *SegmentationMap, edges *loopFilterEdgeMaps, mask *CDEFIndexMap) {
	levels := buildLoopFilterLevelMap(hdr, field, seg)
	defer putLoopFilterLevelMap(levels)
	applyLoopFilter420_16MaskedWithLevels(yPlane, yStride, width, height, uPlane, uStride, chromaWidth, chromaHeight, vPlane, vStride, hdr, bitDepth, levels, edges, mask)
}

func applyLoopFilter420_16MaskedWithLevels(yPlane []uint16, yStride, width, height int, uPlane []uint16, uStride, chromaWidth, chromaHeight int, vPlane []uint16, vStride int, hdr *obu.FrameHeader, bitDepth uint8, levels *loopFilterLevelMap, edges *loopFilterEdgeMaps, mask *CDEFIndexMap) {
	if hdr == nil {
		return
	}
	if lvl := int(hdr.LoopFilter.LevelY[0]); lvl > 0 {
		deblockPlaneVertical16(yPlane, yStride, width, height, lvl, hdr.LoopFilter.Sharpness, bitDepth, false, mask, 0, 0, levels, 0, ternaryEdges(edges, 0))
	}
	if lvl := int(hdr.LoopFilter.LevelY[1]); lvl > 0 {
		deblockPlaneHorizontal16(yPlane, yStride, width, height, lvl, hdr.LoopFilter.Sharpness, bitDepth, false, mask, 0, 0, levels, 1, ternaryEdges(edges, 0))
	}
	if lvl := int(hdr.LoopFilter.LevelU); lvl > 0 {
		deblockPlaneVertical16(uPlane, uStride, chromaWidth, chromaHeight, lvl, hdr.LoopFilter.Sharpness, bitDepth, true, mask, 1, 1, levels, 2, ternaryEdges(edges, 1))
		deblockPlaneHorizontal16(uPlane, uStride, chromaWidth, chromaHeight, lvl, hdr.LoopFilter.Sharpness, bitDepth, true, mask, 1, 1, levels, 2, ternaryEdges(edges, 1))
	}
	if lvl := int(hdr.LoopFilter.LevelV); lvl > 0 {
		deblockPlaneVertical16(vPlane, vStride, chromaWidth, chromaHeight, lvl, hdr.LoopFilter.Sharpness, bitDepth, true, mask, 1, 1, levels, 3, ternaryEdges(edges, 2))
		deblockPlaneHorizontal16(vPlane, vStride, chromaWidth, chromaHeight, lvl, hdr.LoopFilter.Sharpness, bitDepth, true, mask, 1, 1, levels, 3, ternaryEdges(edges, 2))
	}
}

func deblockPlaneVertical16(plane []uint16, stride, width, height, level int, sharpness, bitDepth uint8, chroma bool, mask *CDEFIndexMap, ssX, ssY int, levels *loopFilterLevelMap, levelPlane int, edgeMap *loopFilterPlaneEdges) {
	if stride <= 0 || width < 4 || height <= 0 {
		return
	}
	params := calcLoopFilterParams(level, sharpness, bitDepth)
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
					params = calcLoopFilterParams(edgeLevel, sharpness, bitDepth)
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
			loopFilterEdge16(row, x-1, 1, params, wd, bitDepth)
		}
	}
}

func deblockPlaneHorizontal16(plane []uint16, stride, width, height, level int, sharpness, bitDepth uint8, chroma bool, mask *CDEFIndexMap, ssX, ssY int, levels *loopFilterLevelMap, levelPlane int, edgeMap *loopFilterPlaneEdges) {
	if stride <= 0 || width <= 0 || height < 4 {
		return
	}
	params := calcLoopFilterParams(level, sharpness, bitDepth)
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
					params = calcLoopFilterParams(edgeLevel, sharpness, bitDepth)
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
			loopFilterEdge16(plane, (y-1)*stride+x, stride, params, wd, bitDepth)
		}
	}
}

func simpleLoopFilterThresholdHBD(level int, bitDepth uint8) int {
	thr := simpleLoopFilterThreshold(level)
	if bitDepth <= 8 {
		return thr
	}
	return thr << (bitDepth - 8)
}

func weakFilterPair16(buf []uint16, p1Idx, p0Idx, q0Idx, q1Idx, thr int, bitDepth uint8) {
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
	buf[p0Idx] = clipSample(p0+delta, bitDepth)
	buf[q0Idx] = clipSample(q0-delta, bitDepth)
	if thr < (6 << maxIntLocal(int(bitDepth)-8, 0)) {
		return
	}
	delta1 := clampLoopInt((p1+((p0+q0+1)>>1)-2*p1)>>1, -(thr >> 1), thr>>1)
	delta2 := clampLoopInt((q1+((p0+q0+1)>>1)-2*q1)>>1, -(thr >> 1), thr>>1)
	buf[p1Idx] = clipSample(p1+delta1, bitDepth)
	buf[q1Idx] = clipSample(q1+delta2, bitDepth)
}

func loopFilterEdge16(buf []uint16, p0Idx, step int, params loopFilterParams, wd int, bitDepth uint8) {
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
			buf[p0Idx-5*step] = clipSample((p6+p6+p6+p6+p6+2*p6+2*p5+2*p4+p3+p2+p1+p0+q0+8)>>4, bitDepth)
			buf[p0Idx-4*step] = clipSample((p6+p6+p6+p6+p6+2*p5+2*p4+2*p3+p2+p1+p0+q0+q1+8)>>4, bitDepth)
			buf[p0Idx-3*step] = clipSample((p6+p6+p6+p6+p5+2*p4+2*p3+2*p2+p1+p0+q0+q1+q2+8)>>4, bitDepth)
			buf[p0Idx-2*step] = clipSample((p6+p6+p6+p5+p4+2*p3+2*p2+2*p1+p0+q0+q1+q2+q3+8)>>4, bitDepth)
			buf[p0Idx-1*step] = clipSample((p6+p6+p5+p4+p3+2*p2+2*p1+2*p0+q0+q1+q2+q3+q4+8)>>4, bitDepth)
			buf[p0Idx] = clipSample((p6+p5+p4+p3+p2+2*p1+2*p0+2*q0+q1+q2+q3+q4+q5+8)>>4, bitDepth)
			buf[q0Idx] = clipSample((p5+p4+p3+p2+p1+2*p0+2*q0+2*q1+q2+q3+q4+q5+q6+8)>>4, bitDepth)
			buf[q0Idx+step] = clipSample((p4+p3+p2+p1+p0+2*q0+2*q1+2*q2+q3+q4+q5+q6+q6+8)>>4, bitDepth)
			buf[q0Idx+2*step] = clipSample((p3+p2+p1+p0+q0+2*q1+2*q2+2*q3+q4+q5+q6+q6+q6+8)>>4, bitDepth)
			buf[q0Idx+3*step] = clipSample((p2+p1+p0+q0+q1+2*q2+2*q3+2*q4+q5+q6+q6+q6+q6+8)>>4, bitDepth)
			buf[q0Idx+4*step] = clipSample((p1+p0+q0+q1+q2+2*q3+2*q4+2*q5+q6+q6+q6+q6+q6+8)>>4, bitDepth)
			buf[q0Idx+5*step] = clipSample((p0+q0+q1+q2+q3+2*q4+2*q5+2*q6+q6+q6+q6+q6+q6+8)>>4, bitDepth)
			return
		}
	}
	if wd >= 8 && flat8in {
		buf[p0Idx-2*step] = clipSample((p3+p3+p3+2*p2+p1+p0+q0+4)>>3, bitDepth)
		buf[p0Idx-step] = clipSample((p3+p3+p2+2*p1+p0+q0+q1+4)>>3, bitDepth)
		buf[p0Idx] = clipSample((p3+p2+p1+2*p0+q0+q1+q2+4)>>3, bitDepth)
		buf[q0Idx] = clipSample((p2+p1+p0+2*q0+q1+q2+q3+4)>>3, bitDepth)
		buf[q0Idx+step] = clipSample((p1+p0+q0+2*q1+q2+q3+q3+4)>>3, bitDepth)
		buf[q0Idx+2*step] = clipSample((p0+q0+q1+2*q2+q3+q3+q3+4)>>3, bitDepth)
		return
	}
	if wd == 6 && flat8in {
		buf[p0Idx-step] = clipSample((p2+2*p2+2*p1+2*p0+q0+4)>>3, bitDepth)
		buf[p0Idx] = clipSample((p2+2*p1+2*p0+2*q0+q1+4)>>3, bitDepth)
		buf[q0Idx] = clipSample((p1+2*p0+2*q0+2*q1+q2+4)>>3, bitDepth)
		buf[q0Idx+step] = clipSample((p0+2*q0+2*q1+2*q2+q2+4)>>3, bitDepth)
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
		buf[p0Idx] = clipSample(p0+f2, bitDepth)
		buf[q0Idx] = clipSample(q0-f1, bitDepth)
		return
	}
	f := clampLoopInt(3*(q0-p0), diffMin, diffMax)
	f1 := minInt(f+4, diffMax) >> 3
	f2 := minInt(f+3, diffMax) >> 3
	buf[p0Idx] = clipSample(p0+f2, bitDepth)
	buf[q0Idx] = clipSample(q0-f1, bitDepth)
	f = (f1 + 1) >> 1
	buf[p0Idx-step] = clipSample(p1+f, bitDepth)
	buf[q0Idx+step] = clipSample(q1-f, bitDepth)
}
