// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `style.rs` (template parser, style specs, formatting).

const std = @import("std");
const util = @import("util.zig");
const width = @import("width.zig");
const format = @import("format.zig");
const dt = @import("draw_target.zig");
const state_mod = @import("state.zig");

const allocator = util.allocator;

pub const Alignment = enum {
    left,
    center,
    right,
};

pub const TemplateError = error{InvalidTemplate};

const Placeholder = struct {
    key: []const u8,
    alignment: Alignment = .left,
    width: ?usize = null,
    truncate: bool = false,
    style: []const u8 = "",
    alt_style: []const u8 = "",
};

const TemplatePart = union(enum) {
    literal: state_mod.TabExpandedString,
    placeholder: Placeholder,
    new_line,
};

pub const Template = struct {
    parts: []TemplatePart,

    pub fn fromStr(text: []const u8) TemplateError!Template {
        const ParseState = enum { literal, maybe_open, double_close, key, alignment, width, first_style, alt_style };
        const tab_width = 8;

        var parts = std.ArrayList(TemplatePart).empty;
        var buf = std.ArrayList(u8).empty;
        var state: ParseState = .literal;

        for (text) |c| {
            var new_state = state;
            var keep: ?u8 = null;

            switch (state) {
                .literal => {
                    if (c == '{') {
                        new_state = .maybe_open;
                    } else if (c == '\n') {
                        if (buf.items.len > 0) {
                            parts.append(allocator, .{ .literal = state_mod.TabExpandedString.init(buf.items, tab_width) }) catch return error.InvalidTemplate;
                            buf.clearRetainingCapacity();
                        }
                        parts.append(allocator, .new_line) catch return error.InvalidTemplate;
                    } else if (c == '}') {
                        new_state = .double_close;
                        keep = '}';
                    } else {
                        keep = c;
                    }
                },
                .double_close => {
                    if (c != '}') return error.InvalidTemplate;
                    new_state = .literal;
                },
                .maybe_open => {
                    if (c == '{') {
                        new_state = .literal;
                        keep = '{';
                    } else if (std.ascii.isWhitespace(c)) {
                        // Backtrack: this was a literal `{`.
                        buf.append(allocator, c) catch return error.InvalidTemplate;
                        var literal = std.ArrayList(u8).empty;
                        literal.append(allocator, '{') catch return error.InvalidTemplate;
                        literal.appendSlice(allocator, buf.items) catch return error.InvalidTemplate;
                        buf.clearRetainingCapacity();
                        parts.append(allocator, .{ .literal = state_mod.TabExpandedString.init(literal.items, tab_width) }) catch return error.InvalidTemplate;
                        new_state = .literal;
                    } else if (c != '}' and c != ':') {
                        new_state = .key;
                        keep = c;
                    } else return error.InvalidTemplate;
                },
                .key => {
                    if (std.ascii.isWhitespace(c)) {
                        buf.append(allocator, c) catch return error.InvalidTemplate;
                        var literal = std.ArrayList(u8).empty;
                        literal.append(allocator, '{') catch return error.InvalidTemplate;
                        literal.appendSlice(allocator, buf.items) catch return error.InvalidTemplate;
                        buf.clearRetainingCapacity();
                        parts.append(allocator, .{ .literal = state_mod.TabExpandedString.init(literal.items, tab_width) }) catch return error.InvalidTemplate;
                        new_state = .literal;
                    } else if (c == ':') {
                        new_state = .alignment;
                    } else if (c == '}') {
                        new_state = .literal;
                    } else if (c == '!' and buf.items.len > 0) {
                        parts.append(allocator, .{ .placeholder = .{
                            .key = allocator.dupe(u8, buf.items) catch return error.InvalidTemplate,
                            .truncate = true,
                        } }) catch return error.InvalidTemplate;
                        buf.clearRetainingCapacity();
                        new_state = .width;
                    } else {
                        keep = c;
                    }
                },
                .alignment => {
                    if (c == '<' or c == '^' or c == '>') {
                        if (parts.items.len > 0 and std.meta.activeTag(parts.items[parts.items.len - 1]) == .placeholder) {
                            parts.items[parts.items.len - 1].placeholder.alignment = switch (c) {
                                '<' => .left,
                                '^' => .center,
                                else => .right,
                            };
                        }
                        new_state = .width;
                    } else if (c >= '0' and c <= '9') {
                        new_state = .width;
                        keep = c;
                    } else if (c == '!') {
                        if (parts.items.len > 0 and std.meta.activeTag(parts.items[parts.items.len - 1]) == .placeholder) {
                            parts.items[parts.items.len - 1].placeholder.truncate = true;
                        }
                        new_state = .width;
                    } else if (c == '.') {
                        new_state = .first_style;
                    } else if (c == '}') {
                        new_state = .literal;
                    } else return error.InvalidTemplate;
                },
                .width => {
                    if (c >= '0' and c <= '9') {
                        keep = c;
                    } else if (c == '!') {
                        if (parts.items.len > 0 and std.meta.activeTag(parts.items[parts.items.len - 1]) == .placeholder) {
                            parts.items[parts.items.len - 1].placeholder.truncate = true;
                        }
                    } else if (c == '.') {
                        new_state = .first_style;
                    } else if (c == '}') {
                        new_state = .literal;
                    } else return error.InvalidTemplate;
                },
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

            switch (state) {
                .maybe_open => if (new_state == .key and buf.items.len > 0) {
                    parts.append(allocator, .{ .literal = state_mod.TabExpandedString.init(buf.items, tab_width) }) catch return error.InvalidTemplate;
                    buf.clearRetainingCapacity();
                },
                .key => if ((new_state == .alignment or new_state == .literal) and buf.items.len > 0) {
                    parts.append(allocator, .{ .placeholder = .{
                        .key = allocator.dupe(u8, buf.items) catch return error.InvalidTemplate,
                    } }) catch return error.InvalidTemplate;
                    buf.clearRetainingCapacity();
                },
                .width => if ((new_state == .first_style or new_state == .literal) and buf.items.len > 0) {
                    if (parts.items.len > 0 and std.meta.activeTag(parts.items[parts.items.len - 1]) == .placeholder) {
                        parts.items[parts.items.len - 1].placeholder.width = std.fmt.parseInt(usize, buf.items, 10) catch 0;
                    }
                    buf.clearRetainingCapacity();
                },
                .first_style => if ((new_state == .alt_style or new_state == .literal) and buf.items.len > 0) {
                    if (parts.items.len > 0 and std.meta.activeTag(parts.items[parts.items.len - 1]) == .placeholder) {
                        parts.items[parts.items.len - 1].placeholder.style = allocator.dupe(u8, buf.items) catch return error.InvalidTemplate;
                        buf.clearRetainingCapacity();
                    }
                },
                .alt_style => if (new_state == .literal and buf.items.len > 0) {
                    if (parts.items.len > 0 and std.meta.activeTag(parts.items[parts.items.len - 1]) == .placeholder) {
                        parts.items[parts.items.len - 1].placeholder.alt_style = allocator.dupe(u8, buf.items) catch return error.InvalidTemplate;
                        buf.clearRetainingCapacity();
                    }
                },
                else => {},
            }

            state = new_state;
            if (keep) |ch| buf.append(allocator, ch) catch return error.InvalidTemplate;
        }

        if ((state == .literal or state == .double_close) and buf.items.len > 0) {
            parts.append(allocator, .{ .literal = state_mod.TabExpandedString.init(buf.items, tab_width) }) catch return error.InvalidTemplate;
        }

        return .{ .parts = parts.toOwnedSlice(allocator) catch return error.InvalidTemplate };
    }

    pub fn clone(self: *const Template) Template {
        const parts = allocator.alloc(TemplatePart, self.parts.len) catch return self.*;
        for (self.parts, 0..) |part, i| {
            parts[i] = switch (part) {
                .literal => |lit| .{ .literal = state_mod.TabExpandedString.init(lit.original, lit.tab_width) },
                .placeholder => |p| .{ .placeholder = p },
                .new_line => .new_line,
            };
        }
        return .{ .parts = parts };
    }

    pub fn setTabWidth(self: *Template, tab_width: usize) void {
        for (self.parts) |*part| {
            switch (part.*) {
                .literal => |*lit| lit.setTabWidth(tab_width),
                else => {},
            }
        }
    }
};

/// `console::Style::from_dotted_str` + `impl_fmt`.
const AnsiStyle = struct {
    fg: ?u8 = null,
    fg_bright: bool = false,
    fg_256: ?u8 = null,
    fg_rgb: ?[3]u8 = null,
    bg: ?u8 = null,
    bg_bright: bool = false,
    bg_256: ?u8 = null,
    bg_rgb: ?[3]u8 = null,
    attrs: u16 = 0,

    fn parse(spec: []const u8) AnsiStyle {
        var result = AnsiStyle{};
        var it = std.mem.splitScalar(u8, spec, '.');
        while (it.next()) |part| {
            if (std.mem.eql(u8, part, "black")) {
                result.fg = 0;
            } else if (std.mem.eql(u8, part, "red")) {
                result.fg = 1;
            } else if (std.mem.eql(u8, part, "green")) {
                result.fg = 2;
            } else if (std.mem.eql(u8, part, "yellow")) {
                result.fg = 3;
            } else if (std.mem.eql(u8, part, "blue")) {
                result.fg = 4;
            } else if (std.mem.eql(u8, part, "magenta")) {
                result.fg = 5;
            } else if (std.mem.eql(u8, part, "cyan")) {
                result.fg = 6;
            } else if (std.mem.eql(u8, part, "white")) {
                result.fg = 7;
            } else if (std.mem.eql(u8, part, "bright")) {
                result.fg_bright = true;
            } else if (std.mem.eql(u8, part, "on_black")) {
                result.bg = 0;
            } else if (std.mem.eql(u8, part, "on_red")) {
                result.bg = 1;
            } else if (std.mem.eql(u8, part, "on_green")) {
                result.bg = 2;
            } else if (std.mem.eql(u8, part, "on_yellow")) {
                result.bg = 3;
            } else if (std.mem.eql(u8, part, "on_blue")) {
                result.bg = 4;
            } else if (std.mem.eql(u8, part, "on_magenta")) {
                result.bg = 5;
            } else if (std.mem.eql(u8, part, "on_cyan")) {
                result.bg = 6;
            } else if (std.mem.eql(u8, part, "on_white")) {
                result.bg = 7;
            } else if (std.mem.eql(u8, part, "on_bright")) {
                result.bg_bright = true;
            } else if (std.mem.eql(u8, part, "bold")) {
                result.attrs |= 1 << 0;
            } else if (std.mem.eql(u8, part, "dim")) {
                result.attrs |= 1 << 1;
            } else if (std.mem.eql(u8, part, "italic")) {
                result.attrs |= 1 << 2;
            } else if (std.mem.eql(u8, part, "underlined")) {
                result.attrs |= 1 << 3;
            } else if (std.mem.eql(u8, part, "blink")) {
                result.attrs |= 1 << 4;
            } else if (std.mem.eql(u8, part, "blink_fast")) {
                result.attrs |= 1 << 5;
            } else if (std.mem.eql(u8, part, "reverse")) {
                result.attrs |= 1 << 6;
            } else if (std.mem.eql(u8, part, "hidden")) {
                result.attrs |= 1 << 7;
            } else if (std.mem.eql(u8, part, "strikethrough")) {
                result.attrs |= 1 << 8;
            } else if (std.mem.startsWith(u8, part, "on_#") and part.len == 10) {
                if (hexColor(part[4..10])) |rgb| result.bg_rgb = rgb;
            } else if (std.mem.startsWith(u8, part, "#") and part.len == 7) {
                if (hexColor(part[1..7])) |rgb| result.fg_rgb = rgb;
            } else if (std.mem.startsWith(u8, part, "on_")) {
                result.bg_256 = std.fmt.parseInt(u8, part[3..], 10) catch null;
            } else {
                result.fg_256 = std.fmt.parseInt(u8, part, 10) catch null;
            }
        }
        return result;
    }

    fn hexColor(text: []const u8) ?[3]u8 {
        if (text.len != 6) return null;
        var rgb: [3]u8 = undefined;
        var i: usize = 0;
        while (i < 3) : (i += 1) {
            rgb[i] = std.fmt.parseInt(u8, text[i * 2 .. i * 2 + 2], 16) catch return null;
        }
        return rgb;
    }

    /// foreground -> background -> attributes (ascending) -> text -> reset
    fn render(self: AnsiStyle, alloc: std.mem.Allocator, text: []const u8) []const u8 {
        var out = std.ArrayList(u8).empty;
        var reset = false;

        if (self.fg_rgb) |rgb| {
            out.print(alloc, "\x1b[38;2;{d};{d};{d}m", .{ rgb[0], rgb[1], rgb[2] }) catch {};
            reset = true;
        } else if (self.fg_256) |code| {
            out.print(alloc, "\x1b[38;5;{d}m", .{code}) catch {};
            reset = true;
        } else if (self.fg) |code| {
            if (self.fg_bright) {
                out.print(alloc, "\x1b[38;5;{d}m", .{code + 8}) catch {};
            } else {
                out.print(alloc, "\x1b[{d}m", .{code + 30}) catch {};
            }
            reset = true;
        }

        if (self.bg_rgb) |rgb| {
            out.print(alloc, "\x1b[48;2;{d};{d};{d}m", .{ rgb[0], rgb[1], rgb[2] }) catch {};
            reset = true;
        } else if (self.bg_256) |code| {
            out.print(alloc, "\x1b[48;5;{d}m", .{code}) catch {};
            reset = true;
        } else if (self.bg) |code| {
            if (self.bg_bright) {
                out.print(alloc, "\x1b[48;5;{d}m", .{code + 8}) catch {};
            } else {
                out.print(alloc, "\x1b[{d}m", .{code + 40}) catch {};
            }
            reset = true;
        }

        var bit: u4 = 0;
        while (bit < 9) : (bit += 1) {
            if (self.attrs & (@as(u16, 1) << bit) != 0) {
                out.print(alloc, "\x1b[{d}m", .{bit + 1}) catch {};
                reset = true;
            }
        }

        out.appendSlice(alloc, text) catch {};
        if (reset) out.appendSlice(alloc, "\x1b[0m") catch {};
        return util.finishList(alloc, &out);
    }
};

pub const TrackerFn = *const fn (*const state_mod.ProgressState, *std.ArrayList(u8)) void;

pub const Tracker = struct {
    key: []const u8,
    write: TrackerFn,
};

pub const ProgressStyle = struct {
    tick_strings: [][]const u8,
    progress_chars: [][]const u8,
    template: Template,
    char_width: usize,
    tab_width: usize = 8,
    format_map: []const Tracker = &.{},

    const DEFAULT_TICK_CHARS = "⠁⠁⠉⠙⠚⠒⠂⠂⠒⠲⠴⠤⠄⠄⠤⠠⠠⠤⠦⠖⠒⠐⠐⠒⠓⠋⠉⠈⠈ ";

    pub fn init(template: Template) ProgressStyle {
        const chars = segment("█░");
        return .{
            .tick_strings = segment(DEFAULT_TICK_CHARS),
            .progress_chars = chars,
            .template = template,
            .char_width = clusterWidth(chars),
        };
    }

    pub fn defaultBar() ProgressStyle {
        return init(Template.fromStr("{wide_bar} {pos}/{len}") catch unreachable);
    }

    pub fn defaultSpinner() ProgressStyle {
        return init(Template.fromStr("{spinner} {msg}") catch unreachable);
    }

    pub fn withTemplate(text: []const u8) TemplateError!ProgressStyle {
        return init(try Template.fromStr(text));
    }

    pub fn clone(self: *const ProgressStyle) ProgressStyle {
        return .{
            .tick_strings = self.tick_strings,
            .progress_chars = self.progress_chars,
            .template = self.template.clone(),
            .char_width = self.char_width,
            .tab_width = self.tab_width,
            .format_map = self.format_map,
        };
    }

    /// Adds a custom key to the template (Rust's `with_key`).
    pub fn withKey(self: ProgressStyle, key: []const u8, write: TrackerFn) ProgressStyle {
        var list = std.ArrayList(Tracker).empty;
        list.appendSlice(allocator, self.format_map) catch {};
        list.append(allocator, .{ .key = key, .write = write }) catch {};
        var result = self;
        result.format_map = list.toOwnedSlice(allocator) catch self.format_map;
        return result;
    }

    pub fn setTabWidth(self: *ProgressStyle, new_tab_width: usize) void {
        self.tab_width = new_tab_width;
        self.template.setTabWidth(new_tab_width);
    }

    pub fn tickStrings(self: *ProgressStyle, strings: []const []const u8) void {
        self.tick_strings = allocator.dupe([]const u8, strings) catch self.tick_strings;
        std.debug.assert(self.tick_strings.len >= 2);
    }

    pub fn tickChars(self: *ProgressStyle, chars: []const u8) void {
        self.tick_strings = segment(chars);
        std.debug.assert(self.tick_strings.len >= 2);
    }

    pub fn progressChars(self: *ProgressStyle, chars: []const u8) void {
        self.progress_chars = segment(chars);
        std.debug.assert(self.progress_chars.len >= 2);
        self.char_width = clusterWidth(self.progress_chars);
    }

    pub fn getTickStr(self: *const ProgressStyle, idx: u64) []const u8 {
        return self.tick_strings[@intCast(idx % @as(u64, self.tick_strings.len - 1))];
    }

    pub fn getFinalTickStr(self: *const ProgressStyle) []const u8 {
        return self.tick_strings[self.tick_strings.len - 1];
    }

    fn currentTickStr(self: *const ProgressStyle, state: *const state_mod.ProgressState) []const u8 {
        return if (state.isFinished()) self.getFinalTickStr() else self.getTickStr(state.tick);
    }

    /// `ProgressStyle::format_bar`: both segments are styled separately.
    pub fn formatBar(
        self: *const ProgressStyle,
        alloc: std.mem.Allocator,
        out: *std.ArrayList(u8),
        fraction: f32,
        width_columns: usize,
        style: []const u8,
        alt_style: []const u8,
    ) void {
        const clusters = width_columns / self.char_width;
        const fill = fraction * @as(f32, @floatFromInt(clusters));
        const entirely_filled: usize = @intFromFloat(fill);

        var current: ?usize = null;
        if (fill > 0.0 and entirely_filled < clusters) {
            const n = self.progress_chars.len -| 2;
            if (n == 1) {
                current = 1;
            } else if (n > 1) {
                const fract = fill - @trunc(fill);
                current = n -| @as(usize, @intFromFloat(fract * @as(f32, @floatFromInt(n))));
            }
        }

        const rest_count = clusters -| entirely_filled -| @intFromBool(current != null);

        var filled = std.ArrayList(u8).empty;
        var i: usize = 0;
        while (i < entirely_filled) : (i += 1) filled.appendSlice(alloc, self.progress_chars[0]) catch {};
        if (current) |index| filled.appendSlice(alloc, self.progress_chars[index]) catch {};

        var rest = std.ArrayList(u8).empty;
        i = 0;
        while (i < rest_count) : (i += 1) rest.appendSlice(alloc, self.progress_chars[self.progress_chars.len - 1]) catch {};

        out.appendSlice(alloc, AnsiStyle.parse(style).render(alloc, filled.items)) catch {};
        const rest_style = if (alt_style.len > 0) alt_style else style;
        out.appendSlice(alloc, AnsiStyle.parse(rest_style).render(alloc, rest.items)) catch {};
    }

    /// `ProgressStyle::format_state`.
    pub fn formatState(
        self: *const ProgressStyle,
        state: *const state_mod.ProgressState,
        lines: *std.ArrayList(dt.LineType),
        target_width: usize,
        alloc: std.mem.Allocator,
    ) void {
        var cur = std.ArrayList(u8).empty;
        var buf = std.ArrayList(u8).empty;
        var wide: ?WideElement = null;

        const pos = state.position();
        const len = state.len orelse pos;

        for (self.template.parts) |part| {
            switch (part) {
                .literal => |lit| cur.appendSlice(alloc, lit.expanded()) catch {},
                .new_line => {
                    self.pushLine(lines, &cur, state, &buf, target_width, &wide, alloc);
                },
                .placeholder => |p| {
                    buf.clearRetainingCapacity();

                    var custom = false;
                    for (self.format_map) |tracker| {
                        if (std.mem.eql(u8, tracker.key, p.key)) {
                            tracker.write(state, &buf);
                            custom = true;
                            break;
                        }
                    }

                    if (!custom) {
                        if (std.mem.eql(u8, p.key, "wide_bar")) {
                            wide = .{ .bar = .{ .style = p.style, .alt_style = p.alt_style } };
                            buf.append(alloc, 0) catch {};
                        } else if (std.mem.eql(u8, p.key, "bar")) {
                            self.formatBar(alloc, &buf, state.fraction(), p.width orelse 20, p.style, p.alt_style);
                        } else if (std.mem.eql(u8, p.key, "spinner")) {
                            buf.appendSlice(alloc, self.currentTickStr(state)) catch {};
                        } else if (std.mem.eql(u8, p.key, "wide_msg")) {
                            wide = .{ .message = .{ .alignment = p.alignment } };
                            buf.append(alloc, 0) catch {};
                        } else if (std.mem.eql(u8, p.key, "msg")) {
                            buf.appendSlice(alloc, state.message.expanded()) catch {};
                        } else if (std.mem.eql(u8, p.key, "prefix")) {
                            buf.appendSlice(alloc, state.prefix.expanded()) catch {};
                        } else if (std.mem.eql(u8, p.key, "pos")) {
                            buf.print(alloc, "{d}", .{pos}) catch {};
                        } else if (std.mem.eql(u8, p.key, "human_pos")) {
                            buf.appendSlice(alloc, format.humanCount(alloc, pos)) catch {};
                        } else if (std.mem.eql(u8, p.key, "len")) {
                            buf.print(alloc, "{d}", .{len}) catch {};
                        } else if (std.mem.eql(u8, p.key, "human_len")) {
                            buf.appendSlice(alloc, format.humanCount(alloc, len)) catch {};
                        } else if (std.mem.eql(u8, p.key, "percent")) {
                            buf.print(alloc, "{d:.0}", .{state.fraction() * 100}) catch {};
                        } else if (std.mem.eql(u8, p.key, "percent_precise")) {
                            buf.print(alloc, "{d:.3}", .{state.fraction() * 100}) catch {};
                        } else if (std.mem.eql(u8, p.key, "bytes")) {
                            buf.appendSlice(alloc, format.humanBytes(alloc, pos)) catch {};
                        } else if (std.mem.eql(u8, p.key, "total_bytes")) {
                            buf.appendSlice(alloc, format.humanBytes(alloc, len)) catch {};
                        } else if (std.mem.eql(u8, p.key, "decimal_bytes")) {
                            buf.appendSlice(alloc, format.decimalBytes(alloc, pos)) catch {};
                        } else if (std.mem.eql(u8, p.key, "decimal_total_bytes")) {
                            buf.appendSlice(alloc, format.decimalBytes(alloc, len)) catch {};
                        } else if (std.mem.eql(u8, p.key, "binary_bytes")) {
                            buf.appendSlice(alloc, format.binaryBytes(alloc, pos)) catch {};
                        } else if (std.mem.eql(u8, p.key, "binary_total_bytes")) {
                            buf.appendSlice(alloc, format.binaryBytes(alloc, len)) catch {};
                        } else if (std.mem.eql(u8, p.key, "elapsed_precise")) {
                            buf.appendSlice(alloc, format.formattedDuration(alloc, state.elapsed())) catch {};
                        } else if (std.mem.eql(u8, p.key, "elapsed")) {
                            buf.appendSlice(alloc, format.humanDuration(alloc, state.elapsed(), true)) catch {};
                        } else if (std.mem.eql(u8, p.key, "per_sec")) {
                            if (p.width) |precision| {
                                buf.appendSlice(alloc, format.humanFloatCount(alloc, state.perSec(), precision)) catch {};
                            } else {
                                buf.appendSlice(alloc, format.humanFloatCount(alloc, state.perSec(), 4)) catch {};
                            }
                            buf.appendSlice(alloc, "/s") catch {};
                        } else if (std.mem.eql(u8, p.key, "bytes_per_sec")) {
                            buf.appendSlice(alloc, format.humanBytes(alloc, perSecAsU64(state))) catch {};
                            buf.appendSlice(alloc, "/s") catch {};
                        } else if (std.mem.eql(u8, p.key, "decimal_bytes_per_sec")) {
                            buf.appendSlice(alloc, format.decimalBytes(alloc, perSecAsU64(state))) catch {};
                            buf.appendSlice(alloc, "/s") catch {};
                        } else if (std.mem.eql(u8, p.key, "binary_bytes_per_sec")) {
                            buf.appendSlice(alloc, format.binaryBytes(alloc, perSecAsU64(state))) catch {};
                            buf.appendSlice(alloc, "/s") catch {};
                        } else if (std.mem.eql(u8, p.key, "eta_precise")) {
                            buf.appendSlice(alloc, format.formattedDuration(alloc, state.eta())) catch {};
                        } else if (std.mem.eql(u8, p.key, "eta")) {
                            buf.appendSlice(alloc, format.humanDuration(alloc, state.eta(), true)) catch {};
                        } else if (std.mem.eql(u8, p.key, "duration_precise")) {
                            buf.appendSlice(alloc, format.formattedDuration(alloc, state.duration())) catch {};
                        } else if (std.mem.eql(u8, p.key, "duration")) {
                            buf.appendSlice(alloc, format.humanDuration(alloc, state.duration(), true)) catch {};
                        }
                    }

                    // `format_bar` styles its own segments.
                    const style_spec = if (std.mem.eql(u8, p.key, "bar") or std.mem.eql(u8, p.key, "wide_bar")) "" else p.style;

                    if (p.width) |w| {
                        const padded = pad(alloc, buf.items, w, p.alignment, p.truncate);
                        if (style_spec.len > 0) {
                            cur.appendSlice(alloc, AnsiStyle.parse(style_spec).render(alloc, padded)) catch {};
                        } else {
                            cur.appendSlice(alloc, padded) catch {};
                        }
                    } else {
                        if (style_spec.len > 0) {
                            cur.appendSlice(alloc, AnsiStyle.parse(style_spec).render(alloc, buf.items)) catch {};
                        } else {
                            cur.appendSlice(alloc, buf.items) catch {};
                        }
                    }
                },
            }
        }

        if (cur.items.len > 0) {
            self.pushLine(lines, &cur, state, &buf, target_width, &wide, alloc);
        }
    }

    fn pushLine(
        self: *const ProgressStyle,
        lines: *std.ArrayList(dt.LineType),
        cur: *std.ArrayList(u8),
        state: *const state_mod.ProgressState,
        buf: *std.ArrayList(u8),
        target_width: usize,
        wide: *const ?WideElement,
        alloc: std.mem.Allocator,
    ) void {
        const expanded = if (wide.*) |element| element.expand(alloc, cur.items, self, state, buf, target_width) else alloc.dupe(u8, cur.items) catch "";
        cur.clearRetainingCapacity();

        var start: usize = 0;
        var i: usize = 0;
        while (i <= expanded.len) : (i += 1) {
            if (i == expanded.len or expanded[i] == '\n') {
                const line = expanded[start..i];
                if (i == expanded.len and start == i) break;
                lines.append(allocator, dt.LineType.barLine(line)) catch {};
                start = i + 1;
            }
        }
    }
};

/// Apply a `console` style spec (e.g. `green.bold`) to text.
pub fn applyStyle(text: []const u8, spec: []const u8) []const u8 {
    return AnsiStyle.parse(spec).render(allocator, text);
}

fn perSecAsU64(state: *const state_mod.ProgressState) u64 {
    const value = state.perSec();
    if (std.math.isNan(value) or value <= 0) return 0;
    if (value >= @as(f64, @floatFromInt(std.math.maxInt(u64)))) return std.math.maxInt(u64);
    return @intFromFloat(value);
}

const WideElement = union(enum) {
    bar: struct { style: []const u8, alt_style: []const u8 },
    message: struct { alignment: Alignment },

    fn expand(
        self: WideElement,
        alloc: std.mem.Allocator,
        cur: []const u8,
        style: *const ProgressStyle,
        state: *const state_mod.ProgressState,
        buf: *std.ArrayList(u8),
        target_width: usize,
    ) []const u8 {
        const measured = blk: {
            var it = std.mem.splitScalar(u8, cur, '\n');
            while (it.next()) |line| {
                if (std.mem.indexOfScalar(u8, line, 0) != null) break :blk width.ansiStrWidth(line);
            }
            break :blk width.ansiStrWidth(cur);
        };
        const left = target_width -| measured;

        switch (self) {
            .bar => |bar| {
                buf.clearRetainingCapacity();
                style.formatBar(alloc, buf, state.fraction(), left, bar.style, bar.alt_style);
                return replaceNul(alloc, cur, buf.items);
            },
            .message => |message| {
                buf.clearRetainingCapacity();
                const padded = pad(alloc, state.message.expanded(), left, message.alignment, true);
                const trimmed = if (cur.len > 0 and cur[cur.len - 1] == 0) std.mem.trimEnd(u8, padded, " ") else padded;
                return replaceNul(alloc, cur, trimmed);
            },
        }
    }
};

fn replaceNul(alloc: std.mem.Allocator, cur: []const u8, replacement: []const u8) []const u8 {
    var out = std.ArrayList(u8).empty;
    for (cur) |c| {
        if (c == 0) {
            out.appendSlice(alloc, replacement) catch {};
        } else {
            out.append(alloc, c) catch {};
        }
    }
    return util.finishList(alloc, &out);
}

/// `PaddedStringDisplay`.
pub fn pad(alloc: std.mem.Allocator, text: []const u8, target: usize, alignment: Alignment, truncate: bool) []const u8 {
    const cols = width.ansiStrWidth(text);
    const excess = cols -| target;

    if (excess > 0) {
        if (!truncate) return text;
        const start: usize = switch (alignment) {
            .left => 0,
            .right => excess,
            .center => excess / 2,
        };
        const end: usize = switch (alignment) {
            .left => cols - excess,
            .right => cols,
            .center => cols - excess - excess / 2,
        };
        return writeAnsiRange(alloc, text, start, end);
    }

    const diff = target -| cols;
    const left_pad: usize = switch (alignment) {
        .left => 0,
        .right => diff,
        .center => diff / 2,
    };
    const right_pad = diff - left_pad;

    var out = std.ArrayList(u8).empty;
    out.appendNTimes(alloc, ' ', left_pad) catch {};
    out.appendSlice(alloc, text) catch {};
    out.appendNTimes(alloc, ' ', right_pad) catch {};
    return util.finishList(alloc, &out);
}

/// `write_ansi_range`: keep ANSI sequences, emit visible text in [start, end).
pub fn writeAnsiRange(alloc: std.mem.Allocator, text: []const u8, start: usize, end: usize) []const u8 {
    var out = std.ArrayList(u8).empty;
    var pos: usize = 0;
    var i: usize = 0;

    while (i < text.len) {
        const before = i;
        if (width.skipAnsi(text, &i)) {
            out.appendSlice(alloc, text[before..i]) catch {};
            continue;
        }
        if (pos >= end) {
            _ = width.nextCodepoint(text, &i);
            continue;
        }
        const cp = width.nextCodepoint(text, &i);
        const ch_width = width.charWidth(cp);
        if (start <= pos and pos + ch_width <= end) {
            out.appendSlice(alloc, text[before..i]) catch {};
        }
        pos += ch_width;
    }
    return util.finishList(alloc, &out);
}

fn segment(text: []const u8) [][]const u8 {
    var out = std.ArrayList([]const u8).empty;
    var i: usize = 0;
    while (i < text.len) {
        const start = i;
        _ = width.nextCodepoint(text, &i);
        out.append(allocator, text[start..i]) catch {};
    }
    return out.toOwnedSlice(allocator) catch &.{};
}

fn clusterWidth(clusters: [][]const u8) usize {
    if (clusters.len == 0) return 1;
    const first = width.strWidth(clusters[0]);
    for (clusters) |cluster| {
        std.debug.assert(width.strWidth(cluster) == first);
    }
    return first;
}

test "template parses alignment, width and styles" {
    const template = try Template.fromStr("{prefix:<10} [{bar:20.red/blue}] {pos:>7}/{len:7}");
    try std.testing.expectEqual(@as(usize, 7), template.parts.len);
    try std.testing.expectEqual(Alignment.left, template.parts[0].placeholder.alignment);
    try std.testing.expectEqual(@as(usize, 10), template.parts[0].placeholder.width.?);
    try std.testing.expectEqualStrings("red", template.parts[2].placeholder.style);
    try std.testing.expectEqualStrings("blue", template.parts[2].placeholder.alt_style);
}

test "format bar splits filled and rest styles" {
    var style = ProgressStyle.defaultBar();
    style.progressChars("█░");
    var out = std.ArrayList(u8).empty;
    style.formatBar(allocator, &out, 0.5, 4, "red", "blue");
    try std.testing.expectEqualStrings("\x1b[31m██\x1b[0m\x1b[34m░░\x1b[0m", out.items);
}
