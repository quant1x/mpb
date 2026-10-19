// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var mp = mpb.MultiProgress.init();
    defer mp.deinit();

    var style = try mpb.ProgressStyle.withTemplate(
        "[{elapsed_precise}] {bar:40.cyan/blue} {pos:>7}/{len:7} {msg}",
    );
    style.progressChars("##-");

    const n: u64 = 200;

    var pb = mp.add(mpb.ProgressBar.new(n));
    defer pb.deinit();
    pb.setStyle(style);
    pb.setMessage("todo");

    var pb2 = mp.add(mpb.ProgressBar.new(n));
    defer pb2.deinit();
    pb2.setStyle(style);
    pb2.setMessage("finished");

    var pb3 = mp.insertAfter(pb2, mpb.ProgressBar.new(1024));
    defer pb3.deinit();
    pb3.setStyle(style);

    mp.println("starting!");

    // The Rust example drives pb3 from its own thread; here the 1024 steps are
    // interleaved with the 200 job increments.
    var i: u64 = 0;
    while (i < 1024) : (i += 1) {
        u.sleepMs(io, 2);
        const message = std.fmt.allocPrint(u.allocator, "item #{d}", .{i + 1}) catch "";
        pb3.setMessage(message);
        pb3.inc(1);
    }
    mp.println("pb3 is done!");
    pb3.finishWithMessage("done");

    i = 0;
    while (i < n) : (i += 1) {
        u.sleepMs(io, 15);
        if (i == n / 3) u.sleepSecs(io, 2);
        pb.inc(1);
        if (i % 2 == 0) pb2.inc(1);
    }
    pb.finishWithMessage("all jobs started");

    pb2.finishWithMessage("all jobs done");
    mp.clear();
}
