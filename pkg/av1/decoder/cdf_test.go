package decoder

import "testing"

func TestDefaultModeCDFSeedsExpectedValues(t *testing.T) {
	cdf := NewDefaultModeCDF()

	if got, want := cdf.Skip[0][0], uint16(32768-31671); got != want {
		t.Fatalf("Skip[0][0] = %d, want %d", got, want)
	}
	if got, want := cdf.SkipMode[2][0], uint16(32768-8127); got != want {
		t.Fatalf("SkipMode[2][0] = %d, want %d", got, want)
	}
	if got, want := cdf.Partition[BlockLevel128x128][0][0], uint16(32768-27899); got != want {
		t.Fatalf("Partition[128][0][0] = %d, want %d", got, want)
	}
	if got, want := len(cdf.PartitionCDF(BlockLevel8x8, 0)), 4; got != want {
		t.Fatalf("len(PartitionCDF(8x8)) = %d, want %d", got, want)
	}
	if got, want := cdf.TXSize[0][0][0], uint16(32768-19968); got != want {
		t.Fatalf("TXSize[0][0][0] = %d, want %d", got, want)
	}
	if got, want := cdf.TXPart[0][0][0], uint16(32768-28581); got != want {
		t.Fatalf("TXPart[0][0][0] = %d, want %d", got, want)
	}
	if got, want := cdf.MotionMode[Block64x64][0], uint16(32768-29516); got != want {
		t.Fatalf("MotionMode[64x64][0] = %d, want %d", got, want)
	}
	if got, want := cdf.OBMC[Block64x64][0], uint16(32768-30128); got != want {
		t.Fatalf("OBMC[64x64][0] = %d, want %d", got, want)
	}
}
