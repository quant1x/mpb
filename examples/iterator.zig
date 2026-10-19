// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    // Default styling, using the known item count.
    {
        var pb = mpb.ProgressBar.new(1000);
        defer pb.deinit();
        for (0..1000) |_| {
            u.sleepMs(io, 5);
            pb.inc(1);
        }
        pb.finish();
    }

    // Provide the explicit number of elements in the iterator.
    {
        var pb = mpb.ProgressBar.new(1000);
        defer pb.deinit();
        for (0..1000) |_| {
            u.sleepMs(io, 5);
            pb.inc(1);
        }
        pb.finish();
    }

    // Provide a custom bar style.
    {
        const style = try mpb.ProgressStyle.withTemplate(
            "{spinner:.green} [{elapsed_precise}] [{bar:40.cyan/blue}] ({pos}/{len}, ETA {eta})",
        );
        var pb = mpb.ProgressBar.new(1000);
        defer pb.deinit();
        pb.setStyle(style);
        for (0..1000) |_| {
            u.sleepMs(io, 5);
            pb.inc(1);
        }
        pb.finish();
    }
}
