package obu

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"github.com/codemodify/av1go-codex/internal/bitio"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

type Type uint8

const (
	TypeReserved Type = iota
	TypeSequenceHeader
	TypeTemporalDelimiter
	TypeFrameHeader
	TypeTileGroup
	TypeMetadata
	TypeFrame
	TypeRedundantFrameHeader
	TypeTileList
	TypePadding
)

const (
	OBUReserved           = TypeReserved
	OBUSequenceHeader     = TypeSequenceHeader
	OBUTemporalDelimiter  = TypeTemporalDelimiter
	OBUFrameHeader        = TypeFrameHeader
	OBUTileGroup          = TypeTileGroup
	OBUMetadata           = TypeMetadata
	OBUFrame              = TypeFrame
	OBURedundantFrameHead = TypeRedundantFrameHeader
	OBUTileList           = TypeTileList
	OBUPadding            = TypePadding
)

type Header struct {
	Type         Type
	Extension    bool
	HasSizeField bool
	TemporalID   uint8
	SpatialID    uint8
}

type OBU struct {
	Header  Header
	Payload []byte
	Size    int
}

type AV1C struct {
	Marker                   uint8
	Version                  uint8
	Profile                  av1.Profile
	Level                    uint8
	Tier                     uint8
	HighBitDepth             bool
	TwelveBit                bool
	Monochrome               bool
	ChromaSubsamplingX       bool
	ChromaSubsamplingY       bool
	ChromaSamplePosition     uint8
	InitialPresentationDelay *uint8
	ConfigOBUs               []byte
}

func (c AV1C) BitDepth() uint8 {
	switch {
	case c.TwelveBit:
		return 12
	case c.HighBitDepth:
		return 10
	default:
		return 8
	}
}

func (c AV1C) CodecString() string {
	tier := 'M'
	if c.Tier != 0 {
		tier = 'H'
	}
	return fmt.Sprintf("av01.%d.%02d%c.%02d", c.Profile, c.Level, tier, c.BitDepth())
}

func (c AV1C) Base64() string {
	return base64.StdEncoding.EncodeToString(c.Marshal())
}

func (c AV1C) Marshal() []byte {
	header := make([]byte, 4, 4+len(c.ConfigOBUs))
	header[0] = (1 << 7) | (c.Version & 0x7f)
	header[1] = byte(uint8(c.Profile)<<5 | (c.Level & 0x1f))
	header[2] = byte(c.Tier&0x1)<<7 | boolBit(c.HighBitDepth)<<6 | boolBit(c.TwelveBit)<<5 | boolBit(c.Monochrome)<<4
	header[2] |= boolBit(c.ChromaSubsamplingX)<<3 | boolBit(c.ChromaSubsamplingY)<<2 | (c.ChromaSamplePosition & 0x3)
	if c.InitialPresentationDelay != nil {
		header[3] = 1<<4 | (*c.InitialPresentationDelay & 0xf)
	}
	return append(header, c.ConfigOBUs...)
}

func boolBit(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func ParseAV1C(data []byte) (AV1C, error) {
	if len(data) < 4 {
		return AV1C{}, fmt.Errorf("obu: av1c too short")
	}
	if data[0]>>7 != 1 {
		return AV1C{}, fmt.Errorf("obu: invalid av1c marker")
	}
	c := AV1C{
		Marker:               data[0] >> 7,
		Version:              data[0] & 0x7f,
		Profile:              av1.Profile((data[1] >> 5) & 0x7),
		Level:                data[1] & 0x1f,
		Tier:                 (data[2] >> 7) & 0x1,
		HighBitDepth:         ((data[2] >> 6) & 0x1) == 1,
		TwelveBit:            ((data[2] >> 5) & 0x1) == 1,
		Monochrome:           ((data[2] >> 4) & 0x1) == 1,
		ChromaSubsamplingX:   ((data[2] >> 3) & 0x1) == 1,
		ChromaSubsamplingY:   ((data[2] >> 2) & 0x1) == 1,
		ChromaSamplePosition: data[2] & 0x3,
		ConfigOBUs:           append([]byte(nil), data[4:]...),
	}
	if data[3]>>4 == 1 {
		delay := data[3] & 0xf
		c.InitialPresentationDelay = &delay
	}
	return c, nil
}

func Split(data []byte) ([]OBU, error) {
	var out []OBU
	for offset := 0; offset < len(data); {
		h, consumed, err := parseHeader(data[offset:])
		if err != nil {
			return nil, err
		}
		offset += consumed
		if !h.HasSizeField {
			return nil, fmt.Errorf("obu: size field required")
		}

		v, n, err := bitio.ReadULEB128(data[offset:])
		if err != nil {
			return nil, err
		}
		size := int(v)
		sizeBytes := n
		offset += n
		if size < 0 || offset+size > len(data) {
			return nil, fmt.Errorf("obu: payload exceeds input")
		}
		payload := append([]byte(nil), data[offset:offset+size]...)
		out = append(out, OBU{
			Header:  h,
			Payload: payload,
			Size:    consumed + sizeBytes + size,
		})
		offset += size
	}
	return out, nil
}

func FindSequenceHeader(data []byte) (*av1.SequenceHeader, error) {
	obus, err := Split(data)
	if err != nil {
		return nil, err
	}
	for _, o := range obus {
		if o.Header.Type == TypeSequenceHeader {
			h, err := ParseSequenceHeader(o.Payload)
			if err != nil {
				return nil, err
			}
			return &h, nil
		}
	}
	return nil, fmt.Errorf("obu: no sequence header found")
}

func parseHeader(data []byte) (Header, int, error) {
	if len(data) < 1 {
		return Header{}, 0, fmt.Errorf("obu: header too short")
	}
	first := data[0]
	if first>>7 != 0 {
		return Header{}, 0, fmt.Errorf("obu: forbidden bit set")
	}
	h := Header{
		Type:         Type((first >> 3) & 0xf),
		Extension:    ((first >> 2) & 0x1) == 1,
		HasSizeField: ((first >> 1) & 0x1) == 1,
	}
	consumed := 1
	if h.Extension {
		if len(data) < 2 {
			return Header{}, 0, fmt.Errorf("obu: missing extension header")
		}
		ext := data[1]
		h.TemporalID = (ext >> 5) & 0x7
		h.SpatialID = (ext >> 3) & 0x3
		consumed++
	}
	return h, consumed, nil
}

func ParseSequenceHeader(data []byte) (av1.SequenceHeader, error) {
	r := bitio.NewReader(data)

	read := func(bits int) (uint32, error) { return r.ReadUint(bits) }
	readBool := func() (bool, error) { return r.ReadBool() }

	profileBits, err := read(3)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	stillPicture, err := readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	reducedStill, err := readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}

	h := av1.SequenceHeader{
		Profile:             av1.Profile(profileBits),
		StillPicture:        stillPicture,
		ReducedStillPicture: reducedStill,
	}

	if reducedStill {
		level, err := read(5)
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		h.Level = uint8(level)
		h.Tier = 0
	} else {
		timingInfoPresent, err := readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		decoderModelInfoPresent := false
		if timingInfoPresent {
			if err := r.SkipBits(32 + 32 + 1); err != nil {
				return av1.SequenceHeader{}, err
			}
			decoderModelInfoPresent, err = readBool()
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			if decoderModelInfoPresent {
				if err := r.SkipBits(5 + 32 + 5 + 5); err != nil {
					return av1.SequenceHeader{}, err
				}
			}
		}
		initialDisplayDelayPresent, err := readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		operatingPointsMinus1, err := read(5)
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		for i := uint32(0); i <= operatingPointsMinus1; i++ {
			if _, err := read(12); err != nil {
				return av1.SequenceHeader{}, err
			}
			level, err := read(5)
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			tier := uint32(0)
			if level > 7 {
				tier, err = read(1)
				if err != nil {
					return av1.SequenceHeader{}, err
				}
			}
			if i == 0 {
				h.Level = uint8(level)
				h.Tier = uint8(tier)
			}
			if decoderModelInfoPresent {
				present, err := readBool()
				if err != nil {
					return av1.SequenceHeader{}, err
				}
				if present {
					if err := r.SkipBits(32); err != nil {
						return av1.SequenceHeader{}, err
					}
				}
			}
			if initialDisplayDelayPresent {
				present, err := readBool()
				if err != nil {
					return av1.SequenceHeader{}, err
				}
				if present {
					if err := r.SkipBits(4); err != nil {
						return av1.SequenceHeader{}, err
					}
				}
			}
		}
	}

	widthBits, err := read(4)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	heightBits, err := read(4)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.FrameWidthBitsMinus1 = uint8(widthBits)
	h.FrameHeightBitsMinus1 = uint8(heightBits)
	maxWidth, err := read(int(widthBits) + 1)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	maxHeight, err := read(int(heightBits) + 1)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.MaxFrameWidthMinus1 = maxWidth
	h.MaxFrameHeightMinus1 = maxHeight

	if !reducedStill {
		frameIDNumbersPresent, err := readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		h.FrameIDNumbersPresentFlag = frameIDNumbersPresent
		if frameIDNumbersPresent {
			delta, err := read(4)
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			additional, err := read(3)
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			h.DeltaFrameIDLengthMinus2 = uint8(delta)
			h.AdditionalFrameIDLengthMinus1 = uint8(additional)
		}
	}

	h.Use128x128SuperBlock, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.EnableFilterIntra, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.EnableIntraEdgeFilter, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}

	if !reducedStill {
		h.EnableInterIntraCompound, err = readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		h.EnableMaskedCompound, err = readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		h.EnableWarpedMotion, err = readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		h.EnableDualFilter, err = readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		h.EnableOrderHint, err = readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		if h.EnableOrderHint {
			h.EnableJntComp, err = readBool()
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			h.EnableRefFrameMvs, err = readBool()
			if err != nil {
				return av1.SequenceHeader{}, err
			}
		}
		h.SeqChooseScreenContentTools, err = readBool()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		screenContentTools := uint8(0)
		if h.SeqChooseScreenContentTools {
			screenContentTools = 2
			h.SeqForceScreenContentTools = 2
		} else {
			forceScreenContent, err := read(1)
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			screenContentTools = uint8(forceScreenContent)
			h.SeqForceScreenContentTools = uint8(forceScreenContent)
		}
		if screenContentTools != 0 {
			h.SeqChooseIntegerMV, err = readBool()
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			if h.SeqChooseIntegerMV {
				h.SeqForceIntegerMV = 2
			} else {
				forceIntegerMV, err := read(1)
				if err != nil {
					return av1.SequenceHeader{}, err
				}
				h.SeqForceIntegerMV = uint8(forceIntegerMV)
			}
		} else {
			h.SeqChooseIntegerMV = true
			h.SeqForceIntegerMV = 2
		}
		if h.EnableOrderHint {
			bits, err := read(3)
			if err != nil {
				return av1.SequenceHeader{}, err
			}
			h.OrderHintBitsMinus1 = uint8(bits)
		}
	} else {
		h.SeqChooseScreenContentTools = true
		h.SeqForceScreenContentTools = 2
		h.SeqChooseIntegerMV = true
		h.SeqForceIntegerMV = 2
	}

	h.EnableSuperRes, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.EnableCDEF, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.EnableRestoration, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}

	colorConfig, err := parseColorConfig(r, h.Profile)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	h.ColorConfig = colorConfig

	h.FilmGrainParamsPresent, err = readBool()
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	return h, nil
}

func parseColorConfig(r *bitio.Reader, profile av1.Profile) (av1.ColorConfig, error) {
	highBitDepth, err := r.ReadBool()
	if err != nil {
		return av1.ColorConfig{}, err
	}

	cfg := av1.ColorConfig{}
	if profile == av1.ProfileProfessional && highBitDepth {
		twelveBit, err := r.ReadBool()
		if err != nil {
			return av1.ColorConfig{}, err
		}
		if twelveBit {
			cfg.BitDepth = 12
		} else {
			cfg.BitDepth = 10
		}
	} else if highBitDepth {
		cfg.BitDepth = 10
	} else {
		cfg.BitDepth = 8
	}

	if profile == av1.ProfileHigh {
		cfg.Monochrome = false
	} else {
		cfg.Monochrome, err = r.ReadBool()
		if err != nil {
			return av1.ColorConfig{}, err
		}
	}

	colorDescriptionPresent, err := r.ReadBool()
	if err != nil {
		return av1.ColorConfig{}, err
	}
	colorPrimaries := uint32(2)
	transferCharacteristics := uint32(2)
	matrixCoefficients := uint32(2)
	if colorDescriptionPresent {
		if colorPrimaries, err = r.ReadUint(8); err != nil {
			return av1.ColorConfig{}, err
		}
		if transferCharacteristics, err = r.ReadUint(8); err != nil {
			return av1.ColorConfig{}, err
		}
		if matrixCoefficients, err = r.ReadUint(8); err != nil {
			return av1.ColorConfig{}, err
		}
	}
	cfg.ColorPrimaries = uint8(colorPrimaries)
	cfg.TransferCharacteristics = uint8(transferCharacteristics)
	cfg.MatrixCoefficients = uint8(matrixCoefficients)

	if cfg.Monochrome {
		colorRange, err := r.ReadBool()
		if err != nil {
			return av1.ColorConfig{}, err
		}
		cfg.FullRange = colorRange
		cfg.SubsamplingX = true
		cfg.SubsamplingY = true
		cfg.SeparateUVDeltaQ = false
		return cfg, nil
	}

	isRGBIdentity := colorPrimaries == 1 && transferCharacteristics == 13 && matrixCoefficients == 0
	if isRGBIdentity {
		colorRange, err := r.ReadBool()
		if err != nil {
			return av1.ColorConfig{}, err
		}
		cfg.FullRange = colorRange
		cfg.SubsamplingX = false
		cfg.SubsamplingY = false
	} else {
		colorRange, err := r.ReadBool()
		if err != nil {
			return av1.ColorConfig{}, err
		}
		cfg.FullRange = colorRange
		switch profile {
		case av1.ProfileMain:
			cfg.SubsamplingX = true
			cfg.SubsamplingY = true
		case av1.ProfileHigh:
			cfg.SubsamplingX = false
			cfg.SubsamplingY = false
		default:
			if cfg.BitDepth == 12 {
				x, err := r.ReadBool()
				if err != nil {
					return av1.ColorConfig{}, err
				}
				cfg.SubsamplingX = x
				if x {
					y, err := r.ReadBool()
					if err != nil {
						return av1.ColorConfig{}, err
					}
					cfg.SubsamplingY = y
				}
			} else {
				cfg.SubsamplingX = true
				cfg.SubsamplingY = false
			}
		}
		if cfg.SubsamplingX && cfg.SubsamplingY {
			pos, err := r.ReadUint(2)
			if err != nil {
				return av1.ColorConfig{}, err
			}
			cfg.ChromaSamplePos = uint8(pos)
		}
	}

	cfg.SeparateUVDeltaQ, err = r.ReadBool()
	if err != nil {
		return av1.ColorConfig{}, err
	}
	return cfg, nil
}

func ParseSequenceHeaderFromAV1C(data []byte) (AV1C, av1.SequenceHeader, error) {
	cfg, err := ParseAV1C(data)
	if err != nil {
		return AV1C{}, av1.SequenceHeader{}, err
	}
	header, err := FindSequenceHeader(cfg.ConfigOBUs)
	if err != nil {
		return AV1C{}, av1.SequenceHeader{}, err
	}
	if cfgBitDepth := cfg.BitDepth(); header.ColorConfig.BitDepth == 0 || cfgBitDepth > header.ColorConfig.BitDepth {
		header.ColorConfig.BitDepth = cfgBitDepth
	}
	if header.Level == 0 {
		header.Level = cfg.Level
	}
	header.Profile = cfg.Profile
	return cfg, *header, nil
}

func MakeSizeField(size int) []byte {
	var out []byte
	v := uint64(size)
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

func WrapSequenceHeaderOBU(payload []byte) []byte {
	out := []byte{byte(TypeSequenceHeader) << 3}
	out[0] |= 1 << 1
	out = append(out, MakeSizeField(len(payload))...)
	out = append(out, payload...)
	return out
}

func Uint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
