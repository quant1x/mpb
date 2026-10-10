Output screenshots/multi-tree-ext.gif

# 设置终端外观
Set FontSize 20
Set Width 1200
Set Height 600

# multi-tree 的扩展版（带 ✔ 收尾与 msg 对齐）：20ms 一个 tick，给足 25s
Type "go run github.com/quant1x/mpb/examples/multi-tree-ext"
Enter
Sleep 25s

# 跑不完就中断收尾
Ctrl+C
