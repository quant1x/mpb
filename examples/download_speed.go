// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/download-speed.rs：在 download 的基础上加 {bytes_per_sec}。
package main

import (
	"time"

	"github.com/quant1x/mpb"
)

func runDownloadSpeed() {
	var downloaded uint64
	const totalSize uint64 = 231231231

	pb := mpb.New(totalSize)
	sty, err := mpb.ProgressStyleWithTemplate(
		"{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({bytes_per_sec}, {eta})")
	if err != nil {
		panic(err)
	}
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
