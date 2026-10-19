# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Tests ported from the Go implementation's render tests."""

from __future__ import annotations

import unittest

from mpb import (
    DEFAULT_WIDTH,
    DrawState,
    InMemoryTerm,
    LineType,
    ProgressBar,
    RateLimiter,
    ansi_segments,
    default_progress_style_bar,
    draw_to_term,
    in_memory_draw_target,
    measure_text_width,
    new_std_term_with_writer,
    progress_style_with_template,
)
import time


class CountingWriter:
    def __init__(self) -> None:
        self.writes = 0
        self.bytes = 0

    def write(self, data: str) -> int:
        self.writes += 1
        self.bytes += len(data)
        return len(data)

    def flush(self) -> None:
        pass


class RenderTests(unittest.TestCase):
    def test_render_to_in_memory_term(self) -> None:
        term = InMemoryTerm(10, 80)
        pb = ProgressBar.with_draw_target(10, in_memory_draw_target(term))
        try:
            pb.set_style(default_progress_style_bar())
            pb.inc(3)
            pb.force_draw()
            out = term.contents()
            self.assertIn("3/10", out)
        finally:
            pb.abandon()

    def test_wide_bar_fills_terminal_width(self) -> None:
        term_width = 165
        counter_cols = 10  # " " + " 651" + "/" + "1024"
        term = InMemoryTerm(24, term_width)
        pb = ProgressBar.with_draw_target(None, in_memory_draw_target(term))
        try:
            style = progress_style_with_template("{wide_bar} {pos:>4}/{len}")
            pb.set_length(1024)
            pb.set_style(style)
            pb.set_position(651)
            pb.force_draw()

            line = term.contents()
            bar = line.count("█") + line.count("░")
            self.assertEqual(bar, term_width - counter_cols, line)
            self.assertIn("651/1024", line)
        finally:
            pb.abandon()

    def test_wide_bar_gives_way_for_wide_message(self) -> None:
        term_width = 40
        term = InMemoryTerm(24, term_width)
        pb = ProgressBar.with_draw_target(None, in_memory_draw_target(term))
        try:
            pb.set_length(1024)
            pb.set_style(progress_style_with_template("{wide_bar} {msg}"))
            pb.set_message("１")  # full-width digit: 2 columns
            pb.set_position(512)
            pb.force_draw()

            line = term.contents()
            bar = line.count("█") + line.count("░")
            self.assertEqual(bar, term_width - 3, line)
        finally:
            pb.abandon()


class WidthTests(unittest.TestCase):
    def tearDown(self) -> None:
        DEFAULT_WIDTH.set_default_cjk(False)

    def test_default_width(self) -> None:
        self.assertEqual(DEFAULT_WIDTH.char("█"), 1)
        self.assertEqual(DEFAULT_WIDTH.str("██"), 2)
        self.assertEqual(DEFAULT_WIDTH.char("あ"), 2)

        DEFAULT_WIDTH.set_default_cjk(True)
        self.assertEqual(DEFAULT_WIDTH.char("█"), 2)
        DEFAULT_WIDTH.set_default_cjk(False)

    def test_measure_text_width_matches_upstream(self) -> None:
        cases = [
            ("带 ANSI", "\x1b[31m█\x1b[0m", 1),
            ("真宽字符", "あ", 2),
            ("歧义字符默认 1 列", "█", 1),
            ("全角数字不归一化", "１", 2),
            ("制表符是控制字符", "\t", 0),
            ("回车换行为 0 列", "a\r\nb", 2),
            ("NUL 为 0 列", "a\x00b", 2),
        ]
        for name, text, want in cases:
            with self.subTest(name=name):
                self.assertEqual(measure_text_width(text), want)

        self.assertEqual(
            measure_text_width("👨\u200d👩"), DEFAULT_WIDTH.str("👨\u200d👩")
        )

    def test_ansi_segments_matches_upstream_dfa(self) -> None:
        cases = [
            ("SGR 前后", "\x1b[31m█\x1b[0m", [("\x1b[31m", True), ("█", False), ("\x1b[0m", True)]),
            ("文本在前", "ab\x1b[0m", [("ab", False), ("\x1b[0m", True)]),
            ("真彩色 SGR", "\x1b[38;2;255;0;0m", [("\x1b[38;2;255;0;0m", True)]),
            ("字符集指定", "\x1b(B", [("\x1b(B", True)]),
            ("单字节 CSI 起始符", "\u009b31m", [("\u009b31m", True)]),
            ("OSC 上游不认", "\x1b]8;;url\x07x", [("\x1b]8;;url\x07x", False)]),
            ("结束符不在集合内", "\x1bX", [("\x1bX", False)]),
            ("纯文本", "abc", [("abc", False)]),
        ]
        for name, text, want in cases:
            with self.subTest(name=name):
                self.assertEqual(ansi_segments(text), want)


class FrameTests(unittest.TestCase):
    def _state(self) -> DrawState:
        return DrawState(
            lines=[LineType.BAR, LineType.BAR],
            contents=["first line", "second line"],
        )

    def test_std_term_writes_one_buffer_per_frame(self) -> None:
        frames = 5
        writer = CountingWriter()
        term = new_std_term_with_writer(writer, buffered=True)
        for _ in range(frames):
            draw_to_term(self._state(), term, 0)
        self.assertEqual(writer.writes, frames)

        raw = CountingWriter()
        raw_term = new_std_term_with_writer(raw, buffered=False)
        draw_to_term(self._state(), raw_term, 0)
        self.assertGreater(raw.writes, 1)

    def test_rate_limiter_throttles_to_refresh_hz(self) -> None:
        limiter = RateLimiter(20)
        base = time.monotonic()
        allowed = 0
        for i in range(1000):
            if limiter.allow(base + i / 1000.0):
                allowed += 1
        self.assertGreaterEqual(allowed, 30)
        self.assertLessEqual(allowed, 50)


if __name__ == "__main__":
    unittest.main()
