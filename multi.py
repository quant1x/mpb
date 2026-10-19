# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""MultiProgress / MultiState (port of ``multi.rs``)."""

from __future__ import annotations

import threading
import time
import weakref
from dataclasses import dataclass
from enum import IntEnum
from typing import Any, Callable

from ._types import (
    LineAdjust,
    LineAdjustKind,
    LineType,
    MultiProgressAlignment,
)
from .draw_target import (
    ProgressDrawTarget,
    TermDrawTargetBase,
    hidden_draw_target,
    stderr_draw_target,
)
from .state import (
    BarState,
    DrawState,
    DrawStateWrapper,
    RemoteStub,
    StatusKind,
    draw_to_term,
    split_lines,
    visual_line_count,
)


class InsertLocation(IntEnum):
    END = 0
    INDEX = 1
    INDEX_FROM_BACK = 2
    AFTER = 3
    BEFORE = 4


@dataclass
class MultiStateMember:
    # Keep only a weak reference to the owning BarState. A strong reference
    # here would prevent BarState.__del__ (Rust's Drop emulation) from running.
    sb: weakref.ReferenceType[BarState] | None = None
    draw_state: DrawState | None = None
    is_zombie: bool = False


class MultiProgress:
    def __init__(self, draw_target: ProgressDrawTarget | None = None) -> None:
        if draw_target is None:
            draw_target = stderr_draw_target()
        self.state = MultiState(draw_target)

    @classmethod
    def default(cls) -> "MultiProgress":
        return cls()

    @classmethod
    def with_draw_target(cls, draw_target: ProgressDrawTarget) -> "MultiProgress":
        return cls(draw_target)

    def new(self) -> "MultiProgress":
        return MultiProgress()

    def set_draw_target(self, target: ProgressDrawTarget) -> None:
        with self.state.mu:
            self.state.draw_target.disconnect(time.monotonic())
            self.state.draw_target = MultiDrawTarget(self.state, target)

    def set_move_cursor(self, move_cursor: bool) -> None:
        with self.state.mu:
            self.state.draw_target.set_move_cursor(move_cursor)

    def set_alignment(self, alignment: MultiProgressAlignment) -> None:
        with self.state.mu:
            self.state.alignment = alignment

    def add(self, pb: Any) -> Any:
        return self._internalize(InsertLocation.END, 0, pb)

    def insert(self, index: int, pb: Any) -> Any:
        return self._internalize(InsertLocation.INDEX, index, pb)

    def insert_from_back(self, index: int, pb: Any) -> Any:
        return self._internalize(InsertLocation.INDEX_FROM_BACK, index, pb)

    def insert_before(self, before: Any, pb: Any) -> Any:
        idx, ok = before.index()
        if ok:
            return self._internalize(InsertLocation.BEFORE, idx, pb)
        return self.add(pb)

    def insert_after(self, after: Any, pb: Any) -> Any:
        idx, ok = after.index()
        if ok:
            return self._internalize(InsertLocation.AFTER, idx, pb)
        return self.add(pb)

    def remove(self, pb: Any) -> None:
        remote, idx, ok = _remote_target_of(pb)
        if not ok or remote is None or remote.ms is not self.state:
            return
        # Rust assigns the hidden target directly, without disconnecting.
        pb.state.set_draw_target_no_disconnect(hidden_draw_target())
        with self.state.mu:
            self.state.remove_idx(idx)

    def _internalize(self, location: InsertLocation, pos: int, pb: Any) -> Any:
        state_ref = weakref.ref(pb.state)
        with self.state.mu:
            idx = self.state.insert_with_pos(location, pos)
            if idx < len(self.state.members):
                self.state.members[idx].sb = state_ref
        pb.set_draw_target(RemoteDrawTarget(self.state, idx))
        return pb

    def println(self, msg: str) -> None:
        with self.state.mu:
            self.state.println(msg, time.monotonic())

    def suspend(self, func: Callable[[], Any]) -> Any:
        with self.state.mu:
            return self.state.suspend(func, time.monotonic())

    def clear(self) -> None:
        with self.state.mu:
            self.state.clear(time.monotonic())

    def is_hidden(self) -> bool:
        with self.state.mu:
            return self.state.draw_target.is_hidden()


def _remote_target_of(pb: Any) -> tuple[RemoteStub | None, int, bool]:
    if pb is None or getattr(pb, "state", None) is None:
        return None, 0, False
    return pb.state.remote_target()


class MultiState:
    def __init__(self, draw_target: ProgressDrawTarget) -> None:
        self.mu = threading.RLock()
        self.members: list[MultiStateMember] = []
        self.free_set: list[int] = []
        self.ordering: list[int] = []
        self.alignment = MultiProgressAlignment.TOP
        self.orphan_lines: list[str] = []
        self.zombie_lines_count = 0
        self.draw_target: ProgressDrawTarget = MultiDrawTarget(self, draw_target)

    # -- member state ---------------------------------------------------------

    def draw_state_for(self, idx: int) -> DrawStateWrapper | None:
        if idx >= len(self.members) or idx < 0:
            return None
        member = self.members[idx]
        if member.draw_state is None:
            member.draw_state = DrawState(lines=[], contents=[])
        return DrawStateWrapper(member.draw_state, self.orphan_lines)

    def member_visual_lines(self, idx: int, width: int) -> int:
        if idx >= len(self.members) or idx < 0:
            return 0
        member = self.members[idx]
        if member.draw_state is None:
            return 0
        return visual_line_count(
            member.draw_state.lines, member.draw_state.contents, width
        )

    def mark_zombie(self, index: int) -> None:
        if index >= len(self.members) or index < 0:
            return

        if not self.ordering or index != self.ordering[0]:
            self.members[index].is_zombie = True
            return

        line_count = 0
        width = self.width()
        if width is not None:
            line_count = self.member_visual_lines(index, int(width))

        self.zombie_lines_count += line_count
        self.draw_target.adjust_last_line_count(LineAdjust(LineAdjustKind.KEEP, line_count))
        self.remove_idx(index)

    def width(self) -> int | None:
        return self.draw_target.width()

    def is_hidden(self) -> bool:
        return self.draw_target.is_hidden()

    # -- drawing --------------------------------------------------------------

    def draw(
        self,
        force: bool,
        extra_lines: list[LineType] | None,
        extra_contents: list[str] | None,
        now: float,
    ) -> None:
        width_value: int | None

        reap_indices: list[int] = []
        adjust = 0
        for index in list(self.ordering):
            if index >= len(self.members):
                continue
            member = self.members[index]
            if not member.is_zombie:
                break
            line_count = 0
            width_for_zombie = self.width()
            if width_for_zombie is not None:
                line_count = self.member_visual_lines(index, int(width_for_zombie))
            self.zombie_lines_count += line_count
            adjust += line_count
            reap_indices.append(index)

        extra = bool(extra_lines)
        if extra:
            self.draw_target.adjust_last_line_count(
                LineAdjust(LineAdjustKind.CLEAR, self.zombie_lines_count)
            )
            self.zombie_lines_count = 0

        width_value = self.width()
        if width_value is None:
            return
        width = int(width_value)

        orphan_types = [
            LineType.EMPTY if line == "" else LineType.TEXT for line in self.orphan_lines
        ]
        orphan_visual = visual_line_count(orphan_types, self.orphan_lines, width)
        force = force or orphan_visual > 0

        drawable = self.draw_target.drawable(force, now)
        if drawable is None:
            return
        wrapper = drawable.state()
        ds = wrapper.state
        ds.lines.clear()
        ds.contents.clear()
        ds.alignment = self.alignment
        ds.move_cursor = self.draw_target.is_move_cursor()

        if extra_lines:
            for i, line_type in enumerate(extra_lines):
                ds.lines.append(line_type)
                ds.contents.append(
                    extra_contents[i] if extra_contents and i < len(extra_contents) else ""
                )

        # Orphan lines are drained: each is shown exactly once.
        for line in self.orphan_lines:
            ds.lines.append(LineType.EMPTY if line == "" else LineType.TEXT)
            ds.contents.append(line)
        self.orphan_lines = []

        for idx in self.ordering:
            if idx >= len(self.members):
                continue
            member = self.members[idx]
            if member.draw_state is None:
                continue
            ds.lines.extend(member.draw_state.lines)
            ds.contents.extend(member.draw_state.contents)

        drawable.draw()

        for idx in reap_indices:
            self.remove_idx(idx)

        if not extra:
            self.draw_target.adjust_last_line_count(
                LineAdjust(LineAdjustKind.KEEP, adjust)
            )

    def println(self, msg: str, now: float) -> None:
        lines: list[LineType] = []
        contents: list[str] = []
        if msg == "":
            lines.append(LineType.EMPTY)
            contents.append("")
        else:
            for line in split_lines(msg):
                if line == "":
                    lines.append(LineType.EMPTY)
                else:
                    lines.append(LineType.TEXT)
                contents.append(line)
        self.draw(True, lines, contents, now)

    def suspend(self, func: Callable[[], Any], now: float) -> Any:
        self.clear(now)
        result = func()
        self.draw(True, None, None, time.monotonic())
        return result

    def clear(self, now: float) -> None:
        drawable = self.draw_target.drawable(True, now)
        if drawable is None:
            return
        self.draw_target.adjust_last_line_count(
            LineAdjust(LineAdjustKind.CLEAR, self.zombie_lines_count)
        )
        self.zombie_lines_count = 0
        drawable.clear()

    # -- indices --------------------------------------------------------------

    def visual_index(self, idx: int) -> tuple[int, bool]:
        for i, value in enumerate(self.ordering):
            if value == idx:
                return i, True
        return 0, False

    def insert_with_pos(self, location: InsertLocation, pos: int) -> int:
        if self.free_set:
            idx = self.free_set.pop()
            self.members[idx] = MultiStateMember()
        else:
            self.members.append(MultiStateMember())
            idx = len(self.members) - 1

        if location == InsertLocation.END:
            self.ordering.append(idx)
        elif location == InsertLocation.INDEX:
            position = min(max(pos, 0), len(self.ordering))
            self.ordering.insert(position, idx)
        elif location == InsertLocation.INDEX_FROM_BACK:
            length = len(self.ordering)
            position = length - pos if pos < length else 0
            self.ordering.insert(position, idx)
        elif location == InsertLocation.AFTER:
            for i, value in enumerate(self.ordering):
                if value == pos:
                    self.ordering.insert(i + 1, idx)
                    break
            else:
                self.ordering.append(idx)
        elif location == InsertLocation.BEFORE:
            for i, value in enumerate(self.ordering):
                if value == pos:
                    self.ordering.insert(i, idx)
                    break
            else:
                self.ordering.append(idx)
        return idx

    def remove_idx(self, idx: int) -> None:
        if idx in self.free_set:
            return
        if idx < 0 or idx >= len(self.members):
            return
        self.members[idx] = MultiStateMember()
        self.free_set.append(idx)
        try:
            self.ordering.remove(idx)
        except ValueError:
            pass

    def __len__(self) -> int:
        return len(self.members) - len(self.free_set)


class MultiDrawTarget(ProgressDrawTarget):
    def __init__(self, ms: MultiState, inner: ProgressDrawTarget) -> None:
        self.ms = ms
        self.inner = inner
        self.agg: AggregatedDrawable | None = None

    def is_hidden(self) -> bool:
        return self.inner.is_hidden()

    def is_stderr(self) -> bool:
        return self.inner.is_stderr()

    def width(self) -> int | None:
        return self.inner.width()

    def remote(self) -> tuple[RemoteStub | None, int, bool]:
        return self.inner.remote()

    def disconnect(self, now: float) -> None:
        self.inner.disconnect(now)

    def mark_zombie(self) -> None:
        self.inner.mark_zombie()

    def set_move_cursor(self, value: bool) -> None:
        self.inner.set_move_cursor(value)

    def is_move_cursor(self) -> bool:
        return self.inner.is_move_cursor()

    def adjust_last_line_count(self, adjust: LineAdjust) -> None:
        self.inner.adjust_last_line_count(adjust)

    def drawable(self, force: bool, now: float | None = None) -> Any | None:
        if self.inner.drawable(force, now) is None:
            return None
        if self.agg is None:
            self.agg = AggregatedDrawable(self)
        return self.agg

    def aggregate_draw_bar(self, bar: BarState, force: bool, now: float) -> None:
        self.ms.draw(force, None, None, now)

    def aggregate_draw_origin(
        self,
        origin_lines: list[LineType],
        origin_contents: list[str],
        bar: BarState,
        force: bool,
        now: float,
    ) -> None:
        self.ms.draw(force, origin_lines, origin_contents, now)


class AggregatedDrawable:
    def __init__(self, mdt: MultiDrawTarget) -> None:
        self.mdt = mdt
        self._state = DrawState(lines=[], contents=[])

    # Rust's Drawable trait calls this `state`.
    def state(self) -> DrawStateWrapper:
        return DrawStateWrapper(self._state)

    def width(self) -> int | None:
        return self.mdt.inner.width()

    def draw(self) -> None:
        inner = self.mdt.inner
        if isinstance(inner, TermDrawTargetBase):
            with inner._mu:
                inner.last_line_count = draw_to_term(
                    self._state, inner.term, inner.last_line_count
                )
            return

        drawable = inner.drawable(False)
        if drawable is not None:
            wrapper = drawable.state()
            wrapper.state.lines.clear()
            wrapper.state.contents.clear()
            wrapper.state.lines.extend(self._state.lines)
            wrapper.state.contents.extend(self._state.contents)
            wrapper.close()
            drawable.draw()

    def clear(self) -> None:
        self._state.lines.clear()
        self._state.contents.clear()
        self.draw()


class RemoteDrawTarget(ProgressDrawTarget):
    def __init__(self, ms: MultiState, idx: int) -> None:
        self.ms = ms
        self.idx = idx

    def is_hidden(self) -> bool:
        return self.ms.draw_target.is_hidden()

    def is_stderr(self) -> bool:
        return self.ms.draw_target.is_stderr()

    def width(self) -> int | None:
        return self.ms.draw_target.width()

    def remote(self) -> tuple[RemoteStub | None, int, bool]:
        return RemoteStub(self.ms), self.idx, True

    def disconnect(self, now: float) -> None:
        with self.ms.mu:
            wrapper = self.ms.draw_state_for(self.idx)
            if wrapper is None:
                return
            wrapper.reset()
            self.ms.draw(True, None, None, now)

    def drawable(self, force: bool, now: float | None = None) -> Any | None:
        return None

    def adjust_last_line_count(self, adjust: LineAdjust) -> None:
        self.ms.draw_target.adjust_last_line_count(adjust)

    def mark_zombie(self) -> None:
        with self.ms.mu:
            self.ms.mark_zombie(self.idx)

    def aggregate_draw_bar(self, bar: BarState, force: bool, now: float) -> None:
        self._aggregate_draw(bar, force, now, None, None)

    def aggregate_draw_origin(
        self,
        origin_lines: list[LineType],
        origin_contents: list[str],
        bar: BarState,
        force: bool,
        now: float,
    ) -> None:
        self._aggregate_draw(bar, force, now, origin_lines, origin_contents)

    def _aggregate_draw(
        self,
        bar: BarState,
        force: bool,
        now: float,
        message_lines: list[LineType] | None,
        message_contents: list[str] | None,
    ) -> None:
        with self.ms.mu:
            wrapper = self.ms.draw_state_for(self.idx)
            if wrapper is None:
                return
            ds = wrapper.state
            ds.lines.clear()
            ds.contents.clear()

            if message_lines:
                for i, line_type in enumerate(message_lines):
                    ds.lines.append(line_type)
                    ds.contents.append(
                        message_contents[i]
                        if message_contents and i < len(message_contents)
                        else ""
                    )

            width = self.ms.width()
            if width is not None and bar.state.status.kind != StatusKind.DONE_HIDDEN:
                lines, contents = bar.style.format_state(
                    bar.state, int(width), bar.draw_target.is_stderr()
                )
                ds.lines.extend(lines)
                ds.contents.extend(contents)

            wrapper.close()
            self.ms.draw(force, None, None, now)
