package mpb

import (
	"sync"
)

// 简化的 Rayon 移植：提供把切片或已有 ProgressBarIter 并行执行的辅助方法。
// 目标是支持基本的并行映射（MapParallel），并在每处理一个元素时更新进度条。

// ProgressWithSlice 使用指定的进度条包装切片，返回一个 ProgressBarIter。
func ProgressWithSlice[T any](slice []T, pb *ProgressBar) *ProgressBarIter[T] {
	return NewProgressBarIterFromSlice(slice, pb)
}

// ProgressCount 为切片创建一个新的进度条并返回包装器。
func ProgressCount[T any](slice []T, count uint64) *ProgressBarIter[T] {
	return NewProgressBarIterFromSlice(slice, New(count))
}

// Progress 使用切片长度创建进度条并返回包装器。
func Progress[T any](slice []T) *ProgressBarIter[T] {
	return ProgressCount(slice, uint64(len(slice)))
}

// ProgressWithStyle 为切片创建进度条并设置样式。
func ProgressWithStyle[T any](slice []T, style ProgressStyle) *ProgressBarIter[T] {
	pb := New(uint64(len(slice)))
	pb.SetStyle(style)
	return NewProgressBarIterFromSlice(slice, pb)
}

// MapParallel 对 ProgressBarIter 的元素并行执行函数 fn（worker 数量可控），返回结果切片。
// 每处理一个元素会调用底层进度条的 Inc(1)。
func MapParallel[T any, U any](p *ProgressBarIter[T], workers int, fn func(T) U) []U {
	if workers <= 0 {
		workers = 1
	}
	in := make(chan T)
	out := make(chan U)
	var wg sync.WaitGroup

	// 启动 worker
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for itm := range in {
				res := fn(itm)
				if p.progress != nil {
					p.progress.Inc(1)
				}
				out <- res
			}
		}()
	}

	// 生产者和关闭协程
	go func() {
		for _, itm := range p.it {
			in <- itm
		}
		close(in)
		wg.Wait()
		close(out)
	}()

	res := make([]U, 0, len(p.it))
	for v := range out {
		res = append(res, v)
	}
	if p.progress != nil && !p.progress.IsFinished() {
		p.progress.Finish()
	}
	return res
}
