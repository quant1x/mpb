# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Mimic cargo's build output (port of ``examples/cargo.rs``).

Four worker threads pull crates from a shared list while the main thread
renders the progress bar and the ``Compiling`` lines.
"""

import queue
import random
import sys
import threading
import time

from mpb import ProgressBar, human_duration, progress_style_with_template

CRATES = [
    ("console", "v0.14.1"),
    ("lazy_static", "v1.4.0"),
    ("libc", "v0.2.93"),
    ("regex", "v1.4.6"),
    ("regex-syntax", "v0.6.23"),
    ("terminal_size", "v0.1.16"),
    ("libc", "v0.2.93"),
    ("unicode-width", "v0.1.8"),
    ("lazy_static", "v1.4.0"),
    ("number_prefix", "v0.4.0"),
    ("regex", "v1.4.6"),
    ("rand", "v0.8.3"),
    ("getrandom", "v0.2.2"),
    ("cfg-if", "v1.0.0"),
    ("libc", "v0.2.93"),
    ("rand_chacha", "v0.3.0"),
    ("ppv-lite86", "v0.2.10"),
    ("rand_core", "v0.6.2"),
    ("getrandom", "v0.2.2"),
    ("rand_core", "v0.6.2"),
    ("tokio", "v1.5.0"),
    ("bytes", "v1.0.1"),
    ("pin-project-lite", "v0.2.6"),
    ("slab", "v0.4.3"),
    ("indicatif", "v0.15.0"),
]

NUM_CPUS = 4


def green_bold(text: str) -> str:
    return "\x1b[1;32m" + text + "\x1b[0m"


def term_width() -> int:
    try:
        width = sys.stdout.get_terminal_size().columns
    except (AttributeError, OSError, ValueError):
        return 80
    return width if width > 0 else 80


def main() -> None:
    started = time.monotonic()

    template = "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len}"
    if term_width() > 80:
        template += " {wide_msg}"

    pb = ProgressBar(len(CRATES))
    style = progress_style_with_template(template)
    style.set_progress_chars("=> ")
    pb.set_style(style)
    pb.set_prefix("Building")

    lock = threading.Lock()
    next_index = 0

    def next_crate():
        nonlocal next_index
        with lock:
            if next_index >= len(CRATES):
                return None
            crate = CRATES[next_index]
            next_index += 1
            return crate

    messages: "queue.Queue[tuple[int, tuple[str, str] | None]]" = queue.Queue(
        maxsize=len(CRATES) + NUM_CPUS
    )

    def worker(number: int) -> None:
        while True:
            crate = next_crate()
            messages.put((number, crate))
            if crate is None:
                return
            if crate == CRATES[-1]:
                time.sleep(random.uniform(1.0, 2.0))
            else:
                time.sleep(random.uniform(0.25, 1.0))

    for number in range(NUM_CPUS):
        threading.Thread(target=worker, args=(number,), daemon=True).start()

    processing: list[tuple[str, str] | None] = [None] * NUM_CPUS
    finished = 0
    while finished < NUM_CPUS:
        worker_id, crate = messages.get()
        if crate is None:
            processing[worker_id] = None
            finished += 1
            continue

        processing[worker_id] = crate
        names = [item[0] for item in processing if item is not None]
        pb.set_message(", ".join(names))

        compiling = green_bold(f"{'Compiling':>12}")
        pb.println(f"{compiling} {crate[0]} {crate[1]}")
        pb.inc(1)

    pb.finish_and_clear()

    finished_label = green_bold(f"{'Finished':>12}")
    print(
        f"{finished_label} dev [unoptimized + debuginfo] target(s) in "
        f"{human_duration(time.monotonic() - started)}"
    )


if __name__ == "__main__":
    main()
