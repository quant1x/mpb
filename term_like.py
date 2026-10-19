# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Minimal terminal abstraction (port of ``term_like.rs``)."""

from __future__ import annotations

import io
import os
import sys
import threading
from typing import Protocol, runtime_checkable


@runtime_checkable
class TermLike(Protocol):
    def width(self) -> int: ...
    def height(self) -> int: ...
    def move_cursor_up(self, n: int) -> None: ...
    def move_cursor_down(self, n: int) -> None: ...
    def move_cursor_right(self, n: int) -> None: ...
    def move_cursor_left(self, n: int) -> None: ...
    def write_line(self, s: str) -> None: ...
    def write_str(self, s: str) -> None: ...
    def clear_line(self) -> None: ...
    def flush(self) -> None: ...


class StdTerm:
    """A terminal backed by a text writer.

    Width/height are queried from the underlying file descriptor on every call
    when possible, so resizing the terminal is reflected immediately.  The
    fallback size (80x20) is only used when the size cannot be determined.
    """

    def __init__(
        self,
        writer: io.TextIOBase,
        *,
        buffered: bool = False,
        file: io.TextIOBase | None = None,
    ) -> None:
        self._w = writer
        self._file = file
        self._fallback_width = 80
        self._fallback_height = 20
        self._is_term = False
        if file is not None:
            try:
                self._is_term = bool(file.isatty())  # type: ignore[attr-defined]
            except (AttributeError, ValueError, OSError):
                self._is_term = False
        self._buffered = buffered
        self._buf: list[str] = []
        self._mu = threading.RLock()

        if file is not None:
            size = self._query_size()
            if size is not None:
                self._is_term = True

    # -- construction helpers -------------------------------------------------

    @classmethod
    def from_writer(cls, writer: io.TextIOBase, buffered: bool = False) -> "StdTerm":
        file = writer if _has_fileno(writer) else None
        term = cls(writer, buffered=buffered, file=file)
        if file is not None and term._query_size() is None:
            # A real file, but not a terminal: keep is_term False.
            term._is_term = False
        return term

    # -- terminal probing -----------------------------------------------------

    def _query_size(self) -> tuple[int, int] | None:
        if self._file is None:
            return None
        try:
            fd = self._file.fileno()  # type: ignore[attr-defined]
            size = os.get_terminal_size(fd)
        except (AttributeError, ValueError, OSError):
            return None
        if size.columns <= 0 or size.lines <= 0:
            return None
        return size.columns, size.lines

    def is_term(self) -> bool:
        """``console::Term::is_term()``: true only for real terminals."""
        return self._is_term

    def width(self) -> int:
        size = self._query_size()
        if size is not None:
            return size[0]
        return self._fallback_width

    def height(self) -> int:
        size = self._query_size()
        if size is not None:
            return size[1]
        return self._fallback_height

    # -- low level writing ----------------------------------------------------

    def _write_escape(self, text: str) -> None:
        if self._w is None:
            return
        if not self._buffered:
            self._w.write(text)
            return
        with self._mu:
            self._buf.append(text)

    def move_cursor_up(self, n: int) -> None:
        if n <= 0:
            return
        self._write_escape(f"\x1b[{n}A")

    def move_cursor_down(self, n: int) -> None:
        if n <= 0:
            return
        self._write_escape(f"\x1b[{n}B")

    def move_cursor_right(self, n: int) -> None:
        if n <= 0:
            return
        self._write_escape(f"\x1b[{n}C")

    def move_cursor_left(self, n: int) -> None:
        if n <= 0:
            return
        self._write_escape(f"\x1b[{n}D")

    def write_line(self, s: str) -> None:
        self.write_str(s)
        self.write_str("\n")

    def write_str(self, s: str) -> None:
        if self._w is None:
            return
        if not self._buffered:
            self._w.write(s)
            return
        with self._mu:
            self._buf.append(s)

    def clear_line(self) -> None:
        # console::Term::clear_line emits CR + ESC[2K.
        self._write_escape("\r\x1b[2K")

    def flush(self) -> None:
        if not self._buffered:
            try:
                self._w.flush()
            except (AttributeError, ValueError, OSError):
                pass
            return
        with self._mu:
            if not self._buf:
                return
            data = "".join(self._buf)
            self._buf.clear()
            self._w.write(data)
            try:
                self._w.flush()
            except (AttributeError, ValueError, OSError):
                pass


def enable_console() -> None:
    """在 Windows 控制台上打开 ANSI/VT 序列处理。

    Python 的 ``TextIOWrapper`` 在控制台上已经走 ``WriteConsoleW``（UTF-16），
    因此不存在 OEM 代码页导致的乱码；但 Windows 默认不解析 ANSI 转义序列，
    进度条的 ``\x1b[2K`` / ``\x1b[nA`` 会被原样打印，必须在句柄上打开
    ``ENABLE_VIRTUAL_TERMINAL_PROCESSING``。非 Windows 平台为空操作。
    """
    if sys.platform != "win32":
        return
    try:
        import ctypes
        import msvcrt
    except ImportError:  # pragma: no cover - Windows 一定有
        return

    kernel32 = ctypes.windll.kernel32
    enable_vt = 0x0004
    for stream in (sys.stdout, sys.stderr):
        try:
            handle = msvcrt.get_osfhandle(stream.fileno())
        except (AttributeError, ValueError, OSError):
            continue
        mode = ctypes.c_uint32()
        if not kernel32.GetConsoleMode(handle, ctypes.byref(mode)):
            continue
        if mode.value & enable_vt:
            continue
        kernel32.SetConsoleMode(handle, mode.value | enable_vt)


def _has_fileno(stream: object) -> bool:
    try:
        stream.fileno()  # type: ignore[attr-defined]
        return True
    except (AttributeError, ValueError, io.UnsupportedOperation, OSError):
        return False


def new_std_term() -> StdTerm:
    """Buffered stdout terminal (``Term::buffered_stdout()``)."""
    enable_console()
    return StdTerm(sys.stdout, buffered=True, file=sys.stdout)


def new_stderr_term() -> StdTerm:
    """Buffered stderr terminal (``Term::buffered_stderr()``)."""
    enable_console()
    return StdTerm(sys.stderr, buffered=True, file=sys.stderr)


def new_std_term_with_writer(writer: io.TextIOBase, buffered: bool = False) -> StdTerm:
    """Wrap an arbitrary writer.

    Mirrors Go's ``newStdTermWithWriter``: when the writer is backed by a file
    descriptor, probe ``isatty``/terminal size; otherwise fall back to 80x20
    and ``is_term=False``.
    """
    file = writer if _has_fileno(writer) else None
    return StdTerm(writer, buffered=buffered, file=file)
