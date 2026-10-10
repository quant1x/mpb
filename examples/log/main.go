// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/log.rs：边推进边用 Println 打日志。
// 日志行走 MultiState 的 orphan 行通道，不会把进度条画面冲乱。
package main

import (
	"fmt"
	"time"

	"github.com/quant1x/mpb"
)

func main() {
	pb := mpb.New(100)
	for i := 0; i < 100; i++ {
		time.Sleep(25 * time.Millisecond)
		pb.Println(fmt.Sprintf("[+] finished #%d", i))
		pb.Inc(1)
	}
	pb.FinishWithMessage("done")
}
