package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type TransformUnit struct {
	Local  BlockGeometry
	Global BlockGeometry
	TX     TxfmSize
}

func ReadInterTransformUnits(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, local, global BlockGeometry, segID uint8, skip bool, dec intraEntropyDecoder) ([]TransformUnit, error) {
	return ReadInterTransformUnitsInto(nil, hdr, cdf, above, left, local, global, segID, skip, dec)
}

func ReadInterTransformUnitsInto(dst []TransformUnit, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, local, global BlockGeometry, segID uint8, skip bool, dec intraEntropyDecoder) ([]TransformUnit, error) {
	if hdr == nil {
		return nil, fmt.Errorf("decoder: frame header required")
	}
	if above == nil || left == nil {
		return nil, fmt.Errorf("decoder: block contexts required")
	}
	maxTX := MaxLumaTXSizeForBlock(global.Size)
	if int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
		maxTX = TX4X4
	}
	if maxTX == TX4X4 {
		return buildFixedInterTransformUnitsInto(dst, above, left, local, global, maxTX)
	}
	if skip || hdr.TxfmMode != txModeSwitchable {
		return buildFixedInterTransformUnitsInto(dst, above, left, local, global, maxTX)
	}
	maxUnits := maxTransformUnitCount(local)
	var units []TransformUnit
	if cap(dst) < maxUnits {
		units = make([]TransformUnit, 0, maxUnits)
	} else {
		units = dst[:0]
	}
	var err error
	info := TxfmInfoFor(maxTX)
	for offY4 := 0; offY4 < local.Height4(); offY4 += int(info.H4) {
		for offX4 := 0; offX4 < local.Width4(); offX4 += int(info.W4) {
			localRoot := childTXGeometry(local, maxTX, offX4, offY4)
			globalRoot := childTXGeometry(global, maxTX, offX4, offY4)
			if units, err = readInterTXTreeInto(cdf, above, left, localRoot, globalRoot, maxTX, 0, dec, units); err != nil {
				return nil, err
			}
		}
	}
	return units, nil
}

func debugState4(dec intraEntropyDecoder) [4]int {
	if state, ok := dec.(debugMSACState); ok {
		cur, rng, cnt, pos := state.DebugState()
		return [4]int{int(cur), int(rng), cnt, pos}
	}
	return [4]int{-1, -1, -1, -1}
}

func readInterTXTree(cdf *ModeCDF, above, left *BlockContext, local, global BlockGeometry, tx TxfmSize, depth int, dec intraEntropyDecoder, units *[]TransformUnit) error {
	out, err := readInterTXTreeInto(cdf, above, left, local, global, tx, depth, dec, (*units)[:0])
	if err != nil {
		return err
	}
	*units = out
	return nil
}

func readInterTXTreeInto(cdf *ModeCDF, above, left *BlockContext, local, global BlockGeometry, tx TxfmSize, depth int, dec intraEntropyDecoder, units []TransformUnit) ([]TransformUnit, error) {
	info := TxfmInfoFor(tx)
	split := false
	if depth < 2 && tx > TX4X4 {
		cat := 2*(int(TxfmInfoFor(TX64X64).Max)-int(info.Max)) - depth
		ctx := GetInterTxCtx(above, left, info, local.Start4Y&31, local.Start4X&31)
		cdfRow := cdf.TXPart[cat][ctx][:]
		preCDF := [2]uint16{cdfRow[0], cdfRow[1]}
		preState := debugState4(dec)
		splitBit := dec.DecodeBoolAdapt(cdfRow)
		postState := debugState4(dec)
		postCDF := [2]uint16{cdfRow[0], cdfRow[1]}
		debugTraceInterTXSplit(local, global, tx, depth, cat, ctx, above.Tx[local.Start4X&31], left.Tx[local.Start4Y&31], splitBit, preState, postState, preCDF, postCDF)
		split = splitBit != 0
	}
	if split && tx > TX4X4 {
		sub := info.Sub
		subInfo := TxfmInfoFor(sub)
		var err error
		units, err = readInterTXTreeInto(cdf, above, left, childTXGeometry(local, sub, 0, 0), childTXGeometry(global, sub, 0, 0), sub, depth+1, dec, units)
		if err != nil {
			return nil, err
		}
		if info.W4 >= info.H4 && local.Start4X+int(subInfo.W4) < local.End4X {
			units, err = readInterTXTreeInto(cdf, above, left, childTXGeometry(local, sub, int(subInfo.W4), 0), childTXGeometry(global, sub, int(subInfo.W4), 0), sub, depth+1, dec, units)
			if err != nil {
				return nil, err
			}
		}
		if info.H4 >= info.W4 && local.Start4Y+int(subInfo.H4) < local.End4Y {
			units, err = readInterTXTreeInto(cdf, above, left, childTXGeometry(local, sub, 0, int(subInfo.H4)), childTXGeometry(global, sub, 0, int(subInfo.H4)), sub, depth+1, dec, units)
			if err != nil {
				return nil, err
			}
			if info.W4 >= info.H4 && local.Start4X+int(subInfo.W4) < local.End4X {
				units, err = readInterTXTreeInto(cdf, above, left, childTXGeometry(local, sub, int(subInfo.W4), int(subInfo.H4)), childTXGeometry(global, sub, int(subInfo.W4), int(subInfo.H4)), sub, depth+1, dec, units)
				if err != nil {
					return nil, err
				}
			}
		}
		return units, nil
	}
	if err := applyInterTXContext(above, left, local, tx); err != nil {
		return nil, err
	}
	units = append(units, TransformUnit{Local: local, Global: global, TX: tx})
	return units, nil
}

func applyInterTXContext(above, left *BlockContext, local BlockGeometry, tx TxfmSize) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	info := TxfmInfoFor(tx)
	bx4 := local.Start4X & 31
	by4 := local.Start4Y & 31
	fillRunInt8(above.Tx[:], bx4, minInt(int(info.W4), local.Width4()), int8(info.LW))
	fillRunInt8(left.Tx[:], by4, minInt(int(info.H4), local.Height4()), int8(info.LH))
	return nil
}

func buildFixedInterTransformUnits(above, left *BlockContext, local, global BlockGeometry, tx TxfmSize) ([]TransformUnit, error) {
	return buildFixedInterTransformUnitsInto(nil, above, left, local, global, tx)
}

func buildFixedInterTransformUnitsInto(dst []TransformUnit, above, left *BlockContext, local, global BlockGeometry, tx TxfmSize) ([]TransformUnit, error) {
	info := TxfmInfoFor(tx)
	countX, countY := transformGridCounts(local, tx)
	need := countX * countY
	var units []TransformUnit
	if cap(dst) < need {
		units = make([]TransformUnit, 0, need)
	} else {
		units = dst[:0]
	}
	for offY4 := 0; offY4 < local.Height4(); offY4 += int(info.H4) {
		for offX4 := 0; offX4 < local.Width4(); offX4 += int(info.W4) {
			localTU := childTXGeometry(local, tx, offX4, offY4)
			globalTU := childTXGeometry(global, tx, offX4, offY4)
			if err := applyInterTXContext(above, left, localTU, tx); err != nil {
				return nil, err
			}
			units = append(units, TransformUnit{
				Local:  localTU,
				Global: globalTU,
				TX:     tx,
			})
		}
	}
	return units, nil
}

func childTXGeometry(root BlockGeometry, tx TxfmSize, dx4, dy4 int) BlockGeometry {
	info := TxfmInfoFor(tx)
	return BlockGeometry{
		Level:   root.Level,
		Size:    root.Size,
		Start4X: root.Start4X + dx4,
		Start4Y: root.Start4Y + dy4,
		End4X:   min(root.Start4X+dx4+int(info.W4), root.End4X),
		End4Y:   min(root.Start4Y+dy4+int(info.H4), root.End4Y),
	}
}
