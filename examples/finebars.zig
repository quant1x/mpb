// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

const StyleSpec = struct { prefix: []const u8, chars: []const u8, color: []const u8 };

const styles = [_]StyleSpec{
    .{ .prefix = "Rough bar:", .chars = "█  ", .color = "red" },
    .{ .prefix = "Fine bar: ", .chars = "█▉▊▋▌▍▎▏  ", .color = "yellow" },
    .{ .prefix = "Vertical: ", .chars = "█▇▆▅▄▃▂▁  ", .color = "green" },
    .{ .prefix = "Fade in:  ", .chars = "█▓▒░  ", .color = "blue" },
    .{ .prefix = "Blocky:   ", .chars = "█▛▌▖  ", .color = "magenta" },
};

pub fn run(io: std.Io) !void {
    var mp = mpb.MultiProgress.init();
    defer mp.deinit();

    var bars: [styles.len]mpb.ProgressBar = undefined;
    var created: usize = 0;
    defer {
        for (bars[0..created]) |*bar| bar.deinit();
    }

    for (styles, 0..) |spec, i| {
        bars[i] = mp.add(mpb.ProgressBar.new(512));
        created += 1;

        const template = std.fmt.allocPrint(
            u.allocator,
            "{{prefix:.bold}}▕{{bar:.{s}}}▏{{msg}}",
            .{spec.color},
        ) catch "";
        var style = try mpb.ProgressStyle.withTemplate(template);
        style.progressChars(spec.chars);
        bars[i].setStyle(style);
        bars[i].setPrefix(spec.prefix);
    }

    var rng = u.Rng.init(0x5EED);
    var i: usize = 0;
    while (i < 512) : (i += 1) {
        for (bars) |bar| {
            u.sleepMs(io, rng.range(2, 5));
            bar.inc(1);
            const percent = std.fmt.allocPrint(u.allocator, "{d:>3}%", .{100 * i / 512}) catch "";
            bar.setMessage(percent);
        }
    }

    for (&bars) |*bar| bar.finishWithMessage("100%");
}
