# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""MultiProgress with concurrent updates (port of ``examples/multi.rs``).

With ``-capture`` the frames are rendered into an in-memory terminal and the
plain-text result is written to ``examples/multi_capture.out``.
"""

import argparse
import random
import threading
import time
from pathlib import Path

from mpb import (
    InMemoryTerm,
    MultiProgress,
    ProgressBar,
    in_memory_draw_target,
    progress_style_with_template,
)

ROOT = Path(__file__).resolve().parents[1]
CAPTURE_PATH = ROOT / "examples" / "multi_capture.out"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-capture",
        "--capture",
        action="store_true",
        help="write plain-text capture to examples/multi_capture.out",
    )
    args = parser.parse_args()

    term: InMemoryTerm | None = None
    if args.capture:
        term = InMemoryTerm(2000, 200)
        mp = MultiProgress(in_memory_draw_target(term))
    else:
        mp = MultiProgress()

    style = progress_style_with_template(
        "[{elapsed_precise}] {bar:40.cyan/blue} {pos:>7}/{len:7} {msg}"
    )
    style.set_progress_chars("##-")

    count = 200
    todo = mp.add(ProgressBar(count))
    todo.set_style(style)
    todo.set_message("todo")

    finished = mp.add(ProgressBar(count))
    finished.set_style(style)
    finished.set_message("finished")

    fast = mp.insert_after(finished, ProgressBar(1024))
    fast.set_style(style)

    mp.println("starting!")

    fast_thread = threading.Thread(
        target=_run_fast_bar,
        args=(mp, fast),
        daemon=True,
    )
    fast_thread.start()

    inner_threads: list[threading.Thread] = []
    for i in range(count):
        time.sleep(0.015)
        if i == count // 3:
            time.sleep(2.0)
        todo.inc(1)

        thread = threading.Thread(
            target=_run_slow_bar,
            args=(finished,),
            daemon=True,
        )
        thread.start()
        inner_threads.append(thread)

    todo.finish_with_message("all jobs started")
    for thread in inner_threads:
        thread.join()
    fast_thread.join()
    finished.finish_with_message("all jobs done")

    if args.capture:
        assert term is not None
        CAPTURE_PATH.write_text(term.contents(), encoding="utf-8")
    else:
        mp.clear()


def _run_fast_bar(mp: MultiProgress, bar: ProgressBar) -> None:
    for _ in range(1024):
        time.sleep(0.002)
        bar.inc(1)
    mp.println("pb3 is done!")
    bar.finish_with_message("done")


def _run_slow_bar(bar: ProgressBar) -> None:
    time.sleep(random.uniform(1.0, 5.0))
    bar.inc(1)


if __name__ == "__main__":
    main()
