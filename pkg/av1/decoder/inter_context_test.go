package decoder

import "testing"

func TestGetRefCtxPrefersBackwardWhenNeighborsAreBackward(t *testing.T) {
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Ref[0][4] = 6
	left.Ref[0][5] = 4

	if got := GetRefCtx(&above, &left, 5, 4, true, true); got != 0 {
		t.Fatalf("GetRefCtx() = %d, want 0", got)
	}
}

func TestGetFwdRefContexts(t *testing.T) {
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Ref[0][4] = 0
	left.Ref[0][5] = 3

	if got := GetFwdRefCtx(&above, &left, 5, 4, true, true); got != 1 {
		t.Fatalf("GetFwdRefCtx() = %d, want 1", got)
	}
	if got := GetFwdRef1Ctx(&above, &left, 5, 4, true, true); got != 2 {
		t.Fatalf("GetFwdRef1Ctx() = %d, want 2", got)
	}
	if got := GetFwdRef2Ctx(&above, &left, 5, 4, true, true); got != 0 {
		t.Fatalf("GetFwdRef2Ctx() = %d, want 0", got)
	}
}

func TestGetBwdRefContexts(t *testing.T) {
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	above.Ref[0][4] = 6
	left.Ref[0][5] = 5

	if got := GetBwdRefCtx(&above, &left, 5, 4, true, true); got != 1 {
		t.Fatalf("GetBwdRefCtx() = %d, want 1", got)
	}
	if got := GetBwdRef1Ctx(&above, &left, 5, 4, true, true); got != 0 {
		t.Fatalf("GetBwdRef1Ctx() = %d, want 0", got)
	}
}

func TestGetDRLContext(t *testing.T) {
	if got := GetDRLContext([]int{700, 100}, 0); got != 1 {
		t.Fatalf("GetDRLContext(high,low) = %d, want 1", got)
	}
	if got := GetDRLContext([]int{100, 100}, 0); got != 2 {
		t.Fatalf("GetDRLContext(low,low) = %d, want 2", got)
	}
	if got := GetDRLContext([]int{100, 700}, 0); got != 0 {
		t.Fatalf("GetDRLContext(low,high) = %d, want 0", got)
	}
}

func TestReadSingleRefIndexReadsForwardReference(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // first branch => forward ref
			0, // second branch => refs 0/1
			1, // choose ref 1
		},
	}

	ref, err := ReadSingleRefIndex(cdf, &above, &left, 0, 0, false, false, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefIndex: %v", err)
	}
	if ref != 1 {
		t.Fatalf("ref = %d, want 1", ref)
	}
}

func TestReadSingleRefIndexReadsBackwardReference(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			1, // first branch => backward ref family
			0, // not altref
			1, // choose index 5
		},
	}

	ref, err := ReadSingleRefIndex(cdf, &above, &left, 0, 0, false, false, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefIndex: %v", err)
	}
	if ref != 5 {
		t.Fatalf("ref = %d, want 5", ref)
	}
}
