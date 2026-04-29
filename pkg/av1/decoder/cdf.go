package decoder

type ModeCDF struct {
	Partition       [numBlockLevels][4][10]uint16
	Skip            [3][2]uint16
	SkipMode        [3][2]uint16
	Intra           [4][2]uint16
	Comp            [5][2]uint16
	CompDir         [5][2]uint16
	JntComp         [6][2]uint16
	MaskComp        [6][2]uint16
	WedgeComp       [9][2]uint16
	WedgeIdx        [9][16]uint16
	CompInterMode   [8][8]uint16
	CompFwdRef      [3][3][2]uint16
	CompBwdRef      [2][3][2]uint16
	CompUniRef      [3][3][2]uint16
	InterIntra      [4][2]uint16
	InterIntraMode  [4][4]uint16
	InterIntraWedge [7][2]uint16
	SegPred         [3][2]uint16
	SegID           [3][8]uint16
	RestoreWiener   [2]uint16
	RestoreSGRProj  [2]uint16
	RestoreSwitch   [4]uint16
	DeltaQ          [4]uint16
	DeltaLF         [5][4]uint16
	Filter          [2][8][3]uint16
	MotionMode      [numBlockSizes][3]uint16
	OBMC            [numBlockSizes][2]uint16
	NewMVMode       [6][2]uint16
	GlobalMVMode    [2][2]uint16
	RefMVMode       [6][2]uint16
	DRLBit          [3][2]uint16
	Ref             [6][3][2]uint16
	IntraBC         [2]uint16
	YMode           [4][13]uint16
	KFYMode         [5][5][13]uint16
	UseFilterIntra  [numBlockSizes][2]uint16
	FilterIntra     [5]uint16
	PalY            [7][3][2]uint16
	PalSize         [2][7][8]uint16
	PalUV           [2][2]uint16
	ColorMap        [2][7][5][8]uint16
	UVMode          [2][13][14]uint16
	AngleDelta      [8][7]uint16
	CFLSign         [8]uint16
	CFLAlpha        [6][16]uint16
	TXSize          [4][3][4]uint16
	TXPart          [7][3][2]uint16
	TXTypeInter1    [2][16]uint16
	TXTypeInter2    [12]uint16
	TXTypeInter3    [4][2]uint16
	TXTypeIntra1    [2][13][7]uint16
	TXTypeIntra2    [3][13][5]uint16
}

var defaultModeCDFValue = ModeCDF{
	Partition:       generatedPartitionCDF,
	Skip:            generatedSkipCDF,
	SkipMode:        generatedSkipModeCDF,
	Intra:           generatedIntraCDF,
	Comp:            generatedCompCDF,
	CompDir:         generatedCompDirCDF,
	JntComp:         generatedJntCompCDF,
	MaskComp:        generatedMaskCompCDF,
	WedgeComp:       generatedWedgeCompCDF,
	WedgeIdx:        generatedWedgeIdxCDF,
	CompInterMode:   generatedCompInterModeCDF,
	CompFwdRef:      generatedCompFwdRefCDF,
	CompBwdRef:      generatedCompBwdRefCDF,
	CompUniRef:      generatedCompUniRefCDF,
	InterIntra:      generatedInterIntraCDF,
	InterIntraMode:  generatedInterIntraModeCDF,
	InterIntraWedge: generatedInterIntraWedgeCDF,
	SegPred:         generatedSegPredCDF,
	SegID:           generatedSegIDCDF,
	RestoreWiener:   generatedRestoreWienerCDF,
	RestoreSGRProj:  generatedRestoreSGRProjCDF,
	RestoreSwitch:   generatedRestoreSwitchableCDF,
	DeltaQ:          generatedDeltaQCDF,
	DeltaLF:         generatedDeltaLFCDF,
	Filter:          generatedInterFilterCDF,
	MotionMode:      generatedMotionModeCDF,
	OBMC:            generatedOBMCCDF,
	NewMVMode:       generatedNewMVModeCDF,
	GlobalMVMode:    generatedGlobalMVModeCDF,
	RefMVMode:       generatedRefMVModeCDF,
	DRLBit:          generatedDRLBitCDF,
	Ref:             generatedRefCDF,
	IntraBC:         generatedIntraBCCDF,
	YMode:           generatedYModeCDF,
	KFYMode:         generatedKFYModeCDF,
	UseFilterIntra:  generatedUseFilterIntraCDF,
	FilterIntra:     generatedFilterIntraCDF,
	PalY:            generatedPalYCDF,
	PalSize:         generatedPalSizeCDF,
	PalUV:           generatedPalUVCDF,
	ColorMap:        generatedColorMapCDF,
	UVMode:          generatedUVModeCDF,
	AngleDelta:      generatedAngleDeltaCDF,
	CFLSign:         generatedCFLSignCDF,
	CFLAlpha:        generatedCFLAlphaCDF,
	TXSize:          generatedTXSizeCDF,
	TXPart:          generatedTXPartCDF,
	TXTypeInter1:    generatedTXTypeInter1CDF,
	TXTypeInter2:    generatedTXTypeInter2CDF,
	TXTypeInter3:    generatedTXTypeInter3CDF,
	TXTypeIntra1:    generatedTXTypeIntra1CDF,
	TXTypeIntra2:    generatedTXTypeIntra2CDF,
}

func NewDefaultModeCDF() *ModeCDF {
	cdf := defaultModeCDFValue
	return &cdf
}

func (c *ModeCDF) ResetDefault() {
	if c == nil {
		return
	}
	*c = defaultModeCDFValue
}

func (c *ModeCDF) Clone() *ModeCDF {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}

func (c *ModeCDF) PartitionCDF(level BlockLevel, ctx int) []uint16 {
	n := int(PartitionTypeCount(level))
	return c.Partition[level][ctx][:n+1]
}
