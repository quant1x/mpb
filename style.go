package mpb

import (
	"fmt"
	"math"
	"strings"
)

// 精简版 ProgressStyle 翻译自 src/style.rs，提供基本的模板替换和进度条格式化。
// 注：为便于增量迁移，此实现保留了常用功能但并非完全逐行等价。

type ProgressStyle struct {
	tickStrings   []string
	progressChars []string
	template      string
	charWidth     int
	tabWidth      int
}

func DefaultProgressStyleBar() ProgressStyle {
	ps := ProgressStyle{}
	ps.template = "{wide_bar} {pos}/{len}"
	ps.progressChars = []string{"█", "░"}
	ps.tickStrings = strings.Split("⠁ ⠉ ⠙ ⠚ ⠒ ⠂ ⠂ ⠒ ⠲ ⠴ ⠤ ⠄ ⠄ ⠤ ⠠ ⠠ ⠤ ⠦ ⠖ ⠒ ⠐ ⠐ ⠒ ⠓ ⠋ ⠉ ⠈", " ")
	ps.charWidth = 1
	ps.tabWidth = DEFAULT_TAB_WIDTH
	return ps
}

func DefaultProgressStyleSpinner() ProgressStyle {
	ps := ProgressStyle{}
	ps.template = "{spinner} {msg}"
	ps.progressChars = []string{"", ""}
	ps.tickStrings = strings.Split("- ", " ")
	ps.charWidth = 1
	ps.tabWidth = DEFAULT_TAB_WIDTH
	return ps
}

func (ps *ProgressStyle) SetTabWidth(w int) { ps.tabWidth = w }

// FormatState 将 ProgressState 格式化为若干行及对应文本内容（返回 line types 与 contents）。
// 这是一个简化实现，支持常见占位符替换并返回渲染文本，便于 DrawStateStub.contents 填充。
func (ps *ProgressStyle) FormatState(state *ProgressState, targetWidth int) ([]LineType, []string) {
	tpl := ps.template
	pos := state.Pos()
	var length uint64
	if state.Len() != nil {
		length = *state.Len()
	} else {
		length = pos
	}

	// 替换宽栏占位符
	if strings.Contains(tpl, "{wide_bar}") {
		bar := ps.formatBar(state.Fraction(), targetWidth, nil)
		tpl = strings.ReplaceAll(tpl, "{wide_bar}", bar)
	}
	// 替换其他简单占位符
	tpl = strings.ReplaceAll(tpl, "{pos}", fmt.Sprintf("%d", pos))
	tpl = strings.ReplaceAll(tpl, "{len}", fmt.Sprintf("%d", length))
	tpl = strings.ReplaceAll(tpl, "{msg}", state.message.Expanded())
	tpl = strings.ReplaceAll(tpl, "{prefix}", state.prefix.Expanded())
	// percent
	if strings.Contains(tpl, "{percent}") {
		pct := state.Fraction() * 100.0
		tpl = strings.ReplaceAll(tpl, "{percent}", fmt.Sprintf("%.0f", pct))
	}

	// 目前只返回单行 Bar 的类型与文本
	return []LineType{LineBar}, []string{tpl}
}

// formatBar 返回一个字符串表示的进度条，width 为目标可用列数（近似）。
func (ps *ProgressStyle) formatBar(fract float32, width int, altStyle interface{}) string {
	if width <= 0 {
		width = 20
	}
	// 计算填充数量
	fill := int(math.Floor(float64(fract) * float64(width)))
	if fill < 0 {
		fill = 0
	}
	if fill > width {
		fill = width
	}

	var b strings.Builder
	for i := 0; i < fill; i++ {
		b.WriteString(ps.progressChars[0])
	}
	for i := 0; i < width-fill; i++ {
		b.WriteString(ps.progressChars[len(ps.progressChars)-1])
	}
	return b.String()
}
