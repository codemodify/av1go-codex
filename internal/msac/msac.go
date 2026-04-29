package msac

import "math/bits"

const (
	ecProbShift = 6
	ecMinProb   = 4
	ecWinSize   = 64
)

type Context struct {
	buf            []byte
	pos            int
	dif            uint64
	rng            uint32
	cnt            int
	allowUpdateCDF bool
}

func (c *Context) DebugState() (current uint32, rng uint32, cnt int, pos int) {
	if c == nil {
		return 0, 0, 0, 0
	}
	return uint32(c.dif >> (ecWinSize - 16)), c.rng, c.cnt, c.pos
}

func (c *Context) Clone() *Context {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}

func New(data []byte, disableCDFUpdate bool) *Context {
	c := &Context{
		buf:            data,
		dif:            (uint64(1) << (ecWinSize - 1)) - 1,
		rng:            0x8000,
		cnt:            -15,
		allowUpdateCDF: !disableCDFUpdate,
	}
	c.refill()
	return c
}

func (c *Context) DecodeBoolEqui() uint32 {
	r := c.rng
	dif := c.dif
	v := ((r >> 8) << 7) + ecMinProb
	vw := uint64(v) << (ecWinSize - 16)
	ret := uint32(0)
	if dif >= vw {
		ret = 1
		dif -= vw
		v += r - 2*v
	}
	c.normalize(dif, v)
	return 1 - ret
}

func (c *Context) DecodeBool(f uint32) uint32 {
	r := c.rng
	dif := c.dif
	v := ((r >> 8) * (f >> ecProbShift) >> (7 - ecProbShift)) + ecMinProb
	vw := uint64(v) << (ecWinSize - 16)
	ret := uint32(0)
	if dif >= vw {
		ret = 1
		dif -= vw
		v += r - 2*v
	}
	c.normalize(dif, v)
	return 1 - ret
}

func (c *Context) DecodeBools(n uint) uint32 {
	var v uint32
	for i := uint(0); i < n; i++ {
		v = (v << 1) | c.DecodeBoolEqui()
	}
	return v
}

func (c *Context) DecodeUniform(n uint32) int {
	l := floorLog2(n) + 1
	m := (uint32(1) << l) - n
	v := c.DecodeBools(uint(l - 1))
	if v < m {
		return int(v)
	}
	return int((v << 1) - m + c.DecodeBoolEqui())
}

func (c *Context) DecodeSubexp(ref, n int, k uint) int {
	a := uint32(0)
	if c.DecodeBoolEqui() != 0 {
		if c.DecodeBoolEqui() != 0 {
			k += uint(c.DecodeBoolEqui()) + 1
		}
		a = 1 << k
	}
	v := c.DecodeBools(k) + a
	if ref*2 <= n {
		return int(invRecenter(uint(ref), uint(v)))
	}
	return n - 1 - int(invRecenter(uint(n-1-ref), uint(v)))
}

func (c *Context) DecodeSymbolAdapt(cdf []uint16, nSymbols int) uint32 {
	current := uint32(c.dif >> (ecWinSize - 16))
	r := c.rng >> 8
	upper := c.rng
	lower := uint32(0)
	val := nSymbols

	for i := 0; i < nSymbols; i++ {
		split := r * uint32(cdf[i]>>ecProbShift)
		split >>= 7 - ecProbShift
		split += ecMinProb * uint32(nSymbols-i)
		if current >= split {
			val = i
			lower = split
			break
		}
		upper = split
	}

	c.normalize(c.dif-(uint64(lower)<<(ecWinSize-16)), upper-lower)

	if c.allowUpdateCDF {
		count := cdf[nSymbols]
		rate := 4 + int(count>>4)
		if nSymbols > 2 {
			rate++
		}
		for i := 0; i < val; i++ {
			cdf[i] += (32768 - cdf[i]) >> rate
		}
		for i := val; i < nSymbols; i++ {
			cdf[i] -= cdf[i] >> rate
		}
		if count < 32 {
			cdf[nSymbols] = count + 1
		}
	}

	return uint32(val)
}

func (c *Context) DecodeBoolAdapt(cdf []uint16) uint32 {
	bit := c.DecodeBool(uint32(cdf[0]))
	if c.allowUpdateCDF {
		count := cdf[1]
		rate := 4 + int(count>>4)
		if bit != 0 {
			cdf[0] += (32768 - cdf[0]) >> rate
		} else {
			cdf[0] -= cdf[0] >> rate
		}
		if count < 32 {
			cdf[1] = count + 1
		}
	}
	return bit
}

func (c *Context) DecodeHiTok(cdf []uint16) uint32 {
	tokBR := c.DecodeSymbolAdapt(cdf, 3)
	tok := uint32(3) + tokBR
	if tokBR == 3 {
		tokBR = c.DecodeSymbolAdapt(cdf, 3)
		tok = 6 + tokBR
		if tokBR == 3 {
			tokBR = c.DecodeSymbolAdapt(cdf, 3)
			tok = 9 + tokBR
			if tokBR == 3 {
				tok = 12 + c.DecodeSymbolAdapt(cdf, 3)
			}
		}
	}
	return tok
}

func (c *Context) refill() {
	pos := c.pos
	shift := ecWinSize - c.cnt - 24
	dif := c.dif
	for shift >= 0 && pos < len(c.buf) {
		dif ^= uint64(c.buf[pos]) << uint(shift)
		pos++
		shift -= 8
	}
	c.dif = dif
	c.cnt = ecWinSize - shift - 24
	c.pos = pos
}

func (c *Context) normalize(dif uint64, rng uint32) {
	d := 15 - floorLog2(rng)
	c.cnt -= d
	c.dif = ((dif + 1) << d) - 1
	c.rng = rng << d
	if c.cnt < 0 {
		c.refill()
	}
}

func floorLog2(v uint32) int {
	if v == 0 {
		return 0
	}
	return bits.Len32(v) - 1
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
