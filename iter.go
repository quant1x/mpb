package mpb

import (
	"io"
	"time"
)

// 简化的 iter 翻译：提供泛型 ProgressBarIter 和 SeekMax 的占位实现。

// ProgressBarIter 泛型：用于包装切片迭代并更新进度条（提供 Next 方法）。
type ProgressBarIter[T any] struct {
	it       []T
	idx      int
	progress *ProgressBar
	seekMax  *SeekMax
}

func NewProgressBarIterFromSlice[T any](slice []T, pb *ProgressBar) *ProgressBarIter[T] {
	return &ProgressBarIter[T]{it: slice, idx: 0, progress: pb, seekMax: NewSeekMax()}
}

func (p *ProgressBarIter[T]) WithStyle(style ProgressStyle) *ProgressBarIter[T] {
	p.progress.SetStyle(style)
	return p
}

func (p *ProgressBarIter[T]) WithPrefix(prefix string) *ProgressBarIter[T] {
	p.progress.WithPrefix(prefix)
	return p
}

func (p *ProgressBarIter[T]) WithMessage(msg string) *ProgressBarIter[T] {
	p.progress.WithMessage(msg)
	return p
}

func (p *ProgressBarIter[T]) WithPosition(pos uint64) *ProgressBarIter[T] {
	p.progress.SetPosition(pos)
	return p
}

func (p *ProgressBarIter[T]) WithElapsed(elapsed time.Duration) *ProgressBarIter[T] {
	// simplified: no-op
	_ = elapsed
	return p
}

func (p *ProgressBarIter[T]) WithFinish(f FinishKind) *ProgressBarIter[T] {
	p.progress.WithFinish(ProgressFinish{Kind: f})
	return p
}

// Next 返回下一个元素及是否存在，类似 Go 的迭代风格。
func (p *ProgressBarIter[T]) Next() (T, bool) {
	var zero T
	if p.idx >= len(p.it) {
		if !p.progress.IsFinished() {
			p.progress.Finish()
		}
		return zero, false
	}
	itm := p.it[p.idx]
	p.idx++
	p.progress.Inc(1)
	return itm, true
}

// --- Reader wrapper ---
type ProgressReaderIter struct {
	r        io.Reader
	progress *ProgressBar
	seekMax  *SeekMax
}

func NewProgressReaderIter(r io.Reader, pb *ProgressBar) *ProgressReaderIter {
	return &ProgressReaderIter{r: r, progress: pb, seekMax: NewSeekMax()}
}

func (pr *ProgressReaderIter) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	if n > 0 {
		newPos := pr.seekMax.UpdateSeq(pr.progress.Position(), uint64(n))
		pr.progress.SetPosition(newPos)
	}
	return n, err
}

// --- SeekMax / MaxRingBuf 简化实现 ---
type SeekMax struct {
	buf    []uint64
	head   int
	maxPos int
	seq    int
}

func NewSeekMax() *SeekMax {
	size := 10
	return &SeekMax{buf: make([]uint64, size), head: 0, maxPos: 0, seq: 0}
}

func (s *SeekMax) UpdateSeq(prev uint64, delta uint64) uint64 {
	newPos := prev + delta
	s.seq++
	if s.seq >= 5 {
		// reset
		s.seq = 0
		s.head = 0
		s.maxPos = 0
		for i := range s.buf {
			s.buf[i] = 0
		}
		return newPos
	}
	s.buf[s.head] = newPos
	if newPos > s.buf[s.maxPos] {
		s.maxPos = s.head
	}
	s.head = (s.head + 1) % len(s.buf)
	return s.buf[s.maxPos]
}

func (s *SeekMax) UpdateSeek(newpos uint64) uint64 {
	s.seq = 0
	s.buf[s.head] = newpos
	if newpos > s.buf[s.maxPos] {
		s.maxPos = s.head
	}
	s.head = (s.head + 1) % len(s.buf)
	return s.buf[s.maxPos]
}
