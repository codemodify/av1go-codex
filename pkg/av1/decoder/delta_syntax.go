package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

var (
	generatedDeltaQCDF  = [4]uint16{4608, 648, 91, 0}
	generatedDeltaLFCDF = [5][4]uint16{
		{4608, 648, 91, 0},
		{4608, 648, 91, 0},
		{4608, 648, 91, 0},
		{4608, 648, 91, 0},
		{4608, 648, 91, 0},
	}
)

func decodeDeltaMagnitude(cdf []uint16, dec intraEntropyDecoder) int {
	if dec == nil {
		return 0
	}
	v := int(dec.DecodeSymbolAdapt(cdf, 3))
	if v != 3 {
		return v
	}
	if os.Getenv("DEBUG_DELTA_Q_LEGACY_ESCAPE") != "" {
		return int(dec.DecodeBools(3)) + 1
	}
	remBits := int(dec.DecodeBools(3)) + 1
	absBits := int(dec.DecodeBools(uint(remBits)))
	return absBits + (1 << remBits) + 1
}

func applySignedDelta(mag int, resLog2 uint8, dec intraEntropyDecoder) int {
	if mag == 0 {
		return 0
	}
	if dec != nil && dec.DecodeBoolEqui() != 0 {
		mag = -mag
	}
	return mag << resLog2
}

func clipQIndexRuntime(v int) int {
	if v < 1 {
		return 1
	}
	if v > 255 {
		return 255
	}
	return v
}

func clipLFDelta(v int) int8 {
	if v < -63 {
		return -63
	}
	if v > 63 {
		return 63
	}
	return int8(v)
}

func deltaLFCount(seq av1.SequenceHeader, hdr *obu.FrameHeader) int {
	if hdr == nil || !hdr.Delta.LFMulti {
		return 1
	}
	if seq.ColorConfig.Monochrome {
		return 2
	}
	return 4
}

type deltaQUpdateBehavior struct {
	applyLastQIdx bool
	applyDequant  bool
	applyCoefQCat bool
}

func debugDeltaQUpdateBehavior() deltaQUpdateBehavior {
	return deltaQUpdateBehavior{
		applyLastQIdx: os.Getenv("DEBUG_DELTA_Q_CONSUME_ONLY") == "" && os.Getenv("DEBUG_DELTA_Q_SKIP_LAST_Q_UPDATE") == "",
		applyDequant:  os.Getenv("DEBUG_DELTA_Q_SKIP_DEQUANT_UPDATE") == "",
		applyCoefQCat: os.Getenv("DEBUG_DELTA_Q_APPLY_COEF_Q_UPDATE") != "" && os.Getenv("DEBUG_DELTA_Q_SKIP_COEF_Q_UPDATE") == "",
	}
}

func debugDeltaQTargetQIndex(qidx int) int {
	if override := strings.TrimSpace(os.Getenv("DEBUG_DELTA_Q_FORCE_QINDEX")); override != "" {
		if v, err := strconv.Atoi(override); err == nil {
			return clipQIndexRuntime(v)
		}
	}
	return qidx
}

func applyDeltaQState(seq av1.SequenceHeader, hdr *obu.FrameHeader, ts *TileSyntaxState, nextQIdx int, behavior deltaQUpdateBehavior) {
	if hdr == nil || ts == nil {
		return
	}
	if behavior.applyLastQIdx {
		ts.LastQIdx = clipQIndexRuntime(nextQIdx)
	}
	if behavior.applyDequant {
		ts.CurrentDQ = InitDequantTables(seq, hdr, ts.LastQIdx)
	}
	if behavior.applyCoefQCat {
		ts.Tile.SetActiveCoefQIndex(ts.LastQIdx)
	}
}

func readDeltaQAndLF(seq av1.SequenceHeader, hdr *obu.FrameHeader, ts *TileSyntaxState, g BlockGeometry, skip bool) error {
	if hdr == nil || ts == nil || ts.Tile.Entropy == nil || ts.Tile.CDF == nil {
		return nil
	}
	if !ts.ReadDeltas {
		return nil
	}
	if !hdr.Delta.QPresent {
		return nil
	}
	if skip && g.Size == RootBlockSize(seq) {
		ts.ReadDeltas = false
		return nil
	}
	ts.ReadDeltas = false

	lastQBefore := ts.LastQIdx
	deltaMag := decodeDeltaMagnitude(ts.Tile.CDF.DeltaQ[:], ts.Tile.Entropy)
	deltaQ := applySignedDelta(deltaMag, hdr.Delta.QResLog2, ts.Tile.Entropy)
	targetQIdx := debugDeltaQTargetQIndex(clipQIndexRuntime(ts.LastQIdx + deltaQ))
	behavior := debugDeltaQUpdateBehavior()
	applyDeltaQState(seq, hdr, ts, targetQIdx, behavior)

	if hdr.Delta.LFPresent {
		nLF := deltaLFCount(seq, hdr)
		base := boolToInt(hdr.Delta.LFMulti)
		for i := 0; i < nLF; i++ {
			idx := i + base
			if idx < 0 || idx >= len(ts.Tile.CDF.DeltaLF) {
				return fmt.Errorf("decoder: delta lf index %d out of range", idx)
			}
			deltaLF := applySignedDelta(decodeDeltaMagnitude(ts.Tile.CDF.DeltaLF[idx][:], ts.Tile.Entropy), hdr.Delta.LFResLog2, ts.Tile.Entropy)
			ts.LastLF[i] = clipLFDelta(int(ts.LastLF[i]) + deltaLF)
		}
	}

	if os.Getenv("DEBUG_DELTA_Q_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "trace delta-q block=(%d,%d) size=%d skip=%v last=%d mag=%d res=%d target=%d lastq=%d apply_last=%v apply_dq=%v apply_coef=%v\n",
			g.Start4X*4, g.Start4Y*4, g.Size, skip, lastQBefore, deltaMag, deltaQ, targetQIdx, ts.LastQIdx,
			behavior.applyLastQIdx, behavior.applyDequant, behavior.applyCoefQCat,
		)
	}
	return nil
}

func readDeltaQAndLFAtSuperBlockStart(seq av1.SequenceHeader, hdr *obu.FrameHeader, ts *TileSyntaxState, g BlockGeometry, skip bool) error {
	return readDeltaQAndLF(seq, hdr, ts, g, skip)
}
