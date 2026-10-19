// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestDroppedBarBecomesZombie 锁住 Rust 的 `Drop for BarState` 语义：
// MultiStateMember 只保存弱 id，最后一个 ProgressBar 被回收后 finalizer
// 应按 on_finish 收尾并 mark_zombie，而不是一直占着多进度条的一行。
func TestDroppedBarBecomesZombie(t *testing.T) {
	term := NewInMemoryTerm(10, 60)
	mp := WithDrawTarget(NewInMemoryDrawTarget(term))
	pb := mp.Add(New(10))
	pb.Inc(3)

	if got := term.Contents(); !strings.Contains(got, "3/10") {
		t.Fatalf("前置条件不成立，渲染内容 = %q", got)
	}

	pb = nil
	deadline := time.Now().Add(2 * time.Second)
	for {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)

		mp.State.mu.RLock()
		remaining := len(mp.State.ordering)
		mp.State.mu.RUnlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bar 被回收后仍留在 MultiProgress 中：ordering=%d", remaining)
		}
	}

	if got := term.Contents(); got != "" {
		t.Fatalf("默认 FinishAndClear 应清空最后一行，实际 = %q", got)
	}
}
