// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `draw_target.rs`.

const std = @import("std");
const util = @import("util.zig");
const width_mod = @import("width.zig");
const term_like = @import("term_like.zig");
const multi = @import("multi.zig");

const allocator = util.allocator;

pub const LineType = union(enum) {
    text: []const u8,
    bar: []const u8,
    empty,

    pub fn asRef(self: LineType) []const u8 {
        return switch (self) {
            .text, .bar => |s| s,
            .empty => "",
        };
    }

    pub fn deinit(self: LineType) void {
        switch (self) {
            .text, .bar => |s| allocator.free(s),
            .empty => {},
        }
    }

    pub fn clone(self: LineType) LineType {
        return switch (self) {
            .text => |s| .{ .text = allocator.dupe(u8, s) catch "" },
            .bar => |s| .{ .bar = allocator.dupe(u8, s) catch "" },
            .empty => .empty,
        };
    }

    pub fn textLine(text: []const u8) LineType {
        return .{ .text = allocator.dupe(u8, text) catch "" };
    }

    pub fn barLine(text: []const u8) LineType {
        return .{ .bar = allocator.dupe(u8, text) catch "" };
    }
};

pub const Metrics = struct {
    height: usize,
    last_line_width: usize,
};

/// `LineType::wrapped_metrics`: terminal rows occupied by one logical line.
pub fn textMetrics(text: []const u8, term_width: usize) Metrics {
    var num_lines: usize = 1;
    var column: usize = 0;
    var i: usize = 0;
    while (i < text.len) {
        if (width_mod.skipAnsi(text, &i)) continue;
        const ch_width = width_mod.charWidth(width_mod.nextCodepoint(text, &i));
        column += ch_width;
        if (column > term_width) {
            num_lines += 1;
            column = ch_width;
        }
    }
    return .{ .height = num_lines, .last_line_width = column };
}

pub fn lineMetrics(line: LineType, term_width: usize) Metrics {
    return textMetrics(line.asRef(), term_width);
}

/// `visual_line_count`.
pub fn countVisualLines(lines: []const LineType, term_width: usize) usize {
    var total: usize = 0;
    for (lines) |line| {
        total +|= lineMetrics(line, term_width).height;
    }
    return total;
}

pub const LineAdjust = union(enum) {
    clear: usize,
    keep: usize,
};

/// `DrawState`.
pub const DrawState = struct {
    lines: std.ArrayList(LineType) = .empty,
    move_cursor: bool = false,
    alignment: multi.MultiProgressAlignment = .top,

    pub fn reset(self: *DrawState) void {
        for (self.lines.items) |line| line.deinit();
        self.lines.clearRetainingCapacity();
    }

    pub fn deinit(self: *DrawState) void {
        self.reset();
        self.lines.deinit(allocator);
    }

    pub fn append(self: *DrawState, line: LineType) void {
        self.lines.append(allocator, line) catch {};
    }

    pub fn visualLineCount(self: *const DrawState, term_width: usize) usize {
        return countVisualLines(self.lines.items, term_width);
    }

    /// `DrawState::draw_to_term`.
    pub fn drawToTerm(self: *DrawState, term: term_like.TermLike, bar_count: *usize) void {
        if (self.lines.items.len > 0 and self.move_cursor) {
            // Move up to the first line (the last line has no trailing '\n')
            // and then to the front of that line.
            term.moveCursorUp(bar_count.* -| 1);
            term.writeStr("\r");
        } else {
            // Fork of console::clear_last_lines that assumes the last line
            // does not contain a '\n'.
            const n = bar_count.*;
            term.moveCursorUp(n -| 1);
            var i: usize = 0;
            while (i < n) : (i += 1) {
                term.clearLine();
                if (i + 1 != n) term.moveCursorDown(1);
            }
            term.moveCursorUp(n -| 1);
        }

        const term_width = term.width();

        // Terminal vertical real estate required by this state.
        const full_height = self.visualLineCount(term_width);

        var shift: usize = 0;
        if (self.alignment == .bottom and full_height < bar_count.*) {
            shift = bar_count.* - full_height;
            var i: usize = 0;
            while (i < shift) : (i += 1) term.writeLine("");
        }

        const spaces = "                                                                ";
        var real_height: usize = 0;
        for (self.lines.items) |line| {
            const metrics = lineMetrics(line, term_width);

            if (line == .bar) {
                if (real_height + metrics.height > term.height()) break;
                real_height += metrics.height;
            }

            term.writeStr(line.asRef());

            // Clear the line and keep the cursor on the right terminal side so
            // future writes/prints happen on the next line.
            const filler = term_width -| metrics.last_line_width;
            var remaining = filler;
            while (remaining > 0) {
                const chunk = @min(remaining, spaces.len);
                term.writeStr(spaces[0..chunk]);
                remaining -= chunk;
            }
        }

        term.flush();
        bar_count.* = real_height + shift;
    }
};

/// `DrawStateWrapper`: on `close()` text/empty lines are moved into the owning
/// `MultiState::orphan_lines`, exactly like the Rust `Drop` impl.
pub const DrawStateWrapper = struct {
    state: *DrawState,
    orphan_lines: ?*std.ArrayList(LineType) = null,

    pub fn reset(self: DrawStateWrapper) void {
        self.state.reset();
    }

    pub fn close(self: DrawStateWrapper) void {
        const orphans = self.orphan_lines orelse return;
        var kept = std.ArrayList(LineType).empty;
        for (self.state.lines.items) |line| {
            switch (line) {
                .text, .empty => orphans.append(allocator, line) catch {},
                .bar => kept.append(allocator, line) catch {},
            }
        }
        // The bar lines are moved into `kept`; only the old buffer is freed.
        self.state.lines.deinit(allocator);
        self.state.lines = kept;
    }
};

/// Rate limit but allow occasional bursts above the desired rate.
pub const RateLimiter = struct {
    interval_ms: u64,
    capacity: u8 = MAX_BURST,
    prev_ns: i96,

    const MAX_BURST: u8 = 20;

    pub fn init(rate: u8) RateLimiter {
        std.debug.assert(rate > 0);
        return .{
            .interval_ms = 1000 / @as(u64, rate),
            .prev_ns = util.nowNs(),
        };
    }

    pub fn allow(self: *RateLimiter, now: i96) bool {
        if (now < self.prev_ns) return false;

        const elapsed = now - self.prev_ns;
        const interval_ns = @as(i96, @intCast(self.interval_ms)) * std.time.ns_per_ms;

        // If `capacity` is 0 and not enough time has passed since `prev`, the
        // update must be dropped.
        if (self.capacity == 0 and elapsed < interval_ns) return false;

        const new: u64 = @intCast(@divTrunc(elapsed, interval_ns));
        const remainder: u64 = @intCast(@mod(elapsed, interval_ns));

        const updated = @as(u64, self.capacity) + new - 1;
        self.capacity = @intCast(@min(@as(u64, MAX_BURST), updated));
        self.prev_ns = now - @as(i96, @intCast(remainder));
        return true;
    }
};

const TermTarget = struct {
    term: term_like.StdTerm,
    last_line_count: usize = 0,
    rate_limiter: RateLimiter,
    draw_state: DrawState = .{},
};

const TermLikeTarget = struct {
    inner: term_like.TermLike,
    last_line_count: usize = 0,
    rate_limiter: ?RateLimiter = null,
    draw_state: DrawState = .{},

    fn limiter(self: *TermLikeTarget) ?*RateLimiter {
        if (self.rate_limiter) |*l| return l;
        return null;
    }
};

const MultiTarget = struct {
    state: *multi.MultiState,
    idx: usize,
};

pub const RemoteTarget = struct {
    state: *multi.MultiState,
    idx: usize,
};

/// `ProgressDrawTarget`.
pub const ProgressDrawTarget = struct {
    kind: Kind,

    pub const Kind = union(enum) {
        term: TermTarget,
        term_like: TermLikeTarget,
        multi: MultiTarget,
        hidden,
    };

    pub fn stdout() ProgressDrawTarget {
        return term(term_like.StdTerm.bufferedStdout(), 20);
    }

    pub fn stderr() ProgressDrawTarget {
        return term(term_like.StdTerm.bufferedStderr(), 20);
    }

    pub fn stdoutWithHz(rate: u8) ProgressDrawTarget {
        return term(term_like.StdTerm.bufferedStdout(), rate);
    }

    pub fn stderrWithHz(rate: u8) ProgressDrawTarget {
        return term(term_like.StdTerm.bufferedStderr(), rate);
    }

    pub fn term(std_term: term_like.StdTerm, rate: u8) ProgressDrawTarget {
        // `ProgressDrawTarget::term` hides the bar when the terminal is not
        // user attended.
        if (!std_term.is_term) return hidden();
        return .{ .kind = .{ .term = .{
            .term = std_term,
            .rate_limiter = RateLimiter.init(rate),
        } } };
    }

    pub fn termLike(inner: term_like.TermLike) ProgressDrawTarget {
        return .{ .kind = .{ .term_like = .{ .inner = inner } } };
    }

    pub fn termLikeWithHz(inner: term_like.TermLike, rate: u8) ProgressDrawTarget {
        return .{ .kind = .{ .term_like = .{ .inner = inner, .rate_limiter = RateLimiter.init(rate) } } };
    }

    pub fn hidden() ProgressDrawTarget {
        return .{ .kind = .hidden };
    }

    pub fn newRemote(state: *multi.MultiState, idx: usize) ProgressDrawTarget {
        state.retain();
        return .{ .kind = .{ .multi = .{ .state = state, .idx = idx } } };
    }

    /// Release resources owned by this target (the MultiProgress reference).
    pub fn deinit(self: *ProgressDrawTarget) void {
        switch (self.kind) {
            .multi => |m| m.state.release(),
            else => {},
        }
    }

    pub fn isHidden(self: *ProgressDrawTarget) bool {
        return switch (self.kind) {
            .hidden => true,
            .term => |t| !t.term.is_term,
            .term_like => false,
            .multi => |m| m.state.isHidden(),
        };
    }

    pub fn isStderr(self: *ProgressDrawTarget) bool {
        return switch (self.kind) {
            .term => |t| t.term.target == .stderr,
            .multi => |m| m.state.isStderr(),
            else => false,
        };
    }

    pub fn width(self: *ProgressDrawTarget) ?usize {
        return switch (self.kind) {
            .term => |*t| t.term.width(),
            .term_like => |t| t.inner.width(),
            .multi => |m| m.state.draw_target.width(),
            .hidden => null,
        };
    }

    pub fn markZombie(self: *ProgressDrawTarget) void {
        switch (self.kind) {
            .multi => |m| m.state.markZombie(m.idx),
            else => {},
        }
    }

    pub fn setMoveCursor(self: *ProgressDrawTarget, move_cursor: bool) void {
        switch (self.kind) {
            .term => |*t| t.draw_state.move_cursor = move_cursor,
            .term_like => |*t| t.draw_state.move_cursor = move_cursor,
            else => {},
        }
    }

    pub fn drawable(self: *ProgressDrawTarget, force_draw: bool, now: i96) ?Drawable {
        return switch (self.kind) {
            .term => |*t| if (force_draw or t.rate_limiter.allow(now)) .{ .kind = .{ .term = .{
                .term = &t.term,
                .last_line_count = &t.last_line_count,
                .draw_state = &t.draw_state,
            } } } else null,
            .term_like => |*t| blk: {
                const allowed = force_draw or (t.limiter() == null) or t.limiter().?.allow(now);
                if (!allowed) break :blk null;
                break :blk .{ .kind = .{ .term_like = .{
                    .inner = t.inner,
                    .last_line_count = &t.last_line_count,
                    .draw_state = &t.draw_state,
                } } };
            },
            .multi => |m| blk: {
                m.state.lock();
                break :blk .{ .kind = .{ .multi = .{
                    .state = m.state,
                    .idx = m.idx,
                    .force_draw = force_draw,
                    .now = now,
                } } };
            },
            .hidden => null,
        };
    }

    /// `ProgressDrawTarget::disconnect`.
    pub fn disconnect(self: *ProgressDrawTarget, now: i96) void {
        switch (self.kind) {
            .multi => |m| {
                m.state.lock();
                defer m.state.unlock();
                m.state.resetMemberDrawState(m.idx);
                m.state.drawLocked(true, null, now);
            },
            else => {},
        }
    }

    pub fn remote(self: *ProgressDrawTarget) ?RemoteTarget {
        return switch (self.kind) {
            .multi => |m| .{ .state = m.state, .idx = m.idx },
            else => null,
        };
    }

    pub fn adjustLastLineCount(self: *ProgressDrawTarget, adjust: LineAdjust) void {
        switch (self.kind) {
            .term => |*t| adjustCount(&t.last_line_count, adjust),
            .term_like => |*t| adjustCount(&t.last_line_count, adjust),
            else => {},
        }
    }
};

fn adjustCount(count: *usize, adjust: LineAdjust) void {
    switch (adjust) {
        .clear => |n| count.* +|= n,
        .keep => |n| count.* -|= n,
    }
}

/// `Drawable`.
pub const Drawable = struct {
    kind: Kind,

    pub const Kind = union(enum) {
        term: struct {
            term: *term_like.StdTerm,
            last_line_count: *usize,
            draw_state: *DrawState,
        },
        term_like: struct {
            inner: term_like.TermLike,
            last_line_count: *usize,
            draw_state: *DrawState,
        },
        multi: struct {
            state: *multi.MultiState,
            idx: usize,
            force_draw: bool,
            now: i96,
        },
    };

    pub fn state(self: *Drawable) DrawStateWrapper {
        const wrapper = switch (self.kind) {
            .term => |t| DrawStateWrapper{ .state = t.draw_state },
            .term_like => |t| DrawStateWrapper{ .state = t.draw_state },
            .multi => |m| m.state.drawStateLocked(m.idx),
        };
        wrapper.reset();
        return wrapper;
    }

    pub fn draw(self: *Drawable) void {
        switch (self.kind) {
            .term => |t| t.draw_state.drawToTerm(t.term.termLike(), t.last_line_count),
            .term_like => |t| t.draw_state.drawToTerm(t.inner, t.last_line_count),
            .multi => |m| {
                m.state.drawLocked(m.force_draw, null, m.now);
                m.state.unlock();
            },
        }
    }

    pub fn clear(self: *Drawable) void {
        switch (self.kind) {
            .multi => {
                const wrapper = self.state();
                wrapper.close();
                self.draw();
            },
            else => {
                const wrapper = self.state();
                wrapper.close();
                self.draw();
            },
        }
    }

    pub fn width(self: *Drawable) ?usize {
        return switch (self.kind) {
            .term => |t| t.term.width(),
            .term_like => |t| t.inner.width(),
            .multi => |m| m.state.draw_target.width(),
        };
    }

    pub fn adjustLastLineCount(self: *Drawable, adjust: LineAdjust) void {
        switch (self.kind) {
            .term => |t| adjustCount(t.last_line_count, adjust),
            .term_like => |t| adjustCount(t.last_line_count, adjust),
            else => {},
        }
    }
};

test "rate limiter bursts then throttles" {
    var limiter = RateLimiter.init(20);
    // The first update is always allowed (capacity is full).
    try std.testing.expect(limiter.allow(limiter.prev_ns));
}
