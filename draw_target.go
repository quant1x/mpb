package mpb

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// 简化版 draw_target 实现，替代 state.go 中的占位 simpleDrawTarget。
// 提供 ProgressDrawTarget 的基本实现以及 Drawable 对象，便于增量迁移。

type progressDrawTarget struct {
	hidden bool
	stderr bool
	width  *uint16
	term   TermLike
	mu     sync.Mutex
	// 上次完整绘制高度（行数），用于下一次上移覆盖
	lastHeight int
	// 当前帧标记（毫秒）用于多 Drawable 协调
	lastFrame int64
	// 当前帧已写入行数（累加）
	frameLines int
}

// VisualLines 是一个简化的行计数类型（占位实现）。
type VisualLines int

func (p *progressDrawTarget) IsHidden() bool { return p.hidden }
func (p *progressDrawTarget) IsStderr() bool { return p.stderr }
func (p *progressDrawTarget) Width() *uint16 { return p.width }

func (p *progressDrawTarget) Remote() (*RemoteStub, int, bool) { return nil, 0, false }

func (p *progressDrawTarget) Disconnect(now time.Time) {}

// Drawable 返回一个实现 state.go 中 Drawable 接口的对象。
func (p *progressDrawTarget) Drawable(force bool) Drawable {
	ds := &DrawStateStub{lines: make([]LineType, 0), contents: make([]string, 0)}
	return &simpleDrawable{state: ds, width: p.width, owner: p}
}

// SetMoveCursor 为进度目标设置是否使用移动光标而不是清行（简化实现）。
func (p *progressDrawTarget) SetMoveCursor(move bool) { /* no-op */ }

type simpleDrawable struct {
	state      *DrawStateStub
	width      *uint16
	lastDraw   time.Time
	owner      *progressDrawTarget
	lastHeight int
}

func (d *simpleDrawable) State() *DrawStateStub { return d.state }
func (d *simpleDrawable) Draw() error {
	d.lastDraw = time.Now()
	if d.owner != nil {
		if d.owner.term != nil {
			for _, line := range d.state.contents {
				_ = d.owner.term.WriteLine(line)
			}
			_ = d.owner.term.Flush()
			return nil
		}
		// fallback: write to stderr so examples show output
		out := os.Stderr
		lines := d.state.contents
		newLen := len(lines)
		// 使用 owner 的帧协调：仅在帧的第一个 Drawable 上移光标一次
		nowMs := time.Now().UnixNano() / int64(time.Millisecond)
		d.owner.mu.Lock()
		prevHeight := d.owner.lastHeight
		if d.owner.lastFrame != nowMs {
			// 新帧，先上移到上次绘制的起始行
			if prevHeight > 0 {
				fmt.Fprintf(out, "\x1b[%dA", prevHeight)
			}
			d.owner.lastFrame = nowMs
			d.owner.frameLines = 0
		}
		// 构建整帧输出，避免多次写入导致闪烁或行插入
		var sb strings.Builder
		// 写入本 Drawable 的行，逐行清除并写入新内容；没有内容的行为清空
		for i := 0; i < newLen; i++ {
			sb.WriteString("\x1b[2K")
			sb.WriteString(lines[i])
			sb.WriteString("\n")
		}
		d.owner.frameLines += newLen
		// 如果前一帧比当前帧更高，需要清空多余的行以移除残留
		if prevHeight > d.owner.frameLines {
			rem := prevHeight - d.owner.frameLines
			for i := 0; i < rem; i++ {
				sb.WriteString("\x1b[2K\n")
			}
			// 清空多余行后，当前可见高度应为当前帧行数
			d.owner.lastHeight = d.owner.frameLines
		} else {
			d.owner.lastHeight = d.owner.frameLines
		}
		// 在整帧写出后清除光标下方所有残留内容，避免终端保留旧行
		sb.WriteString("\x1b[0J")
		// 一次性写出整帧内容
		_, _ = out.WriteString(sb.String())
		d.owner.mu.Unlock()
	}
	return nil
}
func (d *simpleDrawable) Clear() error {
	d.state.lines = d.state.lines[:0]
	d.state.contents = d.state.contents[:0]
	return nil
}
func (d *simpleDrawable) Width() *uint16 { return d.width }

// 构造函数
func NewStderrDrawTarget() ProgressDrawTarget {
	w := uint16(80)
	return &progressDrawTarget{hidden: false, stderr: true, width: &w, term: nil}
}
func NewHiddenDrawTarget() ProgressDrawTarget {
	return &progressDrawTarget{hidden: true, stderr: false, width: nil, term: nil}
}

// NewInMemoryDrawTarget 创建一个使用 InMemoryTerm 的 draw target（用于测试）。
func NewInMemoryDrawTarget(t *InMemoryTerm) ProgressDrawTarget {
	w := t.Width()
	return &progressDrawTarget{hidden: false, stderr: false, width: &w, term: t}
}
