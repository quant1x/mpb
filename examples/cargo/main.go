// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/cargo.rs：模仿 cargo 编译时的输出（并不真的调用 cargo）。
// 4 个「worker」并发地取 crate 列表，主线程负责画进度条与打 Compiling 行。
package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/quant1x/mpb"
	"golang.org/x/term"
)

type crate struct {
	name    string
	version string
}

var crates = []crate{
	{"console", "v0.14.1"},
	{"lazy_static", "v1.4.0"},
	{"libc", "v0.2.93"},
	{"regex", "v1.4.6"},
	{"regex-syntax", "v0.6.23"},
	{"terminal_size", "v0.1.16"},
	{"libc", "v0.2.93"},
	{"unicode-width", "v0.1.8"},
	{"lazy_static", "v1.4.0"},
	{"number_prefix", "v0.4.0"},
	{"regex", "v1.4.6"},
	{"rand", "v0.8.3"},
	{"getrandom", "v0.2.2"},
	{"cfg-if", "v1.0.0"},
	{"libc", "v0.2.93"},
	{"rand_chacha", "v0.3.0"},
	{"ppv-lite86", "v0.2.10"},
	{"rand_core", "v0.6.2"},
	{"getrandom", "v0.2.2"},
	{"rand_core", "v0.6.2"},
	{"tokio", "v1.5.0"},
	{"bytes", "v1.0.1"},
	{"pin-project-lite", "v0.2.6"},
	{"slab", "v0.4.3"},
	{"indicatif", "v0.15.0"},
}

// number of cpus
const numCPUs = 4

// greenBold 对应 console 的 Style::new().green().bold()。
// 注意要先补白再套样式：Rust 的 {:>12} 作用在 apply_to() 之后，可见宽度仍是 12。
func greenBold(s string) string { return "\x1b[1;32m" + s + "\x1b[0m" }

// termWidth 对应 Rust 的 Term::stdout().size().1（列数）；拿不到就按 80 算。
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func main() {
	rand.Seed(time.Now().UnixNano())
	start := time.Now()

	// mimic cargo progress bar although it behaves a bit different
	// note that bar size is fixed unlike cargo which is dynamic
	// and also the truncation in cargo uses trailers (`...`)
	tpl := "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len}"
	if termWidth() > 80 {
		tpl = "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len} {wide_msg}"
	}
	pb := mpb.New(uint64(len(crates)))
	sty, err := mpb.ProgressStyleWithTemplate(tpl)
	if err != nil {
		panic(err)
	}
	sty = sty.ProgressChars("=> ")
	pb.SetStyle(sty)
	pb.SetPrefix("Building")

	// crates to be iterated but not exactly a tree
	var mu sync.Mutex
	next := 0
	nextCrate := func() (crate, bool) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(crates) {
			return crate{}, false
		}
		c := crates[next]
		next++
		return c, true
	}

	type message struct {
		worker  int
		current crate
		has     bool
	}
	// Rust 的 mpsc 是无界通道；Go 给足够缓冲，避免 worker 卡在发送上。
	rx := make(chan message, len(crates)+numCPUs)

	// process in another thread
	for n := 0; n < numCPUs; n++ {
		go func(n int) {
			for {
				c, ok := nextCrate()
				// notify main thread if n thread is processing a crate
				rx <- message{worker: n, current: c, has: ok}
				if !ok {
					return
				}
				// last compile and linking is always slow, let's mimic that
				if c == crates[len(crates)-1] {
					time.Sleep(time.Duration(1000+rand.Intn(1000)) * time.Millisecond)
				} else {
					time.Sleep(time.Duration(250+rand.Intn(750)) * time.Millisecond)
				}
			}
		}(n)
	}

	// do progress drawing in main thread
	processing := make([]*crate, numCPUs)
	finished := 0
	for finished < numCPUs {
		m := <-rx
		if !m.has {
			processing[m.worker] = nil
			finished++
			continue
		}
		current := m.current
		processing[m.worker] = &current

		names := make([]string, 0, numCPUs)
		for _, p := range processing {
			if p != nil {
				names = append(names, p.name)
			}
		}
		pb.SetMessage(strings.Join(names, ", "))

		// crate is being built
		pb.Println(fmt.Sprintf("%s %s %s",
			greenBold(fmt.Sprintf("%12s", "Compiling")), current.name, current.version))
		pb.Inc(1)
	}
	pb.FinishAndClear()

	// compilation is finished
	fmt.Printf("%s dev [unoptimized + debuginfo] target(s) in %s\n",
		greenBold(fmt.Sprintf("%12s", "Finished")), mpb.HumanDuration(time.Since(start)))
}
