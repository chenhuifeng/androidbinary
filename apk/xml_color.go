package apk

import (
	"bytes"
	"fmt"
	"image/color"
	"io/ioutil"
	"regexp"
	"strings"

	androidbinary "github.com/chenhuifeng/androidbinary/v2"
)

var (
	gradientItemRe = regexp.MustCompile(`<item[^>]*android:color="([^"]+)"[^>]*android:offset="([^"]+)"`)
	gradientItemRe2 = regexp.MustCompile(`<item[^>]*android:offset="([^"]+)"[^>]*android:color="([^"]+)"`)
)

type svgPaint struct {
	attr string // e.g. fill="url(#g0)" or stroke="none"
	defs string // <linearGradient>...</linearGradient>
}

func (k *Apk) resolveSVGPaint(ref string, config *androidbinary.ResTableConfig, attr string) svgPaint {
	if ref == "" {
		return svgPaint{}
	}

	if androidbinary.IsResID(ref) {
		id, err := androidbinary.ParseResID(ref)
		if err == nil && id.Package() == 0x7f {
			value, err := k.getResource(id, config)
			if err == nil {
				if path, ok := value.(string); ok && strings.HasSuffix(path, ".xml") {
					if paint, ok := k.gradientPaintFromXML(path, config, attr); ok {
						return paint
					}
				}
				if c, ok := colorFromResourceValue(value); ok {
					return solidPaint(attr, c)
				}
			}
		}
	}

	if c, ok := parseColorRef(ref); ok {
		return solidPaint(attr, c)
	}
	return svgPaint{}
}

func solidPaint(attr string, c color.RGBA) svgPaint {
	if c.A == 0 {
		return svgPaint{attr: fmt.Sprintf(`%s="none"`, attr)}
	}
	p := svgPaint{attr: fmt.Sprintf(`%s="#%02X%02X%02X"`, attr, c.R, c.G, c.B)}
	if c.A < 255 {
		p.attr += fmt.Sprintf(` %s-opacity="%.4f"`, attr, float64(c.A)/255)
	}
	return p
}

func colorFromResourceValue(value interface{}) (color.RGBA, bool) {
	switch v := value.(type) {
	case uint32:
		return uint32ToRGBA(v), true
	case int32:
		return uint32ToRGBA(uint32(v)), true
	case int:
		return uint32ToRGBA(uint32(v)), true
	}
	return color.RGBA{}, false
}

func uint32ToRGBA(v uint32) color.RGBA {
	return color.RGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: uint8(v >> 24),
	}
}

func (k *Apk) gradientPaintFromXML(path string, config *androidbinary.ResTableConfig, attr string) (svgPaint, bool) {
	data, err := k.readZipFile(path)
	if err != nil {
		return svgPaint{}, false
	}
	xmlFile, err := androidbinary.NewXMLFile(bytes.NewReader(data))
	if err != nil {
		return svgPaint{}, false
	}
	content, err := ioutil.ReadAll(xmlFile.Reader())
	if err != nil {
		return svgPaint{}, false
	}
	xml := string(content)
	if !strings.Contains(xml, "<gradient") {
		return svgPaint{}, false
	}

	stops := parseGradientStops(xml)
	if len(stops) == 0 {
		return svgPaint{}, false
	}

	gradID := fmt.Sprintf("grad_%s", strings.TrimSuffix(filepathBase(path), ".xml"))
	x1, y1, x2, y2 := parseGradientEndpoints(xml)
	defs := fmt.Sprintf(`<linearGradient id="%s" gradientUnits="userSpaceOnUse" x1="%g" y1="%g" x2="%g" y2="%g">`, gradID, x1, y1, x2, y2)
	for _, s := range stops {
		c, ok := parseColorRef(s.color)
		if !ok {
			continue
		}
		defs += fmt.Sprintf(`<stop offset="%.4f" stop-color="#%02X%02X%02X"/>`, s.offset, c.R, c.G, c.B)
	}
	defs += `</linearGradient>`

	return svgPaint{
		attr: fmt.Sprintf(`%s="url(#%s)"`, attr, gradID),
		defs: defs,
	}, true
}

type gradientStop struct {
	color  string
	offset float64
}

func parseGradientStops(xml string) []gradientStop {
	var stops []gradientStop
	for _, m := range gradientItemRe.FindAllStringSubmatch(xml, -1) {
		stops = append(stops, gradientStop{color: m[1], offset: gradientOffset(m[2])})
	}
	if len(stops) > 0 {
		return stops
	}
	for _, m := range gradientItemRe2.FindAllStringSubmatch(xml, -1) {
		stops = append(stops, gradientStop{color: m[2], offset: gradientOffset(m[1])})
	}
	return stops
}

func gradientOffset(ref string) float64 {
	if strings.Contains(ref, "0x") {
		return float64(parseHexFloat(ref))
	}
	return 0
}

func parseGradientEndpoints(xml string) (x1, y1, x2, y2 float64) {
	x1 = attrFloat(xml, "startX")
	y1 = attrFloat(xml, "startY")
	x2 = attrFloat(xml, "endX")
	y2 = attrFloat(xml, "endY")
	if x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0 {
		return 0, 0, 100, 0
	}
	return x1, y1, x2, y2
}

func attrFloat(xml, name string) float64 {
	re := regexp.MustCompile(name + `="([^"]+)"`)
	m := re.FindStringSubmatch(xml)
	if len(m) < 2 {
		return 0
	}
	if strings.Contains(m[1], "0x") {
		return float64(parseHexFloat(m[1]))
	}
	return 0
}

func filepathBase(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
