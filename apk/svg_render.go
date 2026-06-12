package apk

import (
	"bytes"
	"fmt"
	"image"
	"regexp"
	"strconv"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

var svgViewBoxRe = regexp.MustCompile(`viewBox="0 0 (\d+) (\d+)"`)

func rasterizeSVG(svg string, target renderSize) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewBufferString(svg))
	if err != nil {
		return nil, fmt.Errorf("apk: parse svg: %w", err)
	}

	w, h := target.width, target.height
	if !target.valid() {
		w, h = svgSize(svg)
		if icon.ViewBox.W > 0 && icon.ViewBox.H > 0 {
			w = int(icon.ViewBox.W)
			h = int(icon.ViewBox.H)
		}
	}
	if w <= 0 || h <= 0 {
		w, h = sizeIcon.width, sizeIcon.height
	}

	icon.SetTarget(0, 0, float64(w), float64(h))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)
	return img, nil
}

func svgSize(svg string) (int, int) {
	m := svgViewBoxRe.FindStringSubmatch(svg)
	if len(m) != 3 {
		return 108, 108
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	if w <= 0 || h <= 0 {
		return 108, 108
	}
	return w, h
}
