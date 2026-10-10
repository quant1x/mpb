// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestRenderToInMemoryTerm 在 InMemoryTerm 上绘制进度条并断言渲染文本包含预期内容。
func TestRenderToInMemoryTerm(t *testing.T) {
	term := NewInMemoryTerm(10, 80)
	length := uint64(10)
	dt := NewInMemoryDrawTarget(term)
	pb := NewWithDrawTarget(&length, dt)
	defer pb.Abandon()

	pb.SetStyle(DefaultProgressStyleBar())
	pb.Inc(3)
	// 强制绘制到 draw target
	pb.ForceDraw()

	out := term.Contents()
	if !strings.Contains(out, "3/10") {
		t.Fatalf("render output does not contain expected '3/10', got: %q", out)
	}
}

// TestWideBarFillsTerminalWidthInCJKProcessEnv 锁住一处移植偏差：
//
// go-runewidth 会在 init 里按控制台代码页（Windows CP936/932/949/950）把"歧义宽度"
// 口径切成 CJK —— 于是 `█`(U+2588)、`░`(U+2591) 被算成 2 列。若宽度测量跟着它走，
// `formatBar` 的 `width / char_width` 就会砍半：165 列的终端只画 155/2≈77 个块字符，
// 视觉上条只占一半宽度，而补白又按偏大口径算，光标被推到整行最右侧。
//
// 上游 Rust 的 `Width` 是 `is_cjk: AtomicBool::new(false)`，默认始终按 1 列算
// （它自己的单测断言默认 `WIDTH.char('█') == 1`），与进程所在代码页无关。
// 所以本包不读 runewidth 的全局 DefaultCondition，自己持两套 Condition。
func TestWideBarFillsTerminalWidthInCJKProcessEnv(t *testing.T) {
	// 模拟 CJK 进程环境：整个进程被判成 CJK（用户的 ACP=936 就是这种）
	oldEA, oldDefault := runewidth.EastAsianWidth, runewidth.DefaultCondition.EastAsianWidth
	defer func() {
		runewidth.EastAsianWidth = oldEA
		runewidth.DefaultCondition.EastAsianWidth = oldDefault
		runewidth.CreateLUT() // 与 go-runewidth handleEnv 的做法一致，重建全局 LUT
		DefaultWidth.SetDefaultCJK(false)
	}()
	runewidth.EastAsianWidth = true
	runewidth.DefaultCondition.EastAsianWidth = true
	runewidth.CreateLUT()

	// 前置条件：进程级口径确实是 CJK（否则这条测试失去意义）
	if got := runewidth.DefaultCondition.StringWidth("██"); got != 4 {
		t.Fatalf("前置条件不成立：进程内 CJK 口径应把 \"██\" 算成 4 列，实际 %d", got)
	}

	// 本包口径必须与上游一致：歧义宽度 1 列，真宽字符仍 2 列
	if got := DefaultWidth.char('█'); got != 1 {
		t.Fatalf("Width.char('█') 应为 1（上游默认口径），实际 %d", got)
	}
	if got := DefaultWidth.str("██"); got != 2 {
		t.Fatalf("Width.str(\"██\") 应为 2，实际 %d", got)
	}
	if got := DefaultWidth.char('あ'); got != 2 {
		t.Fatalf("Width.char('あ') 应为 2（真宽字符不受影响），实际 %d", got)
	}

	// 显式打开 CJK 口径后才按 2 列算（对应上游 set_default_cjk(true)）
	DefaultWidth.SetDefaultCJK(true)
	if got := DefaultWidth.char('█'); got != 2 {
		t.Fatalf("SetDefaultCJK(true) 后 Width.char('█') 应为 2，实际 %d", got)
	}
	DefaultWidth.SetDefaultCJK(false)

	// 端到端：165 列终端里 {wide_bar} {pos:>4}/{len} 的条应铺满 155 个块字符
	const termWidth = 165
	const counterCols = 10 // " " + " 651" + "/" + "1024"
	term := NewInMemoryTerm(24, termWidth)
	pb := NewWithDrawTarget(nil, NewInMemoryDrawTarget(term))
	defer pb.Abandon()

	sty, err := ProgressStyleWithTemplate("{wide_bar} {pos:>4}/{len}")
	if err != nil {
		t.Fatal(err)
	}
	pb.SetLength(1024)
	pb.SetStyle(sty)
	pb.SetPosition(651)
	pb.ForceDraw()

	line := term.Contents()
	bar := strings.Count(line, "█") + strings.Count(line, "░")
	if bar != termWidth-counterCols {
		t.Fatalf("165 列终端里条应为 %d 个块字符，实际 %d，行=%q",
			termWidth-counterCols, bar, line)
	}
	if !strings.Contains(line, "651/1024") {
		t.Fatalf("渲染结果缺少计数器，行=%q", line)
	}
}

// TestMeasureTextWidthMatchesUpstream 逐条对齐上游单测（style.rs:1358-1387）与
// `console::measure_text_width` 的语义：只跳 ANSI 段，其余按 unicode-width 累加。
// 期望值取上游自己的断言，不取"我们跑出来的数"。
func TestMeasureTextWidthMatchesUpstream(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"带 ANSI", "\x1b[31m█\x1b[0m", 1}, // 上游：WIDTH.ansi_str("\u{1b}[31m█\u{1b}[0m") == 1
		{"真宽字符", "あ", 2},                  // 上游：WIDTH.str("あ") == 2
		{"歧义字符默认 1 列", "█", 1},            // 上游：WIDTH.str("█") == 1
		{"全角数字不归一化", "１", 2},              // U+FF11 是 2 列；NFKC 归一化会把它缩成 1
		{"制表符是控制字符", "\t", 0},             // 展开期按 tab_width=8 换空格，原始 '\t' 宽 0
		{"回车换行为 0 列", "a\r\nb", 2},
		{"NUL 为 0 列", "a\x00b", 2},
	}
	for _, c := range cases {
		if got := measureTextWidth(c.in); got != c.want {
			t.Errorf("%s：measureTextWidth(%q) = %d，上游应为 %d", c.name, c.in, got, c.want)
		}
	}

	// ZWJ 不特殊处理：与 Width.str 同口径（上游交给 unicode-width 的字形簇逻辑）
	if got, want := measureTextWidth("👨\u200d👩"), DefaultWidth.str("👨\u200d👩"); got != want {
		t.Errorf("ZWJ 不应被特殊处理：measureTextWidth = %d，Width.str = %d", got, want)
	}
}

// TestWideBarGivesWayForWideMessage 看**效果**：消息里的全角字符按 2 列让位（与 Rust 一致），
// 而不是被 NFKC 归一成 1 列后少让一位。
func TestWideBarGivesWayForWideMessage(t *testing.T) {
	const termWidth = 40
	term := NewInMemoryTerm(24, termWidth)
	pb := NewWithDrawTarget(nil, NewInMemoryDrawTarget(term))
	defer pb.Abandon()

	sty, err := ProgressStyleWithTemplate("{wide_bar} {msg}")
	if err != nil {
		t.Fatal(err)
	}
	pb.SetLength(1024)
	pb.SetStyle(sty)
	pb.SetMessage("１") // 全角数字：2 列（不是 1）
	pb.SetPosition(512)
	pb.ForceDraw()

	line := term.Contents()
	bar := strings.Count(line, "█") + strings.Count(line, "░")
	// 40 列 -（空格 1 列 + 全角消息 2 列）= 37
	if bar != termWidth-3 {
		t.Fatalf("全角消息应占 2 列，条应为 %d 个块字符，实际 %d，行=%q", termWidth-3, bar, line)
	}
}
