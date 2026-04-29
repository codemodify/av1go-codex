package msac

import "testing"

func TestDecodeBoolEquiExtremes(t *testing.T) {
	ones := New([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, false)
	for i := 0; i < 4; i++ {
		if got := ones.DecodeBoolEqui(); got != 1 {
			t.Fatalf("all-0xff DecodeBoolEqui[%d] = %d, want 1", i, got)
		}
	}

	zeros := New([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, false)
	for i := 0; i < 4; i++ {
		if got := zeros.DecodeBoolEqui(); got != 0 {
			t.Fatalf("all-0x00 DecodeBoolEqui[%d] = %d, want 0", i, got)
		}
	}
}

func TestDecodeBoolAdaptUpdatesCDF(t *testing.T) {
	ctx := New([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, false)
	cdf := []uint16{16384, 0}

	bit := ctx.DecodeBoolAdapt(cdf)
	if bit != 1 {
		t.Fatalf("DecodeBoolAdapt bit = %d, want 1", bit)
	}
	if got, want := cdf[0], uint16(17408); got != want {
		t.Fatalf("cdf[0] = %d, want %d", got, want)
	}
	if got, want := cdf[1], uint16(1); got != want {
		t.Fatalf("cdf[1] = %d, want %d", got, want)
	}
}

func TestDecodeUniformAndSubexp(t *testing.T) {
	ctx := New([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, false)

	if got := ctx.DecodeUniform(10); got != 0 {
		t.Fatalf("DecodeUniform = %d, want 0", got)
	}
	if got := ctx.DecodeSubexp(0, 64, 3); got != 0 {
		t.Fatalf("DecodeSubexp = %d, want 0", got)
	}
}

func TestDecodeBoolEquiMatchesHalfProbabilityBool(t *testing.T) {
	for seed := 0; seed < 256; seed++ {
		buf := make([]byte, 16)
		for i := range buf {
			buf[i] = byte((seed*37 + i*29) & 0xff)
		}

		equi := New(buf, true)
		half := New(buf, true)
		for i := 0; i < 32; i++ {
			if got, want := equi.DecodeBoolEqui(), half.DecodeBool(16384); got != want {
				t.Fatalf("seed=%d step=%d DecodeBoolEqui=%d DecodeBool(16384)=%d", seed, i, got, want)
			}
		}
	}
}
