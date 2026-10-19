# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Progress state, estimators and terminal rendering (port of ``state.rs``)."""

from __future__ import annotations

import math
import threading
import time
from dataclasses import dataclass
from enum import Enum
from typing import Any, Callable

from ._types import (
    LineType,
    MultiProgressAlignment,
    f32,
    saturating_add,
    saturating_sub,
)
from .style import (
    DEFAULT_WIDTH,
    ProgressStyle,
    ansi_segments,
    default_progress_style_bar,
)

DEFAULT_TAB_WIDTH = 8


# ---------------------------------------------------------------------------
# Finish / status
# ---------------------------------------------------------------------------


class FinishKind(Enum):
    AND_LEAVE = "and_leave"
    WITH_MESSAGE = "with_message"
    AND_CLEAR = "and_clear"
    ABANDON = "abandon"
    ABANDON_WITH_MESSAGE = "abandon_with_message"


@dataclass
class ProgressFinish:
    kind: FinishKind
    msg: str = ""


def default_progress_finish() -> ProgressFinish:
    return ProgressFinish(FinishKind.AND_CLEAR)


class ResetMode(Enum):
    ETA = "eta"
    ELAPSED = "elapsed"
    ALL = "all"


class StatusKind(Enum):
    IN_PROGRESS = "in_progress"
    DONE_VISIBLE = "done_visible"
    DONE_HIDDEN = "done_hidden"


@dataclass
class Status:
    kind: StatusKind
    duration: float = 0.0

    def is_finished(self) -> bool:
        return self.kind != StatusKind.IN_PROGRESS


def status_in_progress() -> Status:
    return Status(StatusKind.IN_PROGRESS)


# ---------------------------------------------------------------------------
# Estimator
# ---------------------------------------------------------------------------

_EXPONENTIAL_WEIGHTING_SECONDS = 15.0


def estimator_weight(age: float) -> float:
    return 0.1 ** (age / _EXPONENTIAL_WEIGHTING_SECONDS)


class Estimator:
    def __init__(self, now: float) -> None:
        self.smoothed = 0.0
        self.double_smoothed = 0.0
        self.prev_steps = 0
        self.prev_time = now
        self.start_time = now

    def record(self, new_steps: int, now: float) -> None:
        if new_steps <= self.prev_steps or now <= self.prev_time:
            if new_steps < self.prev_steps:
                self.prev_steps = new_steps
                self.reset(now)
            return

        delta_steps = new_steps - self.prev_steps
        delta_t = now - self.prev_time
        new_sps = delta_steps / delta_t
        weight = estimator_weight(delta_t)
        self.smoothed = self.smoothed * weight + new_sps * (1 - weight)

        delta_t_start = now - self.start_time
        total_weight = 1.0 - estimator_weight(delta_t_start)
        normalized = self.smoothed / total_weight if total_weight != 0 else 0.0
        self.double_smoothed = self.double_smoothed * weight + normalized * (1 - weight)

        self.prev_steps = new_steps
        self.prev_time = now

    def reset(self, now: float) -> None:
        self.smoothed = 0.0
        self.double_smoothed = 0.0
        self.prev_time = now
        self.start_time = now

    def steps_per_second(self, now: float) -> float:
        delta_t = now - self.prev_time
        reweight = estimator_weight(delta_t)
        delta_t_start = now - self.start_time
        total_weight = 1.0 - estimator_weight(delta_t_start)
        sps = self.smoothed * reweight / total_weight if total_weight != 0 else 0.0
        dsps = self.double_smoothed * reweight + sps * (1 - reweight)
        if total_weight == 0:
            return dsps
        return dsps / total_weight


# ---------------------------------------------------------------------------
# Tab-expanded strings
# ---------------------------------------------------------------------------


@dataclass
class TabExpandedString:
    original: str = ""
    tab_width: int = DEFAULT_TAB_WIDTH

    def expanded(self) -> str:
        if "\t" not in self.original:
            return self.original
        return self.original.replace("\t", " " * self.tab_width)

    def set_tab_width(self, width: int) -> None:
        self.tab_width = width


# ---------------------------------------------------------------------------
# Draw state / line wrapping
# ---------------------------------------------------------------------------


def wrapped_metrics(s: str, width: int) -> tuple[int, int]:
    """Return ``(height, last_line_width)`` for ``s`` at terminal ``width``."""
    if width <= 0:
        return 1, 0

    num_lines = 1
    column = 0
    for text, is_ansi in ansi_segments(s):
        if is_ansi:
            continue
        for ch in text:
            ch_width = DEFAULT_WIDTH.char(ch)
            column += ch_width
            if column > width:
                num_lines += 1
                column = ch_width
    return num_lines, column


def visual_line_count(lines: list[LineType], contents: list[str], width: int) -> int:
    total = 0
    for i in range(len(lines)):
        content = contents[i] if i < len(contents) else ""
        total += wrapped_metrics(content, width)[0]
    return total


@dataclass
class DrawState:
    lines: list[LineType]
    contents: list[str]
    move_cursor: bool = False
    alignment: MultiProgressAlignment = MultiProgressAlignment.TOP


class DrawStateWrapper:
    """RAII wrapper: ``close`` strips text/empty lines into ``orphan_lines``."""

    def __init__(self, state: DrawState, orphan_lines: list[str] | None = None) -> None:
        self.state = state
        self.orphan_lines = orphan_lines

    def reset(self) -> None:
        self.state.lines.clear()
        self.state.contents.clear()

    def close(self) -> None:
        if self.orphan_lines is None:
            return
        old_lines = self.state.lines
        old_contents = self.state.contents
        new_lines: list[LineType] = []
        new_contents: list[str] = []
        for i, line_type in enumerate(old_lines):
            if line_type in (LineType.TEXT, LineType.EMPTY):
                content = old_contents[i] if i < len(old_contents) else ""
                self.orphan_lines.append(content)
            else:
                new_lines.append(line_type)
                new_contents.append(old_contents[i] if i < len(old_contents) else "")
        self.state.lines = new_lines
        self.state.contents = new_contents


# ---------------------------------------------------------------------------
# The actual terminal frame writer
# ---------------------------------------------------------------------------


def draw_to_term(ds: DrawState, term: Any, prev_bar_count: int) -> int:
    """Draw ``ds`` to ``term``; return the new visual line count."""
    n = prev_bar_count
    up = saturating_sub(prev_bar_count, 1)

    if ds.lines and ds.move_cursor:
        if up:
            term.move_cursor_up(up)
        term.write_str("\r")
    else:
        if up:
            term.move_cursor_up(up)
        for i in range(n):
            term.clear_line()
            if i + 1 != n:
                term.move_cursor_down(1)
        if up:
            term.move_cursor_up(up)

    term_width = int(term.width())
    full_height = visual_line_count(ds.lines, ds.contents, term_width)

    shift = 0
    if ds.alignment == MultiProgressAlignment.BOTTOM and full_height < prev_bar_count:
        shift = prev_bar_count - full_height
        for _ in range(shift):
            term.write_line("")

    real_height = 0
    for idx, line_type in enumerate(ds.lines):
        content = ds.contents[idx] if idx < len(ds.contents) else ""
        height, last_line_width = wrapped_metrics(content, term_width)

        if line_type == LineType.BAR:
            if saturating_add(real_height, height) > int(term.height()):
                break
            real_height = saturating_add(real_height, height)

        term.write_str(content)
        filler = term_width - last_line_width
        if filler > 0:
            term.write_str(" " * filler)

    term.flush()
    return saturating_add(real_height, shift)


# ---------------------------------------------------------------------------
# ProgressState
# ---------------------------------------------------------------------------


class ProgressState:
    def __init__(self, length: int | None, pos: "AtomicPosition") -> None:
        now = time.monotonic()
        self.pos_obj = pos
        self.length = length
        self.tick = 0
        self.started = now
        self.status = status_in_progress()
        self.est = Estimator(now)
        self.message = TabExpandedString("", DEFAULT_TAB_WIDTH)
        self.prefix = TabExpandedString("", DEFAULT_TAB_WIDTH)

    def is_finished(self) -> bool:
        return self.status.is_finished()

    def fraction(self) -> float:
        pos = self.pos_obj.pos()
        if self.length is None:
            return 0.0
        if self.length == 0:
            return 1.0
        if pos == 0:
            return 0.0
        pct = f32(f32(float(pos)) / f32(float(self.length)))
        return min(1.0, max(0.0, pct))

    def eta(self) -> float:
        if self.is_finished() or self.length is None:
            return 0.0
        pos = self.pos_obj.pos()
        sps = self.est.steps_per_second(time.monotonic())
        if sps == 0.0:
            return 0.0
        remaining = self.length - pos if self.length > pos else 0
        value = remaining / sps
        return value if value > 0 and math.isfinite(value) else 0.0

    def duration(self) -> float:
        if self.status.kind in (StatusKind.DONE_VISIBLE, StatusKind.DONE_HIDDEN):
            return self.status.duration
        if self.length is None:
            return 0.0
        return self.elapsed() + self.eta()

    def per_sec(self) -> float:
        if self.status.kind == StatusKind.IN_PROGRESS:
            return self.est.steps_per_second(time.monotonic())
        elapsed = self.elapsed()
        if elapsed <= 0:
            return 0.0
        return self.pos_obj.pos() / elapsed

    def elapsed(self) -> float:
        return time.monotonic() - self.started

    def pos(self) -> int:
        return self.pos_obj.pos()

    def set_pos(self, value: int) -> None:
        self.pos_obj.set(value)

    def len(self) -> int | None:
        return self.length

    def set_len(self, value: int) -> None:
        self.length = value


# ---------------------------------------------------------------------------
# AtomicPosition
# ---------------------------------------------------------------------------

_ATOMIC_INTERVAL_NS = 1_000_000  # 1 ms
_ATOMIC_MAX_BURST = 10


class AtomicPosition:
    """Integer position + token bucket used to throttle draws."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._pos = 0
        self._capacity = _ATOMIC_MAX_BURST
        self._prev = 0
        self._start = time.monotonic_ns()

    def inc(self, delta: int) -> None:
        with self._lock:
            self._pos += delta

    def dec(self, delta: int) -> None:
        with self._lock:
            self._pos = max(0, self._pos - delta)

    def set(self, value: int) -> None:
        with self._lock:
            self._pos = value

    def pos(self) -> int:
        with self._lock:
            return self._pos

    def allow(self, now_ns: int | None = None) -> bool:
        if now_ns is None:
            now_ns = time.monotonic_ns()
        with self._lock:
            if now_ns < self._start:
                return False
            elapsed = now_ns - self._start
            diff = elapsed - self._prev if elapsed > self._prev else 0
            if self._capacity == 0 and diff < _ATOMIC_INTERVAL_NS:
                return False

            new_intervals = diff // _ATOMIC_INTERVAL_NS
            remainder = diff % _ATOMIC_INTERVAL_NS
            new_capacity = min(
                _ATOMIC_MAX_BURST,
                self._capacity + new_intervals - 1,
            )
            self._capacity = new_capacity
            self._prev = elapsed - remainder
            return True

    def reset(self, now_ns: int | None = None) -> None:
        if now_ns is None:
            now_ns = time.monotonic_ns()
        with self._lock:
            self._pos = 0
            self._prev = max(0, now_ns - self._start)


# ---------------------------------------------------------------------------
# Remote stub
# ---------------------------------------------------------------------------


class RemoteStub:
    def __init__(self, ms: Any) -> None:
        self.ms = ms


# ---------------------------------------------------------------------------
# Ticker
# ---------------------------------------------------------------------------


class TickerHandle:
    """Background ticker; exits by itself once the bar is finished."""

    def __init__(self, interval: float, pb: Any) -> None:
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._run, args=(interval, pb), daemon=True)
        self._thread.start()

    def _run(self, interval: float, pb: Any) -> None:
        while not pb.state.is_finished():
            pb.state.tick(time.monotonic())
            if self._stop.wait(interval):
                return

    def stop(self) -> None:
        self._stop.set()


# ---------------------------------------------------------------------------
# BarState
# ---------------------------------------------------------------------------


class BarState:
    """All mutable state for one bar, guarded by a re-entrant lock."""

    def __init__(
        self,
        length: int | None,
        draw_target: Any,
        pos: AtomicPosition,
    ) -> None:
        self._lock = threading.RLock()
        self.draw_target = draw_target
        self.on_finish = default_progress_finish()
        self.style: ProgressStyle = default_progress_style_bar()
        self.state = ProgressState(length, pos)
        self.tab_width = DEFAULT_TAB_WIDTH
        self._ticker: TickerHandle | None = None
        self._finalizer: Any = None

    # -- completion -----------------------------------------------------------

    def finish_using_style(self, now: float, finish: ProgressFinish) -> None:
        with self._lock:
            duration = now - self.state.started
            self.state.status = Status(StatusKind.DONE_VISIBLE, duration)
            if finish.kind in (
                FinishKind.AND_LEAVE,
                FinishKind.WITH_MESSAGE,
                FinishKind.AND_CLEAR,
            ):
                if self.state.length is not None:
                    self.state.pos_obj.set(self.state.length)
            if finish.kind == FinishKind.WITH_MESSAGE:
                self.state.message = TabExpandedString(finish.msg, self.tab_width)
            elif finish.kind == FinishKind.AND_CLEAR:
                self.state.status = Status(StatusKind.DONE_HIDDEN, duration)
            elif finish.kind == FinishKind.ABANDON_WITH_MESSAGE:
                self.state.message = TabExpandedString(finish.msg, self.tab_width)
            self._draw_with_now(True, now)

    # -- reset / update -------------------------------------------------------

    def reset(self, now: float, mode: ResetMode) -> None:
        with self._lock:
            self.state.est.reset(now)
            if mode in (ResetMode.ELAPSED, ResetMode.ALL):
                self.state.started = now
            if mode == ResetMode.ALL:
                self.state.pos_obj.reset(int(now * 1_000_000_000))
                self.state.status = status_in_progress()
                for tracker in self.style.format_map.values():
                    tracker.reset(self.state, now)
                self._draw_with_now(False, now)

    def update(
        self, now: float, func: Callable[[ProgressState], None], tick: bool
    ) -> None:
        with self._lock:
            func(self.state)
            if tick:
                self._tick_locked(now)

    def unset_length(self, now: float) -> None:
        with self._lock:
            self.state.length = None
            self._update_estimate_and_draw(now)

    def set_length(self, now: float, length: int) -> None:
        with self._lock:
            self.state.length = length
            self._update_estimate_and_draw(now)

    def inc_length(self, now: float, delta: int) -> None:
        with self._lock:
            if self.state.length is not None:
                self.state.length = self.state.length + delta
            self._update_estimate_and_draw(now)

    def dec_length(self, now: float, delta: int) -> None:
        with self._lock:
            if self.state.length is not None:
                self.state.length = max(0, self.state.length - delta)
            self._update_estimate_and_draw(now)

    def set_tab_width(self, width: int) -> None:
        with self._lock:
            self.tab_width = width
            self.state.message.set_tab_width(width)
            self.state.prefix.set_tab_width(width)
            self.style.set_tab_width(width)

    def set_style(self, style: ProgressStyle) -> None:
        with self._lock:
            if self.draw_target is not None and self.draw_target.is_stderr():
                style.set_for_stderr()
            self.style = style
            self.style.set_tab_width(self.tab_width)

    def tick(self, now: float) -> None:
        with self._lock:
            self._tick_locked(now)

    def _tick_locked(self, now: float) -> None:
        self.state.tick += 1
        self._update_estimate_and_draw(now)

    def _update_estimate_and_draw(self, now: float) -> None:
        pos = self.state.pos_obj.pos()
        self.state.est.record(pos, now)
        for tracker in self.style.format_map.values():
            tracker.tick(self.state, now)
        self._draw_with_now(False, now)

    # -- ticker ---------------------------------------------------------------

    def start_ticker(self, interval: float, pb: Any) -> None:
        if interval <= 0:
            return
        with self._lock:
            if self._ticker is not None:
                self._ticker.stop()
            self._ticker = TickerHandle(interval, pb)

    def disable_ticker(self) -> None:
        with self._lock:
            if self._ticker is not None:
                self._ticker.stop()
                self._ticker = None

    def has_ticker(self) -> bool:
        with self._lock:
            return self._ticker is not None

    # -- drawing --------------------------------------------------------------

    def draw(self, force: bool) -> None:
        now = time.monotonic()
        with self._lock:
            self._draw_with_now(force, now)

    def draw_with_now(self, force: bool, now: float) -> None:
        with self._lock:
            self._draw_with_now(force, now)

    def _draw_with_now(self, force: bool, now: float) -> None:
        force = force or self.state.is_finished()
        aggregate = getattr(self.draw_target, "aggregate_draw_bar", None)
        if aggregate is not None:
            aggregate(self, force, now)
            return

        drawable = self.draw_target.drawable(force, now)
        if drawable is None:
            return
        width = drawable.width()
        wrapper = drawable.state()
        if width is not None and self.state.status.kind != StatusKind.DONE_HIDDEN:
            lines, contents = self.style.format_state(
                self.state, int(width), self.draw_target.is_stderr()
            )
            wrapper.state.lines.extend(lines)
            wrapper.state.contents.extend(contents)
        wrapper.close()
        drawable.draw()

    def println(self, now: float, msg: str) -> None:
        with self._lock:
            helper = getattr(self.draw_target, "aggregate_draw_origin", None)
            if helper is not None:
                lines, contents = _message_lines(msg)
                helper(lines, contents, self, True, now)
                return

            width = self.draw_target.width()
            drawable = self.draw_target.drawable(True, now)
            if drawable is None:
                return
            wrapper = drawable.state()
            lines, contents = _message_lines(msg)
            wrapper.state.lines.extend(lines)
            wrapper.state.contents.extend(contents)
            if width is not None and self.state.status.kind != StatusKind.DONE_HIDDEN:
                bar_lines, bar_contents = self.style.format_state(
                    self.state, int(width), self.draw_target.is_stderr()
                )
                wrapper.state.lines.extend(bar_lines)
                wrapper.state.contents.extend(bar_contents)
            wrapper.close()
            drawable.draw()

    def suspend(self, now: float, func: Callable[[], Any]) -> Any:
        with self._lock:
            remote = self.draw_target.remote()
            remote_stub, _, ok = remote
            if ok and remote_stub is not None:
                ms = remote_stub.ms
                with ms.mu:
                    return ms.suspend(func, now)

            drawable = self.draw_target.drawable(True, now)
            if drawable is not None:
                drawable.clear()
            result = func()
            self._draw_with_now(True, now)
            return result

    # -- lookup ---------------------------------------------------------------

    def index(self) -> tuple[int, bool]:
        with self._lock:
            remote, idx, ok = self.draw_target.remote()
            if ok:
                return int(idx), True
            # MultiDrawTarget fallback: search the ordering for ourselves.
            ms = getattr(self.draw_target, "ms", None)
            if ms is not None:
                with ms.mu:
                    for ordering_index, member_index in enumerate(ms.ordering):
                        if member_index >= len(ms.members):
                            continue
                        member = ms.members[member_index]
                        if member.sb is self:
                            return int(member_index), True
            return 0, False

    def visual_index(self) -> tuple[int, bool]:
        with self._lock:
            remote, idx, ok = self.draw_target.remote()
            if not ok or remote is None:
                return 0, False
            ms = remote.ms
            with ms.mu:
                for i, value in enumerate(ms.ordering):
                    if value == idx:
                        return i, True
            return 0, False

    # -- convenience accessors ------------------------------------------------

    def is_finished(self) -> bool:
        with self._lock:
            return self.state.is_finished()

    def is_hidden(self) -> bool:
        with self._lock:
            return self.draw_target.is_hidden()

    def length(self) -> int | None:
        with self._lock:
            return self.state.length

    def set_pos(self, value: int) -> None:
        with self._lock:
            self.state.set_pos(value)

    def position(self) -> int:
        with self._lock:
            return self.state.pos()

    def message(self) -> str:
        with self._lock:
            return self.state.message.expanded()

    def prefix(self) -> str:
        with self._lock:
            return self.state.prefix.expanded()

    def eta(self) -> float:
        with self._lock:
            return self.state.eta()

    def per_sec(self) -> float:
        with self._lock:
            return self.state.per_sec()

    def elapsed(self) -> float:
        with self._lock:
            return self.state.elapsed()

    def duration(self) -> float:
        with self._lock:
            return self.state.duration()

    def current_style(self) -> ProgressStyle:
        with self._lock:
            return self.style

    def set_on_finish(self, finish: ProgressFinish) -> None:
        with self._lock:
            self.on_finish = finish

    def get_on_finish(self) -> ProgressFinish:
        with self._lock:
            return self.on_finish

    def set_prefix(self, prefix: str) -> None:
        with self._lock:
            self.state.prefix = TabExpandedString(prefix, self.tab_width)
            self._update_estimate_and_draw(time.monotonic())

    def set_message(self, msg: str) -> None:
        with self._lock:
            self.state.message = TabExpandedString(msg, self.tab_width)
            self._update_estimate_and_draw(time.monotonic())

    def set_elapsed(self, elapsed: float) -> None:
        with self._lock:
            start = time.monotonic() - elapsed
            self.state.started = start
            self.state.est.reset(start)
            self._update_estimate_and_draw(time.monotonic())

    def set_draw_target(self, draw_target: Any) -> None:
        with self._lock:
            old = self.draw_target
            self.draw_target = draw_target
        if old is not None:
            old.disconnect(time.monotonic())

    def set_draw_target_no_disconnect(self, draw_target: Any) -> None:
        with self._lock:
            self.draw_target = draw_target

    def format_for_width(self, width: int) -> tuple[list[LineType], list[str]]:
        with self._lock:
            if self.state.status.kind == StatusKind.DONE_HIDDEN:
                return [], []
            return self.style.format_state(
                self.state, width, self.draw_target.is_stderr()
            )


def _message_lines(msg: str) -> tuple[list[LineType], list[str]]:
    lines: list[LineType] = []
    contents: list[str] = []
    parts = split_lines(msg)
    if not parts:
        return [LineType.EMPTY], [""]
    for part in parts:
        if part == "":
            lines.append(LineType.EMPTY)
        else:
            lines.append(LineType.TEXT)
        contents.append(part)
    return lines, contents


def split_lines(s: str) -> list[str]:
    out: list[str] = []
    current: list[str] = []
    for ch in s:
        if ch == "\n":
            out.append("".join(current))
            current = []
        else:
            current.append(ch)
    if current:
        out.append("".join(current))
    return out
