package decoder

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
)

type bundledDecodeBenchmarkInput struct {
	metadata Metadata
	config   []byte
	frames   []ParsedFrame
	shown    int
}

type benchmarkCoefficientEntropy struct {
	adapt     []uint32
	adaptPos  int
	symbol    []uint32
	symbolPos int
	equi      []uint32
	equiPos   int
	hitok     []uint32
	hitokPos  int
}

func (d *benchmarkCoefficientEntropy) Reset(adapt, symbol, equi, hitok []uint32) {
	d.adapt = adapt
	d.adaptPos = 0
	d.symbol = symbol
	d.symbolPos = 0
	d.equi = equi
	d.equiPos = 0
	d.hitok = hitok
	d.hitokPos = 0
}

func (d *benchmarkCoefficientEntropy) DecodeBoolEqui() uint32 {
	if d.equiPos >= len(d.equi) {
		panic("benchmark coefficient entropy: equi script exhausted")
	}
	v := d.equi[d.equiPos]
	d.equiPos++
	return v
}

func (d *benchmarkCoefficientEntropy) DecodeBoolAdapt([]uint16) uint32 {
	if d.adaptPos >= len(d.adapt) {
		panic("benchmark coefficient entropy: adapt script exhausted")
	}
	v := d.adapt[d.adaptPos]
	d.adaptPos++
	return v
}

func (d *benchmarkCoefficientEntropy) DecodeSymbolAdapt([]uint16, int) uint32 {
	if d.symbolPos >= len(d.symbol) {
		panic("benchmark coefficient entropy: symbol script exhausted")
	}
	v := d.symbol[d.symbolPos]
	d.symbolPos++
	return v
}

func (d *benchmarkCoefficientEntropy) DecodeHiTok([]uint16) uint32 {
	if d.hitokPos >= len(d.hitok) {
		panic("benchmark coefficient entropy: hitok script exhausted")
	}
	v := d.hitok[d.hitokPos]
	d.hitokPos++
	return v
}

func loadBundledDecodeBenchmarkInput(b testing.TB, sample string, shownTarget int) bundledDecodeBenchmarkInput {
	b.Helper()

	path := testutil.RepoPath(b, "testvideo", sample)
	if _, err := os.Stat(path); err != nil {
		b.Skipf("sample unavailable: %s: %v", path, err)
	}

	dec, err := OpenMP4(path)
	if err != nil {
		b.Fatalf("OpenMP4(%s): %v", sample, err)
	}
	defer dec.Close()

	input := bundledDecodeBenchmarkInput{
		metadata: dec.Metadata(),
	}
	rawConfig, err := dec.track.AV1C.MarshalBinary()
	if err != nil {
		b.Fatalf("MarshalBinary(%s): %v", sample, err)
	}
	input.config = rawConfig
	for input.shown < shownTarget {
		parsed, err := dec.NextParsedFrame()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			b.Fatalf("NextParsedFrame(%s): %v", sample, err)
		}
		input.frames = append(input.frames, *parsed)
		if parsed.Header.ShowFrame || parsed.Header.ShowExistingFrame {
			input.shown++
		}
	}
	if input.shown == 0 {
		b.Fatalf("expected at least one shown frame for %s", sample)
	}
	return input
}

func benchmarkBundledPureGoDecode(b *testing.B, sample string, shownTarget int) {
	input := loadBundledDecodeBenchmarkInput(b, sample, shownTarget)

	b.ReportAllocs()
	if input.metadata.Width > 0 && input.metadata.Height > 0 && input.shown > 0 {
		b.SetBytes(int64(input.metadata.Width * input.metadata.Height * input.shown))
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dec, err := OpenCodecConfig(input.config)
		if err != nil {
			b.Fatalf("OpenCodecConfig(%s): %v", sample, err)
		}
		shown := 0
		for j := range input.frames {
			frame, err := dec.decodePureGoFrame(&input.frames[j])
			if err != nil {
				b.Fatalf("decodePureGoFrame(%s frame %d): %v", sample, j, err)
			}
			if input.frames[j].Header.ShowFrame || input.frames[j].Header.ShowExistingFrame {
				if frame == nil || frame.Image == nil {
					b.Fatalf("decodePureGoFrame(%s frame %d): missing image", sample, j)
				}
				shown++
			}
			if frame != nil {
				b.StopTimer()
				if err := frame.Close(); err != nil {
					b.Fatalf("Frame.Close(%s frame %d): %v", sample, j, err)
				}
				b.StartTimer()
			}
		}
		if shown != input.shown {
			b.Fatalf("decoded shown frames = %d, want %d", shown, input.shown)
		}
		b.StopTimer()
		if err := dec.Close(); err != nil {
			b.Fatalf("Close(%s): %v", sample, err)
		}
		b.StartTimer()
	}
}

func BenchmarkDecodeBundledPureGoFrames(b *testing.B) {
	cases := []struct {
		name        string
		sample      string
		shownFrames int
	}{
		{name: "Sintel720p", sample: "Sintel_720_10s_20MB.mp4", shownFrames: 4},
		{name: "Sintel1080p", sample: "Sintel_1080_10s_20MB.mp4", shownFrames: 2},
	}
	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			benchmarkBundledPureGoDecode(b, tc.sample, tc.shownFrames)
		})
	}
}

func BenchmarkDecodeLumaCoefficientsDCT2D(b *testing.B) {
	cases := []struct {
		name   string
		adapt  []uint32
		symbol []uint32
		equi   []uint32
		hitok  []uint32
	}{
		{
			name:   "OneAC",
			adapt:  []uint32{0, 0},
			symbol: []uint32{1, 0, 0},
			equi:   []uint32{0},
		},
		{
			name:   "GolombAC",
			adapt:  []uint32{0, 0},
			symbol: []uint32{1, 2, 0},
			equi:   []uint32{0, 0, 1, 0},
			hitok:  []uint32{15},
		},
	}

	g := BlockGeometry{
		Size:    Block4x4,
		Start4X: 0,
		Start4Y: 0,
		End4X:   1,
		End4Y:   1,
	}
	coef := NewDefaultCoefCDF(0)
	dq := DequantTables{{{0, 8}}}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			var dec benchmarkCoefficientEntropy
			var above, left BlockContext

			b.ReportAllocs()
			b.SetBytes(16)
			for i := 0; i < b.N; i++ {
				above.Reset(true, 0)
				left.Reset(true, 0)
				dec.Reset(tc.adapt, tc.symbol, tc.equi, tc.hitok)
				coeffs, _, skip, err := DecodeLumaCoefficientsDCT2D(Block4x4, g, TX4X4, 0, coef, dq, &above, &left, &dec)
				if err != nil {
					b.Fatalf("DecodeLumaCoefficientsDCT2D: %v", err)
				}
				if skip {
					b.Fatal("unexpected coefficient skip in benchmark")
				}
				if len(coeffs) == 0 {
					b.Fatal("expected coefficient buffer")
				}
			}
		})
	}
}

func benchmarkDenseCoeffSet(tx TxfmSize) []int32 {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	coeffs := make([]int32, width*height)
	for i := range coeffs {
		coeffs[i] = int32((i%7)-3) * 4
	}
	coeffs[0] = 64
	coeffs[len(coeffs)/3] = -48
	coeffs[len(coeffs)/2] = 24
	return coeffs
}

func BenchmarkInverseAddDCTDCT8Dense(b *testing.B) {
	cases := []struct {
		name string
		tx   TxfmSize
	}{
		{name: "TX8x8", tx: TX8X8},
		{name: "TX16x16", tx: TX16X16},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			info := TxfmInfoFor(tc.tx)
			width := int(info.W4) * 4
			height := int(info.H4) * 4
			stride := width
			base := make([]byte, width*height)
			for i := range base {
				base[i] = byte((i * 13) & 0xff)
			}
			dst := make([]byte, len(base))
			coeffs := benchmarkDenseCoeffSet(tc.tx)

			copy(dst, base)
			if err := inverseAddDCTDCT8Dense(dst, stride, tc.tx, coeffs); err != nil {
				b.Fatalf("inverseAddDCTDCT8Dense warmup: %v", err)
			}

			b.ReportAllocs()
			b.SetBytes(int64(width * height))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(dst, base)
				if err := inverseAddDCTDCT8Dense(dst, stride, tc.tx, coeffs); err != nil {
					b.Fatalf("inverseAddDCTDCT8Dense: %v", err)
				}
			}
		})
	}
}
