// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ProgressBar 是对 Rust `ProgressBar` 的 Go 移植，尽量保持 1:1 的逻辑。
//
// 设计说明：
// - 内部状态由 `BarState` 表示（在 state.go 中实现）。
// - 使用原子变量记录位置，支持并发更新。
// - 支持后台稳定 ticker（EnableSteadyTick/DisableSteadyTick）。
// - 提供与 Rust 接口相似的方法名（小写导出以匹配包内部使用时可见性要求）。
type ProgressBar struct {
	state *SafeBarState
	pos   *AtomicPosition // 使用原子位置，复现 Rust 的 AtomicPosition 行为
	// ticker 已移动到 SafeBarState 以便在克隆间共享
}

// New 创建一个指定长度的进度条。
func New(length uint64) *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(&length, NewStderrDrawTarget(), pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeakState(pb.state)
	return pb
}

// NoLength 创建一个未知长度的进度条。
func NoLength() *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(nil, NewStderrDrawTarget(), pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeakState(pb.state)
	return pb
}

// Hidden 创建一个隐藏的进度条（不渲染）。
func Hidden() *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(nil, NewHiddenDrawTarget(), pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeakState(pb.state)
	return pb
}

// NewWithDrawTarget 使用指定长度和绘制目标创建进度条。
func NewWithDrawTarget(length *uint64, dt ProgressDrawTarget) *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(length, dt, pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeakState(pb.state)
	return pb
}

// Inc 增加位置。
//
// 对应 Rust：
//
//	self.pos.inc(delta);
//	let now = Instant::now();
//	if self.pos.allow(now) {
//	    self.tick_inner(now);   // ← now 是**同一个**时刻，不会重取
//	}
func (p *ProgressBar) Inc(delta uint64) {
	p.pos.Inc(delta)
	now := time.Now()
	if p.pos.Allow(now) {
		p.tick(now)
	}
}

// Dec 减少位置。
func (p *ProgressBar) Dec(delta uint64) {
	p.pos.Dec(delta)
	now := time.Now()
	if p.pos.Allow(now) {
		p.tick(now)
	}
}

// SetPosition 设置绝对位置。
func (p *ProgressBar) SetPosition(pos uint64) {
	p.pos.Set(pos)
	now := time.Now()
	if p.pos.Allow(now) {
		p.tick(now)
	}
}

// IsHidden 判断是否隐藏。
func (p *ProgressBar) IsHidden() bool { return p.state.IsHidden() }

// IsFinished 判断是否完成。
func (p *ProgressBar) IsFinished() bool { return p.state.IsFinished() }

// Finish 标记完成并保留当前消息。
// 与 Rust 一致：不停 ticker，ticker 线程自己会在下一轮检测到 is_finished 后退出。
func (p *ProgressBar) Finish() {
	if l := p.state.Len(); l != nil {
		p.pos.Set(*l)
		p.state.SetPos(*l)
	}
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAndLeave})
}

// FinishWithMessage 完成并设置消息。
func (p *ProgressBar) FinishWithMessage(msg string) {
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishWithMessage, Msg: msg})
}

// FinishAndClear 完成并清除输出。
func (p *ProgressBar) FinishAndClear() {
	if l := p.state.Len(); l != nil {
		p.pos.Set(*l)
		p.state.SetPos(*l)
	}
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAndClear})
}

// Abandon 标记完成但保留当前进度显示。
func (p *ProgressBar) Abandon() {
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAbandon})
}

// AbandonWithMessage 标记完成但保留当前进度显示，并设置消息。
func (p *ProgressBar) AbandonWithMessage(msg string) {
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAbandonWithMessage, Msg: msg})
}

// SetStyle 设置样式。
func (p *ProgressBar) SetStyle(style ProgressStyle) { p.state.SetStyle(style) }

// EnableSteadyTick 启动后台定时器以周期性 tick。
func (p *ProgressBar) EnableSteadyTick(interval time.Duration) {
	if interval <= 0 {
		return
	}
	p.state.startTicker(interval, p)
}

// DisableSteadyTick 停止后台定时器。
func (p *ProgressBar) DisableSteadyTick() {
	p.state.disableTicker()
}

// tick 触发一次绘制，对应 Rust 的 tick_inner：只在没有安装 steady ticker 时动手。
//
//	fn tick_inner(&self, now: Instant) {
//	    if self.ticker.lock().unwrap().is_none() {
//	        self.state().tick(now);
//	    }
//	}
//
// now 由调用方传进来（Rust 也是复用同一个 Instant），不再重取时钟 —— 超大总数的任务
// 里每步一次时钟读取会累积成可观的开销。
func (p *ProgressBar) tick(now time.Time) {
	if p.state.hasTicker() {
		return
	}
	p.state.Tick(now)
}

// Message 返回当前消息。
func (p *ProgressBar) Message() string { return p.state.Message() }

// Prefix 返回当前前缀。
func (p *ProgressBar) Prefix() string { return p.state.Prefix() }

// Suspend 隐藏进度条，执行 f，再重绘，并返回 f 的结果。
// 对应 Rust 的 `suspend<F: FnOnce() -> R, R>(f) -> R`；Go 的方法不支持类型参数，故 R 退化为 any。
func (p *ProgressBar) Suspend(f func() interface{}) interface{} {
	return p.state.Suspend(time.Now(), f)
}

// ForceDraw 强制重绘。
func (p *ProgressBar) ForceDraw() { _ = p.state.Draw(true) }

// WrapRead 包装 io.Reader，对应 Rust 的 wrap_read：返回的是带进度推进的 Read 包装器
// （Rust 里就是同一个 ProgressBarIter<R>，走 seek_max + set_position，见 iter.go）。
func (p *ProgressBar) WrapRead(r io.Reader) io.Reader { return NewProgressReaderIter(r, p) }

// WrapWrite 包装 io.Writer，对应 Rust 的 wrap_write（同上，见 iter.go）。
func (p *ProgressBar) WrapWrite(w io.Writer) io.Writer { return NewProgressWriterIter(w, p) }

// Update 在内部 ProgressState 上执行 f，对应 Rust 的 `update(f: impl FnOnce(&mut ProgressState))`：
// 闭包拿到的是可变状态；随后仅当没有安装 steady ticker 时才 tick（Rust 的 `self.ticker.lock().unwrap().is_none()`）。
func (p *ProgressBar) Update(f func(*ProgressState)) {
	p.state.Update(time.Now(), f, !p.state.hasTicker())
}

// UnsetLength 取消长度。
func (p *ProgressBar) UnsetLength() { p.state.UnsetLength(time.Now()) }

// SetLength 设置长度。
func (p *ProgressBar) SetLength(l uint64) { p.state.SetLength(time.Now(), l) }

// IncLength 增加长度。
func (p *ProgressBar) IncLength(delta uint64) { p.state.IncLength(time.Now(), delta) }

// DecLength 减少长度。
func (p *ProgressBar) DecLength(delta uint64) { p.state.DecLength(time.Now(), delta) }

// SetPrefix 设置前缀（对应 Rust 的 set_prefix，设置后立即更新估算并重绘）。
func (p *ProgressBar) SetPrefix(prefix string) { p.state.SetPrefix(prefix) }

// SetMessage 设置消息（对应 Rust 的 set_message，设置后立即更新估算并重绘）。
func (p *ProgressBar) SetMessage(msg string) { p.state.SetMessage(msg) }

// SetElapsed 设置已用时间（对应 Rust 的 set_elapsed：把起始时间往前推 elapsed）。
func (p *ProgressBar) SetElapsed(elapsed time.Duration) { p.state.SetElapsed(elapsed) }

// Position 返回当前位置
func (p *ProgressBar) Position() uint64 { return p.state.Position() }

// Length 返回可选长度
func (p *ProgressBar) Length() *uint64 { return p.state.Len() }

// Eta 返回 ETA
func (p *ProgressBar) Eta() time.Duration { return p.state.Eta() }

// PerSec 返回速率
func (p *ProgressBar) PerSec() float64 { return p.state.PerSec() }

// Elapsed 返回已用时间
func (p *ProgressBar) Elapsed() time.Duration { return p.state.Elapsed() }

// Duration 返回预计总时长（已用时间 + ETA）
func (p *ProgressBar) Duration() time.Duration { return p.state.Duration() }

// TabWidth 返回当前的制表符宽度
func (p *ProgressBar) TabWidth() int { return p.state.TabWidth() }

// ResetEta 重置 ETA 估算
func (p *ProgressBar) ResetEta() { p.state.Reset(time.Now(), ResetEta) }

// ResetElapsed 重置已用时间
func (p *ProgressBar) ResetElapsed() { p.state.Reset(time.Now(), ResetElapsed) }

// Reset 重置估算与开始时间
func (p *ProgressBar) Reset() { p.state.Reset(time.Now(), ResetAll) }

// Index 返回在 MultiProgress 中的索引（如果存在）
//
// 对应 Rust 的 `ProgressBar::index()`：返回的是 `MultiState::members` 的**不透明下标**
// （槽位会被 free_set 复用，所以它与屏幕上的先后无关）。要看屏幕位置用 VisualIndex。
func (p *ProgressBar) Index() (uint, bool) { return p.state.Index() }

// VisualIndex 返回在 MultiProgress 中的**屏幕位置**（第几行），不在任何
// MultiProgress 里时返回 false。
//
// 对应 Rust 0.18.6 新增的 `ProgressBar::visual_index()`：
//
//	self.state().draw_target.remote().map(|(remote, idx)| remote.read().unwrap().visual_index(idx))
func (p *ProgressBar) VisualIndex() (uint, bool) { return p.state.VisualIndex() }

// 弱引用类型
type WeakProgressBar struct{ id uint64 }

func NewWeakProgressBar() WeakProgressBar { return WeakProgressBar{id: 0} }
func (p *ProgressBar) Downgrade() WeakProgressBar {
	return WeakProgressBar{id: registerWeakState(p.state)}
}
func (w WeakProgressBar) Upgrade() *ProgressBar { return getWeak(w.id) }

// Clone 返回一个对相同内部状态与位置的浅拷贝，语义类似 Rust 的 Arc::clone。
func (p *ProgressBar) Clone() *ProgressBar {
	nb := &ProgressBar{state: p.state, pos: p.pos}
	registerWeakState(nb.state)
	return nb
}

// Style 返回当前样式的拷贝。
func (p *ProgressBar) Style() ProgressStyle { return p.state.Style() }

// WithStyle 链式设置样式并返回自身。
func (p *ProgressBar) WithStyle(style ProgressStyle) *ProgressBar { p.SetStyle(style); return p }

// SetTabWidth 设置制表符宽度（默认 8），与 Rust 一致：设置后强制重绘一次。
func (p *ProgressBar) SetTabWidth(w int) {
	p.state.SetTabWidth(w)
	_ = p.state.Draw(true)
}

// WithTabWidth 链式设置制表符宽度（与 Rust 一致，链式设置不触发重绘）。
func (p *ProgressBar) WithTabWidth(w int) *ProgressBar { p.state.SetTabWidth(w); return p }

// WithPrefix 链式设置前缀。
func (p *ProgressBar) WithPrefix(prefix string) *ProgressBar {
	p.state.SetPrefix(prefix)
	return p
}

// WithMessage 链式设置消息。
func (p *ProgressBar) WithMessage(msg string) *ProgressBar {
	p.state.SetMessage(msg)
	return p
}

// WithPosition 链式设置位置。
func (p *ProgressBar) WithPosition(pos uint64) *ProgressBar { p.SetPosition(pos); return p }

// WithElapsed 链式设置已用时间（简化实现）。
func (p *ProgressBar) WithElapsed(d time.Duration) *ProgressBar { p.state.SetElapsed(d); return p }

// WithFinish 链式设置完成行为。
func (p *ProgressBar) WithFinish(f ProgressFinish) *ProgressBar { p.state.SetOnFinish(f); return p }

// NewSpinner 创建一个 spinner 风格的进度条（对应 Rust 的 ProgressStyle::default_spinner()）。
func NewSpinner() *ProgressBar {
	pb := NoLength()
	pb.SetStyle(DefaultProgressStyleSpinner())
	return pb
}

// FinishUsingStyle 使用当前存储的样式完成进度条（与 Rust 行为一致）。
func (p *ProgressBar) FinishUsingStyle() {
	p.state.FinishUsingStyle(time.Now(), p.state.GetOnFinish())
}

// SetDrawTarget 切换绘制目标（会断开先前目标）。
func (p *ProgressBar) SetDrawTarget(dt ProgressDrawTarget) { p.state.SetDrawTarget(dt) }

// Println 打印一行到进度条上方。
func (p *ProgressBar) Println(msg string) { p.state.Println(time.Now(), msg) }

// WrapSlice 为切片创建一个 ProgressBarIter（Go 中的替代 wrap_iter）。
func WrapSlice[T any](p *ProgressBar, slice []T) *ProgressBarIter[T] {
	return NewProgressBarIterFromSlice(slice, p)
}

// TickerHandle 是一个简单 ticker 控制句柄，供 EnableSteadyTick 使用。
type TickerHandle struct {
	stop chan struct{}
	once sync.Once
}

// NewTickerHandle 启动后台 ticker，语义对应 Rust 的 `Ticker::new` + `TickerControl::run`：
// 循环体是「先判完成 → tick → 等一个 interval」，进度条一完成就自行退出，不需要调用方显式停。
// 另一条退出路径（进度条被回收）由 SafeBarState 上的 finalizer 兜住，等价于 Rust 的 `Weak::upgrade()` 失败。
func NewTickerHandle(interval time.Duration, pb *ProgressBar) *TickerHandle {
	th := &TickerHandle{stop: make(chan struct{})}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			if pb.state.IsFinished() {
				return
			}
			// 直接 tick BarState：对应 Rust 的 TickerControl::run，不走 ProgressBar::tick_inner 的守卫
			pb.state.Tick(time.Now())

			// 等一个 interval；被通知停则提前返回（Rust 的 wait_timeout_while + timed_out 判定）
			select {
			case <-t.C:
			case <-th.stop:
				return
			}
		}
	}()
	return th
}

func (th *TickerHandle) Stop() { th.once.Do(func() { close(th.stop) }) }

// tickerSlot 封装了一个可选的 TickerHandle 与其互斥锁，目的是将 ticker 与锁组合为单个字段，
// 与 Rust 的 `Arc<Mutex<Option<Ticker>>>` 在结构体字段数上保持一致。
type tickerSlot struct {
	mu sync.Mutex
	th *TickerHandle
}

// AtomicPosition 是对 Rust 中 AtomicPosition 的复现。
// 它包含位置计数、令牌桶（capacity）、上次允许时间（prev）和起始时间（start）。
type AtomicPosition struct {
	pos      uint64 // 原子
	capacity uint32 // 原子，使用 uint32 存储 u8
	prev     uint64 // 原子，纳秒自 start
	start    time.Time
}

const (
	atomicInterval = 1_000_000 // ns (1ms)
	atomicMaxBurst = 10
)

func NewAtomicPosition() *AtomicPosition {
	return &AtomicPosition{
		pos:      0,
		capacity: atomicMaxBurst,
		prev:     0,
		start:    time.Now(),
	}
}

func (a *AtomicPosition) Inc(delta uint64) {
	atomic.AddUint64(&a.pos, delta)
}

func (a *AtomicPosition) Dec(delta uint64) {
	atomic.AddUint64(&a.pos, ^(delta - 1))
}

func (a *AtomicPosition) Set(v uint64) {
	atomic.StoreUint64(&a.pos, v)
}

func (a *AtomicPosition) Pos() uint64 {
	return atomic.LoadUint64(&a.pos)
}

// Allow 模拟 Rust 的 allow() 行为：基于时间窗为 tick 决定是否允许。
func (a *AtomicPosition) Allow(now time.Time) bool {
	if now.Before(a.start) {
		return false
	}

	// 载入当前容量和 prev
	capacity := atomic.LoadUint32(&a.capacity)
	prev := atomic.LoadUint64(&a.prev)

	elapsed := uint64(now.Sub(a.start).Nanoseconds())
	diff := elapsed
	if elapsed > prev {
		diff = elapsed - prev
	} else {
		diff = 0
	}

	if capacity == 0 && diff < atomicInterval {
		return false
	}

	newIntervals := diff / atomicInterval
	remainder := diff % atomicInterval

	// Rust：`capacity = min(MAX_BURST, capacity + new - 1)`
	// 注意顺序：先加 new、再减 1、最后才钳到 MAX_BURST。
	// Go 之前是「先钳到 MAX_BURST 再减 1」，爆量时每次少一个令牌（限流比 Rust 严）。
	var newCap uint64 = uint64(capacity) + uint64(newIntervals) - 1
	if newCap > atomicMaxBurst {
		newCap = atomicMaxBurst
	}

	atomic.StoreUint32(&a.capacity, uint32(newCap))
	atomic.StoreUint64(&a.prev, elapsed-remainder)
	return true
}

func (a *AtomicPosition) Reset(now time.Time) {
	a.Set(0)
	// Rust：`now.saturating_duration_since(self.start)`（now 早于 start 时取 0），
	// Go 直接相减会把负值转成巨大的 uint64，把 prev 写坏。
	elapsed := now.Sub(a.start).Nanoseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	atomic.StoreUint64(&a.prev, uint64(elapsed))
}
