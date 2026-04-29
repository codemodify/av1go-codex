package decoder

import (
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type TemporalMVBlock struct {
	MV  MotionVector
	Ref int8
}

type TemporalMVMap struct {
	Width8      int
	Height8     int
	Stride8     int
	Blocks      []TemporalMVBlock
	FramePOC    uint8
	RefFramePOC [7]uint8
}

type temporalMFMVRef struct {
	ref     int
	ref2cur int
	ref2ref [7]int
}

func newTemporalMVMap(width, height int) *TemporalMVMap {
	width8 := (width + 7) >> 3
	height8 := (height + 7) >> 3
	stride8 := ((width + 127) &^ 127) >> 3
	if stride8 < width8 {
		stride8 = width8
	}
	return &TemporalMVMap{
		Width8:  width8,
		Height8: height8,
		Stride8: stride8,
		Blocks:  make([]TemporalMVBlock, stride8*height8),
	}
}

func (m *TemporalMVMap) block8(x8, y8 int) TemporalMVBlock {
	if m == nil || x8 < 0 || y8 < 0 || x8 >= m.Width8 || y8 >= m.Height8 {
		return TemporalMVBlock{}
	}
	return m.Blocks[y8*m.Stride8+x8]
}

func (m *TemporalMVMap) setBlock8(x8, y8 int, b TemporalMVBlock) {
	if m == nil || x8 < 0 || y8 < 0 || x8 >= m.Width8 || y8 >= m.Height8 {
		return
	}
	m.Blocks[y8*m.Stride8+x8] = b
}

func buildTemporalMVMap(seq av1.SequenceHeader, parsed *ParsedFrame, field *SpatialMVTile) *TemporalMVMap {
	if parsed == nil || field == nil {
		return nil
	}
	out := newTemporalMVMap(parsed.Header.Width, parsed.Header.Height)
	out.FramePOC = parsed.Header.FrameOffset
	out.RefFramePOC = parsed.RefFramePOC
	mfmvSign := [7]bool{}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	for ref := 0; ref < len(mfmvSign); ref++ {
		if orderHintBits > 0 && int(ref) < len(parsed.RefFramePOC) {
			mfmvSign[ref] = compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[ref]), int(parsed.Header.FrameOffset)) < 0
		}
	}
	for y8 := 0; y8 < out.Height8; y8++ {
		x8 := 0
		for x8 < out.Width8 {
			cand := field.block(x8*2+1, y8*2+1)
			bw8 := 1
			block := TemporalMVBlock{}
			if cand != nil && cand.Valid && !cand.Intra {
				dims := BlockDims(cand.Size)
				bw8 = maxIntLocal(1, (int(dims.W4)+1)>>1)
				if cand.Ref[1] >= 0 && int(cand.Ref[1]) < len(mfmvSign) && mfmvSign[cand.Ref[1]] && temporalMVSaveable(cand.MV[1]) {
					block = TemporalMVBlock{MV: cand.MV[1], Ref: cand.Ref[1] + 1}
				} else if cand.Ref[0] >= 0 && int(cand.Ref[0]) < len(mfmvSign) && mfmvSign[cand.Ref[0]] && temporalMVSaveable(cand.MV[0]) {
					block = TemporalMVBlock{MV: cand.MV[0], Ref: cand.Ref[0] + 1}
				}
			}
			for n := 0; n < bw8 && x8 < out.Width8; n++ {
				debugTraceTemporalSave(parsed, x8, y8, cand, block)
				out.setBlock8(x8, y8, block)
				x8++
			}
		}
	}
	return out
}

func temporalMVSaveable(mv MotionVector) bool {
	return absInt(mv.X)|absInt(mv.Y) < 4096
}

func projectTemporalMVMap(seq av1.SequenceHeader, parsed *ParsedFrame, refs [7]*TemporalMVMap) *TemporalMVMap {
	if parsed == nil || !parsed.Header.UseRefFrameMvs || !seq.EnableOrderHint {
		return nil
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	if orderHintBits <= 0 {
		return nil
	}
	out := newTemporalMVMap(parsed.Header.Width, parsed.Header.Height)
	var selected [3]temporalMFMVRef
	nSelected := 0
	addRef := func(ref int) {
		if nSelected >= len(selected) || ref < 0 || ref >= len(refs) || refs[ref] == nil {
			return
		}
		rpoc := int(parsed.RefFramePOC[ref])
		diff1 := compoundPOCDiff(orderHintBits, rpoc, int(parsed.Header.FrameOffset))
		if absInt(diff1) > 31 {
			return
		}
		ref2cur := diff1
		if ref < 4 {
			ref2cur = -diff1
		}
		item := temporalMFMVRef{ref: ref, ref2cur: ref2cur}
		for m := 0; m < len(item.ref2ref); m++ {
			diff2 := compoundPOCDiff(orderHintBits, rpoc, int(refs[ref].RefFramePOC[m]))
			if diff2 < 0 || diff2 > 31 {
				continue
			}
			item.ref2ref[m] = diff2
		}
		selected[nSelected] = item
		nSelected++
	}
	total := 2
	if refs[0] != nil && refs[0].RefFramePOC[6] != parsed.RefFramePOC[3] {
		addRef(0)
		total = 3
	}
	if refs[4] != nil && compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[4]), int(parsed.Header.FrameOffset)) > 0 {
		addRef(4)
	}
	if refs[5] != nil && compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[5]), int(parsed.Header.FrameOffset)) > 0 {
		addRef(5)
	}
	if nSelected < total && refs[6] != nil && compoundPOCDiff(orderHintBits, int(parsed.RefFramePOC[6]), int(parsed.Header.FrameOffset)) > 0 {
		addRef(6)
	}
	if nSelected < total && refs[1] != nil {
		addRef(1)
	}
	if nSelected == 0 {
		return nil
	}

	step8 := SuperBlockSizePx(seq) >> 3
	if step8 <= 0 || step8 > 16 {
		step8 = 16
	}
	for rowStart8 := 0; rowStart8 < out.Height8; rowStart8 += step8 {
		rowEnd8 := minInt(rowStart8+step8, out.Height8)
		projectTemporalMVMapWindow(parsed, refs, out, selected[:nSelected], 0, out.Width8, rowStart8, rowEnd8)
	}
	return out
}

func projectTemporalMVMapWindow(parsed *ParsedFrame, refs [7]*TemporalMVMap, out *TemporalMVMap, selected []temporalMFMVRef, colStart8, colEnd8, rowStart8, rowEnd8 int) {
	if out == nil || rowEnd8 <= rowStart8 || colEnd8 <= colStart8 {
		return
	}
	colStart8 = maxIntLocal(0, colStart8)
	colEnd8 = minInt(out.Width8, colEnd8)
	rowStart8 = maxIntLocal(0, rowStart8)
	rowEnd8 = minInt(out.Height8, rowEnd8)
	for i := 0; i < len(selected); i++ {
		src := refs[selected[i].ref]
		refSign := selected[i].ref - 4
		if src == nil {
			continue
		}
		srcRowEnd8 := minInt(rowEnd8, src.Height8)
		srcColStart8 := maxIntLocal(colStart8-8, 0)
		srcColEnd8 := minInt(colEnd8+8, src.Width8)
		for y := rowStart8; y < srcRowEnd8; y++ {
			ySBAlign := y &^ 7
			yProjStart := maxIntLocal(ySBAlign, rowStart8)
			yProjEnd := minInt(ySBAlign+8, rowEnd8)
			for x := srcColStart8; x < srcColEnd8; x++ {
				rb := src.block8(x, y)
				if rb.Ref <= 0 || int(rb.Ref) > len(selected[i].ref2ref) {
					continue
				}
				ref2ref := selected[i].ref2ref[rb.Ref-1]
				if ref2ref == 0 {
					continue
				}
				baseMV := rb.MV
				offset := projectTemporalMV(baseMV, selected[i].ref2cur, ref2ref)
				posX := x + applySign(absInt(offset.X)>>6, offset.X^refSign)
				posY := y + applySign(absInt(offset.Y)>>6, offset.Y^refSign)
				for {
					xSBAlign := x &^ 7
					if posY >= yProjStart && posY < yProjEnd &&
						posX >= maxIntLocal(xSBAlign-8, colStart8) &&
						posX < minInt(xSBAlign+16, colEnd8) {
						debugTraceTemporalProjection(parsed, selected[i].ref, selected[i].ref2cur, ref2ref, x, y, posX, posY, rb)
						out.setBlock8(posX, posY, TemporalMVBlock{MV: baseMV, Ref: int8(ref2ref)})
					}
					if x+1 >= srcColEnd8 {
						break
					}
					next := src.block8(x+1, y)
					if next.Ref != rb.Ref || next.MV != baseMV {
						break
					}
					x++
					posX++
				}
			}
		}
	}
}

func (m *TemporalMVMap) findCandidatesProjectedInto(dst []RefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, ref0 int8, hdr *obu.FrameHeader, g BlockGeometry) []RefMVCandidate {
	if m == nil || parsed == nil || hdr == nil || ref0 < 0 || int(ref0) >= len(parsed.RefFramePOC) {
		return dst[:0]
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	if orderHintBits <= 0 {
		return dst[:0]
	}
	num := compoundPOCDiff(orderHintBits, int(hdr.FrameOffset), int(parsed.RefFramePOC[ref0]))
	num = clipInt(num, -31, 31)
	cands := dst[:0]
	add := func(x8, y8 int) {
		rb := m.block8(x8, y8)
		if rb.Ref <= 0 {
			return
		}
		mv := projectTemporalMV(rb.MV, num, int(rb.Ref))
		mv = fixMVPrecision(hdr, mv)
		cands = appendOrAccumulateRefMVCandidate(cands, RefMVCandidate{
			MV:     mv,
			Weight: 2,
		})
	}
	m.forEachTemporalCandidatePoint(g, add)
	return cands
}

func (m *TemporalMVMap) findCompoundCandidatesProjectedInto(dst []CompoundRefMVCandidate, seq av1.SequenceHeader, parsed *ParsedFrame, ref0, ref1 int8, hdr *obu.FrameHeader, g BlockGeometry) []CompoundRefMVCandidate {
	if m == nil || parsed == nil || hdr == nil || ref0 < 0 || ref1 < 0 || int(ref0) >= len(parsed.RefFramePOC) || int(ref1) >= len(parsed.RefFramePOC) {
		return dst[:0]
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	if orderHintBits <= 0 {
		return dst[:0]
	}
	num0 := compoundPOCDiff(orderHintBits, int(hdr.FrameOffset), int(parsed.RefFramePOC[ref0]))
	num1 := compoundPOCDiff(orderHintBits, int(hdr.FrameOffset), int(parsed.RefFramePOC[ref1]))
	num0 = clipInt(num0, -31, 31)
	num1 = clipInt(num1, -31, 31)
	cands := dst[:0]
	add := func(x8, y8 int) {
		rb := m.block8(x8, y8)
		if rb.Ref <= 0 {
			debugTraceTemporalCompoundCandidate(parsed, g, x8, y8, rb, num0, num1, MotionVector{}, MotionVector{}, false)
			return
		}
		mv0 := fixMVPrecision(hdr, projectTemporalMV(rb.MV, num0, int(rb.Ref)))
		mv1 := fixMVPrecision(hdr, projectTemporalMV(rb.MV, num1, int(rb.Ref)))
		debugTraceTemporalCompoundCandidate(parsed, g, x8, y8, rb, num0, num1, mv0, mv1, true)
		cand := CompoundRefMVCandidate{
			MV:     [2]MotionVector{mv0, mv1},
			Weight: 2,
		}
		for i := range cands {
			if cands[i].MV == cand.MV {
				cands[i].Weight += cand.Weight
				return
			}
		}
		cands = append(cands, cand)
	}
	m.forEachTemporalCandidatePoint(g, add)
	return cands
}

func (m *TemporalMVMap) forEachTemporalCandidatePoint(g BlockGeometry, visit func(x8, y8 int)) {
	if m == nil || visit == nil {
		return
	}
	bw4, bh4 := nominalBlockDims4Clamped16(g)
	w4 := minInt(bw4, m.Width8*2-g.Start4X)
	h4 := minInt(bh4, m.Height8*2-g.Start4Y)
	if w4 <= 0 || h4 <= 0 {
		return
	}
	bx8 := g.Start4X >> 1
	by8 := g.Start4Y >> 1
	w8 := minInt((w4+1)>>1, 8)
	h8 := minInt((h4+1)>>1, 8)
	stepH := ternaryInt(bw4 >= 16, 2, 1)
	stepV := ternaryInt(bh4 >= 16, 2, 1)
	for y := 0; y < h8; y += stepV {
		for x := 0; x < w8; x += stepH {
			visit(bx8+x, by8+y)
		}
	}
	if minInt(bw4, bh4) < 2 || maxIntLocal(bw4, bh4) >= 16 {
		return
	}
	bw8 := bw4 >> 1
	bh8 := bh4 >> 1
	colStart8 := bx8 &^ 7
	colEnd8 := minInt(m.Width8, (bx8&^7)+8)
	rowEnd8 := minInt(m.Height8, (by8&^7)+8)
	hasBottom := by8+bh8 < rowEnd8
	if hasBottom && bx8-1 >= colStart8 {
		visit(bx8-1, by8+bh8)
	}
	if bx8+bw8 < colEnd8 {
		if hasBottom {
			visit(bx8+bw8, by8+bh8)
		}
		if by8+bh8-1 < rowEnd8 {
			visit(bx8+bw8, by8+bh8-1)
		}
	}
}

func (m *TemporalMVMap) projectedGlobalMVContext(seq av1.SequenceHeader, parsed *ParsedFrame, ref0 int8, hdr *obu.FrameHeader, g BlockGeometry) (int, bool) {
	if m == nil || parsed == nil || hdr == nil || ref0 < 0 || int(ref0) >= len(parsed.RefFramePOC) {
		return 0, false
	}
	orderHintBits := int(seq.OrderHintBitsMinus1) + 1
	if orderHintBits <= 0 {
		return 0, false
	}
	rb := m.block8(g.Start4X>>1, g.Start4Y>>1)
	if rb.Ref <= 0 {
		return 0, false
	}
	num := compoundPOCDiff(orderHintBits, int(hdr.FrameOffset), int(parsed.RefFramePOC[ref0]))
	num = clipInt(num, -31, 31)
	mv := projectTemporalMV(rb.MV, num, int(rb.Ref))
	mv = fixMVPrecision(hdr, mv)
	gmv := approximateGlobalMotionVectorForBlock(hdr, ref0, g)
	return boolToInt(absInt(mv.X-gmv.X)|absInt(mv.Y-gmv.Y) >= 16), true
}

func applySign(v, s int) int {
	if s < 0 {
		return -v
	}
	return v
}
