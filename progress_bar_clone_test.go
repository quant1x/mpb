// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

package mpb

import (
	"runtime"
	"testing"
	"time"
)

// TestCloneSharesState 验证 Clone 返回的 ProgressBar 与原始共享内部状态。
func TestCloneSharesState(t *testing.T) {
	pb := New(100)
	defer pb.Abandon()
	clone := pb.Clone()
	defer clone.Abandon()

	pb.WithMessage("hello")
	if got := clone.Message(); got != "hello" {
		t.Fatalf("clone did not observe message change: got %q", got)
	}

	pb.Inc(5)
	if pos := clone.Position(); pos != 5 {
		t.Fatalf("clone did not observe position change: got %d", pos)
	}

	clone.Finish()
	if !pb.IsFinished() {
		t.Fatalf("finish on clone not visible to original")
	}
}

// TestWeakUpgradeWhileStrongExists 验证在任意强引用存在时，WeakProgressBar.Upgrade 一定返回非 nil。
func TestWeakUpgradeWhileStrongExists(t *testing.T) {
	pb := New(1)
	clone := pb.Clone()
	defer func() { clone.Abandon() }()

	w := pb.Downgrade()
	if got := w.Upgrade(); got == nil {
		t.Fatalf("expected upgrade to succeed while strong refs exist")
	}

	// drop one strong reference and force GC; clone still holds state so Upgrade should succeed
	pb = nil
	runtime.GC()
	time.Sleep(5 * time.Millisecond)
	if got := w.Upgrade(); got == nil {
		t.Fatalf("expected upgrade to succeed while clone strong ref exists")
	}
}
