package apk

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
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
	f         *os.File
	zipreader *zip.Reader
	manifest  Manifest
	table     *androidbinary.TableFile
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

	return k.loadDrawable(bannerPath, resConfig)
}

// Icon returns the app icon (android:icon only) as a raster image.
// Prefers mipmap PNG/WebP over adaptive-icon XML; falls back to vector rasterization when needed.
// If the app has no icon, returns nil image and nil error.
func (k *Apk) Icon(resConfig *androidbinary.ResTableConfig) (image.Image, string, error) {
	iconPath, err := k.drawablePath(k.manifest.App.Icon, resConfig)
	if err != nil {
		return nil, "", err
	}

	if iconPath == "" {
		if len(k.manifest.App.Activities) > 0 {
			iconPath, err = k.drawablePath(k.manifest.App.Activities[0].Icon, resConfig)
			if err != nil {
				return nil, "", err
			}
		}
	}

	if iconPath == "" {
		return nil, "", nil
	}

	if androidbinary.IsResID(iconPath) {
		return nil, "", newError("unable to convert icon-id to icon path")
	}
	return k.loadDrawable(iconPath, resConfig)
}

func (k *Apk) drawablePath(attr androidbinary.String, resConfig *androidbinary.ResTableConfig) (string, error) {
	attr = attr.WithResTableConfig(resConfig)
	ref := attr.Ref()
	if androidbinary.IsResID(ref) {
		id, err := androidbinary.ParseResID(ref)
		if err != nil {
			return "", err
		}
		if path, err := k.table.GetResourcePathPreferRaster(id, resConfig); err == nil {
			return path, nil
		}
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
	if path, err := k.table.GetResourcePathPreferRaster(id, resConfig); err == nil {
		return path, nil
	}
	value, err := k.table.GetResource(id, resConfig)
	if err != nil {
		return "", err
	}
	path, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("apk: resource %s is not a file path (type %T)", resRef, value)
	}
	return path, nil
}

func (k *Apk) loadDrawable(path string, resConfig *androidbinary.ResTableConfig) (image.Image, string, error) {
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
		foreground := regexp.MustCompile(`foreground[^>]*drawable="([^"]+)"`).FindStringSubmatch(content)
		if len(foreground) < 2 {
			return nil, "", newError("adaptive-icon has no foreground drawable")
		}
		foregroundPath, err := k.resolveResPath(foreground[1], resConfig)
		if err != nil {
			return nil, "", err
		}
		return k.loadDrawable(foregroundPath, resConfig)
	}

	fmt.Println("convert xml to svg")
	svg, err := k.ConvertXMLToSVG(content)
	if err != nil {
		return nil, "", err
	}
	if svg == "" {
		return nil, "", newError("unable to convert drawable xml to svg")
	}
	fmt.Println("rasterize svg")
	img, err := rasterizeSVG(svg)
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
	buf := bytes.NewBuffer(nil)
	for _, file := range k.zipreader.File {
		if file.Name != name {
			continue
		}
		rc, er := file.Open()
		if er != nil {
			err = er
			return
		}
		defer rc.Close()
		_, err = io.Copy(buf, rc)
		if err != nil {
			return
		}
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("apk: file %q not found", name)
}

func (k *Apk) ConvertXMLToSVG(xmlContent string) (string, error) {
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
	matchPathLines := regexp.MustCompile(`(<path.*?</path>)`).FindAllString(xmlContent, -1)

	for _, line := range matchPathLines {
		fill := "#ffffff"
		matchFillColor := regexp.MustCompile(`android:fillColor="(.*?)"`).FindStringSubmatch(line)
		if len(matchFillColor) > 1 {
			if len(matchFillColor[1]) > 7 && strings.HasPrefix(matchFillColor[1], "@0xFF") {
				fill = "#" + strings.TrimPrefix(matchFillColor[1], "@0xFF")
			}
		}

		pathData := ""
		matchPathData := regexp.MustCompile(`android:pathData="(.*?)"`).FindStringSubmatch(line)
		if len(matchPathData) > 1 {
			pathData = matchPathData[1]
		}
		pathBlock += fmt.Sprintf("<path fill=\"%s\" d=\"%s\" ", fill, pathData)

		strokeOpacity := `` // stroke-opacity="0.0"
		matchStrokeAlpha := regexp.MustCompile(`android:strokeAlpha="@(.*?)"`).FindStringSubmatch(line)
		if len(matchStrokeAlpha) > 1 {
			strokeOpacity = fmt.Sprintf(`stroke-opacity="%.1f"`, hexToFloat32(matchStrokeAlpha[1]))
		}

		if strokeOpacity != "" {
			pathBlock += " " + strokeOpacity + " "
		}

		fillOpacity := `` // fill-opacity="0.0"
		matchFillOpacity := regexp.MustCompile(`android:fillAlpha="@(.*?)"`).FindStringSubmatch(line)
		if len(matchFillOpacity) > 1 {
			fillOpacity = fmt.Sprintf(`fill-opacity="%.1f"`, hexToFloat32(matchFillOpacity[1]))
		}
		if fillOpacity != "" {
			pathBlock += " " + fillOpacity + " "
		}

		strokeWidth := `` // stroke-width="1.0"
		matchStrokeWidth := regexp.MustCompile(`android:strokeWidth="@(.*?)"`).FindStringSubmatch(line)
		if len(matchStrokeWidth) > 1 {
			strokeWidth = fmt.Sprintf(`stroke-width="%.1f"`, hexToFloat32(matchStrokeWidth[1]))
		}
		if strokeWidth != "" {
			pathBlock += " " + strokeWidth + " "
		}

		stroke := `` // stroke="#000000ff"
		matchStrokeColor := regexp.MustCompile(`android:strokeColor="@(.*?)"`).FindStringSubmatch(line)
		if len(matchStrokeColor) > 1 {
			stroke = fmt.Sprintf(`stroke="%s"`, "#"+strings.TrimPrefix(matchStrokeColor[1], "0xFF"))
		}
		if stroke != "" {
			pathBlock += " " + stroke + " "
		}
		pathBlock += " />\n"
	}

	svgContent := ""
	svgContent += fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %s %s\" >\n", viewBoxWidth, viewBoxHeight)
	svgContent += fmt.Sprintf(pathBlock)
	svgContent += fmt.Sprintf("</svg>")

	return svgContent, nil
}

func hexToFloat32(hexStr string) float32 {
	// trim the prefix "0x"
	hexStr = hexStr[2:]
	// Converts hex string to []bytes
	hBytes, _ := hex.DecodeString(hexStr)
	// convert []bytes to float32
	bits := uint32(hBytes[0])<<24 | uint32(hBytes[1])<<16 | uint32(hBytes[2])<<8 | uint32(hBytes[3])
	return math.Float32frombits(bits)
}
