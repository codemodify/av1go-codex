package obu

import (
	"fmt"

	"github.com/codemodify/av1go-codex/internal/bitio"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

const (
	primaryRefNone    = 7
	filterSwitchable  = 4
	txLargest         = 1
	txSwitchable      = 2
	wmTypeIdentity    = 0
	wmTypeTranslation = 1
	wmTypeRotZoom     = 2
	wmTypeAffine      = 3
)

type FrameType uint8

const (
	FrameTypeKey FrameType = iota
	FrameTypeInter
	FrameTypeIntra
	FrameTypeSwitch
)

type Tiling struct {
	Uniform     bool
	Cols        uint8
	Rows        uint8
	Log2Cols    uint8
	Log2Rows    uint8
	Update      uint16
	SizeBytes   uint8
	ColStartSB  [65]uint16
	RowStartSB  [65]uint16
	MinLog2Cols uint8
	MaxLog2Cols uint8
	MinLog2Rows uint8
	MaxLog2Rows uint8
}

type Quantization struct {
	YAC      uint8
	YDCDelta int8
	UDCDelta int8
	UACDelta int8
	VDCDelta int8
	VACDelta int8
	QM       bool
	QMY      uint8
	QMU      uint8
	QMV      uint8
}

type SegmentData struct {
	DeltaQ    int16
	DeltaLFYV int8
	DeltaLFYH int8
	DeltaLFU  int8
	DeltaLFV  int8
	Ref       int8
	Skip      bool
	GlobalMV  bool
}

type Segmentation struct {
	Enabled         bool
	UpdateMap       bool
	Temporal        bool
	UpdateData      bool
	Preskip         bool
	LastActiveSegID int8
	Data            [8]SegmentData
	Lossless        [8]bool
	QIndex          [8]uint8
}

type DeltaParams struct {
	QPresent  bool
	QResLog2  uint8
	LFPresent bool
	LFResLog2 uint8
	LFMulti   bool
}

type ModeRefDeltas struct {
	Mode [2]int8
	Ref  [8]int8
}

type LoopFilter struct {
	LevelY             [2]uint8
	LevelU             uint8
	LevelV             uint8
	Sharpness          uint8
	ModeRefDeltaEnable bool
	ModeRefDeltaUpdate bool
	ModeRefDeltas      ModeRefDeltas
}

type CDEF struct {
	Damping    uint8
	NBits      uint8
	YStrength  [8]uint8
	UVStrength [8]uint8
}

type Restoration struct {
	Type     [3]uint8
	UnitSize [2]uint8
}

type WarpedMotionParams struct {
	Type   uint8
	Matrix [6]int32
}

type FilmGrainParams struct {
	Update                bool
	RefIndex              uint8
	Seed                  uint16
	NumYPoints            uint8
	YPoints               [14][2]uint8
	ChromaScalingFromLuma bool
	NumUVPoints           [2]uint8
	UVPoints              [2][10][2]uint8
	ScalingShift          uint8
	ARCoeffLag            uint8
	ARCoeffsY             [24]int8
	ARCoeffsUV            [2][25]int8
	ARCoeffShift          uint8
	GrainScaleShift       uint8
	UVMult                [2]int8
	UVLumaMult            [2]int8
	UVOffset              [2]int16
	OverlapFlag           bool
	ClipToRestrictedRange bool
}

type FrameHeader struct {
	TemporalID uint8
	SpatialID  uint8

	ShowExistingFrame bool
	ExistingFrameIdx  uint8
	FrameID           uint32

	FrameType          FrameType
	ShowFrame          bool
	ShowableFrame      bool
	ErrorResilientMode bool
	DisableCDFUpdate   bool
	AllowScreenTools   bool
	ForceIntegerMV     bool
	FrameSizeOverride  bool
	FrameOffset        uint8
	PrimaryRefFrame    uint8
	RefreshFrameFlags  uint8

	Width         int
	UpscaledWidth int
	Height        int
	RenderWidth   int
	RenderHeight  int

	SuperResEnabled     bool
	SuperResDenominator uint8
	AllowIntrabc        bool
	FrameRefShortSignal bool
	RefIdx              [7]int8
	HighPrecisionMV     bool
	SubpelFilterMode    uint8
	SwitchableMotion    bool
	UseRefFrameMvs      bool
	RefreshContext      bool

	Tiling       Tiling
	Quantization Quantization
	Segmentation Segmentation
	Delta        DeltaParams
	AllLossless  bool
	LoopFilter   LoopFilter
	CDEF         CDEF
	Restoration  Restoration
	TxfmMode     uint8

	SwitchableCompRefs bool
	SkipModeAllowed    bool
	SkipModeEnabled    bool
	SkipModeRefs       [2]int8
	WarpMotion         bool
	ReducedTxtpSet     bool
	GMV                [7]WarpedMotionParams

	FilmGrainPresent bool
	FilmGrain        FilmGrainParams
	HeaderBytes      int
}

type FrameContext struct {
	Sequence av1.SequenceHeader
	Refs     [8]*FrameHeader
}

type Tile struct {
	Index int
	Data  []byte
}

type TileGroup struct {
	Start int
	End   int
	Tiles []Tile
	Data  []byte
}

func (ctx *FrameContext) ApplyRefresh(h *FrameHeader) {
	if ctx == nil || h == nil {
		return
	}
	for i := 0; i < len(ctx.Refs); i++ {
		if h.RefreshFrameFlags&(1<<i) == 0 {
			continue
		}
		clone := *h
		ctx.Refs[i] = &clone
	}
}

func ParseFrameOBU(payload []byte, ctx *FrameContext, temporalID, spatialID uint8) (*FrameHeader, []byte, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("obu: frame context required")
	}
	r := bitio.NewReader(payload)
	h := &FrameHeader{
		TemporalID:      temporalID,
		SpatialID:       spatialID,
		PrimaryRefFrame: primaryRefNone,
	}
	for i := range h.RefIdx {
		h.RefIdx[i] = -1
		h.GMV[i] = defaultWMParams()
	}
	if err := parseFrameHeader(r, h, ctx); err != nil {
		return nil, nil, err
	}
	r.ByteAlign()
	h.HeaderBytes = r.Position() / 8
	if h.HeaderBytes > len(payload) {
		return nil, nil, fmt.Errorf("obu: frame header exceeds payload")
	}
	return h, payload[h.HeaderBytes:], nil
}

func ParseTileGroup(h *FrameHeader, payload []byte) (*TileGroup, error) {
	if h == nil {
		return nil, fmt.Errorf("obu: frame header required")
	}
	r := bitio.NewReader(payload)
	nTiles := int(h.Tiling.Cols) * int(h.Tiling.Rows)
	start := 0
	end := nTiles - 1
	if nTiles > 1 {
		haveTilePos, err := r.ReadBool()
		if err != nil {
			return nil, err
		}
		if haveTilePos {
			nBits := int(h.Tiling.Log2Cols + h.Tiling.Log2Rows)
			v, err := r.ReadBits(nBits)
			if err != nil {
				return nil, err
			}
			start = int(v)
			v, err = r.ReadBits(nBits)
			if err != nil {
				return nil, err
			}
			end = int(v)
		}
	}
	r.ByteAlign()
	offset := r.Position() / 8
	if start > end || offset > len(payload) {
		return nil, fmt.Errorf("obu: invalid tile group header")
	}
	group := &TileGroup{
		Start: start,
		End:   end,
		Data:  append([]byte(nil), payload[offset:]...),
	}
	data := payload[offset:]
	size := len(data)
	for tileIndex := start; tileIndex <= end; tileIndex++ {
		tileSize := size
		if tileIndex != end {
			if int(h.Tiling.SizeBytes) > len(data) {
				return nil, fmt.Errorf("obu: truncated tile size prefix")
			}
			tileSize = 0
			for i := 0; i < int(h.Tiling.SizeBytes); i++ {
				tileSize |= int(data[i]) << (8 * i)
			}
			tileSize++
			data = data[h.Tiling.SizeBytes:]
			size -= int(h.Tiling.SizeBytes)
			if tileSize > len(data) {
				return nil, fmt.Errorf("obu: tile size exceeds payload")
			}
		}
		group.Tiles = append(group.Tiles, Tile{
			Index: tileIndex,
			Data:  append([]byte(nil), data[:tileSize]...),
		})
		data = data[tileSize:]
		size -= tileSize
	}
	return group, nil
}

func parseFrameHeader(r *bitio.Reader, h *FrameHeader, ctx *FrameContext) error {
	seq := ctx.Sequence

	if !seq.ReducedStillPicture {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.ShowExistingFrame = v
	}
	if h.ShowExistingFrame {
		idx, err := r.ReadUint8(3)
		if err != nil {
			return err
		}
		h.ExistingFrameIdx = idx
		if seq.FrameIDNumbersPresentFlag {
			bits := frameIDBits(seq)
			id, err := r.ReadBits(bits)
			if err != nil {
				return err
			}
			h.FrameID = uint32(id)
		}
		return nil
	}

	if seq.ReducedStillPicture {
		h.FrameType = FrameTypeKey
		h.ShowFrame = true
	} else {
		ft, err := r.ReadUint8(2)
		if err != nil {
			return err
		}
		h.FrameType = FrameType(ft)
		show, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.ShowFrame = show
	}
	if h.ShowFrame {
		h.ShowableFrame = h.FrameType != FrameTypeKey
	} else {
		showable, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.ShowableFrame = showable
	}

	if h.FrameType == FrameTypeKey && h.ShowFrame || h.FrameType == FrameTypeSwitch || seq.ReducedStillPicture {
		h.ErrorResilientMode = true
	} else {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.ErrorResilientMode = v
	}

	disableCDF, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.DisableCDFUpdate = disableCDF

	screenToolsVal := sequenceScreenContentTools(seq)
	if screenToolsVal == 2 {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.AllowScreenTools = v
	} else {
		h.AllowScreenTools = screenToolsVal != 0
	}
	if h.AllowScreenTools {
		intMVVal := sequenceForceIntegerMV(seq)
		if intMVVal == 2 {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			h.ForceIntegerMV = v
		} else {
			h.ForceIntegerMV = intMVVal != 0
		}
	}
	if isKeyOrIntra(h.FrameType) {
		h.ForceIntegerMV = true
	}

	if seq.FrameIDNumbersPresentFlag {
		id, err := r.ReadBits(frameIDBits(seq))
		if err != nil {
			return err
		}
		h.FrameID = uint32(id)
	}

	if !seq.ReducedStillPicture {
		if h.FrameType == FrameTypeSwitch {
			h.FrameSizeOverride = true
		} else {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			h.FrameSizeOverride = v
		}
	}
	if seq.EnableOrderHint {
		v, err := r.ReadBits(orderHintBits(seq))
		if err != nil {
			return err
		}
		h.FrameOffset = uint8(v)
	}
	if !h.ErrorResilientMode && isInterOrSwitch(h.FrameType) {
		v, err := r.ReadUint8(3)
		if err != nil {
			return err
		}
		h.PrimaryRefFrame = v
	}

	if isKeyOrIntra(h.FrameType) {
		if h.FrameType == FrameTypeKey && h.ShowFrame {
			h.RefreshFrameFlags = 0xff
		} else {
			v, err := r.ReadUint8(8)
			if err != nil {
				return err
			}
			h.RefreshFrameFlags = v
		}
		if h.RefreshFrameFlags != 0xff && h.ErrorResilientMode && seq.EnableOrderHint {
			if err := r.SkipBits(orderHintBits(seq) * 8); err != nil {
				return err
			}
		}
		if err := readFrameSize(r, h, ctx, false); err != nil {
			return err
		}
		if h.AllowScreenTools && !h.SuperResEnabled {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			h.AllowIntrabc = v
		}
	} else {
		if h.FrameType == FrameTypeSwitch {
			h.RefreshFrameFlags = 0xff
		} else {
			v, err := r.ReadUint8(8)
			if err != nil {
				return err
			}
			h.RefreshFrameFlags = v
		}
		if h.ErrorResilientMode && seq.EnableOrderHint {
			if err := r.SkipBits(orderHintBits(seq) * 8); err != nil {
				return err
			}
		}
		if seq.EnableOrderHint {
			shortSignal, err := r.ReadBool()
			if err != nil {
				return err
			}
			h.FrameRefShortSignal = shortSignal
			if shortSignal {
				if err := resolveShortRefs(r, h, ctx); err != nil {
					return err
				}
			}
		}
		for i := 0; i < 7; i++ {
			if !h.FrameRefShortSignal {
				v, err := r.ReadUint8(3)
				if err != nil {
					return err
				}
				h.RefIdx[i] = int8(v)
			}
			if seq.FrameIDNumbersPresentFlag {
				v, err := r.ReadBits(deltaFrameIDBits(seq))
				if err != nil {
					return err
				}
				if ref := ctx.Refs[h.RefIdx[i]]; ref != nil {
					mod := uint32(1 << frameIDBits(seq))
					refID := (h.FrameID + mod - (uint32(v) + 1)) & (mod - 1)
					if ref.FrameID != refID {
						return fmt.Errorf("obu: frame id mismatch for ref %d", i)
					}
				}
			}
		}
		useRef := !h.ErrorResilientMode && h.FrameSizeOverride
		if err := readFrameSize(r, h, ctx, useRef); err != nil {
			return err
		}
		if !h.ForceIntegerMV {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			h.HighPrecisionMV = v
		}
		switchable, err := r.ReadBool()
		if err != nil {
			return err
		}
		if switchable {
			h.SubpelFilterMode = filterSwitchable
		} else {
			v, err := r.ReadUint8(2)
			if err != nil {
				return err
			}
			h.SubpelFilterMode = v
		}
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.SwitchableMotion = v
		if !h.ErrorResilientMode && seq.EnableRefFrameMvs && seq.EnableOrderHint {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			h.UseRefFrameMvs = v
		}
	}

	if !seq.ReducedStillPicture && !h.DisableCDFUpdate {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.RefreshContext = !v
	}

	if err := parseTiling(r, h, seq); err != nil {
		return err
	}
	if err := parseQuantization(r, h, seq); err != nil {
		return err
	}
	if err := parseSegmentation(r, h, ctx, seq); err != nil {
		return err
	}
	if err := parseDeltaAndLossless(r, h, seq); err != nil {
		return err
	}
	if err := parseLoopFilter(r, h, ctx, seq); err != nil {
		return err
	}
	if err := parseCDEF(r, h, seq); err != nil {
		return err
	}
	if err := parseRestoration(r, h, seq); err != nil {
		return err
	}
	if !h.AllLossless {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		if v {
			h.TxfmMode = txSwitchable
		} else {
			h.TxfmMode = txLargest
		}
	}
	if isInterOrSwitch(h.FrameType) {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.SwitchableCompRefs = v
	}
	if h.SwitchableCompRefs && isInterOrSwitch(h.FrameType) && seq.EnableOrderHint {
		resolveSkipMode(h, ctx, seq)
	}
	if h.SkipModeAllowed {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.SkipModeEnabled = v
	}
	if !h.ErrorResilientMode && isInterOrSwitch(h.FrameType) && seq.EnableWarpedMotion {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.WarpMotion = v
	}
	v, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.ReducedTxtpSet = v

	if isInterOrSwitch(h.FrameType) {
		if err := parseGlobalMotion(r, h, ctx, seq); err != nil {
			return err
		}
	}

	if seq.FilmGrainParamsPresent && (h.ShowFrame || h.ShowableFrame) {
		if err := parseFilmGrain(r, h, ctx, seq); err != nil {
			return err
		}
	}

	return nil
}

func parseFilmGrain(r *bitio.Reader, h *FrameHeader, ctx *FrameContext, seq av1.SequenceHeader) error {
	present, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.FilmGrainPresent = present
	if !present {
		h.FilmGrain = FilmGrainParams{}
		return nil
	}

	var grain FilmGrainParams
	seed, err := r.ReadUint16(16)
	if err != nil {
		return err
	}
	grain.Seed = seed

	if h.FrameType != FrameTypeInter {
		grain.Update = true
	} else {
		grain.Update, err = r.ReadBool()
		if err != nil {
			return err
		}
	}

	if !grain.Update {
		refIndex, err := r.ReadUint8(3)
		if err != nil {
			return err
		}
		grain.RefIndex = refIndex
		if !frameUsesRefSlot(h, refIndex) {
			return fmt.Errorf("obu: film grain ref %d not used by frame", refIndex)
		}
		ref := ctx.ref(int(refIndex))
		if ref == nil {
			return fmt.Errorf("obu: missing film grain ref %d", refIndex)
		}
		grain = ref.FilmGrain
		grain.Update = false
		grain.RefIndex = refIndex
		grain.Seed = seed
		h.FilmGrain = grain
		return nil
	}

	numYPoints, err := r.ReadUint8(4)
	if err != nil {
		return err
	}
	if numYPoints > 14 {
		return fmt.Errorf("obu: invalid film grain y point count %d", numYPoints)
	}
	grain.NumYPoints = numYPoints
	for i := 0; i < int(grain.NumYPoints); i++ {
		x, err := r.ReadUint8(8)
		if err != nil {
			return err
		}
		y, err := r.ReadUint8(8)
		if err != nil {
			return err
		}
		if i > 0 && grain.YPoints[i-1][0] >= x {
			return fmt.Errorf("obu: film grain y points not strictly increasing")
		}
		grain.YPoints[i][0] = x
		grain.YPoints[i][1] = y
	}

	if !seq.ColorConfig.Monochrome {
		grain.ChromaScalingFromLuma, err = r.ReadBool()
		if err != nil {
			return err
		}
	}

	subsampling420 := seq.ColorConfig.SubsamplingX && seq.ColorConfig.SubsamplingY
	if seq.ColorConfig.Monochrome || grain.ChromaScalingFromLuma || (subsampling420 && grain.NumYPoints == 0) {
		grain.NumUVPoints[0] = 0
		grain.NumUVPoints[1] = 0
	} else {
		for pl := 0; pl < 2; pl++ {
			n, err := r.ReadUint8(4)
			if err != nil {
				return err
			}
			if n > 10 {
				return fmt.Errorf("obu: invalid film grain uv point count %d", n)
			}
			grain.NumUVPoints[pl] = n
			for i := 0; i < int(n); i++ {
				x, err := r.ReadUint8(8)
				if err != nil {
					return err
				}
				y, err := r.ReadUint8(8)
				if err != nil {
					return err
				}
				if i > 0 && grain.UVPoints[pl][i-1][0] >= x {
					return fmt.Errorf("obu: film grain uv points not strictly increasing")
				}
				grain.UVPoints[pl][i][0] = x
				grain.UVPoints[pl][i][1] = y
			}
		}
	}

	if subsampling420 && (grain.NumUVPoints[0] > 0) != (grain.NumUVPoints[1] > 0) {
		return fmt.Errorf("obu: film grain 4:2:0 uv point presence mismatch")
	}

	scalingShift, err := r.ReadUint8(2)
	if err != nil {
		return err
	}
	grain.ScalingShift = scalingShift + 8

	arCoeffLag, err := r.ReadUint8(2)
	if err != nil {
		return err
	}
	grain.ARCoeffLag = arCoeffLag
	numYPos := 2 * int(grain.ARCoeffLag) * (int(grain.ARCoeffLag) + 1)
	if grain.NumYPoints > 0 {
		for i := 0; i < numYPos; i++ {
			v, err := r.ReadUint8(8)
			if err != nil {
				return err
			}
			grain.ARCoeffsY[i] = int8(int(v) - 128)
		}
	}
	for pl := 0; pl < 2; pl++ {
		if grain.NumUVPoints[pl] == 0 && !grain.ChromaScalingFromLuma {
			continue
		}
		numUVPos := numYPos
		if grain.NumYPoints > 0 {
			numUVPos++
		}
		for i := 0; i < numUVPos; i++ {
			v, err := r.ReadUint8(8)
			if err != nil {
				return err
			}
			grain.ARCoeffsUV[pl][i] = int8(int(v) - 128)
		}
		if grain.NumYPoints == 0 {
			grain.ARCoeffsUV[pl][numUVPos] = 0
		}
	}

	arCoeffShift, err := r.ReadUint8(2)
	if err != nil {
		return err
	}
	grain.ARCoeffShift = arCoeffShift + 6
	grainScaleShift, err := r.ReadUint8(2)
	if err != nil {
		return err
	}
	grain.GrainScaleShift = grainScaleShift

	for pl := 0; pl < 2; pl++ {
		if grain.NumUVPoints[pl] == 0 {
			continue
		}
		uvMult, err := r.ReadUint8(8)
		if err != nil {
			return err
		}
		uvLumaMult, err := r.ReadUint8(8)
		if err != nil {
			return err
		}
		uvOffset, err := r.ReadUint16(9)
		if err != nil {
			return err
		}
		grain.UVMult[pl] = int8(int(uvMult) - 128)
		grain.UVLumaMult[pl] = int8(int(uvLumaMult) - 128)
		grain.UVOffset[pl] = int16(int(uvOffset) - 256)
	}

	grain.OverlapFlag, err = r.ReadBool()
	if err != nil {
		return err
	}
	grain.ClipToRestrictedRange, err = r.ReadBool()
	if err != nil {
		return err
	}

	h.FilmGrain = grain
	return nil
}

func frameUsesRefSlot(h *FrameHeader, refIndex uint8) bool {
	for _, idx := range h.RefIdx {
		if idx == int8(refIndex) {
			return true
		}
	}
	return false
}

func readFrameSize(r *bitio.Reader, h *FrameHeader, ctx *FrameContext, useRef bool) error {
	seq := ctx.Sequence
	if useRef {
		for i := 0; i < 7; i++ {
			use, err := r.ReadBool()
			if err != nil {
				return err
			}
			if !use {
				continue
			}
			ref := ctx.ref(int(h.RefIdx[i]))
			if ref == nil {
				return fmt.Errorf("obu: missing reference frame %d", i)
			}
			h.UpscaledWidth = ref.UpscaledWidth
			h.Height = ref.Height
			h.RenderWidth = ref.RenderWidth
			h.RenderHeight = ref.RenderHeight
			if seq.EnableSuperRes {
				v, err := r.ReadBool()
				if err != nil {
					return err
				}
				h.SuperResEnabled = v
			}
			if h.SuperResEnabled {
				den, err := r.ReadUint8(3)
				if err != nil {
					return err
				}
				h.SuperResDenominator = 9 + den
				h.Width = scaledWidth(h.UpscaledWidth, h.SuperResDenominator)
			} else {
				h.SuperResDenominator = 8
				h.Width = h.UpscaledWidth
			}
			return nil
		}
	}

	if h.FrameSizeOverride {
		v, err := r.ReadBits(int(seq.FrameWidthBitsMinus1) + 1)
		if err != nil {
			return err
		}
		h.UpscaledWidth = int(v) + 1
		v, err = r.ReadBits(int(seq.FrameHeightBitsMinus1) + 1)
		if err != nil {
			return err
		}
		h.Height = int(v) + 1
	} else {
		h.UpscaledWidth = seq.Width()
		h.Height = seq.Height()
	}

	if seq.EnableSuperRes {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.SuperResEnabled = v
	}
	if h.SuperResEnabled {
		den, err := r.ReadUint8(3)
		if err != nil {
			return err
		}
		h.SuperResDenominator = 9 + den
		h.Width = scaledWidth(h.UpscaledWidth, h.SuperResDenominator)
	} else {
		h.SuperResDenominator = 8
		h.Width = h.UpscaledWidth
	}
	haveRender, err := r.ReadBool()
	if err != nil {
		return err
	}
	if haveRender {
		v, err := r.ReadBits(16)
		if err != nil {
			return err
		}
		h.RenderWidth = int(v) + 1
		v, err = r.ReadBits(16)
		if err != nil {
			return err
		}
		h.RenderHeight = int(v) + 1
	} else {
		h.RenderWidth = h.UpscaledWidth
		h.RenderHeight = h.Height
	}
	return nil
}

func parseTiling(r *bitio.Reader, h *FrameHeader, seq av1.SequenceHeader) error {
	v, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.Tiling.Uniform = v
	sbszMin1 := (64 << boolToInt(seq.Use128x128SuperBlock)) - 1
	sbszLog2 := 6 + boolToInt(seq.Use128x128SuperBlock)
	sbw := (h.Width + sbszMin1) >> sbszLog2
	sbh := (h.Height + sbszMin1) >> sbszLog2
	maxTileWidthSB := 4096 >> sbszLog2
	maxTileAreaSB := (4096 * 2304) >> (2 * sbszLog2)
	h.Tiling.MinLog2Cols = uint8(tileLog2(maxTileWidthSB, sbw))
	h.Tiling.MaxLog2Cols = uint8(tileLog2(1, min(sbw, 64)))
	h.Tiling.MaxLog2Rows = uint8(tileLog2(1, min(sbh, 64)))
	minLog2Tiles := max(tileLog2(maxTileAreaSB, sbw*sbh), int(h.Tiling.MinLog2Cols))
	if h.Tiling.Uniform {
		h.Tiling.Log2Cols = h.Tiling.MinLog2Cols
		for h.Tiling.Log2Cols < h.Tiling.MaxLog2Cols {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			if !v {
				break
			}
			h.Tiling.Log2Cols++
		}
		tileW := 1 + ((sbw - 1) >> h.Tiling.Log2Cols)
		for sbx := 0; sbx < sbw; sbx += tileW {
			h.Tiling.ColStartSB[h.Tiling.Cols] = uint16(sbx)
			h.Tiling.Cols++
		}
		h.Tiling.MinLog2Rows = uint8(max(minLog2Tiles-int(h.Tiling.Log2Cols), 0))
		h.Tiling.Log2Rows = h.Tiling.MinLog2Rows
		for h.Tiling.Log2Rows < h.Tiling.MaxLog2Rows {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			if !v {
				break
			}
			h.Tiling.Log2Rows++
		}
		tileH := 1 + ((sbh - 1) >> h.Tiling.Log2Rows)
		for sby := 0; sby < sbh; sby += tileH {
			h.Tiling.RowStartSB[h.Tiling.Rows] = uint16(sby)
			h.Tiling.Rows++
		}
	} else {
		maxTileAreaSB2 := sbw * sbh
		widestTile := 0
		for sbx := 0; sbx < sbw && h.Tiling.Cols < 64; {
			tileWidthSB := min(sbw-sbx, maxTileWidthSB)
			tileW := 1
			if tileWidthSB > 1 {
				v, err := r.ReadUniform(uint32(tileWidthSB))
				if err != nil {
					return err
				}
				tileW = 1 + int(v)
			}
			h.Tiling.ColStartSB[h.Tiling.Cols] = uint16(sbx)
			h.Tiling.Cols++
			sbx += tileW
			widestTile = max(widestTile, tileW)
		}
		h.Tiling.Log2Cols = uint8(tileLog2(1, int(h.Tiling.Cols)))
		if minLog2Tiles > 0 {
			maxTileAreaSB2 >>= minLog2Tiles + 1
		}
		maxTileHeightSB := max(maxTileAreaSB2/max(widestTile, 1), 1)
		for sby := 0; sby < sbh && h.Tiling.Rows < 64; {
			tileHeightSB := min(sbh-sby, maxTileHeightSB)
			tileH := 1
			if tileHeightSB > 1 {
				v, err := r.ReadUniform(uint32(tileHeightSB))
				if err != nil {
					return err
				}
				tileH = 1 + int(v)
			}
			h.Tiling.RowStartSB[h.Tiling.Rows] = uint16(sby)
			h.Tiling.Rows++
			sby += tileH
		}
		h.Tiling.Log2Rows = uint8(tileLog2(1, int(h.Tiling.Rows)))
	}
	h.Tiling.ColStartSB[h.Tiling.Cols] = uint16(sbw)
	h.Tiling.RowStartSB[h.Tiling.Rows] = uint16(sbh)
	if h.Tiling.Log2Cols != 0 || h.Tiling.Log2Rows != 0 {
		v, err := r.ReadBits(int(h.Tiling.Log2Cols + h.Tiling.Log2Rows))
		if err != nil {
			return err
		}
		h.Tiling.Update = uint16(v)
		if int(h.Tiling.Update) >= int(h.Tiling.Cols)*int(h.Tiling.Rows) {
			return fmt.Errorf("obu: invalid tile update index")
		}
		v, err = r.ReadBits(2)
		if err != nil {
			return err
		}
		h.Tiling.SizeBytes = uint8(v) + 1
	}
	return nil
}

func parseQuantization(r *bitio.Reader, h *FrameHeader, seq av1.SequenceHeader) error {
	v, err := r.ReadUint8(8)
	if err != nil {
		return err
	}
	h.Quantization.YAC = v
	if flag, err := r.ReadBool(); err != nil {
		return err
	} else if flag {
		v, err := r.ReadSBits(7)
		if err != nil {
			return err
		}
		h.Quantization.YDCDelta = int8(v)
	}
	if !seq.ColorConfig.Monochrome {
		diffUVDelta := false
		if seq.ColorConfig.SeparateUVDeltaQ {
			v, err := r.ReadBool()
			if err != nil {
				return err
			}
			diffUVDelta = v
		}
		if flag, err := r.ReadBool(); err != nil {
			return err
		} else if flag {
			v, err := r.ReadSBits(7)
			if err != nil {
				return err
			}
			h.Quantization.UDCDelta = int8(v)
		}
		if flag, err := r.ReadBool(); err != nil {
			return err
		} else if flag {
			v, err := r.ReadSBits(7)
			if err != nil {
				return err
			}
			h.Quantization.UACDelta = int8(v)
		}
		if diffUVDelta {
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(7)
				if err != nil {
					return err
				}
				h.Quantization.VDCDelta = int8(v)
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(7)
				if err != nil {
					return err
				}
				h.Quantization.VACDelta = int8(v)
			}
		} else {
			h.Quantization.VDCDelta = h.Quantization.UDCDelta
			h.Quantization.VACDelta = h.Quantization.UACDelta
		}
	}
	flag, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.Quantization.QM = flag
	if flag {
		v, err := r.ReadUint8(4)
		if err != nil {
			return err
		}
		h.Quantization.QMY = v
		v, err = r.ReadUint8(4)
		if err != nil {
			return err
		}
		h.Quantization.QMU = v
		if seq.ColorConfig.SeparateUVDeltaQ {
			v, err = r.ReadUint8(4)
			if err != nil {
				return err
			}
			h.Quantization.QMV = v
		} else {
			h.Quantization.QMV = h.Quantization.QMU
		}
	}
	return nil
}

func parseSegmentation(r *bitio.Reader, h *FrameHeader, ctx *FrameContext, seq av1.SequenceHeader) error {
	v, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.Segmentation.Enabled = v
	if !v {
		for i := range h.Segmentation.Data {
			h.Segmentation.Data[i].Ref = -1
		}
		return nil
	}
	if h.PrimaryRefFrame == primaryRefNone {
		h.Segmentation.UpdateMap = true
		h.Segmentation.UpdateData = true
	} else {
		if v, err := r.ReadBool(); err != nil {
			return err
		} else {
			h.Segmentation.UpdateMap = v
		}
		if h.Segmentation.UpdateMap {
			if v, err := r.ReadBool(); err != nil {
				return err
			} else {
				h.Segmentation.Temporal = v
			}
		}
		if v, err := r.ReadBool(); err != nil {
			return err
		} else {
			h.Segmentation.UpdateData = v
		}
	}
	h.Segmentation.LastActiveSegID = -1
	if h.Segmentation.UpdateData {
		for i := 0; i < 8; i++ {
			seg := &h.Segmentation.Data[i]
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(9)
				if err != nil {
					return err
				}
				seg.DeltaQ = int16(v)
				h.Segmentation.LastActiveSegID = int8(i)
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(7)
				if err != nil {
					return err
				}
				seg.DeltaLFYV = int8(v)
				h.Segmentation.LastActiveSegID = int8(i)
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(7)
				if err != nil {
					return err
				}
				seg.DeltaLFYH = int8(v)
				h.Segmentation.LastActiveSegID = int8(i)
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(7)
				if err != nil {
					return err
				}
				seg.DeltaLFU = int8(v)
				h.Segmentation.LastActiveSegID = int8(i)
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadSBits(7)
				if err != nil {
					return err
				}
				seg.DeltaLFV = int8(v)
				h.Segmentation.LastActiveSegID = int8(i)
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else if flag {
				v, err := r.ReadUint8(3)
				if err != nil {
					return err
				}
				seg.Ref = int8(v)
				h.Segmentation.LastActiveSegID = int8(i)
				h.Segmentation.Preskip = true
			} else {
				seg.Ref = -1
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else {
				seg.Skip = flag
				if flag {
					h.Segmentation.LastActiveSegID = int8(i)
					h.Segmentation.Preskip = true
				}
			}
			if flag, err := r.ReadBool(); err != nil {
				return err
			} else {
				seg.GlobalMV = flag
				if flag {
					h.Segmentation.LastActiveSegID = int8(i)
					h.Segmentation.Preskip = true
				}
			}
		}
		return nil
	}
	if h.PrimaryRefFrame == primaryRefNone {
		return fmt.Errorf("obu: segmentation update_data false without primary ref")
	}
	ref := ctx.ref(int(h.RefIdx[h.PrimaryRefFrame]))
	if ref == nil {
		return fmt.Errorf("obu: missing primary reference for segmentation")
	}
	h.Segmentation = ref.Segmentation
	return nil
}

func parseDeltaAndLossless(r *bitio.Reader, h *FrameHeader, seq av1.SequenceHeader) error {
	if h.Quantization.YAC != 0 {
		v, err := r.ReadBool()
		if err != nil {
			return err
		}
		h.Delta.QPresent = v
		if h.Delta.QPresent {
			v, err := r.ReadUint8(2)
			if err != nil {
				return err
			}
			h.Delta.QResLog2 = v
			if !h.AllowIntrabc {
				v, err := r.ReadBool()
				if err != nil {
					return err
				}
				h.Delta.LFPresent = v
				if h.Delta.LFPresent {
					v, err := r.ReadUint8(2)
					if err != nil {
						return err
					}
					h.Delta.LFResLog2 = v
					multi, err := r.ReadBool()
					if err != nil {
						return err
					}
					h.Delta.LFMulti = multi
				}
			}
		}
	}
	deltaLossless := h.Quantization.YDCDelta == 0 &&
		h.Quantization.UDCDelta == 0 &&
		h.Quantization.UACDelta == 0 &&
		h.Quantization.VDCDelta == 0 &&
		h.Quantization.VACDelta == 0
	h.AllLossless = true
	for i := 0; i < 8; i++ {
		qidx := int(h.Quantization.YAC)
		if h.Segmentation.Enabled {
			qidx += int(h.Segmentation.Data[i].DeltaQ)
		}
		qidx = clipU8(qidx)
		h.Segmentation.QIndex[i] = uint8(qidx)
		h.Segmentation.Lossless[i] = qidx == 0 && deltaLossless
		h.AllLossless = h.AllLossless && h.Segmentation.Lossless[i]
	}
	return nil
}

func parseLoopFilter(r *bitio.Reader, h *FrameHeader, ctx *FrameContext, seq av1.SequenceHeader) error {
	if h.AllLossless || h.AllowIntrabc {
		h.LoopFilter.ModeRefDeltaEnable = true
		h.LoopFilter.ModeRefDeltaUpdate = true
		h.LoopFilter.ModeRefDeltas = defaultModeRefDeltas()
		return nil
	}
	v, err := r.ReadUint8(6)
	if err != nil {
		return err
	}
	h.LoopFilter.LevelY[0] = v
	v, err = r.ReadUint8(6)
	if err != nil {
		return err
	}
	h.LoopFilter.LevelY[1] = v
	if !seq.ColorConfig.Monochrome && (h.LoopFilter.LevelY[0] != 0 || h.LoopFilter.LevelY[1] != 0) {
		v, err = r.ReadUint8(6)
		if err != nil {
			return err
		}
		h.LoopFilter.LevelU = v
		v, err = r.ReadUint8(6)
		if err != nil {
			return err
		}
		h.LoopFilter.LevelV = v
	}
	v, err = r.ReadUint8(3)
	if err != nil {
		return err
	}
	h.LoopFilter.Sharpness = v

	if h.PrimaryRefFrame == primaryRefNone {
		h.LoopFilter.ModeRefDeltas = defaultModeRefDeltas()
	} else {
		ref := ctx.ref(int(h.RefIdx[h.PrimaryRefFrame]))
		if ref == nil {
			return fmt.Errorf("obu: missing primary reference for loopfilter")
		}
		h.LoopFilter.ModeRefDeltas = ref.LoopFilter.ModeRefDeltas
	}
	vb, err := r.ReadBool()
	if err != nil {
		return err
	}
	h.LoopFilter.ModeRefDeltaEnable = vb
	if vb {
		vb, err = r.ReadBool()
		if err != nil {
			return err
		}
		h.LoopFilter.ModeRefDeltaUpdate = vb
		if vb {
			for i := 0; i < 8; i++ {
				flag, err := r.ReadBool()
				if err != nil {
					return err
				}
				if flag {
					v, err := r.ReadSBits(7)
					if err != nil {
						return err
					}
					h.LoopFilter.ModeRefDeltas.Ref[i] = int8(v)
				}
			}
			for i := 0; i < 2; i++ {
				flag, err := r.ReadBool()
				if err != nil {
					return err
				}
				if flag {
					v, err := r.ReadSBits(7)
					if err != nil {
						return err
					}
					h.LoopFilter.ModeRefDeltas.Mode[i] = int8(v)
				}
			}
		}
	}
	return nil
}

func parseCDEF(r *bitio.Reader, h *FrameHeader, seq av1.SequenceHeader) error {
	if h.AllLossless || !seq.EnableCDEF || h.AllowIntrabc {
		return nil
	}
	v, err := r.ReadUint8(2)
	if err != nil {
		return err
	}
	h.CDEF.Damping = v + 3
	v, err = r.ReadUint8(2)
	if err != nil {
		return err
	}
	h.CDEF.NBits = v
	n := 1 << h.CDEF.NBits
	for i := 0; i < n; i++ {
		v, err := r.ReadUint8(6)
		if err != nil {
			return err
		}
		h.CDEF.YStrength[i] = v
		if !seq.ColorConfig.Monochrome {
			v, err = r.ReadUint8(6)
			if err != nil {
				return err
			}
			h.CDEF.UVStrength[i] = v
		}
	}
	return nil
}

func parseRestoration(r *bitio.Reader, h *FrameHeader, seq av1.SequenceHeader) error {
	if (h.AllLossless && !h.SuperResEnabled) || !seq.EnableRestoration || h.AllowIntrabc {
		return nil
	}
	for i := 0; i < 1; i++ {
		v, err := r.ReadUint8(2)
		if err != nil {
			return err
		}
		h.Restoration.Type[i] = v
	}
	if !seq.ColorConfig.Monochrome {
		v, err := r.ReadUint8(2)
		if err != nil {
			return err
		}
		h.Restoration.Type[1] = v
		v, err = r.ReadUint8(2)
		if err != nil {
			return err
		}
		h.Restoration.Type[2] = v
	}
	if h.Restoration.Type[0] != 0 || h.Restoration.Type[1] != 0 || h.Restoration.Type[2] != 0 {
		h.Restoration.UnitSize[0] = uint8(6 + boolToInt(seq.Use128x128SuperBlock))
		flag, err := r.ReadBool()
		if err != nil {
			return err
		}
		if flag {
			h.Restoration.UnitSize[0]++
			if !seq.Use128x128SuperBlock {
				flag, err = r.ReadBool()
				if err != nil {
					return err
				}
				if flag {
					h.Restoration.UnitSize[0]++
				}
			}
		}
		h.Restoration.UnitSize[1] = h.Restoration.UnitSize[0]
		if (h.Restoration.Type[1] != 0 || h.Restoration.Type[2] != 0) &&
			seq.ColorConfig.SubsamplingX && seq.ColorConfig.SubsamplingY {
			flag, err = r.ReadBool()
			if err != nil {
				return err
			}
			if flag {
				h.Restoration.UnitSize[1]--
			}
		}
	} else {
		h.Restoration.UnitSize[0] = 8
	}
	return nil
}

func parseGlobalMotion(r *bitio.Reader, h *FrameHeader, ctx *FrameContext, seq av1.SequenceHeader) error {
	for i := 0; i < 7; i++ {
		first, err := r.ReadBool()
		if err != nil {
			return err
		}
		if !first {
			h.GMV[i].Type = wmTypeIdentity
			continue
		}
		second, err := r.ReadBool()
		if err != nil {
			return err
		}
		if second {
			h.GMV[i].Type = wmTypeRotZoom
		} else {
			third, err := r.ReadBool()
			if err != nil {
				return err
			}
			if third {
				h.GMV[i].Type = wmTypeTranslation
			} else {
				h.GMV[i].Type = wmTypeAffine
			}
		}
		refGMV := defaultWMParams()
		if h.PrimaryRefFrame != primaryRefNone {
			if ref := ctx.ref(int(h.RefIdx[h.PrimaryRefFrame])); ref != nil {
				refGMV = ref.GMV[i]
			}
		}
		mat := &h.GMV[i].Matrix
		refMat := refGMV.Matrix
		bits := 0
		shift := 0
		if h.GMV[i].Type >= wmTypeRotZoom {
			v, err := r.ReadSubexp(int((refMat[2]-(1<<16))>>1), 12)
			if err != nil {
				return err
			}
			mat[2] = int32((1 << 16) + 2*v)
			v, err = r.ReadSubexp(int(refMat[3]>>1), 12)
			if err != nil {
				return err
			}
			mat[3] = int32(2 * v)
			bits = 12
			shift = 10
		} else {
			bits = 9
			shift = 13
			if !h.HighPrecisionMV {
				bits--
				shift++
			}
		}
		if h.GMV[i].Type == wmTypeAffine {
			v, err := r.ReadSubexp(int(refMat[4]>>1), 12)
			if err != nil {
				return err
			}
			mat[4] = int32(2 * v)
			v, err = r.ReadSubexp(int((refMat[5]-(1<<16))>>1), 12)
			if err != nil {
				return err
			}
			mat[5] = int32((1 << 16) + 2*v)
		} else {
			mat[4] = -mat[3]
			mat[5] = mat[2]
		}
		v, err := r.ReadSubexp(int(refMat[0]>>shift), uint(bits))
		if err != nil {
			return err
		}
		mat[0] = int32(v * (1 << shift))
		v, err = r.ReadSubexp(int(refMat[1]>>shift), uint(bits))
		if err != nil {
			return err
		}
		mat[1] = int32(v * (1 << shift))
	}
	return nil
}

func resolveShortRefs(r *bitio.Reader, h *FrameHeader, ctx *FrameContext) error {
	a, err := r.ReadUint8(3)
	if err != nil {
		return err
	}
	b, err := r.ReadUint8(3)
	if err != nil {
		return err
	}
	h.RefIdx[0] = int8(a)
	h.RefIdx[1], h.RefIdx[2] = -1, -1
	h.RefIdx[3] = int8(b)

	frameOffset := make([]int, 9)
	for i := range frameOffset {
		frameOffset[i] = -1 << 30
	}
	earliestRef := -1
	earliestOffset := 1 << 30
	for i := 0; i < 8; i++ {
		ref := ctx.Refs[i]
		if ref == nil {
			return fmt.Errorf("obu: missing reference frame %d", i)
		}
		diff := getPOCDiff(orderHintBits(ctx.Sequence), int(ref.FrameOffset), int(h.FrameOffset))
		frameOffset[i] = diff
		if diff < earliestOffset {
			earliestOffset = diff
			earliestRef = i
		}
	}
	frameOffset[h.RefIdx[0]] = -1 << 30
	frameOffset[h.RefIdx[3]] = -1 << 30

	refidx := -1
	latestOffset := 0
	for i := 0; i < 8; i++ {
		if frameOffset[i] >= latestOffset {
			latestOffset = frameOffset[i]
			refidx = i
		}
	}
	frameOffset[refidx] = -1 << 30
	h.RefIdx[6] = int8(refidx)

	for i := 4; i < 6; i++ {
		refidx = -1
		earliest := 255
		for j := 0; j < 8; j++ {
			hint := frameOffset[j]
			if hint < earliest {
				earliest = hint
				refidx = j
			}
		}
		frameOffset[refidx] = -1 << 30
		h.RefIdx[i] = int8(refidx)
	}
	for i := 1; i < 7; i++ {
		refidx = int(h.RefIdx[i])
		if refidx >= 0 {
			continue
		}
		latest := ^uint8(0)
		for j := 0; j < 8; j++ {
			hint := frameOffset[j]
			if hint >= int(latest) {
				latest = uint8(hint)
				refidx = j
			}
		}
		frameOffset[refidx] = -1 << 30
		if refidx >= 0 {
			h.RefIdx[i] = int8(refidx)
		} else {
			h.RefIdx[i] = int8(earliestRef)
		}
	}
	return nil
}

func resolveSkipMode(h *FrameHeader, ctx *FrameContext, seq av1.SequenceHeader) {
	poc := int(h.FrameOffset)
	offBefore, offAfter := -1, -1
	offBeforeIdx, offAfterIdx := -1, -1
	for i := 0; i < 7; i++ {
		ref := ctx.ref(int(h.RefIdx[i]))
		if ref == nil {
			return
		}
		refPOC := int(ref.FrameOffset)
		diff := getPOCDiff(orderHintBits(seq), refPOC, poc)
		if diff > 0 {
			if offAfter < 0 || getPOCDiff(orderHintBits(seq), offAfter, refPOC) > 0 {
				offAfter = refPOC
				offAfterIdx = i
			}
		} else if diff < 0 && (offBefore < 0 || getPOCDiff(orderHintBits(seq), refPOC, offBefore) > 0) {
			offBefore = refPOC
			offBeforeIdx = i
		}
	}
	if offBefore >= 0 && offAfter >= 0 {
		h.SkipModeRefs[0] = int8(min(offBeforeIdx, offAfterIdx))
		h.SkipModeRefs[1] = int8(max(offBeforeIdx, offAfterIdx))
		h.SkipModeAllowed = true
		return
	}
	if offBefore < 0 {
		return
	}
	offBefore2, offBefore2Idx := -1, -1
	for i := 0; i < 7; i++ {
		ref := ctx.ref(int(h.RefIdx[i]))
		if ref == nil {
			return
		}
		refPOC := int(ref.FrameOffset)
		if getPOCDiff(orderHintBits(seq), refPOC, offBefore) < 0 {
			if offBefore2 < 0 || getPOCDiff(orderHintBits(seq), refPOC, offBefore2) > 0 {
				offBefore2 = refPOC
				offBefore2Idx = i
			}
		}
	}
	if offBefore2 >= 0 {
		h.SkipModeRefs[0] = int8(min(offBeforeIdx, offBefore2Idx))
		h.SkipModeRefs[1] = int8(max(offBeforeIdx, offBefore2Idx))
		h.SkipModeAllowed = true
	}
}

func (ctx *FrameContext) ref(index int) *FrameHeader {
	if index < 0 || index >= len(ctx.Refs) {
		return nil
	}
	return ctx.Refs[index]
}

func isInterOrSwitch(t FrameType) bool {
	return t == FrameTypeInter || t == FrameTypeSwitch
}

func isKeyOrIntra(t FrameType) bool {
	return t == FrameTypeKey || t == FrameTypeIntra
}

func sequenceScreenContentTools(seq av1.SequenceHeader) uint8 {
	if seq.SeqChooseScreenContentTools {
		return 2
	}
	return seq.SeqForceScreenContentTools
}

func sequenceForceIntegerMV(seq av1.SequenceHeader) uint8 {
	if seq.SeqChooseIntegerMV {
		return 2
	}
	return seq.SeqForceIntegerMV
}

func frameIDBits(seq av1.SequenceHeader) int {
	return int(seq.DeltaFrameIDLengthMinus2) + int(seq.AdditionalFrameIDLengthMinus1) + 3
}

func deltaFrameIDBits(seq av1.SequenceHeader) int {
	return int(seq.DeltaFrameIDLengthMinus2) + 2
}

func orderHintBits(seq av1.SequenceHeader) int {
	return int(seq.OrderHintBitsMinus1) + 1
}

func scaledWidth(upscaledWidth int, denominator uint8) int {
	if denominator == 0 {
		return upscaledWidth
	}
	return max((upscaledWidth*8+int(denominator>>1))/int(denominator), min(16, upscaledWidth))
}

func tileLog2(sz, tgt int) int {
	k := 0
	for (sz << k) < tgt {
		k++
	}
	return k
}

func getPOCDiff(orderHintNBits, poc0, poc1 int) int {
	if orderHintNBits == 0 {
		return 0
	}
	mask := 1 << (orderHintNBits - 1)
	diff := poc0 - poc1
	return (diff & (mask - 1)) - (diff & mask)
}

func defaultModeRefDeltas() ModeRefDeltas {
	return ModeRefDeltas{
		Mode: [2]int8{0, 0},
		Ref:  [8]int8{1, 0, 0, 0, -1, 0, -1, -1},
	}
}

func defaultWMParams() WarpedMotionParams {
	return WarpedMotionParams{
		Type: wmTypeIdentity,
		Matrix: [6]int32{
			0, 0, 1 << 16,
			0, 0, 1 << 16,
		},
	}
}

func clipU8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
