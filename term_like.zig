// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Port of `indicatif`'s `TermLike` trait plus the two implementations used by
//! the crate: a real terminal (`console::Term`) and `InMemoryTerm` (vt100).

const std = @import("std");
const util = @import("util.zig");
const width_mod = @import("width.zig");

const allocator = util.allocator;
const io = util.io;

const builtin = @import("builtin");
const is_windows = builtin.os.tag == .windows;
const win = std.os.windows;

/// Windows console access. `WriteFile` uses the console's OEM code page, which
/// mangles the UTF-8 the library emits; `WriteConsoleW` takes UTF-16 directly.
/// The console also needs `ENABLE_VIRTUAL_TERMINAL_PROCESSING` before it
/// interprets the ANSI sequences used for redrawing and clearing.
const win_console = if (is_windows) struct {
    extern "kernel32" fn GetConsoleMode(handle: win.HANDLE, mode: *win.DWORD) callconv(.winapi) win.BOOL;
    extern "kernel32" fn SetConsoleMode(handle: win.HANDLE, mode: win.DWORD) callconv(.winapi) win.BOOL;
    extern "kernel32" fn WriteConsoleW(
        handle: win.HANDLE,
        buffer: [*]const u16,
        len: win.DWORD,
        written: ?*win.DWORD,
        reserved: ?*anyopaque,
    ) callconv(.winapi) win.BOOL;
} else struct {};

/// Minimal vtable-based equivalent of `trait TermLike`.
pub const TermLike = struct {
    ptr: *anyopaque,
    vtable: *const VTable,

    pub const VTable = struct {
        width: *const fn (ptr: *anyopaque) usize,
        height: *const fn (ptr: *anyopaque) usize,
        writeStr: *const fn (ptr: *anyopaque, text: []const u8) void,
        writeLine: *const fn (ptr: *anyopaque, text: []const u8) void,
        clearLine: *const fn (ptr: *anyopaque) void,
        moveCursorUp: *const fn (ptr: *anyopaque, n: usize) void,
        moveCursorDown: *const fn (ptr: *anyopaque, n: usize) void,
        flush: *const fn (ptr: *anyopaque) void,
    };

    pub fn width(self: TermLike) usize {
        return self.vtable.width(self.ptr);
    }
    pub fn height(self: TermLike) usize {
        return self.vtable.height(self.ptr);
    }
    pub fn writeStr(self: TermLike, text: []const u8) void {
        self.vtable.writeStr(self.ptr, text);
    }
    pub fn writeLine(self: TermLike, text: []const u8) void {
        self.vtable.writeLine(self.ptr, text);
    }
    pub fn clearLine(self: TermLike) void {
        self.vtable.clearLine(self.ptr);
    }
    pub fn moveCursorUp(self: TermLike, n: usize) void {
        self.vtable.moveCursorUp(self.ptr, n);
    }
    pub fn moveCursorDown(self: TermLike, n: usize) void {
        self.vtable.moveCursorDown(self.ptr, n);
    }
    pub fn flush(self: TermLike) void {
        self.vtable.flush(self.ptr);
    }
};

pub const Target = enum { stdout, stderr, other };

pub const StdTerm = struct {
    file: std.Io.File,
    target: Target,
    buffered: bool,
    buffer: std.ArrayList(u8) = .empty,
    is_term: bool,
    cached_width: usize = 80,
    cached_height: usize = 25,
    mutex: util.SpinLock = .{},

    pub fn init(file: std.Io.File, target: Target, buffered: bool) StdTerm {
        var term = StdTerm{ .file = file, .target = target, .buffered = buffered, .is_term = false };
        term.is_term = file.isTty(io) catch false;
        if (term.is_term) {
            enableVirtualTerminal(file);
        }
        if (term.is_term) {
            if (terminalSize(file)) |size| {
                term.cached_width = size.cols;
                term.cached_height = size.rows;
            }
        }
        return term;
    }

    pub fn bufferedStderr() StdTerm {
        return init(std.Io.File.stderr(), .stderr, true);
    }

    pub fn bufferedStdout() StdTerm {
        return init(std.Io.File.stdout(), .stdout, true);
    }

    pub fn termLike(self: *StdTerm) TermLike {
        return .{ .ptr = self, .vtable = &vtable };
    }

    pub fn width(self: *StdTerm) usize {
        self.mutex.lock();
        defer self.mutex.unlock();
        if (terminalSize(self.file)) |size| {
            self.cached_width = size.cols;
            self.cached_height = size.rows;
        }
        return self.cached_width;
    }

    pub fn height(self: *StdTerm) usize {
        self.mutex.lock();
        defer self.mutex.unlock();
        if (terminalSize(self.file)) |size| {
            self.cached_width = size.cols;
            self.cached_height = size.rows;
        }
        return self.cached_height;
    }

    pub fn writeStr(self: *StdTerm, text: []const u8) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        if (self.buffered) {
            self.buffer.appendSlice(allocator, text) catch {};
        } else {
            self.writeRaw(text);
        }
    }

    /// Write one chunk to the terminal, using the wide-character console API on
    /// Windows so UTF-8 text is not interpreted as the OEM code page.
    fn writeRaw(self: *StdTerm, text: []const u8) void {
        if (is_windows and self.is_term) {
            const wide = std.unicode.utf8ToUtf16LeAlloc(allocator, text) catch null;
            if (wide) |buffer| {
                defer allocator.free(buffer);
                var written: win.DWORD = 0;
                if (win_console.WriteConsoleW(self.file.handle, buffer.ptr, @intCast(buffer.len), &written, null).toBool()) {
                    return;
                }
            }
        }
        self.file.writeStreamingAll(io, text) catch {};
    }

    pub fn writeLine(self: *StdTerm, text: []const u8) void {
        self.writeStr(text);
        self.writeStr("\n");
    }

    pub fn clearLine(self: *StdTerm) void {
        self.writeStr("\r\x1b[2K");
    }

    pub fn moveCursorUp(self: *StdTerm, n: usize) void {
        if (n == 0) return;
        var buf: [32]u8 = undefined;
        const text = std.fmt.bufPrint(&buf, "\x1b[{d}A", .{n}) catch return;
        self.writeStr(text);
    }

    pub fn moveCursorDown(self: *StdTerm, n: usize) void {
        if (n == 0) return;
        var buf: [32]u8 = undefined;
        const text = std.fmt.bufPrint(&buf, "\x1b[{d}B", .{n}) catch return;
        self.writeStr(text);
    }

    pub fn flush(self: *StdTerm) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        if (self.buffered and self.buffer.items.len > 0) {
            self.writeRaw(self.buffer.items);
            self.buffer.clearRetainingCapacity();
        }
    }

    const vtable = TermLike.VTable{
        .width = vtableWidth,
        .height = vtableHeight,
        .writeStr = vtableWriteStr,
        .writeLine = vtableWriteLine,
        .clearLine = vtableClearLine,
        .moveCursorUp = vtableMoveUp,
        .moveCursorDown = vtableMoveDown,
        .flush = vtableFlush,
    };

    fn vtableWidth(ptr: *anyopaque) usize {
        return @as(*StdTerm, @ptrCast(@alignCast(ptr))).width();
    }
    fn vtableHeight(ptr: *anyopaque) usize {
        return @as(*StdTerm, @ptrCast(@alignCast(ptr))).height();
    }
    fn vtableWriteStr(ptr: *anyopaque, text: []const u8) void {
        @as(*StdTerm, @ptrCast(@alignCast(ptr))).writeStr(text);
    }
    fn vtableWriteLine(ptr: *anyopaque, text: []const u8) void {
        @as(*StdTerm, @ptrCast(@alignCast(ptr))).writeLine(text);
    }
    fn vtableClearLine(ptr: *anyopaque) void {
        @as(*StdTerm, @ptrCast(@alignCast(ptr))).clearLine();
    }
    fn vtableMoveUp(ptr: *anyopaque, n: usize) void {
        @as(*StdTerm, @ptrCast(@alignCast(ptr))).moveCursorUp(n);
    }
    fn vtableMoveDown(ptr: *anyopaque, n: usize) void {
        @as(*StdTerm, @ptrCast(@alignCast(ptr))).moveCursorDown(n);
    }
    fn vtableFlush(ptr: *anyopaque) void {
        @as(*StdTerm, @ptrCast(@alignCast(ptr))).flush();
    }
};

fn enableVirtualTerminal(file: std.Io.File) void {
    if (!is_windows) return;
    var mode: win.DWORD = 0;
    if (!win_console.GetConsoleMode(file.handle, &mode).toBool()) return;
    if (mode & win.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0) return;
    _ = win_console.SetConsoleMode(file.handle, mode | win.ENABLE_VIRTUAL_TERMINAL_PROCESSING);
}

const TermSize = struct { cols: usize, rows: usize };

fn terminalSize(file: std.Io.File) ?TermSize {
    if (@import("builtin").os.tag == .windows) {
        var info = std.os.windows.CONSOLE.USER_IO.GET_SCREEN_BUFFER_INFO;
        const result = info.operate(io, file) catch return null;
        switch (result) {
            .SUCCESS => return .{
                .cols = @intCast(info.Data.dwWindowSize.X),
                .rows = @intCast(info.Data.dwWindowSize.Y),
            },
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

/// Port of `indicatif::in_memory::InMemoryTerm`.
///
/// The original delegates to `vt100`; this implementation keeps a simple cell
/// grid and understands the escape sequences indicatif itself emits.
pub const InMemoryTerm = struct {
    cols: usize,
    rows: usize,
    cells: []u32,
    row: usize = 0,
    col: usize = 0,
    mutex: util.SpinLock = .{},

    /// `InMemoryTerm::new(rows, cols)`.
    pub fn init(rows: usize, cols: usize) !InMemoryTerm {
        std.debug.assert(rows > 0 and cols > 0);
        const cells = try allocator.alloc(u32, rows * cols);
        @memset(cells, ' ');
        return .{ .rows = rows, .cols = cols, .cells = cells };
    }

    pub fn deinit(self: *InMemoryTerm) void {
        allocator.free(self.cells);
    }

    pub fn reset(self: *InMemoryTerm) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        @memset(self.cells, ' ');
        self.row = 0;
        self.col = 0;
    }

    pub fn termLike(self: *InMemoryTerm) TermLike {
        return .{ .ptr = self, .vtable = &vtable };
    }

    pub fn width(self: *InMemoryTerm) usize {
        return self.cols;
    }

    pub fn height(self: *InMemoryTerm) usize {
        return self.rows;
    }

    fn index(self: *InMemoryTerm, row: usize, col: usize) usize {
        return row * self.cols + col;
    }

    fn scroll(self: *InMemoryTerm) void {
        // vt100 scrolls the screen up by one row when the cursor moves past the
        // bottom; mirror that so long-running output keeps the newest rows.
        const row_bytes = self.cols * @sizeOf(u32);
        std.mem.copyForwards(u32, self.cells[0 .. (self.rows - 1) * self.cols], self.cells[self.cols .. self.rows * self.cols]);
        @memset(self.cells[(self.rows - 1) * self.cols .. self.rows * self.cols], ' ');
        _ = row_bytes;
        self.row = self.rows - 1;
    }

    fn newline(self: *InMemoryTerm) void {
        if (self.row + 1 >= self.rows) {
            self.scroll();
        } else {
            self.row += 1;
        }
    }

    fn put(self: *InMemoryTerm, cp: u21) void {
        const w = width_mod.charWidth(cp);
        if (w == 0) {
            // Combining marks and control characters occupy no cell.
            return;
        }
        if (self.col + w > self.cols) {
            self.col = 0;
            self.newline();
        }
        self.cells[self.index(self.row, self.col)] = cp;
        if (w == 2 and self.col + 1 < self.cols) {
            self.cells[self.index(self.row, self.col + 1)] = 0;
        }
        self.col += w;
    }

    fn eraseLine(self: *InMemoryTerm, mode: u8) void {
        const start = self.index(self.row, 0);
        const end = start + self.cols;
        switch (mode) {
            0 => @memset(self.cells[start + @min(self.col, self.cols) .. end], ' '),
            1 => @memset(self.cells[start .. start + @min(self.col + 1, self.cols)], ' '),
            else => @memset(self.cells[start..end], ' '),
        }
    }

    fn eraseDisplay(self: *InMemoryTerm, mode: u8) void {
        switch (mode) {
            0 => {
                self.eraseLine(0);
                var r = self.row + 1;
                while (r < self.rows) : (r += 1) {
                    const start = self.index(r, 0);
                    @memset(self.cells[start .. start + self.cols], ' ');
                }
            },
            2 => {
                @memset(self.cells, ' ');
                self.row = 0;
                self.col = 0;
            },
            else => {
                var r: usize = 0;
                while (r <= self.row) : (r += 1) {
                    const start = self.index(r, 0);
                    @memset(self.cells[start .. start + self.cols], ' ');
                }
            },
        }
    }

    /// Consume a CSI sequence starting at `i` (pointing at `ESC`); return the
    /// number of bytes consumed.
    fn csi(self: *InMemoryTerm, text: []const u8, start: usize) usize {
        var i = start + 2;
        var params: [8]u8 = undefined;
        var count: usize = 0;
        while (i < text.len) : (i += 1) {
            const c = text[i];
            if (c >= '0' and c <= '9') {
                if (count < params.len) params[count] = c;
                count += 1;
                continue;
            }
            if (c == ';' or c == '?') continue;
            const value: usize = if (count > 0) std.fmt.parseInt(usize, params[0..@min(count, params.len)], 10) catch 0 else 0;
            const amount: usize = if (value == 0) 1 else value;
            switch (c) {
                'A' => self.row -|= amount,
                'B' => self.row = @min(self.rows - 1, self.row + amount),
                'C' => self.col = @min(self.cols, self.col + amount),
                'D' => self.col -|= amount,
                'K' => self.eraseLine(@intCast(value)),
                'J' => self.eraseDisplay(@intCast(value)),
                'G' => self.col = @min(self.cols, amount - 1),
                'H', 'f' => {
                    self.row = 0;
                    self.col = 0;
                },
                else => {},
            }
            return i + 1 - start;
        }
        return i - start;
    }

    pub fn writeStr(self: *InMemoryTerm, text: []const u8) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.writeStrLocked(text);
    }

    fn writeStrLocked(self: *InMemoryTerm, text: []const u8) void {
        var i: usize = 0;
        while (i < text.len) {
            if (text[i] == 0x1B) {
                if (i + 1 < text.len and text[i + 1] == '[') {
                    i += self.csi(text, i);
                } else {
                    i += 1;
                }
                continue;
            }
            const cp = width_mod.nextCodepoint(text, &i);
            switch (cp) {
                '\r' => self.col = 0,
                '\n' => self.newline(),
                0 => {},
                else => self.put(cp),
            }
        }
    }

    pub fn writeLine(self: *InMemoryTerm, text: []const u8) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.writeStrLocked(text);
        self.col = 0;
        if (self.row + 1 >= self.rows) self.scroll() else self.row += 1;
    }

    pub fn clearLine(self: *InMemoryTerm) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.col = 0;
        self.eraseLine(2);
    }

    pub fn moveCursorUp(self: *InMemoryTerm, n: usize) void {
        if (n == 0) return;
        self.mutex.lock();
        defer self.mutex.unlock();
        self.row -|= n;
    }

    pub fn moveCursorDown(self: *InMemoryTerm, n: usize) void {
        if (n == 0) return;
        self.mutex.lock();
        defer self.mutex.unlock();
        self.row = @min(self.rows - 1, self.row + n);
    }

    pub fn flush(self: *InMemoryTerm) void {
        _ = self;
    }

    /// Mirror of `InMemoryTerm::contents`.
    pub fn contents(self: *InMemoryTerm) []const u8 {
        self.mutex.lock();
        defer self.mutex.unlock();

        var last: usize = 0;
        var row: usize = 0;
        while (row < self.rows) : (row += 1) {
            if (self.rowWidth(row) > 0) last = row + 1;
        }

        var out = std.ArrayList(u8).empty;
        row = 0;
        while (row < last) : (row += 1) {
            if (row != 0) out.append(allocator, '\n') catch {};
            const n = self.rowWidth(row);
            var col: usize = 0;
            while (col < n) : (col += 1) {
                const cp = self.cells[self.index(row, col)];
                if (cp == 0) continue;
                var encoded: [4]u8 = undefined;
                const len = std.unicode.utf8Encode(@intCast(cp), &encoded) catch 1;
                out.appendSlice(allocator, encoded[0..@intCast(len)]) catch {};
            }
        }
        return out.toOwnedSlice(allocator) catch "";
    }

    fn rowWidth(self: *InMemoryTerm, row: usize) usize {
        var end = self.cols;
        while (end > 0 and self.cells[self.index(row, end - 1)] == ' ') end -= 1;
        return end;
    }

    const vtable = TermLike.VTable{
        .width = vtableWidth,
        .height = vtableHeight,
        .writeStr = vtableWriteStr,
        .writeLine = vtableWriteLine,
        .clearLine = vtableClearLine,
        .moveCursorUp = vtableMoveUp,
        .moveCursorDown = vtableMoveDown,
        .flush = vtableFlush,
    };

    fn vtableWidth(ptr: *anyopaque) usize {
        return @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).width();
    }
    fn vtableHeight(ptr: *anyopaque) usize {
        return @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).height();
    }
    fn vtableWriteStr(ptr: *anyopaque, text: []const u8) void {
        @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).writeStr(text);
    }
    fn vtableWriteLine(ptr: *anyopaque, text: []const u8) void {
        @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).writeLine(text);
    }
    fn vtableClearLine(ptr: *anyopaque) void {
        @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).clearLine();
    }
    fn vtableMoveUp(ptr: *anyopaque, n: usize) void {
        @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).moveCursorUp(n);
    }
    fn vtableMoveDown(ptr: *anyopaque, n: usize) void {
        @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).moveCursorDown(n);
    }
    fn vtableFlush(ptr: *anyopaque) void {
        @as(*InMemoryTerm, @ptrCast(@alignCast(ptr))).flush();
    }
};

test "in-memory terminal keeps rows and trims trailing space" {
    var term = try InMemoryTerm.init(5, 20);
    defer term.deinit();
    term.writeStr("hello\r\nworld");
    try std.testing.expectEqualStrings("hello\nworld", term.contents());
}
