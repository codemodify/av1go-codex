package decoder

var (
	cdefPlaneScratchCache   = stickyScratchCache[byte]{maxStored: 8}
	cdefPlaneScratch16Cache = stickyScratchCache[uint16]{maxStored: 8}
)

func takeCDEFScratch8(n int) *scratchBuffer[byte] {
	return cdefPlaneScratchCache.take(n)
}

func putCDEFScratch8(buf *scratchBuffer[byte]) {
	cdefPlaneScratchCache.put(buf)
}

func takeCDEFScratch16(n int) *scratchBuffer[uint16] {
	return cdefPlaneScratch16Cache.take(n)
}

func putCDEFScratch16(buf *scratchBuffer[uint16]) {
	cdefPlaneScratch16Cache.put(buf)
}
