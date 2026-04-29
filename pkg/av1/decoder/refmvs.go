package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

const (
	refMVFlagGlobal = 1 << iota
	refMVFlagNew
)

const maxRefMVStackSize = 8

const refMVInterIntraSentinel int8 = -2

type RefMVCandidate struct {
	MV     MotionVector
	Weight int
	Flags  uint8
}

type CompoundRefMVCandidate struct {
	MV     [2]MotionVector
	Weight int
	Flags  uint8
}

func sortRefMVCandidatesByWeight(cands []RefMVCandidate) {
	for i := 1; i < len(cands); i++ {
		cur := cands[i]
		j := i
		for j > 0 && cur.Weight > cands[j-1].Weight {
			cands[j] = cands[j-1]
			j--
		}
		cands[j] = cur
	}
}

func sortCompoundCandidatesByWeight(cands []CompoundRefMVCandidate) {
	for i := 1; i < len(cands); i++ {
		cur := cands[i]
		j := i
		for j > 0 && cur.Weight > cands[j-1].Weight {
			cands[j] = cands[j-1]
			j--
		}
		cands[j] = cur
	}
}

func sortCompoundCandidatesByWeightAndFlags(cands []CompoundRefMVCandidate) {
	for i := 1; i < len(cands); i++ {
		cur := cands[i]
		j := i
		for j > 0 {
			prev := cands[j-1]
			if cur.Weight < prev.Weight || (cur.Weight == prev.Weight && cur.Flags >= prev.Flags) {
				break
			}
			cands[j] = prev
			j--
		}
		cands[j] = cur
	}
}

type SpatialMVBlock struct {
	Ref      [2]int8
	Slot     [2]int8
	MV       [2]MotionVector
	Filter   InterFilterPair
	Size     BlockSize
	RefFlags [2]uint8
	Flags    uint8
	Valid    bool
	Intra    bool
}

type SpatialMVTile struct {
	Geometry TileGeometry
	Width4   int
	Height4  int
	Stride4  int
	Base     int
	Blocks   []SpatialMVBlock
	scratch  *scratchBuffer[SpatialMVBlock]
	FramePOC uint8
	SlotPOC  [8]uint8
}

var spatialMVBlockScratchCache = stickyScratchCache[SpatialMVBlock]{maxStored: 16}

func NewSpatialMVTile(geom TileGeometry) *SpatialMVTile {
	width4 := geom.End4X - geom.Start4X
	height4 := geom.End4Y - geom.Start4Y
	if width4 < 0 {
		width4 = 0
	}
	if height4 < 0 {
		height4 = 0
	}
	return &SpatialMVTile{
		Geometry: geom,
		Width4:   width4,
		Height4:  height4,
		Stride4:  width4,
		Blocks:   make([]SpatialMVBlock, width4*height4),
		scratch:  nil,
	}
}

func takeSpatialMVTileScratch(geom TileGeometry) *SpatialMVTile {
	width4 := geom.End4X - geom.Start4X
	height4 := geom.End4Y - geom.Start4Y
	if width4 < 0 {
		width4 = 0
	}
	if height4 < 0 {
		height4 = 0
	}
	blocks := spatialMVBlockScratchCache.take(width4 * height4)
	return &SpatialMVTile{
		Geometry: geom,
		Width4:   width4,
		Height4:  height4,
		Stride4:  width4,
		Blocks:   blocks.buf[:width4*height4],
		scratch:  blocks,
	}
}

func putSpatialMVTileScratch(tile *SpatialMVTile) {
	if tile == nil || tile.scratch == nil {
		return
	}
	spatialMVBlockScratchCache.putZero(tile.scratch)
}

func (t *SpatialMVTile) block(x4, y4 int) *SpatialMVBlock {
	if t == nil {
		return nil
	}
	if x4 < t.Geometry.Start4X || x4 >= t.Geometry.End4X || y4 < t.Geometry.Start4Y || y4 >= t.Geometry.End4Y {
		return nil
	}
	ix := x4 - t.Geometry.Start4X
	iy := y4 - t.Geometry.Start4Y
	stride4 := t.Stride4
	if stride4 == 0 {
		stride4 = t.Width4
	}
	if t.Base == 0 {
		return &t.Blocks[iy*stride4+ix]
	}
	return &t.Blocks[t.Base+iy*stride4+ix]
}

func (t *SpatialMVTile) tileView(geom TileGeometry) SpatialMVTile {
	width4 := geom.End4X - geom.Start4X
	height4 := geom.End4Y - geom.Start4Y
	if width4 < 0 {
		width4 = 0
	}
	if height4 < 0 {
		height4 = 0
	}
	if t == nil {
		return SpatialMVTile{
			Geometry: geom,
			Width4:   width4,
			Height4:  height4,
			Stride4:  width4,
		}
	}
	stride4 := t.Stride4
	if stride4 == 0 {
		stride4 = t.Width4
	}
	base := t.Base
	base += (geom.Start4Y-t.Geometry.Start4Y)*stride4 + (geom.Start4X - t.Geometry.Start4X)
	return SpatialMVTile{
		Geometry: geom,
		Width4:   width4,
		Height4:  height4,
		Stride4:  stride4,
		Blocks:   t.Blocks[base:],
		FramePOC: t.FramePOC,
		SlotPOC:  t.SlotPOC,
	}
}

func (t *SpatialMVTile) SplatIntra(g BlockGeometry) {
	if t == nil {
		return
	}
	for y4 := g.Start4Y; y4 < g.End4Y; y4++ {
		for x4 := g.Start4X; x4 < g.End4X; x4++ {
			b := t.block(x4, y4)
			if b == nil {
				continue
			}
			*b = SpatialMVBlock{
				Size:  g.Size,
				Valid: true,
				Intra: true,
				Ref:   [2]int8{-1, -1},
				Slot:  [2]int8{-1, -1},
			}
		}
	}
}

func (t *SpatialMVTile) SplatIntraBC(g BlockGeometry, mv MotionVector) {
	if t == nil {
		return
	}
	for y4 := g.Start4Y; y4 < g.End4Y; y4++ {
		for x4 := g.Start4X; x4 < g.End4X; x4++ {
			b := t.block(x4, y4)
			if b == nil {
				continue
			}
			*b = SpatialMVBlock{
				Ref:   [2]int8{0, -1},
				Slot:  [2]int8{-1, -1},
				MV:    [2]MotionVector{mv, {}},
				Size:  g.Size,
				Valid: true,
			}
		}
	}
}

func (t *SpatialMVTile) SplatSingleRef(g BlockGeometry, ref0 int8, refSlot int8, mv MotionVector, mode InterPredMode, filter InterFilterPair) {
	t.SplatSingleRefWithInterIntra(g, ref0, refSlot, mv, mode, filter, false)
}

func (t *SpatialMVTile) SplatSingleRefWithInterIntra(g BlockGeometry, ref0 int8, refSlot int8, mv MotionVector, mode InterPredMode, filter InterFilterPair, interIntra bool) {
	if t == nil {
		return
	}
	flags := uint8(0)
	if mode == InterPredGlobal && minInt(g.Width4(), g.Height4()) >= 2 {
		flags |= refMVFlagGlobal
	}
	if mode == InterPredNew {
		flags |= refMVFlagNew
	}
	ref1 := int8(-1)
	if interIntra {
		ref1 = refMVInterIntraSentinel
	}
	for y4 := g.Start4Y; y4 < g.End4Y; y4++ {
		for x4 := g.Start4X; x4 < g.End4X; x4++ {
			b := t.block(x4, y4)
			if b == nil {
				continue
			}
			*b = SpatialMVBlock{
				Ref:      [2]int8{ref0, ref1},
				Slot:     [2]int8{refSlot, -1},
				MV:       [2]MotionVector{mv, {}},
				Filter:   filter,
				Size:     g.Size,
				RefFlags: [2]uint8{flags, 0},
				Flags:    flags,
				Valid:    true,
			}
		}
	}
}

func (t *SpatialMVTile) SplatCompoundRef(g BlockGeometry, ref0, ref1 int8, refSlot0, refSlot1 int8, mv0, mv1 MotionVector, mode0, mode1 InterPredMode, filter InterFilterPair) {
	if t == nil {
		return
	}
	flags0 := uint8(0)
	if mode0 == InterPredGlobal {
		flags0 |= refMVFlagGlobal
	}
	if mode0 == InterPredNew {
		flags0 |= refMVFlagNew
	}
	flags1 := uint8(0)
	if mode1 == InterPredGlobal {
		flags1 |= refMVFlagGlobal
	}
	if mode1 == InterPredNew {
		flags1 |= refMVFlagNew
	}
	for y4 := g.Start4Y; y4 < g.End4Y; y4++ {
		for x4 := g.Start4X; x4 < g.End4X; x4++ {
			b := t.block(x4, y4)
			if b == nil {
				continue
			}
			*b = SpatialMVBlock{
				Ref:      [2]int8{ref0, ref1},
				Slot:     [2]int8{refSlot0, refSlot1},
				MV:       [2]MotionVector{mv0, mv1},
				Filter:   filter,
				Size:     g.Size,
				RefFlags: [2]uint8{flags0, flags1},
				Flags:    flags0 | flags1,
				Valid:    true,
			}
		}
	}
}

func NewFrameMVField(width, height int) *SpatialMVTile {
	return NewSpatialMVTile(TileGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   miGridSize4(width),
		End4Y:   miGridSize4(height),
		EndX:    width,
		EndY:    height,
	})
}

func takeFrameMVField(width, height int) *SpatialMVTile {
	return takeSpatialMVTileScratch(TileGeometry{
		Start4X: 0,
		Start4Y: 0,
		End4X:   miGridSize4(width),
		End4Y:   miGridSize4(height),
		EndX:    width,
		EndY:    height,
	})
}

func CloneMVField(src *SpatialMVTile) *SpatialMVTile {
	if src == nil {
		return nil
	}
	blocks := make([]SpatialMVBlock, src.Width4*src.Height4)
	stride4 := src.Stride4
	if stride4 == 0 {
		stride4 = src.Width4
	}
	if src.Base == 0 && stride4 == src.Width4 {
		copy(blocks, src.Blocks[:len(blocks)])
	} else {
		for y4 := 0; y4 < src.Height4; y4++ {
			srcStart := src.Base + y4*stride4
			dstStart := y4 * src.Width4
			copy(blocks[dstStart:dstStart+src.Width4], src.Blocks[srcStart:srcStart+src.Width4])
		}
	}
	clone := &SpatialMVTile{
		Geometry: src.Geometry,
		Width4:   src.Width4,
		Height4:  src.Height4,
		Stride4:  src.Width4,
		Blocks:   blocks,
		FramePOC: src.FramePOC,
		SlotPOC:  src.SlotPOC,
	}
	return clone
}

func projectTemporalMV(mv MotionVector, num, den int) MotionVector {
	if den == 0 {
		return mv
	}
	if den < 0 {
		den = -den
		num = -num
	}
	divMult := [...]int{
		0, 16384, 8192, 5461, 4096, 3276, 2730, 2340,
		2048, 1820, 1638, 1489, 1365, 1260, 1170, 1092,
		1024, 963, 910, 862, 819, 780, 744, 712,
		682, 655, 630, 606, 585, 564, 546, 528,
	}
	if den >= len(divMult) || num <= -32 || num >= 32 {
		return mv
	}
	frac := num * divMult[den]
	y := mv.Y * frac
	x := mv.X * frac
	return MotionVector{
		Y: clipMVComp((y+8192+signBit(y))>>14, -0x3fff, 0x3fff),
		X: clipMVComp((x+8192+signBit(x))>>14, -0x3fff, 0x3fff),
	}
}

func signBit(v int) int {
	if v < 0 {
		return -1
	}
	return 0
}

func appendOrAccumulateRefMVCandidate(cands []RefMVCandidate, cand RefMVCandidate) []RefMVCandidate {
	for i := range cands {
		if cands[i].MV == cand.MV {
			cands[i].Weight += cand.Weight
			return cands
		}
	}
	if len(cands) >= maxRefMVStackSize {
		return cands
	}
	return append(cands, cand)
}

func nominalBlockDims4(g BlockGeometry) (int, int) {
	dims := BlockDims(g.Size)
	bw4 := int(dims.W4)
	bh4 := int(dims.H4)
	if bw4 <= 0 {
		bw4 = g.Width4()
	}
	if bh4 <= 0 {
		bh4 = g.Height4()
	}
	if bw4 <= 0 {
		bw4 = 1
	}
	if bh4 <= 0 {
		bh4 = 1
	}
	return bw4, bh4
}

func nominalBlockDims4Clamped16(g BlockGeometry) (int, int) {
	bw4, bh4 := nominalBlockDims4(g)
	return minInt(bw4, 16), minInt(bh4, 16)
}

func temporalRefMVCandidatesMatch(a, b MotionVector) bool {
	return absInt(a.Y-b.Y) < 4 && absInt(a.X-b.X) < 4
}

func preferTemporalSupportedRefMVTies(cands []RefMVCandidate, temporal []RefMVCandidate) {
	for i := 1; i < len(cands); i++ {
		cur := cands[i]
		j := i
		curScore := temporalRefMVSupport(cur, temporal)
		for j > 0 && cur.Weight == cands[j-1].Weight {
			prevScore := temporalRefMVSupport(cands[j-1], temporal)
			if curScore <= prevScore {
				break
			}
			cands[j] = cands[j-1]
			j--
		}
		cands[j] = cur
	}
}

func temporalRefMVSupport(cand RefMVCandidate, temporal []RefMVCandidate) int {
	score := 0
	for _, tcand := range temporal {
		if temporalRefMVCandidatesMatch(cand.MV, tcand.MV) {
			score += tcand.Weight
		}
	}
	return score
}

func (t *SpatialMVTile) FindTemporalCandidatesForSlot(g BlockGeometry, refSlot int8) []RefMVCandidate {
	return t.findTemporalCandidatesForSlotInto(nil, g, refSlot)
}

func (t *SpatialMVTile) findTemporalCandidatesForSlotInto(dst []RefMVCandidate, g BlockGeometry, refSlot int8) []RefMVCandidate {
	if t == nil || refSlot < 0 {
		return dst[:0]
	}
	cands := dst[:0]
	t.forEachTemporalCandidatePoint(g, func(x4, y4 int) {
		b := t.block(x4, y4)
		if b == nil || !b.Valid || b.Intra || b.Slot[0] != refSlot {
			return
		}
		mv := b.MV[0]
		cands = appendOrAccumulateRefMVCandidate(cands, RefMVCandidate{
			MV:     mv,
			Weight: 2,
			Flags:  b.Flags,
		})
	})
	return cands
}

func (t *SpatialMVTile) FindTemporalCandidatesForSlotProjected(seq av1.SequenceHeader, parsed *ParsedFrame, ref0, refSlot int8, hdr *obu.FrameHeader, g BlockGeometry) []RefMVCandidate {
	return t.findTemporalCandidatesForSlotProjectedInto(nil, seq, parsed, ref0, refSlot, hdr, g)
}

func (t *SpatialMVTile) findTemporalCandidatesForSlotProjectedInto(dst []RefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, ref0, refSlot int8, hdr *obu.FrameHeader, g BlockGeometry) []RefMVCandidate {
	if t == nil || refSlot < 0 {
		return dst[:0]
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	currentPOC := 0
	targetPOC := 0
	canProject := parsed != nil && hdr != nil && seq.EnableOrderHint && orderHintBits > 0 && ref0 >= 0 && int(ref0) < len(parsed.RefFramePOC)
	if canProject {
		currentPOC = int(hdr.FrameOffset)
		targetPOC = int(parsed.RefFramePOC[ref0])
	}
	cands := dst[:0]
	add := func(x4, y4 int) {
		b := t.block(x4, y4)
		if b == nil || !b.Valid || b.Intra || b.Slot[0] != refSlot {
			return
		}
		mv := b.MV[0]
		if canProject && b.Slot[0] >= 0 && int(b.Slot[0]) < len(t.SlotPOC) {
			sourceRefPOC := int(t.SlotPOC[b.Slot[0]])
			num := compoundPOCDiff(orderHintBits, currentPOC, targetPOC)
			den := compoundPOCDiff(orderHintBits, int(t.FramePOC), sourceRefPOC)
			mv = projectTemporalMV(mv, num, den)
		}
		mv = fixMVPrecision(hdr, mv)
		cands = appendOrAccumulateRefMVCandidate(cands, RefMVCandidate{
			MV:     mv,
			Weight: 2,
			Flags:  b.Flags,
		})
	}
	t.forEachTemporalCandidatePoint(g, add)
	return cands
}

func (t *SpatialMVTile) forEachTemporalCandidatePoint(g BlockGeometry, visit func(x4, y4 int)) {
	if t == nil || visit == nil {
		return
	}
	bw4, bh4 := nominalBlockDims4(g)
	w4 := minInt(minInt(bw4, 16), t.Geometry.End4X-g.Start4X)
	h4 := minInt(minInt(bh4, 16), t.Geometry.End4Y-g.Start4Y)
	if w4 <= 0 || h4 <= 0 {
		return
	}
	bx8 := g.Start4X >> 1
	by8 := g.Start4Y >> 1
	w8 := minInt((w4+1)>>1, 8)
	h8 := minInt((h4+1)>>1, 8)
	stepH := ternaryInt(bw4 >= 16, 2, 1)
	stepV := ternaryInt(bh4 >= 16, 2, 1)
	for y8 := 0; y8 < h8; y8 += stepV {
		for x8 := 0; x8 < w8; x8 += stepH {
			visit((bx8+x8)*2, (by8+y8)*2)
		}
	}
	if minInt(bw4, bh4) < 2 || maxIntLocal(bw4, bh4) >= 16 {
		return
	}
	bw8 := bw4 >> 1
	bh8 := bh4 >> 1
	colStart8 := maxIntLocal(t.Geometry.Start4X>>1, bx8&^7)
	colEnd8 := minInt(t.Geometry.End4X>>1, (bx8&^7)+8)
	rowEnd8 := minInt(t.Geometry.End4Y>>1, (by8&^7)+8)
	hasBottom := by8+bh8 < rowEnd8
	if hasBottom && bx8-1 >= colStart8 {
		visit((bx8-1)*2, (by8+bh8)*2)
	}
	if bx8+bw8 < colEnd8 {
		if hasBottom {
			visit((bx8+bw8)*2, (by8+bh8)*2)
		}
		if by8+bh8-1 < rowEnd8 {
			visit((bx8+bw8)*2, (by8+bh8-1)*2)
		}
	}
}

func (t *SpatialMVTile) FindTemporalCompoundCandidatesForSlots(g BlockGeometry, refSlot0, refSlot1 int8) []CompoundRefMVCandidate {
	return t.findTemporalCompoundCandidatesForSlotsInto(nil, g, refSlot0, refSlot1)
}

func (t *SpatialMVTile) findTemporalCompoundCandidatesForSlotsInto(dst []CompoundRefMVCandidate, g BlockGeometry, refSlot0, refSlot1 int8) []CompoundRefMVCandidate {
	if t == nil || refSlot0 < 0 || refSlot1 < 0 {
		return dst[:0]
	}
	x1 := g.Start4X
	y1 := g.Start4Y
	x2 := g.Start4X + minInt(g.Width4()-1, 2)
	y2 := g.Start4Y + minInt(g.Height4()-1, 2)
	cands := dst[:0]
	add := func(x4, y4 int) {
		b := t.block(x4, y4)
		mv0, mv1, ok := matchCompoundRefBlockBySlot(b, refSlot0, refSlot1)
		if !ok {
			return
		}
		found := false
		for i := range cands {
			if cands[i].MV[0] == mv0 && cands[i].MV[1] == mv1 {
				cands[i].Weight += 2
				found = true
				break
			}
		}
		if found {
			return
		}
		cands = append(cands, CompoundRefMVCandidate{
			MV:     [2]MotionVector{mv0, mv1},
			Weight: 2,
			Flags:  b.Flags,
		})
	}
	add(x1, y1)
	if x2 != x1 {
		add(x2, y1)
	}
	if y2 != y1 {
		add(x1, y2)
	}
	if x2 != x1 && y2 != y1 {
		add(x2, y2)
	}
	return cands
}

func AugmentSingleRefCandidatesTemporal(cands []RefMVCandidate, temporal []RefMVCandidate) []RefMVCandidate {
	write := 0
	for _, cand := range cands {
		if cand.Weight <= 0 {
			continue
		}
		cands[write] = cand
		write++
	}
	base := cands[:write]
	nearestCnt := 0
	for nearestCnt < len(base) && base[nearestCnt].Weight >= 640 {
		nearestCnt++
	}
	var extraBuf [8]RefMVCandidate
	extra := extraBuf[:0]
	for _, cand := range temporal {
		if cand.Weight <= 0 {
			continue
		}
		found := false
		for i := range base {
			if base[i].MV == cand.MV {
				base[i].Weight += cand.Weight
				found = true
				break
			}
		}
		if !found {
			extra = appendOrAccumulateRefMVCandidate(extra, cand)
		}
	}
	if nearestCnt > 1 {
		sortRefMVCandidatesByWeight(base[:nearestCnt])
	}
	if len(extra) > 0 {
		oldLen := len(base)
		base = append(base, extra...)
		copy(base[nearestCnt+len(extra):], base[nearestCnt:oldLen])
		copy(base[nearestCnt:], extra)
	}
	if nearestCnt < len(base) {
		sortRefMVCandidatesByWeight(base[nearestCnt:])
	}
	for len(base) < 2 {
		base = append(base, RefMVCandidate{MV: MotionVector{}, Weight: 0})
	}
	return base
}

func AugmentCompoundCandidatesTemporal(cands []CompoundRefMVCandidate, temporal []CompoundRefMVCandidate) []CompoundRefMVCandidate {
	write := 0
	for _, cand := range cands {
		if cand.Weight <= 0 {
			continue
		}
		cands[write] = cand
		write++
	}
	base := cands[:write]
	nearestCnt := 0
	for nearestCnt < len(base) && base[nearestCnt].Weight >= 640 {
		nearestCnt++
	}
	temporalUnique := make([]CompoundRefMVCandidate, 0, len(temporal))
	for _, cand := range temporal {
		if cand.Weight <= 0 {
			continue
		}
		found := false
		for i := 0; i < nearestCnt; i++ {
			if base[i].MV == cand.MV {
				base[i].Weight += cand.Weight
				found = true
				break
			}
		}
		if !found {
			for i := range temporalUnique {
				if temporalUnique[i].MV == cand.MV {
					temporalUnique[i].Weight += cand.Weight
					found = true
					break
				}
			}
		}
		if !found {
			for i := nearestCnt; i < len(base); i++ {
				if base[i].MV == cand.MV {
					cand.Weight += base[i].Weight
					copy(base[i:], base[i+1:])
					base = base[:len(base)-1]
					break
				}
			}
			temporalUnique = append(temporalUnique, cand)
		}
	}
	if len(temporalUnique) > 0 {
		oldLen := len(base)
		base = append(base, temporalUnique...)
		copy(base[nearestCnt+len(temporalUnique):], base[nearestCnt:oldLen])
		copy(base[nearestCnt:], temporalUnique)
	}
	if nearestCnt > 1 {
		sortCompoundCandidatesByWeight(base[:nearestCnt])
	}
	if nearestCnt < len(base) {
		sortCompoundCandidatesByWeight(base[nearestCnt:])
	}
	for len(base) < 2 {
		base = append(base, CompoundRefMVCandidate{MV: [2]MotionVector{{}, {}}, Weight: 0})
	}
	return base
}

func (t *SpatialMVTile) FindSingleRefCandidates(hdr *obu.FrameHeader, g BlockGeometry, ref0 int8) ([]RefMVCandidate, int) {
	return t.FindSingleRefCandidatesWithEdge(hdr, g, ref0, fullIntraEdgeAvailability)
}

func (t *SpatialMVTile) FindSingleRefCandidatesWithEdge(hdr *obu.FrameHeader, g BlockGeometry, ref0 int8, edge intraEdgeAvailability) ([]RefMVCandidate, int) {
	return t.findSingleRefCandidatesWithEdgeInto(nil, hdr, g, ref0, edge)
}

func (t *SpatialMVTile) findSingleRefCandidatesWithEdgeInto(dst []RefMVCandidate, hdr *obu.FrameHeader, g BlockGeometry, ref0 int8, edge intraEdgeAvailability) ([]RefMVCandidate, int) {
	return t.findSingleRefCandidatesWithEdgeAndTemporalInto(dst, hdr, g, ref0, edge, nil)
}

func (t *SpatialMVTile) findSingleRefCandidatesWithEdgeAndTemporalInto(dst []RefMVCandidate, hdr *obu.FrameHeader, g BlockGeometry, ref0 int8, edge intraEdgeAvailability, temporal []RefMVCandidate) ([]RefMVCandidate, int) {
	if t == nil {
		cands := append(dst[:0], RefMVCandidate{MV: MotionVector{}})
		return cands, 0
	}

	bw4, bh4 := nominalBlockDims4(g)
	bx4 := g.Start4X
	by4 := g.Start4Y
	w4 := minInt(minInt(bw4, 16), t.Geometry.End4X-bx4)
	h4 := minInt(minInt(bh4, 16), t.Geometry.End4Y-by4)

	var (
		cands       = dst[:0]
		haveNewMV   int
		haveRowMVs  int
		haveColMVs  int
		nearestCnt  int
		maxRows     int
		maxCols     int
		nRows       = -1
		nCols       = -1
		dummyNewMV  int
		globalmvCtx = boolToInt(hdr != nil && hdr.UseRefFrameMvs)
	)
	gmv, haveGMV := singleRefCandidateGlobalMotion(hdr, ref0, g)

	if by4 > t.Geometry.Start4Y {
		maxRows = minInt(((by4-t.Geometry.Start4Y)+1)>>1, 2+boolToInt(bh4 > 1))
		nRows = t.scanRow(hdr, &cands, ref0, gmv, haveGMV, bx4, by4-1, bw4, w4, maxRows, ternaryInt(bw4 >= 16, 4, 1), &haveNewMV, &haveRowMVs)
	}
	if bx4 > t.Geometry.Start4X {
		maxCols = minInt(((bx4-t.Geometry.Start4X)+1)>>1, 2+boolToInt(bw4 > 1))
		nCols = t.scanCol(hdr, &cands, ref0, gmv, haveGMV, bx4-1, by4, bh4, h4, maxCols, ternaryInt(bh4 >= 16, 4, 1), &haveNewMV, &haveColMVs)
	}
	if edge.topHasRight && nRows >= 0 && maxIntLocal(bw4, bh4) <= 16 && bx4+bw4 < t.Geometry.End4X {
		t.addSpatialCandidate(hdr, &cands, 4, t.block(bx4+bw4, by4-1), ref0, gmv, haveGMV, &haveNewMV, &haveRowMVs)
	}

	nearestMatch := haveColMVs + haveRowMVs
	nearestCnt = len(cands)
	for i := 0; i < nearestCnt; i++ {
		cands[i].Weight += 640
	}

	for _, cand := range temporal {
		if cand.Weight <= 0 {
			continue
		}
		cands = appendOrAccumulateRefMVCandidate(cands, cand)
	}

	if nRows >= 0 || nCols >= 0 {
		t.addSpatialCandidate(hdr, &cands, 4, t.block(bx4-1, by4-1), ref0, gmv, haveGMV, &dummyNewMV, &haveRowMVs)
	}
	for n := 2; n <= 3; n++ {
		if nRows >= 0 && n > nRows && n <= maxRows {
			y4 := by4 - 2*n + 1
			if y4 >= t.Geometry.Start4Y {
				nRows += t.scanRow(hdr, &cands, ref0, gmv, haveGMV, bx4|1, y4|1, bw4, w4, 1+maxRows-n, ternaryInt(bw4 >= 16, 4, 2), &dummyNewMV, &haveRowMVs)
			}
		}
		if nCols >= 0 && n > nCols && n <= maxCols {
			x4 := (bx4 - n*2 + 1) | 1
			if x4 >= t.Geometry.Start4X {
				nCols += t.scanCol(hdr, &cands, ref0, gmv, haveGMV, x4, by4|1, bh4, h4, 1+maxCols-n, ternaryInt(bh4 >= 16, 4, 2), &dummyNewMV, &haveColMVs)
			}
		}
	}

	refMatchCount := haveColMVs + haveRowMVs
	refmvCtx := 0
	newmvCtx := 0
	switch nearestMatch {
	case 0:
		refmvCtx = minInt(2, refMatchCount)
		newmvCtx = boolToInt(refMatchCount > 0)
	case 1:
		refmvCtx = minInt(refMatchCount*3, 4)
		newmvCtx = 3 - haveNewMV
	default:
		refmvCtx = 5
		newmvCtx = 5 - haveNewMV
	}

	if nearestCnt > 1 {
		sortRefMVCandidatesByWeight(cands[:nearestCnt])
	}
	if nearestCnt < len(cands) {
		sortRefMVCandidatesByWeight(cands[nearestCnt:])
	}
	padMV := MotionVector{}
	if hdr != nil && ref0 >= 0 {
		padMV = approximateGlobalMotionVectorForBlock(hdr, ref0, g)
	}
	for len(cands) < 2 {
		cands = append(cands, RefMVCandidate{MV: padMV})
	}
	return cands, (refmvCtx << 4) | (globalmvCtx << 3) | newmvCtx
}

func (t *SpatialMVTile) FindSingleRefCandidatesParsed(seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0 int8) ([]RefMVCandidate, int) {
	return t.FindSingleRefCandidatesParsedWithEdge(seq, parsed, hdr, g, ref0, fullIntraEdgeAvailability)
}

func (t *SpatialMVTile) FindSingleRefCandidatesParsedWithEdge(seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0 int8, edge intraEdgeAvailability) ([]RefMVCandidate, int) {
	cands, ctx := t.FindSingleRefCandidatesWithEdge(hdr, g, ref0, edge)
	if t == nil || parsed == nil || ref0 <= 0 {
		return cands, ctx
	}
	active := make([]RefMVCandidate, 0, len(cands))
	for _, cand := range cands {
		if cand.Weight > 0 {
			active = append(active, cand)
		}
	}
	if len(active) < 2 {
		active = t.appendSingleRefExtendedCandidates(active, seq, parsed, hdr, g, ref0)
	}
	for len(active) < 2 {
		active = append(active, RefMVCandidate{MV: MotionVector{}})
	}
	return active, ctx
}

func augmentSingleRefCandidatesExtended(cands []RefMVCandidate, t *SpatialMVTile, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0 int8) []RefMVCandidate {
	write := 0
	for _, cand := range cands {
		if cand.Weight <= 0 {
			continue
		}
		cands[write] = cand
		write++
	}
	cands = cands[:write]
	if t != nil && parsed != nil && len(cands) < 2 {
		cands = t.appendSingleRefExtendedCandidates(cands, seq, parsed, hdr, g, ref0)
	}
	padMV := MotionVector{}
	if hdr != nil && ref0 >= 0 {
		padMV = approximateGlobalMotionVectorForBlock(hdr, ref0, g)
	}
	for len(cands) < 2 {
		cands = append(cands, RefMVCandidate{MV: padMV})
	}
	return cands
}

func appendUniqueRefMV(dst []RefMVCandidate, mv MotionVector) []RefMVCandidate {
	for _, cur := range dst {
		if cur.MV == mv {
			return dst
		}
	}
	return append(dst, RefMVCandidate{MV: mv, Weight: 2})
}

func collectSingleRefExtendedCandidate(cands []RefMVCandidate, b *SpatialMVBlock, targetSign int, seq av1.SequenceHeader, parsed *ParsedFrame) []RefMVCandidate {
	if b == nil || !b.Valid || b.Intra {
		return cands
	}
	for n := 0; n < 2 && len(cands) < maxRefMVStackSize; n++ {
		candRef := b.Ref[n]
		if candRef < 0 {
			continue
		}
		candMV := b.MV[n]
		if targetSign^compoundRefSignBias(seq, parsed, candRef) != 0 {
			candMV.X = -candMV.X
			candMV.Y = -candMV.Y
		}
		cands = appendUniqueRefMV(cands, candMV)
	}
	return cands
}

func singleRefCandidateGlobalMotion(hdr *obu.FrameHeader, ref0 int8, g BlockGeometry) (MotionVector, bool) {
	if hdr == nil || ref0 < 0 || int(ref0) >= len(hdr.GMV) {
		return MotionVector{}, false
	}
	if hdr.GMV[ref0].Type <= 1 {
		return MotionVector{}, false
	}
	return approximateGlobalMotionVectorForBlock(hdr, ref0, g), true
}

func (t *SpatialMVTile) FindCompoundRefCandidates(hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesDetailedInto(nil, av1.SequenceHeader{}, nil, hdr, g, ref0, ref1)
}

func (t *SpatialMVTile) FindCompoundRefCandidatesParsed(seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesDetailedInto(nil, seq, parsed, hdr, g, ref0, ref1)
}

func (t *SpatialMVTile) FindCompoundRefCandidatesParsedInto(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesDetailedInto(dst, seq, parsed, hdr, g, ref0, ref1)
}

func (t *SpatialMVTile) findCompoundRefCandidatesDetailedInto(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesDetailedIntoExt(dst, seq, parsed, hdr, g, ref0, ref1, true)
}

func (t *SpatialMVTile) findCompoundRefCandidatesBaseInto(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesDetailedIntoExt(dst, seq, parsed, hdr, g, ref0, ref1, false)
}

func (t *SpatialMVTile) findCompoundRefCandidatesDetailedIntoExt(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8, appendExtended bool) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesWithTemporalInto(dst, seq, parsed, hdr, g, ref0, ref1, appendExtended, nil)
}

func (t *SpatialMVTile) findCompoundRefCandidatesBaseWithTemporalInto(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8, temporal []CompoundRefMVCandidate) ([]CompoundRefMVCandidate, int) {
	return t.findCompoundRefCandidatesWithTemporalInto(dst, seq, parsed, hdr, g, ref0, ref1, false, temporal)
}

func (t *SpatialMVTile) findCompoundRefCandidatesWithTemporalInto(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8, appendExtended bool, temporal []CompoundRefMVCandidate) ([]CompoundRefMVCandidate, int) {
	if t == nil {
		return []CompoundRefMVCandidate{{MV: [2]MotionVector{{}, {}}}}, 0
	}
	bw4, bh4 := nominalBlockDims4(g)
	bx4 := g.Start4X
	by4 := g.Start4Y
	w4 := minInt(minInt(bw4, 16), t.Geometry.End4X-bx4)
	h4 := minInt(minInt(bh4, 16), t.Geometry.End4Y-by4)

	var (
		cands      = dst[:0]
		haveNewMV  int
		haveRowMVs int
		haveColMVs int
		dummyNewMV int
		maxRows    int
		maxCols    int
		nRows      = -1
		nCols      = -1
	)

	if by4 > t.Geometry.Start4Y {
		maxRows = minInt(((by4-t.Geometry.Start4Y)+1)>>1, 2+boolToInt(bh4 > 1))
		nRows = t.scanCompoundRow(&cands, ref0, ref1, bx4, by4-1, bw4, w4, maxRows, ternaryInt(bw4 >= 16, 4, 1), &haveNewMV, &haveRowMVs)
	}
	if bx4 > t.Geometry.Start4X {
		maxCols = minInt(((bx4-t.Geometry.Start4X)+1)>>1, 2+boolToInt(bw4 > 1))
		nCols = t.scanCompoundCol(&cands, ref0, ref1, bx4-1, by4, bh4, h4, maxCols, ternaryInt(bh4 >= 16, 4, 1), &haveNewMV, &haveColMVs)
	}
	if nRows >= 0 && maxIntLocal(bw4, bh4) <= 16 && bx4+bw4 < t.Geometry.End4X {
		addCompoundSpatialCandidate(&cands, 4, t.block(bx4+bw4, by4-1), ref0, ref1, &haveNewMV, &haveRowMVs)
	}

	nearestMatch := haveColMVs + haveRowMVs
	nearestCount := len(cands)
	for i := 0; i < nearestCount; i++ {
		cands[i].Weight += 640
	}
	for _, cand := range temporal {
		if cand.Weight <= 0 {
			continue
		}
		cands = appendOrAccumulateCompoundRefMVCandidate(cands, cand)
	}
	debugTraceCompoundRefStack(parsed, g, "after-nearest", cands, nearestCount, nRows, nCols, haveNewMV, haveRowMVs, haveColMVs)

	if nRows >= 0 || nCols >= 0 {
		addCompoundSpatialCandidate(&cands, 4, t.block(bx4-1, by4-1), ref0, ref1, &dummyNewMV, &haveRowMVs)
	}
	for n := 2; n <= 3; n++ {
		if nRows >= 0 && n > nRows && n <= maxRows {
			y4 := by4 - 2*n + 1
			if y4 >= t.Geometry.Start4Y {
				nRows += t.scanCompoundRow(&cands, ref0, ref1, bx4|1, y4|1, bw4, w4, 1+maxRows-n, ternaryInt(bw4 >= 16, 4, 2), &dummyNewMV, &haveRowMVs)
			}
		}
		if nCols >= 0 && n > nCols && n <= maxCols {
			x4 := (bx4 - n*2 + 1) | 1
			if x4 >= t.Geometry.Start4X {
				nCols += t.scanCompoundCol(&cands, ref0, ref1, x4, by4|1, bh4, h4, 1+maxCols-n, ternaryInt(bh4 >= 16, 4, 2), &dummyNewMV, &haveColMVs)
			}
		}
	}
	debugTraceCompoundRefStack(parsed, g, "after-secondary", cands, nearestCount, nRows, nCols, haveNewMV, haveRowMVs, haveColMVs)

	refMatchCount := haveColMVs + haveRowMVs
	refmvCtx := 0
	newmvCtx := 0
	switch nearestMatch {
	case 0:
		refmvCtx = minInt(2, refMatchCount)
		newmvCtx = boolToInt(refMatchCount > 0)
	case 1:
		refmvCtx = minInt(refMatchCount*3, 4)
		newmvCtx = 3 - haveNewMV
	default:
		refmvCtx = 5
		newmvCtx = 5 - haveNewMV
	}
	compCtx := compoundModeContext(refmvCtx, newmvCtx)

	sortCompoundCandidateSection(cands[:minInt(nearestCount, len(cands))])
	if nearestCount < len(cands) {
		sortCompoundCandidateSection(cands[nearestCount:])
	}
	if appendExtended && parsed != nil && len(cands) < 2 {
		cands = t.appendCompoundExtendedCandidates(cands, seq, parsed, hdr, g, ref0, ref1)
		debugTraceCompoundRefStack(parsed, g, "after-extended", cands, nearestCount, nRows, nCols, haveNewMV, haveRowMVs, haveColMVs)
	}
	for len(cands) < 2 {
		cands = append(cands, CompoundRefMVCandidate{MV: [2]MotionVector{{}, {}}})
	}
	if hdr != nil {
		clampCompoundCandidates(cands, hdr, g)
	}
	debugTraceCompoundRefStack(parsed, g, "final", cands, nearestCount, nRows, nCols, haveNewMV, haveRowMVs, haveColMVs)
	return cands, compCtx
}

func augmentCompoundCandidatesExtended(cands []CompoundRefMVCandidate, t *SpatialMVTile, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) []CompoundRefMVCandidate {
	write := 0
	for _, cand := range cands {
		if cand.Weight <= 0 {
			continue
		}
		cands[write] = cand
		write++
	}
	cands = cands[:write]
	if t != nil && parsed != nil && len(cands) < 2 {
		cands = t.appendCompoundExtendedCandidates(cands, seq, parsed, hdr, g, ref0, ref1)
	}
	for len(cands) < 2 {
		cands = append(cands, CompoundRefMVCandidate{MV: [2]MotionVector{{}, {}}})
	}
	if hdr != nil {
		clampCompoundCandidates(cands, hdr, g)
	}
	return cands
}

func compoundModeContext(refmvCtx, newmvCtx int) int {
	switch refmvCtx >> 1 {
	case 0:
		return minInt(newmvCtx, 1)
	case 1:
		return 1 + minInt(newmvCtx, 3)
	default:
		return clipInt(3+newmvCtx, 4, 7)
	}
}

func compoundRefSignBias(seq av1.SequenceHeader, parsed *ParsedFrame, ref int8) int {
	if parsed == nil || ref < 0 || !seq.EnableOrderHint || int(ref) >= len(parsed.RefFramePOC) {
		return 0
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	if orderHintBits <= 0 {
		return 0
	}
	return boolToInt(compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[ref]), int(parsed.Header.FrameOffset)) > 0)
}

func appendUniqueMV(dst []MotionVector, mv MotionVector, limit int) []MotionVector {
	for _, cur := range dst {
		if cur == mv {
			return dst
		}
	}
	if len(dst) < limit {
		return append(dst, mv)
	}
	return dst
}

func appendLimitedMV(dst []MotionVector, mv MotionVector, limit int) []MotionVector {
	if len(dst) < limit {
		return append(dst, mv)
	}
	return dst
}

func appendUniqueCompoundCandidate(dst []CompoundRefMVCandidate, cand CompoundRefMVCandidate) []CompoundRefMVCandidate {
	for _, cur := range dst {
		if cur.MV == cand.MV {
			return dst
		}
	}
	return append(dst, cand)
}

func appendOrAccumulateCompoundRefMVCandidate(dst []CompoundRefMVCandidate, cand CompoundRefMVCandidate) []CompoundRefMVCandidate {
	for i := range dst {
		if dst[i].MV == cand.MV {
			dst[i].Weight += cand.Weight
			return dst
		}
	}
	if len(dst) >= maxRefMVStackSize {
		return dst
	}
	return append(dst, cand)
}

func compoundAdjustedMV(seq av1.SequenceHeader, parsed *ParsedFrame, candRef int8, targetSign int, mv MotionVector) MotionVector {
	if targetSign^compoundRefSignBias(seq, parsed, candRef) != 0 {
		mv.X = -mv.X
		mv.Y = -mv.Y
	}
	return mv
}

func collectCompoundExtendedCandidate(same, diff *[2][]MotionVector, b *SpatialMVBlock, ref0, ref1 int8, sign0, sign1 int, seq av1.SequenceHeader, parsed *ParsedFrame) {
	if b == nil || !b.Valid || b.Intra {
		return
	}
	for n := 0; n < 2; n++ {
		candRef := b.Ref[n]
		if candRef < 0 {
			break
		}
		candMV := b.MV[n]
		switch candRef {
		case ref0:
			same[0] = appendLimitedMV(same[0], candMV, 2)
			diff[1] = appendLimitedMV(diff[1], compoundAdjustedMV(seq, parsed, candRef, sign1, candMV), 2)
		case ref1:
			same[1] = appendLimitedMV(same[1], candMV, 2)
			diff[0] = appendLimitedMV(diff[0], compoundAdjustedMV(seq, parsed, candRef, sign0, candMV), 2)
		default:
			diff[0] = appendLimitedMV(diff[0], compoundAdjustedMV(seq, parsed, candRef, sign0, candMV), 2)
			diff[1] = appendLimitedMV(diff[1], compoundAdjustedMV(seq, parsed, candRef, sign1, candMV), 2)
		}
	}
}

func chooseCompoundExtendedMV(same, diff []MotionVector, idx int) MotionVector {
	if idx < len(same) {
		return same[idx]
	}
	idx -= len(same)
	if idx < len(diff) {
		return diff[idx]
	}
	return MotionVector{}
}

func (t *SpatialMVTile) appendSingleRefExtendedCandidates(cands []RefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, _ *obu.FrameHeader, g BlockGeometry, ref0 int8) []RefMVCandidate {
	bw4, bh4 := nominalBlockDims4Clamped16(g)
	bx4 := g.Start4X
	by4 := g.Start4Y
	sz4 := minInt(bw4, bh4)
	sign0 := compoundRefSignBias(seq, parsed, ref0)

	if by4 > t.Geometry.Start4Y {
		for x := 0; x < sz4 && len(cands) < 2; {
			b := t.block(bx4+x, by4-1)
			if b == nil {
				x++
				continue
			}
			cands = collectSingleRefExtendedCandidate(cands, b, sign0, seq, parsed)
			step := int(BlockDims(b.Size).W4)
			if step < 1 {
				step = 1
			}
			x += step
		}
	}
	if bx4 > t.Geometry.Start4X {
		for y := 0; y < sz4 && len(cands) < 2; {
			b := t.block(bx4-1, by4+y)
			if b == nil {
				y++
				continue
			}
			cands = collectSingleRefExtendedCandidate(cands, b, sign0, seq, parsed)
			step := int(BlockDims(b.Size).H4)
			if step < 1 {
				step = 1
			}
			y += step
		}
	}
	return cands
}

func (t *SpatialMVTile) appendCompoundExtendedCandidates(cands []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, hdr *obu.FrameHeader, g BlockGeometry, ref0, ref1 int8) []CompoundRefMVCandidate {
	bw4, bh4 := nominalBlockDims4Clamped16(g)
	bx4 := g.Start4X
	by4 := g.Start4Y
	sz4 := minInt(bw4, bh4)
	sign0 := compoundRefSignBias(seq, parsed, ref0)
	sign1 := compoundRefSignBias(seq, parsed, ref1)
	var same [2][]MotionVector
	var diff [2][]MotionVector

	if by4 > t.Geometry.Start4Y {
		for x := 0; x < sz4; {
			b := t.block(bx4+x, by4-1)
			if b == nil {
				x++
				continue
			}
			collectCompoundExtendedCandidate(&same, &diff, b, ref0, ref1, sign0, sign1, seq, parsed)
			step := int(BlockDims(b.Size).W4)
			if step < 1 {
				step = 1
			}
			x += step
		}
	}
	if bx4 > t.Geometry.Start4X {
		for y := 0; y < sz4; {
			b := t.block(bx4-1, by4+y)
			if b == nil {
				y++
				continue
			}
			collectCompoundExtendedCandidate(&same, &diff, b, ref0, ref1, sign0, sign1, seq, parsed)
			step := int(BlockDims(b.Size).H4)
			if step < 1 {
				step = 1
			}
			y += step
		}
	}

	tgmv := [2]MotionVector{}
	if hdr != nil {
		tgmv[0] = approximateGlobalMotionVectorForBlock(hdr, ref0, g)
		tgmv[1] = approximateGlobalMotionVectorForBlock(hdr, ref1, g)
	}
	ext := [2]CompoundRefMVCandidate{
		{Weight: 2},
		{Weight: 2},
	}
	for n := 0; n < 2; n++ {
		srcSame := same[n]
		srcDiff := diff[n]
		m := len(srcSame)
		if m > 2 {
			m = 2
		}
		for i := 0; i < m; i++ {
			ext[i].MV[n] = srcSame[i]
		}
		if m < 2 {
			l := len(srcDiff)
			if l > 0 {
				ext[m].MV[n] = srcDiff[0]
				m++
				if m < 2 && l > 1 {
					ext[m].MV[n] = srcDiff[1]
					m++
				}
			}
		}
		for m < 2 {
			ext[m].MV[n] = tgmv[n]
			m++
		}
	}
	if len(cands) == 1 {
		if cands[0].MV == ext[0].MV {
			cands = append(cands, ext[1])
		} else {
			cands = append(cands, ext[0])
		}
		return cands
	}
	for i := 0; i < len(ext) && len(cands) < 2; i++ {
		cands = append(cands, ext[i])
	}
	return cands
}

func sortCompoundCandidateSection(cands []CompoundRefMVCandidate) {
	sortCompoundCandidatesByWeight(cands)
}

func clampCompoundCandidates(cands []CompoundRefMVCandidate, hdr *obu.FrameHeader, g BlockGeometry) {
	if hdr == nil {
		return
	}
	iw4 := miGridSize4(hdr.Width)
	ih4 := miGridSize4(hdr.Height)
	bw4, bh4 := nominalBlockDims4(g)
	left := -(g.Start4X + bw4 + 4) * 4 * 8
	right := (iw4 - g.Start4X + 4) * 4 * 8
	top := -(g.Start4Y + bh4 + 4) * 4 * 8
	bottom := (ih4 - g.Start4Y + 4) * 4 * 8
	for i := range cands {
		cands[i].MV[0].X = clipMVComp(cands[i].MV[0].X, left, right)
		cands[i].MV[0].Y = clipMVComp(cands[i].MV[0].Y, top, bottom)
		cands[i].MV[1].X = clipMVComp(cands[i].MV[1].X, left, right)
		cands[i].MV[1].Y = clipMVComp(cands[i].MV[1].Y, top, bottom)
	}
}

func clipMVComp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (t *SpatialMVTile) scanRow(hdr *obu.FrameHeader, cands *[]RefMVCandidate, ref0 int8, gmv MotionVector, haveGMV bool, startX4, y4, bw4, w4, maxRows, step int, haveNewMV, haveRowMVs *int) int {
	cand := t.block(startX4, y4)
	if cand == nil {
		return 0
	}
	dims := BlockDims(cand.Size)
	candBW4 := int(dims.W4)
	length := maxIntLocal(step, minInt(bw4, candBW4))
	if bw4 <= candBW4 {
		weight := 2
		if bw4 != 1 {
			weight = maxIntLocal(2, minInt(2*maxRows, int(dims.H4)))
		}
		t.addSpatialCandidate(hdr, cands, length*weight, cand, ref0, gmv, haveGMV, haveNewMV, haveRowMVs)
		return weight >> 1
	}
	for x := 0; ; {
		t.addSpatialCandidate(hdr, cands, length*2, cand, ref0, gmv, haveGMV, haveNewMV, haveRowMVs)
		x += length
		if x >= w4 {
			return 1
		}
		cand = t.block(startX4+x, y4)
		if cand == nil {
			return 1
		}
		candBW4 = int(BlockDims(cand.Size).W4)
		length = maxIntLocal(step, candBW4)
	}
}

func (t *SpatialMVTile) scanCompoundRow(cands *[]CompoundRefMVCandidate, ref0, ref1 int8, startX4, y4, bw4, w4, maxRows, step int, haveNewMV, haveRowMVs *int) int {
	cand := t.block(startX4, y4)
	if cand == nil {
		return 0
	}
	dims := BlockDims(cand.Size)
	candBW4 := int(dims.W4)
	length := maxIntLocal(step, minInt(bw4, candBW4))
	if bw4 <= candBW4 {
		weight := 2
		if bw4 != 1 {
			weight = maxIntLocal(2, minInt(2*maxRows, int(dims.H4)))
		}
		addCompoundSpatialCandidate(cands, length*weight, cand, ref0, ref1, haveNewMV, haveRowMVs)
		return weight >> 1
	}
	for x := 0; ; {
		addCompoundSpatialCandidate(cands, length*2, cand, ref0, ref1, haveNewMV, haveRowMVs)
		x += length
		if x >= w4 {
			return 1
		}
		cand = t.block(startX4+x, y4)
		if cand == nil {
			return 1
		}
		candBW4 = int(BlockDims(cand.Size).W4)
		length = maxIntLocal(step, candBW4)
	}
}

func (t *SpatialMVTile) scanCol(hdr *obu.FrameHeader, cands *[]RefMVCandidate, ref0 int8, gmv MotionVector, haveGMV bool, x4, startY4, bh4, h4, maxCols, step int, haveNewMV, haveColMVs *int) int {
	cand := t.block(x4, startY4)
	if cand == nil {
		return 0
	}
	dims := BlockDims(cand.Size)
	candBH4 := int(dims.H4)
	length := maxIntLocal(step, minInt(bh4, candBH4))
	if bh4 <= candBH4 {
		weight := 2
		if bh4 != 1 {
			weight = maxIntLocal(2, minInt(2*maxCols, int(dims.W4)))
		}
		t.addSpatialCandidate(hdr, cands, length*weight, cand, ref0, gmv, haveGMV, haveNewMV, haveColMVs)
		return weight >> 1
	}
	for y := 0; ; {
		t.addSpatialCandidate(hdr, cands, length*2, cand, ref0, gmv, haveGMV, haveNewMV, haveColMVs)
		y += length
		if y >= h4 {
			return 1
		}
		cand = t.block(x4, startY4+y)
		if cand == nil {
			return 1
		}
		candBH4 = int(BlockDims(cand.Size).H4)
		length = maxIntLocal(step, candBH4)
	}
}

func (t *SpatialMVTile) scanCompoundCol(cands *[]CompoundRefMVCandidate, ref0, ref1 int8, x4, startY4, bh4, h4, maxCols, step int, haveNewMV, haveColMVs *int) int {
	cand := t.block(x4, startY4)
	if cand == nil {
		return 0
	}
	dims := BlockDims(cand.Size)
	candBH4 := int(dims.H4)
	length := maxIntLocal(step, minInt(bh4, candBH4))
	if bh4 <= candBH4 {
		weight := 2
		if bh4 != 1 {
			weight = maxIntLocal(2, minInt(2*maxCols, int(dims.W4)))
		}
		addCompoundSpatialCandidate(cands, length*weight, cand, ref0, ref1, haveNewMV, haveColMVs)
		return weight >> 1
	}
	for y := 0; ; {
		addCompoundSpatialCandidate(cands, length*2, cand, ref0, ref1, haveNewMV, haveColMVs)
		y += length
		if y >= h4 {
			return 1
		}
		cand = t.block(x4, startY4+y)
		if cand == nil {
			return 1
		}
		candBH4 = int(BlockDims(cand.Size).H4)
		length = maxIntLocal(step, candBH4)
	}
}

func (t *SpatialMVTile) addSpatialCandidate(hdr *obu.FrameHeader, cands *[]RefMVCandidate, weight int, b *SpatialMVBlock, ref0 int8, gmv MotionVector, haveGMV bool, haveNewMV, haveRefMatch *int) {
	if b == nil || !b.Valid || b.Intra {
		return
	}
	for i := 0; i < 2; i++ {
		if b.Ref[i] != ref0 {
			continue
		}
		*haveRefMatch = 1
		if b.Flags&refMVFlagNew != 0 {
			*haveNewMV = 1
		}
		mv := b.MV[i]
		if b.Flags&refMVFlagGlobal != 0 && haveGMV {
			mv = gmv
		}
		for idx := range *cands {
			if (*cands)[idx].MV == mv {
				(*cands)[idx].Weight += weight
				return
			}
		}
		if len(*cands) >= maxRefMVStackSize {
			return
		}
		*cands = append(*cands, RefMVCandidate{
			MV:     mv,
			Weight: weight,
			Flags:  b.Flags,
		})
		return
	}
}

func SelectRefMVCandidate(cands []RefMVCandidate, drl uint8) (MotionVector, error) {
	if len(cands) == 0 {
		return MotionVector{}, fmt.Errorf("decoder: no refmv candidates")
	}
	idx := int(drl)
	if idx >= len(cands) {
		idx = len(cands) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return cands[idx].MV, nil
}

func SelectCompoundRefMVCandidate(cands []CompoundRefMVCandidate, drl uint8) (MotionVector, MotionVector, error) {
	if len(cands) == 0 {
		return MotionVector{}, MotionVector{}, fmt.Errorf("decoder: no compound refmv candidates")
	}
	idx := int(drl)
	if idx >= len(cands) {
		idx = len(cands) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return cands[idx].MV[0], cands[idx].MV[1], nil
}

func matchCompoundRefBlockByRef(b *SpatialMVBlock, ref0, ref1 int8) (MotionVector, MotionVector, bool) {
	if b == nil || !b.Valid || b.Intra || b.Ref[1] < 0 {
		return MotionVector{}, MotionVector{}, false
	}
	if b.Ref[0] == ref0 && b.Ref[1] == ref1 {
		return b.MV[0], b.MV[1], true
	}
	if b.Ref[0] == ref1 && b.Ref[1] == ref0 {
		return b.MV[1], b.MV[0], true
	}
	return MotionVector{}, MotionVector{}, false
}

func matchCompoundRefBlockBySlot(b *SpatialMVBlock, slot0, slot1 int8) (MotionVector, MotionVector, bool) {
	if b == nil || !b.Valid || b.Intra || b.Ref[1] < 0 {
		return MotionVector{}, MotionVector{}, false
	}
	if b.Slot[0] == slot0 && b.Slot[1] == slot1 {
		return b.MV[0], b.MV[1], true
	}
	if b.Slot[0] == slot1 && b.Slot[1] == slot0 {
		return b.MV[1], b.MV[0], true
	}
	return MotionVector{}, MotionVector{}, false
}

func addCompoundSpatialCandidate(cands *[]CompoundRefMVCandidate, weight int, b *SpatialMVBlock, ref0, ref1 int8, haveNewMV, haveRefMatch *int) bool {
	mv0, mv1, ok := matchCompoundRefBlockByRef(b, ref0, ref1)
	if !ok {
		return false
	}
	*haveRefMatch = 1
	if b.Flags&refMVFlagNew != 0 {
		*haveNewMV = 1
	}
	for i := range *cands {
		if (*cands)[i].MV[0] == mv0 && (*cands)[i].MV[1] == mv1 {
			(*cands)[i].Weight += weight
			return true
		}
	}
	if len(*cands) >= maxRefMVStackSize {
		return true
	}
	*cands = append(*cands, CompoundRefMVCandidate{
		MV:     [2]MotionVector{mv0, mv1},
		Weight: weight,
		Flags:  b.Flags,
	})
	return true
}

func ternaryInt(cond bool, yes, no int) int {
	if cond {
		return yes
	}
	return no
}
