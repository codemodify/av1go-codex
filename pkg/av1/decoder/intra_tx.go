package decoder

import (
	"fmt"

	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

func ReadIntraTransformUnits(hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, global BlockGeometry, rootTX TxfmSize, segID uint8, dec intraEntropyDecoder) ([]TransformUnit, error) {
	return ReadIntraTransformUnitsInto(nil, hdr, cdf, above, left, global, rootTX, segID, dec)
}

func ReadIntraTransformUnitsInto(dst []TransformUnit, hdr *obu.FrameHeader, cdf *ModeCDF, above, left *BlockContext, global BlockGeometry, rootTX TxfmSize, segID uint8, dec intraEntropyDecoder) ([]TransformUnit, error) {
	if hdr == nil {
		return nil, fmt.Errorf("decoder: frame header required")
	}
	if cdf == nil {
		return nil, fmt.Errorf("decoder: mode cdf required")
	}
	if above == nil || left == nil {
		return nil, fmt.Errorf("decoder: block contexts required")
	}
	maxTX := MaxLumaTXSizeForBlock(global.Size)
	if int(segID) < len(hdr.Segmentation.Lossless) && hdr.Segmentation.Lossless[segID] {
		maxTX = TX4X4
	}
	if rootTX > maxTX {
		return nil, fmt.Errorf("decoder: root tx %d exceeds max tx %d for block size %d", rootTX, maxTX, global.Size)
	}
	// Intra block syntax selects the final luma transform size. Unlike inter
	// blocks, there is no variable transform tree to read afterwards; the block
	// is tiled on a fixed grid of rootTX-sized transform units.
	return buildFixedIntraTransformUnitsInto(dst, above, left, global, rootTX)
}

func readIntraTXTree(cdf *ModeCDF, above, left *BlockContext, global BlockGeometry, tx TxfmSize, depth int, dec intraEntropyDecoder, units *[]TransformUnit) error {
	out, err := readIntraTXTreeInto(cdf, above, left, global, tx, depth, dec, (*units)[:0])
	if err != nil {
		return err
	}
	*units = out
	return nil
}

func readIntraTXTreeInto(cdf *ModeCDF, above, left *BlockContext, global BlockGeometry, tx TxfmSize, depth int, dec intraEntropyDecoder, units []TransformUnit) ([]TransformUnit, error) {
	info := TxfmInfoFor(tx)
	split := false
	if depth < 2 && tx > TX4X4 {
		cat := 2*(int(TxfmInfoFor(TX64X64).Max)-int(info.Max)) - depth
		ctx := GetTxCtx(above, left, info, global.Start4Y&31, global.Start4X&31)
		split = dec.DecodeBoolAdapt(cdf.TXPart[cat][ctx][:]) != 0
	}
	if split && tx > TX4X4 {
		sub := info.Sub
		subInfo := TxfmInfoFor(sub)
		var err error
		units, err = readIntraTXTreeInto(cdf, above, left, childTXGeometry(global, sub, 0, 0), sub, depth+1, dec, units)
		if err != nil {
			return nil, err
		}
		if info.W4 >= info.H4 && global.Start4X+int(subInfo.W4) < global.End4X {
			units, err = readIntraTXTreeInto(cdf, above, left, childTXGeometry(global, sub, int(subInfo.W4), 0), sub, depth+1, dec, units)
			if err != nil {
				return nil, err
			}
		}
		if info.H4 >= info.W4 && global.Start4Y+int(subInfo.H4) < global.End4Y {
			units, err = readIntraTXTreeInto(cdf, above, left, childTXGeometry(global, sub, 0, int(subInfo.H4)), sub, depth+1, dec, units)
			if err != nil {
				return nil, err
			}
			if info.W4 >= info.H4 && global.Start4X+int(subInfo.W4) < global.End4X {
				units, err = readIntraTXTreeInto(cdf, above, left, childTXGeometry(global, sub, int(subInfo.W4), int(subInfo.H4)), sub, depth+1, dec, units)
				if err != nil {
					return nil, err
				}
			}
		}
		return units, nil
	}
	if err := applyIntraTXContext(above, left, global, tx); err != nil {
		return nil, err
	}
	units = append(units, TransformUnit{Local: global, Global: global, TX: tx})
	return units, nil
}

func applyIntraTXContext(above, left *BlockContext, global BlockGeometry, tx TxfmSize) error {
	if above == nil || left == nil {
		return fmt.Errorf("decoder: block contexts required")
	}
	info := TxfmInfoFor(tx)
	bx4 := global.Start4X & 31
	by4 := global.Start4Y & 31
	fillRunInt8(above.TxIntra[:], bx4, minInt(int(info.W4), global.Width4()), int8(info.LW))
	fillRunInt8(left.TxIntra[:], by4, minInt(int(info.H4), global.Height4()), int8(info.LH))
	fillRunInt8(above.Tx[:], bx4, minInt(int(info.W4), global.Width4()), int8(info.LW))
	fillRunInt8(left.Tx[:], by4, minInt(int(info.H4), global.Height4()), int8(info.LH))
	return nil
}

func buildFixedIntraTransformUnits(above, left *BlockContext, global BlockGeometry, tx TxfmSize) ([]TransformUnit, error) {
	return buildFixedIntraTransformUnitsInto(nil, above, left, global, tx)
}

func buildFixedIntraTransformUnitsInto(dst []TransformUnit, above, left *BlockContext, global BlockGeometry, tx TxfmSize) ([]TransformUnit, error) {
	info := TxfmInfoFor(tx)
	countX, countY := transformGridCounts(global, tx)
	need := countX * countY
	var units []TransformUnit
	if cap(dst) < need {
		units = make([]TransformUnit, 0, need)
	} else {
		units = dst[:0]
	}
	for offY4 := 0; offY4 < global.Height4(); offY4 += int(info.H4) {
		for offX4 := 0; offX4 < global.Width4(); offX4 += int(info.W4) {
			tu := childTXGeometry(global, tx, offX4, offY4)
			if err := applyIntraTXContext(above, left, tu, tx); err != nil {
				return nil, err
			}
			units = append(units, TransformUnit{Local: tu, Global: tu, TX: tx})
		}
	}
	return units, nil
}
