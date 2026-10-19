// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Small shared utilities: the allocator, the global `std.Io` instance and a
//! spin lock (Zig 0.17 no longer ships `std.Thread.Mutex`).

const std = @import("std");
const builtin = @import("builtin");

const is_windows = builtin.os.tag == .windows;

const winmm = if (is_windows) struct {
    extern "winmm" fn timeBeginPeriod(period: std.os.windows.UINT) callconv(.winapi) std.os.windows.UINT;
    extern "winmm" fn timeEndPeriod(period: std.os.windows.UINT) callconv(.winapi) std.os.windows.UINT;
} else struct {};

const win_timer = if (is_windows) struct {
    const CREATE_WAITABLE_TIMER_HIGH_RESOLUTION: std.os.windows.DWORD = 0x00000002;
    const TIMER_ALL_ACCESS: std.os.windows.DWORD = 0x1F0003;
    const INFINITE: std.os.windows.DWORD = 0xFFFFFFFF;

    const LARGE_INTEGER = extern struct { QuadPart: i64 };

    extern "kernel32" fn CreateWaitableTimerExW(
        attributes: ?*anyopaque,
        name: ?[*:0]const u16,
        flags: std.os.windows.DWORD,
        access: std.os.windows.DWORD,
    ) callconv(.winapi) ?std.os.windows.HANDLE;

    extern "kernel32" fn SetWaitableTimer(
        timer: std.os.windows.HANDLE,
        due: *const LARGE_INTEGER,
        period: i32,
        completion: ?*anyopaque,
        arg: ?*anyopaque,
        resume_state: std.os.windows.BOOL,
    ) callconv(.winapi) std.os.windows.BOOL;

    extern "kernel32" fn WaitForSingleObject(handle: std.os.windows.HANDLE, milliseconds: std.os.windows.DWORD) callconv(.winapi) std.os.windows.DWORD;
    extern "kernel32" fn CloseHandle(handle: std.os.windows.HANDLE) callconv(.winapi) std.os.windows.BOOL;
} else struct {};

/// Windows' default timer granularity is ~15.6ms, so `sleep(3ms)` really
/// sleeps ~16ms. Rust's runtime requests a 1ms resolution, which is why the
/// same example feels much smoother there; mirror that around timed work.
pub fn beginTimerResolution() void {
    if (is_windows) _ = winmm.timeBeginPeriod(1);
}

pub fn endTimerResolution() void {
    if (is_windows) _ = winmm.timeEndPeriod(1);
}

/// Short sleep with platform-appropriate precision.
///
/// Windows: `std.Io.sleep` / `Sleep` are bounded by the ~15.6ms system timer,
/// so a 50µs backoff really lasts 15.6ms. Use a
/// `CREATE_WAITABLE_TIMER_HIGH_RESOLUTION` waitable timer instead, which is the
/// same approach as `mopr::base::sleep_for_microseconds` (measured 0.53ms for
/// 50µs, matching Rust). The timer is created per call: reusing a handle would
/// need thread-local storage and concurrent users would interfere; creation is
/// ~14µs.
///
/// macOS/POSIX: `nanosleep` (used by `std.Io.sleep`) already has microsecond
/// resolution, so there is no equivalent problem. Note that the spin hint also
/// differs: x86 uses `pause`, arm64 must use `isb` — `std.atomic.spinLoopHint`
/// already does this, while the aarch64 `yield` instruction is aggressively
/// throttled on Apple Silicon and must not be used for backoff.
pub fn sleepMicros(us: u64) void {
    if (is_windows and winSleepMicros(us)) return;
    const clamped = @min(us, 3600 * std.time.us_per_s);
    std.Io.sleep(io, .fromNanoseconds(@intCast(clamped * std.time.ns_per_us)), .awake) catch {};
}

fn winSleepMicros(us: u64) bool {
    if (us == 0) return true;

    const timer = win_timer.CreateWaitableTimerExW(
        null,
        null,
        win_timer.CREATE_WAITABLE_TIMER_HIGH_RESOLUTION,
        win_timer.TIMER_ALL_ACCESS,
    ) orelse return false;
    defer _ = win_timer.CloseHandle(timer);

    const clamped = @min(us, 3600 * std.time.us_per_s);
    // Negative means relative; the unit is 100ns.
    var due = win_timer.LARGE_INTEGER{ .QuadPart = -@as(i64, @intCast(clamped * 10)) };
    if (!win_timer.SetWaitableTimer(timer, &due, 0, null, null, .FALSE).toBool()) return false;
    _ = win_timer.WaitForSingleObject(timer, win_timer.INFINITE);
    return true;
}

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
