package decoder

import (
	"fmt"
	"image"
	"image/draw"
	"time"

	"github.com/codemodify/av1go-codex/internal/displaycolor"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

func buildFrameFromYCbCr(img *image.YCbCr, colorCfg av1.ColorConfig, pts, duration time.Duration) *Frame {
	colorCfg = normalizePreviewColorConfig(colorCfg, colorCfg.ChromaSubsampling(), colorCfg.BitDepth)
	if img == nil {
		return &Frame{
			BitDepth: colorCfg.BitDepth,
			Layout:   colorCfg.ChromaSubsampling(),
			Color:    colorCfg,
			PTS:      pts,
			Duration: duration,
		}
	}
	return &Frame{
		Width:    img.Rect.Dx(),
		Height:   img.Rect.Dy(),
		BitDepth: colorCfg.BitDepth,
		Layout:   colorCfg.ChromaSubsampling(),
		Color:    colorCfg,
		PTS:      pts,
		Duration: duration,
		Image:    img,
		Y:        img.Y,
		U:        img.Cb,
		V:        img.Cr,
		YStride:  img.YStride,
		UStride:  img.CStride,
		VStride:  img.CStride,
	}
}

func buildFrameFromPlanar16(width, height int, colorCfg av1.ColorConfig, pts, duration time.Duration, y []uint16, yStride int, u []uint16, uStride int, v []uint16, vStride int) (*Frame, error) {
	colorCfg = normalizePreviewColorConfig(colorCfg, colorCfg.ChromaSubsampling(), colorCfg.BitDepth)
	img, err := previewImageFromPlanar16(width, height, colorCfg, y, yStride, u, uStride, v, vStride)
	if err != nil {
		return nil, err
	}
	return &Frame{
		Width:    width,
		Height:   height,
		BitDepth: colorCfg.BitDepth,
		Layout:   colorCfg.ChromaSubsampling(),
		Color:    colorCfg,
		PTS:      pts,
		Duration: duration,
		Image:    img,
		Y16:      y,
		U16:      u,
		V16:      v,
		YStride:  yStride,
		UStride:  uStride,
		VStride:  vStride,
	}, nil
}

func buildFrameFromPlanar16NoPreview(width, height int, colorCfg av1.ColorConfig, pts, duration time.Duration, y []uint16, yStride int, u []uint16, uStride int, v []uint16, vStride int) (*Frame, error) {
	colorCfg = normalizePreviewColorConfig(colorCfg, colorCfg.ChromaSubsampling(), colorCfg.BitDepth)
	if err := validatePlanar16(width, height, colorCfg.ChromaSubsampling(), y, yStride, u, uStride, v, vStride); err != nil {
		return nil, err
	}
	return &Frame{
		Width:    width,
		Height:   height,
		BitDepth: colorCfg.BitDepth,
		Layout:   colorCfg.ChromaSubsampling(),
		Color:    colorCfg,
		PTS:      pts,
		Duration: duration,
		Y16:      y,
		U16:      u,
		V16:      v,
		YStride:  yStride,
		UStride:  uStride,
		VStride:  vStride,
	}, nil
}

func validatePlanar16(width, height int, layout av1.ChromaSubsampling, y []uint16, yStride int, u []uint16, uStride int, v []uint16, vStride int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("decoder: invalid frame size %dx%d", width, height)
	}
	if yStride < width || len(y) < (height-1)*yStride+width {
		return fmt.Errorf("decoder: invalid 16-bit luma plane")
	}
	if layout == av1.Chroma400 {
		return nil
	}
	chromaWidth, chromaHeight := planeChromaDims(layout, width, height)
	if chromaWidth <= 0 || chromaHeight <= 0 {
		return fmt.Errorf("decoder: unsupported chroma layout %v", layout)
	}
	if uStride < chromaWidth || vStride < chromaWidth {
		return fmt.Errorf("decoder: invalid 16-bit chroma stride")
	}
	if len(u) < (chromaHeight-1)*uStride+chromaWidth || len(v) < (chromaHeight-1)*vStride+chromaWidth {
		return fmt.Errorf("decoder: invalid 16-bit chroma plane")
	}
	return nil
}

func previewImageFromPlanar8(width, height int, colorCfg av1.ColorConfig, y []byte, yStride int, u []byte, uStride int, v []byte, vStride int) (image.Image, error) {
	return previewImageRGBA(width, height, colorCfg, func(px, py int) (int, int, int) {
		yv := int(y[py*yStride+px])
		uv, vv := sampleChroma8(colorCfg, width, height, u, uStride, v, vStride, px, py)
		return yv, uv, vv
	})
}

func previewImageFromPlanar16(width, height int, colorCfg av1.ColorConfig, y []uint16, yStride int, u []uint16, uStride int, v []uint16, vStride int) (image.Image, error) {
	return previewImageRGBA(width, height, colorCfg, func(px, py int) (int, int, int) {
		yv := int(y[py*yStride+px])
		uv, vv := sampleChroma16(colorCfg, width, height, u, uStride, v, vStride, px, py)
		return yv, uv, vv
	})
}

func previewImageRGBA(width, height int, colorCfg av1.ColorConfig, sample func(px, py int) (int, int, int)) (image.Image, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("decoder: invalid frame size %dx%d", width, height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for py := 0; py < height; py++ {
		row := img.Pix[py*img.Stride:]
		for px := 0; px < width; px++ {
			yv, uv, vv := sample(px, py)
			r, g, b := yuvToRGB(colorCfg, width, height, yv, uv, vv)
			base := px * 4
			row[base+0] = r
			row[base+1] = g
			row[base+2] = b
			row[base+3] = 0xff
		}
	}
	return img, nil
}

func copyPlane16To8(dst []byte, dstStride, width, height int, src []uint16, srcStride int, bitDepth uint8) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if dstStride < width {
		return fmt.Errorf("destination stride %d < width %d", dstStride, width)
	}
	if srcStride < width {
		return fmt.Errorf("source stride %d < width %d", srcStride, width)
	}
	if len(dst) < (height-1)*dstStride+width {
		return fmt.Errorf("destination plane too small")
	}
	if len(src) < (height-1)*srcStride+width {
		return fmt.Errorf("source plane too small")
	}
	for py := 0; py < height; py++ {
		dstRow := dst[py*dstStride:]
		srcRow := src[py*srcStride:]
		for px := 0; px < width; px++ {
			dstRow[px] = downshiftTo8Bit(srcRow[px], bitDepth)
		}
	}
	return nil
}

func downshiftTo8Bit(v uint16, bitDepth uint8) uint8 {
	if bitDepth <= 8 {
		return clipByte(int(v))
	}
	shift := int(bitDepth) - 8
	return clipByte((int(v) + (1 << (shift - 1))) >> shift)
}

func sampleChroma8(colorCfg av1.ColorConfig, width, height int, u []byte, uStride int, v []byte, vStride int, px, py int) (int, int) {
	if colorCfg.Monochrome || colorCfg.ChromaSubsampling() == av1.Chroma400 {
		mid := 1 << maxIntLocal(int(colorCfg.BitDepth)-1, 7)
		return mid, mid
	}
	cx, cy := chromaCoords(colorCfg.ChromaSubsampling(), px, py)
	uw, uh := planeChromaDims(colorCfg.ChromaSubsampling(), width, height)
	cx = previewClampInt(cx, 0, maxIntLocal(uw-1, 0))
	cy = previewClampInt(cy, 0, maxIntLocal(uh-1, 0))
	return int(u[cy*uStride+cx]), int(v[cy*vStride+cx])
}

func sampleChroma16(colorCfg av1.ColorConfig, width, height int, u []uint16, uStride int, v []uint16, vStride int, px, py int) (int, int) {
	if colorCfg.Monochrome || colorCfg.ChromaSubsampling() == av1.Chroma400 {
		mid := 1 << maxIntLocal(int(colorCfg.BitDepth)-1, 7)
		return mid, mid
	}
	cx, cy := chromaCoords(colorCfg.ChromaSubsampling(), px, py)
	uw, uh := planeChromaDims(colorCfg.ChromaSubsampling(), width, height)
	cx = previewClampInt(cx, 0, maxIntLocal(uw-1, 0))
	cy = previewClampInt(cy, 0, maxIntLocal(uh-1, 0))
	return int(u[cy*uStride+cx]), int(v[cy*vStride+cx])
}

func chromaCoords(layout av1.ChromaSubsampling, px, py int) (int, int) {
	switch layout {
	case av1.Chroma420:
		return px >> 1, py >> 1
	case av1.Chroma422:
		return px >> 1, py
	case av1.Chroma444:
		return px, py
	default:
		return px, py
	}
}

func planeChromaDims(layout av1.ChromaSubsampling, width, height int) (int, int) {
	switch layout {
	case av1.Chroma420:
		return (width + 1) >> 1, (height + 1) >> 1
	case av1.Chroma422:
		return (width + 1) >> 1, height
	case av1.Chroma444:
		return width, height
	default:
		return 0, 0
	}
}

func yuvToRGB(colorCfg av1.ColorConfig, width, height int, yv, uv, vv int) (uint8, uint8, uint8) {
	_ = width
	_ = height
	return displaycolor.YUVToDisplayRGB(colorCfg, yv, uv, vv)
}

func previewMatrix(colorCfg av1.ColorConfig, width, height int) int {
	_ = width
	_ = height
	return displaycolor.PreviewMatrix(colorCfg)
}

const (
	previewMatrix709  = displaycolor.Matrix709
	previewMatrix601  = displaycolor.Matrix601
	previewMatrix2020 = displaycolor.Matrix2020
)

func normalizePreviewColorConfig(colorCfg av1.ColorConfig, layout av1.ChromaSubsampling, bitDepth uint8) av1.ColorConfig {
	if bitDepth == 0 {
		bitDepth = colorCfg.BitDepth
	}
	if bitDepth == 0 {
		bitDepth = 8
	}
	colorCfg.BitDepth = bitDepth
	switch layout {
	case av1.Chroma400:
		colorCfg.Monochrome = true
	case av1.Chroma420:
		colorCfg.Monochrome = false
		colorCfg.SubsamplingX = true
		colorCfg.SubsamplingY = true
	case av1.Chroma422:
		colorCfg.Monochrome = false
		colorCfg.SubsamplingX = true
		colorCfg.SubsamplingY = false
	case av1.Chroma444:
		colorCfg.Monochrome = false
		colorCfg.SubsamplingX = false
		colorCfg.SubsamplingY = false
	}
	return colorCfg
}

func previewClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func ycbcrSubsampleRatio(layout av1.ChromaSubsampling) (image.YCbCrSubsampleRatio, bool) {
	switch layout {
	case av1.Chroma420:
		return image.YCbCrSubsampleRatio420, true
	case av1.Chroma422:
		return image.YCbCrSubsampleRatio422, true
	case av1.Chroma444:
		return image.YCbCrSubsampleRatio444, true
	default:
		return image.YCbCrSubsampleRatio420, false
	}
}

func cloneFrame(src *Frame) *Frame {
	if src == nil {
		return nil
	}
	clone := *src
	clone.scratch8 = nil
	clone.Y = append([]byte(nil), src.Y...)
	clone.U = append([]byte(nil), src.U...)
	clone.V = append([]byte(nil), src.V...)
	clone.Y16 = append([]uint16(nil), src.Y16...)
	clone.U16 = append([]uint16(nil), src.U16...)
	clone.V16 = append([]uint16(nil), src.V16...)

	switch {
	case len(clone.Y16) > 0:
		clone.Color = normalizePreviewColorConfig(src.Color, src.Layout, src.BitDepth)
		clone.Image = nil
		return &clone
	case len(clone.Y) > 0:
		if ratio, ok := ycbcrSubsampleRatio(src.Layout); ok && len(clone.U) > 0 && len(clone.V) > 0 {
			clone.Color = normalizePreviewColorConfig(src.Color, src.Layout, src.BitDepth)
			clone.Image = &image.YCbCr{
				Y:              clone.Y,
				Cb:             clone.U,
				Cr:             clone.V,
				YStride:        src.YStride,
				CStride:        src.UStride,
				SubsampleRatio: ratio,
				Rect:           image.Rect(0, 0, src.Width, src.Height),
			}
			return &clone
		}
		colorCfg := normalizePreviewColorConfig(src.Color, src.Layout, src.BitDepth)
		img, err := previewImageFromPlanar8(src.Width, src.Height, colorCfg, clone.Y, src.YStride, clone.U, src.UStride, clone.V, src.VStride)
		if err == nil {
			clone.Color = colorCfg
			clone.Image = img
			return &clone
		}
	}

	if src.Image != nil {
		bounds := src.Image.Bounds()
		img := image.NewRGBA(bounds)
		draw.Draw(img, bounds, src.Image, bounds.Min, draw.Src)
		clone.Image = img
	}
	return &clone
}
