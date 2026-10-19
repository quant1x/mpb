# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""One overall bar tracking many child bars (``examples/morebars.rs``)."""

import time

from mpb import MultiProgress, ProgressBar, progress_style_with_template


def main() -> None:
    mp = MultiProgress()
    style = progress_style_with_template("{bar:40.green/yellow} {pos:>7}/{len:7}")

    overall = mp.add(ProgressBar(5))
    overall.set_style(style)

    # Make sure we show up at all; otherwise no rendering event happens.
    overall.force_draw()

    for _ in range(5):
        child = mp.add(ProgressBar(128))
        child.set_style(style)
        for _ in range(128):
            time.sleep(0.005)
            child.inc(1)
        child.finish()
        overall.inc(1)

    overall.finish_with_message("done")


if __name__ == "__main__":
    main()
