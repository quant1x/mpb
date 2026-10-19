// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

fn manyUnitsOfEasyWork(n: u64, label: []const u8, io: std.Io) void {
    var pb = mpb.ProgressBar.new(n);
    defer pb.deinit();

    var sum: u64 = 0;
    var i: u64 = 0;
    while (i < n) : (i += 1) {
        // Any quick computation, followed by an update to the progress bar.
        sum += 2 * i + 3;
        pb.inc(1);
    }
    pb.finish();

    u.printFmt(io, "[{s}] Sum ({d}) calculated in {s}", .{ label, sum, u.debugDuration(pb.elapsed()) });
}

pub fn run(io: std.Io) !void {
    const n: u64 = 1 << 20;
    manyUnitsOfEasyWork(n, "Default progress bar ", io);
}
