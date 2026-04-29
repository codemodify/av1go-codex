package decoder

import "sync"

var (
	tileModeCDFPool            sync.Pool
	tileMVCDFPool              sync.Pool
	tileCoefCDFPools           [4]sync.Pool
	tileSuperBlockGeometryPool sync.Pool
)

func takeDefaultModeCDF() *ModeCDF {
	if v := tileModeCDFPool.Get(); v != nil {
		cdf := v.(*ModeCDF)
		cdf.ResetDefault()
		return cdf
	}
	cdf := &ModeCDF{}
	cdf.ResetDefault()
	return cdf
}

func putModeCDF(cdf *ModeCDF) {
	if cdf == nil {
		return
	}
	tileModeCDFPool.Put(cdf)
}

func takeDefaultMVCDF() *MVCDF {
	if v := tileMVCDFPool.Get(); v != nil {
		cdf := v.(*MVCDF)
		cdf.ResetDefault()
		return cdf
	}
	cdf := &MVCDF{}
	cdf.ResetDefault()
	return cdf
}

func putMVCDF(cdf *MVCDF) {
	if cdf == nil {
		return
	}
	tileMVCDFPool.Put(cdf)
}

func takeDefaultCoefCDFForQCat(qcat int) *CoefCDF {
	qcat = clampDefaultCoefQCat(qcat)
	if v := tileCoefCDFPools[qcat].Get(); v != nil {
		cdf := v.(*CoefCDF)
		cdf.ResetDefaultForQCat(qcat)
		return cdf
	}
	cdf := &CoefCDF{}
	cdf.ResetDefaultForQCat(qcat)
	return cdf
}

func putCoefCDFForQCat(qcat int, cdf *CoefCDF) {
	if cdf == nil {
		return
	}
	qcat = clampDefaultCoefQCat(qcat)
	tileCoefCDFPools[qcat].Put(cdf)
}

func CloseTileStates(states []TileState) {
	for i := range states {
		closeTileState(&states[i])
	}
}

func closeTileState(state *TileState) {
	if state == nil {
		return
	}
	putModeCDF(state.CDF)
	state.CDF = nil
	putMVCDF(state.MV)
	state.MV = nil
	for qcat, cdf := range state.CoefByQ {
		putCoefCDFForQCat(qcat, cdf)
		state.CoefByQ[qcat] = nil
	}
	state.Coef = nil
	if state.superBlocksScratch != nil {
		putScratch(&tileSuperBlockGeometryPool, state.superBlocksScratch)
		state.superBlocksScratch = nil
	}
	state.Geometry.SuperBlocks = nil
	state.Data = nil
	state.Entropy = nil
}
