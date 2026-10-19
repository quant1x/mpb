# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Draw targets and frame rate limiting (port of ``draw_target.rs``)."""

from __future__ import annotations

import os
import sys
import threading
import time
from typing import Any

from ._types import LineAdjust, LineAdjustKind
from .in_memory import InMemoryTerm
from .state import DrawState, DrawStateWrapper, draw_to_term
from .term_like import TermLike, new_stderr_term, new_std_term

_MAX_BURST = 20


class RateLimiter:
    """Token bucket with a fixed refresh interval.

    Mirrors Rust/Go exactly: ``interval`` is in **milliseconds**, capacity is
    20, and the previous allowance is stored as a monotonic nanosecond stamp.
    """

    def __init__(self, rate: int) -> None:
        self.interval = (1000 // rate) if rate else 1000
        self.interval_ns = self.interval * 1_000_000
        self.capacity = _MAX_BURST
        self.prev_ns = time.monotonic_ns()

    def allow(self, now: float | None = None) -> bool:
        if now is None:
            now_ns = time.monotonic_ns()
        else:
            now_ns = int(now * 1_000_000_000)
        if now_ns < self.prev_ns:
            return False

        elapsed_ns = now_ns - self.prev_ns
        if self.capacity == 0 and elapsed_ns < self.interval_ns:
            return False

        # Rust: elapsed.as_millis() / self.interval, remainder in ns.
        new_intervals = (elapsed_ns // 1_000_000) // self.interval
        remainder = elapsed_ns % self.interval_ns
        capacity = self.capacity + new_intervals - 1
        self.capacity = min(_MAX_BURST, capacity)
        self.prev_ns = now_ns - remainder
        return True


class ProgressDrawTarget:
    def is_hidden(self) -> bool:
        return False

    def is_stderr(self) -> bool:
        return False

    def width(self) -> int | None:
        return None

    def remote(self) -> tuple[Any | None, int, bool]:
        return None, 0, False

    def disconnect(self, now: float) -> None:
        pass

    def adjust_last_line_count(self, adjust: LineAdjust) -> None:
        pass

    def mark_zombie(self) -> None:
        pass

    def drawable(self, force: bool, now: float | None = None) -> Any | None:
        return None

    # Optional helpers used by MultiProgress.
    def set_move_cursor(self, value: bool) -> None:
        pass

    def is_move_cursor(self) -> bool:
        return False


class _TermDrawable:
    def __init__(self, target: "TermDrawTargetBase") -> None:
        self._target = target

    def width(self) -> int | None:
        return self._target.width()

    def state(self) -> DrawStateWrapper:
        # draw_state is persistent across frames; every frame starts by resetting.
        self._target.draw_state.lines.clear()
        self._target.draw_state.contents.clear()
        return DrawStateWrapper(self._target.draw_state)

    def draw(self) -> None:
        target = self._target
        with target._mu:
            target.last_line_count = draw_to_term(
                target.draw_state, target.term, target.last_line_count
            )

    def clear(self) -> None:
        target = self._target
        with target._mu:
            target.draw_state.lines.clear()
            target.draw_state.contents.clear()
            target.last_line_count = draw_to_term(
                target.draw_state, target.term, target.last_line_count
            )


class TermDrawTargetBase(ProgressDrawTarget):
    def __init__(self, term: TermLike, rate: int | None, stderr: bool) -> None:
        self._mu = threading.RLock()
        self.term = term
        self.stderr = stderr
        # Rust captures `term.is_term()` when constructing TargetKind::Term;
        # later changes to the TermLike do not make the bar hide/show.
        self.term_is_term = bool(getattr(term, "is_term", lambda: False)())
        self.rate_limiter = RateLimiter(rate) if rate is not None else None
        self.last_line_count = 0
        self.draw_state = DrawState(lines=[], contents=[])

    def is_stderr(self) -> bool:
        return self.stderr

    def width(self) -> int | None:
        return self.term.width()

    def adjust_last_line_count(self, adjust: LineAdjust) -> None:
        with self._mu:
            if adjust.kind == LineAdjustKind.CLEAR:
                self.last_line_count += adjust.count
            else:
                self.last_line_count = max(0, self.last_line_count - adjust.count)

    def set_move_cursor(self, value: bool) -> None:
        with self._mu:
            self.draw_state.move_cursor = value

    def is_move_cursor(self) -> bool:
        with self._mu:
            return self.draw_state.move_cursor

    def drawable(self, force: bool, now: float | None = None) -> Any | None:
        if not force and self.rate_limiter is not None:
            with self._mu:
                if not self.rate_limiter.allow(now):
                    return None
        return _TermDrawable(self)


class TermDrawTarget(TermDrawTargetBase):
    """A real terminal (``Term`` variant)."""

    def is_hidden(self) -> bool:
        return not self.term_is_term


class TermLikeDrawTarget(TermDrawTargetBase):
    """A ``TermLike`` implementation (in-memory terminal, custom terminal)."""

    def is_hidden(self) -> bool:
        return False


class HiddenDrawTarget(ProgressDrawTarget):
    def is_hidden(self) -> bool:
        return True

    def width(self) -> int | None:
        return None


def is_dumb() -> bool:
    term = os.environ.get("TERM")
    if term is not None:
        return term == "dumb"
    return sys.platform != "win32"


def term_draw_target(
    term: TermLike, refresh_rate: int = 20, stderr: bool = False
) -> ProgressDrawTarget:
    is_term = bool(getattr(term, "is_term", lambda: False)())
    if not is_term or is_dumb():
        return HiddenDrawTarget()
    return TermDrawTarget(term, refresh_rate, stderr)


def term_like_draw_target(
    term_like: TermLike, refresh_rate: int | None = None
) -> ProgressDrawTarget:
    return TermLikeDrawTarget(term_like, refresh_rate, False)


def stdout_draw_target() -> ProgressDrawTarget:
    return stdout_draw_target_with_hz(20)


def stderr_draw_target() -> ProgressDrawTarget:
    return stderr_draw_target_with_hz(20)


def stdout_draw_target_with_hz(refresh_rate: int) -> ProgressDrawTarget:
    return term_draw_target(new_std_term(), refresh_rate, False)


def stderr_draw_target_with_hz(refresh_rate: int) -> ProgressDrawTarget:
    return term_draw_target(new_stderr_term(), refresh_rate, True)


def term_like_draw_target_with_hz(
    term_like: TermLike, refresh_rate: int
) -> ProgressDrawTarget:
    return term_like_draw_target(term_like, refresh_rate)


def hidden_draw_target() -> ProgressDrawTarget:
    return HiddenDrawTarget()


def in_memory_draw_target(term: InMemoryTerm) -> ProgressDrawTarget:
    return term_like_draw_target(term)
