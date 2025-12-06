package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

func main() {
	var input, bgStr string
	flag.StringVar(&input, "i", "", "PNG 文件路径（必填）")
	flag.StringVar(&bgStr, "b", "", "x1,y1,x2,y2|radius|color，在(x1,y1)(x2,y2)区域添加圆角半径为radius，颜色为color的背景，多个背景以;分隔，默认不添加。")
	flag.Parse()

	if input == "" {
		fmt.Println("请指定输入 PNG 文件：-i <file.png>")
		return
	}

	if bgStr == "" {
		fmt.Println("请输入背景参数：-b \"x1,y1,x2,y2|radius|color\"")
		return
	}

	var bgs []BgRegion
	for bi, bg := range strings.Split(bgStr, ";") {
		var br = BgRegion{
			Color: color.RGBA{255, 255, 255, 255},
		}
		for i, s := range strings.Split(bg, "|") {
			switch i {
			case 0:
				for j, a := range strings.Split(s, ",") {
					if j >= 4 {
						break
					}
					v, err := strconv.ParseFloat(strings.TrimSpace(a), 64)
					if err != nil || v < 0 {
						fmt.Println("背景区域参数无效:", a)
						return
					}
					br.Area[j] = v
				}
			case 1:
				v, err := strconv.Atoi(strings.TrimSpace(s))
				if err == nil && v > 0 {
					br.Radius = v
				}
			case 2:
				br.Color = toColor(s)
			}
		}
		fmt.Printf("添加背景%d：", bi+1)
		if br.Area[2] <= br.Area[0] || br.Area[3] <= br.Area[1] {
			fmt.Printf("区域参数无效，%g <= %g or %g <= %g\n", br.Area[2], br.Area[0], br.Area[3], br.Area[1])
			return
		}
		fmt.Printf("area:%v, radius:%d, color:%v\n", br.Area, br.Radius, br.Color)
		bgs = append(bgs, br)
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

	outFile := input[0:len(input)-len(filepath.Ext(input))] + "_bg.png"

	out, err := os.Create(outFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer out.Close()

	if err = png.Encode(out, addBackground(imageToRGBA(src), bgs)); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("生成成功:", outFile)
}

func imageToRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

type BgRegion struct {
	Area   [4]float64
	Radius int
	Color  color.RGBA
}

// 给 RGBA 图像在任意矩形区域
func addBackground(img *image.RGBA, bgs []BgRegion) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(b)

	// 先填充透明
	for i := range dst.Pix {
		dst.Pix[i] = 0
	}

	for _, bg := range bgs {

		// 计算实际像素坐标
		rect := image.Rect(
			int(float64(w)*bg.Area[0]/100),
			int(float64(h)*bg.Area[1]/100),
			int(float64(w)*bg.Area[2]/100),
			int(float64(h)*bg.Area[3]/100),
		)

		// 在指定矩形区域绘制圆角白底
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				corner := false

				// 左上角
				if x < rect.Min.X+bg.Radius && y < rect.Min.Y+bg.Radius {
					if distance(x, y, rect.Min.X+bg.Radius-1, rect.Min.Y+bg.Radius-1) > float64(bg.Radius) {
						corner = true
					}
				}
				// 右上角
				if x >= rect.Max.X-bg.Radius && y < rect.Min.Y+bg.Radius {
					if distance(x, y, rect.Max.X-bg.Radius, rect.Min.Y+bg.Radius-1) > float64(bg.Radius) {
						corner = true
					}
				}
				// 左下角
				if x < rect.Min.X+bg.Radius && y >= rect.Max.Y-bg.Radius {
					if distance(x, y, rect.Min.X+bg.Radius-1, rect.Max.Y-bg.Radius) > float64(bg.Radius) {
						corner = true
					}
				}
				// 右下角
				if x >= rect.Max.X-bg.Radius && y >= rect.Max.Y-bg.Radius {
					if distance(x, y, rect.Max.X-bg.Radius, rect.Max.Y-bg.Radius) > float64(bg.Radius) {
						corner = true
					}
				}

				if !corner {
					dst.SetRGBA(x, y, bg.Color)
				}
			}
		}
	}

	// 将原图绘制到背景上
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}

func distance(x1, y1, x2, y2 int) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)
	return math.Sqrt(dx*dx + dy*dy)
}

// "#RGB", "#RRGGBB", "R,G,B" (十进制)
func toColor(input string) (dfbg color.RGBA) {
	dfbg = color.RGBA{255, 255, 255, 255}

	input = strings.TrimSpace(input)
	if len(input) == 0 {
		return
	}

	// 处理十六进制 (#RGB 或 #RRGGBB)
	if hex, ok := strings.CutPrefix(input, "#"); ok {
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 {
			return
		}
		r, _ := strconv.ParseUint(hex[0:2], 16, 8)
		g, _ := strconv.ParseUint(hex[2:4], 16, 8)
		b, _ := strconv.ParseUint(hex[4:6], 16, 8)
		return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
	}

	// 处理十进制 "R,G,B"
	parts := strings.Split(input, ",")
	if len(parts) != 3 {
		return
	}
	r, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || r < 0 || r > 255 {
		return
	}
	g, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || g < 0 || g > 255 {
		return
	}
	b, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil || b < 0 || b > 255 {
		return
	}
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}
