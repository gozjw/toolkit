package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

func main() {
	var input, sizesStr string
	flag.StringVar(&input, "i", "", "PNG 文件路径（必填）")
	flag.StringVar(&sizesStr, "s", "16,32,48,64,128,256", "ICO尺寸，用逗号分隔")
	flag.Parse()

	if input == "" {
		fmt.Println("请指定输入 PNG 文件：-i <file.png>")
		return
	}

	outFile := input[0:len(input)-len(filepath.Ext(input))] + ".ico"

	var sizes []int
	for _, s := range strings.Split(sizesStr, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v <= 0 {
			fmt.Println("尺寸参数无效:", s)
			return
		}
		sizes = append(sizes, v)
	}

	in, err := os.Open(input)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer in.Close()

	src, err := png.Decode(in)
	if err != nil {
		fmt.Println(err)
		return
	}

	out, err := os.Create(outFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer out.Close()

	binary.Write(out, binary.LittleEndian, uint16(0))
	binary.Write(out, binary.LittleEndian, uint16(1))
	binary.Write(out, binary.LittleEndian, uint16(len(sizes)))

	type dirEntry struct {
		w, h byte
		size uint32
		off  uint32
	}
	entries := make([]dirEntry, len(sizes))
	dataBlobs := make([][]byte, len(sizes))

	offset := uint32(6 + len(sizes)*16)

	for i, sz := range sizes {
		rgba := resizeCatmullRom(src, sz, sz)
		dib := encodePNG(rgba)

		entries[i] = dirEntry{
			w:    byte(sz),
			h:    byte(sz),
			size: uint32(len(dib)),
			off:  offset,
		}
		dataBlobs[i] = dib
		offset += uint32(len(dib))
	}

	for _, e := range entries {
		out.Write([]byte{e.w})
		out.Write([]byte{e.h})
		out.Write([]byte{0})
		out.Write([]byte{0})
		binary.Write(out, binary.LittleEndian, uint16(1))
		binary.Write(out, binary.LittleEndian, uint16(32))
		binary.Write(out, binary.LittleEndian, e.size)
		binary.Write(out, binary.LittleEndian, e.off)
	}

	for _, blob := range dataBlobs {
		out.Write(blob)
	}

	fmt.Println("生成成功:", outFile)
}

func resizeCatmullRom(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)
	return dst
}

func encodePNG(img *image.RGBA) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
