# androidbinary

[![Build Status](https://github.com/chenhuifeng/androidbinary/workflows/Test/badge.svg)](https://github.com/chenhuifeng/androidbinary/actions)
[![GoDoc](https://pkg.go.dev/badge/github.com/chenhuifeng/androidbinary/v2)](https://pkg.go.dev/github.com/chenhuifeng/androidbinary/v2)

Go library for parsing Android binary formats: compiled XML (`AndroidManifest.xml`), `resources.arsc`, and high-level APK metadata.

## Features

- Parse binary XML and resource tables (`resources.arsc`)
- Open APK, zip bundles (e.g. `disney.zip` with embedded `base.apk`), and XAPK (e.g. `Emby.xapk`)
- Extract **app icon** (`android:icon` / `ic_launcher`) and **TV banner** (`android:banner`) as raster images
- Prefer mipmap PNG/WebP over adaptive-icon XML; rasterize vector/adaptive icons when needed
- CLI tool: [`apk/cmd/extract-icons`](apk/README.md#extract-icons)

## Install

```bash
go get github.com/chenhuifeng/androidbinary/v2@latest
```

Requires Go 1.17+.

## Quick start — APK

```go
package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/chenhuifeng/androidbinary/v2/apk"
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

### v2.0.2

- Fix Go module path: `github.com/chenhuifeng/androidbinary/v2` (required for `go get` v2.x)

### v2.0.1

- APK icon/banner extraction, zip/xapk support, `extract-icons` CLI (broken `go get` — module path lacked `/v2`; use v2.0.2)

### Earlier releases

See [git tags](https://github.com/chenhuifeng/androidbinary/tags).

## Development

```bash
go test ./...
cd apk && go test -v ./...
```

APK integration tests need local files under `apk/testdata/` (not committed). See [apk/testdata/README.md](apk/testdata/README.md).

```bash
go run ./cmd/extract-icons -apk testdata/base.apk -o ./out
```

## License

MIT — see [LICENSE](LICENSE).
