// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/download.rs：模拟下载进度。
// 演示 {bytes}/{total_bytes} 与自定义 eta key（Rust 的 with_key）。
package main

import (
	"fmt"
	"time"

	"github.com/quant1x/mpb"
)

func runDownload() {
	var downloaded uint64
	const totalSize uint64 = 231231231

	pb := mpb.New(totalSize)
	sty, err := mpb.ProgressStyleWithTemplate(
		"{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})")
	if err != nil {
		panic(err)
	}
	// Rust：.with_key("eta", |state, w| write!(w, "{:.1}s", state.eta().as_secs_f64()))
	// Go 的 WithKey 是指针接收者、原地修改，不返回新值。
	sty.WithKey("eta", func(state *mpb.ProgressState) string {
		return fmt.Sprintf("%.1fs", state.Eta().Seconds())
	})
	sty = sty.ProgressChars("#>-")
	pb.SetStyle(sty)

	for downloaded < totalSize {
		newPos := downloaded + 223211
		if newPos > totalSize {
			newPos = totalSize
		}
		downloaded = newPos
		pb.SetPosition(newPos)
		time.Sleep(12 * time.Millisecond)
	}

	pb.FinishWithMessage("downloaded")
}
