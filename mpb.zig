// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! mpb Zig port (indicatif 0.18.6 semantics).
//!
//! This is the first core milestone: template/style, width, rate limiting,
//! single progress bar and the in-memory terminal.  MultiProgress will be built
//! on top of the same BarState/DrawTarget design.

const std = @import("std");
const uw = @import("unicode_width.zig");

pub const allocator = std.heap.page_allocator;
const io = std.Io.Threaded.global_single_threaded.io();

pub const LineType = enum { text, bar, empty };

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

// ---------------------------------------------------------------------------
// Width
// ---------------------------------------------------------------------------

fn findRange(cp: u21) ?uw.Range {
    var lo: usize = 0;
    var hi: usize = uw.widths.len;
    while (lo < hi) {
        const mid = lo + (hi - lo) / 2;
        if (uw.widths[mid].last < cp) {
            lo = mid + 1;
        } else {
            hi = mid;
        }
    }
    if (lo < uw.widths.len and uw.widths[lo].first <= cp and cp <= uw.widths[lo].last) {
        return uw.widths[lo];
    }
    return null;
}

pub fn charWidth(cp: u21, cjk: bool) usize {
    if (findRange(cp)) |range| return if (cjk) range.width_cjk else range.width;
    return 1;
}

fn nextCodepoint(text: []const u8, index: *usize) u21 {
    if (index.* >= text.len) return 0;
    const first = text[index.*];
    const len = std.unicode.utf8ByteSequenceLength(first) catch {
        index.* += 1;
        return 0xFFFD;
    };
    if (index.* + len > text.len) {
        index.* += 1;
        return 0xFFFD;
    }
    const cp = std.unicode.utf8Decode(text[index.* .. index.* + len]) catch 0xFFFD;
    index.* += @intCast(len);
    return cp;
}

pub fn strWidth(text: []const u8) usize {
    var width: usize = 0;
    var i: usize = 0;
    while (i < text.len) {
        const cp = nextCodepoint(text, &i);
        width += charWidth(cp, false);
    }
    return width;
}

pub fn ansiStrWidth(text: []const u8) usize {
    var width: usize = 0;
    var i: usize = 0;
    while (i < text.len) {
        if (text[i] == 0x1B) {
            i += 1;
            if (i < text.len and text[i] == '[') {
                i += 1;
                while (i < text.len) : (i += 1) {
                    const c = text[i];
                    if ((c >= 'A' and c <= 'Z') or (c >= 'a' and c <= 'z')) break;
                }
                if (i < text.len) i += 1;
            }
            continue;
        }
        const before = i;
        const cp = nextCodepoint(text, &i);
        if (i == before) i += 1;
        width += charWidth(cp, false);
    }
    return width;
}

fn padAndTruncate(buf: []const u8, width: usize, alignment: u8) []const u8 {
    const cols = ansiStrWidth(buf);
    if (cols >= width) return buf;
    const pad = width - cols;
    var out = std.ArrayList(u8).empty;
    defer out.deinit(allocator);
    switch (alignment) {
        '^' => {
            const left = pad / 2;
            out.appendNTimes(allocator, ' ', left) catch return buf;
            out.appendSlice(allocator, buf) catch return buf;
            out.appendNTimes(allocator, ' ', pad - left) catch return buf;
        },
        '>' => {
            out.appendNTimes(allocator, ' ', pad) catch return buf;
            out.appendSlice(allocator, buf) catch return buf;
        },
        else => {
            out.appendSlice(allocator, buf) catch return buf;
            out.appendNTimes(allocator, ' ', pad) catch return buf;
        },
    }
    return out.toOwnedSlice(allocator) catch buf;
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

pub fn humanBytes(value: u64) []const u8 {
    const units = [_][]const u8{ "KiB", "MiB", "GiB", "TiB", "PiB" };
    if (value < 1024) return std.fmt.allocPrint(allocator, "{d} B", .{value}) catch "";
    const number: f64 = @floatFromInt(value);
    var divisor: f64 = 1024;
    for (units, 0..) |unit, i| {
        if (number < divisor * 1024 or i + 1 == units.len) {
            return std.fmt.allocPrint(allocator, "{d:.2} {s}", .{ number / divisor, unit }) catch "";
        }
        divisor *= 1024;
    }
    return "";
}

pub fn humanDuration(ns: i96) []const u8 {
    const seconds: f64 = @as(f64, @floatFromInt(ns)) / @as(f64, @floatFromInt(std.time.ns_per_s));
    const units = [_]struct { seconds: f64, name: []const u8, short: []const u8 }{
        .{ .seconds = 365 * 86400, .name = "year", .short = "y" },
        .{ .seconds = 7 * 86400, .name = "week", .short = "w" },
        .{ .seconds = 86400, .name = "day", .short = "d" },
        .{ .seconds = 3600, .name = "hour", .short = "h" },
        .{ .seconds = 60, .name = "minute", .short = "m" },
        .{ .seconds = 1, .name = "second", .short = "s" },
    };
    var index: usize = 0;
    for (units, 0..) |unit, i| {
        index = i;
        if (i + 1 < units.len) {
            const next = units[i + 1].seconds;
            if (seconds + next / 2 >= unit.seconds + unit.seconds / 2) break;
        }
    }
    var value: i64 = @intFromFloat(@round(seconds / units[index].seconds));
    if (index + 1 < units.len and value < 2) value = 2;
    if (value == 1) return std.fmt.allocPrint(allocator, "{d} {s}", .{ value, units[index].name }) catch "";
    return std.fmt.allocPrint(allocator, "{d} {s}s", .{ value, units[index].name }) catch "";
}

pub fn humanFloatCount(value: f64, precision: usize) []const u8 {
    _ = precision;
    if (std.math.isNan(value)) return "NaN";
    if (std.math.isInf(value)) return if (value > 0) "+Inf" else "-Inf";
    return std.fmt.allocPrint(allocator, "{d:.4}", .{value}) catch "";
}

// ---------------------------------------------------------------------------
// Template and style
// ---------------------------------------------------------------------------

const Part = struct {
    placeholder: bool = false,
    text: []const u8 = "",
    key: []const u8 = "",
    alignment: u8 = '<',
    width: usize = 0,
    truncate: bool = false,
    style: []const u8 = "",
    alt_style: []const u8 = "",
};

const Template = struct {
    parts: []Part,
};

const ParseState = enum { literal, maybe_open, key, alignment, width, first_style, alt_style, double_close };

fn templateFromStr(text: []const u8) error{InvalidTemplate}!Template {
    var parts = std.ArrayList(Part).empty;
    defer parts.deinit(allocator);
    var buf = std.ArrayList(u8).empty;
    defer buf.deinit(allocator);
    var state: ParseState = .literal;

    for (text) |c| {
        var new_state = state;
        var keep: ?u8 = null;
        switch (state) {
            .literal => if (c == '{') {
                new_state = .maybe_open;
            } else if (c == '}') {
                new_state = .double_close;
                keep = c;
            } else {
                keep = c;
            },
            .double_close => {
                if (c != '}') return error.InvalidTemplate;
                new_state = .literal;
            },
            .maybe_open => if (c == '{') {
                new_state = .literal;
                keep = c;
            } else if (c == ' ' or c == '\t' or c == '\n' or c == '\r') {
                var lit = std.ArrayList(u8).empty;
                defer lit.deinit(allocator);
                lit.append(allocator, '{') catch return error.InvalidTemplate;
                lit.appendSlice(allocator, buf.items) catch return error.InvalidTemplate;
                lit.append(allocator, c) catch return error.InvalidTemplate;
                parts.append(allocator, .{ .text = lit.toOwnedSlice(allocator) catch return error.InvalidTemplate }) catch return error.InvalidTemplate;
                buf.clearRetainingCapacity();
                new_state = .literal;
            } else if (c != '}' and c != ':') {
                new_state = .key;
                keep = c;
            } else return error.InvalidTemplate,
            .key => if (c == ' ' or c == '\t' or c == '\n' or c == '\r') {
                var lit = std.ArrayList(u8).empty;
                defer lit.deinit(allocator);
                lit.append(allocator, '{') catch return error.InvalidTemplate;
                lit.appendSlice(allocator, buf.items) catch return error.InvalidTemplate;
                lit.append(allocator, c) catch return error.InvalidTemplate;
                parts.append(allocator, .{ .text = lit.toOwnedSlice(allocator) catch return error.InvalidTemplate }) catch return error.InvalidTemplate;
                buf.clearRetainingCapacity();
                new_state = .literal;
            } else if (c == '}' or c == ':') {
                new_state = if (c == '}') .literal else .alignment;
            } else if (c == '!' and buf.items.len > 0) {
                parts.append(allocator, .{ .placeholder = true, .key = buf.toOwnedSlice(allocator) catch return error.InvalidTemplate, .truncate = true }) catch return error.InvalidTemplate;
                buf = .empty;
                new_state = .width;
            } else {
                keep = c;
            },
            .alignment => if (c == '<' or c == '^' or c == '>') {
                if (parts.items.len > 0 and parts.items[parts.items.len - 1].placeholder) {
                    parts.items[parts.items.len - 1].alignment = c;
                }
                new_state = .width;
            } else if (c >= '0' and c <= '9') {
                new_state = .width;
                keep = c;
            } else if (c == '!') {
                if (parts.items.len > 0 and parts.items[parts.items.len - 1].placeholder) {
                    parts.items[parts.items.len - 1].truncate = true;
                }
                new_state = .width;
            } else if (c == '.') {
                new_state = .first_style;
            } else if (c == '}') {
                new_state = .literal;
            } else return error.InvalidTemplate,
            .width => if (c >= '0' and c <= '9') {
                keep = c;
            } else if (c == '!') {
                if (parts.items.len > 0 and parts.items[parts.items.len - 1].placeholder) {
                    parts.items[parts.items.len - 1].truncate = true;
                }
            } else if (c == '.') {
                new_state = .first_style;
            } else if (c == '}') {
                new_state = .literal;
            } else return error.InvalidTemplate,
            .first_style => {
                if (c == '/') {
                    new_state = .alt_style;
                } else if (c == '}') {
                    new_state = .literal;
                } else {
                    keep = c;
                }
            },
            .alt_style => {
                if (c == '}') {
                    new_state = .literal;
                } else {
                    keep = c;
                }
            },
        }

        if (state == .maybe_open and new_state == .key and buf.items.len > 0) {
            parts.append(allocator, .{ .text = buf.toOwnedSlice(allocator) catch return error.InvalidTemplate }) catch return error.InvalidTemplate;
            buf = .empty;
        } else if (state == .key and (new_state == .alignment or new_state == .literal) and buf.items.len > 0) {
            parts.append(allocator, .{ .placeholder = true, .key = buf.toOwnedSlice(allocator) catch return error.InvalidTemplate }) catch return error.InvalidTemplate;
            buf = .empty;
        } else if (state == .width and (new_state == .first_style or new_state == .literal) and buf.items.len > 0) {
            if (parts.items.len > 0 and parts.items[parts.items.len - 1].placeholder) {
                parts.items[parts.items.len - 1].width = std.fmt.parseInt(usize, buf.items, 10) catch 0;
            }
            buf.clearRetainingCapacity();
        } else if (state == .first_style and (new_state == .alt_style or new_state == .literal) and buf.items.len > 0) {
            if (parts.items.len > 0 and parts.items[parts.items.len - 1].placeholder) {
                parts.items[parts.items.len - 1].style = buf.toOwnedSlice(allocator) catch return error.InvalidTemplate;
                buf = .empty;
            }
        } else if (state == .alt_style and new_state == .literal and buf.items.len > 0) {
            if (parts.items.len > 0 and parts.items[parts.items.len - 1].placeholder) {
                parts.items[parts.items.len - 1].alt_style = buf.toOwnedSlice(allocator) catch return error.InvalidTemplate;
                buf = .empty;
            }
        }

        state = new_state;
        if (keep) |ch| buf.append(allocator, ch) catch return error.InvalidTemplate;
    }
    if (buf.items.len > 0 and (state == .literal or state == .double_close)) {
        parts.append(allocator, .{ .text = buf.toOwnedSlice(allocator) catch return error.InvalidTemplate }) catch return error.InvalidTemplate;
    }
    return .{ .parts = parts.toOwnedSlice(allocator) catch return error.InvalidTemplate };
}

const Style = struct {
    template: Template = undefined,
    progress_chars: [2][]const u8 = .{ "█", "░" },
    tick_strings: []const []const u8 = &.{},
    tab_width: usize = 8,

    fn init(template_text: []const u8) !Style {
        return .{ .template = try templateFromStr(template_text) };
    }

    fn setProgressChars(self: *Style, chars: []const u8) void {
        if (chars.len == 0) {
            self.progress_chars = .{ "#", "-" };
            return;
        }
        // rune-aware split
        var i: usize = 0;
        var count: usize = 0;
        while (i < chars.len) {
            _ = nextCodepoint(chars, &i);
            count += 1;
        }
        var out = std.ArrayList([]const u8).empty;
        i = 0;
        while (i < chars.len) {
            const start = i;
            _ = nextCodepoint(chars, &i);
            out.append(allocator, chars[start..i]) catch return;
        }
        self.progress_chars = .{ out.items[0], out.items[out.items.len - 1] };
    }
};

fn applyStyle(text: []const u8, spec: []const u8) []const u8 {
    if (spec.len == 0) return text;
    var out = std.ArrayList(u8).empty;
    defer out.deinit(allocator);
    var reset = false;
    var it = std.mem.splitScalar(u8, spec, '.');
    while (it.next()) |token| {
        if (std.mem.eql(u8, token, "red")) {
            out.appendSlice(allocator, "\x1b[31m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "green")) {
            out.appendSlice(allocator, "\x1b[32m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "yellow")) {
            out.appendSlice(allocator, "\x1b[33m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "blue")) {
            out.appendSlice(allocator, "\x1b[34m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "cyan")) {
            out.appendSlice(allocator, "\x1b[36m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "magenta")) {
            out.appendSlice(allocator, "\x1b[35m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "bold")) {
            out.appendSlice(allocator, "\x1b[1m") catch return text;
            reset = true;
        } else if (std.mem.eql(u8, token, "dim")) {
            out.appendSlice(allocator, "\x1b[2m") catch return text;
            reset = true;
        }
    }
    out.appendSlice(allocator, text) catch return text;
    if (reset) out.appendSlice(allocator, "\x1b[0m") catch {};
    return out.toOwnedSlice(allocator) catch text;
}

// ---------------------------------------------------------------------------
// In-memory terminal
// ---------------------------------------------------------------------------

const InMemoryTerm = struct {
    width_: usize,
    height_: usize,
    cells: []u32,
    row: usize = 0,
    col: usize = 0,
    mutex: SpinLock = .{},

    fn init(columns: usize, rows: usize) !InMemoryTerm {
        const cells = try allocator.alloc(u32, columns * rows);
        @memset(cells, ' ');
        return .{ .width_ = columns, .height_ = rows, .cells = cells };
    }

    fn width(self: *InMemoryTerm) usize {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.width_;
    }

    fn height(self: *InMemoryTerm) usize {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.height_;
    }

    fn clearLine(self: *InMemoryTerm) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        const start = self.row * self.width_;
        @memset(self.cells[start .. start + self.width_], ' ');
        self.col = 0;
    }

    fn moveCursorUp(self: *InMemoryTerm, n: usize) void {
        if (n == 0) return;
        self.mutex.lock();
        defer self.mutex.unlock();
        self.row = if (n > self.row) 0 else self.row - n;
    }

    fn writeStr(self: *InMemoryTerm, text: []const u8) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        var i: usize = 0;
        while (i < text.len) {
            if (text[i] == 0x1B) {
                i += 1;
                if (i < text.len and text[i] == '[') {
                    i += 1;
                    while (i < text.len) : (i += 1) {
                        const c = text[i];
                        if ((c >= 'A' and c <= 'Z') or (c >= 'a' and c <= 'z')) break;
                    }
                    if (i < text.len) i += 1;
                }
                continue;
            }
            const before = i;
            const cp = nextCodepoint(text, &i);
            if (i == before) i += 1;
            if (cp == '\n') {
                self.col = 0;
                self.row += 1;
                if (self.row >= self.height_) self.row = self.height_ - 1;
            } else if (cp == '\r') {
                self.col = 0;
            } else {
                if (self.col >= self.width_) {
                    self.col = 0;
                    self.row += 1;
                    if (self.row >= self.height_) self.row = self.height_ - 1;
                }
                self.cells[self.row * self.width_ + self.col] = cp;
                self.col += 1;
            }
        }
    }

    fn writeLine(self: *InMemoryTerm, text: []const u8) void {
        self.writeStr(text);
        self.writeStr("\n");
    }

    fn flush(self: *InMemoryTerm) void {
        _ = self;
    }

    fn contents(self: *InMemoryTerm) []const u8 {
        self.mutex.lock();
        defer self.mutex.unlock();
        var out = std.ArrayList(u8).empty;
        defer out.deinit(allocator);
        var last_nonempty: usize = 0;
        var row: usize = 0;
        while (row < self.height_) : (row += 1) {
            var end = self.width_;
            while (end > 0 and self.cells[row * self.width_ + end - 1] == ' ') end -= 1;
            if (end > 0) last_nonempty = row + 1;
        }
        row = 0;
        while (row < last_nonempty) : (row += 1) {
            var end = self.width_;
            while (end > 0 and self.cells[row * self.width_ + end - 1] == ' ') end -= 1;
            if (row != 0) out.append(allocator, '\n') catch {};
            var col: usize = 0;
            while (col < end) : (col += 1) {
                var encoded: [4]u8 = undefined;
                const cp = self.cells[row * self.width_ + col];
                const len = std.unicode.utf8Encode(@intCast(cp), &encoded) catch 1;
                out.appendSlice(allocator, encoded[0..@intCast(len)]) catch {};
            }
        }
        return out.toOwnedSlice(allocator) catch "";
    }
};

const SpinLock = struct {
    locked: std.atomic.Value(bool) = .init(false),

    fn lock(self: *SpinLock) void {
        while (self.locked.cmpxchgWeak(false, true, .acquire, .monotonic) != null) {
            std.atomic.spinLoopHint();
        }
    }

    fn unlock(self: *SpinLock) void {
        self.locked.store(false, .release);
    }
};


// ---------------------------------------------------------------------------
// Standard terminal
// ---------------------------------------------------------------------------

const StdTerm = struct {
    file: std.Io.File,
    buffered: bool,
    buffer: std.ArrayList(u8) = .empty,
    is_term: bool = false,
    hidden: bool = false,
    width_: usize = 80,
    height_: usize = 20,

    fn init(file: std.Io.File, buffered: bool) StdTerm {
        var term = StdTerm{ .file = file, .buffered = buffered };
        term.is_term = file.isTty(io) catch false;
        if (!term.is_term) {
            term.hidden = true;
            return term;
        }
        if (terminalSize(file)) |size| {
            term.width_ = size.cols;
            term.height_ = size.rows;
        }
        return term;
    }

    fn width(self: *StdTerm) usize {
        if (terminalSize(self.file)) |size| self.width_ = size.cols;
        return self.width_;
    }
    fn height(self: *StdTerm) usize {
        if (terminalSize(self.file)) |size| self.height_ = size.rows;
        return self.height_;
    }
    fn writeStr(self: *StdTerm, text: []const u8) void {
        if (self.buffered) {
            self.buffer.appendSlice(allocator, text) catch {};
        } else {
            self.file.writeStreamingAll(io, text) catch {};
        }
    }
    fn writeLine(self: *StdTerm, text: []const u8) void {
        self.writeStr(text);
        self.writeStr("\n");
    }
    fn clearLine(self: *StdTerm) void { self.writeStr(&[_]u8{13, 27, '[', '2', 'K'}); }
    fn moveCursorUp(self: *StdTerm, n: usize) void {
        if (n == 0) return;
        var buf: [32]u8 = undefined;
        const text = std.fmt.bufPrint(&buf, "\x1b[{d}A", .{n}) catch return;
        self.writeStr(text);
    }
    fn flush(self: *StdTerm) void {
        if (self.buffered and self.buffer.items.len > 0) {
            self.file.writeStreamingAll(io, self.buffer.items) catch {};
            self.buffer.clearRetainingCapacity();
        }
    }
};

const TermSize = struct { cols: usize, rows: usize };

fn terminalSize(file: std.Io.File) ?TermSize {
    if (@import("builtin").os.tag == .windows) {
        var info = std.os.windows.CONSOLE.USER_IO.GET_SCREEN_BUFFER_INFO;
        const result = info.operate(io, file) catch return null;
        switch (result) {
            .SUCCESS => return .{ .cols = @intCast(info.Data.dwWindowSize.X), .rows = @intCast(info.Data.dwWindowSize.Y) },
            else => return null,
        }
    } else {
        var ws: std.posix.winsize = .{ .row = 0, .col = 0, .xpixel = 0, .ypixel = 0 };
        const result = io.operate(.{ .device_io_control = .{
            .file = file,
            .code = std.posix.T.IOCGWINSZ,
            .arg = &ws,
        } }) catch return null;
        if (result.device_io_control >= 0 and ws.col > 0 and ws.row > 0) {
            return .{ .cols = ws.col, .rows = ws.row };
        }
        return null;
    }
}

const Term = union(enum) {
    memory: *InMemoryTerm,
    std: StdTerm,

    fn width(self: *Term) usize {
        return switch (self.*) {
            .memory => |term| term.width(),
            .std => |*term| term.width(),
        };
    }
    fn height(self: *Term) usize {
        return switch (self.*) {
            .memory => |term| term.height(),
            .std => |*term| term.height(),
        };
    }
    fn writeStr(self: *Term, text: []const u8) void {
        switch (self.*) {
            .memory => |term| term.writeStr(text),
            .std => |*term| term.writeStr(text),
        }
    }
    fn writeLine(self: *Term, text: []const u8) void {
        switch (self.*) {
            .memory => |term| term.writeLine(text),
            .std => |*term| term.writeLine(text),
        }
    }
    fn clearLine(self: *Term) void {
        switch (self.*) {
            .memory => |term| term.clearLine(),
            .std => |*term| term.clearLine(),
        }
    }
    fn moveCursorUp(self: *Term, n: usize) void {
        switch (self.*) {
            .memory => |term| term.moveCursorUp(n),
            .std => |*term| term.moveCursorUp(n),
        }
    }
    fn flush(self: *Term) void {
        switch (self.*) {
            .memory => |term| term.flush(),
            .std => |*term| term.flush(),
        }
    }
    fn hidden(self: *Term) bool {
        return switch (self.*) {
            .memory => false,
            .std => |term| term.hidden,
        };
    }
};

// ---------------------------------------------------------------------------
// Progress bar (single)
// ---------------------------------------------------------------------------

const RateLimiter = struct {
    interval_ms: u64 = 50,
    capacity: u8 = 20,
    prev_ns: i96,

    fn init(rate: u8) RateLimiter {
        return .{ .interval_ms = if (rate == 0) 1000 else @as(u64, 1000) / @as(u64, rate), .prev_ns = std.Io.Clock.now(.awake, io).nanoseconds };
    }

    fn allow(self: *RateLimiter, now_ns: i96) bool {
        if (now_ns < self.prev_ns) return false;
        const elapsed = now_ns - self.prev_ns;
        const interval_ns = @as(i96, @intCast(self.interval_ms)) * std.time.ns_per_ms;
        if (self.capacity == 0 and elapsed < interval_ns) return false;
        const new_intervals: u64 = @intCast(@divTrunc(elapsed, interval_ns));
        if (new_intervals > 0) {
            const updated = @as(u64, self.capacity) + new_intervals - 1;
            self.capacity = @intCast(@min(@as(u64, 20), updated));
        } else if (self.capacity > 0) {
            self.capacity -= 1;
        }
        const remainder = @mod(elapsed, interval_ns);
        self.prev_ns = now_ns - remainder;
        return true;
    }
};

pub const ProgressBar = struct {
    length: ?u64,
    start_ns: i96,
    pos: std.atomic.Value(u64) = .init(0),
    style: Style,
    message: []const u8 = "",
    prefix: []const u8 = "",
    term: Term,
    limiter: RateLimiter,
    last_line_count: usize = 0,
    finished: bool = false,
    mutex: SpinLock = .{},

    pub fn initMemory(length: ?u64, term: *InMemoryTerm) !ProgressBar {
        return .{
            .length = length,
            .start_ns = std.Io.Clock.now(.awake, io).nanoseconds,
            .style = try Style.init("{wide_bar} {pos}/{len}"),
            .term = .{ .memory = term },
            .limiter = RateLimiter.init(20),
        };
    }

    pub fn initStderr(length: ?u64) ProgressBar {
        return .{
            .length = length,
            .start_ns = std.Io.Clock.now(.awake, io).nanoseconds,
            .style = Style.init("{wide_bar} {pos}/{len}") catch unreachable,
            .term = .{ .std = StdTerm.init(std.Io.File.stderr(), true) },
            .limiter = RateLimiter.init(20),
        };
    }

    pub fn setStyle(self: *ProgressBar, style: Style) void {
        self.style = style;
    }

    pub fn setMessage(self: *ProgressBar, message: []const u8) void {
        self.message = message;
        self.draw(false);
    }

    pub fn setPrefix(self: *ProgressBar, prefix: []const u8) void {
        self.prefix = prefix;
        self.draw(false);
    }

    pub fn inc(self: *ProgressBar, delta: u64) void {
        _ = self.pos.fetchAdd(delta, .monotonic);
        self.draw(false);
    }

    pub fn setPosition(self: *ProgressBar, value: u64) void {
        self.pos.store(value, .monotonic);
        self.draw(false);
    }

    pub fn position(self: *ProgressBar) u64 {
        return self.pos.load(.monotonic);
    }

    pub fn forceDraw(self: *ProgressBar) void {
        self.draw(true);
    }

    pub fn finishWithMessage(self: *ProgressBar, message: []const u8) void {
        if (self.length) |length| self.pos.store(length, .monotonic);
        self.message = message;
        self.finished = true;
        self.draw(true);
    }

    pub fn finishAndClear(self: *ProgressBar) void {
        if (self.length) |length| self.pos.store(length, .monotonic);
        self.finished = true;
        self.term.clearLine();
        self.term.flush();
    }

    fn draw(self: *ProgressBar, force: bool) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        const now = std.Io.Clock.now(.awake, io).nanoseconds;
        if (!force and !self.limiter.allow(now)) return;

        if (self.term.hidden()) return;
        const width = self.term.width();
        const content = self.format(width);
        if (self.last_line_count > 0) self.term.clearLine();
        self.term.writeStr(content);
        const used = ansiStrWidth(content);
        if (used < width) {
            const padding = width - used;
            var buf: [256]u8 = undefined;
            if (padding <= buf.len) {
                @memset(buf[0..padding], ' ');
                self.term.writeStr(buf[0..padding]);
            }
        }
        self.term.flush();
        self.last_line_count = 1;
    }

    fn format(self: *ProgressBar, width: usize) []const u8 {
        var out = std.ArrayList(u8).empty;
        defer out.deinit(allocator);
        const length = self.length orelse self.position();
        const fraction: f32 = if (length == 0) 1 else @as(f32, @floatFromInt(self.position())) / @as(f32, @floatFromInt(length));

        for (self.style.template.parts) |part| {
            if (!part.placeholder) {
                out.appendSlice(allocator, part.text) catch {};
                continue;
            }
            const key = part.key;
            var buf = std.ArrayList(u8).empty;
            defer buf.deinit(allocator);
            if (std.mem.eql(u8, key, "wide_bar")) {
                const used = ansiStrWidth(out.items);
                const left = if (width > used) width - used else 0;
                buf.appendSlice(allocator, formatBar(fraction, left, "", "", self.style.progress_chars)) catch {};
            } else if (std.mem.eql(u8, key, "bar")) {
                const bar_width = if (part.width > 0) part.width else 20;
                buf.appendSlice(allocator, formatBar(fraction, bar_width, part.style, part.alt_style, self.style.progress_chars)) catch {};
            } else if (std.mem.eql(u8, key, "pos")) {
                const text = std.fmt.allocPrint(allocator, "{d}", .{self.position()}) catch "";
                buf.appendSlice(allocator, text) catch {};
            } else if (std.mem.eql(u8, key, "len")) {
                const text = std.fmt.allocPrint(allocator, "{d}", .{length}) catch "";
                buf.appendSlice(allocator, text) catch {};
            } else if (std.mem.eql(u8, key, "percent")) {
                const text = std.fmt.allocPrint(allocator, "{d:.0}", .{fraction * 100}) catch "";
                buf.appendSlice(allocator, text) catch {};
            } else if (std.mem.eql(u8, key, "msg")) {
                buf.appendSlice(allocator, self.message) catch {};
            } else if (std.mem.eql(u8, key, "prefix")) {
                buf.appendSlice(allocator, self.prefix) catch {};
            } else if (std.mem.eql(u8, key, "bytes")) {
                buf.appendSlice(allocator, humanBytes(self.position())) catch {};
            } else if (std.mem.eql(u8, key, "total_bytes")) {
                buf.appendSlice(allocator, humanBytes(length)) catch {};
            } else if (std.mem.eql(u8, key, "elapsed")) {
                buf.appendSlice(allocator, humanDuration(std.Io.Clock.now(.awake, io).nanoseconds - self.start_ns)) catch {};
            }
            var rendered: []const u8 = buf.items;
            if (part.width > 0) rendered = padAndTruncate(rendered, part.width, part.alignment);
            if (part.style.len > 0) rendered = applyStyle(rendered, part.style);
            out.appendSlice(allocator, rendered) catch {};
        }
        return out.toOwnedSlice(allocator) catch "";
    }
};

fn formatBar(fraction: f32, width: usize, style: []const u8, alt_style: []const u8, chars: [2][]const u8) []const u8 {
    var out = std.ArrayList(u8).empty;
    defer out.deinit(allocator);
    const fill = @as(f32, @floatFromInt(width)) * fraction;
    const filled: usize = @intFromFloat(fill);
    var i: usize = 0;
    while (i < filled) : (i += 1) out.appendSlice(allocator, chars[0]) catch {};
    i = filled;
    while (i < width) : (i += 1) out.appendSlice(allocator, chars[1]) catch {};
    const text = out.items;
    if (style.len == 0 and alt_style.len == 0) return out.toOwnedSlice(allocator) catch "";
    return applyStyle(text, style);
}

test "single bar renders to in-memory terminal" {
    var term = try InMemoryTerm.init(80, 10);
    var pb = try ProgressBar.initMemory(10, &term);
    pb.inc(3);
    pb.forceDraw();
    try std.testing.expect(std.mem.indexOf(u8, term.contents(), "3/10") != null);
}

test "width table uses rust unicode-width values" {
    try std.testing.expectEqual(@as(usize, 1), charWidth('█', false));
    try std.testing.expectEqual(@as(usize, 2), charWidth('█', true));
    try std.testing.expectEqual(@as(usize, 2), charWidth('あ', false));
}
