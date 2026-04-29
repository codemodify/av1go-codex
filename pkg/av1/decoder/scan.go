package decoder

import "fmt"

var generatedLastNonzeroColFromEOB = buildLastNonzeroColFromEOB()

func ScanOrder(tx TxfmSize) []uint16 {
	if int(tx) >= len(generatedScans) {
		return nil
	}
	return generatedScans[tx]
}

func LastNonzeroColFromEOB(tx TxfmSize) []uint8 {
	if int(tx) >= len(generatedLastNonzeroColFromEOB) {
		return nil
	}
	return generatedLastNonzeroColFromEOB[tx]
}

func ScanCoeffArea(tx TxfmSize) (width, height int) {
	info := TxfmInfoFor(tx)
	return 4 << minInt(int(info.LW), int(TxfmInfoFor(TX32X32).LW)),
		4 << minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
}

func CoeffIndexFromScan(tx TxfmSize, rc uint32) (int, error) {
	info := TxfmInfoFor(tx)
	fullWidth := int(info.W4) * 4
	fullHeight := int(info.H4) * 4
	scanWidth, scanHeight := ScanCoeffArea(tx)
	if scanWidth <= 0 || scanHeight <= 0 {
		return 0, fmt.Errorf("decoder: invalid scan area for tx %d", tx)
	}
	x := int(rc) / scanHeight
	y := int(rc) % scanHeight
	if y < 0 || y >= scanHeight || x < 0 || x >= scanWidth {
		return 0, fmt.Errorf("decoder: scan rc=%d out of %dx%d range for tx %d", rc, scanWidth, scanHeight, tx)
	}
	if x >= fullWidth || y >= fullHeight {
		return 0, fmt.Errorf("decoder: scan rc=%d maps outside full %dx%d coeff plane for tx %d", rc, fullWidth, fullHeight, tx)
	}
	return y*fullWidth + x, nil
}

func buildLastNonzeroColFromEOB() [numRectTxfmSizes][]uint8 {
	var out [numRectTxfmSizes][]uint8
	for tx := TxfmSize(0); tx < numRectTxfmSizes; tx++ {
		scan := ScanOrder(tx)
		if len(scan) == 0 {
			continue
		}
		info := TxfmInfoFor(tx)
		h := 4 << minInt(int(info.LH), int(TxfmInfoFor(TX32X32).LH))
		table := make([]uint8, len(scan))
		maxCol := 0
		for i, rc := range scan {
			col := int(rc) & (h - 1)
			if col > maxCol {
				maxCol = col
			}
			table[i] = uint8(maxCol)
		}
		out[tx] = table
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
