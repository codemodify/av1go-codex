package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

type sttsEntry struct {
	count uint32
	delta uint32
}

type cttsEntry struct {
	count  uint32
	offset int32
}

type stscEntry struct {
	firstChunk             uint32
	samplesPerChunk        uint32
	sampleDescriptionIndex uint32
}

// Open parses an unfragmented MP4 file and returns the first AV1 video track.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	file := &File{
		r:      f,
		closer: f,
		size:   stat.Size(),
	}
	track, err := parseFile(f, uint64(stat.Size()))
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	file.Video = track
	return file, nil
}

func parseFile(r io.ReaderAt, size uint64) (*Track, error) {
	var video *Track
	if err := walkBoxes(r, 0, size, func(b box) error {
		if b.typ != "moov" {
			return nil
		}
		t, err := parseMoov(r, b)
		if err != nil {
			return err
		}
		if t != nil && video == nil {
			video = t
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if video == nil {
		return nil, ErrNotAV1VideoTrack
	}
	return video, nil
}

func parseMoov(r io.ReaderAt, moov box) (*Track, error) {
	var track *Track
	if err := walkBoxes(r, moov.dataOffset, payloadSize(moov), func(b box) error {
		if b.typ != "trak" || track != nil {
			return nil
		}
		t, err := parseTrak(r, b)
		if err != nil {
			return err
		}
		if t != nil {
			track = t
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return track, nil
}

func parseTrak(r io.ReaderAt, trak box) (*Track, error) {
	var (
		handlerType string
		timescale   uint32
		duration    uint64
		trackID     uint32
		width       int
		height      int
		av1c        AV1CodecConfig
		samples     []Sample
	)

	var mdia box
	if err := walkBoxes(r, trak.dataOffset, payloadSize(trak), func(b box) error {
		switch b.typ {
		case "tkhd":
			id, w, h, err := parseTkhd(r, b)
			if err != nil {
				return err
			}
			trackID = id
			width = w
			height = h
		case "mdia":
			mdia = b
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if mdia.typ == "" {
		return nil, fmt.Errorf("%w: missing mdia", ErrInvalidMP4)
	}

	var stbl box
	if err := walkBoxes(r, mdia.dataOffset, payloadSize(mdia), func(b box) error {
		switch b.typ {
		case "mdhd":
			ts, dur, err := parseMdhd(r, b)
			if err != nil {
				return err
			}
			timescale = ts
			duration = dur
		case "hdlr":
			ht, err := parseHdlr(r, b)
			if err != nil {
				return err
			}
			handlerType = ht
		case "minf":
			if err := walkBoxes(r, b.dataOffset, payloadSize(b), func(child box) error {
				if child.typ == "stbl" {
					stbl = child
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if handlerType != "vide" {
		return nil, nil
	}
	if stbl.typ == "" {
		return nil, fmt.Errorf("%w: missing stbl", ErrInvalidMP4)
	}

	tables, err := parseStbl(r, stbl)
	if err != nil {
		if errors.Is(err, ErrNotAV1VideoTrack) {
			return nil, nil
		}
		return nil, err
	}
	width = tables.width
	height = tables.height
	av1c = tables.av1c
	samples, err = buildSamples(tables)
	if err != nil {
		return nil, err
	}
	if trackID == 0 {
		trackID = 1
	}
	return &Track{
		ID:        trackID,
		Width:     width,
		Height:    height,
		Timescale: timescale,
		Duration:  duration,
		AV1C:      av1c,
		Samples:   samples,
		r:         r,
	}, nil
}

func parseTkhd(r io.ReaderAt, b box) (trackID uint32, width int, height int, err error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(buf) < 84 {
		return 0, 0, 0, fmt.Errorf("%w: tkhd too small", ErrInvalidMP4)
	}
	version := buf[0]
	switch version {
	case 0:
		if len(buf) < 84 {
			return 0, 0, 0, fmt.Errorf("%w: tkhd too small", ErrInvalidMP4)
		}
		trackID = binary.BigEndian.Uint32(buf[12:16])
		width = int(binary.BigEndian.Uint32(buf[76:80]) >> 16)
		height = int(binary.BigEndian.Uint32(buf[80:84]) >> 16)
	case 1:
		if len(buf) < 96 {
			return 0, 0, 0, fmt.Errorf("%w: tkhd too small", ErrInvalidMP4)
		}
		trackID = binary.BigEndian.Uint32(buf[20:24])
		width = int(binary.BigEndian.Uint32(buf[88:92]) >> 16)
		height = int(binary.BigEndian.Uint32(buf[92:96]) >> 16)
	default:
		return 0, 0, 0, fmt.Errorf("%w: tkhd version %d", ErrInvalidMP4, version)
	}
	return trackID, width, height, nil
}

func parseMdhd(r io.ReaderAt, b box) (timescale uint32, duration uint64, err error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return 0, 0, err
	}
	if len(buf) < 20 {
		return 0, 0, fmt.Errorf("%w: mdhd too small", ErrInvalidMP4)
	}
	switch buf[0] {
	case 0:
		if len(buf) < 24 {
			return 0, 0, fmt.Errorf("%w: mdhd too small", ErrInvalidMP4)
		}
		timescale = binary.BigEndian.Uint32(buf[12:16])
		duration = uint64(binary.BigEndian.Uint32(buf[16:20]))
	case 1:
		if len(buf) < 36 {
			return 0, 0, fmt.Errorf("%w: mdhd too small", ErrInvalidMP4)
		}
		timescale = binary.BigEndian.Uint32(buf[20:24])
		duration = binary.BigEndian.Uint64(buf[24:32])
	default:
		return 0, 0, fmt.Errorf("%w: mdhd version %d", ErrInvalidMP4, buf[0])
	}
	return timescale, duration, nil
}

func parseHdlr(r io.ReaderAt, b box) (string, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return "", err
	}
	if len(buf) < 12 {
		return "", fmt.Errorf("%w: hdlr too small", ErrInvalidMP4)
	}
	return string(buf[8:12]), nil
}

type stblTables struct {
	width   int
	height  int
	av1c    AV1CodecConfig
	stts    []sttsEntry
	ctts    []cttsEntry
	stsc    []stscEntry
	sizes   []uint32
	offsets []uint64
	sync    map[uint32]bool
}

func parseStbl(r io.ReaderAt, stbl box) (*stblTables, error) {
	tables := &stblTables{sync: map[uint32]bool{}}
	var stsd box
	if err := walkBoxes(r, stbl.dataOffset, payloadSize(stbl), func(b box) error {
		switch b.typ {
		case "stsd":
			stsd = b
		case "stts":
			entries, err := parseStts(r, b)
			if err != nil {
				return err
			}
			tables.stts = entries
		case "ctts":
			entries, err := parseCtts(r, b)
			if err != nil {
				return err
			}
			tables.ctts = entries
		case "stsc":
			entries, err := parseStsc(r, b)
			if err != nil {
				return err
			}
			tables.stsc = entries
		case "stsz":
			sizes, err := parseStsz(r, b)
			if err != nil {
				return err
			}
			tables.sizes = sizes
		case "stco":
			offsets, err := parseStco(r, b)
			if err != nil {
				return err
			}
			tables.offsets = offsets
		case "co64":
			offsets, err := parseCo64(r, b)
			if err != nil {
				return err
			}
			tables.offsets = offsets
		case "stss":
			sync, err := parseStss(r, b)
			if err != nil {
				return err
			}
			tables.sync = sync
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if stsd.typ == "" {
		return nil, fmt.Errorf("%w: missing stsd", ErrInvalidMP4)
	}
	width, height, av1c, err := parseAv01SampleDescription(r, stsd)
	if err != nil {
		return nil, err
	}
	tables.width = width
	tables.height = height
	tables.av1c = av1c
	if len(tables.stts) == 0 || len(tables.stsc) == 0 || len(tables.sizes) == 0 || len(tables.offsets) == 0 {
		return nil, fmt.Errorf("%w: incomplete sample tables", ErrUnsupportedSample)
	}
	return tables, nil
}

func parseAv01SampleDescription(r io.ReaderAt, stsd box) (int, int, AV1CodecConfig, error) {
	buf, err := readBoxBytes(r, stsd)
	if err != nil {
		return 0, 0, AV1CodecConfig{}, err
	}
	if len(buf) < 8 {
		return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: stsd too small", ErrInvalidMP4)
	}
	if len(buf) < 16 {
		return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: stsd too small", ErrInvalidMP4)
	}
	entryCount := binary.BigEndian.Uint32(buf[4:8])
	if entryCount == 0 {
		return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: empty stsd", ErrInvalidMP4)
	}
	pos := uint64(8)
	for i := uint32(0); i < entryCount; i++ {
		if pos+8 > uint64(len(buf)) {
			return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: stsd entry out of range", ErrInvalidMP4)
		}
		size := binary.BigEndian.Uint32(buf[pos : pos+4])
		typ := string(buf[pos+4 : pos+8])
		if size == 0 || pos+uint64(size) > uint64(len(buf)) {
			return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: stsd entry size", ErrInvalidMP4)
		}
		if typ != "av01" {
			pos += uint64(size)
			continue
		}
		entry := buf[pos : pos+uint64(size)]
		if len(entry) < 86 {
			return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: av01 sample entry too small", ErrInvalidMP4)
		}
		width := int(binary.BigEndian.Uint16(entry[32:34]))
		height := int(binary.BigEndian.Uint16(entry[34:36]))
		av1c, err := parseAV1CodecConfigFromEntry(entry[86:])
		if err != nil {
			return 0, 0, AV1CodecConfig{}, err
		}
		return width, height, av1c, nil
	}
	return 0, 0, AV1CodecConfig{}, fmt.Errorf("%w: no av01 sample entry", ErrNotAV1VideoTrack)
}

func parseAV1CodecConfigFromEntry(data []byte) (AV1CodecConfig, error) {
	var cfg AV1CodecConfig
	pos := 0
	for pos+8 <= len(data) {
		size := binary.BigEndian.Uint32(data[pos : pos+4])
		typ := string(data[pos+4 : pos+8])
		if size < 8 || pos+int(size) > len(data) {
			return AV1CodecConfig{}, fmt.Errorf("%w: child box %s", ErrInvalidMP4, typ)
		}
		if typ == "av1C" {
			payload := append([]byte(nil), data[pos+8:pos+int(size)]...)
			cfg.Raw = payload
			if err := parseAV1CRaw(&cfg, payload); err != nil {
				return AV1CodecConfig{}, err
			}
			return cfg, nil
		}
		pos += int(size)
	}
	return AV1CodecConfig{}, fmt.Errorf("%w: missing av1C", ErrNotAV1VideoTrack)
}

func parseAV1CRaw(cfg *AV1CodecConfig, payload []byte) error {
	if len(payload) < 4 {
		return fmt.Errorf("%w: av1C too small", ErrInvalidMP4)
	}
	cfg.Version = payload[0] & 0x7f
	cfg.SeqProfile = (payload[1] >> 5) & 0x7
	cfg.SeqLevelIdx0 = payload[1] & 0x1f
	cfg.SeqTier0 = (payload[2] >> 7) & 0x1
	cfg.HighBitDepth = payload[2]&0x40 != 0
	cfg.TwelveBit = payload[2]&0x20 != 0
	cfg.Monochrome = payload[2]&0x10 != 0
	cfg.ChromaSubsamplingX = payload[2]&0x08 != 0
	cfg.ChromaSubsamplingY = payload[2]&0x04 != 0
	cfg.ChromaSamplePosition = payload[2] & 0x03
	cfg.InitialPresentationDelayPresent = payload[3]&0x80 != 0
	cfg.InitialPresentationDelayMinusOne = (payload[3] >> 3) & 0x0f
	return nil
}

func parseStts(r io.ReaderAt, b box) ([]sttsEntry, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: stts too small", ErrInvalidMP4)
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	pos := 8
	out := make([]sttsEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		if pos+8 > len(buf) {
			return nil, fmt.Errorf("%w: stts truncated", ErrInvalidMP4)
		}
		out = append(out, sttsEntry{
			count: binary.BigEndian.Uint32(buf[pos : pos+4]),
			delta: binary.BigEndian.Uint32(buf[pos+4 : pos+8]),
		})
		pos += 8
	}
	return out, nil
}

func parseCtts(r io.ReaderAt, b box) ([]cttsEntry, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: ctts too small", ErrInvalidMP4)
	}
	version := buf[0]
	count := binary.BigEndian.Uint32(buf[4:8])
	pos := 8
	out := make([]cttsEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		if pos+8 > len(buf) {
			return nil, fmt.Errorf("%w: ctts truncated", ErrInvalidMP4)
		}
		off := int32(binary.BigEndian.Uint32(buf[pos+4 : pos+8]))
		if version == 1 {
			off = int32(binary.BigEndian.Uint32(buf[pos+4 : pos+8]))
		}
		out = append(out, cttsEntry{
			count:  binary.BigEndian.Uint32(buf[pos : pos+4]),
			offset: off,
		})
		pos += 8
	}
	return out, nil
}

func parseStsc(r io.ReaderAt, b box) ([]stscEntry, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: stsc too small", ErrInvalidMP4)
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	pos := 8
	out := make([]stscEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		if pos+12 > len(buf) {
			return nil, fmt.Errorf("%w: stsc truncated", ErrInvalidMP4)
		}
		out = append(out, stscEntry{
			firstChunk:             binary.BigEndian.Uint32(buf[pos : pos+4]),
			samplesPerChunk:        binary.BigEndian.Uint32(buf[pos+4 : pos+8]),
			sampleDescriptionIndex: binary.BigEndian.Uint32(buf[pos+8 : pos+12]),
		})
		pos += 12
	}
	return out, nil
}

func parseStsz(r io.ReaderAt, b box) ([]uint32, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 12 {
		return nil, fmt.Errorf("%w: stsz too small", ErrInvalidMP4)
	}
	sampleSize := binary.BigEndian.Uint32(buf[4:8])
	sampleCount := binary.BigEndian.Uint32(buf[8:12])
	out := make([]uint32, 0, sampleCount)
	if sampleSize != 0 {
		for i := uint32(0); i < sampleCount; i++ {
			out = append(out, sampleSize)
		}
		return out, nil
	}
	pos := 12
	for i := uint32(0); i < sampleCount; i++ {
		if pos+4 > len(buf) {
			return nil, fmt.Errorf("%w: stsz truncated", ErrInvalidMP4)
		}
		out = append(out, binary.BigEndian.Uint32(buf[pos:pos+4]))
		pos += 4
	}
	return out, nil
}

func parseStco(r io.ReaderAt, b box) ([]uint64, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: stco too small", ErrInvalidMP4)
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	pos := 8
	out := make([]uint64, 0, count)
	for i := uint32(0); i < count; i++ {
		if pos+4 > len(buf) {
			return nil, fmt.Errorf("%w: stco truncated", ErrInvalidMP4)
		}
		out = append(out, uint64(binary.BigEndian.Uint32(buf[pos:pos+4])))
		pos += 4
	}
	return out, nil
}

func parseCo64(r io.ReaderAt, b box) ([]uint64, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: co64 too small", ErrInvalidMP4)
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	pos := 8
	out := make([]uint64, 0, count)
	for i := uint32(0); i < count; i++ {
		if pos+8 > len(buf) {
			return nil, fmt.Errorf("%w: co64 truncated", ErrInvalidMP4)
		}
		out = append(out, binary.BigEndian.Uint64(buf[pos:pos+8]))
		pos += 8
	}
	return out, nil
}

func parseStss(r io.ReaderAt, b box) (map[uint32]bool, error) {
	buf, err := readBoxBytes(r, b)
	if err != nil {
		return nil, err
	}
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: stss too small", ErrInvalidMP4)
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	pos := 8
	out := make(map[uint32]bool, count)
	for i := uint32(0); i < count; i++ {
		if pos+4 > len(buf) {
			return nil, fmt.Errorf("%w: stss truncated", ErrInvalidMP4)
		}
		out[binary.BigEndian.Uint32(buf[pos:pos+4])] = true
		pos += 4
	}
	return out, nil
}

func buildSamples(t *stblTables) ([]Sample, error) {
	sampleCount := len(t.sizes)
	durations, err := expandStts(t.stts, sampleCount)
	if err != nil {
		return nil, err
	}
	offsets, err := expandCtts(t.ctts, sampleCount)
	if err != nil {
		return nil, err
	}
	chunkSamples, err := expandStsc(t.stsc, len(t.offsets))
	if err != nil {
		return nil, err
	}
	out := make([]Sample, 0, sampleCount)
	sampleIndex := 0
	var dts int64
	for chunkIndex, chunkOffset := range t.offsets {
		count := chunkSamples[chunkIndex]
		curOffset := chunkOffset
		for i := uint32(0); i < count; i++ {
			if sampleIndex >= sampleCount {
				return nil, fmt.Errorf("%w: sample table overflow", ErrInvalidMP4)
			}
			size := t.sizes[sampleIndex]
			duration := durations[sampleIndex]
			compositionOffset := offsets[sampleIndex]
			keyframe := t.sync[uint32(sampleIndex+1)]
			if len(t.sync) == 0 {
				keyframe = true
			}
			out = append(out, Sample{
				Index:             sampleIndex + 1,
				Offset:            int64(curOffset),
				Size:              int(size),
				Duration:          duration,
				CompositionOffset: compositionOffset,
				DTS:               dts,
				PTS:               dts + int64(compositionOffset),
				Keyframe:          keyframe,
			})
			curOffset += uint64(size)
			dts += int64(duration)
			sampleIndex++
		}
	}
	if sampleIndex != sampleCount {
		return nil, fmt.Errorf("%w: sample table mismatch", ErrInvalidMP4)
	}
	return out, nil
}

func expandStts(entries []sttsEntry, sampleCount int) ([]uint32, error) {
	out := make([]uint32, 0, sampleCount)
	for _, e := range entries {
		for i := uint32(0); i < e.count; i++ {
			out = append(out, e.delta)
		}
	}
	if len(out) != sampleCount {
		return nil, fmt.Errorf("%w: stts count mismatch", ErrInvalidMP4)
	}
	return out, nil
}

func expandCtts(entries []cttsEntry, sampleCount int) ([]int32, error) {
	out := make([]int32, 0, sampleCount)
	for _, e := range entries {
		for i := uint32(0); i < e.count; i++ {
			out = append(out, e.offset)
		}
	}
	if len(out) == 0 {
		out = make([]int32, sampleCount)
	}
	if len(out) != sampleCount {
		return nil, fmt.Errorf("%w: ctts count mismatch", ErrInvalidMP4)
	}
	return out, nil
}

func expandStsc(entries []stscEntry, chunkCount int) ([]uint32, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: empty stsc", ErrInvalidMP4)
	}
	out := make([]uint32, chunkCount)
	for i := 0; i < len(entries); i++ {
		start := int(entries[i].firstChunk) - 1
		end := chunkCount
		if i+1 < len(entries) {
			end = int(entries[i+1].firstChunk) - 1
		}
		if start < 0 || start >= chunkCount {
			return nil, fmt.Errorf("%w: stsc chunk range", ErrInvalidMP4)
		}
		if end < start {
			return nil, fmt.Errorf("%w: stsc ordering", ErrInvalidMP4)
		}
		for chunk := start; chunk < end && chunk < chunkCount; chunk++ {
			out[chunk] = entries[i].samplesPerChunk
		}
	}
	for i, v := range out {
		if v == 0 {
			return nil, fmt.Errorf("%w: missing stsc entry for chunk %d", ErrInvalidMP4, i+1)
		}
	}
	return out, nil
}

func ensureSupportedAV1(c AV1CodecConfig) error {
	if len(c.Raw) == 0 {
		return fmt.Errorf("%w: missing av1C", ErrInvalidMP4)
	}
	if c.Version != 1 {
		return fmt.Errorf("%w: av1C version %d", ErrUnsupportedAV1C, c.Version)
	}
	return nil
}
