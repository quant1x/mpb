// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// Package mpb 是 Rust 进度条库 indicatif 的 Go 移植。
//
// # 来源
//
// 上游仓库 github.com/console-rs/indicatif，对齐的版本是 0.18.6
// （commit 3adb05c80429415743d8142503c24fa8ee639453）。
// 移植的原则是**逐语义对齐**而不是逐语法翻译：同样的模板语法、同样的宽度与
// 换行度量、同样的僵尸行（zombie）算法、同样的绘制限速。Rust 的枚举与 trait
// 在 Go 里落成接口与结构体字段，但行为保持一致。
//
// # 模块
//
//	style.go        模板与样式（{bar:40.red/blue}、on_* 背景色、256 色、真彩色、宽度度量）
//	state.go        进度条状态、绘制状态与出帧
//	draw_target.go  绘制目标（终端 / term-like / multi / hidden）、限速、行数调整
//	multi.go        MultiProgress / MultiState、成员排序、orphan 行、僵尸行
//	progress_bar.go ProgressBar 门面
//	iter.go         迭代器包装（rayon.go 提供并行版）
//	format.go       HumanBytes / HumanDuration / HumanFloatCount 等格式化
//	in_memory.go    内存终端，测试用
//	term_like.go    终端抽象
//
// # 用法
//
// 单条进度条：
//
//	pb := mpb.New(100)
//	pb.SetStyle(mpb.DefaultProgressStyleBar())
//	for i := 0; i < 100; i++ {
//		pb.Inc(1)
//		time.Sleep(10 * time.Millisecond)
//	}
//	pb.Finish()
//
// 多条进度条：
//
//	mp := mpb.NewMultiProgress()
//	pb1 := mp.Add(mpb.New(100))
//	pb2 := mp.Add(mpb.New(200))
//
// # 两个容易踩的点
//
//  1. 绘制目标默认是 stderr。按上游语义，stderr 不是真终端（重定向到文件或
//     管道）时整条进度条会隐藏 —— 见 ProgressDrawTarget.IsHidden。测试里要拿
//     渲染结果请用 NewInMemoryTerm 配 NewInMemoryDrawTarget。
//  2. ProgressBar.Index 返回的是 MultiState 里**不透明**的槽位下标（会被复用），
//     要「屏幕上是第几行」请用 ProgressBar.VisualIndex。
package mpb
