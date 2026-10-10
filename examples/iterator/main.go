// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/iterator.rs：三种迭代器包装。
//  1. Progress(slice)        —— 用切片长度当总数（Rust 的 size_hint）
//  2. ProgressCount(slice, n) —— 显式指定元素个数
//  3. ProgressWithSlice(slice, pb) —— 用自定义样式的进度条
//
// 取完元素时迭代器会按样式里的完成行为收尾（默认 FinishUsingStyle）。
package main

import (
	"fmt"
	"time"

	"github.com/quant1x/mpb"
)

func main() {
	items := make([]int, 1000)

	// Default styling, attempt to use Iterator::size_hint to count input size
	it := mpb.Progress(items)
	for {
		if _, ok := it.Next(); !ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Provide explicit number of elements in iterator
	it2 := mpb.ProgressCount(items, 1000)
	for {
		if _, ok := it2.Next(); !ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Provide a custom bar style
	pb := mpb.New(1000)
	sty, err := mpb.ProgressStyleWithTemplate(
		"{spinner:.green} [{elapsed_precise}] [{bar:40.cyan/blue}] ({pos}/{len}, ETA {eta})")
	if err != nil {
		panic(err)
	}
	pb.SetStyle(sty)
	it3 := mpb.ProgressWithSlice(items, pb)
	for {
		if _, ok := it3.Next(); !ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	fmt.Println("done")
}
