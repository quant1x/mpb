# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Tests for MultiProgress, iterators, state sharing and the in-memory terminal."""

from __future__ import annotations

import io
import threading
import time
import unittest

from mpb import (
    InMemoryTerm,
    MultiProgress,
    MultiProgressAlignment,
    ProgressBar,
    ProgressBarIter,
    ProgressReader,
    ProgressWriter,
    SeekMax,
    FinishKind,
    ProgressFinish,
    default_progress_style_bar,
    hidden_draw_target,
    in_memory_draw_target,
    map_parallel,
    progress_with,
)


class MultiTests(unittest.TestCase):
    def setUp(self) -> None:
        self.term = InMemoryTerm(10, 60)
        self.mp = MultiProgress.with_draw_target(in_memory_draw_target(self.term))

    def test_two_bars(self) -> None:
        p1 = self.mp.add(ProgressBar(10))
        p2 = self.mp.add(ProgressBar(20))
        p1.inc(3)
        p2.inc(5)

        lines = self.term.contents().splitlines()
        self.assertEqual(len(lines), 2)
        self.assertIn("3/10", lines[0])
        self.assertIn("5/20", lines[1])

    def test_visual_index_order(self) -> None:
        p1 = self.mp.add(ProgressBar(10))
        p2 = self.mp.add(ProgressBar(10))
        p3 = self.mp.insert_before(p2, ProgressBar(10))

        self.assertEqual(p1.visual_index(), (0, True))
        self.assertEqual(p3.visual_index(), (1, True))
        self.assertEqual(p2.visual_index(), (2, True))

        p4 = self.mp.insert_after(p1, ProgressBar(10))
        self.assertEqual(p4.visual_index(), (1, True))
        self.assertEqual(p1.visual_index(), (0, True))

    def test_remove(self) -> None:
        p1 = self.mp.add(ProgressBar(10))
        p2 = self.mp.add(ProgressBar(10))
        p1.inc(3)
        p2.inc(4)
        self.mp.remove(p1)
        p2.inc(1)

        lines = self.term.contents().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertIn("5/10", lines[0])

    def test_println_stays_above_bars(self) -> None:
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        self.mp.println("hello")
        pb.inc(1)

        lines = self.term.contents().splitlines()
        self.assertEqual(lines[0], "hello")
        self.assertIn("4/10", lines[1])

    def test_clear(self) -> None:
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        self.mp.clear()
        self.assertEqual(self.term.contents(), "")

    def test_suspend(self) -> None:
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        seen: list[str] = []

        def action() -> str:
            seen.append(self.term.contents())
            return "result"

        result = self.mp.suspend(action)
        self.assertEqual(result, "result")
        self.assertEqual(seen, [""])
        self.assertIn("3/10", self.term.contents())

    def test_force_draw_does_not_duplicate_member_lines(self) -> None:
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        pb.force_draw()
        pb.force_draw()
        lines = self.term.contents().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertIn("3/10", lines[0])

    def test_finish_and_clear(self) -> None:
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        self.assertIn("3/10", self.term.contents())
        pb.finish_and_clear()
        self.assertEqual(self.term.contents(), "")

    def test_finish_leaves_full_bar(self) -> None:
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        pb.finish()
        self.assertIn("10/10", self.term.contents())

    def test_single_bar_println(self) -> None:
        term = InMemoryTerm(10, 50)
        pb = ProgressBar.with_draw_target(10, in_memory_draw_target(term))
        pb.inc(3)
        pb.println("log line")
        pb.inc(1)
        lines = term.contents().splitlines()
        self.assertEqual(lines[0], "log line")
        self.assertIn("4/10", lines[1])

    def test_bottom_alignment(self) -> None:
        self.mp.set_alignment(MultiProgressAlignment.BOTTOM)
        pb = self.mp.add(ProgressBar(10))
        pb.inc(3)
        # One bar in a tall terminal is shifted to the bottom.
        lines = self.term.contents().splitlines()
        self.assertIn("3/10", lines[-1])

    def test_concurrent_updates(self) -> None:
        term = InMemoryTerm(20, 60)
        mp = MultiProgress.with_draw_target(in_memory_draw_target(term))
        bars = [mp.add(ProgressBar(100)) for _ in range(4)]
        threads = [
            threading.Thread(target=lambda bar=bar: [bar.inc(1) for _ in range(100)])
            for bar in bars
        ]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join(timeout=5)
            self.assertFalse(thread.is_alive())
        self.assertTrue(all(bar.position() == 100 for bar in bars))


class IterTests(unittest.TestCase):
    def test_progress_advances_and_finishes(self) -> None:
        items = list(range(5))
        pb = ProgressBar.with_draw_target(5, hidden_draw_target())
        it = progress_with(items, pb)
        collected = list(it)
        self.assertEqual(collected, items)
        self.assertEqual(pb.position(), 5)
        self.assertTrue(pb.is_finished())

    def test_progress_with_custom_bar(self) -> None:
        pb = ProgressBar.with_draw_target(3, hidden_draw_target())
        it = progress_with(["a", "b", "c"], pb)
        self.assertEqual(list(it), ["a", "b", "c"])
        self.assertEqual(pb.position(), 3)
        self.assertTrue(pb.is_finished())

    def test_progress_count(self) -> None:
        pb = ProgressBar.with_draw_target(10, hidden_draw_target())
        it = ProgressBarIter(iter(range(10)), pb)
        self.assertEqual(len(it), 10)
        next(it)
        self.assertEqual(len(it), 9)

    def test_next_back(self) -> None:
        pb = ProgressBar.with_draw_target(4, hidden_draw_target())
        it = ProgressBarIter([1, 2, 3, 4], pb)
        self.assertEqual(it.next_back(), 4)
        self.assertEqual(next(it), 1)
        self.assertEqual(len(it), 2)
        self.assertEqual(list(it), [2, 3])
        self.assertTrue(pb.is_finished())

    def test_map_parallel_preserves_order(self) -> None:
        pb = ProgressBar.with_draw_target(10, hidden_draw_target())
        it = ProgressBarIter(list(range(10)), pb)
        result = map_parallel(it, 4, lambda value: value * value)
        self.assertEqual(result, [value * value for value in range(10)])
        self.assertEqual(pb.position(), 10)
        self.assertFalse(pb.is_finished())
        self.assertEqual(len(it), 0)

    def test_reader_writer(self) -> None:
        pb = ProgressBar.with_draw_target(100, hidden_draw_target())
        reader = ProgressReader(io.BytesIO(b"x" * 20), pb)
        self.assertEqual(len(reader.read(10)), 10)
        self.assertEqual(pb.position(), 10)

        pb2 = ProgressBar.with_draw_target(100, hidden_draw_target())
        sink = io.BytesIO()
        writer = ProgressWriter(sink, pb2)
        writer.write(b"y" * 7)
        writer.flush()
        self.assertEqual(pb2.position(), 7)
        self.assertEqual(sink.getvalue(), b"y" * 7)

    def test_seek_max(self) -> None:
        seek = SeekMax()
        self.assertEqual(seek.update_seq(10, 5), 15)

        self.assertEqual(seek.update_seek(100), 100)
        self.assertEqual(seek.update_seq(100, 1), 101)
        # A backwards seek does not lower the reported position.
        self.assertEqual(seek.update_seq(50, 1), 101)
        self.assertEqual(seek.max(), 101)

        # After SEEK_MAX_RESET sequential updates the history is dropped and
        # positions become pass-through again.
        for _ in range(2):
            seek.update_seq(seek.max(), 1)
        seek.update_seq(seek.max(), 1)  # this one crosses the reset threshold
        self.assertEqual(seek.update_seq(1, 1), 2)


class SharingTests(unittest.TestCase):
    def test_clone_shares_state(self) -> None:
        pb = ProgressBar.with_draw_target(100, hidden_draw_target())
        clone = pb.clone()
        try:
            pb.with_message("hello")
            self.assertEqual(clone.message(), "hello")

            pb.inc(5)
            self.assertEqual(clone.position(), 5)

            clone.finish()
            self.assertTrue(pb.is_finished())
        finally:
            pb.abandon()

    def test_weak_upgrade(self) -> None:
        pb = ProgressBar.with_draw_target(1, hidden_draw_target())
        clone = pb.clone()
        weak = pb.downgrade()
        self.assertIsNotNone(weak.upgrade())
        self.assertIsNotNone(weak.upgrade())
        self.assertIs(weak.upgrade().state, clone.state)
        clone.abandon()

    def test_finish_variants(self) -> None:
        pb = ProgressBar.with_draw_target(10, hidden_draw_target())
        pb.finish_with_message("done")
        self.assertTrue(pb.is_finished())
        self.assertEqual(pb.position(), 10)
        self.assertEqual(pb.message(), "done")

        pb2 = ProgressBar.with_draw_target(10, hidden_draw_target())
        pb2.abandon_with_message("stopped")
        self.assertTrue(pb2.is_finished())
        self.assertEqual(pb2.position(), 0)
        self.assertEqual(pb2.message(), "stopped")

    def test_update(self) -> None:
        pb = ProgressBar.with_draw_target(10, hidden_draw_target())
        pb.update(lambda state: state.set_len(3))
        self.assertEqual(pb.length(), 3)

    def test_steady_tick(self) -> None:
        pb = ProgressBar.with_draw_target(None, hidden_draw_target())
        pb.enable_steady_tick(0.01)
        time.sleep(0.05)
        pb.disable_steady_tick()
        self.assertGreater(pb.state.state.tick, 0)


class InMemoryTermTests(unittest.TestCase):
    def test_write_and_contents(self) -> None:
        term = InMemoryTerm(3, 10)
        term.write_str("hello")
        self.assertEqual(term.contents(), "hello")

    def test_write_line_and_scroll(self) -> None:
        term = InMemoryTerm(2, 10)
        term.write_line("a")
        term.write_line("b")
        term.write_line("c")
        # The simplified screen buffer clamps at the last row instead of
        # scrolling; this matches the Go port's vte performer.
        self.assertEqual(term.contents(), "a\nc")

    def test_clear_line(self) -> None:
        term = InMemoryTerm(2, 10)
        term.write_str("abcdef")
        term.move_cursor_left(3)
        term.clear_line()
        self.assertEqual(term.contents(), "")

    def test_moves_history(self) -> None:
        term = InMemoryTerm(5, 10)
        term.move_cursor_down(2)
        term.write_str("x")
        term.move_cursor_up(1)
        term.flush()
        self.assertEqual(
            term.moves_since_last_check(),
            'Down(2)\nStr("x")\nUp(1)\nFlush\n',
        )
        self.assertEqual(term.moves_since_last_check(), "")

    def test_contents_formatted(self) -> None:
        term = InMemoryTerm(3, 10)
        term.write_str("ab")
        self.assertEqual(term.contents_formatted(), b"ab\x1b[m")

    def test_reset(self) -> None:
        term = InMemoryTerm(2, 10)
        term.write_str("x")
        term.reset()
        self.assertEqual(term.contents(), "")


if __name__ == "__main__":
    unittest.main()
