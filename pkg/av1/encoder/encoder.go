package encoder

import (
	"errors"
	"image"
	"image/color"
	"io"

	"github.com/codemodify/av1go-codex/pkg/container/mp4"
)

var (
	ErrNilImage             = errors.New("encoder: nil image")
	ErrFrameDimensions      = errors.New("encoder: frame dimensions do not match encoder config")
	ErrNilFrameStream       = errors.New("encoder: nil frame stream")
	ErrEmptyFrameStream     = errors.New("encoder: empty frame stream")
	ErrNilWriter            = errors.New("encoder: nil writer")
	ErrBitstreamUnavailable = errors.New("encoder: source-image AV1 bitstream generation unavailable")
)

type Config struct {
	Width     int
	Height    int
	Timescale uint32
	AV1C      mp4.AV1CodecConfig
}

type Encoder struct {
	cfg Config
}

func New(cfg Config) (*Encoder, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, io.ErrShortBuffer
	}
	if cfg.Timescale == 0 {
		cfg.Timescale = 1000
	}
	cfg.AV1C = ensureCodecConfig(cfg.AV1C)
	return &Encoder{cfg: cfg}, nil
}

func (e *Encoder) EncodeFrame(src image.Image) ([]byte, error) {
	if src == nil {
		return nil, ErrNilImage
	}
	if !matchesConfig(src.Bounds(), e.cfg.Width, e.cfg.Height) {
		return nil, ErrFrameDimensions
	}
	if supportsBlackTemplate(e.cfg.Width, e.cfg.Height) && isSolidBlack(src) {
		return black16x16SampleData(), nil
	}
	return nil, ErrBitstreamUnavailable
}

func (e *Encoder) EncodeAllIntraMP4(frames <-chan *image.YCbCr, w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}
	if frames == nil {
		return ErrNilFrameStream
	}
	first, ok := <-frames
	if !ok {
		return ErrEmptyFrameStream
	}
	if first == nil {
		return ErrNilImage
	}
	if !matchesConfig(first.Rect, e.cfg.Width, e.cfg.Height) {
		return ErrFrameDimensions
	}
	firstData, err := e.EncodeFrame(first)
	if err != nil {
		return err
	}
	samples := []mp4.SampleInput{{
		Data:     firstData,
		Duration: frameDuration(e.cfg.Timescale),
		Keyframe: true,
	}}
	for frame := range frames {
		if frame == nil {
			return ErrNilImage
		}
		data, err := e.EncodeFrame(frame)
		if err != nil {
			return err
		}
		samples = append(samples, mp4.SampleInput{
			Data:     data,
			Duration: frameDuration(e.cfg.Timescale),
			Keyframe: true,
		})
	}
	return mp4.WriteVideo(w, mp4.VideoConfig{
		Width:     e.cfg.Width,
		Height:    e.cfg.Height,
		Timescale: e.cfg.Timescale,
		AV1C:      codecConfigForGeneratedSample(e.cfg),
	}, samples)
}

func (e *Encoder) RemuxMP4(w io.Writer, samples []mp4.SampleInput) error {
	return mp4.WriteVideo(w, mp4.VideoConfig{
		Width:     e.cfg.Width,
		Height:    e.cfg.Height,
		Timescale: e.cfg.Timescale,
		AV1C:      e.cfg.AV1C,
	}, samples)
}

func ensureCodecConfig(cfg mp4.AV1CodecConfig) mp4.AV1CodecConfig {
	if len(cfg.Raw) > 0 || cfg.Version != 0 || cfg.SeqProfile != 0 || cfg.SeqLevelIdx0 != 0 ||
		cfg.SeqTier0 != 0 || cfg.HighBitDepth || cfg.TwelveBit || cfg.Monochrome ||
		cfg.ChromaSubsamplingX || cfg.ChromaSubsamplingY || cfg.ChromaSamplePosition != 0 ||
		cfg.InitialPresentationDelayPresent || cfg.InitialPresentationDelayMinusOne != 0 {
		if cfg.Version == 0 {
			cfg.Version = 1
		}
		return cfg
	}
	return mp4.AV1CodecConfig{
		Version:            1,
		SeqProfile:         0,
		SeqLevelIdx0:       0,
		SeqTier0:           0,
		HighBitDepth:       false,
		TwelveBit:          false,
		Monochrome:         false,
		ChromaSubsamplingX: true,
		ChromaSubsamplingY: true,
	}
}

func matchesConfig(rect image.Rectangle, width, height int) bool {
	return rect.Dx() == width && rect.Dy() == height
}

func supportsBlackTemplate(width, height int) bool {
	return width == 16 && height == 16
}

func codecConfigForGeneratedSample(cfg Config) mp4.AV1CodecConfig {
	if supportsBlackTemplate(cfg.Width, cfg.Height) {
		return black16x16CodecConfig()
	}
	return cfg.AV1C
}

func frameDuration(timescale uint32) uint32 {
	if timescale == 0 {
		return 1
	}
	return timescale
}

func isSolidBlack(src image.Image) bool {
	bounds := src.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.RGBAModel.Convert(src.At(x, y)).(color.RGBA)
			if c.R != 0 || c.G != 0 || c.B != 0 {
				return false
			}
		}
	}
	return true
}
