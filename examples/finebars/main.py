# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Five independently advancing bars (port of ``examples/finebars.rs``)."""

import random
import threading
import time

from mpb import MultiProgress, ProgressBar, progress_style_with_template


def main() -> None:
    styles = [
        ("Rough bar:", "█  ", "red"),
        ("Fine bar: ", "█▉▊▋▌▍▎▏  ", "yellow"),
        ("Vertical: ", "█▇▆▅▄▃▂▁  ", "green"),
        ("Fade in:  ", "█▓▒░  ", "blue"),
        ("Blocky:   ", "█▛▌▖  ", "magenta"),
    ]

    mp = MultiProgress()
    threads: list[threading.Thread] = []

    for prefix, chars, color in styles:
        pb = mp.add(ProgressBar(512))
        style = progress_style_with_template(f"{{prefix:.bold}}▕{{bar:.{color}}}▏{{msg}}")
        style.set_progress_chars(chars)
        pb.set_style(style)
        pb.set_prefix(prefix)

        wait = random.uniform(0.01, 0.03)

        def work(bar: ProgressBar = pb, delay: float = wait) -> None:
            for i in range(512):
                time.sleep(delay)
                bar.inc(1)
                bar.set_message(f"{100 * i / 512:3.0f}%")
            bar.finish_with_message("100%")

        thread = threading.Thread(target=work)
        thread.start()
        threads.append(thread)

    for thread in threads:
        thread.join()


if __name__ == "__main__":
    main()
