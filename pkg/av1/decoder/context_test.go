package decoder

import "testing"

func TestBlockContextResetKeyframe(t *testing.T) {
	var ctx BlockContext
	ctx.Reset(true, 0)

	for i := range ctx.Intra {
		if ctx.Intra[i] != 1 {
			t.Fatalf("Intra[%d] = %d, want 1", i, ctx.Intra[i])
		}
		if ctx.UVMode[i] != dcPredMode {
			t.Fatalf("UVMode[%d] = %d, want %d", i, ctx.UVMode[i], dcPredMode)
		}
		if ctx.Mode[i] != dcPredMode {
			t.Fatalf("Mode[%d] = %d, want %d", i, ctx.Mode[i], dcPredMode)
		}
		if ctx.TxIntra[i] != -1 {
			t.Fatalf("TxIntra[%d] = %d, want -1", i, ctx.TxIntra[i])
		}
		if ctx.Tx[i] != txSize64x64 {
			t.Fatalf("Tx[%d] = %d, want %d", i, ctx.Tx[i], txSize64x64)
		}
		if ctx.Filter[0][i] != switchableFilterUnset || ctx.Filter[1][i] != switchableFilterUnset {
			t.Fatalf("Filter[%d] = (%d,%d), want (%d,%d)", i, ctx.Filter[0][i], ctx.Filter[1][i], switchableFilterUnset, switchableFilterUnset)
		}
	}
}

func TestBlockContextResetInter(t *testing.T) {
	var ctx BlockContext
	ctx.Reset(false, 0)

	for i := range ctx.Intra {
		if ctx.Intra[i] != 0 {
			t.Fatalf("Intra[%d] = %d, want 0", i, ctx.Intra[i])
		}
		if ctx.Ref[0][i] != -1 || ctx.Ref[1][i] != -1 {
			t.Fatalf("Ref[%d] = (%d,%d), want (-1,-1)", i, ctx.Ref[0][i], ctx.Ref[1][i])
		}
	}
}

func TestGetPartitionCtx(t *testing.T) {
	var above, left BlockContext
	above.Partition[2] = 1 << (4 - BlockLevel64x64)
	left.Partition[3] = 1 << (4 - BlockLevel64x64)

	got := GetPartitionCtx(&above, &left, BlockLevel64x64, 3, 2)
	if got != 3 {
		t.Fatalf("GetPartitionCtx = %d, want 3", got)
	}
}

func TestPartitionGatherProbabilities(t *testing.T) {
	cdf := NewDefaultModeCDF()
	left := GatherLeftPartitionProb(cdf.PartitionCDF(BlockLevel64x64, 0), BlockLevel64x64)
	top := GatherTopPartitionProb(cdf.PartitionCDF(BlockLevel64x64, 0), BlockLevel64x64)

	if left == 0 {
		t.Fatal("left partition probability = 0, want non-zero")
	}
	if top == 0 {
		t.Fatal("top partition probability = 0, want non-zero")
	}
}

func TestGatherTopPartitionProbUsesSignedDifference(t *testing.T) {
	pcdf := []uint16{26646, 25957, 24132, 5347, 5069, 3869, 3455, 2853, 2423, 3}
	if got := GatherTopPartitionProb(pcdf, BlockLevel64x64); got != 24327 {
		t.Fatalf("GatherTopPartitionProb() = %d, want 24327", got)
	}
}

func TestGetIntraCtx(t *testing.T) {
	var above, left BlockContext
	above.Intra[4] = 1
	left.Intra[5] = 1

	if got := GetIntraCtx(&above, &left, 5, 4, true, true); got != 3 {
		t.Fatalf("GetIntraCtx(top+left) = %d, want 3", got)
	}
	if got := GetIntraCtx(&above, &left, 5, 4, false, true); got != 2 {
		t.Fatalf("GetIntraCtx(left) = %d, want 2", got)
	}
	if got := GetIntraCtx(&above, &left, 5, 4, true, false); got != 2 {
		t.Fatalf("GetIntraCtx(top) = %d, want 2", got)
	}
	if got := GetIntraCtx(&above, &left, 5, 4, false, false); got != 0 {
		t.Fatalf("GetIntraCtx(none) = %d, want 0", got)
	}
}

func TestGetTxCtxUsesGreaterOrEqualNeighborWidths(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	if got := GetTxCtx(&above, &left, TxfmInfoFor(TX8X8), 0, 0); got != 0 {
		t.Fatalf("GetTxCtx(reset) = %d, want 0", got)
	}

	above.TxIntra[0] = 1
	left.TxIntra[0] = 1
	if got := GetTxCtx(&above, &left, TxfmInfoFor(TX8X8), 0, 0); got != 2 {
		t.Fatalf("GetTxCtx(equal) = %d, want 2", got)
	}

	above.TxIntra[0] = 0
	left.TxIntra[0] = 1
	if got := GetTxCtx(&above, &left, TxfmInfoFor(TX8X8), 0, 0); got != 1 {
		t.Fatalf("GetTxCtx(mixed) = %d, want 1", got)
	}
}

func TestGetInterTxCtxUsesSmallerNeighborWidths(t *testing.T) {
	var above, left BlockContext
	above.Reset(true, 0)
	left.Reset(true, 0)

	above.Tx[0] = 0
	left.Tx[0] = 0
	if got := GetInterTxCtx(&above, &left, TxfmInfoFor(TX8X8), 0, 0); got != 2 {
		t.Fatalf("GetInterTxCtx(smaller) = %d, want 2", got)
	}

	above.Tx[0] = 1
	left.Tx[0] = 1
	if got := GetInterTxCtx(&above, &left, TxfmInfoFor(TX8X8), 0, 0); got != 0 {
		t.Fatalf("GetInterTxCtx(equal) = %d, want 0", got)
	}

	above.Tx[0] = 0
	left.Tx[0] = 2
	if got := GetInterTxCtx(&above, &left, TxfmInfoFor(RTX4X16), 0, 0); got != 0 {
		t.Fatalf("GetInterTxCtx(rectangular) = %d, want 0", got)
	}
}
