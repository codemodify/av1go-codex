package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
)

func getLoCtxDav1dRef2D(rawLevels []uint8, tx TxfmSize, rc int) int {
	if rc == 0 {
		return 0
	}
	info := TxfmInfoFor(tx)
	h := 4 << minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	shift := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH)) + 2
	x := rc >> shift
	y := rc & (h - 1)
	stride := h + 4

	encoded := make([]uint8, len(rawLevels))
	for i, tok := range rawLevels {
		encoded[i] = encodeCoeffContextLevel(tok)
	}

	mag := uint32(coeffContextLevel(encoded, stride, x+0, y+1)) +
		uint32(coeffContextLevel(encoded, stride, x+1, y+0)) +
		uint32(coeffContextLevel(encoded, stride, x+1, y+1)) +
		uint32(coeffContextLevel(encoded, stride, x+0, y+2)) +
		uint32(coeffContextLevel(encoded, stride, x+2, y+0))
	offset := generatedLoContextOffsets[loContextOffsetGroup(tx)][minInt(y, 4)][minInt(x, 4)]
	return int(offset) + minInt(int((mag+64)>>7), 4)
}

func getBRCtxDav1dRef2D(rawLevels []uint8, tx TxfmSize, rc int) int {
	info := TxfmInfoFor(tx)
	h := 4 << minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
	shift := minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH)) + 2
	x := rc >> shift
	y := rc & (h - 1)
	stride := h + 4

	encoded := make([]uint8, len(rawLevels))
	for i, tok := range rawLevels {
		encoded[i] = encodeCoeffContextLevel(tok)
	}

	mag := uint32(coeffContextLevel(encoded, stride, x+0, y+1)) +
		uint32(coeffContextLevel(encoded, stride, x+1, y+0)) +
		uint32(coeffContextLevel(encoded, stride, x+1, y+1))
	ctx := hiTokenContextOffset(mag & 0x3f)
	if rc == 0 {
		return ctx
	}
	if x < 2 && y < 2 {
		return 7 + ctx
	}
	return 14 + ctx
}

func TestDecodeGolomb(t *testing.T) {
	tests := []struct {
		name string
		bits []uint32
		want uint32
	}{
		{name: "zero", bits: []uint32{1}, want: 0},
		{name: "one", bits: []uint32{0, 1, 0}, want: 1},
		{name: "two", bits: []uint32{0, 1, 1}, want: 2},
		{name: "three", bits: []uint32{0, 0, 1, 0, 0}, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := 0
			got := decodeGolomb(func() uint32 {
				if i >= len(tt.bits) {
					t.Fatal("decodeGolomb read past end of bit slice")
				}
				bit := tt.bits[i]
				i++
				return bit
			})
			if got != tt.want {
				t.Fatalf("decodeGolomb() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetDCSignCtx(t *testing.T) {
	if got := GetDCSignCtx(TX8X8, []uint8{0x40, 0x40}, []uint8{0x40, 0x40}); got != 0 {
		t.Fatalf("neutral dc sign ctx = %d, want 0", got)
	}
	if got := GetDCSignCtx(TX8X8, []uint8{0x80, 0x40}, []uint8{0x40, 0x40}); got != 2 {
		t.Fatalf("positive dc sign ctx = %d, want 2", got)
	}
	if got := GetDCSignCtx(TX8X8, []uint8{0x00, 0x40}, []uint8{0x40, 0x40}); got != 1 {
		t.Fatalf("negative dc sign ctx = %d, want 1", got)
	}
}

func TestGetSkipCtxLumaWholeBlock(t *testing.T) {
	got := GetSkipCtx(TxfmInfoFor(TX8X8), Block8x8, []uint8{0x40, 0x40}, []uint8{0x40, 0x40}, false, av1.Chroma420)
	if got != 0 {
		t.Fatalf("GetSkipCtx() = %d, want 0 for whole-block luma tx", got)
	}
}

func TestGetSkipCtxLumaSubTx(t *testing.T) {
	got := GetSkipCtx(TxfmInfoFor(TX8X8), Block16x16, []uint8{3, 0x40}, []uint8{0x40, 0x40}, false, av1.Chroma420)
	if want := int(generatedSkipContext[3][0]); got != want {
		t.Fatalf("GetSkipCtx() = %d, want %d", got, want)
	}
}

func TestGetSkipCtxWithNeighborsLumaCornerSubTx(t *testing.T) {
	got := GetSkipCtxWithNeighbors(TxfmInfoFor(TX8X8), Block32x32, []uint8{0x40, 0x40}, []uint8{0x40, 0x40}, false, av1.Chroma420, false, false)
	if want := int(generatedSkipContext[0][0]); got != want {
		t.Fatalf("GetSkipCtxWithNeighbors() = %d, want %d for split luma tx with neutral edge contexts", got, want)
	}
}

func TestGetSkipCtxChroma(t *testing.T) {
	got := GetSkipCtx(TxfmInfoFor(TX4X4), Block16x16, []uint8{0x80}, []uint8{0x40}, true, av1.Chroma420)
	if got != 11 {
		t.Fatalf("GetSkipCtx(chroma) = %d, want 11", got)
	}
}

func TestGetLoCtx2D(t *testing.T) {
	rc := 6 // x=1, y=2 in a 4x4 coefficient plane
	x, y, stride := coeffContextXY(TX4X4, rc)
	levels := make([]uint8, coeffLevelsBufferSize(TX4X4))
	levels[(x+0)*stride+(y+1)] = 1
	levels[(x+1)*stride+y] = 2
	levels[(x+2)*stride+y] = 4
	levels[(x+0)*stride+(y+2)] = 5
	levels[(x+1)*stride+(y+1)] = 3

	got := GetLoCtx(levels, TxClass2D, TX4X4, rc)
	want := int(generatedLoContextOffsets[loContextOffsetGroup(TX4X4)][minInt(y, 4)][minInt(x, 4)]) + 4
	if got != want {
		t.Fatalf("GetLoCtx(2D) = %d, want %d", got, want)
	}
}

func TestGetBRCtx2D(t *testing.T) {
	rc := 6 // x=1, y=2 in a 4x4 coefficient plane
	x, y, stride := coeffContextXY(TX4X4, rc)
	levels := make([]uint8, coeffLevelsBufferSize(TX4X4))
	levels[(x+0)*stride+(y+1)] = 1
	levels[(x+1)*stride+y] = 2
	levels[(x+1)*stride+(y+1)] = 3

	if got := GetBRCtx(levels, TxClass2D, TX4X4, rc); got != 17 {
		t.Fatalf("GetBRCtx(2D) = %d, want 17", got)
	}
	if got := GetBRCtx(levels, TxClass2D, TX4X4, 0); got != 0 {
		t.Fatalf("GetBRCtx(2D, dc) = %d, want 0", got)
	}
}

func TestGetLoCtx1D(t *testing.T) {
	rc := 16 // x=2, y=0 in an RTX4X8 coefficient plane
	x, y, stride := coeffContextXY(RTX4X8, rc)
	levels := make([]uint8, coeffLevelsBufferSize(RTX4X8))
	levels[(x+1)*stride+y] = 1
	levels[(x+0)*stride+(y+1)] = 2
	levels[(x+2)*stride+y] = 3
	levels[(x+3)*stride+y] = 4
	levels[(x+4)*stride+y] = 5

	if got := GetLoCtx(levels, TxClassH, RTX4X8, rc); got != 40 {
		t.Fatalf("GetLoCtx(1D) = %d, want 40", got)
	}
	if got := GetBRCtx(levels, TxClassH, RTX4X8, rc); got != 17 {
		t.Fatalf("GetBRCtx(1D) = %d, want 17", got)
	}
}

func TestGetLoCtx2DMatchesDav1dReferenceAcrossScanPositions(t *testing.T) {
	for tx := TxfmSize(0); tx < numRectTxfmSizes; tx++ {
		info := TxfmInfoFor(tx)
		if info.W4 == 0 || info.H4 == 0 {
			continue
		}
		scanWidth, scanHeight := ScanCoeffArea(tx)
		if scanWidth == 0 || scanHeight == 0 {
			continue
		}
		rawLevels := make([]uint8, coeffLevelsBufferSize(tx))
		stride := scanHeight + 4
		for x := 0; x < scanWidth+2; x++ {
			for y := 0; y < scanHeight+2; y++ {
				rawLevels[x*stride+y] = uint8((x*7 + y*11 + int(tx)) % 6)
			}
		}
		for rc := 0; rc < scanWidth*scanHeight; rc++ {
			got := GetLoCtx(rawLevels, TxClass2D, tx, rc)
			want := getLoCtxDav1dRef2D(rawLevels, tx, rc)
			if got != want {
				t.Fatalf("tx=%d rc=%d GetLoCtx()=%d want=%d", tx, rc, got, want)
			}
		}
	}
}

func TestGetBRCtx2DMatchesDav1dReferenceAcrossScanPositions(t *testing.T) {
	for tx := TxfmSize(0); tx < numRectTxfmSizes; tx++ {
		info := TxfmInfoFor(tx)
		if info.W4 == 0 || info.H4 == 0 {
			continue
		}
		scanWidth, scanHeight := ScanCoeffArea(tx)
		if scanWidth == 0 || scanHeight == 0 {
			continue
		}
		rawLevels := make([]uint8, coeffLevelsBufferSize(tx))
		stride := scanHeight + 4
		for x := 0; x < scanWidth+2; x++ {
			for y := 0; y < scanHeight+2; y++ {
				rawLevels[x*stride+y] = uint8((x*5 + y*9 + int(tx)) % 6)
			}
		}
		for rc := 0; rc < scanWidth*scanHeight; rc++ {
			if rc == 0 {
				continue
			}
			got := GetBRCtx(rawLevels, TxClass2D, tx, rc)
			want := getBRCtxDav1dRef2D(rawLevels, tx, rc)
			if got != want {
				t.Fatalf("tx=%d rc=%d GetBRCtx()=%d want=%d", tx, rc, got, want)
			}
		}
	}
}
