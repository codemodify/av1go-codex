//go:build !av1debug

package decoder

func debugTraceInterLeaf(_ *ParsedFrame, _ BlockGeometry, _ InterBlockSyntax) {}

func debugTraceInterTransformUnits(_ *ParsedFrame, _ BlockGeometry, _ InterBlockSyntax, _ []TransformUnit) {
}

func debugTraceInterTXSplit(_, _ BlockGeometry, _ TxfmSize, _, _, _ int, _, _ int8, _ uint32, _, _ [4]int, _, _ [2]uint16) {
}

func debugTraceInterPartition(_ *ParsedFrame, _, _ BlockGeometry, _ BlockPartition, _ *BlockContext, _ *BlockContext, _, _ [4]int, _, _ [10]uint16) {
}

type debugMSACState interface {
	DebugState() (uint32, uint32, int, int)
}

func debugTraceInterMSAC(_ *ParsedFrame, _ BlockGeometry, _ string, _ debugMSACState) {
}

func debugTraceInterSkipPrelude(_ *ParsedFrame, _, _ BlockGeometry, _, _ bool, _ uint8, _ bool, _, _ int, _, _, _, _ uint8, _ InterBlockSyntax, _, _ [4]int, _, _, _, _ [2]uint16) {
}

func debugTraceInterIntraFlag(_ *ParsedFrame, _, _ BlockGeometry, _, _ bool, _ uint8, _ bool, _ int, _ int8, _ bool, _ bool, _, _ [4]int, _, _ [2]uint16) {
}

func debugTraceInterResidualStep(_ *ParsedFrame, _ BlockGeometry, _ int, _ BlockGeometry, _ string, _ debugMSACState) {
}

func debugTraceInterEntropyExhausted(_ *ParsedFrame, _ BlockGeometry, _ string, _ debugMSACState, _ int) {
}

func debugTraceInterMVCandidates(_ *ParsedFrame, _ BlockGeometry, _ int8, _ int, _ []RefMVCandidate, _ []RefMVCandidate) {
}

func debugTraceSingleRefStack(_ *ParsedFrame, _ BlockGeometry, _ string, _ int8, _ int, _ []RefMVCandidate) {
}

func debugTraceInterMVNeighbors(_ *ParsedFrame, _ BlockGeometry, _ *SpatialMVTile) {
}

func debugTraceInterModeInputs(_ *ParsedFrame, _ BlockGeometry, _, _ int, _ []int, _ *ModeCDF) {
}

func debugTraceCompoundModeInputs(_ *ParsedFrame, _ BlockGeometry, _, _ int, _ bool, _, _ []RefMVCandidate, _ []CompoundRefMVCandidate, _ []int, _ *ModeCDF) {
}

func debugTraceCompoundRefStack(_ *ParsedFrame, _ BlockGeometry, _ string, _ []CompoundRefMVCandidate, _, _, _, _, _, _ int) {
}

func debugTraceInterWarp(_ *ParsedFrame, _ BlockGeometry, _ affineWarp) {
}

func debugTraceRestorationUnit(_ SuperBlockGeometry, _, _ int, _ int, _ RestorationUnit, _, _ [4]int) {
}

func debugTraceInterLumaCoeffSkip(_ *ParsedFrame, _ BlockGeometry, _, _ int, _ uint32, _, _ [4]int, _, _ []uint16) {
}

func debugTraceInterLumaCoeffDecoded(_ *ParsedFrame, _ BlockGeometry, _ TxType, _ uint8, _ []int32, _ debugMSACState) {
}

func debugTraceInterLumaSamples(_ *ParsedFrame, _ BlockGeometry, _ string, _ []byte, _, _, _ int) {
}

func debugTraceCompoundLumaPrediction(_ *ParsedFrame, _ BlockGeometry, _ *Frame, _ MotionVector, _ InterFilterPair, _ bool, _ affineWarp, _ *Frame, _ MotionVector, _ InterFilterPair, _ bool, _ affineWarp, _ int) {
}

func debugTraceCompoundLumaPrediction16(_ *ParsedFrame, _ BlockGeometry, _ []uint16, _, _, _ int, _ MotionVector, _ InterFilterPair, _ bool, _ affineWarp, _ []uint16, _, _, _ int, _ MotionVector, _ InterFilterPair, _ bool, _ affineWarp, _ uint8, _ int) {
}

func debugTraceInterFrameLumaSample(_ *ParsedFrame, _ string, _ []byte, _, _, _ int) {
}

func debugTraceInterFrameLumaSample16(_ *ParsedFrame, _ string, _ []uint16, _, _, _ int) {
}

func debugTraceSingleRefPrediction16(_ *ParsedFrame, _ BlockGeometry, _ int8, _ uint8, _ *Frame, _ []uint16, _, _, _ int, _ MotionVector, _ InterFilterPair, _ []uint16, _, _, _ int) {
}

func debugTraceOBMCSample16(_ *ParsedFrame, _ string, _ BlockGeometry, _ *SpatialMVBlock, _, _, _, _ int, _ []uint16, _ int, _ []uint16, _, _, _ int) {
}

func debugTraceInterFrameChromaSample(_ *ParsedFrame, _, _ string, _ []byte, _, _, _ int) {
}

func debugTraceInterFrameChromaSample16(_ *ParsedFrame, _, _ string, _ []uint16, _, _, _ int) {
}

func debugTraceInterChromaSamples(_ *ParsedFrame, _ BlockGeometry, _, _ string, _ []byte, _, _, _ int) {
}

func debugTraceTemporalProjection(_ *ParsedFrame, _, _, _, _, _, _, _ int, _ TemporalMVBlock) {
}

func debugTraceTemporalSave(_ *ParsedFrame, _, _ int, _ *SpatialMVBlock, _ TemporalMVBlock) {
}

func debugTraceTemporalCompoundCandidate(_ *ParsedFrame, _ BlockGeometry, _, _ int, _ TemporalMVBlock, _, _ int, _, _ MotionVector, _ bool) {
}

func debugTraceSingleRefBit(_ *ParsedFrame, _ BlockGeometry, _ string, _ int, _ []uint16, _ int, _ intraEntropyDecoder) {
}

func debugTraceSingleRefModeBit(_ *ParsedFrame, _ BlockGeometry, _ string, _ int, _ [2]uint16, _ []uint16, _ int, _ intraEntropyDecoder) {
}
