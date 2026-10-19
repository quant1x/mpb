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
    // The Rust example drives pb3 and the individual jobs from separate
    // threads; keep that concurrency so all three bars move together.
    var pb3_thread = try std.Thread.spawn(.{}, pb3Worker, .{ io, &mp, pb3.clone() });
    var jobs_thread = try std.Thread.spawn(.{}, jobsWorker, .{ io, pb2.clone() });

    var i: u64 = 0;
    while (i < n) : (i += 1) {
        u.sleepMs(io, 15);
        if (i == n / 3) u.sleepSecs(io, 2);
        pb.inc(1);
    }
    pb.finishWithMessage("all jobs started");

    pb3_thread.join();
    jobs_thread.join();
    mp.clear();
}

fn pb3Worker(io: std.Io, mp: *mpb.MultiProgress, handle: mpb.ProgressBar) void {
    var bar = handle;
    defer bar.deinit();

    var i: u64 = 0;
    while (i < 1024) : (i += 1) {
        u.sleepMs(io, 2);
        const message = std.fmt.allocPrint(u.allocator, "item #{d}", .{i + 1}) catch "";
        bar.setMessage(message);
        bar.inc(1);
    }
    mp.println("pb3 is done!");
    bar.finishWithMessage("done");
}

fn jobsWorker(io: std.Io, handle: mpb.ProgressBar) void {
    var bar = handle;
    defer bar.deinit();

    // The Rust version spawns one thread per job (each finishing after 1-5s);
    // spreading 200 increments over five seconds gives the same picture.
    var i: u64 = 0;
    while (i < 200) : (i += 1) {
        u.sleepMs(io, 25);
        bar.inc(1);
    }
    bar.finishWithMessage("all jobs done");
}
