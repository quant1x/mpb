package mpb

import (
	"strings"
	"testing"
)

// TestRenderToInMemoryTerm 在 InMemoryTerm 上绘制进度条并断言渲染文本包含预期内容。
func TestRenderToInMemoryTerm(t *testing.T) {
	term := NewInMemoryTerm(10, 80)
	length := uint64(10)
	dt := NewInMemoryDrawTarget(term)
	pb := NewWithDrawTarget(&length, dt)
	defer pb.Abandon()

	pb.SetStyle(DefaultProgressStyleBar())
	pb.Inc(3)
	// 强制绘制到 draw target
	pb.ForceDraw()

	out := term.Contents()
	if !strings.Contains(out, "3/10") {
		t.Fatalf("render output does not contain expected '3/10', got: %q", out)
	}
}
