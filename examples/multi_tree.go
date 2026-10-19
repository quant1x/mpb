// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/multi-tree.rs：树形排列的进度条。
// 元素按缩进逐级加入 MultiProgress，进度条各自推进到满；整棵树填满后收尾，
// 最后把树按缩进打印出来做对照。
package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/quant1x/mpb"
)

type Elem struct {
	key         string
	index       int
	indent      int
	progressBar *mpb.ProgressBar
}

// Rust 里是 Lazy<[Elem; 9]>，进度条在静态初始化时就建好（还没挂到 MultiProgress 上）。
var elements = []*Elem{
	{indent: 1, index: 0, progressBar: mpb.New(32), key: "jumps"},
	{indent: 2, index: 1, progressBar: mpb.New(32), key: "lazy"},
	{indent: 0, index: 0, progressBar: mpb.New(32), key: "the"},
	{indent: 3, index: 3, progressBar: mpb.New(32), key: "dog"},
	{indent: 2, index: 2, progressBar: mpb.New(32), key: "over"},
	{indent: 2, index: 1, progressBar: mpb.New(32), key: "brown"},
	{indent: 1, index: 1, progressBar: mpb.New(32), key: "quick"},
	{indent: 3, index: 5, progressBar: mpb.New(32), key: "a"},
	{indent: 3, index: 3, progressBar: mpb.New(32), key: "fox"},
}

type action struct {
	addProgressBar bool
	idx            int
}

// getAction 保证返回一个对当前树合法的动作：
//   - 全填满且元素全加完 → 结束
//   - 无空位可推进        → 加一条新的进度条
//   - 否则               → 随机挑一条还没满的推进
func getAction(tree *[]*Elem) (action, bool) {
	list := *tree
	elemLen := uint64(len(elements))
	listLen := uint64(len(list))

	sumFree := uint64(0)
	for _, e := range list {
		sumFree += *e.progressBar.Length() - e.progressBar.Position()
	}

	if sumFree == 0 && listLen == elemLen {
		// nothing to do more
		return action{}, false
	} else if sumFree == 0 && listLen < elemLen {
		// there is no place to make an increment
		return action{addProgressBar: true, idx: len(list)}, true
	}

	for {
		k := rand.Intn(17)
		if k == 0 && listLen < elemLen {
			return action{addProgressBar: true, idx: len(list)}, true
		}
		if listLen == 0 {
			// 不可达：走到这里说明 sumFree > 0，树必然非空。留个兜底防除零。
			return action{addProgressBar: true, idx: 0}, true
		}
		l := k % int(listLen)
		if list[l].progressBar.Position() < *list[l].progressBar.Length() {
			return action{addProgressBar: false, idx: l}, true
		}
	}
}

func runMultiTree() {
	rand.Seed(time.Now().UnixNano())

	mp := mpb.NewMultiProgress()
	styMain, err := mpb.ProgressStyleWithTemplate("{bar:40.green/yellow} {pos:>4}/{len:4}")
	if err != nil {
		panic(err)
	}
	styAux, err := mpb.ProgressStyleWithTemplate("{spinner:.green} {msg} {pos:>4}/{len:4}")
	if err != nil {
		panic(err)
	}

	var total uint64
	for _, e := range elements {
		total += *e.progressBar.Length()
	}
	pbMain := mp.Add(mpb.New(total))
	pbMain.SetStyle(styMain)
	for _, e := range elements {
		e.progressBar.SetStyle(styAux)
	}

	tree := make([]*Elem, 0, len(elements))

	// Rust 起了一个线程再立刻 join，等价于顺序执行；Go 直接在主 goroutine 里跑。
	pbMain.ForceDraw() // Rust: pb_main.tick()
	for {
		time.Sleep(15 * time.Millisecond)
		act, ok := getAction(&tree)
		if !ok {
			// all elements were exhausted
			pbMain.Finish()
			break
		}
		if act.addProgressBar {
			elem := elements[act.idx]
			pb := mp.Insert(uint(elem.index+1), elem.progressBar)
			pb.SetMessage(strings.Repeat("  ", elem.indent) + "  " + elem.key)
			// Rust: tree.lock().unwrap().insert(elem.index, elem)
			tree = append(tree, nil)
			copy(tree[elem.index+1:], tree[elem.index:])
			tree[elem.index] = elem
		} else {
			elem := tree[act.idx]
			elem.progressBar.Inc(1)
			if elem.progressBar.Position() >= *elem.progressBar.Length() {
				elem.progressBar.FinishWithMessage(
					strings.Repeat("  ", elem.indent) + "✔ " + elem.key)
			}
			pbMain.Inc(1)
		}
	}

	fmt.Println("===============================")
	fmt.Println("the tree should be the same as:")
	for _, e := range tree {
		fmt.Println(strings.Repeat("  ", e.indent) + "  " + e.key)
	}
}
