package decoder

import (
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func applyCDEF420_16(yPlane []uint16, yStride, width, height int, uPlane []uint16, uStride, chromaWidth, chromaHeight int, vPlane []uint16, vStride int, hdr *obu.FrameHeader, bitDepth uint8, cdef *CDEFIndexMap) {
	if hdr == nil || !cdef.valid() || hdr.CDEF.Damping == 0 {
		return
	}
	scratch := takeCDEFScratch16(len(yPlane) + len(uPlane) + len(vPlane))
	if scratch == nil {
		return
	}
	defer putCDEFScratch16(scratch)
	srcY := scratch.buf[:len(yPlane)]
	srcU := scratch.buf[len(yPlane) : len(yPlane)+len(uPlane)]
	srcV := scratch.buf[len(yPlane)+len(uPlane):]
	copy(srcY, yPlane)
	copy(srcU, uPlane)
	copy(srcV, vPlane)
	damping := int(hdr.CDEF.Damping) + maxIntLocal(int(bitDepth)-8, 0)
	for y64 := 0; y64 < cdef.CellsH; y64++ {
		for x64 := 0; x64 < cdef.CellsW; x64++ {
			idx := cdef.at(x64, y64)
			if idx < 0 || int(idx) >= len(hdr.CDEF.YStrength) || int(idx) >= len(hdr.CDEF.UVStrength) {
				continue
			}
			yPri, ySec := cdefStrengths16(hdr.CDEF.YStrength[idx], bitDepth)
			uvPri, uvSec := cdefStrengths16(hdr.CDEF.UVStrength[idx], bitDepth)
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
					dir, variance := cdefFindDir16(srcY, yStride, px, py, bitDepth)
					if yPri != 0 || ySec != 0 {
						adjPri := cdefAdjustStrength(yPri, variance)
						if adjPri != 0 || ySec != 0 {
							cdefFilterBlock16(yPlane, srcY, yStride, width, height, px, py, 8, 8, adjPri, ySec, dir, damping, bitDepth)
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
					cdefFilterBlock16(uPlane, srcU, uStride, chromaWidth, chromaHeight, cx, cy, 4, 4, uvPri, uvSec, uvDir, uvDamping, bitDepth)
					cdefFilterBlock16(vPlane, srcV, vStride, chromaWidth, chromaHeight, cx, cy, 4, 4, uvPri, uvSec, uvDir, uvDamping, bitDepth)
				}
			}
		}
	}
}

func cdefStrengths16(level uint8, bitDepth uint8) (int, int) {
	shift := maxIntLocal(int(bitDepth)-8, 0)
	pri := int(level>>2) << shift
	sec := int(level & 3)
	if sec == 3 {
		sec++
	}
	sec <<= shift
	return pri, sec
}

func cdefFindDir16(src []uint16, stride, x, y int, bitDepth uint8) (int, uint) {
	var partialHV [2][8]int
	var partialDiag [2][15]int
	var partialAlt [4][11]int
	shift := maxIntLocal(int(bitDepth)-8, 0)

	for yy := 0; yy < 8; yy++ {
		row := src[(y+yy)*stride:]
		for xx := 0; xx < 8; xx++ {
			px := int(row[x+xx]>>shift) - 128
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

func cdefFilterBlock16(dst, src []uint16, stride, width, height, x, y, blockW, blockH, priStrength, secStrength, dir, damping int, bitDepth uint8) {
	if x < 0 || y < 0 || x+blockW > width || y+blockH > height {
		return
	}
	var tmp [12 * 12]int16
	cdefPadBlock16(tmp[:], src, stride, width, height, x, y, blockW, blockH)
	priShift := 0
	if priStrength > 0 {
		priShift = maxIntLocal(0, damping-ulog2(priStrength))
	}
	secShift := 0
	if secStrength > 0 {
		secShift = maxIntLocal(0, damping-ulog2(secStrength))
	}
	bitDepthShift := maxIntLocal(int(bitDepth)-8, 0)
	priTap0 := 4
	if ((priStrength >> bitDepthShift) & 1) != 0 {
		priTap0 = 3
	}
	priTap1 := (priTap0 & 3) | 2

	for yy := 0; yy < blockH; yy++ {
		for xx := 0; xx < blockW; xx++ {
			idx := (y+yy)*stride + x + xx
			tidx := (yy+2)*12 + xx + 2
			px := int(tmp[tidx])
			sum := 0
			if priStrength > 0 && secStrength > 0 {
				minV := px
				maxV := px
				for k := 0; k < 2; k++ {
					tap := priTap0
					if k > 0 {
						tap = priTap1
					}
					sum += cdefDirectionalAccumTemp16(tmp[:], tidx, px, cdefPrimary[dir][k], priStrength, priShift, tap, &minV, &maxV)
					sum += cdefSecondaryAccumTemp16(tmp[:], tidx, px, (dir+2)&7, secStrength, secShift, 2-k, &minV, &maxV)
					sum += cdefSecondaryAccumTemp16(tmp[:], tidx, px, (dir+6)&7, secStrength, secShift, 2-k, &minV, &maxV)
				}
				filtered := px + ((sum - boolToInt(sum < 0) + 8) >> 4)
				if filtered < minV {
					filtered = minV
				} else if filtered > maxV {
					filtered = maxV
				}
				dst[idx] = clipSample(filtered, bitDepth)
				continue
			}
			if priStrength > 0 {
				for k := 0; k < 2; k++ {
					tap := priTap0
					if k > 0 {
						tap = priTap1
					}
					sum += cdefDirectionalAccumTempNoClamp16(tmp[:], tidx, px, cdefPrimary[dir][k], priStrength, priShift, tap)
				}
			}
			if secStrength > 0 {
				for k := 0; k < 2; k++ {
					sum += cdefSecondaryAccumTempNoClamp16(tmp[:], tidx, px, (dir+2)&7, secStrength, secShift, 2-k)
					sum += cdefSecondaryAccumTempNoClamp16(tmp[:], tidx, px, (dir+6)&7, secStrength, secShift, 2-k)
				}
			}
			dst[idx] = clipSample(px+((sum-boolToInt(sum < 0)+8)>>4), bitDepth)
		}
	}
}

func cdefPadBlock16(tmp []int16, src []uint16, stride, width, height, x, y, blockW, blockH int) {
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

func cdefDirectionalAccumTemp16(tmp []int16, centerIdx, center int, delta [2]int, strength, shift, tap int, minV, maxV *int) int {
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

func cdefSecondaryAccumTemp16(tmp []int16, centerIdx, center, dir, strength, shift, tap int, minV, maxV *int) int {
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

func cdefDirectionalAccumTempNoClamp16(tmp []int16, centerIdx, center int, delta [2]int, strength, shift, tap int) int {
	sum := 0
	for sign := -1; sign <= 1; sign += 2 {
		off := sign * (delta[1]*12 + delta[0])
		v := int(tmp[centerIdx+off])
		if v == cdefPadSentinel {
			continue
		}
		sum += tap * cdefConstrain(v-center, strength, shift)
	}
	return sum
}

func cdefSecondaryAccumTempNoClamp16(tmp []int16, centerIdx, center, dir, strength, shift, tap int) int {
	sum := 0
	for _, delta := range cdefPrimary[dir] {
		for sign := -1; sign <= 1; sign += 2 {
			off := sign * (delta[1]*12 + delta[0])
			v := int(tmp[centerIdx+off])
			if v == cdefPadSentinel {
				continue
			}
			sum += tap * cdefConstrain(v-center, strength, shift)
		}
	}
	return sum
}

func cdefPrimaryAccum16(src []uint16, stride, width, height, x, y, center int, delta [2]int, strength, shift, tap int, minV, maxV *int) int {
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

func cdefSecondaryAccum16(src []uint16, stride, width, height, x, y, center, dir, strength, shift, tap int, minV, maxV *int) int {
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
