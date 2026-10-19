# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""mpb - a Python port of the indicatif-style progress bar library.

The package mirrors the Go package ``github.com/quant1x/mpb`` module by
module, which in turn is a semantic port of Rust's ``indicatif`` 0.18.6.

Quick start::

    from mpb import ProgressBar, ProgressStyle

    pb = ProgressBar(100)
    pb.set_style(ProgressStyle("{wide_bar} {pos}/{len}"))
    for _ in range(100):
        pb.inc(1)
    pb.finish()

Multiple bars::

    from mpb import MultiProgress, ProgressBar

    mp = MultiProgress()
    pb = mp.add(ProgressBar(100))
"""

from __future__ import annotations

from ._types import (
    LineAdjust,
    LineAdjustKind,
    LineType,
    MultiProgressAlignment,
)
from .draw_target import (
    HiddenDrawTarget,
    ProgressDrawTarget,
    RateLimiter,
    TermDrawTarget,
    TermLikeDrawTarget,
    hidden_draw_target,
    in_memory_draw_target,
    is_dumb,
    stderr_draw_target,
    stderr_draw_target_with_hz,
    stdout_draw_target,
    stdout_draw_target_with_hz,
    term_draw_target,
    term_like_draw_target,
    term_like_draw_target_with_hz,
)
from .format import (
    binary_bytes,
    decimal_bytes,
    formatted_duration,
    human_bytes,
    human_count,
    human_duration,
    human_float_count,
)
from .in_memory import InMemoryTerm, Move
from .iter import (
    ProgressBarIter,
    ProgressReader,
    ProgressWriter,
    SeekMax,
    map_parallel,
    progress,
    progress_count,
    progress_with,
    progress_with_style,
    try_progress,
)
from .multi import (
    AggregatedDrawable,
    InsertLocation,
    MultiDrawTarget,
    MultiProgress,
    MultiState,
    MultiStateMember,
    RemoteDrawTarget,
)
from .progress_bar import (
    ProgressBar,
    WeakProgressBar,
    default_progress_finish_value,
    hidden,
    new,
    new_spinner,
    new_with_draw_target,
    no_length,
)
from .state import (
    AtomicPosition,
    BarState,
    DrawState,
    DrawStateWrapper,
    Estimator,
    FinishKind,
    ProgressFinish,
    ProgressState,
    RemoteStub,
    ResetMode,
    Status,
    StatusKind,
    TabExpandedString,
    TickerHandle,
    default_progress_finish,
    draw_to_term,
    split_lines,
    visual_line_count,
    wrapped_metrics,
)
from .style import (
    DEFAULT_WIDTH,
    FuncTracker,
    ProgressStyle,
    ProgressTracker,
    Style,
    TabWriter,
    Template,
    TemplateError,
    TemplatePart,
    Width,
    ansi_segments,
    ansi_slice,
    apply_ansi_style,
    default_progress_style_bar,
    default_progress_style_spinner,
    measure_text_width,
    pad_and_truncate,
    progress_style_with_template,
    template_from_str,
    template_from_str_with_tab_width,
)
from .term_like import StdTerm, TermLike, new_stderr_term, new_std_term, new_std_term_with_writer

try:
    from ._version import __version__
except ImportError:  # running from a source checkout before an editable install
    __version__ = "0.1.5"

__all__ = [
    "__version__",
    # progress bars
    "ProgressBar",
    "WeakProgressBar",
    "new",
    "no_length",
    "hidden",
    "new_with_draw_target",
    "new_spinner",
    "default_progress_finish",
    "default_progress_finish_value",
    # styles
    "ProgressStyle",
    "Style",
    "Template",
    "TemplatePart",
    "TemplateError",
    "ProgressTracker",
    "FuncTracker",
    "TabWriter",
    "default_progress_style_bar",
    "default_progress_style_spinner",
    "progress_style_with_template",
    "template_from_str",
    "template_from_str_with_tab_width",
    "Width",
    "DEFAULT_WIDTH",
    "measure_text_width",
    "ansi_segments",
    "ansi_slice",
    "apply_ansi_style",
    "pad_and_truncate",
    # state
    "AtomicPosition",
    "BarState",
    "ProgressState",
    "ProgressFinish",
    "FinishKind",
    "ResetMode",
    "Status",
    "StatusKind",
    "Estimator",
    "TabExpandedString",
    "DrawState",
    "DrawStateWrapper",
    "RemoteStub",
    "TickerHandle",
    "draw_to_term",
    "split_lines",
    "visual_line_count",
    "wrapped_metrics",
    # multi
    "MultiProgress",
    "MultiState",
    "MultiStateMember",
    "MultiProgressAlignment",
    "InsertLocation",
    "MultiDrawTarget",
    "RemoteDrawTarget",
    "AggregatedDrawable",
    # draw targets
    "ProgressDrawTarget",
    "TermDrawTarget",
    "TermLikeDrawTarget",
    "HiddenDrawTarget",
    "RateLimiter",
    "term_draw_target",
    "term_like_draw_target",
    "term_like_draw_target_with_hz",
    "stdout_draw_target",
    "stderr_draw_target",
    "stdout_draw_target_with_hz",
    "stderr_draw_target_with_hz",
    "hidden_draw_target",
    "in_memory_draw_target",
    "is_dumb",
    # terminals
    "TermLike",
    "StdTerm",
    "new_std_term",
    "new_stderr_term",
    "new_std_term_with_writer",
    "InMemoryTerm",
    "Move",
    # iterators
    "ProgressBarIter",
    "ProgressReader",
    "ProgressWriter",
    "SeekMax",
    "map_parallel",
    "progress",
    "progress_count",
    "progress_with",
    "progress_with_style",
    "try_progress",
    # formatting
    "formatted_duration",
    "human_duration",
    "human_bytes",
    "binary_bytes",
    "decimal_bytes",
    "human_count",
    "human_float_count",
    # shared types
    "LineType",
    "LineAdjust",
    "LineAdjustKind",
]
