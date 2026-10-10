# Changelog
All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.1.2] - 2026-10-11
### Changed
- fix[go]：Drawable::state() 每帧先 reset，修进度条逐帧往下换行
  对应 Rust draw_target.rs：

      let mut state = match self { Drawable::Term { draw_state, .. } => ... };
      state.reset();
      state

  Term / TermLike 两个变体的 draw_state 挂在 TargetKind 上，是跨帧持久的。
  不 reset 时 format_state 每帧往同一份 lines 里 append，行数一帧帧涨
  （第 1 帧 1 行、第 2 帧 2 行……），进度条就一路往下换行，real_height
  跟着涨又让 MoveCursorUp 越移越多。
- fix[go]：终端宽度每帧现问，跟随窗口缩放
  对应 Rust console::Term::size_checked() -> terminal_size(self)：

      pub fn size(&self) -> (u16, u16) { self.size_checked().unwrap_or((24, DEFAULT_WIDTH)) }

  之前 StdTerm 在构造时 GetSize 量一次存进 width/height，Width() 直接返回常量，
  窗口缩放、以及「启动时控制台还是默认尺寸、随后被宿主改成实际尺寸」（ConPTY
  与 IDE 终端面板的典型开局）都感知不到：宽度偏大则整行溢出折行，偏小则只画一截。

  新增 sizeChecked()：每次现问，失败才回落到构造时的值。width/height 现在只是
  回落值，不再是当前宽度。
- fix[go]：宽度口径不再沿用 go-runewidth 全局，修 {wide_bar} 只画一半
  formatBar 里 clusters = width / char_width 这行与上游逐字等价，但上游那个
  char_width 背后是显式默认、我们背后是隐式探测，语义不同：

  - 上游 Width::new() 是 is_cjk: AtomicBool::new(false)，UnicodeWidthChar::width
    把 █(U+2588)、░(U+2591) 这类 East Asian Ambiguous 按 1 列算（其单测断言
    默认 WIDTH.char('█') == 1），与进程所在代码页无关；
  - go-runewidth 会在 init() 里按控制台代码页（CP936/932/949/950）把全局
    DefaultCondition 切成 CJK 口径，█ 变成 2 列。

  于是 165 列的终端里 char_width=2，条只画 155/2≈77 个块字符（视觉上一半宽度），
  而 drawToTerm 的补白按同一份偏大口径算，光标被顶到整行最右侧。

  改为本包自持两套 Condition（narrow / CJK），默认 narrow 不看环境；
  SetDefaultCJK 只切本包开关，不再改 go-runewidth 的全局状态。
  measureTextWidth 用同一口径。附回归测试：模拟 CJK 进程环境，断言 165 列下条
  必须是 155 个块字符（旧实现下该用例会红）。
- fix[go]：measureTextWidth 与上游语义对齐（不做 NFKC、不展开制表符、不删 ZWJ）
  上游 console::measure_text_width 与 indicatif Width::ansi_str 的定义是
  「跳过 ANSI 段（AnsiCodeIterator），其余各段按 unicode-width 累加」，仅此而已
  （style.rs 用它算 {wide_bar} 该让出多少列、以及行末补白几格）。

  之前的移植额外做了三件上游没有的事：

  1. NFKC 归一化：把全角/兼容字符改宽窄（１(U+FF11) 2 列 -> 1 列、ﬁ -> fi），
     于是含全角文本时 {wide_bar} 的让位与补白都比 Rust 少；
  2. 制表符按 4 空格展开：上游由 TabExpandedString 在展开期按 tab_width(=8)
     换空格，走到宽度函数的原始 \t 是控制字符，宽 0；
  3. 删除 ZWJ/ZWNJ：上游交给 unicode-width 的字形簇逻辑，自己删会把组合
     emoji 拆成若干个 2 列，反而偏大。

  改为直接复用 ansiSegments（本包的 AnsiCodeIterator 等价物）逐段累加。
  测试期望值取上游 style.rs 单测的断言（ansi_str 带 ANSI 的 █ == 1、str(あ) == 2），
  并补一条效果用例：40 列终端里全角消息按 2 列让位，条为 37 个块字符。
- fix[go]：ansiSegments 照搬上游手写 DFA，不再用任意字母判 ANSI 结束
  上游 console::ansi 用一条手写 DFA（与 strip_ansi_re 等价）识别 ANSI 序列：

      [\\x1b\\x9b]([()][012AB]|[\\[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-PRZcf-nqry=><])

  之前的移植是「遇到 ESC 起，碰到任意字母 A-Za-z 结束」，两处与上游不同：
  1. 结束符是固定集合 [0-9A-PRZcf-nqry=><]（无 Q、S~Y、a、b、d、e、o、p、s~x、z），
     不是任意字母；
  2. 起始符除 ESC 外还有 U+009B；且 ] 不在前缀类里，所以 OSC（\\x1b]8;;url\\x07）
     在上游眼里是可见文本，不是 ANSI 序列。

  此外上游的匹配是贪心的：推进到 Trap 或串尾，取最后一个曾为 final 的位置作结尾。

  照搬完整状态机（Start/S1..S11/Trap 与 transition 表），扫描按 rune（起始符含
  U+009B）而切片按字节下标。测试覆盖 SGR、真彩色 SGR、字符集指定、U+009B 起始、
  OSC 不识别、结束符不在集合内等分支。
- docs：README 列出 16 个示例及其效果动画
  用 VHS 逐个录制 examples 下的 16 个示例，tape 与 GIF 同名放在 screenshots/：

  - tape 沿用原有 cargo.tape 的风格：1200x600、FontSize 20，跑
    go run github.com/quant1x/mpb/examples/示例名；Sleep 按各示例的实际步数×间隔定
    （single 8s、download 系列 16s、multi-tree 系列 25s 等），可能跑不完的
    （cargo、multi-tree、multi-tree-ext）末尾补 Ctrl+C。
  - cargowrap 特殊：它跑 cargo build --release，必须在 Rust 工程里，而 go run 会把
    cwd 锁在 Go 模块内 —— 先编成 exe，再到临时目录现建一个带 serde 依赖的空 crate
    （--offline，本地 registry 已缓存）来跑，这样编译有 6 秒、spinner 的帧才看得清；
    否则空 crate 1 秒编完，帧一闪而过。
  - README 新增「示例与效果」一节：每个示例一段（说明取自各示例 main.go 的头注释 +
    运行命令 + GIF），并给出重录命令（vhs 一次只吃一个 tape，所以要循环）。

  自检：README 引用的 GIF、实际 GIF、examples 目录三者都是 16，一一对应、无缺失无多余。
- feat：16 个示例程序纳入版本管理
  与上游 examples/ 一一对应：cargo / cargowrap / download / download-continued /
  download-speed / fastbar / finebars / iterator / log / long-spinner / morebars /
  multi-tree / multi-tree-ext / single / tokio / yarnish。每个 main.go 的头注释都
  写明它对应上游哪个 .rs，以及演示点。

  此前这些目录一直未跟踪，而 README 的「示例与效果」一节与 screenshots/ 里的录屏
  都按 go run ./examples/示例名 引用它们 —— 对新克隆不成立，故补齐。

  本次只含 16 个 main.go，无构建产物。
- release version 0.1.2

## [0.1.1] - 2026-10-10
### Changed
- docs：来源说明改用变量写法描述本地路径
  - README：上游本地副本改用 `<UPSTREAM_DIR>`，对比命令里的 `<旧commit>` 统一为 `<OLD_COMMIT>`
  - 新增变量说明段：尖括号表示需自行替换的变量；具体路径只作为「例如」出现一次，
    正文与命令里一律用变量
  - 补一句已知同步点：`<OLD_COMMIT>` 取 `3adb05c` 之前那次提交，即可看到 0.18.6 这批改动
  - CHANGELOG 里的同名路径是提交 31218eb 的提交信息原文，属于历史记录，保持不动
- release version 0.1.1

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


[Unreleased]: https://gitee.com/quant1x/mpb/compare/v0.1.2...HEAD
[0.1.2]: https://gitee.com/quant1x/mpb/compare/v0.1.1...v0.1.2
[0.1.1]: https://gitee.com/quant1x/mpb/compare/v0.1.0...v0.1.1
[0.1.0]: https://gitee.com/quant1x/mpb/compare/v0.0.1...v0.1.0

[0.0.1]: https://gitee.com/quant1x/mpb/releases/tag/v0.0.1
