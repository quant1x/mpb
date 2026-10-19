// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `multi.rs`.

const std = @import("std");
const util = @import("util.zig");
const dt = @import("draw_target.zig");
const progress_bar = @import("progress_bar.zig");

const allocator = util.allocator;
const ProgressBar = progress_bar.ProgressBar;

pub const MultiProgressAlignment = enum { top, bottom };

/// Opaque index into `MultiState::members`.
pub const MultiStateIndex = usize;

const MultiStateMember = struct {
    /// `None` for members that have not been drawn yet and for entries that
    /// correspond to something in the free set.
    draw_state: ?*dt.DrawState = null,
    /// Whether the corresponding `BarState` has been dropped.
    is_zombie: bool = false,
};

const InsertLocation = union(enum) {
    end,
    index: usize,
    index_from_back: usize,
    after: MultiStateIndex,
    before: MultiStateIndex,
};

/// `MultiState`: the shared state behind a `MultiProgress`.
pub const MultiState = struct {
    mutex: util.SpinLock = .{},
    refs: std.atomic.Value(usize) = .init(1),

    members: std.ArrayList(MultiStateMember) = .empty,
    free_set: std.ArrayList(MultiStateIndex) = .empty,
    ordering: std.ArrayList(MultiStateIndex) = .empty,
    draw_target: dt.ProgressDrawTarget,
    alignment: MultiProgressAlignment = .top,
    /// Lines drawn above everything else; fed by `ProgressBar::println`.
    orphan_lines: std.ArrayList(dt.LineType) = .empty,
    zombie_lines_count: usize = 0,

    pub fn init(draw_target: dt.ProgressDrawTarget) MultiState {
        return .{ .draw_target = draw_target };
    }

    pub fn lock(self: *MultiState) void {
        self.mutex.lock();
    }

    pub fn unlock(self: *MultiState) void {
        self.mutex.unlock();
    }

    pub fn retain(self: *MultiState) void {
        _ = self.refs.fetchAdd(1, .monotonic);
    }

    pub fn release(self: *MultiState) void {
        if (self.refs.fetchSub(1, .acq_rel) == 1) self.destroy();
    }

    fn destroy(self: *MultiState) void {
        for (self.members.items) |member| {
            if (member.draw_state) |state| {
                state.deinit();
                allocator.destroy(state);
            }
        }
        self.members.deinit(allocator);
        self.free_set.deinit(allocator);
        self.ordering.deinit(allocator);
        for (self.orphan_lines.items) |line| line.deinit();
        self.orphan_lines.deinit(allocator);
        self.draw_target.deinit();
        allocator.destroy(self);
    }

    pub fn isHidden(self: *MultiState) bool {
        self.lock();
        defer self.unlock();
        return self.draw_target.isHidden();
    }

    pub fn isStderr(self: *MultiState) bool {
        self.lock();
        defer self.unlock();
        return self.draw_target.isStderr();
    }

    pub fn width(self: *MultiState) ?usize {
        self.lock();
        defer self.unlock();
        return self.draw_target.width();
    }

    pub fn markZombie(self: *MultiState, index: MultiStateIndex) void {
        self.lock();
        defer self.unlock();
        self.markZombieLocked(index);
    }

    fn markZombieLocked(self: *MultiState, index: MultiStateIndex) void {
        if (index >= self.members.items.len) return;

        // If the zombie is not the first visual bar, defer it to the next draw.
        if (self.ordering.items.len == 0 or index != self.ordering.items[0]) {
            self.members.items[index].is_zombie = true;
            return;
        }

        var line_count: usize = 0;
        if (self.members.items[index].draw_state) |state| {
            if (self.draw_target.width()) |w| line_count = state.visualLineCount(w);
        }

        // Track the total number of zombie lines on the screen.
        self.zombie_lines_count +|= line_count;

        // Make the draw target forget about the zombie lines so they are not
        // cleared on the next draw.
        self.draw_target.adjustLastLineCount(.{ .keep = line_count });

        self.removeIdxLocked(index);
    }

    /// `MultiState::draw`, called with the lock held.
    pub fn drawLocked(
        self: *MultiState,
        force_draw_in: bool,
        extra_lines: ?[]const dt.LineType,
        now: i96,
    ) void {
        var force_draw = force_draw_in;

        const term_width = self.draw_target.width() orelse return;

        var reap_indices = std.ArrayList(MultiStateIndex).empty;
        defer reap_indices.deinit(allocator);

        // Reap all consecutive zombies from the head of the list.
        var adjust: usize = 0;
        for (self.ordering.items) |index| {
            const member = self.members.items[index];
            if (!member.is_zombie) break;

            var line_count: usize = 0;
            if (member.draw_state) |state| line_count = state.visualLineCount(term_width);
            self.zombie_lines_count +|= line_count;
            adjust +|= line_count;
            reap_indices.append(allocator, index) catch {};
        }

        // A `println` must erase all zombie lines because the message appears
        // above everything else.
        if (extra_lines != null) {
            self.draw_target.adjustLastLineCount(.{ .clear = self.zombie_lines_count });
            self.zombie_lines_count = 0;
        }

        const orphan_visual_lines = dt.countVisualLines(self.orphan_lines.items, term_width);
        force_draw = force_draw or orphan_visual_lines > 0;

        var drawable = self.draw_target.drawable(force_draw, now) orelse {
            if (extra_lines) |lines| for (lines) |line| line.deinit();
            return;
        };
        const draw_state = drawable.state();

        if (extra_lines) |lines| {
            for (lines) |line| draw_state.state.append(line);
        }

        // Orphan lines are moved into this frame, so they are only shown once.
        for (self.orphan_lines.items) |line| draw_state.state.append(line);
        self.orphan_lines.clearRetainingCapacity();

        for (self.ordering.items) |index| {
            const member = self.members.items[index];
            if (member.draw_state) |state| {
                for (state.lines.items) |line| draw_state.state.append(line.clone());
            }
        }

        draw_state.close();
        drawable.draw();

        for (reap_indices.items) |index| self.removeIdxLocked(index);

        // The zombie lines were drawn for the last time; forget about them.
        if (extra_lines == null) {
            self.draw_target.adjustLastLineCount(.{ .keep = adjust });
        }
    }

    pub fn println(self: *MultiState, msg: []const u8, now: i96) void {
        var lines = std.ArrayList(dt.LineType).empty;
        // Ownership of the line contents is moved into the draw state below.
        defer lines.deinit(allocator);

        if (msg.len == 0) {
            lines.append(allocator, .empty) catch {};
        } else {
            var start: usize = 0;
            var i: usize = 0;
            while (i <= msg.len) : (i += 1) {
                if (i == msg.len or msg[i] == '\n') {
                    // `str::lines` does not yield a final empty line for a
                    // trailing newline.
                    if (i == msg.len and start == i) break;
                    var line = msg[start..i];
                    if (line.len > 0 and line[line.len - 1] == '\r') line = line[0 .. line.len - 1];
                    if (line.len == 0) {
                        lines.append(allocator, .empty) catch {};
                    } else {
                        lines.append(allocator, dt.LineType.textLine(line)) catch {};
                    }
                    start = i + 1;
                }
            }
        }

        self.lock();
        defer self.unlock();
        self.drawLocked(true, lines.items, now);
    }

    /// `MultiState::draw_state`: persistent per-member state, with the orphan
    /// collector attached. Called with the lock held.
    pub fn drawStateLocked(self: *MultiState, idx: MultiStateIndex) dt.DrawStateWrapper {
        const member = &self.members.items[idx];
        if (member.draw_state == null) {
            const state = allocator.create(dt.DrawState) catch unreachable;
            state.* = .{};
            member.draw_state = state;
        }
        return .{ .state = member.draw_state.?, .orphan_lines = &self.orphan_lines };
    }

    pub fn resetMemberDrawState(self: *MultiState, idx: MultiStateIndex) void {
        if (idx >= self.members.items.len) return;
        if (self.members.items[idx].draw_state) |state| state.reset();
    }

    pub fn @"suspend"(self: *MultiState, now: i96, f: *const fn () void) void {
        self.lock();
        defer self.unlock();
        self.clearLocked(now);
        f();
        self.drawLocked(true, null, util.nowNs());
    }

    pub fn clear(self: *MultiState, now: i96) void {
        self.lock();
        defer self.unlock();
        self.clearLocked(now);
    }

    fn clearLocked(self: *MultiState, now: i96) void {
        var drawable = self.draw_target.drawable(true, now) orelse return;
        drawable.adjustLastLineCount(.{ .clear = self.zombie_lines_count });
        self.zombie_lines_count = 0;
        drawable.clear();
    }

    fn insert(self: *MultiState, location: InsertLocation) MultiStateIndex {
        const idx: MultiStateIndex = if (self.free_set.pop()) |free| blk: {
            self.members.items[free] = .{};
            break :blk free;
        } else blk: {
            self.members.append(allocator, .{}) catch unreachable;
            break :blk self.members.items.len - 1;
        };

        switch (location) {
            .end => self.ordering.append(allocator, idx) catch {},
            .index => |pos| {
                const clamped = @min(pos, self.ordering.items.len);
                self.ordering.insert(allocator, clamped, idx) catch {};
            },
            .index_from_back => |pos| {
                const clamped = self.ordering.items.len -| pos;
                self.ordering.insert(allocator, clamped, idx) catch {};
            },
            .after => |after_idx| {
                const pos = self.visualIndex(after_idx) + 1;
                self.ordering.insert(allocator, pos, idx) catch {};
            },
            .before => |before_idx| {
                const pos = self.visualIndex(before_idx);
                self.ordering.insert(allocator, pos, idx) catch {};
            },
        }

        std.debug.assert(self.len() == self.ordering.items.len);
        return idx;
    }

    fn removeIdxLocked(self: *MultiState, idx: MultiStateIndex) void {
        if (idx >= self.members.items.len) return;
        if (std.mem.indexOfScalar(usize, self.free_set.items, idx) != null) return;

        if (self.members.items[idx].draw_state) |state| {
            state.deinit();
            allocator.destroy(state);
        }
        self.members.items[idx] = .{};
        self.free_set.append(allocator, idx) catch {};

        for (self.ordering.items, 0..) |value, i| {
            if (value == idx) {
                _ = self.ordering.orderedRemove(i);
                break;
            }
        }

        std.debug.assert(self.len() == self.ordering.items.len);
    }

    pub fn len(self: *MultiState) usize {
        return self.members.items.len - self.free_set.items.len;
    }

    /// Map from opaque index to position on screen.
    pub fn visualIndex(self: *MultiState, opaque_index: MultiStateIndex) usize {
        for (self.ordering.items, 0..) |value, i| {
            if (value == opaque_index) return i;
        }
        @panic("no such member");
    }
};

/// `MultiProgress`.
pub const MultiProgress = struct {
    state: *MultiState,

    pub fn init() MultiProgress {
        return withDrawTarget(dt.ProgressDrawTarget.stderr());
    }

    pub fn withDrawTarget(draw_target: dt.ProgressDrawTarget) MultiProgress {
        const state = allocator.create(MultiState) catch unreachable;
        state.* = MultiState.init(draw_target);
        return .{ .state = state };
    }

    pub fn deinit(self: *MultiProgress) void {
        self.state.release();
    }

    pub fn setDrawTarget(self: *MultiProgress, target: dt.ProgressDrawTarget) void {
        self.state.lock();
        defer self.state.unlock();
        self.state.draw_target.disconnect(util.nowNs());
        self.state.draw_target.deinit();
        self.state.draw_target = target;
    }

    pub fn setMoveCursor(self: *MultiProgress, move_cursor: bool) void {
        self.state.lock();
        defer self.state.unlock();
        self.state.draw_target.setMoveCursor(move_cursor);
    }

    pub fn setAlignment(self: *MultiProgress, alignment: MultiProgressAlignment) void {
        self.state.lock();
        defer self.state.unlock();
        self.state.alignment = alignment;
    }

    pub fn add(self: *MultiProgress, pb: ProgressBar) ProgressBar {
        return self.internalize(.end, pb);
    }

    pub fn insert(self: *MultiProgress, index: usize, pb: ProgressBar) ProgressBar {
        return self.internalize(.{ .index = index }, pb);
    }

    pub fn insertFromBack(self: *MultiProgress, index: usize, pb: ProgressBar) ProgressBar {
        return self.internalize(.{ .index_from_back = index }, pb);
    }

    pub fn insertBefore(self: *MultiProgress, before: ProgressBar, pb: ProgressBar) ProgressBar {
        const idx = before.visualIndex() orelse @panic("no such member");
        return self.internalize(.{ .before = idx }, pb);
    }

    pub fn insertAfter(self: *MultiProgress, after: ProgressBar, pb: ProgressBar) ProgressBar {
        const idx = after.visualIndex() orelse @panic("no such member");
        return self.internalize(.{ .after = idx }, pb);
    }

    pub fn remove(self: *MultiProgress, pb: ProgressBar) void {
        const remote = pb.remoteTarget() orelse return;
        std.debug.assert(remote.state == self.state);

        pb.setDrawTargetDirect(dt.ProgressDrawTarget.hidden());

        self.state.lock();
        defer self.state.unlock();
        self.state.removeIdxLocked(remote.idx);
    }

    fn internalize(self: *MultiProgress, location: InsertLocation, pb: ProgressBar) ProgressBar {
        self.state.lock();
        const idx = self.state.insert(location);
        self.state.unlock();

        pb.setDrawTarget(dt.ProgressDrawTarget.newRemote(self.state, idx));
        return pb;
    }

    pub fn println(self: *MultiProgress, msg: []const u8) void {
        self.state.println(msg, util.nowNs());
    }

    pub fn @"suspend"(self: *MultiProgress, f: *const fn () void) void {
        self.state.@"suspend"(util.nowNs(), f);
    }

    pub fn clear(self: *MultiProgress) void {
        self.state.clear(util.nowNs());
    }

    pub fn isHidden(self: *MultiProgress) bool {
        return self.state.isHidden();
    }
};

test "multi inserts reuse free slots" {
    var mp = MultiProgress.withDrawTarget(dt.ProgressDrawTarget.hidden());
    defer mp.deinit();
    var p0 = mp.add(ProgressBar.new(1));
    var p1 = mp.add(ProgressBar.new(1));
    var p2 = mp.add(ProgressBar.new(1));
    defer p0.deinit();
    defer p1.deinit();
    defer p2.deinit();
    mp.remove(p1);
    try std.testing.expectEqual(@as(usize, 2), mp.state.len());

    var p3 = mp.insert(1, ProgressBar.new(1));
    defer p3.deinit();
    try std.testing.expectEqual(@as(usize, 3), mp.state.len());
    try std.testing.expectEqual(@as(usize, 1), p3.visualIndex().?);
}

test "multi renders println above the bars and drops zombies" {
    const term_like = @import("term_like.zig");

    var term = try term_like.InMemoryTerm.init(10, 60);
    defer term.deinit();

    var mp = MultiProgress.withDrawTarget(dt.ProgressDrawTarget.termLike(term.termLike()));
    defer mp.deinit();

    var pb = mp.add(ProgressBar.new(10));
    pb.forceDraw();
    try std.testing.expect(std.mem.indexOf(u8, term.contents(), "0/10") != null);

    mp.println("hello");
    const contents = term.contents();
    const hello = std.mem.indexOf(u8, contents, "hello").?;
    const bar = std.mem.indexOf(u8, contents, "0/10").?;
    try std.testing.expect(hello < bar);

    // Dropping a finished bar (DoneHidden) removes its line on the next draw.
    pb.deinit();
    try std.testing.expect(std.mem.indexOf(u8, term.contents(), "0/10") == null);
}

test "multi redraw replaces the previous frame" {
    const term_like = @import("term_like.zig");

    var term = try term_like.InMemoryTerm.init(20, 80);
    defer term.deinit();

    var mp = MultiProgress.withDrawTarget(dt.ProgressDrawTarget.termLike(term.termLike()));
    defer mp.deinit();

    var first = mp.add(ProgressBar.new(10));
    defer first.deinit();
    var second = mp.add(ProgressBar.new(10));
    defer second.deinit();

    first.forceDraw();
    second.forceDraw();
    first.inc(1);
    second.inc(1);
    first.forceDraw();

    const contents = term.contents();
    var count: usize = 0;
    var index: usize = 0;
    while (std.mem.indexOfPos(u8, contents, index, "1/10")) |found| {
        count += 1;
        index = found + 4;
    }
    try std.testing.expectEqual(@as(usize, 2), count);
}
