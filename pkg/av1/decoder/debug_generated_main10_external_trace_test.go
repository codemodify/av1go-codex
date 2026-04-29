package decoder

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func TestDebugGeneratedMain10ExternalTrace(t *testing.T) {
	if os.Getenv("DEBUG_MAIN10_EXTERNAL_TRACE") == "" {
		t.Skip("debug only")
	}
	path := os.Getenv("DEBUG_MAIN10_TRACE_INPUT")
	if path == "" {
		t.Fatal("DEBUG_MAIN10_TRACE_INPUT must point at the MP4 used for the external reference trace")
	}
	maxCalls := debugExternalTraceMaxCalls()

	seq, hdr := debugExternalTraceFirstKeyframe(t, path)
	captured := make(map[int]debugTargetLeafAdvanceState, maxCalls)
	debugDecodeGeneratedMain10RawWithLeafReadersAndCapture(t, path, nil, func(callIdx int, state debugTargetLeafAdvanceState) {
		if callIdx < maxCalls {
			captured[callIdx] = state
		}
	})

	for call := 0; call < maxCalls; call++ {
		state, ok := captured[call]
		if !ok {
			t.Fatalf("missing captured call %d", call)
		}
		debugLogExternalTraceLeaf(t, seq, hdr, call, state)
	}
}

func debugExternalTraceMaxCalls() int {
	const defaultMax = 12
	raw := os.Getenv("DEBUG_MAIN10_TRACE_MAX_CALLS")
	if raw == "" {
		return defaultMax
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultMax
	}
	return n
}

func debugExternalTraceFirstKeyframe(t *testing.T, path string) (av1.SequenceHeader, *obu.FrameHeader) {
	t.Helper()

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	for {
		pf, err := dec.NextParsedFrame()
		if err == io.EOF {
			t.Fatal("no keyframe found")
		}
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if pf.Header.FrameType == obu.FrameTypeKey && !pf.Header.ShowExistingFrame {
			hdr := pf.Header
			return dec.header, &hdr
		}
	}
}

func debugLogExternalTraceLeaf(t *testing.T, seq av1.SequenceHeader, hdr *obu.FrameHeader, call int, state debugTargetLeafAdvanceState) {
	t.Helper()

	g := state.syntax.Geometry
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	topMode := state.preSyntaxAbove.Mode[bx4]
	leftMode := state.preSyntaxLeft.Mode[by4]
	topCtx := generatedIntraModeContext[topMode]
	leftCtx := generatedIntraModeContext[leftMode]
	hasChroma := hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y)
	cflAllowed := hasChroma && cflAllowedForSegment(seq, hdr, g.Size, state.syntax.Segment)

	t.Logf("AV1GO leaf[%02d] geom=(%d,%d)-(%d,%d) size=%s top=%v left=%v chroma=%v cflAllowed=%v qidx=%d readDeltas=%v entryMSA=%v preSyntaxMSA=%v syntaxMSA=%v postMSA=%v",
		call,
		g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4,
		blockName(g.Size), state.haveTop, state.haveLeft, hasChroma, cflAllowed,
		state.preSyntaxQIdx, state.preSyntaxReadDeltas,
		debugGeneratedMain10MSACState(state.entryDec),
		debugGeneratedMain10MSACState(state.preSyntaxDec),
		debugGeneratedMain10MSACState(state.dec),
		debugGeneratedMain10MSACState(state.postDec))
	t.Logf("AV1GO leaf[%02d] ctx bx4=%d by4=%d topMode=%s(%d ctx=%d) leftMode=%s(%d ctx=%d) topSkip=%d leftSkip=%d topPal=%d leftPal=%d local=%s",
		call, bx4, by4,
		intraModeName(IntraPredMode(topMode)), topMode, topCtx,
		intraModeName(IntraPredMode(leftMode)), leftMode, leftCtx,
		state.preSyntaxAbove.Skip[bx4], state.preSyntaxLeft.Skip[by4],
		state.preSyntaxAbove.PalSz[bx4], state.preSyntaxLeft.PalSz[by4],
		debugExternalTraceGeometry(state.entryLocal))
	t.Logf("AV1GO leaf[%02d] syntax skipMode=%v skip=%v seg=%d y=%s yAngle=%d uv=%s uvAngle=%d cfl=%v tx=%s uvtx=%s palY=%v palYIdx16=%v palUV=%v palUVIdx16=%v",
		call,
		state.syntax.SkipMode, state.syntax.Skip, state.syntax.Segment,
		intraModeName(state.syntax.YMode), state.syntax.YAngle,
		intraModeName(state.syntax.UVMode), state.syntax.UVAngle,
		state.syntax.CFLAlpha, txName(state.syntax.TX), txName(state.syntax.UVTX),
		state.syntax.PalY[:state.syntax.PalYSize],
		append([]uint8(nil), state.syntax.PalYIdx[:minInt(len(state.syntax.PalYIdx), 16)]...),
		state.syntax.PalU[:state.syntax.PalUVSize],
		append([]uint8(nil), state.syntax.PalUVIdx[:minInt(len(state.syntax.PalUVIdx), 16)]...))
	for i, frame := range state.traversal {
		t.Logf("AV1GO leaf[%02d] stack[%d] geom=%s local=%s part=%d child=%d/%d children=%s",
			call, i,
			debugExternalTraceGeometry(frame.Geometry),
			debugExternalTraceGeometry(frame.Local),
			frame.Part, frame.ChildIndex, frame.ChildCount,
			debugExternalTraceChildren(frame))
	}
	debugLogExternalTraceCFLReplay(t, seq, hdr, call, state)

	mode := state.preSyntaxMode.Clone()
	above := state.preSyntaxAbove
	left := state.preSyntaxLeft
	dec := state.preSyntaxDec.Clone()
	if mode == nil || dec == nil {
		return
	}
	_, stages, err := debugTraceIntraSyntaxStages(seq, hdr, mode, &above, &left, state.entryLocal, state.syntax.Segment, dec)
	if err != nil {
		t.Logf("AV1GO leaf[%02d] stage replay failed: %v", call, err)
		return
	}
	for _, stage := range stages {
		t.Logf("AV1GO leaf[%02d] stage=%s value=%s msa=%v rng=%d",
			call, stage.name, stage.value, stage.msa, stage.msa[1])
	}
}

func debugLogExternalTraceCFLReplay(t *testing.T, seq av1.SequenceHeader, hdr *obu.FrameHeader, call int, state debugTargetLeafAdvanceState) {
	t.Helper()
	if state.preSyntaxMode == nil || state.preSyntaxDec == nil {
		return
	}
	cdf := state.preSyntaxMode.Clone()
	above := state.preSyntaxAbove
	left := state.preSyntaxLeft
	dec := state.preSyntaxDec.Clone()
	if cdf == nil || dec == nil {
		return
	}
	g := state.entryLocal
	bx4 := g.Start4X & 31
	by4 := g.Start4Y & 31
	syntax := IntraBlockSyntax{
		Geometry: g,
		Size:     g.Size,
		Segment:  state.syntax.Segment,
		YMode:    IntraPredDC,
		UVMode:   IntraPredDC,
		TX:       MaxLumaTXSizeForBlock(g.Size),
		UVTX:     MaxTXSizeForBlock(g.Size, seq.ColorConfig.ChromaSubsampling()),
	}
	syntax.YMode = IntraPredMode(dec.DecodeSymbolAdapt(intraYModeCDF(hdr, cdf, &above, &left, g, by4, bx4), int(numIntraPredModes-1)))
	if needsIntraAngleDelta(syntax.YMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.YMode)
		if err != nil {
			t.Logf("AV1GO leaf[%02d] cfl replay y angle failed: %v", call, err)
			return
		}
		syntax.YAngle = angle
	}
	if !supportsLumaIntraPredictor(syntax.YMode, syntax.YAngle) {
		syntax.YMode = IntraPredDC
		syntax.YAngle = 0
	}
	if !hasChromaPlane(seq, g.Size, g.Start4X, g.Start4Y) {
		return
	}
	cflAllowed := cflAllowedForSegment(seq, hdr, g.Size, syntax.Segment)
	lastSymbol := int(numIntraPredModes - 1)
	if cflAllowed {
		lastSymbol++
	}
	syntax.UVMode = IntraPredMode(dec.DecodeSymbolAdapt(cdf.UVMode[boolToInt(cflAllowed)][syntax.YMode][:], lastSymbol))
	if needsIntraAngleDelta(syntax.UVMode, g.Size) {
		angle, err := readIntraAngleDelta(cdf, dec, syntax.UVMode)
		if err != nil {
			t.Logf("AV1GO leaf[%02d] cfl replay uv angle failed: %v", call, err)
			return
		}
		syntax.UVAngle = angle
	}
	if syntax.UVMode != IntraPredCFL {
		return
	}
	t.Logf("AV1GO leaf[%02d] cfl replay before sign cdfSign=%v cdfAlpha0=%v cdfAlpha1=%v cdfAlpha2=%v cdfAlpha3=%v cdfAlpha4=%v cdfAlpha5=%v",
		call,
		cdf.CFLSign,
		cdf.CFLAlpha[0], cdf.CFLAlpha[1], cdf.CFLAlpha[2],
		cdf.CFLAlpha[3], cdf.CFLAlpha[4], cdf.CFLAlpha[5])
	if err := debugTraceCall4ReadCFLAlpha(t, fmt.Sprintf("AV1GO leaf[%02d]", call), cdf, dec, &syntax); err != nil {
		t.Logf("AV1GO leaf[%02d] cfl replay failed: %v", call, err)
		return
	}
	t.Logf("AV1GO leaf[%02d] cfl replay after sign cdfSign=%v cdfAlpha=%v",
		call, cdf.CFLSign, cdf.CFLAlpha)
}

func debugExternalTraceGeometry(g BlockGeometry) string {
	if g.Width4() == 0 || g.Height4() == 0 {
		return "(empty)"
	}
	return fmt.Sprintf("(%d,%d)-(%d,%d)/%s", g.Start4X*4, g.Start4Y*4, g.End4X*4, g.End4Y*4, blockName(g.Size))
}

func debugExternalTraceChildren(frame debugTraversalFrame) string {
	if frame.ChildCount == 0 {
		return "[]"
	}
	out := "["
	for i := 0; i < frame.ChildCount; i++ {
		if i > 0 {
			out += " "
		}
		out += debugExternalTraceGeometry(frame.Children[i])
	}
	out += "]"
	return out
}
