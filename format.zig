// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `format.rs`.
//!
//! Every function takes an allocator: the progress formatter passes a
//! per-draw arena, examples pass the global allocator.

const std = @import("std");
const util = @import("util.zig");

const SECOND: i96 = std.time.ns_per_s;
const MINUTE: i96 = 60 * SECOND;
const HOUR: i96 = 60 * MINUTE;
const DAY: i96 = 24 * HOUR;
const WEEK: i96 = 7 * DAY;
const YEAR: i96 = 365 * DAY;

const Unit = struct { duration: i96, name: []const u8, alt: []const u8 };
const UNITS = [_]Unit{
    .{ .duration = YEAR, .name = "year", .alt = "y" },
    .{ .duration = WEEK, .name = "week", .alt = "w" },
    .{ .duration = DAY, .name = "day", .alt = "d" },
    .{ .duration = HOUR, .name = "hour", .alt = "h" },
    .{ .duration = MINUTE, .name = "minute", .alt = "m" },
    .{ .duration = SECOND, .name = "second", .alt = "s" },
};

/// `FormattedDuration`: `01:02:03` or `2d 01:02:03`.
pub fn formattedDuration(alloc: std.mem.Allocator, ns: i96) []const u8 {
    const total: u64 = @intCast(@max(ns, 0));
    var t = total / std.time.ns_per_s;
    const seconds = t % 60;
    t /= 60;
    const minutes = t % 60;
    t /= 60;
    const hours = t % 24;
    t /= 24;
    if (t > 0) {
        return std.fmt.allocPrint(alloc, "{d}d {d:0>2}:{d:0>2}:{d:0>2}", .{ t, hours, minutes, seconds }) catch "";
    }
    return std.fmt.allocPrint(alloc, "{d:0>2}:{d:0>2}:{d:0>2}", .{ hours, minutes, seconds }) catch "";
}

/// `HumanDuration`: `1 second`, `2 hours`, or `2h` in alternate mode.
pub fn humanDuration(alloc: std.mem.Allocator, ns: i96, alt: bool) []const u8 {
    var idx: usize = 0;
    for (UNITS, 0..) |unit, i| {
        idx = i;
        if (i + 1 < UNITS.len and ns + @divTrunc(UNITS[i + 1].duration, 2) >= unit.duration + @divTrunc(unit.duration, 2)) {
            break;
        }
    }
    const unit = UNITS[idx];
    var value: usize = @intFromFloat(@round(@as(f64, @floatFromInt(ns)) / @as(f64, @floatFromInt(unit.duration))));
    if (idx + 1 < UNITS.len) value = @max(value, 2);

    if (alt) return std.fmt.allocPrint(alloc, "{d}{s}", .{ value, unit.alt }) catch "";
    if (value == 1) return std.fmt.allocPrint(alloc, "{d} {s}", .{ value, unit.name }) catch "";
    return std.fmt.allocPrint(alloc, "{d} {s}s", .{ value, unit.name }) catch "";
}

const Prefix = struct { value: f64, prefix: []const u8 };

const PrefixResult = union(enum) { standalone: f64, prefixed: Prefix };

fn binaryPrefix(bytes: u64) PrefixResult {
    const binary_units = [_][]const u8{ "Ki", "Mi", "Gi", "Ti", "Pi", "Ei" };
    var number: f64 = @floatFromInt(bytes);
    if (number < 1024) return .{ .standalone = number };
    for (binary_units) |unit| {
        number /= 1024;
        if (number < 1024) return .{ .prefixed = .{ .value = number, .prefix = unit } };
    }
    return .{ .prefixed = .{ .value = number, .prefix = binary_units[binary_units.len - 1] } };
}

fn decimalPrefix(bytes: u64) PrefixResult {
    const decimal_units = [_][]const u8{ "k", "M", "G", "T", "P", "E" };
    var number: f64 = @floatFromInt(bytes);
    if (number < 1000) return .{ .standalone = number };
    for (decimal_units) |unit| {
        number /= 1000;
        if (number < 1000) return .{ .prefixed = .{ .value = number, .prefix = unit } };
    }
    return .{ .prefixed = .{ .value = number, .prefix = decimal_units[decimal_units.len - 1] } };
}

fn formatPrefixed(alloc: std.mem.Allocator, result: PrefixResult) []const u8 {
    return switch (result) {
        .standalone => |number| std.fmt.allocPrint(alloc, "{d:.0} B", .{number}) catch "",
        .prefixed => |p| std.fmt.allocPrint(alloc, "{d:.2} {s}B", .{ p.value, p.prefix }) catch "",
    };
}

pub fn humanBytes(alloc: std.mem.Allocator, bytes: u64) []const u8 {
    return formatPrefixed(alloc, binaryPrefix(bytes));
}

pub fn binaryBytes(alloc: std.mem.Allocator, bytes: u64) []const u8 {
    return formatPrefixed(alloc, binaryPrefix(bytes));
}

pub fn decimalBytes(alloc: std.mem.Allocator, bytes: u64) []const u8 {
    return formatPrefixed(alloc, decimalPrefix(bytes));
}

fn appendGroupedDigits(alloc: std.mem.Allocator, out: *std.ArrayList(u8), digits: []const u8) void {
    for (digits, 0..) |c, idx| {
        const pos = digits.len - idx - 1;
        out.append(alloc, c) catch {};
        if (pos > 0 and pos % 3 == 0) out.append(alloc, ',') catch {};
    }
}

/// `HumanCount`: 1234567 -> `1,234,567`.
pub fn humanCount(alloc: std.mem.Allocator, value: u64) []const u8 {
    var buf: [32]u8 = undefined;
    const num = std.fmt.bufPrint(&buf, "{d}", .{value}) catch return "";
    var out = std.ArrayList(u8).empty;
    appendGroupedDigits(alloc, &out, num);
    return util.finishList(alloc, &out);
}

fn floatToString(alloc: std.mem.Allocator, value: f64, precision: usize) []const u8 {
    return switch (precision) {
        0 => std.fmt.allocPrint(alloc, "{d:.0}", .{value}) catch "",
        1 => std.fmt.allocPrint(alloc, "{d:.1}", .{value}) catch "",
        2 => std.fmt.allocPrint(alloc, "{d:.2}", .{value}) catch "",
        3 => std.fmt.allocPrint(alloc, "{d:.3}", .{value}) catch "",
        5 => std.fmt.allocPrint(alloc, "{d:.5}", .{value}) catch "",
        6 => std.fmt.allocPrint(alloc, "{d:.6}", .{value}) catch "",
        else => std.fmt.allocPrint(alloc, "{d:.4}", .{value}) catch "",
    };
}

/// `HumanFloatCount`: grouped digits, trailing fractional zeros removed.
pub fn humanFloatCount(alloc: std.mem.Allocator, value: f64, precision: usize) []const u8 {
    if (std.math.isNan(value)) return "NaN";
    if (std.math.isInf(value)) return if (value > 0) "inf" else "-inf";

    const num = floatToString(alloc, value, precision);
    const dot = std.mem.indexOfScalar(u8, num, '.');
    var int_part = num;
    var frac_part: []const u8 = "";
    if (dot) |index| {
        int_part = num[0..index];
        frac_part = num[index + 1 ..];
    }
    var frac_trimmed = frac_part;
    while (frac_trimmed.len > 0 and frac_trimmed[frac_trimmed.len - 1] == '0') {
        frac_trimmed = frac_trimmed[0 .. frac_trimmed.len - 1];
    }

    var sign: []const u8 = "";
    var digits = int_part;
    if (std.mem.startsWith(u8, int_part, "-")) {
        sign = "-";
        digits = int_part[1..];
    }
    if (frac_trimmed.len == 0) {
        var all_zero = true;
        for (digits) |c| {
            if (c != '0') all_zero = false;
        }
        if (all_zero) sign = "";
    }

    var out = std.ArrayList(u8).empty;
    out.appendSlice(alloc, sign) catch {};
    appendGroupedDigits(alloc, &out, digits);
    if (frac_trimmed.len > 0) {
        out.append(alloc, '.') catch {};
        out.appendSlice(alloc, frac_trimmed) catch {};
    }
    return util.finishList(alloc, &out);
}

test "human duration matches rust rounding" {
    const alloc = util.allocator;
    try std.testing.expectEqualStrings("0 seconds", humanDuration(alloc, 0, false));
    try std.testing.expectEqualStrings("1 second", humanDuration(alloc, 500 * std.time.ns_per_ms, false));
    try std.testing.expectEqualStrings("60 seconds", humanDuration(alloc, MINUTE, false));
    try std.testing.expectEqualStrings("60 minutes", humanDuration(alloc, HOUR, false));
    try std.testing.expectEqualStrings("2y", humanDuration(alloc, 2 * YEAR, true));
}

test "human bytes matches rust" {
    const alloc = util.allocator;
    try std.testing.expectEqualStrings("15 B", humanBytes(alloc, 15));
    try std.testing.expectEqualStrings("1.46 KiB", humanBytes(alloc, 1500));
    try std.testing.expectEqualStrings("1.50 kB", decimalBytes(alloc, 1500));
}

test "human counts group digits" {
    const alloc = util.allocator;
    try std.testing.expectEqualStrings("1,234,567", humanCount(alloc, 1_234_567));
    try std.testing.expectEqualStrings("-1,000", humanFloatCount(alloc, -1000.0, 0));
    try std.testing.expectEqualStrings("1,234.5", humanFloatCount(alloc, 1234.5, 4));
    try std.testing.expectEqualStrings("inf", humanFloatCount(alloc, std.math.inf(f64), 4));
}
