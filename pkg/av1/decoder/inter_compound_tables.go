package decoder

type CompoundType uint8

const (
	compoundTypeNone CompoundType = iota
	compoundTypeWeightedAvg
	compoundTypeAvg
	compoundTypeSeg
	compoundTypeWedge
)

type InterIntraType uint8

const (
	interIntraNone InterIntraType = iota
	interIntraBlend
	interIntraWedge
)

type CompoundInterPredMode uint8

const (
	compoundNearestNearest CompoundInterPredMode = iota
	compoundNearNear
	compoundNearestNew
	compoundNewNearest
	compoundNearNew
	compoundNewNear
	compoundGlobalGlobal
	compoundNewNew
)

var generatedCompInterPredModes = [8][2]InterPredMode{
	compoundNearestNearest: {InterPredNearest, InterPredNearest},
	compoundNearNear:       {InterPredNear, InterPredNear},
	compoundNearestNew:     {InterPredNearest, InterPredNew},
	compoundNewNearest:     {InterPredNew, InterPredNearest},
	compoundNearNew:        {InterPredNear, InterPredNew},
	compoundNewNear:        {InterPredNew, InterPredNear},
	compoundGlobalGlobal:   {InterPredGlobal, InterPredGlobal},
	compoundNewNew:         {InterPredNew, InterPredNew},
}

var generatedWedgeCtxLUT = [numBlockSizes]uint8{
	Block32x32: 6,
	Block32x16: 5,
	Block32x8:  8,
	Block16x32: 4,
	Block16x16: 3,
	Block16x8:  2,
	Block8x32:  7,
	Block8x16:  1,
	Block8x8:   0,
}

const wedgeAllowedMask = (1 << Block32x32) |
	(1 << Block32x16) |
	(1 << Block32x8) |
	(1 << Block16x32) |
	(1 << Block16x16) |
	(1 << Block16x8) |
	(1 << Block8x32) |
	(1 << Block8x16) |
	(1 << Block8x8)

const interIntraAllowedMask = (1 << Block32x32) |
	(1 << Block32x16) |
	(1 << Block16x32) |
	(1 << Block16x16) |
	(1 << Block16x8) |
	(1 << Block8x16) |
	(1 << Block8x8)

var generatedInterIntraModes = [4]IntraPredMode{
	IntraPredDC,
	IntraPredVertical,
	IntraPredHorizontal,
	IntraPredSmooth,
}

func wedgeAllowedForBlock(bs BlockSize) bool {
	return bs < numBlockSizes && (wedgeAllowedMask&(1<<bs)) != 0
}

func interIntraAllowedForBlock(bs BlockSize) bool {
	return bs < numBlockSizes && (interIntraAllowedMask&(1<<bs)) != 0
}

func wedgeContextForBlock(bs BlockSize) int {
	if bs >= numBlockSizes {
		return 0
	}
	return int(generatedWedgeCtxLUT[bs])
}

func interIntraSizeGroup(bs BlockSize) int {
	if int(bs) >= len(generatedYModeSizeContext) {
		return 0
	}
	return int(generatedYModeSizeContext[bs])
}
