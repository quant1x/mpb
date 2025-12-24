package mpb

import (
	"runtime"
	"sync"
)

// 简单的弱引用注册表：为每个注册的对象分配一个 id，finalizer 会在对象被 GC 时移除注册表项。
type weakRegistry struct {
	mu   sync.Mutex
	m    map[uint64]*ProgressBar
	next uint64
}

func newWeakRegistry() *weakRegistry {
	return &weakRegistry{m: make(map[uint64]*ProgressBar)}
}

func (r *weakRegistry) register(pb *ProgressBar) uint64 {
	r.mu.Lock()
	r.next++
	id := r.next
	r.m[id] = pb
	r.mu.Unlock()
	// set finalizer to stop ticker and remove registry entry
	runtime.SetFinalizer(pb, func(p *ProgressBar) {
		p.stopTicker()
		r.delete(id)
	})
	return id
}

func (r *weakRegistry) delete(id uint64) {
	r.mu.Lock()
	delete(r.m, id)
	r.mu.Unlock()
}

func (r *weakRegistry) get(id uint64) *ProgressBar {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[id]
}

var defaultWeakReg = newWeakRegistry()

func registerWeak(pb *ProgressBar) uint64 { return defaultWeakReg.register(pb) }
func getWeak(id uint64) *ProgressBar      { return defaultWeakReg.get(id) }
