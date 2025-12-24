package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"gitee.com/quant1x/mpb"
)

func main() {
	fmt.Println("start")
	m := mpb.NewMultiProgress()

	sty := mpb.DefaultProgressStyleBar()

	n := 5
	pb := m.Add(mpb.New(uint64(n)))
	pb.SetStyle(sty)
	pb.WithMessage("todo")

	pb2 := m.Add(mpb.New(uint64(n)))
	pb2.SetStyle(sty)
	pb2.WithMessage("finished")

	pb3 := m.InsertAfter(pb2, mpb.New(10))
	pb3.SetStyle(sty)

	m.Println("starting!")

	var wg sync.WaitGroup

	// background worker for pb3
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			time.Sleep(1 * time.Millisecond)
			pb3.WithMessage("item #" + strconv.Itoa(i+1))
			pb3.Inc(1)
		}
		m.Println("pb3 is done!")
		pb3.FinishWithMessage("done")
	}()

	// spawn n tasks that advance pb and pb2
	var innerWg sync.WaitGroup
	for i := 0; i < n; i++ {
		time.Sleep(1 * time.Millisecond)
		if i == n/3 {
			time.Sleep(10 * time.Millisecond)
		}
		pb.Inc(1)
		innerWg.Add(1)
		go func() {
			defer innerWg.Done()
			// random sleep between 10ms and 50ms
			dur := time.Duration(10+rand.Intn(40)) * time.Millisecond
			time.Sleep(dur)
			pb2.Inc(1)
		}()
	}

	pb.FinishWithMessage("all jobs started")
	innerWg.Wait()
	wg.Wait()
	pb2.FinishWithMessage("all jobs done")
	m.Clear()
	fmt.Println("end")
}
