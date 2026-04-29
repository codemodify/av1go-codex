package av1

import "errors"

var (
	ErrUnsupportedCodec = errors.New("av1: unsupported codec configuration")
	ErrUnsupportedMP4   = errors.New("av1: unsupported mp4 structure")
	ErrUnsupportedOBU   = errors.New("av1: unsupported obu")
)

type Profile uint8

const (
	ProfileMain Profile = iota
	ProfileHigh
	ProfileProfessional
)

type ChromaSubsampling uint8

const (
	ChromaUnknown ChromaSubsampling = iota
	Chroma420
	Chroma422
	Chroma444
	Chroma400
)

type ColorConfig struct {
	BitDepth                uint8
	Monochrome              bool
	SubsamplingX            bool
	SubsamplingY            bool
	ChromaSamplePos         uint8
	SeparateUVDeltaQ        bool
	FullRange               bool
	ColorPrimaries          uint8
	TransferCharacteristics uint8
	MatrixCoefficients      uint8
}

func (c ColorConfig) ChromaSubsampling() ChromaSubsampling {
	switch {
	case c.Monochrome:
		return Chroma400
	case c.SubsamplingX && c.SubsamplingY:
		return Chroma420
	case c.SubsamplingX && !c.SubsamplingY:
		return Chroma422
	case !c.SubsamplingX && !c.SubsamplingY:
		return Chroma444
	default:
		return ChromaUnknown
	}
}

func (c ChromaSubsampling) String() string {
	switch c {
	case Chroma420:
		return "4:2:0"
	case Chroma422:
		return "4:2:2"
	case Chroma444:
		return "4:4:4"
	case Chroma400:
		return "4:0:0"
	default:
		return "unknown"
	}
}

type SequenceHeader struct {
	Profile                       Profile
	StillPicture                  bool
	ReducedStillPicture           bool
	Level                         uint8
	Tier                          uint8
	MaxFrameWidthMinus1           uint32
	MaxFrameHeightMinus1          uint32
	FrameWidthBitsMinus1          uint8
	FrameHeightBitsMinus1         uint8
	FrameIDNumbersPresentFlag     bool
	DeltaFrameIDLengthMinus2      uint8
	AdditionalFrameIDLengthMinus1 uint8
	Use128x128SuperBlock          bool
	EnableFilterIntra             bool
	EnableIntraEdgeFilter         bool
	EnableInterIntraCompound      bool
	EnableMaskedCompound          bool
	EnableWarpedMotion            bool
	EnableDualFilter              bool
	EnableOrderHint               bool
	OrderHintBitsMinus1           uint8
	EnableJntComp                 bool
	EnableRefFrameMvs             bool
	SeqChooseScreenContentTools   bool
	SeqForceScreenContentTools    uint8
	SeqChooseIntegerMV            bool
	SeqForceIntegerMV             uint8
	EnableSuperRes                bool
	EnableCDEF                    bool
	EnableRestoration             bool
	FilmGrainParamsPresent        bool
	ColorConfig                   ColorConfig
}

func (h SequenceHeader) Width() int {
	return int(h.MaxFrameWidthMinus1) + 1
}

func (h SequenceHeader) Height() int {
	return int(h.MaxFrameHeightMinus1) + 1
}

func (h SequenceHeader) SupportedMain8Bit420() bool {
	return h.Profile == ProfileMain &&
		h.ColorConfig.BitDepth == 8 &&
		h.ColorConfig.ChromaSubsampling() == Chroma420
}

func (h SequenceHeader) SupportedMain10Bit420() bool {
	return h.Profile == ProfileMain &&
		h.ColorConfig.BitDepth == 10 &&
		h.ColorConfig.ChromaSubsampling() == Chroma420
}
