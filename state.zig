// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `state.rs`.

const std = @import("std");
const util = @import("util.zig");
const dt = @import("draw_target.zig");
const style = @import("style.zig");

const allocator = util.allocator;

pub const FinishKind = enum {
    and_leave,
    with_message,
    and_clear,
    abandon,
    abandon_with_message,
};

pub const ProgressFinish = struct {
    kind: FinishKind = .and_clear,
    message: []const u8 = "",
};

pub const Status = union(enum) {
    in_progress,
    done_visible: i96,
    done_hidden: i96,

    pub fn isFinished(self: Status) bool {
        return switch (self) {
            .in_progress => false,
            .done_visible, .done_hidden => true,
        };
    }
};

pub const ResetMode = enum { eta, elapsed, all };

/// `TabExpandedString`: eagerly expands tabs so `expanded()` can stay `const`.
pub const TabExpandedString = struct {
    original: []const u8,
    text: []const u8,
    tab_width: usize,

    pub fn init(value: []const u8, tab_width: usize) TabExpandedString {
        const original = allocator.dupe(u8, value) catch "";
        return .{
            .original = original,
            .text = expand(original, tab_width),
            .tab_width = tab_width,
        };
    }

    fn expand(value: []const u8, tab_width: usize) []const u8 {
        if (std.mem.indexOfScalar(u8, value, '\t') == null) return allocator.dupe(u8, value) catch "";

        var out = std.ArrayList(u8).empty;
        for (value) |c| {
            if (c == '\t') {
                out.appendNTimes(allocator, ' ', tab_width) catch {};
            } else {
                out.append(allocator, c) catch {};
            }
        }
        return out.toOwnedSlice(allocator) catch "";
    }

    pub fn expanded(self: *const TabExpandedString) []const u8 {
        return self.text;
    }

    pub fn setTabWidth(self: *TabExpandedString, new_tab_width: usize) void {
        if (self.tab_width == new_tab_width) return;
        self.tab_width = new_tab_width;
        if (std.mem.indexOfScalar(u8, self.original, '\t') != null) {
            allocator.free(self.text);
            self.text = expand(self.original, new_tab_width);
        }
    }
};

/// Rate-limited atomic position, mirroring `state::AtomicPosition`.
pub const AtomicPosition = struct {
    pos: std.atomic.Value(u64) = .init(0),
    capacity: std.atomic.Value(u8) = .init(MAX_BURST),
    prev: std.atomic.Value(u64) = .init(0),
    start: i96,

    const INTERVAL: u64 = 1_000_000; // 1ms in ns
    const MAX_BURST: u8 = 10;

    pub fn init() AtomicPosition {
        return .{ .start = util.nowNs() };
    }

    pub fn allow(self: *AtomicPosition, now: i96) bool {
        if (now < self.start) return false;

        var capacity = self.capacity.load(.acquire);
        const prev = self.prev.load(.acquire);
        const elapsed: u64 = @intCast(now - self.start);
        const diff = elapsed -| prev;

        if (capacity == 0 and diff < INTERVAL) return false;

        const new = diff / INTERVAL;
        const remainder = diff % INTERVAL;
        capacity = @intCast(@min(@as(u64, MAX_BURST), @as(u64, capacity) + new - 1));

        self.capacity.store(capacity, .release);
        self.prev.store(elapsed - remainder, .release);
        return true;
    }

    pub fn reset(self: *AtomicPosition, now: i96) void {
        self.set(0);
        const elapsed: u64 = if (now > self.start) @intCast(now - self.start) else 0;
        self.prev.store(elapsed, .release);
    }

    pub fn inc(self: *AtomicPosition, delta: u64) void {
        _ = self.pos.fetchAdd(delta, .seq_cst);
    }

    pub fn dec(self: *AtomicPosition, delta: u64) void {
        _ = self.pos.fetchSub(delta, .seq_cst);
    }

    pub fn set(self: *AtomicPosition, value: u64) void {
        self.pos.store(value, .release);
    }
};

/// Double-smoothed exponentially weighted estimator.
pub const Estimator = struct {
    smoothed_steps_per_sec: f64 = 0,
    double_smoothed_steps_per_sec: f64 = 0,
    prev_steps: u64 = 0,
    prev_time: i96,
    start_time: i96,

    pub fn init(now: i96) Estimator {
        return .{ .prev_time = now, .start_time = now };
    }

    pub fn record(self: *Estimator, new_steps: u64, now: i96) void {
        if (new_steps <= self.prev_steps or now <= self.prev_time) {
            if (new_steps < self.prev_steps) {
                self.prev_steps = new_steps;
                self.reset(now);
            }
            return;
        }

        const delta_steps = new_steps - self.prev_steps;
        const delta_t = util.nsToSecs(now - self.prev_time);
        const new_steps_per_second = @as(f64, @floatFromInt(delta_steps)) / delta_t;

        const weight = estimatorWeight(delta_t);
        self.smoothed_steps_per_sec = self.smoothed_steps_per_sec * weight + new_steps_per_second * (1.0 - weight);

        const delta_t_start = util.nsToSecs(now - self.start_time);
        const total_weight = 1.0 - estimatorWeight(delta_t_start);
        const normalized = self.smoothed_steps_per_sec / total_weight;

        self.double_smoothed_steps_per_sec = self.double_smoothed_steps_per_sec * weight + normalized * (1.0 - weight);

        self.prev_steps = new_steps;
        self.prev_time = now;
    }

    pub fn reset(self: *Estimator, now: i96) void {
        self.smoothed_steps_per_sec = 0;
        self.double_smoothed_steps_per_sec = 0;
        self.prev_time = now;
        self.start_time = now;
    }

    pub fn stepsPerSecond(self: *const Estimator, now: i96) f64 {
        const delta_t = util.nsToSecs(now - self.prev_time);
        const reweight = estimatorWeight(delta_t);

        const delta_t_start = util.nsToSecs(now - self.start_time);
        const total_weight = 1.0 - estimatorWeight(delta_t_start);

        const sps = self.smoothed_steps_per_sec * reweight / total_weight;
        const dsps = self.double_smoothed_steps_per_sec * reweight + sps * (1.0 - reweight);
        return dsps / total_weight;
    }
};

fn estimatorWeight(age: f64) f64 {
    const EXPONENTIAL_WEIGHTING_SECONDS = 15.0;
    return std.math.pow(f64, 0.1, age / EXPONENTIAL_WEIGHTING_SECONDS);
}

/// `secs_to_duration`.
pub fn secsToDuration(seconds: f64) i96 {
    if (seconds <= 0 or std.math.isNan(seconds)) return 0;
    const secs: i64 = @intFromFloat(@trunc(seconds));
    const nanos: i64 = @intFromFloat((seconds - @as(f64, @floatFromInt(secs))) * 1e9);
    return @as(i96, secs) * std.time.ns_per_s + @as(i96, nanos);
}

/// `ProgressState`.
pub const ProgressState = struct {
    pos: *AtomicPosition,
    len: ?u64 = null,
    tick: u64 = 0,
    started: i96,
    status: Status = .in_progress,
    est: Estimator,
    message: TabExpandedString,
    prefix: TabExpandedString,

    pub fn init(len: ?u64, pos: *AtomicPosition) ProgressState {
        const now = util.nowNs();
        return .{
            .pos = pos,
            .len = len,
            .started = now,
            .est = Estimator.init(now),
            .message = TabExpandedString.init("", 8),
            .prefix = TabExpandedString.init("", 8),
        };
    }

    pub fn isFinished(self: *const ProgressState) bool {
        return self.status.isFinished();
    }

    pub fn position(self: *const ProgressState) u64 {
        return self.pos.pos.load(.monotonic);
    }

    pub fn fraction(self: *const ProgressState) f32 {
        const pos = self.position();
        const len = self.len orelse return 0.0;
        if (len == 0) return 1.0;
        if (pos == 0) return 0.0;
        const value = @as(f32, @floatFromInt(pos)) / @as(f32, @floatFromInt(len));
        return @max(0.0, @min(1.0, value));
    }

    pub fn eta(self: *const ProgressState) i96 {
        if (self.isFinished()) return 0;
        const len = self.len orelse return 0;
        const pos = self.position();
        const sps = self.est.stepsPerSecond(util.nowNs());
        if (sps == 0.0) return 0;
        return secsToDuration(@as(f64, @floatFromInt(len -| pos)) / sps);
    }

    pub fn duration(self: *const ProgressState) i96 {
        return switch (self.status) {
            .done_visible => |dur| dur,
            .done_hidden => |dur| dur,
            .in_progress => if (self.len != null) (util.nowNs() - self.started) + self.eta() else 0,
        };
    }

    pub fn perSec(self: *const ProgressState) f64 {
        if (self.status == .in_progress) return self.est.stepsPerSecond(util.nowNs());
        const secs = util.nsToSecs(util.nowNs() - self.started);
        return @as(f64, @floatFromInt(self.position())) / secs;
    }

    pub fn elapsed(self: *const ProgressState) i96 {
        return util.nowNs() - self.started;
    }
};

/// `BarState` (guarded by `mutex`, mirroring `Mutex<BarState>` in Rust).
pub const BarState = struct {
    draw_target: dt.ProgressDrawTarget,
    on_finish: ProgressFinish = .{},
    style: style.ProgressStyle,
    state: ProgressState,
    tab_width: usize = 8,
    refs: std.atomic.Value(usize) = .init(1),
    mutex: util.SpinLock = .{},
    ticker: ?*anyopaque = null,

    pub fn init(len: ?u64, draw_target: dt.ProgressDrawTarget, pos: *AtomicPosition) BarState {
        return .{
            .draw_target = draw_target,
            .style = style.ProgressStyle.defaultBar(),
            .state = ProgressState.init(len, pos),
        };
    }

    pub fn lock(self: *BarState) void {
        self.mutex.lock();
    }

    pub fn unlock(self: *BarState) void {
        self.mutex.unlock();
    }

    pub fn retain(self: *BarState) void {
        _ = self.refs.fetchAdd(1, .monotonic);
    }

    /// `Drop for BarState`.
    pub fn release(self: *BarState) void {
        if (self.refs.fetchSub(1, .acq_rel) != 1) return;

        self.lock();
        const finished = self.state.isFinished();
        if (!finished) self.finishUsingStyleLocked(util.nowNs(), self.on_finish);
        self.draw_target.markZombie();
        self.unlock();

        self.draw_target.deinit();
        allocator.destroy(self);
    }

    pub fn finishUsingStyle(self: *BarState, now: i96, finish: ProgressFinish) void {
        self.lock();
        defer self.unlock();
        self.finishUsingStyleLocked(now, finish);
    }

    fn finishUsingStyleLocked(self: *BarState, now: i96, finish: ProgressFinish) void {
        const duration = now - self.state.started;
        self.state.status = .{ .done_visible = duration };

        switch (finish.kind) {
            .and_leave => {
                if (self.state.len) |len| self.state.pos.set(len);
            },
            .with_message => {
                if (self.state.len) |len| self.state.pos.set(len);
                self.state.message = TabExpandedString.init(finish.message, self.tab_width);
            },
            .and_clear => {
                if (self.state.len) |len| self.state.pos.set(len);
                self.state.status = .{ .done_hidden = duration };
            },
            .abandon => {},
            .abandon_with_message => {
                self.state.message = TabExpandedString.init(finish.message, self.tab_width);
            },
        }

        self.drawLocked(true, now);
    }

    pub fn reset(self: *BarState, now: i96, mode: ResetMode) void {
        self.lock();
        defer self.unlock();
        self.state.est.reset(now);

        if (mode == .elapsed or mode == .all) self.state.started = now;

        if (mode == .all) {
            self.state.pos.reset(now);
            self.state.status = .in_progress;
            self.drawLocked(false, now);
        }
    }

    pub fn unsetLength(self: *BarState, now: i96) void {
        self.lock();
        defer self.unlock();
        self.state.len = null;
        self.updateEstimateAndDrawLocked(now);
    }

    pub fn setLength(self: *BarState, now: i96, len: u64) void {
        self.lock();
        defer self.unlock();
        self.state.len = len;
        self.updateEstimateAndDrawLocked(now);
    }

    pub fn incLength(self: *BarState, now: i96, delta: u64) void {
        self.lock();
        defer self.unlock();
        if (self.state.len) |len| self.state.len = len +| delta;
        self.updateEstimateAndDrawLocked(now);
    }

    pub fn decLength(self: *BarState, now: i96, delta: u64) void {
        self.lock();
        defer self.unlock();
        if (self.state.len) |len| self.state.len = len -| delta;
        self.updateEstimateAndDrawLocked(now);
    }

    pub fn setTabWidth(self: *BarState, tab_width: usize) void {
        self.lock();
        defer self.unlock();
        self.tab_width = tab_width;
        self.state.message.setTabWidth(tab_width);
        self.state.prefix.setTabWidth(tab_width);
        self.style.setTabWidth(tab_width);
    }

    pub fn setStyle(self: *BarState, new_style: style.ProgressStyle) void {
        self.lock();
        defer self.unlock();
        self.style = new_style;
        self.style.setTabWidth(self.tab_width);
    }

    pub fn styleCopy(self: *BarState) style.ProgressStyle {
        self.lock();
        defer self.unlock();
        return self.style.clone();
    }

    pub fn setMessage(self: *BarState, message: []const u8, now: i96) void {
        self.lock();
        defer self.unlock();
        self.state.message = TabExpandedString.init(message, self.tab_width);
        self.updateEstimateAndDrawLocked(now);
    }

    pub fn setPrefix(self: *BarState, prefix: []const u8, now: i96) void {
        self.lock();
        defer self.unlock();
        self.state.prefix = TabExpandedString.init(prefix, self.tab_width);
        self.updateEstimateAndDrawLocked(now);
    }

    pub fn tick(self: *BarState, now: i96) void {
        self.lock();
        defer self.unlock();
        self.state.tick +|= 1;
        self.updateEstimateAndDrawLocked(now);
    }

    fn updateEstimateAndDrawLocked(self: *BarState, now: i96) void {
        self.state.est.record(self.state.pos.pos.load(.monotonic), now);
        self.drawLocked(false, now);
    }

    pub fn println(self: *BarState, now: i96, msg: []const u8) void {
        self.lock();
        defer self.unlock();

        const drawable_width = self.draw_target.width();
        var drawable = self.draw_target.drawable(true, now) orelse return;
        const wrapper = drawable.state();

        var start: usize = 0;
        var i: usize = 0;
        var wrote = false;
        while (i <= msg.len) : (i += 1) {
            if (i == msg.len or msg[i] == '\n') {
                if (i == msg.len and start == i) break;
                var line = msg[start..i];
                if (line.len > 0 and line[line.len - 1] == '\r') line = line[0 .. line.len - 1];
                if (line.len == 0) {
                    wrapper.state.append(.empty);
                } else {
                    wrapper.state.append(dt.LineType.textLine(line));
                }
                wrote = true;
                start = i + 1;
            }
        }
        // An empty message should still trigger a newline.
        if (!wrote) wrapper.state.append(.empty);

        if (drawable_width) |w| {
            if (self.state.status != .done_hidden) {
                var scratch = util.Scratch.init(64 * 1024) catch return;
                defer scratch.deinit();
                self.style.formatState(&self.state, &wrapper.state.lines, w, scratch.allocatorFor());
            }
        }

        wrapper.close();
        drawable.draw();
    }

    pub fn @"suspend"(self: *BarState, now: i96, f: *const fn () void) void {
        self.lock();
        defer self.unlock();

        if (self.draw_target.remote()) |remote| {
            remote.state.@"suspend"(now, f);
            return;
        }

        if (self.draw_target.drawable(true, now)) |drawable| {
            var d = drawable;
            d.clear();
        }

        f();
        self.drawLocked(true, util.nowNs());
    }

    pub fn draw(self: *BarState, force_draw: bool, now: i96) void {
        self.lock();
        defer self.unlock();
        self.drawLocked(force_draw, now);
    }

    fn drawLocked(self: *BarState, force_draw_in: bool, now: i96) void {
        // `|= is_finished()` mirrors Rust: finished bars are always drawn.
        const force_draw = force_draw_in or self.state.isFinished();

        var drawable = self.draw_target.drawable(force_draw, now) orelse return;

        const drawable_width = drawable.width();
        const wrapper = drawable.state();

        if (drawable_width) |w| {
            if (self.state.status != .done_hidden) {
                var scratch = util.Scratch.init(64 * 1024) catch return;
                defer scratch.deinit();
                self.style.formatState(&self.state, &wrapper.state.lines, w, scratch.allocatorFor());
            }
        }

        wrapper.close();
        drawable.draw();
    }
};
