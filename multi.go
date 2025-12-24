package mpb

import (
	"fmt"
	"os"
	"strings"
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
	return mp.internalize(InsertLocationEnd, pb)
}

func (mp *MultiProgress) Insert(index int, pb *ProgressBar) *ProgressBar {
	// simplified: ignore index and append
	return mp.internalize(InsertLocationEnd, pb)
}

func (mp *MultiProgress) InsertFromBack(index int, pb *ProgressBar) *ProgressBar {
	// simplified: ignore index and append
	return mp.internalize(InsertLocationEnd, pb)
}

func (mp *MultiProgress) InsertBefore(before *ProgressBar, pb *ProgressBar) *ProgressBar {
	// simplified: append
	return mp.Add(pb)
}

func (mp *MultiProgress) InsertAfter(after *ProgressBar, pb *ProgressBar) *ProgressBar {
	// simplified: append
	return mp.Add(pb)
}

func (mp *MultiProgress) Remove(pb *ProgressBar) {
	// best-effort no-op removal in simplified implementation
	if idx, ok := pb.Index(); ok {
		mp.state.removeIdx(idx)
	}
}

func (mp *MultiProgress) internalize(loc InsertLocation, pb *ProgressBar) *ProgressBar {
	mp.state.mu.Lock()
	idx := mp.state.insert(loc)
	// register this ProgressBar's SafeBarState in members
	if idx >= 0 && idx < len(mp.state.members) {
		mp.state.members[idx].sb = pb.state
	}
	// ensure the bar draws into multi draw target so aggregation works
	pb.SetDrawTarget(mp.state.draw_target)
	mp.state.mu.Unlock()
	return pb
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

func (a *aggregatedDrawable) State() *DrawStateStub { return a.state }

func (a *aggregatedDrawable) Draw() error {
	ds := a.state
	contents := ds.contents
	newLen := len(contents)

	// Prefer to use concrete progressDrawTarget fast path when available
	if pt, ok := a.mdt.inner.(*progressDrawTarget); ok {
		out := os.Stderr
		pt.mu.Lock()
		prevHeight := pt.lastHeight
		if prevHeight > 0 {
			fmt.Fprintf(out, "\x1b[%dA", prevHeight)
		}
		var sb strings.Builder
		for i := 0; i < newLen; i++ {
			sb.WriteString("\x1b[2K")
			sb.WriteString(contents[i])
			sb.WriteString("\n")
		}
		pt.frameLines += newLen
		if prevHeight > pt.frameLines {
			rem := prevHeight - pt.frameLines
			for i := 0; i < rem; i++ {
				sb.WriteString("\x1b[2K\n")
			}
			pt.lastHeight = pt.frameLines
		} else {
			pt.lastHeight = pt.frameLines
		}
		sb.WriteString("\x1b[0J")
		_, _ = out.WriteString(sb.String())
		pt.mu.Unlock()
		return nil
	}

	// Fallback: populate inner drawable state and call its Draw()
	if a.mdt.inner != nil {
		inner := a.mdt.inner.Drawable(false)
		if inner != nil {
			ids := inner.State()
			ids.lines = ids.lines[:0]
			ids.contents = ids.contents[:0]
			for _, c := range contents {
				ids.contents = append(ids.contents, c)
			}
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
	ms.members[index].isZombie = true
}

func (ms *MultiState) draw(force bool, originLines []LineType, originContents []string, origin *BarState, now time.Time) (interface{}, error) {
	// Aggregate all orphan lines and member bar renderings into a single drawable
	drawable := ms.draw_target.Drawable(force)
	if drawable == nil {
		return nil, nil
	}
	ds := drawable.State()
	ds.lines = ds.lines[:0]
	ds.contents = ds.contents[:0]

	// width from draw target
	wptr := ms.width()
	var width int
	if wptr != nil {
		width = int(*wptr)
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

	// perform a single draw
	// trim trailing empty lines to avoid emitting unnecessary blank lines
	for len(ds.contents) > 0 {
		last := ds.contents[len(ds.contents)-1]
		if last == "" {
			ds.contents = ds.contents[:len(ds.contents)-1]
			ds.lines = ds.lines[:len(ds.lines)-1]
			continue
		}
		break
	}
	_ = drawable.Draw()
	return nil, nil
}

func (ms *MultiState) println(msg string, now time.Time) (interface{}, error) {
	if msg == "" {
		ms.orphanLines = append(ms.orphanLines, "")
	} else {
		for _, l := range splitLines(msg) {
			ms.orphanLines = append(ms.orphanLines, l)
		}
	}
	return ms.draw(false, nil, nil, nil, now)
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
	return f()
}

func (ms *MultiState) clear(now time.Time) (interface{}, error) {
	ms.orphanLines = nil
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

func (ms *MultiState) insert(loc InsertLocation) int {
	if len(ms.freeSet) > 0 {
		idx := ms.freeSet[len(ms.freeSet)-1]
		ms.freeSet = ms.freeSet[:len(ms.freeSet)-1]
		ms.members[idx] = MultiStateMember{}
		ms.ordering = append(ms.ordering, idx)
		return idx
	}
	ms.members = append(ms.members, MultiStateMember{})
	idx := len(ms.members) - 1
	ms.ordering = append(ms.ordering, idx)
	return idx
}

func (ms *MultiState) removeIdx(idx int) {
	for i, v := range ms.ordering {
		if v == idx {
			ms.ordering = append(ms.ordering[:i], ms.ordering[i+1:]...)
			break
		}
	}
	ms.freeSet = append(ms.freeSet, idx)
}

func (ms *MultiState) len() int { return len(ms.members) - len(ms.freeSet) }
