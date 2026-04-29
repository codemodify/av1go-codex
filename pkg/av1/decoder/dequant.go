package decoder

import (
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
)

type DequantTables [8][3][2]uint16

func InitDequantTables(seq av1.SequenceHeader, hdr *obu.FrameHeader, qidx int) DequantTables {
	var dq DequantTables
	if hdr == nil {
		return dq
	}

	bd := bitDepthIndex(seq.ColorConfig.BitDepth)
	nSeg := 8
	if !hdr.Segmentation.Enabled {
		nSeg = 1
	}

	for i := 0; i < nSeg; i++ {
		yac := qidx
		if hdr.Segmentation.Enabled {
			yac += int(hdr.Segmentation.Data[i].DeltaQ)
		}
		yac = clipU8(yac)
		ydc := clipU8(yac + int(hdr.Quantization.YDCDelta))
		uac := clipU8(yac + int(hdr.Quantization.UACDelta))
		udc := clipU8(yac + int(hdr.Quantization.UDCDelta))
		vac := clipU8(yac + int(hdr.Quantization.VACDelta))
		vdc := clipU8(yac + int(hdr.Quantization.VDCDelta))

		dq[i][0][0] = generatedDQTable[bd][ydc][0]
		dq[i][0][1] = generatedDQTable[bd][yac][1]
		dq[i][1][0] = generatedDQTable[bd][udc][0]
		dq[i][1][1] = generatedDQTable[bd][uac][1]
		dq[i][2][0] = generatedDQTable[bd][vdc][0]
		dq[i][2][1] = generatedDQTable[bd][vac][1]
	}

	if !hdr.Segmentation.Enabled {
		for i := 1; i < len(dq); i++ {
			dq[i] = dq[0]
		}
	}

	return dq
}

func bitDepthIndex(bitDepth uint8) int {
	switch {
	case bitDepth <= 8:
		return 0
	case bitDepth <= 10:
		return 1
	default:
		return 2
	}
}

func clipU8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
