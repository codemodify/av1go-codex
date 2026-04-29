//go:build av1debug

package decoder

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugSPBTVInterHeaders(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}

	count := 0
	switchableMotion := 0
	filterMode := map[uint8]int{}
	highPrecision := 0
	forceInteger := 0
	globalMotion := 0
	showExisting := 0
	frameType := map[obu.FrameType]int{}

	for count < 40 {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		hdr := parsed.Header
		if hdr.ShowExistingFrame {
			showExisting++
			continue
		}
		frameType[hdr.FrameType]++
		if !isInterOrSwitchFrameType(hdr.FrameType) {
			count++
			continue
		}
		if hdr.SwitchableMotion {
			switchableMotion++
		}
		filterMode[hdr.SubpelFilterMode]++
		if hdr.HighPrecisionMV {
			highPrecision++
		}
		if hdr.ForceIntegerMV {
			forceInteger++
		}
		for _, gmv := range hdr.GMV {
			if gmv.Type != 0 {
				globalMotion++
				break
			}
		}
		count++
	}

	t.Logf("first40 frameType=%v showExisting=%d switchableMotion=%d filterMode=%v highPrecision=%d forceInteger=%d globalMotionFrames=%d",
		frameType, showExisting, switchableMotion, filterMode, highPrecision, forceInteger, globalMotion)
}

func TestDebugSPBTVFirstFrameTiles(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}

	for {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if parsed.Header.FrameType != obu.FrameTypeKey || parsed.Header.ShowExistingFrame {
			continue
		}
		states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
		if err != nil {
			t.Fatalf("BuildTileStates: %v", err)
		}
		for i, st := range states {
			g := st.Geometry
			t.Logf("tile %d: sbx=[%d,%d) sby=[%d,%d)", i, g.StartSBX, g.EndSBX, g.StartSBY, g.EndSBY)
		}
		return
	}
}

func TestDebugSPBTVInterMVStats(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}

	var interParsed []*ParsedFrame
	for len(interParsed) < 3 {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if isInterOrSwitchFrameType(parsed.Header.FrameType) && !parsed.Header.ShowExistingFrame {
			interParsed = append(interParsed, parsed)
		}
	}

	stats := debugInterMVStats{
		ref0: map[int8]int{},
		mode: map[InterPredMode]int{},
	}
	for _, parsed := range interParsed {
		if err := debugWalkInterMVStats(dec.header, parsed, &stats); err != nil {
			t.Fatalf("walk: %v", err)
		}
	}

	t.Logf("frames=%d blocks=%d intra=%d skip=%d integer=%d fracX=%d fracY=%d ref0=%v mode=%v",
		len(interParsed), stats.blocks, stats.intra, stats.skip, stats.integer, stats.fracX, stats.fracY, stats.ref0, stats.mode)
}

func TestDebugSPBTVInterMotionModeStats(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}

	var interParsed []*ParsedFrame
	for len(interParsed) < 8 {
		parsed, err := dec.NextParsedFrame()
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if isInterOrSwitchFrameType(parsed.Header.FrameType) && !parsed.Header.ShowExistingFrame {
			interParsed = append(interParsed, parsed)
		}
	}

	counts := map[InterMotionMode]int{}
	total := 0
	for _, parsed := range interParsed {
		n, err := debugWalkInterMotionModes(dec.header, parsed, counts)
		if err != nil {
			t.Fatalf("walk motion modes: %v", err)
		}
		total += n
	}

	t.Logf("frames=%d total=%d translation=%d obmc=%d warp=%d",
		len(interParsed), total, counts[interMotionTranslation], counts[interMotionOBMC], counts[interMotionWarp])
}

type debugInterMVStats struct {
	blocks  int
	intra   int
	skip    int
	integer int
	fracX   int
	fracY   int
	ref0    map[int8]int
	mode    map[InterPredMode]int
}

func debugWalkInterMVStats(seq av1.SequenceHeader, parsed *ParsedFrame, stats *debugInterMVStats) error {
	states, err := BuildTileStates(seq, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		return err
	}
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, &parsed.Header, tile, 0)
		if err != nil {
			return err
		}
		mvTile := NewSpatialMVTile(tile.Geometry)
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			row, err := syntax.NewRowState(sbRow)
			if err != nil {
				return err
			}
			for _, sb := range row.SuperBlocks {
				root := BlockGeometry{
					Level:   RootBlockLevel(seq),
					Size:    RootBlockSize(seq),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}
				if err := debugWalkInterMVBlock(seq, &parsed.Header, row, mvTile, sb, root, stats); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func debugWalkInterMotionModes(seq av1.SequenceHeader, parsed *ParsedFrame, counts map[InterMotionMode]int) (int, error) {
	states, err := BuildTileStates(seq, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(seq, &parsed.Header, tile, 0)
		if err != nil {
			return 0, err
		}
		mvTile := NewSpatialMVTile(tile.Geometry)
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			row, err := syntax.NewRowState(sbRow)
			if err != nil {
				return 0, err
			}
			for _, sb := range row.SuperBlocks {
				root := BlockGeometry{
					Level:   RootBlockLevel(seq),
					Size:    RootBlockSize(seq),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}
				n, err := debugWalkInterMotionBlock(seq, &parsed.Header, row, mvTile, sb, root, counts)
				if err != nil {
					return 0, err
				}
				total += n
			}
		}
	}
	return total, nil
}

func debugWalkInterMotionBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, g BlockGeometry, counts map[InterMotionMode]int) (int, error) {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return 0, err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return 0, err
	}
	total := 0
	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for _, child := range splitChildren(g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				n, err := debugWalkInterMotionBlock(seq, hdr, row, mvTile, sb, child, counts)
				if err != nil {
					return 0, err
				}
				total += n
			}
		} else {
			children, _, err := partitionChildren(g, part)
			if err != nil {
				return 0, err
			}
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				n, err := debugWalkInterMotionBlock(seq, hdr, row, mvTile, sb, child, counts)
				if err != nil {
					return 0, err
				}
				total += n
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return 0, err
		}
		for _, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			n, err := debugReadInterMotionLeaf(seq, hdr, row, mvTile, child, counts)
			if err != nil {
				return 0, err
			}
			total += n
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return 0, err
		}
		if !split {
			n, err := debugReadInterMotionLeaf(seq, hdr, row, mvTile, g, counts)
			if err != nil {
				return 0, err
			}
			total += n
		} else {
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				n, err := debugReadInterMotionLeaf(seq, hdr, row, mvTile, child, counts)
				if err != nil {
					return 0, err
				}
				total += n
			}
		}
	}
	if part != PartitionSplit || g.Level == BlockLevel8x8 {
		if err := ApplyPartitionContext(above, &row.Left, local, part); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func debugReadInterMotionLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, g BlockGeometry, counts map[InterMotionMode]int) (int, error) {
	above, local, err := contextForBlock(seq, row, SuperBlockGeometry{}, g)
	if err != nil {
		return 0, err
	}
	haveTop := g.Start4Y > row.Tile.Tile.Geometry.Start4Y
	haveLeft := g.Start4X > row.Tile.Tile.Geometry.Start4X
	syntax, err := ReadInterBlockPreludeWithNeighbors(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, haveTop, haveLeft, 0, row.Tile.Tile.Entropy)
	if err != nil {
		return 0, err
	}
	if syntax.Intra {
		intra, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, haveTop, haveLeft, 0, row.Tile.Tile.Entropy)
		if err != nil {
			return 0, err
		}
		if err := ApplyIntraBlockContext(seq, hdr, above, &row.Left, intra); err != nil {
			return 0, err
		}
		mvTile.SplatIntra(g)
		return 0, nil
	}
	ref0, err := ReadSingleRefIndex(row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, haveTop, haveLeft, row.Tile.Tile.Entropy)
	if err != nil {
		return 0, err
	}
	cands, mvCtx := mvTile.FindSingleRefCandidates(hdr, g, ref0)
	weights := make([]int, len(cands))
	for i := range cands {
		weights[i] = cands[i].Weight
	}
	mode, drl, err := ReadSingleRefInterMode(row.Tile.Tile.CDF, mvCtx, len(cands), weights, row.Tile.Tile.Entropy)
	if err != nil {
		return 0, err
	}
	mv, err := SelectRefMVCandidate(cands, drl)
	if err != nil {
		return 0, err
	}
	if mode == InterPredGlobal {
		mv = MotionVector{}
	}
	if mode == InterPredNew {
		mvPrec := mvResidualPrecision(hdr)
		mv, err = ReadMVResidual(row.Tile.Tile.MV, mv, mvPrec, row.Tile.Tile.Entropy)
		if err != nil {
			return 0, err
		}
	}
	syntax.Ref0 = ref0
	syntax.Mode = mode
	syntax.DRLIndex = drl
	syntax.MV = mv
	motion, err := ReadInterMotionMode(hdr, row.Tile.Tile.CDF, above, &row.Left, mvTile, g, syntax, haveTop, haveLeft, fullIntraEdgeAvailability, row.Tile.Tile.Entropy)
	if err != nil {
		return 0, err
	}
	counts[motion]++
	if motion != interMotionWarp {
		filter, err := ReadInterFilterPair(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, ref0, false, mv, row.Tile.Tile.Entropy)
		if err != nil {
			return 0, err
		}
		syntax.Filter = filter
	} else {
		syntax.Filter = defaultInterFilterPair()
	}
	if err := ApplyInterBlockContext(seq, hdr, above, &row.Left, syntax); err != nil {
		return 0, err
	}
	mvTile.SplatSingleRef(g, ref0, -1, mv, mode, syntax.Filter)
	return 1, nil
}

func debugWalkInterMVBlock(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, sb SuperBlockGeometry, g BlockGeometry, stats *debugInterMVStats) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if part == PartitionSplit && g.Level < BlockLevel8x8 {
		dims := BlockDims(g.Size)
		halfW4 := int(dims.W4) >> 1
		halfH4 := int(dims.H4) >> 1
		if g.Width4() <= halfW4 && g.Height4() <= halfH4 {
			for _, child := range splitChildren(g) {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugWalkInterMVBlock(seq, hdr, row, mvTile, sb, child, stats); err != nil {
					return err
				}
			}
		} else {
			children, _, err := partitionChildren(g, part)
			if err != nil {
				return err
			}
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugWalkInterMVBlock(seq, hdr, row, mvTile, sb, child, stats); err != nil {
					return err
				}
			}
		}
	} else if part == PartitionSplit && g.Level == BlockLevel8x8 {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if err := debugReadInterMVLeaf(seq, hdr, row, mvTile, child, stats); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugReadInterMVLeaf(seq, hdr, row, mvTile, g, stats); err != nil {
				return err
			}
		} else {
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugReadInterMVLeaf(seq, hdr, row, mvTile, child, stats); err != nil {
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

func debugReadInterMVLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, mvTile *SpatialMVTile, g BlockGeometry, stats *debugInterMVStats) error {
	above, local, err := contextForBlock(seq, row, SuperBlockGeometry{}, g)
	if err != nil {
		return err
	}
	syntax, err := ReadInterBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, g, 0, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	stats.blocks++
	if syntax.Skip {
		stats.skip++
	}
	if syntax.Intra {
		stats.intra++
		if syntax.IntraBlk != nil {
			if err := ApplyIntraBlockContext(seq, hdr, above, &row.Left, *syntax.IntraBlk); err != nil {
				return err
			}
		}
		mvTile.SplatIntra(g)
		return nil
	}

	ref0, err := ReadSingleRefIndex(row.Tile.Tile.CDF, above, &row.Left, local.Start4Y&31, local.Start4X&31, local.Start4Y > 0, local.Start4X > 0, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	cands, mvCtx := mvTile.FindSingleRefCandidates(hdr, g, ref0)
	weights := make([]int, len(cands))
	for i := range cands {
		weights[i] = cands[i].Weight
	}
	mode, drl, err := ReadSingleRefInterMode(row.Tile.Tile.CDF, mvCtx, len(cands), weights, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	mv, err := SelectRefMVCandidate(cands, drl)
	if err != nil {
		return err
	}
	if mode == InterPredGlobal {
		mv = MotionVector{}
	}
	if mode == InterPredNew {
		mvPrec := mvResidualPrecision(hdr)
		mv, err = ReadMVResidual(row.Tile.Tile.MV, mv, mvPrec, row.Tile.Tile.Entropy)
		if err != nil {
			return err
		}
	}

	stats.ref0[ref0]++
	stats.mode[mode]++
	if mv.X&7 != 0 {
		stats.fracX++
	}
	if mv.Y&7 != 0 {
		stats.fracY++
	}
	if mv.X&7 == 0 && mv.Y&7 == 0 {
		stats.integer++
	}

	if err := ApplyInterBlockContext(seq, hdr, above, &row.Left, InterBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Skip:     syntax.Skip,
		Ref0:     ref0,
		Mode:     mode,
	}); err != nil {
		return err
	}
	mvTile.SplatSingleRef(g, ref0, -1, mv, mode, defaultInterFilterPair())
	return nil
}

func TestDebugSPBTVFirstFrameSyntax(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	parsed, err := dec.NextParsedFrame()
	if err != nil {
		t.Fatalf("NextParsedFrame: %v", err)
	}
	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	stats := debugSyntaxStats{
		yMode:  map[IntraPredMode]int{},
		uvMode: map[IntraPredMode]int{},
		tx:     map[TxfmSize]int{},
		size:   map[BlockSize]int{},
	}
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, tile, 0)
		if err != nil {
			t.Fatalf("NewTileSyntaxState: %v", err)
		}
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			row, err := syntax.NewRowState(sbRow)
			if err != nil {
				t.Fatalf("NewRowState: %v", err)
			}
			for _, sb := range row.SuperBlocks {
				if err := debugWalkIntraSyntax(dec.header, &parsed.Header, row, sb, BlockGeometry{
					Level:   RootBlockLevel(dec.header),
					Size:    RootBlockSize(dec.header),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}, &stats); err != nil {
					t.Fatalf("walk: %v", err)
				}
			}
		}
	}
	t.Logf("total=%d palY=%d palUV=%d size=%v tx=%v yMode=%v uvMode=%v palBlocks=%v", stats.total, stats.palY, stats.palUV, stats.size, stats.tx, stats.yMode, stats.uvMode, stats.palBlocks)
}

type debugSyntaxStats struct {
	total     int
	palY      int
	palUV     int
	size      map[BlockSize]int
	yMode     map[IntraPredMode]int
	uvMode    map[IntraPredMode]int
	tx        map[TxfmSize]int
	palBlocks []string
}

func TestDebugSPBTVFirstFrameResidualStats(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testvideo", "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	parsed, err := dec.NextParsedFrame()
	if err != nil {
		t.Fatalf("NextParsedFrame: %v", err)
	}
	states, err := BuildTileStates(dec.header, &parsed.Header, &parsed.TileGroup)
	if err != nil {
		t.Fatalf("BuildTileStates: %v", err)
	}
	dq := InitDequantTables(dec.header, &parsed.Header, int(parsed.Header.Quantization.YAC))
	stats := debugResidualStats{
		yTxType:  map[TxType]int{},
		uvTxType: map[TxType]int{},
	}
	for _, tile := range states {
		syntax, err := NewTileSyntaxState(dec.header, &parsed.Header, tile, 0)
		if err != nil {
			t.Fatalf("NewTileSyntaxState: %v", err)
		}
		for sbRow := tile.Geometry.StartSBY; sbRow < tile.Geometry.EndSBY; sbRow++ {
			row, err := syntax.NewRowState(sbRow)
			if err != nil {
				t.Fatalf("NewRowState: %v", err)
			}
			for _, sb := range row.SuperBlocks {
				root := BlockGeometry{
					Level:   RootBlockLevel(dec.header),
					Size:    RootBlockSize(dec.header),
					Start4X: sb.Start4X,
					Start4Y: sb.Start4Y,
					End4X:   sb.End4X,
					End4Y:   sb.End4Y,
				}
				if err := debugWalkIntraResiduals(dec.header, &parsed.Header, row, sb, root, dq, &stats); err != nil {
					t.Fatalf("walk residuals: %v", err)
				}
			}
		}
	}
	t.Logf("luma_tu=%d luma_nonzero=%d luma_dc_only=%d luma_skip=%d yTxType=%v chroma_tu=%d chroma_nonzero=%d chroma_dc_only=%d chroma_skip=%d uvTxType=%v",
		stats.lumaTU, stats.lumaNonZero, stats.lumaDCOnly, stats.lumaSkip, stats.yTxType,
		stats.chromaTU, stats.chromaNonZero, stats.chromaDCOnly, stats.chromaSkip, stats.uvTxType)
}

type debugResidualStats struct {
	lumaTU        int
	lumaNonZero   int
	lumaDCOnly    int
	lumaSkip      int
	chromaTU      int
	chromaNonZero int
	chromaDCOnly  int
	chromaSkip    int
	yTxType       map[TxType]int
	uvTxType      map[TxType]int
}

func debugWalkIntraResiduals(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, stats *debugResidualStats) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if part == PartitionSplit {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if g.Level < BlockLevel8x8 && child.Width4() <= g.Width4()/2 && child.Height4() <= g.Height4()/2 {
				if err := debugWalkIntraResiduals(seq, hdr, row, sb, child, dq, stats); err != nil {
					return err
				}
				continue
			}
			if err := debugReadIntraResidualLeaf(seq, hdr, row, sb, child, dq, stats); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugReadIntraResidualLeaf(seq, hdr, row, sb, g, dq, stats); err != nil {
				return err
			}
		} else {
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugReadIntraResidualLeaf(seq, hdr, row, sb, child, dq, stats); err != nil {
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

func debugReadIntraResidualLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, dq DequantTables, stats *debugResidualStats) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, g.Start4Y > 0, g.Start4X > 0, 0, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}

	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	txInfo := TxfmInfoFor(syntax.TX)
	for offY4 := 0; offY4 < blockHeight4; offY4 += int(txInfo.H4) {
		for offX4 := 0; offX4 < blockWidth4; offX4 += int(txInfo.W4) {
			tu := BlockGeometry{
				Level:   g.Level,
				Size:    g.Size,
				Start4X: g.Start4X + offX4,
				Start4Y: g.Start4Y + offY4,
				End4X:   g.Start4X + offX4 + int(txInfo.W4),
				End4Y:   g.Start4Y + offY4 + int(txInfo.H4),
			}
			yTxType, err := ReadIntraTxType(hdr, row.Tile.Tile.CDF, syntax, syntax.TX, g.Start4Y > 0 || offY4 > 0, g.Start4X > 0 || offX4 > 0, row.Tile.Tile.Entropy)
			if err != nil {
				return err
			}
			stats.yTxType[yTxType]++
			stats.lumaTU++
			coeffs, resCtx, skip, err := DecodeLumaCoefficients(syntax.Size, tu, syntax.TX, yTxType, syntax.Segment, row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
			if err != nil {
				return err
			}
			if skip {
				stats.lumaSkip++
			}
			if coeffsNonZero(coeffs) {
				stats.lumaNonZero++
				if coeffsDCOnly(coeffs) {
					stats.lumaDCOnly++
				}
			}
			if err := ApplyLumaResidualContext(above, &row.Left, tu, syntax.TX, resCtx); err != nil {
				return err
			}
		}
	}

	if hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		chromaGeom := chromaGeometryForBlock(seq.ColorConfig.ChromaSubsampling(), g)
		uvTxType := IntraUVTxType(hdr, syntax.Segment, syntax.UVTX, syntax.UVMode)
		stats.uvTxType[uvTxType]++
		uvInfo := TxfmInfoFor(syntax.UVTX)
		for offY4 := 0; offY4 < chromaGeom.Height4(); offY4 += int(uvInfo.H4) {
			for offX4 := 0; offX4 < chromaGeom.Width4(); offX4 += int(uvInfo.W4) {
				tu := BlockGeometry{
					Level:   chromaGeom.Level,
					Size:    chromaGeom.Size,
					Start4X: chromaGeom.Start4X + offX4,
					Start4Y: chromaGeom.Start4Y + offY4,
					End4X:   min(chromaGeom.Start4X+offX4+int(uvInfo.W4), chromaGeom.End4X),
					End4Y:   min(chromaGeom.Start4Y+offY4+int(uvInfo.H4), chromaGeom.End4Y),
				}
				for plane := 1; plane <= 2; plane++ {
					stats.chromaTU++
					coeffs, resCtx, skip, err := DecodeChromaCoefficients(syntax.Size, tu, syntax.UVTX, uvTxType, syntax.Segment, plane, seq.ColorConfig.ChromaSubsampling(), row.Tile.Tile.Coef, dq, above, &row.Left, row.Tile.Tile.Entropy)
					if err != nil {
						return err
					}
					if skip {
						stats.chromaSkip++
					}
					if coeffsNonZero(coeffs) {
						stats.chromaNonZero++
						if coeffsDCOnly(coeffs) {
							stats.chromaDCOnly++
						}
					}
					if err := ApplyChromaResidualContext(above, &row.Left, plane, tu, syntax.UVTX, resCtx); err != nil {
						return err
					}
				}
			}
		}
	}

	return ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax)
}

func coeffsNonZero(coeffs []int32) bool {
	for _, v := range coeffs {
		if v != 0 {
			return true
		}
	}
	return false
}

func coeffsDCOnly(coeffs []int32) bool {
	if len(coeffs) == 0 || coeffs[0] == 0 {
		return false
	}
	for i := 1; i < len(coeffs); i++ {
		if coeffs[i] != 0 {
			return false
		}
	}
	return true
}

func debugWalkIntraSyntax(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, stats *debugSyntaxStats) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	part, err := DecodePartitionDecision(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	if part == PartitionSplit {
		children, _, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child.Width4() == 0 || child.Height4() == 0 {
				continue
			}
			if g.Level < BlockLevel8x8 && child.Width4() <= g.Width4()/2 && child.Height4() <= g.Height4()/2 {
				if err := debugWalkIntraSyntax(seq, hdr, row, sb, child, stats); err != nil {
					return err
				}
				continue
			}
			if err := debugReadIntraLeaf(seq, hdr, row, sb, child, stats); err != nil {
				return err
			}
		}
	} else {
		children, split, err := partitionChildren(g, part)
		if err != nil {
			return err
		}
		if !split {
			if err := debugReadIntraLeaf(seq, hdr, row, sb, g, stats); err != nil {
				return err
			}
		} else {
			for _, child := range children {
				if child.Width4() == 0 || child.Height4() == 0 {
					continue
				}
				if err := debugReadIntraLeaf(seq, hdr, row, sb, child, stats); err != nil {
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

func debugReadIntraLeaf(seq av1.SequenceHeader, hdr *obu.FrameHeader, row *TileRowSyntaxState, sb SuperBlockGeometry, g BlockGeometry, stats *debugSyntaxStats) error {
	above, local, err := contextForBlock(seq, row, sb, g)
	if err != nil {
		return err
	}
	syntax, err := ReadIntraBlockSyntax(seq, hdr, row.Tile.Tile.CDF, above, &row.Left, local, g.Start4Y > 0, g.Start4X > 0, 0, row.Tile.Tile.Entropy)
	if err != nil {
		return err
	}
	stats.total++
	stats.size[syntax.Size]++
	if syntax.PalYSize != 0 {
		stats.palY++
		stats.palBlocks = append(stats.palBlocks, fmt.Sprintf("Y@(%d,%d) size=%d pal=%d", g.Start4X*4, g.Start4Y*4, syntax.Size, syntax.PalYSize))
	}
	if syntax.PalUVSize != 0 {
		stats.palUV++
		stats.palBlocks = append(stats.palBlocks, fmt.Sprintf("UV@(%d,%d) size=%d pal=%d", g.Start4X*4, g.Start4Y*4, syntax.Size, syntax.PalUVSize))
	}
	stats.yMode[syntax.YMode]++
	stats.uvMode[syntax.UVMode]++
	stats.tx[syntax.TX]++
	return ApplyIntraBlockContext(seq, hdr, above, &row.Left, syntax)
}
