package mpb

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"strings"
	"time"
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
	ticks := "⠁⠁⠉⠙⠚⠒⠂⠂⠒⠲⠴⠤⠄⠄⠤⠠⠠⠤⠦⠖⠒⠐⠐⠒⠓⠋⠉⠈"
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
	var wideParts []*TemplatePart

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
		// custom formatter hook (may return an optional Style)
		if ps.formatMap != nil {
			if tracker, ok := ps.formatMap[key]; ok {
				var b strings.Builder
				tw := &tabWriter{b: &b, tabWidth: ps.tabWidth}
				tracker.Write(state, tw)
				rendered := b.String()
				// apply placeholder-level styles if any
				if part.Style != nil || part.AltStyle != nil {
					if state.IsFinished() && part.AltStyle != nil {
						rendered = part.AltStyle.ApplyFor(rendered, forStderr)
					} else if part.Style != nil {
						rendered = part.Style.ApplyFor(rendered, forStderr)
					}
				}
				lines[len(lines)-1] += rendered
				continue
			}
		}
		switch key {
		case "wide_bar":
			lines[len(lines)-1] += "\x00"
			p := part
			wideParts = append(wideParts, &p)
		case "wide_msg":
			lines[len(lines)-1] += "\x00"
			p := part
			wideParts = append(wideParts, &p)
		case "bar":
			// allow per-placeholder width (e.g. {bar:40.cyan/blue})
			w := part.Width
			if w == 0 {
				w = targetWidth
			}
			rendered := ps.formatBar(state.Fraction(), w, part.AltStyle, forStderr)
			// apply placeholder-level style to whole bar if present
			if part.Style != nil {
				rendered = part.Style.ApplyFor(rendered, forStderr)
			}
			lines[len(lines)-1] += rendered
		case "spinner":
			{
				rendered := ps.currentTickStr(state)
				if part.Style != nil || part.AltStyle != nil {
					if state.IsFinished() && part.AltStyle != nil {
						rendered = part.AltStyle.ApplyFor(rendered, forStderr)
					} else if part.Style != nil {
						rendered = part.Style.ApplyFor(rendered, forStderr)
					}
				}
				lines[len(lines)-1] += rendered
			}
		case "msg":
			{
				rendered := state.message.Expanded()
				// apply width/align/truncate if specified on placeholder
				if part.Width > 0 {
					rendered = padAndTruncate(rendered, part.Width, part.Align, part.Truncate)
				}
				if part.Style != nil || part.AltStyle != nil {
					if state.IsFinished() && part.AltStyle != nil {
						rendered = part.AltStyle.ApplyFor(rendered, forStderr)
					} else if part.Style != nil {
						rendered = part.Style.ApplyFor(rendered, forStderr)
					}
				}
				lines[len(lines)-1] += rendered
			}
		case "prefix":
			{
				rendered := state.prefix.Expanded()
				if part.Width > 0 {
					rendered = padAndTruncate(rendered, part.Width, part.Align, part.Truncate)
				}
				if part.Style != nil || part.AltStyle != nil {
					if state.IsFinished() && part.AltStyle != nil {
						rendered = part.AltStyle.ApplyFor(rendered, forStderr)
					} else if part.Style != nil {
						rendered = part.Style.ApplyFor(rendered, forStderr)
					}
				}
				lines[len(lines)-1] += rendered
			}
		case "pos":
			r := fmt.Sprintf("%d", pos)
			if part.Width > 0 {
				r = padAndTruncate(r, part.Width, part.Align, part.Truncate)
			}
			lines[len(lines)-1] += r
		case "len":
			r := fmt.Sprintf("%d", length)
			if part.Width > 0 {
				r = padAndTruncate(r, part.Width, part.Align, part.Truncate)
			}
			lines[len(lines)-1] += r
		case "percent":
			pct := state.Fraction() * 100.0
			r := fmt.Sprintf("%.0f", pct)
			if part.Width > 0 {
				r = padAndTruncate(r, part.Width, part.Align, part.Truncate)
			}
			lines[len(lines)-1] += r
		case "percent_precise":
			r := fmt.Sprintf("%.*f", 3, state.Fraction()*100.0)
			if part.Width > 0 {
				r = padAndTruncate(r, part.Width, part.Align, part.Truncate)
			}
			lines[len(lines)-1] += r
		case "human_pos":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanCount(pos))
		case "human_len":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanCount(length))
		case "human_pos_precise":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanCount(pos))
		case "bytes":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanBytes(pos))
		case "total_bytes":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanBytes(length))
		case "decimal_bytes":
			lines[len(lines)-1] += fmt.Sprintf("%v", DecimalBytes(pos))
		case "decimal_total_bytes":
			lines[len(lines)-1] += fmt.Sprintf("%v", DecimalBytes(length))
		case "binary_bytes":
			lines[len(lines)-1] += fmt.Sprintf("%v", BinaryBytes(pos))
		case "binary_total_bytes":
			lines[len(lines)-1] += fmt.Sprintf("%v", BinaryBytes(length))
		case "elapsed_precise":
			lines[len(lines)-1] += fmt.Sprintf("%v", FormattedDuration(state.Duration()))
		case "elapsed":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanDuration(state.Elapsed()))
		case "duration_precise":
			lines[len(lines)-1] += fmt.Sprintf("%v", FormattedDuration(state.Duration()))
		case "duration":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanDuration(state.Duration()))
		case "per_sec":
			lines[len(lines)-1] += fmt.Sprintf("%v", HumanFloatCount(state.PerSec()))
		case "bytes_per_sec":
			lines[len(lines)-1] += fmt.Sprintf("%v/s", HumanBytes(uint64(state.PerSec())))
		case "decimal_bytes_per_sec":
			lines[len(lines)-1] += fmt.Sprintf("%v/s", DecimalBytes(uint64(state.PerSec())))
		case "binary_bytes_per_sec":
			lines[len(lines)-1] += fmt.Sprintf("%v/s", BinaryBytes(uint64(state.PerSec())))
		default:
			lines[len(lines)-1] += "{" + key + "}"
		}
	}

	// Resolve wide placeholders per line
	results := make([]string, 0, len(lines))
	for li, line := range lines {
		if !strings.Contains(line, "\x00") {
			results = append(results, line)
			continue
		}
		// find corresponding widePart for this line (use first available)
		var wp *TemplatePart
		if li < len(wideParts) {
			wp = wideParts[li]
		} else if len(wideParts) > 0 {
			wp = wideParts[0]
		}
		without := strings.ReplaceAll(line, "\x00", "")
		prefixCols := measureTextWidth(without)
		_ = prefixCols
		left := targetWidth - prefixCols
		if left < 0 {
			left = 0
		}
		if wp != nil && wp.Key == "wide_msg" {
			rendered := padAndTruncate(state.message.Expanded(), left, wp.Align, wp.Truncate)
			// apply placeholder-level style if present
			if wp.Style != nil {
				rendered = wp.Style.ApplyFor(rendered, forStderr)
			}
			results = append(results, strings.ReplaceAll(line, "\x00", rendered))
		} else {
			var alt *Style
			if wp != nil {
				alt = wp.AltStyle
			}
			bar := ps.formatBar(state.Fraction(), left, alt, forStderr)
			// if wp has placeholder-level style, apply it to whole bar
			if wp != nil && wp.Style != nil {
				bar = wp.Style.ApplyFor(bar, forStderr)
			}
			results = append(results, strings.ReplaceAll(line, "\x00", bar))
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

	// iterate segments and collect visible runes between start and end columns
	var out strings.Builder
	col := 0
	// keep track of active ANSI sequences to re-emit before visible text
	activeAnsi := ""
	for _, seg := range segments {
		if seg.isAnsi {
			// append to active ansi; include as-is when outputting visible text
			activeAnsi += seg.text
			continue
		}
		// non-ansi text: iterate runes
		rs := []rune(seg.text)
		for _, r := range rs {
			w := runewidth.RuneWidth(r)
			if w <= 0 {
				w = 0
			}
			if col+w <= start {
				col += w
				continue
			}
			if col >= end {
				break
			}
			if out.Len() == 0 && activeAnsi != "" {
				out.WriteString(activeAnsi)
				activeAnsi = ""
			}
			out.WriteRune(r)
			col += w
		}
		if col >= end {
			break
		}
	}

	res := out.String()
	if res != "" {
		res = res + "\x1b[0m"
	}
	return res
}

// applyAnsiStyle applies a simple dot-separated style spec like "red.bold" or "dim"
// and wraps the text with corresponding ANSI SGR codes. Unsupported tokens are ignored.
func applyAnsiStyle(text, spec string) string {
	if spec == "" || text == "" {
		return text
	}
	parts := strings.Split(spec, ".")
	codes := make([]string, 0, len(parts))
	for _, p := range parts {
		switch p {
		case "reset":
			codes = append(codes, "0")
		case "bold":
			codes = append(codes, "1")
		case "dim":
			codes = append(codes, "2")
		case "italic":
			codes = append(codes, "3")
		case "underline":
			codes = append(codes, "4")
		case "blink":
			codes = append(codes, "5")
		case "reverse":
			codes = append(codes, "7")
		case "hidden":
			codes = append(codes, "8")
		case "black":
			codes = append(codes, "30")
		case "red":
			codes = append(codes, "31")
		case "green":
			codes = append(codes, "32")
		case "yellow":
			codes = append(codes, "33")
		case "blue":
			codes = append(codes, "34")
		case "magenta":
			codes = append(codes, "35")
		case "cyan":
			codes = append(codes, "36")
		case "white":
			codes = append(codes, "37")
		case "bright_black":
			codes = append(codes, "90")
		case "bright_red":
			codes = append(codes, "91")
		case "bright_green":
			codes = append(codes, "92")
		case "bright_yellow":
			codes = append(codes, "93")
		case "bright_blue":
			codes = append(codes, "94")
		case "bright_magenta":
			codes = append(codes, "95")
		case "bright_cyan":
			codes = append(codes, "96")
		case "bright_white":
			codes = append(codes, "97")
		default:
			// ignore unknown token
		}
	}
	if len(codes) == 0 {
		return text
	}
	start := "\x1b[" + strings.Join(codes, ";") + "m"
	end := "\x1b[0m"
	return start + text + end
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

// Template_from_str_with_tab_width mirrors Rust's Template::from_str_with_tab_width(s, tab_width)
// It parses the template string and stores original text for tab expansion.
// Tabs are not expanded immediately; they are expanded on first SetTabWidth call.
func Template_from_str_with_tab_width(s string, tab_width int) (Template, error) {
	if tab_width <= 0 {
		tab_width = DEFAULT_TAB_WIDTH
	}
	parts := make([]TemplatePart, 0)
	for i := 0; i < len(s); {
		if s[i] == '{' {
			// find closing
			j := i + 1
			// parse key
			for j < len(s) && s[j] != '}' && s[j] != ':' {
				j++
			}
			if j >= len(s) {
				// no closing, treat as literal
				parts = append(parts, TemplatePart{IsPlaceholder: false, Text: string(s[i])})
				i++
				continue
			}
			key := s[i+1 : j]
			tp := TemplatePart{IsPlaceholder: true, Key: key, Align: 0, Width: 0, Truncate: false}
			if j < len(s) && s[j] == ':' {
				// parse optional align, width and optional '!' for truncate
				k := j + 1
				// optional align
				if k < len(s) && (s[k] == '<' || s[k] == '^' || s[k] == '>') {
					tp.Align = rune(s[k])
					k++
				}
				// parse digits for width
				width := 0
				hasWidth := false
				for k < len(s) && s[k] >= '0' && s[k] <= '9' {
					hasWidth = true
					width = width*10 + int(s[k]-'0')
					k++
				}
				if hasWidth {
					tp.Width = width
				}
				// optional truncate marker '!'
				if k < len(s) && s[k] == '!' {
					tp.Truncate = true
					k++
				}
				// optional style specifier starting with '.' then style string
				if k < len(s) && s[k] == '.' {
					k++
					start := k
					for k < len(s) && s[k] != '/' && s[k] != '}' {
						k++
					}
					if k > start {
						tp.Style = &Style{Spec: s[start:k]}
					}
					if k < len(s) && s[k] == '/' {
						k++
						start2 := k
						for k < len(s) && s[k] != '}' {
							k++
						}
						if k > start2 {
							tp.AltStyle = &Style{Spec: s[start2:k]}
						}
					}
				}
				// advance to closing brace if needed
				j = k
				for j < len(s) && s[j] != '}' {
					j++
				}
			}
			if j < len(s) && s[j] == '}' {
				parts = append(parts, tp)
				i = j + 1
				continue
			}
		}
		// text - store original text without expanding tabs
		j := i
		for j < len(s) && s[j] != '{' {
			j++
		}
		parts = append(parts, TemplatePart{IsPlaceholder: false, Text: s[i:j]})
		i = j
	}
	// Create template. Do NOT expand tabs immediately here to mirror Rust's
	// Template::from_str_with_tab_width which stores the original text and
	// tab width for lazy expansion. Store originals for parts that contain
	// tabs and record the provided tab width so expansion can occur later.
	tmpl := Template{Parts: parts, currentTabWidth: 0}
	tmpl.originalTexts = make([]string, len(tmpl.Parts))
	for i := range tmpl.Parts {
		if !tmpl.Parts[i].IsPlaceholder && strings.Contains(tmpl.Parts[i].Text, "\t") {
			tmpl.originalTexts[i] = tmpl.Parts[i].Text
		}
	}
	// Record the tab width to be used when expanding later.
	tmpl.currentTabWidth = tab_width
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
	if width <= 0 {
		width = 20
	}
	// follow Rust semantics: account for char width (clusters)
	// number of clusters available
	clusters := width / ps.charWidth
	if clusters <= 0 {
		clusters = 1
	}

	fillf := float32(clusters) * fract
	if fillf < 0 {
		fillf = 0
	}
	if fillf > float32(clusters) {
		fillf = float32(clusters)
	}
	entirelyFilled := int(fillf)

	// determine current (fractional) char index
	var curIdx *int
	if fillf > 0 && entirelyFilled < clusters {
		n := len(ps.progressChars) - 2
		if n <= 0 {
			// no current chars
			curIdx = nil
		} else if n == 1 {
			v := 1
			curIdx = &v
		} else {
			v := n - int((fillf-float32(entirelyFilled))*float32(n))
			if v < 1 {
				v = 1
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
	// current
	if curIdx != nil {
		// index into progressChars: len-2 - (cur-1)
		idx := len(ps.progressChars) - 2 - (*curIdx - 1)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(ps.progressChars)-1 {
			idx = len(ps.progressChars) - 2
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
