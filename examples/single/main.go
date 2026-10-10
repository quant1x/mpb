// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/single.rs：最基础的单条进度条。
package main

import (
	"time"

	"github.com/quant1x/mpb"
)

func main() {
	pb := mpb.New(1024)
	for i := 0; i < 1024; i++ {
		time.Sleep(5 * time.Millisecond)
		pb.Inc(1)
	}
	pb.FinishWithMessage("done")
}
