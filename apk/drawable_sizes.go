package apk

// renderSize is the target pixel size for vector/XML drawables.
// Raster PNG/WebP assets keep their decoded dimensions.
type renderSize struct {
	width, height int
}

func (s renderSize) valid() bool {
	return s.width > 0 && s.height > 0
}

var (
	sizeIcon   = renderSize{width: 192, height: 192}
	sizeBanner = renderSize{width: 320, height: 180}
)
