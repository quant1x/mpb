# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Resume a download from a non-zero position (``download-continued.rs``).

Also calls ``reset_eta()`` so the estimate restarts from the current position.
"""

import time

from mpb import ProgressBar, progress_style_with_template


def main() -> None:
    downloaded = 69369369
    total_size = 231231231

    pb = ProgressBar(total_size)
    style = progress_style_with_template(
        "{spinner:.green} [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})"
    )
    style.set_progress_chars("#>-")
    pb.set_style(style)
    pb.set_position(downloaded)
    pb.reset_eta()

    while downloaded < total_size:
        downloaded = min(downloaded + 123211, total_size)
        pb.set_position(downloaded)
        time.sleep(0.012)

    pb.finish_with_message("downloaded")


if __name__ == "__main__":
    main()
