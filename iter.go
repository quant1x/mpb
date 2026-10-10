package mpb

import (
	"io"
	"time"
)

// 本文件对应 Rust 的 iter.rs。

// SeekMax 的两个常量与 Rust 的 `SeekMax<RESET, HISTORY>` 默认值一致：
// RESET=5（连续 5 次顺序读写后丢弃历史）、HISTORY=10（环形缓冲容量）。
const (
	seekMaxReset   = 5
	seekMaxHistory = 10
)

// ProgressBarIter 对应 Rust 的 ProgressBarIter<T>：包装一个迭代器（Go 里是切片），
// 每产出一个元素就把进度条往前推一格。
type ProgressBarIter[T any] struct {
	it       []T
	idx      int // 头游标
	end      int // 尾游标（供 NextBack 用）
	progress *ProgressBar
	seekMax  *SeekMax
}

func NewProgressBarIterFromSlice[T any](slice []T, pb *ProgressBar) *ProgressBarIter[T] {
	return &ProgressBarIter[T]{it: slice, idx: 0, end: len(slice), progress: pb, seekMax: NewSeekMax()}
}

// --- 对应 Rust 的 ProgressIterator trait（Go 里切片即迭代器）---

// ProgressWithSlice 对应 `progress_with(progress)`：用已有的进度条包装。
func ProgressWithSlice[T any](slice []T, pb *ProgressBar) *ProgressBarIter[T] {
	return NewProgressBarIterFromSlice(slice, pb)
}

// ProgressCount 对应 `progress_count(len)`：显式给定元素个数。
func ProgressCount[T any](slice []T, count uint64) *ProgressBarIter[T] {
	return NewProgressBarIterFromSlice(slice, New(count))
}

// Progress 对应 `progress()`：用切片长度作为元素个数。
func Progress[T any](slice []T) *ProgressBarIter[T] {
	return ProgressCount(slice, uint64(len(slice)))
}

// TryProgress 对应 `try_progress()`：Rust 里只有 size_hint 的上界已知才返回 Some，
// Go 的切片长度恒已知，所以这里恒返回 true。
func TryProgress[T any](slice []T) (*ProgressBarIter[T], bool) {
	return Progress(slice), true
}

// ProgressWithStyle 对应 `progress_with_style(style)`。
func ProgressWithStyle[T any](slice []T, style ProgressStyle) *ProgressBarIter[T] {
	pb := New(uint64(len(slice)))
	pb.SetStyle(style)
	return NewProgressBarIterFromSlice(slice, pb)
}

// --- builder 方法（与 Rust 的 impl ProgressBarIter<T> 一一对应）---

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
	p.progress.WithElapsed(elapsed)
	return p
}

// WithFinish 对应 Rust 的 with_finish(finish: ProgressFinish)，整个 ProgressFinish（含 Msg）透传。
func (p *ProgressBarIter[T]) WithFinish(f ProgressFinish) *ProgressBarIter[T] {
	p.progress.WithFinish(f)
	return p
}

// Next 对应 Rust 的 Iterator::next：
// 取到元素就 inc(1)；取完了且进度条未结束，则 finish_using_style()（用样式里存的完成行为，
// 默认是 AndClear，不是 Finish() 的 AndLeave）。
func (p *ProgressBarIter[T]) Next() (T, bool) {
	var zero T
	if p.idx >= p.end {
		if !p.progress.IsFinished() {
			p.progress.FinishUsingStyle()
		}
		return zero, false
	}
	item := p.it[p.idx]
	p.idx++
	p.progress.Inc(1)
	return item, true
}

// NextBack 对应 Rust 的 DoubleEndedIterator::next_back，推进逻辑与 next 相同。
func (p *ProgressBarIter[T]) NextBack() (T, bool) {
	var zero T
	if p.idx >= p.end {
		if !p.progress.IsFinished() {
			p.progress.FinishUsingStyle()
		}
		return zero, false
	}
	p.end--
	item := p.it[p.end]
	p.progress.Inc(1)
	return item, true
}

// Len 对应 Rust 的 ExactSizeIterator::len：返回剩余元素个数。
func (p *ProgressBarIter[T]) Len() int { return p.end - p.idx }

// --- 对应 Rust 的 `impl io::Read for ProgressBarIter<R>` / `impl io::Write for ProgressBarIter<W>` ---
//
// 注意 Rust 这里推进位置用的是 set_position(seek_max.update_seq(position(), n))，
// 不是 inc(n)：读写可能被 seek 打断，位置要过一遍 SeekMax 才稳定。

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
	pr.progress.SetPosition(pr.seekMax.UpdateSeq(pr.progress.Position(), uint64(n)))
	return n, err
}

type ProgressWriterIter struct {
	w        io.Writer
	progress *ProgressBar
	seekMax  *SeekMax
}

func NewProgressWriterIter(w io.Writer, pb *ProgressBar) *ProgressWriterIter {
	return &ProgressWriterIter{w: w, progress: pb, seekMax: NewSeekMax()}
}

func (pw *ProgressWriterIter) Write(b []byte) (int, error) {
	n, err := pw.w.Write(b)
	pw.progress.SetPosition(pw.seekMax.UpdateSeq(pw.progress.Position(), uint64(n)))
	return n, err
}

// Flush 对应 Rust 的 io::Write::flush：直接透传给底层 writer。
func (pw *ProgressWriterIter) Flush() error {
	if f, ok := pw.w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// --- SeekMax / MaxRingBuf，对应 Rust 的 SeekMax 与 MaxRingBuf ---

// SeekMax 记住最近 HISTORY 次位置里最大的那个，用来把抖动的 seek 摊平成单调的位置显示。
// 与 Rust 一致：缓冲只在发生过 seek 之后才启用（Rust 里是 Option），
// 连续 RESET 次顺序读写之后丢弃历史回到「直通」模式。
type SeekMax struct {
	buf    []uint64
	head   int
	maxPos int
	seq    int
	active bool
}

func NewSeekMax() *SeekMax {
	return &SeekMax{buf: make([]uint64, seekMaxHistory), active: false}
}

// UpdateSeq 对应 Rust 的 update_seq。
func (s *SeekMax) UpdateSeq(prev uint64, delta uint64) uint64 {
	newPos := prev + delta
	if !s.active {
		return newPos
	}
	s.seq++
	if s.seq >= seekMaxReset {
		s.reset()
		return newPos
	}
	s.update(newPos)
	return s.max()
}

// UpdateSeek 对应 Rust 的 update_seek：一旦 seek 过就启用缓冲并清掉顺序计数。
func (s *SeekMax) UpdateSeek(newpos uint64) uint64 {
	s.active = true
	s.seq = 0
	s.update(newpos)
	return s.max()
}

func (s *SeekMax) reset() {
	s.active = false
	s.seq = 0
	s.head = 0
	s.maxPos = 0
	for i := range s.buf {
		s.buf[i] = 0
	}
}

// update 对应 Rust 的 MaxRingBuf::update。
func (s *SeekMax) update(new uint64) {
	prevMax := s.buf[s.maxPos]
	s.buf[s.head] = new

	switch {
	case new > prevMax:
		// 新的最大值
		s.maxPos = s.head
	case s.maxPos == s.head && new < prevMax:
		// 记最大值的槽被覆盖成了更小的数，线性扫描找新的最大值
		idx := 0
		for i := 1; i < len(s.buf); i++ {
			if s.buf[i] > s.buf[idx] {
				idx = i
			}
		}
		s.maxPos = idx
	}

	s.head = (s.head + 1) % len(s.buf)
}

func (s *SeekMax) max() uint64 { return s.buf[s.maxPos] }
