package mpb

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"
)

// 注意：此文件按 src/state.rs 逻辑翻译，尽量一行一行对应。

// ProgressFinish 表示完成时的行为（对应 Rust 的 enum）
type FinishKind int

const (
	FinishAndLeave FinishKind = iota
	FinishWithMessage
	FinishAndClear
	FinishAbandon
	FinishAbandonWithMessage
)

type ProgressFinish struct {
	Kind FinishKind
	Msg  string
}

func DefaultProgressFinish() ProgressFinish { return ProgressFinish{Kind: FinishAndClear} }

// BarState 对应 Rust 的 BarState
type BarState struct {
	drawTarget ProgressDrawTarget
	onFinish   ProgressFinish
	style      ProgressStyle
	state      ProgressState
	tabWidth   int
}

const DEFAULT_TAB_WIDTH = 8

// NewBarState 创建 BarState，接受 pos（AtomicPosition）以与 Rust 保持一致
func NewBarState(len *uint64, drawTarget ProgressDrawTarget, pos *AtomicPosition) *BarState {
	return &BarState{
		drawTarget: drawTarget,
		onFinish:   DefaultProgressFinish(),
		style:      DefaultProgressStyleBar(),
		state:      NewProgressState(len, pos),
		tabWidth:   DEFAULT_TAB_WIDTH,
	}
}

// FinishUsingStyle 完成时应用样式行为
func (b *BarState) FinishUsingStyle(now time.Time, finish ProgressFinish) {
	duration := now.Sub(b.state.started)
	b.state.status = StatusDoneVisible(duration)
	switch finish.Kind {
	case FinishAndLeave:
		if b.state.len != nil {
			b.state.pos.Set(*b.state.len)
		}
	case FinishWithMessage:
		if b.state.len != nil {
			b.state.pos.Set(*b.state.len)
		}
		b.state.message = NewTabExpandedString(finish.Msg, b.tabWidth)
	case FinishAndClear:
		if b.state.len != nil {
			b.state.pos.Set(*b.state.len)
		}
		b.state.status = StatusDoneHidden(duration)
	case FinishAbandon:
		// do nothing
	case FinishAbandonWithMessage:
		b.state.message = NewTabExpandedString(finish.Msg, b.tabWidth)
	}

	// draw
	_ = b.draw(true)
}

// ResetMode 与 Rust 的 Reset
type ResetMode int

const (
	ResetEta ResetMode = iota
	ResetElapsed
	ResetAll
)

func (b *BarState) Reset(now time.Time, mode ResetMode) {
	b.state.est.Reset(now)
	if mode == ResetElapsed || mode == ResetAll {
		b.state.started = now
	}
	if mode == ResetAll {
		b.state.pos.Reset(now)
		b.state.status = StatusInProgress
		// reset trackers if any (omitted detailed tracker implementations)
		_ = b.draw(false)
	}
}

// Update 更新内部 ProgressState 并在需要时 tick
func (b *BarState) Update(now time.Time, f func(*ProgressState), tick bool) {
	f(&b.state)
	if tick {
		b.Tick(now)
	}
}

func (b *BarState) UnsetLength(now time.Time) {
	b.state.len = nil
	b.UpdateEstimateAndDraw(now)
}

func (b *BarState) SetLength(now time.Time, l uint64) {
	b.state.len = &l
	b.UpdateEstimateAndDraw(now)
}

func (b *BarState) IncLength(now time.Time, delta uint64) {
	if b.state.len != nil {
		*b.state.len = (*b.state.len) + delta
	}
	b.UpdateEstimateAndDraw(now)
}

func (b *BarState) DecLength(now time.Time, delta uint64) {
	if b.state.len != nil {
		if *b.state.len > delta {
			*b.state.len = (*b.state.len) - delta
		} else {
			*b.state.len = 0
		}
	}
	b.UpdateEstimateAndDraw(now)
}

func (b *BarState) SetTabWidth(tabWidth int) {
	b.tabWidth = tabWidth
	b.state.message.SetTabWidth(tabWidth)
	b.state.prefix.SetTabWidth(tabWidth)
	b.style.SetTabWidth(tabWidth)
}

func (b *BarState) SetStyle(style ProgressStyle) {
	// If the current draw target is stderr, apply stderr variant of styles to match Rust
	if b.drawTarget != nil && b.drawTarget.IsStderr() {
		style.SetForStderr()
	}
	b.style = style
	b.style.SetTabWidth(b.tabWidth)
}

func (b *BarState) Tick(now time.Time) {
	b.state.tick = b.state.tick + 1
	b.UpdateEstimateAndDraw(now)
}

// Index 返回进度条在 MultiProgress 中的索引（如果存在）。
func (b *BarState) Index() (int, bool) {
	// If the draw target is a remoteDrawTarget, use its Remote() method to get the index
	if _, idx, ok := b.drawTarget.Remote(); ok {
		return idx, true
	}
	// Fallback: If the draw target is a multiDrawTarget, search its ordering for this BarState
	if md, ok := b.drawTarget.(*multiDrawTarget); ok && md != nil {
		md.ms.mu.RLock()
		defer md.ms.mu.RUnlock()
		for _, idx := range md.ms.ordering {
			if idx < 0 || idx >= len(md.ms.members) {
				continue
			}
			m := md.ms.members[idx]
			if m.sb != nil && m.sb.inner == b {
				return idx, true
			}
		}
	}
	return 0, false
}

func (b *BarState) UpdateEstimateAndDraw(now time.Time) {
	pos := b.state.pos.Pos()
	b.state.est.Record(pos, now)

	// tick trackers omitted - call their tick functions if implemented

	_ = b.draw(false)
}

func (b *BarState) Println(now time.Time, msg string) {
	width := b.drawTarget.Width()
	drawable := b.drawTarget.Drawable(true)
	if drawable == nil {
		return
	}
	dsWrap := drawable.State()
	ds := dsWrap.StatePtr
	parts := splitLines(msg)
	if len(parts) == 0 {
		ds.lines = append(ds.lines, LineEmpty)
		ds.contents = append(ds.contents, "")
	} else {
		for _, part := range parts {
			if part == "" {
				ds.lines = append(ds.lines, LineEmpty)
				ds.contents = append(ds.contents, "")
			} else {
				ds.lines = append(ds.lines, LineText)
				ds.contents = append(ds.contents, part)
			}
		}
	}

	if w := width; w != nil {
		if !b.state.IsFinished() {
			isErr := false
			if b.drawTarget != nil {
				isErr = b.drawTarget.IsStderr()
			}
			lines, contents := b.style.FormatState(&b.state, int(*w), isErr)
			ds.lines = append(ds.lines, lines...)
			for _, c := range contents {
				ds.contents = append(ds.contents, c)
			}
		}
	}

	dsWrap.Close()
	drawable.Draw()
}

func (b *BarState) Suspend(now time.Time, f func() interface{}) interface{} {
	if remote, _, ok := b.drawTarget.Remote(); ok && remote != nil {
		state := remote.StateWrite()
		return state.Suspend(f, now)
	}

	if d := b.drawTarget.Drawable(true); d != nil {
		_ = d.Clear()
	}

	ret := f()
	_ = b.draw(true)
	return ret
}

func (b *BarState) draw(force bool) error { return b.drawWithNow(force, time.Now()) }
func (b *BarState) drawWithNow(force bool, now time.Time) error {
	force = force || b.state.IsFinished()
	// If draw target supports aggregated draws (MultiProgress), delegate to it.
	if agg, ok := b.drawTarget.(interface {
		AggregateDrawBar(*BarState, bool, time.Time) error
	}); ok {
		return agg.AggregateDrawBar(b, force, now)
	}

	drawable := b.drawTarget.Drawable(force)
	if drawable == nil {
		return nil
	}
	width := drawable.Width()
	dsWrap := drawable.State()
	ds := dsWrap.StatePtr
	if width != nil {
		if !b.state.IsFinished() {
			isErr := false
			if b.drawTarget != nil {
				isErr = b.drawTarget.IsStderr()
			}
			lines, contents := b.style.FormatState(&b.state, int(*width), isErr)
			ds.lines = append(ds.lines, lines...)
			for _, c := range contents {
				ds.contents = append(ds.contents, c)
			}
		}
	}
	dsWrap.Close()
	drawable.Draw()
	return nil
}

// --- ProgressState ---

type Status int

const (
	StatusInProgress Status = iota
	statusDoneVisible
	statusDoneHidden
)

func StatusDoneVisible(d time.Duration) Status { return statusDoneVisible }
func StatusDoneHidden(d time.Duration) Status  { return statusDoneHidden }

type ProgressState struct {
	pos     *AtomicPosition
	len     *uint64
	tick    uint64
	started time.Time
	status  Status
	est     Estimator
	message TabExpandedString
	prefix  TabExpandedString
}

func NewProgressState(len *uint64, pos *AtomicPosition) ProgressState {
	now := time.Now()
	return ProgressState{
		pos:     pos,
		len:     len,
		tick:    0,
		status:  StatusInProgress,
		started: now,
		est:     NewEstimator(now),
		message: NewTabExpandedString("", DEFAULT_TAB_WIDTH),
		prefix:  NewTabExpandedString("", DEFAULT_TAB_WIDTH),
	}
}

func (ps *ProgressState) IsFinished() bool {
	return ps.status != StatusInProgress
}

func (ps *ProgressState) Fraction() float32 {
	pos := ps.pos.Pos()
	if ps.len == nil {
		return 0.0
	}
	if *ps.len == 0 {
		return 1.0
	}
	if pos == 0 {
		return 0.0
	}
	pct := float32(pos) / float32(*ps.len)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	return pct
}

func (ps *ProgressState) Eta() time.Duration {
	if ps.IsFinished() {
		return 0
	}
	if ps.len == nil {
		return 0
	}
	pos := ps.pos.Pos()
	sps := ps.est.StepsPerSecond(time.Now())
	if sps == 0.0 {
		return 0
	}
	return secsToDuration(float64((*ps.len)-pos) / sps)
}

func (ps *ProgressState) Duration() time.Duration {
	return ps.started.Sub(time.Now())
}

func (ps *ProgressState) PerSec() float64 {
	return ps.est.StepsPerSecond(time.Now())
}

func (ps *ProgressState) Elapsed() time.Duration { return time.Since(ps.started) }

func (ps *ProgressState) Pos() uint64 { return ps.pos.Pos() }

func (ps *ProgressState) SetPos(v uint64) { ps.pos.Set(v) }

func (ps *ProgressState) Len() *uint64 { return ps.len }

func (ps *ProgressState) SetLen(v uint64) { ps.len = &v }

// --- Estimator ---

type Estimator struct {
	smoothed       float64
	doubleSmoothed float64
	prevSteps      uint64
	prevTime       time.Time
	startTime      time.Time
}

func NewEstimator(now time.Time) Estimator {
	return Estimator{smoothed: 0, doubleSmoothed: 0, prevSteps: 0, prevTime: now, startTime: now}
}

func (e *Estimator) Record(newSteps uint64, now time.Time) {
	if newSteps <= e.prevSteps || !now.After(e.prevTime) {
		if newSteps < e.prevSteps {
			e.prevSteps = newSteps
			e.Reset(now)
		}
		return
	}
	deltaSteps := newSteps - e.prevSteps
	deltaT := durationToSecs(now.Sub(e.prevTime))
	newSps := float64(deltaSteps) / deltaT
	weight := estimatorWeight(deltaT)
	e.smoothed = e.smoothed*weight + newSps*(1-weight)
	deltaTStart := durationToSecs(now.Sub(e.startTime))
	totalWeight := 1.0 - estimatorWeight(deltaTStart)
	normalized := 0.0
	if totalWeight != 0 {
		normalized = e.smoothed / totalWeight
	}
	e.doubleSmoothed = e.doubleSmoothed*weight + normalized*(1-weight)
	e.prevSteps = newSteps
	e.prevTime = now
}

func (e *Estimator) Reset(now time.Time) {
	e.smoothed = 0
	e.doubleSmoothed = 0
	e.prevTime = now
	e.startTime = now
}

func (e *Estimator) StepsPerSecond(now time.Time) float64 {
	deltaT := durationToSecs(now.Sub(e.prevTime))
	reweight := estimatorWeight(deltaT)
	deltaTStart := durationToSecs(now.Sub(e.startTime))
	totalWeight := 1.0 - estimatorWeight(deltaTStart)
	sps := 0.0
	if totalWeight != 0 {
		sps = e.smoothed * reweight / totalWeight
	}
	dsps := e.doubleSmoothed*reweight + sps*(1-reweight)
	if totalWeight == 0 {
		return dsps
	}
	return dsps / totalWeight
}

func durationToSecs(d time.Duration) float64 {
	return float64(d.Seconds())
}

func estimatorWeight(age float64) float64 {
	const EXPONENTIAL_WEIGHTING_SECONDS = 15.0
	return math.Pow(0.1, age/EXPONENTIAL_WEIGHTING_SECONDS)
}

func secsToDuration(s float64) time.Duration {
	secs := int64(math.Trunc(s))
	nanos := int64((s - float64(secs)) * 1e9)
	return time.Duration(secs)*time.Second + time.Duration(nanos)*time.Nanosecond
}

// --- TabExpandedString ---
type TabExpandedString struct {
	original string
	tabWidth int
}

func NewTabExpandedString(s string, tabWidth int) TabExpandedString {
	return TabExpandedString{original: s, tabWidth: tabWidth}
}

func (t *TabExpandedString) Expanded() string {
	if !strings.Contains(t.original, "\t") {
		return t.original
	}
	return strings.ReplaceAll(t.original, "\t", strings.Repeat(" ", t.tabWidth))
}

func (t *TabExpandedString) SetTabWidth(w int) { t.tabWidth = w }

// --- Helpers / stubs ---

// ProgressDrawTarget, ProgressStyle, LineType and Drawable related stubs are referenced
// from other translated files. Provide minimal interfaces so this file compiles during
// incremental translation. These will be replaced with full translations.

// ProgressDrawTarget 接口：抽象出绘制目标的必要方法（逐步完善）
type ProgressDrawTarget interface {
	IsHidden() bool
	IsStderr() bool
	Drawable(force bool) Drawable
	Width() *uint16
	// 返回 (remoteState, idx, ok)
	Remote() (*RemoteStub, int, bool)
	Disconnect(now time.Time)
	AdjustLastLineCount(adjust LineAdjust)
}

type DrawableStub interface {
	State() *DrawStateStub
	Draw() error
	Clear() error
}

// RemoteStub references a MultiState for remote operations
type RemoteStub struct{ ms *MultiState }

func (r *RemoteStub) StateWrite() *MultiStateStub { return &MultiStateStub{ms: r.ms} }

// draw target 的具体实现位于 draw_target.go（增量翻译）。
// DrawState mirrors Rust's DrawState: it holds the lines to print,
// the move_cursor flag and the alignment for multi progress.
type DrawState struct {
	// The lines to print (can contain ANSI codes)
	lines    []LineType
	contents []string
	// True if we should move the cursor up when possible instead of clearing lines.
	move_cursor bool
	// Controls how the multi progress is aligned if some of its progress bars get removed
	alignment MultiProgressAlignment
}

// Keep DrawStateStub as an alias for compatibility where referenced.
type DrawStateStub = DrawState

// VisualLines represents a count of visual lines (wrap-aware).
type VisualLines int

func (v VisualLines) SaturatingAdd(o VisualLines) VisualLines {
	return VisualLines(int(v) + int(o))
}

func (v VisualLines) SaturatingSub(o VisualLines) VisualLines {
	res := int(v) - int(o)
	if res < 0 {
		res = 0
	}
	return VisualLines(res)
}

func (v VisualLines) AsInt() int { return int(v) }

func VisualLinesFromInt(i int) VisualLines {
	if i < 0 {
		i = 0
	}
	return VisualLines(i)
}

// visualLineCount computes visual height for a slice of LineType given parallel contents and a terminal width.
func visualLineCount(lines []LineType, contents []string, width int) VisualLines {
	var acc VisualLines = 0
	for i := range lines {
		var c string
		if i < len(contents) {
			c = contents[i]
		} else {
			c = ""
		}
		acc += wrappedHeightForContent(c, width)
	}
	return acc
}

// wrappedHeightForContent computes wrapped visual height for a content string.
func wrappedHeightForContent(s string, width int) VisualLines {
	if width <= 0 {
		return VisualLines(1)
	}
	cw := measureTextWidth(s)
	terminalLen := (cw + width - 1) / width
	if terminalLen < 1 {
		terminalLen = 1
	}
	return VisualLines(terminalLen)
}

// measureTextWidth approximates console::measure_text_width by stripping ANSI CSI sequences and returning rune count.
func measureTextWidth(s string) int {
	if s == "" {
		return 0
	}
	// Normalize and remove certain invisible characters to better match Rust's console behavior
	s = norm.NFKC.String(s)
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	// expand tabs to spaces (conservative)
	s = strings.ReplaceAll(s, "\t", "    ")
	// remove zero-width joiner/non-joiner which may affect widths in console crate
	s = strings.ReplaceAll(s, "\u200d", "")
	s = strings.ReplaceAll(s, "\u200c", "")
	// strip ANSI CSI sequences
	rs := []rune(s)
	out := make([]rune, 0, len(rs))
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\x1b' {
			// skip CSI sequence
			i++
			if i < len(rs) && rs[i] == '[' {
				i++
				for i < len(rs) {
					if (rs[i] >= 'A' && rs[i] <= 'Z') || (rs[i] >= 'a' && rs[i] <= 'z') {
						break
					}
					i++
				}
			}
			continue
		}
		out = append(out, rs[i])
	}
	return runewidth.StringWidth(string(out))
}

// DrawStateStub.drawToTerm implements Rust's DrawState::draw_to_term semantics
// term: destination implementing TermLike
// barCount: pointer to last_line_count (number of dynamic lines printed previously)
func (ds *DrawState) drawToTerm(term TermLike, prevBarCount VisualLines) (VisualLines, error) {
	// move cursor / clear previous lines
	// Note: follow Rust behavior: base branch decision on presence of `lines`,
	// but always perform the clearing/move logic even when no new lines are present
	// to remove previously printed dynamic lines.
	// (no debug logging)

	// Conservatively always clear previous dynamic lines and reset cursor
	// position instead of attempting move-cursor optimization. This is
	// slightly less efficient but avoids complex cursor-positioning bugs
	// that can cause bars to be written to the wrong lines.
	n := prevBarCount.AsInt()
	// Debug: log prevBarCount and number of lines
	fmt.Fprintf(os.Stderr, "drawToTerm: prevBarCount=%d, len(lines)=%d, len(contents)=%d\n", n, len(ds.lines), len(ds.contents))
	up := prevBarCount.SaturatingSub(VisualLinesFromInt(1)).AsInt()
	if up > 0 {
		if err := term.MoveCursorUp(up); err != nil {
			return VisualLines(0), err
		}
	}
	for i := 0; i < n; i++ {
		if err := term.ClearLine(); err != nil {
			return VisualLines(0), err
		}
		if i+1 != n {
			if err := term.MoveCursorDown(1); err != nil {
				return VisualLines(0), err
			}
		}
	}
	if up > 0 {
		if err := term.MoveCursorUp(up); err != nil {
			return VisualLines(0), err
		}
	}

	termWidth := int(term.Width())

	// full height needed (including orphan/text and bar lines)
	fullHeight := visualLineCount(ds.lines, ds.contents, termWidth)
	fmt.Fprintf(os.Stderr, "drawToTerm: fullHeight=%d\n", fullHeight)

	// shift for bottom alignment: write blank cleared lines
	var shift VisualLines = 0
	if ds.alignment == MultiProgressAlignmentBottom && fullHeight < prevBarCount {
		shift = prevBarCount.SaturatingSub(fullHeight)
		for i := 0; i < shift.AsInt(); i++ {
			if err := term.WriteLine(""); err != nil {
				return VisualLines(0), err
			}
		}
	}

	var realHeight VisualLines = 0

	for idx, lt := range ds.lines {
		// find corresponding content
		var content string
		if idx < len(ds.contents) {
			content = ds.contents[idx]
		} else {
			content = ""
		}

		lineHeight := wrappedHeightForContent(content, termWidth)

		// For Bar lines we must check terminal height and may early-stop
		if lt == LineBar {
			if realHeight.SaturatingAdd(lineHeight).AsInt() > int(term.Height()) {
				break
			}
			realHeight = realHeight.SaturatingAdd(lineHeight)
		}

		// Print a new line if this is not the first line printed this tick
		// the first line will automatically wrap due to the filler below
		if idx != 0 {
			if err := term.WriteLine(""); err != nil {
				return VisualLines(0), err
			}
		}

		if err := term.WriteStr(content); err != nil {
			return VisualLines(0), err
		}

		if idx+1 == len(ds.lines) {
			filler := int(lineHeight)*termWidth - measureTextWidth(content)
			if filler > 0 {
				if err := term.WriteStr(strings.Repeat(" ", filler)); err != nil {
					return VisualLines(0), err
				}
			}
		}
	}

	if err := term.Flush(); err != nil {
		return VisualLines(0), err
	}

	return realHeight.SaturatingAdd(shift), nil
}

type Drawable interface {
	State() *DrawStateWrapper
	Draw() error
	Clear() error
	Width() *uint16
}

// DrawStateWrapper is a RAII-style wrapper around DrawStateStub used to
// emulate Rust's DrawStateWrapper behavior: when Close() is called and
// an orphanLines slice was provided, non-bar lines are moved into orphanLines.
type DrawStateWrapper struct {
	StatePtr    *DrawStateStub
	orphanLines *[]string
}

func (w *DrawStateWrapper) Reset() {
	if w.StatePtr != nil {
		w.StatePtr.lines = w.StatePtr.lines[:0]
		w.StatePtr.contents = w.StatePtr.contents[:0]
	}
}

// Close performs the Drop-like behavior: filters non-bar lines into orphanLines
// and leaves only bar lines in the wrapped state.
func (w *DrawStateWrapper) Close() {
	if w.StatePtr == nil {
		return
	}
	// If no orphan collector provided, simply strip non-bar lines from state
	oldLines := w.StatePtr.lines
	oldContents := w.StatePtr.contents
	newLines := make([]LineType, 0, len(oldLines))
	newContents := make([]string, 0, len(oldContents))
	for i := 0; i < len(oldLines); i++ {
		lt := oldLines[i]
		switch lt {
		case LineText, LineEmpty:
			if w.orphanLines != nil {
				var content string
				if i < len(oldContents) {
					content = oldContents[i]
				} else {
					content = ""
				}
				*w.orphanLines = append(*w.orphanLines, content)
			}
		default:
			newLines = append(newLines, lt)
			if i < len(oldContents) {
				newContents = append(newContents, oldContents[i])
			} else {
				newContents = append(newContents, "")
			}
		}
	}
	w.StatePtr.lines = newLines
	w.StatePtr.contents = newContents
}

func DrawStateWrapperForTerm(s *DrawStateStub) *DrawStateWrapper {
	return &DrawStateWrapper{StatePtr: s, orphanLines: nil}
}

func DrawStateWrapperForMulti(s *DrawStateStub, orphan *[]string) *DrawStateWrapper {
	return &DrawStateWrapper{StatePtr: s, orphanLines: orphan}
}

type LineType int

const (
	LineText LineType = iota
	LineBar
	LineEmpty
)

func LineTextVal(s string) LineType { return LineText }
func LineEmptyVal() LineType        { return LineEmpty }

type LineAdjustKind int

const (
	LineAdjustClear LineAdjustKind = iota
	LineAdjustKeep
)

type LineAdjust struct {
	Kind  LineAdjustKind
	Count VisualLines
}

// MultiStateStub with reference to actual MultiState to expose Suspend
type MultiStateStub struct{ ms *MultiState }

func (m *MultiStateStub) Suspend(f func() interface{}, now time.Time) interface{} {
	m.ms.mu.Lock()
	defer m.ms.mu.Unlock()
	return m.ms.suspend(f, now)
}

// End of state.go

// SafeBarState 是对 BarState 的线程安全包装，模拟 Rust 中的 Arc<Mutex<BarState>> 行为。
type SafeBarState struct {
	mu     sync.Mutex
	inner  *BarState
	ticker *tickerSlot
}

func NewSafeBarState(b *BarState) *SafeBarState { return &SafeBarState{inner: b} }

func (s *SafeBarState) startTicker(interval time.Duration, pb *ProgressBar) {
	if interval <= 0 {
		return
	}
	s.mu.Lock()
	if s.ticker == nil {
		s.ticker = &tickerSlot{}
	}
	s.mu.Unlock()

	s.ticker.mu.Lock()
	defer s.ticker.mu.Unlock()
	if s.ticker.th != nil {
		s.ticker.th.Stop()
	}
	s.ticker.th = NewTickerHandle(interval, pb)
}

func (s *SafeBarState) stopTicker() {
	s.mu.Lock()
	t := s.ticker
	s.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.th != nil {
		t.th.Stop()
		t.th = nil
	}
	t.mu.Unlock()
}

func (s *SafeBarState) disableTicker() {
	s.stopTicker()
}

func (s *SafeBarState) Tick(now time.Time) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.Tick(now)
}
func (s *SafeBarState) FinishUsingStyle(now time.Time, f ProgressFinish) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.FinishUsingStyle(now, f)
}
func (s *SafeBarState) SetStyle(style ProgressStyle) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.SetStyle(style)
}
func (s *SafeBarState) SetTabWidth(w int) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.SetTabWidth(w)
}
func (s *SafeBarState) UnsetLength(now time.Time) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.UnsetLength(now)
}
func (s *SafeBarState) SetLength(now time.Time, l uint64) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.SetLength(now, l)
}
func (s *SafeBarState) IncLength(now time.Time, d uint64) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.IncLength(now, d)
}
func (s *SafeBarState) DecLength(now time.Time, d uint64) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.DecLength(now, d)
}
func (s *SafeBarState) Update(now time.Time, f func(*ProgressState), tick bool) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.Update(now, f, tick)
}
func (s *SafeBarState) Suspend(now time.Time, f func() interface{}) interface{} {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	return inner.Suspend(now, f)
}
func (s *SafeBarState) Draw(force bool) error {
	// Capture formatted origin lines under lock, then perform aggregated draw without holding lock
	s.mu.Lock()
	var originLines []LineType
	var originContents []string
	if wptr := s.inner.drawTarget.Width(); wptr != nil {
		if !s.inner.state.IsFinished() {
			originLines, originContents = s.inner.style.FormatState(&s.inner.state, int(*wptr), s.inner.drawTarget.IsStderr())
		}
	}
	s.mu.Unlock()

	if agg, ok := s.inner.drawTarget.(interface {
		AggregateDrawOrigin([]LineType, []string, *BarState, bool, time.Time) error
	}); ok {
		return agg.AggregateDrawOrigin(originLines, originContents, s.inner, force, time.Now())
	}

	return s.inner.draw(force)
}
func (s *SafeBarState) DrawWithNow(force bool, now time.Time) error {
	s.mu.Lock()
	var originLines []LineType
	var originContents []string
	if wptr := s.inner.drawTarget.Width(); wptr != nil {
		if !s.inner.state.IsFinished() {
			originLines, originContents = s.inner.style.FormatState(&s.inner.state, int(*wptr), s.inner.drawTarget.IsStderr())
		}
	}
	s.mu.Unlock()

	if agg, ok := s.inner.drawTarget.(interface {
		AggregateDrawOrigin([]LineType, []string, *BarState, bool, time.Time) error
	}); ok {
		return agg.AggregateDrawOrigin(originLines, originContents, s.inner, force, now)
	}

	return s.inner.drawWithNow(force, now)
}
func (s *SafeBarState) Println(now time.Time, msg string) {
	// Build origin lines (message + formatted bar) under lock, then delegate to
	// draw target's AggregateDrawOrigin if available to ensure atomic multi-bar draws.
	s.mu.Lock()
	inner := s.inner
	var originLines []LineType
	var originContents []string
	// message lines
	parts := splitLines(msg)
	if len(parts) == 0 {
		originLines = append(originLines, LineEmpty)
		originContents = append(originContents, "")
	} else {
		for _, part := range parts {
			if part == "" {
				originLines = append(originLines, LineEmpty)
				originContents = append(originContents, "")
			} else {
				originLines = append(originLines, LineText)
				originContents = append(originContents, part)
			}
		}
	}
	// formatted bar lines
	if wptr := inner.drawTarget.Width(); wptr != nil {
		if !inner.state.IsFinished() {
			lines, contents := inner.style.FormatState(&inner.state, int(*wptr), inner.drawTarget.IsStderr())
			originLines = append(originLines, lines...)
			for _, c := range contents {
				originContents = append(originContents, c)
			}
		}
	}
	s.mu.Unlock()

	if agg, ok := inner.drawTarget.(interface {
		AggregateDrawOrigin([]LineType, []string, *BarState, bool, time.Time) error
	}); ok {
		_ = agg.AggregateDrawOrigin(originLines, originContents, inner, true, now)
		return
	}

	// fallback to previous behavior
	inner.Println(now, msg)
}
func (s *SafeBarState) Reset(now time.Time, mode ResetMode) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.Reset(now, mode)
}

// Accessors for inner ProgressState values
func (s *SafeBarState) IsFinished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.IsFinished()
}
func (s *SafeBarState) Len() *uint64    { s.mu.Lock(); defer s.mu.Unlock(); return s.inner.state.Len() }
func (s *SafeBarState) SetPos(v uint64) { s.mu.Lock(); defer s.mu.Unlock(); s.inner.state.SetPos(v) }
func (s *SafeBarState) Pos() uint64     { s.mu.Lock(); defer s.mu.Unlock(); return s.inner.state.Pos() }
func (s *SafeBarState) Message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.message.Expanded()
}
func (s *SafeBarState) Prefix() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.prefix.Expanded()
}
func (s *SafeBarState) Position() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.Pos()
}
func (s *SafeBarState) Eta() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.Eta()
}
func (s *SafeBarState) PerSec() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.PerSec()
}
func (s *SafeBarState) Elapsed() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.state.Elapsed()
}
func (s *SafeBarState) Index() (int, bool) { s.mu.Lock(); defer s.mu.Unlock(); return s.inner.Index() }

func (s *SafeBarState) SetPrefix(prefix string) {
	s.mu.Lock()
	inner := s.inner
	inner.state.prefix = NewTabExpandedString(prefix, inner.tabWidth)
	s.mu.Unlock()
	inner.UpdateEstimateAndDraw(time.Now())
}
func (s *SafeBarState) SetMessage(msg string) {
	s.mu.Lock()
	inner := s.inner
	inner.state.message = NewTabExpandedString(msg, inner.tabWidth)
	s.mu.Unlock()
	inner.UpdateEstimateAndDraw(time.Now())
}
func (s *SafeBarState) Style() ProgressStyle { s.mu.Lock(); defer s.mu.Unlock(); return s.inner.style }
func (s *SafeBarState) SetOnFinish(f ProgressFinish) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner.onFinish = f
}
func (s *SafeBarState) GetOnFinish() ProgressFinish {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.onFinish
}
func (s *SafeBarState) SetDrawTarget(dt ProgressDrawTarget) {
	s.mu.Lock()
	inner := s.inner
	old := inner.drawTarget
	inner.drawTarget = dt
	s.mu.Unlock()
	if old != nil {
		old.Disconnect(time.Now())
	}
}

func (s *SafeBarState) IsHidden() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.drawTarget.IsHidden()
}

// SetElapsed sets the progress start time so that elapsed time equals d.
func (s *SafeBarState) SetElapsed(d time.Duration) {
	s.mu.Lock()
	inner := s.inner
	start := time.Now().Add(-d)
	inner.state.started = start
	inner.state.est.Reset(start)
	s.mu.Unlock()
	inner.UpdateEstimateAndDraw(time.Now())
}

// FormatForWidth returns the formatted lines and contents for the bar under the SafeBarState lock.
func (s *SafeBarState) FormatForWidth(width int) ([]LineType, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inner == nil {
		fmt.Fprintf(os.Stderr, "FormatForWidth: s.inner == nil\n")
		return nil, nil
	}
	// Only skip formatting if the status is DoneHidden (like Rust)
	// DoneVisible should still be displayed
	if s.inner.state.status == statusDoneHidden {
		fmt.Fprintf(os.Stderr, "FormatForWidth: statusDoneHidden\n")
		return nil, nil
	}
	isErr := false
	if s.inner != nil && s.inner.drawTarget != nil {
		isErr = s.inner.drawTarget.IsStderr()
	}
	lines, contents := s.inner.style.FormatState(&s.inner.state, width, isErr)
	fmt.Fprintf(os.Stderr, "FormatForWidth: returning len(lines)=%d, len(contents)=%d\n", len(lines), len(contents))
	return lines, contents
}
