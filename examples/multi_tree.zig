// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Tree-like collection of progress bars: elements are added on the fly and
//! advanced until every element exists and every bar is finished.

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

const PB_LEN: u64 = 32;

const Spec = struct { key: []const u8, index: usize, indent: usize };

const SPECS = [_]Spec{
    .{ .key = "jumps", .index = 0, .indent = 1 },
    .{ .key = "lazy", .index = 1, .indent = 2 },
    .{ .key = "the", .index = 0, .indent = 0 },
    .{ .key = "dog", .index = 3, .indent = 3 },
    .{ .key = "over", .index = 2, .indent = 2 },
    .{ .key = "brown", .index = 1, .indent = 2 },
    .{ .key = "quick", .index = 1, .indent = 1 },
    .{ .key = "a", .index = 5, .indent = 3 },
    .{ .key = "fox", .index = 3, .indent = 3 },
};

const Action = union(enum) {
    add: usize,
    inc: usize,
    done,
};

fn indentString(indent: usize) []const u8 {
    return "                        "[0 .. indent * 2];
}

fn getAction(rng: *u.Rng, tree: *std.ArrayList(usize), bars: []const mpb.ProgressBar) Action {
    const list_len = tree.items.len;

    var sum_free: u64 = 0;
    for (tree.items) |el| {
        const bar = bars[el];
        sum_free += bar.length().? - bar.position();
    }

    if (sum_free == 0 and list_len == SPECS.len) return .done;
    if (sum_free == 0 and list_len < SPECS.len) return .{ .add = list_len };

    while (true) {
        const k = rng.range(0, 17);
        if (k == 0 and list_len < SPECS.len) return .{ .add = list_len };
        if (list_len == 0) continue;
        const pos = @as(usize, @intCast(k)) % list_len;
        const bar = bars[tree.items[pos]];
        if (bar.position() < bar.length().?) return .{ .inc = pos };
    }
}

pub fn run(io: std.Io) !void {
    var mp = mpb.MultiProgress.init();
    defer mp.deinit();

    const sty_main = try mpb.ProgressStyle.withTemplate("{bar:40.green/yellow} {pos:>4}/{len:4}");
    const sty_aux = try mpb.ProgressStyle.withTemplate("{spinner:.green} {msg} {pos:>4}/{len:4}");

    var bars: [SPECS.len]mpb.ProgressBar = undefined;
    for (SPECS, 0..) |_, i| {
        bars[i] = mpb.ProgressBar.new(PB_LEN);
        bars[i].setStyle(sty_aux.clone());
    }
    defer {
        for (&bars) |*bar| bar.deinit();
    }

    var pb_main = mp.add(mpb.ProgressBar.new(PB_LEN * SPECS.len));
    defer pb_main.deinit();
    pb_main.setStyle(sty_main);

    var tree = std.ArrayList(usize).empty;
    defer tree.deinit(u.allocator);

    var rng = u.Rng.init(0xA11CE);
    pb_main.tick();

    while (true) {
        u.sleepMs(io, 15);
        switch (getAction(&rng, &tree, &bars)) {
            .done => {
                pb_main.finish();
                break;
            },
            .add => |el| {
                const spec = SPECS[el];
                var pb = mp.insert(spec.index + 1, bars[el]);
                const message = std.fmt.allocPrint(u.allocator, "{s}  {s}", .{ indentString(spec.indent), spec.key }) catch "";
                pb.setMessage(message);
                try tree.insert(u.allocator, spec.index, el);
            },
            .inc => |pos| {
                const el = tree.items[pos];
                const bar = bars[el];
                bar.inc(1);
                if (bar.position() >= bar.length().?) {
                    const message = std.fmt.allocPrint(
                        u.allocator,
                        "{s}✔ {s}",
                        .{ indentString(SPECS[el].indent), SPECS[el].key },
                    ) catch "";
                    bar.finishWithMessage(message);
                }
                pb_main.inc(1);
            },
        }
    }

    u.println(io, "===============================");
    u.println(io, "the tree should be the same as:");
    for (tree.items) |el| {
        u.printFmt(io, "{s}  {s}", .{ indentString(SPECS[el].indent), SPECS[el].key });
    }
}
