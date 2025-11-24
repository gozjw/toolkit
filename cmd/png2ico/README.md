# png2ico
png to ico

png图片转ico

## 使用方法：
1. 运行：
```
./png2ico.exe -i cs.png -b "10,10,20,20|15|#FFF;80,10,90,20||#0F0"
```

## 参数：
- -b string    
      x1,y1,x2,y2|radius|color，在(x1,y1)(x2,y2)区域添加圆角半径为radius，颜色为color的背景，多个背景以;分隔，默认不添加。

- -i string    
      输入 PNG 文件路径（必填）

- -s string    
      ICO尺寸，用逗号分隔 (default "16,32,48,64,128,256")