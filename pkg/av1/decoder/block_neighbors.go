package decoder

func blockNeighborFlags(tile TileGeometry, g BlockGeometry) (haveTop, haveLeft bool) {
	return g.Start4Y > tile.Start4Y, g.Start4X > tile.Start4X
}
