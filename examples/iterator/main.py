# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Three iterator wrappers (port of ``examples/iterator.rs``)."""

import time

from mpb import ProgressBar, progress_style_with_template, progress_with
from mpb.iter import progress, progress_count


def main() -> None:
    items = list(range(1000))

    for _ in progress(items):
        time.sleep(0.005)

    for _ in progress_count(items, 1000):
        time.sleep(0.005)

    pb = ProgressBar(1000)
    style = progress_style_with_template(
        "{spinner:.green} [{elapsed_precise}] [{bar:40.cyan/blue}] "
        "({pos}/{len}, ETA {eta})"
    )
    pb.set_style(style)
    for _ in progress_with(items, pb):
        time.sleep(0.005)

    print("done")


if __name__ == "__main__":
    main()
