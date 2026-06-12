package apk

import (
	"image"
	"image/draw"
	"regexp"

	"github.com/chenhuifeng/androidbinary/v2"
	xdraw "golang.org/x/image/draw"
)

var (
	adaptiveBackgroundRe = regexp.MustCompile(`background[^>]*drawable="([^"]+)"`)
	adaptiveForegroundRe = regexp.MustCompile(`foreground[^>]*drawable="([^"]+)"`)
)

func (k *Apk) loadAdaptiveIcon(content string, resConfig *androidbinary.ResTableConfig, target renderSize) (image.Image, string, error) {
	if !target.valid() {
		target = sizeIcon
	}
	w, h := target.width, target.height
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))

	if m := adaptiveBackgroundRe.FindStringSubmatch(content); len(m) >= 2 {
		bgPath, err := k.resolveResPath(m[1], resConfig)
		if err == nil {
			bg, _, err := k.loadDrawable(bgPath, resConfig, target)
			if err == nil && bg != nil {
				draw.Draw(canvas, canvas.Bounds(), fitImageRect(bg, w, h), image.Point{}, draw.Over)
			}
		}
	}

	m := adaptiveForegroundRe.FindStringSubmatch(content)
	if len(m) < 2 {
		return nil, "", newError("adaptive-icon has no foreground drawable")
	}
	fgPath, err := k.resolveResPath(m[1], resConfig)
	if err != nil {
		return nil, "", err
	}
	fg, _, err := k.loadDrawable(fgPath, resConfig, target)
	if err != nil {
		return nil, "", err
	}
	if fg != nil {
		draw.Draw(canvas, canvas.Bounds(), fitImageRect(fg, w, h), image.Point{}, draw.Over)
	}
	return canvas, "", nil
}

func fitImageRect(src image.Image, w, h int) image.Image {
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}
