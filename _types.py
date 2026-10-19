# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Shared enums / small value types used across modules."""

from __future__ import annotations

import struct
from dataclasses import dataclass
from enum import Enum, IntEnum


def f32(value: float) -> float:
    """Round a Python float to IEEE-754 single precision.

    Rust/Go compute the fraction and bar fill with ``f32``; using the same
    precision keeps block-character boundaries identical.
    """
    return struct.unpack("<f", struct.pack("<f", value))[0]


class LineType(IntEnum):
    TEXT = 0
    BAR = 1
    EMPTY = 2


class LineAdjustKind(Enum):
    CLEAR = "clear"
    KEEP = "keep"


@dataclass
class LineAdjust:
    kind: LineAdjustKind
    count: int = 0


class MultiProgressAlignment(Enum):
    TOP = "top"
    BOTTOM = "bottom"


def saturating_add(a: int, b: int) -> int:
    return a + b


def saturating_sub(a: int, b: int) -> int:
    return max(0, a - b)
