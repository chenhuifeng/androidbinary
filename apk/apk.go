package apk

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/chenhuifeng/androidbinary/v2"

	"github.com/chai2010/webp"
	_ "image/jpeg" // handle jpeg format
	_ "image/png"  // handle png format
)

// Apk is an application package file for android.
type Apk struct {
	f            *os.File
	zipreader    *zip.Reader
	splits       []splitApk
	containerZip *zip.Reader
	xapkIcon     string
	manifest     Manifest
	table        *androidbinary.TableFile
}

// OpenFile opens an APK file or a zip/xapk container (e.g. disney.zip, Emby.xapk) that embeds a base APK.
func OpenFile(filename string) (apk *Apk, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	apk, err = openFromReaderAt(f, fi.Size())
	if err != nil {
		return nil, err
	}
	apk.f = f
	return
}

// OpenZipReader has same arguments like zip.NewReader.
// The reader must point at an APK zip (with AndroidManifest.xml at root), not an outer container zip.
func OpenZipReader(r io.ReaderAt, size int64) (*Apk, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	if !zipHasFile(zr, "AndroidManifest.xml") {
		return nil, newError("not an APK archive: AndroidManifest.xml missing")
	}
	return newApkFromZip(zr)
}

// Close is avaliable only if apk is created with OpenFile
func (k *Apk) Close() error {
	if k.f == nil {
		return nil
	}
	return k.f.Close()
}

// Banner returns the TV banner image of the APK (android:banner only).
// If the app has no banner, returns nil image and nil error.
func (k *Apk) Banner(resConfig *androidbinary.ResTableConfig) (image.Image, string, error) {
	bannerPath, err := k.drawablePath(k.manifest.App.Banner, resConfig)
	if err != nil {
		return nil, "", err
	}

	if bannerPath == "" {
		if len(k.manifest.App.Activities) > 0 {
			bannerPath, err = k.drawablePath(k.manifest.App.Activities[0].Banner, resConfig)
			if err != nil {
				return nil, "", err
			}
		}
	}

	if bannerPath == "" {
		return nil, "", nil
	}

	if androidbinary.IsResID(bannerPath) {
		return nil, "", newError("unable to convert banner-id to banner path")
	}

	return k.loadDrawable(bannerPath, resConfig, sizeBanner)
}

// Icon returns the app icon (android:icon only) as a raster image.
// Prefers mipmap PNG/WebP over adaptive-icon XML; falls back to vector rasterization when needed.
// If the app has no icon, returns nil image and nil error.
func (k *Apk) Icon(resConfig *androidbinary.ResTableConfig) (image.Image, string, error) {
	iconPath, err := k.drawablePath(k.manifest.App.Icon, resConfig)
	if err != nil {
		if img, ierr := k.loadXapkIcon(); ierr == nil && img != nil {
			return img, "", nil
		}
		return nil, "", err
	}

	if iconPath == "" {
		if len(k.manifest.App.Activities) > 0 {
			iconPath, err = k.drawablePath(k.manifest.App.Activities[0].Icon, resConfig)
			if err != nil {
				if img, ierr := k.loadXapkIcon(); ierr == nil && img != nil {
					return img, "", nil
				}
				return nil, "", err
			}
		}
	}

	if iconPath == "" {
		if img, ierr := k.loadXapkIcon(); ierr == nil && img != nil {
			return img, "", nil
		}
		return nil, "", nil
	}

	if androidbinary.IsResID(iconPath) {
		return nil, "", newError("unable to convert icon-id to icon path")
	}
	img, svg, err := k.loadDrawable(iconPath, resConfig, sizeIcon)
	if err != nil || img == nil {
		if fallback, ierr := k.loadXapkIcon(); ierr == nil && fallback != nil {
			return fallback, "", nil
		}
	}
	return img, svg, err
}

func (k *Apk) drawablePath(attr androidbinary.String, resConfig *androidbinary.ResTableConfig) (string, error) {
	attr = attr.WithResTableConfig(resConfig)
	ref := attr.Ref()
	if androidbinary.IsResID(ref) {
		id, err := androidbinary.ParseResID(ref)
		if err != nil {
			return "", err
		}
		if path, err := k.getResourcePathPreferRaster(id, resConfig); err == nil {
			return path, nil
		}
		value, err := k.getResource(id, resConfig)
		if err != nil {
			return "", err
		}
		path, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("apk: resource %s is not a file path (type %T)", ref, value)
		}
		return path, nil
	}
	if ref != "" {
		return ref, nil
	}
	return attr.String()
}

func (k *Apk) resolveResPath(resRef string, resConfig *androidbinary.ResTableConfig) (string, error) {
	if !androidbinary.IsResID(resRef) {
		return resRef, nil
	}
	id, err := androidbinary.ParseResID(resRef)
	if err != nil {
		return "", err
	}
	if path, err := k.getResourcePathPreferRaster(id, resConfig); err == nil {
		return path, nil
	}
	value, err := k.getResource(id, resConfig)
	if err != nil {
		return "", err
	}
	path, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("apk: resource %s is not a file path (type %T)", resRef, value)
	}
	return path, nil
}

func (k *Apk) loadDrawable(path string, resConfig *androidbinary.ResTableConfig, target renderSize) (image.Image, string, error) {
	imgData, err := k.readZipFile(path)
	if err != nil {
		return nil, "", err
	}

	if !strings.HasSuffix(path, ".xml") {
		return k.decodeRaster(imgData)
	}

	xmlFile, err := androidbinary.NewXMLFile(bytes.NewReader(imgData))
	if err != nil {
		return nil, "", err
	}
	xmlContent, err := ioutil.ReadAll(xmlFile.Reader())
	if err != nil {
		return nil, "", err
	}
	content := string(xmlContent)

	if strings.Contains(content, "<adaptive-icon") {
		return k.loadAdaptiveIcon(content, resConfig, target)
	}

	if strings.Contains(content, "<gradient") {
		if !target.valid() {
			target = sizeIcon
		}
		img, err := renderGradientShape(content, target.width, target.height)
		return img, "", err
	}

	svg, err := k.ConvertXMLToSVG(content, resConfig)
	if err != nil {
		return nil, "", err
	}
	if svg == "" {
		return nil, "", newError("unable to convert drawable xml to svg")
	}
	if !target.valid() {
		target = sizeIcon
	}
	img, err := rasterizeSVG(svg, target)
	if err != nil {
		return nil, "", err
	}

	return img, "", nil
}

func (k *Apk) decodeRaster(imgData []byte) (image.Image, string, error) {
	m, imageType, err := image.Decode(bytes.NewReader(imgData))
	if err != nil && (imageType == "webp" || imageType == "") {
		m, err = webp.Decode(bytes.NewReader(imgData))
	}
	return m, "", err
}

// Label returns the label of the APK.
func (k *Apk) Label(resConfig *androidbinary.ResTableConfig) (s string, err error) {
	s, err = k.manifest.App.Label.WithResTableConfig(resConfig).String()
	if err != nil {
		return
	}
	if androidbinary.IsResID(s) {
		err = newError("unable to convert label-id to string")
	}
	return
}

// Manifest returns the manifest of the APK.
func (k *Apk) Manifest() Manifest {
	return k.manifest
}

// PackageName returns the package name of the APK.
func (k *Apk) PackageName() string {
	return k.manifest.Package.MustString()
}

// VersionCode returns the versionCode of the APK.
func (k *Apk) VersionCode() int32 {
	return k.manifest.VersionCode.MustInt32()
}

// VersionName returns the version name of the APK.
func (k *Apk) VersionName() string {
	return k.manifest.VersionName.MustString()
}

// Size returns the size of the APK.
func (k *Apk) Size() int64 {
	fInfo, _ := k.f.Stat()
	return fInfo.Size()
}

func isMainIntentFilter(intent ActivityIntentFilter) bool {
	ok := false
	for _, action := range intent.Actions {
		s, err := action.Name.String()
		if err == nil && s == "android.intent.action.MAIN" {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	ok = false
	for _, category := range intent.Categories {
		s, err := category.Name.String()
		if err == nil && s == "android.intent.category.LAUNCHER" {
			ok = true
			break
		}
	}
	return ok
}

// MainActivity returns the name of the main activity.
func (k *Apk) MainActivity() (activity string, err error) {
	for _, act := range k.manifest.App.Activities {
		for _, intent := range act.IntentFilters {
			if isMainIntentFilter(intent) {
				return act.Name.String()
			}
		}
	}
	for _, act := range k.manifest.App.ActivityAliases {
		for _, intent := range act.IntentFilters {
			if isMainIntentFilter(intent) {
				return act.TargetActivity.String()
			}
		}
	}

	return "", newError("No main activity found")
}

func (k *Apk) parseManifest() error {
	xmlData, err := k.readZipFile("AndroidManifest.xml")
	if err != nil {
		return errorf("failed to read AndroidManifest.xml: %w", err)
	}
	xmlfile, err := androidbinary.NewXMLFile(bytes.NewReader(xmlData))
	if err != nil {
		return errorf("failed to parse AndroidManifest.xml: %w", err)
	}
	return xmlfile.Decode(&k.manifest, k.table, nil)
}

func (k *Apk) parseResources() (err error) {
	resData, err := k.readZipFile("resources.arsc")
	if err != nil {
		return
	}
	k.table, err = androidbinary.NewTableFile(bytes.NewReader(resData))
	return
}

func (k *Apk) readZipFile(name string) (data []byte, err error) {
	for _, zr := range k.allZipReaders() {
		data, err = readZipFileFrom(zr, name)
		if err == nil {
			return data, nil
		}
	}
	if k.containerZip != nil {
		data, err = readZipFileFrom(k.containerZip, name)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("apk: file %q not found", name)
}

func (k *Apk) ConvertXMLToSVG(xmlContent string, resConfig *androidbinary.ResTableConfig) (string, error) {
	viewBoxWidth := "0"
	viewBoxHeight := "0"

	viewportWidth := regexp.MustCompile(`android:viewportWidth="@(.*?)"`).FindStringSubmatch(xmlContent)
	if len(viewportWidth) > 1 {
		viewBoxWidth = viewportWidth[1]
		viewBoxWidth = fmt.Sprintf("%d", int(hexToFloat32(viewBoxWidth)))

	}

	viewportHeight := regexp.MustCompile(`android:viewportHeight="@(.*?)"`).FindStringSubmatch(xmlContent)
	if len(viewportHeight) > 1 {
		viewBoxHeight = viewportHeight[1]
		viewBoxHeight = fmt.Sprintf("%d", int(hexToFloat32(viewBoxHeight)))
	}

	pathBlock := ""
	gradientDefs := ""
	matchPathLines := regexp.MustCompile(`(<path.*?</path>)`).FindAllString(xmlContent, -1)

	groupTransform := vectorGroupTransform(xmlContent)

	for _, line := range matchPathLines {
		fillPaint := k.resolveSVGPaint(attrValue(line, "fillColor"), resConfig, "fill")
		if fillPaint.attr == "" {
			fillPaint = solidPaint("fill", color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
		gradientDefs += fillPaint.defs

		pathData := attrValue(line, "pathData")
		pathBlock += fmt.Sprintf("<path %s d=\"%s\"", fillPaint.attr, pathData)

		if strokeOpacity := attrOpacity(line, "strokeAlpha"); strokeOpacity != "" {
			pathBlock += " " + strokeOpacity
		}
		if fillOpacity := attrOpacity(line, "fillAlpha"); fillOpacity != "" {
			pathBlock += " " + fillOpacity
		}
		if strokeWidth := attrDimension(line, "strokeWidth"); strokeWidth != "" {
			pathBlock += " " + strokeWidth
		}

		strokePaint := k.resolveSVGPaint(attrValue(line, "strokeColor"), resConfig, "stroke")
		gradientDefs += strokePaint.defs
		if strokePaint.attr != "" && !strings.Contains(strokePaint.attr, `="none"`) {
			pathBlock += " " + strokePaint.attr
		}
		pathBlock += " />\n"
	}

	svgContent := ""
	svgContent += fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %s %s\" >\n", viewBoxWidth, viewBoxHeight)
	if gradientDefs != "" {
		svgContent += "<defs>" + gradientDefs + "</defs>\n"
	}
	if groupTransform != "" {
		svgContent += fmt.Sprintf("<g transform=\"%s\">\n", groupTransform)
	}
	svgContent += pathBlock
	if groupTransform != "" {
		svgContent += "</g>\n"
	}
	svgContent += "</svg>"

	return svgContent, nil
}

func attrValue(line, name string) string {
	re := regexp.MustCompile(name + `="([^"]+)"`)
	m := re.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func attrOpacity(line, name string) string {
	v := attrValue(line, name)
	if v == "" || !strings.Contains(v, "0x") {
		return ""
	}
	opacityAttr := "fill-opacity"
	if strings.HasPrefix(name, "stroke") {
		opacityAttr = "stroke-opacity"
	}
	return fmt.Sprintf(`%s="%.4f"`, opacityAttr, float64(hexToFloat32(v)))
}

func attrDimension(line, name string) string {
	v := attrValue(line, name)
	if v == "" || !strings.Contains(v, "0x") {
		return ""
	}
	return fmt.Sprintf(`stroke-width="%.4f"`, float64(hexToFloat32(v)))
}

func vectorGroupTransform(xmlContent string) string {
	groupRe := regexp.MustCompile(`<group([^>]*)>`)
	m := groupRe.FindStringSubmatch(xmlContent)
	if len(m) < 2 {
		return ""
	}
	attrs := m[1]
	sx, sxOk := vectorAttrFloat(attrs, "scaleX")
	sy, syOk := vectorAttrFloat(attrs, "scaleY")
	tx, txOk := vectorAttrFloat(attrs, "translateX")
	ty, tyOk := vectorAttrFloat(attrs, "translateY")
	if !sxOk {
		sx = 1
	}
	if !syOk {
		sy = 1
	}
	if !txOk {
		tx = 0
	}
	if !tyOk {
		ty = 0
	}
	if sx == 1 && sy == 1 && tx == 0 && ty == 0 {
		return ""
	}
	return fmt.Sprintf("translate(%g,%g) scale(%g,%g)", tx, ty, sx, sy)
}

func vectorAttrFloat(attrs, name string) (float32, bool) {
	re := regexp.MustCompile(name + `="([^"]+)"`)
	m := re.FindStringSubmatch(attrs)
	if len(m) < 2 || !strings.Contains(m[1], "0x") {
		return 0, false
	}
	return parseHexFloat(m[1]), true
}

func parseHexFloat(ref string) float32 {
	ref = strings.TrimPrefix(ref, "@")
	if !strings.HasPrefix(ref, "0x") {
		return 0
	}
	return hexToFloat32(ref)
}

func hexToFloat32(hexStr string) float32 {
	hexStr = strings.TrimPrefix(hexStr, "@")
	if !strings.HasPrefix(hexStr, "0x") {
		return 0
	}
	hexStr = hexStr[2:]
	if len(hexStr) < 8 {
		return 0
	}
	hBytes, err := hex.DecodeString(hexStr[:8])
	if err != nil || len(hBytes) < 4 {
		return 0
	}
	bits := uint32(hBytes[0])<<24 | uint32(hBytes[1])<<16 | uint32(hBytes[2])<<8 | uint32(hBytes[3])
	return math.Float32frombits(bits)
}
