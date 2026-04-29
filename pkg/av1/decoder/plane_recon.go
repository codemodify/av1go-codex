package decoder

import "fmt"

func ReconstructDCLumaBlock8(plane []byte, stride, planeWidth, planeHeight, x, y int, tx TxfmSize, coeff int32) error {
	info := TxfmInfoFor(tx)
	width := int(info.W4) * 4
	height := int(info.H4) * 4
	if x < 0 || y < 0 || x+width > planeWidth || y+height > planeHeight {
		return fmt.Errorf("decoder: block (%d,%d %dx%d) out of plane bounds %dx%d", x, y, width, height, planeWidth, planeHeight)
	}
	if stride < planeWidth {
		return fmt.Errorf("decoder: plane stride %d < width %d", stride, planeWidth)
	}
	if len(plane) < (planeHeight-1)*stride+planeWidth {
		return fmt.Errorf("decoder: plane buffer too small")
	}

	dst := plane[y*stride+x:]
	var top []byte
	var left []byte
	haveTop := y > 0
	haveLeft := x > 0
	if haveTop {
		top = plane[(y-1)*stride+x:]
	}
	if haveLeft {
		left = make([]byte, height)
		for i := 0; i < height; i++ {
			left[i] = plane[(y+i)*stride+x-1]
		}
	}
	if err := PredictDCLuma8(dst, stride, width, height, top, left, haveTop, haveLeft); err != nil {
		return err
	}
	return InverseAddDCTDCTDC8(dst, stride, tx, coeff)
}
