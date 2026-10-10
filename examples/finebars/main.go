// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/finebars.rs：同一个 MultiProgress 里挂 5 条不同粒度、
// 不同颜色的进度条，各自独立推进（Rust 用 thread::spawn，Go 用 goroutine）。
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/quant1x/mpb"
)

func main() {
	styles := []struct {
		prefix string
		chars  string
		color  string
	}{
		{"Rough bar:", "█  ", "red"},
		{"Fine bar: ", "█▉▊▋▌▍▎▏  ", "yellow"},
		{"Vertical: ", "█▇▆▅▄▃▂▁  ", "green"},
		{"Fade in:  ", "█▓▒░  ", "blue"},
		{"Blocky:   ", "█▛▌▖  ", "magenta"},
	}

	m := mpb.NewMultiProgress()

	var wg sync.WaitGroup
	for _, s := range styles {
		pb := m.Add(mpb.New(512))
		sty, err := mpb.ProgressStyleWithTemplate(
			fmt.Sprintf("{prefix:.bold}▕{bar:.%s}▏{msg}", s.color))
		if err != nil {
			panic(err)
		}
		sty.SetProgressChars(s.chars)
		pb.SetStyle(sty)
		pb.SetPrefix(s.prefix)

		wait := time.Duration(10+rand.Intn(20)) * time.Millisecond
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 512; i++ {
				time.Sleep(wait)
				pb.Inc(1)
				pb.SetMessage(fmt.Sprintf("%3d%%", 100*i/512))
			}
			pb.FinishWithMessage("100%")
		}()
	}

	wg.Wait()
}
