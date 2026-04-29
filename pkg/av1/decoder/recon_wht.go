package decoder

import "fmt"

func InverseAddWHTWHT4X48(dst []byte, stride int, coeffs []int32) error {
	if stride < 4 {
		return fmt.Errorf("decoder: inverse wht stride %d < width 4", stride)
	}
	if len(dst) < 3*stride+4 {
		return fmt.Errorf("decoder: inverse wht buffer too small")
	}
	if len(coeffs) < 16 {
		return fmt.Errorf("decoder: inverse wht coefficient buffer too small")
	}

	var tmp [16]int32
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			tmp[y*4+x] = coeffs[y*4+x] >> 2
		}
		invWHT4_1D(tmp[y*4:y*4+4], 1)
	}
	for x := 0; x < 4; x++ {
		invWHT4_1D(tmp[x:], 4)
	}
	for y := 0; y < 4; y++ {
		row := dst[y*stride:]
		for x := 0; x < 4; x++ {
			row[x] = clipByte(int(row[x]) + int(tmp[y*4+x]))
		}
	}
	return nil
}

func invWHT4_1D(c []int32, stride int) {
	in0 := c[0*stride]
	in1 := c[1*stride]
	in2 := c[2*stride]
	in3 := c[3*stride]

	t0 := in0 + in1
	t2 := in2 - in3
	t4 := (t0 - t2) >> 1
	t3 := t4 - in3
	t1 := t4 - in1

	c[0*stride] = t0 - t3
	c[1*stride] = t3
	c[2*stride] = t1
	c[3*stride] = t2 + t1
}
