package mp4

import (
	"errors"
	"fmt"
	"io"
)

var (
	ErrUnsupportedBox    = errors.New("mp4: unsupported box")
	ErrUnsupportedTrack  = errors.New("mp4: unsupported track")
	ErrUnsupportedSample = errors.New("mp4: unsupported sample table")
	ErrUnsupportedAV1C   = errors.New("mp4: unsupported av1C")
	ErrInvalidMP4        = errors.New("mp4: invalid mp4")
	ErrNotAV1VideoTrack  = errors.New("mp4: no av1 video track found")
)

type AV1CodecConfig struct {
	Raw []byte

	Version                          uint8
	SeqProfile                       uint8
	SeqLevelIdx0                     uint8
	SeqTier0                         uint8
	HighBitDepth                     bool
	TwelveBit                        bool
	Monochrome                       bool
	ChromaSubsamplingX               bool
	ChromaSubsamplingY               bool
	ChromaSamplePosition             uint8
	InitialPresentationDelayPresent  bool
	InitialPresentationDelayMinusOne uint8
}

func (c AV1CodecConfig) Clone() AV1CodecConfig {
	out := c
	if len(c.Raw) > 0 {
		out.Raw = append([]byte(nil), c.Raw...)
	}
	return out
}

func (c AV1CodecConfig) MarshalBinary() ([]byte, error) {
	if len(c.Raw) > 0 {
		return append([]byte(nil), c.Raw...), nil
	}
	if c.Version == 0 {
		c.Version = 1
	}
	out := make([]byte, 4)
	out[0] = 0x80 | (c.Version & 0x7f)
	out[1] = (c.SeqProfile & 0x7) << 5
	out[1] |= c.SeqLevelIdx0 & 0x1f
	out[2] = (c.SeqTier0 & 0x1) << 7
	if c.HighBitDepth {
		out[2] |= 1 << 6
	}
	if c.TwelveBit {
		out[2] |= 1 << 5
	}
	if c.Monochrome {
		out[2] |= 1 << 4
	}
	if c.ChromaSubsamplingX {
		out[2] |= 1 << 3
	}
	if c.ChromaSubsamplingY {
		out[2] |= 1 << 2
	}
	out[2] |= c.ChromaSamplePosition & 0x3
	if c.InitialPresentationDelayPresent {
		out[3] = 1 << 7
		out[3] |= (c.InitialPresentationDelayMinusOne & 0x0f) << 3
		out[3] |= 0x07
	} else {
		out[3] = 0x07
	}
	return out, nil
}

func (c AV1CodecConfig) CodecString() string {
	tier := "M"
	if c.SeqTier0 != 0 {
		tier = "H"
	}
	level := c.SeqLevelIdx0
	return fmt.Sprintf("av01.%d.%02d%s.%02d", c.SeqProfile, level, tier, c.BitDepth())
}

func (c AV1CodecConfig) BitDepth() uint8 {
	switch {
	case c.TwelveBit:
		return 12
	case c.HighBitDepth:
		return 10
	default:
		return 8
	}
}

type Sample struct {
	Index             int
	Offset            int64
	Size              int
	Duration          uint32
	CompositionOffset int32
	DTS               int64
	PTS               int64
	Keyframe          bool
}

type SampleInput struct {
	Data              []byte
	Duration          uint32
	CompositionOffset int32
	Keyframe          bool
}

type VideoConfig struct {
	Width     int
	Height    int
	Timescale uint32
	AV1C      AV1CodecConfig
}

type File struct {
	r      io.ReaderAt
	closer io.Closer
	size   int64
	Video  *Track
}

type Track struct {
	ID        uint32
	Width     int
	Height    int
	Timescale uint32
	Duration  uint64
	AV1C      AV1CodecConfig
	Samples   []Sample

	r io.ReaderAt
}

func (f *File) Close() error {
	if f.closer != nil {
		return f.closer.Close()
	}
	return nil
}

func (f *File) AV1VideoTrack() (*Track, error) {
	if f.Video == nil {
		return nil, ErrNotAV1VideoTrack
	}
	return f.Video, nil
}

func (t *Track) SampleCount() int {
	return len(t.Samples)
}

func (t *Track) ReadSample(i int) ([]byte, error) {
	if i < 0 || i >= len(t.Samples) {
		return nil, fmt.Errorf("mp4: sample index %d out of range", i)
	}
	s := t.Samples[i]
	if t.r == nil {
		return nil, errors.New("mp4: sample reader unavailable")
	}
	buf := make([]byte, s.Size)
	if _, err := readAtFull(t.r, uint64(s.Offset), buf); err != nil {
		return nil, err
	}
	return buf, nil
}
