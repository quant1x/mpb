# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Yarn-style four-phase output (port of ``examples/yarnish.rs``)."""

import random
import threading
import time

from mpb import MultiProgress, ProgressBar, human_duration, progress_style_with_template

PACKAGES = [
    "fs-events",
    "my-awesome-module",
    "emoji-speaker",
    "wrap-ansi",
    "stream-browserify",
    "acorn-dynamic-import",
]

COMMANDS = [
    "cmake .",
    "make",
    "make clean",
    "gcc foo.c -o foo",
    "gcc bar.c -o bar",
    "./helper.sh rebuild-cache",
    "make all-clean",
    "make test",
]

LOOKING_GLASS = "🔍  "
TRUCK = "🚚  "
CLIP = "🔗  "
PAPER = "📃  "
SPARKLE = "✨ "


def bold_dim(text: str) -> str:
    return "\x1b[1;2m" + text + "\x1b[0m"


def frames(text: str) -> list[str]:
    return list(text)


def main() -> None:
    started = time.monotonic()

    spinner_style = progress_style_with_template("{prefix:.bold.dim} {spinner} {wide_msg}")
    spinner_style.tick_strings_setting(frames("⠁⠂⠄⡀⢀⠠⠐⠈ "))

    print(f"{bold_dim('[1/4]')} {LOOKING_GLASS}Resolving packages...")
    print(f"{bold_dim('[2/4]')} {TRUCK}Fetching packages...")
    print(f"{bold_dim('[3/4]')} {CLIP}Linking dependencies...")

    deps = 1232
    pb = ProgressBar(deps)
    for _ in range(deps):
        time.sleep(0.003)
        pb.inc(1)
    pb.finish_and_clear()

    print(f"{bold_dim('[4/4]')} {PAPER}Building fresh packages...")

    mp = MultiProgress()
    threads: list[threading.Thread] = []
    for i in range(4):
        count = 30 + random.randrange(50)
        bar = mp.add(ProgressBar(count))
        bar.set_style(spinner_style)
        bar.set_prefix(f"[{i + 1}/?]")

        thread = threading.Thread(
            target=_build_package,
            args=(bar, count),
            daemon=True,
        )
        thread.start()
        threads.append(thread)

    for thread in threads:
        thread.join()
    mp.clear()

    print(f"{SPARKLE} Done in {human_duration(time.monotonic() - started)}")


def _build_package(bar: ProgressBar, count: int) -> None:
    package = random.choice(PACKAGES)
    for _ in range(count):
        command = random.choice(COMMANDS)
        time.sleep(0.025 + random.random() * 0.175)
        bar.set_message(f"{package}: {command}")
        bar.inc(1)
    bar.finish_with_message("waiting...")


if __name__ == "__main__":
    main()
