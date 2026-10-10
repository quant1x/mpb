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
