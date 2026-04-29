package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type BlockLevel uint8

const (
	BlockLevel128x128 BlockLevel = iota
	BlockLevel64x64
	BlockLevel32x32
	BlockLevel16x16
	BlockLevel8x8

	numBlockLevels
)

type BlockPartition uint8

const (
	PartitionNone BlockPartition = iota
	PartitionH
	PartitionV
	PartitionSplit
	PartitionTTopSplit
	PartitionTBottomSplit
	PartitionTLeftSplit
	PartitionTRightSplit
	PartitionH4
	PartitionV4

	numPartitions
	numSub8x8Partitions = PartitionTTopSplit
)

type BlockSize uint8

const (
	Block128x128 BlockSize = iota
	Block128x64
	Block64x128
	Block64x64
	Block64x32
	Block64x16
	Block32x64
	Block32x32
	Block32x16
	Block32x8
	Block16x64
	Block16x32
	Block16x16
	Block16x8
	Block16x4
	Block8x32
	Block8x16
	Block8x8
	Block8x4
	Block4x16
	Block4x8
	Block4x4

	numBlockSizes
)

type BlockDimensions struct {
	W4     uint8
	H4     uint8
	Log2W4 uint8
	Log2H4 uint8
}

type SuperBlockGeometry struct {
	Index    int
	SBX      int
	SBY      int
	Start4X  int
	Start4Y  int
	End4X    int
	End4Y    int
	StartX   int
	StartY   int
	EndX     int
	EndY     int
	Width4   int
	Height4  int
	WidthPx  int
	HeightPx int
}

type TileGeometry struct {
	Index       int
	Col         int
	Row         int
	StartSBX    int
	EndSBX      int
	StartSBY    int
	EndSBY      int
	Start4X     int
	End4X       int
	Start4Y     int
	End4Y       int
	StartX      int
	EndX        int
	StartY      int
	EndY        int
	SuperBlocks []SuperBlockGeometry
}

type TileState struct {
	Geometry           TileGeometry
	Data               []byte
	Entropy            *msac.Context
	CDF                *ModeCDF
	Coef               *CoefCDF
	CoefByQ            [4]*CoefCDF
	MV                 *MVCDF
	superBlocksScratch *scratchBuffer[SuperBlockGeometry]
}

type FrameCDFState struct {
	Mode    *ModeCDF
	MV      *MVCDF
	CoefByQ [4]*CoefCDF
}

func (s *FrameCDFState) Clone() *FrameCDFState {
	if s == nil {
		return nil
	}
	return cloneFrameCDFState(s)
}

func (t *TileState) SetActiveCoefQIndex(qidx int) {
	if t == nil {
		return
	}
	qcat := DefaultCoefQCat(uint8(clipQIndexRuntime(qidx)))
	t.SetActiveCoefQCat(qcat)
}

func (t *TileState) SetActiveCoefQCat(qcat int) {
	if t == nil {
		return
	}
	if override := strings.TrimSpace(os.Getenv("DEBUG_FORCE_COEF_QCAT")); override != "" {
		if v, err := strconv.Atoi(override); err == nil {
			qcat = clampDefaultCoefQCat(v)
		}
	}
	qcat = clampDefaultCoefQCat(qcat)
	if t.CoefByQ[qcat] == nil {
		t.CoefByQ[qcat] = takeDefaultCoefCDFForQCat(qcat)
	}
	t.Coef = t.CoefByQ[qcat]
}

var alignPartitionContext = [2][numBlockLevels][numPartitions]int8{
	{
		{0x00, 0x00, 0x10, -1, 0x00, 0x10, 0x10, 0x10, -1, -1},
		{0x10, 0x10, 0x18, -1, 0x10, 0x18, 0x18, 0x18, 0x10, 0x1c},
		{0x18, 0x18, 0x1c, -1, 0x18, 0x1c, 0x1c, 0x1c, 0x18, 0x1e},
		{0x1c, 0x1c, 0x1e, -1, 0x1c, 0x1e, 0x1e, 0x1e, 0x1c, 0x1f},
		{0x1e, 0x1e, 0x1f, 0x1f, -1, -1, -1, -1, -1, -1},
	},
	{
		{0x00, 0x10, 0x00, -1, 0x10, 0x10, 0x00, 0x10, -1, -1},
		{0x10, 0x18, 0x10, -1, 0x18, 0x18, 0x10, 0x18, 0x1c, 0x10},
		{0x18, 0x1c, 0x18, -1, 0x1c, 0x1c, 0x18, 0x1c, 0x1e, 0x18},
		{0x1c, 0x1e, 0x1c, -1, 0x1e, 0x1e, 0x1c, 0x1e, 0x1f, 0x1c},
		{0x1e, 0x1f, 0x1e, 0x1f, -1, -1, -1, -1, -1, -1},
	},
}

var partitionBlockSizes = [numBlockLevels][numPartitions][2]BlockSize{
	BlockLevel128x128: {
		PartitionNone:         {Block128x128, 0},
		PartitionH:            {Block128x64, 0},
		PartitionV:            {Block64x128, 0},
		PartitionTTopSplit:    {Block64x64, Block128x64},
		PartitionTBottomSplit: {Block128x64, Block64x64},
		PartitionTLeftSplit:   {Block64x64, Block64x128},
		PartitionTRightSplit:  {Block64x128, Block64x64},
	},
	BlockLevel64x64: {
		PartitionNone:         {Block64x64, 0},
		PartitionH:            {Block64x32, 0},
		PartitionV:            {Block32x64, 0},
		PartitionTTopSplit:    {Block32x32, Block64x32},
		PartitionTBottomSplit: {Block64x32, Block32x32},
		PartitionTLeftSplit:   {Block32x32, Block32x64},
		PartitionTRightSplit:  {Block32x64, Block32x32},
		PartitionH4:           {Block64x16, 0},
		PartitionV4:           {Block16x64, 0},
	},
	BlockLevel32x32: {
		PartitionNone:         {Block32x32, 0},
		PartitionH:            {Block32x16, 0},
		PartitionV:            {Block16x32, 0},
		PartitionTTopSplit:    {Block16x16, Block32x16},
		PartitionTBottomSplit: {Block32x16, Block16x16},
		PartitionTLeftSplit:   {Block16x16, Block16x32},
		PartitionTRightSplit:  {Block16x32, Block16x16},
		PartitionH4:           {Block32x8, 0},
		PartitionV4:           {Block8x32, 0},
	},
	BlockLevel16x16: {
		PartitionNone:         {Block16x16, 0},
		PartitionH:            {Block16x8, 0},
		PartitionV:            {Block8x16, 0},
		PartitionTTopSplit:    {Block8x8, Block16x8},
		PartitionTBottomSplit: {Block16x8, Block8x8},
		PartitionTLeftSplit:   {Block8x8, Block8x16},
		PartitionTRightSplit:  {Block8x16, Block8x8},
		PartitionH4:           {Block16x4, 0},
		PartitionV4:           {Block4x16, 0},
	},
	BlockLevel8x8: {
		PartitionNone:  {Block8x8, 0},
		PartitionH:     {Block8x4, 0},
		PartitionV:     {Block4x8, 0},
		PartitionSplit: {Block4x4, 0},
	},
}

var blockDimensions = [numBlockSizes]BlockDimensions{
	Block128x128: {32, 32, 5, 5},
	Block128x64:  {32, 16, 5, 4},
	Block64x128:  {16, 32, 4, 5},
	Block64x64:   {16, 16, 4, 4},
	Block64x32:   {16, 8, 4, 3},
	Block64x16:   {16, 4, 4, 2},
	Block32x64:   {8, 16, 3, 4},
	Block32x32:   {8, 8, 3, 3},
	Block32x16:   {8, 4, 3, 2},
	Block32x8:    {8, 2, 3, 1},
	Block16x64:   {4, 16, 2, 4},
	Block16x32:   {4, 8, 2, 3},
	Block16x16:   {4, 4, 2, 2},
	Block16x8:    {4, 2, 2, 1},
	Block16x4:    {4, 1, 2, 0},
	Block8x32:    {2, 8, 1, 3},
	Block8x16:    {2, 4, 1, 2},
	Block8x8:     {2, 2, 1, 1},
	Block8x4:     {2, 1, 1, 0},
	Block4x16:    {1, 4, 0, 2},
	Block4x8:     {1, 2, 0, 1},
	Block4x4:     {1, 1, 0, 0},
}

var partitionTypeCount = [numBlockLevels]uint8{
	BlockLevel128x128: uint8(numPartitions) - 3,
	BlockLevel64x64:   uint8(numPartitions) - 1,
	BlockLevel32x32:   uint8(numPartitions) - 1,
	BlockLevel16x16:   uint8(numPartitions) - 1,
	BlockLevel8x8:     uint8(numSub8x8Partitions) - 1,
}

func RootBlockLevel(seq av1.SequenceHeader) BlockLevel {
	if seq.Use128x128SuperBlock {
		return BlockLevel128x128
	}
	return BlockLevel64x64
}

func RootBlockSize(seq av1.SequenceHeader) BlockSize {
	if seq.Use128x128SuperBlock {
		return Block128x128
	}
	return Block64x64
}

func SuperBlockSizePx(seq av1.SequenceHeader) int {
	if seq.Use128x128SuperBlock {
		return 128
	}
	return 64
}

func SuperBlockShift4(seq av1.SequenceHeader) uint8 {
	if seq.Use128x128SuperBlock {
		return 5
	}
	return 4
}

func BlockDims(size BlockSize) BlockDimensions {
	return blockDimensions[size]
}

func PartitionTypeCount(level BlockLevel) uint8 {
	return partitionTypeCount[level]
}

func PartitionChildSizes(level BlockLevel, part BlockPartition) [2]BlockSize {
	return partitionBlockSizes[level][part]
}

func AlignPartitionContext(axis int, level BlockLevel, part BlockPartition) int8 {
	return alignPartitionContext[axis][level][part]
}

func BuildTileStates(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup) ([]TileState, error) {
	if hdr == nil {
		return nil, fmt.Errorf("decoder: frame header required")
	}
	return BuildTileStatesWithInitialCDFState(seq, hdr, group, DefaultCoefQCat(hdr.Quantization.YAC), nil)
}

func BuildTileStatesWithInitialCoefQCat(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, initialCoefQCat int) ([]TileState, error) {
	return BuildTileStatesWithInitialCDFState(seq, hdr, group, initialCoefQCat, nil)
}

func BuildTileStatesWithInitialCDFState(seq av1.SequenceHeader, hdr *obu.FrameHeader, group *obu.TileGroup, initialCoefQCat int, seed *FrameCDFState) ([]TileState, error) {
	if hdr == nil {
		return nil, fmt.Errorf("decoder: frame header required")
	}
	if group == nil {
		return nil, fmt.Errorf("decoder: tile group required")
	}
	if len(group.Tiles) == 0 {
		return nil, fmt.Errorf("decoder: empty tile group")
	}

	states := make([]TileState, len(group.Tiles))
	disableCDFUpdate := hdr.DisableCDFUpdate || os.Getenv("DEBUG_FORCE_DISABLE_CDF_UPDATE") != ""
	for i, tile := range group.Tiles {
		geometry, superBlocksScratch, err := buildTileGeometry(seq, hdr, tile.Index)
		if err != nil {
			CloseTileStates(states[:i])
			return nil, err
		}
		state := &states[i]
		*state = TileState{
			Geometry:           geometry,
			Data:               tile.Data,
			Entropy:            msac.New(tile.Data, disableCDFUpdate),
			CDF:                takeDefaultModeCDF(),
			MV:                 takeDefaultMVCDF(),
			superBlocksScratch: superBlocksScratch,
		}
		seedTileCDFState(state, seed, initialCoefQCat)
	}
	return states, nil
}

func seedTileCDFState(state *TileState, seed *FrameCDFState, initialCoefQCat int) {
	if state == nil {
		return
	}
	if seed != nil {
		if seed.Mode != nil && state.CDF != nil {
			*state.CDF = *seed.Mode
		}
		if seed.MV != nil && state.MV != nil {
			*state.MV = *seed.MV
		}
		if os.Getenv("DEBUG_DISABLE_PUREGO_COEF_CDF_SEED") == "" {
			for qcat, coef := range seed.CoefByQ {
				if coef == nil {
					continue
				}
				if state.CoefByQ[qcat] == nil {
					state.CoefByQ[qcat] = takeDefaultCoefCDFForQCat(qcat)
				}
				*state.CoefByQ[qcat] = *coef
			}
		}
	}
	state.SetActiveCoefQCat(initialCoefQCat)
}

func buildTileGeometry(seq av1.SequenceHeader, hdr *obu.FrameHeader, tileIndex int) (TileGeometry, *scratchBuffer[SuperBlockGeometry], error) {
	cols := int(hdr.Tiling.Cols)
	rows := int(hdr.Tiling.Rows)
	total := cols * rows
	if cols <= 0 || rows <= 0 || tileIndex < 0 || tileIndex >= total {
		return TileGeometry{}, nil, fmt.Errorf("decoder: invalid tile index %d", tileIndex)
	}

	col := tileIndex % cols
	row := tileIndex / cols
	startSBX := int(hdr.Tiling.ColStartSB[col])
	endSBX := int(hdr.Tiling.ColStartSB[col+1])
	startSBY := int(hdr.Tiling.RowStartSB[row])
	endSBY := int(hdr.Tiling.RowStartSB[row+1])

	sbSizePx := SuperBlockSizePx(seq)
	sbShift4 := int(SuperBlockShift4(seq))
	frameWidth := hdr.Width
	if frameWidth <= 0 {
		frameWidth = hdr.UpscaledWidth
	}
	frameHeight := hdr.Height
	frameWidth4 := miGridSize4(frameWidth)
	frameHeight4 := miGridSize4(frameHeight)
	superBlockCount := maxIntLocal(0, endSBX-startSBX) * maxIntLocal(0, endSBY-startSBY)
	superBlocksScratch := takeScratch[SuperBlockGeometry](&tileSuperBlockGeometryPool, superBlockCount)
	superBlocks := superBlocksScratch.buf[:superBlockCount]

	geometry := TileGeometry{
		Index:       tileIndex,
		Col:         col,
		Row:         row,
		StartSBX:    startSBX,
		EndSBX:      endSBX,
		StartSBY:    startSBY,
		EndSBY:      endSBY,
		Start4X:     startSBX << sbShift4,
		End4X:       min(endSBX<<sbShift4, frameWidth4),
		Start4Y:     startSBY << sbShift4,
		End4Y:       min(endSBY<<sbShift4, frameHeight4),
		StartX:      startSBX * sbSizePx,
		EndX:        min(endSBX*sbSizePx, frameWidth),
		StartY:      startSBY * sbSizePx,
		EndY:        min(endSBY*sbSizePx, frameHeight),
		SuperBlocks: superBlocks,
	}

	sbIndex := 0
	for sby := startSBY; sby < endSBY; sby++ {
		for sbx := startSBX; sbx < endSBX; sbx++ {
			start4X := sbx << sbShift4
			end4X := min((sbx+1)<<sbShift4, frameWidth4)
			start4Y := sby << sbShift4
			end4Y := min((sby+1)<<sbShift4, frameHeight4)
			startX := sbx * sbSizePx
			endX := min((sbx+1)*sbSizePx, frameWidth)
			startY := sby * sbSizePx
			endY := min((sby+1)*sbSizePx, frameHeight)

			superBlocks[sbIndex] = SuperBlockGeometry{
				Index:    sbIndex,
				SBX:      sbx,
				SBY:      sby,
				Start4X:  start4X,
				Start4Y:  start4Y,
				End4X:    end4X,
				End4Y:    end4Y,
				StartX:   startX,
				StartY:   startY,
				EndX:     endX,
				EndY:     endY,
				Width4:   end4X - start4X,
				Height4:  end4Y - start4Y,
				WidthPx:  endX - startX,
				HeightPx: endY - startY,
			}
			sbIndex++
		}
	}
	if sbIndex < len(superBlocks) {
		geometry.SuperBlocks = superBlocks[:sbIndex]
		superBlocksScratch.buf = geometry.SuperBlocks
	}

	return geometry, superBlocksScratch, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func miGridSize4(pixels int) int {
	if pixels <= 0 {
		return 0
	}
	return 2 * ((pixels + 7) >> 3)
}
