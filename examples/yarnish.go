// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/yarnish.rs：模拟 yarn 安装时的四段式输出
// （解析 → 拉取 → 链接 → 构建），最后用 HumanDuration 报总耗时。
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/quant1x/mpb"
)

var packages = []string{
	"fs-events",
	"my-awesome-module",
	"emoji-speaker",
	"wrap-ansi",
	"stream-browserify",
	"acorn-dynamic-import",
}

var commands = []string{
	"cmake .",
	"make",
	"make clean",
	"gcc foo.c -o foo",
	"gcc bar.c -o bar",
	"./helper.sh rebuild-cache",
	"make all-clean",
	"make test",
}

// Rust 里 Emoji 在非 emoji 终端会退化成 ASCII；这里直接写字面量。
const (
	lookingGlass = "🔍  "
	truck        = "🚚  "
	clip         = "🔗  "
	paper        = "📃  "
	sparkle      = "✨ "
)

// style(...).bold().dim() —— console 的粗体+暗色，Go 侧直接写 ANSI。
func boldDim(s string) string { return "\x1b[1;2m" + s + "\x1b[0m" }

// tick_chars("⠁⠂⠄⡀⢀⠠⠐⠈ ") —— Rust 是逐字符成帧，末尾那个空格也算一帧。
func frames(s string) []string {
	out := make([]string, 0, len([]rune(s)))
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

func runYarnish() {
	rand.Seed(time.Now().UnixNano())
	started := time.Now()
	spinnerStyle, err := mpb.ProgressStyleWithTemplate("{prefix:.bold.dim} {spinner} {wide_msg}")
	if err != nil {
		panic(err)
	}
	spinnerStyle = spinnerStyle.TickStrings(frames("⠁⠂⠄⡀⢀⠠⠐⠈ "))

	fmt.Printf("%s %sResolving packages...\n", boldDim("[1/4]"), lookingGlass)
	fmt.Printf("%s %sFetching packages...\n", boldDim("[2/4]"), truck)
	fmt.Printf("%s %sLinking dependencies...\n", boldDim("[3/4]"), clip)

	deps := 1232
	pb := mpb.New(uint64(deps))
	for i := 0; i < deps; i++ {
		time.Sleep(3 * time.Millisecond)
		pb.Inc(1)
	}
	pb.FinishAndClear()

	fmt.Printf("%s %sBuilding fresh packages...\n", boldDim("[4/4]"), paper)

	m := mpb.NewMultiProgress()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		count := 30 + rand.Intn(50)
		bar := m.Add(mpb.New(uint64(count)))
		bar.SetStyle(spinnerStyle)
		bar.SetPrefix(fmt.Sprintf("[%d/?]", i+1))
		wg.Add(1)
		go func() {
			defer wg.Done()
			pkg := packages[rand.Intn(len(packages))]
			for j := 0; j < count; j++ {
				cmd := commands[rand.Intn(len(commands))]
				time.Sleep(time.Duration(25+rand.Intn(175)) * time.Millisecond)
				bar.SetMessage(fmt.Sprintf("%s: %s", pkg, cmd))
				bar.Inc(1)
			}
			bar.FinishWithMessage("waiting...")
		}()
	}
	wg.Wait()
	if err := m.Clear(); err != nil {
		panic(err)
	}

	fmt.Printf("%s Done in %s\n", sparkle, mpb.HumanDuration(time.Since(started)))
}
