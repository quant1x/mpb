package mpb

import (
	"runtime"
	"sync"
	"unsafe"
)

// 简单的弱引用注册表：为每个注册的对象分配一个 id，finalizer 会在对象被 GC 时移除注册表项。
type weakRegistry struct {
	mu sync.Mutex
	// map id -> SafeBarState pointer value (uintptr)
	m map[uint64]uintptr
	// map from SafeBarState pointer value to registered ids so we set finalizer once
	stateToIDs map[uintptr][]uint64
	next       uint64
}

func newWeakRegistry() *weakRegistry {
	return &weakRegistry{m: make(map[uint64]uintptr), stateToIDs: make(map[uintptr][]uint64)}
}

// registerState registers a weak reference by SafeBarState pointer so that all ProgressBar
// instances sharing the same state are considered the same strong owner group.
func (r *weakRegistry) registerState(s *SafeBarState) uint64 {
	r.mu.Lock()
	r.next++
	id := r.next
	ptr := uintptr(unsafe.Pointer(s))
	r.m[id] = ptr
	ids := r.stateToIDs[ptr]
	r.stateToIDs[ptr] = append(ids, id)
	first := len(ids) == 0
	r.mu.Unlock()
	if first {
		runtime.SetFinalizer(s, func(ss *SafeBarState) {
			ss.stopTicker()
			r.mu.Lock()
			p := uintptr(unsafe.Pointer(ss))
			for _, iid := range r.stateToIDs[p] {
				delete(r.m, iid)
			}
			delete(r.stateToIDs, p)
			r.mu.Unlock()
		})
	}
	return id
}

func (r *weakRegistry) delete(id uint64) {
	r.mu.Lock()
	delete(r.m, id)
	r.mu.Unlock()
}

func (r *weakRegistry) get(id uint64) *ProgressBar {
	r.mu.Lock()
	up := r.m[id]
	r.mu.Unlock()
	if up == 0 {
		return nil
	}
	s := (*SafeBarState)(unsafe.Pointer(up))
	// construct a new ProgressBar that shares the same SafeBarState and the state's position
	var pos *AtomicPosition
	if s != nil && s.inner != nil {
		pos = s.inner.state.pos
	}
	return &ProgressBar{state: s, pos: pos}
}

var defaultWeakReg = newWeakRegistry()

func registerWeakState(s *SafeBarState) uint64 { return defaultWeakReg.registerState(s) }
func getWeak(id uint64) *ProgressBar           { return defaultWeakReg.get(id) }
