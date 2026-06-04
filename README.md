# androidbinary

[![Build Status](https://github.com/chenhuifeng/androidbinary/workflows/Test/badge.svg)](https://github.com/chenhuifeng/androidbinary/actions)
[![GoDoc](https://pkg.go.dev/badge/github.com/chenhuifeng/androidbinary)](https://pkg.go.dev/github.com/chenhuifeng/androidbinary)

Go library for parsing Android binary formats: compiled XML (`AndroidManifest.xml`), `resources.arsc`, and high-level APK metadata.

## Features

- Parse binary XML and resource tables (`resources.arsc`)
- Open APK, zip bundles (e.g. `disney.zip` with embedded `base.apk`), and XAPK (e.g. `Emby.xapk`)
- Extract **app icon** (`android:icon` / `ic_launcher`) and **TV banner** (`android:banner`) as raster images
- Prefer mipmap PNG/WebP over adaptive-icon XML; rasterize vector/adaptive icons when needed
- CLI tool: [`apk/cmd/extract-icons`](apk/README.md#extract-icons)

## Install

```bash
go get github.com/chenhuifeng/androidbinary@latest
```

Requires Go 1.17+.

## Quick start — APK

```go
package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/chenhuifeng/androidbinary/apk"
)

func main() {
	pkg, err := apk.OpenFile("app.apk") // also: disney.zip, Emby.xapk
	if err != nil {
		panic(err)
	}
	defer pkg.Close()

	fmt.Println("package:", pkg.PackageName())
	fmt.Println("version:", pkg.VersionName())

	icon, _, err := pkg.Icon(nil)
	if err != nil {
		panic(err)
	}
	if icon != nil {
		f, _ := os.Create("ic_launcher.png")
		defer f.Close()
		png.Encode(f, icon)
	}

	banner, _, err := pkg.Banner(nil)
	if err != nil {
		panic(err)
	}
	if banner != nil {
		f, _ := os.Create("banner.png")
		defer f.Close()
		png.Encode(f, banner)
	}
}
```

See [apk/README.md](apk/README.md) for icon/banner behavior, supported containers, and the `extract-icons` command.

## High-level API

| Method | Description |
|--------|-------------|
| `OpenFile(path)` | Open `.apk`, zip with embedded APK, or `.xapk` |
| `Icon(config)` | App icon as `image.Image` (nil if not set) |
| `Banner(config)` | TV banner only (nil if not set) |
| `Label(config)` | Localized app label |
| `PackageName()` | Package name from manifest |
| `VersionName()` / `VersionCode()` | Version info |
| `MainActivity()` | Launcher activity name |
| `Manifest()` | Parsed manifest struct |

## Low-level API

### Binary XML

```go
f, _ := os.Open("AndroidManifest.xml")
xf, _ := androidbinary.NewXMLFile(f)
data, _ := io.ReadAll(xf.Reader())
// use encoding/xml on data
```

### Resource table

```go
f, _ := os.Open("resources.arsc")
tf, _ := androidbinary.NewTableFile(f)
value, _ := tf.GetResource(resID, nil)
```

## Changelog

### v2.0.1

- APK icon/banner extraction with adaptive-icon and vector drawable support
- `GetResourcePathPreferRaster` — prefer mipmap PNG/WebP over `mipmap-anydpi-v26` XML
- `OpenFile` supports zip containers and XAPK (selects base APK with `resources.arsc`)
- `Icon` / `Banner` only resolve their own manifest attributes (no cross-fallback)
- New `extract-icons` CLI under `apk/cmd/extract-icons`
- Dependencies: `oksvg`, `rasterx`, `webp`

### Earlier releases

See [git tags](https://github.com/chenhuifeng/androidbinary/tags).

## Development

```bash
go test ./...
cd apk && go test -v ./...
go run ./cmd/extract-icons -apk testdata/base.apk -o ./out
```

## License

MIT — see [LICENSE](LICENSE).
