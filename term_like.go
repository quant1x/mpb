// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"fmt"
	"io"
	"os"

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
	// width/height 只是**取不到尺寸时**的回落值（Rust 是 (24, DEFAULT_WIDTH)），
	// 不是"当前宽度"—— 终端窗口随时可缩放，尺寸必须每次现问。
	width  uint16
	height uint16
	// isTerm 对应 Rust 的 `console::Term::is_term()`：构造时探测到真实终端尺寸即为真终端。
	// 写入目标是文件/管道时是 false —— Rust 的 `is_hidden()` 会据此把整条进度条隐藏。
	isTerm bool
}

// NewStdTerm 使用 stdout 创建默认终端（宽度 80，高度 20，可在需要时扩展）。
func NewStdTerm() *StdTerm {
	w := os.Stdout
	if width, height, err := termpkg.GetSize(int(w.Fd())); err == nil {
		return &StdTerm{w: w, file: w, width: uint16(width), height: uint16(height), isTerm: true}
	}
	return &StdTerm{w: os.Stdout, file: os.Stdout, width: 80, height: 20}
}

// NewStderrTerm 使用 stderr 创建终端，对应 Rust 的 `Term::buffered_stderr()`。
// 进度条的默认绘制目标是 stderr，所以 `ProgressDrawTarget::stderr()` 走的是这个。
func NewStderrTerm() *StdTerm { return NewStdTermWithWriter(os.Stderr) }

// NewStdTermWithWriter creates a StdTerm that writes to the provided writer.
func NewStdTermWithWriter(w io.Writer) *StdTerm {
	// Try to derive terminal size if writer is an *os.File
	if f, ok := w.(*os.File); ok {
		if width, height, err := termpkg.GetSize(int(f.Fd())); err == nil {
			return &StdTerm{w: w, file: f, width: uint16(width), height: uint16(height), isTerm: true}
		}
		return &StdTerm{w: w, file: f, width: 80, height: 20}
	}
	return &StdTerm{w: w, width: 80, height: 20}
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
	_, err := fmt.Fprintf(t.w, format, a...)
	return err
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
func (t *StdTerm) WriteStr(s string) error { _, err := io.WriteString(t.w, s); return err }

// ClearLine 使用 ANSI 序列清除当前行并回到行首。
func (t *StdTerm) ClearLine() error {
	// Match console::Term.clear_line which emits CR + ESC[2K ("\r\x1b[2K").
	return t.writeEscape("\r\x1b[2K")
}

// Flush 如果底层 writer 实现了 Flusher 接口则调用 Flush。
func (t *StdTerm) Flush() error {
	type flusher interface{ Flush() error }
	if f, ok := t.w.(flusher); ok {
		return f.Flush()
	}
	return nil
}
