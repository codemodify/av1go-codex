package decoder

import (
	"errors"
	"fmt"
	"image"
	"io"
	"time"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
	"github.com/codemodify/av1go-codex/pkg/container/mp4"
)

var errNeedMoreInput = errors.New("decoder: need more input")

type Metadata struct {
	Profile           av1.Profile
	Level             uint8
	Tier              uint8
	BitDepth          uint8
	Width             int
	Height            int
	Chroma            av1.ChromaSubsampling
	CodecString       string
	HasSequenceHeader bool
}

type Sample struct {
	Index      int
	Offset     int64
	Data       []byte
	DTS        time.Duration
	PTS        time.Duration
	Duration   time.Duration
	IsKeyframe bool
}

type Frame struct {
	Index    int
	Image    image.Image
	Width    int
	Height   int
	BitDepth uint8
	Layout   av1.ChromaSubsampling
	Color    av1.ColorConfig
	PTS      time.Duration
	Duration time.Duration
	Y        []byte
	U        []byte
	V        []byte
	Y16      []uint16
	U16      []uint16
	V16      []uint16
	YStride  int
	UStride  int
	VStride  int
	scratch8 *scratchBuffer[byte]
	onClose  func()
}

// Close releases pooled frame storage and any optional attached resources.
func (f *Frame) Close() error {
	if f == nil {
		return nil
	}
	if f.scratch8 != nil {
		putFrameScratch(f)
	}
	if f.onClose != nil {
		onClose := f.onClose
		f.onClose = nil
		onClose()
	}
	f.Image = nil
	f.Y = nil
	f.U = nil
	f.V = nil
	f.Y16 = nil
	f.U16 = nil
	f.V16 = nil
	f.YStride = 0
	f.UStride = 0
	f.VStride = 0
	return nil
}

// SetCloseHook attaches an optional resource release hook that runs once from
// Close after any decoder-owned scratch is returned.
func (f *Frame) SetCloseHook(fn func()) {
	if f == nil {
		return
	}
	f.onClose = fn
}

type ParsedFrame struct {
	Index           int
	SampleIndex     int
	SampleOffset    int64
	DTS             time.Duration
	PTS             time.Duration
	Duration        time.Duration
	IsKeyframe      bool
	Header          obu.FrameHeader
	RefFramePOC     [7]uint8
	InitialCoefQCat int
	TileGroup       obu.TileGroup
}

type Decoder struct {
	metadata            Metadata
	header              av1.SequenceHeader
	config              []byte
	file                *mp4.File
	track               *mp4.Track
	index               int
	frameNo             int
	backend             *pixelBackend
	pending             *Sample
	configSent          bool
	closed              bool
	frameCtx            obu.FrameContext
	cdfRefQCat          [8]int
	cdfRefQCatValid     [8]bool
	parseSample         *Sample
	parseOBUs           []obu.OBU
	parseOBUIndex       int
	pendingHeader       *obu.FrameHeader
	pendingHeaderSample *Sample
	pendingTileGroup    obu.TileGroup
	parsedFrameNo       int
	lastPureGoFrame     *Frame
	pureGoRefs          [8]*Frame
	pureGoRefPOC        [8]uint8
	pureGoSegRefs       [8]*SegmentationMap
	pureGoTemporalRefs  [8]*TemporalMVMap
	currentSegField     *SegmentationMap
	lastPureGoMVField   *SpatialMVTile
	pureGoMVRefs        [8]*SpatialMVTile
	currentMVField      *SpatialMVTile
	pureGoCDFRefs       [8]*FrameCDFState
	currentCDFState     *FrameCDFState
}

func New(codecConfig []byte) (*Decoder, error) {
	return OpenCodecConfig(codecConfig)
}

func OpenCodecConfig(codecConfig []byte) (*Decoder, error) {
	av1c, header, err := obu.ParseSequenceHeaderFromAV1C(codecConfig)
	if err != nil {
		return nil, err
	}
	return &Decoder{
		metadata: Metadata{
			Profile:           header.Profile,
			Level:             header.Level,
			Tier:              header.Tier,
			BitDepth:          header.ColorConfig.BitDepth,
			Width:             header.Width(),
			Height:            header.Height(),
			Chroma:            header.ColorConfig.ChromaSubsampling(),
			CodecString:       av1c.CodecString(),
			HasSequenceHeader: true,
		},
		header:   header,
		config:   append([]byte(nil), av1c.ConfigOBUs...),
		frameCtx: obu.FrameContext{Sequence: header},
	}, nil
}

func OpenMP4(path string) (*Decoder, error) {
	f, err := mp4.Open(path)
	if err != nil {
		return nil, err
	}
	track, err := f.AV1VideoTrack()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	raw, err := track.AV1C.MarshalBinary()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	d, err := OpenCodecConfig(raw)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	d.file = f
	d.track = track
	backend, err := newPixelBackend()
	if err == nil {
		d.backend = backend
	}
	if d.metadata.Width == 0 {
		d.metadata.Width = track.Width
	}
	if d.metadata.Height == 0 {
		d.metadata.Height = track.Height
	}
	return d, nil
}

func (d *Decoder) Metadata() Metadata {
	return d.metadata
}

func (d *Decoder) CanDecodeMain8Bit420() bool {
	return d.header.SupportedMain8Bit420()
}

func (d *Decoder) CanDecodeMain10Bit420() bool {
	return d.header.SupportedMain10Bit420()
}

func (d *Decoder) NextSample() (*Sample, error) {
	if d.track == nil {
		return nil, io.EOF
	}
	if d.index >= len(d.track.Samples) {
		return nil, io.EOF
	}
	s := d.track.Samples[d.index]
	readIndex := d.index
	d.index++
	data, err := d.track.ReadSample(readIndex)
	if err != nil {
		return nil, err
	}
	return &Sample{
		Index:      s.Index,
		Offset:     s.Offset,
		Data:       data,
		DTS:        scaleTime(s.DTS, d.track.Timescale),
		PTS:        scaleTime(s.PTS, d.track.Timescale),
		Duration:   scaleTime(int64(s.Duration), d.track.Timescale),
		IsKeyframe: s.Keyframe,
	}, nil
}

func (d *Decoder) NextFrame() (*Frame, error) {
	if d.backend == nil {
		if d.track == nil {
			return nil, io.EOF
		}
		for {
			parsed, err := d.NextParsedFrame()
			if err != nil {
				return nil, err
			}
			frame, err := d.decodePureGoFrame(parsed)
			if err != nil {
				return nil, err
			}
			if parsed.Header.ShowFrame || parsed.Header.ShowExistingFrame {
				frame.Index = d.frameNo
				d.frameNo++
				return frame, nil
			}
		}
	}
	if d.track == nil {
		return nil, io.EOF
	}
	for {
		if d.pending == nil && d.index < len(d.track.Samples) {
			s := d.track.Samples[d.index]
			data, err := d.track.ReadSample(d.index)
			if err != nil {
				return nil, err
			}
			d.pending = &Sample{
				Index:      s.Index,
				Offset:     s.Offset,
				Data:       data,
				DTS:        scaleTime(s.DTS, d.track.Timescale),
				PTS:        scaleTime(s.PTS, d.track.Timescale),
				Duration:   scaleTime(int64(s.Duration), d.track.Timescale),
				IsKeyframe: s.Keyframe,
			}
		}

		if !d.configSent && len(d.config) > 0 {
			if d.pending == nil || sampleHasSequenceHeader(d.pending.Data) {
				d.configSent = true
			} else {
				sent, err := d.backend.sendSample(&Sample{
					Index:  -1,
					Offset: -1,
					Data:   d.config,
				})
				if err != nil {
					return nil, err
				}
				if sent {
					d.configSent = true
				}
			}
		}

		if d.pending != nil {
			sent, err := d.backend.sendSample(d.pending)
			if err != nil {
				if d.recoverFromDecodeError() {
					continue
				}
				return nil, err
			}
			if sent {
				d.index++
				d.pending = nil
			}
		} else if d.index >= len(d.track.Samples) {
			frame, err := d.backend.drainPicture()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil, io.EOF
				}
				return nil, err
			}
			if frame != nil {
				frame.Index = d.frameNo
				d.frameNo++
				return frame, nil
			}
			return nil, io.EOF
		}

		frame, err := d.backend.getPicture(d.frameNo)
		if err != nil {
			if errors.Is(err, errNeedMoreInput) {
				if d.pending != nil {
					continue
				}
				if d.index >= len(d.track.Samples) {
					continue
				}
				continue
			}
			if d.recoverFromDecodeError() {
				continue
			}
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			return nil, err
		}
		if frame != nil {
			frame.Index = d.frameNo
			d.frameNo++
			return frame, nil
		}
	}
}

func (d *Decoder) NextParsedFrame() (*ParsedFrame, error) {
	if d.track == nil {
		return nil, io.EOF
	}
	for {
		if d.parseSample == nil || d.parseOBUIndex >= len(d.parseOBUs) {
			sample, err := d.NextSample()
			if err != nil {
				if errors.Is(err, io.EOF) && d.pendingHeader != nil {
					return nil, fmt.Errorf("decoder: unfinished frame before end of stream")
				}
				return nil, err
			}
			obus, err := obu.Split(sample.Data)
			if err != nil {
				return nil, err
			}
			d.parseSample = sample
			d.parseOBUs = obus
			d.parseOBUIndex = 0
		}

		unit := d.parseOBUs[d.parseOBUIndex]
		d.parseOBUIndex++

		switch unit.Header.Type {
		case obu.OBUSequenceHeader:
			seq, err := obu.ParseSequenceHeader(unit.Payload)
			if err != nil {
				return nil, err
			}
			d.header = seq
			d.frameCtx.Sequence = seq
		case obu.OBUFrame:
			hdr, tilePayload, err := obu.ParseFrameOBU(unit.Payload, &d.frameCtx, unit.Header.TemporalID, unit.Header.SpatialID)
			if err != nil {
				return nil, err
			}
			group, err := parseFrameTileGroup(hdr, tilePayload)
			if err != nil {
				return nil, err
			}
			refPOC := snapshotParsedRefFramePOC(hdr, &d.frameCtx)
			initialCoefQCat := d.snapshotInitialCoefQCat(hdr)
			d.applyCDFRefRefresh(hdr, initialCoefQCat)
			d.frameCtx.ApplyRefresh(hdr)
			return d.buildParsedFrame(d.parseSample, hdr, group, refPOC, initialCoefQCat), nil
		case obu.OBUFrameHeader, obu.OBURedundantFrameHead:
			hdr, tilePayload, err := obu.ParseFrameOBU(unit.Payload, &d.frameCtx, unit.Header.TemporalID, unit.Header.SpatialID)
			if err != nil {
				return nil, err
			}
			if len(tilePayload) != 0 {
				return nil, fmt.Errorf("decoder: expected standalone frame header, found %d bytes of tile payload", len(tilePayload))
			}
			if hdr.ShowExistingFrame {
				refPOC := snapshotParsedRefFramePOC(hdr, &d.frameCtx)
				initialCoefQCat := d.snapshotInitialCoefQCat(hdr)
				d.applyCDFRefRefresh(hdr, initialCoefQCat)
				d.frameCtx.ApplyRefresh(hdr)
				return d.buildParsedFrame(d.parseSample, hdr, obu.TileGroup{}, refPOC, initialCoefQCat), nil
			}
			headerCopy := *hdr
			d.pendingHeader = &headerCopy
			d.pendingHeaderSample = d.parseSample
			d.pendingTileGroup = obu.TileGroup{}
		case obu.OBUTileGroup:
			if d.pendingHeader == nil {
				return nil, fmt.Errorf("decoder: tile group without preceding frame header")
			}
			group, err := obu.ParseTileGroup(d.pendingHeader, unit.Payload)
			if err != nil {
				return nil, err
			}
			if len(d.pendingTileGroup.Tiles) == 0 {
				d.pendingTileGroup.Start = group.Start
			}
			d.pendingTileGroup.End = group.End
			d.pendingTileGroup.Tiles = append(d.pendingTileGroup.Tiles, group.Tiles...)
			d.pendingTileGroup.Data = append(d.pendingTileGroup.Data, group.Data...)
			if group.End == totalTiles(d.pendingHeader)-1 {
				hdr := *d.pendingHeader
				refPOC := snapshotParsedRefFramePOC(&hdr, &d.frameCtx)
				initialCoefQCat := d.snapshotInitialCoefQCat(&hdr)
				finalGroup := d.pendingTileGroup
				sample := d.pendingHeaderSample
				d.pendingHeader = nil
				d.pendingHeaderSample = nil
				d.pendingTileGroup = obu.TileGroup{}
				d.applyCDFRefRefresh(&hdr, initialCoefQCat)
				d.frameCtx.ApplyRefresh(&hdr)
				return d.buildParsedFrame(sample, &hdr, finalGroup, refPOC, initialCoefQCat), nil
			}
		}
	}
}

func snapshotParsedRefFramePOC(hdr *obu.FrameHeader, ctx *obu.FrameContext) [7]uint8 {
	var out [7]uint8
	if hdr == nil || ctx == nil {
		return out
	}
	for i, refIdx := range hdr.RefIdx {
		if refIdx < 0 || int(refIdx) >= len(ctx.Refs) {
			continue
		}
		if ref := ctx.Refs[refIdx]; ref != nil {
			out[i] = ref.FrameOffset
		}
	}
	return out
}

func snapshotParsedInitialCoefQCat(hdr *obu.FrameHeader, ctx *obu.FrameContext) int {
	if hdr == nil {
		return 0
	}
	qidx := hdr.Quantization.YAC
	if ctx != nil && hdr.PrimaryRefFrame < uint8(len(hdr.RefIdx)) {
		slot := hdr.RefIdx[hdr.PrimaryRefFrame]
		if slot >= 0 && int(slot) < len(ctx.Refs) {
			if ref := ctx.Refs[slot]; ref != nil {
				qidx = ref.Quantization.YAC
			}
		}
	}
	return DefaultCoefQCat(qidx)
}

func (d *Decoder) snapshotInitialCoefQCat(hdr *obu.FrameHeader) int {
	if d == nil || hdr == nil {
		return snapshotParsedInitialCoefQCat(hdr, nil)
	}
	if hdr.PrimaryRefFrame < uint8(len(hdr.RefIdx)) {
		slot := hdr.RefIdx[hdr.PrimaryRefFrame]
		if slot >= 0 && int(slot) < len(d.cdfRefQCat) && d.cdfRefQCatValid[slot] {
			return clampDefaultCoefQCat(d.cdfRefQCat[slot])
		}
	}
	return snapshotParsedInitialCoefQCat(hdr, &d.frameCtx)
}

func (d *Decoder) applyCDFRefRefresh(hdr *obu.FrameHeader, inputQCat int) {
	if d == nil || hdr == nil {
		return
	}
	qcat := clampDefaultCoefQCat(inputQCat)
	for slot := range d.cdfRefQCat {
		if hdr.RefreshFrameFlags&(1<<slot) == 0 {
			continue
		}
		d.cdfRefQCat[slot] = qcat
		d.cdfRefQCatValid[slot] = true
	}
}

func (d *Decoder) buildParsedFrame(sample *Sample, hdr *obu.FrameHeader, group obu.TileGroup, refPOC [7]uint8, initialCoefQCat int) *ParsedFrame {
	header := *hdr
	if header.Width == 0 {
		header.Width = d.metadata.Width
	}
	if header.Height == 0 {
		header.Height = d.metadata.Height
	}
	if header.UpscaledWidth == 0 {
		header.UpscaledWidth = header.Width
	}
	if header.RenderWidth == 0 {
		header.RenderWidth = header.Width
	}
	if header.RenderHeight == 0 {
		header.RenderHeight = header.Height
	}
	frame := &ParsedFrame{
		Index:           d.parsedFrameNo,
		Header:          header,
		RefFramePOC:     refPOC,
		InitialCoefQCat: clampDefaultCoefQCat(initialCoefQCat),
		TileGroup:       group,
	}
	if sample != nil {
		frame.SampleIndex = sample.Index
		frame.SampleOffset = sample.Offset
		frame.DTS = sample.DTS
		frame.PTS = sample.PTS
		frame.Duration = sample.Duration
		frame.IsKeyframe = sample.IsKeyframe
	}
	d.parsedFrameNo++
	return frame
}

func parseFrameTileGroup(hdr *obu.FrameHeader, payload []byte) (obu.TileGroup, error) {
	if hdr == nil || hdr.ShowExistingFrame || len(payload) == 0 {
		return obu.TileGroup{}, nil
	}
	group, err := obu.ParseTileGroup(hdr, payload)
	if err != nil {
		return obu.TileGroup{}, err
	}
	return *group, nil
}

func totalTiles(hdr *obu.FrameHeader) int {
	if hdr == nil {
		return 0
	}
	return int(hdr.Tiling.Cols) * int(hdr.Tiling.Rows)
}

func (d *Decoder) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	d.releasePureGoResources()
	d.releasePureGoCDFState()
	var firstErr error
	if d.backend != nil {
		if err := d.backend.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		d.backend = nil
	}
	if d.file != nil {
		err := d.file.Close()
		d.file = nil
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func scaleTime(value int64, timescale uint32) time.Duration {
	if timescale == 0 {
		return 0
	}
	return time.Duration(float64(value) / float64(timescale) * float64(time.Second))
}

func sampleHasSequenceHeader(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	_, err := obu.FindSequenceHeader(data)
	return err == nil
}

func (d *Decoder) recoverFromDecodeError() bool {
	if d.track == nil {
		return false
	}

	next := d.index
	if d.pending != nil {
		next++
	}
	for next < len(d.track.Samples) && !d.track.Samples[next].Keyframe {
		next++
	}
	if next >= len(d.track.Samples) {
		return false
	}

	if d.backend != nil {
		_ = d.backend.Close()
		d.backend = nil
	}
	backend, err := newPixelBackend()
	if err != nil {
		return false
	}

	d.backend = backend
	d.index = next
	d.pending = nil
	d.configSent = false
	d.releasePureGoCDFState()
	return true
}
