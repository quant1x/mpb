Output screenshots/cargo.gif
#Require typing

# 设置终端外观
Set FontSize 20
Set Width 1200
Set Height 340
Set Padding 6
#Theme "Catppuccin Mocha"

# 模拟输入和运行
Type "go run github.com/quant1x/mpb/examples/cargo"
Enter
Sleep 20s

# 模拟按下 Ctrl+C 结束
Ctrl+C