# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Progress updates interleaved with log lines (port of ``examples/log.rs``)."""

import time

from mpb import ProgressBar


def main() -> None:
    pb = ProgressBar(100)
    for i in range(100):
        time.sleep(0.025)
        pb.println(f"[+] finished #{i}")
        pb.inc(1)
    pb.finish_with_message("done")


if __name__ == "__main__":
    main()
