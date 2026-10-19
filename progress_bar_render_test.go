// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"strings"
	"testing"
	"time"

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

type countingWriter struct {
	writes int
	bytes  int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	w.bytes += len(p)
	return len(p), nil
}

// TestStdTermWritesOneBufferPerFrame 锁住"不闪"的关键。
//
// 上游的 stdout() / stderr() 是 `Term::buffered_stdout()` / `buffered_stderr()`
// —— console::Term 的 `inner.buffer` 是 Some(..)，write_str 只进缓冲，
// indicatif 在 `draw_to_term` 末尾调 `term.flush()` 一次落盘。于是一帧里的
// move_cursor_up / clear_line / 逐行内容 / 补白这几十次 write_str 合成**一次** write(2)，
// 终端每帧只重绘一次 —— 这就是"丝滑"。
//
// 之前这里每次 write_str 都直接 write：一帧几十次系统调用，终端在"清屏"和"重画"
// 之间反复重绘（肉眼就是闪烁），系统调用次数也跟着涨。
func TestStdTermWritesOneBufferPerFrame(t *testing.T) {
	newState := func() *DrawState {
		return &DrawState{
			lines:    []LineType{LineBar, LineBar},
			contents: []string{"first line", "second line"},
		}
	}

	const frames = 5

	// 带缓冲：每帧一次 write（flush）
	cw := &countingWriter{}
	term := newStdTermWithWriter(cw, true)
	for i := 0; i < frames; i++ {
		if _, err := newState().drawToTerm(term, 0); err != nil {
			t.Fatal(err)
		}
	}
	if cw.writes != frames {
		t.Fatalf("带缓冲时每帧应只写 1 次：%d 帧共写 %d 次（应 %d）", frames, cw.writes, frames)
	}

	// 不带缓冲：同一帧会拆成多次写（闪烁与系统调用开销的来源），用来证明上面那条不是白测
	raw := &countingWriter{}
	rawTerm := newStdTermWithWriter(raw, false)
	if _, err := newState().drawToTerm(rawTerm, 0); err != nil {
		t.Fatal(err)
	}
	if raw.writes <= 1 {
		t.Fatalf("不带缓冲时一帧应拆成多次写，实际 %d 次", raw.writes)
	}
}

// TestRateLimiterThrottlesToRefreshHz 对齐上游 RateLimiter 的口径：
// 默认刷新率 20Hz（interval = 1000/20 = 50ms），突发上限 MAX_BURST = 20。
//
// 上游 `drawable(force_draw, now)` 里是 `force_draw || rate_limiter.allow(now)`，
// 所以**不是每个 tick 都会重绘**：tight loop 里每秒上千次 Inc，真正落盘的只有 ~20 帧。
func TestRateLimiterThrottlesToRefreshHz(t *testing.T) {
	rl := newRateLimiter(20)
	base := time.Now()

	const calls = 1000 // 每 1ms 一次调用，共 1 秒
	allowed := 0
	for i := 0; i < calls; i++ {
		if rl.allow(base.Add(time.Duration(i) * time.Millisecond)) {
			allowed++
		}
	}

	// 起始突发 20 次，之后每 50ms 放行一次 → 1 秒内约 20 + 19 = 39 次
	if allowed < 30 || allowed > 50 {
		t.Fatalf("20Hz 限流器 1 秒内放行 %d/%d 次（上游口径约 39：突发 20 + 每秒 20），节流失效？",
			allowed, calls)
	}
}

// TestAnsiSegmentsMatchesUpstreamDFA 对齐上游 console::ansi 的手写 DFA：
// 起始符含 U+009B、结束符是固定集合而不是任意字母、OSC 不计为 ANSI。
func TestAnsiSegmentsMatchesUpstreamDFA(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ansiSegment
	}{
		{"SGR 前后", "\x1b[31m█\x1b[0m", []ansiSegment{{"\x1b[31m", true}, {"█", false}, {"\x1b[0m", true}}},
		{"文本在前", "ab\x1b[0m", []ansiSegment{{"ab", false}, {"\x1b[0m", true}}},
		{"真彩色 SGR", "\x1b[38;2;255;0;0m", []ansiSegment{{"\x1b[38;2;255;0;0m", true}}},
		{"字符集指定", "\x1b(B", []ansiSegment{{"\x1b(B", true}}},
		{"单字节 CSI 起始符", "\u009b31m", []ansiSegment{{"\u009b31m", true}}},
		{"OSC 上游不认", "\x1b]8;;url\x07x", []ansiSegment{{"\x1b]8;;url\x07x", false}}},
		{"结束符不在集合内", "\x1bX", []ansiSegment{{"\x1bX", false}}},
		{"纯文本", "abc", []ansiSegment{{"abc", false}}},
	}
	for _, c := range cases {
		got := ansiSegments(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("%s：段数 = %d，应为 %d（%#v）", c.name, len(got), len(c.want), got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s：第 %d 段 = %#v，应为 %#v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

// TestForceDrawDoesNotDuplicateMultiLines 锁住 ForceDraw 只格式化一次：
// 之前 SafeBarState.Draw 把已格式化的行当作 origin lines 再传一遍，
// aggregateDraw 又格式化一次，多进度条里会画出两条相同的 bar。
func TestForceDrawDoesNotDuplicateMultiLines(t *testing.T) {
	term := NewInMemoryTerm(10, 60)
	mp := WithDrawTarget(NewInMemoryDrawTarget(term))
	pb := mp.Add(New(10))
	defer pb.Abandon()

	pb.Inc(3)
	pb.ForceDraw()

	lines := strings.Split(term.Contents(), "\n")
	if len(lines) != 1 {
		t.Fatalf("ForceDraw 不应把同一条 bar 画成多行：%q", term.Contents())
	}
	if !strings.Contains(lines[0], "3/10") {
		t.Fatalf("ForceDraw 后缺少计数器：%q", term.Contents())
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

func TestItalicStyle(t *testing.T) {
	s := &Style{Spec: "italic"}
	if got, want := s.ApplyFor("x", false), "\x1b[3mx\x1b[0m"; got != want {
		t.Fatalf("italic style = %q, want %q", got, want)
	}
	if got, want := s.ApplyFor("x", true), "\x1b[3mx\x1b[0m"; got != want {
		t.Fatalf("italic style (stderr) = %q, want %q", got, want)
	}
}
