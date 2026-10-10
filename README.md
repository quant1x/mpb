# mpb

`github.com/quant1x/mpb` 是 Rust 进度条库 [indicatif](https://github.com/console-rs/indicatif) 的 Go 移植，
按模块逐一对照实现（`style` / `state` / `draw_target` / `multi` / `progress_bar` / `iter` / `format` / `in_memory` / `term_like`），
目标是与 Rust 侧**逐语义对齐**：同样的模板语法、同样的宽度与换行度量、同样的僵尸行（zombie）算法。

## 来源

| 项 | 值 |
|---|---|
| 上游仓库 | <https://github.com/console-rs/indicatif> |
| **上游版本** | **0.18.6** |
| **上游 commit** | **3adb05c80429415743d8142503c24fa8ee639453**（短号 `3adb05c`，2026-10-06） |
| 上游本地副本 | `d:\projects\rust\indicatif` |

本仓库**不再随附** Rust 源码：上游的 `.rs`、`examples/`、`tests/`、`Cargo.toml` 等已全部移除，
改 Go 时直接看上游本地副本（`d:\projects\rust\indicatif`）。要对比两个版本的差异：

```powershell
git -C d:/projects/rust/indicatif --no-pager diff <旧commit> HEAD -- src/
```

## 模块对照

| Rust（上游） | Go（本仓库） | 内容 |
|---|---|---|
| `style.rs` | `style.go` | 模板解析（`{bar:40.red/blue}`）、样式（`on_*` 背景、256 色、真彩色）、宽度度量 |
| `state.rs` | `state.go` | `BarState` / `ProgressState`、绘制状态与出帧 |
| `draw_target.rs` | `draw_target.go` | 绘制目标（终端 / term-like / multi / hidden）、限速、行数调整 |
| `multi.rs` | `multi.go` | `MultiProgress` / `MultiState`、成员排序、orphan 行、僵尸行 |
| `progress_bar.rs` | `progress_bar.go` | `ProgressBar` 门面 |
| `iter.rs` / `rayon.rs` | `iter.go` / `rayon.go` | 迭代器包装 |
| `format.rs` | `format.go` | `HumanBytes` / `HumanDuration` / `HumanFloatCount` 等格式化 |
| `in_memory.rs` / `term_like.rs` | `in_memory.go` / `term_like.go` | 内存终端（测试用）与终端抽象 |
| `lib.rs` | `doc.go` | 包文档（`go doc` 可见） |

（左侧是上游文件名，本仓库只有右列的 Go 文件；上游源码见上面的本地副本路径。）

## 索引的两套语义（与 Rust 一致）

| API | Rust | Go | 含义 |
|---|---|---|---|
| `Index()` | `index() -> Option<MultiStateIndex>`（`usize`） | `(uint, bool)` | **不透明槽位下标**，会被 `free_set` 复用，与屏幕先后无关 |
| `VisualIndex()` | `visual_index() -> Option<usize>` | `(uint, bool)` | **屏幕位置**（第几行） |
| `Insert(idx, pb)` / `InsertFromBack(idx, pb)` | `insert(index: usize, ..)` | `index uint` | 插入位置 |

要"第几条 bar"用 `VisualIndex()`，不要用 `Index()`。

## 与上游的一处有意差异

- **宽度开关**：Rust 的 `Width::set_default_cjk(bool)` 在 Go 侧是 `DefaultWidth.SetDefaultCJK(bool)`
  （底层是 go-runewidth 的 `EastAsianWidth` 条件）。
- `MultiProgress` 的 `State` 字段已导出（对应 Rust 0.18.6 的 `pub state`）；访问前要先加
  `MultiState` 自带的 `RWMutex`（等价于 Rust 的 `read()` / `write()`）。

## 构建与验证

```powershell
go build ./...
go test ./...
go run ./examples -capture   # 渲染到内存终端，输出落在 examples/multi_capture.out
```
