package decoder

import (
	"fmt"
	"image"
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

var obmcByteScratchPool sync.Pool
var obmcUint16ScratchPool sync.Pool

var obmcMasks = [64]uint8{
	0, 0,
	19, 0,
	25, 14, 5, 0,
	28, 22, 16, 11, 7, 3, 0, 0,
	30, 27, 24, 21, 18, 15, 12, 10, 8, 6, 4, 3, 0, 0, 0, 0,
	31, 29, 28, 26, 24, 23, 21, 20, 19, 17, 16, 14, 13, 12, 11, 9,
	8, 7, 6, 5, 4, 4, 3, 2, 0, 0, 0, 0, 0, 0, 0, 0,
}

func blendOBMCVertical8(dst []byte, dstStride int, tmp []byte, width, height int) {
	if width <= 1 || height <= 0 {
		return
	}
	mask := obmcMasks[width:]
	blendWidth := minInt(width, (width*3)>>2)
	for y := 0; y < height; y++ {
		row := dst[y*dstStride:]
		src := tmp[y*width:]
		for x := 0; x < blendWidth; x++ {
			m := int(mask[x])
			row[x] = clipByte((int(row[x])*(64-m) + int(src[x])*m + 32) >> 6)
		}
	}
}

func blendOBMCHorizontal8(dst []byte, dstStride int, tmp []byte, width, height int) {
	if width <= 0 || height <= 1 {
		return
	}
	mask := obmcMasks[height:]
	blendHeight := minInt(height, (height*3)>>2)
	for y := 0; y < blendHeight; y++ {
		row := dst[y*dstStride:]
		src := tmp[y*width:]
		m := int(mask[y])
		for x := 0; x < width; x++ {
			row[x] = clipByte((int(row[x])*(64-m) + int(src[x])*m + 32) >> 6)
		}
	}
}

func blendOBMCVertical16(dst []uint16, dstStride int, tmp []uint16, width, height int, bitDepth uint8) {
	if width <= 1 || height <= 0 {
		return
	}
	mask := obmcMasks[width:]
	blendWidth := minInt(width, (width*3)>>2)
	for y := 0; y < height; y++ {
		row := dst[y*dstStride:]
		src := tmp[y*width:]
		for x := 0; x < blendWidth; x++ {
			m := int(mask[x])
			row[x] = clipSample((int(row[x])*(64-m)+int(src[x])*m+32)>>6, bitDepth)
		}
	}
}

func blendOBMCHorizontal16(dst []uint16, dstStride int, tmp []uint16, width, height int, bitDepth uint8) {
	if width <= 0 || height <= 1 {
		return
	}
	mask := obmcMasks[height:]
	blendHeight := minInt(height, (height*3)>>2)
	for y := 0; y < blendHeight; y++ {
		row := dst[y*dstStride:]
		src := tmp[y*width:]
		m := int(mask[y])
		for x := 0; x < width; x++ {
			row[x] = clipSample((int(row[x])*(64-m)+int(src[x])*m+32)>>6, bitDepth)
		}
	}
}

func chromaSubsamplingShifts(layout av1.ChromaSubsampling) (int, int) {
	ssHor := 0
	ssVer := 0
	if layout != av1.Chroma444 {
		ssHor = 1
	}
	if layout == av1.Chroma420 {
		ssVer = 1
	}
	return ssHor, ssVer
}

func (d *Decoder) applyInterOBMCLuma8(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, img *image.YCbCr) error {
	if parsed == nil || mvTile == nil || img == nil {
		return nil
	}
	x := g.Start4X * 4
	y := g.Start4Y * 4
	w := minInt(g.Width4()*4, parsedPlaneWidth(img)-x)
	h := minInt(g.Height4()*4, parsedPlaneHeight(img)-y)
	if w <= 0 || h <= 0 {
		return nil
	}
	if g.Start4Y > mvTile.Geometry.Start4Y {
		if err := d.applyInterOBMCTop8(parsed, mvTile, g, img, x, y, w, h); err != nil {
			return err
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X {
		if err := d.applyInterOBMCLeft8(parsed, mvTile, g, img, x, y, w, h); err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) applyInterOBMCChroma8(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, img *image.YCbCr) error {
	if parsed == nil || mvTile == nil || img == nil {
		return nil
	}
	layout := d.header.ColorConfig.ChromaSubsampling()
	if layout == av1.Chroma400 {
		return nil
	}
	chroma := chromaGeometryForBlock(layout, g)
	planeWidth := parsedChromaPlaneWidth(img)
	planeHeight := parsedChromaPlaneHeight(img)
	x := chroma.Start4X * 4
	y := chroma.Start4Y * 4
	w := minInt(chroma.Width4()*4, planeWidth-x)
	h := minInt(chroma.Height4()*4, planeHeight-y)
	if w <= 0 || h <= 0 {
		return nil
	}
	if g.Start4Y > mvTile.Geometry.Start4Y {
		if err := d.applyInterOBMCTopChroma8(parsed, mvTile, g, img.Cb, img.CStride, planeWidth, planeHeight, 1, x, y, w, h); err != nil {
			return err
		}
		if err := d.applyInterOBMCTopChroma8(parsed, mvTile, g, img.Cr, img.CStride, planeWidth, planeHeight, 2, x, y, w, h); err != nil {
			return err
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X {
		if err := d.applyInterOBMCLeftChroma8(parsed, mvTile, g, img.Cb, img.CStride, planeWidth, planeHeight, 1, x, y, w, h); err != nil {
			return err
		}
		if err := d.applyInterOBMCLeftChroma8(parsed, mvTile, g, img.Cr, img.CStride, planeWidth, planeHeight, 2, x, y, w, h); err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) applyInterOBMCLuma16(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []uint16, stride, planeWidth, planeHeight int) error {
	if parsed == nil || mvTile == nil || len(plane) == 0 {
		return nil
	}
	x := g.Start4X * 4
	y := g.Start4Y * 4
	w := minInt(g.Width4()*4, planeWidth-x)
	h := minInt(g.Height4()*4, planeHeight-y)
	if w <= 0 || h <= 0 {
		return nil
	}
	if g.Start4Y > mvTile.Geometry.Start4Y {
		if err := d.applyInterOBMCTop16(parsed, mvTile, g, plane, stride, x, y, w, h); err != nil {
			return err
		}
		debugTraceInterFrameLumaSample16(parsed, "post-single-obmc-top", plane, stride, planeWidth, planeHeight)
	}
	if g.Start4X > mvTile.Geometry.Start4X {
		if err := d.applyInterOBMCLeft16(parsed, mvTile, g, plane, stride, x, y, w, h); err != nil {
			return err
		}
		debugTraceInterFrameLumaSample16(parsed, "post-single-obmc-left", plane, stride, planeWidth, planeHeight)
	}
	return nil
}

func (d *Decoder) applyInterOBMCChroma16(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, uPlane []uint16, uStride, uWidth, uHeight int, vPlane []uint16, vStride, vWidth, vHeight int) error {
	if parsed == nil || mvTile == nil {
		return nil
	}
	layout := d.header.ColorConfig.ChromaSubsampling()
	if layout == av1.Chroma400 {
		return nil
	}
	chroma := chromaGeometryForBlock(layout, g)
	x := chroma.Start4X * 4
	y := chroma.Start4Y * 4
	uW := minInt(chroma.Width4()*4, uWidth-x)
	uH := minInt(chroma.Height4()*4, uHeight-y)
	vW := minInt(chroma.Width4()*4, vWidth-x)
	vH := minInt(chroma.Height4()*4, vHeight-y)
	if uW <= 0 || uH <= 0 || vW <= 0 || vH <= 0 {
		return nil
	}
	if g.Start4Y > mvTile.Geometry.Start4Y {
		if err := d.applyInterOBMCTopChroma16(parsed, mvTile, g, uPlane, uStride, uWidth, uHeight, 1, x, y, uW, uH); err != nil {
			return err
		}
		if err := d.applyInterOBMCTopChroma16(parsed, mvTile, g, vPlane, vStride, vWidth, vHeight, 2, x, y, vW, vH); err != nil {
			return err
		}
	}
	if g.Start4X > mvTile.Geometry.Start4X {
		if err := d.applyInterOBMCLeftChroma16(parsed, mvTile, g, uPlane, uStride, uWidth, uHeight, 1, x, y, uW, uH); err != nil {
			return err
		}
		if err := d.applyInterOBMCLeftChroma16(parsed, mvTile, g, vPlane, vStride, vWidth, vHeight, 2, x, y, vW, vH); err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) applyInterOBMCTop8(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, img *image.YCbCr, x, y, w, h int) error {
	dims := BlockDims(g.Size)
	overlapH := (minInt(int(dims.H4), 16) >> 1) * 4
	overlapH = minInt(overlapH, h)
	if overlapH <= 1 {
		return nil
	}
	scratch := takeScratch[byte](&obmcByteScratchPool, w*overlapH)
	defer putScratch(&obmcByteScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2W4), 4)
	for segments, x4 := 0, 0; x4 < g.Width4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X+x4+1, g.Start4Y-1)
		step4 := overlapStep4(b, true)
		segW := minInt(step4*4, w-x4*4)
		if segW <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc top reference frame for ref %d", b.Ref[0])
			}
			tmp := scratch.buf[:segW*overlapH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], 0, d.header.ColorConfig.ChromaSubsampling())
				if ok {
					if err := predictInterPlane8WarpAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+x4*4, y, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, warp); err != nil {
						return err
					}
				} else {
					predictInterLumaBlock8WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+x4*4, y, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, b.MV[0], b.Filter)
				}
			} else {
				predictInterLumaBlock8WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+x4*4, y, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, b.MV[0], b.Filter)
			}
			dst := img.Y[y*img.YStride+x+x4*4:]
			blendOBMCHorizontal8(dst, img.YStride, tmp, segW, overlapH)
			segments++
		}
		x4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCTop16(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []uint16, stride, x, y, w, h int) error {
	bitDepth := d.header.ColorConfig.BitDepth
	dims := BlockDims(g.Size)
	overlapH := (minInt(int(dims.H4), 16) >> 1) * 4
	overlapH = minInt(overlapH, h)
	if overlapH <= 1 {
		return nil
	}
	scratch := takeScratch[uint16](&obmcUint16ScratchPool, w*overlapH)
	defer putScratch(&obmcUint16ScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2W4), 4)
	for segments, x4 := 0, 0; x4 < g.Width4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X+x4+1, g.Start4Y-1)
		step4 := overlapStep4(b, true)
		segW := minInt(step4*4, w-x4*4)
		if segW <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc top hbd reference frame for ref %d", b.Ref[0])
			}
			refPlane, refStride, refWidth, refHeight := referenceLuma16(refFrame, bitDepth)
			tmp := scratch.buf[:segW*overlapH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], 0, d.header.ColorConfig.ChromaSubsampling())
				if ok {
					if err := predictInterPlane16WarpAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+x4*4, y, refPlane, refStride, refWidth, refHeight, warp, bitDepth); err != nil {
						return err
					}
				} else if err := predictInterLumaBlock16WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+x4*4, y, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
					return err
				}
			} else if err := predictInterLumaBlock16WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+x4*4, y, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
				return err
			}
			dst := plane[y*stride+x+x4*4:]
			debugTraceOBMCSample16(parsed, "top-pre", g, b, x+x4*4, y, segW, overlapH, tmp, segW, plane, stride, parsed.Header.Width, parsed.Header.Height)
			blendOBMCHorizontal16(dst, stride, tmp, segW, overlapH, bitDepth)
			debugTraceOBMCSample16(parsed, "top-post", g, b, x+x4*4, y, segW, overlapH, tmp, segW, plane, stride, parsed.Header.Width, parsed.Header.Height)
			segments++
		}
		x4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCTopChroma8(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []byte, stride, planeWidth, planeHeight, planeIdx, x, y, w, h int) error {
	layout := d.header.ColorConfig.ChromaSubsampling()
	dims := BlockDims(g.Size)
	ssHor, ssVer := chromaSubsamplingShifts(layout)
	pelStepX := 4 >> ssHor
	pelStepY := 4 >> ssVer
	if int(dims.W4)*pelStepX+int(dims.H4)*pelStepY < 16 {
		return nil
	}
	overlapH := (minInt(int(dims.H4), 16) >> 1) * pelStepY
	overlapH = minInt(overlapH, h)
	if overlapH <= 1 {
		return nil
	}
	scratch := takeScratch[byte](&obmcByteScratchPool, w*overlapH)
	defer putScratch(&obmcByteScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2W4), 4)
	for segments, x4 := 0, 0; x4 < g.Width4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X+x4+1, g.Start4Y-1)
		step4 := overlapStep4(b, true)
		segX := x4 * pelStepX
		segW := minInt(step4*pelStepX, w-segX)
		if segW <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc top chroma reference frame for ref %d", b.Ref[0])
			}
			var refPlane []byte
			var refStride, refWidth, refHeight int
			switch planeIdx {
			case 1:
				refPlane = refFrame.U
				refStride = refFrame.UStride
			case 2:
				refPlane = refFrame.V
				refStride = refFrame.VStride
			default:
				return fmt.Errorf("decoder: invalid chroma plane index %d", planeIdx)
			}
			refWidth, refHeight = planeChromaDims(layout, refFrame.Width, refFrame.Height)
			tmp := scratch.buf[:segW*overlapH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], planeIdx, layout)
				if ok {
					if err := predictInterPlane8WarpAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+segX, y, refPlane, refStride, refWidth, refHeight, warp); err != nil {
						return err
					}
				} else {
					predictInterChromaBlock8WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+segX, y, refPlane, refStride, refWidth, refHeight, b.MV[0], b.Filter)
				}
			} else {
				predictInterChromaBlock8WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+segX, y, refPlane, refStride, refWidth, refHeight, b.MV[0], b.Filter)
			}
			dst := plane[y*stride+x+segX:]
			blendOBMCHorizontal8(dst, stride, tmp, segW, overlapH)
			segments++
		}
		x4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCTopChroma16(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []uint16, stride, planeWidth, planeHeight, planeIdx, x, y, w, h int) error {
	layout := d.header.ColorConfig.ChromaSubsampling()
	bitDepth := d.header.ColorConfig.BitDepth
	dims := BlockDims(g.Size)
	ssHor, ssVer := chromaSubsamplingShifts(layout)
	pelStepX := 4 >> ssHor
	pelStepY := 4 >> ssVer
	if int(dims.W4)*pelStepX+int(dims.H4)*pelStepY < 16 {
		return nil
	}
	overlapH := (minInt(int(dims.H4), 16) >> 1) * pelStepY
	overlapH = minInt(overlapH, h)
	if overlapH <= 1 {
		return nil
	}
	scratch := takeScratch[uint16](&obmcUint16ScratchPool, w*overlapH)
	defer putScratch(&obmcUint16ScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2W4), 4)
	for segments, x4 := 0, 0; x4 < g.Width4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X+x4+1, g.Start4Y-1)
		step4 := overlapStep4(b, true)
		segX := x4 * pelStepX
		segW := minInt(step4*pelStepX, w-segX)
		if segW <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc top hbd chroma reference frame for ref %d", b.Ref[0])
			}
			refPlane, refStride, refWidth, refHeight := referenceChroma16(refFrame, planeIdx, bitDepth)
			tmp := scratch.buf[:segW*overlapH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], planeIdx, layout)
				if ok {
					if err := predictInterPlane16WarpAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+segX, y, refPlane, refStride, refWidth, refHeight, warp, bitDepth); err != nil {
						return err
					}
				} else if err := predictInterChromaBlock16WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+segX, y, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
					return err
				}
			} else if err := predictInterChromaBlock16WithFiltersAt(tmp, segW, segW, overlapH, 0, 0, segW, overlapH, x+segX, y, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
				return err
			}
			dst := plane[y*stride+x+segX:]
			blendOBMCHorizontal16(dst, stride, tmp, segW, overlapH, bitDepth)
			segments++
		}
		x4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCLeft8(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, img *image.YCbCr, x, y, w, h int) error {
	dims := BlockDims(g.Size)
	overlapW := (minInt(int(dims.W4), 16) >> 1) * 4
	overlapW = minInt(overlapW, w)
	if overlapW <= 1 {
		return nil
	}
	scratch := takeScratch[byte](&obmcByteScratchPool, overlapW*h)
	defer putScratch(&obmcByteScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2H4), 4)
	for segments, y4 := 0, 0; y4 < g.Height4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X-1, g.Start4Y+y4+1)
		step4 := overlapStep4(b, false)
		segH := minInt(step4*4, h-y4*4)
		if segH <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc left reference frame for ref %d", b.Ref[0])
			}
			tmp := scratch.buf[:overlapW*segH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], 0, d.header.ColorConfig.ChromaSubsampling())
				if ok {
					if err := predictInterPlane8WarpAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+y4*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, warp); err != nil {
						return err
					}
				} else {
					predictInterLumaBlock8WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+y4*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, b.MV[0], b.Filter)
				}
			} else {
				predictInterLumaBlock8WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+y4*4, refFrame.Y, refFrame.YStride, refFrame.Width, refFrame.Height, b.MV[0], b.Filter)
			}
			dst := img.Y[(y+y4*4)*img.YStride+x:]
			blendOBMCVertical8(dst, img.YStride, tmp, overlapW, segH)
			segments++
		}
		y4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCLeft16(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []uint16, stride, x, y, w, h int) error {
	bitDepth := d.header.ColorConfig.BitDepth
	dims := BlockDims(g.Size)
	overlapW := (minInt(int(dims.W4), 16) >> 1) * 4
	overlapW = minInt(overlapW, w)
	if overlapW <= 1 {
		return nil
	}
	scratch := takeScratch[uint16](&obmcUint16ScratchPool, overlapW*h)
	defer putScratch(&obmcUint16ScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2H4), 4)
	for segments, y4 := 0, 0; y4 < g.Height4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X-1, g.Start4Y+y4+1)
		step4 := overlapStep4(b, false)
		segH := minInt(step4*4, h-y4*4)
		if segH <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc left hbd reference frame for ref %d", b.Ref[0])
			}
			refPlane, refStride, refWidth, refHeight := referenceLuma16(refFrame, bitDepth)
			tmp := scratch.buf[:overlapW*segH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], 0, d.header.ColorConfig.ChromaSubsampling())
				if ok {
					if err := predictInterPlane16WarpAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+y4*4, refPlane, refStride, refWidth, refHeight, warp, bitDepth); err != nil {
						return err
					}
				} else if err := predictInterLumaBlock16WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+y4*4, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
					return err
				}
			} else if err := predictInterLumaBlock16WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+y4*4, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
				return err
			}
			dst := plane[(y+y4*4)*stride+x:]
			debugTraceOBMCSample16(parsed, "left-pre", g, b, x, y+y4*4, overlapW, segH, tmp, overlapW, plane, stride, parsed.Header.Width, parsed.Header.Height)
			blendOBMCVertical16(dst, stride, tmp, overlapW, segH, bitDepth)
			debugTraceOBMCSample16(parsed, "left-post", g, b, x, y+y4*4, overlapW, segH, tmp, overlapW, plane, stride, parsed.Header.Width, parsed.Header.Height)
			segments++
		}
		y4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCLeftChroma8(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []byte, stride, planeWidth, planeHeight, planeIdx, x, y, w, h int) error {
	layout := d.header.ColorConfig.ChromaSubsampling()
	dims := BlockDims(g.Size)
	ssHor, ssVer := chromaSubsamplingShifts(layout)
	pelStepX := 4 >> ssHor
	pelStepY := 4 >> ssVer
	overlapW := (minInt(int(dims.W4), 16) >> 1) * pelStepX
	overlapW = minInt(overlapW, w)
	if overlapW <= 1 {
		return nil
	}
	scratch := takeScratch[byte](&obmcByteScratchPool, overlapW*h)
	defer putScratch(&obmcByteScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2H4), 4)
	for segments, y4 := 0, 0; y4 < g.Height4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X-1, g.Start4Y+y4+1)
		step4 := overlapStep4(b, false)
		segY := y4 * pelStepY
		segH := minInt(step4*pelStepY, h-segY)
		if segH <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc left chroma reference frame for ref %d", b.Ref[0])
			}
			var refPlane []byte
			var refStride, refWidth, refHeight int
			switch planeIdx {
			case 1:
				refPlane = refFrame.U
				refStride = refFrame.UStride
			case 2:
				refPlane = refFrame.V
				refStride = refFrame.VStride
			default:
				return fmt.Errorf("decoder: invalid chroma plane index %d", planeIdx)
			}
			refWidth, refHeight = planeChromaDims(layout, refFrame.Width, refFrame.Height)
			tmp := scratch.buf[:overlapW*segH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], planeIdx, layout)
				if ok {
					if err := predictInterPlane8WarpAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+segY, refPlane, refStride, refWidth, refHeight, warp); err != nil {
						return err
					}
				} else {
					predictInterChromaBlock8WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+segY, refPlane, refStride, refWidth, refHeight, b.MV[0], b.Filter)
				}
			} else {
				predictInterChromaBlock8WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+segY, refPlane, refStride, refWidth, refHeight, b.MV[0], b.Filter)
			}
			dst := plane[(y+segY)*stride+x:]
			blendOBMCVertical8(dst, stride, tmp, overlapW, segH)
			segments++
		}
		y4 += step4
	}
	return nil
}

func (d *Decoder) applyInterOBMCLeftChroma16(parsed *ParsedFrame, mvTile *SpatialMVTile, g BlockGeometry, plane []uint16, stride, planeWidth, planeHeight, planeIdx, x, y, w, h int) error {
	layout := d.header.ColorConfig.ChromaSubsampling()
	bitDepth := d.header.ColorConfig.BitDepth
	dims := BlockDims(g.Size)
	ssHor, ssVer := chromaSubsamplingShifts(layout)
	pelStepX := 4 >> ssHor
	pelStepY := 4 >> ssVer
	overlapW := (minInt(int(dims.W4), 16) >> 1) * pelStepX
	overlapW = minInt(overlapW, w)
	if overlapW <= 1 {
		return nil
	}
	scratch := takeScratch[uint16](&obmcUint16ScratchPool, overlapW*h)
	defer putScratch(&obmcUint16ScratchPool, scratch)
	maxSegments := minInt(int(dims.Log2H4), 4)
	for segments, y4 := 0, 0; y4 < g.Height4() && segments < maxSegments; {
		b := mvTile.block(g.Start4X-1, g.Start4Y+y4+1)
		step4 := overlapStep4(b, false)
		segY := y4 * pelStepY
		segH := minInt(step4*pelStepY, h-segY)
		if segH <= 0 {
			break
		}
		if validOBMCBlock(b) {
			refFrame := d.resolvePureGoBlockReference(&parsed.Header, b.Ref[0])
			if refFrame == nil {
				return fmt.Errorf("decoder: missing obmc left hbd chroma reference frame for ref %d", b.Ref[0])
			}
			refPlane, refStride, refWidth, refHeight := referenceChroma16(refFrame, planeIdx, bitDepth)
			tmp := scratch.buf[:overlapW*segH]
			if b.RefFlags[0]&refMVFlagGlobal != 0 && globalMotionUsesWarp(&parsed.Header, b.Ref[0]) {
				warp, ok := globalMotionWarpForPlane(&parsed.Header, b.Ref[0], planeIdx, layout)
				if ok {
					if err := predictInterPlane16WarpAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+segY, refPlane, refStride, refWidth, refHeight, warp, bitDepth); err != nil {
						return err
					}
				} else if err := predictInterChromaBlock16WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+segY, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
					return err
				}
			} else if err := predictInterChromaBlock16WithFiltersAt(tmp, overlapW, overlapW, segH, 0, 0, overlapW, segH, x, y+segY, refPlane, refStride, refWidth, refHeight, b.MV[0], bitDepth, b.Filter); err != nil {
				return err
			}
			dst := plane[(y+segY)*stride+x:]
			blendOBMCVertical16(dst, stride, tmp, overlapW, segH, bitDepth)
			segments++
		}
		y4 += step4
	}
	return nil
}

func validOBMCBlock(b *SpatialMVBlock) bool {
	return b != nil && b.Valid && !b.Intra && b.Ref[0] >= 0
}

func defaultInterFilterPair() InterFilterPair {
	return InterFilterPair{interFilterRegular, interFilterRegular}
}
