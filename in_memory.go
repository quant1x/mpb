package mpb

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// InMemoryTerm 是一个轻量的内存终端实现（对应 src/in_memory.rs）。
// 它维护固定大小的字符缓冲区、光标位置和操作历史，方便在测试中断言输出。
type InMemoryTerm struct {
	mu    sync.Mutex
	state *inMemoryTermState
}

// NewInMemoryTerm 使用指定的行数和列数创建一个新的内存终端。
func NewInMemoryTerm(rows uint16, cols uint16) *InMemoryTerm {
	if rows == 0 || cols == 0 {
		panic("rows and cols must be > 0")
	}
	return &InMemoryTerm{state: newInMemoryTermState(rows, cols)}
}

// Reset 重置终端内容为初始状态。
func (t *InMemoryTerm) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = newInMemoryTermState(t.state.height, t.state.width)
}

// Contents 返回当前屏幕的文本内容（去除行末空白，并去掉尾部空行）。
func (t *InMemoryTerm) Contents() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	lines := make([]string, 0, t.state.height)
	for r := 0; r < int(t.state.height); r++ {
		line := string(t.state.buf[r])
		// remove ANSI escape sequences that may have been written into the buffer
		// before trimming trailing spaces so the resulting plain text is clean.
		ansiRe := regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
		line = ansiRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		lines = append(lines, line)
	}

	// 从末端去掉空行
	i := len(lines) - 1
	for i >= 0 && lines[i] == "" {
		i--
	}
	if i < 0 {
		return ""
	}
	return strings.Join(lines[:i+1], "\n")
}

// ContentsFormatted 返回每行后面附加重置 ANSI 序列的字节表示（行为接近原始实现）。
func (t *InMemoryTerm) ContentsFormatted() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	reset := []byte("\x1b[m")
	var buf bytes.Buffer
	for r := 0; r < int(t.state.height); r++ {
		line := string(t.state.buf[r])
		line = strings.TrimRight(line, " ")
		buf.WriteString(line)
		buf.Write(reset)
		buf.WriteByte('\n')
	}
	// 移除最后一个换行，但保留最后的 reset
	out := buf.Bytes()
	if len(out) > 0 {
		// 每行至少有 reset 的长度 + 0..n chars + '\n', 所以安全地去掉最后一个 '\n'
		out = out[:len(out)-1]
	}
	return out
}

// MovesSinceLastCheck 返回自上次检查以来的移动历史（并清空历史）。
func (t *InMemoryTerm) MovesSinceLastCheck() string {
	t.mu.Lock()
	hist := t.state.history
	t.state.history = nil
	t.mu.Unlock()

	var b strings.Builder
	for _, m := range hist {
		switch m.kind {
		case "Up":
			fmt.Fprintf(&b, "Up(%d)\n", m.n)
		case "Down":
			fmt.Fprintf(&b, "Down(%d)\n", m.n)
		case "Left":
			fmt.Fprintf(&b, "Left(%d)\n", m.n)
		case "Right":
			fmt.Fprintf(&b, "Right(%d)\n", m.n)
		case "Str":
			// 模拟 Rust Debug 的格式化 Str("...")
			s := strings.ReplaceAll(m.s, "\"", "\\\"")
			fmt.Fprintf(&b, "Str(\"%s\")\n", s)
		case "NewLine":
			b.WriteString("NewLine\n")
		case "Clear":
			b.WriteString("Clear\n")
		case "Flush":
			b.WriteString("Flush\n")
		}
	}
	return b.String()
}

// 以下实现 TermLike 接口
func (t *InMemoryTerm) Width() uint16  { return t.state.width }
func (t *InMemoryTerm) Height() uint16 { return t.state.height }

func (t *InMemoryTerm) MoveCursorUp(n int) error {
	if n == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveUp(n))
	if n > t.state.cursorRow {
		t.state.cursorRow = 0
	} else {
		t.state.cursorRow -= n
	}
	if t.state.cursorRow < 0 {
		t.state.cursorRow = 0
	}
	return nil
}

func (t *InMemoryTerm) MoveCursorDown(n int) error {
	if n == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveDown(n))
	t.state.cursorRow += n
	if t.state.cursorRow >= int(t.state.height) {
		t.state.cursorRow = int(t.state.height) - 1
	}
	return nil
}

func (t *InMemoryTerm) MoveCursorRight(n int) error {
	if n == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveRight(n))
	t.state.cursorCol += n
	if t.state.cursorCol > int(t.state.width) {
		t.state.cursorCol = int(t.state.width)
	}
	return nil
}

func (t *InMemoryTerm) MoveCursorLeft(n int) error {
	if n == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveLeft(n))
	if n > t.state.cursorCol {
		t.state.cursorCol = 0
	} else {
		t.state.cursorCol -= n
	}
	return nil
}

func (t *InMemoryTerm) WriteLine(s string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveStr(s))
	t.state.writeStr(s)
	t.state.history = append(t.state.history, moveNewLine())
	// 新行：列归零，行增加
	t.state.cursorCol = 0
	t.state.cursorRow++
	if t.state.cursorRow >= int(t.state.height) {
		t.state.cursorRow = int(t.state.height) - 1
	}
	return nil
}

func (t *InMemoryTerm) WriteStr(s string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveStr(s))
	t.state.writeStr(s)
	return nil
}

func (t *InMemoryTerm) ClearLine() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveClear())
	r := t.state.cursorRow
	for c := 0; c < int(t.state.width); c++ {
		t.state.buf[r][c] = ' '
	}
	t.state.cursorCol = 0
	return nil
}

func (t *InMemoryTerm) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.history = append(t.state.history, moveFlush())
	return nil
}

// 内部状态与辅助类型
type inMemoryTermState struct {
	width     uint16
	height    uint16
	buf       [][]rune
	cursorRow int
	cursorCol int
	history   []move
}

func newInMemoryTermState(rows, cols uint16) *inMemoryTermState {
	buf := make([][]rune, rows)
	for i := range buf {
		row := make([]rune, cols)
		for j := range row {
			row[j] = ' '
		}
		buf[i] = row
	}
	return &inMemoryTermState{width: cols, height: rows, buf: buf, cursorRow: 0, cursorCol: 0, history: nil}
}

// writeStr 将字符串写入缓冲区，处理换行与自动换行（固定列宽）。
func (s *inMemoryTermState) writeStr(str string) {
	for _, r := range str {
		if r == '\n' || r == '\r' {
			// 视为新的逻辑行起始
			s.cursorCol = 0
			s.cursorRow++
			if s.cursorRow >= int(s.height) {
				s.cursorRow = int(s.height) - 1
			}
			continue
		}
		// 如果列已到达宽度，先换行
		if s.cursorCol >= int(s.width) {
			s.cursorRow++
			s.cursorCol = 0
			if s.cursorRow >= int(s.height) {
				s.cursorRow = int(s.height) - 1
			}
		}
		s.buf[s.cursorRow][s.cursorCol] = r
		s.cursorCol++
	}
}

// 历史移动类型的简化实现
type move struct {
	kind string
	n    int
	s    string
}

func moveUp(n int) move     { return move{kind: "Up", n: n} }
func moveDown(n int) move   { return move{kind: "Down", n: n} }
func moveLeft(n int) move   { return move{kind: "Left", n: n} }
func moveRight(n int) move  { return move{kind: "Right", n: n} }
func moveStr(s string) move { return move{kind: "Str", s: s} }
func moveNewLine() move     { return move{kind: "NewLine"} }
func moveClear() move       { return move{kind: "Clear"} }
func moveFlush() move       { return move{kind: "Flush"} }
