// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

const std = @import("std");
const single = @import("single.zig");

fn usage() void {
    std.debug.print("usage: zig build run -- <example>\n", .{});
    std.debug.print("examples: single\n", .{});
}

pub fn main(init: std.process.Init) !void {
    var args = try init.minimal.args.iterateAllocator(init.gpa);
    defer args.deinit();
    _ = args.next();
    const name = args.next() orelse {
        usage();
        return;
    };
    if (std.mem.eql(u8, name, "single")) {
        return single.run(init.io);
    }
    usage();
}
