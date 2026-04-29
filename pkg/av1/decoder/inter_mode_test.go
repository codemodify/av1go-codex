package decoder

import "testing"

func TestReadSingleRefInterModeReadsGlobalMV(t *testing.T) {
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			1, // non-NEWMV branch
			0, // GLOBALMV branch
		},
	}

	mode, drl, err := ReadSingleRefInterMode(cdf, 0, 1, nil, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefInterMode: %v", err)
	}
	if mode != InterPredGlobal {
		t.Fatalf("mode = %d, want GLOBALMV", mode)
	}
	if drl != 0 {
		t.Fatalf("drl = %d, want 0", drl)
	}
}

func TestReadSingleRefInterModeReadsNearMVWithDRL(t *testing.T) {
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			1, // non-NEWMV branch
			1, // not GLOBALMV
			1, // NEARMV
			1, // DRL bit 2 => NEAR
			1, // DRL bit 3 => NEARISH
		},
	}

	mode, drl, err := ReadSingleRefInterMode(cdf, 0, 4, []int{700, 100, 100, 0}, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefInterMode: %v", err)
	}
	if mode != InterPredNear {
		t.Fatalf("mode = %d, want NEARMV", mode)
	}
	if drl != drlNearish {
		t.Fatalf("drl = %d, want %d", drl, drlNearish)
	}
}

func TestReadSingleRefInterModeReadsNewMVWithDRL(t *testing.T) {
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // NEWMV branch
			1, // DRL bit 1 => NEARER
			1, // DRL bit 2 => NEAR
		},
	}

	mode, drl, err := ReadSingleRefInterMode(cdf, 0, 3, []int{700, 100, 0}, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefInterMode: %v", err)
	}
	if mode != InterPredNew {
		t.Fatalf("mode = %d, want NEWMV", mode)
	}
	if drl != drlNear {
		t.Fatalf("drl = %d, want %d", drl, drlNear)
	}
}

func TestReadSingleRefInterSyntaxReadsRefAndMode(t *testing.T) {
	cdf := NewDefaultModeCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // ref family => forward
			0, // fwd family => ref 0/1
			1, // choose ref 1
			1, // non-NEWMV branch
			0, // GLOBALMV
		},
	}

	syntax, err := ReadSingleRefInterSyntax(cdf, &above, &left, 0, 0, false, false, 0, 1, nil, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefInterSyntax: %v", err)
	}
	if syntax.Ref0 != 1 || syntax.Ref1 != -1 {
		t.Fatalf("refs = (%d,%d), want (1,-1)", syntax.Ref0, syntax.Ref1)
	}
	if syntax.Mode != InterPredGlobal {
		t.Fatalf("mode = %d, want GLOBALMV", syntax.Mode)
	}
	if syntax.DRLIndex != 0 {
		t.Fatalf("drl = %d, want 0", syntax.DRLIndex)
	}
}

func TestReadSingleRefInterPayloadReadsNewMVResidual(t *testing.T) {
	modeCDF := NewDefaultModeCDF()
	mvCDF := NewDefaultMVCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // ref family => forward
			0, // fwd family => refs 0/1
			0, // choose ref 0
			0, // NEWMV branch
			0, // y sign
			0, // y class0
			1, // x sign
			0, // x class0
		},
		symbols: []uint32{
			uint32(MVJointHV), // joint
			0,                // y class
			0,                // x class
		},
	}

	syntax, mv, err := ReadSingleRefInterPayload(modeCDF, mvCDF, &above, &left, 0, 0, false, false, 0, 1, nil, MotionVector{Y: 4, X: 9}, -1, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefInterPayload: %v", err)
	}
	if syntax.Ref0 != 0 || syntax.Mode != InterPredNew {
		t.Fatalf("syntax = %+v, want ref0=0 mode=NEWMV", syntax)
	}
	if mv.Y != 12 || mv.X != 1 {
		t.Fatalf("mv = {%d %d}, want {12 1}", mv.Y, mv.X)
	}
}

func TestReadSingleRefInterPayloadKeepsReferenceMVForNearest(t *testing.T) {
	modeCDF := NewDefaultModeCDF()
	mvCDF := NewDefaultMVCDF()
	var above, left BlockContext
	above.Reset(false, 0)
	left.Reset(false, 0)
	dec := &scriptedIntraEntropy{
		adapt: []uint32{
			0, // ref family => forward
			0, // fwd family => refs 0/1
			1, // choose ref 1
			1, // non-NEWMV branch
			1, // not GLOBALMV
			0, // not NEARMV => NEARESTMV
		},
	}

	syntax, mv, err := ReadSingleRefInterPayload(modeCDF, mvCDF, &above, &left, 0, 0, false, false, 0, 1, nil, MotionVector{Y: 7, X: -3}, -1, dec)
	if err != nil {
		t.Fatalf("ReadSingleRefInterPayload: %v", err)
	}
	if syntax.Ref0 != 1 || syntax.Mode != InterPredNearest {
		t.Fatalf("syntax = %+v, want ref0=1 mode=NEARESTMV", syntax)
	}
	if mv.Y != 7 || mv.X != -3 {
		t.Fatalf("mv = {%d %d}, want {7 -3}", mv.Y, mv.X)
	}
}
