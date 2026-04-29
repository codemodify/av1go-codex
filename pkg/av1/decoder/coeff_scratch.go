package decoder

import "sync"

var coeffScratchPool sync.Pool

func coeffBufferLen(tx TxfmSize) int {
	info := TxfmInfoFor(tx)
	return int(info.W4) * 4 * int(info.H4) * 4
}

func takeCoeffScratch(tx TxfmSize) *scratchBuffer[int32] {
	return takeScratch[int32](&coeffScratchPool, coeffBufferLen(tx))
}

func takeZeroCoeffScratch(tx TxfmSize) *scratchBuffer[int32] {
	coeffs := takeCoeffScratch(tx)
	clear(coeffs.buf)
	return coeffs
}

func putCoeffScratch(coeffs *scratchBuffer[int32]) {
	putZeroScratch(&coeffScratchPool, coeffs)
}
