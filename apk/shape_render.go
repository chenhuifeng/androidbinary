package apk

import (
	"image"
	"image/color"
	"math"
	"regexp"
	"strings"
)

var (
	gradientStartColorRe = regexp.MustCompile(`startColor="([^"]+)"`)
	gradientEndColorRe   = regexp.MustCompile(`endColor="([^"]+)"`)
	gradientAngleRe      = regexp.MustCompile(`angle="([^"]+)"`)
)

func renderGradientShape(xmlContent string, width, height int) (image.Image, error) {
	start, ok1 := parseXMLColor(gradientStartColorRe, xmlContent)
	end, ok2 := parseXMLColor(gradientEndColorRe, xmlContent)
	if !ok1 || !ok2 {
		return nil, newError("gradient shape missing start/end color")
	}

	angle := 0.0
	if m := gradientAngleRe.FindStringSubmatch(xmlContent); len(m) > 1 {
		angle = float64(parseHexFloat(m[1]))
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	x0, y0, x1, y1 := gradientLine(width, height, angle)
	dx, dy := x1-x0, y1-y0
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		fillSolid(img, start)
		return img, nil
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			t := ((float64(x)-x0)*dx + (float64(y)-y0)*dy) / len2
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			img.SetRGBA(x, y, lerpRGBA(start, end, t))
		}
	}
	return img, nil
}

// gradientLine returns start/end points for Android GradientDrawable angle (degrees).
func gradientLine(w, h int, angle float64) (x0, y0, x1, y1 float64) {
	rad := (angle - 90) * math.Pi / 180
	cos := math.Cos(rad)
	sin := math.Sin(rad)
	cx, cy := float64(w)/2, float64(h)/2
	diag := math.Sqrt(float64(w*w+h*h)) / 2
	return cx - cos*diag, cy - sin*diag, cx + cos*diag, cy + sin*diag
}

func parseXMLColor(re *regexp.Regexp, xml string) (color.RGBA, bool) {
	m := re.FindStringSubmatch(xml)
	if len(m) < 2 {
		return color.RGBA{}, false
	}
	return parseColorRef(m[1])
}

func parseColorRef(ref string) (color.RGBA, bool) {
	if !strings.HasPrefix(ref, "@0x") {
		return color.RGBA{}, false
	}
	hex := strings.TrimPrefix(ref, "@0x")
	if len(hex) == 8 {
		var r, g, b, a uint8
		fmtSscanf(hex, &a, &r, &g, &b)
		return color.RGBA{R: r, G: g, B: b, A: a}, true
	}
	if len(hex) == 6 {
		var r, g, b uint8
		fmtSscanf(hex, &r, &g, &b)
		return color.RGBA{R: r, G: g, B: b, A: 255}, true
	}
	if strings.HasPrefix(ref, "@0xFF") && len(ref) >= 10 {
		rgb := strings.TrimPrefix(ref, "@0xFF")
		var r, g, b uint8
		fmtSscanf(rgb, &r, &g, &b)
		return color.RGBA{R: r, G: g, B: b, A: 255}, true
	}
	return color.RGBA{}, false
}

func fmtSscanf(hex string, vals ...*uint8) {
	for i := 0; i < len(vals) && i*2+2 <= len(hex); i++ {
		var v uint8
		for _, c := range hex[i*2 : i*2+2] {
			v <<= 4
			if c >= '0' && c <= '9' {
				v |= uint8(c - '0')
			} else if c >= 'a' && c <= 'f' {
				v |= uint8(c - 'a' + 10)
			} else if c >= 'A' && c <= 'F' {
				v |= uint8(c - 'A' + 10)
			}
		}
		*vals[i] = v
	}
}

func lerpRGBA(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return color.RGBA{
		R: lerp(a.R, b.R),
		G: lerp(a.G, b.G),
		B: lerp(a.B, b.B),
		A: lerp(a.A, b.A),
	}
}

func fillSolid(img *image.RGBA, c color.RGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}
