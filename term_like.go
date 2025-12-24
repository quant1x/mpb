package mpb

import (
	"fmt"
	"io"
	"os"
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
}

// NewStdTerm 使用 stdout 创建默认终端（宽度 80，高度 20，可在需要时扩展）。
func NewStdTerm() *StdTerm {
	return &StdTerm{w: os.Stdout, width: 80, height: 20}
}

func (t *StdTerm) Width() uint16  { return t.width }
func (t *StdTerm) Height() uint16 { return t.height }

func (t *StdTerm) writeEscape(format string, a ...interface{}) error {
	if t.w == nil {
		return nil
	}
	_, err := fmt.Fprintf(t.w, format, a...)
	return err
}

func (t *StdTerm) MoveCursorUp(n int) error    { return t.writeEscape("\x1b[%dA", n) }
func (t *StdTerm) MoveCursorDown(n int) error  { return t.writeEscape("\x1b[%dB", n) }
func (t *StdTerm) MoveCursorRight(n int) error { return t.writeEscape("\x1b[%dC", n) }
func (t *StdTerm) MoveCursorLeft(n int) error  { return t.writeEscape("\x1b[%dD", n) }

func (t *StdTerm) WriteLine(s string) error { _, err := fmt.Fprintln(t.w, s); return err }
func (t *StdTerm) WriteStr(s string) error  { _, err := io.WriteString(t.w, s); return err }

// ClearLine 使用 ANSI 序列清除当前行并回到行首。
func (t *StdTerm) ClearLine() error {
	// ESC[2K 清除整行，ESC[0G 光标回到开头
	return t.writeEscape("\x1b[2K\x1b[0G")
}

// Flush 如果底层 writer 实现了 Flusher 接口则调用 Flush。
func (t *StdTerm) Flush() error {
	type flusher interface{ Flush() error }
	if f, ok := t.w.(flusher); ok {
		return f.Flush()
	}
	return nil
}
