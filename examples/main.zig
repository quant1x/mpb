// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Example dispatcher: `zig build run -- <name>`.

const std = @import("std");
const u = @import("util.zig");

const single = @import("single.zig");
const fastbar = @import("fastbar.zig");
const finebars = @import("finebars.zig");
const log = @import("log.zig");
const morebars = @import("morebars.zig");
const multi = @import("multi.zig");
const multi_tree = @import("multi_tree.zig");
const multi_tree_ext = @import("multi_tree_ext.zig");
const cargo = @import("cargo.zig");
const cargowrap = @import("cargowrap.zig");
const download = @import("download.zig");
const download_continued = @import("download_continued.zig");
const download_speed = @import("download_speed.zig");
const iterator = @import("iterator.zig");
const long_spinner = @import("long_spinner.zig");
const tokio = @import("tokio.zig");
const yarnish = @import("yarnish.zig");

const examples = [_][]const u8{
    "single",
    "fastbar",
    "finebars",
    "log",
    "morebars",
    "multi",
    "multi-tree",
    "multi-tree-ext",
    "cargo",
    "cargowrap",
    "download",
    "download-continued",
    "download-speed",
    "iterator",
    "long-spinner",
    "tokio",
    "yarnish",
};

fn usage() void {
    std.debug.print("usage: zig build run -- <example> [args]\n", .{});
    std.debug.print("examples:", .{});
    for (examples) |name| std.debug.print(" {s}", .{name});
    std.debug.print("\n", .{});
}

pub fn main(init: std.process.Init) !void {
    u.raiseTimerResolution();
    defer u.restoreTimerResolution();

    var args = try init.minimal.args.iterateAllocator(init.gpa);
    defer args.deinit();
    _ = args.next();

    const raw_name = args.next() orelse {
        usage();
        return;
    };
    // Accept both `multi-tree` (upstream name) and `multi_tree` (file name).
    var name_buf: [64]u8 = undefined;
    const name = if (std.mem.indexOfScalar(u8, raw_name, '-') != null and raw_name.len <= name_buf.len) blk: {
        for (raw_name, 0..) |c, i| name_buf[i] = if (c == '-') '_' else c;
        break :blk name_buf[0..raw_name.len];
    } else raw_name;

    var rest = std.ArrayList([]const u8).empty;
    defer rest.deinit(init.gpa);
    while (args.next()) |arg| try rest.append(init.gpa, arg);

    if (std.mem.eql(u8, name, "single")) return single.run(init.io);
    if (std.mem.eql(u8, name, "fastbar")) return fastbar.run(init.io);
    if (std.mem.eql(u8, name, "finebars")) return finebars.run(init.io);
    if (std.mem.eql(u8, name, "log")) return log.run(init.io);
    if (std.mem.eql(u8, name, "morebars")) return morebars.run(init.io);
    if (std.mem.eql(u8, name, "multi")) return multi.run(init.io);
    if (std.mem.eql(u8, name, "multi_tree")) return multi_tree.run(init.io);
    if (std.mem.eql(u8, name, "multi_tree_ext")) return multi_tree_ext.run(init.io, rest.items);
    if (std.mem.eql(u8, name, "cargo")) return cargo.run(init.io);
    if (std.mem.eql(u8, name, "cargowrap")) return cargowrap.run(init.io);
    if (std.mem.eql(u8, name, "download")) return download.run(init.io);
    if (std.mem.eql(u8, name, "download_continued")) return download_continued.run(init.io);
    if (std.mem.eql(u8, name, "download_speed")) return download_speed.run(init.io);
    if (std.mem.eql(u8, name, "iterator")) return iterator.run(init.io);
    if (std.mem.eql(u8, name, "long_spinner")) return long_spinner.run(init.io);
    if (std.mem.eql(u8, name, "tokio")) return tokio.run(init.io);
    if (std.mem.eql(u8, name, "yarnish")) return yarnish.run(init.io);

    usage();
}
