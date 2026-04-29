package displaycolor

import (
	"math"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

const (
	Matrix709  = 709
	Matrix601  = 601
	Matrix2020 = 2020

	transferBT709     = 1
	transferLinear    = 8
	transferSRGB      = 13
	transferBT2020_10 = 14
	transferBT2020_12 = 15
	transferST2084    = 16
	transferHLG       = 18

	primariesBT2020 = 9
)

func PreviewMatrix(colorCfg av1.ColorConfig) int {
	switch colorCfg.MatrixCoefficients {
	case 1:
		return Matrix709
	case 5, 6, 7:
		return Matrix601
	case 9, 10:
		return Matrix2020
	case 0:
		return Matrix709
	default:
		return Matrix601
	}
}

func CanUseSimple8BitLUT(colorCfg av1.ColorConfig) bool {
	if colorCfg.BitDepth != 8 {
		return false
	}
	if colorCfg.MatrixCoefficients == 0 && colorCfg.ChromaSubsampling() == av1.Chroma444 && !colorCfg.Monochrome {
		return false
	}
	return !needsAdvancedDisplayTransform(colorCfg)
}

func YUVToDisplayRGB(colorCfg av1.ColorConfig, yv, uv, vv int) (uint8, uint8, uint8) {
	if colorCfg.MatrixCoefficients == 0 && colorCfg.ChromaSubsampling() == av1.Chroma444 && !colorCfg.Monochrome {
		if colorCfg.BitDepth <= 8 {
			return clipByte(yv), clipByte(uv), clipByte(vv)
		}
		return downshiftTo8Bit(uint16(yv), colorCfg.BitDepth), downshiftTo8Bit(uint16(uv), colorCfg.BitDepth), downshiftTo8Bit(uint16(vv), colorCfg.BitDepth)
	}

	r, g, b := yuvToRGBSignal(colorCfg, yv, uv, vv)
	r, g, b = displayTransformRGB(colorCfg, r, g, b)
	return floatToByte(r), floatToByte(g), floatToByte(b)
}

func yuvToRGBSignal(colorCfg av1.ColorConfig, yv, uv, vv int) (float64, float64, float64) {
	bitDepth := maxInt(int(colorCfg.BitDepth), 8)
	scaleInt := 1 << uint(maxInt(bitDepth-8, 0))
	maxValueInt := (1 << uint(bitDepth)) - 1
	scale := float64(scaleInt)
	maxValue := float64(maxValueInt)
	yf := float64(yv)
	uf := float64(uv)
	vf := float64(vv)
	var yNorm, cb, cr float64
	if colorCfg.FullRange {
		yNorm = yf / maxValue
		cb = uf/maxValue - 0.5
		cr = vf/maxValue - 0.5
	} else {
		yNorm = (yf - 16.0*scale) / (219.0 * scale)
		cb = (uf - 128.0*scale) / (224.0 * scale)
		cr = (vf - 128.0*scale) / (224.0 * scale)
	}
	if yNorm < 0 {
		yNorm = 0
	}
	switch PreviewMatrix(colorCfg) {
	case Matrix601:
		return yNorm + 1.4020*cr, yNorm - 0.344136*cb - 0.714136*cr, yNorm + 1.7720*cb
	case Matrix2020:
		return yNorm + 1.4746*cr, yNorm - 0.164553*cb - 0.571353*cr, yNorm + 1.8814*cb
	default:
		return yNorm + 1.5748*cr, yNorm - 0.187324*cb - 0.468124*cr, yNorm + 1.8556*cb
	}
}

func displayTransformRGB(colorCfg av1.ColorConfig, r, g, b float64) (float64, float64, float64) {
	if !needsAdvancedDisplayTransform(colorCfg) {
		return clamp01(r), clamp01(g), clamp01(b)
	}

	lr := inverseTransfer(colorCfg.TransferCharacteristics, clamp01(r))
	lg := inverseTransfer(colorCfg.TransferCharacteristics, clamp01(g))
	lb := inverseTransfer(colorCfg.TransferCharacteristics, clamp01(b))

	if colorCfg.ColorPrimaries == primariesBT2020 {
		lr, lg, lb = bt2020ToBT709Linear(lr, lg, lb)
	}

	switch colorCfg.TransferCharacteristics {
	case transferST2084:
		lr = toneMapPQLinear(lr)
		lg = toneMapPQLinear(lg)
		lb = toneMapPQLinear(lb)
	case transferHLG:
		lr = toneMapHLGLinear(lr)
		lg = toneMapHLGLinear(lg)
		lb = toneMapHLGLinear(lb)
	}

	return linearToSRGB(clamp01(lr)), linearToSRGB(clamp01(lg)), linearToSRGB(clamp01(lb))
}

func needsAdvancedDisplayTransform(colorCfg av1.ColorConfig) bool {
	switch colorCfg.TransferCharacteristics {
	case 0, transferBT709:
		return colorCfg.ColorPrimaries == primariesBT2020
	case transferLinear, transferSRGB, transferBT2020_10, transferBT2020_12, transferST2084, transferHLG:
		return true
	default:
		return colorCfg.ColorPrimaries == primariesBT2020
	}
}

func inverseTransfer(transfer uint8, v float64) float64 {
	switch transfer {
	case transferLinear:
		return v
	case transferSRGB:
		return sRGBToLinear(v)
	case transferST2084:
		return pqToLinear(v)
	case transferHLG:
		return hlgToLinear(v)
	case transferBT2020_10, transferBT2020_12, transferBT709, 0:
		return bt709ToLinear(v)
	default:
		return bt709ToLinear(v)
	}
}

func bt709ToLinear(v float64) float64 {
	v = clamp01(v)
	if v < 0.081 {
		return v / 4.5
	}
	return math.Pow((v+0.099)/1.099, 1.0/0.45)
}

func sRGBToLinear(v float64) float64 {
	v = clamp01(v)
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSRGB(v float64) float64 {
	v = clamp01(v)
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1.0/2.4) - 0.055
}

func pqToLinear(v float64) float64 {
	v = clamp01(v)
	const (
		m1 = 2610.0 / 16384.0
		m2 = 2523.0 / 32.0
		c1 = 3424.0 / 4096.0
		c2 = 2413.0 / 128.0
		c3 = 2392.0 / 128.0
	)
	p := math.Pow(v, 1.0/m2)
	num := maxFloat(p-c1, 0)
	den := c2 - c3*p
	if den <= 0 {
		return 0
	}
	return math.Pow(num/den, 1.0/m1)
}

func hlgToLinear(v float64) float64 {
	v = clamp01(v)
	const (
		a = 0.17883277
		b = 0.28466892
		c = 0.55991073
	)
	if v <= 0.5 {
		return (v * v) / 3.0
	}
	return (math.Exp((v-c)/a) + b) / 12.0
}

func toneMapPQLinear(v float64) float64 {
	// v is linear light relative to 10000 nits. Compress that into an SDR-ish
	// preview range aggressively enough that HDR frames stay visually useful.
	x := clamp01(v) * 100.0
	return x / (1.0 + x)
}

func toneMapHLGLinear(v float64) float64 {
	x := maxFloat(v, 0) * 4.0
	return x / (1.0 + x)
}

func bt2020ToBT709Linear(r, g, b float64) (float64, float64, float64) {
	return 1.6605*r - 0.5876*g - 0.0728*b,
		-0.1246*r + 1.1329*g - 0.0083*b,
		-0.0182*r - 0.1006*g + 1.1187*b
}

func floatToByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v*255.0 + 0.5)
}

func downshiftTo8Bit(v uint16, bitDepth uint8) uint8 {
	if bitDepth <= 8 {
		return clipByte(int(v))
	}
	shift := int(bitDepth) - 8
	return clipByte((int(v) + (1 << (shift - 1))) >> shift)
}

func clipByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
