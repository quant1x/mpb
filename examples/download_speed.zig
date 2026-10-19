// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var downloaded: u64 = 0;
    const total_size: u64 = 231231231;

    var style = try mpb.ProgressStyle.withTemplate(
        "{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({bytes_per_sec}, {eta})",
    );
    style.progressChars("#>-");

    var pb = mpb.ProgressBar.new(total_size);
    defer pb.deinit();
    pb.setStyle(style);

    while (downloaded < total_size) {
        downloaded = @min(downloaded + 223211, total_size);
        pb.setPosition(downloaded);
        u.sleepMs(io, 12);
    }
    pb.finishWithMessage("downloaded");
}
