// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var pb = mpb.ProgressBar.newSpinner();
    defer pb.deinit();

    pb.enableSteadyTick(120 * std.time.ns_per_ms);
    var style = try mpb.ProgressStyle.withTemplate("{spinner:.blue} {msg}");
    style.tickStrings(&.{
        "▹▹▹▹▹",
        "▸▹▹▹▹",
        "▹▸▹▹▹",
        "▹▹▸▹▹",
        "▹▹▹▸▹",
        "▹▹▹▹▸",
        "▪▪▪▪▪",
    });
    pb.setStyle(style);
    pb.setMessage("Calculating...");

    u.sleepSecs(io, 5);
    pb.finishWithMessage("Done");
}
