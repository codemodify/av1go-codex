package decoder

type MVComponentCDF struct {
	Classes  [11]uint16
	Sign     [2]uint16
	Class0   [2]uint16
	Class0FP [2][4]uint16
	Class0HP [2]uint16
	ClassN   [10][2]uint16
	ClassNFP [4]uint16
	ClassNHP [2]uint16
}

type MVCDF struct {
	Comp  [2]MVComponentCDF
	Joint [4]uint16
}

var defaultMVComponentCDF = MVComponentCDF{
	Classes:  generatedMVClassesCDF,
	Sign:     generatedMVSignCDF,
	Class0:   generatedMVClass0CDF,
	Class0FP: generatedMVClass0FPCDF,
	Class0HP: generatedMVClass0HPCDF,
	ClassN:   generatedMVClassNCDF,
	ClassNFP: generatedMVClassNFPCDF,
	ClassNHP: generatedMVClassNHPCDF,
}

var defaultMVCDFValue = MVCDF{
	Comp:  [2]MVComponentCDF{defaultMVComponentCDF, defaultMVComponentCDF},
	Joint: generatedMVJointCDF,
}

func NewDefaultMVCDF() *MVCDF {
	cdf := defaultMVCDFValue
	return &cdf
}

func (c *MVCDF) ResetDefault() {
	if c == nil {
		return
	}
	*c = defaultMVCDFValue
}

func (c *MVCDF) Clone() *MVCDF {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}
