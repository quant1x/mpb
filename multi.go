// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"sync"
	"time"
)

// 简化的 MultiProgress / MultiState 实现，便于增量迁移。只实现必要的方法和数据结构，行为为保守的占位实现。

// MultiProgress 对应 Rust 的 `pub struct MultiProgress { state: Arc<RwLock<MultiState>> }`。
//
// Rust 0.18.6 把 `state` 从 `pub(crate)` 改成了 `pub`（同时 `MultiState` 也 pub 了），
// 便于外部直接读写 multi 的状态（成员、orphan 行、对齐方式等）。Go 这边照做：字段导出。
// 注意 MultiState 自带 RWMutex，访问前要像 Rust 的 `read()/write()` 那样先加锁。
type MultiProgress struct {
	State *MultiState
}

type MultiProgressAlignment int

const (
	MultiProgressAlignmentTop MultiProgressAlignment = iota
	MultiProgressAlignmentBottom
)

func NewMultiProgress() *MultiProgress {
	return DefaultMultiProgress()
}

func DefaultMultiProgress() *MultiProgress {
	return WithDrawTarget(NewStderrDrawTarget())
}

func WithDrawTarget(dt ProgressDrawTarget) *MultiProgress {
	return &MultiProgress{State: NewMultiState(dt)}
}

func (mp *MultiProgress) New() *MultiProgress { return DefaultMultiProgress() }

func (mp *MultiProgress) WithDrawTarget(dt ProgressDrawTarget) *MultiProgress {
	return WithDrawTarget(dt)
}

// SetDrawTarget 对应 Rust 的 MultiProgress::set_draw_target：
//
//	state.draw_target.disconnect(Instant::now());   // 先断开旧的
//	state.draw_target = target;                     // 再换新的
//
// Go 这边 MultiState 持有的是 multiDrawTarget 包装层，所以断开的是包装层里的旧目标，
// 换上去的也重新包一层（包装层负责聚合出帧，不能直接丢）。
func (mp *MultiProgress) SetDrawTarget(target ProgressDrawTarget) {
	mp.State.mu.Lock()
	defer mp.State.mu.Unlock()
	mp.State.draw_target.Disconnect(time.Now())
	mp.State.draw_target = &multiDrawTarget{ms: mp.State, inner: target}
}

func (mp *MultiProgress) SetMoveCursor(moveCursor bool) {
	mp.State.mu.Lock()
	if setter, ok := mp.State.draw_target.(interface{ SetMoveCursor(bool) }); ok {
		setter.SetMoveCursor(moveCursor)
	}
	mp.State.mu.Unlock()
}

func (mp *MultiProgress) SetAlignment(alignment MultiProgressAlignment) {
	mp.State.mu.Lock()
	mp.State.alignment = alignment
	mp.State.mu.Unlock()
}

func (mp *MultiProgress) Add(pb *ProgressBar) *ProgressBar {
	return mp.internalize(InsertLocationEnd, 0, pb)
}

// Insert / InsertFromBack 的 index 是 uint，对应 Rust 的 `insert(index: usize, ..)`。
func (mp *MultiProgress) Insert(index uint, pb *ProgressBar) *ProgressBar {
	return mp.internalize(InsertLocationIndex, index, pb)
}

func (mp *MultiProgress) InsertFromBack(index uint, pb *ProgressBar) *ProgressBar {
	return mp.internalize(InsertLocationIndexFromBack, index, pb)
}

func (mp *MultiProgress) InsertBefore(before *ProgressBar, pb *ProgressBar) *ProgressBar {
	if idx, ok := before.Index(); ok {
		return mp.internalize(InsertLocationBefore, idx, pb)
	}
	return mp.Add(pb)
}

func (mp *MultiProgress) InsertAfter(after *ProgressBar, pb *ProgressBar) *ProgressBar {
	// Rust：`self.internalize(InsertLocation::After(after.index().unwrap()), pb)`
	idx, ok := after.Index()
	if ok {
		return mp.internalize(InsertLocationAfter, idx, pb)
	}
	return mp.Add(pb)
}

// Remove 对应 Rust 的 MultiProgress::remove：
//  1. 只移除「确实插在本 MultiProgress 上」的进度条（Rust 是 `assert!(Arc::ptr_eq(..))`，
//     Go 里没有 Arc，退化成比 Remote() 拿到的 MultiState 是否同一个）；
//  2. 先把这条 bar 的绘制目标置成 hidden，再回收槽位 —— 不置的话，旧 ProgressBar 仍指向
//     该槽位，槽位被后来的 bar 复用后它会画到别人的行上。
func (mp *MultiProgress) Remove(pb *ProgressBar) {
	// Rust：`let idx = match &state.draw_target.remote() { Some((state, idx)) => { assert!(Arc::ptr_eq(&self.state, state)); *idx }, _ => return };`
	remote, idx, ok := remoteTargetOf(pb)
	if !ok || remote == nil || remote.ms != mp.State {
		return
	}
	// Rust：`state.draw_target = ProgressDrawTarget::hidden();`
	// —— 注意这里是**直接替换字段**，不走 `set_draw_target`（也就不触发 disconnect）。
	setDrawTargetNoDisconnect(pb, NewHiddenDrawTarget())

	mp.State.mu.Lock()
	defer mp.State.mu.Unlock()
	mp.State.removeIdx(idx)
}

// setDrawTargetNoDisconnect 直接替换进度条的绘制目标，不触发旧目标的 disconnect。
// 对应 Rust 里 `MultiProgress::remove` 的 `state.draw_target = ProgressDrawTarget::hidden();`
// （Rust 的 `set_draw_target` 会 disconnect，但 remove 走的是直接赋值）。
func setDrawTargetNoDisconnect(pb *ProgressBar, dt ProgressDrawTarget) {
	if pb == nil || pb.state == nil {
		return
	}
	pb.state.mu.Lock()
	defer pb.state.mu.Unlock()
	if pb.state.inner != nil {
		pb.state.inner.drawTarget = dt
	}
}

// remoteTargetOf 取进度条当前绘制目标上的 remote 信息，对应 Rust 的 `pb.state().draw_target.remote()`。
func remoteTargetOf(pb *ProgressBar) (*RemoteStub, uint, bool) {
	if pb == nil || pb.state == nil {
		return nil, 0, false
	}
	pb.state.mu.Lock()
	defer pb.state.mu.Unlock()
	if pb.state.inner == nil || pb.state.inner.drawTarget == nil {
		return nil, 0, false
	}
	return pb.state.inner.drawTarget.Remote()
}

func (mp *MultiProgress) internalize(loc InsertLocation, pos uint, pb *ProgressBar) *ProgressBar {
	// Install/refresh the weak-registry id before taking the multi lock; the
	// member only stores this number, so it never keeps BarState alive.
	sbID := pb.state.weakID()
	mp.State.mu.Lock()
	idx := mp.State.insertWithPos(loc, pos)
	// register this ProgressBar's weak id in members
	if idx < uint(len(mp.State.members)) {
		mp.State.members[idx].sbID = sbID
	}
	// Rust：`drop(state)` —— 先放开 multi 的写锁，再去动进度条自己的绘制目标，
	// 否则锁顺序（multi → bar）与别处（bar → multi）相反。
	mp.State.mu.Unlock()

	// ensure the bar draws into per-bar remote draw target so multi can identify it
	pb.SetDrawTarget(NewRemoteDrawTarget(mp.State, idx))
	return pb
}

// remoteDrawTarget is a per-bar draw target that forwards aggregate draw requests
// to the owning MultiState while exposing remote information.
type remoteDrawTarget struct {
	ms  *MultiState
	idx uint
}

func NewRemoteDrawTarget(ms *MultiState, idx uint) ProgressDrawTarget {
	return &remoteDrawTarget{ms: ms, idx: idx}
}

func (r *remoteDrawTarget) IsHidden() bool { return r.ms.draw_target.IsHidden() }
func (r *remoteDrawTarget) IsStderr() bool { return r.ms.draw_target.IsStderr() }
func (r *remoteDrawTarget) Width() *uint16 { return r.ms.draw_target.Width() }
func (r *remoteDrawTarget) Remote() (*RemoteStub, uint, bool) {
	return &RemoteStub{ms: r.ms}, r.idx, true
}

// Disconnect 对应 Rust 的 ProgressDrawTarget::disconnect 的 Multi（= Go 的 remote）分支：
//
//	TargetKind::Multi { idx, state, .. } => Drawable::Multi { state, idx, force_draw: true, now }.clear()
//
// 即：把这条 bar 在 multi 里的 draw_state **清空**并重绘（BarState::clear 的语义），
// 而不是把它从 multi 里摘掉。之前是 no-op，导致 set_draw_target 换目标后旧行残留。
func (r *remoteDrawTarget) Disconnect(now time.Time) {
	r.ms.mu.Lock()
	defer r.ms.mu.Unlock()
	dsWrap := r.ms.drawStateFor(r.idx)
	if dsWrap == nil {
		return
	}
	// Rust 的 `Drawable::clear()`：`self.state()`（会 reset）+ `self.draw()`
	dsWrap.Reset()
	_, _ = r.ms.draw(true, nil, nil, now)
}

// Drawable / DrawableWithNow：multi 的 per-bar 目标不自己出 drawable，
// 绘制统一走 AggregateDrawBar（对应 Rust 的 Drawable::Multi）。
func (r *remoteDrawTarget) Drawable(force bool) Drawable                       { return nil }
func (r *remoteDrawTarget) DrawableWithNow(force bool, now time.Time) Drawable { return nil }
func (r *remoteDrawTarget) AdjustLastLineCount(adjust LineAdjust) {
	r.ms.draw_target.AdjustLastLineCount(adjust)
}

// MarkZombie 对应 Rust 的 `TargetKind::Remote { state, idx } => state.write().unwrap().mark_zombie(*idx)`。
// 这是僵尸机制的入口：进度条"死亡"（Rust 里是 BarState 被 drop）时通知所属的 MultiProgress。
func (r *remoteDrawTarget) MarkZombie() {
	r.ms.mu.Lock()
	defer r.ms.mu.Unlock()
	r.ms.markZombie(r.idx)
}

// aggregateDraw 对应 Rust 里 `Drawable::Multi` 那条路径（BarState::draw / println）：
//
//	let mut draw_state = drawable.state();          // = MultiState::draw_state(idx)，且会先 reset
//	if let Some(width) = drawable.width() {
//	    if !matches!(self.state.status, Status::DoneHidden(_)) {
//	        self.style.format_state(&self.state, &mut draw_state.lines, width);
//	    }
//	}
//	drop(draw_state);   // Text/Empty 行被剥进 orphan_lines
//	drawable.draw()     // = MultiState::draw(force, None, now)
//
// 关键：这条 bar 的行是写进**它自己的成员 draw_state**（持久），不是塞进本次调用的
// extra_lines —— 所以组帧时每个成员都贡献自己最后那帧，与"谁触发的绘制"无关。
func (r *remoteDrawTarget) aggregateDraw(b *BarState, force bool, now time.Time, msgLines []LineType, msgContents []string) error {
	r.ms.mu.Lock()
	defer r.ms.mu.Unlock()

	dsWrap := r.ms.drawStateFor(r.idx)
	if dsWrap == nil {
		return nil
	}
	ds := dsWrap.StatePtr
	// Rust 的 `Drawable::state()` 里会 `state.reset()`
	ds.lines = ds.lines[:0]
	ds.contents = ds.contents[:0]

	// ProgressBar::println 的消息行先写进去，随后被 Close() 剥到 orphan 队列，
	// 本帧就出现在所有 bar 之上。
	for i, lt := range msgLines {
		ds.lines = append(ds.lines, lt)
		if i < len(msgContents) {
			ds.contents = append(ds.contents, msgContents[i])
		} else {
			ds.contents = append(ds.contents, "")
		}
	}

	if wptr := r.ms.width(); wptr != nil {
		// Rust 只跳过 DoneHidden：finish/finish_with_message/abandon 属 DoneVisible，最终帧仍要画
		if b.state.status.kind != StatusKindDoneHidden {
			isErr := false
			if b.drawTarget != nil {
				isErr = b.drawTarget.IsStderr()
			}
			lines, contents := b.style.FormatState(&b.state, int(*wptr), isErr)
			ds.lines = append(ds.lines, lines...)
			ds.contents = append(ds.contents, contents...)
		}
	}
	// 对应 Rust 的 `drop(draw_state)`：把 Text/Empty 行剥进 ms.orphanLines
	dsWrap.Close()

	_, err := r.ms.draw(force, nil, nil, now)
	return err
}

func (r *remoteDrawTarget) AggregateDrawBar(b *BarState, force bool, now time.Time) error {
	return r.aggregateDraw(b, force, now, nil, nil)
}

func (r *remoteDrawTarget) AggregateDrawOrigin(originLines []LineType, originContents []string, b *BarState, force bool, now time.Time) error {
	return r.aggregateDraw(b, force, now, originLines, originContents)
}

// Println 对应 Rust 的 MultiProgress::println：返回底层 io 错误（之前恒返回 nil，错误被吞）。
func (mp *MultiProgress) Println(msg string) error {
	mp.State.mu.Lock()
	defer mp.State.mu.Unlock()
	_, err := mp.State.println(msg, time.Now())
	return err
}

func (mp *MultiProgress) Suspend(f func() interface{}) interface{} {
	mp.State.mu.Lock()
	defer mp.State.mu.Unlock()
	return mp.State.suspend(f, time.Now())
}

// Clear 对应 Rust 的 MultiProgress::clear：返回底层 io 错误（之前恒返回 nil，错误被吞）。
func (mp *MultiProgress) Clear() error {
	mp.State.mu.Lock()
	defer mp.State.mu.Unlock()
	_, err := mp.State.clear(time.Now())
	return err
}

func (mp *MultiProgress) IsHidden() bool {
	mp.State.mu.RLock()
	defer mp.State.mu.RUnlock()
	return msIsHidden(mp.State)
}
func msIsHidden(ms *MultiState) bool { return ms.draw_target.IsHidden() }

// --- MultiState and helpers ---
type MultiState struct {
	mu               sync.RWMutex
	members          []MultiStateMember
	freeSet          []uint
	ordering         []uint
	draw_target      ProgressDrawTarget
	alignment        MultiProgressAlignment
	orphanLines      []string
	zombieLinesCount VisualLines
}

func NewMultiState(dt ProgressDrawTarget) *MultiState {
	ms := &MultiState{
		members:          []MultiStateMember{},
		freeSet:          []uint{},
		ordering:         []uint{},
		draw_target:      dt,
		alignment:        MultiProgressAlignmentTop,
		orphanLines:      []string{},
		zombieLinesCount: VisualLines(0),
	}
	// wrap the provided draw target so MultiState can aggregate per-bar draws
	ms.draw_target = &multiDrawTarget{ms: ms, inner: dt}
	return ms
}

// MultiStateMember 对应 Rust 的 `struct MultiStateMember`：
//
//	draw_state: Option<DrawState>  —— 成员上一次绘制留下的行
//	is_zombie: bool                —— 对应的 ProgressBar（准确说是 BarState）是否已"死亡"
type MultiStateMember struct {
	// sbID is the weak-registry id of the owning SafeBarState, or 0 when the
	// slot is free. It must stay a plain number: a strong *SafeBarState here
	// would prevent the finalizer from emulating Rust's `Drop for BarState`.
	sbID uint64
	// drawState 对应 Rust 的 `member.draw_state`：成员上次绘制产出的行。
	// 未绘制过、或槽位已被 remove_idx 回收时为 nil（Rust 的 `None`）。
	//
	// 之所以要持久保存：Rust 组帧时是「把每个成员自己的 draw_state 拼起来」，
	// 与"这次是谁触发的绘制"无关 —— 某条 bar 不 tick 也不会从画面上消失。
	drawState *DrawStateStub
	isZombie  bool
}

// drawStateFor 对应 Rust 的 MultiState::draw_state(idx)：取成员的持久 DrawState
// （没有就新建一个），并挂上 orphan 收集器 —— 文本行随后会被 Close() 剥到 orphan 队列。
func (ms *MultiState) drawStateFor(idx uint) *DrawStateWrapper {
	if idx >= uint(len(ms.members)) {
		return nil
	}
	m := &ms.members[idx]
	if m.drawState == nil {
		m.drawState = &DrawStateStub{lines: make([]LineType, 0), contents: make([]string, 0)}
	}
	return DrawStateWrapperForMulti(m.drawState, &ms.orphanLines)
}

// memberVisualLines 对应 Rust 里用 `member.draw_state.visual_line_count(..)` 数僵尸行：
// 取的是成员**上次绘制**留下的行数，而不是重新格式化一遍。
func (ms *MultiState) memberVisualLines(idx uint, width int) VisualLines {
	if idx >= uint(len(ms.members)) {
		return VisualLines(0)
	}
	if m := &ms.members[idx]; m.drawState != nil {
		return visualLineCount(m.drawState.lines, m.drawState.contents, width)
	}
	return VisualLines(0)
}

// multiDrawTarget wraps an underlying ProgressDrawTarget and provides an
// Aggregate method used by BarState to request an aggregated draw.
type multiDrawTarget struct {
	ms    *MultiState
	inner ProgressDrawTarget
	agg   *aggregatedDrawable
}

func (m *multiDrawTarget) IsHidden() bool                    { return m.inner.IsHidden() }
func (m *multiDrawTarget) IsStderr() bool                    { return m.inner.IsStderr() }
func (m *multiDrawTarget) Width() *uint16                    { return m.inner.Width() }
func (m *multiDrawTarget) Remote() (*RemoteStub, uint, bool) { return m.inner.Remote() }
func (m *multiDrawTarget) Disconnect(now time.Time)          { m.inner.Disconnect(now) }
func (m *multiDrawTarget) MarkZombie() {
	if mz, ok := m.inner.(interface{ MarkZombie() }); ok {
		mz.MarkZombie()
	}
}

// Return a single aggregated drawable so MultiState can prepare its
// combined frame into the returned state's buffer and call Draw() once.
func (m *multiDrawTarget) Drawable(force bool) Drawable {
	return m.DrawableWithNow(force, time.Now())
}

// DrawableWithNow 同 Drawable，但用调用方给的 now 去做速率判定（Rust 的 drawable(force, now)）。
func (m *multiDrawTarget) DrawableWithNow(force bool, now time.Time) Drawable {
	// Check rate limit by calling DrawableWithNow on the inner target
	if pt, ok := m.inner.(*progressDrawTarget); ok {
		if drawable := pt.DrawableWithNow(force, now); drawable != nil {
			// Rate limiter allowed drawing, create aggregated drawable
			if m.agg == nil {
				m.agg = &aggregatedDrawable{state: &DrawStateStub{lines: make([]LineType, 0), contents: make([]string, 0)}, mdt: m}
			}
			return m.agg
		}
		// Rate limited, return nil to skip this draw
		return nil
	}
	// Fallback for other draw target types
	if m.agg == nil {
		m.agg = &aggregatedDrawable{state: &DrawStateStub{lines: make([]LineType, 0), contents: make([]string, 0)}, mdt: m}
	}
	return m.agg
}
func (m *multiDrawTarget) SetMoveCursor(move bool) {
	if setter, ok := m.inner.(interface{ SetMoveCursor(bool) }); ok {
		setter.SetMoveCursor(move)
	}
}
func (m *multiDrawTarget) IsMoveCursor() bool {
	if getter, ok := m.inner.(interface{ IsMoveCursor() bool }); ok {
		return getter.IsMoveCursor()
	}
	return false
}

func (m *multiDrawTarget) AdjustLastLineCount(adjust LineAdjust) {
	if setter, ok := m.inner.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
		setter.AdjustLastLineCount(adjust)
	}
}

// AggregateDrawBar triggers an aggregated draw for the whole MultiState.
// （这条 bar 的行由 remoteDrawTarget 写进成员 draw_state，这里只负责触发整体绘制。）
func (m *multiDrawTarget) AggregateDrawBar(b *BarState, force bool, now time.Time) error {
	_, err := m.ms.draw(force, nil, nil, now)
	return err
}

func (m *multiDrawTarget) AggregateDrawOrigin(originLines []LineType, originContents []string, b *BarState, force bool, now time.Time) error {
	_, err := m.ms.draw(force, originLines, originContents, now)
	return err
}

// aggregatedDrawable is a single, reusable drawable used by MultiState to
// produce one atomic frame write. MultiState populates its `state` buffer
// and then calls Draw() once.
type aggregatedDrawable struct {
	state *DrawStateStub
	mdt   *multiDrawTarget
}

func (a *aggregatedDrawable) State() *DrawStateWrapper { return DrawStateWrapperForTerm(a.state) }

func (a *aggregatedDrawable) Draw() error {
	ds := a.state
	contents := ds.contents
	// Debug: log when Draw is called
	// fmt.Fprintf(os.Stderr, "aggregatedDrawable.Draw() called with %d lines\n", len(ds.lines))
	// (alignment handling will be applied in the fast-path using concrete progressDrawTarget)

	// Prefer to use concrete progressDrawTarget fast path when available
	if pt, ok := a.mdt.inner.(*progressDrawTarget); ok {
		pt.kind.mu.Lock()
		// Delegate entire frame drawing to DrawStateStub.drawToTerm to match Rust behavior.
		// Delegate entire frame drawing to DrawStateStub.drawToTerm to match Rust behavior.
		// Handle both Term and TermLike variants so InMemoryTerm (term-like)
		// is used when provided instead of defaulting to stderr.
		if pt.kind.variant == variantTerm {
			var term TermLike
			if pt.kind.termTerm != nil {
				term = pt.kind.termTerm
			} else {
				term = NewStderrTerm()
			}
			newCount, _ := ds.drawToTerm(term, pt.kind.termLastLineCount)
			pt.kind.termLastLineCount = newCount
			pt.kind.frameLines = newCount.AsInt()
			pt.kind.mu.Unlock()
			return nil
		}
		if pt.kind.variant == variantTermLike {
			// use tlInner and tlLastLineCount
			var term TermLike
			if pt.kind.tlInner != nil {
				term = pt.kind.tlInner
			} else {
				term = NewStderrTerm()
			}
			newCount, _ := ds.drawToTerm(term, pt.kind.tlLastLineCount)
			pt.kind.tlLastLineCount = newCount
			pt.kind.frameLines = newCount.AsInt()
			pt.kind.mu.Unlock()
			return nil
		}
		pt.kind.mu.Unlock()
	}

	// Fallback: populate inner drawable state and call its Draw()
	if a.mdt.inner != nil {
		inner := a.mdt.inner.Drawable(false)
		if inner != nil {
			idsWrap := inner.State()
			ids := idsWrap.StatePtr
			ids.lines = ids.lines[:0]
			ids.contents = ids.contents[:0]
			for _, c := range contents {
				ids.contents = append(ids.contents, c)
			}
			idsWrap.Close()
			_ = inner.Draw()
		}
	}
	return nil
}

func (a *aggregatedDrawable) Clear() error {
	// Clear the state by setting it to empty, then draw it to clear the terminal
	a.state.lines = a.state.lines[:0]
	a.state.contents = a.state.contents[:0]
	// Call Draw() to actually clear the terminal with empty state
	return a.Draw()
}

func (a *aggregatedDrawable) Width() *uint16 { return a.mdt.inner.Width() }

// markZombie / memberVisualLines / visualIndex / insertWithPos / removeIdx 的索引
// 一律是 uint，对应 Rust 的 `MultiStateIndex(usize)` 与 `usize` 下标
// （`visual_index()` 返回的也是 usize）。
func (ms *MultiState) markZombie(index uint) {
	if index >= uint(len(ms.members)) {
		return
	}
	// If the zombie is not the first visual bar, mark and defer
	if len(ms.ordering) == 0 || index != ms.ordering[0] {
		ms.members[index].isZombie = true
		return
	}

	// Rust：`member.draw_state.as_ref().zip(width).map(|(d, width)| d.visual_line_count(.., width))`
	// 数的是成员上次绘制留下的行，不是重新格式化一遍。
	var lineCount VisualLines = VisualLines(0)
	if wptr := ms.width(); wptr != nil {
		lineCount = ms.memberVisualLines(index, int(*wptr))
	}

	// Track total zombie lines and inform draw target not to clear them
	ms.zombieLinesCount = ms.zombieLinesCount + lineCount
	if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
		adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustKeep, Count: lineCount})
	}

	ms.removeIdx(index)
}

// draw 对应 Rust 的 `MultiState::draw(force_draw, extra_lines, now)`。
// extraLines 只有 MultiProgress::println 会传（per-bar 的绘制走 nil）。
func (ms *MultiState) draw(force bool, extraLines []LineType, extraContents []string, now time.Time) (interface{}, error) {
	// Use a consistent frame marker for this draw invocation so draw targets
	// and simple drawables can coordinate first-draw-per-frame behavior.
	nowMs := now.UnixNano() / int64(time.Millisecond)

	if pt, ok := ms.draw_target.(*progressDrawTarget); ok {
		pt.kind.mu.Lock()
		pt.kind.lastFrame = nowMs
		pt.kind.mu.Unlock()
	}
	// Reap consecutive zombies from head of ordering
	var reapIndices []uint
	var adjust VisualLines = 0
	for _, index := range ms.ordering {
		member := ms.members[index]
		if !member.isZombie {
			break
		}
		// Rust：`member.draw_state.as_ref().map(|d| d.visual_line_count(.., width))`（同上，数上次绘制的行）
		var lineCount VisualLines = VisualLines(0)
		if wptr := ms.width(); wptr != nil {
			lineCount = ms.memberVisualLines(index, int(*wptr))
		}
		// Track totals exactly as Rust: add to zombieLinesCount and to adjust
		ms.zombieLinesCount = ms.zombieLinesCount + lineCount
		adjust = adjust + lineCount
		reapIndices = append(reapIndices, index)
	}

	// Rust：`if extra_lines.is_some() { adjust_last_line_count(Clear(zombie_lines_count)) }`
	// extra_lines 只有 `MultiProgress::println` 会传 Some；per-bar 的绘制走的是 None
	// （消息行走的是成员 draw_state → orphan_lines 那条路）。
	extra := len(extraLines) > 0
	if extra {
		if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustClear, Count: ms.zombieLinesCount})
		}
		ms.zombieLinesCount = VisualLines(0)
	}

	// Rust：`let width = match self.width() { Some(w) => w as usize, None => return Ok(()) }`
	// —— 拿不到宽度（hidden 目标）直接不绘制。
	wptr := ms.width()
	if wptr == nil {
		return nil, nil
	}
	width := int(*wptr)

	// Rust：`let orphan_visual_line_count = visual_line_count(&self.orphan_lines, width); force_draw |= ... > 0`
	// 换行感知的真实行数（之前是「几条就算几行」，长消息换行时会少算）。
	orphanTypes := make([]LineType, 0, len(ms.orphanLines))
	for _, ol := range ms.orphanLines {
		if ol == "" {
			orphanTypes = append(orphanTypes, LineEmpty)
		} else {
			orphanTypes = append(orphanTypes, LineText)
		}
	}
	orphanVisual := visualLineCount(orphanTypes, ms.orphanLines, width)
	force = force || (orphanVisual > VisualLines(0))

	drawable := ms.draw_target.DrawableWithNow(force, now)
	if drawable == nil {
		return nil, nil
	}
	dsWrap := drawable.State()
	ds := dsWrap.StatePtr
	ds.lines = ds.lines[:0]
	ds.contents = ds.contents[:0]

	// set drawable state alignment and move_cursor so Draw() can honor them
	ds.alignment = ms.alignment
	// 取底层目标的 move_cursor 模式。之前直接断言 *progressDrawTarget，而 ms.draw_target
	// 是 multiDrawTarget 包装层，断言恒失败 → move_cursor 永远 false。
	if mc, ok := ms.draw_target.(interface{ IsMoveCursor() bool }); ok {
		ds.move_cursor = mc.IsMoveCursor()
	} else {
		ds.move_cursor = false
	}

	// Rust 的组帧顺序：extra_lines → orphan_lines → 各成员（rs:332-344）
	// 1) extra_lines：MultiProgress::println 的消息，出现在所有内容之上
	if extra {
		for i, lt := range extraLines {
			ds.lines = append(ds.lines, lt)
			if i < len(extraContents) {
				ds.contents = append(ds.contents, extraContents[i])
			} else {
				ds.contents = append(ds.contents, "")
			}
		}
	}

	// 2) orphanLines：Rust 是 `draw_state.lines.append(&mut self.orphan_lines)` —— **抽干**，
	// 每条只显示一帧（它们不在 last_line_count 里，所以会留在屏幕上，bar 块在下方被反复重绘）。
	// Go 之前是持久列表，每帧原样重画，屏幕上会一帧一行地往下堆。
	for _, ol := range ms.orphanLines {
		if ol == "" {
			ds.lines = append(ds.lines, LineEmpty)
			ds.contents = append(ds.contents, "")
		} else {
			ds.lines = append(ds.lines, LineText)
			ds.contents = append(ds.contents, ol)
		}
	}
	ms.orphanLines = nil

	// 3) 各成员（按 ordering）
	//
	// Rust：`if let Some(state) = &member.draw_state { draw_state.lines.extend_from_slice(&state.lines[..]) }`
	// —— 每个成员贡献自己**上次绘制**留下的行：
	//   * 与"这次是谁触发的绘制"无关（触发者自己的行已由 aggregateDraw 刷新过）；
	//   * 没绘制过的成员（draw_state 为 None）不出现在画面上；
	//   * zombie 成员**不跳过**，它们要最后入帧一次，draw 结束才 remove_idx。
	for _, idx := range ms.ordering {
		if idx >= uint(len(ms.members)) {
			continue
		}
		m := ms.members[idx]
		if m.drawState == nil {
			continue
		}
		ds.lines = append(ds.lines, m.drawState.lines...)
		ds.contents = append(ds.contents, m.drawState.contents...)
	}

	// NOTE: Do not trim trailing empty lines here — keep parity with Rust which
	// relies on the draw state as assembled (no trimming).

	// Rust 这里是 `drop(draw_state); drawable.draw()` —— 没有"画完再把文本行回灌"这一步。
	// Go 之前会把本帧的文本行收回来存进 ms.orphanLines，配合不抽干的实现，导致同一条消息
	// 每帧重画并逐帧下移（捕获文件里会堆出上千行）。消息只应由 orphan 队列进、由抽干出。
	//
	// Rust 的 `drawable.draw()` 返回 io::Result，错误照原样传出去（之前一律忽略）。
	// 注意顺序：draw 之后才回收僵尸、才做 Keep(adjust)。
	err := drawable.Draw()

	// (zombie lines already cleared pre-draw for println paths)

	// remove reaped indices from state
	for _, idx := range reapIndices {
		ms.removeIdx(idx)
	}

	// Rust：`if extra_lines.is_none() { adjust_last_line_count(Keep(adjust)) }`
	// 僵尸行已经画完最后一帧，让 draw target 忘掉它们，下一帧不要再去清。
	if !extra {
		if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustKeep, Count: adjust})
		}
	}

	return nil, err
}

func (ms *MultiState) println(msg string, now time.Time) (interface{}, error) {
	// Convert message to LineType slice (like Rust's println implementation)
	var lines []LineType
	var contents []string
	if msg == "" {
		lines = append(lines, LineEmpty)
		contents = append(contents, "")
	} else {
		for _, l := range splitLines(msg) {
			if l == "" {
				lines = append(lines, LineEmpty)
				contents = append(contents, "")
			} else {
				lines = append(lines, LineText)
				contents = append(contents, l)
			}
		}
	}
	// Rust：`self.draw(true, Some(lines), now)`
	return ms.draw(true, lines, contents, now)
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (ms *MultiState) suspend(f func() interface{}, now time.Time) interface{} {
	// Hide all progress bars (clear), run f while holding lock, then redraw
	// Note: callers generally hold ms.mu when invoking suspend; replicate behavior conservatively.
	_, _ = ms.clear(now)
	ret := f()
	// Rust：`self.draw(true, None, Instant::now())`
	_, _ = ms.draw(true, nil, nil, time.Now())
	return ret
}

// clear 对应 Rust 的 MultiState::clear：
//
//	Some(mut drawable) => {
//	    drawable.adjust_last_line_count(LineAdjust::Clear(self.zombie_lines_count));  // 先
//	    self.zombie_lines_count = 0;
//	    drawable.clear()                                                              // 后
//	}
//
// 顺序必须是「先 adjust 再 clear」：adjust 是让 last_line_count 记住这些僵尸行，
// 随后的 clear 才会把它们一并擦掉（之前 Go 是反的，僵尸行擦不干净）。
// Rust 的 clear **不动** orphan_lines。
func (ms *MultiState) clear(now time.Time) (interface{}, error) {
	if drawable := ms.draw_target.DrawableWithNow(true, now); drawable != nil {
		if adj, ok := drawable.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adj.AdjustLastLineCount(LineAdjust{Kind: LineAdjustClear, Count: ms.zombieLinesCount})
		} else if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustClear, Count: ms.zombieLinesCount})
		}
		ms.zombieLinesCount = VisualLines(0)
		// Rust 的 clear 返回 io::Result，照原样传出
		return nil, drawable.Clear()
	}
	return nil, nil
}

func (ms *MultiState) isHidden() bool { return ms.draw_target.IsHidden() }

func (ms *MultiState) width() *uint16 { return ms.draw_target.Width() }

type InsertLocation int

const (
	InsertLocationEnd InsertLocation = iota
	InsertLocationIndex
	InsertLocationIndexFromBack
	InsertLocationAfter
	InsertLocationBefore
)

// visualIndex 对应 Rust 的 `MultiState::visual_index(opaque_index)`：
// 把 `members` 的不透明下标映射到**屏幕位置**（ordering 里的第几个）。
//
// Rust 找不到时是 `.expect("no such member")`（panic）；Go 用第二个返回值表示。
func (ms *MultiState) visualIndex(idx uint) (uint, bool) {
	for i, v := range ms.ordering {
		if v == idx {
			return uint(i), true
		}
	}
	return 0, false
}

func (ms *MultiState) insertWithPos(loc InsertLocation, pos uint) uint {
	var idx uint
	if len(ms.freeSet) > 0 {
		idx = ms.freeSet[len(ms.freeSet)-1]
		ms.freeSet = ms.freeSet[:len(ms.freeSet)-1]
		ms.members[idx] = MultiStateMember{}
	} else {
		ms.members = append(ms.members, MultiStateMember{})
		idx = uint(len(ms.members) - 1)
	}

	switch loc {
	case InsertLocationEnd:
		ms.ordering = append(ms.ordering, idx)
	case InsertLocationIndex:
		// pos 是 uint：`pos > len(...)` 之外没有负数这一支（Rust 的 usize 同理）
		if pos > uint(len(ms.ordering)) {
			pos = uint(len(ms.ordering))
		}
		ms.ordering = append(ms.ordering[:pos], append([]uint{idx}, ms.ordering[pos:]...)...)
	case InsertLocationIndexFromBack:
		// Rust：`let pos = self.ordering.len().saturating_sub(pos);`
		// Go 的 uint 减法会绕回，所以这里显式做 saturating_sub
		var p uint
		n := uint(len(ms.ordering))
		if pos < n {
			p = n - pos
		}
		ms.ordering = append(ms.ordering[:p], append([]uint{idx}, ms.ordering[p:]...)...)
	case InsertLocationAfter:
		// pos is index of after
		for i, v := range ms.ordering {
			if v == pos {
				ms.ordering = append(ms.ordering[:i+1], append([]uint{idx}, ms.ordering[i+1:]...)...)
				goto done
			}
		}
		// fallback: append
		ms.ordering = append(ms.ordering, idx)
	case InsertLocationBefore:
		for i, v := range ms.ordering {
			if v == pos {
				ms.ordering = append(ms.ordering[:i], append([]uint{idx}, ms.ordering[i:]...)...)
				goto done
			}
		}
		ms.ordering = append(ms.ordering, idx)
	}
done:
	return idx
}

func (ms *MultiState) removeIdx(idx uint) {
	// if already removed, no-op
	for _, v := range ms.freeSet {
		if v == idx {
			return
		}
	}

	ms.members[idx] = MultiStateMember{}
	ms.freeSet = append(ms.freeSet, idx)
	// remove from ordering
	for i, v := range ms.ordering {
		if v == idx {
			ms.ordering = append(ms.ordering[:i], ms.ordering[i+1:]...)
			break
		}
	}
}

func (ms *MultiState) len() int { return len(ms.members) - len(ms.freeSet) }
