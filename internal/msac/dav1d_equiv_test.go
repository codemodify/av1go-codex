package msac

import (
	"math/bits"
	"math/rand"
	"testing"
)

type dav1dRefContext struct {
	buf            []byte
	pos            int
	dif            uint64
	rng            uint32
	cnt            int
	allowUpdateCDF bool
}

func newDav1dRefContext(buf []byte, disableCDFUpdate bool) *dav1dRefContext {
	c := &dav1dRefContext{
		buf:            buf,
		rng:            0x8000,
		cnt:            -15,
		allowUpdateCDF: !disableCDFUpdate,
	}
	c.refill()
	return c
}

func (c *dav1dRefContext) refill() {
	shift := ecWinSize - c.cnt - 24
	for shift >= 0 {
		if c.pos >= len(c.buf) {
			c.dif |= ^(^uint64(0xff) << uint(shift))
			break
		}
		c.dif |= uint64(c.buf[c.pos]^0xff) << uint(shift)
		c.pos++
		shift -= 8
	}
	c.cnt = ecWinSize - shift - 24
}

func (c *dav1dRefContext) normalize(dif uint64, rng uint32) {
	d := 15 - dav1dFloorLog2(rng)
	c.dif = dif << uint(d)
	c.rng = rng << uint(d)
	c.cnt -= d
	if c.cnt < 0 {
		c.refill()
	}
}

func (c *dav1dRefContext) debugState() (current uint32, rng uint32, cnt int, pos int) {
	return uint32(c.dif >> (ecWinSize - 16)), c.rng, c.cnt, c.pos
}

func (c *dav1dRefContext) decodeBoolEqui() uint32 {
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

func (c *dav1dRefContext) decodeBool(prob uint32) uint32 {
	r := c.rng
	dif := c.dif
	v := ((r >> 8) * (prob >> ecProbShift) >> (7 - ecProbShift)) + ecMinProb
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

func (c *dav1dRefContext) decodeBoolAdapt(cdf []uint16) uint32 {
	bit := c.decodeBool(uint32(cdf[0]))
	if c.allowUpdateCDF {
		count := cdf[1]
		rate := 4 + int(count>>4)
		if bit != 0 {
			cdf[0] += (32768 - cdf[0]) >> rate
		} else {
			cdf[0] -= cdf[0] >> rate
		}
		cdf[1] = count + boolToUint16(count < 32)
	}
	return bit
}

func (c *dav1dRefContext) decodeSymbolAdapt(cdf []uint16, nSymbols int) uint32 {
	current := uint32(c.dif >> (ecWinSize - 16))
	r := c.rng >> 8
	val := -1
	upper := uint32(0)
	lower := c.rng

	for {
		val++
		upper = lower
		if val < nSymbols {
			lower = r * uint32(cdf[val]>>ecProbShift)
			lower >>= 7 - ecProbShift
			lower += ecMinProb * uint32(nSymbols-val)
		} else {
			lower = 0
		}
		if current >= lower {
			break
		}
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
		cdf[nSymbols] = count + boolToUint16(count < 32)
	}

	return uint32(val)
}

func (c *dav1dRefContext) decodeHiTok(cdf []uint16) uint32 {
	tokBR := c.decodeSymbolAdapt(cdf, 3)
	tok := uint32(3) + tokBR
	if tokBR == 3 {
		tokBR = c.decodeSymbolAdapt(cdf, 3)
		tok = 6 + tokBR
		if tokBR == 3 {
			tokBR = c.decodeSymbolAdapt(cdf, 3)
			tok = 9 + tokBR
			if tokBR == 3 {
				tok = 12 + c.decodeSymbolAdapt(cdf, 3)
			}
		}
	}
	return tok
}

func (c *dav1dRefContext) decodeBools(n uint) uint32 {
	var v uint32
	for i := uint(0); i < n; i++ {
		v = (v << 1) | c.decodeBoolEqui()
	}
	return v
}

func (c *dav1dRefContext) decodeUniform(n uint32) int {
	l := dav1dFloorLog2(n) + 1
	m := (uint32(1) << l) - n
	v := c.decodeBools(uint(l - 1))
	if v < m {
		return int(v)
	}
	return int((v << 1) - m + c.decodeBoolEqui())
}

func dav1dFloorLog2(v uint32) int {
	if v == 0 {
		return 0
	}
	return bits.Len32(v) - 1
}

func boolToUint16(ok bool) uint16 {
	if ok {
		return 1
	}
	return 0
}

func randomSymbolCDF(r *rand.Rand, nSymbols int) []uint16 {
	out := make([]uint16, nSymbols+1)
	remaining := 32767
	for i := 0; i < nSymbols; i++ {
		minNext := nSymbols - i - 1
		next := 1 + r.Intn(maxInt(1, remaining-minNext))
		remaining -= next
		out[i] = uint16(remaining)
	}
	out[nSymbols] = uint16(r.Intn(33))
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestMSACMatchesDav1dReferenceOnMixedAdaptiveSequences(t *testing.T) {
	for seed := int64(0); seed < 32; seed++ {
		r := rand.New(rand.NewSource(seed))
		buf := make([]byte, 64)
		for i := range buf {
			buf[i] = byte(r.Intn(256))
		}
		for _, disableUpdates := range []bool{false, true} {
			got := New(buf, disableUpdates)
			want := newDav1dRefContext(buf, disableUpdates)

			for step := 0; step < 128; step++ {
				switch r.Intn(6) {
				case 0:
					if g, w := got.DecodeBoolEqui(), want.decodeBoolEqui(); g != w {
						t.Fatalf("seed=%d disableUpdates=%v step=%d op=boolEqui got=%d want=%d", seed, disableUpdates, step, g, w)
					}
				case 1:
					prob := uint32(1 + r.Intn(32767))
					if g, w := got.DecodeBool(prob), want.decodeBool(prob); g != w {
						t.Fatalf("seed=%d disableUpdates=%v step=%d op=bool prob=%d got=%d want=%d", seed, disableUpdates, step, prob, g, w)
					}
				case 2:
					cdfGot := []uint16{uint16(1 + r.Intn(32767)), uint16(r.Intn(33))}
					cdfWant := append([]uint16(nil), cdfGot...)
					g := got.DecodeBoolAdapt(cdfGot)
					w := want.decodeBoolAdapt(cdfWant)
					if g != w || cdfGot[0] != cdfWant[0] || cdfGot[1] != cdfWant[1] {
						t.Fatalf("seed=%d disableUpdates=%v step=%d op=boolAdapt got=(%d,%v) want=(%d,%v)", seed, disableUpdates, step, g, cdfGot, w, cdfWant)
					}
				case 3:
					nSymbols := 1 + r.Intn(7)
					cdfGot := randomSymbolCDF(r, nSymbols)
					cdfWant := append([]uint16(nil), cdfGot...)
					g := got.DecodeSymbolAdapt(cdfGot, nSymbols)
					w := want.decodeSymbolAdapt(cdfWant, nSymbols)
					if g != w || !equalUint16s(cdfGot, cdfWant) {
						t.Fatalf("seed=%d disableUpdates=%v step=%d op=symbol n=%d got=(%d,%v) want=(%d,%v)", seed, disableUpdates, step, nSymbols, g, cdfGot, w, cdfWant)
					}
				case 4:
					cdfGot := randomSymbolCDF(r, 3)
					cdfWant := append([]uint16(nil), cdfGot...)
					g := got.DecodeHiTok(cdfGot)
					w := want.decodeHiTok(cdfWant)
					if g != w || !equalUint16s(cdfGot, cdfWant) {
						t.Fatalf("seed=%d disableUpdates=%v step=%d op=hiTok got=(%d,%v) want=(%d,%v)", seed, disableUpdates, step, g, cdfGot, w, cdfWant)
					}
				case 5:
					n := uint32(2 + r.Intn(15))
					if g, w := got.DecodeUniform(n), want.decodeUniform(n); g != w {
						t.Fatalf("seed=%d disableUpdates=%v step=%d op=uniform n=%d got=%d want=%d", seed, disableUpdates, step, n, g, w)
					}
				}
				gCur, gRng, gCnt, gPos := got.DebugState()
				wCur, wRng, wCnt, wPos := want.debugState()
				if gCur != wCur || gRng != wRng || gCnt != wCnt || gPos != wPos {
					t.Fatalf(
						"seed=%d disableUpdates=%v step=%d state got=(%d,%d,%d,%d) want=(%d,%d,%d,%d)",
						seed, disableUpdates, step,
						gCur, gRng, gCnt, gPos,
						wCur, wRng, wCnt, wPos,
					)
				}
			}
		}
	}
}

func equalUint16s(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
