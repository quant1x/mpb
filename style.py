# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Templates, styles, ANSI handling and width measurement (port of ``style.rs``)."""

from __future__ import annotations

import math
import string
import unicodedata
from dataclasses import dataclass, field
from typing import Any, Callable, Iterable

from .format import (
    binary_bytes,
    decimal_bytes,
    formatted_duration,
    human_bytes,
    human_count,
    human_duration,
    human_float_count,
)
from ._types import U64_MAX, LineType, f32

# ---------------------------------------------------------------------------
# Width
# ---------------------------------------------------------------------------


class Width:
    """Width measurement entry point (``Width`` in Rust 0.18.6).

    Ambiguous East-Asian characters (``█``, ``○``, ``±`` ...) are one column
    by default.  ``set_default_cjk(True)`` switches them to two columns.

    Rust keeps this switch in a global atomic, so every ``Width`` value shares
    it; Python mirrors that with a module-level flag.
    """

    def set_default_cjk(self, is_cjk: bool) -> None:
        global _CJK_WIDE
        _CJK_WIDE = bool(is_cjk)

    def char(self, ch: str) -> int:
        return char_width(ch, _CJK_WIDE)

    def str(self, s: str) -> int:
        return str_width(s, _CJK_WIDE)

    def ansi_str(self, s: str) -> int:
        return measure_text_width(s)


_CJK_WIDE = False
DEFAULT_WIDTH = Width()


def char_width(ch: str, cjk: bool = False) -> int:
    if not ch:
        return 0
    cp = ord(ch)
    if cp < 0x20 or 0x7F <= cp <= 0x9F:
        return 0
    category = unicodedata.category(ch)
    if category in ("Mn", "Me", "Cf", "Cc", "Cs"):
        return 0
    east_asian = unicodedata.east_asian_width(ch)
    if east_asian in ("W", "F"):
        return 2
    if east_asian == "A":
        return 2 if cjk else 1
    return 1


def str_width(s: str, cjk: bool = False) -> int:
    return sum(char_width(ch, cjk) for ch in s)


# ---------------------------------------------------------------------------
# ANSI segmentation (the hand-written DFA from console::ansi)
# ---------------------------------------------------------------------------

_ANSI_START = 0
_ANSI_S1, _ANSI_S2, _ANSI_S3, _ANSI_S4 = 1, 2, 3, 4
_ANSI_S5, _ANSI_S6, _ANSI_S7, _ANSI_S8 = 5, 6, 7, 8
_ANSI_S9, _ANSI_S10, _ANSI_S11, _ANSI_TRAP = 9, 10, 11, 12

_FINAL_STATES = frozenset((_ANSI_S3, _ANSI_S5, _ANSI_S6, _ANSI_S7, _ANSI_S8, _ANSI_S9, _ANSI_S11))


def _is_digit_0_2(c: str) -> bool:
    return "0" <= c <= "2"


def _is_digit_3_9(c: str) -> bool:
    return "3" <= c <= "9"


def _is_final_char(c: str) -> bool:
    return (
        ("A" <= c <= "P")
        or c in ("R", "Z", "c", "q", "r", "y", "=", ">", "<")
        or ("f" <= c <= "n")
    )


def _transition(state: int, c: str) -> int:
    code = ord(c)
    if code in (0x1B, 0x9B):
        if state == _ANSI_START:
            return _ANSI_S1
        return _ANSI_TRAP

    if c in ("(", ")"):
        if state == _ANSI_S1:
            return _ANSI_S2
        if state in (_ANSI_S2, _ANSI_S4):
            return _ANSI_S4
        return _ANSI_TRAP

    if c == ";":
        if state in (_ANSI_S1, _ANSI_S2, _ANSI_S4):
            return _ANSI_S4
        if state in (_ANSI_S5, _ANSI_S6, _ANSI_S7, _ANSI_S8, _ANSI_S10):
            return _ANSI_S10
        return _ANSI_TRAP

    if c in ("[", "#", "?"):
        if state in (_ANSI_S1, _ANSI_S2, _ANSI_S4):
            return _ANSI_S4
        return _ANSI_TRAP

    if _is_digit_0_2(c):
        if state in (_ANSI_S1, _ANSI_S4):
            return _ANSI_S5
        if state == _ANSI_S2:
            return _ANSI_S3
        if state == _ANSI_S5:
            return _ANSI_S6
        if state == _ANSI_S6:
            return _ANSI_S7
        if state == _ANSI_S7:
            return _ANSI_S8
        if state == _ANSI_S8:
            return _ANSI_S9
        if state == _ANSI_S10:
            return _ANSI_S5
        return _ANSI_TRAP

    if _is_digit_3_9(c):
        if state in (_ANSI_S1, _ANSI_S4):
            return _ANSI_S5
        if state == _ANSI_S2:
            return _ANSI_S5
        if state == _ANSI_S5:
            return _ANSI_S6
        if state == _ANSI_S6:
            return _ANSI_S7
        if state == _ANSI_S7:
            return _ANSI_S8
        if state == _ANSI_S8:
            return _ANSI_S9
        if state == _ANSI_S10:
            return _ANSI_S5
        return _ANSI_TRAP

    if _is_final_char(c):
        if state in (_ANSI_S1, _ANSI_S2, _ANSI_S4, _ANSI_S5, _ANSI_S6, _ANSI_S7, _ANSI_S8, _ANSI_S10):
            return _ANSI_S11
        return _ANSI_TRAP

    return _ANSI_TRAP


def _match_ansi_code_at(s: str, index: int) -> tuple[int, bool]:
    state = _ANSI_START
    maybe_end = -1
    j = index
    while j < len(s):
        state = _transition(state, s[j])
        if state in _FINAL_STATES:
            maybe_end = j
        if state == _ANSI_TRAP:
            break
        j += 1
    if maybe_end < 0:
        return 0, False
    return maybe_end + 1, True


def ansi_segments(s: str) -> list[tuple[str, bool]]:
    """Split ``s`` into ``(text, is_ansi)`` segments."""
    segments: list[tuple[str, bool]] = []
    start = 0
    i = 0
    while i < len(s):
        if s[i] in ("\x1b", "\x9b"):
            end, ok = _match_ansi_code_at(s, i)
            if ok:
                if i > start:
                    segments.append((s[start:i], False))
                segments.append((s[i:end], True))
                start = end
                i = end
                continue
        i += 1
    if start < len(s):
        segments.append((s[start:], False))
    return segments


def measure_text_width(s: str) -> int:
    """``console::measure_text_width``: skip ANSI, sum the rest."""
    if not s:
        return 0
    total = 0
    for text, is_ansi in ansi_segments(s):
        if not is_ansi:
            total += str_width(text, _CJK_WIDE)
    return total


def ansi_slice(s: str, start: int, end: int) -> str:
    """Substring corresponding to visible columns ``[start, end)``."""
    if start < 0:
        start = 0
    if end <= start:
        return ""

    out: list[str] = []
    col = 0
    for text, is_ansi in ansi_segments(s):
        if is_ansi:
            out.append(text)
            continue
        for ch in text:
            w = DEFAULT_WIDTH.char(ch)
            if start <= col and col + w <= end:
                out.append(ch)
            col += w
            if col > end:
                break
        if col > end:
            break
    return "".join(out)


def pad_and_truncate(s: str, width: int, align: str, truncate: bool) -> str:
    if width <= 0:
        return ""
    cols = measure_text_width(s)
    if cols == width:
        return s
    if cols > width:
        if not truncate:
            return s
        if align == "^":
            start_col = (cols - width) // 2
        elif align == ">":
            start_col = cols - width
        else:
            start_col = 0
        start_col = max(0, start_col)
        end_col = start_col + width
        s = ansi_slice(s, start_col, end_col)
        cols = measure_text_width(s)

    pad = width - cols
    if align == "^":
        left_pad = pad // 2
        right_pad = pad - left_pad
    elif align == ">":
        left_pad, right_pad = pad, 0
    else:
        left_pad, right_pad = 0, pad
    return " " * left_pad + s + " " * right_pad


def _u64_from_float(value: float) -> int:
    """Rust's saturating ``f64 as u64`` cast (NaN -> 0, +Inf -> u64::MAX)."""
    if math.isnan(value) or value <= 0:
        return 0
    if math.isinf(value):
        return U64_MAX
    return min(int(value), U64_MAX)


# ---------------------------------------------------------------------------
# Style specs (console::Style)
# ---------------------------------------------------------------------------

_ATTR_BOLD = 1 << 0
_ATTR_DIM = 1 << 1
_ATTR_ITALIC = 1 << 2
_ATTR_UNDERLINED = 1 << 3
_ATTR_BLINK = 1 << 4
_ATTR_BLINK_FAST = 1 << 5
_ATTR_REVERSE = 1 << 6
_ATTR_HIDDEN = 1 << 7
_ATTR_STRIKETHROUGH = 1 << 8

_FG_NAMES = {
    "black": 0,
    "red": 1,
    "green": 2,
    "yellow": 3,
    "blue": 4,
    "magenta": 5,
    "cyan": 6,
    "white": 7,
}

_ATTR_NAMES = {
    "bold": _ATTR_BOLD,
    "dim": _ATTR_DIM,
    "italic": _ATTR_ITALIC,
    "underlined": _ATTR_UNDERLINED,
    "blink": _ATTR_BLINK,
    "blink_fast": _ATTR_BLINK_FAST,
    "reverse": _ATTR_REVERSE,
    "hidden": _ATTR_HIDDEN,
    "strikethrough": _ATTR_STRIKETHROUGH,
}


@dataclass
class _DottedStyle:
    fg: int = 0
    fg_set: bool = False
    fg_256: int = -1
    fg_rgb: tuple[int, int, int] = (0, 0, 0)
    fg_is_rgb: bool = False
    fg_bright: bool = False
    bg: int = 0
    bg_set: bool = False
    bg_256: int = -1
    bg_rgb: tuple[int, int, int] = (0, 0, 0)
    bg_is_rgb: bool = False
    bg_bright: bool = False
    attr_bits: int = 0

    def render(self, text: str) -> str:
        out: list[str] = []
        reset = False

        if self.fg_is_rgb:
            out.append(f"\x1b[38;2;{self.fg_rgb[0]};{self.fg_rgb[1]};{self.fg_rgb[2]}m")
            reset = True
        elif self.fg_256 >= 0:
            out.append(f"\x1b[38;5;{self.fg_256}m")
            reset = True
        elif self.fg_set:
            if self.fg_bright:
                out.append(f"\x1b[38;5;{self.fg + 8}m")
            else:
                out.append(f"\x1b[{self.fg + 30}m")
            reset = True

        if self.bg_is_rgb:
            out.append(f"\x1b[48;2;{self.bg_rgb[0]};{self.bg_rgb[1]};{self.bg_rgb[2]}m")
            reset = True
        elif self.bg_256 >= 0:
            out.append(f"\x1b[48;5;{self.bg_256}m")
            reset = True
        elif self.bg_set:
            if self.bg_bright:
                out.append(f"\x1b[48;5;{self.bg + 8}m")
            else:
                out.append(f"\x1b[{self.bg + 40}m")
            reset = True

        for bit in range(9):
            if self.attr_bits & (1 << bit):
                out.append(f"\x1b[{bit + 1}m")
                reset = True

        out.append(text)
        if reset:
            out.append("\x1b[0m")
        return "".join(out)


def _parse_hex(value: str) -> int | None:
    if len(value) != 2 or any(c not in string.hexdigits for c in value):
        return None
    return int(value, 16)


def _parse_dotted_style(spec: str) -> _DottedStyle:
    style = _DottedStyle()
    for part in spec.split("."):
        if part in _FG_NAMES:
            style.fg = _FG_NAMES[part]
            style.fg_set = True
        elif part == "bright":
            style.fg_bright = True
        elif part.startswith("on_") and part[3:] in _FG_NAMES:
            style.bg = _FG_NAMES[part[3:]]
            style.bg_set = True
        elif part == "on_bright":
            style.bg_bright = True
        elif part in _ATTR_NAMES:
            style.attr_bits |= _ATTR_NAMES[part]
        elif part.startswith("on_#") and len(part) == 10:
            r, g, b = (_parse_hex(part[4:6]), _parse_hex(part[6:8]), _parse_hex(part[8:10]))
            if r is not None and g is not None and b is not None:
                style.bg_rgb = (r, g, b)
                style.bg_is_rgb = True
        elif part.startswith("#") and len(part) == 7:
            r, g, b = (_parse_hex(part[1:3]), _parse_hex(part[3:5]), _parse_hex(part[5:7]))
            if r is not None and g is not None and b is not None:
                style.fg_rgb = (r, g, b)
                style.fg_is_rgb = True
        elif part.startswith("on_"):
            try:
                n = int(part[3:])
            except ValueError:
                continue
            if 0 <= n <= 255:
                style.bg_256 = n
        else:
            try:
                n = int(part)
            except ValueError:
                continue
            if 0 <= n <= 255:
                style.fg_256 = n
    return style


def apply_ansi_style(text: str, spec: str) -> str:
    if not spec or not text:
        return text
    return _parse_dotted_style(spec).render(text)


@dataclass
class Style:
    spec: str = ""
    spec_err: str = ""

    def apply_for(self, text: str, for_stderr: bool = False) -> str:
        spec = self.spec_err if (for_stderr and self.spec_err) else self.spec
        if not spec:
            return text
        return apply_ansi_style(text, spec)


# ---------------------------------------------------------------------------
# Template
# ---------------------------------------------------------------------------


@dataclass
class TemplatePart:
    is_placeholder: bool = False
    text: str = ""
    key: str = ""
    align: str = "<"
    width: int = 0
    truncate: bool = False
    style: Style | None = None
    alt_style: Style | None = None


class TemplateError(Exception):
    def __init__(self, state: str, next_char: str) -> None:
        super().__init__(f'TemplateError: unexpected character {next_char!r} in state {state!r}')
        self.state = state
        self.next_char = next_char


@dataclass
class Template:
    parts: list[TemplatePart] = field(default_factory=list)
    original_texts: list[str] | None = None
    current_tab_width: int = 0

    @classmethod
    def from_str(cls, text: str) -> "Template":
        return template_from_str(text)

    @classmethod
    def from_str_with_tab_width(cls, text: str, tab_width: int) -> "Template":
        return template_from_str_with_tab_width(text, tab_width)

    def clone(self) -> "Template":
        def copy_part(part: TemplatePart) -> TemplatePart:
            return TemplatePart(
                is_placeholder=part.is_placeholder,
                text=part.text,
                key=part.key,
                align=part.align,
                width=part.width,
                truncate=part.truncate,
                style=None
                if part.style is None
                else Style(spec=part.style.spec, spec_err=part.style.spec_err),
                alt_style=None
                if part.alt_style is None
                else Style(spec=part.alt_style.spec, spec_err=part.alt_style.spec_err),
            )

        return Template(
            parts=[copy_part(part) for part in self.parts],
            original_texts=None
            if self.original_texts is None
            else list(self.original_texts),
            current_tab_width=self.current_tab_width,
        )

    def set_tab_width(self, new_tab_width: int) -> None:
        if new_tab_width <= 0 or self.current_tab_width == new_tab_width:
            return
        tab = " " * new_tab_width

        if self.original_texts is None:
            self.original_texts = [""] * len(self.parts)
            for i, part in enumerate(self.parts):
                if not part.is_placeholder and "\t" in part.text:
                    self.original_texts[i] = part.text
                    part.text = part.text.replace("\t", tab)
            self.current_tab_width = new_tab_width
            return

        for i, part in enumerate(self.parts):
            if i < len(self.original_texts) and self.original_texts[i] != "":
                part.text = self.original_texts[i].replace("\t", tab)
        self.current_tab_width = new_tab_width


_STATE_LITERAL = "Literal"
_STATE_MAYBE_OPEN = "MaybeOpen"
_STATE_KEY = "Key"
_STATE_ALIGN = "Align"
_STATE_WIDTH = "Width"
_STATE_FIRST_STYLE = "FirstStyle"
_STATE_ALT_STYLE = "AltStyle"
_STATE_DOUBLE_CLOSE = "DoubleClose"

_WHITESPACE = " \t\n\r\x0b\x0c"


def template_from_str_with_tab_width(s: str, tab_width: int) -> Template:
    if tab_width <= 0:
        tab_width = 8
    parts: list[TemplatePart] = []
    buf: list[str] = []
    state = _STATE_LITERAL

    def buf_text() -> str:
        return "".join(buf)

    def clear_buf() -> None:
        buf.clear()

    def append_literal(text: str) -> None:
        parts.append(TemplatePart(is_placeholder=False, text=text))

    def last_placeholder() -> TemplatePart | None:
        if not parts:
            return None
        part = parts[-1]
        return part if part.is_placeholder else None

    for c in s:
        new_state = state
        keep: str | None = None

        if state == _STATE_LITERAL:
            if c == "{":
                new_state = _STATE_MAYBE_OPEN
            elif c == "\n":
                if buf:
                    append_literal(buf_text())
                    clear_buf()
                append_literal("\n")
                new_state = _STATE_LITERAL
            elif c == "}":
                new_state = _STATE_DOUBLE_CLOSE
                keep = c
            else:
                new_state = _STATE_LITERAL
                keep = c

        elif state == _STATE_DOUBLE_CLOSE:
            if c == "}":
                new_state = _STATE_LITERAL
            else:
                raise TemplateError(state, c)

        elif state == _STATE_MAYBE_OPEN:
            if c == "{":
                new_state = _STATE_LITERAL
                keep = c
            elif c in _WHITESPACE:
                append_literal("{" + buf_text() + c)
                clear_buf()
                new_state = _STATE_LITERAL
            elif c != "}" and c != ":":
                new_state = _STATE_KEY
                keep = c
            else:
                raise TemplateError(state, c)

        elif state == _STATE_KEY:
            if c in _WHITESPACE:
                append_literal("{" + buf_text() + c)
                clear_buf()
                new_state = _STATE_LITERAL
            elif c == "}" or c == ":":
                new_state = _STATE_LITERAL if c == "}" else _STATE_ALIGN
            elif c == "!" and buf:
                parts.append(
                    TemplatePart(is_placeholder=True, key=buf_text(), align="<", truncate=True)
                )
                clear_buf()
                new_state = _STATE_WIDTH
            else:
                new_state = _STATE_KEY
                keep = c

        elif state == _STATE_ALIGN:
            if c in ("<", "^", ">"):
                part = last_placeholder()
                if part is not None:
                    part.align = c
                new_state = _STATE_WIDTH
            elif "0" <= c <= "9":
                new_state = _STATE_WIDTH
                keep = c
            elif c == "!":
                part = last_placeholder()
                if part is not None:
                    part.truncate = True
                new_state = _STATE_WIDTH
            elif c == ".":
                new_state = _STATE_FIRST_STYLE
            elif c == "}":
                new_state = _STATE_LITERAL
            else:
                raise TemplateError(state, c)

        elif state == _STATE_WIDTH:
            if "0" <= c <= "9":
                new_state = _STATE_WIDTH
                keep = c
            elif c == "!":
                part = last_placeholder()
                if part is not None:
                    part.truncate = True
                new_state = _STATE_WIDTH
            elif c == ".":
                new_state = _STATE_FIRST_STYLE
            elif c == "}":
                new_state = _STATE_LITERAL
            else:
                raise TemplateError(state, c)

        elif state == _STATE_FIRST_STYLE:
            if c == "/":
                new_state = _STATE_ALT_STYLE
            elif c == "}":
                new_state = _STATE_LITERAL
            else:
                new_state = _STATE_FIRST_STYLE
                keep = c

        elif state == _STATE_ALT_STYLE:
            if c == "}":
                new_state = _STATE_LITERAL
            else:
                new_state = _STATE_ALT_STYLE
                keep = c

        # The transition block: decide what to do with `buf` based on the
        # (old state, new state) pair.
        if state == _STATE_MAYBE_OPEN and new_state == _STATE_KEY and buf:
            append_literal(buf_text())
            clear_buf()
        elif state == _STATE_KEY and new_state in (_STATE_ALIGN, _STATE_LITERAL) and buf:
            parts.append(
                TemplatePart(is_placeholder=True, key=buf_text(), align="<")
            )
            clear_buf()
        elif state == _STATE_WIDTH and new_state in (_STATE_FIRST_STYLE, _STATE_LITERAL) and buf:
            part = last_placeholder()
            if part is not None:
                try:
                    value = int(buf_text())
                except ValueError:
                    pass
                else:
                    # Go's strconv.Atoi overflows at MaxInt64 and the width
                    # stays 0; mirror that instead of allocating huge strings.
                    if 0 <= value <= (1 << 63) - 1:
                        part.width = value
            clear_buf()
        elif state == _STATE_FIRST_STYLE and new_state in (_STATE_ALT_STYLE, _STATE_LITERAL) and buf:
            part = last_placeholder()
            if part is not None:
                part.style = Style(spec=buf_text())
            clear_buf()
        elif state == _STATE_ALT_STYLE and new_state == _STATE_LITERAL and buf:
            part = last_placeholder()
            if part is not None:
                part.alt_style = Style(spec=buf_text())
            clear_buf()

        state = new_state
        if keep is not None:
            buf.append(keep)

    if state in (_STATE_LITERAL, _STATE_DOUBLE_CLOSE) and buf:
        append_literal(buf_text())
        clear_buf()

    tmpl = Template(parts=parts, original_texts=[""] * len(parts), current_tab_width=tab_width)
    for i, part in enumerate(parts):
        if not part.is_placeholder and "\t" in part.text:
            tmpl.original_texts[i] = part.text
    return tmpl


def template_from_str(s: str) -> Template:
    return template_from_str_with_tab_width(s, 8)


# ---------------------------------------------------------------------------
# Trackers
# ---------------------------------------------------------------------------


class TabWriter:
    def __init__(self, tab_width: int) -> None:
        self._tab_width = tab_width
        self._parts: list[str] = []

    def write_str(self, s: str) -> None:
        if self._tab_width <= 0 or "\t" not in s:
            self._parts.append(s)
            return
        self._parts.append(s.replace("\t", " " * self._tab_width))

    def value(self) -> str:
        return "".join(self._parts)


class ProgressTracker:
    def clone(self) -> "ProgressTracker":
        raise NotImplementedError

    def tick(self, state: Any, now: float) -> None:
        pass

    def reset(self, state: Any, now: float) -> None:
        pass

    def write(self, state: Any, writer: TabWriter) -> None:
        raise NotImplementedError


class FuncTracker(ProgressTracker):
    def __init__(self, func: Callable[[Any, TabWriter], None]) -> None:
        self._func = func

    def clone(self) -> "ProgressTracker":
        return FuncTracker(self._func)

    def write(self, state: Any, writer: TabWriter) -> None:
        self._func(state, writer)


# ---------------------------------------------------------------------------
# ProgressStyle
# ---------------------------------------------------------------------------

_DEFAULT_TICKS = "⠁⠁⠉⠙⠚⠒⠂⠂⠒⠲⠴⠤⠄⠄⠤⠠⠠⠤⠦⠖⠒⠐⠐⠒⠓⠋⠉⠈⠈ "


class ProgressStyle:
    def __init__(self, template: Template | str) -> None:
        if isinstance(template, str):
            template = template_from_str(template)
        self.tick_strings: list[str] = list(_DEFAULT_TICKS)
        self.progress_chars: list[str] = ["█", "░"]
        self.template = template
        self.tab_width = 8
        self.format_map: dict[str, ProgressTracker] = {}
        width = measure_text_width(self.progress_chars[0])
        self.char_width = width if width > 0 else 1

    def clone(self) -> "ProgressStyle":
        """Value-copy semantics, matching Go's ``ProgressStyle`` copy.

        The template and tracker map are copied deeply enough that mutating
        the clone (``set_tab_width``, ``set_for_stderr``, ``with_key`` ...)
        cannot change the original.
        """
        new = object.__new__(ProgressStyle)
        new.tick_strings = list(self.tick_strings)
        new.progress_chars = list(self.progress_chars)
        new.template = self.template.clone()
        new.tab_width = self.tab_width
        new.char_width = self.char_width
        new.format_map = {
            key: tracker.clone() for key, tracker in self.format_map.items()
        }
        return new

    # -- configuration --------------------------------------------------------

    def set_tab_width(self, width: int) -> None:
        self.tab_width = width
        self.template.set_tab_width(width)

    def set_for_stderr(self) -> None:
        for part in self.template.parts:
            if not part.is_placeholder:
                continue
            if part.style is not None:
                part.style.spec_err = part.style.spec
            if part.alt_style is not None:
                part.alt_style.spec_err = part.alt_style.spec

    def set_progress_chars(self, chars: str) -> None:
        if not chars:
            self.progress_chars = ["#", "-"]
            self.char_width = 1
            return
        self.progress_chars = list(chars)
        width = measure_text_width(self.progress_chars[0])
        self.char_width = width if width > 0 else 1

    def progress_chars_setting(self, chars: str) -> "ProgressStyle":
        self.set_progress_chars(chars)
        return self

    def with_progress_chars(self, chars: str) -> "ProgressStyle":
        new = self.clone()
        new.set_progress_chars(chars)
        return new

    def tick_chars(self, chars: str) -> "ProgressStyle":
        parts = list(chars)
        if len(parts) < 2:
            return self
        self.tick_strings = parts
        return self

    def with_tick_chars(self, chars: str) -> "ProgressStyle":
        new = self.clone()
        new.tick_chars(chars)
        return new

    def tick_strings_setting(self, strings: Iterable[str]) -> "ProgressStyle":
        values = list(strings)
        if values:
            self.tick_strings = values
        return self

    def with_tick_strings(self, strings: Iterable[str]) -> "ProgressStyle":
        new = self.clone()
        new.tick_strings_setting(strings)
        return new

    def with_template(self, template: str) -> "ProgressStyle":
        new = self.clone()
        new.template = template_from_str(template)
        return new

    def with_key(self, key: str, func: Callable[[Any], str]) -> "ProgressStyle":
        self.format_map[key] = FuncTracker(
            lambda state, writer: writer.write_str(func(state))
        )
        return self

    def with_key_style(
        self,
        key: str,
        func: Callable[[Any, bool], tuple[str, Style | None]],
    ) -> "ProgressStyle":
        def write(state: Any, writer: TabWriter) -> None:
            rendered, _ = func(state, False)
            writer.write_str(rendered)

        self.format_map[key] = FuncTracker(write)
        return self

    def with_tracker(self, key: str, tracker: ProgressTracker) -> "ProgressStyle":
        self.format_map[key] = tracker.clone()
        return self

    # -- formatting -----------------------------------------------------------

    def current_tick_str(self, state: Any) -> str:
        if state.is_finished():
            return self.tick_strings[-1] if self.tick_strings else ""
        if len(self.tick_strings) <= 1:
            return self.tick_strings[0] if self.tick_strings else ""
        return self.tick_strings[int(state.tick) % (len(self.tick_strings) - 1)]

    def format_state(
        self, state: Any, target_width: int, for_stderr: bool = False
    ) -> tuple[list[LineType], list[str]]:
        pos = state.pos()
        length = state.len()
        if length is None:
            length = pos

        lines: list[str] = [""]
        wide: TemplatePart | None = None

        for idx, part in enumerate(self.template.parts):
            if not part.is_placeholder:
                text = part.text
                if (
                    self.template.original_texts is not None
                    and idx < len(self.template.original_texts)
                    and self.template.original_texts[idx] != ""
                ):
                    text = self.template.original_texts[idx].replace(
                        "\t", " " * self.template.current_tab_width
                    )
                segments = text.split("\n")
                for i, segment in enumerate(segments):
                    lines[-1] += segment
                    if i != len(segments) - 1:
                        lines.append("")
                continue

            key = part.key
            style = part.style
            if key in ("bar", "wide_bar"):
                style = None

            def emit(buf: str, part: TemplatePart = part, style: Style | None = style) -> str:
                if part.width > 0:
                    buf = pad_and_truncate(buf, part.width, part.align, part.truncate)
                if style is not None:
                    buf = style.apply_for(buf, for_stderr)
                return buf

            tracker = self.format_map.get(key)
            if tracker is not None:
                writer = TabWriter(self.tab_width)
                tracker.write(state, writer)
                lines[-1] += emit(writer.value())
                continue

            buf = ""
            if key == "wide_bar":
                buf = "\x00"
                wide = part
            elif key == "wide_msg":
                buf = "\x00"
                wide = part
            elif key == "bar":
                width = part.width if part.width > 0 else 20
                buf = self.format_bar(
                    state.fraction(), width, part.style, part.alt_style, for_stderr
                )
            elif key == "spinner":
                buf = self.current_tick_str(state)
            elif key == "msg":
                buf = state.message.expanded()
            elif key == "prefix":
                buf = state.prefix.expanded()
            elif key == "pos":
                buf = str(pos)
            elif key == "len":
                buf = str(length)
            elif key == "percent":
                percent = f32(f32(state.fraction()) * f32(100.0))
                buf = f"{percent:.0f}"
            elif key == "percent_precise":
                percent = f32(f32(state.fraction()) * f32(100.0))
                buf = f"{percent:.3f}"
            elif key == "human_pos":
                buf = human_count(pos)
            elif key == "human_len":
                buf = human_count(length)
            elif key == "bytes":
                buf = human_bytes(pos)
            elif key == "total_bytes":
                buf = human_bytes(length)
            elif key == "decimal_bytes":
                buf = decimal_bytes(pos)
            elif key == "decimal_total_bytes":
                buf = decimal_bytes(length)
            elif key == "binary_bytes":
                buf = binary_bytes(pos)
            elif key == "binary_total_bytes":
                buf = binary_bytes(length)
            elif key == "elapsed_precise":
                buf = formatted_duration(state.elapsed())
            elif key == "elapsed":
                buf = human_duration(state.elapsed(), alt=True)
            elif key == "duration_precise":
                buf = formatted_duration(state.duration())
            elif key == "duration":
                buf = human_duration(state.duration(), alt=True)
            elif key == "eta_precise":
                buf = formatted_duration(state.eta())
            elif key == "eta":
                buf = human_duration(state.eta(), alt=True)
            elif key == "per_sec":
                per_sec = state.per_sec()
                if part.width > 0:
                    buf = human_float_count(per_sec, part.width) + "/s"
                else:
                    buf = human_float_count(per_sec) + "/s"
            elif key == "bytes_per_sec":
                buf = human_bytes(_u64_from_float(state.per_sec())) + "/s"
            elif key == "decimal_bytes_per_sec":
                buf = decimal_bytes(_u64_from_float(state.per_sec())) + "/s"
            elif key == "binary_bytes_per_sec":
                buf = binary_bytes(_u64_from_float(state.per_sec())) + "/s"

            lines[-1] += emit(buf)

        results: list[str] = []

        def push_line(expanded: str) -> None:
            results.extend(expanded.split("\n"))

        for line in lines:
            if "\x00" not in line or wide is None:
                push_line(line)
                continue

            src = line
            if "\n" in line:
                for sub in line.split("\n"):
                    if "\x00" in sub:
                        src = sub
                        break
            left = target_width - measure_text_width(src.replace("\x00", ""))
            if left < 0:
                left = 0

            if wide.key == "wide_msg":
                rendered = pad_and_truncate(
                    state.message.expanded(), left, wide.align, True
                )
                if line.endswith("\x00"):
                    rendered = rendered.rstrip(" ")
                push_line(line.replace("\x00", rendered))
            else:
                bar = self.format_bar(
                    state.fraction(), left, wide.style, wide.alt_style, for_stderr
                )
                push_line(line.replace("\x00", bar))

        return [LineType.BAR] * len(results), results

    def format_bar(
        self,
        fract: float,
        width: int,
        style: Style | None,
        alt_style: Style | None,
        for_stderr: bool = False,
    ) -> str:
        char_width_value = self.char_width if self.char_width > 0 else 1
        clusters = width // char_width_value

        fillf = f32(f32(float(clusters)) * f32(fract))
        if fillf < 0:
            fillf = 0.0
        limit = f32(float(clusters))
        if fillf > limit:
            fillf = limit
        entirely_filled = int(fillf)

        cur_idx: int | None = None
        if fillf > 0 and entirely_filled < clusters:
            n = len(self.progress_chars) - 2
            if n <= 0:
                cur_idx = None
            elif n == 1:
                cur_idx = 1
            else:
                fraction = f32(fillf - f32(float(entirely_filled)))
                value = n - int(f32(fraction * f32(float(n))))
                cur_idx = max(0, min(value, n))

        filled_parts = [self.progress_chars[0] * entirely_filled]
        if cur_idx is not None:
            idx = cur_idx
            if idx >= len(self.progress_chars) - 1:
                idx = len(self.progress_chars) - 2
            if idx < 0:
                idx = 0
            filled_parts.append(self.progress_chars[idx])

        background = clusters - entirely_filled
        if cur_idx is not None:
            background = max(0, background - 1)
        rest_parts = [self.progress_chars[-1] * background]

        filled = style.apply_for("".join(filled_parts), for_stderr) if style else "".join(filled_parts)
        rest_style = alt_style if alt_style is not None else style
        rest = (
            rest_style.apply_for("".join(rest_parts), for_stderr)
            if rest_style is not None
            else "".join(rest_parts)
        )
        return filled + rest


# ---------------------------------------------------------------------------
# Defaults and constructors
# ---------------------------------------------------------------------------


def default_progress_style_bar() -> ProgressStyle:
    return ProgressStyle(template_from_str("{wide_bar} {pos}/{len}"))


def default_progress_style_spinner() -> ProgressStyle:
    return ProgressStyle(template_from_str("{spinner} {msg}"))


def progress_style_with_template(template: str) -> ProgressStyle:
    return ProgressStyle(template_from_str(template))
