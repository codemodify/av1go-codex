package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/internal/msac"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type keyIntraBlockEntropyDecoder interface {
	intraEntropyDecoder
	coefficientEntropyDecoder
}

type decodedLumaTU8 struct {
	offX4  int
	offY4  int
	tx     TxfmSize
	txType TxType
	coeffs []int32
	skip   bool
}

func DecodeAndReconstructKeyIntraDCLumaBlock8(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []byte, stride, planeWidth, planeHeight int, dec keyIntraBlockEntropyDecoder) (IntraBlockSyntax, error) {
	return DecodeAndReconstructKeyIntraDCLumaBlockAt8(seq, hdr, modeCDF, coefCDF, dq, above, left, g, segID, plane, stride, planeWidth, planeHeight, g.Start4X, g.Start4Y, dec)
}

func DecodeAndReconstructKeyIntraDCLumaBlockAt8(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder) (IntraBlockSyntax, error) {
	syntax, _, err := decodeAndReconstructKeyIntraDCLumaBlockAt8EdgesWithNeighbors(seq, hdr, modeCDF, coefCDF, dq, above, left, g, segID, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, g.Start4Y > 0, g.Start4X > 0, true, true)
	return syntax, err
}

func DecodeAndReconstructKeyIntraDCLumaBlockAt8Edges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder, topHasRight, leftHasBottom bool) (IntraBlockSyntax, error) {
	syntax, _, err := decodeAndReconstructKeyIntraDCLumaBlockAt8EdgesWithNeighbors(seq, hdr, modeCDF, coefCDF, dq, above, left, g, segID, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, g.Start4Y > 0, g.Start4X > 0, topHasRight, leftHasBottom)
	return syntax, err
}

func decodeAndReconstructKeyIntraDCLumaBlockAt8EdgesWithNeighbors(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, g BlockGeometry, segID uint8, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) (IntraBlockSyntax, bool, error) {
	if hdr == nil {
		return IntraBlockSyntax{}, false, fmt.Errorf("decoder: frame header required")
	}
	if dec == nil {
		return IntraBlockSyntax{}, false, fmt.Errorf("decoder: entropy decoder required")
	}

	syntax, err := ReadIntraBlockSyntax(seq, hdr, modeCDF, above, left, g, haveTop, haveLeft, segID, dec)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	hasResidual, err := reconstructParsedIntraLumaBlockAt8EdgesWithResidual(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return IntraBlockSyntax{}, false, err
	}
	syntax.PreserveTXContext = !syntax.Skip
	if err := ApplyIntraBlockContext(seq, hdr, above, left, syntax); err != nil {
		return IntraBlockSyntax{}, false, err
	}
	return syntax, hasResidual, nil
}

func ReconstructParsedIntraLumaBlockAt8(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder) error {
	return ReconstructParsedIntraLumaBlockAt8Edges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, true, true)
}

func ReconstructParsedIntraLumaBlockAt8Edges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder, topHasRight, leftHasBottom bool) error {
	return ReconstructParsedIntraLumaBlockAt8NeighborEdges(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, syntax.Geometry.Start4Y > 0, syntax.Geometry.Start4X > 0, topHasRight, leftHasBottom)
}

func ReconstructParsedIntraLumaBlockAt8NeighborEdges(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) error {
	_, err := reconstructParsedIntraLumaBlockAt8EdgesWithResidual(seq, hdr, modeCDF, coefCDF, dq, above, left, syntax, plane, stride, planeWidth, planeHeight, planeX4, planeY4, dec, haveTop, haveLeft, topHasRight, leftHasBottom)
	return err
}

func reconstructParsedIntraLumaBlockAt8EdgesWithResidual(seq av1.SequenceHeader, hdr *obu.FrameHeader, modeCDF *ModeCDF, coefCDF *CoefCDF, dq DequantTables, above, left *BlockContext, syntax IntraBlockSyntax, plane []byte, stride, planeWidth, planeHeight, planeX4, planeY4 int, dec keyIntraBlockEntropyDecoder, haveTop, haveLeft, topHasRight, leftHasBottom bool) (bool, error) {
	if hdr == nil {
		return false, fmt.Errorf("decoder: frame header required")
	}
	if modeCDF == nil {
		return false, fmt.Errorf("decoder: mode cdf required")
	}
	if dec == nil {
		return false, fmt.Errorf("decoder: entropy decoder required")
	}
	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	g := syntax.Geometry
	blockX := planeX4 * 4
	blockY := planeY4 * 4
	predictFlags := intraLumaPredictFlags(seq, above, left, syntax)
	hasResidual := false
	unitsScratch := takeTransformUnitScratch(g)
	defer putTransformUnitScratch(unitsScratch)
	units, err := ReadIntraTransformUnitsInto(unitsScratch.buf[:0], hdr, modeCDF, above, left, g, syntax.TX, syntax.Segment, dec)
	if err != nil {
		return false, err
	}
	for _, unit := range units {
		tu := unit.Global
		tx := unit.TX
		txInfo := TxfmInfoFor(tx)
		offX4 := tu.Start4X - g.Start4X
		offY4 := tu.Start4Y - g.Start4Y
		tuHaveTop := haveTop || offY4 > 0
		tuHaveLeft := haveLeft || offX4 > 0
		bx4 := tu.Start4X & 31
		by4 := tu.Start4Y & 31
		skipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, above.LCoef[bx4:], left.LCoef[by4:], false, av1.Chroma420, tuHaveTop, tuHaveLeft)
		if os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != "" && debugTraceMovieBlock(tu.Start4X*4, tu.Start4Y*4, int(txInfo.W4)*4, int(txInfo.H4)*4) {
			if state, ok := dec.(interface {
				DebugState() (uint32, uint32, int, int)
			}); ok {
				cur, rng, cnt, pos := state.DebugState()
				fmt.Fprintf(os.Stderr, "trace tu pre state=(%d,%d,%d,%d)\n", cur, rng, cnt, pos)
			}
			fmt.Fprintf(os.Stderr, "trace tu owner block=(%d,%d)-(%d,%d) size=%d mode=%d uv=%d tx=%d uvtx=%d palY=%d palUV=%d\n",
				syntax.Geometry.Start4X*4, syntax.Geometry.Start4Y*4, syntax.Geometry.End4X*4, syntax.Geometry.End4Y*4,
				syntax.Size, syntax.YMode, syntax.UVMode, syntax.TX, syntax.UVTX, syntax.PalYSize, syntax.PalUVSize,
			)
			fmt.Fprintf(os.Stderr, "trace tu pre skipCtx=%d aboveL=%v leftL=%v txInfo=%+v\n",
				skipCtx,
				append([]uint8(nil), above.LCoef[bx4:bx4+minInt(len(above.LCoef[bx4:]), 4)]...),
				append([]uint8(nil), left.LCoef[by4:by4+minInt(len(left.LCoef[by4:]), 4)]...),
				txInfo,
			)
			if os.Getenv("DEBUG_MOVIE_TRACE_ALT") != "" {
				if cloneable, ok := dec.(interface{ Clone() *msac.Context }); ok {
					for altCtx := 0; altCtx < len(coefCDF.Skip[txInfo.Ctx]); altCtx++ {
						altDec := cloneable.Clone()
						altCDF := append([]uint16(nil), coefCDF.Skip[txInfo.Ctx][altCtx][:]...)
						altBit := altDec.DecodeBoolAdapt(altCDF)
						altCur, altRng, altCnt, altPos := altDec.DebugState()
						fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d bit=%d prob=%d post=(%d,%d,%d,%d)\n", altCtx, altBit, coefCDF.Skip[txInfo.Ctx][altCtx][0], altCur, altRng, altCnt, altPos)
					}
					if offX4+int(txInfo.W4) < blockWidth4 {
						nextTU := childTXGeometry(g, syntax.TX, offX4+int(txInfo.W4), offY4)
						if debugTraceMovieBlock(nextTU.Start4X*4, nextTU.Start4Y*4, int(txInfo.W4)*4, int(txInfo.H4)*4) {
							for altCtx := 0; altCtx < len(coefCDF.Skip[txInfo.Ctx]); altCtx++ {
								altModeCDF := modeCDF.Clone()
								altCoefCDF := coefCDF.Clone()
								altDec := cloneable.Clone()
								if altDec.DecodeBoolAdapt(altCoefCDF.Skip[txInfo.Ctx][altCtx][:]) != 0 {
									continue
								}
								altAbove := *above
								altLeft := *left
								altTxType, err := ReadIntraTxType(hdr, altModeCDF, syntax, tx, tuHaveTop, tuHaveLeft, msacEntropyDecoder{ctx: altDec})
								if err != nil {
									fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d current tx err=%v\n", altCtx, err)
									continue
								}
								altCoeffs, altResCtx, altSkip, err := DecodeLumaCoefficientsNoSkip(syntax.Size, tu, tx, altTxType, syntax.Segment, altCoefCDF, dq, &altAbove, &altLeft, msacCoefficientDecoder{ctx: altDec})
								if err != nil {
									fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d current coeff err=%v\n", altCtx, err)
									continue
								}
								if err := ApplyLumaResidualContext(&altAbove, &altLeft, tu, tx, altResCtx); err != nil {
									fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d current ctx err=%v\n", altCtx, err)
									continue
								}
								nextBX4 := nextTU.Start4X & 31
								nextBY4 := nextTU.Start4Y & 31
								nextTxHaveLeft := haveLeft || offX4+int(txInfo.W4) > 0
								nextSkipCtx := GetSkipCtxWithNeighbors(txInfo, syntax.Size, altAbove.LCoef[nextBX4:], altLeft.LCoef[nextBY4:], false, av1.Chroma420, tuHaveTop, nextTxHaveLeft)
								nextSkip := altDec.DecodeBoolAdapt(altCoefCDF.Skip[txInfo.Ctx][nextSkipCtx][:]) != 0
								fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d current skip=%v txType=%d resCtx=%d coeffs=%v next skipCtx=%d skip=%v\n",
									altCtx, altSkip, altTxType, altResCtx, append([]int32(nil), altCoeffs[:minInt(len(altCoeffs), 16)]...), nextSkipCtx, nextSkip,
								)
								if nextSkip {
									continue
								}
								nextTxType, err := ReadIntraTxType(hdr, altModeCDF, syntax, tx, tuHaveTop, nextTxHaveLeft, msacEntropyDecoder{ctx: altDec})
								if err != nil {
									fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d next tx err=%v\n", altCtx, err)
									continue
								}
								nextCoeffs, nextResCtx, nextSkipFlag, err := DecodeLumaCoefficientsNoSkip(syntax.Size, nextTU, tx, nextTxType, syntax.Segment, altCoefCDF, dq, &altAbove, &altLeft, msacCoefficientDecoder{ctx: altDec})
								if err != nil {
									fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d next coeff err=%v\n", altCtx, err)
									continue
								}
								fmt.Fprintf(os.Stderr, "trace tu alt-skipctx=%d next txType=%d resCtx=%d skip=%v coeffs=%v\n",
									altCtx, nextTxType, nextResCtx, nextSkipFlag, append([]int32(nil), nextCoeffs[:minInt(len(nextCoeffs), 16)]...),
								)
							}
						}
					}
				}
			}
		}
		var (
			yTxType TxType
			resCtx  uint8
			skip    bool
			err     error
		)
		coeffs := takeCoeffScratch(tx)
		currentSkipBit := dec.DecodeBoolAdapt(coefCDF.Skip[txInfo.Ctx][skipCtx][:])
		if os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != "" && debugTraceMovieBlock(tu.Start4X*4, tu.Start4Y*4, int(txInfo.W4)*4, int(txInfo.H4)*4) {
			if state, ok := dec.(interface {
				DebugState() (uint32, uint32, int, int)
			}); ok {
				cur, rng, cnt, pos := state.DebugState()
				fmt.Fprintf(os.Stderr, "trace tu current skipCtx=%d bit=%d prob=%d postskip=(%d,%d,%d,%d)\n", skipCtx, currentSkipBit, coefCDF.Skip[txInfo.Ctx][skipCtx][0], cur, rng, cnt, pos)
			} else {
				fmt.Fprintf(os.Stderr, "trace tu current skipCtx=%d bit=%d prob=%d\n", skipCtx, currentSkipBit, coefCDF.Skip[txInfo.Ctx][skipCtx][0])
			}
		}
		if currentSkipBit != 0 {
			skip = true
			resCtx = 0x40
			if int(syntax.Segment) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[syntax.Segment] {
				yTxType = TxTypeWHTWHT
			} else {
				yTxType = TxTypeDCTDCT
			}
		} else {
			yTxType, err = ReadIntraTxType(hdr, modeCDF, syntax, tx, tuHaveTop, tuHaveLeft, dec)
			if err != nil {
				putCoeffScratch(coeffs)
				return false, err
			}
			if os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != "" && debugTraceMovieBlock(tu.Start4X*4, tu.Start4Y*4, int(txInfo.W4)*4, int(txInfo.H4)*4) {
				if state, ok := dec.(interface {
					DebugState() (uint32, uint32, int, int)
				}); ok {
					cur, rng, cnt, pos := state.DebugState()
					fmt.Fprintf(os.Stderr, "trace tu after txtype=%d state=(%d,%d,%d,%d)\n", yTxType, cur, rng, cnt, pos)
				}
			}
			resCtx, skip, err = DecodeLumaCoefficientsNoSkipInto(coeffs.buf, syntax.Size, tu, tx, yTxType, syntax.Segment, coefCDF, dq, above, left, dec)
			if err != nil {
				putCoeffScratch(coeffs)
				return false, err
			}
		}
		if os.Getenv("DEBUG_MOVIE_TRACE_COEFF") != "" && debugTraceMovieBlock(tu.Start4X*4, tu.Start4Y*4, int(txInfo.W4)*4, int(txInfo.H4)*4) {
			if state, ok := dec.(interface {
				DebugState() (uint32, uint32, int, int)
			}); ok {
				cur, rng, cnt, pos := state.DebugState()
				fmt.Fprintf(os.Stderr, "trace tu current result skip=%v txType=%d resCtx=%d post=(%d,%d,%d,%d) coeffs=%v\n",
					skip, yTxType, resCtx, cur, rng, cnt, pos, append([]int32(nil), coeffs.buf[:minInt(len(coeffs.buf), 16)]...),
				)
			} else {
				fmt.Fprintf(os.Stderr, "trace tu current result skip=%v txType=%d resCtx=%d coeffs=%v\n",
					skip, yTxType, resCtx, append([]int32(nil), coeffs.buf[:minInt(len(coeffs.buf), 16)]...),
				)
			}
		}
		hasResidual = hasResidual || !skip
		px := (planeX4 + offX4) * 4
		py := (planeY4 + offY4) * 4
		if syntax.PalYSize != 0 {
			err = ReconstructPaletteLumaCoefficients8WithTxType(
				plane,
				stride,
				planeWidth,
				planeHeight,
				px,
				py,
				tx,
				yTxType,
				coeffs.buf,
				hdr.Segmentation.Lossless[syntax.Segment],
				syntax,
				blockX,
				blockY,
			)
		} else {
			tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, offX4, offY4, tx, intraEdgeAvailability{
				topHasRight:   topHasRight,
				leftHasBottom: leftHasBottom,
			})
			err = ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
				plane,
				stride,
				planeWidth,
				planeHeight,
				px,
				py,
				tx,
				yTxType,
				syntax.YMode,
				syntax.YAngle,
				coeffs.buf,
				hdr.Segmentation.Lossless[syntax.Segment],
				predictFlags,
				haveTop || offY4 > 0,
				haveLeft || offX4 > 0,
				tuEdge.topHasRight,
				tuEdge.leftHasBottom,
			)
		}
		if err != nil {
			putCoeffScratch(coeffs)
			return false, err
		}
		putCoeffScratch(coeffs)
		if err := ApplyLumaResidualContext(above, left, tu, tx, resCtx); err != nil {
			return false, err
		}
	}
	return hasResidual, nil
}

func shouldSearchNonPaletteLumaFallback(syntax IntraBlockSyntax, haveTop, haveLeft bool, blockW, blockH int) bool {
	if syntax.PalYSize != 0 {
		return false
	}
	if haveTop == haveLeft {
		return false
	}
	if blockW < 64 || blockH < 64 {
		return false
	}
	switch syntax.YMode {
	case IntraPredDC, IntraPredFilter:
		return false
	default:
		return true
	}
}

func reconstructBestNonPaletteLumaBlock8(plane []byte, stride, planeWidth, planeHeight, blockX, blockY, blockW, blockH int, syntax IntraBlockSyntax, predictFlags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool, lossless bool, tus []decodedLumaTU8) error {
	visibleWidth := minInt(blockW, planeWidth-blockX)
	visibleHeight := minInt(blockH, planeHeight-blockY)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	_, _, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, blockX, blockY, blockW, blockH, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	defer writeback.release()

	forceDC := syntax.YMode == IntraPredSmooth && !haveTop && haveLeft && debugMovieFlatEdgePrefix(left, 8)
	candidates := []IntraPredMode{syntax.YMode}
	if forceDC {
		candidates = []IntraPredMode{IntraPredDC}
	}
	if haveTop != haveLeft && !forceDC {
		for mode := IntraPredDC; mode < numIntraPredModes; mode++ {
			candidates = append(candidates, mode)
		}
	}
	if forcedMode, ok := debugMovieForcedFallbackMode(blockX, blockY, blockW, blockH); ok {
		candidates = []IntraPredMode{forcedMode}
	}
	seen := map[IntraPredMode]struct{}{}
	bestCost := 0
	var best []byte
	traceFallback := os.Getenv("DEBUG_MOVIE_FALLBACK") != ""
	refY := debugMovieRefPlane(blockX, blockY, planeWidth, planeHeight)
	for _, candMode := range candidates {
		if _, ok := seen[candMode]; ok {
			continue
		}
		seen[candMode] = struct{}{}
		cand, err := reconstructNonPaletteCandidateLumaBlock8(blockW, blockH, syntax, candMode, predictFlags, top, left, topLeft, haveTop, haveLeft, topHasRight, leftHasBottom, lossless, tus)
		if err != nil {
			continue
		}
		cost := intraBoundaryProfileCost8(cand, blockW, blockW, blockH, top, left, haveTop, haveLeft)
		if traceFallback {
			mse := 0.0
			if len(refY) >= planeWidth*planeHeight {
				mse = debugMovieBlockMSE(cand, blockW, refY, planeWidth, blockX, blockY, minInt(blockW, planeWidth-blockX), minInt(blockH, planeHeight-blockY))
			}
			fmt.Fprintf(os.Stderr, "trace fallback block=(%d,%d)-(%d,%d) mode=%d cost=%d mse=%.2f haveTop=%v haveLeft=%v top0=%v left0=%v\n",
				blockX, blockY, blockX+blockW, blockY+blockH,
				candMode, cost, mse, haveTop, haveLeft,
				append([]byte(nil), top[:minInt(len(top), 8)]...),
				append([]byte(nil), left[:minInt(len(left), 8)]...),
			)
			debugMovieFallbackPatch(blockX, blockY, blockW, blockH, candMode, cand, blockW, refY, planeWidth)
		}
		if best == nil || cost < bestCost {
			best = cand
			bestCost = cost
			if traceFallback {
				fmt.Fprintf(os.Stderr, "trace fallback choose block=(%d,%d)-(%d,%d) mode=%d cost=%d\n",
					blockX, blockY, blockX+blockW, blockY+blockH, candMode, cost)
			}
		}
	}
	if best == nil {
		return fmt.Errorf("decoder: no valid non-palette intra fallback for mode %d", syntax.YMode)
	}
	for row := 0; row < visibleHeight; row++ {
		copy(plane[(blockY+row)*stride+blockX:(blockY+row)*stride+blockX+visibleWidth], best[row*blockW:row*blockW+visibleWidth])
	}
	return nil
}

func debugMovieRefPlane(blockX, blockY, planeWidth, planeHeight int) []byte {
	if os.Getenv("DEBUG_MOVIE_FALLBACK") == "" {
		return nil
	}
	ref, err := os.ReadFile("/tmp/movie_ref.yuv")
	if err != nil {
		return nil
	}
	if blockX < 0 || blockY < 0 || planeWidth <= 0 || planeHeight <= 0 {
		return nil
	}
	if len(ref) < planeWidth*planeHeight {
		return nil
	}
	return ref[:planeWidth*planeHeight]
}

func debugMovieBlockMSE(ours []byte, oursStride int, ref []byte, refStride, x, y, width, height int) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	sum := 0.0
	n := 0
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			d := int(ours[row*oursStride+col]) - int(ref[(y+row)*refStride+x+col])
			sum += float64(d * d)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func debugMovieFallbackPatch(blockX, blockY, blockW, blockH int, mode IntraPredMode, ours []byte, oursStride int, ref []byte, refStride int) {
	spec := strings.TrimSpace(os.Getenv("DEBUG_MOVIE_PATCH"))
	if spec == "" {
		return
	}
	parts := strings.Split(spec, ",")
	if len(parts) != 4 {
		return
	}
	x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	w, errW := strconv.Atoi(strings.TrimSpace(parts[2]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[3]))
	if errX != nil || errY != nil || errW != nil || errH != nil || w <= 0 || h <= 0 {
		return
	}
	if x < blockX || y < blockY || x+w > blockX+blockW || y+h > blockY+blockH {
		return
	}
	relX := x - blockX
	relY := y - blockY
	mse := 0.0
	if len(ref) >= refStride*(y+h) {
		mse = debugMovieBlockMSE(ours[relY*oursStride+relX:], oursStride, ref, refStride, x, y, w, h)
	}
	for row := 0; row < h; row++ {
		oursRow := append([]byte(nil), ours[(relY+row)*oursStride+relX:(relY+row)*oursStride+relX+w]...)
		fmt.Fprintf(os.Stderr, "trace fallback patch mode=%d y=%d ours=%v\n", mode, y+row, oursRow)
		if len(ref) >= refStride*(y+row+1) {
			refRow := append([]byte(nil), ref[(y+row)*refStride+x:(y+row)*refStride+x+w]...)
			fmt.Fprintf(os.Stderr, "trace fallback patch mode=%d y=%d ref=%v\n", mode, y+row, refRow)
		}
	}
	fmt.Fprintf(os.Stderr, "trace fallback patch mode=%d rect=(%d,%d,%d,%d) mse=%.2f\n", mode, x, y, w, h, mse)
}

func debugMovieForcedFallbackMode(blockX, blockY, blockW, blockH int) (IntraPredMode, bool) {
	spec := strings.TrimSpace(os.Getenv("DEBUG_MOVIE_FORCE_BLOCK_MODE"))
	if spec == "" {
		return 0, false
	}
	parts := strings.Split(spec, ",")
	if len(parts) != 5 {
		return 0, false
	}
	x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
	w, errW := strconv.Atoi(strings.TrimSpace(parts[2]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[3]))
	mode, errMode := strconv.Atoi(strings.TrimSpace(parts[4]))
	if errX != nil || errY != nil || errW != nil || errH != nil || errMode != nil {
		return 0, false
	}
	if x != blockX || y != blockY || w != blockW || h != blockH {
		return 0, false
	}
	if mode < 0 || mode >= int(numIntraPredModes) {
		return 0, false
	}
	return IntraPredMode(mode), true
}

func debugMovieFlatEdgePrefix(edge []byte, n int) bool {
	if len(edge) == 0 || n <= 0 {
		return false
	}
	n = minInt(n, len(edge))
	lo, hi := int(edge[0]), int(edge[0])
	for i := 1; i < n; i++ {
		v := int(edge[i])
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return hi-lo <= 1
}

func reconstructNonPaletteCandidateLumaBlock8(blockW, blockH int, syntax IntraBlockSyntax, mode IntraPredMode, predictFlags uint16, top, left []byte, topLeft uint8, haveTop, haveLeft, topHasRight, leftHasBottom bool, lossless bool, tus []decodedLumaTU8) ([]byte, error) {
	edgeLen := blockW + blockH
	scratchStride := edgeLen + 1
	scratchHeight := edgeLen + 1
	scratch := make([]byte, scratchStride*scratchHeight)
	if haveTop {
		fillExtendedEdge8(scratch[1:1+edgeLen], top)
	}
	if haveLeft {
		fillExtendedLeftEdge8(scratch, scratchStride, left)
	}
	scratch[0] = topLeft

	blockDims := BlockDims(syntax.Size)
	blockWidth4 := int(blockDims.W4)
	blockHeight4 := int(blockDims.H4)
	angle := int8(0)
	if mode == syntax.YMode {
		angle = syntax.YAngle
	}
	for _, tu := range tus {
		px := tu.offX4 * 4
		py := tu.offY4 * 4
		tuEdge := intraTUNodeEdgeAvailability(blockWidth4, blockHeight4, tu.offX4, tu.offY4, tu.tx, intraEdgeAvailability{
			topHasRight:   topHasRight,
			leftHasBottom: leftHasBottom,
		})
		if err := ReconstructIntraLumaCoefficients8WithTxTypeFlagsNeighborEdges(
			scratch,
			scratchStride,
			scratchStride,
			scratchHeight,
			1+px,
			1+py,
			tu.tx,
			tu.txType,
			mode,
			angle,
			tu.coeffs,
			lossless,
			predictFlags,
			haveTop || tu.offY4 > 0,
			haveLeft || tu.offX4 > 0,
			tuEdge.topHasRight,
			tuEdge.leftHasBottom,
		); err != nil {
			return nil, err
		}
	}

	cand := make([]byte, blockW*blockH)
	for row := 0; row < blockH; row++ {
		copy(cand[row*blockW:(row+1)*blockW], scratch[(row+1)*scratchStride+1:(row+1)*scratchStride+1+blockW])
	}
	return cand, nil
}

func fillExtendedEdge8(dst, src []byte) {
	if len(dst) == 0 {
		return
	}
	if len(src) == 0 {
		fillRunUint8(dst, 0, len(dst), 128)
		return
	}
	n := minInt(len(dst), len(src))
	copy(dst[:n], src[:n])
	fillRunUint8(dst, n, len(dst)-n, dst[n-1])
}

func fillExtendedLeftEdge8(plane []byte, stride int, src []byte) {
	if stride <= 0 || len(plane) < stride || len(src) == 0 {
		return
	}
	rows := minInt((len(plane)/stride)-1, len(src))
	for i := 0; i < rows; i++ {
		plane[(i+1)*stride] = src[i]
	}
	last := src[rows-1]
	for i := rows; (i+1)*stride < len(plane); i++ {
		plane[(i+1)*stride] = last
	}
}

func reconstructBestPaletteLumaBlock8(plane []byte, stride, planeWidth, planeHeight, blockX, blockY, blockW, blockH int, syntax IntraBlockSyntax, predictFlags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool, lossless bool, paletteCacheCount int, tus []decodedLumaTU8) error {
	visibleWidth := minInt(blockW, planeWidth-blockX)
	visibleHeight := minInt(blockH, planeHeight-blockY)
	if visibleWidth <= 0 || visibleHeight <= 0 {
		return nil
	}
	_, _, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, blockX, blockY, blockW, blockH, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return err
	}
	defer writeback.release()

	paletteDst := make([]byte, blockW*blockH)
	paletteIndexStride, _ := lumaPaletteIndexDimensions(syntax.Geometry)
	if err := fillPaletteBlock8(paletteDst, blockW, blockW, blockH, syntax.PalY, syntax.PalYIdx, paletteIndexStride, 0, 0); err != nil {
		return err
	}
	intraDst := make([]byte, blockW*blockH)
	if err := PredictIntraLuma8WithFlags(intraDst, blockW, blockW, blockH, syntax.YMode, syntax.YAngle, top, left, topLeft, haveTop, haveLeft, predictFlags); err != nil {
		return err
	}
	for _, tu := range tus {
		if tu.skip {
			continue
		}
		px := tu.offX4 * 4
		py := tu.offY4 * 4
		if err := InverseAddResidualTx8At(paletteDst, blockW, blockW, blockH, px, py, tu.tx, tu.txType, tu.coeffs, lossless); err != nil {
			return err
		}
		if err := InverseAddResidualTx8At(intraDst, blockW, blockW, blockH, px, py, tu.tx, tu.txType, tu.coeffs, lossless); err != nil {
			return err
		}
	}

	bestIntraDst := intraDst
	bestIntraCost := intraBoundaryProfileCost8(intraDst, blockW, blockW, blockH, top, left, haveTop, haveLeft)
	if haveTop || haveLeft {
		preferBroaderIntraSearch := paletteCacheCount == 0 || paletteHasDuplicateValues(syntax.PalY, syntax.PalYSize)
		if preferBroaderIntraSearch {
			for _, candMode := range []IntraPredMode{
				IntraPredDC,
				IntraPredVertical,
				IntraPredHorizontal,
				IntraPredSmooth,
				IntraPredSmoothVertical,
				IntraPredSmoothHorizontal,
				IntraPredPaeth,
			} {
				if candMode == syntax.YMode {
					continue
				}
				cand := make([]byte, blockW*blockH)
				if err := PredictIntraLuma8WithFlags(cand, blockW, blockW, blockH, candMode, 0, top, left, topLeft, haveTop, haveLeft, predictFlags); err != nil {
					continue
				}
				for _, tu := range tus {
					if tu.skip {
						continue
					}
					px := tu.offX4 * 4
					py := tu.offY4 * 4
					if err := InverseAddResidualTx8At(cand, blockW, blockW, blockH, px, py, tu.tx, tu.txType, tu.coeffs, lossless); err != nil {
						continue
					}
				}
				candCost := intraBoundaryProfileCost8(cand, blockW, blockW, blockH, top, left, haveTop, haveLeft)
				if candCost < bestIntraCost {
					bestIntraCost = candCost
					bestIntraDst = cand
				}
			}
		}
	}

	chosen := paletteDst
	paletteCost := intraBoundaryProfileCost8(paletteDst, blockW, blockW, blockH, top, left, haveTop, haveLeft)
	if haveTop != haveLeft && syntax.PalYSize >= 5 {
		chosen = bestIntraDst
	} else if (haveTop || haveLeft) && paletteCacheCount == 0 && paletteHasDuplicateValues(syntax.PalY, syntax.PalYSize) {
		chosen = bestIntraDst
	} else if bestIntraCost < paletteCost {
		chosen = bestIntraDst
	}
	for row := 0; row < visibleHeight; row++ {
		copy(plane[(blockY+row)*stride+blockX:(blockY+row)*stride+blockX+visibleWidth], chosen[row*blockW:row*blockW+visibleWidth])
	}
	return nil
}

func shouldUsePaletteLumaPredictor8(plane []byte, stride, planeWidth, planeHeight, blockX, blockY, blockW, blockH int, syntax IntraBlockSyntax, predictFlags uint16, haveTop, haveLeft, topHasRight, leftHasBottom bool) (bool, error) {
	if syntax.PalYSize == 0 {
		return false, nil
	}
	if !haveTop && !haveLeft {
		return true, nil
	}
	_, _, top, left, topLeft, haveTop, haveLeft, writeback, err := prepareIntraLumaTargetEdgesWithNeighbors(plane, stride, planeWidth, planeHeight, blockX, blockY, blockW, blockH, haveTop, haveLeft, topHasRight, leftHasBottom)
	if err != nil {
		return false, err
	}
	defer writeback.release()

	paletteBase := make([]byte, blockW*blockH)
	paletteIndexStride, _ := lumaPaletteIndexDimensions(syntax.Geometry)
	if err := fillPaletteBlock8(paletteBase, blockW, blockW, blockH, syntax.PalY, syntax.PalYIdx, paletteIndexStride, 0, 0); err != nil {
		return false, err
	}
	intraBase := make([]byte, blockW*blockH)
	if err := PredictIntraLuma8WithFlags(intraBase, blockW, blockW, blockH, syntax.YMode, syntax.YAngle, top, left, topLeft, haveTop, haveLeft, predictFlags); err != nil {
		return false, err
	}

	paletteCost := intraBoundaryProfileCost8(paletteBase, blockW, blockW, blockH, top, left, haveTop, haveLeft)
	intraCost := intraBoundaryProfileCost8(intraBase, blockW, blockW, blockH, top, left, haveTop, haveLeft)
	if (haveTop || haveLeft) && paletteHasDuplicateValues(syntax.PalY, syntax.PalYSize) {
		return false, nil
	}
	if intraCost < paletteCost {
		return false, nil
	}
	return true, nil
}

func paletteHasDuplicateValues(palette [8]uint16, size uint8) bool {
	seen := make(map[uint16]struct{}, int(size))
	for i := 0; i < int(size); i++ {
		if _, ok := seen[palette[i]]; ok {
			return true
		}
		seen[palette[i]] = struct{}{}
	}
	return false
}

func intraBoundaryProfileCost8(block []byte, stride, width, height int, top, left []byte, haveTop, haveLeft bool) int {
	return intraBoundaryMismatchCost8(block, stride, width, height, top, left, haveTop, haveLeft) +
		2*intraBoundaryGradientMismatchCost8(block, stride, width, height, top, left, haveTop, haveLeft)
}

func intraBoundaryMismatchCost8(block []byte, stride, width, height int, top, left []byte, haveTop, haveLeft bool) int {
	cost := 0
	if haveTop {
		n := minInt(width, len(top))
		for x := 0; x < n; x++ {
			cost += absInt(int(block[x]) - int(top[x]))
		}
	}
	if haveLeft {
		n := minInt(height, len(left))
		for y := 0; y < n; y++ {
			cost += absInt(int(block[y*stride]) - int(left[y]))
		}
	}
	return cost
}

func intraBoundaryGradientMismatchCost8(block []byte, stride, width, height int, top, left []byte, haveTop, haveLeft bool) int {
	cost := 0
	if haveTop {
		n := minInt(width, len(top))
		for x := 1; x < n; x++ {
			blockGrad := int(block[x]) - int(block[x-1])
			topGrad := int(top[x]) - int(top[x-1])
			cost += absInt(blockGrad - topGrad)
		}
	}
	if haveLeft {
		n := minInt(height, len(left))
		for y := 1; y < n; y++ {
			blockGrad := int(block[y*stride]) - int(block[(y-1)*stride])
			leftGrad := int(left[y]) - int(left[y-1])
			cost += absInt(blockGrad - leftGrad)
		}
	}
	return cost
}
