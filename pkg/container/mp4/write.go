package mp4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// WriteVideo writes a minimal single-track AV1 MP4 file.
func WriteVideo(w io.Writer, cfg VideoConfig, samples []SampleInput) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("%w: invalid dimensions", ErrInvalidMP4)
	}
	if cfg.Timescale == 0 {
		cfg.Timescale = 1000
	}
	if len(samples) == 0 {
		return fmt.Errorf("%w: no samples", ErrInvalidMP4)
	}
	for i, s := range samples {
		if len(s.Data) == 0 {
			return fmt.Errorf("%w: empty sample %d", ErrInvalidMP4, i)
		}
		if s.Duration == 0 {
			return fmt.Errorf("%w: zero duration sample %d", ErrInvalidMP4, i)
		}
	}
	ftyp := buildFTYP()
	moov0, err := buildMoov(cfg, samples, 0)
	if err != nil {
		return err
	}
	mdatDataOffset := uint64(len(ftyp) + len(moov0) + 8)
	moov, err := buildMoov(cfg, samples, mdatDataOffset)
	if err != nil {
		return err
	}
	if _, err := w.Write(ftyp); err != nil {
		return err
	}
	if _, err := w.Write(moov); err != nil {
		return err
	}
	mdatSize := 8
	for _, s := range samples {
		mdatSize += len(s.Data)
	}
	if err := writeUint32BoxHeader(w, "mdat", uint64(mdatSize)); err != nil {
		return err
	}
	for _, s := range samples {
		if _, err := w.Write(s.Data); err != nil {
			return err
		}
	}
	return nil
}

func buildFTYP() []byte {
	buf := &bytes.Buffer{}
	_ = writeBox(buf, "ftyp", func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, [4]byte{'i', 's', 'o', 'm'}); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(512)); err != nil {
			return err
		}
		for _, brand := range []string{"isom", "av01", "iso2", "mp41"} {
			if _, err := b.WriteString(brand); err != nil {
				return err
			}
		}
		return nil
	})
	return buf.Bytes()
}

func buildMoov(cfg VideoConfig, samples []SampleInput, mdatDataOffset uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeBox(buf, "moov", func(m *bytes.Buffer) error {
		mvhd, err := buildMvhd(cfg.Timescale, totalDuration(samples))
		if err != nil {
			return err
		}
		if _, err := m.Write(mvhd); err != nil {
			return err
		}
		trak, err := buildTrak(cfg, samples, mdatDataOffset)
		if err != nil {
			return err
		}
		if _, err := m.Write(trak); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func totalDuration(samples []SampleInput) uint64 {
	var total uint64
	for _, s := range samples {
		total += uint64(s.Duration)
	}
	return total
}

func buildMvhd(timescale uint32, duration uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeFullBox(buf, "mvhd", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, timescale); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(duration)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0x00010000)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0x0100)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, [9]uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, [6]uint32{0, 0, 0, 0, 0, 0}); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(2)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildTrak(cfg VideoConfig, samples []SampleInput, mdatDataOffset uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeBox(buf, "trak", func(t *bytes.Buffer) error {
		tkhd, err := buildTkhd(cfg, totalDuration(samples))
		if err != nil {
			return err
		}
		if _, err := t.Write(tkhd); err != nil {
			return err
		}
		mdia, err := buildMdia(cfg, samples, mdatDataOffset)
		if err != nil {
			return err
		}
		if _, err := t.Write(mdia); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildTkhd(cfg VideoConfig, duration uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeFullBox(buf, "tkhd", 0, 0x000007, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(duration)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, [9]uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(cfg.Width<<16)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(cfg.Height<<16)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildMdia(cfg VideoConfig, samples []SampleInput, mdatDataOffset uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeBox(buf, "mdia", func(m *bytes.Buffer) error {
		mdhd, err := buildMdhd(cfg.Timescale, totalDuration(samples))
		if err != nil {
			return err
		}
		if _, err := m.Write(mdhd); err != nil {
			return err
		}
		hdlr := buildHdlr()
		if _, err := m.Write(hdlr); err != nil {
			return err
		}
		minf, err := buildMinf(cfg, samples, mdatDataOffset)
		if err != nil {
			return err
		}
		if _, err := m.Write(minf); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildMdhd(timescale uint32, duration uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeFullBox(buf, "mdhd", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, timescale); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(duration)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0x55c4)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildHdlr() []byte {
	buf := &bytes.Buffer{}
	writeFullBox(buf, "hdlr", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if _, err := b.WriteString("vide"); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if _, err := b.WriteString("AV1 Video"); err != nil {
			return err
		}
		if err := b.WriteByte(0); err != nil {
			return err
		}
		return nil
	})
	return buf.Bytes()
}

func buildMinf(cfg VideoConfig, samples []SampleInput, mdatDataOffset uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeBox(buf, "minf", func(m *bytes.Buffer) error {
		vmhd := buildVmhd()
		if _, err := m.Write(vmhd); err != nil {
			return err
		}
		dinf := buildDinf()
		if _, err := m.Write(dinf); err != nil {
			return err
		}
		stbl, err := buildStbl(cfg, samples, mdatDataOffset)
		if err != nil {
			return err
		}
		if _, err := m.Write(stbl); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildVmhd() []byte {
	buf := &bytes.Buffer{}
	writeFullBox(buf, "vmhd", 0, 1, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		return nil
	})
	return buf.Bytes()
}

func buildDinf() []byte {
	buf := &bytes.Buffer{}
	writeBox(buf, "dinf", func(d *bytes.Buffer) error {
		dref := &bytes.Buffer{}
		writeFullBox(dref, "dref", 0, 0, func(b *bytes.Buffer) error {
			if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
				return err
			}
			url := &bytes.Buffer{}
			writeFullBox(url, "url ", 0, 1, func(*bytes.Buffer) error { return nil })
			if _, err := b.Write(url.Bytes()); err != nil {
				return err
			}
			return nil
		})
		if _, err := d.Write(dref.Bytes()); err != nil {
			return err
		}
		return nil
	})
	return buf.Bytes()
}

func buildStbl(cfg VideoConfig, samples []SampleInput, mdatDataOffset uint64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := writeBox(buf, "stbl", func(s *bytes.Buffer) error {
		stsd, err := buildStsd(cfg)
		if err != nil {
			return err
		}
		if _, err := s.Write(stsd); err != nil {
			return err
		}
		if _, err := s.Write(buildStts(samples)); err != nil {
			return err
		}
		if _, err := s.Write(buildStsc(samples)); err != nil {
			return err
		}
		if _, err := s.Write(buildStsz(samples)); err != nil {
			return err
		}
		if _, err := s.Write(buildStco(mdatDataOffset)); err != nil {
			return err
		}
		if stss := buildStss(samples); len(stss) > 0 {
			if _, err := s.Write(stss); err != nil {
				return err
			}
		}
		if ctts, err := buildCtts(samples); err != nil {
			return err
		} else if len(ctts) > 0 {
			if _, err := s.Write(ctts); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildStsd(cfg VideoConfig) ([]byte, error) {
	av1c, err := cfg.AV1C.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	if err := writeFullBox(buf, "stsd", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
			return err
		}
		entry := &bytes.Buffer{}
		if err := writeBox(entry, "av01", func(box *bytes.Buffer) error {
			if err := binary.Write(box, binary.BigEndian, [6]byte{}); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint16(1)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, [16]byte{}); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint16(cfg.Width)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint16(cfg.Height)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint32(0x00480000)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint32(0x00480000)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint32(0)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint16(1)); err != nil {
				return err
			}
			name := [32]byte{}
			if _, err := box.Write(name[:]); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint16(0x0018)); err != nil {
				return err
			}
			if err := binary.Write(box, binary.BigEndian, uint16(0xffff)); err != nil {
				return err
			}
			return writeBox(box, "av1C", func(b *bytes.Buffer) error {
				_, err := b.Write(av1c)
				return err
			})
		}); err != nil {
			return err
		}
		if _, err := b.Write(entry.Bytes()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildStts(samples []SampleInput) []byte {
	type run struct {
		dur   uint32
		count uint32
	}
	var runs []run
	for _, s := range samples {
		if len(runs) > 0 && runs[len(runs)-1].dur == s.Duration {
			runs[len(runs)-1].count++
			continue
		}
		runs = append(runs, run{dur: s.Duration, count: 1})
	}
	buf := &bytes.Buffer{}
	writeFullBox(buf, "stts", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(len(runs))); err != nil {
			return err
		}
		for _, r := range runs {
			if err := binary.Write(b, binary.BigEndian, r.count); err != nil {
				return err
			}
			if err := binary.Write(b, binary.BigEndian, r.dur); err != nil {
				return err
			}
		}
		return nil
	})
	return buf.Bytes()
}

func buildStsc(samples []SampleInput) []byte {
	buf := &bytes.Buffer{}
	writeFullBox(buf, "stsc", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(len(samples))); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
			return err
		}
		return nil
	})
	return buf.Bytes()
}

func buildStsz(samples []SampleInput) []byte {
	buf := &bytes.Buffer{}
	writeFullBox(buf, "stsz", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(0)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(len(samples))); err != nil {
			return err
		}
		for _, s := range samples {
			if err := binary.Write(b, binary.BigEndian, uint32(len(s.Data))); err != nil {
				return err
			}
		}
		return nil
	})
	return buf.Bytes()
}

func buildStco(mdatDataOffset uint64) []byte {
	buf := &bytes.Buffer{}
	writeFullBox(buf, "stco", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(1)); err != nil {
			return err
		}
		if err := binary.Write(b, binary.BigEndian, uint32(mdatDataOffset)); err != nil {
			return err
		}
		return nil
	})
	return buf.Bytes()
}

func buildStss(samples []SampleInput) []byte {
	var syncs []uint32
	for i, s := range samples {
		if s.Keyframe {
			syncs = append(syncs, uint32(i+1))
		}
	}
	if len(syncs) == 0 {
		return nil
	}
	buf := &bytes.Buffer{}
	writeFullBox(buf, "stss", 0, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(len(syncs))); err != nil {
			return err
		}
		for _, n := range syncs {
			if err := binary.Write(b, binary.BigEndian, n); err != nil {
				return err
			}
		}
		return nil
	})
	return buf.Bytes()
}

func buildCtts(samples []SampleInput) ([]byte, error) {
	type run struct {
		off   int32
		count uint32
	}
	var runs []run
	for _, s := range samples {
		if len(runs) > 0 && runs[len(runs)-1].off == s.CompositionOffset {
			runs[len(runs)-1].count++
			continue
		}
		runs = append(runs, run{off: s.CompositionOffset, count: 1})
	}
	allZero := true
	for _, r := range runs {
		if r.off != 0 {
			allZero = false
			break
		}
	}
	if len(runs) == 0 || allZero {
		return nil, nil
	}
	version := uint8(0)
	for _, r := range runs {
		if r.off < 0 {
			version = 1
			break
		}
	}
	buf := &bytes.Buffer{}
	if err := writeFullBox(buf, "ctts", version, 0, func(b *bytes.Buffer) error {
		if err := binary.Write(b, binary.BigEndian, uint32(len(runs))); err != nil {
			return err
		}
		for _, r := range runs {
			if err := binary.Write(b, binary.BigEndian, r.count); err != nil {
				return err
			}
			if version == 1 {
				if err := binary.Write(b, binary.BigEndian, uint32(r.off)); err != nil {
					return err
				}
			} else {
				if r.off < 0 {
					return fmt.Errorf("%w: negative ctts offset", ErrInvalidMP4)
				}
				if err := binary.Write(b, binary.BigEndian, uint32(r.off)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeUint32BoxHeader(w io.Writer, typ string, size uint64) error {
	return writeBoxHeader(w, typ, size)
}

func writeBox(buf *bytes.Buffer, typ string, fn func(*bytes.Buffer) error) error {
	payload := &bytes.Buffer{}
	if err := fn(payload); err != nil {
		return err
	}
	if err := writeBoxHeader(buf, typ, uint64(payload.Len()+8)); err != nil {
		return err
	}
	_, err := buf.Write(payload.Bytes())
	return err
}

func writeFullBox(buf *bytes.Buffer, typ string, version uint8, flags uint32, fn func(*bytes.Buffer) error) error {
	payload := &bytes.Buffer{}
	if err := payload.WriteByte(version); err != nil {
		return err
	}
	if err := payload.WriteByte(byte(flags >> 16)); err != nil {
		return err
	}
	if err := payload.WriteByte(byte(flags >> 8)); err != nil {
		return err
	}
	if err := payload.WriteByte(byte(flags)); err != nil {
		return err
	}
	if err := fn(payload); err != nil {
		return err
	}
	if err := writeBoxHeader(buf, typ, uint64(payload.Len()+8)); err != nil {
		return err
	}
	_, err := buf.Write(payload.Bytes())
	return err
}

func writeBoxHeader(w io.Writer, typ string, size uint64) error {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(size))
	copy(hdr[4:8], typ)
	_, err := w.Write(hdr[:])
	return err
}
