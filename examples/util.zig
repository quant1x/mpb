// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Small helpers shared by the Zig examples (the Rust examples use `rand`,
//! `thread::sleep` and `println!`; these are the Zig equivalents).

const std = @import("std");
pub const mpb = @import("mpb");

pub const allocator = mpb.util.allocator;

pub fn sleepMs(io: std.Io, ms: u64) void {
    std.Io.sleep(io, .fromMilliseconds(@intCast(ms)), .awake) catch {};
}

pub fn sleepSecs(io: std.Io, seconds: u64) void {
    std.Io.sleep(io, .fromMilliseconds(@intCast(seconds * 1000)), .awake) catch {};
}

pub fn print(io: std.Io, text: []const u8) void {
    std.Io.File.stdout().writeStreamingAll(io, text) catch {};
}

pub fn println(io: std.Io, text: []const u8) void {
    print(io, text);
    print(io, "\n");
}

pub fn printFmt(io: std.Io, comptime fmt: []const u8, args: anytype) void {
    const text = std.fmt.allocPrint(allocator, fmt, args) catch return;
    println(io, text);
}

/// Deterministic replacement for `rand`: a 64-bit LCG.
pub const Rng = struct {
    state: u64,

    pub fn init(seed: u64) Rng {
        return .{ .state = seed };
    }

    pub fn next(self: *Rng) u64 {
        self.state = self.state *% 6364136223846793005 +% 1442695040888963407;
        return self.state >> 33;
    }

    /// `random_range(min..max)`, i.e. min inclusive, max exclusive.
    pub fn range(self: *Rng, min: u64, max: u64) u64 {
        if (max <= min) return min;
        return min + self.next() % (max - min);
    }

    pub fn pick(self: *Rng, items: []const []const u8) []const u8 {
        return items[self.next() % items.len];
    }
};

/// Format a duration the way Rust's `{:?}` does for short spans (e.g. `1.234s`).
pub fn debugDuration(ns: i96) []const u8 {
    return std.fmt.allocPrint(allocator, "{d:.3}s", .{@as(f64, @floatFromInt(ns)) / 1e9}) catch "";
}
