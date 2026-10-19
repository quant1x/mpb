// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Extension of `multi_tree`: bars can be both inserted and removed, and
//! `--bottom-alignment` selects the vertical alignment.

const std = @import("std");
const u = @import("util.zig");
const mpb = u.mpb;

const PB_LEN: u64 = 32;

const Item = struct { key: []const u8, index: usize, indent: usize };

const Elem = union(enum) {
    add: Item,
    remove: usize,
};

const ELEMENTS = [_]Elem{
    .{ .add = .{ .key = "dog", .index = 0, .indent = 9 } },
    .{ .add = .{ .key = "temp_1", .index = 0, .indent = 0 } },
    .{ .add = .{ .key = "lazy", .index = 1, .indent = 8 } },
    .{ .add = .{ .key = "temp_2", .index = 1, .indent = 0 } },
    .{ .add = .{ .key = "the", .index = 0, .indent = 1 } },
    .{ .add = .{ .key = "temp_3", .index = 0, .indent = 0 } },
    .{ .add = .{ .key = "a", .index = 3, .indent = 7 } },
    .{ .add = .{ .key = "temp_4", .index = 3, .indent = 0 } },
    .{ .add = .{ .key = "over", .index = 2, .indent = 6 } },
    .{ .remove = 6 },
    .{ .remove = 4 },
    .{ .remove = 3 },
    .{ .remove = 0 },
    .{ .add = .{ .key = "temp_5", .index = 2, .indent = 0 } },
    .{ .add = .{ .key = "fox", .index = 1, .indent = 4 } },
    .{ .add = .{ .key = "temp_6", .index = 1, .indent = 0 } },
    .{ .add = .{ .key = "quick", .index = 1, .indent = 2 } },
    .{ .add = .{ .key = "temp_7", .index = 1, .indent = 0 } },
    .{ .add = .{ .key = "jumps", .index = 5, .indent = 5 } },
    .{ .add = .{ .key = "temp_8", .index = 5, .indent = 0 } },
    .{ .add = .{ .key = "brown", .index = 4, .indent = 3 } },
    .{ .add = .{ .key = "temp_9", .index = 3, .indent = 0 } },
    .{ .remove = 10 },
    .{ .remove = 7 },
    .{ .remove = 4 },
    .{ .remove = 3 },
    .{ .remove = 1 },
};

const Action = union(enum) {
    modify_tree: usize,
    inc: usize,
    stop,
};

fn indentString(indent: usize) []const u8 {
    return "                        "[0 .. indent * 2];
}

fn itemCount() usize {
    var total: u64 = 0;
    for (ELEMENTS) |elem| {
        total += switch (elem) {
            .add => PB_LEN,
            .remove => 1,
        };
    }
    return @intCast(total);
}

fn getAction(rng: *u.Rng, next: *usize, items: []const usize, bars: []const mpb.ProgressBar) Action {
    var uncompleted = std.ArrayList(usize).empty;
    defer uncompleted.deinit(u.allocator);

    for (items, 0..) |el, idx| {
        const bar = bars[el];
        if (bar.position() < bar.length().?) uncompleted.append(u.allocator, idx) catch {};
    }

    const elem_idx = next.*;
    const k = rng.range(0, 16);

    if ((k > 0 or (k == 0 and elem_idx == ELEMENTS.len)) and uncompleted.items.len > 0) {
        const idx = rng.range(0, uncompleted.items.len);
        return .{ .inc = uncompleted.items[@intCast(idx)] };
    } else if (elem_idx < ELEMENTS.len) {
        next.* += 1;
        return .{ .modify_tree = elem_idx };
    }
    return .stop;
}

pub fn run(io: std.Io, args: []const []const u8) !void {
    var bottom_alignment = false;
    for (args) |arg| {
        if (std.mem.eql(u8, arg, "--bottom-alignment")) bottom_alignment = true;
    }

    var mp = mpb.MultiProgress.init();
    defer mp.deinit();
    mp.setAlignment(if (bottom_alignment) .bottom else .top);

    const sty_main = try mpb.ProgressStyle.withTemplate("{bar:40.green/yellow} {pos:>4}/{len:4}");
    const sty_aux = try mpb.ProgressStyle.withTemplate("[{pos:>2}/{len:2}] {prefix}{spinner:.green} {msg}");
    const sty_fin = try mpb.ProgressStyle.withTemplate("[{pos:>2}/{len:2}] {prefix}{msg}");

    var bars: [ELEMENTS.len]mpb.ProgressBar = undefined;
    for (ELEMENTS, 0..) |elem, i| {
        bars[i] = mpb.ProgressBar.new(PB_LEN);
        switch (elem) {
            .add => bars[i].setStyle(sty_aux.clone()),
            .remove => {},
        }
    }
    defer {
        for (&bars) |*bar| bar.deinit();
    }

    var pb_main = mp.add(mpb.ProgressBar.new(itemCount()));
    defer pb_main.deinit();
    pb_main.setStyle(sty_main);

    var items = std.ArrayList(usize).empty;
    defer items.deinit(u.allocator);

    var rng = u.Rng.init(0xBEEF);
    var next_elem: usize = 0;
    pb_main.tick();

    while (true) {
        const action = getAction(&rng, &next_elem, items.items, &bars);
        switch (action) {
            .stop => {
                pb_main.finish();
                return;
            },
            .modify_tree => |elem_idx| {
                switch (ELEMENTS[elem_idx]) {
                    .add => |item| {
                        var pb = mp.insert(item.index, bars[elem_idx]);
                        pb.setPrefix(indentString(item.indent));
                        pb.setMessage(item.key);
                        try items.insert(u.allocator, item.index, elem_idx);
                    },
                    .remove => |index| {
                        const el = items.orderedRemove(index);
                        const bar = bars[el];
                        mp.remove(bar);
                        pb_main.inc(bar.length().? - bar.position());
                    },
                }
            },
            .inc => |item_idx| {
                const el = items.items[item_idx];
                const bar = bars[el];
                bar.inc(1);
                if (bar.position() >= bar.length().?) {
                    bar.setStyle(sty_fin.clone());
                    const done = std.fmt.allocPrint(u.allocator, "✔ {s}", .{
                        switch (ELEMENTS[el]) {
                            .add => |item| item.key,
                            .remove => "",
                        },
                    }) catch "";
                    bar.finishWithMessage(done);
                }
                pb_main.inc(1);
            },
        }
        u.sleepMs(io, 20);
    }
}
