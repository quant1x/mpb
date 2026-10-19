// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

/// The Rust example overrides the built-in `eta` key with `{:.1}s`.
fn writeEtaSeconds(state: *const mpb.state.ProgressState, out: *std.ArrayList(u8)) void {
    const secs = @as(f64, @floatFromInt(state.eta())) / @as(f64, @floatFromInt(std.time.ns_per_s));
    out.print(u.allocator, "{d:.1}s", .{secs}) catch {};
}

pub fn run(io: std.Io) !void {
    var downloaded: u64 = 0;
    const total_size: u64 = 231231231;

    var style = try mpb.ProgressStyle.withTemplate(
        "{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})",
    );
    style.progressChars("#>-");
    style = style.withKey("eta", writeEtaSeconds);

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
