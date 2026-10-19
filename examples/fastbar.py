# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""A huge number of tiny updates (port of ``examples/fastbar.rs``).

Shows how the default 20 Hz rate limiter behaves in a tight loop and how much
wall time ``elapsed`` reports.
"""

from mpb import ProgressBar


def many_units_of_easy_work(count: int, label: str) -> None:
    pb = ProgressBar(count)

    total = 0
    for i in range(count):
        total += 2 * i + 3
        pb.inc(1)
    pb.finish()

    print(f"[{label}] Sum ({total}) calculated in {pb.elapsed():.6f}s")


def main() -> None:
    many_units_of_easy_work(1 << 20, "Default progress bar ")


if __name__ == "__main__":
    main()
