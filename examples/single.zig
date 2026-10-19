// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var pb = mpb.ProgressBar.new(1024);
    defer pb.deinit();

    for (0..1024) |_| {
        u.sleepMs(io, 5);
        pb.inc(1);
    }
    pb.finishWithMessage("done");
}
