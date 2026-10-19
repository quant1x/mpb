// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/tokio.rs：定时器驱动进度条。
// Rust 用 tokio 的 interval + block_on；Go 用 time.Ticker，语义等价
// （每 5ms 一个 tick，共 1024 步）。
package main

import (
	"time"

	"github.com/quant1x/mpb"
)

func runTokio() {
	// Plain progress bar, totaling 1024 steps.
	const steps = 1024
	pb := mpb.New(steps)

	// Stream of events, triggering every 5ms.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	// Drive the "future" to completion, blocking until done.
	for i := 0; i < steps; i++ {
		<-ticker.C
		pb.Inc(1)
	}

	// Mark the progress bar as finished.
	pb.Finish()
}
