package decoder

import (
	"fmt"
	"math"
)

type affineSample struct {
	x float64
	y float64
	u float64
	v float64
}

type affineWarp struct {
	a00    float64
	a01    float64
	a02    float64
	a10    float64
	a11    float64
	a12    float64
	matrix [6]int32
	abcd   [4]int16
	fixed  bool
}

type localWarpPoint struct {
	inX  int
	inY  int
	outX int
	outY int
}

func roundFloatToInt(v float64) int {
	if v >= 0 {
		return int(v + 0.5)
	}
	return int(v - 0.5)
}

func translationWarp(mv MotionVector) affineWarp {
	return affineWarp{
		a00: 1, a01: 0, a02: float64(mv.X) / 8.0,
		a10: 0, a11: 1, a12: float64(mv.Y) / 8.0,
	}
}

func (w affineWarp) mapPoint(x, y float64) (float64, float64) {
	return w.a00*x + w.a01*y + w.a02, w.a10*x + w.a11*y + w.a12
}

func deriveLocalAffineWarp(mvTile *SpatialMVTile, g BlockGeometry, ref0 int8, currentMV MotionVector, edge intraEdgeAvailability) (affineWarp, bool) {
	base := translationWarp(currentMV)
	if mvTile == nil || ref0 < 0 {
		return base, false
	}
	var pointsBuf [8]localWarpPoint
	points := collectLocalWarpPointsInto(pointsBuf[:0], mvTile, g, ref0, edge)
	if len(points) == 0 {
		return base, false
	}
	warp, ok := findAffineWarpInt(points, g.Width4(), g.Height4(), currentMV, g.Start4X, g.Start4Y)
	if !ok {
		return base, false
	}
	return warp, true
}

func collectLocalWarpPointsInto(points []localWarpPoint, mvTile *SpatialMVTile, g BlockGeometry, ref0 int8, edge intraEdgeAvailability) []localWarpPoint {
	points = points[:0]
	if mvTile == nil {
		return points
	}
	if g.Start4Y > mvTile.Geometry.Start4Y {
		for off := 0; off < g.Width4() && len(points) < cap(points); {
			b := mvTile.block(g.Start4X+off, g.Start4Y-1)
			if matchingSingleRefBlock(b, ref0) {
				sampleOff := off
				if off == 0 {
					if inner := blockInnerOffset4(g.Start4X, b, true); inner > 0 {
						sampleOff = -inner
					}
				}
				points = appendLocalWarpPoint(points, sampleOff, 0, 1, -1, b)
			}
			off += localWarpNeighborStep4(g.Start4X, off, b, true)
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X {
		for off := 0; off < g.Height4() && len(points) < cap(points); {
			b := mvTile.block(g.Start4X-1, g.Start4Y+off)
			if matchingSingleRefBlock(b, ref0) {
				sampleOff := off
				if off == 0 {
					if inner := blockInnerOffset4(g.Start4Y, b, false); inner > 0 {
						sampleOff = -inner
					}
				}
				points = appendLocalWarpPoint(points, 0, sampleOff, -1, 1, b)
			}
			off += localWarpNeighborStep4(g.Start4Y, off, b, false)
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X && g.Start4Y > mvTile.Geometry.Start4Y && len(points) < cap(points) {
		if b := mvTile.block(g.Start4X-1, g.Start4Y-1); matchingSingleRefBlock(b, ref0) {
			points = appendLocalWarpPoint(points, 0, 0, -1, -1, b)
		}
	}
	if edge.topHasRight && g.Start4Y > mvTile.Geometry.Start4Y && g.End4X < mvTile.Geometry.End4X && len(points) < cap(points) {
		if b := mvTile.block(g.End4X, g.Start4Y-1); matchingSingleRefBlock(b, ref0) {
			points = appendLocalWarpPoint(points, g.Width4(), 0, 1, -1, b)
		}
	}
	return points
}

func appendLocalWarpPoint(points []localWarpPoint, dx, dy, sx, sy int, b *SpatialMVBlock) []localWarpPoint {
	if b == nil || len(points) >= cap(points) {
		return points
	}
	dims := BlockDims(b.Size)
	inX := 16*(2*dx+sx*int(dims.W4)) - 8
	inY := 16*(2*dy+sy*int(dims.H4)) - 8
	return append(points, localWarpPoint{
		inX:  inX,
		inY:  inY,
		outX: inX + b.MV[0].X,
		outY: inY + b.MV[0].Y,
	})
}

func blockInnerOffset4(pos4 int, b *SpatialMVBlock, horizontal bool) int {
	if b == nil {
		return 0
	}
	dims := BlockDims(b.Size)
	size4 := int(dims.H4)
	if horizontal {
		size4 = int(dims.W4)
	}
	if size4 <= 1 {
		return 0
	}
	return pos4 & (size4 - 1)
}

func localWarpNeighborStep4(start4, off int, b *SpatialMVBlock, horizontal bool) int {
	step := blockStep4(b, horizontal)
	if off == 0 {
		if inner := blockInnerOffset4(start4, b, horizontal); inner > 0 && inner < step {
			step -= inner
		}
	}
	return maxIntLocal(1, step)
}

func findAffineWarpInt(points []localWarpPoint, bw4, bh4 int, mv MotionVector, bx4, by4 int) (affineWarp, bool) {
	if len(points) == 0 || bw4 <= 0 || bh4 <= 0 {
		return affineWarp{}, false
	}

	filtered := points[:0]
	thresh := 4 * clampInt(maxIntLocal(bw4, bh4), 4, 28)
	for _, p := range points {
		mvd := absInt(p.outX-p.inX-mv.X) + absInt(p.outY-p.inY-mv.Y)
		if mvd <= thresh {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		filtered = points[:1]
	}

	var a00, a01, a11 int
	var bx0, bx1, by0, by1 int
	rsuy := 2*bh4 - 1
	rsux := 2*bw4 - 1
	suy := rsuy * 8
	sux := rsux * 8
	duy := suy + mv.Y
	dux := sux + mv.X
	isuy := by4*4 + rsuy
	isux := bx4*4 + rsux

	used := 0
	for _, p := range filtered {
		dx := p.outX - dux
		dy := p.outY - duy
		sx := p.inX - sux
		sy := p.inY - suy
		if absInt(sx-dx) >= 256 || absInt(sy-dy) >= 256 {
			continue
		}
		a00 += ((sx * sx) >> 2) + sx*2 + 8
		a01 += ((sx * sy) >> 2) + sx + sy + 4
		a11 += ((sy * sy) >> 2) + sy*2 + 8
		bx0 += ((sx * dx) >> 2) + sx + dx + 8
		bx1 += ((sy * dx) >> 2) + sy + dx + 4
		by0 += ((sx * dy) >> 2) + sx + dy + 4
		by1 += ((sy * dy) >> 2) + sy + dy + 8
		used++
	}
	if used == 0 {
		return affineWarp{}, false
	}

	det := int64(a00)*int64(a11) - int64(a01)*int64(a01)
	if det == 0 {
		return affineWarp{}, false
	}

	var mat [6]int32
	mat[2] = int32(clipInt(int(roundDivSigned64((int64(a11)*int64(bx0)-int64(a01)*int64(bx1))<<16, det)), 0xe001, 0x11fff))
	mat[3] = int32(clipInt(int(roundDivSigned64((int64(a00)*int64(bx1)-int64(a01)*int64(bx0))<<16, det)), -0x1fff, 0x1fff))
	mat[4] = int32(clipInt(int(roundDivSigned64((int64(a11)*int64(by0)-int64(a01)*int64(by1))<<16, det)), -0x1fff, 0x1fff))
	mat[5] = int32(clipInt(int(roundDivSigned64((int64(a00)*int64(by1)-int64(a01)*int64(by0))<<16, det)), 0xe001, 0x11fff))
	mat[0] = int32(clipInt(mv.X*0x2000-(isux*(int(mat[2])-0x10000)+isuy*int(mat[3])), -0x800000, 0x7fffff))
	mat[1] = int32(clipInt(mv.Y*0x2000-(isux*int(mat[4])+isuy*(int(mat[5])-0x10000)), -0x800000, 0x7fffff))

	return affineWarpFromMatrix(mat)
}

func affineWarpFromMatrix(mat [6]int32) (affineWarp, bool) {
	abcd, ok := shearParamsFromMatrix(mat)
	if !ok {
		return affineWarp{}, false
	}
	return affineWarp{
		a00:    float64(mat[2]) / 65536.0,
		a01:    float64(mat[3]) / 65536.0,
		a02:    float64(mat[0]) / 65536.0,
		a10:    float64(mat[4]) / 65536.0,
		a11:    float64(mat[5]) / 65536.0,
		a12:    float64(mat[1]) / 65536.0,
		matrix: mat,
		abcd:   abcd,
		fixed:  true,
	}, true
}

func shearParamsFromMatrix(mat [6]int32) ([4]int16, bool) {
	if mat[2] <= 0 {
		return [4]int16{}, false
	}
	alpha := iclipWarpParam(int(mat[2]) - 0x10000)
	beta := iclipWarpParam(int(mat[3]))
	gamma := iclipWarpParam(int(roundDivSigned64(int64(mat[4])*0x10000, int64(mat[2]))))
	delta := iclipWarpParam(int(mat[5]) - int(roundDivSigned64(int64(mat[3])*int64(mat[4]), int64(mat[2]))) - 0x10000)
	if 4*absInt(alpha)+7*absInt(beta) >= 0x10000 || 4*absInt(gamma)+4*absInt(delta) >= 0x10000 {
		return [4]int16{}, false
	}
	return [4]int16{int16(alpha), int16(beta), int16(gamma), int16(delta)}, true
}

func iclipWarpParam(v int) int {
	v = clipInt(v, -32768, 32767)
	mag := (absInt(v) + 32) >> 6
	if v < 0 {
		return -mag << 6
	}
	return mag << 6
}

func roundDivSigned64(num, den int64) int64 {
	if den < 0 {
		num = -num
		den = -den
	}
	if den == 0 {
		return 0
	}
	if num < 0 {
		return -(((-num) + den/2) / den)
	}
	return (num + den/2) / den
}

func collectLocalWarpSamplesInto(samples []affineSample, mvTile *SpatialMVTile, g BlockGeometry, ref0 int8, currentMV MotionVector, edge intraEdgeAvailability) []affineSample {
	samples = samples[:0]
	cx := float64(g.Start4X*4+g.Width4()*2) - 0.5
	cy := float64(g.Start4Y*4+g.Height4()*2) - 0.5
	samples = append(samples, affineSample{
		x: cx, y: cy,
		u: cx + float64(currentMV.X)/8.0,
		v: cy + float64(currentMV.Y)/8.0,
	})

	if g.Start4Y > mvTile.Geometry.Start4Y {
		y := float64(g.Start4Y*4) - 0.5
		for x4 := g.Start4X; x4 < g.End4X; x4++ {
			if b := mvTile.block(x4, g.Start4Y-1); matchingSingleRefBlock(b, ref0) {
				x := float64(x4*4) + 1.5
				samples = append(samples, affineSample{
					x: x, y: y,
					u: x + float64(b.MV[0].X)/8.0,
					v: y + float64(b.MV[0].Y)/8.0,
				})
			}
		}
		if edge.topHasRight && g.End4X < mvTile.Geometry.End4X {
			if b := mvTile.block(g.End4X, g.Start4Y-1); matchingSingleRefBlock(b, ref0) {
				x := float64(g.End4X*4) + 1.5
				samples = append(samples, affineSample{
					x: x, y: y,
					u: x + float64(b.MV[0].X)/8.0,
					v: y + float64(b.MV[0].Y)/8.0,
				})
			}
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X {
		x := float64(g.Start4X*4) - 0.5
		for y4 := g.Start4Y; y4 < g.End4Y; y4++ {
			if b := mvTile.block(g.Start4X-1, y4); matchingSingleRefBlock(b, ref0) {
				y := float64(y4*4) + 1.5
				samples = append(samples, affineSample{
					x: x, y: y,
					u: x + float64(b.MV[0].X)/8.0,
					v: y + float64(b.MV[0].Y)/8.0,
				})
			}
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X && g.Start4Y > mvTile.Geometry.Start4Y {
		if b := mvTile.block(g.Start4X-1, g.Start4Y-1); matchingSingleRefBlock(b, ref0) {
			x := float64(g.Start4X*4) - 0.5
			y := float64(g.Start4Y*4) - 0.5
			samples = append(samples, affineSample{
				x: x, y: y,
				u: x + float64(b.MV[0].X)/8.0,
				v: y + float64(b.MV[0].Y)/8.0,
			})
		}
	}
	return samples
}

func fitAffineWarp(samples []affineSample) (affineWarp, bool) {
	var m [3][3]float64
	var bu [3]float64
	var bv [3]float64
	for _, s := range samples {
		r0, r1, r2 := s.x, s.y, 1.0
		m[0][0] += r0 * r0
		m[0][1] += r0 * r1
		m[0][2] += r0 * r2
		m[1][0] += r1 * r0
		m[1][1] += r1 * r1
		m[1][2] += r1 * r2
		m[2][0] += r2 * r0
		m[2][1] += r2 * r1
		m[2][2] += r2 * r2
		bu[0] += r0 * s.u
		bu[1] += r1 * s.u
		bu[2] += r2 * s.u
		bv[0] += r0 * s.v
		bv[1] += r1 * s.v
		bv[2] += r2 * s.v
	}
	ax, ok := solve3x3(m, bu)
	if !ok {
		return affineWarp{}, false
	}
	ay, ok := solve3x3(m, bv)
	if !ok {
		return affineWarp{}, false
	}
	return affineWarp{
		a00: ax[0], a01: ax[1], a02: ax[2],
		a10: ay[0], a11: ay[1], a12: ay[2],
	}, true
}

func solve3x3(m [3][3]float64, b [3]float64) ([3]float64, bool) {
	a := [3][4]float64{
		{m[0][0], m[0][1], m[0][2], b[0]},
		{m[1][0], m[1][1], m[1][2], b[1]},
		{m[2][0], m[2][1], m[2][2], b[2]},
	}
	for col := 0; col < 3; col++ {
		pivot := col
		for row := col + 1; row < 3; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][col]) < 1e-9 {
			return [3]float64{}, false
		}
		if pivot != col {
			a[col], a[pivot] = a[pivot], a[col]
		}
		scale := a[col][col]
		for j := col; j < 4; j++ {
			a[col][j] /= scale
		}
		for row := 0; row < 3; row++ {
			if row == col {
				continue
			}
			f := a[row][col]
			if f == 0 {
				continue
			}
			for j := col; j < 4; j++ {
				a[row][j] -= f * a[col][j]
			}
		}
	}
	return [3]float64{a[0][3], a[1][3], a[2][3]}, true
}

func warpReasonable(w affineWarp, g BlockGeometry, currentMV MotionVector) bool {
	if math.Abs(w.a00-1.0) > 0.5 || math.Abs(w.a11-1.0) > 0.5 {
		return false
	}
	if math.Abs(w.a01) > 0.5 || math.Abs(w.a10) > 0.5 {
		return false
	}
	cx := float64(g.Start4X*4+g.Width4()*2) - 0.5
	cy := float64(g.Start4Y*4+g.Height4()*2) - 0.5
	u, v := w.mapPoint(cx, cy)
	expectU := cx + float64(currentMV.X)/8.0
	expectV := cy + float64(currentMV.Y)/8.0
	return math.Abs(u-expectU) <= 16.0 && math.Abs(v-expectV) <= 16.0
}

func PredictInterPlane8Warp(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, warp affineWarp) error {
	return predictInterPlane8WarpAt(dst, dstStride, planeWidth, planeHeight, x, y, width, height, x, y, refPlane, refStride, refWidth, refHeight, warp)
}

func predictInterPlane8WarpAt(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height, sampleX, sampleY int, refPlane []byte, refStride, refWidth, refHeight int, warp affineWarp) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if dstStride < planeWidth || refStride < refWidth {
		return fmt.Errorf("decoder: invalid warp strides")
	}
	if len(dst) < (planeHeight-1)*dstStride+planeWidth {
		return fmt.Errorf("decoder: destination plane buffer too small")
	}
	if len(refPlane) < (refHeight-1)*refStride+refWidth {
		return fmt.Errorf("decoder: reference plane buffer too small")
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	if warp.fixed {
		predictInterPlane8WarpFixedAt(dst, dstStride, x, y, visibleWidth, visibleHeight, sampleX, sampleY, refPlane, refStride, refWidth, refHeight, warp)
		return nil
	}
	useSmallH := width <= 4
	useSmallV := height <= 4
	rowSampleX := float64(sampleX)
	rowSampleY := float64(sampleY)
	rowU := warp.a00*rowSampleX + warp.a01*rowSampleY + warp.a02
	rowV := warp.a10*rowSampleX + warp.a11*rowSampleY + warp.a12
	stepU := warp.a00
	stepV := warp.a10
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		u := rowU
		v := rowV
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = sampleWarped8(refPlane, refStride, refWidth, refHeight, u, v, useSmallH, useSmallV)
			u += stepU
			v += stepV
		}
		rowU += warp.a01
		rowV += warp.a11
	}
	return nil
}

func predictInterPlane8WarpFixedAt(dst []byte, dstStride, dstX, dstY, visibleWidth, visibleHeight, sampleX, sampleY int, refPlane []byte, refStride, refWidth, refHeight int, warp affineWarp) {
	for by := 0; by < visibleHeight; by += 8 {
		blockH := minInt(8, visibleHeight-by)
		srcY := sampleY + by + 4
		mat3Y := int64(warp.matrix[3])*int64(srcY) + int64(warp.matrix[0])
		mat5Y := int64(warp.matrix[5])*int64(srcY) + int64(warp.matrix[1])
		for bx := 0; bx < visibleWidth; bx += 8 {
			blockW := minInt(8, visibleWidth-bx)
			srcX := sampleX + bx + 4
			mvx := int64(warp.matrix[2])*int64(srcX) + mat3Y
			mvy := int64(warp.matrix[4])*int64(srcX) + mat5Y
			dx := int(mvx>>16) - 4
			mx := (int(mvx&0xffff) - int(warp.abcd[0])*4 - int(warp.abcd[1])*7) &^ 0x3f
			dy := int(mvy>>16) - 4
			my := (int(mvy&0xffff) - int(warp.abcd[2])*4 - int(warp.abcd[3])*4) &^ 0x3f
			warp8x8Fixed8(dst[(dstY+by)*dstStride+dstX+bx:], dstStride, blockW, blockH, refPlane, refStride, refWidth, refHeight, dx, dy, mx, my, warp.abcd)
		}
	}
}

func warp8x8Fixed8(dst []byte, dstStride, blockW, blockH int, refPlane []byte, refStride, refWidth, refHeight, dx, dy, mx, my int, abcd [4]int16) {
	const intermediateBits = 4
	var mid [15 * 8]int
	for py := 0; py < 15; py++ {
		rowMX := mx + py*int(abcd[1])
		for px := 0; px < 8; px++ {
			filter := av1WarpFilter[warpFilterIndex(rowMX+px*int(abcd[0]))]
			sum := 0
			for tap := 0; tap < 8; tap++ {
				sum += filter[tap] * int(refSample8(refPlane, refStride, refWidth, refHeight, dx+px+tap-3, dy+py-3))
			}
			mid[py*8+px] = filterWarpRND(sum, 7-intermediateBits)
		}
	}
	for py := 0; py < blockH; py++ {
		rowMY := my + py*int(abcd[3])
		out := dst[py*dstStride:]
		for px := 0; px < blockW; px++ {
			filter := av1WarpFilter[warpFilterIndex(rowMY+px*int(abcd[2]))]
			sum := 0
			for tap := 0; tap < 8; tap++ {
				sum += filter[tap] * mid[(py+tap)*8+px]
			}
			out[px] = clipByte(filterWarpRND(sum, 7+intermediateBits))
		}
	}
}

func warpFilterIndex(phase int) int {
	idx := 64 + ((phase + 512) >> 10)
	if idx < 0 {
		return 0
	}
	if idx >= len(av1WarpFilter) {
		return len(av1WarpFilter) - 1
	}
	return idx
}

func filterWarpRND(sum, shift int) int {
	return (sum + ((1 << shift) >> 1)) >> shift
}

func PredictInterLumaBlock8Warp(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, warp affineWarp) error {
	return PredictInterPlane8Warp(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, warp)
}

func PredictInterChromaBlock8Warp(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []byte, refStride, refWidth, refHeight int, warp affineWarp) error {
	return PredictInterPlane8Warp(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, warp)
}

func sampleWarped8(plane []byte, stride, width, height int, x, y float64, useSmallH, useSmallV bool) uint8 {
	srcX16 := roundFloatToInt(x * 16.0)
	srcY16 := roundFloatToInt(y * 16.0)
	sx, fx := floorDivMod16(srcX16)
	sy, fy := floorDivMod16(srcY16)
	if sx >= 0 && sy >= 0 && sx < width && sy < height && fx == 0 && fy == 0 {
		return plane[sy*stride+sx]
	}
	p00 := refSample8(plane, stride, width, height, sx, sy)
	if fx == 0 && fy == 0 {
		return p00
	}
	hf := inter8TapCoeffs(interFilterRegular, fx, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(interFilterRegular, fy, ternaryInt(useSmallV, 4, 8))
	hInterior := sx >= 3 && sx+5 <= width
	vInterior := sy >= 3 && sy+5 <= height
	switch {
	case hf != nil && vf != nil:
		if hInterior && vInterior {
			h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
			h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
			v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
			v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
			sx0 := sx - 3
			r0, r1 := plane[(sy-3)*stride+sx0:], plane[(sy-2)*stride+sx0:]
			r2 := plane[(sy-1)*stride+sx0:]
			r3 := plane[sy*stride+sx0:]
			r4 := plane[(sy+1)*stride+sx0:]
			r5 := plane[(sy+2)*stride+sx0:]
			r6, r7 := plane[(sy+3)*stride+sx0:], plane[(sy+4)*stride+sx0:]
			if h0 == 0 && h7 == 0 && v0 == 0 && v7 == 0 {
				m1 := (h1*int(r1[1]) + h2*int(r1[2]) + h3*int(r1[3]) + h4*int(r1[4]) + h5*int(r1[5]) + h6*int(r1[6]) + 2) >> 2
				m2 := (h1*int(r2[1]) + h2*int(r2[2]) + h3*int(r2[3]) + h4*int(r2[4]) + h5*int(r2[5]) + h6*int(r2[6]) + 2) >> 2
				m3 := (h1*int(r3[1]) + h2*int(r3[2]) + h3*int(r3[3]) + h4*int(r3[4]) + h5*int(r3[5]) + h6*int(r3[6]) + 2) >> 2
				m4 := (h1*int(r4[1]) + h2*int(r4[2]) + h3*int(r4[3]) + h4*int(r4[4]) + h5*int(r4[5]) + h6*int(r4[6]) + 2) >> 2
				m5 := (h1*int(r5[1]) + h2*int(r5[2]) + h3*int(r5[3]) + h4*int(r5[4]) + h5*int(r5[5]) + h6*int(r5[6]) + 2) >> 2
				m6 := (h1*int(r6[1]) + h2*int(r6[2]) + h3*int(r6[3]) + h4*int(r6[4]) + h5*int(r6[5]) + h6*int(r6[6]) + 2) >> 2
				sum := v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6
				return clipByte((sum + 512) >> 10)
			}
			m0 := (h0*int(r0[0]) + h1*int(r0[1]) + h2*int(r0[2]) + h3*int(r0[3]) + h4*int(r0[4]) + h5*int(r0[5]) + h6*int(r0[6]) + h7*int(r0[7]) + 2) >> 2
			m1 := (h0*int(r1[0]) + h1*int(r1[1]) + h2*int(r1[2]) + h3*int(r1[3]) + h4*int(r1[4]) + h5*int(r1[5]) + h6*int(r1[6]) + h7*int(r1[7]) + 2) >> 2
			m2 := (h0*int(r2[0]) + h1*int(r2[1]) + h2*int(r2[2]) + h3*int(r2[3]) + h4*int(r2[4]) + h5*int(r2[5]) + h6*int(r2[6]) + h7*int(r2[7]) + 2) >> 2
			m3 := (h0*int(r3[0]) + h1*int(r3[1]) + h2*int(r3[2]) + h3*int(r3[3]) + h4*int(r3[4]) + h5*int(r3[5]) + h6*int(r3[6]) + h7*int(r3[7]) + 2) >> 2
			m4 := (h0*int(r4[0]) + h1*int(r4[1]) + h2*int(r4[2]) + h3*int(r4[3]) + h4*int(r4[4]) + h5*int(r4[5]) + h6*int(r4[6]) + h7*int(r4[7]) + 2) >> 2
			m5 := (h0*int(r5[0]) + h1*int(r5[1]) + h2*int(r5[2]) + h3*int(r5[3]) + h4*int(r5[4]) + h5*int(r5[5]) + h6*int(r5[6]) + h7*int(r5[7]) + 2) >> 2
			m6 := (h0*int(r6[0]) + h1*int(r6[1]) + h2*int(r6[2]) + h3*int(r6[3]) + h4*int(r6[4]) + h5*int(r6[5]) + h6*int(r6[6]) + h7*int(r6[7]) + 2) >> 2
			m7 := (h0*int(r7[0]) + h1*int(r7[1]) + h2*int(r7[2]) + h3*int(r7[3]) + h4*int(r7[4]) + h5*int(r7[5]) + h6*int(r7[6]) + h7*int(r7[7]) + 2) >> 2
			sum := v0*m0 + v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6 + v7*m7
			return clipByte((sum + 512) >> 10)
		}
		h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
		h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
		v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
		v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
		maxY := height - 1
		xBase := sx - 3
		if hInterior {
			var rows [8][]byte
			fillClampedRows8(&rows, plane, stride, maxY, sy)
			r0, r1, r2, r3 := rows[0], rows[1], rows[2], rows[3]
			r4, r5, r6, r7 := rows[4], rows[5], rows[6], rows[7]
			if h0 == 0 && h7 == 0 && v0 == 0 && v7 == 0 {
				m1 := (h1*int(r1[xBase+1]) + h2*int(r1[xBase+2]) + h3*int(r1[xBase+3]) + h4*int(r1[xBase+4]) + h5*int(r1[xBase+5]) + h6*int(r1[xBase+6]) + 2) >> 2
				m2 := (h1*int(r2[xBase+1]) + h2*int(r2[xBase+2]) + h3*int(r2[xBase+3]) + h4*int(r2[xBase+4]) + h5*int(r2[xBase+5]) + h6*int(r2[xBase+6]) + 2) >> 2
				m3 := (h1*int(r3[xBase+1]) + h2*int(r3[xBase+2]) + h3*int(r3[xBase+3]) + h4*int(r3[xBase+4]) + h5*int(r3[xBase+5]) + h6*int(r3[xBase+6]) + 2) >> 2
				m4 := (h1*int(r4[xBase+1]) + h2*int(r4[xBase+2]) + h3*int(r4[xBase+3]) + h4*int(r4[xBase+4]) + h5*int(r4[xBase+5]) + h6*int(r4[xBase+6]) + 2) >> 2
				m5 := (h1*int(r5[xBase+1]) + h2*int(r5[xBase+2]) + h3*int(r5[xBase+3]) + h4*int(r5[xBase+4]) + h5*int(r5[xBase+5]) + h6*int(r5[xBase+6]) + 2) >> 2
				m6 := (h1*int(r6[xBase+1]) + h2*int(r6[xBase+2]) + h3*int(r6[xBase+3]) + h4*int(r6[xBase+4]) + h5*int(r6[xBase+5]) + h6*int(r6[xBase+6]) + 2) >> 2
				sum := v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6
				return clipByte((sum + 512) >> 10)
			}
			m0 := (h0*int(r0[xBase+0]) + h1*int(r0[xBase+1]) + h2*int(r0[xBase+2]) + h3*int(r0[xBase+3]) + h4*int(r0[xBase+4]) + h5*int(r0[xBase+5]) + h6*int(r0[xBase+6]) + h7*int(r0[xBase+7]) + 2) >> 2
			m1 := (h0*int(r1[xBase+0]) + h1*int(r1[xBase+1]) + h2*int(r1[xBase+2]) + h3*int(r1[xBase+3]) + h4*int(r1[xBase+4]) + h5*int(r1[xBase+5]) + h6*int(r1[xBase+6]) + h7*int(r1[xBase+7]) + 2) >> 2
			m2 := (h0*int(r2[xBase+0]) + h1*int(r2[xBase+1]) + h2*int(r2[xBase+2]) + h3*int(r2[xBase+3]) + h4*int(r2[xBase+4]) + h5*int(r2[xBase+5]) + h6*int(r2[xBase+6]) + h7*int(r2[xBase+7]) + 2) >> 2
			m3 := (h0*int(r3[xBase+0]) + h1*int(r3[xBase+1]) + h2*int(r3[xBase+2]) + h3*int(r3[xBase+3]) + h4*int(r3[xBase+4]) + h5*int(r3[xBase+5]) + h6*int(r3[xBase+6]) + h7*int(r3[xBase+7]) + 2) >> 2
			m4 := (h0*int(r4[xBase+0]) + h1*int(r4[xBase+1]) + h2*int(r4[xBase+2]) + h3*int(r4[xBase+3]) + h4*int(r4[xBase+4]) + h5*int(r4[xBase+5]) + h6*int(r4[xBase+6]) + h7*int(r4[xBase+7]) + 2) >> 2
			m5 := (h0*int(r5[xBase+0]) + h1*int(r5[xBase+1]) + h2*int(r5[xBase+2]) + h3*int(r5[xBase+3]) + h4*int(r5[xBase+4]) + h5*int(r5[xBase+5]) + h6*int(r5[xBase+6]) + h7*int(r5[xBase+7]) + 2) >> 2
			m6 := (h0*int(r6[xBase+0]) + h1*int(r6[xBase+1]) + h2*int(r6[xBase+2]) + h3*int(r6[xBase+3]) + h4*int(r6[xBase+4]) + h5*int(r6[xBase+5]) + h6*int(r6[xBase+6]) + h7*int(r6[xBase+7]) + 2) >> 2
			m7 := (h0*int(r7[xBase+0]) + h1*int(r7[xBase+1]) + h2*int(r7[xBase+2]) + h3*int(r7[xBase+3]) + h4*int(r7[xBase+4]) + h5*int(r7[xBase+5]) + h6*int(r7[xBase+6]) + h7*int(r7[xBase+7]) + 2) >> 2
			sum := v0*m0 + v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6 + v7*m7
			return clipByte((sum + 512) >> 10)
		}
		x0, x1, x2, x3, x4, x5, x6, x7 := clamp8TapIndices(xBase, width-1)
		var rows [8][]byte
		fillClampedRows8(&rows, plane, stride, maxY, sy)
		r0, r1, r2, r3 := rows[0], rows[1], rows[2], rows[3]
		r4, r5, r6, r7 := rows[4], rows[5], rows[6], rows[7]
		if h0 == 0 && h7 == 0 && v0 == 0 && v7 == 0 {
			m1 := (h1*int(r1[x1]) + h2*int(r1[x2]) + h3*int(r1[x3]) + h4*int(r1[x4]) + h5*int(r1[x5]) + h6*int(r1[x6]) + 2) >> 2
			m2 := (h1*int(r2[x1]) + h2*int(r2[x2]) + h3*int(r2[x3]) + h4*int(r2[x4]) + h5*int(r2[x5]) + h6*int(r2[x6]) + 2) >> 2
			m3 := (h1*int(r3[x1]) + h2*int(r3[x2]) + h3*int(r3[x3]) + h4*int(r3[x4]) + h5*int(r3[x5]) + h6*int(r3[x6]) + 2) >> 2
			m4 := (h1*int(r4[x1]) + h2*int(r4[x2]) + h3*int(r4[x3]) + h4*int(r4[x4]) + h5*int(r4[x5]) + h6*int(r4[x6]) + 2) >> 2
			m5 := (h1*int(r5[x1]) + h2*int(r5[x2]) + h3*int(r5[x3]) + h4*int(r5[x4]) + h5*int(r5[x5]) + h6*int(r5[x6]) + 2) >> 2
			m6 := (h1*int(r6[x1]) + h2*int(r6[x2]) + h3*int(r6[x3]) + h4*int(r6[x4]) + h5*int(r6[x5]) + h6*int(r6[x6]) + 2) >> 2
			sum := v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6
			return clipByte((sum + 512) >> 10)
		}
		m0 := (h0*int(r0[x0]) + h1*int(r0[x1]) + h2*int(r0[x2]) + h3*int(r0[x3]) + h4*int(r0[x4]) + h5*int(r0[x5]) + h6*int(r0[x6]) + h7*int(r0[x7]) + 2) >> 2
		m1 := (h0*int(r1[x0]) + h1*int(r1[x1]) + h2*int(r1[x2]) + h3*int(r1[x3]) + h4*int(r1[x4]) + h5*int(r1[x5]) + h6*int(r1[x6]) + h7*int(r1[x7]) + 2) >> 2
		m2 := (h0*int(r2[x0]) + h1*int(r2[x1]) + h2*int(r2[x2]) + h3*int(r2[x3]) + h4*int(r2[x4]) + h5*int(r2[x5]) + h6*int(r2[x6]) + h7*int(r2[x7]) + 2) >> 2
		m3 := (h0*int(r3[x0]) + h1*int(r3[x1]) + h2*int(r3[x2]) + h3*int(r3[x3]) + h4*int(r3[x4]) + h5*int(r3[x5]) + h6*int(r3[x6]) + h7*int(r3[x7]) + 2) >> 2
		m4 := (h0*int(r4[x0]) + h1*int(r4[x1]) + h2*int(r4[x2]) + h3*int(r4[x3]) + h4*int(r4[x4]) + h5*int(r4[x5]) + h6*int(r4[x6]) + h7*int(r4[x7]) + 2) >> 2
		m5 := (h0*int(r5[x0]) + h1*int(r5[x1]) + h2*int(r5[x2]) + h3*int(r5[x3]) + h4*int(r5[x4]) + h5*int(r5[x5]) + h6*int(r5[x6]) + h7*int(r5[x7]) + 2) >> 2
		m6 := (h0*int(r6[x0]) + h1*int(r6[x1]) + h2*int(r6[x2]) + h3*int(r6[x3]) + h4*int(r6[x4]) + h5*int(r6[x5]) + h6*int(r6[x6]) + h7*int(r6[x7]) + 2) >> 2
		m7 := (h0*int(r7[x0]) + h1*int(r7[x1]) + h2*int(r7[x2]) + h3*int(r7[x3]) + h4*int(r7[x4]) + h5*int(r7[x5]) + h6*int(r7[x6]) + h7*int(r7[x7]) + 2) >> 2
		sum := v0*m0 + v1*m1 + v2*m2 + v3*m3 + v4*m4 + v5*m5 + v6*m6 + v7*m7
		return clipByte((sum + 512) >> 10)
	case hf != nil:
		if hInterior && sy >= 0 && sy < height {
			h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
			h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
			src := plane[sy*stride+sx-3:]
			if h0 == 0 && h7 == 0 {
				sum := h1*int(src[1]) + h2*int(src[2]) + h3*int(src[3]) + h4*int(src[4]) + h5*int(src[5]) + h6*int(src[6])
				return clipByte((sum + 34) >> 6)
			}
			sum := h0*int(src[0]) + h1*int(src[1]) + h2*int(src[2]) + h3*int(src[3]) + h4*int(src[4]) + h5*int(src[5]) + h6*int(src[6]) + h7*int(src[7])
			return clipByte((sum + 34) >> 6)
		}
		h0, h1, h2, h3 := hf[0], hf[1], hf[2], hf[3]
		h4, h5, h6, h7 := hf[4], hf[5], hf[6], hf[7]
		x0, x1, x2, x3, x4, x5, x6, x7 := clamp8TapIndices(sx-3, width-1)
		if sy < 0 {
			sy = 0
		} else if sy >= height {
			sy = height - 1
		}
		src := plane[sy*stride:]
		if h0 == 0 && h7 == 0 {
			sum := h1*int(src[x1]) + h2*int(src[x2]) + h3*int(src[x3]) + h4*int(src[x4]) + h5*int(src[x5]) + h6*int(src[x6])
			return clipByte((sum + 34) >> 6)
		}
		sum := h0*int(src[x0]) + h1*int(src[x1]) + h2*int(src[x2]) + h3*int(src[x3]) + h4*int(src[x4]) + h5*int(src[x5]) + h6*int(src[x6]) + h7*int(src[x7])
		return clipByte((sum + 34) >> 6)
	case vf != nil:
		if vInterior && sx >= 0 && sx < width {
			v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
			v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
			if v0 == 0 && v7 == 0 {
				sum := v1*int(plane[(sy-2)*stride+sx]) +
					v2*int(plane[(sy-1)*stride+sx]) +
					v3*int(plane[sy*stride+sx]) +
					v4*int(plane[(sy+1)*stride+sx]) +
					v5*int(plane[(sy+2)*stride+sx]) +
					v6*int(plane[(sy+3)*stride+sx])
				return clipByte((sum + 32) >> 6)
			}
			sum := v0*int(plane[(sy-3)*stride+sx]) +
				v1*int(plane[(sy-2)*stride+sx]) +
				v2*int(plane[(sy-1)*stride+sx]) +
				v3*int(plane[sy*stride+sx]) +
				v4*int(plane[(sy+1)*stride+sx]) +
				v5*int(plane[(sy+2)*stride+sx]) +
				v6*int(plane[(sy+3)*stride+sx]) +
				v7*int(plane[(sy+4)*stride+sx])
			return clipByte((sum + 32) >> 6)
		}
		v0, v1, v2, v3 := vf[0], vf[1], vf[2], vf[3]
		v4, v5, v6, v7 := vf[4], vf[5], vf[6], vf[7]
		if sx < 0 {
			sx = 0
		} else if sx >= width {
			sx = width - 1
		}
		var rows [8][]byte
		fillClampedRows8(&rows, plane, stride, height-1, sy)
		if v0 == 0 && v7 == 0 {
			sum := v1*int(rows[1][sx]) + v2*int(rows[2][sx]) + v3*int(rows[3][sx]) + v4*int(rows[4][sx]) + v5*int(rows[5][sx]) + v6*int(rows[6][sx])
			return clipByte((sum + 32) >> 6)
		}
		sum := v0*int(rows[0][sx]) + v1*int(rows[1][sx]) + v2*int(rows[2][sx]) + v3*int(rows[3][sx]) + v4*int(rows[4][sx]) + v5*int(rows[5][sx]) + v6*int(rows[6][sx]) + v7*int(rows[7][sx])
		return clipByte((sum + 32) >> 6)
	default:
		return p00
	}
}

func clamp8TapIndices(base, max int) (int, int, int, int, int, int, int, int) {
	x0 := base
	x1 := base + 1
	x2 := base + 2
	x3 := base + 3
	x4 := base + 4
	x5 := base + 5
	x6 := base + 6
	x7 := base + 7
	if x0 < 0 {
		x0 = 0
	} else if x0 > max {
		x0 = max
	}
	if x1 < 0 {
		x1 = 0
	} else if x1 > max {
		x1 = max
	}
	if x2 < 0 {
		x2 = 0
	} else if x2 > max {
		x2 = max
	}
	if x3 < 0 {
		x3 = 0
	} else if x3 > max {
		x3 = max
	}
	if x4 < 0 {
		x4 = 0
	} else if x4 > max {
		x4 = max
	}
	if x5 < 0 {
		x5 = 0
	} else if x5 > max {
		x5 = max
	}
	if x6 < 0 {
		x6 = 0
	} else if x6 > max {
		x6 = max
	}
	if x7 < 0 {
		x7 = 0
	} else if x7 > max {
		x7 = max
	}
	return x0, x1, x2, x3, x4, x5, x6, x7
}

func PredictInterPlane16Warp(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, warp affineWarp, bitDepth uint8) error {
	return predictInterPlane16WarpAt(dst, dstStride, planeWidth, planeHeight, x, y, width, height, x, y, refPlane, refStride, refWidth, refHeight, warp, bitDepth)
}

func predictInterPlane16WarpAt(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height, sampleX, sampleY int, refPlane []uint16, refStride, refWidth, refHeight int, warp affineWarp, bitDepth uint8) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if dstStride < planeWidth || refStride < refWidth {
		return fmt.Errorf("decoder: invalid warp strides")
	}
	if len(dst) < (planeHeight-1)*dstStride+planeWidth {
		return fmt.Errorf("decoder: destination plane buffer too small")
	}
	if len(refPlane) < (refHeight-1)*refStride+refWidth {
		return fmt.Errorf("decoder: reference plane buffer too small")
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	useSmallH := width <= 4
	useSmallV := height <= 4
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			sx, sy := warp.mapPoint(float64(sampleX+px), float64(sampleY+py))
			row[x+px] = sampleWarped16(refPlane, refStride, refWidth, refHeight, sx, sy, bitDepth, useSmallH, useSmallV)
		}
	}
	return nil
}

func PredictInterLumaBlock16Warp(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, warp affineWarp, bitDepth uint8) error {
	return PredictInterPlane16Warp(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, warp, bitDepth)
}

func PredictInterChromaBlock16Warp(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int, refPlane []uint16, refStride, refWidth, refHeight int, warp affineWarp, bitDepth uint8) error {
	return PredictInterPlane16Warp(dst, dstStride, planeWidth, planeHeight, x, y, width, height, refPlane, refStride, refWidth, refHeight, warp, bitDepth)
}

func sampleWarped16(plane []uint16, stride, width, height int, x, y float64, bitDepth uint8, useSmallH, useSmallV bool) uint16 {
	srcX16 := roundFloatToInt(x * 16.0)
	srcY16 := roundFloatToInt(y * 16.0)
	sx, fx := floorDivMod16(srcX16)
	sy, fy := floorDivMod16(srcY16)
	p00 := refSample16(plane, stride, width, height, sx, sy)
	if fx == 0 && fy == 0 {
		return p00
	}
	hf := inter8TapCoeffs(interFilterRegular, fx, ternaryInt(useSmallH, 4, 8))
	vf := inter8TapCoeffs(interFilterRegular, fy, ternaryInt(useSmallV, 4, 8))
	switch {
	case hf != nil && vf != nil:
		var mid [8]int
		for tapY := 0; tapY < 8; tapY++ {
			sum := 0
			sampleY := sy + tapY - 3
			for tapX := 0; tapX < 8; tapX++ {
				sum += hf[tapX] * int(refSample16(plane, stride, width, height, sx+tapX-3, sampleY))
			}
			mid[tapY] = (sum + 2) >> 2
		}
		sum := 0
		for tapY := 0; tapY < 8; tapY++ {
			sum += vf[tapY] * mid[tapY]
		}
		return clipSample((sum+512)>>10, bitDepth)
	case hf != nil:
		sum := 0
		for tapX := 0; tapX < 8; tapX++ {
			sum += hf[tapX] * int(refSample16(plane, stride, width, height, sx+tapX-3, sy))
		}
		return clipSample((sum+34)>>6, bitDepth)
	case vf != nil:
		sum := 0
		for tapY := 0; tapY < 8; tapY++ {
			sum += vf[tapY] * int(refSample16(plane, stride, width, height, sx, sy+tapY-3))
		}
		return clipSample((sum+32)>>6, bitDepth)
	default:
		return p00
	}
}
