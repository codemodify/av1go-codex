package testutil

import (
	"fmt"
	"testing"
)

// OBUHeader captures the parsed AV1 OBU header bits.
type OBUHeader struct {
	Type         uint8
	Extension    bool
	HasSizeField bool
	TemporalID   uint8
	SpatialID    uint8
	HeaderBytes  int
}

// OBU is a parsed AV1 OBU with header and payload bytes.
type OBU struct {
	Header  OBUHeader
	Size    uint64
	Payload []byte
	Total   int
}

// ParseOBUHeader parses the AV1 OBU header and optional extension bytes.
func ParseOBUHeader(tb testing.TB, data []byte) (OBUHeader, error) {
	tb.Helper()
	if len(data) < 1 {
		return OBUHeader{}, fmt.Errorf("obu header truncated")
	}

	first := data[0]
	if first&0x80 != 0 {
		return OBUHeader{}, fmt.Errorf("obu forbidden_zero_bit set")
	}

	hdr := OBUHeader{
		Type:         (first >> 3) & 0x0f,
		Extension:    first&0x04 != 0,
		HasSizeField: first&0x02 != 0,
	}
	if first&0x01 != 0 {
		return OBUHeader{}, fmt.Errorf("obu reserved bit set")
	}

	consumed := 1
	if hdr.Extension {
		if len(data) < 2 {
			return OBUHeader{}, fmt.Errorf("obu extension truncated")
		}
		second := data[1]
		hdr.TemporalID = second >> 5
		hdr.SpatialID = (second >> 3) & 0x03
		if second&0x07 != 0 {
			return OBUHeader{}, fmt.Errorf("obu extension reserved bits set")
		}
		consumed++
	}
	hdr.HeaderBytes = consumed
	return hdr, nil
}

// ParseULEB128 parses an unsigned LEB128 value.
func ParseULEB128(tb testing.TB, data []byte) (uint64, int, error) {
	tb.Helper()
	var value uint64
	var shift uint
	for i, b := range data {
		if shift >= 64 {
			return 0, 0, fmt.Errorf("uleb128 overflow")
		}
		value |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return value, i + 1, nil
		}
		shift += 7
	}
	return 0, 0, fmt.Errorf("uleb128 truncated")
}

// ParseOBU parses one OBU, including the variable-length size field if present.
func ParseOBU(tb testing.TB, data []byte) (OBU, error) {
	tb.Helper()
	hdr, err := ParseOBUHeader(tb, data)
	if err != nil {
		return OBU{}, err
	}
	off := hdr.HeaderBytes
	if !hdr.HasSizeField {
		return OBU{Header: hdr, Payload: data[off:], Total: len(data)}, nil
	}
	size, n, err := ParseULEB128(tb, data[off:])
	if err != nil {
		return OBU{}, err
	}
	off += n
	if uint64(len(data[off:])) < size {
		return OBU{}, fmt.Errorf("obu payload truncated: want %d bytes, have %d", size, len(data[off:]))
	}
	payloadEnd := off + int(size)
	payload := append([]byte(nil), data[off:payloadEnd]...)
	return OBU{
		Header:  hdr,
		Size:    size,
		Payload: payload,
		Total:   payloadEnd,
	}, nil
}
