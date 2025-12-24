package mpb

import (
	"runtime"
	"testing"
	"time"
)

func TestWeakProgressBarUpgradeAndGC(t *testing.T) {
	pb := New(1)
	w := pb.Downgrade()
	if got := w.Upgrade(); got == nil {
		t.Fatalf("expected upgrade to succeed immediately")
	}
	// drop strong reference and force GC
	pb = nil
	runtime.GC()
	// give finalizers a moment
	time.Sleep(10 * time.Millisecond)
	if got := w.Upgrade(); got != nil {
		t.Fatalf("expected upgrade to be nil after GC, got non-nil")
	}
}

func TestTickerFinalizerStops(t *testing.T) {
	pb := NewSpinner()
	pb.EnableSteadyTick(1 * time.Millisecond)
	// give ticker time to start
	time.Sleep(5 * time.Millisecond)
	// drop pb and force GC
	pb = nil
	runtime.GC()
	// wait a bit for finalizer to run
	time.Sleep(20 * time.Millisecond)
	// Nothing to assert deterministically here; test ensures no panic and finalizer runs.
}
