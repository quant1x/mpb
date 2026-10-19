# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""The public ``ProgressBar`` facade (port of ``progress_bar.rs``)."""

from __future__ import annotations

import time
import weakref
from typing import Any, Callable, Iterable

from .draw_target import (
    ProgressDrawTarget,
    hidden_draw_target,
    stderr_draw_target,
)
from .state import (
    AtomicPosition,
    BarState,
    FinishKind,
    ProgressFinish,
    ProgressState,
    ResetMode,
    default_progress_finish,
)
from .style import (
    ProgressStyle,
    default_progress_style_spinner,
)


def _check_length(length: int | None) -> None:
    if length is not None and length < 0:
        raise ValueError("length must be >= 0")


class ProgressBar:
    """A single progress bar.

    The constructor mirrors the Go ``New(length)`` helper; ``None`` means an
    unknown length.  Use :meth:`no_length`, :meth:`hidden`,
    :meth:`with_draw_target` and :meth:`spinner` for the other constructors.
    """

    def __init__(self, length: int | None = None) -> None:
        _check_length(length)
        self.pos = AtomicPosition()
        self.state = BarState(length, stderr_draw_target(), self.pos)

    # -- alternative constructors --------------------------------------------

    @classmethod
    def _from_state(cls, state: BarState, pos: AtomicPosition) -> "ProgressBar":
        obj = object.__new__(cls)
        obj.state = state
        obj.pos = pos
        return obj

    @classmethod
    def no_length(cls) -> "ProgressBar":
        return cls(None)

    @classmethod
    def hidden(cls) -> "ProgressBar":
        obj = object.__new__(cls)
        obj.pos = AtomicPosition()
        obj.state = BarState(None, hidden_draw_target(), obj.pos)
        return obj

    @classmethod
    def with_draw_target(
        cls, length: int | None, draw_target: ProgressDrawTarget
    ) -> "ProgressBar":
        _check_length(length)
        obj = object.__new__(cls)
        obj.pos = AtomicPosition()
        obj.state = BarState(length, draw_target, obj.pos)
        return obj

    @classmethod
    def spinner(cls) -> "ProgressBar":
        pb = cls(None)
        pb.set_style(default_progress_style_spinner())
        return pb

    # -- progress -------------------------------------------------------------

    def inc(self, delta: int = 1) -> None:
        if delta < 0:
            raise ValueError("delta must be >= 0")
        self.pos.inc(delta)
        now_ns = time.monotonic_ns()
        if self.pos.allow(now_ns):
            self._tick(now_ns / 1_000_000_000)

    def dec(self, delta: int = 1) -> None:
        if delta < 0:
            raise ValueError("delta must be >= 0")
        self.pos.dec(delta)
        now_ns = time.monotonic_ns()
        if self.pos.allow(now_ns):
            self._tick(now_ns / 1_000_000_000)

    def set_position(self, position: int) -> None:
        if position < 0:
            raise ValueError("position must be >= 0")
        self.pos.set(position)
        now_ns = time.monotonic_ns()
        if self.pos.allow(now_ns):
            self._tick(now_ns / 1_000_000_000)

    def _tick(self, now: float) -> None:
        if self.state.has_ticker():
            return
        self.state.tick(now)

    # -- status ---------------------------------------------------------------

    def is_hidden(self) -> bool:
        return self.state.is_hidden()

    def is_finished(self) -> bool:
        return self.state.is_finished()

    def finish(self) -> None:
        length = self.state.length()
        if length is not None:
            self.pos.set(length)
            self.state.set_pos(length)
        self.state.finish_using_style(
            time.monotonic(), ProgressFinish(FinishKind.AND_LEAVE)
        )

    def finish_with_message(self, msg: str) -> None:
        self.state.finish_using_style(
            time.monotonic(), ProgressFinish(FinishKind.WITH_MESSAGE, msg)
        )

    def finish_and_clear(self) -> None:
        length = self.state.length()
        if length is not None:
            self.pos.set(length)
            self.state.set_pos(length)
        self.state.finish_using_style(
            time.monotonic(), ProgressFinish(FinishKind.AND_CLEAR)
        )

    def abandon(self) -> None:
        self.state.finish_using_style(
            time.monotonic(), ProgressFinish(FinishKind.ABANDON)
        )

    def abandon_with_message(self, msg: str) -> None:
        self.state.finish_using_style(
            time.monotonic(), ProgressFinish(FinishKind.ABANDON_WITH_MESSAGE, msg)
        )

    def finish_using_style(self) -> None:
        self.state.finish_using_style(
            time.monotonic(), self.state.get_on_finish()
        )

    # -- configuration --------------------------------------------------------

    def set_style(self, style: ProgressStyle) -> None:
        self.state.set_style(style)

    def enable_steady_tick(self, interval: float) -> None:
        if interval <= 0:
            return
        self.state.start_ticker(interval, self)

    def disable_steady_tick(self) -> None:
        self.state.disable_ticker()

    def set_draw_target(self, draw_target: ProgressDrawTarget) -> None:
        self.state.set_draw_target(draw_target)

    def set_tab_width(self, width: int) -> None:
        self.state.set_tab_width(width)
        self.state.draw(True)

    def with_tab_width(self, width: int) -> "ProgressBar":
        self.state.set_tab_width(width)
        return self

    # -- messages / state mutation -------------------------------------------

    def message(self) -> str:
        return self.state.message()

    def prefix(self) -> str:
        return self.state.prefix()

    def set_prefix(self, prefix: str) -> None:
        self.state.set_prefix(prefix)

    def set_message(self, msg: str) -> None:
        self.state.set_message(msg)

    def set_elapsed(self, elapsed: float) -> None:
        if elapsed < 0:
            raise ValueError("elapsed must be >= 0")
        self.state.set_elapsed(elapsed)

    def update(self, func: Callable[[ProgressState], None]) -> None:
        self.state.update(time.monotonic(), func, not self.state.has_ticker())

    def unset_length(self) -> None:
        self.state.unset_length(time.monotonic())

    def set_length(self, length: int) -> None:
        _check_length(length)
        self.state.set_length(time.monotonic(), length)

    def inc_length(self, delta: int) -> None:
        if delta < 0:
            raise ValueError("delta must be >= 0")
        self.state.inc_length(time.monotonic(), delta)

    def dec_length(self, delta: int) -> None:
        if delta < 0:
            raise ValueError("delta must be >= 0")
        self.state.dec_length(time.monotonic(), delta)

    # -- values ---------------------------------------------------------------

    def position(self) -> int:
        return self.state.position()

    def length(self) -> int | None:
        return self.state.length()

    def eta(self) -> float:
        return self.state.eta()

    def per_sec(self) -> float:
        return self.state.per_sec()

    def elapsed(self) -> float:
        return self.state.elapsed()

    def duration(self) -> float:
        return self.state.duration()

    def tab_width(self) -> int:
        with self.state._lock:
            return self.state.tab_width

    def style(self) -> ProgressStyle:
        return self.state.current_style()

    def index(self) -> tuple[int, bool]:
        return self.state.index()

    def visual_index(self) -> tuple[int, bool]:
        return self.state.visual_index()

    # -- drawing / console ----------------------------------------------------

    def force_draw(self) -> None:
        self.state.draw(True)

    def println(self, msg: str) -> None:
        self.state.println(time.monotonic(), msg)

    def suspend(self, func: Callable[[], Any]) -> Any:
        return self.state.suspend(time.monotonic(), func)

    def reset_eta(self) -> None:
        self.state.reset(time.monotonic(), ResetMode.ETA)

    def reset_elapsed(self) -> None:
        self.state.reset(time.monotonic(), ResetMode.ELAPSED)

    def reset(self) -> None:
        self.state.reset(time.monotonic(), ResetMode.ALL)

    # -- IO wrappers ----------------------------------------------------------

    def wrap_read(self, reader: Any) -> Any:
        from .iter import ProgressReader

        return ProgressReader(reader, self)

    def wrap_write(self, writer: Any) -> Any:
        from .iter import ProgressWriter

        return ProgressWriter(writer, self)

    def wrap_slice(self, iterable: Iterable[Any]) -> Any:
        from .iter import ProgressBarIter

        return ProgressBarIter(iterable, self)

    # -- chaining -------------------------------------------------------------

    def with_style(self, style: ProgressStyle) -> "ProgressBar":
        self.set_style(style)
        return self

    def with_prefix(self, prefix: str) -> "ProgressBar":
        self.set_prefix(prefix)
        return self

    def with_message(self, msg: str) -> "ProgressBar":
        self.set_message(msg)
        return self

    def with_position(self, position: int) -> "ProgressBar":
        self.set_position(position)
        return self

    def with_elapsed(self, elapsed: float) -> "ProgressBar":
        self.set_elapsed(elapsed)
        return self

    def with_finish(self, finish: ProgressFinish) -> "ProgressBar":
        self.state.set_on_finish(finish)
        return self

    # -- sharing --------------------------------------------------------------

    def clone(self) -> "ProgressBar":
        """A shallow copy sharing the same state and position."""
        return ProgressBar._from_state(self.state, self.pos)

    def downgrade(self) -> "WeakProgressBar":
        return WeakProgressBar(weakref.ref(self.state), self.pos)


class WeakProgressBar:
    def __init__(self, ref: Any, pos: AtomicPosition | None = None) -> None:
        self._ref = ref
        self._pos = pos

    def upgrade(self) -> ProgressBar | None:
        state = self._ref()
        if state is None:
            return None
        pos = self._pos or state.state.pos_obj
        return ProgressBar._from_state(state, pos)


# ---------------------------------------------------------------------------
# Module level constructors (parity with the Go package API)
# ---------------------------------------------------------------------------


def new(length: int | None = None) -> ProgressBar:
    return ProgressBar(length)


def no_length() -> ProgressBar:
    return ProgressBar.no_length()


def hidden() -> ProgressBar:
    return ProgressBar.hidden()


def new_with_draw_target(
    length: int | None, draw_target: ProgressDrawTarget
) -> ProgressBar:
    return ProgressBar.with_draw_target(length, draw_target)


def new_spinner() -> ProgressBar:
    return ProgressBar.spinner()


def default_progress_finish_value() -> ProgressFinish:
    return default_progress_finish()
