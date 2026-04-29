package bitio

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrUnderflow = errors.New("bitio: buffer underflow")

type Reader struct {
	data []byte
	bit  int
}

func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

func (r *Reader) RemainingBits() int {
	return len(r.data)*8 - r.bit
}

func (r *Reader) Position() int {
	return r.bit
}

func (r *Reader) ReadBit() (bool, error) {
	if r.RemainingBits() < 1 {
		return false, ErrUnderflow
	}
	byteIndex := r.bit / 8
	bitIndex := 7 - (r.bit % 8)
	r.bit++
	return ((r.data[byteIndex] >> bitIndex) & 0x1) == 1, nil
}

func (r *Reader) ReadBits(n int) (uint64, error) {
	if n < 0 || n > 64 {
		return 0, fmt.Errorf("bitio: invalid bit count %d", n)
	}
	if r.RemainingBits() < n {
		return 0, ErrUnderflow
	}
	var v uint64
	for i := 0; i < n; i++ {
		b, err := r.ReadBit()
		if err != nil {
			return 0, err
		}
		v <<= 1
		if b {
			v |= 1
		}
	}
	return v, nil
}

func (r *Reader) ReadSBits(n int) (int32, error) {
	if n <= 0 || n > 32 {
		return 0, fmt.Errorf("bitio: invalid signed bit count %d", n)
	}
	v, err := r.ReadBits(n)
	if err != nil {
		return 0, err
	}
	shift := 64 - n
	return int32(int64(v<<shift) >> shift), nil
}

func (r *Reader) ReadUint(n int) (uint32, error) {
	v, err := r.ReadBits(n)
	return uint32(v), err
}

func (r *Reader) ReadBool() (bool, error) {
	return r.ReadBit()
}

func (r *Reader) ReadFlag() (bool, error) {
	return r.ReadBool()
}

func (r *Reader) ReadUint8(n int) (uint8, error) {
	v, err := r.ReadBits(n)
	return uint8(v), err
}

func (r *Reader) ReadUint16(n int) (uint16, error) {
	v, err := r.ReadBits(n)
	return uint16(v), err
}

func (r *Reader) ReadUint32(n uint) (uint32, error) {
	v, err := r.ReadBits(int(n))
	return uint32(v), err
}

func (r *Reader) ReadUE() (uint32, error) {
	leadingZeroBits := 0
	for {
		bit, err := r.ReadBit()
		if err != nil {
			return 0, err
		}
		if bit {
			break
		}
		leadingZeroBits++
	}
	if leadingZeroBits == 0 {
		return 0, nil
	}
	suffix, err := r.ReadBits(leadingZeroBits)
	if err != nil {
		return 0, err
	}
	return uint32((1 << leadingZeroBits) - 1 + int(suffix)), nil
}

func (r *Reader) SkipBits(n int) error {
	if n < 0 {
		return fmt.Errorf("bitio: invalid skip count %d", n)
	}
	if r.RemainingBits() < n {
		return ErrUnderflow
	}
	r.bit += n
	return nil
}

func (r *Reader) ByteAlign() {
	if rem := r.bit % 8; rem != 0 {
		r.bit += 8 - rem
	}
}

func (r *Reader) ReadULEB128() (uint32, error) {
	var (
		value uint64
		shift uint
	)
	for i := 0; i < 8; i++ {
		b, err := r.ReadBits(8)
		if err != nil {
			return 0, err
		}
		value |= (b & 0x7f) << shift
		if (b & 0x80) == 0 {
			if value > uint64(^uint32(0)) {
				return 0, errors.New("bitio: uleb128 overflow")
			}
			return uint32(value), nil
		}
		shift += 7
	}
	return 0, errors.New("bitio: invalid uleb128 value")
}

func (r *Reader) ReadUniform(max uint32) (uint32, error) {
	if max <= 1 {
		return 0, fmt.Errorf("bitio: invalid uniform max %d", max)
	}
	l := floorLog2(max) + 1
	m := (uint32(1) << l) - max
	v, err := r.ReadBits(l - 1)
	if err != nil {
		return 0, err
	}
	if uint32(v) < m {
		return uint32(v), nil
	}
	bit, err := r.ReadBit()
	if err != nil {
		return 0, err
	}
	if bit {
		return (uint32(v) << 1) - m + 1, nil
	}
	return (uint32(v) << 1) - m, nil
}

func (r *Reader) ReadVLC() (uint32, error) {
	bit, err := r.ReadBit()
	if err != nil {
		return 0, err
	}
	if bit {
		return 0, nil
	}

	nBits := 0
	for {
		bit, err := r.ReadBit()
		if err != nil {
			return 0, err
		}
		nBits++
		if nBits == 32 {
			return ^uint32(0), nil
		}
		if bit {
			break
		}
	}

	suffix, err := r.ReadBits(nBits)
	if err != nil {
		return 0, err
	}
	return ((uint32(1) << nBits) - 1) + uint32(suffix), nil
}

func (r *Reader) ReadSubexp(ref int, n uint) (int, error) {
	base := 1 << n
	v, err := r.readSubexpU(uint(ref+base), uint(2*base))
	if err != nil {
		return 0, err
	}
	return int(v) - base, nil
}

func (r *Reader) readSubexpU(ref, n uint) (uint, error) {
	v := uint(0)
	for i := 0; ; i++ {
		b := 3
		if i > 0 {
			b += i - 1
		}

		if n < v+3*(1<<b) {
			u, err := r.ReadUniform(uint32(n - v + 1))
			if err != nil {
				return 0, err
			}
			v += uint(u)
			break
		}

		flag, err := r.ReadBit()
		if err != nil {
			return 0, err
		}
		if !flag {
			bits, err := r.ReadBits(b)
			if err != nil {
				return 0, err
			}
			v += uint(bits)
			break
		}

		v += 1 << b
	}

	if ref*2 <= n {
		return invRecenter(ref, v), nil
	}
	return n - invRecenter(n-ref, v), nil
}

func ReadULEB128(data []byte) (value uint64, consumed int, err error) {
	for i := 0; i < len(data) && i < 8; i++ {
		b := data[i]
		value |= uint64(b&0x7f) << (7 * i)
		consumed++
		if (b & 0x80) == 0 {
			return value, consumed, nil
		}
	}
	if consumed == 0 {
		return 0, 0, ErrUnderflow
	}
	return 0, consumed, errors.New("bitio: invalid uleb128 value")
}

func PutULEB128(dst []byte, value uint64) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if value == 0 {
			return dst
		}
	}
}

func PutUint32(dst []byte, value uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], value)
	return append(dst, buf[:]...)
}

func floorLog2(v uint32) int {
	out := 0
	for v > 1 {
		v >>= 1
		out++
	}
	return out
}

func invRecenter(r, v uint) uint {
	if v > (r << 1) {
		return v
	}
	if (v & 1) == 0 {
		return (v >> 1) + r
	}
	return r - ((v + 1) >> 1)
}
