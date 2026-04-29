package decoder

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type tx1DKind uint8

const (
	tx1DKindDCT tx1DKind = iota
	tx1DKindADST
	tx1DKindFlipADST
	tx1DKindIdentity
)

var generatedTX1DTypes = [numTxTypesPlusLL][2]tx1DKind{
	TxTypeDCTDCT:           {tx1DKindDCT, tx1DKindDCT},
	TxTypeADSTDCT:          {tx1DKindDCT, tx1DKindADST},
	TxTypeDCTADST:          {tx1DKindADST, tx1DKindDCT},
	TxTypeADSTADST:         {tx1DKindADST, tx1DKindADST},
	TxTypeFlipADSTDCT:      {tx1DKindDCT, tx1DKindFlipADST},
	TxTypeDCTFlipADST:      {tx1DKindFlipADST, tx1DKindDCT},
	TxTypeFlipADSTFlipADST: {tx1DKindFlipADST, tx1DKindFlipADST},
	TxTypeADSTFlipADST:     {tx1DKindFlipADST, tx1DKindADST},
	TxTypeFlipADSTADST:     {tx1DKindADST, tx1DKindFlipADST},
	TxTypeIdentity:         {tx1DKindIdentity, tx1DKindIdentity},
	TxTypeVDCT:             {tx1DKindIdentity, tx1DKindDCT},
	TxTypeHDCT:             {tx1DKindDCT, tx1DKindIdentity},
	TxTypeVADST:            {tx1DKindIdentity, tx1DKindADST},
	TxTypeHADST:            {tx1DKindADST, tx1DKindIdentity},
	TxTypeVFlipADST:        {tx1DKindIdentity, tx1DKindFlipADST},
	TxTypeHFlipADST:        {tx1DKindFlipADST, tx1DKindIdentity},
	TxTypeWHTWHT:           {tx1DKindIdentity, tx1DKindIdentity},
}

func tx1DTypesFor(txType TxType) (tx1DKind, tx1DKind, error) {
	if txType >= numTxTypesPlusLL {
		return 0, 0, fmt.Errorf("decoder: unsupported tx type %d", txType)
	}
	kinds := generatedTX1DTypes[txType]
	return kinds[0], kinds[1], nil
}

func intraModeForTxType(mode IntraPredMode, filterIntra int8) IntraPredMode {
	if mode != IntraPredFilter {
		return mode
	}
	if filterIntra >= 0 && int(filterIntra) < len(generatedFilterModeToYMode) {
		return IntraPredMode(generatedFilterModeToYMode[filterIntra])
	}
	return IntraPredDC
}

func intraModeForTxTypeContext(mode IntraPredMode, arg int8, haveTop, haveLeft bool) IntraPredMode {
	baseMode := intraModeForTxType(mode, arg)
	switch baseMode {
	case IntraPredPaeth:
		if haveTop && haveLeft {
			return IntraPredPaeth
		}
		return IntraPredDC
	case IntraPredSmooth:
		switch {
		case haveTop && haveLeft:
			return IntraPredSmooth
		case haveTop:
			return IntraPredSmoothVertical
		case haveLeft:
			return IntraPredSmoothHorizontal
		default:
			return IntraPredDC
		}
	case IntraPredSmoothVertical:
		switch {
		case haveTop:
			return IntraPredSmoothVertical
		case haveLeft:
			return IntraPredSmoothHorizontal
		default:
			return IntraPredDC
		}
	case IntraPredSmoothHorizontal:
		switch {
		case haveLeft:
			return IntraPredSmoothHorizontal
		case haveTop:
			return IntraPredSmoothVertical
		default:
			return IntraPredDC
		}
	}
	effMode, _ := effectiveIntraMode(baseMode, 0, haveTop, haveLeft)
	switch effMode {
	case intraPredZ1, intraPredZ2, intraPredZ3:
		return intraModeForContext(baseMode)
	default:
		return intraModeForContext(effMode)
	}
}

func intraModeForContext(mode IntraPredMode) IntraPredMode {
	if mode == IntraPredFilter {
		return IntraPredDC
	}
	return mode
}

func debugIntraTxTypeNeighborOverride(haveTop, haveLeft bool) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_INTRA_TXTYPE_NEIGHBORS"))) {
	case "", "actual", "default":
		return haveTop, haveLeft
	case "both":
		return true, true
	case "top":
		return true, false
	case "left":
		return false, true
	case "none":
		return false, false
	default:
		return haveTop, haveLeft
	}
}

func debugIntraTxTypeModeOverride(current, raw IntraPredMode, arg int8) IntraPredMode {
	normalize := func(mode IntraPredMode) IntraPredMode {
		mode = intraModeForTxType(mode, arg)
		if mode >= numIntraPredModes {
			return current
		}
		return mode
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_INTRA_TXTYPE_MODE"))) {
	case "", "context", "default":
		return current
	case "raw":
		return normalize(raw)
	case "dc":
		return IntraPredDC
	case "vert", "vertical":
		return IntraPredVertical
	case "horiz", "horizontal":
		return IntraPredHorizontal
	case "smooth":
		return IntraPredSmooth
	case "smooth_v", "smooth_vertical":
		return IntraPredSmoothVertical
	case "smooth_h", "smooth_horizontal":
		return IntraPredSmoothHorizontal
	case "paeth":
		return IntraPredPaeth
	default:
		return current
	}
}

func ReadIntraTxType(hdr *obu.FrameHeader, cdf *ModeCDF, syntax IntraBlockSyntax, tx TxfmSize, haveTop, haveLeft bool, dec intraEntropyDecoder) (TxType, error) {
	if hdr == nil {
		return 0, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return 0, fmt.Errorf("decoder: mode cdf required")
	}
	if dec == nil {
		return 0, fmt.Errorf("decoder: entropy decoder required")
	}
	if int(syntax.Segment) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[syntax.Segment] {
		return TxTypeWHTWHT, nil
	}

	info := TxfmInfoFor(tx)
	if int(info.Max)+1 >= int(TX64X64) {
		return TxTypeDCTDCT, nil
	}
	if int(syntax.Segment) < len(hdr.Segmentation.QIndex) && hdr.Segmentation.QIndex[syntax.Segment] == 0 {
		return TxTypeDCTDCT, nil
	}

	yMode := intraModeForTxType(syntax.YMode, syntax.YAngle)
	yMode = debugIntraTxTypeModeOverride(yMode, syntax.YMode, syntax.YAngle)
	switch {
	case hdr.ReducedTxtpSet || info.Min == uint8(TX16X16):
		idx := dec.DecodeSymbolAdapt(cdf.TXTypeIntra2[info.Min][yMode][:], 4)
		return TxType(generatedTXTypesPerSet[idx]), nil
	default:
		idx := dec.DecodeSymbolAdapt(cdf.TXTypeIntra1[info.Min][yMode][:], 6)
		return TxType(generatedTXTypesPerSet[idx+5]), nil
	}
}

func ReadInterTxType(hdr *obu.FrameHeader, cdf *ModeCDF, segID uint8, tx TxfmSize, dec intraEntropyDecoder) (TxType, error) {
	if hdr == nil {
		return 0, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return 0, fmt.Errorf("decoder: mode cdf required")
	}
	if dec == nil {
		return 0, fmt.Errorf("decoder: entropy decoder required")
	}
	if int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
		return TxTypeWHTWHT, nil
	}

	info := TxfmInfoFor(tx)
	if info.Max >= uint8(TX64X64) {
		return TxTypeDCTDCT, nil
	}
	if int(segID) < len(hdr.Segmentation.QIndex) && hdr.Segmentation.QIndex[segID] == 0 {
		return TxTypeDCTDCT, nil
	}

	trace := debugInterTxTypeTraceEnabled()
	switch {
	case hdr.ReducedTxtpSet || info.Max == uint8(TX32X32):
		row := cdf.TXTypeInter3[info.Min][:]
		preCDF := debugInterTxTypeCDFSnapshot(row, trace)
		preState := debugInterTxTypeState(dec, trace)
		idx := dec.DecodeBoolAdapt(row)
		txType := TxTypeDCTDCT
		if idx == 0 {
			txType = TxTypeIdentity
		}
		debugTraceInterTxTypeDecision(tx, info, "inter3", 1, idx, txType, preState, debugInterTxTypeState(dec, trace), preCDF, row)
		return txType, nil
	case info.Min == uint8(TX16X16):
		row := cdf.TXTypeInter2[:]
		preCDF := debugInterTxTypeCDFSnapshot(row, trace)
		preState := debugInterTxTypeState(dec, trace)
		idx := dec.DecodeSymbolAdapt(row, 11)
		txType := TxType(generatedTXTypesPerSet[idx+12])
		debugTraceInterTxTypeDecision(tx, info, "inter2", 11, idx, txType, preState, debugInterTxTypeState(dec, trace), preCDF, row)
		return txType, nil
	default:
		row := cdf.TXTypeInter1[info.Min][:]
		preCDF := debugInterTxTypeCDFSnapshot(row, trace)
		preState := debugInterTxTypeState(dec, trace)
		idx := dec.DecodeSymbolAdapt(row, 15)
		txType := TxType(generatedTXTypesPerSet[idx+24])
		debugTraceInterTxTypeDecision(tx, info, "inter1", 15, idx, txType, preState, debugInterTxTypeState(dec, trace), preCDF, row)
		return txType, nil
	}
}

func debugInterTxTypeTraceEnabled() bool {
	return strings.TrimSpace(os.Getenv("DEBUG_INTER_TXTYPE_TRACE")) != ""
}

func debugInterTxTypeCDFSnapshot(cdf []uint16, trace bool) []uint16 {
	if !trace {
		return nil
	}
	return append([]uint16(nil), cdf...)
}

func debugInterTxTypeState(dec intraEntropyDecoder, trace bool) [4]int {
	if !trace {
		return [4]int{}
	}
	if state, ok := dec.(debugMSACState); ok {
		cur, rng, cnt, pos := state.DebugState()
		return [4]int{int(cur), int(rng), cnt, pos}
	}
	return [4]int{-1, -1, -1, -1}
}

func debugTraceInterTxTypeDecision(tx TxfmSize, info TxfmInfo, set string, n int, idx uint32, txType TxType, preState, postState [4]int, preCDF, postCDF []uint16) {
	if !debugInterTxTypeTraceEnabled() {
		return
	}
	limit := n + 1
	if limit > len(preCDF) {
		limit = len(preCDF)
	}
	postLimit := n + 1
	if postLimit > len(postCDF) {
		postLimit = len(postCDF)
	}
	fmt.Fprintf(os.Stderr, "DEBUG_INTER_TXTYPE tx=%d min=%d max=%d ctx=%d set=%s n=%d idx=%d txType=%d pre=(cur=%d,r=%d,cnt=%d,pos=%d) post=(cur=%d,r=%d,cnt=%d,pos=%d) preCDF=%v postCDF=%v\n",
		tx, info.Min, info.Max, info.Ctx, set, n, idx, txType,
		preState[0], preState[1], preState[2], preState[3],
		postState[0], postState[1], postState[2], postState[3],
		preCDF[:limit], postCDF[:postLimit],
	)
}

func InterUVTxType(tx TxfmSize, yTxType TxType) TxType {
	info := TxfmInfoFor(tx)
	if info.Max == uint8(TX32X32) {
		if yTxType == TxTypeIdentity {
			return TxTypeIdentity
		}
		return TxTypeDCTDCT
	}
	if info.Min == uint8(TX16X16) {
		switch yTxType {
		case TxTypeHFlipADST, TxTypeVFlipADST, TxTypeHADST, TxTypeVADST:
			return TxTypeDCTDCT
		}
	}
	return yTxType
}

func IntraUVTxType(hdr *obu.FrameHeader, segID uint8, tx TxfmSize, mode IntraPredMode) TxType {
	if override, ok := debugIntraUVTxTypeOverride(); ok {
		return override
	}
	if hdr != nil {
		if int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
			return TxTypeWHTWHT
		}
		if int(segID) < len(hdr.Segmentation.QIndex) && hdr.Segmentation.QIndex[segID] == 0 {
			return TxTypeDCTDCT
		}
	}
	if int(TxfmInfoFor(tx).Max)+1 >= int(TX64X64) {
		return TxTypeDCTDCT
	}
	if int(mode) >= len(generatedTXTypeFromUVMode) {
		return TxTypeDCTDCT
	}
	return TxType(generatedTXTypeFromUVMode[mode])
}

func debugIntraUVTxTypeOverride() (TxType, bool) {
	raw := strings.TrimSpace(os.Getenv("DEBUG_INTRA_UV_TX_TYPE"))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 || v >= int(numTxTypesPlusLL) {
		return 0, false
	}
	return TxType(v), true
}
