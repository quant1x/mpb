// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/morebars.rs：MultiProgress 里一条总进度条统着多条子进度条。
package main

import (
	"time"

	"github.com/quant1x/mpb"
)

func runMorebars() {
	m := mpb.NewMultiProgress()
	sty, err := mpb.ProgressStyleWithTemplate("{bar:40.green/yellow} {pos:>7}/{len:7}")
	if err != nil {
		panic(err)
	}

	pb := m.Add(mpb.New(5))
	pb.SetStyle(sty)

	// make sure we show up at all.  otherwise no rendering event.
	// Rust 这里是 pb.tick()；Go 的 tick 未导出，用 ForceDraw 强制出一帧。
	pb.ForceDraw()
	for i := 0; i < 5; i++ {
		pb2 := m.Add(mpb.New(128))
		pb2.SetStyle(sty)
		for j := 0; j < 128; j++ {
			time.Sleep(5 * time.Millisecond)
			pb2.Inc(1)
		}
		pb2.Finish()
		pb.Inc(1)
	}
	pb.FinishWithMessage("done")
}
