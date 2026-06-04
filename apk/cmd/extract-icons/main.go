// 从 APK、zip 或 xapk（如 disney.zip、Emby.xapk）提取 ic_launcher 与 banner 并保存为 PNG。
//
// 用法:
//
//	go run ./cmd/extract-icons -apk testdata/disney.zip -o ./out
//	go run ./cmd/extract-icons -apk testdata/base.apk -o ./out
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/chenhuifeng/androidbinary/v2/apk"
)

func main() {
	apkPath := flag.String("apk", "", "APK 文件路径")
	outDir := flag.String("o", ".", "输出目录")
	flag.Parse()

	if *apkPath == "" && flag.NArg() > 0 {
		*apkPath = flag.Arg(0)
	}
	if *apkPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("创建输出目录: %v", err)
	}

	pkg, err := apk.OpenFile(*apkPath)
	if err != nil {
		log.Fatalf("打开 APK: %v", err)
	}
	defer pkg.Close()

	base := filepath.Base(*apkPath)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}

	icon, _, err := pkg.Icon(nil)
	if err != nil {
		log.Fatalf("提取 ic_launcher: %v", err)
	}
	if icon == nil {
		log.Println("ic_launcher: 未配置或无法解析，已跳过")
		return
	}
	iconPath := filepath.Join(*outDir, base+"_ic_launcher.png")
	if err := savePNG(iconPath, icon); err != nil {
		log.Fatalf("保存 ic_launcher: %v", err)
	}
	fmt.Printf("ic_launcher -> %s (%dx%d)\n", iconPath, icon.Bounds().Dx(), icon.Bounds().Dy())

	banner, _, err := pkg.Banner(nil)
	if err != nil {
		log.Fatalf("提取 banner: %v", err)
	}
	if banner == nil {
		log.Println("banner: 未配置或无法解析，已跳过")
		return
	}
	bannerPath := filepath.Join(*outDir, base+"_banner.png")
	if err := savePNG(bannerPath, banner); err != nil {
		log.Fatalf("保存 banner: %v", err)
	}
	fmt.Printf("banner     -> %s (%dx%d)\n", bannerPath, banner.Bounds().Dx(), banner.Bounds().Dy())
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return f.Close()
}
