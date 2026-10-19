// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! mpb — a Zig port of Rust's `indicatif` 0.18.6.
//!
//! The modules mirror the Rust crate one-for-one:
//!
//! | Rust             | Zig                 |
//! |------------------|---------------------|
//! | `draw_target.rs` | `draw_target.zig`   |
//! | `multi.rs`       | `multi.zig`         |
//! | `progress_bar.rs`| `progress_bar.zig`  |
//! | `state.rs`       | `state.zig`         |
//! | `style.rs`       | `style.zig`         |
//! | `format.rs`      | `format.zig`        |
//! | `in_memory.rs`   | `term_like.zig`     |

const std = @import("std");

pub const util = @import("util.zig");
pub const width = @import("width.zig");
pub const format = @import("format.zig");
pub const term_like = @import("term_like.zig");
pub const draw_target = @import("draw_target.zig");
pub const multi = @import("multi.zig");
pub const state = @import("state.zig");
pub const style = @import("style.zig");
pub const progress_bar = @import("progress_bar.zig");

pub const ProgressBar = progress_bar.ProgressBar;
pub const MultiProgress = multi.MultiProgress;
pub const MultiProgressAlignment = multi.MultiProgressAlignment;
pub const ProgressDrawTarget = draw_target.ProgressDrawTarget;
pub const ProgressStyle = style.ProgressStyle;
pub const ProgressFinish = state.ProgressFinish;
pub const FinishKind = state.FinishKind;
pub const InMemoryTerm = term_like.InMemoryTerm;
pub const LineType = draw_target.LineType;
pub const DrawState = draw_target.DrawState;
pub const RateLimiter = draw_target.RateLimiter;

pub const charWidth = width.charWidth;
pub const strWidth = width.strWidth;
pub const ansiStrWidth = width.ansiStrWidth;
pub const setDefaultCjk = width.setDefaultCjk;

test {
    std.testing.refAllDecls(@This());
    _ = @import("width.zig");
    _ = @import("format.zig");
    _ = @import("term_like.zig");
    _ = @import("draw_target.zig");
    _ = @import("multi.zig");
    _ = @import("state.zig");
    _ = @import("style.zig");
    _ = @import("progress_bar.zig");
}
