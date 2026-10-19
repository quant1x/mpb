# mpb

`github.com/quant1x/mpb` 是 Rust 进度条库 [indicatif](https://github.com/console-rs/indicatif) 的
Go / Python / C++ / Zig 四语言移植。四个实现都按模块逐一对照上游（`style` / `state` /
`draw_target` / `multi` / `progress_bar` / `iter` / `format` / `in_memory` / `term_like`），
目标是与 Rust 0.18.6 **逐语义对齐**：同样的模板语法、同样的宽度与换行度量、
同样的节流刷新、多进度条/树形嵌套、清除与僵尸行（zombie）算法。
Python、C++ 与 Zig 版本均已完成对齐；Python 代码与 Go 并排放在仓库根目录（组织方式同 `mopr`），
C++ 代码以 `.h` / `.cpp` 与它们同目录并排，Zig 代码为同目录的 `*.zig`，`pyproject.toml` / `CMakeLists.txt` / `build.zig` 也在根目录。

## 来源

| 项 | 值 |
|---|---|
| 上游仓库 | <https://github.com/console-rs/indicatif> |
| **上游版本** | **0.18.6** |
| **上游 commit** | **3adb05c80429415743d8142503c24fa8ee639453**（短号 `3adb05c`，2026-10-06） |
| 上游本地副本 | `<UPSTREAM_DIR>` |

本文档用尖括号表示需要自行替换的变量：

- `<UPSTREAM_DIR>` —— 本地克隆 indicatif 的目录，例如 `d:/projects/rust/indicatif`
- `<OLD_COMMIT>` —— 要对比的上游提交

源码树里**不再随附** Rust 源码：上游的 `.rs`、`examples/`、`tests/`、`Cargo.toml` 等已全部移除，
改 Go/Python 时直接看 `<UPSTREAM_DIR>`。要对比两个版本的差异：

```powershell
git -C <UPSTREAM_DIR> --no-pager diff <OLD_COMMIT> HEAD -- src/
```

已知同步点：`<OLD_COMMIT>` 取 `3adb05c` 之前的那次提交，即可看到 0.18.6 这一批改动。

## 模块对照

| Rust（上游） | Go | Python | C++ | Zig | 内容 |
|---|---|---|---|---|---|
| `style.rs` | `style.go` | `style.py` | `style.h` / `style.cpp` | `style.zig` | 模板解析（`{bar:40.red/blue}`）、样式（`on_*` 背景、256 色、真彩色）、宽度度量 |
| `state.rs` | `state.go` | `state.py` | `state.h` / `state.cpp` | `state.zig` | `BarState` / `ProgressState`、绘制状态与出帧、Estimator |
| `draw_target.rs` | `draw_target.go` | `draw_target.py` | `draw_target.h` / `draw_target.cpp` | `draw_target.zig` | 绘制目标（终端 / term-like / multi / hidden）、限速、行数调整 |
| `multi.rs` | `multi.go` | `multi.py` | `multi.h` / `multi.cpp` | `multi.zig` | `MultiProgress` / `MultiState`、成员排序、orphan 行、僵尸行 |
| `progress_bar.rs` | `progress_bar.go` | `progress_bar.py` | `progress_bar.h` / `progress_bar.cpp` | `progress_bar.zig` | `ProgressBar` 门面 |
| `iter.rs` / `rayon.rs` | `iter.go` / `rayon.go` | `iter.py` | `iter.h` | — | 迭代器包装（Zig 待移植） |
| `format.rs` | `format.go` | `format.py` | `format.h` / `format.cpp` | `format.zig` | `HumanBytes` / `HumanDuration` / `HumanFloatCount` 等格式化 |
| `in_memory.rs` / `term_like.rs` | `in_memory.go` / `term_like.go` | `in_memory.py` / `term_like.py` | `in_memory.h` / `in_memory.cpp`、`term_like.h` / `term_like.cpp` | `term_like.zig` | 内存终端（测试用）与终端抽象 |
| `lib.rs` | `doc.go` | `__init__.py` | `mpb.h` | `mpb.zig` | 包文档与入口 |

C++ 另有 `types.h`（共享枚举/时间类型）、`utf8.h`（UTF-8 编解码）、
`unicode_width_tables.h`（Unicode 宽度区间表）；构建入口是根目录 `CMakeLists.txt`，
版本号由 `git_tag_info.cmake` 从 git tag 获取（与 `mopr` 的做法一致）。

Rust 的 `Drop for BarState` 由 Go 的 `weakref.go`、Python 的 `WeakProgressBar` +
`BarState.__del__`、C++ 的 `std::weak_ptr` + `BarState` 析构函数、Zig 的引用计数 +
`ProgressBar.deinit()` 对应实现。

表中的 Go / Python / C++ 三列**同目录并排**存放：`style.go`、`style.py`、`style.h`、
`style.cpp` 是同级文件，`examples/<name>.go`、`<name>.py`、`<name>.cpp` 也是同级文件。
上游源码见上面的本地副本路径。

## 示例与效果

16 个录屏示例与上游 `examples/` 一一对应，动画用 [VHS](https://github.com/charmbracelet/vhs) 录制，
tape 文件与 GIF 同名，都在 `screenshots/`：

批量重录用 `scripts/` 下的两个脚本（PowerShell 与 bash 各一份，行为一致；脚本自己会切到
仓库根目录，所以从任何地方调用都行 —— tape 里的 `Output` 路径相对 cwd 解析）：

```powershell
./scripts/generate.ps1              # 全部 16 条
./scripts/generate.ps1 single log   # 只重录指定的几条（基名，不带扩展名）

$env:VHS = 'c:\runtime\work\bin\vhs.exe'   # vhs 不在 PATH 里时这么指定
./scripts/generate.ps1
```

```bash
./scripts/generate.sh              # 全部 16 条
./scripts/generate.sh single log   # 只重录指定的几条

VHS=/path/to/vhs ./scripts/generate.sh
```

两条脚本都会跳过 `_` 开头的探针 tape，逐条打印产物大小与耗时，失败时列出失败的 tape 并以
退出码 1 结束。

### `cargo`

模拟 cargo 编译时的输出（**并不真的调用 cargo**）：4 个 worker 并发从 crate 列表取任务，
主线程负责画进度条并打 `Compiling` 行。

```powershell
go run ./examples cargo
```

![cargo](screenshots/cargo.gif)

### `cargowrap`

把 `cargo build --release` 的 stderr **逐行喂给一个 spinner**（对应上游 `cargowrap.rs`）。
本机需装 cargo，且要在 Rust 工程目录里跑 —— 录屏里先编成 exe，再到 `%TEMP%` 现建一个带依赖的
空 crate 来跑（见 `screenshots/cargowrap.tape`）。

```powershell
go run ./examples cargowrap
```

![cargowrap](screenshots/cargowrap.gif)

### `download`

模拟下载进度，演示 `{bytes}/{total_bytes}` 与自定义 eta key（Rust 的 `with_key`）。

```powershell
go run ./examples download
```

![download](screenshots/download.gif)

### `download-continued`

断点续传：起始位置不为 0，并调用 `ResetEta()` 让 ETA 从当前位置重新估算。

```powershell
go run ./examples download-continued
```

![download-continued](screenshots/download-continued.gif)

### `download-speed`

在 `download` 的基础上多了 `{bytes_per_sec}`。

```powershell
go run ./examples download-speed
```

![download-speed](screenshots/download-speed.gif)

### `fastbar`

极大量、每次更新都极快的场景，用来看限速（默认 20Hz）下的出帧率与 `Elapsed()` 的耗时统计。

```powershell
go run ./examples fastbar
```

![fastbar](screenshots/fastbar.gif)

### `finebars`

同一个 `MultiProgress` 里 5 条不同粒度、不同颜色的条各自独立推进
（Rust 用 `thread::spawn`，Go 用 goroutine）。

```powershell
go run ./examples finebars
```

![finebars](screenshots/finebars.gif)

### `iterator`

三种迭代器包装：

1. `Progress(slice)` —— 用切片长度当总数（Rust 的 `size_hint`）
2. `ProgressCount(slice, n)` —— 显式指定元素个数
3. `ProgressWithSlice(slice, pb)` —— 用自定义样式的进度条

取完元素时迭代器会按样式里的完成行为收尾（默认 `FinishUsingStyle`）。

```powershell
go run ./examples iterator
```

![iterator](screenshots/iterator.gif)

### `log`

边推进边用 `Println` 打日志：日志行走 `MultiState` 的 orphan 行通道，不会把进度条画面冲乱。

```powershell
go run ./examples log
```

![log](screenshots/log.gif)

### `long-spinner`

steady tick + 自定义 spinner 帧（更多帧见
[cli-spinners](https://github.com/sindresorhus/cli-spinners/blob/master/spinners.json)）。

```powershell
go run ./examples long-spinner
```

![long-spinner](screenshots/long-spinner.gif)

### `morebars`

`MultiProgress` 里一条总进度条统着多条子进度条。

```powershell
go run ./examples morebars
```

![morebars](screenshots/morebars.gif)

### `multi-tree`

树形排列的进度条：元素按缩进逐级加入 `MultiProgress`，各自推进到满；整棵树填满后收尾，
最后把树按缩进打印出来做对照。

```powershell
go run ./examples multi-tree
```

![multi-tree](screenshots/multi-tree.gif)

### `multi-tree-ext`

`multi-tree` 的扩展版：除了「加进度条 / 推进进度条」，还能从树里摘掉进度条
（要摘的都是 `temp_*`），并演示 `MultiProgress` 的垂直对齐 —— 加 `--bottom-alignment` 改成底部对齐。

```powershell
go run ./examples multi-tree-ext              # 录屏用的是这个
go run ./examples multi-tree-ext --bottom-alignment
```

![multi-tree-ext](screenshots/multi-tree-ext.gif)

### `single`

最基础的单个进度条（1024 步 × 5ms，默认模板 `{wide_bar} {pos}/{len}`）。

```powershell
go run ./examples single
```

![single](screenshots/single.gif)

### `tokio`

定时器驱动进度条：Rust 用 tokio 的 `interval` + `block_on`，Go 用 `time.Ticker`，语义等价
（每 5ms 一个 tick，共 1024 步）。

```powershell
go run ./examples tokio
```

![tokio](screenshots/tokio.gif)

### `yarnish`

模拟 yarn 安装时的四段式输出（解析 → 拉取 → 链接 → 构建），最后用 `HumanDuration` 报总耗时。

```powershell
go run ./examples yarnish
```

![yarnish](screenshots/yarnish.gif)

## 索引的两套语义（与 Rust 一致）

| API | Rust | Go | 含义 |
|---|---|---|---|
| `Index()` | `index() -> Option<MultiStateIndex>`（`usize`） | `(uint, bool)` | **不透明槽位下标**，会被 `free_set` 复用，与屏幕先后无关 |
| `VisualIndex()` | `visual_index() -> Option<usize>` | `(uint, bool)` | **屏幕位置**（第几行） |
| `Insert(idx, pb)` / `InsertFromBack(idx, pb)` | `insert(index: usize, ..)` | `index uint` | 插入位置 |

要"第几条 bar"用 `VisualIndex()`，不要用 `Index()`。

## 与上游的有意差异

- **宽度开关**：Rust 的 `Width::set_default_cjk(bool)` 在 Go 侧是
  `DefaultWidth.SetDefaultCJK(bool)`，Python 是 `mpb.DEFAULT_WIDTH.set_default_cjk(bool)`，
  C++ 是 `mpb::Width::set_default_cjk(bool)`。
- `MultiProgress` 的 `State` 字段已导出（对应 Rust 0.18.6 的 `pub state`）；访问前要先加
  `MultiState` 自带的 `RWMutex`（等价于 Rust 的 `read()` / `write()`）。

## Go 构建与验证

```powershell
go build ./...
go test ./...
go run ./examples multi -capture   # 渲染到内存终端，输出落在 examples/multi_capture.out
```

## C++ 版本

C++ 实现与 Go/Python **同目录并排**：`style.h` / `style.cpp` 与 `style.go` / `style.py`
同级，入口头文件是 `mpb.h`，构建入口是根目录 `CMakeLists.txt`。运行时不依赖第三方库，
要求 C++20；版本号由 `git_tag_info.cmake` 从 git tag 读取（与 `mopr` 的做法一致）。

**C++ 版本已实现与 Rust `indicatif 0.18.6` 的语义对齐**，覆盖模板/样式/ANSI、宽度与换行
度量、20Hz 节流与令牌桶、估算器、单/多进度条、树形嵌套、`println` / `clear` / `suspend`、
`u64` 边界、Drop/僵尸行与内存终端。宽度表由 Rust `unicode-width` 0.2.2（indicatif 使用的
同一版本）生成，包含 `width` / `width_cjk` 以及 emoji ZWJ、VS16、keycap 等序列规则；
极少数阿拉伯/希伯来/印度文字连字仍按字符合成（未复刻该 crate 的完整连字状态机）。

```cpp
#include "mpb.h"

int main() {
    auto pb = mpb::ProgressBar::make(1024);
    pb->set_style(mpb::progress_style_with_template("{wide_bar} {pos}/{len}"));
    for (int i = 0; i < 1024; ++i) {
        pb->inc();
    }
    pb->finish_with_message("done");
}
```

构建与测试：

```bash
# Linux / macOS（GCC / Clang）
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -j
ctest --test-dir build --output-on-failure
```

```powershell
# Windows（MSVC）
cmake -S . -B build-msvc -G "Visual Studio 17 2022" -A x64
cmake --build build-msvc --config Release -j
ctest --test-dir build-msvc -C Release --output-on-failure
```

非 git 源码树可用 `-DMPB_VERSION_OVERRIDE=0.1.5` 显式指定版本号。

17 个示例都同时有 `examples/<name>.go`、`<name>.py`、`<name>.cpp`。所有 `examples/*.cpp`
编译进一个 `mpb_examples` 入口（参数是示例名），例如：

```bash
cmake --build build --target mpb_examples
./build/mpb_examples single
./build/mpb_examples multi --capture   # 输出 examples/multi_capture.out
```

不需要示例时用 `-DMPB_BUILD_EXAMPLES=OFF`。

本地验证：GCC 15（MinGW）、Clang 23、MSVC 19.44 均构建通过，`mpb_tests` 全部通过
（格式、模板/ANSI、宽条、MultiProgress、Drop/僵尸、迭代器、节流器、内存终端）；17 个
C++ 示例也在三套编译器下构建通过。

C++ API 使用 `snake_case` 方法（`inc` / `set_style` / `finish_with_message`），
时间参数与返回值使用 `std::chrono::nanoseconds`，Bar 共享状态用 `std::shared_ptr`，
`MultiState` 成员用 `std::weak_ptr` 恢复 Rust 的 Drop/僵尸语义。


## Zig 版本

Zig 实现使用 **Zig 0.17.0**，模块划分直接对应 Rust 源码（不是按其他语言版本改写）：

| Rust 源文件 | Zig 文件 | 内容 |
|---|---|---|
| `draw_target.rs` | `draw_target.zig` | `LineType`、`DrawState`、换行视觉行数、`RateLimiter`、`ProgressDrawTarget`、`Drawable` |
| `multi.rs` | `multi.zig` | `MultiState`（槽位/`free_set`/`ordering`/orphan 行/僵尸行/对齐）、`MultiProgress` |
| `progress_bar.rs` | `progress_bar.zig` | `ProgressBar` API、steady tick 线程、绘制目标切换 |
| `state.rs` | `state.zig` | `AtomicPosition`、`Estimator`、`ProgressState`、`BarState`、`Status` |
| `style.rs` | `style.zig` | 模板状态机、ANSI 样式解析、`format_state`、`format_bar`、补齐/截断 |
| `format.rs` | `format.zig` | `HumanBytes` / `DecimalBytes` / `BinaryBytes` / `HumanDuration` / `HumanCount` |
| `in_memory.rs` + `term_like.rs` | `term_like.zig` | `TermLike` 接口、真实终端 `StdTerm`、`InMemoryTerm` |
| `unicode-width` crate | `width.zig` + `unicode_width.zig` | 宽度算法与由 `unicode-width 0.2.2` 生成的区间表 |
| `lib.rs` | `mpb.zig` | 统一导出 |

已覆盖：模板全部 key（`bar`/`wide_bar`/`spinner`/`msg`/`wide_msg`/`prefix`/`pos`/`len`/
`percent`/`bytes`/`elapsed`/`eta`/`per_sec`/`bytes_per_sec`/`duration` 及 `_precise` 变体）、
`with_key` 自定义 key、20Hz 节流与 1ms 位置令牌桶、15 秒双指数平滑估算器、
`MultiProgress` 的插入/删除/对齐/`println`/`clear`/`suspend`、orphan 行与僵尸行回收、
`finish` / `finish_with_message` / `finish_and_clear` / `abandon` 等结束语义、
真实终端（Windows `GET_SCREEN_BUFFER_INFO` / POSIX `TIOCGWINSZ`）与内存终端。

Zig 没有隐式析构，`ProgressBar` 用引用计数模拟 `Arc<BarState>`：最后一个 `deinit()`
执行 Rust `Drop for BarState` 的收尾（`finish_using_style` + `mark_zombie`）；
`MultiState` 同样按引用计数在最后一个引用释放时销毁，因此进度条可以在
`MultiProgress` 之后释放而不会悬空。

```bash
zig build test                      # 单元测试（含内存终端渲染、僵尸回收）
zig build                           # 构建 mpb_examples
zig build run -- single             # 17 个示例，参数为 Rust examples/ 中的名字
zig build run -- multi_tree_ext -- --bottom-alignment
```

17 个示例与 Rust `examples/` 同名：`single`、`fastbar`、`finebars`、`log`、`morebars`、
`multi`、`multi_tree`、`multi_tree_ext`、`cargo`、`cargowrap`、`download`、
`download_continued`、`download_speed`、`iterator`、`long_spinner`、`tokio`、`yarnish`。
Rust 示例中的 `rand` 用确定性 LCG 替代；线程调度改为单线程交错（`finebars`、`multi`、
`yarnish` 等），进度条行为不变。`cargowrap` 不能在示例里递归构建，改为回放构建输出流；
`tokio` 用等价的 5ms 循环替代 async runtime。


## Python 版本

Python 实现与 Go/C++ 实现**同目录并排**存放（与 `mopr` 的组织方式一致）：仓库根目录的
`*.py` 与同名 `*.go` / `*.h` / `*.cpp` 一一对应，`pyproject.toml` 也在根目录，包名即 `mpb`。
无第三方运行时依赖（Python ≥ 3.10）。

**Python 版本已实现与 Rust `indicatif 0.18.6`（commit `3adb05c`）的语义对齐**，覆盖
模板/样式/ANSI、宽度与换行度量、20Hz 节流与令牌桶、估算器、单/多进度条、树形嵌套、
`println` / `clear` / `suspend`、`u64` 边界、Drop/僵尸行与内存终端。Go 与 Python 使用
同一组输入逐项比对：53 个模板解析、36,928 个 `format_bar` 组合、16 个渲染场景、
31 个 ANSI 样式用例以及 MultiProgress 场景，结果一致。

模块对应关系见上文「模块对照」。

安装与使用（在仓库根目录执行）：

```bash
pip install -e .
```

```python
from mpb import ProgressBar, ProgressStyle

pb = ProgressBar(1024)
pb.set_style(ProgressStyle("{wide_bar} {pos}/{len}"))
for _ in range(1024):
    pb.inc(1)
pb.finish_with_message("done")
```

多条进度条：

```python
from mpb import MultiProgress, ProgressBar

mp = MultiProgress()
one = mp.add(ProgressBar(100))
two = mp.add(ProgressBar(200))
mp.println("start")
```

公开 API 使用 Python 的 `snake_case`（`inc` / `set_style` / `finish_with_message`），
渲染与状态语义与 Rust/Go 一致：同样的模板语法、限速、僵尸行算法与终端宽度度量。
默认绘制目标仍是 stderr，重定向到文件/管道时整条进度条隐藏；要在测试里拿渲染结果请用
`InMemoryTerm` + `in_memory_draw_target`。

### Python 与 Go 的语言层差异

下面这些差异来自语言/运行时，**不是 Rust 语义偏差**；Rust 的模板、限速、多进度条、
清除与 Drop/僵尸行语义均已完成对齐：

- **时间单位**：Python 的 `eta()` / `elapsed()` / `duration()` / `set_elapsed()` /
  `enable_steady_tick()` / `human_duration()` / `formatted_duration()` 使用秒（`float`），
  Go 使用 `time.Duration`；数值等价。
- **错误处理**：Python 用异常（`TemplateError`、`ValueError`），Go 用 `(value, error)`；
  `MultiProgress.println()` / `clear()` 在 Python 返回 `None`。
- **迭代器**：Python 的 `progress()` 支持任意可迭代对象/生成器（未知长度时 bar 无总长），
  `next_back()` 需要可索引序列；Go 只包装切片。`map_parallel()` 用线程池保持输入顺序
  （GIL 下 CPU 密集不会真正并行，I/O 密集可用）。
- **格式化 API**：Go 的 `HumanBytes` / `HumanDuration` / `HumanFloatCount` 等是实现了
  `String`/`Format` 的类型，Python 是对应函数；输出逐字一致。
- **样式拷贝**：Python 的 `set_style()` / `style()` / `with_*` 按值语义克隆 `ProgressStyle`
  （template、tracker map 一并复制），避免 Go 结构体浅拷贝导致的共享；渲染结果一致。
- **整数边界**：位置/长度按 Rust/Go 的 `u64` 语义处理（位置回绕、长度与增量饱和）；
  Python 内部是任意精度整数，因此在边界处显式取模/钳位，负数位置/增量直接抛 `ValueError`
  （Go 由 `uint64` 类型在编译期拒绝）。
- **宽度表**：Python 用 `unicodedata` + 手写规则，Rust 用 `unicode-width` 0.2.2，
  Go 用 `go-runewidth`，C++ 直接由 Rust `unicode-width` 0.2.2 生成
  （`width` / `width_cjk` + emoji 序列规则）。Python/Go 与 Rust 在
  ASCII/Latin/CJK/emoji/方块/盲文等常用区间一致；约 4k 个生僻/未分配码点
  （主要是组合记号与 Unicode 版本差异）可能有 0/1/2 列差异。
- **内存终端**：Python 的 `InMemoryTerm` 是 VT100 子集（CSI A/B/C/D/H/f/K/J、SGR 忽略），
  Go 用 `go-vte`；mpb 自身产生的序列两边一致。
- **标准终端**：Python `StdTerm` 接收文本流（`str`），Go `io.Writer` 接收字节；
  重定向/非 TTY 时两者都隐藏进度条。
- **锁**：Python 使用 `RLock`，`suspend` 回调内重入同一 bar/multi 不会死锁；Go/Rust 的
  Mutex 非重入，这是 Python 侧更宽松的行为。
- **回收机制**：两版 `MultiStateMember` 都只保存 `BarState` 的弱引用，最后一个
  `ProgressBar` 被回收时按 `on_finish` 收尾并 `mark_zombie`（与 Rust 的 Drop 一致）。
  Go 用 `runtime.SetFinalizer` + weak registry，Python 用 `weakref` + `BarState.__del__`；
  具体回收时机由运行时决定，不保证在确定时刻发生。

Python 示例从包入口 `examples/__main__.py` 调用：使用 `python -m examples <name>`，
不需要直接执行 `examples/main.py`（它只是内部 dispatcher）。

测试与示例（Python 测试沿用 `*_test.py` 命名，与 mopr 一致，用 pytest 收集）：

```bash
python -m pytest                       # 65 个测试
python -m examples single         # 17 个示例，与 Go/C++ 示例一一对应
python -m examples multi-tree
```

每个示例在 `examples/` 下都有 `<name>.go`、`<name>.py` 与 `<name>.cpp`，例如
`cargo`、`cargowrap`、`download`、`download-continued`、`download-speed`、
`fastbar`、`finebars`、`iterator`、`log`、`long-spinner`、`morebars`、`multi`、
`multi-tree`、`multi-tree-ext`、`single`、`tokio`、`yarnish`。

宽度口径与上游一致：`█`、`░` 等歧义宽度字符按 1 列，调用
`mpb.DEFAULT_WIDTH.set_default_cjk(True)` 后按 2 列。版本号由 `setuptools-scm`
写入 `_version.py`（源码直接运行时回退到 `0.1.5`）。

