package decoder

import (
	"image"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type CDEFIndexMap struct {
	CellsW int
	CellsH int
	Values []int8
	MaskW  int
	MaskH  int
	NoSkip []bool
}

func NewCDEFIndexMap(width, height int) *CDEFIndexMap {
	cellsW := (width + 63) >> 6
	cellsH := (height + 63) >> 6
	values := make([]int8, cellsW*cellsH)
	for i := range values {
		values[i] = -1
	}
	maskW := (width + 7) >> 3
	maskH := (height + 7) >> 3
	return &CDEFIndexMap{
		CellsW: cellsW,
		CellsH: cellsH,
		Values: values,
		MaskW:  maskW,
		MaskH:  maskH,
		NoSkip: make([]bool, maskW*maskH),
	}
}

func (m *CDEFIndexMap) valid() bool {
	return m != nil && m.CellsW > 0 && m.CellsH > 0 && len(m.Values) >= m.CellsW*m.CellsH
}

func (m *CDEFIndexMap) at(x64, y64 int) int8 {
	if !m.valid() || x64 < 0 || y64 < 0 || x64 >= m.CellsW || y64 >= m.CellsH {
		return -1
	}
	return m.Values[y64*m.CellsW+x64]
}

func (m *CDEFIndexMap) set(x64, y64 int, v int8) {
	if !m.valid() || x64 < 0 || y64 < 0 || x64 >= m.CellsW || y64 >= m.CellsH {
		return
	}
	m.Values[y64*m.CellsW+x64] = v
}

func (m *CDEFIndexMap) ensureBlockIndex(g BlockGeometry, nBits uint8, skip bool, dec intraEntropyDecoder) error {
	if !m.valid() || skip || dec == nil || g.End4X <= g.Start4X || g.End4Y <= g.Start4Y {
		return nil
	}
	x0 := maxIntLocal(0, g.Start4X>>4)
	y0 := maxIntLocal(0, g.Start4Y>>4)
	if x0 >= m.CellsW || y0 >= m.CellsH {
		return nil
	}
	if m.at(x0, y0) >= 0 {
		return nil
	}
	v := int8(0)
	if nBits > 0 {
		v = int8(dec.DecodeBools(uint(nBits)))
	}
	dims := BlockDims(g.Size)
	w4 := int(dims.W4)
	h4 := int(dims.H4)
	if w4 <= 0 {
		w4 = g.Width4()
	}
	if h4 <= 0 {
		h4 = g.Height4()
	}
	xCells := maxIntLocal(1, (w4+15)>>4)
	yCells := maxIntLocal(1, (h4+15)>>4)
	x1 := minInt(m.CellsW-1, x0+xCells-1)
	y1 := minInt(m.CellsH-1, y0+yCells-1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			m.set(x, y, v)
		}
	}
	return nil
}

func (m *CDEFIndexMap) markNoSkip(g BlockGeometry, skip bool) {
	if !m.valid() || skip || m.MaskW <= 0 || m.MaskH <= 0 || len(m.NoSkip) < m.MaskW*m.MaskH {
		return
	}
	x0 := maxIntLocal(0, g.Start4X>>1)
	y0 := maxIntLocal(0, g.Start4Y>>1)
	x1 := minInt(m.MaskW-1, (g.End4X-1)>>1)
	y1 := minInt(m.MaskH-1, (g.End4Y-1)>>1)
	for y := y0; y <= y1; y++ {
		row := y * m.MaskW
		for x := x0; x <= x1; x++ {
			m.NoSkip[row+x] = true
		}
	}
}

func (m *CDEFIndexMap) hasNoSkip(x8, y8 int) bool {
	if m == nil || x8 < 0 || y8 < 0 || x8 >= m.MaskW || y8 >= m.MaskH || len(m.NoSkip) < m.MaskW*m.MaskH {
		return false
	}
	return m.NoSkip[y8*m.MaskW+x8]
}

func (m *CDEFIndexMap) cellHasNoSkip(x64, y64 int) bool {
	if !m.valid() || m.MaskW <= 0 || m.MaskH <= 0 || len(m.NoSkip) < m.MaskW*m.MaskH {
		return false
	}
	x0 := x64 * 8
	y0 := y64 * 8
	x1 := minInt(m.MaskW, x0+8)
	y1 := minInt(m.MaskH, y0+8)
	for y := y0; y < y1; y++ {
		row := y * m.MaskW
		for x := x0; x < x1; x++ {
			if m.NoSkip[row+x] {
				return true
			}
		}
	}
	return false
}

func (m *CDEFIndexMap) readSuperBlockIndexes(g BlockGeometry, nBits uint8, dec intraEntropyDecoder) error {
	if !m.valid() || dec == nil || g.End4X <= g.Start4X || g.End4Y <= g.Start4Y {
		return nil
	}
	x0 := maxIntLocal(0, g.Start4X>>4)
	y0 := maxIntLocal(0, g.Start4Y>>4)
	x1 := minInt(m.CellsW-1, (g.End4X-1)>>4)
	y1 := minInt(m.CellsH-1, (g.End4Y-1)>>4)
	if x1 < x0 || y1 < y0 {
		return nil
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if !m.cellHasNoSkip(x, y) {
				m.set(x, y, -1)
				continue
			}
			v := int8(0)
			if nBits > 0 {
				v = int8(dec.DecodeBools(uint(nBits)))
			}
			m.set(x, y, v)
		}
	}
	return nil
}

func applyCDEF420_8(img *image.YCbCr, hdr *obu.FrameHeader, cdef *CDEFIndexMap) {
	if img == nil || hdr == nil || !cdef.valid() || hdr.CDEF.Damping == 0 {
		return
	}
	scratch := takeCDEFScratch8(len(img.Y) + len(img.Cb) + len(img.Cr))
	if scratch == nil {
		return
	}
	defer putCDEFScratch8(scratch)
	srcY := scratch.buf[:len(img.Y)]
	srcU := scratch.buf[len(img.Y) : len(img.Y)+len(img.Cb)]
	srcV := scratch.buf[len(img.Y)+len(img.Cb):]
	copy(srcY, img.Y)
	copy(srcU, img.Cb)
	copy(srcV, img.Cr)
	width := parsedPlaneWidth(img)
	height := parsedPlaneHeight(img)
	chromaWidth := parsedChromaPlaneWidth(img)
	chromaHeight := parsedChromaPlaneHeight(img)
	damping := int(hdr.CDEF.Damping)
	for y64 := 0; y64 < cdef.CellsH; y64++ {
		for x64 := 0; x64 < cdef.CellsW; x64++ {
			idx := cdef.at(x64, y64)
			if idx < 0 || int(idx) >= len(hdr.CDEF.YStrength) || int(idx) >= len(hdr.CDEF.UVStrength) {
				continue
			}
			yStrength := hdr.CDEF.YStrength[idx]
			yPri, ySec := cdefStrengths8(yStrength)
			uvStrength := hdr.CDEF.UVStrength[idx]
			uvPri, uvSec := cdefStrengths8(uvStrength)
			if yPri == 0 && ySec == 0 && uvPri == 0 && uvSec == 0 {
				continue
			}
			cellX := x64 << 6
			cellY := y64 << 6
			cellW := minInt(64, width-cellX)
			cellH := minInt(64, height-cellY)
			for by := 0; by+8 <= cellH; by += 8 {
				for bx := 0; bx+8 <= cellW; bx += 8 {
					px := cellX + bx
					py := cellY + by
					if !cdef.hasNoSkip(px>>3, py>>3) {
						continue
					}
					dir, variance := cdefFindDir8(srcY, img.YStride, px, py)
					if yPri != 0 || ySec != 0 {
						adjPri := cdefAdjustStrength(yPri, variance)
						if adjPri != 0 || ySec != 0 {
							cdefFilterBlock(img.Y, srcY, img.YStride, width, height, px, py, 8, 8, adjPri, ySec, dir, damping)
						}
					}
					if uvPri == 0 && uvSec == 0 {
						continue
					}
					cx := px >> 1
					cy := py >> 1
					if cx+4 > chromaWidth || cy+4 > chromaHeight {
						continue
					}
					uvDir := cdefUVDir(av1.Chroma420, uvPri, dir)
					uvDamping := cdefUVDamping(damping)
					cdefFilterBlock(img.Cb, srcU, img.CStride, chromaWidth, chromaHeight, cx, cy, 4, 4, uvPri, uvSec, uvDir, uvDamping)
					cdefFilterBlock(img.Cr, srcV, img.CStride, chromaWidth, chromaHeight, cx, cy, 4, 4, uvPri, uvSec, uvDir, uvDamping)
				}
			}
		}
	}
}

var cdefUVDirs422 = [8]int{7, 0, 2, 4, 5, 6, 6, 6}

func cdefUVDir(layout av1.ChromaSubsampling, priStrength, dir int) int {
	if priStrength == 0 {
		return 0
	}
	switch layout {
	case av1.Chroma422:
		return cdefUVDirs422[dir&7]
	default:
		return dir & 7
	}
}

func cdefUVDamping(damping int) int {
	return maxIntLocal(0, damping-1)
}

func cdefStrengths8(level uint8) (int, int) {
	pri := int(level >> 2)
	sec := int(level & 3)
	if sec == 3 {
		sec++
	}
	return pri, sec
}

func cdefAdjustStrength(strength int, variance uint) int {
	if strength == 0 || variance == 0 {
		return 0
	}
	i := 0
	if variance>>6 != 0 {
		i = minInt(ulog2(int(variance>>6)), 12)
	}
	return (strength*(4+i) + 8) >> 4
}

var cdefPrimary = [8][2][2]int{
	{{1, -1}, {2, -2}},
	{{1, 0}, {2, -1}},
	{{1, 0}, {2, 0}},
	{{1, 0}, {2, 1}},
	{{1, 1}, {2, 2}},
	{{0, 1}, {1, 2}},
	{{0, 1}, {0, 2}},
	{{0, 1}, {-1, 2}},
}

var cdefPrimaryOffsets = [8][2]int{
	{1 + -1*12, 2 + -2*12},
	{1, 2 + -1*12},
	{1, 2},
	{1, 2 + 1*12},
	{1 + 1*12, 2 + 2*12},
	{12, 1 + 2*12},
	{12, 2 * 12},
	{12, -1 + 2*12},
}

const cdefPadSentinel = -32768

func cdefFindDir8(src []byte, stride, x, y int) (int, uint) {
	var partialHV [2][8]int
	var partialDiag [2][15]int
	var partialAlt [4][11]int

	for yy := 0; yy < 8; yy++ {
		row := src[(y+yy)*stride:]
		for xx := 0; xx < 8; xx++ {
			px := int(row[x+xx]) - 128
			partialDiag[0][yy+xx] += px
			partialAlt[0][yy+(xx>>1)] += px
			partialHV[0][yy] += px
			partialAlt[1][3+yy-(xx>>1)] += px
			partialDiag[1][7+yy-xx] += px
			partialAlt[2][3-(yy>>1)+xx] += px
			partialHV[1][xx] += px
			partialAlt[3][(yy>>1)+xx] += px
		}
	}

	var cost [8]uint
	for n := 0; n < 8; n++ {
		cost[2] += uint(partialHV[0][n] * partialHV[0][n])
		cost[6] += uint(partialHV[1][n] * partialHV[1][n])
	}
	cost[2] *= 105
	cost[6] *= 105

	divTable := [7]int{840, 420, 280, 210, 168, 140, 120}
	for n := 0; n < 7; n++ {
		d := uint(divTable[n])
		cost[0] += uint(partialDiag[0][n]*partialDiag[0][n]+partialDiag[0][14-n]*partialDiag[0][14-n]) * d
		cost[4] += uint(partialDiag[1][n]*partialDiag[1][n]+partialDiag[1][14-n]*partialDiag[1][14-n]) * d
	}
	cost[0] += uint(partialDiag[0][7] * partialDiag[0][7] * 105)
	cost[4] += uint(partialDiag[1][7] * partialDiag[1][7] * 105)

	for n := 0; n < 4; n++ {
		costIdx := n*2 + 1
		for m := 0; m < 5; m++ {
			cost[costIdx] += uint(partialAlt[n][3+m] * partialAlt[n][3+m])
		}
		cost[costIdx] *= 105
		for m := 0; m < 3; m++ {
			d := uint(divTable[2*m+1])
			cost[costIdx] += uint(partialAlt[n][m]*partialAlt[n][m]+partialAlt[n][10-m]*partialAlt[n][10-m]) * d
		}
	}

	bestDir := 0
	bestCost := cost[0]
	for n := 1; n < 8; n++ {
		if cost[n] > bestCost {
			bestCost = cost[n]
			bestDir = n
		}
	}
	return bestDir, (bestCost - cost[bestDir^4]) >> 10
}

func cdefFilterBlock(dst, src []byte, stride, width, height, x, y, blockW, blockH, priStrength, secStrength, dir, damping int) {
	if x < 0 || y < 0 || x+blockW > width || y+blockH > height {
		return
	}
	var tmp [12 * 12]int16
	cdefPadBlock8(tmp[:], src, stride, width, height, x, y, blockW, blockH)
	priShift := 0
	if priStrength > 0 {
		priShift = maxIntLocal(0, damping-ulog2(priStrength))
	}
	secShift := 0
	if secStrength > 0 {
		secShift = maxIntLocal(0, damping-ulog2(secStrength))
	}

	priTap0 := 4
	if priStrength&1 != 0 {
		priTap0 = 3
	}
	priTap1 := (priTap0 & 3) | 2
	priOff0 := cdefPrimaryOffsets[dir][0]
	priOff1 := cdefPrimaryOffsets[dir][1]
	secOff20 := cdefPrimaryOffsets[(dir+2)&7][0]
	secOff21 := cdefPrimaryOffsets[(dir+2)&7][1]
	secOff60 := cdefPrimaryOffsets[(dir+6)&7][0]
	secOff61 := cdefPrimaryOffsets[(dir+6)&7][1]

	for yy := 0; yy < blockH; yy++ {
		for xx := 0; xx < blockW; xx++ {
			idx := (y+yy)*stride + x + xx
			tidx := (yy+2)*12 + xx + 2
			px := int(tmp[tidx])
			sum := 0
			if priStrength > 0 && secStrength > 0 {
				minV := px
				maxV := px
				sum += cdefAccumPairTempClamp(tmp[:], tidx, px, priOff0, priStrength, priShift, priTap0, &minV, &maxV)
				sum += cdefAccumSecondaryTempClamp(tmp[:], tidx, px, secOff20, secOff21, secStrength, secShift, 3, &minV, &maxV)
				sum += cdefAccumSecondaryTempClamp(tmp[:], tidx, px, secOff60, secOff61, secStrength, secShift, 3, &minV, &maxV)
				sum += cdefAccumPairTempClamp(tmp[:], tidx, px, priOff1, priStrength, priShift, priTap1, &minV, &maxV)
				filtered := px + ((sum - boolToInt(sum < 0) + 8) >> 4)
				if filtered < minV {
					filtered = minV
				} else if filtered > maxV {
					filtered = maxV
				}
				dst[idx] = clipByte(filtered)
				continue
			}
			if priStrength > 0 {
				sum += cdefAccumPairTempNoClamp(tmp[:], tidx, px, priOff0, priStrength, priShift, priTap0)
				sum += cdefAccumPairTempNoClamp(tmp[:], tidx, px, priOff1, priStrength, priShift, priTap1)
			}
			if secStrength > 0 {
				sum += cdefAccumSecondaryTempNoClamp(tmp[:], tidx, px, secOff20, secOff21, secStrength, secShift, 3)
				sum += cdefAccumSecondaryTempNoClamp(tmp[:], tidx, px, secOff60, secOff61, secStrength, secShift, 3)
			}
			dst[idx] = clipByte(px + ((sum - boolToInt(sum < 0) + 8) >> 4))
		}
	}
}

func cdefPadBlock8(tmp []int16, src []byte, stride, width, height, x, y, blockW, blockH int) {
	for i := range tmp {
		tmp[i] = cdefPadSentinel
	}
	for yy := -2; yy < blockH+2; yy++ {
		sy := y + yy
		if sy < 0 || sy >= height {
			continue
		}
		for xx := -2; xx < blockW+2; xx++ {
			sx := x + xx
			if sx < 0 || sx >= width {
				continue
			}
			tmp[(yy+2)*12+(xx+2)] = int16(src[sy*stride+sx])
		}
	}
}

func cdefDirectionalAccumTemp(tmp []int16, centerIdx, center int, delta [2]int, strength, shift, tap int, minV, maxV *int) int {
	sum := 0
	for sign := -1; sign <= 1; sign += 2 {
		off := sign * (delta[1]*12 + delta[0])
		v := int(tmp[centerIdx+off])
		if v == cdefPadSentinel {
			continue
		}
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	return sum
}

func cdefSecondaryAccumTemp(tmp []int16, centerIdx, center, dir, strength, shift, tap int, minV, maxV *int) int {
	sum := 0
	for _, delta := range cdefPrimary[dir] {
		for sign := -1; sign <= 1; sign += 2 {
			off := sign * (delta[1]*12 + delta[0])
			v := int(tmp[centerIdx+off])
			if v == cdefPadSentinel {
				continue
			}
			sum += tap * cdefConstrain(v-center, strength, shift)
			if v < *minV {
				*minV = v
			}
			if v > *maxV {
				*maxV = v
			}
		}
	}
	return sum
}

func cdefDirectionalAccumTempNoClamp(tmp []int16, centerIdx, center int, delta [2]int, strength, shift, tap int) int {
	return cdefAccumPairTempNoClamp(tmp, centerIdx, center, delta[1]*12+delta[0], strength, shift, tap)
}

func cdefAccumPairTempNoClamp(tmp []int16, centerIdx, center, off, strength, shift, tap int) int {
	sum := 0
	v := int(tmp[centerIdx+off])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	v = int(tmp[centerIdx-off])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	return sum
}

func cdefSecondaryAccumTempNoClamp(tmp []int16, centerIdx, center, dir, strength, shift, tap int) int {
	return cdefAccumSecondaryTempNoClamp(tmp, centerIdx, center, cdefPrimaryOffsets[dir][0], cdefPrimaryOffsets[dir][1], strength, shift, tap)
}

func cdefAccumSecondaryTempNoClamp(tmp []int16, centerIdx, center, off0, off1, strength, shift, tap int) int {
	sum := 0
	v := int(tmp[centerIdx+off0])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	v = int(tmp[centerIdx-off0])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	v = int(tmp[centerIdx+off1])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	v = int(tmp[centerIdx-off1])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	return sum
}

func cdefAccumPairTempClamp(tmp []int16, centerIdx, center, off, strength, shift, tap int, minV, maxV *int) int {
	sum := 0
	v := int(tmp[centerIdx+off])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	v = int(tmp[centerIdx-off])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	return sum
}

func cdefAccumSecondaryTempClamp(tmp []int16, centerIdx, center, off0, off1, strength, shift, tap int, minV, maxV *int) int {
	sum := 0
	v := int(tmp[centerIdx+off0])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	v = int(tmp[centerIdx-off0])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	v = int(tmp[centerIdx+off1])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	v = int(tmp[centerIdx-off1])
	if v != cdefPadSentinel {
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	return sum
}

func cdefPrimaryAccum(src []byte, stride, width, height, x, y, center int, delta [2]int, strength, shift, tap int, minV, maxV *int) int {
	sum := 0
	for sign := -1; sign <= 1; sign += 2 {
		sx := x + delta[0]*sign
		sy := y + delta[1]*sign
		if sx < 0 || sy < 0 || sx >= width || sy >= height {
			continue
		}
		v := int(src[sy*stride+sx])
		sum += tap * cdefConstrain(v-center, strength, shift)
		if v < *minV {
			*minV = v
		}
		if v > *maxV {
			*maxV = v
		}
	}
	return sum
}

func cdefSecondaryAccum(src []byte, stride, width, height, x, y, center, dir, strength, shift, tap int, minV, maxV *int) int {
	sum := 0
	for _, delta := range cdefPrimary[dir] {
		for sign := -1; sign <= 1; sign += 2 {
			sx := x + delta[0]*sign
			sy := y + delta[1]*sign
			if sx < 0 || sy < 0 || sx >= width || sy >= height {
				continue
			}
			v := int(src[sy*stride+sx])
			sum += tap * cdefConstrain(v-center, strength, shift)
			if v < *minV {
				*minV = v
			}
			if v > *maxV {
				*maxV = v
			}
		}
	}
	return sum
}

func cdefConstrain(diff, strength, shift int) int {
	adiff := absInt(diff)
	limited := minInt(adiff, maxIntLocal(0, strength-(adiff>>shift)))
	if diff < 0 {
		return -limited
	}
	return limited
}
