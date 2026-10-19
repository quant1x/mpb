// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var mp = mpb.MultiProgress.init();
    defer mp.deinit();

    const style = try mpb.ProgressStyle.withTemplate("{bar:40.green/yellow} {pos:>7}/{len:7}");

    var pb = mp.add(mpb.ProgressBar.new(5));
    defer pb.deinit();
    pb.setStyle(style);

    // Make sure we show up at all, otherwise no rendering event.
    pb.tick();

    var i: usize = 0;
    while (i < 5) : (i += 1) {
        var pb2 = mp.add(mpb.ProgressBar.new(128));
        defer pb2.deinit();
        pb2.setStyle(style);
        for (0..128) |_| {
            u.sleepMs(io, 5);
            pb2.inc(1);
        }
        pb2.finish();
        pb.inc(1);
    }
    pb.finishWithMessage("done");
}
