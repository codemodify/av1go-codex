package decoder

import "sync"

var transformUnitScratchPool sync.Pool
var blockGeometryScratchPool sync.Pool

func maxTransformUnitCount(g BlockGeometry) int {
	return maxIntLocal(0, g.Width4()) * maxIntLocal(0, g.Height4())
}

func takeTransformUnitScratch(g BlockGeometry) *scratchBuffer[TransformUnit] {
	return takeScratch[TransformUnit](&transformUnitScratchPool, maxTransformUnitCount(g))
}

func putTransformUnitScratch(units *scratchBuffer[TransformUnit]) {
	putScratch(&transformUnitScratchPool, units)
}

func takeBlockGeometryScratch(n int) *scratchBuffer[BlockGeometry] {
	return takeScratch[BlockGeometry](&blockGeometryScratchPool, maxIntLocal(0, n))
}

func putBlockGeometryScratch(geoms *scratchBuffer[BlockGeometry]) {
	putScratch(&blockGeometryScratchPool, geoms)
}

func takeBlockGeometryBuffer(dst []BlockGeometry, n int) ([]BlockGeometry, *scratchBuffer[BlockGeometry]) {
	n = maxIntLocal(0, n)
	if cap(dst) >= n {
		return dst[:0], nil
	}
	scratch := takeBlockGeometryScratch(n)
	return scratch.buf[:0], scratch
}

func putBlockGeometryBuffer(scratch *scratchBuffer[BlockGeometry]) {
	putBlockGeometryScratch(scratch)
}
