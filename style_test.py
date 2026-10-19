# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Tests for templates, styling, and formatting."""

from __future__ import annotations

import unittest

from mpb import (
    AtomicPosition,
    ProgressStyle,
    ProgressState,
    Style,
    TemplateError,
    apply_ansi_style,
    formatted_duration,
    human_bytes,
    human_count,
    human_duration,
    human_float_count,
    measure_text_width,
    pad_and_truncate,
    template_from_str,
)


def make_state(pos: int, length: int | None, msg: str = "") -> ProgressState:
    atomic = AtomicPosition()
    atomic.set(pos)
    state = ProgressState(length, atomic)
    state.message.original = msg
    return state


class TemplateTests(unittest.TestCase):
    def test_placeholder_with_width_align_style(self) -> None:
        tpl = template_from_str("{bar:40.red/blue}")
        self.assertEqual(len(tpl.parts), 1)
        part = tpl.parts[0]
        self.assertTrue(part.is_placeholder)
        self.assertEqual(part.key, "bar")
        self.assertEqual(part.width, 40)
        self.assertEqual(part.style, Style(spec="red"))
        self.assertEqual(part.alt_style, Style(spec="blue"))

    def test_align_and_truncate(self) -> None:
        tpl = template_from_str("{msg:>10!}")
        part = tpl.parts[0]
        self.assertEqual(part.key, "msg")
        self.assertEqual(part.align, ">")
        self.assertEqual(part.width, 10)
        self.assertTrue(part.truncate)

    def test_escaped_braces(self) -> None:
        tpl = template_from_str("{{a}} b")
        texts = [p.text for p in tpl.parts if not p.is_placeholder]
        self.assertEqual("".join(texts), "{a} b")

    def test_whitespace_backtracks_to_literal(self) -> None:
        tpl = template_from_str("{not a key}")
        self.assertFalse(any(p.is_placeholder for p in tpl.parts))
        self.assertEqual(
            "".join(p.text for p in tpl.parts), "{not a key}"
        )

    def test_invalid_braces_raise(self) -> None:
        with self.assertRaises(TemplateError):
            template_from_str("{}")
        with self.assertRaises(TemplateError):
            template_from_str("{:>4}")

    def test_literal_newlines(self) -> None:
        tpl = template_from_str("a\nb")
        self.assertEqual(
            [p.text for p in tpl.parts], ["a", "\n", "b"]
        )


class StyleTests(unittest.TestCase):
    def test_padding(self) -> None:
        self.assertEqual(pad_and_truncate("abc", 5, "<", False), "abc  ")
        self.assertEqual(pad_and_truncate("abc", 5, ">", False), "  abc")
        self.assertEqual(pad_and_truncate("abc", 5, "^", False), " abc ")
        self.assertEqual(pad_and_truncate("abcdef", 4, ">", True), "cdef")
        self.assertEqual(pad_and_truncate("abcdef", 4, "<", True), "abcd")

    def test_ansi_style_tokens(self) -> None:
        self.assertEqual(apply_ansi_style("x", "red"), "\x1b[31mx\x1b[0m")
        self.assertEqual(
            apply_ansi_style("x", "bright.red"), "\x1b[38;5;9mx\x1b[0m"
        )
        self.assertEqual(
            apply_ansi_style("x", "on_blue"), "\x1b[44mx\x1b[0m"
        )
        self.assertEqual(
            apply_ansi_style("x", "38;5;9"), "x"
        )  # dotted spec has no ';' tokens
        self.assertEqual(
            apply_ansi_style("x", "9"), "\x1b[38;5;9mx\x1b[0m"
        )
        self.assertEqual(
            apply_ansi_style("x", "on_12"), "\x1b[48;5;12mx\x1b[0m"
        )
        self.assertEqual(
            apply_ansi_style("x", "#ff0000"), "\x1b[38;2;255;0;0mx\x1b[0m"
        )
        self.assertEqual(
            apply_ansi_style("x", "on_#00ff00"),
            "\x1b[48;2;0;255;0mx\x1b[0m",
        )
        self.assertEqual(
            apply_ansi_style("x", "bold.underlined"),
            "\x1b[1m\x1b[4mx\x1b[0m",
        )
        self.assertEqual(apply_ansi_style("x", "italic"), "x")

    def test_format_bar_splits_style_and_alt_style(self) -> None:
        style = ProgressStyle(template_from_str("{bar:20.red/blue}"))
        state = make_state(5, 10)
        _, lines = style.format_state(state, 80)
        self.assertEqual(len(lines), 1)
        self.assertTrue(lines[0].startswith("\x1b[31m"))
        self.assertIn("\x1b[34m", lines[0])
        self.assertEqual(lines[0].count("█"), 10)
        self.assertEqual(lines[0].count("░"), 10)

    def test_format_state_width_alignment_and_percent(self) -> None:
        style = ProgressStyle(template_from_str("{pos:>4}/{len} {percent}%"))
        state = make_state(3, 10)
        _, lines = style.format_state(state, 80)
        self.assertEqual(lines, ["   3/10 30%"])

    def test_format_state_truncates_message(self) -> None:
        style = ProgressStyle(template_from_str("{msg:>4!}"))
        state = make_state(0, 10, "abcdef")
        _, lines = style.format_state(state, 80)
        self.assertEqual(lines, ["cdef"])

    def test_wide_msg_is_right_trimmed(self) -> None:
        style = ProgressStyle(template_from_str("x{wide_msg}"))
        state = make_state(0, 10, "hi")
        _, lines = style.format_state(state, 10)
        self.assertEqual(lines, ["xhi"])

    def test_custom_key(self) -> None:
        style = ProgressStyle(template_from_str("{custom}"))
        style.with_key("custom", lambda s: f"pos={s.pos()}")
        state = make_state(7, 10)
        _, lines = style.format_state(state, 80)
        self.assertEqual(lines, ["pos=7"])

    def test_unknown_key_is_empty(self) -> None:
        style = ProgressStyle(template_from_str("a{unknown}b"))
        state = make_state(0, 10)
        _, lines = style.format_state(state, 80)
        self.assertEqual(lines, ["ab"])

    def test_tab_width(self) -> None:
        tpl = template_from_str("a\tb")
        style = ProgressStyle(tpl)
        style.set_tab_width(4)
        state = make_state(0, 10)
        _, lines = style.format_state(state, 80)
        self.assertEqual(lines, ["a    b"])

    def test_measure_ansi(self) -> None:
        self.assertEqual(measure_text_width("\x1b[31mabc\x1b[0m"), 3)


class FormatTests(unittest.TestCase):
    def test_human_bytes(self) -> None:
        self.assertEqual(human_bytes(0), "0 B")
        self.assertEqual(human_bytes(1536), "1.50 KiB")
        self.assertEqual(human_bytes(1024 * 1024), "1.00 MiB")

    def test_human_count(self) -> None:
        self.assertEqual(human_count(0), "0")
        self.assertEqual(human_count(1234567), "1,234,567")

    def test_human_float_count(self) -> None:
        self.assertEqual(human_float_count(1234.5678, 2), "1,234.57")
        self.assertEqual(human_float_count(1234.5678), "1,234.5678")
        self.assertEqual(human_float_count(-100000, 0), "-100,000")
        self.assertEqual(human_float_count(-0.4, 0), "0")

    def test_durations(self) -> None:
        self.assertEqual(formatted_duration(3661), "01:01:01")
        self.assertEqual(formatted_duration(90061), "1d 01:01:01")
        self.assertEqual(human_duration(5), "5 seconds")
        self.assertEqual(human_duration(5, alt=True), "5s")
        self.assertEqual(human_duration(1), "1 second")


if __name__ == "__main__":
    unittest.main()
