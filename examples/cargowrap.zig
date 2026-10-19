// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Spinner + message updates while a build tool streams its output.
//!
//! The Rust example reads `cargo build --release` from a pipe. Spawning a
//! nested build from an example would recurse into this build, so the output
//! stream is replayed here; the progress-bar handling is identical.

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

const LINES = [_][]const u8{
    "   Compiling proc-macro2 v1.0.55",
    "   Compiling unicode-ident v1.0.8",
    "   Compiling quote v1.0.26",
    "   Compiling syn v2.0.10",
    "   Compiling libc v0.2.140",
    "   Compiling cfg-if v1.0.0",
    "   Compiling once_cell v1.17.1",
    "   Compiling console v0.15.5",
    "   Compiling indicatif v0.17.3",
    "    Finished release [optimized] target(s) in 12.34s",
};

pub fn run(io: std.Io) !void {
    const started = mpb.util.nowNs();
    u.println(io, "Compiling package in release mode...");

    var pb = mpb.ProgressBar.newSpinner();
    defer pb.deinit();

    pb.enableSteadyTick(200 * std.time.ns_per_ms);

    var style = try mpb.ProgressStyle.withTemplate("{spinner:.dim.bold} cargo: {wide_msg}");
    style.tickChars("/|\\- ");
    pb.setStyle(style);

    for (LINES) |line| {
        const stripped = std.mem.trim(u8, line, " \t\r\n");
        if (stripped.len > 0) pb.setMessage(stripped);
        pb.tick();
        u.sleepMs(io, 120);
    }

    pb.finishAndClear();
    u.printFmt(io, "Done in {s}", .{mpb.format.humanDuration(u.allocator, mpb.util.nowNs() - started, false)});
}
