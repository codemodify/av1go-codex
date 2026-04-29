package decoder

import (
	"image"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

const (
	restorationTypeNone uint8 = iota
	restorationTypeSwitchable
	restorationTypeWiener
	restorationTypeSGRProj
)

var (
	generatedRestoreWienerCDF     = [2]uint16{21198, 0}
	generatedRestoreSGRProjCDF    = [2]uint16{15913, 0}
	generatedRestoreSwitchableCDF = [4]uint16{23355, 10187, 0, 0}
	restoreSGRParams              = [16][2]uint16{
		{140, 3236}, {112, 2158}, {93, 1618}, {80, 1438},
		{70, 1295}, {58, 1177}, {47, 1079}, {37, 996},
		{30, 925}, {25, 863}, {0, 2589}, {0, 1618},
		{0, 1177}, {0, 925}, {56, 0}, {22, 0},
	}
)

type restorationEntropyDecoder interface {
	DecodeBoolAdapt([]uint16) uint32
	DecodeSymbolAdapt([]uint16, int) uint32
	DecodeSubexp(ref, n int, k uint) int
	DecodeBools(n uint) uint32
}

type RestorationUnit struct {
	Type       uint8
	Parsed     bool
	FilterH    [3]int8
	FilterV    [3]int8
	SGRWeights [2]int8
}

type RestorationPlane struct {
	Enabled      bool
	UnitSizeLog2 uint8
	UnitSize     int
	Width        int
	Height       int
	UnitsX       int
	UnitsY       int
	Units        []RestorationUnit
}

type RestorationState struct {
	Planes [3]RestorationPlane
}

func NewRestorationState(seq av1.SequenceHeader, hdr *obu.FrameHeader) *RestorationState {
	if hdr == nil || hdr.SuperResEnabled || seq.ColorConfig.ChromaSubsampling() != av1.Chroma420 {
		return nil
	}
	state := &RestorationState{}
	frameWidth := hdr.UpscaledWidth
	if frameWidth <= 0 {
		frameWidth = hdr.Width
	}
	frameHeight := hdr.Height
	enabled := false
	for plane := 0; plane < 3; plane++ {
		frameType := hdr.Restoration.Type[plane]
		if frameType == restorationTypeNone {
			continue
		}
		ssX, ssY := restorationSubsampling(plane)
		width := frameWidth >> ssX
		height := frameHeight >> ssY
		if width <= 0 || height <= 0 {
			continue
		}
		unitLog2 := hdr.Restoration.UnitSize[boolToInt(plane > 0)]
		if unitLog2 == 0 {
			continue
		}
		unitSize := 1 << unitLog2
		unitsX := maxIntLocal(1, (width+(unitSize>>1))>>unitLog2)
		unitsY := maxIntLocal(1, (height+(unitSize>>1))>>unitLog2)
		state.Planes[plane] = RestorationPlane{
			Enabled:      true,
			UnitSizeLog2: unitLog2,
			UnitSize:     unitSize,
			Width:        width,
			Height:       height,
			UnitsX:       unitsX,
			UnitsY:       unitsY,
			Units:        make([]RestorationUnit, unitsX*unitsY),
		}
		enabled = true
	}
	if !enabled {
		return nil
	}
	return state
}

func newTileRestorationRefs() [3]RestorationUnit {
	var refs [3]RestorationUnit
	for plane := range refs {
		refs[plane] = defaultRestorationUnit()
	}
	return refs
}

func defaultRestorationUnit() RestorationUnit {
	return RestorationUnit{
		FilterH:    [3]int8{3, -7, 15},
		FilterV:    [3]int8{3, -7, 15},
		SGRWeights: [2]int8{-32, 31},
	}
}

func restorationSubsampling(plane int) (int, int) {
	if plane == 0 {
		return 0, 0
	}
	return 1, 1
}

func parseRestorationUnitsForSuperBlock(hdr *obu.FrameHeader, cdf *ModeCDF, dec restorationEntropyDecoder, sb SuperBlockGeometry, state *RestorationState, refs *[3]RestorationUnit) error {
	if hdr == nil || cdf == nil || dec == nil || state == nil || refs == nil {
		return nil
	}
	for plane := range state.Planes {
		planeState := &state.Planes[plane]
		if !planeState.Enabled || planeState.UnitSize <= 0 {
			continue
		}
		ssX, ssY := restorationSubsampling(plane)
		x := sb.StartX >> ssX
		y := sb.StartY >> ssY
		unitX, unitY, ok := planeState.unitCoordsAt(x, y)
		if !ok {
			continue
		}
		idx := planeState.index(unitX, unitY)
		if idx < 0 || idx >= len(planeState.Units) || planeState.Units[idx].Parsed {
			continue
		}
		var preState [4]int
		if dbg, ok := dec.(debugMSACState); ok {
			cur, rng, cnt, pos := dbg.DebugState()
			preState = [4]int{int(cur), int(rng), cnt, pos}
		}
		unit, err := readRestorationUnit(cdf, dec, plane, hdr.Restoration.Type[plane], refs[plane])
		if err != nil {
			return err
		}
		var postState [4]int
		if dbg, ok := dec.(debugMSACState); ok {
			cur, rng, cnt, pos := dbg.DebugState()
			postState = [4]int{int(cur), int(rng), cnt, pos}
		}
		unit.Parsed = true
		planeState.Units[idx] = unit
		debugTraceRestorationUnit(sb, plane, unitX, unitY, unit, preState, postState)
		if unit.Type != restorationTypeNone {
			refs[plane] = unit
		}
	}
	return nil
}

func (p *RestorationPlane) index(unitX, unitY int) int {
	return unitY*p.UnitsX + unitX
}

func (p *RestorationPlane) unitCoordsAt(x, y int) (int, int, bool) {
	if p == nil || !p.Enabled || p.UnitSize <= 0 || x < 0 || y < 0 || x >= p.Width || y >= p.Height {
		return 0, 0, false
	}
	mask := p.UnitSize - 1
	half := p.UnitSize >> 1
	if x&mask != 0 || y&mask != 0 {
		return 0, 0, false
	}
	if x != 0 && x+half > p.Width {
		return 0, 0, false
	}
	if y != 0 && y+half > p.Height {
		return 0, 0, false
	}
	unitX := x >> p.UnitSizeLog2
	unitY := y >> p.UnitSizeLog2
	if unitX >= p.UnitsX || unitY >= p.UnitsY {
		return 0, 0, false
	}
	return unitX, unitY, true
}

func readRestorationUnit(cdf *ModeCDF, dec restorationEntropyDecoder, plane int, frameType uint8, ref RestorationUnit) (RestorationUnit, error) {
	unit := RestorationUnit{}
	unitType := frameType
	switch frameType {
	case restorationTypeSwitchable:
		switch dec.DecodeSymbolAdapt(cdf.RestoreSwitch[:], 2) {
		case 0:
			unitType = restorationTypeNone
		case 1:
			unitType = restorationTypeWiener
		default:
			unitType = restorationTypeSGRProj
		}
	case restorationTypeWiener:
		if dec.DecodeBoolAdapt(cdf.RestoreWiener[:]) == 0 {
			unitType = restorationTypeNone
		}
	case restorationTypeSGRProj:
		if dec.DecodeBoolAdapt(cdf.RestoreSGRProj[:]) == 0 {
			unitType = restorationTypeNone
		}
	}
	unit.Type = unitType
	if unitType == restorationTypeNone {
		return unit, nil
	}
	unit.FilterH = ref.FilterH
	unit.FilterV = ref.FilterV
	unit.SGRWeights = ref.SGRWeights
	switch unitType {
	case restorationTypeWiener:
		if plane > 0 {
			unit.FilterV[0] = 0
		} else {
			unit.FilterV[0] = int8(dec.DecodeSubexp(int(ref.FilterV[0])+5, 16, 1) - 5)
		}
		unit.FilterV[1] = int8(dec.DecodeSubexp(int(ref.FilterV[1])+23, 32, 2) - 23)
		unit.FilterV[2] = int8(dec.DecodeSubexp(int(ref.FilterV[2])+17, 64, 3) - 17)
		if plane > 0 {
			unit.FilterH[0] = 0
		} else {
			unit.FilterH[0] = int8(dec.DecodeSubexp(int(ref.FilterH[0])+5, 16, 1) - 5)
		}
		unit.FilterH[1] = int8(dec.DecodeSubexp(int(ref.FilterH[1])+23, 32, 2) - 23)
		unit.FilterH[2] = int8(dec.DecodeSubexp(int(ref.FilterH[2])+17, 64, 3) - 17)
	case restorationTypeSGRProj:
		idx := int(dec.DecodeBools(4))
		if idx >= len(restoreSGRParams) {
			idx = len(restoreSGRParams) - 1
		}
		unit.Type = restorationTypeSGRProj + uint8(idx)
		params := restoreSGRParams[idx]
		if params[0] != 0 {
			unit.SGRWeights[0] = int8(dec.DecodeSubexp(int(ref.SGRWeights[0])+96, 128, 4) - 96)
		} else {
			unit.SGRWeights[0] = 0
		}
		if params[1] != 0 {
			unit.SGRWeights[1] = int8(dec.DecodeSubexp(int(ref.SGRWeights[1])+32, 128, 4) - 32)
		} else {
			unit.SGRWeights[1] = 95
		}
	}
	return unit, nil
}

func applyRestoration420_8(img *image.YCbCr, hdr *obu.FrameHeader, state *RestorationState) {
	if img == nil || hdr == nil || state == nil {
		return
	}
	srcY := append([]byte(nil), img.Y...)
	srcU := append([]byte(nil), img.Cb...)
	srcV := append([]byte(nil), img.Cr...)
	applyRestorationPlane8(img.Y, srcY, img.YStride, parsedPlaneWidth(img), parsedPlaneHeight(img), &state.Planes[0])
	applyRestorationPlane8(img.Cb, srcU, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), &state.Planes[1])
	applyRestorationPlane8(img.Cr, srcV, img.CStride, parsedChromaPlaneWidth(img), parsedChromaPlaneHeight(img), &state.Planes[2])
}

func applyRestorationPlane8(dst, src []byte, stride, width, height int, plane *RestorationPlane) {
	if plane == nil || !plane.Enabled || len(plane.Units) == 0 || stride <= 0 || width <= 0 || height <= 0 {
		return
	}
	for unitY := 0; unitY < plane.UnitsY; unitY++ {
		for unitX := 0; unitX < plane.UnitsX; unitX++ {
			unit := plane.Units[plane.index(unitX, unitY)]
			if !unit.Parsed || unit.Type == restorationTypeNone {
				continue
			}
			x0 := unitX * plane.UnitSize
			y0 := unitY * plane.UnitSize
			x1 := minInt(width, x0+plane.UnitSize)
			y1 := minInt(height, y0+plane.UnitSize)
			if x1 <= x0 || y1 <= y0 {
				continue
			}
			if unit.Type == restorationTypeWiener {
				applyWienerRestorationUnit8(dst, src, stride, width, height, x0, y0, x1, y1, unit)
				continue
			}
			if unit.Type >= restorationTypeSGRProj {
				sgrIdx := int(unit.Type - restorationTypeSGRProj)
				if sgrIdx < len(restoreSGRParams) {
					applySGRRestorationUnit8(dst, src, stride, width, height, x0, y0, x1, y1, unit, restoreSGRParams[sgrIdx])
				}
			}
		}
	}
}

func applyWienerRestorationUnit8(dst, src []byte, stride, width, height, x0, y0, x1, y1 int, unit RestorationUnit) {
	unitW := x1 - x0
	unitH := y1 - y0
	if unitW <= 0 || unitH <= 0 {
		return
	}
	tmp := make([]int, unitW*unitH)
	fh := restorationWienerHorizontal(unit)
	fv := restorationWienerVertical(unit)
	for y := 0; y < unitH; y++ {
		for x := 0; x < unitW; x++ {
			px := x0 + x
			py := y0 + y
			sum := 1 << 14
			sum += int(sampleRestorationPlane8(src, stride, width, height, px, py)) * 128
			for k := -3; k <= 3; k++ {
				sum += int(sampleRestorationPlane8(src, stride, width, height, px+k, py)) * fh[k+3]
			}
			v := (sum + 4) >> 3
			if v < 0 {
				v = 0
			} else if v > 8191 {
				v = 8191
			}
			tmp[y*unitW+x] = v
		}
	}
	for y := 0; y < unitH; y++ {
		row := dst[(y0+y)*stride:]
		for x := 0; x < unitW; x++ {
			sum := -(1 << 18)
			for k := -3; k <= 3; k++ {
				yy := minInt(unitH-1, maxIntLocal(0, y+k))
				sum += tmp[yy*unitW+x] * fv[k+3]
			}
			row[x0+x] = clipByte((sum + (1 << 10)) >> 11)
		}
	}
}

func restorationWienerHorizontal(unit RestorationUnit) [7]int {
	a := int(unit.FilterH[0])
	b := int(unit.FilterH[1])
	c := int(unit.FilterH[2])
	return [7]int{a, b, c, -2 * (a + b + c), c, b, a}
}

func restorationWienerVertical(unit RestorationUnit) [7]int {
	a := int(unit.FilterV[0])
	b := int(unit.FilterV[1])
	c := int(unit.FilterV[2])
	return [7]int{a, b, c, 128 - 2*(a+b+c), c, b, a}
}

func applySGRRestorationUnit8(dst, src []byte, stride, width, height, x0, y0, x1, y1 int, unit RestorationUnit, params [2]uint16) {
	w0 := int(unit.SGRWeights[0])
	w1 := 128 - (int(unit.SGRWeights[0]) + int(unit.SGRWeights[1]))
	for py := y0; py < y1; py++ {
		row := dst[py*stride:]
		for px := x0; px < x1; px++ {
			srcV := int(src[py*stride+px])
			delta := 0
			if params[0] != 0 {
				filt5 := sgrFilteredPixel8(src, stride, width, height, px, py, 2, int(params[0]))
				delta += w0 * (filt5 - srcV)
			}
			if params[1] != 0 {
				filt3 := sgrFilteredPixel8(src, stride, width, height, px, py, 1, int(params[1]))
				delta += w1 * (filt3 - srcV)
			}
			row[px] = clipByte(srcV + ((delta + 64) >> 7))
		}
	}
}

func sgrFilteredPixel8(src []byte, stride, width, height, x, y, radius, strength int) int {
	n := 2*radius + 1
	count := n * n
	sum := 0
	sumSq := 0
	for yy := -radius; yy <= radius; yy++ {
		sy := minInt(height-1, maxIntLocal(0, y+yy))
		row := src[sy*stride:]
		for xx := -radius; xx <= radius; xx++ {
			sx := minInt(width-1, maxIntLocal(0, x+xx))
			v := int(row[sx])
			sum += v
			sumSq += v * v
		}
	}
	p := count*sumSq - sum*sum
	if p < 0 {
		p = 0
	}
	z := (p*strength + (1 << 19)) >> 20
	if z > 255 {
		z = 255
	}
	xByX := 255
	if z > 0 {
		xByX = 256 / z
		if xByX > 255 {
			xByX = 255
		}
	}
	srcV := int(src[y*stride+x])
	mean := (sum + count/2) / count
	return srcV + ((xByX*(mean-srcV) + 128) >> 8)
}

func sampleRestorationPlane8(src []byte, stride, width, height, x, y int) uint8 {
	if x < 0 {
		x = 0
	} else if x >= width {
		x = width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= height {
		y = height - 1
	}
	return src[y*stride+x]
}
