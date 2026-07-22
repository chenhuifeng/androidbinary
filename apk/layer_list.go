package apk

import (
	"image"
	"image/draw"
	"regexp"
	"strings"

	"github.com/chenhuifeng/androidbinary/v2"
	xdraw "golang.org/x/image/draw"
)

var (
	layerItemDrawableRe = regexp.MustCompile(`<item[^>]*drawable="([^"]+)"`)
	bitmapSrcRe         = regexp.MustCompile(`<bitmap[^>]*src="([^"]+)"`)
)

// loadLayerListDrawable loads a <layer-list> by resolving each item's drawable and compositing.
func (k *Apk) loadLayerListDrawable(content string, resConfig *androidbinary.ResTableConfig, target renderSize) (image.Image, string, error) {
	refs := layerItemDrawableRe.FindAllStringSubmatch(content, -1)
	if len(refs) == 0 {
		return nil, "", newError("layer-list has no item drawables")
	}

	var layers []image.Image
	for _, m := range refs {
		path, err := k.resolveResPath(m[1], resConfig)
		if err != nil {
			continue
		}
		img, _, err := k.loadDrawable(path, resConfig, target)
		if err != nil || img == nil {
			continue
		}
		layers = append(layers, img)
	}
	if len(layers) == 0 {
		return nil, "", newError("layer-list: no drawable layers loaded")
	}
	if len(layers) == 1 {
		return layers[0], "", nil
	}
	return compositeLayers(layers), "", nil
}

// loadBitmapDrawable loads a <bitmap android:src="..."/> wrapper.
func (k *Apk) loadBitmapDrawable(content string, resConfig *androidbinary.ResTableConfig, target renderSize) (image.Image, string, error) {
	m := bitmapSrcRe.FindStringSubmatch(content)
	if len(m) < 2 {
		return nil, "", newError("bitmap drawable has no src")
	}
	path, err := k.resolveResPath(m[1], resConfig)
	if err != nil {
		return nil, "", err
	}
	return k.loadDrawable(path, resConfig, target)
}

func compositeLayers(layers []image.Image) image.Image {
	w, h := 0, 0
	for _, layer := range layers {
		b := layer.Bounds()
		if b.Dx() > w {
			w = b.Dx()
		}
		if b.Dy() > h {
			h = b.Dy()
		}
	}
	if w == 0 || h == 0 {
		return layers[0]
	}

	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	for _, layer := range layers {
		b := layer.Bounds()
		if b.Dx() == w && b.Dy() == h {
			draw.Draw(canvas, canvas.Bounds(), layer, b.Min, draw.Over)
			continue
		}
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), layer, b, draw.Over, nil)
		draw.Draw(canvas, canvas.Bounds(), scaled, image.Point{}, draw.Over)
	}
	return canvas
}

func isLayerList(content string) bool {
	return strings.Contains(content, "<layer-list")
}

func isBitmapDrawable(content string) bool {
	return strings.Contains(content, "<bitmap")
}
