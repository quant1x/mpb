# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Feed ``cargo build --release`` stderr into a spinner (``cargowrap.rs``).

Requires cargo and must be run inside a Rust project.
"""

import shutil
import subprocess
import time

from mpb import ProgressBar, human_duration, progress_style_with_template


def frames(text: str) -> list[str]:
    return list(text)


def main() -> None:
    started = time.monotonic()
    print("Compiling package in release mode...")

    if shutil.which("cargo") is None:
        raise SystemExit("cargo not found in PATH")

    pb = ProgressBar.spinner()
    pb.enable_steady_tick(0.2)

    style = progress_style_with_template("{spinner:.dim.bold} cargo: {wide_msg}")
    style.tick_strings_setting(frames("/|\\- "))
    pb.set_style(style)

    process = subprocess.Popen(
        ["cargo", "build", "--release"],
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    assert process.stderr is not None
    for raw_line in process.stderr:
        line = raw_line.strip()
        if line:
            pb.set_message(line)
        pb.force_draw()

    process.wait()
    pb.finish_and_clear()
    print(f"Done in {human_duration(time.monotonic() - started)}")


if __name__ == "__main__":
    main()
