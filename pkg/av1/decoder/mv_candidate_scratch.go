package decoder

import "sync"

const (
	refMVCandidateScratchCap      = 32
	compoundMVCandidateScratchCap = 24
)

var (
	refMVCandidateScratchPool      sync.Pool
	compoundMVCandidateScratchPool sync.Pool
)

func takeRefMVCandidatePairScratch() (*scratchBuffer[RefMVCandidate], []RefMVCandidate, []RefMVCandidate) {
	scratch := takeScratch[RefMVCandidate](&refMVCandidateScratchPool, 2*refMVCandidateScratchCap)
	buf := scratch.buf
	return scratch,
		buf[:0:refMVCandidateScratchCap],
		buf[refMVCandidateScratchCap : refMVCandidateScratchCap : 2*refMVCandidateScratchCap]
}

func takeRefMVCandidateTripletScratch() (*scratchBuffer[RefMVCandidate], []RefMVCandidate, []RefMVCandidate, []RefMVCandidate) {
	scratch := takeScratch[RefMVCandidate](&refMVCandidateScratchPool, 3*refMVCandidateScratchCap)
	buf := scratch.buf
	return scratch,
		buf[:0:refMVCandidateScratchCap],
		buf[refMVCandidateScratchCap : refMVCandidateScratchCap : 2*refMVCandidateScratchCap],
		buf[2*refMVCandidateScratchCap : 2*refMVCandidateScratchCap : 3*refMVCandidateScratchCap]
}

func putRefMVCandidateScratch(scratch *scratchBuffer[RefMVCandidate]) {
	putScratch(&refMVCandidateScratchPool, scratch)
}

func takeCompoundMVCandidatePairScratch() (*scratchBuffer[CompoundRefMVCandidate], []CompoundRefMVCandidate, []CompoundRefMVCandidate) {
	scratch := takeScratch[CompoundRefMVCandidate](&compoundMVCandidateScratchPool, 2*compoundMVCandidateScratchCap)
	buf := scratch.buf
	return scratch,
		buf[:0:compoundMVCandidateScratchCap],
		buf[compoundMVCandidateScratchCap : compoundMVCandidateScratchCap : 2*compoundMVCandidateScratchCap]
}

func putCompoundMVCandidateScratch(scratch *scratchBuffer[CompoundRefMVCandidate]) {
	putScratch(&compoundMVCandidateScratchPool, scratch)
}
