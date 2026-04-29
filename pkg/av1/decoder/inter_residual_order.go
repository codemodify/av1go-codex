package decoder

import "github.com/codemodify/av1go-codex/pkg/av1"

func residualChunkLumaGeometry(g BlockGeometry, offX4, offY4 int) BlockGeometry {
	return BlockGeometry{
		Level:   g.Level,
		Size:    g.Size,
		Start4X: g.Start4X + offX4,
		Start4Y: g.Start4Y + offY4,
		End4X:   min(g.Start4X+offX4+16, g.End4X),
		End4Y:   min(g.Start4Y+offY4+16, g.End4Y),
	}
}

func residualChunkChromaGeometry(layout av1.ChromaSubsampling, g BlockGeometry, offX4, offY4 int) BlockGeometry {
	chroma := chromaGeometryForBlock(layout, g)
	ssX, ssY := chromaSubsamplingShifts(layout)
	startX := offX4 >> ssX
	startY := offY4 >> ssY
	endX := min(chroma.Width4(), (offX4+16)>>ssX)
	endY := min(chroma.Height4(), (offY4+16)>>ssY)
	return BlockGeometry{
		Level:   chroma.Level,
		Size:    chroma.Size,
		Start4X: chroma.Start4X + startX,
		Start4Y: chroma.Start4Y + startY,
		End4X:   chroma.Start4X + endX,
		End4Y:   chroma.Start4Y + endY,
	}
}

func geometryContainsStart(container, g BlockGeometry) bool {
	return g.Start4X >= container.Start4X &&
		g.Start4X < container.End4X &&
		g.Start4Y >= container.Start4Y &&
		g.Start4Y < container.End4Y
}
