# mpb

`quant1x/mpb` 把 Rust 进度条库 [indicatif](https://github.com/console-rs/indicatif) 作为**唯一规范**，
用五套语言实现同一套进度条语义。

| 角色 | 内容 |
|---|---|
| **规范** | Rust `indicatif 0.18.6`（commit `3adb05c`），任何行为以它为准 |
| **移植** | Go / Python / C++ / Zig 四个独立实现，逐模块对照上游 |
| **封装** | Rust 侧同名 `mpb` crate，只有 `pub use indicatif::*;`，不做二次实现 |

五个入口共享同一套能力：模板与 ANSI 样式、宽度与换行度量、20Hz 节流与令牌桶、15 秒双指数
平滑估算器、单/多进度条、树形嵌套、`println` / `clear` / `suspend`、`u64` 边界、
Drop/僵尸行（zombie）算法、内存终端。

## 上游与版本

| 项 | 值 |
|---|---|
| 上游仓库 | <https://github.com/console-rs/indicatif> |
| **上游版本** | **0.18.6** |
| **上游 commit** | **3adb05c80429415743d8142503c24fa8ee639453**（短号 `3adb05c`，2026-10-06） |
| 上游本地副本 | `<UPSTREAM_DIR>`，例如 `d:/projects/rust/indicatif` |

仓库**不随附**上游的 `.rs` / `examples/` / `tests/` / `Cargo.toml`，移植时直接读 `<UPSTREAM_DIR>`。
对比两个上游版本：

```bash
git -C <UPSTREAM_DIR> --no-pager diff <OLD_COMMIT> HEAD -- src/
```

`<OLD_COMMIT>` 取 `3adb05c` 的前一次提交，即可看到 0.18.6 这一批改动。

## 语言矩阵

| 语言 | 入口 | 构建 | 测试 | 示例 | 版本来源 |
|---|---|---|---|---|---|
| Go | `doc.go` | `go build ./...` | `go test ./...` | `go run ./examples <name>` | —（无版本清单） |
| Python | `__init__.py` | `pip install -e .` | `python -m pytest` | `python -m examples <name>` | `setuptools-scm` |
| C++ | `mpb.h` | `cmake` + `cmake --build` | `ctest --test-dir build` | `mpb_examples <name>` | `git describe`（`git_tag_info.cmake`） |
| Zig | `mpb.zig` | `zig build` | `zig build test` | `zig build run -- <name>` | `build.zig.zon` |
| Rust | `lib.rs` | `cargo build` | `cargo test` | `cargo run --example examples -- <name>` | `Cargo.toml` |

版本号由 [quant1x/auto-changelog](https://github.com/quant1x/auto-changelog) 在发版时统一写入
`Cargo.toml`、`build.zig.zon` 等清单文件，不手工维护。

## 目录结构

五种语言的源文件**同目录并排**（与 `mopr` 的组织方式一致）：

```text
mpb/
├── *.go                     go.mod
├── *.py                     pyproject.toml
├── *.h / *.cpp              CMakeLists.txt、git_tag_info.cmake
├── *.zig                    build.zig、build.zig.zon、unicode_width.zig
├── Cargo.toml               lib.rs
├── examples/
│   ├── main.{go,py,cpp,zig,rs}      每种语言一个入口，按名字分派
│   └── <name>.{go,py,cpp,zig,rs}    17 个示例，同名同语义
├── scripts/generate.{ps1,sh}        重录 screenshots/
└── screenshots/                     16 条 VHS 录屏
```

## 模块对照

| Rust（上游） | Go | Python | C++ | Zig | 内容 |
|---|---|---|---|---|---|
| `style.rs` | `style.go` | `style.py` | `style.h` / `style.cpp` | `style.zig` | 模板解析、ANSI 样式、宽度度量 |
| `state.rs` | `state.go` | `state.py` | `state.h` / `state.cpp` | `state.zig` | `BarState` / `ProgressState`、出帧、Estimator |
| `draw_target.rs` | `draw_target.go` | `draw_target.py` | `draw_target.h` / `draw_target.cpp` | `draw_target.zig` | 绘制目标、限速、行数调整 |
| `multi.rs` | `multi.go` | `multi.py` | `multi.h` / `multi.cpp` | `multi.zig` | `MultiProgress` / `MultiState`、orphan 行、僵尸行 |
| `progress_bar.rs` | `progress_bar.go` | `progress_bar.py` | `progress_bar.h` / `progress_bar.cpp` | `progress_bar.zig` | `ProgressBar` 门面 |
| `iter.rs` / `rayon.rs` | `iter.go` / `rayon.go` | `iter.py` | `iter.h` | —（未移植） | 迭代器包装 |
| `format.rs` | `format.go` | `format.py` | `format.h` / `format.cpp` | `format.zig` | `HumanBytes` / `HumanDuration` / `HumanCount` 等 |
| `in_memory.rs` / `term_like.rs` | `in_memory.go` / `term_like.go` | `in_memory.py` / `term_like.py` | `in_memory.h` / `in_memory.cpp`、`term_like.h` / `term_like.cpp` | `term_like.zig` | 内存终端与终端抽象 |
| `lib.rs` | `doc.go` | `__init__.py` | `mpb.h` | `mpb.zig` | 包入口与导出 |

说明：

- Rust 列即上游规范；Rust 侧的同名 `mpb` crate 直接 re-export，因此没有独立的模块文件。
- C++ 另有 `types.h`（共享枚举/时间类型）、`utf8.h`（UTF-8 编解码）、`unicode_width_tables.h`
  （由 Rust `unicode-width 0.2.2` 生成的宽度区间表）。
- Zig 另有 `width.zig` + `unicode_width.zig`：宽度算法与同一份 `unicode-width 0.2.2` 区间表。
- Rust 的 `Drop for BarState` 在四个移植里的对应实现：
  Go `weakref.go` + finalizer、Python `weakref` + `BarState.__del__`、
  C++ `std::weak_ptr` + 析构函数、Zig 引用计数 + `ProgressBar.deinit()`。

## 快速开始

### Go

```go
package main

import "github.com/quant1x/mpb"

func main() {
    pb := mpb.New(1024)
    for i := 0; i < 1024; i++ {
        pb.Inc(1)
    }
    pb.FinishWithMessage("done")
}
```

```bash
go build ./...
go test ./...
```

### Python

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

```bash
python -m pytest
```

### C++

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

非 git 源码树用 `-DMPB_VERSION_OVERRIDE=x.y.z` 显式指定版本；`-DMPB_BUILD_EXAMPLES=OFF`
可以只构建库。

### Zig

```zig
const mpb = @import("mpb");

var pb = mpb.ProgressBar.new(1024);
defer pb.deinit();
for (0..1024) |_| pb.inc(1);
pb.finishWithMessage("done");
```

```bash
zig build test      # 单元测试
zig build           # 构建 mpb_examples
```

### Rust

```toml
[dependencies]
# 发布后：mpb = "0.1"；仓库内开发时也可以直接用 path 依赖
mpb = { path = "../mpb" }
```

```rust
use mpb::{ProgressBar, ProgressStyle};

let pb = ProgressBar::new(1024);
pb.set_style(ProgressStyle::with_template("{wide_bar} {pos}/{len}").unwrap());
for _ in 0..1024 {
    pb.inc(1);
}
pb.finish_with_message("done");
```

```bash
cargo test
cargo build --example examples
```

`mpb` 没有自己的 feature：`in_memory` / `improved_unicode` / `futures` / `rayon` / `tokio`
全部默认打开。`mpb::ProgressBar` 就是 `indicatif::ProgressBar`，不是包装类型。

## 示例

17 个示例与上游 `examples/` 同名，统一入口按名字分派。名字使用上游的连字符写法
（`multi-tree`），五个入口同时接受下划线写法（`multi_tree`）。

| 示例 | 演示内容 | 录屏 |
|---|---|---|
| `single` | 单条进度条，1024 步 × 5ms | [gif](screenshots/single.gif) |
| `fastbar` | 百万次极快更新下的 20Hz 出帧与耗时统计 | [gif](screenshots/fastbar.gif) |
| `finebars` | 5 条不同粒度/颜色的条并发推进 | [gif](screenshots/finebars.gif) |
| `log` | `println` 打日志不破坏进度条画面 | [gif](screenshots/log.gif) |
| `morebars` | 一条总条 + 多条子条的嵌套推进 | [gif](screenshots/morebars.gif) |
| `multi` | 三条并发进度条 + `println` + `insert_after` | — |
| `multi-tree` | 树形嵌套：元素按缩进逐级加入 | [gif](screenshots/multi-tree.gif) |
| `multi-tree-ext` | 树形增删 + `--bottom-alignment` | [gif](screenshots/multi-tree-ext.gif) |
| `cargo` | 模拟 cargo 编译：4 个 worker 抢 crate 队列 | [gif](screenshots/cargo.gif) |
| `cargowrap` | 把 `cargo build --release` 的 stderr 喂给 spinner | [gif](screenshots/cargowrap.gif) |
| `download` | `{bytes}/{total_bytes}` + 自定义 eta key | [gif](screenshots/download.gif) |
| `download-continued` | 断点续传：非零起点 + `reset_eta()` | [gif](screenshots/download-continued.gif) |
| `download-speed` | 在 `download` 基础上加 `{bytes_per_sec}` | [gif](screenshots/download-speed.gif) |
| `iterator` | `progress()` / `progress_count()` / `progress_with()` | [gif](screenshots/iterator.gif) |
| `long-spinner` | steady tick + 自定义 spinner 帧 | [gif](screenshots/long-spinner.gif) |
| `tokio` | 定时器驱动：每 5ms 一个 tick | [gif](screenshots/tokio.gif) |
| `yarnish` | 四段式安装输出，最后用 `HumanDuration` 报耗时 | [gif](screenshots/yarnish.gif) |

![single](screenshots/single.gif)

五种语言的运行方式：

```bash
go run ./examples multi-tree
python -m examples multi-tree
./build/mpb_examples multi-tree          # MSVC 多配置生成器在 build/Release/ 下
zig build run -- multi-tree
cargo run --example examples -- multi-tree
```

带参数的示例（Zig/Cargo 需要 `--` 分隔自己的参数）：

```bash
go run ./examples multi-tree-ext --bottom-alignment
python -m examples multi-tree-ext --bottom-alignment
./build/mpb_examples multi-tree-ext --bottom-alignment
zig build run -- multi-tree-ext -- --bottom-alignment
cargo run --example examples -- multi-tree-ext -- --bottom-alignment
```

重新录制 `screenshots/`（脚本会自己切到仓库根目录，PowerShell 与 bash 行为一致）：

```powershell
./scripts/generate.ps1              # 全部 16 条
./scripts/generate.ps1 single log   # 只重录指定几条
```

```bash
./scripts/generate.sh               # 全部 16 条
./scripts/generate.sh single log    # 只重录指定几条
```

## 核心语义

### 模板 key

| 类别 | key |
|---|---|
| 进度条 | `bar`、`wide_bar`、`percent`、`percent_precise` |
| 位置/长度 | `pos`、`len`、`human_pos`、`human_len` |
| 字节 | `bytes`、`total_bytes`、`decimal_bytes`、`decimal_total_bytes`、`binary_bytes`、`binary_total_bytes` |
| 速率 | `per_sec`、`bytes_per_sec`、`decimal_bytes_per_sec`、`binary_bytes_per_sec` |
| 时间 | `elapsed`、`elapsed_precise`、`eta`、`eta_precise`、`duration`、`duration_precise` |
| 文本 | `msg`、`wide_msg`、`prefix`、`spinner` |
| 自定义 | `with_key(name, fn)`（Rust/Go/Python/C++/Zig 均支持） |

模板语法与上游一致：`{key:对齐宽度.样式/替代样式}`、`{key!}` 截断、`{{` / `}}` 转义。
`bar` 的示例：`{bar:40.cyan/blue}`、`{wide_bar:.red}`。

### 节流与估算

- 终端出帧默认 **20Hz**（`RateLimiter`，`MAX_BURST = 20`，允许短时突发）；
- 位置更新另有一层 **1ms 令牌桶**（`AtomicPosition`，`MAX_BURST = 10`），避免高频 `inc` 打爆状态；
- ETA/速率用 **15 秒半衰期**的双指数平滑估算器；
- 非 TTY（管道、重定向、CI）时整条进度条隐藏，与上游一致。

### 多进度条与僵尸行

`MultiState` 用 `members` + `free_set` + `ordering` 维护槽位与屏幕顺序：

- **orphan 行**：`println` 的消息先入队列，下一帧出现在所有进度条上方；
- **僵尸行**：`ProgressBar` 被释放后，它最后一次绘制的行保留到下一次重绘，然后回收；
- **对齐**：`Top`（默认）/ `Bottom`，删除中间条时决定整体上移还是下移；
- `clear` / `suspend` / `println` 都会正确擦除僵尸行。

### 索引的两套语义

| API | Rust | Go | 含义 |
|---|---|---|---|
| `Index()` | `index() -> Option<MultiStateIndex>` | `(uint, bool)` | **不透明槽位下标**，会被 `free_set` 复用，与屏幕顺序无关 |
| `VisualIndex()` | `visual_index() -> Option<usize>` | `(uint, bool)` | **屏幕位置**（第几行） |
| `Insert` / `InsertFromBack` | `insert(index, ..)` | `index uint` | 插入位置 |

需要“第几条 bar”时用 `VisualIndex()`，不要用 `Index()`。

## 语言层差异

### 运行时机制

| 维度 | Go | Python | C++ | Zig | Rust |
|---|---|---|---|---|---|
| 时间类型 | `time.Duration` | `float` 秒 | `std::chrono::nanoseconds` | `i96` 纳秒 | `std::time::Duration` |
| 迭代器 | `iter.go`（切片） | 任意可迭代对象 | `iter.h` | 未移植 | `ProgressIterator` |
| 宽度表 | `go-runewidth` | `unicodedata` + 手写规则 | Rust `unicode-width` 表 | Rust `unicode-width` 表 | `unicode-width` |
| 生命周期 | finalizer + weak registry | `weakref` + `__del__` | `weak_ptr` + 析构 | 引用计数 + `deinit()` | `Drop` |
| 跨平台时序 | 标准库 | 标准库 | 标准库 | 自带高精度 sleep / 控制台处理 | 标准库 + `console` |

### Python 与 Go 的差异

下面这些来自语言/运行时，不是 Rust 语义偏差：

- **错误处理**：Python 用异常（`TemplateError`、`ValueError`），Go 用 `(value, error)`；
  `MultiProgress.println()` / `clear()` 在 Python 返回 `None`。
- **迭代器**：Python 支持任意可迭代对象/生成器；Go 只包装切片。
- **格式化 API**：Go 的 `HumanBytes` 等是实现 `String`/`Format` 的类型，Python 是普通函数，输出逐字一致。
- **样式拷贝**：Python 的 `set_style()` / `style()` / `with_*` 按值克隆，避免浅拷贝共享。
- **整数边界**：位置/长度按 `u64` 语义处理；Python 在边界处显式取模/钳位，负数抛 `ValueError`。
- **锁**：Python 用 `RLock`，`suspend` 回调内可重入；Go 的 Mutex 非重入。
- **回收时机**：Go/Python 依赖运行时，不保证在确定时刻触发；语义与 Rust 的 Drop 一致。

### Zig 的平台处理

- **短休眠**：Windows 用 `CREATE_WAITABLE_TIMER_HIGH_RESOLUTION` 可等待定时器（约 0.5ms），
  绕开默认 15.6ms 粒度；macOS/POSIX 用 `nanosleep`；
- **自旋提示**：x86 `pause`、arm64 `isb`（与 Rust `core::hint::spin_loop` 一致）；
  不使用 arm64 `yield`，避免 Apple Silicon 的线程节流形成 convoy；
- **控制台**：Windows 通过 `WriteConsoleW` 写 UTF-16，并打开
  `ENABLE_VIRTUAL_TERMINAL_PROCESSING`，否则 `█░` / emoji 会按 OEM 代码页乱码、清行失效。

### 与上游的有意差异

- **宽度开关命名**：Rust `Width::set_default_cjk(bool)`；Go `DefaultWidth.SetDefaultCJK(bool)`；
  Python `mpb.DEFAULT_WIDTH.set_default_cjk(bool)`；C++ `mpb::Width::set_default_cjk(bool)`；
  Zig `mpb.setDefaultCjk(bool)`。
- **`MultiProgress` 状态导出**：Go 的 `State` 字段对应 Rust 0.18.6 的 `pub state`，
  访问前需自行加 `MultiState` 的锁。

## 已知限制

- **Zig**：`iter` / `rayon` 尚未移植；示例里的随机数用固定种子的 LCG 替代 `rand`，
  `cargowrap` 回放构建输出而不是递归构建，`tokio` 示例用等价的 5ms 循环；
  `InMemoryTerm` 是 VT100 子集（覆盖 mpb 自身产生的序列）。
- **Rust 示例**：同样用确定性 LCG 替代 `rand`；`cargowrap` 会真的执行
  `cargo build --release`（因此需要 cargo 且在 Rust 工程目录内运行）。
- **宽度表**：Python 用 `unicodedata`、Go 用 `go-runewidth`，与 Rust `unicode-width 0.2.2`
  在 ASCII/Latin/CJK/emoji/方块/盲文等常用区间一致；约 4k 个生僻/未分配码点
  （组合记号、Unicode 版本差异）可能有 0/1/2 列差异。C++ 与 Zig 直接使用 Rust 的表。
- **C++**：极少数阿拉伯/希伯来/印度文字连字仍按字符合成，未复刻 `unicode-width` 的完整连字状态机。

## 许可证

MIT。
