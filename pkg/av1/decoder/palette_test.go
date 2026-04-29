package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type scriptedPaletteEntropy struct {
	t                *testing.T
	uniforms         []int
	symbols          []uint32
	expectedNSymbols []int
}

func (s *scriptedPaletteEntropy) DecodeBool(uint32) uint32        { return 0 }
func (s *scriptedPaletteEntropy) DecodeBoolEqui() uint32          { return 0 }
func (s *scriptedPaletteEntropy) DecodeBoolAdapt([]uint16) uint32 { return 0 }
func (s *scriptedPaletteEntropy) DecodeBools(uint) uint32         { return 0 }

func (s *scriptedPaletteEntropy) DecodeUniform(uint32) int {
	if len(s.uniforms) == 0 {
		s.t.Fatal("unexpected DecodeUniform")
	}
	v := s.uniforms[0]
	s.uniforms = s.uniforms[1:]
	return v
}

func (s *scriptedPaletteEntropy) DecodeSymbolAdapt(_ []uint16, n int) uint32 {
	if len(s.expectedNSymbols) == 0 {
		s.t.Fatal("unexpected DecodeSymbolAdapt")
	}
	wantN := s.expectedNSymbols[0]
	s.expectedNSymbols = s.expectedNSymbols[1:]
	if n != wantN {
		s.t.Fatalf("DecodeSymbolAdapt n = %d, want %d", n, wantN)
	}
	if len(s.symbols) == 0 {
		s.t.Fatal("missing scripted symbol")
	}
	v := s.symbols[0]
	s.symbols = s.symbols[1:]
	return v
}

func TestPaletteOrderForCellUsesNeighborScores(t *testing.T) {
	pal := []uint8{
		0, 1, 0,
		0, 0, 0,
	}
	order, ctx := paletteOrderForCell(pal, 3, 1, 1, 3)

	if got, want := ctx, 2; got != want {
		t.Fatalf("ctx = %d, want %d", got, want)
	}
	if got, want := order[0], uint8(0); got != want {
		t.Fatalf("order[0] = %d, want %d", got, want)
	}
	if got, want := order[1], uint8(1); got != want {
		t.Fatalf("order[1] = %d, want %d", got, want)
	}
	if got, want := order[2], uint8(2); got != want {
		t.Fatalf("order[2] = %d, want %d", got, want)
	}
}

func TestPaletteOrderForCellUsesNeighborContextBuckets(t *testing.T) {
	tests := []struct {
		name string
		pal  []uint8
		x    int
		y    int
		size int
		ctx  int
		head [3]uint8
	}{
		{
			name: "all neighbors same",
			pal: []uint8{
				3, 3,
				3, 0,
			},
			x:    1,
			y:    1,
			size: 4,
			ctx:  4,
			head: [3]uint8{3, 0, 1},
		},
		{
			name: "left and top same",
			pal: []uint8{
				1, 2,
				2, 0,
			},
			x:    1,
			y:    1,
			size: 4,
			ctx:  3,
			head: [3]uint8{2, 1, 0},
		},
		{
			name: "all neighbors distinct",
			pal: []uint8{
				2, 3,
				1, 0,
			},
			x:    1,
			y:    1,
			size: 4,
			ctx:  1,
			head: [3]uint8{1, 3, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, ctx := paletteOrderForCell(tt.pal, 2, tt.x, tt.y, tt.size)
			if got := ctx; got != tt.ctx {
				t.Fatalf("ctx = %d, want %d", got, tt.ctx)
			}
			for i, want := range tt.head {
				if got := order[i]; got != want {
					t.Fatalf("order[%d] = %d, want %d", i, got, want)
				}
			}
		})
	}
}

func TestPaletteOrderForCellBoundaryContext(t *testing.T) {
	pal := []uint8{
		4, 7, 0,
		0, 0, 0,
	}
	order, ctx := paletteOrderForCell(pal, 3, 1, 0, 8)

	if got, want := ctx, 0; got != want {
		t.Fatalf("ctx = %d, want %d", got, want)
	}
	if got, want := order[0], uint8(4); got != want {
		t.Fatalf("order[0] = %d, want %d", got, want)
	}
}

func TestPaletteAllowedForBlockMatchesMiSizeGate(t *testing.T) {
	hdr := &obu.FrameHeader{AllowScreenTools: true}
	tests := []struct {
		name string
		size BlockSize
		want bool
	}{
		{name: "4x8", size: Block4x8, want: false},
		{name: "8x4", size: Block8x4, want: false},
		{name: "4x16", size: Block4x16, want: true},
		{name: "16x4", size: Block16x4, want: true},
		{name: "8x8", size: Block8x8, want: true},
		{name: "16x16", size: Block16x16, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dims := BlockDims(tc.size)
			g := BlockGeometry{Size: tc.size, End4X: int(dims.W4), End4Y: int(dims.H4)}
			if got := paletteAllowedForBlock(hdr, g); got != tc.want {
				t.Fatalf("paletteAllowedForBlock(%s) = %v, want %v", blockName(tc.size), got, tc.want)
			}
		})
	}
}

func TestPaletteIndexDimensionsUseVisibleGeometry(t *testing.T) {
	g := BlockGeometry{
		Size:    Block32x16,
		Start4X: 0,
		Start4Y: 204,
		End4X:   8,
		End4Y:   206,
	}
	w, h := lumaPaletteIndexDimensions(g)
	if got, want := [2]int{w, h}, [2]int{32, 8}; got != want {
		t.Fatalf("lumaPaletteIndexDimensions = %v, want %v", got, want)
	}

	w, h = chromaPaletteIndexDimensions(av1.Chroma420, g)
	if got, want := [2]int{w, h}, [2]int{16, 4}; got != want {
		t.Fatalf("chromaPaletteIndexDimensions = %v, want %v", got, want)
	}
}

func TestReadPaletteIndicesMapsImplicitLastColor(t *testing.T) {
	dec := &scriptedPaletteEntropy{
		t:                t,
		uniforms:         []int{2},
		symbols:          []uint32{2},
		expectedNSymbols: []int{2},
	}
	var cdf ModeCDF
	idx, err := readPaletteIndices(&cdf, dec, 3, 0, 2, 1)
	if err != nil {
		t.Fatalf("readPaletteIndices: %v", err)
	}
	if len(idx) != 2 {
		t.Fatalf("len(idx) = %d, want 2", len(idx))
	}
	if got, want := idx[0], uint8(2); got != want {
		t.Fatalf("idx[0] = %d, want %d", got, want)
	}
	// With left-only context, dav1d-style ordering is [2,0,1]. Symbol 2 is the
	// implicit last color and must decode to palette index 1.
	if got, want := idx[1], uint8(1); got != want {
		t.Fatalf("idx[1] = %d, want %d", got, want)
	}
}

func TestPaletteDiagonalXOrderDefaultsToDescendingX(t *testing.T) {
	start, end, step := paletteDiagonalXOrder(3, 4, 4)
	if start != 3 || end != 0 || step != -1 {
		t.Fatalf("paletteDiagonalXOrder default = (%d,%d,%d), want (3,0,-1)", start, end, step)
	}
}

func TestPaletteDiagonalXOrderSupportsReverseDiagOverride(t *testing.T) {
	t.Setenv("DEBUG_PALETTE_INDEX_ORDER", "reverse_diag")
	start, end, step := paletteDiagonalXOrder(3, 4, 4)
	if start != 0 || end != 3 || step != 1 {
		t.Fatalf("paletteDiagonalXOrder reverse_diag = (%d,%d,%d), want (0,3,1)", start, end, step)
	}
}

func TestDebugApplyPaletteCacheModeDisablesCache(t *testing.T) {
	t.Setenv("DEBUG_PALETTE_CACHE_MODE", "disable")
	cache := [16]uint16{11, 29}
	got, n := debugApplyPaletteCacheMode(cache, 2)
	if n != 0 {
		t.Fatalf("cache size = %d, want 0", n)
	}
	if got[0] != 11 || got[1] != 29 {
		t.Fatalf("cache contents changed unexpectedly: %v", got[:2])
	}
}

func TestDebugPaletteYBitsAdjust(t *testing.T) {
	if got := debugPaletteYBitsAdjust(); got != 0 {
		t.Fatalf("default bits adjust = %d, want 0", got)
	}
	t.Setenv("DEBUG_PALETTE_Y_BITS_ADJUST", "-1")
	if got := debugPaletteYBitsAdjust(); got != -1 {
		t.Fatalf("bits adjust = %d, want -1", got)
	}
}

func TestDebugPaletteYMinStepOverride(t *testing.T) {
	if got, ok := debugPaletteYMinStepOverride(); ok || got != 0 {
		t.Fatalf("default min step override = (%d,%v), want (0,false)", got, ok)
	}
	t.Setenv("DEBUG_PALETTE_Y_MIN_STEP", "0")
	if got, ok := debugPaletteYMinStepOverride(); !ok || got != 0 {
		t.Fatalf("min step override = (%d,%v), want (0,true)", got, ok)
	}
}

func TestDebugApplyPaletteCacheModeReversesCache(t *testing.T) {
	t.Setenv("DEBUG_PALETTE_CACHE_MODE", "reverse")
	cache := [16]uint16{11, 29, 41}
	got, n := debugApplyPaletteCacheMode(cache, 3)
	if n != 3 {
		t.Fatalf("cache size = %d, want 3", n)
	}
	if got[0] != 41 || got[1] != 29 || got[2] != 11 {
		t.Fatalf("cache = %v, want [41 29 11]", got[:3])
	}
}

func TestPaletteCacheSkipsAboveEntryAtSB64BoundaryInside128Superblock(t *testing.T) {
	var above, left BlockContext
	above.PalSz[3] = 2
	above.Pal[0][3][0] = 11
	above.Pal[0][3][1] = 29

	_, n := paletteCache(&above, &left, 0, 3, 16)
	if got, want := n, 0; got != want {
		t.Fatalf("cache size = %d, want %d", got, want)
	}
}

func TestPaletteCacheUsesAboveEntryBelowSB64BoundaryInside128Superblock(t *testing.T) {
	var above, left BlockContext
	above.PalSz[3] = 2
	above.Pal[0][3][0] = 11
	above.Pal[0][3][1] = 29

	cache, n := paletteCache(&above, &left, 0, 3, 17)
	if got, want := n, 2; got != want {
		t.Fatalf("cache size = %d, want %d", got, want)
	}
	if got, want := cache[0], uint16(11); got != want {
		t.Fatalf("cache[0] = %d, want %d", got, want)
	}
	if got, want := cache[1], uint16(29); got != want {
		t.Fatalf("cache[1] = %d, want %d", got, want)
	}
}

func TestPaletteCacheSkipsAboveEntryAtTopOfLocalRow(t *testing.T) {
	var above, left BlockContext
	above.PalSz[3] = 1
	above.Pal[0][3][0] = 17

	cache, n := paletteCache(&above, &left, 0, 3, 0)
	if got, want := n, 0; got != want {
		t.Fatalf("cache size = %d, want %d", got, want)
	}
	if got := cache[0]; got != 0 {
		t.Fatalf("cache[0] = %d, want 0", got)
	}
}
