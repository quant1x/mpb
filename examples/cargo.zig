// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Mimics cargo's build progress: a shared bar, "Compiling" log lines and a
//! message listing the crates that are currently being built.

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

const Crate = struct { name: []const u8, version: []const u8 };

const CRATES = [_]Crate{
    .{ .name = "console", .version = "v0.14.1" },
    .{ .name = "lazy_static", .version = "v1.4.0" },
    .{ .name = "libc", .version = "v0.2.93" },
    .{ .name = "regex", .version = "v1.4.6" },
    .{ .name = "regex-syntax", .version = "v0.6.23" },
    .{ .name = "terminal_size", .version = "v0.1.16" },
    .{ .name = "libc", .version = "v0.2.93" },
    .{ .name = "unicode-width", .version = "v0.1.8" },
    .{ .name = "lazy_static", .version = "v1.4.0" },
    .{ .name = "number_prefix", .version = "v0.4.0" },
    .{ .name = "regex", .version = "v1.4.6" },
    .{ .name = "rand", .version = "v0.8.3" },
    .{ .name = "getrandom", .version = "v0.2.2" },
    .{ .name = "cfg-if", .version = "v1.0.0" },
    .{ .name = "libc", .version = "v0.2.93" },
    .{ .name = "rand_chacha", .version = "v0.3.0" },
    .{ .name = "ppv-lite86", .version = "v0.2.10" },
    .{ .name = "rand_core", .version = "v0.6.2" },
    .{ .name = "getrandom", .version = "v0.2.2" },
    .{ .name = "rand_core", .version = "v0.6.2" },
    .{ .name = "tokio", .version = "v1.5.0" },
    .{ .name = "bytes", .version = "v1.0.1" },
    .{ .name = "pin-project-lite", .version = "v0.2.6" },
    .{ .name = "slab", .version = "v0.4.3" },
    .{ .name = "indicatif", .version = "v0.15.0" },
};

const NUM_CPUS = 4;

pub fn run(io: std.Io) !void {
    const start = mpb.util.nowNs();

    var stdout_term = mpb.term_like.StdTerm.bufferedStdout();
    const template = if (stdout_term.width() > 80)
        "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len} {wide_msg}"
    else
        "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len}";

    var style = try mpb.ProgressStyle.withTemplate(template);
    style.progressChars("=> ");

    var pb = mpb.ProgressBar.new(CRATES.len);
    defer pb.deinit();
    pb.setStyle(style);
    pb.setPrefix("Building");

    var processing: [NUM_CPUS]?usize = .{ null, null, null, null };
    var rng = u.Rng.init(0xC0FFEE);

    for (CRATES, 0..) |krate, i| {
        const slot = i % NUM_CPUS;
        processing[slot] = i;

        var message = std.ArrayList(u8).empty;
        for (processing) |maybe| {
            if (maybe) |index| {
                if (message.items.len > 0) message.appendSlice(u.allocator, ", ") catch {};
                message.appendSlice(u.allocator, CRATES[index].name) catch {};
            }
        }
        pb.setMessage(message.items);

        const compiling = mpb.style.applyStyle("Compiling", "green.bold");
        const line = std.fmt.allocPrint(u.allocator, "{s:>12} {s} {s}", .{
            compiling, krate.name, krate.version,
        }) catch "";
        pb.println(line);

        u.sleepMs(io, rng.range(250, 1000));
        pb.inc(1);
    }
    pb.finishAndClear();

    const finished = mpb.style.applyStyle("Finished", "green.bold");
    u.printFmt(io, "{s:>12} dev [unoptimized + debuginfo] target(s) in {s}", .{
        finished, mpb.format.humanDuration(u.allocator, mpb.util.nowNs() - start, false),
    });
}
