# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Ticker-driven progress bar (port of ``examples/tokio.rs``).

Rust drives a tokio interval with ``block_on``; Python uses ``time.sleep``
for the same 5 ms / 1024-step stream.
"""

import time

from mpb import ProgressBar


def main() -> None:
    steps = 1024
    pb = ProgressBar(steps)

    next_tick = time.monotonic()
    for _ in range(steps):
        next_tick += 0.005
        delay = next_tick - time.monotonic()
        if delay > 0:
            time.sleep(delay)
        pb.inc(1)

    pb.finish()


if __name__ == "__main__":
    main()
