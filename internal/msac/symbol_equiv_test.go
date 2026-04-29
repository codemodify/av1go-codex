package msac

import "testing"

func TestDecodeSymbolAdaptMatchesBoolAdaptForBinaryCDF(t *testing.T) {
	for prob := 1; prob < 32768; prob += 997 {
		for seed := 0; seed < 64; seed++ {
			buf := make([]byte, 16)
			for i := range buf {
				buf[i] = byte((seed*37 + i*29 + prob) & 0xff)
			}

			boolCtx := New(buf, true)
			symCtx := boolCtx.Clone()

			boolBit := boolCtx.DecodeBoolAdapt([]uint16{uint16(prob), 0})
			symBit := symCtx.DecodeSymbolAdapt([]uint16{uint16(prob), 0}, 1)
			if boolBit != symBit {
				t.Fatalf("prob=%d seed=%d bool=%d symbol=%d", prob, seed, boolBit, symBit)
			}
		}
	}
}

func TestDecodeSymbolAdaptMatchesBoolAdaptForBinaryCDFWithUpdates(t *testing.T) {
	for prob := 1; prob < 32768; prob += 1993 {
		for seed := 0; seed < 32; seed++ {
			buf := make([]byte, 16)
			for i := range buf {
				buf[i] = byte((seed*53 + i*17 + prob) & 0xff)
			}

			boolCtx := New(buf, false)
			symCtx := boolCtx.Clone()
			boolCDF := []uint16{uint16(prob), 0}
			symCDF := []uint16{uint16(prob), 0}

			for step := 0; step < 32; step++ {
				boolBit := boolCtx.DecodeBoolAdapt(boolCDF)
				symBit := symCtx.DecodeSymbolAdapt(symCDF, 1)
				if boolBit != symBit {
					t.Fatalf("prob=%d seed=%d step=%d bool=%d symbol=%d", prob, seed, step, boolBit, symBit)
				}
				if boolCDF[0] != symCDF[0] || boolCDF[1] != symCDF[1] {
					t.Fatalf("prob=%d seed=%d step=%d boolCDF=%v symCDF=%v", prob, seed, step, boolCDF, symCDF)
				}
			}
		}
	}
}
