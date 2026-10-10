// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/long-spinner.rs：steady tick + 自定义 spinner 帧。
// 更多 spinner 帧可看 cli-spinners：
// https://github.com/sindresorhus/cli-spinners/blob/master/spinners.json
package main

import (
	"time"

	"github.com/quant1x/mpb"
)

func main() {
	pb := mpb.NewSpinner()
	pb.EnableSteadyTick(120 * time.Millisecond)
	sty, err := mpb.ProgressStyleWithTemplate("{spinner:.blue} {msg}")
	if err != nil {
		panic(err)
	}
	sty = sty.TickStrings([]string{
		"▹▹▹▹▹",
		"▸▹▹▹▹",
		"▹▸▹▹▹",
		"▹▹▸▹▹",
		"▹▹▹▸▹",
		"▹▹▹▹▸",
		"▪▪▪▪▪",
	})
	pb.SetStyle(sty)
	pb.SetMessage("Calculating...")
	time.Sleep(5 * time.Second)
	pb.FinishWithMessage("Done")
}
