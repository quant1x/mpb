Output screenshots/multi-tree.gif

# 设置终端外观
Set FontSize 20
Set Width 1200
Set Height 540
Set Padding 6

# 主条 + 动态挂子条：15ms 一个 tick，规模随机，给足 25s
Type "go run github.com/quant1x/mpb/examples/multi-tree"
Enter
Sleep 25s

# 跑不完就中断收尾
Ctrl+C
