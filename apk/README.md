# apk — Android APK parser

High-level API for reading APK metadata and extracting launcher icon and TV banner images.

## Supported inputs

`apk.OpenFile` accepts:

| Input | Example | Behavior |
|-------|---------|----------|
| Standard APK | `app.apk` | Root contains `AndroidManifest.xml` |
| Zip bundle | `disney.zip` | Finds `base.apk` or largest `.apk` with `resources.arsc` |
| XAPK | `Emby.xapk` | Reads `manifest.json` base split, or skips `config.*.apk` / `split_*.apk` |

## Icon and Banner

### `Icon(resConfig)`

- Reads `android:icon` on `<application>`, then first `<activity>` if empty
- Returns `(image.Image, string, error)` — second value is reserved (empty for raster output)
- **Raster first**: walks `resources.arsc` and picks the highest-density mipmap PNG/WebP
- **Fallback**: adaptive-icon XML → foreground vector → SVG → rasterize via `oksvg`/`rasterx`
- If no icon is configured: `(nil, "", nil)` — not an error

### `Banner(resConfig)`

- Reads `android:banner` on `<application>`, then first `<activity>` if empty
- Does **not** fall back to `Icon` when banner is missing
- Same drawable loading as `Icon`
- If no banner: `(nil, "", nil)`

### Save as PNG

```go
icon, _, err := pkg.Icon(nil)
if err != nil {
	return err
}
if icon == nil {
	return fmt.Errorf("no icon")
}
f, _ := os.Create("ic_launcher.png")
defer f.Close()
return png.Encode(f, icon)
```

## extract-icons

Command-line tool to export `ic_launcher` and `banner` from an APK, zip, or xapk.

```bash
cd apk

# flags
go run ./cmd/extract-icons -apk testdata/disney.zip -o ./out

# positional APK path
go run ./cmd/extract-icons testdata/Emby.xapk

# build binary
go build -o extract-icons ./cmd/extract-icons
./extract-icons -apk testdata/base.apk -o ./out
```

Output files (prefix = APK basename without extension):

- `{name}_ic_launcher.png`
- `{name}_banner.png` (skipped if app has no banner)

## Tests

```bash
go test -v -run TestParseAPKFile      # base.apk (Disney+)
go test -v -run TestOpenFileZipContainer  # disney.zip
go test -v -run TestOpenFileXapk      # Emby.xapk
```

Test fixtures live in `testdata/` (`.apk`, `.zip`, `.xapk`). Unpacked APK trees under `testdata/*/` are gitignored.

## Example — read package info

```go
pkg, err := apk.OpenFile("testdata/base.apk")
if err != nil {
	log.Fatal(err)
}
defer pkg.Close()

log.Println(pkg.PackageName())
log.Println(pkg.VersionName(), pkg.VersionCode())
log.Println(pkg.MainActivity())
```
