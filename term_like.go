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
	w      io.Writer
	width  uint16
	height uint16
	// isTerm 对应 Rust 的 `console::Term::is_term()`：构造时探测到真实终端尺寸即为真终端。
	// 写入目标是文件/管道时是 false —— Rust 的 `is_hidden()` 会据此把整条进度条隐藏。
	isTerm bool
}

// NewStdTerm 使用 stdout 创建默认终端（宽度 80，高度 20，可在需要时扩展）。
func NewStdTerm() *StdTerm {
	w := os.Stdout
	if width, height, err := termpkg.GetSize(int(os.Stdout.Fd())); err == nil {
		return &StdTerm{w: w, width: uint16(width), height: uint16(height), isTerm: true}
	}
	return &StdTerm{w: os.Stdout, width: 80, height: 20}
}

// NewStderrTerm 使用 stderr 创建终端，对应 Rust 的 `Term::buffered_stderr()`。
// 进度条的默认绘制目标是 stderr，所以 `ProgressDrawTarget::stderr()` 走的是这个。
func NewStderrTerm() *StdTerm { return NewStdTermWithWriter(os.Stderr) }

// NewStdTermWithWriter creates a StdTerm that writes to the provided writer.
func NewStdTermWithWriter(w io.Writer) *StdTerm {
	// Try to derive terminal size if writer is an *os.File
	if f, ok := w.(*os.File); ok {
		if width, height, err := termpkg.GetSize(int(f.Fd())); err == nil {
			return &StdTerm{w: w, width: uint16(width), height: uint16(height), isTerm: true}
		}
	}
	return &StdTerm{w: w, width: 80, height: 20}
}

// IsTerm 对应 Rust 的 `console::Term::is_term()`：是不是真实终端（而非文件/管道）。
func (t *StdTerm) IsTerm() bool { return t.isTerm }

func (t *StdTerm) Width() uint16  { return t.width }
func (t *StdTerm) Height() uint16 { return t.height }

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
