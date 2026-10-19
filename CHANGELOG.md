# Changelog
All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.1.7] - 2026-10-19
### Changed
- feat[zig]：核心单条进度条、Rust 宽度表与构建入口
- fix：HumanFloatCount 的非有限值按 Rust 输出 inf/-inf
  Rust 的 {:.*} 对 f64 非有限值输出 inf / -inf / NaN；Go 的 %f 输出
  +Inf / -Inf，Python/C++ 之前也照抄成 +Inf。以 Rust 为锚点统一。
- feat[zig]：按 Rust 模块结构完成多进度条、完整模板 key 与 17 个示例
  - 源码按 Rust 一一拆分：draw_target/multi/state/style/format/term_like/progress_bar，
    不再以其他语言实现为参照。
  - MultiProgress：members/free_set/ordering、orphan 行、僵尸行回收、Top/Bottom 对齐、
    insert/insert_from_back/insert_before/insert_after/remove/println/clear/suspend。
  - 模板 key 全量：bar/wide_bar/spinner/msg/wide_msg/prefix/pos/len/percent/
    bytes/elapsed/eta/per_sec/bytes_per_sec/duration 及 _precise、human_*、decimal/binary 变体；
    新增 with_key 自定义 key 与 console 风格解析（256 色/真彩色/属性）。
  - 绘制：DrawState 视觉行数、宽字符换行、进度条只统计 bar 行高度、底部对齐补行、20Hz 节流。
  - 生命周期：ProgressBar 引用计数模拟 Arc<BarState>，最后一个 deinit() 执行 Drop 语义；
    MultiState 引用计数保证进度条晚于 MultiProgress 释放也安全。
  - 每帧格式化使用固定缓冲 scratch，行文本在 DrawState 重置时释放。
  - 17 个示例与 Rust examples/ 同名，统一由 examples/main.zig 按名称分派。
- fix[zig]：Windows 控制台改用宽字符写入并启用 VT 处理
  Windows 下 WriteFile 按控制台 OEM 代码页解释字节，UTF-8 的 █░ / ✔ / emoji
  会被拆成 GBK 乱码（鈻戔枒），宽度随之算错，清行与光标上移失效，画面不断
  堆叠。改为：

  - 终端句柄通过 WriteConsoleW 写 UTF-16（UTF-8 先转换，失败时回退到字节写入，
    兼容 mintty/MSYS pty）。
  - 初始化终端时打开 ENABLE_VIRTUAL_TERMINAL_PROCESSING，使 \x1b[2K / \x1b[nA
    真正生效。
  - 示例的 stdout 也走 StdTerm，避免中文/emoji 输出乱码。

  另加一条 MultiProgress 重绘回归测试：连续绘制后每帧只保留一份内容。
- perf[zig]：恢复示例并发与 Windows 1ms 定时精度
  刷新手感差的根因不在节流：

  1. Windows 默认定时器精度约 15.6ms，Io.sleep(3ms) 实际睡 16ms，
     yarnish 的依赖阶段从 4s 变成 19s。示例入口与 steady tick 线程现在
     调用 timeBeginPeriod(1)，退出/结束时还原（对应 Rust runtime 的行为）。
  2. 示例移植时把 Rust 的多线程改成了单线程顺序 sleep，各进度条互相等待：
     - yarnish：每个包条一个线程（4 个并发），28s -> 12s；
     - cargo：4 个 worker 线程共享 crate 队列，15s -> 5s；
     - multi：pb3 与作业条各自后台线程，与主线程的总条并发推进。

  节流本身（20Hz + MAX_BURST=20、位置 1ms 令牌桶）未改动，与 Rust 一致。
- perf[zig]：短休眠改用高精度可等待定时器（对齐 mopr）
  参考 mopr base/platform.cpp 的 sleep_for_microseconds 与 ringbuffer 的
  退避说明：

  - Windows：std.Io.sleep / Sleep 受默认 15.6ms 定时器粒度支配，50us 退避
    实际停 15.6ms，而 Rust 约 0.55ms（相差 28 倍）。改用
    CREATE_WAITABLE_TIMER_HIGH_RESOLUTION 可等待定时器（实测 ~0.53ms），
    句柄按次创建（约 14us）；失败回退到 Io.sleep。
  - macOS/POSIX：nanosleep 本身是微秒级，无需特殊处理。
  - 自旋提示与 Rust core::hint::spin_loop 一致：x86 pause、arm64 isb；
    不使用 arm64 yield（Apple Silicon 会激进节流自旋线程，形成 convoy）。
    Zig 的 std.atomic.spinLoopHint 已按此实现。
  - 示例与 steady tick 统一走 util.sleepMicros；timeBeginPeriod(1) 仅作为
    高精度定时器不可用时的兜底。
- release version 0.1.7

## [0.1.6] - 2026-10-19
### Changed
- refactor[go]：examples/multi 改为独立目录
- feat[python]：新增 Python 版本（模块与示例与 Go 对应）
- fix[go]：ForceDraw 多进度条重复渲染并补齐 italic 样式
- fix[python]：对齐 Go/Rust 的边界、样式与迭代器语义
- chore：纳入 .githooks 新标准钩子
- chore：设置 .githooks 脚本可执行位
- chore：统一 .githooks 许可证头为 MIT
- fix[go]：MultiState 成员改用弱引用，恢复 Drop/僵尸语义
- fix[python]：MultiState 成员改用弱引用，恢复 Drop/僵尸语义
- docs：说明 Python 版本已完成 Rust 语义对齐
- docs：模块对照补充 Python 列
- docs：模块对照去掉“本仓库”表述
- feat[cpp]：新增与 Rust/Go/Python 对齐的 C++ 实现
- feat[cpp]：补齐 map_parallel 并行迭代适配
- fix[cpp]：宽度表改用 Rust unicode-width 0.2.2
- docs[cpp]：注明宽度表来源与连字边界
- feat[cpp]：补齐 17 个示例（与 Go/Python 同目录）
- refactor[examples]：各语言扁平化为单一 main 入口，按名称分派
- docs：修正扁平化示例入口说明
- release version 0.1.6

## [0.1.5] - 2026-10-11
### Changed
- fix[go]: 清理未引用的依赖
- release version 0.1.5

## [0.1.4] - 2026-10-11
### Changed
- fix[docs]: 适配新版本的版权信息配置文件名
- chore：批量重录 GIF 的脚本收进 scripts/
  把"用 VHS 重录 screenshots 下所有 GIF"这件重复劳动固化成脚本，PowerShell 与 bash 各一份、
  行为一致（scripts/generate.ps1、scripts/generate.sh）：

  - 自己切到仓库根目录再跑：tape 里的 Output 相对 cwd 解析，所以从任何目录调用都对；
  - 跳过 _ 开头的探针 tape；
  - 逐条打印产物大小与耗时，末尾汇总条数 / 总用时 / GIF 合计大小；
  - 单条失败不中断，最后列出失败项并以退出码 1 结束，便于 CI 或人工发现漏录；
  - vhs 不在 PATH 里时用环境变量 VHS 指定路径。

  两个环境坑直接写进文件，免得下次再踩：

  - ps1 必须存成**带 BOM 的 UTF-8** —— Windows PowerShell 5.1 读无 BOM 的 UTF-8 会按 GBK
    解，中文注释会把脚本解析崩掉；
  - 参数用 $args 读，不声明 [string[]]$Name —— 数组参数的位置绑定会把第一个实参喂给
    后面的 -Vhs（实测踩到过）。

  generate.sh 置可执行位：本机 core.filemode=false，git 不读文件系统位，用
  git update-index --chmod=+x 写索引（100755）。

  README 里那段手工 `Get-ChildItem | ForEach-Object { vhs ... }` 换成脚本用法。
- docs：重录 16 个示例 GIF（压缩留白、按内容定高度）
  留白：原先是 VHS 的默认留白，实测上下各约 65px、左 62px，看着像很宽的边框。每条 tape
  加 Set Padding 6 后降到上下 6~20px、左右 6~8px。

  高度：之前一律 600px，多数示例下面空着十几行。先标定 FontSize 20 下的尺子（行距 25px、
  留白常量约 25px，Padding 6 时 Height ≈ 37 + 25 × 行数），再按每个示例的实际内容行数定
  高度，各留一行余量：

  - single / tokio / download / download-continued / download-speed / long-spinner：140
  - iterator 165、fastbar 190、finebars 215、morebars 265、yarnish 290
  - cargo、log（长流滚动）：340；cargowrap（会话式输出）：390
  - multi-tree-ext 365；multi-tree 540（结尾要显示完整的树对照）

  行数是像素级量出来的（把文字区压成一列灰度数亮带）；注意从 GIF 抽出的 PNG 是索引色，
  直接读"灰度"读的是调色板下标，全是假数，改为解码成 RGB 再算亮度。回量确认没有内容被
  切掉（cargo / log / cargowrap 是滚动流，占满属预期）。

  GIF 合计 10.7MB → 6.4MB。
- release version 0.1.4

## [0.1.3] - 2026-10-11
### Changed
- perf[go]：StdTerm 改为带缓冲，一帧只写一次终端
  用户反馈：进度条"视觉上很闪"，Rust 版丝滑。查下来刷新频率与上游完全一致
  （draw target 的 RateLimiter 是 20Hz：interval = 1000/20 = 50ms、MAX_BURST = 20；
  pos.allow(now) 的 INTERVAL = 1ms / MAX_BURST = 10 也与上游常量逐字相同；单条与
  MultiProgress 两条路径都过限流器。实测 300 万次 Inc(1) 只出 20 帧、单步 12ns），
  闪的根源在**写终端的方式**：

  - 上游 stdout() / stderr() 是 Term::buffered_stdout() / buffered_stderr()，
    console::Term 的 inner.buffer 为 Some(..)，write_str 只进缓冲，indicatif 在
    draw_to_term 末尾调一次 term.flush()（draw_target.rs:583）才落盘 —— 一帧 = 一次
    write(2)，终端每帧只重绘一次；
  - 本端口 StdTerm.WriteStr 是直写，drawToTerm 一帧里的光标上移、clear_line×N、
    下移×N、逐行内容、逐行补白共几十次 write 全是独立系统调用，终端在"清屏"与
    "重画"之间反复重绘，肉眼就是闪烁（5 条 bar 约 20 次/帧）。

  改动：

  - term_like.go：StdTerm 补上 inner.buffer 的等价物（buffered + mu + buf），
    WriteStr / writeEscape 进缓冲，Flush() 一次性 write 并清空（持锁写，与上游一致）；
    NewStdTerm / NewStderrTerm 改为带缓冲（对应 buffered_stdout / buffered_stderr），
    NewStdTermWithWriter 保持不缓冲并在注释里写明差别；draw_target.go 那条写 stderr
    的兜底路径改用带缓冲构造。drawToTerm 末尾本就调 Flush()，无需改动。
  - progress_bar.go：tick 改为接收 now（对应上游 tick_inner(now) 复用同一个 Instant），
    去掉每步多读一次时钟。

  测试（期望值取上游语义，不取本地跑出来的数）：

  - TestStdTermWritesOneBufferPerFrame：5 帧 → 5 次 write；同一帧不带缓冲则 >1 次，
    用来证明这条不是白测。
  - TestRateLimiterThrottlesToRefreshHz：1 秒内 1000 次调用只放行约 39 次
    （突发 20 + 每秒 20），把 20Hz 的口径钉住。
- release version 0.1.3

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


[Unreleased]: https://gitee.com/quant1x/mpb/compare/v0.1.7...HEAD
[0.1.7]: https://gitee.com/quant1x/mpb/compare/v0.1.6...v0.1.7
[0.1.6]: https://gitee.com/quant1x/mpb/compare/v0.1.5...v0.1.6
[0.1.5]: https://gitee.com/quant1x/mpb/compare/v0.1.4...v0.1.5
[0.1.4]: https://gitee.com/quant1x/mpb/compare/v0.1.3...v0.1.4
[0.1.3]: https://gitee.com/quant1x/mpb/compare/v0.1.2...v0.1.3
[0.1.2]: https://gitee.com/quant1x/mpb/compare/v0.1.1...v0.1.2
[0.1.1]: https://gitee.com/quant1x/mpb/compare/v0.1.0...v0.1.1
[0.1.0]: https://gitee.com/quant1x/mpb/compare/v0.0.1...v0.1.0

[0.0.1]: https://gitee.com/quant1x/mpb/releases/tag/v0.0.1
