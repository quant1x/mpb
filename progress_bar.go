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
	// 与 Rust 中的 `Arc<Mutex<Option<Ticker>>>` 对应：使用一个带锁的槽来保存可选的 ticker
	ticker *tickerSlot
}

// New 创建一个指定长度的进度条。
func New(length uint64) *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(&length, NewStderrDrawTarget(), pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeak(pb)
	return pb
}

// NoLength 创建一个未知长度的进度条。
func NoLength() *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(nil, NewStderrDrawTarget(), pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeak(pb)
	return pb
}

// Hidden 创建一个隐藏的进度条（不渲染）。
func Hidden() *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(nil, NewHiddenDrawTarget(), pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeak(pb)
	return pb
}

// NewWithDrawTarget 使用指定长度和绘制目标创建进度条。
func NewWithDrawTarget(length *uint64, dt ProgressDrawTarget) *ProgressBar {
	pos := NewAtomicPosition()
	s := NewBarState(length, dt, pos)
	pb := &ProgressBar{state: NewSafeBarState(s), pos: pos}
	registerWeak(pb)
	return pb
}

// Inc 增加位置。
func (p *ProgressBar) Inc(delta uint64) {
	p.pos.Inc(delta)
	now := time.Now()
	if p.pos.Allow(now) {
		p.tick()
	}
}

// Dec 减少位置。
func (p *ProgressBar) Dec(delta uint64) {
	p.pos.Dec(delta)
	now := time.Now()
	if p.pos.Allow(now) {
		p.tick()
	}
}

// SetPosition 设置绝对位置。
func (p *ProgressBar) SetPosition(pos uint64) {
	p.pos.Set(pos)
	now := time.Now()
	if p.pos.Allow(now) {
		p.tick()
	}
}

// IsHidden 判断是否隐藏。
func (p *ProgressBar) IsHidden() bool { return p.state.IsHidden() }

// IsFinished 判断是否完成。
func (p *ProgressBar) IsFinished() bool { return p.state.IsFinished() }

// Finish 标记完成并保留当前消息。
func (p *ProgressBar) Finish() {
	p.stopTicker()
	if l := p.state.Len(); l != nil {
		p.pos.Set(*l)
		p.state.SetPos(*l)
	}
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAndLeave})
}

// FinishWithMessage 完成并设置消息。
func (p *ProgressBar) FinishWithMessage(msg string) {
	p.stopTicker()
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishWithMessage, Msg: msg})
}

// FinishAndClear 完成并清除输出。
func (p *ProgressBar) FinishAndClear() {
	p.stopTicker()
	if l := p.state.Len(); l != nil {
		p.pos.Set(*l)
		p.state.SetPos(*l)
	}
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAndClear})
}

// Abandon 标记完成但保留当前进度显示。
func (p *ProgressBar) Abandon() {
	p.stopTicker()
	p.state.FinishUsingStyle(time.Now(), ProgressFinish{Kind: FinishAbandon})
}

// SetStyle 设置样式。
func (p *ProgressBar) SetStyle(style ProgressStyle) { p.state.SetStyle(style) }

// EnableSteadyTick 启动后台定时器以周期性 tick。
func (p *ProgressBar) EnableSteadyTick(interval time.Duration) {
	if interval <= 0 {
		return
	}
	if p.ticker == nil {
		p.ticker = &tickerSlot{}
	}
	p.ticker.mu.Lock()
	defer p.ticker.mu.Unlock()
	if p.ticker.th != nil {
		p.ticker.th.Stop()
	}
	p.ticker.th = NewTickerHandle(interval, p)
}

// DisableSteadyTick 停止后台定时器。
func (p *ProgressBar) DisableSteadyTick() {
	if p.ticker == nil {
		return
	}
	p.ticker.mu.Lock()
	if p.ticker.th != nil {
		p.ticker.th.Stop()
		p.ticker.th = nil
	}
	p.ticker.mu.Unlock()
}

func (p *ProgressBar) stopTicker() {
	if p.ticker == nil {
		return
	}
	p.ticker.mu.Lock()
	if p.ticker.th != nil {
		p.ticker.th.Stop()
		p.ticker.th = nil
	}
	p.ticker.mu.Unlock()
}

// tick 触发一次绘制（如果没有稳定 ticker）。
func (p *ProgressBar) tick() {
	p.state.Tick(time.Now())
}

// Message 返回当前消息。
func (p *ProgressBar) Message() string { return p.state.Message() }

// Prefix 返回当前前缀。
func (p *ProgressBar) Prefix() string { return p.state.Prefix() }

// Suspend 隐藏进度条并在 f 执行后重绘。
func (p *ProgressBar) Suspend(f func()) {
	_ = p.state.Suspend(time.Now(), func() interface{} { f(); return nil })
}

// ForceDraw 强制重绘。
func (p *ProgressBar) ForceDraw() { _ = p.state.Draw(true) }

// WrapRead 包装 io.Reader，使读取时更新进度。
func (p *ProgressBar) WrapRead(r io.Reader) io.Reader { return &ProgressReader{r: r, pb: p} }

// WrapWrite 包装 io.Writer，使写入时更新进度。
func (p *ProgressBar) WrapWrite(w io.Writer) io.Writer { return &ProgressWriter{w: w, pb: p} }

// Update 执行闭包并触发 redraw。
func (p *ProgressBar) Update(f func()) { f(); p.tick() }

// UnsetLength 取消长度。
func (p *ProgressBar) UnsetLength() { p.state.UnsetLength(time.Now()) }

// SetLength 设置长度。
func (p *ProgressBar) SetLength(l uint64) { p.state.SetLength(time.Now(), l) }

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

// ResetEta 重置 ETA 估算
func (p *ProgressBar) ResetEta() { p.state.Reset(time.Now(), ResetEta) }

// ResetElapsed 重置已用时间
func (p *ProgressBar) ResetElapsed() { p.state.Reset(time.Now(), ResetElapsed) }

// Reset 重置估算与开始时间
func (p *ProgressBar) Reset() { p.state.Reset(time.Now(), ResetAll) }

// Index 返回在 MultiProgress 中的索引（如果存在）
func (p *ProgressBar) Index() (int, bool) { return p.state.Index() }

// 弱引用类型
type WeakProgressBar struct{ id uint64 }

func NewWeakProgressBar() WeakProgressBar         { return WeakProgressBar{id: 0} }
func (p *ProgressBar) Downgrade() WeakProgressBar { return WeakProgressBar{id: registerWeak(p)} }
func (w WeakProgressBar) Upgrade() *ProgressBar   { return getWeak(w.id) }

// 辅助包装类型：ProgressReader/Writer
type ProgressReader struct {
	r  io.Reader
	pb *ProgressBar
}

func (pr *ProgressReader) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	if n > 0 {
		pr.pb.Inc(uint64(n))
	}
	return n, err
}

type ProgressWriter struct {
	w  io.Writer
	pb *ProgressBar
}

func (pw *ProgressWriter) Write(b []byte) (int, error) {
	n, err := pw.w.Write(b)
	if n > 0 {
		pw.pb.Inc(uint64(n))
	}
	return n, err
}

// Style 返回当前样式的拷贝。
func (p *ProgressBar) Style() ProgressStyle { return p.state.Style() }

// WithStyle 链式设置样式并返回自身。
func (p *ProgressBar) WithStyle(style ProgressStyle) *ProgressBar { p.SetStyle(style); return p }

// WithTabWidth 链式设置制表符宽度。
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
func (p *ProgressBar) WithElapsed(d time.Duration) *ProgressBar { _ = d; return p }

// WithFinish 链式设置完成行为。
func (p *ProgressBar) WithFinish(f ProgressFinish) *ProgressBar { p.state.SetOnFinish(f); return p }

// NewSpinner 创建一个 spinner 风格的进度条。
func NewSpinner() *ProgressBar {
	pb := NoLength()
	pb.SetStyle(DefaultProgressStyleBar())
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

func NewTickerHandle(interval time.Duration, pb *ProgressBar) *TickerHandle {
	th := &TickerHandle{stop: make(chan struct{})}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				pb.tick()
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

	// 计算新的 capacity：capacity + newIntervals - 1，且不超过 MAX
	var newCap uint32 = capacity
	if newIntervals > 0 {
		// 避 overflow
		added := uint32(newIntervals)
		if uint64(newCap)+uint64(added) > atomicMaxBurst {
			newCap = atomicMaxBurst
		} else {
			newCap = newCap + added
		}
	}
	if newCap > 0 {
		newCap = newCap - 1
	}
	if newCap > atomicMaxBurst {
		newCap = atomicMaxBurst
	}

	atomic.StoreUint32(&a.capacity, newCap)
	atomic.StoreUint64(&a.prev, elapsed-remainder)
	return true
}

func (a *AtomicPosition) Reset(now time.Time) {
	a.Set(0)
	elapsed := uint64(now.Sub(a.start).Nanoseconds())
	atomic.StoreUint64(&a.prev, elapsed)
}
