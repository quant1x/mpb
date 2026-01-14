package mpb

import (
	"os"
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

func (p *progressDrawTarget) IsHidden() bool { return p.kind.hidden }
func (p *progressDrawTarget) IsStderr() bool { return p.kind.stderr }
func (p *progressDrawTarget) Width() *uint16 { return p.kind.width }

func (p *progressDrawTarget) Remote() (*RemoteStub, int, bool) { return nil, 0, false }

func (p *progressDrawTarget) Disconnect(now time.Time) {}

func (p *progressDrawTarget) AdjustLastLineCount(adjust LineAdjust) {
	p.kind.mu.Lock()
	defer p.kind.mu.Unlock()
	switch adjust.Kind {
	// Align with Rust semantics: Keep => add (retain zombie lines), Clear => sub (remove tracked lines)
	case LineAdjustClear:
		if p.kind.variant == variantTerm {
			p.kind.termLastLineCount = p.kind.termLastLineCount.SaturatingSub(adjust.Count)
		} else if p.kind.variant == variantTermLike {
			p.kind.tlLastLineCount = p.kind.tlLastLineCount.SaturatingSub(adjust.Count)
		}
	case LineAdjustKeep:
		if p.kind.variant == variantTerm {
			p.kind.termLastLineCount = p.kind.termLastLineCount.SaturatingAdd(adjust.Count)
		} else if p.kind.variant == variantTermLike {
			p.kind.tlLastLineCount = p.kind.tlLastLineCount.SaturatingAdd(adjust.Count)
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

	// 检查速率限制（如果 force 为 true 则跳过）
	allowDraw := force
	if !allowDraw {
		switch p.kind.variant {
		case variantTerm:
			if p.kind.termRateLimiter != nil {
				allowDraw = p.kind.termRateLimiter.allow(now)
			}
		case variantTermLike:
			if p.kind.tlRateLimiter != nil {
				allowDraw = p.kind.tlRateLimiter.allow(now)
			}
		default:
			// 没有速率限制
		}
	}

	if !allowDraw {
		return nil // 速率限制，跳过本次绘制
	}

	// Return a drawable that matches the configured variant semantics.
	switch p.kind.variant {
	case variantTerm:
		return &termDrawable{state: &p.kind.termDrawState, width: p.kind.width, term: p.kind.termTerm, lastCount: &p.kind.termLastLineCount}
	case variantTermLike:
		return &termLikeDrawable{state: &p.kind.tlDrawState, width: p.kind.width, termLike: p.kind.tlInner, lastCount: &p.kind.tlLastLineCount}
	case variantHidden:
		return nil
	default:
		// fallback to simple drawable
		ds := &DrawStateStub{lines: make([]LineType, 0), contents: make([]string, 0)}
		return &simpleDrawable{state: ds, width: p.kind.width, owner: p}
	}
}

// termDrawable implements Drawable for the Term variant.
type termDrawable struct {
	state     *DrawStateStub
	width     *uint16
	term      TermLike
	lastCount *VisualLines
}

func (t *termDrawable) State() *DrawStateWrapper { return DrawStateWrapperForTerm(t.state) }
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

func (t *termLikeDrawable) State() *DrawStateWrapper { return DrawStateWrapperForTerm(t.state) }
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
				term = NewStdTermWithWriter(out)
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

// 构造函数
func NewStderrDrawTarget() ProgressDrawTarget {
	w := uint16(80)
	return &progressDrawTarget{kind: targetKind{variant: variantTermLike, hidden: false, stderr: true, width: &w, tlInner: NewStdTerm(), tlRateLimiter: newRateLimiter(20)}}
}
func NewHiddenDrawTarget() ProgressDrawTarget {
	return &progressDrawTarget{kind: targetKind{variant: variantHidden, hidden: true, stderr: false, width: nil}}
}

// NewInMemoryDrawTarget 创建一个使用 InMemoryTerm 的 draw target（用于测试）。
func NewInMemoryDrawTarget(t *InMemoryTerm) ProgressDrawTarget {
	w := t.Width()
	return &progressDrawTarget{kind: targetKind{variant: variantTermLike, hidden: false, stderr: false, width: &w, tlInner: t, tlRateLimiter: newRateLimiter(20)}}
}
