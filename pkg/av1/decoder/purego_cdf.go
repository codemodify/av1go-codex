package decoder

import (
	"reflect"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func cloneFrameCDFState(src *FrameCDFState) *FrameCDFState {
	if src == nil {
		return nil
	}
	dst := &FrameCDFState{}
	if src.Mode != nil {
		dst.Mode = src.Mode.Clone()
	}
	if src.MV != nil {
		dst.MV = src.MV.Clone()
	}
	for i, coef := range src.CoefByQ {
		if coef != nil {
			dst.CoefByQ[i] = coef.Clone()
		}
	}
	return dst
}

func cdfStateFromTileState(tile *TileState) *FrameCDFState {
	if tile == nil {
		return nil
	}
	state := &FrameCDFState{}
	if tile.CDF != nil {
		state.Mode = tile.CDF.Clone()
	}
	if tile.MV != nil {
		state.MV = tile.MV.Clone()
	}
	for i, coef := range tile.CoefByQ {
		if coef != nil {
			state.CoefByQ[i] = coef.Clone()
		}
	}
	if state.Mode == nil && state.MV == nil {
		emptyCoef := true
		for _, coef := range state.CoefByQ {
			if coef != nil {
				emptyCoef = false
				break
			}
		}
		if emptyCoef {
			return nil
		}
	}
	resetFrameCDFStateSymbolCounts(state)
	return state
}

func resetFrameCDFStateSymbolCounts(state *FrameCDFState) {
	if state == nil {
		return
	}
	resetCDFSymbolCounts(state.Mode)
	resetModeCDFVariableSymbolCounts(state.Mode)
	resetCDFSymbolCounts(state.MV)
	for _, coef := range state.CoefByQ {
		resetCDFSymbolCounts(coef)
	}
}

func resetModeCDFVariableSymbolCounts(cdf *ModeCDF) {
	if cdf == nil {
		return
	}

	for level := range cdf.Partition {
		countIdx := int(PartitionTypeCount(BlockLevel(level)))
		for ctx := range cdf.Partition[level] {
			cdf.Partition[level][ctx][countIdx] = 0
		}
	}

	for cfl := range cdf.UVMode {
		countIdx := int(numIntraPredModes - 1)
		if cfl != 0 {
			countIdx++
		}
		for yMode := range cdf.UVMode[cfl] {
			cdf.UVMode[cfl][yMode][countIdx] = 0
		}
	}

	for plane := range cdf.PalSize {
		for sizeCtx := range cdf.PalSize[plane] {
			cdf.PalSize[plane][sizeCtx][6] = 0
		}
	}

	for plane := range cdf.ColorMap {
		for palSizeMinus2 := range cdf.ColorMap[plane] {
			countIdx := palSizeMinus2 + 1
			for ctx := range cdf.ColorMap[plane][palSizeMinus2] {
				cdf.ColorMap[plane][palSizeMinus2][ctx][countIdx] = 0
			}
		}
	}

	for maxIdx := range cdf.TXSize {
		countIdx := minInt(maxIdx+1, 2)
		for txCtx := range cdf.TXSize[maxIdx] {
			cdf.TXSize[maxIdx][txCtx][countIdx] = 0
		}
	}

	cdf.RestoreSwitch[2] = 0
}

func resetCDFSymbolCounts(cdf any) {
	v := reflect.ValueOf(cdf)
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	resetCDFSymbolCountsValue(v.Elem())
}

func resetCDFSymbolCountsValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			resetCDFSymbolCountsValue(v.Field(i))
		}
	case reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint16 {
			if v.Len() > 0 && v.Index(v.Len()-1).CanSet() {
				v.Index(v.Len() - 1).SetUint(0)
			}
			return
		}
		for i := 0; i < v.Len(); i++ {
			resetCDFSymbolCountsValue(v.Index(i))
		}
	case reflect.Ptr:
		if !v.IsNil() {
			resetCDFSymbolCountsValue(v.Elem())
		}
	}
}

func captureCDFStateFromTiles(states []TileState) *FrameCDFState {
	if len(states) == 0 {
		return nil
	}
	best := 0
	bestSize := len(states[0].Data)
	for i := 1; i < len(states); i++ {
		if size := len(states[i].Data); size > bestSize {
			best = i
			bestSize = size
		}
	}
	return cdfStateFromTileState(&states[best])
}

func (d *Decoder) initialPureGoCDFState(hdr *obu.FrameHeader) *FrameCDFState {
	if d == nil || hdr == nil || hdr.PrimaryRefFrame >= uint8(len(hdr.RefIdx)) {
		return nil
	}
	slot := hdr.RefIdx[hdr.PrimaryRefFrame]
	if slot < 0 || int(slot) >= len(d.pureGoCDFRefs) {
		return nil
	}
	return d.pureGoCDFRefs[slot]
}

func (d *Decoder) buildPureGoTileStates(parsed *ParsedFrame) ([]TileState, error) {
	return BuildTileStatesWithInitialCDFState(
		d.header,
		&parsed.Header,
		&parsed.TileGroup,
		parsed.InitialCoefQCat,
		d.initialPureGoCDFState(&parsed.Header),
	)
}

func (d *Decoder) capturePureGoCDFState(states []TileState) {
	if d == nil {
		return
	}
	d.currentCDFState = captureCDFStateFromTiles(states)
}

func (d *Decoder) snapshotPureGoCDFState(refreshFlags uint8, hdr *obu.FrameHeader) {
	if d == nil || hdr == nil {
		return
	}
	source := d.initialPureGoCDFState(hdr)
	if hdr.RefreshContext {
		source = d.currentCDFState
	}
	for i := range d.pureGoCDFRefs {
		if refreshFlags&(1<<i) == 0 {
			continue
		}
		if source == nil {
			d.pureGoCDFRefs[i] = nil
			continue
		}
		d.pureGoCDFRefs[i] = source.Clone()
	}
}

func (d *Decoder) releasePureGoCDFState() {
	if d == nil {
		return
	}
	d.currentCDFState = nil
	for i := range d.pureGoCDFRefs {
		d.pureGoCDFRefs[i] = nil
	}
}
