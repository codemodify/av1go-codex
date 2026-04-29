//go:build av1debug

package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

var debugInterExhaustPrinted bool

func debugTraceInterLeaf(parsed *ParsedFrame, g BlockGeometry, syntax InterBlockSyntax) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x >= 0 && (x < g.Start4X*4 || x >= g.End4X*4) {
		return
	}
	if y >= 0 && (y < g.Start4Y*4 || y >= g.End4Y*4) {
		return
	}
	intraMode := "<nil>"
	uvMode := "<nil>"
	tx := TxfmSize(0)
	uvtx := TxfmSize(0)
	if syntax.IntraBlk != nil {
		intraMode = fmt.Sprintf("%d", syntax.IntraBlk.YMode)
		uvMode = fmt.Sprintf("%d", syntax.IntraBlk.UVMode)
		tx = syntax.IntraBlk.TX
		uvtx = syntax.IntraBlk.UVTX
	}
	fmt.Printf("DEBUG_INTER_TRACE parsed=%d show=%v offset=%d geom=(%d,%d)-(%d,%d) size=%d seg=%d skipMode=%v skip=%v intra=%v ref=(%d,%d) mode=(%d,%d) drl=%d mv=(%d,%d) mv1=(%d,%d) motion=%d filter=%v comp=%d maskSign=%v wedge=%d ii=%d iiMode=%d yMode=%s uvMode=%s tx=%d uvtx=%d\n",
		parsed.Index, parsed.Header.ShowFrame, parsed.Header.FrameOffset,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, g.Size,
		syntax.Segment, syntax.SkipMode, syntax.Skip, syntax.Intra,
		syntax.Ref0, syntax.Ref1, syntax.Mode, syntax.Mode1, syntax.DRLIndex,
		syntax.MV.X, syntax.MV.Y, syntax.MV1.X, syntax.MV1.Y,
		syntax.Motion, syntax.Filter, syntax.CompType, syntax.MaskSign, syntax.WedgeIdx,
		syntax.InterIntraType, syntax.InterIntraMode,
		intraMode, uvMode, tx, uvtx,
	)
}

func debugTraceInterTransformUnits(parsed *ParsedFrame, g BlockGeometry, syntax InterBlockSyntax, units []TransformUnit) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_INTER_TX parsed=%d geom=(%d,%d)-(%d,%d) skip=%v units=%d",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, syntax.Skip, len(units))
	for _, tu := range units {
		fmt.Printf(" tu=(%d,%d)-(%d,%d)/tx%d",
			tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.Global.End4X*4, tu.Global.End4Y*4, tu.TX)
	}
	fmt.Println()
}

func debugTraceInterTXSplit(local, global BlockGeometry, tx TxfmSize, depth, cat, ctx int, aboveTx, leftTx int8, split uint32, preState, postState [4]int, preCDF, postCDF [2]uint16) {
	if strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x >= 0 && (x < global.Start4X*4 || x >= global.End4X*4) {
		return
	}
	if y >= 0 && (y < global.Start4Y*4 || y >= global.End4Y*4) {
		return
	}
	fmt.Printf("DEBUG_INTER_TX_SPLIT geom=(%d,%d)-(%d,%d) local4=(%d,%d)-(%d,%d) tx=%d depth=%d cat=%d ctx=%d aboveTx=%d leftTx=%d split=%d pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
		global.Start4X*4, global.Start4Y*4, global.End4X*4, global.End4Y*4,
		local.Start4X, local.Start4Y, local.End4X, local.End4Y,
		tx, depth, cat, ctx, aboveTx, leftTx, split,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3],
		preCDF, postCDF,
	)
}

func debugTraceInterPartition(parsed *ParsedFrame, global, local BlockGeometry, part BlockPartition, above, left *BlockContext, preState, postState [4]int, preCDF, postCDF [10]uint16) {
	if !debugInterPartTraceMatches(parsed, global) {
		return
	}
	dims := BlockDims(global.Size)
	halfW4 := int(dims.W4) >> 1
	halfH4 := int(dims.H4) >> 1
	haveHSplit := global.Width4() > halfW4
	haveVSplit := global.Height4() > halfH4
	bx8 := local.Start4X >> 1
	by8 := local.Start4Y >> 1
	ctx := -1
	var abovePart, leftPart uint8
	if above != nil && left != nil && bx8 >= 0 && bx8 < len(above.Partition) && by8 >= 0 && by8 < len(left.Partition) {
		ctx = GetPartitionCtx(above, left, global.Level, by8, bx8)
		abovePart = above.Partition[bx8]
		leftPart = left.Partition[by8]
	}
	fmt.Printf("DEBUG_INTER_PART parsed=%d show=%v offset=%d geom=(%d,%d)-(%d,%d) bybx4=(%d,%d) local4=(%d,%d)-(%d,%d) level=%d size=%d part=%d ctx=%d abovePart=%d leftPart=%d haveH=%v haveV=%v pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
		parsed.Index, parsed.Header.ShowFrame, parsed.Header.FrameOffset,
		global.Start4X*4, global.Start4Y*4, global.End4X*4, global.End4Y*4,
		global.Start4Y, global.Start4X,
		local.Start4X, local.Start4Y, local.End4X, local.End4Y,
		global.Level, global.Size, part, ctx, abovePart, leftPart, haveHSplit, haveVSplit,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3],
		preCDF, postCDF,
	)
}

type debugMSACState interface {
	DebugState() (uint32, uint32, int, int)
}

func debugTraceInterMSAC(parsed *ParsedFrame, g BlockGeometry, stage string, state debugMSACState) {
	if !debugInterTraceMatches(parsed, g) || state == nil {
		return
	}
	cur, rng, cnt, pos := state.DebugState()
	fmt.Printf("DEBUG_INTER_MSAC parsed=%d geom=(%d,%d)-(%d,%d) stage=%s cur=%d r=%d cnt=%d pos=%d\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, stage, cur, rng, cnt, pos)
}

func debugTraceInterSkipPrelude(parsed *ParsedFrame, g, local BlockGeometry, haveTop, haveLeft bool, segID uint8, skipModeEligible bool, skipModeCtx, skipCtx int, aboveSkipMode, leftSkipMode, aboveSkip, leftSkip uint8, syntax InterBlockSyntax, preState, postState [4]int, preSkipModeCDF, postSkipModeCDF, preSkipCDF, postSkipCDF [2]uint16) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_INTER_SKIP_PRELUDE parsed=%d geom=(%d,%d)-(%d,%d) local4=(%d,%d)-(%d,%d) haveTop=%v haveLeft=%v seg=%d smEligible=%v smCtx=%d skipCtx=%d aboveSM=%d leftSM=%d aboveSkip=%d leftSkip=%d skipMode=%v skip=%v pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preSMCDF=%v postSMCDF=%v preSkipCDF=%v postSkipCDF=%v\n",
		parsed.Index,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		local.Start4X, local.Start4Y, local.End4X, local.End4Y,
		haveTop, haveLeft, segID, skipModeEligible, skipModeCtx, skipCtx,
		aboveSkipMode, leftSkipMode, aboveSkip, leftSkip, syntax.SkipMode, syntax.Skip,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3],
		preSkipModeCDF, postSkipModeCDF, preSkipCDF, postSkipCDF,
	)
}

func debugTraceInterIntraFlag(parsed *ParsedFrame, g, local BlockGeometry, haveTop, haveLeft bool, segID uint8, skipMode bool, intraCtx int, segRef int8, segGlobalMV bool, intra bool, preState, postState [4]int, preCDF, postCDF [2]uint16) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_INTER_INTRA_FLAG parsed=%d geom=(%d,%d)-(%d,%d) local4=(%d,%d)-(%d,%d) haveTop=%v haveLeft=%v seg=%d skipMode=%v segRef=%d segGlobalMV=%v intraCtx=%d intra=%v pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
		parsed.Index,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		local.Start4X, local.Start4Y, local.End4X, local.End4Y,
		haveTop, haveLeft, segID, skipMode, segRef, segGlobalMV, intraCtx, intra,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3],
		preCDF, postCDF,
	)
}

func debugTraceInterResidualStep(parsed *ParsedFrame, block BlockGeometry, plane int, tu BlockGeometry, stage string, state debugMSACState) {
	if strings.TrimSpace(os.Getenv("DEBUG_INTER_RESIDUAL_TRACE")) == "" || !debugInterTraceMatches(parsed, block) || state == nil {
		return
	}
	cur, rng, cnt, pos := state.DebugState()
	fmt.Printf("DEBUG_INTER_RESIDUAL parsed=%d block=(%d,%d)-(%d,%d) plane=%d tu=(%d,%d)-(%d,%d) stage=%s cur=%d r=%d cnt=%d pos=%d\n",
		parsed.Index,
		block.Start4X*4, block.Start4Y*4, block.End4X*4, block.End4Y*4,
		plane, tu.Start4X*4, tu.Start4Y*4, tu.End4X*4, tu.End4Y*4,
		stage, cur, rng, cnt, pos)
}

func debugTraceInterEntropyExhausted(parsed *ParsedFrame, g BlockGeometry, stage string, state debugMSACState, tileLen int) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_EXHAUST_TRACE")) == "" || state == nil || debugInterExhaustPrinted {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	_, rng, cnt, pos := state.DebugState()
	if cnt >= 0 && (tileLen <= 0 || pos < tileLen) {
		return
	}
	debugInterExhaustPrinted = true
	fmt.Printf("DEBUG_INTER_EXHAUST parsed=%d show=%v offset=%d geom=(%d,%d)-(%d,%d) stage=%s r=%d cnt=%d pos=%d tileLen=%d\n",
		parsed.Index, parsed.Header.ShowFrame, parsed.Header.FrameOffset,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		stage, rng, cnt, pos, tileLen)
}

func debugTraceInterMVCandidates(parsed *ParsedFrame, g BlockGeometry, ref0 int8, mvCtx int, cands []RefMVCandidate, temporal []RefMVCandidate) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_INTER_MV_CANDS parsed=%d geom=(%d,%d)-(%d,%d) ref0=%d mvCtx=%d n=%d active=%d",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, ref0, mvCtx, len(cands), activeRefMVCandidateCount(cands))
	for i, cand := range cands {
		fmt.Printf(" cand%d=(x:%d,y:%d,w:%d,flags:%d)", i, cand.MV.X, cand.MV.Y, cand.Weight, cand.Flags)
	}
	if len(temporal) > 0 {
		fmt.Printf(" temporal=%d", len(temporal))
		for i, cand := range temporal {
			fmt.Printf(" tcand%d=(x:%d,y:%d,w:%d,flags:%d)", i, cand.MV.X, cand.MV.Y, cand.Weight, cand.Flags)
		}
	}
	fmt.Println()
}

func debugTraceSingleRefStack(parsed *ParsedFrame, g BlockGeometry, stage string, ref0 int8, mvCtx int, cands []RefMVCandidate) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_SINGLE_REF_STACK parsed=%d geom=(%d,%d)-(%d,%d) stage=%s ref0=%d mvCtx=%d n=%d",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, stage, ref0, mvCtx, len(cands))
	for i, cand := range cands {
		fmt.Printf(" cand%d=(x:%d,y:%d,w:%d,flags:%d)", i, cand.MV.X, cand.MV.Y, cand.Weight, cand.Flags)
	}
	fmt.Println()
}

func debugTraceInterMVNeighbors(parsed *ParsedFrame, g BlockGeometry, mvTile *SpatialMVTile) {
	if !debugInterTraceMatches(parsed, g) || mvTile == nil {
		return
	}
	points := []struct {
		name string
		x4   int
		y4   int
	}{
		{"top0", g.Start4X, g.Start4Y - 1},
		{"top1", g.Start4X + 1, g.Start4Y - 1},
		{"topR", g.End4X, g.Start4Y - 1},
		{"left0", g.Start4X - 1, g.Start4Y},
		{"tl", g.Start4X - 1, g.Start4Y - 1},
	}
	fmt.Printf("DEBUG_INTER_MV_NEIGH parsed=%d geom=(%d,%d)-(%d,%d)",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4)
	for _, pt := range points {
		b := mvTile.block(pt.x4, pt.y4)
		if b == nil {
			fmt.Printf(" %s=nil", pt.name)
			continue
		}
		fmt.Printf(" %s=(valid:%v,intra:%v,ref:%v,slot:%v,mv0:%+v,mv1:%+v,size:%d,flags:%d,rf:%v)",
			pt.name, b.Valid, b.Intra, b.Ref, b.Slot, b.MV[0], b.MV[1], b.Size, b.Flags, b.RefFlags)
	}
	fmt.Println()
}

func debugTraceInterModeInputs(parsed *ParsedFrame, g BlockGeometry, mvCtx, nMVs int, weights []int, cdf *ModeCDF) {
	if !debugInterTraceMatches(parsed, g) || cdf == nil {
		return
	}
	newCtx := minInt(mvCtx&7, len(cdf.NewMVMode)-1)
	globalCtx := minInt((mvCtx>>3)&1, len(cdf.GlobalMVMode)-1)
	refCtx := minInt((mvCtx>>4)&15, len(cdf.RefMVMode)-1)
	drlCtx0 := minInt(GetDRLContext(weights, 0), len(cdf.DRLBit)-1)
	drlCtx1 := minInt(GetDRLContext(weights, 1), len(cdf.DRLBit)-1)
	drlCtx2 := minInt(GetDRLContext(weights, 2), len(cdf.DRLBit)-1)
	fmt.Printf("DEBUG_INTER_MODE_INPUT parsed=%d geom=(%d,%d)-(%d,%d) mvCtx=%d newCtx=%d globalCtx=%d refCtx=%d nMVs=%d weights=%v newCDF=%v globalCDF=%v refCDF=%v drlCtx=[%d %d %d] drlCDF=[%v %v %v]\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		mvCtx, newCtx, globalCtx, refCtx, nMVs, append([]int(nil), weights...),
		cdf.NewMVMode[newCtx], cdf.GlobalMVMode[globalCtx], cdf.RefMVMode[refCtx],
		drlCtx0, drlCtx1, drlCtx2, cdf.DRLBit[drlCtx0], cdf.DRLBit[drlCtx1], cdf.DRLBit[drlCtx2])
}

func debugTraceCompoundModeInputs(parsed *ParsedFrame, g BlockGeometry, mvCtx, nMVs int, hasPairCands bool, cands0, cands1 []RefMVCandidate, compCands []CompoundRefMVCandidate, weights []int, cdf *ModeCDF) {
	if !debugInterTraceMatches(parsed, g) || cdf == nil {
		return
	}
	ctx := minInt(mvCtx, len(cdf.CompInterMode)-1)
	fmt.Printf("DEBUG_COMP_MODE_INPUT parsed=%d geom=(%d,%d)-(%d,%d) mvCtx=%d ctx=%d nMVs=%d hasPair=%v weights=%v compCDF=%v",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		mvCtx, ctx, nMVs, hasPairCands, append([]int(nil), weights...), cdf.CompInterMode[ctx])
	for i, cand := range cands0 {
		fmt.Printf(" c0_%d=(x:%d,y:%d,w:%d,flags:%d)", i, cand.MV.X, cand.MV.Y, cand.Weight, cand.Flags)
	}
	for i, cand := range cands1 {
		fmt.Printf(" c1_%d=(x:%d,y:%d,w:%d,flags:%d)", i, cand.MV.X, cand.MV.Y, cand.Weight, cand.Flags)
	}
	for i, cand := range compCands {
		fmt.Printf(" cc_%d=(x0:%d,y0:%d,x1:%d,y1:%d,w:%d,flags:%d)", i, cand.MV[0].X, cand.MV[0].Y, cand.MV[1].X, cand.MV[1].Y, cand.Weight, cand.Flags)
	}
	fmt.Println()
}

func debugTraceCompoundRefStack(parsed *ParsedFrame, g BlockGeometry, stage string, cands []CompoundRefMVCandidate, nearestCount, nRows, nCols, haveNewMV, haveRowMVs, haveColMVs int) {
	if !debugInterTraceMatches(parsed, g) || strings.TrimSpace(os.Getenv("DEBUG_COMP_REF_STACK_TRACE")) == "" {
		return
	}
	fmt.Printf("DEBUG_COMP_REF_STACK parsed=%d geom=(%d,%d)-(%d,%d) stage=%s nearest=%d nRows=%d nCols=%d haveNew=%d haveRow=%d haveCol=%d n=%d",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		stage, nearestCount, nRows, nCols, haveNewMV, haveRowMVs, haveColMVs, len(cands))
	for i, cand := range cands {
		fmt.Printf(" cand%d=(x0:%d,y0:%d,x1:%d,y1:%d,w:%d,flags:%d)",
			i, cand.MV[0].X, cand.MV[0].Y, cand.MV[1].X, cand.MV[1].Y, cand.Weight, cand.Flags)
	}
	fmt.Println()
}

func debugTraceInterWarp(parsed *ParsedFrame, g BlockGeometry, warp affineWarp) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_INTER_WARP parsed=%d geom=(%d,%d)-(%d,%d) fixed=%v matrix=[%x %x %x %x %x %x] abcd=[%x %x %x %x] float=[%.6f %.6f %.6f %.6f %.6f %.6f]\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		warp.fixed,
		warp.matrix[0], warp.matrix[1], warp.matrix[2], warp.matrix[3], warp.matrix[4], warp.matrix[5],
		warp.abcd[0], warp.abcd[1], warp.abcd[2], warp.abcd[3],
		warp.a00, warp.a01, warp.a02, warp.a10, warp.a11, warp.a12)
}

func debugTraceRestorationUnit(sb SuperBlockGeometry, plane, unitX, unitY int, unit RestorationUnit, preState, postState [4]int) {
	if strings.TrimSpace(os.Getenv("DEBUG_RESTORATION_TRACE")) == "" {
		return
	}
	fmt.Printf("DEBUG_RESTORATION sb=(%d,%d)-(%d,%d) plane=%d unit=(%d,%d) type=%d filterH=%v filterV=%v sgr=%v pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d)\n",
		sb.StartX, sb.StartY, sb.EndX, sb.EndY, plane, unitX, unitY, unit.Type,
		unit.FilterH, unit.FilterV, unit.SGRWeights,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3])
}

func debugTraceInterLumaCoeffSkip(parsed *ParsedFrame, g BlockGeometry, txCtx, skipCtx int, skip uint32, preState, postState [4]int, preCDF, postCDF []uint16) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	fmt.Printf("DEBUG_INTER_COEFF_SKIP parsed=%d geom=(%d,%d)-(%d,%d) txCtx=%d skipCtx=%d skip=%d pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, txCtx, skipCtx, skip,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3],
		preCDF, postCDF,
	)
}

func debugTraceInterLumaCoeffDecoded(parsed *ParsedFrame, g BlockGeometry, txType TxType, resCtx uint8, coeffs []int32, state debugMSACState) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	cur, rng, cnt, pos := uint32(0), uint32(0), 0, 0
	if state != nil {
		cur, rng, cnt, pos = state.DebugState()
	}
	fmt.Printf("DEBUG_INTER_COEFF_DECODED parsed=%d geom=(%d,%d)-(%d,%d) txType=%d resCtx=%d coeffs=%v state=(cur=%d,r=%d,cnt=%d,pos=%d)\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		txType, resCtx, append([]int32(nil), coeffs[:minInt(len(coeffs), 64)]...),
		cur, rng, cnt, pos)
}

func debugTraceInterLumaSamples(parsed *ParsedFrame, g BlockGeometry, stage string, plane []byte, stride, planeWidth, planeHeight int) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", g.Start4X*4)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", g.Start4Y*4)
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || stride <= 0 || len(plane) < (planeHeight-1)*stride+planeWidth {
		return
	}
	patchX := x &^ 3
	patchY := y &^ 3
	fmt.Printf("DEBUG_INTER_LUMA parsed=%d geom=(%d,%d)-(%d,%d) stage=%s target=(%d,%d) value=%d patch=(%d,%d)",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, stage, x, y, plane[y*stride+x], patchX, patchY)
	for py := 0; py < 4 && patchY+py < planeHeight; py++ {
		fmt.Print(" row", py, "=")
		for px := 0; px < 4 && patchX+px < planeWidth; px++ {
			if px > 0 {
				fmt.Print(",")
			}
			fmt.Print(plane[(patchY+py)*stride+patchX+px])
		}
	}
	fmt.Println()
}

func debugTraceCompoundLumaPrediction(parsed *ParsedFrame, g BlockGeometry, refFrame0 *Frame, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp, refFrame1 *Frame, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, weight int) {
	if !debugInterTraceMatches(parsed, g) || refFrame0 == nil || refFrame1 == nil {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", g.Start4X*4)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", g.Start4Y*4)
	blockX := g.Start4X * 4
	blockY := g.Start4Y * 4
	width := g.Width4() * 4
	height := g.Height4() * 4
	visibleWidth := minInt(width, parsedPlaneWidthForHeader(parsed)-blockX)
	visibleHeight := minInt(height, parsedPlaneHeightForHeader(parsed)-blockY)
	if visibleWidth <= 0 || visibleHeight <= 0 || x < blockX || y < blockY || x >= blockX+visibleWidth || y >= blockY+visibleHeight {
		return
	}
	tmp0 := make([]byte, visibleWidth*visibleHeight)
	tmp1 := make([]byte, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef8(tmp0, visibleWidth, visibleWidth, visibleHeight, blockX, blockY, refFrame0.Y, refFrame0.YStride, refFrame0.Width, refFrame0.Height, mv0, filter0, useWarp0, warp0); err != nil {
		fmt.Printf("DEBUG_COMP_PRED parsed=%d geom=(%d,%d)-(%d,%d) err0=%v\n", parsed.Index, blockX, blockY, g.End4X*4, g.End4Y*4, err)
		return
	}
	if err := predictCompoundLumaRef8(tmp1, visibleWidth, visibleWidth, visibleHeight, blockX, blockY, refFrame1.Y, refFrame1.YStride, refFrame1.Width, refFrame1.Height, mv1, filter1, useWarp1, warp1); err != nil {
		fmt.Printf("DEBUG_COMP_PRED parsed=%d geom=(%d,%d)-(%d,%d) err1=%v\n", parsed.Index, blockX, blockY, g.End4X*4, g.End4Y*4, err)
		return
	}
	lx := x - blockX
	ly := y - blockY
	v0 := int(tmp0[ly*visibleWidth+lx])
	v1 := int(tmp1[ly*visibleWidth+lx])
	blend := clipByte((v0*weight + v1*(16-weight) + 8) >> 4)
	fmt.Printf("DEBUG_COMP_PRED parsed=%d geom=(%d,%d)-(%d,%d) target=(%d,%d) mv0=(%d,%d) mv1=(%d,%d) filter0=%v filter1=%v warp=(%v,%v) weight=%d ref0=%d ref1=%d blend=%d\n",
		parsed.Index, blockX, blockY, g.End4X*4, g.End4Y*4, x, y,
		mv0.X, mv0.Y, mv1.X, mv1.Y, filter0, filter1, useWarp0, useWarp1, weight, v0, v1, blend)
}

func debugTraceCompoundLumaPrediction16(parsed *ParsedFrame, g BlockGeometry, refPlane0 []uint16, refStride0, refWidth0, refHeight0 int, mv0 MotionVector, filter0 InterFilterPair, useWarp0 bool, warp0 affineWarp, refPlane1 []uint16, refStride1, refWidth1, refHeight1 int, mv1 MotionVector, filter1 InterFilterPair, useWarp1 bool, warp1 affineWarp, bitDepth uint8, weight int) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", g.Start4X*4)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", g.Start4Y*4)
	blockX := g.Start4X * 4
	blockY := g.Start4Y * 4
	width := g.Width4() * 4
	height := g.Height4() * 4
	visibleWidth := minInt(width, parsedPlaneWidthForHeader(parsed)-blockX)
	visibleHeight := minInt(height, parsedPlaneHeightForHeader(parsed)-blockY)
	if visibleWidth <= 0 || visibleHeight <= 0 || x < blockX || y < blockY || x >= blockX+visibleWidth || y >= blockY+visibleHeight {
		return
	}
	tmp0 := make([]uint16, visibleWidth*visibleHeight)
	tmp1 := make([]uint16, visibleWidth*visibleHeight)
	if err := predictCompoundLumaRef16(tmp0, visibleWidth, visibleWidth, visibleHeight, blockX, blockY, refPlane0, refStride0, refWidth0, refHeight0, mv0, filter0, useWarp0, warp0, bitDepth); err != nil {
		fmt.Printf("DEBUG_COMP_PRED16 parsed=%d geom=(%d,%d)-(%d,%d) err0=%v\n", parsed.Index, blockX, blockY, g.End4X*4, g.End4Y*4, err)
		return
	}
	if err := predictCompoundLumaRef16(tmp1, visibleWidth, visibleWidth, visibleHeight, blockX, blockY, refPlane1, refStride1, refWidth1, refHeight1, mv1, filter1, useWarp1, warp1, bitDepth); err != nil {
		fmt.Printf("DEBUG_COMP_PRED16 parsed=%d geom=(%d,%d)-(%d,%d) err1=%v\n", parsed.Index, blockX, blockY, g.End4X*4, g.End4Y*4, err)
		return
	}
	lx := x - blockX
	ly := y - blockY
	v0 := int(tmp0[ly*visibleWidth+lx])
	v1 := int(tmp1[ly*visibleWidth+lx])
	blend := clipSample((v0*weight+v1*(16-weight)+8)>>4, bitDepth)
	fmt.Printf("DEBUG_COMP_PRED16 parsed=%d geom=(%d,%d)-(%d,%d) target=(%d,%d) mv0=(%d,%d) mv1=(%d,%d) filter0=%v filter1=%v warp=(%v,%v) weight=%d ref0=%d ref1=%d blend=%d\n",
		parsed.Index, blockX, blockY, g.End4X*4, g.End4Y*4, x, y,
		mv0.X, mv0.Y, mv1.X, mv1.Y, filter0, filter1, useWarp0, useWarp1, weight, v0, v1, blend)
}

func parsedPlaneWidthForHeader(parsed *ParsedFrame) int {
	if parsed == nil {
		return 0
	}
	return parsed.Header.Width
}

func parsedPlaneHeightForHeader(parsed *ParsedFrame) int {
	if parsed == nil {
		return 0
	}
	return parsed.Header.Height
}

func debugTraceInterFrameLumaSample(parsed *ParsedFrame, stage string, plane []byte, stride, planeWidth, planeHeight int) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || stride <= 0 || len(plane) < (planeHeight-1)*stride+planeWidth {
		return
	}
	fmt.Printf("DEBUG_INTER_FRAME_LUMA parsed=%d stage=%s target=(%d,%d) value=%d\n",
		parsed.Index, stage, x, y, plane[y*stride+x])
	if strings.TrimSpace(os.Getenv("DEBUG_INTER_FRAME_PATCH")) == "" {
		return
	}
	x0 := maxIntLocal(0, x-8)
	y0 := maxIntLocal(0, y-8)
	x1 := minInt(planeWidth, x+9)
	y1 := minInt(planeHeight, y+9)
	for py := y0; py < y1; py++ {
		fmt.Printf("DEBUG_INTER_FRAME_PATCH parsed=%d stage=%s y=%d x0=%d", parsed.Index, stage, py, x0)
		for px := x0; px < x1; px++ {
			fmt.Printf(" %d", plane[py*stride+px])
		}
		fmt.Println()
	}
}

func debugTraceInterFrameLumaSample16(parsed *ParsedFrame, stage string, plane []uint16, stride, planeWidth, planeHeight int) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || stride <= 0 || len(plane) < (planeHeight-1)*stride+planeWidth {
		return
	}
	fmt.Printf("DEBUG_INTER_FRAME_LUMA16 parsed=%d stage=%s target=(%d,%d) value=%d\n",
		parsed.Index, stage, x, y, plane[y*stride+x])
	if strings.TrimSpace(os.Getenv("DEBUG_INTER_FRAME_PATCH")) == "" {
		return
	}
	x0 := maxIntLocal(0, x-8)
	y0 := maxIntLocal(0, y-8)
	x1 := minInt(planeWidth, x+9)
	y1 := minInt(planeHeight, y+9)
	for py := y0; py < y1; py++ {
		fmt.Printf("DEBUG_INTER_FRAME_PATCH16 parsed=%d stage=%s y=%d x0=%d", parsed.Index, stage, py, x0)
		for px := x0; px < x1; px++ {
			fmt.Printf(" %d", plane[py*stride+px])
		}
		fmt.Println()
	}
}

func debugTraceSingleRefPrediction16(parsed *ParsedFrame, g BlockGeometry, refSlot int8, refPOC uint8, refFrame *Frame, refPlane []uint16, refStride, refWidth, refHeight int, mv MotionVector, filter InterFilterPair, dst []uint16, dstStride, planeWidth, planeHeight int) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || dstStride <= 0 || len(dst) < (planeHeight-1)*dstStride+planeWidth {
		return
	}
	baseX8 := x*8 + mv.X
	baseY8 := y*8 + mv.Y
	srcX, fx := floorDivMod8(baseX8)
	srcY, fy := floorDivMod8(baseY8)
	refSample := -1
	if refWidth > 0 && refHeight > 0 && refStride > 0 && len(refPlane) >= (refHeight-1)*refStride+refWidth {
		refSample = int(refSample16(refPlane, refStride, refWidth, refHeight, srcX, srcY))
	}
	fmt.Printf("DEBUG_SINGLE_REF_PRED16 parsed=%d geom=(%d,%d)-(%d,%d) target=(%d,%d) value=%d refSlot=%d refPOC=%d refNil=%v refSize=%dx%d mv=(%d,%d) src=(%d,%d) frac=(%d,%d) refSample=%d filter=%v\n",
		parsed.Index,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		x, y, dst[y*dstStride+x],
		refSlot, refPOC, refFrame == nil, refWidth, refHeight,
		mv.X, mv.Y, srcX, srcY, fx, fy, refSample, filter)
}

func debugTraceOBMCSample16(parsed *ParsedFrame, stage string, g BlockGeometry, b *SpatialMVBlock, segX, segY, segW, segH int, tmp []uint16, tmpStride int, dst []uint16, dstStride, planeWidth, planeHeight int) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x < segX || y < segY || x >= segX+segW || y >= segY+segH {
		return
	}
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || dstStride <= 0 || tmpStride <= 0 {
		return
	}
	lx := x - segX
	ly := y - segY
	if len(tmp) < ly*tmpStride+lx+1 || len(dst) < y*dstStride+x+1 {
		return
	}
	var ref [2]int8
	var slot [2]int8
	var mv [2]MotionVector
	var filter InterFilterPair
	var size BlockSize
	var flags [2]uint8
	valid, intra := false, false
	if b != nil {
		ref = b.Ref
		slot = b.Slot
		mv = b.MV
		filter = b.Filter
		size = b.Size
		flags = b.RefFlags
		valid = b.Valid
		intra = b.Intra
	}
	fmt.Printf("DEBUG_OBMC16 parsed=%d stage=%s geom=(%d,%d)-(%d,%d) target=(%d,%d) seg=(%d,%d)+%dx%d local=(%d,%d) dst=%d tmp=%d neigh=(valid:%v,intra:%v,ref:%v,slot:%v,mv0:(x:%d,y:%d),mv1:(x:%d,y:%d),filter:%v,size:%d,flags:%v)\n",
		parsed.Index, stage,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		x, y, segX, segY, segW, segH, lx, ly, dst[y*dstStride+x], tmp[ly*tmpStride+lx],
		valid, intra, ref, slot, mv[0].X, mv[0].Y, mv[1].X, mv[1].Y, filter, size, flags)
}

func debugTraceInterFrameChromaSample(parsed *ParsedFrame, stage, planeName string, plane []byte, stride, planeWidth, planeHeight int) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return
	}
	wantPlane := strings.ToUpper(strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE_PLANE")))
	if wantPlane != "" && wantPlane != strings.ToUpper(planeName) {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_CHROMA_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_CHROMA_Y", -1)
	if x < 0 || y < 0 {
		x = debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1) >> 1
		y = debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1) >> 1
	}
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || stride <= 0 || len(plane) < (planeHeight-1)*stride+planeWidth {
		return
	}
	fmt.Printf("DEBUG_INTER_FRAME_CHROMA parsed=%d stage=%s plane=%s target=(%d,%d) value=%d\n",
		parsed.Index, stage, planeName, x, y, plane[y*stride+x])
}

func debugTraceInterFrameChromaSample16(parsed *ParsedFrame, stage, planeName string, plane []uint16, stride, planeWidth, planeHeight int) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return
	}
	wantPlane := strings.ToUpper(strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE_PLANE")))
	if wantPlane != "" && wantPlane != strings.ToUpper(planeName) {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_CHROMA_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_CHROMA_Y", -1)
	if x < 0 || y < 0 {
		x = debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1) >> 1
		y = debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1) >> 1
	}
	if x < 0 || y < 0 || x >= planeWidth || y >= planeHeight || stride <= 0 || len(plane) < (planeHeight-1)*stride+planeWidth {
		return
	}
	fmt.Printf("DEBUG_INTER_FRAME_CHROMA16 parsed=%d stage=%s plane=%s target=(%d,%d) value=%d\n",
		parsed.Index, stage, planeName, x, y, plane[y*stride+x])
}

func debugTraceInterChromaSamples(parsed *ParsedFrame, g BlockGeometry, stage, planeName string, plane []byte, stride, planeWidth, planeHeight int) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	debugTraceInterFrameChromaSample(parsed, stage, planeName, plane, stride, planeWidth, planeHeight)
}

func debugTraceTemporalProjection(parsed *ParsedFrame, selectedRef, ref2cur, ref2ref, srcX8, srcY8, posX8, posY8 int, rb TemporalMVBlock) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_TEMPORAL_PROJ_TRACE")) == "" {
		return
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return
	}
	targetX8 := debugInterTraceEnvInt("DEBUG_TEMPORAL_TRACE_X8", -1)
	targetY8 := debugInterTraceEnvInt("DEBUG_TEMPORAL_TRACE_Y8", -1)
	if targetX8 >= 0 && posX8 != targetX8 {
		return
	}
	if targetY8 >= 0 && posY8 != targetY8 {
		return
	}
	fmt.Printf("DEBUG_TEMPORAL_PROJ parsed=%d offset=%d selRef=%d ref2cur=%d ref2ref=%d src=(%d,%d) pos=(%d,%d) rb=(x:%d,y:%d,ref:%d)\n",
		parsed.Index, parsed.Header.FrameOffset, selectedRef, ref2cur, ref2ref, srcX8, srcY8, posX8, posY8, rb.MV.X, rb.MV.Y, rb.Ref)
}

func debugTraceTemporalSave(parsed *ParsedFrame, x8, y8 int, cand *SpatialMVBlock, block TemporalMVBlock) {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_TEMPORAL_SAVE_TRACE")) == "" {
		return
	}
	frameOffset := debugInterTraceEnvInt("DEBUG_TEMPORAL_TRACE_OFFSET", -1)
	if frameOffset >= 0 && int(parsed.Header.FrameOffset) != frameOffset {
		return
	}
	targetX8 := debugInterTraceEnvInt("DEBUG_TEMPORAL_TRACE_X8", -1)
	targetY8 := debugInterTraceEnvInt("DEBUG_TEMPORAL_TRACE_Y8", -1)
	if targetX8 >= 0 && x8 != targetX8 {
		return
	}
	if targetY8 >= 0 && y8 != targetY8 {
		return
	}
	if cand == nil {
		fmt.Printf("DEBUG_TEMPORAL_SAVE parsed=%d offset=%d cell=(%d,%d) cand=nil saved=(x:%d,y:%d,ref:%d)\n",
			parsed.Index, parsed.Header.FrameOffset, x8, y8, block.MV.X, block.MV.Y, block.Ref)
		return
	}
	fmt.Printf("DEBUG_TEMPORAL_SAVE parsed=%d offset=%d cell=(%d,%d) cand=(valid:%v,intra:%v,size:%d,ref:%v,slot:%v,mv0:(x:%d,y:%d),mv1:(x:%d,y:%d),flags:%d) saved=(x:%d,y:%d,ref:%d)\n",
		parsed.Index, parsed.Header.FrameOffset, x8, y8,
		cand.Valid, cand.Intra, cand.Size, cand.Ref, cand.Slot,
		cand.MV[0].X, cand.MV[0].Y, cand.MV[1].X, cand.MV[1].Y, cand.Flags,
		block.MV.X, block.MV.Y, block.Ref)
}

func debugTraceTemporalCompoundCandidate(parsed *ParsedFrame, g BlockGeometry, x8, y8 int, rb TemporalMVBlock, num0, num1 int, mv0, mv1 MotionVector, valid bool) {
	if !debugInterTraceMatches(parsed, g) || strings.TrimSpace(os.Getenv("DEBUG_TEMPORAL_COMP_CAND_TRACE")) == "" {
		return
	}
	fmt.Printf("DEBUG_TEMPORAL_COMP_CAND parsed=%d geom=(%d,%d)-(%d,%d) cell=(%d,%d) rb=(x:%d,y:%d,ref:%d) nums=(%d,%d) valid=%v mv0=(%d,%d) mv1=(%d,%d)\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		x8, y8, rb.MV.X, rb.MV.Y, rb.Ref, num0, num1, valid, mv0.X, mv0.Y, mv1.X, mv1.Y)
}

func debugTraceSingleRefBit(parsed *ParsedFrame, g BlockGeometry, stage string, ctx int, cdf []uint16, bit int, dec intraEntropyDecoder) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	if state, ok := dec.(debugMSACState); ok {
		cur, rng, cnt, pos := state.DebugState()
		fmt.Printf("DEBUG_REF_BIT parsed=%d geom=(%d,%d)-(%d,%d) stage=%s ctx=%d bit=%d cdf=%v post=(cur=%d,r=%d,cnt=%d,pos=%d)\n",
			parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
			stage, ctx, bit, cdf, cur, rng, cnt, pos)
		return
	}
	fmt.Printf("DEBUG_REF_BIT parsed=%d geom=(%d,%d)-(%d,%d) stage=%s ctx=%d bit=%d cdf=%v\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		stage, ctx, bit, cdf)
}

func debugTraceSingleRefModeBit(parsed *ParsedFrame, g BlockGeometry, stage string, ctx int, before [2]uint16, after []uint16, bit int, dec intraEntropyDecoder) {
	if !debugInterTraceMatches(parsed, g) {
		return
	}
	if state, ok := dec.(debugMSACState); ok {
		cur, rng, cnt, pos := state.DebugState()
		fmt.Printf("DEBUG_INTER_MODE_BIT parsed=%d geom=(%d,%d)-(%d,%d) stage=%s ctx=%d bit=%d preCDF=%v postCDF=%v post=(cur=%d,r=%d,cnt=%d,pos=%d)\n",
			parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
			stage, ctx, bit, before, after, cur, rng, cnt, pos)
		return
	}
	fmt.Printf("DEBUG_INTER_MODE_BIT parsed=%d geom=(%d,%d)-(%d,%d) stage=%s ctx=%d bit=%d preCDF=%v postCDF=%v\n",
		parsed.Index, g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		stage, ctx, bit, before, after)
}

func debugInterTraceMatches(parsed *ParsedFrame, g BlockGeometry) bool {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_TRACE")) == "" {
		return false
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return false
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x >= 0 && (x < g.Start4X*4 || x >= g.End4X*4) {
		return false
	}
	if y >= 0 && (y < g.Start4Y*4 || y >= g.End4Y*4) {
		return false
	}
	return true
}

func debugInterPartTraceMatches(parsed *ParsedFrame, g BlockGeometry) bool {
	if parsed == nil || strings.TrimSpace(os.Getenv("DEBUG_INTER_PART_TRACE")) == "" {
		return false
	}
	frameIndex := debugInterTraceEnvInt("DEBUG_INTER_TRACE_FRAME", -1)
	if frameIndex >= 0 && parsed.Index != frameIndex {
		return false
	}
	x := debugInterTraceEnvInt("DEBUG_INTER_TRACE_X", -1)
	y := debugInterTraceEnvInt("DEBUG_INTER_TRACE_Y", -1)
	if x >= 0 && (x < g.Start4X*4 || x >= g.End4X*4) {
		return false
	}
	if y >= 0 && (y < g.Start4Y*4 || y >= g.End4Y*4) {
		return false
	}
	return true
}

func debugInterTraceEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
