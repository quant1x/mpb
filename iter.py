# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Iterator and IO wrappers (port of ``iter.rs`` / ``rayon.rs``)."""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from typing import Any, Callable, Iterable, Iterator

from .progress_bar import ProgressBar
from .state import ProgressFinish
from .style import ProgressStyle

_SEEK_MAX_RESET = 5
_SEEK_MAX_HISTORY = 10


class SeekMax:
    """Remember the maximum of the last N positions to smooth seeks."""

    def __init__(self) -> None:
        self._buf = [0] * _SEEK_MAX_HISTORY
        self._head = 0
        self._max_pos = 0
        self._seq = 0
        self._active = False

    def update_seq(self, prev: int, delta: int) -> int:
        new_pos = prev + delta
        if not self._active:
            return new_pos
        self._seq += 1
        if self._seq >= _SEEK_MAX_RESET:
            self.reset()
            return new_pos
        self.update(new_pos)
        return self.max()

    def update_seek(self, new_pos: int) -> int:
        self._active = True
        self._seq = 0
        self.update(new_pos)
        return self.max()

    def reset(self) -> None:
        self._active = False
        self._seq = 0
        self._head = 0
        self._max_pos = 0
        self._buf = [0] * _SEEK_MAX_HISTORY

    def update(self, new: int) -> None:
        prev_max = self._buf[self._max_pos]
        self._buf[self._head] = new

        if new > prev_max:
            self._max_pos = self._head
        elif self._max_pos == self._head and new < prev_max:
            best = 0
            for i in range(1, len(self._buf)):
                if self._buf[i] > self._buf[best]:
                    best = i
            self._max_pos = best

        self._head = (self._head + 1) % len(self._buf)

    def max(self) -> int:
        return self._buf[self._max_pos]


class ProgressBarIter:
    """Wrap an iterable so every consumed item advances a progress bar."""

    def __init__(self, iterable: Iterable[Any], progress: ProgressBar) -> None:
        self._iterable = iterable
        self._progress = progress
        self._seek_max = SeekMax()
        self._exhausted = False
        self._indexed = hasattr(iterable, "__len__") and hasattr(iterable, "__getitem__")
        if self._indexed:
            self._idx = 0
            self._end = len(iterable)  # type: ignore[arg-type]
            self._iter: Iterator[Any] | None = None
        else:
            self._iter = iter(iterable)
            self._idx = 0
            self._end = -1

    def __iter__(self) -> "ProgressBarIter":
        return self

    def __next__(self) -> Any:
        if self._exhausted:
            self._finish()
            raise StopIteration

        if self._indexed:
            if self._idx >= self._end:
                self._exhausted = True
                self._finish()
                raise StopIteration
            item = self._iterable[self._idx]  # type: ignore[index]
            self._idx += 1
            self._progress.inc(1)
            return item

        assert self._iter is not None
        try:
            item = next(self._iter)
        except StopIteration:
            self._exhausted = True
            self._finish()
            raise
        self._progress.inc(1)
        return item

    def next_back(self) -> Any:
        if not self._indexed:
            raise TypeError("next_back requires a sized/indexable iterable")
        if self._exhausted:
            self._finish()
            raise StopIteration
        if self._idx >= self._end:
            self._exhausted = True
            self._finish()
            raise StopIteration
        self._end -= 1
        item = self._iterable[self._end]  # type: ignore[index]
        self._progress.inc(1)
        return item

    def _finish(self) -> None:
        self._exhausted = True
        if not self._progress.is_finished():
            self._progress.finish_using_style()

    def __len__(self) -> int:
        if self._exhausted:
            return 0
        if self._indexed:
            return self._end - self._idx
        length = getattr(self._progress, "length", lambda: None)()
        if length is None:
            raise TypeError("object of unknown length")
        return max(0, int(length) - self._progress.position())

    # -- builder methods ------------------------------------------------------

    def with_style(self, style: ProgressStyle) -> "ProgressBarIter":
        self._progress.set_style(style)
        return self

    def with_prefix(self, prefix: str) -> "ProgressBarIter":
        self._progress.set_prefix(prefix)
        return self

    def with_message(self, msg: str) -> "ProgressBarIter":
        self._progress.set_message(msg)
        return self

    def with_position(self, position: int) -> "ProgressBarIter":
        self._progress.set_position(position)
        return self

    def with_elapsed(self, elapsed: float) -> "ProgressBarIter":
        self._progress.set_elapsed(elapsed)
        return self

    def with_finish(self, finish: ProgressFinish) -> "ProgressBarIter":
        self._progress.with_finish(finish)
        return self


class ProgressReader:
    def __init__(self, reader: Any, progress: ProgressBar) -> None:
        self._reader = reader
        self._progress = progress
        self._seek_max = SeekMax()

    def read(self, size: int = -1) -> Any:
        data = self._reader.read(size)
        self._progress.set_position(
            self._seek_max.update_seq(self._progress.position(), len(data))
        )
        return data

    def __getattr__(self, name: str) -> Any:
        return getattr(self._reader, name)


class ProgressWriter:
    def __init__(self, writer: Any, progress: ProgressBar) -> None:
        self._writer = writer
        self._progress = progress
        self._seek_max = SeekMax()

    def write(self, data: Any) -> int:
        written = self._writer.write(data)
        count = written if isinstance(written, int) else len(data)
        self._progress.set_position(
            self._seek_max.update_seq(self._progress.position(), count)
        )
        return written

    def flush(self) -> None:
        flush = getattr(self._writer, "flush", None)
        if flush is not None:
            flush()

    def __getattr__(self, name: str) -> Any:
        return getattr(self._writer, name)


# ---------------------------------------------------------------------------
# Rust's ProgressIterator extension trait
# ---------------------------------------------------------------------------


def _size_hint(iterable: Iterable[Any]) -> int | None:
    try:
        return len(iterable)  # type: ignore[arg-type]
    except TypeError:
        return None


def progress(iterable: Iterable[Any]) -> ProgressBarIter:
    length = _size_hint(iterable)
    pb = ProgressBar(length)
    return ProgressBarIter(iterable, pb)


def progress_count(iterable: Iterable[Any], count: int) -> ProgressBarIter:
    return ProgressBarIter(iterable, ProgressBar(count))


def progress_with(iterable: Iterable[Any], pb: ProgressBar) -> ProgressBarIter:
    return ProgressBarIter(iterable, pb)


def progress_with_style(iterable: Iterable[Any], style: ProgressStyle) -> ProgressBarIter:
    length = _size_hint(iterable)
    pb = ProgressBar(length)
    pb.set_style(style)
    return ProgressBarIter(iterable, pb)


def try_progress(iterable: Iterable[Any]) -> tuple[ProgressBarIter, bool]:
    length = _size_hint(iterable)
    if length is None:
        return ProgressBarIter(iterable, ProgressBar(None)), False
    return ProgressBarIter(iterable, ProgressBar(length)), True


def map_parallel(
    progress_iter: ProgressBarIter,
    workers: int,
    func: Callable[[Any], Any],
) -> list[Any]:
    """Parallel ``map`` over the remaining items, preserving input order.

    This mirrors Go's ``MapParallel`` (Rust's rayon adapter): each consumed
    item advances the bar *before* the function runs, the results keep the
    input order, and the bar is **not** finished automatically.
    """
    if workers <= 0:
        workers = 1

    if progress_iter._indexed:
        items = list(progress_iter._iterable[progress_iter._idx : progress_iter._end])  # type: ignore[index]
        progress_iter._idx = progress_iter._end
    else:
        items = list(progress_iter._iter) if progress_iter._iter is not None else []
        progress_iter._idx = 0
        progress_iter._end = 0
        progress_iter._iter = None
    progress_iter._exhausted = True

    def work(item: Any) -> Any:
        progress_iter._progress.inc(1)
        return func(item)

    if not items:
        return []
    with ThreadPoolExecutor(max_workers=workers) as executor:
        return list(executor.map(work, items))
