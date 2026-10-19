// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `progress_bar.rs`.

const std = @import("std");
const util = @import("util.zig");
const dt = @import("draw_target.zig");
const state_mod = @import("state.zig");
const style_mod = @import("style.zig");

const allocator = util.allocator;

pub const ProgressBar = struct {
    state: *state_mod.BarState,
    pos: *state_mod.AtomicPosition,

    pub fn init(len: ?u64, draw_target: dt.ProgressDrawTarget) ProgressBar {
        const pos = allocator.create(state_mod.AtomicPosition) catch unreachable;
        pos.* = state_mod.AtomicPosition.init();

        const state = allocator.create(state_mod.BarState) catch unreachable;
        state.* = state_mod.BarState.init(len, draw_target, pos);

        return .{ .state = state, .pos = pos };
    }

    pub fn new(len: u64) ProgressBar {
        return init(len, dt.ProgressDrawTarget.stderr());
    }

    pub fn noLength() ProgressBar {
        return init(null, dt.ProgressDrawTarget.stderr());
    }

    pub fn hidden() ProgressBar {
        return init(null, dt.ProgressDrawTarget.hidden());
    }

    pub fn newSpinner() ProgressBar {
        var bar = init(null, dt.ProgressDrawTarget.stderr());
        bar.setStyle(style_mod.ProgressStyle.defaultSpinner());
        return bar;
    }

    pub fn clone(self: ProgressBar) ProgressBar {
        self.state.retain();
        return .{ .state = self.state, .pos = self.pos };
    }

    pub fn deinit(self: *ProgressBar) void {
        self.disableSteadyTick();
        self.state.release();
    }

    pub fn withStyle(self: ProgressBar, new_style: style_mod.ProgressStyle) ProgressBar {
        self.setStyle(new_style);
        return self;
    }

    pub fn withPrefix(self: ProgressBar, text: []const u8) ProgressBar {
        self.setPrefix(text);
        return self;
    }

    pub fn withMessage(self: ProgressBar, text: []const u8) ProgressBar {
        self.setMessage(text);
        return self;
    }

    pub fn withPosition(self: ProgressBar, value: u64) ProgressBar {
        self.state.lock();
        defer self.state.unlock();
        self.state.state.pos.set(value);
        return self;
    }

    pub fn withElapsed(self: ProgressBar, elapsed_ns: i96) ProgressBar {
        self.state.lock();
        defer self.state.unlock();
        self.state.state.started = util.nowNs() - elapsed_ns;
        return self;
    }

    pub fn withFinish(self: ProgressBar, finish_kind: state_mod.ProgressFinish) ProgressBar {
        self.state.lock();
        defer self.state.unlock();
        self.state.on_finish = finish_kind;
        return self;
    }

    pub fn style(self: ProgressBar) style_mod.ProgressStyle {
        return self.state.styleCopy();
    }

    pub fn setStyle(self: ProgressBar, new_style: style_mod.ProgressStyle) void {
        self.state.setStyle(new_style);
    }

    pub fn setTabWidth(self: ProgressBar, tab_width: usize) void {
        self.state.setTabWidth(tab_width);
        self.state.draw(true, util.nowNs());
    }

    pub fn tick(self: ProgressBar) void {
        if (self.state.ticker != null) return;
        self.state.tick(util.nowNs());
    }

    fn tickInner(self: ProgressBar, now: i96) void {
        if (self.state.ticker != null) return;
        self.state.tick(now);
    }

    pub fn inc(self: ProgressBar, delta: u64) void {
        self.pos.inc(delta);
        const now = util.nowNs();
        if (self.pos.allow(now)) self.tickInner(now);
    }

    pub fn dec(self: ProgressBar, delta: u64) void {
        self.pos.dec(delta);
        const now = util.nowNs();
        if (self.pos.allow(now)) self.tickInner(now);
    }

    pub fn isHidden(self: ProgressBar) bool {
        self.state.lock();
        defer self.state.unlock();
        return self.state.draw_target.isHidden();
    }

    pub fn isFinished(self: ProgressBar) bool {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.isFinished();
    }

    pub fn println(self: ProgressBar, msg: []const u8) void {
        self.state.println(util.nowNs(), msg);
    }

    pub fn setPosition(self: ProgressBar, value: u64) void {
        self.pos.set(value);
        const now = util.nowNs();
        if (self.pos.allow(now)) self.tickInner(now);
    }

    pub fn unsetLength(self: ProgressBar) void {
        self.state.unsetLength(util.nowNs());
    }

    pub fn setLength(self: ProgressBar, len: u64) void {
        self.state.setLength(util.nowNs(), len);
    }

    pub fn incLength(self: ProgressBar, delta: u64) void {
        self.state.incLength(util.nowNs(), delta);
    }

    pub fn decLength(self: ProgressBar, delta: u64) void {
        self.state.decLength(util.nowNs(), delta);
    }

    pub fn setPrefix(self: ProgressBar, text: []const u8) void {
        self.state.setPrefix(text, util.nowNs());
    }

    pub fn setMessage(self: ProgressBar, text: []const u8) void {
        self.state.setMessage(text, util.nowNs());
    }

    pub fn setElapsed(self: ProgressBar, elapsed_ns: i96) void {
        self.state.lock();
        defer self.state.unlock();
        self.state.state.started = util.nowNs() - elapsed_ns;
    }

    pub fn resetEta(self: ProgressBar) void {
        self.state.reset(util.nowNs(), .eta);
    }

    pub fn resetElapsed(self: ProgressBar) void {
        self.state.reset(util.nowNs(), .elapsed);
    }

    pub fn reset(self: ProgressBar) void {
        self.state.reset(util.nowNs(), .all);
    }

    pub fn finish(self: ProgressBar) void {
        self.state.finishUsingStyle(util.nowNs(), .{ .kind = .and_leave });
    }

    pub fn finishWithMessage(self: ProgressBar, msg: []const u8) void {
        self.state.finishUsingStyle(util.nowNs(), .{ .kind = .with_message, .message = msg });
    }

    pub fn finishAndClear(self: ProgressBar) void {
        self.state.finishUsingStyle(util.nowNs(), .{ .kind = .and_clear });
    }

    pub fn abandon(self: ProgressBar) void {
        self.state.finishUsingStyle(util.nowNs(), .{ .kind = .abandon });
    }

    pub fn abandonWithMessage(self: ProgressBar, msg: []const u8) void {
        self.state.finishUsingStyle(util.nowNs(), .{ .kind = .abandon_with_message, .message = msg });
    }

    pub fn finishUsingStyle(self: ProgressBar) void {
        self.state.lock();
        const finish_kind = self.state.on_finish;
        self.state.unlock();
        self.state.finishUsingStyle(util.nowNs(), finish_kind);
    }

    pub fn forceDraw(self: ProgressBar) void {
        self.state.draw(true, util.nowNs());
    }

    pub fn @"suspend"(self: ProgressBar, f: *const fn () void) void {
        self.state.@"suspend"(util.nowNs(), f);
    }

    pub fn position(self: ProgressBar) u64 {
        return self.pos.pos.load(.monotonic);
    }

    pub fn length(self: ProgressBar) ?u64 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.len;
    }

    pub fn eta(self: ProgressBar) i96 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.eta();
    }

    pub fn perSec(self: ProgressBar) f64 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.perSec();
    }

    pub fn duration(self: ProgressBar) i96 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.duration();
    }

    pub fn elapsed(self: ProgressBar) i96 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.elapsed();
    }

    pub fn tabWidth(self: ProgressBar) usize {
        self.state.lock();
        defer self.state.unlock();
        return self.state.tab_width;
    }

    pub fn message(self: ProgressBar) []const u8 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.message.expanded();
    }

    pub fn prefix(self: ProgressBar) []const u8 {
        self.state.lock();
        defer self.state.unlock();
        return self.state.state.prefix.expanded();
    }

    /// Position of this bar inside its `MultiProgress`, if any.
    pub fn visualIndex(self: ProgressBar) ?usize {
        self.state.lock();
        const remote = self.state.draw_target.remote();
        self.state.unlock();
        const target = remote orelse return null;

        target.state.lock();
        defer target.state.unlock();
        return target.state.visualIndex(target.idx);
    }

    pub fn setDrawTarget(self: ProgressBar, target: dt.ProgressDrawTarget) void {
        self.state.lock();
        defer self.state.unlock();
        self.state.draw_target.disconnect(util.nowNs());
        self.state.draw_target.deinit();
        self.state.draw_target = target;
    }

    /// Direct assignment without `disconnect`, used by `MultiProgress::remove`.
    pub fn setDrawTargetDirect(self: ProgressBar, target: dt.ProgressDrawTarget) void {
        self.state.lock();
        defer self.state.unlock();
        self.state.draw_target.deinit();
        self.state.draw_target = target;
    }

    pub fn remoteTarget(self: ProgressBar) ?dt.RemoteTarget {
        self.state.lock();
        defer self.state.unlock();
        return self.state.draw_target.remote();
    }

    /// Spawns a background thread to tick the bar.
    pub fn enableSteadyTick(self: ProgressBar, interval_ns: i96) void {
        self.disableSteadyTick();
        const ticker = Ticker.spawn(self, interval_ns) orelse return;
        self.state.lock();
        defer self.state.unlock();
        self.state.ticker = ticker;
    }

    pub fn disableSteadyTick(self: ProgressBar) void {
        self.state.lock();
        const ticker = self.state.ticker;
        self.state.ticker = null;
        self.state.unlock();

        if (ticker) |ptr| {
            const t: *Ticker = @ptrCast(@alignCast(ptr));
            t.stopAndJoin();
            allocator.destroy(t);
        }
    }
};

const Ticker = struct {
    thread: std.Thread,
    stop: std.atomic.Value(bool) = .init(false),

    fn spawn(pb: ProgressBar, interval_ns: i96) ?*Ticker {
        const ticker = allocator.create(Ticker) catch return null;
        // The thread keeps the bar alive for as long as it runs.
        var held = pb.clone();
        ticker.* = .{ .thread = undefined };
        ticker.thread = std.Thread.spawn(.{}, run, .{ ticker, held, interval_ns }) catch {
            held.deinit();
            allocator.destroy(ticker);
            return null;
        };
        return ticker;
    }

    fn run(self: *Ticker, pb: ProgressBar, interval_ns: i96) void {
        defer pb.state.release();

        while (!self.stop.load(.monotonic)) {
            pb.state.tick(util.nowNs());
            if (pb.isFinished()) break;
            std.Io.sleep(util.io, .fromNanoseconds(interval_ns), .awake) catch break;
        }
    }

    fn stopAndJoin(self: *Ticker) void {
        self.stop.store(true, .monotonic);
        self.thread.join();
    }
};

test "single bar renders into a term-like target" {
    const term_like = @import("term_like.zig");

    var term = try term_like.InMemoryTerm.init(10, 60);
    defer term.deinit();

    var pb = ProgressBar.init(10, dt.ProgressDrawTarget.termLike(term.termLike()));
    defer pb.deinit();

    pb.inc(3);
    pb.forceDraw();
    try std.testing.expect(std.mem.indexOf(u8, term.contents(), "3/10") != null);
}
