# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Human-readable formatting helpers (port of ``format.rs``).

All durations are expressed in **seconds** (``float``) throughout this
package; the Go port uses ``time.Duration`` and the Rust original uses
``std::time::Duration``.  Seconds keep the arithmetic simple and are what
the estimators produce.
"""

from __future__ import annotations

import math

SECOND = 1.0
MINUTE = 60.0
HOUR = 3600.0
DAY = 24.0 * HOUR
WEEK = 7.0 * DAY
YEAR = 365.0 * DAY

_UNITS = (
    (YEAR, "year", "y"),
    (WEEK, "week", "w"),
    (DAY, "day", "d"),
    (HOUR, "hour", "h"),
    (MINUTE, "minute", "m"),
    (SECOND, "second", "s"),
)


def _round_half_away(value: float) -> int:
    """``math.Round`` semantics: halves go away from zero."""
    if value >= 0:
        return int(math.floor(value + 0.5))
    return int(math.ceil(value - 0.5))


def formatted_duration(seconds: float) -> str:
    """``FormattedDuration``: ``HH:MM:SS`` (with a ``Nd `` prefix for days)."""
    if not math.isfinite(seconds):
        seconds = 0.0
    total = int(seconds)  # truncate towards zero, like Go's Duration division
    secs = total % 60
    total //= 60
    minutes = total % 60
    total //= 60
    hours = total % 24
    total //= 24
    if total > 0:
        return f"{total}d {hours:02d}:{minutes:02d}:{secs:02d}"
    return f"{hours:02d}:{minutes:02d}:{secs:02d}"


def human_duration(seconds: float, alt: bool = False) -> str:
    """``HumanDuration``.

    ``alt=True`` selects the short form (``5s`` instead of ``5 seconds``),
    matching Rust's ``{:#}`` formatting.
    """
    if not math.isfinite(seconds):
        seconds = 0.0

    idx = 0
    for i in range(len(_UNITS)):
        idx = i
        if i + 1 < len(_UNITS):
            cur = _UNITS[i][0]
            nxt = _UNITS[i + 1][0]
            if seconds + nxt / 2 >= cur + cur / 2:
                break

    unit, name, short = _UNITS[idx]
    value = _round_half_away(seconds / unit)
    if idx < len(_UNITS) - 1 and value < 2:
        value = 2

    if alt:
        return f"{value}{short}"
    if value == 1:
        return f"{value} {name}"
    return f"{value} {name}s"


def _binary_prefix(value: int) -> str:
    if value < 1024:
        return f"{value} B"
    units = ("KiB", "MiB", "GiB", "TiB", "PiB")
    number = float(value)
    for i, unit in enumerate(units):
        divisor = 1024.0 ** (i + 1)
        if number < divisor * 1024 or i == len(units) - 1:
            return f"{number / divisor:.2f} {unit}"
    return f"{value} B"


def _decimal_prefix(value: int) -> str:
    if value < 1000:
        return f"{value} B"
    units = ("kB", "MB", "GB", "TB", "PB")
    number = float(value)
    for i, unit in enumerate(units):
        divisor = 1000.0 ** (i + 1)
        if number < divisor * 1000 or i == len(units) - 1:
            return f"{number / divisor:.2f} {unit}"
    return f"{value} B"


def human_bytes(value: int) -> str:
    """``HumanBytes`` (binary prefixes)."""
    return _binary_prefix(int(value))


def binary_bytes(value: int) -> str:
    """``BinaryBytes`` (binary prefixes)."""
    return _binary_prefix(int(value))


def decimal_bytes(value: int) -> str:
    """``DecimalBytes`` (SI prefixes)."""
    return _decimal_prefix(int(value))


def _group_thousands(digits: str) -> str:
    out: list[str] = []
    length = len(digits)
    for i, ch in enumerate(digits):
        pos = length - i - 1
        out.append(ch)
        if pos > 0 and pos % 3 == 0:
            out.append(",")
    return "".join(out)


def human_count(value: int) -> str:
    """``HumanCount``: group integer digits with commas."""
    return _group_thousands(str(int(value)))


def human_float_count(value: float, precision: int = 4) -> str:
    """``HumanFloatCount``: fixed precision, thousands separators, no trailing zeros."""
    if not math.isfinite(value):
        return str(value)
    text = f"{value:.{precision}f}"
    int_part, _, frac_part = text.partition(".")
    frac_part = frac_part.rstrip("0")

    sign = ""
    digits = int_part
    if digits.startswith("-"):
        sign = "-"
        digits = digits[1:]
    # Negative values may round to zero at the requested precision; drop the
    # sign so we never print a bare "-0".
    if frac_part == "" and digits.strip("0") == "":
        sign = ""

    grouped = sign + _group_thousands(digits)
    if frac_part:
        grouped += "." + frac_part
    return grouped
