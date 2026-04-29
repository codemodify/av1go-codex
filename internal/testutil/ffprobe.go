package testutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

type AV1StreamProbe struct {
	Index           int
	CodecName       string
	Profile         string
	Width           int
	Height          int
	PixFmt          string
	AvgFrameRate    string
	TimeBase        string
	CodecTagString  string
	MimeCodecString string
	Extradata       []byte
}

// ProbeAV1VideoStream uses ffprobe to read the first video stream and its codec config.
func ProbeAV1VideoStream(tb testing.TB, path string) AV1StreamProbe {
	tb.Helper()
	RequireTool(tb, "ffprobe")
	RequireFile(tb, path)

	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=index,codec_name,profile,width,height,pix_fmt,avg_frame_rate,time_base,codec_tag_string,mime_codec_string,extradata",
		"-show_data",
		"-of", "json",
		path,
	}
	cmd := exec.Command("ffprobe", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("ffprobe failed for %s: %v\n%s", path, err, strings.TrimSpace(string(out)))
	}

	var parsed struct {
		Streams []struct {
			Index           int    `json:"index"`
			CodecName       string `json:"codec_name"`
			Profile         string `json:"profile"`
			Width           int    `json:"width"`
			Height          int    `json:"height"`
			PixFmt          string `json:"pix_fmt"`
			AvgFrameRate    string `json:"avg_frame_rate"`
			TimeBase        string `json:"time_base"`
			CodecTagString  string `json:"codec_tag_string"`
			MimeCodecString string `json:"mime_codec_string"`
			Extradata       string `json:"extradata"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		tb.Fatalf("parse ffprobe json: %v", err)
	}
	if len(parsed.Streams) == 0 {
		tb.Fatalf("ffprobe returned no video streams for %s", path)
	}

	s := parsed.Streams[0]
	return AV1StreamProbe{
		Index:           s.Index,
		CodecName:       s.CodecName,
		Profile:         s.Profile,
		Width:           s.Width,
		Height:          s.Height,
		PixFmt:          s.PixFmt,
		AvgFrameRate:    s.AvgFrameRate,
		TimeBase:        s.TimeBase,
		CodecTagString:  s.CodecTagString,
		MimeCodecString: s.MimeCodecString,
		Extradata:       decodeFFProbeHexDump(tb, s.Extradata),
	}
}

func decodeFFProbeHexDump(tb testing.TB, dump string) []byte {
	tb.Helper()

	var data []byte
	lines := strings.Split(dump, "\n")
	for _, line := range lines {
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		for _, field := range fields {
			if len(field) < 2 || len(field)%2 != 0 {
				break
			}
			if !allHex(field) {
				break
			}
			chunk, err := hex.DecodeString(field)
			if err != nil {
				tb.Fatalf("decode ffprobe extradata field %q: %v", field, err)
			}
			data = append(data, chunk...)
		}
	}
	if len(data) == 0 {
		tb.Fatalf("ffprobe extradata dump was empty")
	}
	return data
}

func allHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// CodecConfig is the tiny AV1 codec-config subset we can verify from av1C.
type CodecConfig struct {
	Profile      av1.Profile
	Level        uint8
	Tier         uint8
	BitDepth     uint8
	Monochrome   bool
	SubsamplingX bool
	SubsamplingY bool
	ChromaPos    uint8
	Raw          []byte
}

// ParseCodecConfigFromAV1C parses the leading av1C bytes from MP4 extradata.
func ParseCodecConfigFromAV1C(tb testing.TB, extradata []byte) (CodecConfig, error) {
	tb.Helper()
	if len(extradata) < 3 {
		return CodecConfig{}, fmt.Errorf("extradata too short: %d", len(extradata))
	}
	if extradata[0]&0x80 == 0 {
		return CodecConfig{}, fmt.Errorf("invalid av1C marker bit")
	}

	profile := av1.Profile((extradata[1] >> 5) & 0x07)
	level := extradata[1] & 0x1f

	highBitDepth := extradata[2]&0x40 != 0
	twelveBit := extradata[2]&0x20 != 0
	monochrome := extradata[2]&0x10 != 0
	subsamplingX := extradata[2]&0x08 != 0
	subsamplingY := extradata[2]&0x04 != 0
	chromaPos := extradata[2] & 0x03

	bitDepth := uint8(8)
	switch {
	case twelveBit:
		bitDepth = 12
	case highBitDepth:
		bitDepth = 10
	}

	return CodecConfig{
		Profile:      profile,
		Level:        level,
		Tier:         0,
		BitDepth:     bitDepth,
		Monochrome:   monochrome,
		SubsamplingX: subsamplingX,
		SubsamplingY: subsamplingY,
		ChromaPos:    chromaPos,
		Raw:          append([]byte(nil), extradata...),
	}, nil
}

// SequenceHeaderFromProbe converts ffprobe metadata into an av1.SequenceHeader.
func SequenceHeaderFromProbe(tb testing.TB, probe AV1StreamProbe) (av1.SequenceHeader, error) {
	tb.Helper()
	cfg, err := ParseCodecConfigFromAV1C(tb, probe.Extradata)
	if err != nil {
		return av1.SequenceHeader{}, err
	}
	if probe.Width <= 0 || probe.Height <= 0 {
		return av1.SequenceHeader{}, fmt.Errorf("invalid dimensions %dx%d", probe.Width, probe.Height)
	}

	return av1.SequenceHeader{
		Profile:              cfg.Profile,
		Level:                cfg.Level,
		Tier:                 cfg.Tier,
		MaxFrameWidthMinus1:  uint32(probe.Width - 1),
		MaxFrameHeightMinus1: uint32(probe.Height - 1),
		ColorConfig: av1.ColorConfig{
			BitDepth:        cfg.BitDepth,
			Monochrome:      cfg.Monochrome,
			SubsamplingX:    cfg.SubsamplingX,
			SubsamplingY:    cfg.SubsamplingY,
			ChromaSamplePos: cfg.ChromaPos,
		},
	}, nil
}

// ParsePositiveInt parses a non-negative integer string.
func ParsePositiveInt(tb testing.TB, s string) int {
	tb.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		tb.Fatalf("parse integer %q: %v", s, err)
	}
	return n
}
