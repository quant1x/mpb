// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/multi.rs：MultiProgress 的并发更新与 Println；
// 带 -capture 时把内存终端的纯文本帧写入 examples/multi_capture.out。
package main

import (
	"flag"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/quant1x/mpb"
)

func runMulti(args []string) {
	// seed the RNG to mirror Rust's thread-local RNG behavior
	rand.Seed(time.Now().UnixNano())
	flags := flag.NewFlagSet("multi", flag.ContinueOnError)
	capture := flags.Bool("capture", false, "write plain-text capture to examples/multi_capture.out")
	_ = flags.Parse(args)

	var m *mpb.MultiProgress
	var memTerm *mpb.InMemoryTerm
	var outFile *os.File
	if *capture {
		// create capture file but do not redirect stdout/stderr; instead
		// use an InMemoryTerm as draw target so no ANSI/cursor control
		// sequences are emitted. We'll write the captured plain text
		// to the file at the end.
		var err error
		outFile, err = os.Create("examples/multi_capture.out")
		if err != nil {
			panic(err)
		}
		// choose sufficiently large rows/cols to avoid truncation
		memTerm = mpb.NewInMemoryTerm(2000, 200)
		m = mpb.WithDrawTarget(mpb.NewInMemoryDrawTarget(memTerm))
		defer func() {
			// flush captured contents (plain text) to file
			if outFile != nil {
				_, _ = outFile.WriteString(memTerm.Contents())
				_ = outFile.Close()
			}

		}()
	} else {
		m = mpb.NewMultiProgress()
	}

	sty, err := mpb.ProgressStyleWithTemplate("[{elapsed_precise}] {bar:40.cyan/blue} {pos:>7}/{len:7} {msg}")
	if err != nil {
		panic(err)
	}
	sty = sty.ProgressChars("##-")

	n := 200
	pb := m.Add(mpb.New(uint64(n)))
	pb.SetStyle(sty)
	pb.WithMessage("todo")

	pb2 := m.Add(mpb.New(uint64(n)))
	pb2.SetStyle(sty)
	pb2.WithMessage("finished")

	pb3 := m.InsertAfter(pb2, mpb.New(1024))
	pb3.SetStyle(sty)

	if err := m.Println("starting!"); err != nil {
		panic(err)
	}

	var wg sync.WaitGroup

	// background worker for pb3 (faster)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1024; i++ {
			time.Sleep(2 * time.Millisecond)
			//pb3.WithMessage("item #" + strconv.Itoa(i+1))
			pb3.Inc(1)
		}
		if err := m.Println("pb3 is done!"); err != nil {
			panic(err)
		}
		pb3.FinishWithMessage("done")
	}()

	// spawn n tasks that advance pb and pb2
	var innerWg sync.WaitGroup
	for i := 0; i < n; i++ {
		time.Sleep(15 * time.Millisecond)
		if i == n/3 {
			time.Sleep(2 * time.Second)
		}
		pb.Inc(1)
		innerWg.Add(1)
		// capture pb2 into a local variable for the goroutine (mirror Rust's clone)
		pb2copy := pb2
		go func() {
			defer innerWg.Done()
			// random sleep between 1s and 5s
			dur := time.Duration(1000+rand.Intn(4000)) * time.Millisecond
			time.Sleep(dur)
			pb2copy.Inc(1)
		}()
	}

	pb.FinishWithMessage("all jobs started")
	innerWg.Wait()
	wg.Wait()
	pb2.FinishWithMessage("all jobs done")
	if !*capture {
		if err := m.Clear(); err != nil {
			panic(err)
		}
	}
}
