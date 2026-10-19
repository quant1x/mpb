# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Download with throughput (port of ``examples/download-speed.rs``)."""

import time

from mpb import ProgressBar, progress_style_with_template


def main() -> None:
    total_size = 231231231
    downloaded = 0

    pb = ProgressBar(total_size)
    style = progress_style_with_template(
        "{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] "
        "{bytes}/{total_bytes} ({bytes_per_sec}, {eta})"
    )
    style.set_progress_chars("#>-")
    pb.set_style(style)

    while downloaded < total_size:
        downloaded = min(downloaded + 223211, total_size)
        pb.set_position(downloaded)
        time.sleep(0.012)

    pb.finish_with_message("downloaded")


if __name__ == "__main__":
    main()
