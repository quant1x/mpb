// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `style::Width` / `unicode_width` usage.
//!
//! The range table in `unicode_width.zig` is generated from Rust's
//! `unicode-width 0.2.2`, the exact crate version `indicatif 0.18.6` depends
//! on.  A code point that is not covered by the table has width 1, matching
//! `UnicodeWidthChar::width`, which only returns `None` for control
//! characters (they are explicitly present in the table with width 0).

const std = @import("std");
const uw = @import("unicode_width.zig");

/// Global CJK ambiguous-width flag, mirroring `Width::set_default_cjk`.
pub var cjk = std.atomic.Value(bool).init(false);

pub fn setDefaultCjk(value: bool) void {
    cjk.store(value, .monotonic);
}

pub fn isCjk() bool {
    return cjk.load(.monotonic);
}

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

pub fn charWidth(cp: u21) usize {
    if (findRange(cp)) |range| {
        return if (isCjk()) range.width_cjk else range.width;
    }
    return 1;
}

pub fn nextCodepoint(text: []const u8, index: *usize) u21 {
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
        width += charWidth(nextCodepoint(text, &i));
    }
    return width;
}

pub fn skipAnsi(text: []const u8, index: *usize) bool {
    if (index.* + 1 >= text.len or text[index.*] != 0x1B) return false;
    var i = index.* + 1;
    if (text[i] == '[') {
        i += 1;
        while (i < text.len) : (i += 1) {
            const c = text[i];
            if ((c >= 'A' and c <= 'Z') or (c >= 'a' and c <= 'z')) {
                i += 1;
                break;
            }
        }
    } else {
        i += 1;
    }
    index.* = i;
    return true;
}

pub fn ansiStrWidth(text: []const u8) usize {
    var width: usize = 0;
    var i: usize = 0;
    while (i < text.len) {
        if (skipAnsi(text, &i)) continue;
        width += charWidth(nextCodepoint(text, &i));
    }
    return width;
}

test "width uses the rust unicode-width table" {
    try std.testing.expectEqual(@as(usize, 1), charWidth('a'));
    try std.testing.expectEqual(@as(usize, 0), charWidth(0x1B));
    try std.testing.expectEqual(@as(usize, 2), charWidth(0x3042));
    try std.testing.expectEqual(@as(usize, 2), charWidth(0x4E2D));
}
