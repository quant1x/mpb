// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/cargowrap.rs：把 cargo build --release 的 stderr 逐行
// 喂给一个 spinner。需要本机装了 cargo 才能跑。
package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/quant1x/mpb"
)

// tick_chars("/|\\- ") —— Rust 逐字符成帧，末尾空格也算一帧。
func frames(s string) []string {
	out := make([]string, 0, len([]rune(s)))
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

func main() {
	started := time.Now()

	fmt.Println("Compiling package in release mode...")

	pb := mpb.NewSpinner()
	pb.EnableSteadyTick(200 * time.Millisecond)
	sty, err := mpb.ProgressStyleWithTemplate("{spinner:.dim.bold} cargo: {wide_msg}")
	if err != nil {
		panic(err)
	}
	sty = sty.TickStrings(frames("/|\\- "))
	pb.SetStyle(sty)

	cmd := exec.Command("cargo", "build", "--release")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			pb.SetMessage(line)
		}
		pb.ForceDraw() // Rust: pb.tick()
	}
	_ = cmd.Wait()

	pb.FinishAndClear()

	fmt.Printf("Done in %s\n", mpb.HumanDuration(time.Since(started)))
}
