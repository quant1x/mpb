// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const mpb = @import("mpb");

pub fn run(io: std.Io) !void {
    var pb = mpb.ProgressBar.initStderr(1024);
    for (0..1024) |_| {
        try std.Io.sleep(io, std.Io.Duration.fromMilliseconds(5), .awake);
        pb.inc(1);
    }
    pb.finishWithMessage("done");
}
