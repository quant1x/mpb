// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/fastbar.rs：极大量、每次更新都极快的场景，
// 用来看限速（默认 20Hz）下的表现与 Elapsed() 的耗时统计。
package main

import (
	"fmt"

	"github.com/quant1x/mpb"
)

func manyUnitsOfEasyWork(n uint64, label string) {
	pb := mpb.New(n)

	var sum uint64
	for i := uint64(0); i < n; i++ {
		// Any quick computation, followed by an update to the progress bar.
		sum += 2*i + 3
		pb.Inc(1)
	}
	pb.Finish()

	fmt.Printf("[%s] Sum (%d) calculated in %v\n", label, sum, pb.Elapsed())
}

func runFastbar() {
	const N uint64 = 1 << 20

	// Perform a long sequence of many simple computations monitored by a
	// default progress bar.
	manyUnitsOfEasyWork(N, "Default progress bar ")
}
