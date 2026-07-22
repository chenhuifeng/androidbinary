package apk

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/draw"

	"github.com/chenhuifeng/androidbinary/v2"
)

// bannerAspectThreshold: images wider than this are treated as banner-shaped.
// Android TV banners are typically ~16:9 (≈1.78).
const bannerAspectThreshold = 1.35

// normalizeIconShape center-crops a banner-shaped image to a square,
// unless skip is true (icon and banner are the same wide asset).
func normalizeIconShape(img image.Image, skip bool) image.Image {
	if img == nil || skip {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return img
	}
	aspect := float64(w) / float64(h)
	if aspect < 1 {
		aspect = 1 / aspect
	}
	if aspect <= bannerAspectThreshold {
		return img
	}

	side := h
	if w < h {
		side = w
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	crop := image.Rect(x0, y0, x0+side, y0+side)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(dst, dst.Bounds(), img, crop.Min, draw.Src)
	return dst
}

func (k *Apk) iconSameAsBannerAsset(iconPath string, resConfig *androidbinary.ResTableConfig) bool {
	bannerPath, err := k.drawablePath(k.manifest.App.Banner, resConfig)
	if err != nil || bannerPath == "" {
		if len(k.manifest.App.Activities) > 0 {
			bannerPath, err = k.drawablePath(k.manifest.App.Activities[0].Banner, resConfig)
		}
	}
	if err != nil || bannerPath == "" || iconPath == "" {
		return false
	}
	if iconPath == bannerPath {
		return true
	}
	return sameZipFileContent(k, iconPath, bannerPath)
}

func sameZipFileContent(k *Apk, a, b string) bool {
	da, err := k.readZipFile(a)
	if err != nil {
		return false
	}
	db, err := k.readZipFile(b)
	if err != nil {
		return false
	}
	if len(da) != len(db) {
		return false
	}
	if bytes.Equal(da, db) {
		return true
	}
	ha := sha256.Sum256(da)
	hb := sha256.Sum256(db)
	return ha == hb
}
