// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var pb = mpb.ProgressBar.new(100);
    defer pb.deinit();

    for (0..100) |i| {
        u.sleepMs(io, 25);
        const line = std.fmt.allocPrint(u.allocator, "[+] finished #{d}", .{i}) catch "";
        pb.println(line);
        pb.inc(1);
    }
    pb.finishWithMessage("done");
}
