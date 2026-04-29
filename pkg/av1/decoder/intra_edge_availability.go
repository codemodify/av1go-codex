package decoder

import "github.com/codemodify/av1go-codex/pkg/av1"

type intraEdgeAvailability struct {
	topHasRight      bool
	topHasRight422   bool
	topHasRight420   bool
	leftHasBottom    bool
	leftHasBottom422 bool
	leftHasBottom420 bool
}

var fullIntraEdgeAvailability = intraEdgeAvailabilityWithAll(true, true)

func intraEdgeAvailabilityWithAll(topHasRight, leftHasBottom bool) intraEdgeAvailability {
	return intraEdgeAvailability{
		topHasRight:      topHasRight,
		topHasRight422:   topHasRight,
		topHasRight420:   topHasRight,
		leftHasBottom:    leftHasBottom,
		leftHasBottom422: leftHasBottom,
		leftHasBottom420: leftHasBottom,
	}
}

func rootIntraEdgeAvailabilityForBlock(tile TileGeometry, g BlockGeometry) intraEdgeAvailability {
	return intraEdgeAvailabilityWithAll(g.End4X < tile.End4X, false)
}

func (e intraEdgeAvailability) forChromaLayout(layout av1.ChromaSubsampling) intraEdgeAvailability {
	switch layout {
	case av1.Chroma444:
		return intraEdgeAvailability{topHasRight: e.topHasRight, leftHasBottom: e.leftHasBottom}
	case av1.Chroma422:
		return intraEdgeAvailability{topHasRight: e.topHasRight422, leftHasBottom: e.leftHasBottom422}
	case av1.Chroma420:
		return intraEdgeAvailability{topHasRight: e.topHasRight420, leftHasBottom: e.leftHasBottom420}
	default:
		return intraEdgeAvailability{}
	}
}

func (e intraEdgeAvailability) withAllTopHasRight() intraEdgeAvailability {
	e.topHasRight = true
	e.topHasRight422 = true
	e.topHasRight420 = true
	return e
}

func (e intraEdgeAvailability) withAllLeftHasBottom() intraEdgeAvailability {
	e.leftHasBottom = true
	e.leftHasBottom422 = true
	e.leftHasBottom420 = true
	return e
}

func (e intraEdgeAvailability) onlyAllTopHasRight() intraEdgeAvailability {
	return intraEdgeAvailability{
		topHasRight:    e.topHasRight,
		topHasRight422: e.topHasRight422,
		topHasRight420: e.topHasRight420,
	}
}

func (e intraEdgeAvailability) onlyAllLeftHasBottom() intraEdgeAvailability {
	return intraEdgeAvailability{
		leftHasBottom:    e.leftHasBottom,
		leftHasBottom422: e.leftHasBottom422,
		leftHasBottom420: e.leftHasBottom420,
	}
}

func (e intraEdgeAvailability) h1Edge(level BlockLevel) intraEdgeAvailability {
	out := e.onlyAllLeftHasBottom()
	if level == BlockLevel8x8 {
		out.topHasRight420 = e.topHasRight420
	}
	return out
}

func (e intraEdgeAvailability) v1Edge(level BlockLevel) intraEdgeAvailability {
	out := e.onlyAllTopHasRight()
	if level == BlockLevel8x8 {
		out.leftHasBottom422 = e.leftHasBottom422
		out.leftHasBottom420 = e.leftHasBottom420
	}
	return out
}

func (e intraEdgeAvailability) h4Edge(level BlockLevel) intraEdgeAvailability {
	out := intraEdgeAvailabilityWithAll(false, true)
	if level == BlockLevel16x16 {
		out.topHasRight420 = e.topHasRight420
	}
	return out
}

func (e intraEdgeAvailability) v4Edge(level BlockLevel) intraEdgeAvailability {
	out := intraEdgeAvailabilityWithAll(true, false)
	if level == BlockLevel16x16 {
		out.leftHasBottom422 = e.leftHasBottom422
		out.leftHasBottom420 = e.leftHasBottom420
	}
	return out
}

func frameIntraNeighborAvailability(g BlockGeometry) (bool, bool) {
	return g.Start4Y > 0, g.Start4X > 0
}

func tileIntraNeighborAvailability(tile TileGeometry, g BlockGeometry) (bool, bool) {
	return g.Start4Y > tile.Start4Y, g.Start4X > tile.Start4X
}

func intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, tuOffX4, tuOffY4 int, tx TxfmSize, edge intraEdgeAvailability) intraEdgeAvailability {
	txInfo := TxfmInfoFor(tx)
	tuWidth4 := int(txInfo.W4)
	tuHeight4 := int(txInfo.H4)
	useInternalTopRight := tuOffX4+tuWidth4 < blockWidth4
	useExternalTopRight := tuOffY4 == 0 && edge.topHasRight
	useInternalLeftBottom := tuOffX4 == 0 && tuOffY4+tuHeight4 < blockHeight4
	useExternalLeftBottom := tuOffX4 == 0 && edge.leftHasBottom
	return intraEdgeAvailability{
		topHasRight:   useInternalTopRight || useExternalTopRight,
		leftHasBottom: useInternalLeftBottom || useExternalLeftBottom,
	}
}

func partitionIntraEdgeChildren(level BlockLevel, part BlockPartition, edge intraEdgeAvailability) ([4]intraEdgeAvailability, bool) {
	h0 := edge.withAllLeftHasBottom()
	h1 := edge.h1Edge(level)
	v0 := edge.withAllTopHasRight()
	v1 := edge.v1Edge(level)
	switch part {
	case PartitionNone:
		return [4]intraEdgeAvailability{}, false
	case PartitionSplit:
		if level == BlockLevel8x8 {
			return [4]intraEdgeAvailability{
				fullIntraEdgeAvailability,
				{
					topHasRight:      edge.topHasRight,
					topHasRight422:   edge.topHasRight422,
					topHasRight420:   edge.topHasRight420,
					leftHasBottom422: true,
				},
				{
					topHasRight:      true,
					topHasRight422:   edge.topHasRight422,
					topHasRight420:   edge.topHasRight420,
					leftHasBottom:    edge.leftHasBottom,
					leftHasBottom422: edge.leftHasBottom422,
					leftHasBottom420: edge.leftHasBottom420,
				},
				{
					topHasRight420:   edge.topHasRight420,
					leftHasBottom422: edge.leftHasBottom422,
					leftHasBottom420: edge.leftHasBottom420,
				},
			}, true
		}
		return [4]intraEdgeAvailability{
			fullIntraEdgeAvailability,
			edge.onlyAllTopHasRight(),
			edge.onlyAllLeftHasBottom().withAllTopHasRight(),
			{},
		}, true
	case PartitionH:
		return [4]intraEdgeAvailability{h0, h1}, true
	case PartitionV:
		return [4]intraEdgeAvailability{v0, v1}, true
	case PartitionTTopSplit:
		return [4]intraEdgeAvailability{
			fullIntraEdgeAvailability,
			v1,
			h1,
		}, true
	case PartitionTBottomSplit:
		return [4]intraEdgeAvailability{
			h0,
			v0,
			{topHasRight: false, leftHasBottom: false},
		}, true
	case PartitionTLeftSplit:
		return [4]intraEdgeAvailability{
			fullIntraEdgeAvailability,
			h1,
			v1,
		}, true
	case PartitionTRightSplit:
		return [4]intraEdgeAvailability{
			v0,
			h0,
			{topHasRight: false, leftHasBottom: false},
		}, true
	case PartitionH4:
		return [4]intraEdgeAvailability{
			h0,
			edge.h4Edge(level),
			intraEdgeAvailabilityWithAll(false, true),
			h1,
		}, true
	case PartitionV4:
		return [4]intraEdgeAvailability{
			v0,
			edge.v4Edge(level),
			intraEdgeAvailabilityWithAll(true, false),
			v1,
		}, true
	default:
		return [4]intraEdgeAvailability{}, false
	}
}
