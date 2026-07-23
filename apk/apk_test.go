package apk

import (
	"os"
	_ "image/jpeg"
	_ "image/png"
	"testing"
)

func requireFixture(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture %s not found — add test APKs under apk/testdata/ (see README.md)", path)
	}
}

func TestOpenFileSplitXapk(t *testing.T) {
	const path = "testdata/com.jakob.speedtest-1.25.08.13.1.xapk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile speedtest xapk: %v", err)
	}
	defer apk.Close()

	if apk.PackageName() != "com.jakob.speedtest" {
		t.Errorf("PackageName: got %s", apk.PackageName())
	}

	icon, _, err := apk.Icon(nil)
	if err != nil || icon == nil {
		t.Fatalf("Icon from speedtest xapk: err=%v icon=%v", err, icon)
	}
	b := icon.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("Icon has empty bounds: %v", b)
	}
}

func TestOpenFileXapk(t *testing.T) {
	const path = "testdata/Emby.xapk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile Emby.xapk: %v", err)
	}
	defer apk.Close()

	if apk.PackageName() != "com.mb.android" {
		t.Errorf("PackageName: got %s", apk.PackageName())
	}

	icon, _, err := apk.Icon(nil)
	if err != nil || icon == nil {
		t.Fatalf("Icon from Emby.xapk: err=%v icon=%v", err, icon)
	}
}

func TestOpenFileChromeManifestNoNamespaceChunks(t *testing.T) {
	const path = "testdata/Chrome_135.0.7049.113.apk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile Chrome: %v", err)
	}
	defer apk.Close()

	if apk.PackageName() != "com.android.chrome" {
		t.Errorf("PackageName: got %s", apk.PackageName())
	}
	if apk.VersionName() == "" {
		t.Error("VersionName is empty")
	}
}

func TestOpenFileTVIconSameAsBannerNoCrop(t *testing.T) {
	const path = "testdata/de.sky.online-5.8.1-AndroidTV-DE.apk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile Sky TV: %v", err)
	}
	defer apk.Close()

	icon, _, err := apk.Icon(nil)
	if err != nil || icon == nil {
		t.Fatalf("Icon: err=%v", err)
	}
	banner, _, err := apk.Banner(nil)
	if err != nil || banner == nil {
		t.Fatalf("Banner: err=%v", err)
	}

	ib, bb := icon.Bounds(), banner.Bounds()
	// Same wide asset: do not center-crop the icon.
	if ib.Dx() != bb.Dx() || ib.Dy() != bb.Dy() {
		t.Errorf("Icon should keep banner size when same asset, icon=%dx%d banner=%dx%d",
			ib.Dx(), ib.Dy(), bb.Dx(), bb.Dy())
	}
	if ib.Dx() <= ib.Dy() {
		t.Errorf("Icon should remain wide (not cropped), got %dx%d", ib.Dx(), ib.Dy())
	}
}

func TestOpenFileSquareIconNotBanner(t *testing.T) {
	const path = "testdata/com.digiturk.iq.mobil-1.25.1.apk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile Digitürk: %v", err)
	}
	defer apk.Close()

	icon, _, err := apk.Icon(nil)
	if err != nil || icon == nil {
		t.Fatalf("Icon: err=%v", err)
	}
	ib := icon.Bounds()
	if ib.Dx() != ib.Dy() {
		t.Errorf("Icon should be square, got %dx%d", ib.Dx(), ib.Dy())
	}

	banner, _, err := apk.Banner(nil)
	if err != nil || banner == nil {
		t.Fatalf("Banner: err=%v", err)
	}
	bb := banner.Bounds()
	if bb.Dx() == bb.Dy() {
		t.Errorf("Banner should be wide, got %dx%d", bb.Dx(), bb.Dy())
	}
	if ib.Dx() == bb.Dx() && ib.Dy() == bb.Dy() {
		t.Fatal("Icon and Banner must not be the same image size")
	}
}

func TestOpenFileLayerListBanner(t *testing.T) {
	const path = "testdata/com.skyshowtime.skyshowtime.google-1.9.96.apk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile Showtime: %v", err)
	}
	defer apk.Close()

	banner, _, err := apk.Banner(nil)
	if err != nil {
		t.Fatalf("Banner: %v", err)
	}
	if banner == nil {
		t.Fatal("Banner is nil")
	}
	b := banner.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Fatalf("Banner has empty bounds: %v", b)
	}
	// Must not be a fully transparent blank image.
	opaque := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := banner.At(x, y).RGBA()
			if a != 0 {
				opaque++
			}
		}
	}
	if opaque == 0 {
		t.Fatal("Banner is fully transparent (blank)")
	}
}

func TestOpenFileVectorBanner(t *testing.T) {
	const path = "testdata/LiveUltra-v573.apk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile LiveUltra: %v", err)
	}
	defer apk.Close()

	icon, _, err := apk.Icon(nil)
	if err != nil || icon == nil {
		t.Fatalf("Icon: err=%v", err)
	}

	banner, _, err := apk.Banner(nil)
	if err != nil {
		t.Fatalf("Banner: %v", err)
	}
	if banner == nil {
		t.Fatal("Banner is nil")
	}
	b := banner.Bounds()
	if b.Dx() != 320 || b.Dy() != 180 {
		t.Errorf("Banner size want 320x180, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestOpenFileZipContainer(t *testing.T) {
	const path = "testdata/disney.zip"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile disney.zip: %v", err)
	}
	defer apk.Close()

	if apk.PackageName() != "com.disney.disneyplus" {
		t.Errorf("PackageName: got %s", apk.PackageName())
	}

	icon, _, err := apk.Icon(nil)
	if err != nil || icon == nil {
		t.Fatalf("Icon from disney.zip: err=%v icon=%v", err, icon)
	}
}

func TestParseAPKFile(t *testing.T) {
	const path = "testdata/base.apk"
	requireFixture(t, path)

	apk, err := OpenFile(path)
	if err != nil {
		t.Errorf("OpenFile error: %v", err)
	}
	defer apk.Close()

	icon, _, err := apk.Icon(nil)
	if err != nil {
		t.Errorf("Icon error: %v", err)
	}
	if icon == nil {
		t.Fatal("Icon is nil")
	}
	b := icon.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("Icon has empty bounds: %v", b)
	}

	banner, bannerSVG, err := apk.Banner(nil)
	if err != nil {
		t.Errorf("Banner error: %v", err)
	}
	if banner == nil && bannerSVG == "" {
		t.Error("Banner is nil")
	}

	label, err := apk.Label(nil)
	if err != nil {
		t.Errorf("Label error: %v", err)
	}
	if label != "Disney+" {
		t.Errorf("Label is not Disney+: %s", label)
	}
	t.Log("app label:", label)

	if apk.PackageName() != "com.disney.disneyplus" {
		t.Errorf("PackageName is not com.disney.disneyplus: %s", apk.PackageName())
	}

	if apk.VersionCode() != 24071200 {
		t.Errorf("VersionCode is not 24071200: %d", apk.VersionCode())
	}

	if apk.VersionName() != "3.5.0-rc4" {
		t.Errorf("VersionName is not 3.5.0-rc4: %s", apk.VersionName())
	}

	manifest := apk.Manifest()
	if manifest.SDK.Target.MustInt32() != int32(34) {
		t.Errorf("SDK target is not 34: %d", manifest.SDK.Target.MustInt32())
	}

	mainActivity, err := apk.MainActivity()
	if err != nil {
		t.Errorf("MainActivity error: %v", err)
	}
	if mainActivity != "com.bamtechmedia.dominguez.main.MainActivity" {
		t.Errorf("MainActivity is not com.bamtechmedia.dominguez.main.MainActivity: %s", mainActivity)
	}
}
