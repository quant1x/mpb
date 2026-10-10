package mpb

import (
	"sync"
)

// 本文件对应 Rust 的 rayon.rs（并行迭代器适配）。
//
// 说明：Rust 里串行迭代器（std::iter::Iterator）与并行迭代器（rayon::ParallelIterator）是两套
// trait，所以 iter.rs 与 rayon.rs 各有一组 progress / progress_count / progress_with /
// progress_with_style。Go 没有这层区分：同一个 *ProgressBarIter[T] 既被 Next() 串行消费，
// 也被下面的 MapParallel 并行消费，那组构造器因此只有一份，放在 iter.go 里共用
// （不另造一套"并行版"构造器，免得同一语义出现两个入口）。

// MapParallel 对应 Rust 里对包装后的并行迭代器做 `map(...).collect()`：
//   - 与 ProgressFolder::consume 一致：每消费一个元素 inc(1)（先推进再干活）；
//   - 与 IndexedParallelIterator 的 collect 一致：**结果按输入顺序排列**
//     （原来的实现从 out channel 里先到先收，顺序是乱的）；
//   - 与 Rust 一致：消费完不收尾进度条（Folder::complete 只 complete 底层，
//     不调 finish；是否收尾由调用方决定）；
//   - 迭代器被消费掉（Rust 的 drive 取得所有权），消费后 Len() 为 0。
//
// workers 对应 rayon 的分片并发度：Rust 那边由全局线程池决定，Go 这里显式给。
func MapParallel[T any, U any](p *ProgressBarIter[T], workers int, fn func(T) U) []U {
	if workers <= 0 {
		workers = 1
	}

	items := p.it[p.idx:p.end]
	res := make([]U, len(items))

	in := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				if p.progress != nil {
					p.progress.Inc(1)
				}
				res[j] = fn(items[j])
			}
		}()
	}

	for j := range items {
		in <- j
	}
	close(in)
	wg.Wait()

	// 消费掉迭代器（Rust 的 drive(self)）
	p.idx = p.end
	return res
}
