package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	args := os.Args[1:]

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "使用方法:")
		fmt.Fprintln(os.Stderr, "  1. 自适应模式: png2ico <一张高清PNG路径> [可选输出ICO路径]")
		fmt.Fprintln(os.Stderr, "  2. 缝合模式: png2ico <输出ICO路径> <输入PNG_1> <输入PNG_2> ...")
		fmt.Fprintln(os.Stderr, "\n示例:")
		fmt.Fprintln(os.Stderr, "  png2ico app.png               -> 生成等比缩小所有标准尺寸的 app.ico")
		fmt.Fprintln(os.Stderr, "  png2ico out.ico 16.png 32.png -> 将多张 PNG 缝合打包（自动识别像素并去重）")
		os.Exit(1)
	}

	type iconData struct {
		sz  int
		dib []byte
	}
	var icons []iconData
	var outFile string

	if len(args) >= 2 && strings.HasSuffix(strings.ToLower(args[0]), ".ico") {
		outFile = args[0]

		iconMap := make(map[int][]byte)
		for _, pngPath := range args[1:] {
			sz, dib, err := loadAndEncodePNG(pngPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误：读取或解码 PNG 失败:", err, "路径:", pngPath)
				os.Exit(1)
			}
			iconMap[sz] = dib
		}

		for sz, dib := range iconMap {
			icons = append(icons, iconData{sz: sz, dib: dib})
		}
	} else {
		inputPNG := args[0]
		if len(args) >= 2 {
			outFile = args[1]
		} else {
			outFile = strings.TrimSuffix(inputPNG, filepath.Ext(inputPNG)) + ".ico"
		}

		in, err := os.Open(inputPNG)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误：无法打开 PNG 文件:", err)
			os.Exit(1)
		}
		src, err := png.Decode(in)
		in.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误：PNG 解码失败:", err)
			os.Exit(1)
		}

		bounds := src.Bounds()
		srcW, srcH := bounds.Dx(), bounds.Dy()
		if srcW != srcH {
			fmt.Fprintln(os.Stderr, "错误：输入图片不是正方形，ICO 仅支持 1:1 比例的分辨率")
			os.Exit(1)
		}

		defaultSizes := []int{16, 32, 48, 64, 128, 256}
		for _, sz := range defaultSizes {
			if srcW < sz {
				continue
			}

			rgba := resizeNearest(src, sz, sz)

			var buf bytes.Buffer
			_ = png.Encode(&buf, rgba)
			icons = append(icons, iconData{sz: sz, dib: buf.Bytes()})
		}
	}

	absPath, _ := filepath.Abs(outFile)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "错误：无法创建 ICO 目录:", err)
		os.Exit(1)
	}

	out, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误：无法创建 ICO 文件:", err)
		os.Exit(1)
	}
	defer out.Close()

	_ = binary.Write(out, binary.LittleEndian, uint16(0))
	_ = binary.Write(out, binary.LittleEndian, uint16(1))
	_ = binary.Write(out, binary.LittleEndian, uint16(len(icons)))

	offset := uint32(6 + len(icons)*16)

	for _, ic := range icons {
		wByte, hByte := byte(ic.sz), byte(ic.sz)
		// 严密守护微软底层规范：当图标高宽为 256 像素时，目录信息小格子里必须强行记为 0
		if ic.sz == 256 {
			wByte, hByte = 0, 0
		}
		_, _ = out.Write([]byte{wByte, hByte, 0, 0})
		_ = binary.Write(out, binary.LittleEndian, uint16(1))
		_ = binary.Write(out, binary.LittleEndian, uint16(32))
		_ = binary.Write(out, binary.LittleEndian, uint32(len(ic.dib)))
		_ = binary.Write(out, binary.LittleEndian, offset)
		offset += uint32(len(ic.dib))
	}

	for _, ic := range icons {
		_, _ = out.Write(ic.dib)
	}
}

func loadAndEncodePNG(path string) (int, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return 0, nil, err
	}

	if cfg.Width != cfg.Height {
		return 0, nil, fmt.Errorf("图片非正方形比例(%dx%d)", cfg.Width, cfg.Height)
	}

	_, _ = f.Seek(0, 0)
	img, err := png.Decode(f)
	if err != nil {
		return 0, nil, err
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return cfg.Width, buf.Bytes(), nil
}

func resizeNearest(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	for y := range h {
		for x := range w {
			srcX := bounds.Min.X + (x * srcW / w)
			srcY := bounds.Min.Y + (y * srcH / h)
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}
