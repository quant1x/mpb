# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""The simplest single progress bar (port of ``examples/single.rs``)."""

import time

from mpb import ProgressBar


def main() -> None:
    pb = ProgressBar(1024)
    for _ in range(1024):
        time.sleep(0.005)
        pb.inc(1)
    pb.finish_with_message("done")


if __name__ == "__main__":
    main()
