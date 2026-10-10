// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/multi-tree-ext.rs：multi-tree 的扩展版。
// 除了「加进度条 / 推进进度条」，还能从树里摘掉进度条（要摘的都是 "temp_*"），
// 并演示 MultiProgress 的垂直对齐：加 --bottom-alignment 改成底部对齐。
//
//	go run ./examples/multi-tree-ext --bottom-alignment
package main

import (
	"flag"
	"math/rand"
	"strings"
	"time"

	"github.com/quant1x/mpb"
)

const pbLen = 32

type Item struct {
	key         string
	index       int
	indent      int
	progressBar *mpb.ProgressBar
}

type elemKind int

const (
	addItem elemKind = iota
	removeItem
)

type Elem struct {
	kind  elemKind
	item  *Item // addItem
	index int   // removeItem
}

func add(indent, index int, key string) Elem {
	return Elem{
		kind: addItem,
		item: &Item{indent: indent, index: index, key: key, progressBar: mpb.New(pbLen)},
	}
}

func remove(index int) Elem { return Elem{kind: removeItem, index: index} }

// Rust 里是 Lazy<[Elem; 27]>：进度条在静态初始化时建好，还没挂到 MultiProgress 上。
var elements = []Elem{
	add(9, 0, "dog"),
	add(0, 0, "temp_1"),
	add(8, 1, "lazy"),
	add(0, 1, "temp_2"),
	add(1, 0, "the"),
	add(0, 0, "temp_3"),
	add(7, 3, "a"),
	add(0, 3, "temp_4"),
	add(6, 2, "over"),
	remove(6),
	remove(4),
	remove(3),
	remove(0),
	add(0, 2, "temp_5"),
	add(4, 1, "fox"),
	add(0, 1, "temp_6"),
	add(2, 1, "quick"),
	add(0, 1, "temp_7"),
	add(5, 5, "jumps"),
	add(0, 5, "temp_8"),
	add(3, 4, "brown"),
	add(0, 3, "temp_9"),
	remove(10),
	remove(7),
	remove(4),
	remove(3),
	remove(1),
}

// elemIdx 对应 Rust 的 static ELEM_IDX: AtomicUsize。
var elemIdx int

type actionKind int

const (
	actionStop actionKind = iota
	actionModifyTree
	actionIncProgressBar
)

type action struct {
	kind actionKind
	idx  int
}

// getAction 保证返回一个对当前树合法的动作。
func getAction(items []*Item) action {
	// the indices of those items, that not completed yet
	var uncompleted []int
	for i, item := range items {
		if item.progressBar.Position() < *item.progressBar.Length() {
			uncompleted = append(uncompleted, i)
		}
	}

	k := rand.Intn(16)
	if (k > 0 || (k == 0 && elemIdx == len(elements))) && len(uncompleted) > 0 {
		return action{kind: actionIncProgressBar, idx: uncompleted[rand.Intn(len(uncompleted))]}
	} else if elemIdx < len(elements) {
		elemIdx++
		return action{kind: actionModifyTree, idx: elemIdx - 1}
	}
	// nothing to do more
	return action{kind: actionStop}
}

// green 对应 console 的 style("✔").green()。
func green(s string) string { return "\x1b[32m" + s + "\x1b[0m" }

func main() {
	bottomAlignment := flag.Bool("bottom-alignment", false, "align the MultiProgress to the bottom")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	mp := mpb.NewMultiProgress()
	alignment := mpb.MultiProgressAlignmentTop
	if *bottomAlignment {
		alignment = mpb.MultiProgressAlignmentBottom
	}
	mp.SetAlignment(alignment)

	styMain, err := mpb.ProgressStyleWithTemplate("{bar:40.green/yellow} {pos:>4}/{len:4}")
	if err != nil {
		panic(err)
	}
	styAux, err := mpb.ProgressStyleWithTemplate("[{pos:>2}/{len:2}] {prefix}{spinner:.green} {msg}")
	if err != nil {
		panic(err)
	}
	styFin, err := mpb.ProgressStyleWithTemplate("[{pos:>2}/{len:2}] {prefix}{msg}")
	if err != nil {
		panic(err)
	}

	var total uint64
	for _, e := range elements {
		switch e.kind {
		case addItem:
			total += *e.item.progressBar.Length()
		case removeItem:
			total += 1
		}
	}
	pbMain := mp.Add(mpb.New(total))
	pbMain.SetStyle(styMain)
	for _, e := range elements {
		if e.kind == addItem {
			e.item.progressBar.SetStyle(styAux)
		}
	}

	items := make([]*Item, 0, len(elements))

	pbMain.ForceDraw() // Rust: pb_main.tick()
	for {
		switch act := getAction(items); act.kind {
		case actionStop:
			// all elements were exhausted
			pbMain.Finish()
			return
		case actionModifyTree:
			switch e := elements[act.idx]; e.kind {
			case addItem:
				item := e.item
				pb := mp.Insert(uint(item.index), item.progressBar)
				pb.SetPrefix(strings.Repeat("  ", item.indent))
				pb.SetMessage(item.key)
				items = append(items, nil)
				copy(items[item.index+1:], items[item.index:])
				items[item.index] = item
			case removeItem:
				item := items[e.index]
				items = append(items[:e.index], items[e.index+1:]...)
				pb := item.progressBar
				mp.Remove(pb)
				pbMain.Inc(*pb.Length() - pb.Position())
			}
		case actionIncProgressBar:
			item := items[act.idx]
			item.progressBar.Inc(1)
			if item.progressBar.Position() >= *item.progressBar.Length() {
				item.progressBar.SetStyle(styFin)
				item.progressBar.FinishWithMessage(green("✔") + " " + item.key)
			}
			pbMain.Inc(1)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
