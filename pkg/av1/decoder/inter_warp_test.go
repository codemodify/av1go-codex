package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestFitAffineWarpTranslation(t *testing.T) {
	warp, ok := fitAffineWarp([]affineSample{
		{x: 0, y: 0, u: 2, v: 3},
		{x: 10, y: 0, u: 12, v: 3},
		{x: 0, y: 10, u: 2, v: 13},
		{x: 10, y: 10, u: 12, v: 13},
	})
	if !ok {
		t.Fatal("fitAffineWarp returned !ok")
	}
	if diff := absFloat(warp.a00 - 1); diff > 1e-6 {
		t.Fatalf("a00 diff=%f", diff)
	}
	if diff := absFloat(warp.a11 - 1); diff > 1e-6 {
		t.Fatalf("a11 diff=%f", diff)
	}
	if diff := absFloat(warp.a02 - 2); diff > 1e-6 {
		t.Fatalf("a02 diff=%f", diff)
	}
	if diff := absFloat(warp.a12 - 3); diff > 1e-6 {
		t.Fatalf("a12 diff=%f", diff)
	}
}

func TestPredictInterLumaBlock8WarpTranslation(t *testing.T) {
	ref := make([]byte, 8*8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			ref[y*8+x] = uint8(y*8 + x)
		}
	}
	dst := make([]byte, 4*4)
	warp := translationWarp(MotionVector{X: 8, Y: 0})
	if err := PredictInterLumaBlock8Warp(dst, 4, 4, 4, 0, 0, 4, 4, ref, 8, 8, 8, warp); err != nil {
		t.Fatalf("PredictInterLumaBlock8Warp: %v", err)
	}
	if got, want := dst[0], ref[1]; got != want {
		t.Fatalf("dst[0]=%d want %d", got, want)
	}
	if got, want := dst[3], ref[4]; got != want {
		t.Fatalf("dst[3]=%d want %d", got, want)
	}
}

func TestPredictInterLumaBlock8WarpMatchesRegularFilterForFractionalTranslation(t *testing.T) {
	ref := make([]byte, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			ref[y*16+x] = uint8((x*7 + y*11) & 0xff)
		}
	}
	got := make([]byte, 8*8)
	want := make([]byte, 8*8)
	warp := translationWarp(MotionVector{X: 4, Y: 8})
	if err := PredictInterLumaBlock8Warp(got, 8, 8, 8, 0, 0, 8, 8, ref, 16, 16, 16, warp); err != nil {
		t.Fatalf("PredictInterLumaBlock8Warp: %v", err)
	}
	if err := PredictInterLumaBlock8WithFilters(want, 8, 8, 8, 0, 0, 8, 8, ref, 16, 16, 16, MotionVector{X: 4, Y: 8}, defaultInterFilterPair()); err != nil {
		t.Fatalf("PredictInterLumaBlock8WithFilters: %v", err)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestSingleRefGlobalMotionForPlaneTranslation(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.GMV[0] = obu.WarpedMotionParams{
		Type: 1,
		Matrix: [6]int32{
			1 << 16, 2 << 16,
			1 << 16, 0,
			0, 1 << 16,
		},
	}
	g := BlockGeometry{
		Start4X: 4,
		Start4Y: 8,
		End4X:   8,
		End4Y:   12,
	}
	mv, warp, ok := singleRefGlobalMotionForPlane(hdr, 0, 0, av1.Chroma420, g)
	if !ok {
		t.Fatal("singleRefGlobalMotionForPlane returned !ok")
	}
	if mv.X != 16 || mv.Y != 8 {
		t.Fatalf("mv=%+v want {16,8}", mv)
	}
	x, y := warp.mapPoint(0, 0)
	if x != 1 || y != 2 {
		t.Fatalf("warp translation=(%v,%v) want (1,2)", x, y)
	}
}

func TestGlobalMotionUsesWarpRejectsTranslation(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.GMV[0] = obu.WarpedMotionParams{Type: 1}
	if globalMotionUsesWarp(hdr, 0) {
		t.Fatal("translation global motion must use regular motion compensation, not affine warp")
	}
	hdr.GMV[0] = obu.WarpedMotionParams{Type: 2}
	if !globalMotionUsesWarp(hdr, 0) {
		t.Fatal("rotzoom global motion should be warp-eligible")
	}
	hdr.ForceIntegerMV = true
	if globalMotionUsesWarp(hdr, 0) {
		t.Fatal("force-integer-MV global motion must not use affine warp")
	}
}

func TestGlobalMotionRotZoomUsesFixedWarpMatrix(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.GMV[0] = obu.WarpedMotionParams{
		Type: 2,
		Matrix: [6]int32{
			64 << 10, -32 << 10,
			1<<16 + 128, 64,
			-64, 1<<16 + 128,
		},
	}
	warp, ok := globalMotionWarpForPlane(hdr, 0, 0, av1.Chroma420)
	if !ok {
		t.Fatal("globalMotionWarpForPlane returned !ok")
	}
	if !warp.fixed {
		t.Fatal("rotzoom global motion must use fixed-point warped-motion filtering")
	}
	if warp.matrix != hdr.GMV[0].Matrix {
		t.Fatalf("matrix=%v want %v", warp.matrix, hdr.GMV[0].Matrix)
	}
}

func TestGlobalMotionFixedWarpScalesChromaMatrix(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.GMV[0] = obu.WarpedMotionParams{
		Type: 3,
		Matrix: [6]int32{
			64 << 10, -32 << 10,
			1<<16 + 128, 96,
			-64, 1<<16 + 256,
		},
	}
	warp, ok := globalMotionWarpForPlane(hdr, 0, 1, av1.Chroma420)
	if !ok {
		t.Fatal("globalMotionWarpForPlane returned !ok")
	}
	if !warp.fixed {
		t.Fatal("affine global chroma motion must use fixed-point warped-motion filtering")
	}
	want := hdr.GMV[0].Matrix
	want[0] >>= 1
	want[1] >>= 1
	if warp.matrix != want {
		t.Fatalf("matrix=%v want %v", warp.matrix, want)
	}
}

func TestCollectLocalWarpPointsOrdersTopRightAfterLeftAndTopLeft(t *testing.T) {
	tile := NewSpatialMVTile(TileGeometry{Start4X: 0, Start4Y: 0, End4X: 8, End4Y: 8})
	target := BlockGeometry{Start4X: 2, Start4Y: 2, End4X: 4, End4Y: 4, Size: Block8x8}
	tile.SplatIntra(BlockGeometry{Start4X: 2, Start4Y: 0, End4X: 4, End4Y: 2, Size: Block8x8})
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 2, End4X: 2, End4Y: 4, Size: Block8x8}, 0, 0, MotionVector{X: 32, Y: 3}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 0, Start4Y: 0, End4X: 2, End4Y: 2, Size: Block8x8}, 0, 0, MotionVector{X: 24, Y: 5}, InterPredNearest, defaultInterFilterPair())
	tile.SplatSingleRef(BlockGeometry{Start4X: 4, Start4Y: 0, End4X: 6, End4Y: 2, Size: Block8x8}, 0, 0, MotionVector{X: 1, Y: 6}, InterPredNearest, defaultInterFilterPair())

	var buf [8]localWarpPoint
	points := collectLocalWarpPointsInto(buf[:0], tile, target, 0, fullIntraEdgeAvailability)
	if len(points) != 3 {
		t.Fatalf("len(points)=%d want 3", len(points))
	}
	want := []MotionVector{{X: 32, Y: 3}, {X: 24, Y: 5}, {X: 1, Y: 6}}
	for i, p := range points {
		got := MotionVector{X: p.outX - p.inX, Y: p.outY - p.inY}
		if got != want[i] {
			t.Fatalf("point %d mv=%+v want %+v", i, got, want[i])
		}
	}
}

func TestPredictInterPlane8WarpAtMatchesRegularFilterAtOffset(t *testing.T) {
	ref := make([]byte, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			ref[y*16+x] = uint8((x*9 + y*5) & 0xff)
		}
	}
	got := make([]byte, 4*4)
	wantFull := make([]byte, 16*16)
	warp := translationWarp(MotionVector{X: 4, Y: 8})
	if err := predictInterPlane8WarpAt(got, 4, 4, 4, 0, 0, 4, 4, 6, 7, ref, 16, 16, 16, warp); err != nil {
		t.Fatalf("predictInterPlane8WarpAt: %v", err)
	}
	if err := PredictInterLumaBlock8WithFilters(wantFull, 16, 16, 16, 6, 7, 4, 4, ref, 16, 16, 16, MotionVector{X: 4, Y: 8}, defaultInterFilterPair()); err != nil {
		t.Fatalf("PredictInterLumaBlock8WithFilters: %v", err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			gotV := got[y*4+x]
			wantV := wantFull[(7+y)*16+6+x]
			if gotV != wantV {
				t.Fatalf("sample (%d,%d)=%d want %d", x, y, gotV, wantV)
			}
		}
	}
}

func TestPredictInterPlane8FixedWarpIdentityCopiesLinearInterior(t *testing.T) {
	ref := make([]byte, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			ref[y*16+x] = uint8(x + 3*y)
		}
	}
	got := make([]byte, 8*8)
	warp := affineWarp{
		matrix: [6]int32{0, 0, 1 << 16, 0, 0, 1 << 16},
		fixed:  true,
	}
	if err := predictInterPlane8WarpAt(got, 8, 8, 8, 0, 0, 8, 8, 4, 4, ref, 16, 16, 16, warp); err != nil {
		t.Fatalf("predictInterPlane8WarpAt: %v", err)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if got[y*8+x] != ref[(4+y)*16+4+x] {
				t.Fatalf("sample (%d,%d)=%d want %d", x, y, got[y*8+x], ref[(4+y)*16+4+x])
			}
		}
	}
}

func TestPredictInterPlane8FixedWarpPreservesConstantAtEdges(t *testing.T) {
	ref := make([]byte, 8*8)
	for i := range ref {
		ref[i] = 137
	}
	got := make([]byte, 8*8)
	warp := affineWarp{
		matrix: [6]int32{0, 0, 1 << 16, 0, 0, 1 << 16},
		fixed:  true,
	}
	if err := PredictInterPlane8Warp(got, 8, 8, 8, 0, 0, 8, 8, ref, 8, 8, 8, warp); err != nil {
		t.Fatalf("PredictInterPlane8Warp: %v", err)
	}
	for i, v := range got {
		if v != 137 {
			t.Fatalf("sample %d=%d want 137", i, v)
		}
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
