package decoder

import (
	"sync"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

type loopFilterPlaneEdges struct {
	Width4     int
	Height4    int
	VStride    int
	HStride    int
	Vertical   []bool
	Horizontal []bool
	vertical   *scratchBuffer[bool]
	horizontal *scratchBuffer[bool]
}

type loopFilterEdgeMaps struct {
	Y *loopFilterPlaneEdges
	U *loopFilterPlaneEdges
	V *loopFilterPlaneEdges
}

var loopFilterBoolScratchPool sync.Pool

func newLoopFilterPlaneEdges(width, height int) *loopFilterPlaneEdges {
	width4 := (width + 3) >> 2
	height4 := (height + 3) >> 2
	vertical := takeScratch[bool](&loopFilterBoolScratchPool, (width4+1)*height4)
	horizontal := takeScratch[bool](&loopFilterBoolScratchPool, width4*(height4+1))
	return &loopFilterPlaneEdges{
		Width4:     width4,
		Height4:    height4,
		VStride:    width4 + 1,
		HStride:    width4,
		Vertical:   vertical.buf[:(width4+1)*height4],
		Horizontal: horizontal.buf[:width4*(height4+1)],
		vertical:   vertical,
		horizontal: horizontal,
	}
}

func newLoopFilterEdgeMaps(width, height int, layout av1.ChromaSubsampling) *loopFilterEdgeMaps {
	y := newLoopFilterPlaneEdges(width, height)
	cw, ch := planeChromaDims(layout, width, height)
	uv := newLoopFilterPlaneEdges(cw, ch)
	return &loopFilterEdgeMaps{
		Y: y,
		U: uv,
		V: uv,
	}
}

func putLoopFilterPlaneEdges(edges *loopFilterPlaneEdges) {
	if edges == nil {
		return
	}
	putZeroScratch(&loopFilterBoolScratchPool, edges.vertical)
	putZeroScratch(&loopFilterBoolScratchPool, edges.horizontal)
	edges.vertical = nil
	edges.horizontal = nil
	edges.Vertical = nil
	edges.Horizontal = nil
}

func putLoopFilterEdgeMaps(edges *loopFilterEdgeMaps) {
	if edges == nil {
		return
	}
	putLoopFilterPlaneEdges(edges.Y)
	if edges.U != edges.Y {
		putLoopFilterPlaneEdges(edges.U)
	}
	if edges.V != edges.Y && edges.V != edges.U {
		putLoopFilterPlaneEdges(edges.V)
	}
	edges.Y = nil
	edges.U = nil
	edges.V = nil
}

func (m *loopFilterPlaneEdges) markBlock(g BlockGeometry) {
	if m == nil || m.Width4 <= 0 || m.Height4 <= 0 {
		return
	}
	y0 := maxIntLocal(0, g.Start4Y)
	y1 := minInt(m.Height4, g.End4Y)
	x0 := maxIntLocal(0, g.Start4X)
	x1 := minInt(m.Width4, g.End4X)
	m.markVertical(x0, y0, y1)
	m.markVertical(x1, y0, y1)
	m.markHorizontal(y0, x0, x1)
	m.markHorizontal(y1, x0, x1)
}

func (m *loopFilterPlaneEdges) markTransform(g BlockGeometry) {
	if m == nil {
		return
	}
	m.markBlock(g)
}

func (m *loopFilterPlaneEdges) markVertical(x4, y0, y1 int) {
	if m == nil || x4 <= 0 || x4 >= m.Width4 {
		return
	}
	y0 = maxIntLocal(0, y0)
	y1 = minInt(m.Height4, y1)
	for y4 := y0; y4 < y1; y4++ {
		m.Vertical[y4*m.VStride+x4] = true
	}
}

func (m *loopFilterPlaneEdges) markHorizontal(y4, x0, x1 int) {
	if m == nil || y4 <= 0 || y4 >= m.Height4 {
		return
	}
	x0 = maxIntLocal(0, x0)
	x1 = minInt(m.Width4, x1)
	row := y4 * m.HStride
	for x4 := x0; x4 < x1; x4++ {
		m.Horizontal[row+x4] = true
	}
}

func (m *loopFilterPlaneEdges) hasVertical(x, y int) bool {
	if m == nil || x <= 0 || y < 0 || x >= m.Width4 || y >= m.Height4 {
		return false
	}
	return m.Vertical[y*m.VStride+x]
}

func (m *loopFilterPlaneEdges) hasHorizontal(x, y int) bool {
	if m == nil || x < 0 || y <= 0 || x >= m.Width4 || y >= m.Height4 {
		return false
	}
	return m.Horizontal[y*m.HStride+x]
}

func transformGridCounts(root BlockGeometry, tx TxfmSize) (countX, countY int) {
	info := TxfmInfoFor(tx)
	w4 := int(info.W4)
	h4 := int(info.H4)
	width4 := maxIntLocal(0, root.Width4())
	height4 := maxIntLocal(0, root.Height4())
	return (width4 + w4 - 1) / w4, (height4 + h4 - 1) / h4
}

func buildPlaneTransformUnits(root BlockGeometry, tx TxfmSize) []BlockGeometry {
	info := TxfmInfoFor(tx)
	countX, countY := transformGridCounts(root, tx)
	units := make([]BlockGeometry, 0, countX*countY)
	for offY4 := 0; offY4 < root.Height4(); offY4 += int(info.H4) {
		for offX4 := 0; offX4 < root.Width4(); offX4 += int(info.W4) {
			units = append(units, childTXGeometry(root, tx, offX4, offY4))
		}
	}
	return units
}

func transformUnitBlocks(units []TransformUnit) []BlockGeometry {
	out := make([]BlockGeometry, len(units))
	for i, tu := range units {
		out[i] = tu.Global
	}
	return out
}

func markTransformGrid(edges *loopFilterPlaneEdges, root BlockGeometry, tx TxfmSize) {
	if edges == nil {
		return
	}
	info := TxfmInfoFor(tx)
	for offY4 := 0; offY4 < root.Height4(); offY4 += int(info.H4) {
		for offX4 := 0; offX4 < root.Width4(); offX4 += int(info.W4) {
			edges.markTransform(childTXGeometry(root, tx, offX4, offY4))
		}
	}
}

func markInterBlockEdges(edges *loopFilterEdgeMaps, layout av1.ChromaSubsampling, g BlockGeometry, hasChroma bool) {
	if edges == nil {
		return
	}
	if edges.Y != nil {
		edges.Y.markBlock(g)
	}
	if hasChroma && edges.U != nil {
		chroma := chromaGeometryForBlock(layout, g)
		edges.U.markBlock(chroma)
	}
}

func markInterTransformEdges(edges *loopFilterEdgeMaps, lumaUnits []BlockGeometry, chromaUnits []BlockGeometry) {
	if edges == nil {
		return
	}
	if edges.Y != nil {
		for _, tu := range lumaUnits {
			edges.Y.markTransform(tu)
		}
	}
	if edges.U != nil {
		for _, tu := range chromaUnits {
			edges.U.markTransform(tu)
		}
	}
}

func markInterTransformEdgesFromUnits(edges *loopFilterEdgeMaps, lumaUnits []TransformUnit, chromaUnits []BlockGeometry) {
	if edges == nil {
		return
	}
	if edges.Y != nil {
		for _, tu := range lumaUnits {
			edges.Y.markTransform(tu.Global)
		}
	}
	if edges.U != nil {
		for _, tu := range chromaUnits {
			edges.U.markTransform(tu)
		}
	}
}

func markInterTransformEdgesForBlock(edges *loopFilterEdgeMaps, layout av1.ChromaSubsampling, g BlockGeometry, lumaTX, chromaTX TxfmSize, hasChroma bool) {
	if edges == nil {
		return
	}
	markTransformGrid(edges.Y, g, lumaTX)
	if hasChroma && edges.U != nil {
		markTransformGrid(edges.U, chromaGeometryForBlock(layout, g), chromaTX)
	}
}
