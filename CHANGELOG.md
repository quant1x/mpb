# Changelog
All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.1.0] - 2026-10-10
### Changed
- 删除test目录的测试代码
- chore: remove debug instrumentation
- 暂存一个版本
- 暂存代码
- 暂存代码
- 优化多进度条
- refactor[go]：按 Rust 规范对齐 multi/style/draw_target 的渲染语义
  - multi：成员改用持久 draw_state，修掉「触发绘制的那条 bar 自己不显示」的问题；组帧顺序改为 extra_lines → orphan_lines → 各成员
  - multi：orphan 行按帧抽干并删掉画完回灌，修掉消息每帧重画、逐帧下移堆出上千行的问题；SafeBarState.Println 只传消息行
  - multi：set_draw_target 先 disconnect 旧目标；move_cursor 补包装层透传并在 drawToTerm 生效；Println/Suspend/Clear 不再吞 io 错误
  - multi：接上僵尸链（ProgressDrawTarget.MarkZombie + BarState 的 Drop 语义）；Remove 补归属校验并先置 hidden；internalize 不再持 multi 锁去动 bar 的绘制目标
  - style：模板解析器换成 Rust 同一套状态机，支持 {{/}} 转义、键名空白回退、! 截断位，解析错误以 TemplateError 返回
  - style：按 console 的 from_dotted_str token 表重写样式渲染，补齐 on_* 背景色、bright/on_bright、256 色与 #rrggbb 真彩色
  - style：ANSI 截断按 write_ansi_range 对齐（宽字符跨界整丢、ANSI 段原样写出）；wide 占位改为单个后者覆盖、不按行重置；alt_style 只作用于 bar/wide_bar
  - style：所有 key 统一走「补白截断 → 套样式」的收尾；删掉 Rust 没有的 human_pos_precise
  - draw_target：is_hidden 补 Rust 的 !term.is_term()，重定向到文件/管道时隐藏进度条
  - 清理：删除改完遗留的 drawWithExtraLines、抽干后废弃的 orphanSet、未使用的 hasAnyFmt
- chore：删除随仓库保存的 Rust 上游项目文件
  仓库此前把上游 indicatif 的源码与配置一并随仓库保存，作为对照用的 spec 副本。
  本次全部移除，上游源码只保留在 d:\projects\rust\indicatif，改 Go 时直接看那里。

  - 删除上游源码副本：根目录 11 个 *.rs、examples/*.rs（17 个）、tests/*.rs（3 个）
  - 删除 Rust 项目配置：Cargo.toml、Cargo.lock、rust-toolchain.toml
  - 删除上游随附文件：indicatif-LICENSE、indicatif-README.md、screenshots/*.gif
  - 删除从 src/lib.rs 翻译的 indicatif_doc.go（包文档改由 doc.go 承接）
  - 删除 Rust 时代的捕获产物 examples/multi_capture.escaped、multi_rs_capture.escaped
  - .gitignore 去掉 /target/（Rust 构建产物）规则
- refactor[go]：对齐 indicatif 0.18.6（3adb05c）的渲染、宽度与索引语义
  按上游 0.18.6（commit 3adb05c80429415743d8142503c24fa8ee639453）同步。

  - style：format_bar 拆两段上色，已填充段用 style、背景段用 alt_style.or(style)；
    bar/wide_bar 在收尾阶段不再被套第二层样式
  - style：新增 Width / DefaultWidth 与 SetDefaultCJK，宽度度量统一收口
    （底层是 go-runewidth 的 EastAsianWidth 条件）
  - style：换行度量 wrappedMetrics 改为逐字符推进，宽字符放不下就整挪下一行、末行留空列
    （原按总宽取整会少算一行）
  - style：修 ansiSegments 把字节当 rune 拼接导致多字节 UTF-8 被拆坏的问题
    （"国" 的 E5 9B BD 被拆成三个 latin1 字符），截断与换行度量随之修正
  - draw_target：draw_to_term 每行补到行宽由终端自行折行，去掉行间显式换行；
    新增 is_dumb()，哑终端判定按上游提前到目标构造期
  - format：HumanFloatCount 修正三处 —— 负号不参与千分位分组、舍入到零时去掉负号、
    无小数位时直接用舍入后的串
  - multi：新增 visualIndex；ProgressBar 新增 VisualIndex（屏幕位置），
    Index 仍是不透明槽位（会被 free_set 复用），两者不可混用
  - multi/progress_bar：索引类型统一为 uint（对应上游的 usize / MultiStateIndex）；
    MultiProgress.state 按上游 0.18.6 导出为 State
  - 新增 doc.go 作为包文档，接替上一笔删除的 indicatif_doc.go
  - 新增 README.md 记录来源版本与 commit；新增 license_header.toml
    （删除 Cargo.toml 后 mopr-license-header 需要的兜底配置）
  - 全部 Go 文件补版权头（Copyright (C) 2026 Quant1X / SPDX-License-Identifier: MIT）
- chore：LICENSE 改用 Quant1X 的 MIT
  与 license_header.toml 里的 spdx = "MIT" 保持一致，版权持有人写入
  Quant1X <wangfengxy@sina.cn>。原文件是未填持有人的 Apache-2.0 模板。
- chore：补 .editorconfig 与 .gitattributes
  - .editorconfig：统一换行（LF）、UTF-8 无 BOM、末尾空行等编辑器约定
  - .gitattributes：按语言固定 eol，并声明 go module 发布时的 export-ignore 范围
- release version 0.1.0

## [0.0.1] - 2026-01-13
### Changed
- Initial commit
- mpb 克隆自rust indicatif
- 调整多进度条demo
- 调整git仓库忽略文件类型
- 删除rust编译文件
- fix go module
- update changelog


[Unreleased]: https://gitee.com/quant1x/mpb/compare/v0.1.0...HEAD
[0.1.0]: https://gitee.com/quant1x/mpb/compare/v0.0.1...v0.1.0

[0.0.1]: https://gitee.com/quant1x/mpb/releases/tag/v0.0.1
