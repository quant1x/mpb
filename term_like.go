// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"fmt"
	"io"
	"os"
	"sync"

	termpkg "golang.org/x/term"
)

// TermLike 表示最小的终端行为抽象（源自 src/term_like.rs），中文注释对应 Rust 注释。
type TermLike interface {
	Width() uint16
	Height() uint16
	MoveCursorUp(n int) error
	MoveCursorDown(n int) error
	MoveCursorRight(n int) error
	MoveCursorLeft(n int) error
	WriteLine(s string) error
	WriteStr(s string) error
	ClearLine() error
	Flush() error
}

// StdTerm 是一个简单的终端实现，向指定的 io.Writer 写入，并通过 ANSI 控制序列移动光标。
type StdTerm struct {
	w    io.Writer
	file *os.File // 构造时探测到的 *os.File；非 nil 时 Width/Height 每次都重新查询尺寸
	// handle 是 Windows 控制台句柄（非控制台为 0）。非 0 时写入走 WriteConsoleW
	// （UTF-8 先转 UTF-16），绕开 OEM 代码页；见 console_windows.go。
	handle uintptr
	// width/height 只是**取不到尺寸时**的回落值（Rust 是 (24, DEFAULT_WIDTH)），
	// 不是"当前宽度"—— 终端窗口随时可缩放，尺寸必须每次现问。
	width  uint16
	height uint16
	// isTerm 对应 Rust 的 `console::Term::is_term()`：构造时探测到真实终端尺寸即为真终端。
	// 写入目标是文件/管道时是 false —— Rust 的 `is_hidden()` 会据此把整条进度条隐藏。
	isTerm bool

	// 下面三个字段对应 console::Term 的 `inner.buffer: Option<Mutex<Vec<u8>>>`：
	// Some(..) 时 `write_str` 只往缓冲里追加，`flush()` 才一次性 write_through。
	//
	// 上游的 `stdout()` / `stderr()` 分别是 `Term::buffered_stdout()` /
	// `buffered_stderr()` —— 也就是说**默认目标都是带缓冲的**，一帧里的
	// move_cursor_up / clear_line / 逐行内容 / 补白 这几十次 write_str 最终只合成
	// 一次 write(2)（indicatif 在 draw_to_term 末尾调 term.flush()）。
	// 这是"不闪"的关键：终端只在帧结束时重绘一次。
	//
	// 之前这里是每次 write_str 都直接 write，一帧几十次系统调用，终端在"清屏"和
	// "重画"之间反复重绘，肉眼就是闪烁；系统调用次数也跟着涨。
	buffered bool
	mu       sync.Mutex
	buf      []byte
}

// NewStdTerm 使用 stdout 创建默认终端，对应 Rust 的 `Term::buffered_stdout()`
// （`ProgressDrawTarget::stdout()` 用的就是这个，带缓冲）。
func NewStdTerm() *StdTerm {
	w := os.Stdout
	handle := detectConsole(w)
	enableVirtualTerminal(handle)
	if width, height, err := termpkg.GetSize(int(w.Fd())); err == nil {
		return &StdTerm{w: w, file: w, handle: handle, width: uint16(width), height: uint16(height), isTerm: true, buffered: true}
	}
	return &StdTerm{w: os.Stdout, file: os.Stdout, handle: handle, width: 80, height: 20, buffered: true}
}

// NewStderrTerm 使用 stderr 创建终端，对应 Rust 的 `Term::buffered_stderr()`。
// 进度条的默认绘制目标是 stderr，所以 `ProgressDrawTarget::stderr()` 走的是这个。
func NewStderrTerm() *StdTerm { return newStdTermWithWriter(os.Stderr, true) }

// NewStdTermWithWriter creates a StdTerm that writes to the provided writer.
//
// 注意：这个便捷构造**不带缓冲**（写什么立刻进 writer）。上游没有直接对应的构造，
// 带缓冲的那两个是 `buffered_stdout()` / `buffered_stderr()` —— 要走缓冲语义请用
// NewStdTerm / NewStderrTerm。
func NewStdTermWithWriter(w io.Writer) *StdTerm { return newStdTermWithWriter(w, false) }

// newStdTermWithWriter 是内部构造：buffered 决定是否启用 console::Term 的 inner.buffer。
func newStdTermWithWriter(w io.Writer, buffered bool) *StdTerm {
	// Try to derive terminal size if writer is an *os.File
	if f, ok := w.(*os.File); ok {
		handle := detectConsole(f)
		enableVirtualTerminal(handle)
		if width, height, err := termpkg.GetSize(int(f.Fd())); err == nil {
			return &StdTerm{w: w, file: f, handle: handle, width: uint16(width), height: uint16(height), isTerm: true, buffered: buffered}
		}
		return &StdTerm{w: w, file: f, handle: handle, width: 80, height: 20, buffered: buffered}
	}
	return &StdTerm{w: w, width: 80, height: 20, buffered: buffered}
}

// IsTerm 对应 Rust 的 `console::Term::is_term()`：是不是真实终端（而非文件/管道）。
func (t *StdTerm) IsTerm() bool { return t.isTerm }

// sizeChecked 对应 Rust 的 `console::Term::size_checked()` → `terminal_size(self)`：
// **每次现问**，不缓存。取不到（不是文件/已被重定向/查询失败）时返回 false。
func (t *StdTerm) sizeChecked() (uint16, uint16, bool) {
	if t.file == nil {
		return 0, 0, false
	}
	width, height, err := termpkg.GetSize(int(t.file.Fd()))
	if err != nil || width <= 0 || height <= 0 || width > 0xFFFF || height > 0xFFFF {
		return 0, 0, false
	}
	return uint16(width), uint16(height), true
}

// Width 对应 Rust 的 `console::Term::width()`：
//
//	let (_, w) = term.size(); // size() = size_checked().unwrap_or((24, DEFAULT_WIDTH))
//
// 终端窗口是可缩放的，进度条每帧都要按**当前**宽度重排；构造时量一次就存死的话，
// 缩放之后要么超宽折行、要么只画一截补不满整行。
func (t *StdTerm) Width() uint16 {
	if w, _, ok := t.sizeChecked(); ok {
		return w
	}
	return t.width
}

// Height 同 Width：每次现问（Rust 的 `Term::height()` → `size().0`）。
func (t *StdTerm) Height() uint16 {
	if _, h, ok := t.sizeChecked(); ok {
		return h
	}
	return t.height
}

func (t *StdTerm) writeEscape(format string, a ...interface{}) error {
	if t.w == nil {
		return nil
	}
	if !t.buffered {
		_, err := fmt.Fprintf(t.w, format, a...)
		return err
	}
	// 对应 Rust：`term.write_str(&format!(...))` —— 带缓冲时进 buf，不落盘。
	t.mu.Lock()
	t.buf = fmt.Appendf(t.buf, format, a...)
	t.mu.Unlock()
	return nil
}

func (t *StdTerm) MoveCursorUp(n int) error {
	if n <= 0 {
		return nil
	}
	return t.writeEscape("\x1b[%dA", n)
}

func (t *StdTerm) MoveCursorDown(n int) error {
	if n <= 0 {
		return nil
	}
	return t.writeEscape("\x1b[%dB", n)
}

func (t *StdTerm) MoveCursorRight(n int) error {
	if n <= 0 {
		return nil
	}
	return t.writeEscape("\x1b[%dC", n)
}

func (t *StdTerm) MoveCursorLeft(n int) error {
	if n <= 0 {
		return nil
	}
	return t.writeEscape("\x1b[%dD", n)
}

func (t *StdTerm) WriteLine(s string) error {
	if err := t.WriteStr(s); err != nil {
		return err
	}
	if err := t.WriteStr("\n"); err != nil {
		return err
	}
	return nil
}

// WriteStr 对应 Rust 的 `Term::write_str()`：
//
//	match self.inner.buffer {
//	    Some(ref buffer) => buffer.lock().unwrap().write_all(s.as_bytes()),
//	    None => self.write_through(s.as_bytes()),
//	}
func (t *StdTerm) WriteStr(s string) error {
	if t.w == nil {
		return nil
	}
	if !t.buffered {
		return t.writeThrough(s)
	}
	t.mu.Lock()
	t.buf = append(t.buf, s...)
	t.mu.Unlock()
	return nil
}

// ClearLine 使用 ANSI 序列清除当前行并回到行首。
func (t *StdTerm) ClearLine() error {
	// Match console::Term.clear_line which emits CR + ESC[2K ("\r\x1b[2K").
	return t.writeEscape("\r\x1b[2K")
}

// Flush 对应 Rust 的 `Term::flush()`：
//
//	if let Some(ref buffer) = self.inner.buffer {
//	    let mut buffer = buffer.lock().unwrap();
//	    if !buffer.is_empty() {
//	        self.write_through(&buffer[..])?;   // 一次性落盘
//	        buffer.clear();
//	    }
//	}
//
// 一帧只 flush 一次（indicatif 在 draw_to_term 末尾调），所以终端每帧只重绘一次 ——
// 这就是"丝滑"与"闪"的分界：带缓冲时清屏与重画合成一次写，不带缓冲时终端会看到
// 中间的清屏状态。不带缓冲时退化成原来"底层 writer 有 Flush 就调"的行为。
func (t *StdTerm) Flush() error {
	type flusher interface{ Flush() error }
	if !t.buffered {
		if f, ok := t.w.(flusher); ok {
			return f.Flush()
		}
		return nil
	}
	if t.w == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buf) == 0 {
		return nil
	}
	if err := t.writeThrough(string(t.buf)); err != nil {
		return err
	}
	t.buf = t.buf[:0]
	return nil
}

// writeThrough 是唯一的落盘出口：控制台走 WriteConsoleW，其余走原始字节。
func (t *StdTerm) writeThrough(s string) error {
	if t.handle != 0 && writeConsole(t.handle, s) {
		return nil
	}
	_, err := io.WriteString(t.w, s)
	return err
}
