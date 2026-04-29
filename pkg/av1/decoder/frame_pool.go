package decoder

import (
	"image"
	"time"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

var frameScratch8Cache = stickyScratchCache[byte]{maxStored: 16}

func takeReferenceFrame420(width, height int, colorCfg av1.ColorConfig, pts, duration time.Duration) *Frame {
	chromaWidth := (width + 1) >> 1
	chromaHeight := (height + 1) >> 1
	yLen := width * height
	cLen := chromaWidth * chromaHeight
	scratch := frameScratch8Cache.take(yLen + 2*cLen)
	if scratch == nil {
		return nil
	}
	buf := scratch.buf
	y := buf[:yLen]
	u := buf[yLen : yLen+cLen]
	v := buf[yLen+cLen : yLen+2*cLen]
	img := &image.YCbCr{
		Y:              y,
		Cb:             u,
		Cr:             v,
		YStride:        width,
		CStride:        chromaWidth,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, width, height),
	}
	colorCfg = normalizePreviewColorConfig(colorCfg, av1.Chroma420, 8)
	return &Frame{
		Width:    width,
		Height:   height,
		BitDepth: 8,
		Layout:   av1.Chroma420,
		Color:    colorCfg,
		PTS:      pts,
		Duration: duration,
		Image:    img,
		Y:        y,
		U:        u,
		V:        v,
		YStride:  width,
		UStride:  chromaWidth,
		VStride:  chromaWidth,
		scratch8: scratch,
	}
}

func cloneFrameForReference(src *Frame) *Frame {
	if src == nil {
		return nil
	}
	if len(src.Y) > 0 && src.Layout == av1.Chroma420 && src.BitDepth <= 8 {
		clone := takeReferenceFrame420(src.Width, src.Height, src.Color, src.PTS, src.Duration)
		if clone == nil {
			return cloneFrame(src)
		}
		copyReferenceFrame420(clone, src)
		return clone
	}
	return cloneFrame(src)
}

func fillFrame420Neutral(frame *Frame) {
	if frame == nil {
		return
	}
	clear(frame.Y)
	fillUint8(frame.U, 128)
	fillUint8(frame.V, 128)
}

func copyReferenceFrame420(frame *Frame, ref *Frame) {
	if frame == nil {
		return
	}
	if ref == nil {
		fillFrame420Neutral(frame)
		return
	}
	if len(ref.Y) >= len(frame.Y) {
		copy(frame.Y, ref.Y)
	} else {
		clear(frame.Y)
		copy(frame.Y, ref.Y)
	}
	if len(ref.U) >= len(frame.U) {
		copy(frame.U, ref.U)
	} else {
		fillUint8(frame.U, 128)
		copy(frame.U, ref.U)
	}
	if len(ref.V) >= len(frame.V) {
		copy(frame.V, ref.V)
	} else {
		fillUint8(frame.V, 128)
		copy(frame.V, ref.V)
	}
}

func putFrameScratch(frame *Frame) {
	if frame == nil || frame.scratch8 == nil {
		return
	}
	frameScratch8Cache.put(frame.scratch8)
	frame.scratch8 = nil
}
