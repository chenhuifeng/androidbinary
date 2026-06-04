package apk

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type xapkManifest struct {
	SplitApks []struct {
		File string `json:"file"`
		ID   string `json:"id"`
	} `json:"split_apks"`
}

func zipHasFile(zr *zip.Reader, name string) bool {
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

func isSplitConfigApk(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "config.") || strings.HasPrefix(lower, "split_")
}

func zipFileByBaseName(zr *zip.Reader, baseName string) *zip.File {
	for _, f := range zr.File {
		if filepath.Base(f.Name) == baseName {
			return f
		}
	}
	return nil
}

func findXapkBaseApk(zr *zip.Reader) *zip.File {
	entry := zipFileByBaseName(zr, "manifest.json")
	if entry == nil {
		return nil
	}
	data, err := readZipEntry(entry)
	if err != nil {
		return nil
	}
	var manifest xapkManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	for _, split := range manifest.SplitApks {
		if split.ID != "base" {
			continue
		}
		if f := zipFileByBaseName(zr, filepath.Base(split.File)); f != nil {
			return f
		}
	}
	return nil
}

func apkEntryHasFile(entry *zip.File, name string) bool {
	data, err := readZipEntry(entry)
	if err != nil {
		return false
	}
	inner, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	return zipHasFile(inner, name)
}

func findApkWithResources(zr *zip.Reader) *zip.File {
	if f := zipFileByBaseName(zr, "base.apk"); f != nil && apkEntryHasFile(f, "resources.arsc") {
		return f
	}

	var best *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if !strings.HasSuffix(strings.ToLower(base), ".apk") {
			continue
		}
		if isSplitConfigApk(base) {
			continue
		}
		if !apkEntryHasFile(f, "resources.arsc") {
			continue
		}
		if best == nil || len(f.Name) < len(best.Name) {
			best = f
		}
	}
	return best
}

func findEmbeddedApk(zr *zip.Reader) (*zip.File, error) {
	if f := findXapkBaseApk(zr); f != nil {
		return f, nil
	}
	if f := findApkWithResources(zr); f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("apk: no base APK with resources.arsc found inside container")
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func newApkFromZip(zr *zip.Reader) (*Apk, error) {
	apk := &Apk{zipreader: zr}
	if err := apk.parseResources(); err != nil {
		return nil, err
	}
	if err := apk.parseManifest(); err != nil {
		return nil, errorf("parse-manifest: %w", err)
	}
	return apk, nil
}

func openFromReaderAt(r io.ReaderAt, size int64) (*Apk, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("apk: not a valid zip archive: %w", err)
	}

	if zipHasFile(zr, "AndroidManifest.xml") {
		return newApkFromZip(zr)
	}

	entry, err := findEmbeddedApk(zr)
	if err != nil {
		return nil, err
	}

	data, err := readZipEntry(entry)
	if err != nil {
		return nil, fmt.Errorf("apk: read %s: %w", entry.Name, err)
	}

	inner, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("apk: invalid embedded apk %s: %w", entry.Name, err)
	}
	if !zipHasFile(inner, "resources.arsc") {
		return nil, fmt.Errorf("apk: %s has no resources.arsc", entry.Name)
	}
	if !zipHasFile(inner, "AndroidManifest.xml") {
		return nil, fmt.Errorf("apk: %s is not a valid APK", entry.Name)
	}
	return newApkFromZip(inner)
}
