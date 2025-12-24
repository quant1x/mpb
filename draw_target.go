package mpb

import (
	"time"
)

// 简化版 draw_target 实现，替代 state.go 中的占位 simpleDrawTarget。
// 提供 ProgressDrawTarget 的基本实现以及 Drawable 对象，便于增量迁移。

type progressDrawTarget struct {
	hidden bool
	stderr bool
	width  *uint16
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
	ds := &DrawStateStub{lines: make([]LineType, 0)}
	return &simpleDrawable{state: ds, width: p.width}
}

// SetMoveCursor 为进度目标设置是否使用移动光标而不是清行（简化实现）。
func (p *progressDrawTarget) SetMoveCursor(move bool) { /* no-op */ }

type simpleDrawable struct {
	state *DrawStateStub
	width *uint16
}

func (d *simpleDrawable) State() *DrawStateStub { return d.state }
func (d *simpleDrawable) Draw() error           { return nil }
func (d *simpleDrawable) Clear() error {
	d.state.lines = append(d.state.lines, LineEmpty)
	return nil
}
func (d *simpleDrawable) Width() *uint16 { return d.width }

// 构造函数
func NewStderrDrawTarget() ProgressDrawTarget {
	w := uint16(80)
	return &progressDrawTarget{hidden: false, stderr: true, width: &w}
}
func NewHiddenDrawTarget() ProgressDrawTarget {
	return &progressDrawTarget{hidden: true, stderr: false, width: nil}
}
