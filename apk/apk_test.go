package apk

import (
	_ "image/jpeg"
	_ "image/png"
	"testing"
)

func TestOpenFileXapk(t *testing.T) {
	apk, err := OpenFile("testdata/Emby.xapk")
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

func TestOpenFileZipContainer(t *testing.T) {
	apk, err := OpenFile("testdata/disney.zip")
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
	apk, err := OpenFile("testdata/base.apk")
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
