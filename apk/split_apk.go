package apk

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"path/filepath"

	"github.com/chenhuifeng/androidbinary/v2"
)

type splitApk struct {
	zip   *zip.Reader
	table *androidbinary.TableFile
	data  []byte
}

type xapkManifestFull struct {
	Icon      string `json:"icon"`
	SplitApks []struct {
		File string `json:"file"`
		ID   string `json:"id"`
	} `json:"split_apks"`
}

func (k *Apk) loadContainerExtras(container *zip.Reader) error {
	k.containerZip = container

	entry := zipFileByBaseName(container, "manifest.json")
	if entry == nil {
		return nil
	}
	data, err := readZipEntry(entry)
	if err != nil {
		return err
	}
	var manifest xapkManifestFull
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	k.xapkIcon = manifest.Icon

	for _, split := range manifest.SplitApks {
		if split.ID == "base" {
			continue
		}
		f := zipFileByBaseName(container, filepath.Base(split.File))
		if f == nil {
			continue
		}
		if err := k.loadSplitApk(f); err != nil {
			continue
		}
	}
	return nil
}

func (k *Apk) loadSplitApk(entry *zip.File) error {
	data, err := readZipEntry(entry)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if !zipHasFile(zr, "resources.arsc") {
		return fmt.Errorf("split %s has no resources.arsc", entry.Name)
	}
	resData, err := readZipFileFrom(zr, "resources.arsc")
	if err != nil {
		return err
	}
	tf, err := androidbinary.NewTableFile(bytes.NewReader(resData))
	if err != nil {
		return err
	}
	k.splits = append(k.splits, splitApk{zip: zr, table: tf, data: data})
	return nil
}

func (k *Apk) allZipReaders() []*zip.Reader {
	readers := []*zip.Reader{k.zipreader}
	for i := range k.splits {
		readers = append(readers, k.splits[i].zip)
	}
	return readers
}

func (k *Apk) getResource(id androidbinary.ResID, config *androidbinary.ResTableConfig) (interface{}, error) {
	if k.table != nil {
		v, err := k.table.GetResource(id, config)
		if err == nil {
			return v, nil
		}
	}
	for i := range k.splits {
		v, err := k.splits[i].table.GetResource(id, config)
		if err == nil {
			return v, nil
		}
	}
	return nil, fmt.Errorf("androidbinary: entry 0x%04X not found", id.Entry())
}

func (k *Apk) getResourcePathPreferRaster(id androidbinary.ResID, config *androidbinary.ResTableConfig) (string, error) {
	if k.table != nil {
		path, err := k.table.GetResourcePathPreferRaster(id, config)
		if err == nil {
			return path, nil
		}
	}
	for i := range k.splits {
		path, err := k.splits[i].table.GetResourcePathPreferRaster(id, config)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("androidbinary: no raster resource for %s", id)
}

func (k *Apk) loadXapkIcon() (image.Image, error) {
	if k.containerZip == nil || k.xapkIcon == "" {
		return nil, newError("no xapk icon")
	}
	data, err := readZipFileFrom(k.containerZip, k.xapkIcon)
	if err != nil {
		return nil, err
	}
	img, _, err := k.decodeRaster(data)
	return img, err
}

func readZipFileFrom(zr *zip.Reader, name string) ([]byte, error) {
	for _, file := range zr.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("apk: file %q not found", name)
}
