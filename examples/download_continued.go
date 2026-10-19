// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/download-continued.rs：断点续传 —— 起始位置不为 0，
// 并调用 ResetEta() 让 ETA 从当前位置重新估算。
package main

import (
	"time"

	"github.com/quant1x/mpb"
)

func runDownloadContinued() {
	var downloaded uint64 = 69369369
	const totalSize uint64 = 231231231

	pb := mpb.New(totalSize)
	sty, err := mpb.ProgressStyleWithTemplate(
		"{spinner:.green} [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})")
	if err != nil {
		panic(err)
	}
	sty = sty.ProgressChars("#>-")
	pb.SetStyle(sty)
	pb.SetPosition(downloaded)
	pb.ResetEta()

	for downloaded < totalSize {
		downloaded += 123211
		if downloaded > totalSize {
			downloaded = totalSize
		}
		pb.SetPosition(downloaded)
		time.Sleep(12 * time.Millisecond)
	}

	pb.FinishWithMessage("downloaded")
}
