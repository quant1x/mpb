# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Steady tick with custom spinner frames (port of ``examples/long-spinner.rs``)."""

import time

from mpb import ProgressBar, progress_style_with_template


def main() -> None:
    pb = ProgressBar.spinner()
    pb.enable_steady_tick(0.120)

    style = progress_style_with_template("{spinner:.blue} {msg}")
    style.tick_strings_setting(
        [
            "▹▹▹▹▹",
            "▸▹▹▹▹",
            "▹▸▹▹▹",
            "▹▹▸▹▹",
            "▹▹▹▸▹",
            "▹▹▹▹▸",
            "▪▪▪▪▪",
        ]
    )
    pb.set_style(style)
    pb.set_message("Calculating...")
    time.sleep(5.0)
    pb.finish_with_message("Done")


if __name__ == "__main__":
    main()
