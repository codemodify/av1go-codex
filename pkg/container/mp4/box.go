package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

type box struct {
	typ        string
	size       uint64
	headerSize uint64
	dataOffset uint64
}

func readAtFull(r io.ReaderAt, off uint64, p []byte) (int, error) {
	var n int
	for n < len(p) {
		m, err := r.ReadAt(p[n:], int64(off)+int64(n))
		n += m
		if err != nil {
			if err == io.EOF && n == len(p) {
				return n, nil
			}
			return n, err
		}
	}
	return n, nil
}

func readBoxHeader(r io.ReaderAt, off uint64, limit uint64) (box, error) {
	if off+8 > limit {
		return box{}, io.EOF
	}
	var hdr [8]byte
	if _, err := readAtFull(r, off, hdr[:]); err != nil {
		return box{}, err
	}
	size32 := binary.BigEndian.Uint32(hdr[0:4])
	typ := string(hdr[4:8])
	out := box{typ: typ, size: uint64(size32), headerSize: 8, dataOffset: off + 8}
	switch size32 {
	case 0:
		out.size = limit - off
	case 1:
		var ext [8]byte
		if _, err := readAtFull(r, off+8, ext[:]); err != nil {
			return box{}, err
		}
		out.size = binary.BigEndian.Uint64(ext[:])
		out.headerSize = 16
		out.dataOffset = off + 16
	}
	if out.size < out.headerSize {
		return box{}, fmt.Errorf("%w: box %s too small", ErrInvalidMP4, typ)
	}
	if off+out.size > limit {
		return box{}, fmt.Errorf("%w: box %s overruns file", ErrInvalidMP4, typ)
	}
	return out, nil
}

func payloadSize(b box) uint64 {
	return b.size - b.headerSize
}

func walkBoxes(r io.ReaderAt, start uint64, size uint64, fn func(box) error) error {
	pos := start
	limit := start + size
	for pos < limit {
		b, err := readBoxHeader(r, pos, limit)
		if err != nil {
			return err
		}
		if err := fn(b); err != nil {
			return err
		}
		pos += b.size
	}
	return nil
}

func readBoxBytes(r io.ReaderAt, b box) ([]byte, error) {
	buf := make([]byte, payloadSize(b))
	if _, err := readAtFull(r, b.dataOffset, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
