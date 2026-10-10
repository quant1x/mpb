// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"os"
	"runtime"
	"sync"
	"time"
)

// RateLimiter mirrors the Rust implementation: interval in milliseconds,
// a capacity allowing occasional bursts, and a timestamp of the previous
// allowance calculation.
type RateLimiter struct {
	interval uint16 // milliseconds
	capacity uint8
	prev     time.Time
}

const maxBurst = 20

// newRateLimiter creates a RateLimiter with the desired refresh rate per second.
func newRateLimiter(rate uint8) *RateLimiter {
	if rate == 0 {
		return &RateLimiter{interval: 1000, capacity: uint8(maxBurst), prev: time.Now()}
	}
	return &RateLimiter{interval: uint16(1000 / int(rate)), capacity: uint8(maxBurst), prev: time.Now()}
}

// allow returns true if a draw is allowed at the provided time. It mirrors
// the Rust RateLimiter::allow semantics including burst handling.
func (r *RateLimiter) allow(now time.Time) bool {
	if now.Before(r.prev) {
		return false
	}

	elapsed := now.Sub(r.prev)

	if r.capacity == 0 && elapsed < time.Duration(r.interval)*time.Millisecond {
		return false
	}

	// Calculate how many intervals have passed and the remainder in nanoseconds
	newIntervals := elapsed.Milliseconds() / int64(r.interval)
	remainder := elapsed.Nanoseconds() % (int64(r.interval) * 1_000_000)

	// Update capacity: add newIntervals, subtract one for this allowance
	var cap uint64 = uint64(r.capacity)
	if newIntervals > 0 {
		cap = cap + uint64(newIntervals) - 1
	} else {
		// no new intervals, just consume one
		if cap > 0 {
			cap = cap - 1
		} else {
			// cap == 0 and elapsed was >= interval handled earlier
			// but fallthrough to set prev and return true
		}
	}
	if cap > uint64(maxBurst) {
		cap = uint64(maxBurst)
	}
	r.capacity = uint8(cap)

	// Store prev adjusted by remainder so timing stays aligned with Rust's logic
	r.prev = now.Add(-time.Duration(remainder) * time.Nanosecond)
	return true
}

// 简化版 draw_target 实现，替代 state.go 中的占位 simpleDrawTarget。
// 提供 ProgressDrawTarget 的基本实现以及 Drawable 对象，便于增量迁移。

type progressDrawTarget struct {
	kind targetKind
}

// internal holder for variant-specific data. This groups all
// implementation details under a single `kind` field so the
// external shape of `progressDrawTarget` mirrors the Rust
// `ProgressDrawTarget { kind: TargetKind }` abstraction.
type targetKind struct {
	variant targetVariant // tag for which variant this kind represents
	// common metadata
	hidden bool
	stderr bool
	width  *uint16
	mu     sync.Mutex
	// Aggregation helpers
	frameLines int
	// frame marker for per-frame coordination
	lastFrame int64

	// Term variant fields
	termTerm          TermLike
	termLastLineCount VisualLines
	termRateLimiter   *RateLimiter
	termDrawState     DrawStateStub
	// termIsTerm 对应 Rust 的 `console::Term::is_term()`：目标是不是真终端
	// （被重定向到文件/管道时是 false）。Rust 的 `is_hidden()` 对 Term 变体取 `!is_term()`。
	termIsTerm bool

	// TermLike variant fields
	tlInner         TermLike
	tlLastLineCount VisualLines
	tlRateLimiter   *RateLimiter
	tlDrawState     DrawStateStub

	// Multi variant fields
	multiState *MultiState
	multiIdx   int
}

type targetVariant int

const (
	variantTerm targetVariant = iota
	variantMulti
	variantHidden
	variantTermLike
)

// IsHidden 对应 Rust 的 ProgressDrawTarget::is_hidden：
//
//	TargetKind::Hidden          => true
//	TargetKind::Term { term, }  => !term.is_term()
//	TargetKind::Multi { state } => state.read().unwrap().draw_target.is_hidden()
//	_                           => false
func (p *progressDrawTarget) IsHidden() bool {
	switch p.kind.variant {
	case variantHidden:
		return true
	case variantTerm:
		return !p.kind.termIsTerm
	case variantMulti:
		if p.kind.multiState != nil {
			return p.kind.multiState.isHidden()
		}
		return false
	}
	return false
}
func (p *progressDrawTarget) IsStderr() bool { return p.kind.stderr }

// Width 对应 Rust 的 ProgressDrawTarget::width()：Term/TermLike 每次实时问终端宽度，
// Multi 问 MultiState，Hidden 返回 None（Go 里是 nil）。
func (p *progressDrawTarget) Width() *uint16 {
	switch p.kind.variant {
	case variantTerm:
		if p.kind.termTerm != nil {
			w := p.kind.termTerm.Width()
			return &w
		}
	case variantTermLike:
		if p.kind.tlInner != nil {
			w := p.kind.tlInner.Width()
			return &w
		}
	}
	return p.kind.width
}

func (p *progressDrawTarget) Remote() (*RemoteStub, uint, bool) { return nil, 0, false }

func (p *progressDrawTarget) Disconnect(now time.Time) {}

// MarkZombie 对应 Rust 的 ProgressDrawTarget::mark_zombie：
// 只有 Remote 变体有行为，Term/TermLike/Hidden 都是空实现。
func (p *progressDrawTarget) MarkZombie() {}

// AdjustLastLineCount 对应 Rust 的 TargetKind::adjust_last_line_count：
//
//	Clear(count) => last_line_count.saturating_add(count)  // 下一帧把这些行一并清掉
//	Keep(count)  => last_line_count.saturating_sub(count)  // 让 draw target 忘掉这些行，下一帧不清
//
// 之前这里是反的（Clear 做减、Keep 做加），与 multi.go 里照 Rust 写的调用点（标记 zombie 用 Keep、
// 清 zombie 用 Clear）正好南辕北辙。
func (p *progressDrawTarget) AdjustLastLineCount(adjust LineAdjust) {
	p.kind.mu.Lock()
	defer p.kind.mu.Unlock()
	switch adjust.Kind {
	case LineAdjustClear:
		if p.kind.variant == variantTerm {
			p.kind.termLastLineCount = p.kind.termLastLineCount.SaturatingAdd(adjust.Count)
		} else if p.kind.variant == variantTermLike {
			p.kind.tlLastLineCount = p.kind.tlLastLineCount.SaturatingAdd(adjust.Count)
		}
	case LineAdjustKeep:
		if p.kind.variant == variantTerm {
			p.kind.termLastLineCount = p.kind.termLastLineCount.SaturatingSub(adjust.Count)
		} else if p.kind.variant == variantTermLike {
			p.kind.tlLastLineCount = p.kind.tlLastLineCount.SaturatingSub(adjust.Count)
		}
	}
}

// Drawable 返回一个实现 state.go 中 Drawable 接口的对象。
// 如果速率限制不允许绘制，返回 nil。
func (p *progressDrawTarget) Drawable(force bool) Drawable {
	return p.DrawableWithNow(force, time.Now())
}

// DrawableWithNow 允许指定时间以进行速率限制检查。
// 如果速率限制不允许绘制，返回 nil。
func (p *progressDrawTarget) DrawableWithNow(force bool, now time.Time) Drawable {
	p.kind.mu.Lock()
	defer p.kind.mu.Unlock()

	// 检查速率限制（如果 force 为 true 则跳过）。
	// 对应 Rust：Term 变体的限速器必有；TermLike 变体是 Option，`map_or(true, |r| r.allow(now))`
	// —— 没装限速器就一律放行（term_like() 造出来的目标正是这种）。
	allowDraw := force
	if !allowDraw {
		switch p.kind.variant {
		case variantTerm:
			allowDraw = p.kind.termRateLimiter == nil || p.kind.termRateLimiter.allow(now)
		case variantTermLike:
			allowDraw = p.kind.tlRateLimiter == nil || p.kind.tlRateLimiter.allow(now)
		default:
			// Multi 恒可绘制，Hidden 在下面返回 nil
			allowDraw = true
		}
	}

	if !allowDraw {
		return nil // 速率限制，跳过本次绘制
	}

	// Return a drawable that matches the configured variant semantics.
	switch p.kind.variant {
	case variantTerm:
		return &termDrawable{state: &p.kind.termDrawState, width: p.Width(), term: p.kind.termTerm, lastCount: &p.kind.termLastLineCount}
	case variantTermLike:
		return &termLikeDrawable{state: &p.kind.tlDrawState, width: p.Width(), termLike: p.kind.tlInner, lastCount: &p.kind.tlLastLineCount}
	case variantHidden:
		return nil
	default:
		// fallback to simple drawable
		ds := &DrawStateStub{lines: make([]LineType, 0), contents: make([]string, 0)}
		return &simpleDrawable{state: ds, width: p.Width(), owner: p}
	}
}

// termDrawable implements Drawable for the Term variant.
type termDrawable struct {
	state     *DrawStateStub
	width     *uint16
	term      TermLike
	lastCount *VisualLines
}

// State 对应 Rust 的 `Drawable::state()`：**必须先 reset**。
//
//	let mut state = match self { Drawable::Term { draw_state, .. } => ... };
//	state.reset();   // <- draw_target.rs
//	state
//
// Term 变体的 draw_state 是**跨帧持久**的（挂在 TargetKind::Term 上），不 reset 的话
// 每帧 format_state 都往同一份 lines 里 append，行数一帧帧涨：
// 第 1 帧 1 行、第 2 帧 2 行…… 进度条就会一直往下换行（real_height 也跟着涨，
// 于是 MoveCursorUp 越移越多，画面彻底乱掉）。
func (t *termDrawable) State() *DrawStateWrapper {
	t.state.lines = t.state.lines[:0]
	t.state.contents = t.state.contents[:0]
	return DrawStateWrapperForTerm(t.state)
}

func (t *termDrawable) Draw() error {
	// delegate to DrawStateStub.drawToTerm; update lastCount with returned value
	newCount, err := t.state.drawToTerm(t.term, *t.lastCount)
	if err != nil {
		return err
	}
	*t.lastCount = newCount
	return nil
}
func (t *termDrawable) Clear() error {
	// Reset draw state and delegate to centralized draw logic so that
	// clearing actually emits terminal clear sequences (parity with Rust).
	t.state.lines = t.state.lines[:0]
	t.state.contents = t.state.contents[:0]
	newCount, err := t.state.drawToTerm(t.term, *t.lastCount)
	if err != nil {
		return err
	}
	*t.lastCount = newCount
	return nil
}
func (t *termDrawable) Width() *uint16 { return t.width }

// termLikeDrawable implements Drawable for the TermLike variant.
type termLikeDrawable struct {
	state     *DrawStateStub
	width     *uint16
	termLike  TermLike
	lastCount *VisualLines
}

// State 同 termDrawable.State()：TermLike 变体的 draw_state 也是跨帧持久的，
// 每帧都要先 reset（见 Rust 的 `Drawable::state()`）。
func (t *termLikeDrawable) State() *DrawStateWrapper {
	t.state.lines = t.state.lines[:0]
	t.state.contents = t.state.contents[:0]
	return DrawStateWrapperForTerm(t.state)
}
func (t *termLikeDrawable) Draw() error {
	newCount, err := t.state.drawToTerm(t.termLike, *t.lastCount)
	if err != nil {
		return err
	}
	*t.lastCount = newCount
	return nil
}
func (t *termLikeDrawable) Clear() error {
	t.state.lines = t.state.lines[:0]
	t.state.contents = t.state.contents[:0]
	newCount, err := t.state.drawToTerm(t.termLike, *t.lastCount)
	if err != nil {
		return err
	}
	*t.lastCount = newCount
	return nil
}
func (t *termLikeDrawable) Width() *uint16 { return t.width }

// SetMoveCursor 为进度目标设置是否使用移动光标而不是清行（简化实现）。
func (p *progressDrawTarget) SetMoveCursor(move bool) {
	p.kind.mu.Lock()
	switch p.kind.variant {
	case variantTerm:
		p.kind.termDrawState.move_cursor = move
	case variantTermLike:
		p.kind.tlDrawState.move_cursor = move
	default:
		// no-op for other variants
	}
	p.kind.mu.Unlock()
}

func (p *progressDrawTarget) IsMoveCursor() bool {
	p.kind.mu.Lock()
	var v bool
	switch p.kind.variant {
	case variantTerm:
		v = p.kind.termDrawState.move_cursor
	case variantTermLike:
		v = p.kind.tlDrawState.move_cursor
	default:
		v = false
	}
	p.kind.mu.Unlock()
	return v
}

type simpleDrawable struct {
	state           *DrawStateStub
	width           *uint16
	lastDraw        time.Time
	owner           *progressDrawTarget
	lastHeight      int
	lastFrameMarker int64
}

func (d *simpleDrawable) State() *DrawStateWrapper { return DrawStateWrapperForTerm(d.state) }
func (d *simpleDrawable) Draw() error {
	d.lastDraw = time.Now()
	if d.owner != nil {
		if d.owner.kind.termTerm != nil {
			for _, line := range d.state.contents {
				_ = d.owner.kind.termTerm.WriteLine(line)
			}
			_ = d.owner.kind.termTerm.Flush()
			return nil
		}
		// fallback: write to stderr so examples show output
		out := os.Stderr
		// 使用 owner 的帧协调：仅在帧的第一个 Drawable 上移光标一次
		d.owner.kind.mu.Lock()
		currentMarker := d.owner.kind.lastFrame
		if d.lastFrameMarker != currentMarker {
			// First drawable in this frame: delegate to DrawStateStub.drawToTerm
			d.lastFrameMarker = currentMarker
			d.owner.kind.frameLines = 0

			var term TermLike
			if d.owner.kind.termTerm != nil {
				term = d.owner.kind.termTerm
			} else {
				// 这里写的是真实 stderr，对应上游的 Term 变体（buffered_*），用带缓冲构造
				term = newStdTermWithWriter(out, true)
			}
			// call centralized draw logic which will update per-variant last-line count
			switch d.owner.kind.variant {
			case variantTerm:
				newCount, _ := d.state.drawToTerm(term, d.owner.kind.termLastLineCount)
				d.owner.kind.termLastLineCount = newCount
				d.owner.kind.frameLines = newCount.AsInt()
			case variantTermLike:
				newCount, _ := d.state.drawToTerm(term, d.owner.kind.tlLastLineCount)
				d.owner.kind.tlLastLineCount = newCount
				d.owner.kind.frameLines = newCount.AsInt()
			default:
				// fallback to termLastLineCount
				newCount, _ := d.state.drawToTerm(term, d.owner.kind.termLastLineCount)
				d.owner.kind.termLastLineCount = newCount
				d.owner.kind.frameLines = newCount.AsInt()
			}
			d.owner.kind.mu.Unlock()
			return nil
		}
	}
	return nil
}
func (d *simpleDrawable) Clear() error {
	d.state.lines = d.state.lines[:0]
	d.state.contents = d.state.contents[:0]
	return nil
}
func (d *simpleDrawable) Width() *uint16 { return d.width }

// 以下构造函数一一对应 Rust 的 `impl ProgressDrawTarget` 关联函数。

// newTermDrawTarget 对应 `term(term, refresh_rate)`：Term 变体，带速率限制。
// stderr 标记对应 Rust 里 `term.target() == TermTarget::Stderr` 的判定（is_stderr()）。
// isDumb 对应 Rust 的 console::is_dumb()：
//
//	Ok(term) => term == "dumb",
//	Err(_)   => windows: false / 其它平台: true
//
// 即：TERM 显式为 "dumb" 时是哑终端；未设置时 Windows 不算哑（原生终端通常不设 TERM），
// 其它平台算哑。
func isDumb() bool {
	defaultDumb := true
	if runtime.GOOS == "windows" {
		defaultDumb = false
	}
	if term, ok := os.LookupEnv("TERM"); ok {
		return term == "dumb"
	}
	return defaultDumb
}

func newTermDrawTarget(term TermLike, refreshRate uint8, stderr bool) ProgressDrawTarget {
	var w uint16
	if term != nil {
		w = term.Width()
	}
	// Rust 的 `TargetKind::Term { term, .. } => !term.is_term()`：
	// 只有真终端才算可见；被重定向到文件/管道时整条进度条隐藏（"piping to logfiles" 的行为）。
	isTerm := false
	if term != nil {
		if it, ok := term.(interface{ IsTerm() bool }); ok {
			isTerm = it.IsTerm()
		}
	}
	// Rust 0.18.6 把判定提前到 `ProgressDrawTarget::term()` 构造期：
	//
	//	if !term.is_term() || is_dumb() { return Self::hidden(); }
	//
	// 之前这个判定在 `drawable()` 里（每次取 drawable 时才返回 None）。
	if !isTerm || isDumb() {
		return NewHiddenDrawTarget()
	}
	return &progressDrawTarget{kind: targetKind{
		variant:         variantTerm,
		hidden:          false,
		stderr:          stderr,
		width:           &w,
		termTerm:        term,
		termIsTerm:      isTerm,
		termRateLimiter: newRateLimiter(refreshRate),
	}}
}

// newTermLikeDrawTarget 对应 `term_like(term_like)`：TermLike 变体。
// rate 为 nil 表示不装速率限制（Rust 的 `rate_limiter: None`）。
func newTermLikeDrawTarget(termLike TermLike, rate *uint8) ProgressDrawTarget {
	var w uint16
	if termLike != nil {
		w = termLike.Width()
	}
	// targetKind 里含 sync.Mutex，不能先造值再拷进结构体（go vet 会报 copies lock value）
	dt := &progressDrawTarget{}
	dt.kind.variant = variantTermLike
	dt.kind.hidden = false
	dt.kind.stderr = false
	dt.kind.width = &w
	dt.kind.tlInner = termLike
	if rate != nil {
		dt.kind.tlRateLimiter = newRateLimiter(*rate)
	}
	return dt
}

// NewTermDrawTarget 对应 Rust 的 `term(term, refresh_rate)`。
func NewTermDrawTarget(term TermLike, refreshRate uint8) ProgressDrawTarget {
	return newTermDrawTarget(term, refreshRate, false)
}

// NewStdoutDrawTarget 对应 Rust 的 `stdout()`：buffered stdout，每秒最多 20 次。
func NewStdoutDrawTarget() ProgressDrawTarget { return NewStdoutDrawTargetWithHz(20) }

// NewStderrDrawTarget 对应 Rust 的 `stderr()`：buffered stderr，每秒最多 20 次。
// 这是进度条的默认绘制目标（New / NoLength / NewSpinner 都用它）。
func NewStderrDrawTarget() ProgressDrawTarget { return NewStderrDrawTargetWithHz(20) }

// NewStdoutDrawTargetWithHz 对应 Rust 的 `stdout_with_hz(refresh_rate)`。
func NewStdoutDrawTargetWithHz(refreshRate uint8) ProgressDrawTarget {
	return newTermDrawTarget(NewStdTerm(), refreshRate, false)
}

// NewStderrDrawTargetWithHz 对应 Rust 的 `stderr_with_hz(refresh_rate)`。
func NewStderrDrawTargetWithHz(refreshRate uint8) ProgressDrawTarget {
	return newTermDrawTarget(NewStderrTerm(), refreshRate, true)
}

// NewTermLikeDrawTarget 对应 Rust 的 `term_like(term_like)`：
// 注意它没有默认刷新率（不限速），需要限速请用 NewTermLikeDrawTargetWithHz。
func NewTermLikeDrawTarget(termLike TermLike) ProgressDrawTarget {
	return newTermLikeDrawTarget(termLike, nil)
}

// NewTermLikeDrawTargetWithHz 对应 Rust 的 `term_like_with_hz(term_like, refresh_rate)`。
func NewTermLikeDrawTargetWithHz(termLike TermLike, refreshRate uint8) ProgressDrawTarget {
	return newTermLikeDrawTarget(termLike, &refreshRate)
}

// NewHiddenDrawTarget 对应 Rust 的 `hidden()`：完全不渲染。
func NewHiddenDrawTarget() ProgressDrawTarget {
	return &progressDrawTarget{kind: targetKind{variant: variantHidden, hidden: true, stderr: false, width: nil}}
}

// NewInMemoryDrawTarget 创建一个使用 InMemoryTerm 的 draw target（用于测试）。
// 对应 Rust 里 `ProgressDrawTarget::term_like(Box::new(InMemoryTerm::new(..)))` 的用法：不限速。
func NewInMemoryDrawTarget(t *InMemoryTerm) ProgressDrawTarget {
	return NewTermLikeDrawTarget(t)
}
