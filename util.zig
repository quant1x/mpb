// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Small shared utilities: the allocator, the global `std.Io` instance and a
//! spin lock (Zig 0.17 no longer ships `std.Thread.Mutex`).

const std = @import("std");

pub const allocator = std.heap.smp_allocator;
pub const io = std.Io.Threaded.global_single_threaded.io();

pub const SpinLock = struct {
    locked: std.atomic.Value(bool) = .init(false),

    pub fn lock(self: *SpinLock) void {
        var spin: usize = 0;
        while (self.locked.cmpxchgWeak(false, true, .acquire, .monotonic) != null) {
            spin += 1;
            if (spin < 64) {
                std.atomic.spinLoopHint();
            } else {
                std.Thread.yield() catch {};
                spin = 0;
            }
        }
    }

    pub fn unlock(self: *SpinLock) void {
        self.locked.store(false, .release);
    }
};

pub fn nowNs() i96 {
    return std.Io.Clock.now(.awake, io).nanoseconds;
}

pub fn nsToSecs(ns: i96) f64 {
    return @as(f64, @floatFromInt(ns)) / @as(f64, @floatFromInt(std.time.ns_per_s));
}

/// Copy an `ArrayList(u8)` into a freshly allocated slice and release the list.
///
/// `toOwnedSlice` uses `Allocator.remap`, which cannot be used with slices
/// handed out by `ArenaAllocator` (they are not page aligned); the formatter
/// uses a per-draw arena, so finish buffers this way instead.
pub fn finishList(alloc: std.mem.Allocator, list: *std.ArrayList(u8)) []const u8 {
    const result = alloc.dupe(u8, list.items) catch "";
    list.deinit(alloc);
    return result;
}

/// Per-draw scratch space.
///
/// The formatter allocates many short-lived strings (padding, ANSI escapes,
/// human-readable numbers). They all die with the frame, so they go into a
/// fixed buffer whose only requirement is that allocation never escapes the
/// draw call.
pub const Scratch = struct {
    memory: []u8,
    fba: std.heap.FixedBufferAllocator,

    pub fn init(capacity: usize) !Scratch {
        const memory = try allocator.alloc(u8, capacity);
        return .{ .memory = memory, .fba = std.heap.FixedBufferAllocator.init(memory) };
    }

    pub fn deinit(self: *Scratch) void {
        allocator.free(self.memory);
    }

    pub fn allocatorFor(self: *Scratch) std.mem.Allocator {
        return self.fba.allocator();
    }
};
