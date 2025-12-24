package mpb

import (
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
	_ = mp.state.insert(loc)
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
	orphanLines      []LineType
	zombieLinesCount VisualLines
}

func NewMultiState(dt ProgressDrawTarget) *MultiState {
	return &MultiState{
		members:          []MultiStateMember{},
		freeSet:          []int{},
		ordering:         []int{},
		draw_target:      dt,
		alignment:        MultiProgressAlignmentTop,
		orphanLines:      []LineType{},
		zombieLinesCount: VisualLines(0),
	}
}

type MultiStateMember struct {
	drawState *DrawStateStub
	isZombie  bool
}

func (ms *MultiState) markZombie(index int) {
	if index < 0 || index >= len(ms.members) {
		return
	}
	ms.members[index].isZombie = true
}

func (ms *MultiState) draw(force bool, extraLines []LineType, now time.Time) (interface{}, error) {
	return nil, nil
}

func (ms *MultiState) println(msg string, now time.Time) (interface{}, error) {
	if msg == "" {
		ms.orphanLines = append(ms.orphanLines, LineEmpty)
	} else {
		for _, l := range splitLines(msg) {
			ms.orphanLines = append(ms.orphanLines, LineTextVal(l))
		}
	}
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
