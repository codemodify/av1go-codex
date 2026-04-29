package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func decodeKeyframeMain10WithIntraBC(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, dq DequantTables, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, segState *SegmentationState, field *SpatialMVTile) error {
	states, err := BuildTileStates(seq, hdr, group)
	if err != nil {
		return err
	}
	defer CloseTileStates(states)
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, hdr, tile, 0)
		if err != nil {
			return err
		}
		defer syntax.Close()
		mvTile := field.tileView(tile.Geometry)
		var row TileRowSyntaxState
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			if err := syntax.ResetRowState(&row, sbRow); err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				if err := decodeKeyframeMain10ScreenContentSuperBlock(seq, hdr, &row, &mvTile, sb, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func decodeKeyframeMain10ScreenContentSuperBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, dq DequantTables, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, segState *SegmentationState) error {
	root := BlockGeometry{
		Level:   RootBlockLevel(seq),
		Size:    RootBlockSize(seq),
		Start4X: sb.Start4X,
		Start4Y: sb.Start4Y,
		End4X:   sb.End4X,
		End4Y:   sb.End4Y,
	}
	return decodeKeyframeMain10ScreenContentBlock(seq, hdr, row, mvTile, sb, root, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, rootIntraEdgeAvailabilityForBlock(row.Tile.Tile.Geometry, root))
}

func decodeKeyframeMain10ScreenContentBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	var childrenBuf [4]BlockGeometry

	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for i, child := range splitChildrenInto(childrenBuf[:0], g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain10ScreenContentBlock(seq, hdr, row, mvTile, sb, child, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, childEdges[i]); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildrenInto(childrenBuf[:0], g, part)
			if err != nil {
				return err
			}
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain10ScreenContentBlock(seq, hdr, row, mvTile, sb, child, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, childEdges[i]); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
		for i, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := decodeKeyframeMain10ScreenContentLeaf(seq, hdr, row, mvTile, sb, child, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, childEdges[i]); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildrenInto(childrenBuf[:0], g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := decodeKeyframeMain10ScreenContentLeaf(seq, hdr, row, mvTile, sb, g, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, edge); err != nil {
				return err
			}
		} else {
			childEdges, _ := partitionIntraEdgeChildren(g.Level, part, edge)
			for i, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := decodeKeyframeMain10ScreenContentLeaf(seq, hdr, row, mvTile, sb, child, dq, yPlane, yStride, uPlane, uStride, vPlane, vStride, cdef, segState, childEdges[i]); err != nil {
					return err
				}
			}
		}
	}

	if part != PartitionSplit || g.Level == BlockLevel8x8 {
		if err := ApplyPartitionContext(above, &row.Left, local, part); err != nil {
			return err
		}
	}
	return nil
}

func decodeKeyframeMain10ScreenContentLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, yPlane []uint16, yStride int, uPlane []uint16, uStride int, vPlane []uint16, vStride int, cdef *CDEFIndexMap, segState *SegmentationState, edge intraEdgeAvailability) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	if row.Tile.Tile.CDF == nil || row.Tile.Tile.Entropy == nil {
		return fmt.Errorf("decoder: intrabc leaf requires tile entropy state")
	}
	ctxHaveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	ctxHaveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	reconHaveTop, reconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
	chromaReconHaveTop, chromaReconHaveLeft := tileIntraNeighborAvailability(row.Tile.Tile.Geometry, g)
	chromaEdge := edge.forChromaLayout(seq.ColorConfig.ChromaSubsampling())

	segID := uint8(0)
	segPred := false
	if hdr.Segmentation.Enabled && hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, ctxHaveTop, ctxHaveLeft, false, false, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}
	screenSyntax, err := ReadScreenContentBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, segID, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if hdr.Segmentation.Enabled && !hdr.Segmentation.Preskip {
		segID, segPred, err = readBlockSegmentID(hdr, row.Tile.Tile.CDF, above, &row.Left, segState, g, ctxHaveTop, ctxHaveLeft, true, screenSyntax.Skip, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
		screenSyntax.Segment = segID
	}
	screenSyntax.SegPred = segPred

	if screenSyntax.Intra {
		if screenSyntax.IntraBlk == nil {
			return fmt.Errorf("decoder: missing screen-content intra syntax")
		}
		screenSyntax.IntraBlk.SegPred = segPred
		screenSyntax.IntraBlk.SkipMode = screenSyntax.SkipMode
		screenSyntax.IntraBlk.Skip = screenSyntax.Skip
		if err := ReconstructParsedIntraLumaBlockAt16WithSkipNeighborEdges(
			seq,
			hdr,
			row.Tile.Tile.CDF,
			row.Tile.Tile.Coef,
			dq,
			above,
			&row.Left,
			*screenSyntax.IntraBlk,
			yPlane,
			yStride,
			hdr.Width,
			hdr.Height,
			g.Start4X,
			g.Start4Y,
			seq.ColorConfig.BitDepth,
			screenSyntax.Skip,
			row.Tile.Tile.Entropy,
			reconHaveTop,
			reconHaveLeft,
			edge.topHasRight,
			edge.leftHasBottom,
		); err != nil {
			return err
		}
		screenSyntax.IntraBlk.PreserveTXContext = !screenSyntax.Skip
		if err := ApplyIntraBlockContext(seq, hdr, above, &row.Left, *screenSyntax.IntraBlk); err != nil {
			return err
		}
		if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, screenSyntax.Skip, row.Tile.Tile.Entropy); err != nil {
			return err
		}
		cdef.markNoSkip(g, screenSyntax.Skip)
		if err := reconstructParsedIntraChromaBlockAt16WithSkipNeighborEdges(
			seq,
			hdr,
			row.Tile.Tile.Coef,
			dq,
			above,
			&row.Left,
			*screenSyntax.IntraBlk,
			yPlane,
			yStride,
			hdr.Width,
			hdr.Height,
			uPlane,
			uStride,
			(hdr.Width+1)>>1,
			(hdr.Height+1)>>1,
			vPlane,
			vStride,
			(hdr.Width+1)>>1,
			(hdr.Height+1)>>1,
			seq.ColorConfig.BitDepth,
			screenSyntax.Skip,
			row.Tile.Tile.Entropy,
			chromaReconHaveTop,
			chromaReconHaveLeft,
			chromaEdge.topHasRight,
			chromaEdge.leftHasBottom,
		); err != nil {
			return err
		}
		mvTile.SplatIntra(g)
		return nil
	}

	cands, _ := mvTile.FindSingleRefCandidates(hdr, g, 0)
	refMV := chooseIntraBCReferenceMV(seq, row.Tile.Tile.Geometry, g, cands)
	mv, err := ReadMVResidual(row.Tile.Tile.MV, refMV, -1, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	hasChroma := hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y)
	mv, err = resolveIntraBCMotionVector(seq, row.Tile.Tile.Geometry, sb, g, refMV, mv, cands, hasChroma)
	if err != nil {
		return err
	}

	if err := PredictInterLumaBlock16(yPlane, yStride, hdr.Width, hdr.Height, g.Start4X*4, g.Start4Y*4, g.Width4()*4, g.Height4()*4, yPlane, yStride, hdr.Width, hdr.Height, mv, seq.ColorConfig.BitDepth); err != nil {
		return err
	}
	if hasChroma {
		chromaX, chromaY, chromaW, chromaH := chromaSampleRectForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		chromaWidth := (hdr.Width + 1) >> 1
		chromaHeight := (hdr.Height + 1) >> 1
		if err := PredictInterChromaBlock16(uPlane, uStride, chromaWidth, chromaHeight, chromaX, chromaY, chromaW, chromaH, uPlane, uStride, chromaWidth, chromaHeight, mv, seq.ColorConfig.BitDepth); err != nil {
			return err
		}
		if err := PredictInterChromaBlock16(vPlane, vStride, chromaWidth, chromaHeight, chromaX, chromaY, chromaW, chromaH, vPlane, vStride, chromaWidth, chromaHeight, mv, seq.ColorConfig.BitDepth); err != nil {
			return err
		}
	}

	unitsScratch := takeTransformUnitScratch(local)
	defer putTransformUnitScratch(unitsScratch)
	units, err := ReadInterTransformUnitsInto(unitsScratch.buf[:0], hdr, row.Tile.Tile.CDF, above, &row.Left, local, g, screenSyntax.Segment, screenSyntax.Skip, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	for _, tu := range units {
		resCtx := uint8(0x40)
		if !screenSyntax.Skip {
			coeffs, ctx, _, err := DecodeLumaCoefficientsWithBitDepth(g.Size, tu.Local, tu.TX, TxTypeDCTDCT, screenSyntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, seq.ColorConfig.BitDepth)
			if err != nil {
				return err
			}
			if err := ReconstructInterLumaCoefficients16(yPlane, yStride, hdr.Width, hdr.Height, tu.Global.Start4X*4, tu.Global.Start4Y*4, tu.TX, coeffs, seq.ColorConfig.BitDepth, hdr.Segmentation.Lossless[screenSyntax.Segment]); err != nil {
				return err
			}
			resCtx = ctx
		}
		if err := ApplyLumaResidualContext(above, &row.Left, tu.Local, tu.TX, resCtx); err != nil {
			return err
		}
	}
	if hasChroma {
		uvtx := interChromaTXForBlock(seq, hdr, g.Size, screenSyntax.Segment)
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		countX, countY := transformGridCounts(chromaGeom, uvtx)
		var chromaUnitsBuf [16]BlockGeometry
		chromaDst, chromaScratch := takeBlockGeometryBuffer(chromaUnitsBuf[:0], countX*countY)
		defer putBlockGeometryBuffer(chromaScratch)
		chromaUnits := buildChromaTransformUnitsInto(chromaDst, seq.ColorConfig.ChromaSubsampling(), g, uvtx)
		chromaWidth := (hdr.Width + 1) >> 1
		chromaHeight := (hdr.Height + 1) >> 1
		for _, tu := range chromaUnits {
			uResCtx := uint8(0x40)
			vResCtx := uint8(0x40)
			if !screenSyntax.Skip {
				uCoeffs, ctx, _, err := DecodeChromaCoefficientsDCT2DWithBitDepth(g.Size, tu, uvtx, screenSyntax.Segment, 1, seq.ColorConfig.ChromaSubsampling(), row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, seq.ColorConfig.BitDepth)
				if err != nil {
					return err
				}
				if err := ReconstructInterChromaCoefficients16(uPlane, uStride, chromaWidth, chromaHeight, tu.Start4X*4, tu.Start4Y*4, uvtx, uCoeffs, seq.ColorConfig.BitDepth, hdr.Segmentation.Lossless[screenSyntax.Segment]); err != nil {
					return err
				}
				uResCtx = ctx

				vCoeffs, ctx, _, err := DecodeChromaCoefficientsDCT2DWithBitDepth(g.Size, tu, uvtx, screenSyntax.Segment, 2, seq.ColorConfig.ChromaSubsampling(), row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy, seq.ColorConfig.BitDepth)
				if err != nil {
					return err
				}
				if err := ReconstructInterChromaCoefficients16(vPlane, vStride, chromaWidth, chromaHeight, tu.Start4X*4, tu.Start4Y*4, uvtx, vCoeffs, seq.ColorConfig.BitDepth, hdr.Segmentation.Lossless[screenSyntax.Segment]); err != nil {
					return err
				}
				vResCtx = ctx
			}
			if err := ApplyChromaResidualContextWithLayout(above, &row.Left, 1, seq.ColorConfig.ChromaSubsampling(), tu, uvtx, uResCtx); err != nil {
				return err
			}
			if err := ApplyChromaResidualContextWithLayout(above, &row.Left, 2, seq.ColorConfig.ChromaSubsampling(), tu, uvtx, vResCtx); err != nil {
				return err
			}
		}
	}
	if err := cdef.ensureBlockIndex(g, hdr.CDEF.NBits, screenSyntax.Skip, row.Tile.Tile.Entropy); err != nil {
		return err
	}
	cdef.markNoSkip(g, screenSyntax.Skip)

	if err := ApplyIntraBCContext(seq, above, &row.Left, g, screenSyntax.Skip, screenSyntax.SegPred); err != nil {
		return err
	}
	mvTile.SplatIntraBC(g, mv)
	return nil
}
