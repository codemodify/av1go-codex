package obu

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/codemodify/av1go-codex/internal/bitio"
	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/container/mp4"
)

func TestParseAndSplit(t *testing.T) {
	var data []byte

	data = append(data, byte(0x0a)) // sequence header, size field present
	data = bitio.PutULEB128(data, 3)
	data = append(data, []byte{0x11, 0x22, 0x33}...)

	data = append(data, byte(0x12)) // temporal delimiter, size field present
	data = bitio.PutULEB128(data, 1)
	data = append(data, 0x99)

	units, err := Split(data)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("Split got %d units", len(units))
	}
	if units[0].Header.Type != OBUSequenceHeader {
		t.Fatalf("first unit type = %d", units[0].Header.Type)
	}
	if !bytes.Equal(units[0].Payload, []byte{0x11, 0x22, 0x33}) {
		t.Fatalf("first payload mismatch: %x", units[0].Payload)
	}
	if units[1].Header.Type != OBUTemporalDelimiter {
		t.Fatalf("second unit type = %d", units[1].Header.Type)
	}
	if !bytes.Equal(units[1].Payload, []byte{0x99}) {
		t.Fatalf("second payload mismatch: %x", units[1].Payload)
	}
}

func TestSplitRejectsNoSizeField(t *testing.T) {
	_, err := Split([]byte{0x08, 0x00})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSequenceHeaderFromAV1CSampleMain8Bit(t *testing.T) {
	raw, err := hex.DecodeString("81040c000a0b00000024cf7f0dbfff3008")
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}

	cfg, header, err := ParseSequenceHeaderFromAV1C(raw)
	if err != nil {
		t.Fatalf("ParseSequenceHeaderFromAV1C: %v", err)
	}

	if got, want := cfg.CodecString(), "av01.0.04M.08"; got != want {
		t.Fatalf("cfg.CodecString = %q, want %q", got, want)
	}
	if got, want := header.Profile, av1.ProfileMain; got != want {
		t.Fatalf("Profile = %v, want %v", got, want)
	}
	if got, want := header.Width(), 960; got != want {
		t.Fatalf("Width = %d, want %d", got, want)
	}
	if got, want := header.Height(), 540; got != want {
		t.Fatalf("Height = %d, want %d", got, want)
	}
	if got, want := header.ColorConfig.BitDepth, uint8(8); got != want {
		t.Fatalf("BitDepth = %d, want %d", got, want)
	}
	if got, want := header.ColorConfig.ChromaSubsampling(), av1.Chroma420; got != want {
		t.Fatalf("Chroma = %v, want %v", got, want)
	}
	if !header.EnableOrderHint {
		t.Fatal("expected order hint enabled")
	}
	if got, want := header.OrderHintBitsMinus1, uint8(6); got != want {
		t.Fatalf("OrderHintBitsMinus1 = %d, want %d", got, want)
	}
}

func TestParseFrameOBUFixtureSequence(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "frame-seq.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   4,
		BitDepth: 8,
	})

	f, err := mp4.Open(fixture.Path)
	if err != nil {
		t.Fatalf("mp4.Open: %v", err)
	}
	defer f.Close()

	track, err := f.AV1VideoTrack()
	if err != nil {
		t.Fatalf("AV1VideoTrack: %v", err)
	}
	raw, err := track.AV1C.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	_, seq, err := ParseSequenceHeaderFromAV1C(raw)
	if err != nil {
		t.Fatalf("ParseSequenceHeaderFromAV1C: %v", err)
	}
	ctx := &FrameContext{Sequence: seq}

	parsed := 0
	for sampleIdx := 0; sampleIdx < min(len(track.Samples), 3); sampleIdx++ {
		data, err := track.ReadSample(sampleIdx)
		if err != nil {
			t.Fatalf("ReadSample(%d): %v", sampleIdx, err)
		}
		obus, err := Split(data)
		if err != nil {
			t.Fatalf("Split sample %d: %v", sampleIdx, err)
		}
		for _, obu := range obus {
			if obu.Header.Type != OBUFrame {
				continue
			}
			hdr, tilePayload, err := ParseFrameOBU(obu.Payload, ctx, obu.Header.TemporalID, obu.Header.SpatialID)
			if err != nil {
				t.Fatalf("ParseFrameOBU sample %d: %v", sampleIdx, err)
			}
			group, err := ParseTileGroup(hdr, tilePayload)
			if err != nil {
				t.Fatalf("ParseTileGroup sample %d: %v", sampleIdx, err)
			}
			if hdr.Width != fixture.Width || hdr.Height != fixture.Height {
				t.Fatalf("frame dims = %dx%d, want %dx%d", hdr.Width, hdr.Height, fixture.Width, fixture.Height)
			}
			if parsed == 0 && hdr.FrameType != FrameTypeKey {
				t.Fatalf("first frame type = %d, want key", hdr.FrameType)
			}
			if len(group.Tiles) == 0 {
				t.Fatal("expected at least one tile")
			}
			ctx.ApplyRefresh(hdr)
			parsed++
		}
	}
	if parsed == 0 {
		t.Fatal("expected at least one parsed frame header")
	}
}
