package mpb

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

type ProgressStyle struct {
	tickStrings   []string
	progressChars []string
	template      Template
	charWidth     int
	tabWidth      int
	// formatMap maps a key to a formatter that returns rendered text and
	// an optional Style to apply to that text. The Style is applied with
	// higher precedence than placeholder-level style when provided by the
	// formatter itself.
	// formatMap stores per-key ProgressTracker implementations.
	formatMap map[string]ProgressTracker
}

// ProgressTracker mirrors Rust's trait: it can be stateful, tick/reset,
// and write its representation into a writer.
type ProgressTracker interface {
	Clone() ProgressTracker
	Tick(state *ProgressState, now time.Time)
	Reset(state *ProgressState, now time.Time)
	Write(state *ProgressState, w *tabWriter)
}

// tabWriter expands tabs to spaces when writing.
type tabWriter struct {
	b        *strings.Builder
	tabWidth int
}

func (t *tabWriter) WriteString(s string) {
	if t.tabWidth <= 0 || !strings.Contains(s, "\t") {
		t.b.WriteString(s)
		return
	}
	for _, r := range s {
		if r == '\t' {
			t.b.WriteString(strings.Repeat(" ", t.tabWidth))
		} else {
			t.b.WriteRune(r)
		}
	}
}

// funcTracker wraps a simple function into a ProgressTracker
type funcTracker struct {
	f func(*ProgressState, *tabWriter)
}

func (ft *funcTracker) Clone() ProgressTracker               { return &funcTracker{f: ft.f} }
func (ft *funcTracker) Tick(_ *ProgressState, _ time.Time)   {}
func (ft *funcTracker) Reset(_ *ProgressState, _ time.Time)  {}
func (ft *funcTracker) Write(s *ProgressState, w *tabWriter) { ft.f(s, w) }

func DefaultProgressStyleBar() ProgressStyle {
	tpl, _ := Template_from_str("{wide_bar} {pos}/{len}")
	return NewProgressStyle(tpl)
}

func DefaultProgressStyleSpinner() ProgressStyle {
	tpl, _ := Template_from_str("{spinner} {msg}")
	return NewProgressStyle(tpl)
}

// NewProgressStyle constructs a ProgressStyle using the provided template and
// initializes other defaults (tick strings, progress chars, char width, tab width).
// This mirrors Rust's `Self::new(template)` constructor so callers doing
// `ProgressStyle::with_template(template)` will get the same initialization order.
func NewProgressStyle(tpl Template) ProgressStyle {
	ps := ProgressStyle{}
	ps.template = tpl
	// default progress chars (filled, empty)
	ps.progressChars = []string{"█", "░"}
	// tick strings as individual runes (approx grapheme clusters)
	// 与 Rust 的 `tick_strings` 逐字一致（30 个）：最后一个是"完成态"用的终帧（空格），
	// 其余 29 个由 `tick_strings[idx % (len-1)]` 循环取用。
	ticks := "⠁⠁⠉⠙⠚⠒⠂⠂⠒⠲⠴⠤⠄⠄⠤⠠⠠⠤⠦⠖⠒⠐⠐⠒⠓⠋⠉⠈⠈ "
	ps.tickStrings = make([]string, 0, len(ticks))
	for _, r := range ticks {
		ps.tickStrings = append(ps.tickStrings, string(r))
	}
	if len(ps.progressChars) > 0 {
		ps.charWidth = measureTextWidth(ps.progressChars[0])
		if ps.charWidth == 0 {
			ps.charWidth = 1
		}
	} else {
		ps.charWidth = 1
	}
	ps.tabWidth = DEFAULT_TAB_WIDTH
	if ps.formatMap == nil {
		ps.formatMap = make(map[string]ProgressTracker)
	}
	return ps
}

func (ps *ProgressStyle) SetTabWidth(w int) { ps.tabWidth = w; ps.template.SetTabWidth(w) }

// SetForStderr updates placeholder-level styles to their stderr variants.
func (ps *ProgressStyle) SetForStderr() {
	for i := range ps.template.Parts {
		if !ps.template.Parts[i].IsPlaceholder {
			continue
		}
		if ps.template.Parts[i].Style != nil {
			// for parity: use same spec as stderr-specific spec
			ps.template.Parts[i].Style.SpecErr = ps.template.Parts[i].Style.Spec
		}
		if ps.template.Parts[i].AltStyle != nil {
			ps.template.Parts[i].AltStyle.SpecErr = ps.template.Parts[i].AltStyle.Spec
		}
	}
}

func (ps *ProgressStyle) currentTickStr(state *ProgressState) string {
	if state.IsFinished() {
		if len(ps.tickStrings) == 0 {
			return ""
		}
		return ps.tickStrings[len(ps.tickStrings)-1]
	}
	if len(ps.tickStrings) <= 1 {
		if len(ps.tickStrings) == 1 {
			return ps.tickStrings[0]
		}
		return ""
	}
	return ps.tickStrings[int(state.tick)%(len(ps.tickStrings)-1)]
}

// FormatState 将 ProgressState 格式化为若干行及对应文本内容（返回 line types 与 contents）。
// 这是一个简化实现，支持常见占位符替换并返回渲染文本，便于 DrawStateStub.contents 填充。
func (ps *ProgressStyle) FormatState(state *ProgressState, targetWidth int, forStderr bool) ([]LineType, []string) {
	pos := state.Pos()
	var length uint64
	if state.Len() != nil {
		length = *state.Len()
	} else {
		length = pos
	}

	// Build lines respecting explicit newlines in template parts.
	var lines []string
	lines = append(lines, "")
	// Rust 的 format_state 里 `wide` 是**单个** `Option<WideElement>`：
	// 遇到 wide_bar / wide_msg 就覆盖（后者胜），而且 push_line **不会**把它重置。
	// Go 之前是"每个 wide 占位收进切片、再按行号取"，多占位/跨行时会错位。
	var wide *TemplatePart

	for idx, part := range ps.template.Parts {
		if !part.IsPlaceholder {
			// choose between already-expanded text or lazily-expand using
			// originalTexts/currentTabWidth if available (mirrors Rust behavior)
			text := part.Text
			if ps.template.originalTexts != nil && idx < len(ps.template.originalTexts) && ps.template.originalTexts[idx] != "" {
				tab := strings.Repeat(" ", ps.template.currentTabWidth)
				text = strings.ReplaceAll(ps.template.originalTexts[idx], "\t", tab)
			}
			// handle newlines inside text
			segs := strings.Split(text, "\n")
			for i, seg := range segs {
				lines[len(lines)-1] += seg
				if i != len(segs)-1 {
					lines = append(lines, "")
				}
			}
			continue
		}
		key := part.Key

		// Rust 的收尾（style.rs:381-400）：**所有** key 都走同一条，不分彼此：
		//
		//	match width {
		//	    Some(width) => {
		//	        let padded = PaddedStringDisplay { str: &buf, width, align, truncate };
		//	        match style { Some(s) => s.apply_to(padded), None => padded }
		//	    }
		//	    None => match style { Some(s) => s.apply_to(&buf), None => buf },
		//	}
		//
		// Go 之前只给 msg/prefix/pos/len/percent 补白，其余 key 的宽度说明符被静默忽略
		// （`{bytes:>10}` 不补白、`{elapsed:>8}` 不补白……）。现在统一。
		emit := func(buf string) string {
			if part.Width > 0 {
				buf = padAndTruncate(buf, part.Width, part.Align, part.Truncate)
			}
			// Rust：非 bar 的 key 只认 `style`（alt_style 只有 format_bar 用得到）
			if part.Style != nil {
				buf = part.Style.ApplyFor(buf, forStderr)
			}
			return buf
		}

		// custom formatter hook
		if ps.formatMap != nil {
			if tracker, ok := ps.formatMap[key]; ok {
				var b strings.Builder
				tw := &tabWriter{b: &b, tabWidth: ps.tabWidth}
				tracker.Write(state, tw)
				lines[len(lines)-1] += emit(b.String())
				continue
			}
		}

		var buf string
		switch key {
		case "wide_bar":
			buf = "\x00"
			wp := part
			wide = &wp
		case "wide_msg":
			buf = "\x00"
			wp := part
			wide = &wp
		case "bar":
			// Rust：`width.unwrap_or(20)` —— 没给宽度说明符时固定 20 列，不是整屏宽度
			w := part.Width
			if w == 0 {
				w = 20
			}
			buf = ps.formatBar(state.Fraction(), w, part.AltStyle, forStderr)
		case "spinner":
			buf = ps.currentTickStr(state)
		case "msg":
			buf = state.message.Expanded()
		case "prefix":
			buf = state.prefix.Expanded()
		case "pos":
			buf = fmt.Sprintf("%d", pos)
		case "len":
			buf = fmt.Sprintf("%d", length)
		case "percent":
			buf = fmt.Sprintf("%.0f", state.Fraction()*100.0)
		case "percent_precise":
			buf = fmt.Sprintf("%.*f", 3, state.Fraction()*100.0)
		case "human_pos":
			buf = fmt.Sprintf("%v", HumanCount(pos))
		case "human_len":
			buf = fmt.Sprintf("%v", HumanCount(length))
		case "bytes":
			buf = fmt.Sprintf("%v", HumanBytes(pos))
		case "total_bytes":
			buf = fmt.Sprintf("%v", HumanBytes(length))
		case "decimal_bytes":
			buf = fmt.Sprintf("%v", DecimalBytes(pos))
		case "decimal_total_bytes":
			buf = fmt.Sprintf("%v", DecimalBytes(length))
		case "binary_bytes":
			buf = fmt.Sprintf("%v", BinaryBytes(pos))
		case "binary_total_bytes":
			buf = fmt.Sprintf("%v", BinaryBytes(length))
		case "elapsed_precise":
			// Rust：`FormattedDuration(state.elapsed())`（之前误用了 duration = elapsed+eta）
			buf = fmt.Sprintf("%v", FormattedDuration(state.Elapsed()))
		case "elapsed":
			// Rust：`{:#}` → 短式（"5s"，而非 "5 seconds"）
			buf = humanDurationFmt(state.Elapsed(), true)
		case "duration_precise":
			buf = fmt.Sprintf("%v", FormattedDuration(state.Duration()))
		case "duration":
			buf = humanDurationFmt(state.Duration(), true)
		case "eta_precise":
			buf = fmt.Sprintf("%v", FormattedDuration(state.Eta()))
		case "eta":
			buf = humanDurationFmt(state.Eta(), true)
		case "per_sec":
			// Rust：有宽度时 `{:.width$}/s`（宽度当精度），且恒带 "/s" 后缀
			if part.Width > 0 {
				buf = fmt.Sprintf("%.*v/s", part.Width, HumanFloatCount(state.PerSec()))
			} else {
				buf = fmt.Sprintf("%v/s", HumanFloatCount(state.PerSec()))
			}
		case "bytes_per_sec":
			buf = fmt.Sprintf("%v/s", HumanBytes(uint64(state.PerSec())))
		case "decimal_bytes_per_sec":
			buf = fmt.Sprintf("%v/s", DecimalBytes(uint64(state.PerSec())))
		case "binary_bytes_per_sec":
			buf = fmt.Sprintf("%v/s", BinaryBytes(uint64(state.PerSec())))
		default:
			// Rust：`_ => ()` —— 未识别的 key 输出空串（不是把字面量 `{key}` 打出来）
		}
		lines[len(lines)-1] += emit(buf)
	}

	// 对应 Rust 的 push_line：解 wide 占位，再按 '\n' 拆成多行（消息里带换行时也算行数）
	results := make([]string, 0, len(lines))
	pushLine := func(expanded string) {
		for _, l := range strings.Split(expanded, "\n") {
			results = append(results, l)
		}
	}
	for _, line := range lines {
		if !strings.Contains(line, "\x00") || wide == nil {
			pushLine(line)
			continue
		}
		// Rust 的 `left`：
		//   width - measure_text_width(cur.lines().find(|line| line.contains('\x00')) 去掉 \x00)
		src := line
		if strings.Contains(line, "\n") {
			for _, sl := range strings.Split(line, "\n") {
				if strings.Contains(sl, "\x00") {
					src = sl
					break
				}
			}
		}
		left := targetWidth - measureTextWidth(strings.ReplaceAll(src, "\x00", ""))
		if left < 0 {
			left = 0
		}
		// 注意：Rust 的 `WideElement::expand` **不碰 style** —— style 早在占位符那一层
		// 就套在被补白过的 `\x00` 外面了（`\x1b[31m\x00\x1b[0m`），这里把 `\x00` 换掉即可，
		// 换完正好是 `\x1b[31m<bar>\x1b[0m`。Go 之前在这里又套一次，会套两层。
		if wide.Key == "wide_msg" {
			// Rust：`PaddedStringDisplay { truncate: true }` —— wide_msg 的截断是硬编码打开的
			rendered := padAndTruncate(state.message.Expanded(), left, wide.Align, true)
			// Rust：若 `\x00` 就是行末最后一个字节，则 trim_end 去掉右侧补白
			if strings.HasSuffix(line, "\x00") {
				rendered = strings.TrimRight(rendered, " ")
			}
			pushLine(strings.ReplaceAll(line, "\x00", rendered))
		} else {
			// Rust 只有 format_bar 用得到 alt_style（bar / wide_bar）
			bar := ps.formatBar(state.Fraction(), left, wide.AltStyle, forStderr)
			pushLine(strings.ReplaceAll(line, "\x00", bar))
		}
	}

	// All template-generated lines are treated as bar lines here for simplicity.
	ltypes := make([]LineType, len(results))
	for i := range ltypes {
		ltypes[i] = LineBar
	}
	return ltypes, results
}

// padAndTruncate returns a string that when measured occupies exactly width columns
// according to measureTextWidth. If truncate is true, content is truncated to fit.
func padAndTruncate(s string, width int, align rune, truncate bool) string {
	if width <= 0 {
		return ""
	}
	cols := measureTextWidth(s)
	if cols == width {
		return s
	}
	if cols > width {
		if !truncate {
			return s
		}
		// ANSI-aware truncation: pick visible-range (start,end) based on align
		// then extract substring preserving ANSI sequences.
		// compute visible start/end positions (in columns)
		var startCol int
		switch align {
		case '^':
			startCol = (cols - width) / 2
		case '>':
			startCol = cols - width
		default:
			startCol = 0
		}
		if startCol < 0 {
			startCol = 0
		}
		endCol := startCol + width
		s = ansiSlice(s, startCol, endCol)
		cols = measureTextWidth(s)
	}
	// pad
	pad := width - cols
	leftPad := 0
	rightPad := 0
	switch align {
	case '^':
		leftPad = pad / 2
		rightPad = pad - leftPad
	case '>':
		leftPad = pad
		rightPad = 0
	default:
		leftPad = 0
		rightPad = pad
	}
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

// ansiSlice returns the substring of s that corresponds to visible columns
// in the range [start, end), preserving ANSI escape sequences surrounding
// the included visible text.
func ansiSlice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return ""
	}

	// split into segments: ANSI sequences and normal text
	segments := make([]struct {
		text   string
		isAnsi bool
	}, 0)
	run := ""
	inAnsi := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inAnsi {
			if c == '\x1b' {
				// flush run
				if run != "" {
					segments = append(segments, struct {
						text   string
						isAnsi bool
					}{run, false})
					run = ""
				}
				inAnsi = true
				run += string(c)
			} else {
				run += string(c)
			}
		} else {
			run += string(c)
			// end of CSI: letter in A-Za-z
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				segments = append(segments, struct {
					text   string
					isAnsi bool
				}{run, true})
				run = ""
				inAnsi = false
			}
		}
	}
	if run != "" {
		segments = append(segments, struct {
			text   string
			isAnsi bool
		}{run, inAnsi})
	}

	// 对应 Rust 的 write_ansi_range：
	//   for (s, is_ansi) in AnsiCodeIterator::new(text) {
	//       if is_ansi { formatter.write_str(s)?; continue; }        // ANSI 原样写出（含尾部的）
	//       else if pos >= end { continue; }
	//       for c in s.chars() {
	//           if start <= pos && pos + c_width <= end { write_char(c) }   // 跨界宽字符整丢
	//           pos += c_width;
	//           if pos > end { break }
	//       }
	//   }
	var out strings.Builder
	col := 0
	for _, seg := range segments {
		if seg.isAnsi {
			// Rust 对 ANSI 段是无条件原样写出（即使已经在可见范围之外）
			out.WriteString(seg.text)
			continue
		}
		for _, r := range []rune(seg.text) {
			w := runewidth.RuneWidth(r)
			if w < 0 {
				w = 0
			}
			// Rust：`start <= pos && pos + c_width <= end` —— 宽字符一旦跨界就整个不写，
			// Go 之前是 `col < end`，跨界时也写，会多占一列。
			if start <= col && col+w <= end {
				out.WriteRune(r)
			}
			col += w
			if col > end {
				break
			}
		}
		if col > end {
			break
		}
	}
	return out.String()
}

// applyAnsiStyle 对应 Rust 的 console::Style::from_dotted_str + Display：
//
//	按 '.' 切分逐个施加，未知 token 忽略。token 表（console src/utils.rs）：
//	  black red green yellow blue magenta cyan white          —— 前景色（ansi_num 0..7）
//	  bright                                                  —— 前景转亮（不是取色）
//	  on_black .. on_white                                    —— **背景色**
//	  on_bright                                               —— 背景转亮
//	  bold dim underlined blink blink_fast reverse hidden strikethrough
//	  9                                                       —— 前景 256 色
//	  on_12                                                   —— 背景 256 色
//	  #rrggbb / on_#rrggbb                                    —— 真彩色（前景/背景）
//
// 输出顺序与 Rust 的 impl_fmt 一致：前景 → 背景 → 各属性（各占一条转义）→ 文本 → 复位。
// 注：Rust 的表里**没有** `italic`（它会落进"解析成数字"的兜底分支被忽略），
// `underline` 的拼写是 `underlined`。
func applyAnsiStyle(text, spec string) string {
	if spec == "" || text == "" {
		return text
	}
	ds := parseDottedStyle(spec)
	return ds.render(text)
}

// dottedStyle 是 console::Style 的最小等价物（fg/bg/属性 + 两个 bright 开关）。
type dottedStyle struct {
	fg       int  // 0..7（ansi_num），仅当 fgSet && !fg256 && !fgRGB
	fgSet    bool // 是否设过前景
	fg256    int  // 256 色号，>=0 表示设过
	fgRGB    [3]int
	fgIsRGB  bool
	fgBright bool
	bg       int
	bgSet    bool
	bg256    int
	bgRGB    [3]int
	bgIsRGB  bool
	bgBright bool
	attrBits uint16 // 与 Rust 的 Attributes(u16) 同构：bit i 对应 Attribute::MAP[i]
}

// Attribute::MAP 的顺序（console src/utils.rs），ansi 编号 = bit+1
const (
	attrBold = 1 << iota
	attrDim
	attrItalic
	attrUnderlined
	attrBlink
	attrBlinkFast
	attrReverse
	attrHidden
	attrStrikeThrough
)

func parseDottedStyle(spec string) *dottedStyle {
	d := &dottedStyle{fg256: -1, bg256: -1}
	for _, part := range strings.Split(spec, ".") {
		switch part {
		case "black":
			d.fg, d.fgSet = 0, true
		case "red":
			d.fg, d.fgSet = 1, true
		case "green":
			d.fg, d.fgSet = 2, true
		case "yellow":
			d.fg, d.fgSet = 3, true
		case "blue":
			d.fg, d.fgSet = 4, true
		case "magenta":
			d.fg, d.fgSet = 5, true
		case "cyan":
			d.fg, d.fgSet = 6, true
		case "white":
			d.fg, d.fgSet = 7, true
		case "bright":
			d.fgBright = true
		case "on_black":
			d.bg, d.bgSet = 0, true
		case "on_red":
			d.bg, d.bgSet = 1, true
		case "on_green":
			d.bg, d.bgSet = 2, true
		case "on_yellow":
			d.bg, d.bgSet = 3, true
		case "on_blue":
			d.bg, d.bgSet = 4, true
		case "on_magenta":
			d.bg, d.bgSet = 5, true
		case "on_cyan":
			d.bg, d.bgSet = 6, true
		case "on_white":
			d.bg, d.bgSet = 7, true
		case "on_bright":
			d.bgBright = true
		case "bold":
			d.attrBits |= attrBold
		case "dim":
			d.attrBits |= attrDim
		case "underlined":
			d.attrBits |= attrUnderlined
		case "blink":
			d.attrBits |= attrBlink
		case "blink_fast":
			d.attrBits |= attrBlinkFast
		case "reverse":
			d.attrBits |= attrReverse
		case "hidden":
			d.attrBits |= attrHidden
		case "strikethrough":
			d.attrBits |= attrStrikeThrough
		default:
			// Rust 的兜底分支：on_#rrggbb（长度 10、纯 ASCII）→ 背景真彩色
			if strings.HasPrefix(part, "on_#") && len(part) == 10 && isASCII(part) {
				if r, g, b, ok := parseHex2(part[4:6], part[6:8], part[8:10]); ok {
					d.bgRGB = [3]int{r, g, b}
					d.bgIsRGB = true
				}
				continue
			}
			// #rrggbb（长度 7、纯 ASCII）→ 前景真彩色
			if strings.HasPrefix(part, "#") && len(part) == 7 && isASCII(part) {
				if r, g, b, ok := parseHex2(part[1:3], part[3:5], part[5:7]); ok {
					d.fgRGB = [3]int{r, g, b}
					d.fgIsRGB = true
				}
				continue
			}
			// on_<n> → 背景 256 色
			if strings.HasPrefix(part, "on_") {
				if n, err := strconv.Atoi(part[3:]); err == nil && n >= 0 && n <= 255 {
					d.bg256 = n
				}
				continue
			}
			// <n> → 前景 256 色
			if n, err := strconv.Atoi(part); err == nil && n >= 0 && n <= 255 {
				d.fg256 = n
				continue
			}
			// 未知 token：Rust 是 `continue`（忽略）
		}
	}
	return d
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func parseHex2(a, b, c string) (int, int, int, bool) {
	ra, err := strconv.ParseUint(a, 16, 8)
	if err != nil {
		return 0, 0, 0, false
	}
	rb, err := strconv.ParseUint(b, 16, 8)
	if err != nil {
		return 0, 0, 0, false
	}
	rc, err := strconv.ParseUint(c, 16, 8)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(ra), int(rb), int(rc), true
}

// render 对应 Rust 的 impl_fmt：前景 → 背景 → 属性 → 文本 → \x1b[0m（有格式化才写复位）。
func (d *dottedStyle) render(text string) string {
	var out strings.Builder
	reset := false

	// 前景
	switch {
	case d.fgIsRGB:
		fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm", d.fgRGB[0], d.fgRGB[1], d.fgRGB[2])
		reset = true
	case d.fg256 >= 0:
		fmt.Fprintf(&out, "\x1b[38;5;%dm", d.fg256)
		reset = true
	case d.fgSet:
		if d.fgBright {
			fmt.Fprintf(&out, "\x1b[38;5;%dm", d.fg+8)
		} else {
			fmt.Fprintf(&out, "\x1b[%dm", d.fg+30)
		}
		reset = true
	}

	// 背景
	switch {
	case d.bgIsRGB:
		fmt.Fprintf(&out, "\x1b[48;2;%d;%d;%dm", d.bgRGB[0], d.bgRGB[1], d.bgRGB[2])
		reset = true
	case d.bg256 >= 0:
		fmt.Fprintf(&out, "\x1b[48;5;%dm", d.bg256)
		reset = true
	case d.bgSet:
		if d.bgBright {
			fmt.Fprintf(&out, "\x1b[48;5;%dm", d.bg+8)
		} else {
			fmt.Fprintf(&out, "\x1b[%dm", d.bg+40)
		}
		reset = true
	}

	// 属性：Rust 的 Attributes::Display 按 bit 升序各写一条 `\x1b[{bit+1}m`
	for bit := 0; bit < 9; bit++ {
		if d.attrBits&(1<<uint(bit)) != 0 {
			fmt.Fprintf(&out, "\x1b[%dm", bit+1)
			reset = true
		}
	}

	out.WriteString(text)
	if reset {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// Template -----------------------------------------------------------------
type TemplatePart struct {
	IsPlaceholder bool
	Text          string
	Key           string
	Align         rune // '<' '^' '>' or 0
	Width         int  // 0 means none
	Truncate      bool
	Style         *Style
	AltStyle      *Style
}

type Template struct {
	Parts           []TemplatePart
	originalTexts   []string // store original unexpanded text for parts with tabs
	currentTabWidth int      // track current tab width (0 means not set yet)
}

func (t *Template) SetTabWidth(newTabWidth int) {
	if newTabWidth <= 0 || t.currentTabWidth == newTabWidth {
		return
	}

	tab := strings.Repeat(" ", newTabWidth)

	// First call: scan parts and store original text for those with tabs
	if t.originalTexts == nil {
		t.originalTexts = make([]string, len(t.Parts))
		for i := range t.Parts {
			if !t.Parts[i].IsPlaceholder && strings.Contains(t.Parts[i].Text, "\t") {
				t.originalTexts[i] = t.Parts[i].Text
				t.Parts[i].Text = strings.ReplaceAll(t.Parts[i].Text, "\t", tab)
			}
		}
		t.currentTabWidth = newTabWidth
		return
	}

	// Subsequent calls: only re-expand parts that originally had tabs
	for i := range t.Parts {
		if t.originalTexts[i] != "" {
			t.Parts[i].Text = strings.ReplaceAll(t.originalTexts[i], "\t", tab)
		}
	}
	t.currentTabWidth = newTabWidth
}

// templateState 对应 Rust 的 `enum State`：
// Literal / MaybeOpen / Key / Align / Width / FirstStyle / AltStyle / DoubleClose
type templateState int

const (
	stateLiteral templateState = iota
	stateMaybeOpen
	stateKey
	stateAlign
	stateWidth
	stateFirstStyle
	stateAltStyle
	stateDoubleClose
)

func (s templateState) String() string {
	switch s {
	case stateLiteral:
		return "Literal"
	case stateMaybeOpen:
		return "MaybeOpen"
	case stateKey:
		return "Key"
	case stateAlign:
		return "Align"
	case stateWidth:
		return "Width"
	case stateFirstStyle:
		return "FirstStyle"
	case stateAltStyle:
		return "AltStyle"
	case stateDoubleClose:
		return "DoubleClose"
	}
	return "?"
}

// TemplateError 对应 Rust 的 `pub struct TemplateError { state, next }`
// （Rust 的 Display：`TemplateError: unexpected character {:?} in state {:?}`）。
type TemplateError struct {
	State templateState
	Next  rune
}

func (e *TemplateError) Error() string {
	return fmt.Sprintf("TemplateError: unexpected character %q in state %v", e.Next, e.State)
}

// Template_from_str_with_tab_width 是 Rust 的 Template::from_str_with_tab_width 的直译：
// 同一个状态机、同一套分支，包括
//   - `{{` → 字面量 `{`、`}}` → 字面量 `}`（Rust 的 MaybeOpen/DoubleClose 两支）
//   - 键名位置遇到 ASCII 空白 → 回退成字面量 "{...}"（Rust 的 backtrack）
//   - `{key!}` / `{key:>10!}` 的截断位
//   - 无法匹配的状态+字符 → 返回 TemplateError（Rust 是 Err，Go 之前恒返回 nil）
func Template_from_str_with_tab_width(s string, tab_width int) (Template, error) {
	if tab_width <= 0 {
		tab_width = DEFAULT_TAB_WIDTH
	}
	parts := make([]TemplatePart, 0)
	buf := ""
	state := stateLiteral

	appendLiteral := func(text string) {
		parts = append(parts, TemplatePart{IsPlaceholder: false, Text: text})
	}
	// Rust 的 `if let Some(TemplatePart::Placeholder{..}) = parts.last_mut()`
	lastPlaceholder := func() *TemplatePart {
		if len(parts) == 0 {
			return nil
		}
		p := &parts[len(parts)-1]
		if !p.IsPlaceholder {
			return nil
		}
		return p
	}

	for _, c := range s {
		var newState templateState
		var keep rune // 需要追加到 buf 的字符（Rust 的 Some(c)）
		var keepOK bool

		switch state {
		case stateLiteral:
			switch c {
			case '{':
				newState = stateMaybeOpen
			case '\n':
				// Rust：先收掉 buf，再 push NewLine。
				// Go 没有 NewLine 变体：字面量 "\n" 在 FormatState 里会被切成新的一行。
				if buf != "" {
					appendLiteral(buf)
					buf = ""
				}
				appendLiteral("\n")
				newState = stateLiteral
			case '}':
				newState = stateDoubleClose
				keep, keepOK = c, true
			default:
				newState = stateLiteral
				keep, keepOK = c, true
			}

		case stateDoubleClose:
			if c == '}' {
				// `}}` → 字面量 `}`（buf 里已经有一个 '}' 了，这个 '}' 被吞掉）
				newState = stateLiteral
			} else {
				return Template{}, &TemplateError{State: state, Next: c}
			}

		case stateMaybeOpen:
			switch {
			case c == '{':
				// `{{` → 字面量 `{`
				newState = stateLiteral
				keep, keepOK = c, true
			case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x0b' || c == '\x0c':
				// Rust：键名位置出现空白 → 回退，整段当字面量 "{buf + c}"
				lit := "{" + buf + string(c)
				buf = ""
				appendLiteral(lit)
				newState = stateLiteral
			case c != '}' && c != ':':
				newState = stateKey
				keep, keepOK = c, true
			default:
				// `{}`、`{:...}`：Rust 没有匹配分支 → Err
				return Template{}, &TemplateError{State: state, Next: c}
			}

		case stateKey:
			switch {
			case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x0b' || c == '\x0c':
				lit := "{" + buf + string(c)
				buf = ""
				appendLiteral(lit)
				newState = stateLiteral
			case c == '}' || c == ':':
				// 由下面的「跃迁块」统一收尾占位符
				if c == '}' {
					newState = stateLiteral
				} else {
					newState = stateAlign
				}
			case c == '!' && buf != "":
				parts = append(parts, TemplatePart{
					IsPlaceholder: true,
					Key:           buf,
					Align:         '<',
					Truncate:      true,
				})
				buf = ""
				newState = stateWidth
			default:
				newState = stateKey
				keep, keepOK = c, true
			}

		case stateAlign:
			switch {
			case c == '<' || c == '^' || c == '>':
				if p := lastPlaceholder(); p != nil {
					p.Align = c
				}
				newState = stateWidth
			case c >= '0' && c <= '9':
				newState = stateWidth
				keep, keepOK = c, true
			case c == '!':
				if p := lastPlaceholder(); p != nil {
					p.Truncate = true
				}
				newState = stateWidth
			case c == '.':
				newState = stateFirstStyle
			case c == '}':
				newState = stateLiteral
			default:
				return Template{}, &TemplateError{State: state, Next: c}
			}

		case stateWidth:
			switch {
			case c >= '0' && c <= '9':
				newState = stateWidth
				keep, keepOK = c, true
			case c == '!':
				if p := lastPlaceholder(); p != nil {
					p.Truncate = true
				}
				newState = stateWidth
			case c == '.':
				newState = stateFirstStyle
			case c == '}':
				newState = stateLiteral
			default:
				return Template{}, &TemplateError{State: state, Next: c}
			}

		case stateFirstStyle:
			switch c {
			case '/':
				newState = stateAltStyle
			case '}':
				newState = stateLiteral
			default:
				newState = stateFirstStyle
				keep, keepOK = c, true
			}

		case stateAltStyle:
			if c == '}' {
				newState = stateLiteral
			} else {
				newState = stateAltStyle
				keep, keepOK = c, true
			}
		}

		// Rust 的跃迁块：按 (旧状态, 新状态) 决定 buf 怎么落地
		switch {
		case state == stateMaybeOpen && newState == stateKey && buf != "":
			appendLiteral(buf)
			buf = ""
		case state == stateKey && (newState == stateAlign || newState == stateLiteral) && buf != "":
			parts = append(parts, TemplatePart{
				IsPlaceholder: true,
				Key:           buf,
				Align:         '<',
			})
			buf = ""
		case state == stateWidth && (newState == stateFirstStyle || newState == stateLiteral) && buf != "":
			if p := lastPlaceholder(); p != nil {
				if n, err := strconv.Atoi(buf); err == nil {
					p.Width = n
				}
			}
			buf = ""
		case state == stateFirstStyle && (newState == stateAltStyle || newState == stateLiteral) && buf != "":
			if p := lastPlaceholder(); p != nil {
				p.Style = &Style{Spec: buf}
			}
			buf = ""
		case state == stateAltStyle && newState == stateLiteral && buf != "":
			if p := lastPlaceholder(); p != nil {
				p.AltStyle = &Style{Spec: buf}
			}
			buf = ""
		}

		state = newState
		if keepOK {
			buf += string(keep)
		}
	}

	// Rust：`if matches!(state, Literal | DoubleClose) && !buf.is_empty()`
	if (state == stateLiteral || state == stateDoubleClose) && buf != "" {
		appendLiteral(buf)
		buf = ""
	}

	// Create template. Do NOT expand tabs immediately here to mirror Rust's
	// Template::from_str_with_tab_width which stores the original text and
	// tab width for lazy expansion.
	tmpl := Template{Parts: parts, currentTabWidth: tab_width}
	tmpl.originalTexts = make([]string, len(tmpl.Parts))
	for i := range tmpl.Parts {
		if !tmpl.Parts[i].IsPlaceholder && strings.Contains(tmpl.Parts[i].Text, "\t") {
			tmpl.originalTexts[i] = tmpl.Parts[i].Text
		}
	}
	return tmpl, nil
}

// Template_from_str mirrors Rust's Template::from_str(s)
func Template_from_str(s string) (Template, error) {
	return Template_from_str_with_tab_width(s, DEFAULT_TAB_WIDTH)
}

// WithKey registers a custom formatter for a template key.
func (ps *ProgressStyle) WithKey(key string, f func(*ProgressState) string) {
	if ps.formatMap == nil {
		ps.formatMap = make(map[string]ProgressTracker)
	}
	// Wrap simple string-returning formatter into a funcTracker
	ps.formatMap[key] = &funcTracker{f: func(s *ProgressState, w *tabWriter) {
		w.WriteString(f(s))
	}}
}

// WithKeyStyle registers a custom formatter that may also return a *Style to
// apply to the rendered text. Formatter-provided styles take precedence over
// placeholder-level style/alt-style.
func (ps *ProgressStyle) WithKeyStyle(key string, f func(*ProgressState, bool) (string, *Style)) {
	if ps.formatMap == nil {
		ps.formatMap = make(map[string]ProgressTracker)
	}
	// Wrap this function into a funcTracker that ignores forStderr flag
	ps.formatMap[key] = &funcTracker{f: func(s *ProgressState, w *tabWriter) {
		rendered, _ := f(s, false)
		w.WriteString(rendered)
	}}
}

// WithTracker registers a ProgressTracker directly (closest to Rust API).
func (ps *ProgressStyle) WithTracker(key string, t ProgressTracker) {
	if ps.formatMap == nil {
		ps.formatMap = make(map[string]ProgressTracker)
	}
	ps.formatMap[key] = t.Clone()
}

// Style represents a simple style descriptor (wraps the style spec string)
// that can be applied to text to produce ANSI-wrapped output.
type Style struct {
	Spec    string
	SpecErr string // optional alternate spec for stderr
}

func (s *Style) ApplyFor(text string, forStderr bool) string {
	if s == nil {
		return text
	}
	spec := s.Spec
	if forStderr && s.SpecErr != "" {
		spec = s.SpecErr
	}
	if spec == "" {
		return text
	}
	return applyAnsiStyle(text, spec)
}

// ProgressStyleWithTemplate creates a ProgressStyle based on the default bar
// but replaces the template with the provided template string.
// This mirrors the Rust API `ProgressStyle::with_template("...")` used in tests.
func ProgressStyleWithTemplate(template string) (ProgressStyle, error) {
	tpl, err := Template_from_str(template)
	if err != nil {
		return ProgressStyle{}, err
	}
	return NewProgressStyle(tpl), nil
}

// formatBar 返回一个字符串表示的进度条，width 为目标可用列数（近似）。
func (ps *ProgressStyle) formatBar(fract float32, width int, altStyle *Style, forStderr bool) string {
	// Rust：`let width = width / self.char_width;` —— 没有兜底值：
	// 终端太窄（width=0 或不足一个 cluster）时就是画空条，而不是强行画满 20 列。
	charWidth := ps.charWidth
	if charWidth <= 0 {
		charWidth = 1
	}
	clusters := width / charWidth

	fillf := float32(clusters) * fract
	if fillf < 0 {
		fillf = 0
	}
	if fillf > float32(clusters) {
		fillf = float32(clusters)
	}
	entirelyFilled := int(fillf)

	// determine current (fractional) char index
	// Rust：
	//   let n = self.progress_chars.len().saturating_sub(2);
	//   cur = match n { 0 => None, 1 => Some(1), _ => Some(n.saturating_sub((fill.fract() * n as f32) as usize)) }
	// 即 cur 是 progress_chars 的**直接下标**（0 号是"已填充"字符，1..n 是"当前"字符，n+1 是背景字符）。
	var curIdx *int
	if fillf > 0 && entirelyFilled < clusters {
		n := len(ps.progressChars) - 2
		if n < 0 {
			n = 0
		}
		if n <= 0 {
			// no current chars
			curIdx = nil
		} else if n == 1 {
			v := 1
			curIdx = &v
		} else {
			v := n - int((fillf-float32(entirelyFilled))*float32(n))
			if v < 0 {
				v = 0
			}
			if v > n {
				v = n
			}
			curIdx = &v
		}
	}

	// build bar by clusters
	var b strings.Builder
	// filled
	for i := 0; i < entirelyFilled; i++ {
		b.WriteString(ps.progressChars[0])
	}
	// current：Rust 是 `self.chars[cur]`（直接下标），Go 之前用 len-2-(cur-1) 把顺序弄反了
	if curIdx != nil {
		idx := *curIdx
		if idx < 0 {
			idx = 0
		}
		if idx >= len(ps.progressChars)-1 {
			idx = len(ps.progressChars) - 2
			if idx < 0 {
				idx = 0
			}
		}
		b.WriteString(ps.progressChars[idx])
	}
	// background
	bg := clusters - entirelyFilled
	if curIdx != nil {
		bg = bg - 1
		if bg < 0 {
			bg = 0
		}
	}
	// build background string (rest) and apply altStyle to it if provided
	var restBuilder strings.Builder
	for i := 0; i < bg; i++ {
		restBuilder.WriteString(ps.progressChars[len(ps.progressChars)-1])
	}
	restStr := restBuilder.String()
	if altStyle != nil {
		restStr = altStyle.ApplyFor(restStr, forStderr)
	}
	b.WriteString(restStr)
	return b.String()
}

// SetProgressChars sets the progress characters sequence like "##-".
// Each rune becomes a cluster in progressChars; updates charWidth accordingly.
func (ps *ProgressStyle) SetProgressChars(s string) {
	if s == "" {
		ps.progressChars = []string{"#", "-"}
		ps.charWidth = 1
		return
	}
	parts := make([]string, 0, len(s))
	for _, r := range s {
		parts = append(parts, string(r))
	}
	ps.progressChars = parts
	if len(parts) > 0 {
		ps.charWidth = measureTextWidth(parts[0])
		if ps.charWidth == 0 {
			ps.charWidth = 1
		}
	}
}

// Builder-style methods to mirror Rust's chaining API (consume-and-return Self)
// ProgressChars mirrors Rust's `progress_chars` and returns an updated copy.
func (ps ProgressStyle) ProgressChars(s string) ProgressStyle {
	ps.SetProgressChars(s)
	return ps
}

// TickChars mirrors Rust's `tick_chars` which accepts a string of grapheme clusters.
func (ps ProgressStyle) TickChars(s string) ProgressStyle {
	// split into runes (grapheme approximation)
	parts := make([]string, 0, len(s))
	for _, r := range s {
		parts = append(parts, string(r))
	}
	// require at least 2, match Rust assert behavior
	if len(parts) < 2 {
		// keep existing tickStrings unchanged if invalid input
		return ps
	}
	ps.tickStrings = parts
	return ps
}

// TickStrings mirrors Rust's `tick_strings` which accepts a slice of strings.
func (ps ProgressStyle) TickStrings(ss []string) ProgressStyle {
	if len(ss) == 0 {
		return ps
	}
	ps.tickStrings = make([]string, len(ss))
	copy(ps.tickStrings, ss)
	return ps
}

// Template mirrors Rust's `template(mut self, s: &str) -> Result<Self, TemplateError>`
// It parses the template and returns the updated ProgressStyle or an error.
func (ps ProgressStyle) Template(s string) (ProgressStyle, error) {
	tpl, err := Template_from_str(s)
	if err != nil {
		return ps, err
	}
	ps.template = tpl
	return ps, nil
}
