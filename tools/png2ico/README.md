# png2ico
png to ico

png转ico

## 使用方法：
* 自适应模式: png2ico <一张高清PNG路径> [可选输出ICO路径]

* 缝合模式: png2ico <输出ICO路径> <输入PNG_1> <输入PNG_2> ...

## 示例:
```
// 生成等比缩小所有标准尺寸的 app.ico
png2ico app.png
```
```
// 将多张 PNG 缝合打包（自动识别像素并去重）
png2ico out.ico 16.png 32.png
```