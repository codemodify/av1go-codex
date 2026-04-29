package obu

import (
	"testing"

	"github.com/codemodify/av1go-codex/internal/bitio"
	"github.com/codemodify/av1go-codex/pkg/av1"
)

type testBitWriter struct {
	data []byte
	bit  int
}

func (w *testBitWriter) WriteBit(v bool) {
	if w.bit%8 == 0 {
		w.data = append(w.data, 0)
	}
	if v {
		w.data[w.bit/8] |= 1 << (7 - (w.bit % 8))
	}
	w.bit++
}

func (w *testBitWriter) WriteBits(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.WriteBit(((v >> i) & 1) != 0)
	}
}

func (w *testBitWriter) Bytes() []byte {
	return append([]byte(nil), w.data...)
}

func TestParseFilmGrainInline(t *testing.T) {
	seq := av1.SequenceHeader{
		FilmGrainParamsPresent: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:     8,
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	h := &FrameHeader{
		FrameType: FrameTypeKey,
		ShowFrame: true,
	}
	for i := range h.RefIdx {
		h.RefIdx[i] = -1
	}

	var bits testBitWriter
	bits.WriteBit(true)           // present
	bits.WriteBits(0x1234, 16)    // seed
	bits.WriteBits(1, 4)          // num_y_points
	bits.WriteBits(10, 8)         // y x
	bits.WriteBits(20, 8)         // y y
	bits.WriteBit(false)          // chroma_scaling_from_luma
	bits.WriteBits(1, 4)          // num_uv_points[0]
	bits.WriteBits(15, 8)         // uv0 x
	bits.WriteBits(25, 8)         // uv0 y
	bits.WriteBits(1, 4)          // num_uv_points[1]
	bits.WriteBits(16, 8)         // uv1 x
	bits.WriteBits(26, 8)         // uv1 y
	bits.WriteBits(1, 2)          // scaling_shift -> 9
	bits.WriteBits(0, 2)          // ar_coeff_lag
	bits.WriteBits(130, 8)        // ar_coeffs_uv[0][0] -> 2
	bits.WriteBits(131, 8)        // ar_coeffs_uv[1][0] -> 3
	bits.WriteBits(2, 2)          // ar_coeff_shift -> 8
	bits.WriteBits(3, 2)          // grain_scale_shift
	bits.WriteBits(129, 8)        // uv_mult[0] -> 1
	bits.WriteBits(127, 8)        // uv_luma_mult[0] -> -1
	bits.WriteBits(300, 9)        // uv_offset[0] -> 44
	bits.WriteBits(128, 8)        // uv_mult[1] -> 0
	bits.WriteBits(126, 8)        // uv_luma_mult[1] -> -2
	bits.WriteBits(255, 9)        // uv_offset[1] -> -1
	bits.WriteBit(true)           // overlap_flag
	bits.WriteBit(false)          // clip_to_restricted_range

	if err := parseFilmGrain(bitio.NewReader(bits.Bytes()), h, &FrameContext{Sequence: seq}, seq); err != nil {
		t.Fatalf("parseFilmGrain: %v", err)
	}

	if !h.FilmGrainPresent {
		t.Fatal("FilmGrainPresent = false, want true")
	}
	if !h.FilmGrain.Update {
		t.Fatal("FilmGrain.Update = false, want true")
	}
	if got, want := h.FilmGrain.Seed, uint16(0x1234); got != want {
		t.Fatalf("Seed = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.NumYPoints, uint8(1); got != want {
		t.Fatalf("NumYPoints = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.YPoints[0], [2]uint8{10, 20}; got != want {
		t.Fatalf("YPoints[0] = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.NumUVPoints, [2]uint8{1, 1}; got != want {
		t.Fatalf("NumUVPoints = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.UVPoints[0][0], [2]uint8{15, 25}; got != want {
		t.Fatalf("UVPoints[0][0] = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.UVPoints[1][0], [2]uint8{16, 26}; got != want {
		t.Fatalf("UVPoints[1][0] = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.ScalingShift, uint8(9); got != want {
		t.Fatalf("ScalingShift = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.ARCoeffsUV[0][0], int8(2); got != want {
		t.Fatalf("ARCoeffsUV[0][0] = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.ARCoeffsUV[1][0], int8(3); got != want {
		t.Fatalf("ARCoeffsUV[1][0] = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.ARCoeffShift, uint8(8); got != want {
		t.Fatalf("ARCoeffShift = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.GrainScaleShift, uint8(3); got != want {
		t.Fatalf("GrainScaleShift = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.UVMult, [2]int8{1, 0}; got != want {
		t.Fatalf("UVMult = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.UVLumaMult, [2]int8{-1, -2}; got != want {
		t.Fatalf("UVLumaMult = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.UVOffset, [2]int16{44, -1}; got != want {
		t.Fatalf("UVOffset = %v, want %v", got, want)
	}
	if !h.FilmGrain.OverlapFlag {
		t.Fatal("OverlapFlag = false, want true")
	}
	if h.FilmGrain.ClipToRestrictedRange {
		t.Fatal("ClipToRestrictedRange = true, want false")
	}
}

func TestParseFilmGrainCopiesReferenceParams(t *testing.T) {
	seq := av1.SequenceHeader{
		FilmGrainParamsPresent: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:     8,
			SubsamplingX: true,
			SubsamplingY: true,
		},
	}
	ref := &FrameHeader{
		FilmGrainPresent: true,
		FilmGrain: FilmGrainParams{
			Update:          true,
			Seed:            0x1111,
			NumYPoints:      1,
			YPoints:         [14][2]uint8{{12, 34}},
			ScalingShift:    9,
			ARCoeffShift:    7,
			GrainScaleShift: 2,
		},
	}
	ctx := &FrameContext{Sequence: seq}
	ctx.Refs[4] = ref

	h := &FrameHeader{
		FrameType: FrameTypeInter,
		ShowFrame: true,
	}
	for i := range h.RefIdx {
		h.RefIdx[i] = -1
	}
	h.RefIdx[0] = 4

	var bits testBitWriter
	bits.WriteBit(true)        // present
	bits.WriteBits(0x2222, 16) // seed
	bits.WriteBit(false)       // update
	bits.WriteBits(4, 3)       // ref index

	if err := parseFilmGrain(bitio.NewReader(bits.Bytes()), h, ctx, seq); err != nil {
		t.Fatalf("parseFilmGrain: %v", err)
	}

	if !h.FilmGrainPresent {
		t.Fatal("FilmGrainPresent = false, want true")
	}
	if h.FilmGrain.Update {
		t.Fatal("FilmGrain.Update = true, want false")
	}
	if got, want := h.FilmGrain.RefIndex, uint8(4); got != want {
		t.Fatalf("RefIndex = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.Seed, uint16(0x2222); got != want {
		t.Fatalf("Seed = %d, want %d", got, want)
	}
	if got, want := h.FilmGrain.YPoints[0], ref.FilmGrain.YPoints[0]; got != want {
		t.Fatalf("YPoints[0] = %v, want %v", got, want)
	}
	if got, want := h.FilmGrain.ScalingShift, ref.FilmGrain.ScalingShift; got != want {
		t.Fatalf("ScalingShift = %d, want %d", got, want)
	}
}
