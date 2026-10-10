package mpb

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	vte "github.com/danielgatis/go-vte"
)

// Minimal vt100 performer + parser integration using github.com/danielgatis/go-vte
// This mirrors the Rust implementation where writes are fed into a parser which
// dispatches actions that update an internal screen buffer.

// InMemoryTerm 是一个轻量的内存终端实现（对应 src/in_memory.rs）。
//
// Rust 那边是 `Arc<Mutex<InMemoryTermState>>`（"so it can be freely cloned"），
// Go 里 state 是指针，*InMemoryTerm 之间天然共享同一份 state，语义等价。
type InMemoryTerm struct {
	state *inMemoryTermState
}

// NewInMemoryTerm 使用指定的行数和列数创建一个新的内存终端（Rust 里 rows/cols 必须 > 0）。
func NewInMemoryTerm(rows uint16, cols uint16) *InMemoryTerm {
	if rows == 0 || cols == 0 {
		panic("rows and cols must be > 0")
	}
	return &InMemoryTerm{state: newInMemoryTermState(rows, cols)}
}

// Reset 重置终端内容为初始状态。
// 对应 Rust 的 `*state = InMemoryTermState::new(..)`：改的是共享的 state 本体，
// 所以所有共享该 state 的 InMemoryTerm 都能看到重置（不是只换自己那根指针）。
func (t *InMemoryTerm) Reset() {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.reset(t.state.height, t.state.width)
}

// Contents 返回当前屏幕的文本内容（去掉行尾空白，并去掉尾部空行）。
//
// 对应 Rust 的 contents()：rev().skip_while(空).map(trim_end).rev().join("\n")。
// 注意顺序：Rust 那边 vt100 的 `screen().rows(..)` 返回的行本身就不带尾部空格，所以「判空」和
// 「trim」孰先孰后都一样；Go 的 buf 是空格填充的，必须先 trim 再判空，否则尾部空行会被当成非空。
func (t *InMemoryTerm) Contents() string {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()

	lines := make([]string, 0, int(t.state.height))
	for r := 0; r < int(t.state.height); r++ {
		lines = append(lines, strings.TrimRight(string(t.state.buf[r]), " "))
	}

	// 从末端去掉空行
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[:end], "\n")
}

// ContentsFormatted 返回每行后面附加重置 ANSI 序列的字节表示。
// 与 Rust 一致：先去掉尾部空行，再给每行拼 "\x1b[m" + '\n'，最后砍掉末尾那个 '\n'
// （保留末尾的 reset）。
func (t *InMemoryTerm) ContentsFormatted() []byte {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()

	// 与 contents() 同序：先 trim 行尾，再去掉尾部空行（Rust 的 rows_formatted 同样先 skip_while 空行）
	lines := make([]string, 0, int(t.state.height))
	for r := 0; r < int(t.state.height); r++ {
		lines = append(lines, strings.TrimRight(string(t.state.buf[r]), " "))
	}
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	lines = lines[:end]

	reset := []byte("\x1b[m")
	total := 0
	for _, l := range lines {
		total += len(l) + len(reset) + 1
	}

	var buf bytes.Buffer
	buf.Grow(total)
	for _, l := range lines {
		buf.WriteString(l)
		buf.Write(reset)
		buf.WriteByte('\n')
	}
	// Remove last newline again, but leave the reset code
	out := buf.Bytes()
	if len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out
}

// MovesSinceLastCheck 返回自上次检查以来的移动历史（并清空历史）。
// 与 Rust 一致：对每条 Move 用 `writeln!(s, "{line:?}")` 打印，即 Debug 格式。
func (t *InMemoryTerm) MovesSinceLastCheck() string {
	t.state.mu.Lock()
	hist := t.state.history
	t.state.history = nil
	t.state.mu.Unlock()

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
			// Rust 打印的是 Move::Str 的 Debug：外层引号由 Debug 加，内层做转义
			fmt.Fprintf(&b, "Str(\"%s\")\n", debugEscapeStr(m.s))
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

// debugEscapeStr 近似 Rust `{:?}` 对 String 的 Debug 输出：
// 转义引号/反斜杠/常见控制字符，其余不可打印字符用 `\u{..}` 表示。
func debugEscapeStr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u{%x}`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// 以下实现 TermLike 接口（与 Rust 的 `impl TermLike for InMemoryTerm` 一一对应）

func (t *InMemoryTerm) Width() uint16 {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	return t.state.width
}

func (t *InMemoryTerm) Height() uint16 {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	return t.state.height
}

// 说明：Rust 的 move_cursor_* 是「先记历史，再把转义序列喂给 parser，由 vt100 移动光标」；
// Go 这里直接改光标位置，效果等价（parser 不参与 history 记录，见 vtePerformer）。

func (t *InMemoryTerm) MoveCursorUp(n int) error {
	if n == 0 {
		return nil
	}
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.history = append(t.state.history, moveUp(n))
	if n > t.state.cursorRow {
		t.state.cursorRow = 0
	} else {
		t.state.cursorRow -= n
	}
	return nil
}

func (t *InMemoryTerm) MoveCursorDown(n int) error {
	if n == 0 {
		return nil
	}
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
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
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
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
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.history = append(t.state.history, moveLeft(n))
	if n > t.state.cursorCol {
		t.state.cursorCol = 0
	} else {
		t.state.cursorCol -= n
	}
	return nil
}

func (t *InMemoryTerm) WriteLine(s string) error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	// Rust：history.push(Str(s)); history.push(NewLine)
	t.state.history = append(t.state.history, moveStr(s))
	t.state.history = append(t.state.history, moveNewLine())

	t.state.writeStrLocked(s)
	t.state.writeStrLocked("\r\n")
	return nil
}

func (t *InMemoryTerm) WriteStr(s string) error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.history = append(t.state.history, moveStr(s))
	t.state.writeStrLocked(s)
	return nil
}

func (t *InMemoryTerm) ClearLine() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	// Rust：history.push(Clear) 然后把 "\r\x1b[2K" 喂给 parser
	t.state.history = append(t.state.history, moveClear())
	t.state.writeStrLocked("\r\x1b[2K")
	return nil
}

func (t *InMemoryTerm) Flush() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.history = append(t.state.history, moveFlush())
	return nil
}

// 内部状态与辅助类型

type inMemoryTermState struct {
	mu        sync.Mutex
	width     uint16
	height    uint16
	buf       [][]rune
	parser    *vte.Parser
	cursorRow int
	cursorCol int
	history   []move
}

func newInMemoryTermState(rows, cols uint16) *inMemoryTermState {
	s := &inMemoryTermState{width: cols, height: rows, cursorRow: 0, cursorCol: 0, history: nil}
	s.buf = make([][]rune, rows)
	for i := range s.buf {
		row := make([]rune, cols)
		for j := range row {
			row[j] = ' '
		}
		s.buf[i] = row
	}
	// create performer and parser
	s.parser = vte.NewParser(&vtePerformer{state: s})
	return s
}

// reset 原地重置为初始状态（Rust 是 `*state = InMemoryTermState::new(..)`，
// 所有共享该 state 的 InMemoryTerm 都看到重置）。
func (s *inMemoryTermState) reset(rows, cols uint16) {
	fresh := newInMemoryTermState(rows, cols)
	s.width = fresh.width
	s.height = fresh.height
	s.buf = fresh.buf
	s.cursorRow = 0
	s.cursorCol = 0
	s.history = nil
	// parser（及其 performer）必须重新挂到 s 自己身上：Rust 是整体换掉 state 本体，
	// 换完之后 InMemoryTerm 持有的就是新 state；Go 这里换的是 s 的内容，
	// 若沿用 fresh 的 parser，光标会更新到 fresh 上而 s.cursorRow 永远不动。
	s.parser = vte.NewParser(&vtePerformer{state: s})
}

// writeStrLocked 将字符串喂进 vte parser（对应 Rust 的 `self.parser.write_all(..)`）。
// 调用方必须已持有 s.mu：Rust 那边整个写操作都在 MutexGuard 下完成，
// 不会出现两个 goroutine 的写入相互穿插。
func (s *inMemoryTermState) writeStrLocked(str string) {
	for _, b := range []byte(str) {
		s.parser.Advance(b)
	}
}

// writeRuneLocked 写一个字符到缓冲区，处理换行与自动换行（固定列宽）。调用方必须持有 s.mu。
func (s *inMemoryTermState) writeRuneLocked(r rune) {
	if r == '\n' || r == '\r' {
		s.cursorCol = 0
		s.cursorRow++
		if s.cursorRow >= int(s.height) {
			s.cursorRow = int(s.height) - 1
		}
		return
	}
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

// vte performer implementation
//
// 注意：vte 的回调全部在「调用方持有 s.mu」的前提下执行（Rust 里 parser 也在 MutexGuard 下），
// 所以这里不能再加锁（Go 的 Mutex 不可重入，会死锁）。
// 另外：Rust 的 history 只由 TermLike 的那几个方法追加，parser 从不碰 history，
// 所以这里的 CSI 分支也不记录 history。
type vtePerformer struct {
	state *inMemoryTermState
}

func (p *vtePerformer) Print(r rune) { p.state.writeRuneLocked(r) }

func (p *vtePerformer) Execute(b byte) {
	switch b {
	case '\r':
		p.state.cursorCol = 0
	case '\n':
		p.state.cursorCol = 0
		p.state.cursorRow++
		if p.state.cursorRow >= int(p.state.height) {
			p.state.cursorRow = int(p.state.height) - 1
		}
	default:
		// ignore other control codes for now
	}
}

func (p *vtePerformer) Put(b byte) {
	// treat as a single-byte printable
	r, _ := utf8.DecodeRune([]byte{b})
	p.Print(r)
}

func (p *vtePerformer) Unhook()                                                           {}
func (p *vtePerformer) Hook(params [][]uint16, intermediates []byte, ignore bool, r rune) {}
func (p *vtePerformer) OscDispatch(params [][]byte, bellTerminated bool)                  {}

func (p *vtePerformer) CsiDispatch(params [][]uint16, intermediates []byte, ignore bool, r rune) {
	get := func(def int) int {
		if len(params) == 0 || len(params[0]) == 0 {
			return def
		}
		return int(params[0][0])
	}
	switch r {
	case 'A': // CUU
		n := get(1)
		if n > p.state.cursorRow {
			p.state.cursorRow = 0
		} else {
			p.state.cursorRow -= n
		}
	case 'B': // CUD
		p.state.cursorRow += get(1)
		if p.state.cursorRow >= int(p.state.height) {
			p.state.cursorRow = int(p.state.height) - 1
		}
	case 'C': // CUF
		p.state.cursorCol += get(1)
		if p.state.cursorCol > int(p.state.width) {
			p.state.cursorCol = int(p.state.width)
		}
	case 'D': // CUB
		n := get(1)
		if n > p.state.cursorCol {
			p.state.cursorCol = 0
		} else {
			p.state.cursorCol -= n
		}
	case 'H', 'f': // CUP
		row := 1
		col := 1
		if len(params) > 0 && len(params[0]) > 0 {
			row = int(params[0][0])
		}
		if len(params) > 1 && len(params[1]) > 0 {
			col = int(params[1][0])
		}
		// clamp and convert to 0-based
		if row <= 0 {
			row = 1
		}
		if col <= 0 {
			col = 1
		}
		r := row - 1
		c := col - 1
		if r >= int(p.state.height) {
			r = int(p.state.height) - 1
		}
		if c >= int(p.state.width) {
			c = int(p.state.width) - 1
		}
		p.state.cursorRow = r
		p.state.cursorCol = c
	case 'K': // EL：整行清除
		rr := p.state.cursorRow
		for i := 0; i < int(p.state.width); i++ {
			p.state.buf[rr][i] = ' '
		}
	default:
		// ignore other CSI for now
		_ = r
	}
}

func (p *vtePerformer) EscDispatch(intermediates []byte, ignore bool, b byte) {}

func (p *vtePerformer) SosPmApcDispatch(kind vte.SosPmApcKind, data []byte, bellTerminated bool) {}

// 历史移动类型（对应 Rust 的 `enum Move`）
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
