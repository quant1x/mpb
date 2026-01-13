package mpb

import (
	"os"
	"sync"
	"time"
)

// 简化的 MultiProgress / MultiState 实现，便于增量迁移。只实现必要的方法和数据结构，行为为保守的占位实现。

type MultiProgress struct {
	state *MultiState
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
	return &MultiProgress{state: NewMultiState(dt)}
}

func (mp *MultiProgress) New() *MultiProgress { return DefaultMultiProgress() }

func (mp *MultiProgress) WithDrawTarget(dt ProgressDrawTarget) *MultiProgress {
	return WithDrawTarget(dt)
}

func (mp *MultiProgress) SetDrawTarget(target ProgressDrawTarget) {
	mp.state.mu.Lock()
	defer mp.state.mu.Unlock()
	mp.state.draw_target = target
}

func (mp *MultiProgress) SetMoveCursor(moveCursor bool) {
	mp.state.mu.Lock()
	if setter, ok := mp.state.draw_target.(interface{ SetMoveCursor(bool) }); ok {
		setter.SetMoveCursor(moveCursor)
	}
	mp.state.mu.Unlock()
}

func (mp *MultiProgress) SetAlignment(alignment MultiProgressAlignment) {
	mp.state.mu.Lock()
	mp.state.alignment = alignment
	mp.state.mu.Unlock()
}

func (mp *MultiProgress) Add(pb *ProgressBar) *ProgressBar {
	return mp.internalize(InsertLocationEnd, 0, pb)
}

func (mp *MultiProgress) Insert(index int, pb *ProgressBar) *ProgressBar {
	return mp.internalize(InsertLocationIndex, index, pb)
}

func (mp *MultiProgress) InsertFromBack(index int, pb *ProgressBar) *ProgressBar {
	return mp.internalize(InsertLocationIndexFromBack, index, pb)
}

func (mp *MultiProgress) InsertBefore(before *ProgressBar, pb *ProgressBar) *ProgressBar {
	if idx, ok := before.Index(); ok {
		return mp.internalize(InsertLocationBefore, idx, pb)
	}
	return mp.Add(pb)
}

func (mp *MultiProgress) InsertAfter(after *ProgressBar, pb *ProgressBar) *ProgressBar {
	if idx, ok := after.Index(); ok {
		return mp.internalize(InsertLocationAfter, idx, pb)
	}
	return mp.Add(pb)
}

func (mp *MultiProgress) Remove(pb *ProgressBar) {
	// best-effort no-op removal in simplified implementation
	if idx, ok := pb.Index(); ok {
		mp.state.removeIdx(idx)
	}
}

func (mp *MultiProgress) internalize(loc InsertLocation, pos int, pb *ProgressBar) *ProgressBar {
	mp.state.mu.Lock()
	idx := mp.state.insertWithPos(loc, pos)
	// register this ProgressBar's SafeBarState in members
	if idx >= 0 && idx < len(mp.state.members) {
		mp.state.members[idx].sb = pb.state
	}
	// ensure the bar draws into per-bar remote draw target so multi can identify it
	pb.SetDrawTarget(NewRemoteDrawTarget(mp.state, idx))
	mp.state.mu.Unlock()
	return pb
}

// remoteDrawTarget is a per-bar draw target that forwards aggregate draw requests
// to the owning MultiState while exposing remote information.
type remoteDrawTarget struct {
	ms  *MultiState
	idx int
}

func NewRemoteDrawTarget(ms *MultiState, idx int) ProgressDrawTarget {
	return &remoteDrawTarget{ms: ms, idx: idx}
}

func (r *remoteDrawTarget) IsHidden() bool { return r.ms.draw_target.IsHidden() }
func (r *remoteDrawTarget) IsStderr() bool { return r.ms.draw_target.IsStderr() }
func (r *remoteDrawTarget) Width() *uint16 { return r.ms.draw_target.Width() }
func (r *remoteDrawTarget) Remote() (*RemoteStub, int, bool) {
	return &RemoteStub{ms: r.ms}, r.idx, true
}
func (r *remoteDrawTarget) Disconnect(now time.Time)     { /* no-op */ }
func (r *remoteDrawTarget) Drawable(force bool) Drawable { return nil }
func (r *remoteDrawTarget) AdjustLastLineCount(adjust LineAdjust) {
	r.ms.draw_target.AdjustLastLineCount(adjust)
}

func (r *remoteDrawTarget) AggregateDrawBar(b *BarState, force bool, now time.Time) error {
	_, err := r.ms.draw(force, nil, nil, b, now)
	return err
}

func (r *remoteDrawTarget) AggregateDrawOrigin(originLines []LineType, originContents []string, b *BarState, force bool, now time.Time) error {
	_, err := r.ms.draw(force, originLines, originContents, b, now)
	return err
}

func (mp *MultiProgress) Println(msg string) error {
	mp.state.mu.Lock()
	defer mp.state.mu.Unlock()
	_, _ = mp.state.println(msg, time.Now())
	return nil
}

func (mp *MultiProgress) Suspend(f func() interface{}) interface{} {
	mp.state.mu.Lock()
	defer mp.state.mu.Unlock()
	return mp.state.suspend(f, time.Now())
}

func (mp *MultiProgress) Clear() error {
	mp.state.mu.Lock()
	defer mp.state.mu.Unlock()
	_, _ = mp.state.clear(time.Now())
	return nil
}

func (mp *MultiProgress) IsHidden() bool {
	mp.state.mu.RLock()
	defer mp.state.mu.RUnlock()
	return msIsHidden(mp.state)
}
func msIsHidden(ms *MultiState) bool { return ms.draw_target.IsHidden() }

// --- MultiState and helpers ---
type MultiState struct {
	mu               sync.RWMutex
	members          []MultiStateMember
	freeSet          []int
	ordering         []int
	draw_target      ProgressDrawTarget
	alignment        MultiProgressAlignment
	orphanLines      []string
	orphanSet        map[string]bool
	zombieLinesCount VisualLines
}

func NewMultiState(dt ProgressDrawTarget) *MultiState {
	ms := &MultiState{
		members:          []MultiStateMember{},
		freeSet:          []int{},
		ordering:         []int{},
		draw_target:      dt,
		alignment:        MultiProgressAlignmentTop,
		orphanLines:      []string{},
		orphanSet:        map[string]bool{},
		zombieLinesCount: VisualLines(0),
	}
	// wrap the provided draw target so MultiState can aggregate per-bar draws
	ms.draw_target = &multiDrawTarget{ms: ms, inner: dt}
	return ms
}

type MultiStateMember struct {
	sb       *SafeBarState
	isZombie bool
}

// multiDrawTarget wraps an underlying ProgressDrawTarget and provides an
// Aggregate method used by BarState to request an aggregated draw.
type multiDrawTarget struct {
	ms    *MultiState
	inner ProgressDrawTarget
	agg   *aggregatedDrawable
}

func (m *multiDrawTarget) IsHidden() bool                   { return m.inner.IsHidden() }
func (m *multiDrawTarget) IsStderr() bool                   { return m.inner.IsStderr() }
func (m *multiDrawTarget) Width() *uint16                   { return m.inner.Width() }
func (m *multiDrawTarget) Remote() (*RemoteStub, int, bool) { return m.inner.Remote() }
func (m *multiDrawTarget) Disconnect(now time.Time)         { m.inner.Disconnect(now) }

// Return a single aggregated drawable so MultiState can prepare its
// combined frame into the returned state's buffer and call Draw() once.
func (m *multiDrawTarget) Drawable(force bool) Drawable {
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

func (m *multiDrawTarget) AdjustLastLineCount(adjust LineAdjust) {
	if setter, ok := m.inner.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
		setter.AdjustLastLineCount(adjust)
	}
}

// AggregateDrawBar triggers an aggregated draw for the whole MultiState.
func (m *multiDrawTarget) AggregateDrawBar(b *BarState, force bool, now time.Time) error {
	_, err := m.ms.draw(force, nil, nil, b, now)
	return err
}

func (m *multiDrawTarget) AggregateDrawOrigin(originLines []LineType, originContents []string, b *BarState, force bool, now time.Time) error {
	_, err := m.ms.draw(force, originLines, originContents, b, now)
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
	// (alignment handling will be applied in the fast-path using concrete progressDrawTarget)

	// Prefer to use concrete progressDrawTarget fast path when available
	if pt, ok := a.mdt.inner.(*progressDrawTarget); ok {
		pt.kind.mu.Lock()
		// Delegate entire frame drawing to DrawStateStub.drawToTerm to match Rust behavior.
		// Handle both Term and TermLike variants so InMemoryTerm (term-like)
		// is used when provided instead of defaulting to stderr.
		if pt.kind.variant == variantTerm {
			var term TermLike
			if pt.kind.termTerm != nil {
				term = pt.kind.termTerm
			} else {
				term = NewStdTermWithWriter(os.Stderr)
			}
			_ = ds.drawToTerm(term, &pt.kind.termLastLineCount)
			pt.kind.frameLines = pt.kind.termLastLineCount.AsInt()
			pt.kind.mu.Unlock()
			return nil
		}
		if pt.kind.variant == variantTermLike {
			// use tlInner and tlLastLineCount
			var term TermLike
			if pt.kind.tlInner != nil {
				term = pt.kind.tlInner
			} else {
				term = NewStdTermWithWriter(os.Stderr)
			}
			_ = ds.drawToTerm(term, &pt.kind.tlLastLineCount)
			pt.kind.frameLines = pt.kind.tlLastLineCount.AsInt()
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
	a.state.lines = a.state.lines[:0]
	a.state.contents = a.state.contents[:0]
	return nil
}

func (a *aggregatedDrawable) Width() *uint16 { return a.mdt.inner.Width() }

func (ms *MultiState) markZombie(index int) {
	if index < 0 || index >= len(ms.members) {
		return
	}
	// If the zombie is not the first visual bar, mark and defer
	if len(ms.ordering) == 0 || index != ms.ordering[0] {
		ms.members[index].isZombie = true
		return
	}

	// compute visible line count for this member using wrap-aware visualLineCount
	var lineCount VisualLines = VisualLines(0)
	if wptr := ms.width(); wptr != nil {
		if ms.members[index].sb != nil {
			lines, contents := ms.members[index].sb.FormatForWidth(int(*wptr))
			lineCount = visualLineCount(lines, contents, int(*wptr))
		}
	}

	// Track total zombie lines and inform draw target not to clear them
	ms.zombieLinesCount = ms.zombieLinesCount + lineCount
	if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
		adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustKeep, Count: lineCount})
	}

	ms.removeIdx(index)
}

func (ms *MultiState) draw(force bool, originLines []LineType, originContents []string, origin *BarState, now time.Time) (interface{}, error) {
	// Use a consistent frame marker for this draw invocation so draw targets
	// and simple drawables can coordinate first-draw-per-frame behavior.
	nowMs := now.UnixNano() / int64(time.Millisecond)

	if pt, ok := ms.draw_target.(*progressDrawTarget); ok {
		pt.kind.mu.Lock()
		pt.kind.lastFrame = nowMs
		pt.kind.mu.Unlock()
	}
	// Reap consecutive zombies from head of ordering
	var reapIndices []int
	var adjust VisualLines = 0
	for _, index := range ms.ordering {
		member := ms.members[index]
		if !member.isZombie {
			break
		}
		var lineCount VisualLines = VisualLines(0)
		if member.sb != nil {
			if wptr := ms.width(); wptr != nil {
				lines, contents := member.sb.FormatForWidth(int(*wptr))
				lineCount = visualLineCount(lines, contents, int(*wptr))
			}
		}
		// Track totals exactly as Rust: add to zombieLinesCount and to adjust
		ms.zombieLinesCount = ms.zombieLinesCount + lineCount
		adjust = adjust + lineCount
		reapIndices = append(reapIndices, index)
	}

	// If this draw is due to a println/originLines, then clear all zombie lines
	if originLines != nil {
		if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustClear, Count: ms.zombieLinesCount})
		}
		ms.zombieLinesCount = VisualLines(0)
	}

	// orphan visual lines count (simple length-based approximation)
	orphanVisual := VisualLines(len(ms.orphanLines))
	force = force || (orphanVisual > VisualLines(0))

	drawable := ms.draw_target.Drawable(force)
	if drawable == nil {
		return nil, nil
	}
	dsWrap := drawable.State()
	ds := dsWrap.StatePtr
	ds.lines = ds.lines[:0]
	ds.contents = ds.contents[:0]

	// width from draw target
	wptr := ms.width()
	var width int
	if wptr != nil {
		width = int(*wptr)
	}

	// set drawable state alignment and move_cursor so Draw() can honor them
	ds.alignment = ms.alignment
	// query underlying concrete draw target for move-cursor mode when available
	if pt, ok := ms.draw_target.(*progressDrawTarget); ok {
		ds.move_cursor = pt.IsMoveCursor()
	} else {
		ds.move_cursor = false
	}

	// first, orphanLines (printed above bars)
	for _, ol := range ms.orphanLines {
		if ol == "" {
			ds.lines = append(ds.lines, LineEmpty)
			ds.contents = append(ds.contents, "")
		} else {
			ds.lines = append(ds.lines, LineText)
			ds.contents = append(ds.contents, ol)
		}
	}

	// then, each member in ordering
	for _, idx := range ms.ordering {
		if idx < 0 || idx >= len(ms.members) {
			continue
		}
		m := ms.members[idx]
		if m.isZombie || m.sb == nil {
			continue
		}
		// if this is the origin bar, use originLines/originContents provided by caller
		if origin != nil && m.sb.inner == origin {
			for _, lt := range originLines {
				ds.lines = append(ds.lines, lt)
			}
			for _, c := range originContents {
				ds.contents = append(ds.contents, c)
			}
			continue
		}
		// get formatted representation from SafeBarState (thread-safe)
		if wptr != nil {
			lines, contents := m.sb.FormatForWidth(width)
			for _, lt := range lines {
				ds.lines = append(ds.lines, lt)
			}
			for _, c := range contents {
				ds.contents = append(ds.contents, c)
			}
		}
	}

	// NOTE: Do not trim trailing empty lines here — keep parity with Rust which
	// relies on the draw state as assembled (no trimming).

	// If this drawable is the aggregatedDrawable we can capture non-bar
	// contents after drawing by providing a temporary orphan collector to
	// DrawStateWrapperForMulti. This preserves Rust-like semantics: draw
	// first, then Close() captures non-bar lines for persistence.
	if agg, ok := drawable.(*aggregatedDrawable); ok {
		var orphanContents []string
		dsWrap = DrawStateWrapperForMulti(agg.state, &orphanContents)
		ds = dsWrap.StatePtr
		// ensure we pass the assembled state into the drawable's Draw path
		_ = drawable.Draw()
		dsWrap.Close()
		// persist collected orphanContents into ms.orphanLines (dedupe)
		for _, c := range orphanContents {
			// avoid duplicate consecutive empty strings
			if c == "" {
				if len(ms.orphanLines) > 0 && ms.orphanLines[len(ms.orphanLines)-1] == "" {
					continue
				}
				ms.orphanLines = append(ms.orphanLines, "")
				continue
			}
			if ms.orphanSet == nil {
				ms.orphanSet = map[string]bool{}
			}
			if ms.orphanSet[c] {
				continue
			}
			ms.orphanSet[c] = true
			ms.orphanLines = append(ms.orphanLines, c)
		}
	} else {
		_ = drawable.Draw()
	}

	// remove reaped indices from state
	for _, idx := range reapIndices {
		ms.removeIdx(idx)
	}

	// If this was not a println (originLines == nil), we need to tell draw target
	// to keep the zombie lines count for next draw
	if originLines == nil {
		if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustKeep, Count: adjust})
		}
	}

	return nil, nil
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
	// Do not persist here; delegate to ms.draw which will capture and
	// persist non-bar lines after drawing (Rust-like semantics).
	return ms.draw(true, lines, contents, nil, now)
}

// drawWithExtraLines mirrors Rust MultiState::draw with extra_lines parameter
// This is used by println to print text above all bars
func (ms *MultiState) drawWithExtraLines(force bool, extraLines []LineType, extraContents []string, now time.Time) (interface{}, error) {
	// Use a consistent frame marker for this draw invocation so draw targets
	// and simple drawables can coordinate first-draw-per-frame behavior.
	nowMs := now.UnixNano() / int64(time.Millisecond)
	if pt, ok := ms.draw_target.(*progressDrawTarget); ok {
		pt.kind.mu.Lock()
		pt.kind.lastFrame = nowMs
		pt.kind.mu.Unlock()
	}

	// Get width
	wptr := ms.width()
	var width int
	if wptr != nil {
		width = int(*wptr)
	}

	force = force || (width > 0)

	drawable := ms.draw_target.Drawable(force)
	if drawable == nil {
		return nil, nil
	}

	dsWrap := drawable.State()
	ds := dsWrap.StatePtr
	ds.lines = ds.lines[:0]
	ds.contents = ds.contents[:0]

	// Set alignment and move_cursor from draw target
	ds.alignment = ms.alignment
	if pt, ok := ms.draw_target.(*progressDrawTarget); ok {
		ds.move_cursor = pt.IsMoveCursor()
	} else {
		ds.move_cursor = false
	}

	// Extra lines come from println calls (appear above all bars)
	// They are provided for this draw only and MUST NOT mutate ms.orphanLines
	// extraLines and extraContents are parallel slices. First render extra
	// lines for this frame, then render persistent orphanLines.
	for i, lt := range extraLines {
		if lt == LineEmpty {
			ds.lines = append(ds.lines, LineEmpty)
			ds.contents = append(ds.contents, "")
		} else {
			if i < len(extraContents) {
				ds.lines = append(ds.lines, LineText)
				ds.contents = append(ds.contents, extraContents[i])
			} else {
				ds.lines = append(ds.lines, LineEmpty)
				ds.contents = append(ds.contents, "")
			}
		}
	}

	// Then, persistent orphanLines (printed above bars)
	for _, ol := range ms.orphanLines {
		if ol == "" {
			ds.lines = append(ds.lines, LineEmpty)
			ds.contents = append(ds.contents, "")
		} else {
			ds.lines = append(ds.lines, LineText)
			ds.contents = append(ds.contents, ol)
		}
	}

	// then, each member in ordering
	for _, idx := range ms.ordering {
		if idx < 0 || idx >= len(ms.members) {
			continue
		}
		m := ms.members[idx]
		if m.isZombie || m.sb == nil {
			continue
		}
		if wptr != nil {
			lines, contents := m.sb.FormatForWidth(width)
			for _, lt := range lines {
				ds.lines = append(ds.lines, lt)
			}
			for _, c := range contents {
				ds.contents = append(ds.contents, c)
			}
		}
	}

	// Do not trim trailing empty lines (keep parity with Rust)
	dsWrap.Close()
	_ = drawable.Draw()

	return nil, nil
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
	_, _ = ms.draw(true, nil, nil, nil, time.Now())
	return ret
}

func (ms *MultiState) clear(now time.Time) (interface{}, error) {
	// Try to obtain a drawable and clear it. Also clear zombie counter.
	if drawable := ms.draw_target.Drawable(true); drawable != nil {
		// Let drawable handle clearing and adjust last line count
		drawable.Clear()
		if adjTarget, ok := ms.draw_target.(interface{ AdjustLastLineCount(LineAdjust) }); ok {
			adjTarget.AdjustLastLineCount(LineAdjust{Kind: LineAdjustClear, Count: ms.zombieLinesCount})
		}
		ms.zombieLinesCount = VisualLines(0)
	}
	ms.orphanLines = nil
	ms.orphanSet = map[string]bool{}
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

func (ms *MultiState) insertWithPos(loc InsertLocation, pos int) int {
	var idx int
	if len(ms.freeSet) > 0 {
		idx = ms.freeSet[len(ms.freeSet)-1]
		ms.freeSet = ms.freeSet[:len(ms.freeSet)-1]
		ms.members[idx] = MultiStateMember{}
	} else {
		ms.members = append(ms.members, MultiStateMember{})
		idx = len(ms.members) - 1
	}

	switch loc {
	case InsertLocationEnd:
		ms.ordering = append(ms.ordering, idx)
	case InsertLocationIndex:
		if pos < 0 {
			pos = 0
		}
		if pos > len(ms.ordering) {
			pos = len(ms.ordering)
		}
		ms.ordering = append(ms.ordering[:pos], append([]int{idx}, ms.ordering[pos:]...)...)
	case InsertLocationIndexFromBack:
		// position counted from back
		p := len(ms.ordering) - pos
		if p < 0 {
			p = 0
		}
		if p > len(ms.ordering) {
			p = len(ms.ordering)
		}
		ms.ordering = append(ms.ordering[:p], append([]int{idx}, ms.ordering[p:]...)...)
	case InsertLocationAfter:
		// pos is index of after
		for i, v := range ms.ordering {
			if v == pos {
				ms.ordering = append(ms.ordering[:i+1], append([]int{idx}, ms.ordering[i+1:]...)...)
				goto done
			}
		}
		// fallback: append
		ms.ordering = append(ms.ordering, idx)
	case InsertLocationBefore:
		for i, v := range ms.ordering {
			if v == pos {
				ms.ordering = append(ms.ordering[:i], append([]int{idx}, ms.ordering[i:]...)...)
				goto done
			}
		}
		ms.ordering = append(ms.ordering, idx)
	}
done:
	return idx
}

func (ms *MultiState) removeIdx(idx int) {
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
