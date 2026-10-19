// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    // Plain progress bar, totaling 1024 steps.
    const steps: u64 = 1024;
    var pb = mpb.ProgressBar.new(steps);
    defer pb.deinit();

    // The Rust example drives a tokio interval; the same 5ms cadence is
    // expressed here with a plain loop.
    var i: u64 = 0;
    while (i < steps) : (i += 1) {
        u.sleepMs(io, 5);
        pb.inc(1);
    }

    pb.finish();
}
