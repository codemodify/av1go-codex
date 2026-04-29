package decoder

import "fmt"

var generatedFilterIntraTaps = [5][8][7]int8{
	{
		{-6, 10, 0, 0, 0, 12, 0},
		{-5, 2, 10, 0, 0, 9, 0},
		{-3, 1, 1, 10, 0, 7, 0},
		{-3, 1, 1, 2, 10, 5, 0},
		{-4, 6, 0, 0, 0, 2, 12},
		{-3, 2, 6, 0, 0, 2, 9},
		{-3, 2, 2, 6, 0, 2, 7},
		{-3, 1, 2, 2, 6, 3, 5},
	},
	{
		{-10, 16, 0, 0, 0, 10, 0},
		{-6, 0, 16, 0, 0, 6, 0},
		{-4, 0, 0, 16, 0, 4, 0},
		{-2, 0, 0, 0, 16, 2, 0},
		{-10, 16, 0, 0, 0, 0, 10},
		{-6, 0, 16, 0, 0, 0, 6},
		{-4, 0, 0, 16, 0, 0, 4},
		{-2, 0, 0, 0, 16, 0, 2},
	},
	{
		{-8, 8, 0, 0, 0, 16, 0},
		{-8, 0, 8, 0, 0, 16, 0},
		{-8, 0, 0, 8, 0, 16, 0},
		{-8, 0, 0, 0, 8, 16, 0},
		{-4, 4, 0, 0, 0, 0, 16},
		{-4, 0, 4, 0, 0, 0, 16},
		{-4, 0, 0, 4, 0, 0, 16},
		{-4, 0, 0, 0, 4, 0, 16},
	},
	{
		{-2, 8, 0, 0, 0, 10, 0},
		{-1, 3, 8, 0, 0, 6, 0},
		{-1, 2, 3, 8, 0, 4, 0},
		{0, 1, 2, 3, 8, 2, 0},
		{-1, 4, 0, 0, 0, 3, 10},
		{-1, 3, 4, 0, 0, 4, 6},
		{-1, 2, 3, 4, 0, 4, 4},
		{-1, 2, 2, 3, 4, 3, 3},
	},
	{
		{-12, 14, 0, 0, 0, 14, 0},
		{-10, 0, 14, 0, 0, 12, 0},
		{-9, 0, 0, 14, 0, 11, 0},
		{-8, 0, 0, 0, 14, 10, 0},
		{-10, 12, 0, 0, 0, 0, 14},
		{-9, 1, 12, 0, 0, 0, 12},
		{-8, 0, 0, 12, 0, 1, 11},
		{-7, 0, 0, 1, 12, 1, 9},
	},
}

func PredictFilterIntra8(dst []byte, stride, width, height int, filterIdx int8, top, left []byte, topLeft uint8, haveTop, haveLeft bool) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("decoder: invalid filter intra dimensions %dx%d", width, height)
	}
	if stride < width {
		return fmt.Errorf("decoder: filter intra stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: filter intra buffer too small")
	}
	if filterIdx < 0 || int(filterIdx) >= len(generatedFilterIntraTaps) {
		return fmt.Errorf("decoder: invalid filter intra index %d", filterIdx)
	}
	edgeScratch, topExt, _, leftExt, _ := extendIntraEdges(width, height, top, left, topLeft, haveTop, haveLeft)
	defer putZeroScratch(&intraEdgeScratchPool, edgeScratch)
	taps := generatedFilterIntraTaps[filterIdx]
	for y := 0; y < height; y += 2 {
		rows := minInt(2, height-y)
		for x := 0; x < width; x += 4 {
			cols := minInt(4, width-x)
			p0 := filterTopLeft8(dst, stride, x, y, topExt, leftExt, topLeft)
			p1 := filterTopSample8(dst, stride, x+0, y, width, topExt)
			p2 := filterTopSample8(dst, stride, x+1, y, width, topExt)
			p3 := filterTopSample8(dst, stride, x+2, y, width, topExt)
			p4 := filterTopSample8(dst, stride, x+3, y, width, topExt)
			p5 := filterLeftSample8(dst, stride, x, y, leftExt)
			p6 := filterLeftSample8(dst, stride, x, y+1, leftExt)
			for yy := 0; yy < rows; yy++ {
				row := dst[(y+yy)*stride:]
				for xx := 0; xx < cols; xx++ {
					f := taps[yy*4+xx]
					acc := int(f[0])*p0 + int(f[1])*p1 + int(f[2])*p2 + int(f[3])*p3 + int(f[4])*p4 + int(f[5])*p5 + int(f[6])*p6
					row[x+xx] = clipByte((acc + 8) >> 4)
				}
			}
		}
	}
	return nil
}

func PredictFilterIntra16(dst []uint16, stride, width, height int, filterIdx int8, top, left []uint16, topLeft uint16, haveTop, haveLeft bool, bitDepth uint8) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("decoder: invalid filter intra dimensions %dx%d", width, height)
	}
	if stride < width {
		return fmt.Errorf("decoder: filter intra stride %d < width %d", stride, width)
	}
	if len(dst) < (height-1)*stride+width {
		return fmt.Errorf("decoder: filter intra buffer too small")
	}
	if filterIdx < 0 || int(filterIdx) >= len(generatedFilterIntraTaps) {
		return fmt.Errorf("decoder: invalid filter intra index %d", filterIdx)
	}
	topExt, _, leftExt, _ := extendIntraEdges16(width, height, top, left, topLeft, haveTop, haveLeft, bitDepth)
	taps := generatedFilterIntraTaps[filterIdx]
	for y := 0; y < height; y += 2 {
		rows := minInt(2, height-y)
		for x := 0; x < width; x += 4 {
			cols := minInt(4, width-x)
			p0 := filterTopLeft16(dst, stride, x, y, topExt, leftExt, topLeft)
			p1 := filterTopSample16(dst, stride, x+0, y, width, topExt)
			p2 := filterTopSample16(dst, stride, x+1, y, width, topExt)
			p3 := filterTopSample16(dst, stride, x+2, y, width, topExt)
			p4 := filterTopSample16(dst, stride, x+3, y, width, topExt)
			p5 := filterLeftSample16(dst, stride, x, y, leftExt)
			p6 := filterLeftSample16(dst, stride, x, y+1, leftExt)
			for yy := 0; yy < rows; yy++ {
				row := dst[(y+yy)*stride:]
				for xx := 0; xx < cols; xx++ {
					f := taps[yy*4+xx]
					acc := int(f[0])*p0 + int(f[1])*p1 + int(f[2])*p2 + int(f[3])*p3 + int(f[4])*p4 + int(f[5])*p5 + int(f[6])*p6
					row[x+xx] = clipSample((acc+8)>>4, bitDepth)
				}
			}
		}
	}
	return nil
}

func filterTopLeft8(dst []byte, stride, x, y int, top, left []byte, topLeft uint8) int {
	switch {
	case x == 0 && y == 0:
		return int(topLeft)
	case x == 0:
		return int(left[minInt(y-1, len(left)-1)])
	case y == 0:
		return int(top[minInt(x-1, len(top)-1)])
	default:
		return int(dst[(y-1)*stride+x-1])
	}
}

func filterTopSample8(dst []byte, stride, x, y, width int, top []byte) int {
	x = minInt(x, width-1)
	if y == 0 {
		return int(top[minInt(x, len(top)-1)])
	}
	return int(dst[(y-1)*stride+x])
}

func filterLeftSample8(dst []byte, stride, x, y int, left []byte) int {
	if x == 0 {
		return int(left[minInt(y, len(left)-1)])
	}
	return int(dst[y*stride+x-1])
}

func filterTopLeft16(dst []uint16, stride, x, y int, top, left []uint16, topLeft uint16) int {
	switch {
	case x == 0 && y == 0:
		return int(topLeft)
	case x == 0:
		return int(left[minInt(y-1, len(left)-1)])
	case y == 0:
		return int(top[minInt(x-1, len(top)-1)])
	default:
		return int(dst[(y-1)*stride+x-1])
	}
}

func filterTopSample16(dst []uint16, stride, x, y, width int, top []uint16) int {
	x = minInt(x, width-1)
	if y == 0 {
		return int(top[minInt(x, len(top)-1)])
	}
	return int(dst[(y-1)*stride+x])
}

func filterLeftSample16(dst []uint16, stride, x, y int, left []uint16) int {
	if x == 0 {
		return int(left[minInt(y, len(left)-1)])
	}
	return int(dst[y*stride+x-1])
}
