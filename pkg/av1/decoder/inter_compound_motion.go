package decoder

import (
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func globalMotionWarpForPlane(hdr *obu.FrameHeader, ref int8, plane int, chroma av1.ChromaSubsampling) (affineWarp, bool) {
	if hdr == nil || ref < 0 || int(ref) >= len(hdr.GMV) {
		return affineWarp{}, false
	}
	gmv := hdr.GMV[ref]
	if gmv.Type == 0 {
		return affineWarp{}, false
	}
	ssHor, ssVer := 0, 0
	if plane != 0 {
		switch chroma {
		case av1.Chroma420:
			ssHor, ssVer = 1, 1
		case av1.Chroma422:
			ssHor, ssVer = 1, 0
		}
	}
	if gmv.Type > 1 {
		mat := scaledGlobalMotionMatrixForPlane(gmv.Matrix, ssHor, ssVer)
		return affineWarpFromMatrix(mat)
	}
	scaleXInt := 1 << ssHor
	scaleYInt := 1 << ssVer
	scaleX := float64(scaleXInt)
	scaleY := float64(scaleYInt)
	return affineWarp{
		a00: float64(gmv.Matrix[2]) / 65536.0,
		a01: float64(gmv.Matrix[3]) * scaleY / (65536.0 * scaleX),
		a02: float64(gmv.Matrix[0]) / (65536.0 * scaleX),
		a10: float64(gmv.Matrix[4]) * scaleX / (65536.0 * scaleY),
		a11: float64(gmv.Matrix[5]) / 65536.0,
		a12: float64(gmv.Matrix[1]) / (65536.0 * scaleY),
	}, true
}

func scaledGlobalMotionMatrixForPlane(mat [6]int32, ssHor, ssVer int) [6]int32 {
	if ssHor == 0 && ssVer == 0 {
		return mat
	}
	scaled := mat
	scaled[0] = scaleGlobalMotionParam(mat[0], ssHor, 0)
	scaled[1] = scaleGlobalMotionParam(mat[1], ssVer, 0)
	scaled[3] = scaleGlobalMotionParam(mat[3], ssHor, ssVer)
	scaled[4] = scaleGlobalMotionParam(mat[4], ssVer, ssHor)
	return scaled
}

func scaleGlobalMotionParam(v int32, divisorShift, multiplierShift int) int32 {
	if divisorShift > multiplierShift {
		return v >> uint(divisorShift-multiplierShift)
	}
	if multiplierShift > divisorShift {
		return v << uint(multiplierShift-divisorShift)
	}
	return v
}

func globalMotionUsesWarp(hdr *obu.FrameHeader, ref int8) bool {
	return hdr != nil && !hdr.ForceIntegerMV && globalMotionType(hdr, ref) > 1
}

func approximateGlobalMotionVectorForBlock(hdr *obu.FrameHeader, ref int8, g BlockGeometry) MotionVector {
	if hdr == nil || ref < 0 || int(ref) >= len(hdr.GMV) {
		return MotionVector{}
	}
	gmv := hdr.GMV[ref]
	switch gmv.Type {
	case 1: // translation
		mv := MotionVector{
			Y: int(gmv.Matrix[0] >> 13),
			X: int(gmv.Matrix[1] >> 13),
		}
		if hdr.ForceIntegerMV {
			mv = MotionVector{
				Y: fixIntMVPrecisionComponent(mv.Y),
				X: fixIntMVPrecisionComponent(mv.X),
			}
		}
		return mv
	case 2, 3: // rotzoom / affine
		x := int64(g.Start4X*4 + g.Width4()*2 - 1)
		y := int64(g.Start4Y*4 + g.Height4()*2 - 1)
		xc := int64(gmv.Matrix[2]-(1<<16))*x + int64(gmv.Matrix[3])*y + int64(gmv.Matrix[0])
		yc := int64(gmv.Matrix[5]-(1<<16))*y + int64(gmv.Matrix[4])*x + int64(gmv.Matrix[1])
		shift := 13
		postShift := 0
		if !hdr.HighPrecisionMV {
			shift = 14
			postShift = 1
		}
		round := int64(1 << (shift - 1))
		mv := MotionVector{
			Y: applySignInt(((absInt64(yc)+round)>>uint(shift))<<uint(postShift), yc),
			X: applySignInt(((absInt64(xc)+round)>>uint(shift))<<uint(postShift), xc),
		}
		if hdr.ForceIntegerMV {
			mv = MotionVector{
				Y: fixIntMVPrecisionComponent(mv.Y),
				X: fixIntMVPrecisionComponent(mv.X),
			}
		}
		return mv
	default:
		return MotionVector{}
	}
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func applySignInt(v int64, signSrc int64) int {
	if signSrc < 0 {
		return -int(v)
	}
	return int(v)
}

func singleRefGlobalMotionForPlane(hdr *obu.FrameHeader, ref int8, plane int, chroma av1.ChromaSubsampling, g BlockGeometry) (MotionVector, affineWarp, bool) {
	if hdr == nil {
		return MotionVector{}, affineWarp{}, false
	}
	mv := approximateGlobalMotionVectorForBlock(hdr, ref, g)
	warp, ok := globalMotionWarpForPlane(hdr, ref, plane, chroma)
	return mv, warp, ok
}

func predictCompoundLumaRef8(dst []byte, stride, width, height, sampleX, sampleY int, ref []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair, useWarp bool, warp affineWarp) error {
	if useWarp {
		return predictInterPlane8WarpAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, warp)
	}
	predictInterLumaBlock8WithFiltersAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, mv, filter)
	return nil
}

func predictCompoundChromaRef8(dst []byte, stride, width, height, sampleX, sampleY int, ref []byte, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair, useWarp bool, warp affineWarp) error {
	if useWarp {
		return predictInterPlane8WarpAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, warp)
	}
	predictInterChromaBlock8WithFiltersAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, mv, filter)
	return nil
}

func predictCompoundLumaRef16(dst []uint16, stride, width, height, sampleX, sampleY int, ref []uint16, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair, useWarp bool, warp affineWarp, bitDepth uint8) error {
	if useWarp {
		return predictInterPlane16WarpAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, warp, bitDepth)
	}
	return predictInterLumaBlock16WithFiltersAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, mv, bitDepth, filter)
}

func predictCompoundChromaRef16(dst []uint16, stride, width, height, sampleX, sampleY int, ref []uint16, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair, useWarp bool, warp affineWarp, bitDepth uint8) error {
	if useWarp {
		return predictInterPlane16WarpAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, warp, bitDepth)
	}
	return predictInterChromaBlock16WithFiltersAt(dst, stride, width, height, 0, 0, width, height, sampleX, sampleY, ref, refStride, refWidth, refHeight, mv, bitDepth, filter)
}

func PredictWeightedCompoundInterLumaBlock8Motion(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, weight int) error {
	if !useWarp0 && !useWarp1 {
		return PredictWeightedCompoundInterLumaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, weight)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]byte, visibleWidth*visibleHeight)
	tmp1 := make([]byte, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0); err != nil {
		return err
	}
	if err := predictCompoundLumaRef8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = clipByte((int(tmp0[py*visibleWidth+px])*weight + int(tmp1[py*visibleWidth+px])*(16-weight) + 8) >> 4)
		}
	}
	return nil
}

func PredictWeightedCompoundInterChromaBlock8Motion(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, weight int) error {
	if !useWarp0 && !useWarp1 {
		return PredictWeightedCompoundInterChromaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, weight)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]byte, visibleWidth*visibleHeight)
	tmp1 := make([]byte, visibleWidth*visibleHeight)
	if err := predictCompoundChromaRef8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0); err != nil {
		return err
	}
	if err := predictCompoundChromaRef8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = clipByte((int(tmp0[py*visibleWidth+px])*weight + int(tmp1[py*visibleWidth+px])*(16-weight) + 8) >> 4)
		}
	}
	return nil
}

func PredictWeightedCompoundInterLumaBlock16Motion(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, bitDepth uint8, weight int) error {
	if !useWarp0 && !useWarp1 {
		return PredictWeightedCompoundInterLumaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, weight)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef16(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0, bitDepth); err != nil {
		return err
	}
	if err := predictCompoundLumaRef16(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1, bitDepth); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*weight+int(tmp1[py*visibleWidth+px])*(16-weight)+8)>>4, bitDepth)
		}
	}
	return nil
}

func PredictWeightedCompoundInterChromaBlock16Motion(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, bitDepth uint8, weight int) error {
	if !useWarp0 && !useWarp1 {
		return PredictWeightedCompoundInterChromaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, weight)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictCompoundChromaRef16(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0, bitDepth); err != nil {
		return err
	}
	if err := predictCompoundChromaRef16(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1, bitDepth); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		for px := 0; px < visibleWidth; px++ {
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*weight+int(tmp1[py*visibleWidth+px])*(16-weight)+8)>>4, bitDepth)
		}
	}
	return nil
}

func PredictMaskedCompoundInterLumaBlock8Motion(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, mask []uint8, maskStride int) error {
	if !useWarp0 && !useWarp1 {
		return PredictMaskedCompoundInterLumaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, mask, maskStride)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]byte, visibleWidth*visibleHeight)
	tmp1 := make([]byte, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0); err != nil {
		return err
	}
	if err := predictCompoundLumaRef8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < visibleWidth; px++ {
			w0 := int(mr[px])
			row[x+px] = clipByte((int(tmp0[py*visibleWidth+px])*w0 + int(tmp1[py*visibleWidth+px])*(64-w0) + 32) >> 6)
		}
	}
	return nil
}

func PredictMaskedCompoundInterChromaBlock8Motion(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, mask []uint8, maskStride int) error {
	if !useWarp0 && !useWarp1 {
		return PredictMaskedCompoundInterChromaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, mask, maskStride)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]byte, visibleWidth*visibleHeight)
	tmp1 := make([]byte, visibleWidth*visibleHeight)
	if err := predictCompoundChromaRef8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0); err != nil {
		return err
	}
	if err := predictCompoundChromaRef8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < visibleWidth; px++ {
			w0 := int(mr[px])
			row[x+px] = clipByte((int(tmp0[py*visibleWidth+px])*w0 + int(tmp1[py*visibleWidth+px])*(64-w0) + 32) >> 6)
		}
	}
	return nil
}

func PredictMaskedCompoundInterLumaBlock16Motion(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, bitDepth uint8, mask []uint8, maskStride int) error {
	if !useWarp0 && !useWarp1 {
		return PredictMaskedCompoundInterLumaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, mask, maskStride)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef16(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0, bitDepth); err != nil {
		return err
	}
	if err := predictCompoundLumaRef16(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1, bitDepth); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < visibleWidth; px++ {
			w0 := int(mr[px])
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*w0+int(tmp1[py*visibleWidth+px])*(64-w0)+32)>>6, bitDepth)
		}
	}
	return nil
}

func PredictMaskedCompoundInterChromaBlock16Motion(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, bitDepth uint8, mask []uint8, maskStride int) error {
	if !useWarp0 && !useWarp1 {
		return PredictMaskedCompoundInterChromaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, mask, maskStride)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictCompoundChromaRef16(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0, bitDepth); err != nil {
		return err
	}
	if err := predictCompoundChromaRef16(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1, bitDepth); err != nil {
		return err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*maskStride:]
		for px := 0; px < visibleWidth; px++ {
			w0 := int(mr[px])
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*w0+int(tmp1[py*visibleWidth+px])*(64-w0)+32)>>6, bitDepth)
		}
	}
	return nil
}

func PredictDiffWeightedCompoundInterLumaBlock8Motion(dst []byte, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []byte, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []byte, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, sign bool) ([]uint8, error) {
	if !useWarp0 && !useWarp1 {
		return PredictDiffWeightedCompoundInterLumaBlock8WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, sign)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	mask := make([]uint8, width*height)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return mask, nil
	}
	tmp0 := make([]byte, visibleWidth*visibleHeight)
	tmp1 := make([]byte, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef8(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0); err != nil {
		return nil, err
	}
	if err := predictCompoundLumaRef8(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1); err != nil {
		return nil, err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*width:]
		for px := 0; px < visibleWidth; px++ {
			w0 := diffWeightedMaskValue(absInt(int(tmp0[py*visibleWidth+px])-int(tmp1[py*visibleWidth+px])), 8)
			if sign {
				w0 = 64 - w0
			}
			mr[px] = w0
			row[x+px] = clipByte((int(tmp0[py*visibleWidth+px])*int(w0) + int(tmp1[py*visibleWidth+px])*(64-int(w0)) + 32) >> 6)
		}
	}
	return mask, nil
}

func PredictDiffWeightedCompoundInterLumaBlock16Motion(dst []uint16, dstStride, planeWidth, planeHeight, x, y, width, height int,
	ref0 []uint16, ref0Stride, ref0Width, ref0Height int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp,
	ref1 []uint16, ref1Stride, ref1Width, ref1Height int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, bitDepth uint8, sign bool) ([]uint8, error) {
	if !useWarp0 && !useWarp1 {
		return PredictDiffWeightedCompoundInterLumaBlock16WithFilters(dst, dstStride, planeWidth, planeHeight, x, y, width, height, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, bitDepth, sign)
	}
	visibleWidth := minInt(width, planeWidth-x)
	visibleHeight := minInt(height, planeHeight-y)
	mask := make([]uint8, width*height)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return mask, nil
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef16(tmp0, visibleWidth, visibleWidth, visibleHeight, x, y, ref0, ref0Stride, ref0Width, ref0Height, mv0, filter0, useWarp0, warp0, bitDepth); err != nil {
		return nil, err
	}
	if err := predictCompoundLumaRef16(tmp1, visibleWidth, visibleWidth, visibleHeight, x, y, ref1, ref1Stride, ref1Width, ref1Height, mv1, filter1, useWarp1, warp1, bitDepth); err != nil {
		return nil, err
	}
	for py := 0; py < visibleHeight; py++ {
		row := dst[(y+py)*dstStride:]
		mr := mask[py*width:]
		for px := 0; px < visibleWidth; px++ {
			w0 := diffWeightedMaskValue(absInt(int(tmp0[py*visibleWidth+px])-int(tmp1[py*visibleWidth+px])), bitDepth)
			if sign {
				w0 = 64 - w0
			}
			mr[px] = w0
			row[x+px] = clipSample((int(tmp0[py*visibleWidth+px])*int(w0)+int(tmp1[py*visibleWidth+px])*(64-int(w0))+32)>>6, bitDepth)
		}
	}
	return mask, nil
}
