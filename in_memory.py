# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""In-memory terminal used by tests (port of ``in_memory.rs``)."""

from __future__ import annotations

import threading
from dataclasses import dataclass


@dataclass
class Move:
    kind: str
    n: int = 0
    s: str = ""


@dataclass
class _ParserState:
    mode: str = "normal"  # normal | esc | csi | osc | osc_esc
    params: str = ""


class _AnsiParser:
    """A small VT100 subset parser.

    Only the sequences emitted by :mod:`mpb` need to be understood:
    cursor movement (CUU/CUD/CUF/CUB/CUP), erase-in-line and SGR (ignored).
    """

    def __init__(self, term: "InMemoryTerm") -> None:
        self._term = term

    def feed(self, text: str) -> None:
        for ch in text:
            self._feed_char(ch)

    def _feed_char(self, ch: str) -> None:
        term = self._term
        state = term._state
        mode = state.mode

        if mode == "normal":
            if ch == "\x1b":
                state.mode = "esc"
            elif ch == "\x9b":
                state.mode = "csi"
                state.params = ""
            elif ch == "\r":
                term._cursor_col = 0
            elif ch == "\n":
                term._cursor_col = 0
                term._cursor_row += 1
                if term._cursor_row >= term._height:
                    term._cursor_row = term._height - 1
            elif ch == "\x08":  # backspace
                if term._cursor_col > 0:
                    term._cursor_col -= 1
            elif ch == "\t":
                # The library expands tabs before rendering; treat a raw tab as
                # one printable cell to avoid losing alignment completely.
                term._write_char(" ")
            elif ch >= " " and ch != "\x7f":
                term._write_char(ch)
            return

        if mode == "esc":
            if ch == "[":
                state.mode = "csi"
                state.params = ""
            elif ch == "]":
                state.mode = "osc"
            elif ch in "()":
                # Character-set designation: consume the next char too.
                state.mode = "esc_charset"
            else:
                state.mode = "normal"
            return

        if mode == "esc_charset":
            state.mode = "normal"
            return

        if mode == "osc":
            if ch == "\x07":
                state.mode = "normal"
            elif ch == "\x1b":
                state.mode = "osc_esc"
            return

        if mode == "osc_esc":
            # ST is ESC \\; anything else is unexpected but tolerated.
            state.mode = "normal"
            return

        if mode == "csi":
            code = ord(ch)
            if 0x40 <= code <= 0x7E:
                self._dispatch_csi(state.params, ch)
                state.mode = "normal"
                state.params = ""
            else:
                state.params += ch
            return

    def _dispatch_csi(self, raw: str, final: str) -> None:
        term = self._term
        params: list[int] = []
        if raw:
            for piece in raw.split(";"):
                if piece == "":
                    params.append(0)
                elif piece.isdigit():
                    params.append(int(piece))
                else:
                    # Private / intermediate parameters are not used by the
                    # library; ignore the whole sequence.
                    return

        def get(index: int, default: int) -> int:
            if index < len(params) and params[index] != 0:
                return params[index]
            return default

        if final == "A":
            term._move_up(get(0, 1))
        elif final == "B":
            term._move_down(get(0, 1))
        elif final == "C":
            term._move_right(get(0, 1))
        elif final == "D":
            term._move_left(get(0, 1))
        elif final in ("H", "f"):
            row = get(0, 1) - 1
            col = get(1, 1) - 1
            term._cursor_row = max(0, min(row, term._height - 1))
            term._cursor_col = max(0, min(col, term._width))
        elif final == "K":
            row = term._cursor_row
            for i in range(term._width):
                term._buf[row][i] = " "
        elif final == "J":
            # Erase display: clear from the cursor to the end.
            term._clear_from_cursor()
        # SGR (m) and everything else is intentionally ignored.


class InMemoryTerm:
    """A fixed-size screen buffer with cursor movement and a move history."""

    def __init__(self, rows: int, cols: int) -> None:
        if rows <= 0 or cols <= 0:
            raise ValueError("rows and cols must be > 0")
        self._mu = threading.RLock()
        self._parser = _AnsiParser(self)
        self._state = _ParserState()
        self._width = cols
        self._height = rows
        self._cursor_row = 0
        self._cursor_col = 0
        self._history: list[Move] = []
        self._buf: list[list[str]] = [[" "] * cols for _ in range(rows)]

    # -- screen access --------------------------------------------------------

    def reset(self) -> None:
        with self._mu:
            self._cursor_row = 0
            self._cursor_col = 0
            self._history = []
            self._buf = [[" "] * self._width for _ in range(self._height)]
            self._state = _ParserState()
            self._parser = _AnsiParser(self)

    def contents(self) -> str:
        with self._mu:
            lines = ["".join(row).rstrip(" ") for row in self._buf]
            end = len(lines)
            while end > 0 and lines[end - 1] == "":
                end -= 1
            return "\n".join(lines[:end])

    def contents_formatted(self) -> bytes:
        with self._mu:
            lines = ["".join(row).rstrip(" ") for row in self._buf]
            end = len(lines)
            while end > 0 and lines[end - 1] == "":
                end -= 1
            lines = lines[:end]
        reset = b"\x1b[m"
        out = bytearray()
        for line in lines:
            out += line.encode("utf-8")
            out += reset
            out += b"\n"
        if out:
            out.pop()
        return bytes(out)

    def moves_since_last_check(self) -> str:
        with self._mu:
            history = self._history
            self._history = []
        parts: list[str] = []
        for move in history:
            if move.kind in ("Up", "Down", "Left", "Right"):
                parts.append(f"{move.kind}({move.n})")
            elif move.kind == "Str":
                parts.append(f'Str("{_debug_escape(move.s)}")')
            else:
                parts.append(move.kind)
        if not parts:
            return ""
        return "\n".join(parts) + "\n"

    # -- TermLike implementation ---------------------------------------------

    def width(self) -> int:
        with self._mu:
            return self._width

    def height(self) -> int:
        with self._mu:
            return self._height

    def move_cursor_up(self, n: int) -> None:
        if n == 0:
            return
        with self._mu:
            self._history.append(Move("Up", n=n))
            self._move_up(n)

    def move_cursor_down(self, n: int) -> None:
        if n == 0:
            return
        with self._mu:
            self._history.append(Move("Down", n=n))
            self._move_down(n)

    def move_cursor_right(self, n: int) -> None:
        if n == 0:
            return
        with self._mu:
            self._history.append(Move("Right", n=n))
            self._move_right(n)

    def move_cursor_left(self, n: int) -> None:
        if n == 0:
            return
        with self._mu:
            self._history.append(Move("Left", n=n))
            self._move_left(n)

    def write_line(self, s: str) -> None:
        with self._mu:
            self._history.append(Move("Str", s=s))
            self._history.append(Move("NewLine"))
            self._write_text(s)
            self._write_text("\r\n")

    def write_str(self, s: str) -> None:
        with self._mu:
            self._history.append(Move("Str", s=s))
            self._write_text(s)

    def clear_line(self) -> None:
        with self._mu:
            self._history.append(Move("Clear"))
            self._write_text("\r\x1b[2K")

    def flush(self) -> None:
        with self._mu:
            self._history.append(Move("Flush"))

    # -- internal cursor/buffer operations (caller holds the lock) -----------

    def _write_text(self, text: str) -> None:
        self._parser.feed(text)

    def _write_char(self, ch: str) -> None:
        if self._cursor_col >= self._width:
            self._cursor_row += 1
            self._cursor_col = 0
            if self._cursor_row >= self._height:
                self._cursor_row = self._height - 1
        self._buf[self._cursor_row][self._cursor_col] = ch
        self._cursor_col += 1

    def _move_up(self, n: int) -> None:
        if n > self._cursor_row:
            self._cursor_row = 0
        else:
            self._cursor_row -= n

    def _move_down(self, n: int) -> None:
        self._cursor_row += n
        if self._cursor_row >= self._height:
            self._cursor_row = self._height - 1

    def _move_right(self, n: int) -> None:
        self._cursor_col += n
        if self._cursor_col > self._width:
            self._cursor_col = self._width

    def _move_left(self, n: int) -> None:
        if n > self._cursor_col:
            self._cursor_col = 0
        else:
            self._cursor_col -= n

    def _clear_from_cursor(self) -> None:
        row = self._cursor_row
        for col in range(self._cursor_col, self._width):
            self._buf[row][col] = " "
        for r in range(row + 1, self._height):
            for col in range(self._width):
                self._buf[r][col] = " "


def _debug_escape(s: str) -> str:
    out: list[str] = []
    for ch in s:
        if ch == '"':
            out.append('\\"')
        elif ch == "\\":
            out.append("\\\\")
        elif ch == "\n":
            out.append("\\n")
        elif ch == "\r":
            out.append("\\r")
        elif ch == "\t":
            out.append("\\t")
        elif ord(ch) < 0x20 or ord(ch) == 0x7F:
            out.append(f"\\u{{{ord(ch):x}}}")
        else:
            out.append(ch)
    return "".join(out)
