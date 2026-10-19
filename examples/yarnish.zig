// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

const PACKAGES = [_][]const u8{
    "fs-events",
    "my-awesome-module",
    "emoji-speaker",
    "wrap-ansi",
    "stream-browserify",
    "acorn-dynamic-import",
};

const COMMANDS = [_][]const u8{
    "cmake .",
    "make",
    "make clean",
    "gcc foo.c -o foo",
    "gcc bar.c -o bar",
    "./helper.sh rebuild-cache",
    "make all-clean",
    "make test",
};

fn stage(io: std.Io, text: []const u8, emoji: []const u8) void {
    const label = mpb.style.applyStyle(text, "bold.dim");
    u.printFmt(io, "{s} {s}", .{ label, emoji });
}

pub fn run(io: std.Io) !void {
    var rng = u.Rng.init(0x7A11);
    const started = mpb.util.nowNs();

    var spinner_style = try mpb.ProgressStyle.withTemplate("{prefix:.bold.dim} {spinner} {wide_msg}");
    spinner_style.tickChars("⠁⠂⠄⡀⢀⠠⠐⠈ ");

    stage(io, "[1/4]", "🔍  Resolving packages...");
    stage(io, "[2/4]", "🚚  Fetching packages...");
    stage(io, "[3/4]", "🔗  Linking dependencies...");

    const deps: u64 = 1232;
    {
        var pb = mpb.ProgressBar.new(deps);
        defer pb.deinit();
        for (0..deps) |_| {
            u.sleepMs(io, 3);
            pb.inc(1);
        }
        pb.finishAndClear();
    }

    stage(io, "[4/4]", "📃  Building fresh packages...");

    var mp = mpb.MultiProgress.init();
    defer mp.deinit();

    var bars: [4]mpb.ProgressBar = undefined;
    for (&bars, 0..) |*bar, i| {
        const count = rng.range(30, 80);
        bar.* = mp.add(mpb.ProgressBar.new(count));
        bar.setStyle(spinner_style);
        const prefix = std.fmt.allocPrint(u.allocator, "[{d}/?]", .{i + 1}) catch "";
        bar.setPrefix(prefix);
    }
    defer {
        for (&bars) |*bar| bar.deinit();
    }

    const counts = [_]u64{ bars[0].length().?, bars[1].length().?, bars[2].length().?, bars[3].length().? };
    var step: u64 = 0;
    const max_count = @max(@max(counts[0], counts[1]), @max(counts[2], counts[3]));
    while (step < max_count) : (step += 1) {
        for (&bars, 0..) |*bar, i| {
            if (step >= counts[i]) continue;
            const package = rng.pick(&PACKAGES);
            const command = rng.pick(&COMMANDS);
            u.sleepMs(io, rng.range(25, 200));
            const message = std.fmt.allocPrint(u.allocator, "{s}: {s}", .{ package, command }) catch "";
            bar.setMessage(message);
            bar.inc(1);
            if (bar.position() >= counts[i]) bar.finishWithMessage("waiting...");
        }
    }
    mp.clear();

    u.printFmt(io, "✨ Done in {s}", .{mpb.format.humanDuration(u.allocator, mpb.util.nowNs() - started, false)});
}
