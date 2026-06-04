# Local test fixtures (not in git)

Place APK / zip / xapk files here for `go test` and `extract-icons`. These binaries are **not** committed to the repository.

## Files used by tests

| File | Used by |
|------|---------|
| `base.apk` | `TestParseAPKFile` (Disney+) |
| `disney.zip` | `TestOpenFileZipContainer` |
| `Emby.xapk` | `TestOpenFileXapk` |
| `helloworld.apk` | optional manual / local checks |
| `amazon.apk` | optional manual / local checks |

Tests **skip** automatically when a required file is missing.

## Example

```bash
# copy your own samples into this directory, then:
cd apk
go test -v -run TestParseAPKFile
go run ./cmd/extract-icons -apk testdata/base.apk -o ./out
```
