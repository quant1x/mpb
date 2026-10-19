// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

pub fn run(io: std.Io) !void {
    var downloaded: u64 = 69369369;
    const total_size: u64 = 231231231;

    var style = try mpb.ProgressStyle.withTemplate(
        "{spinner:.green} [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})",
    );
    style.progressChars("#>-");

    var pb = mpb.ProgressBar.new(total_size);
    defer pb.deinit();
    pb.setStyle(style);
    pb.setPosition(downloaded);
    pb.resetEta();

    while (downloaded < total_size) {
        downloaded = @min(downloaded + 123211, total_size);
        pb.setPosition(downloaded);
        u.sleepMs(io, 12);
    }
    pb.finishWithMessage("downloaded");
}
