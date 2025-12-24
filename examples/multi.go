package main

import (
	"math/rand"
	"strconv"
	"sync"
	"time"

	"gitee.com/quant1x/mpb"
)

func main() {
	m := mpb.NewMultiProgress()

	sty := mpb.DefaultProgressStyleBar()

	n := 200
	pb := m.Add(mpb.New(uint64(n)))
	pb.SetStyle(sty)
	pb.WithMessage("todo")

	pb2 := m.Add(mpb.New(uint64(n)))
	pb2.SetStyle(sty)
	pb2.WithMessage("finished")

	pb3 := m.InsertAfter(pb2, mpb.New(1024))
	pb3.SetStyle(sty)

	m.Println("starting!")

	var wg sync.WaitGroup

	// background worker for pb3
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1024; i++ {
			time.Sleep(2 * time.Millisecond)
			pb3.WithMessage("item #" + strconv.Itoa(i+1))
			pb3.Inc(1)
		}
		m.Println("pb3 is done!")
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
		go func() {
			defer innerWg.Done()
			// random sleep between 1s and 5s
			dur := time.Duration(1+rand.Intn(5)) * time.Second
			time.Sleep(dur)
			pb2.Inc(1)
		}()
	}

	pb.FinishWithMessage("all jobs started")
	innerWg.Wait()
	wg.Wait()
	pb2.FinishWithMessage("all jobs done")
	m.Clear()
}
