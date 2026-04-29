package decoder

type CoefCDF struct {
	QCat       int
	Skip       [5][13][2]uint16
	EOBBin16   [2][2][5]uint16
	EOBBin32   [2][2][6]uint16
	EOBBin64   [2][2][7]uint16
	EOBBin128  [2][2][8]uint16
	EOBBin256  [2][2][9]uint16
	EOBBin512  [2][10]uint16
	EOBBin1024 [2][11]uint16
	EOBHiBit   [5][2][9][2]uint16
	EOBBaseTok [5][2][4][3]uint16
	BaseTok    [5][2][41][4]uint16
	DCSign     [2][3][2]uint16
	BRTok      [4][2][21][4]uint16
}

func clampDefaultCoefQCat(qcat int) int {
	if qcat < 0 {
		return 0
	}
	if qcat >= len(generatedCoefSkipCDF) {
		return len(generatedCoefSkipCDF) - 1
	}
	return qcat
}

func defaultCoefCDFValueForQCat(qcat int) CoefCDF {
	qcat = clampDefaultCoefQCat(qcat)
	return CoefCDF{
		QCat:       qcat,
		Skip:       generatedCoefSkipCDF[qcat],
		EOBBin16:   generatedEOBBin16CDF[qcat],
		EOBBin32:   generatedEOBBin32CDF[qcat],
		EOBBin64:   generatedEOBBin64CDF[qcat],
		EOBBin128:  generatedEOBBin128CDF[qcat],
		EOBBin256:  generatedEOBBin256CDF[qcat],
		EOBBin512:  generatedEOBBin512CDF[qcat],
		EOBBin1024: generatedEOBBin1024CDF[qcat],
		EOBHiBit:   generatedEOBHiBitCDF[qcat],
		EOBBaseTok: generatedEOBBaseTokCDF[qcat],
		BaseTok:    generatedBaseTokCDF[qcat],
		DCSign:     generatedDCSignCDF[qcat],
		BRTok:      generatedBRTokCDF[qcat],
	}
}

func DefaultCoefQCat(qidx uint8) int {
	return boolToInt(qidx > 20) + boolToInt(qidx > 60) + boolToInt(qidx > 120)
}

func NewDefaultCoefCDFForQCat(qcat int) *CoefCDF {
	cdf := defaultCoefCDFValueForQCat(qcat)
	return &cdf
}

func NewDefaultCoefCDF(qidx uint8) *CoefCDF {
	return NewDefaultCoefCDFForQCat(DefaultCoefQCat(qidx))
}

func (c *CoefCDF) ResetDefaultForQCat(qcat int) {
	if c == nil {
		return
	}
	*c = defaultCoefCDFValueForQCat(qcat)
}

func (c *CoefCDF) Clone() *CoefCDF {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}
