package decoder

import (
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestIntraModeForTxTypeContextFallsBackAtFrameEdge(t *testing.T) {
	if got := intraModeForTxTypeContext(IntraPredPaeth, 0, false, true); got != IntraPredDC {
		t.Fatalf("paeth tx-type mode = %v, want DC fallback", got)
	}
	if got := intraModeForTxTypeContext(IntraPredSmooth, 0, false, true); got != IntraPredSmoothHorizontal {
		t.Fatalf("smooth tx-type mode = %v, want SMOOTH_H fallback", got)
	}
	if got := intraModeForTxTypeContext(IntraPredSmooth, 0, true, false); got != IntraPredSmoothVertical {
		t.Fatalf("smooth tx-type mode = %v, want SMOOTH_V fallback", got)
	}
	if got := intraModeForTxTypeContext(IntraPredPaeth, 0, false, false); got != IntraPredDC {
		t.Fatalf("paeth tx-type mode without edges = %v, want DC fallback", got)
	}
}

func TestReadIntraTxTypeUsesRawModeCDFAtFrameEdge(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.QIndex[0] = 1
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{symbols: []uint32{1}}

	got, err := ReadIntraTxType(hdr, cdf, IntraBlockSyntax{
		Segment: 0,
		YMode:   IntraPredPaeth,
	}, TX4X4, false, true, dec)
	if err != nil {
		t.Fatalf("ReadIntraTxType: %v", err)
	}
	if got != TxTypeDCTDCT {
		t.Fatalf("txType = %d, want DCT_DCT", got)
	}
	if len(dec.cdf0) != 1 {
		t.Fatalf("expected one tx-type symbol read from raw mode row, saw %d", len(dec.cdf0))
	}
	info := TxfmInfoFor(TX4X4)
	wantCDF0 := cdf.TXTypeIntra1[info.Min][IntraPredPaeth][0]
	if dec.cdf0[0] != wantCDF0 {
		t.Fatalf("tx-type CDF[0] = %d, want raw PAETH CDF %d", dec.cdf0[0], wantCDF0)
	}
}

func TestReadIntraTxTypeUsesRawSmoothModeCDF(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.QIndex[0] = 1
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{symbols: []uint32{0}}

	if _, err := ReadIntraTxType(hdr, cdf, IntraBlockSyntax{
		Segment: 0,
		YMode:   IntraPredSmooth,
	}, TX4X4, false, true, dec); err != nil {
		t.Fatalf("ReadIntraTxType: %v", err)
	}
	if len(dec.cdf0) != 1 {
		t.Fatalf("expected one tx-type symbol read, saw %d", len(dec.cdf0))
	}
	info := TxfmInfoFor(TX4X4)
	wantCDF0 := cdf.TXTypeIntra1[info.Min][IntraPredSmooth][0]
	if dec.cdf0[0] != wantCDF0 {
		t.Fatalf("tx-type CDF[0] = %d, want raw SMOOTH CDF %d", dec.cdf0[0], wantCDF0)
	}
}

func TestReadIntraTxTypeUsesRawModeWithoutNeighbors(t *testing.T) {
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.QIndex[0] = 1
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{symbols: []uint32{0}}

	if _, err := ReadIntraTxType(hdr, cdf, IntraBlockSyntax{
		Segment: 0,
		YMode:   IntraPredSmooth,
	}, TX4X4, false, false, dec); err != nil {
		t.Fatalf("ReadIntraTxType: %v", err)
	}
	if len(dec.cdf0) != 1 {
		t.Fatalf("expected one tx-type symbol read, saw %d", len(dec.cdf0))
	}
	info := TxfmInfoFor(TX4X4)
	wantCDF0 := cdf.TXTypeIntra1[info.Min][IntraPredSmooth][0]
	if dec.cdf0[0] != wantCDF0 {
		t.Fatalf("tx-type CDF[0] = %d, want raw SMOOTH CDF %d", dec.cdf0[0], wantCDF0)
	}
}

func TestDebugIntraUVTxTypeOverride(t *testing.T) {
	if _, ok := debugIntraUVTxTypeOverride(); ok {
		t.Fatal("default debugIntraUVTxTypeOverride unexpectedly set")
	}

	t.Setenv("DEBUG_INTRA_UV_TX_TYPE", "3")
	if got, ok := debugIntraUVTxTypeOverride(); !ok || got != TxTypeADSTADST {
		t.Fatalf("debugIntraUVTxTypeOverride = (%d,%v), want (%d,true)", got, ok, TxTypeADSTADST)
	}
}

func TestIntraUVTxTypeDebugOverrideWins(t *testing.T) {
	t.Setenv("DEBUG_INTRA_UV_TX_TYPE", "2")
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.QIndex[0] = 1

	if got := IntraUVTxType(hdr, 0, TX8X8, IntraPredPaeth); got != TxTypeDCTADST {
		t.Fatalf("IntraUVTxType override = %d, want %d", got, TxTypeDCTADST)
	}
}

func TestDebugIntraTxTypeNeighborOverride(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		haveTop  bool
		haveLeft bool
		wantTop  bool
		wantLeft bool
	}{
		{name: "default", env: "", haveTop: false, haveLeft: true, wantTop: false, wantLeft: true},
		{name: "both", env: "both", haveTop: false, haveLeft: false, wantTop: true, wantLeft: true},
		{name: "top", env: "top", haveTop: false, haveLeft: true, wantTop: true, wantLeft: false},
		{name: "left", env: "left", haveTop: true, haveLeft: false, wantTop: false, wantLeft: true},
		{name: "none", env: "none", haveTop: true, haveLeft: true, wantTop: false, wantLeft: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("DEBUG_INTRA_TXTYPE_NEIGHBORS", tc.env)
			}
			gotTop, gotLeft := debugIntraTxTypeNeighborOverride(tc.haveTop, tc.haveLeft)
			if gotTop != tc.wantTop || gotLeft != tc.wantLeft {
				t.Fatalf("debugIntraTxTypeNeighborOverride(%v,%v) = (%v,%v), want (%v,%v)", tc.haveTop, tc.haveLeft, gotTop, gotLeft, tc.wantTop, tc.wantLeft)
			}
		})
	}
}

func TestDebugIntraTxTypeModeOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want IntraPredMode
	}{
		{name: "default", env: "", want: IntraPredSmoothHorizontal},
		{name: "raw", env: "raw", want: IntraPredSmooth},
		{name: "dc", env: "dc", want: IntraPredDC},
		{name: "paeth", env: "paeth", want: IntraPredPaeth},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("DEBUG_INTRA_TXTYPE_MODE", tc.env)
			}
			got := debugIntraTxTypeModeOverride(IntraPredSmoothHorizontal, IntraPredSmooth, 0)
			if got != tc.want {
				t.Fatalf("debugIntraTxTypeModeOverride = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDebugIntraTxTypeModeOverrideNormalizesFilterRawMode(t *testing.T) {
	t.Setenv("DEBUG_INTRA_TXTYPE_MODE", "raw")
	got := debugIntraTxTypeModeOverride(IntraPredDC, IntraPredFilter, 2)
	if got != IntraPredHorizontal {
		t.Fatalf("debugIntraTxTypeModeOverride(filter) = %v, want %v", got, IntraPredHorizontal)
	}
}

func TestReadIntraTxTypeIgnoresDebugNeighborOverrideForModeRow(t *testing.T) {
	t.Setenv("DEBUG_INTRA_TXTYPE_NEIGHBORS", "none")
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.QIndex[0] = 1
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{symbols: []uint32{0}}

	if _, err := ReadIntraTxType(hdr, cdf, IntraBlockSyntax{
		Segment: 0,
		YMode:   IntraPredSmooth,
	}, TX4X4, true, true, dec); err != nil {
		t.Fatalf("ReadIntraTxType: %v", err)
	}
	if len(dec.cdf0) != 1 {
		t.Fatalf("expected one tx-type symbol read, saw %d", len(dec.cdf0))
	}
	info := TxfmInfoFor(TX4X4)
	wantCDF0 := cdf.TXTypeIntra1[info.Min][IntraPredSmooth][0]
	if dec.cdf0[0] != wantCDF0 {
		t.Fatalf("tx-type CDF[0] = %d, want raw SMOOTH CDF %d", dec.cdf0[0], wantCDF0)
	}
}

func TestReadIntraTxTypeDebugModeOverrideUsesRawModeRow(t *testing.T) {
	t.Setenv("DEBUG_INTRA_TXTYPE_MODE", "raw")
	hdr := &obu.FrameHeader{}
	hdr.Segmentation.QIndex[0] = 1
	cdf := NewDefaultModeCDF()
	dec := &scriptedIntraEntropy{symbols: []uint32{0}}

	if _, err := ReadIntraTxType(hdr, cdf, IntraBlockSyntax{
		Segment: 0,
		YMode:   IntraPredSmooth,
	}, TX4X4, false, true, dec); err != nil {
		t.Fatalf("ReadIntraTxType: %v", err)
	}
	if len(dec.cdf0) != 1 {
		t.Fatalf("expected one tx-type symbol read, saw %d", len(dec.cdf0))
	}
	info := TxfmInfoFor(TX4X4)
	wantCDF0 := cdf.TXTypeIntra1[info.Min][IntraPredSmooth][0]
	if dec.cdf0[0] != wantCDF0 {
		t.Fatalf("debug mode tx-type CDF[0] = %d, want raw-mode CDF %d", dec.cdf0[0], wantCDF0)
	}
}
