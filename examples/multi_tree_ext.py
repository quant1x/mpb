# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Extended tree demo (port of ``examples/multi-tree-ext.rs``).

Adds and removes bars from a ``MultiProgress`` tree and supports
``--bottom-alignment``.
"""

import argparse
import random
import time
from dataclasses import dataclass

from mpb import (
    MultiProgress,
    MultiProgressAlignment,
    ProgressBar,
    progress_style_with_template,
)

PB_LEN = 32


@dataclass
class Item:
    key: str
    index: int
    indent: int
    bar: ProgressBar


def add(indent: int, index: int, key: str) -> tuple[str, Item]:
    return "add", Item(key=key, index=index, indent=indent, bar=ProgressBar(PB_LEN))


def remove(index: int) -> tuple[str, int]:
    return "remove", index


ELEMENTS: list[tuple[str, Item | int]] = [
    add(9, 0, "dog"),
    add(0, 0, "temp_1"),
    add(8, 1, "lazy"),
    add(0, 1, "temp_2"),
    add(1, 0, "the"),
    add(0, 0, "temp_3"),
    add(7, 3, "a"),
    add(0, 3, "temp_4"),
    add(6, 2, "over"),
    remove(6),
    remove(4),
    remove(3),
    remove(0),
    add(0, 2, "temp_5"),
    add(4, 1, "fox"),
    add(0, 1, "temp_6"),
    add(2, 1, "quick"),
    add(0, 1, "temp_7"),
    add(5, 5, "jumps"),
    add(0, 5, "temp_8"),
    add(3, 4, "brown"),
    add(0, 3, "temp_9"),
    remove(10),
    remove(7),
    remove(4),
    remove(3),
    remove(1),
]


def green(text: str) -> str:
    return "\x1b[32m" + text + "\x1b[0m"


def get_action(
    items: list[Item], elem_index: int
) -> tuple[tuple[str, int], int]:
    uncompleted = [
        i
        for i, item in enumerate(items)
        if item.bar.position() < (item.bar.length() or 0)
    ]

    k = random.randrange(16)
    if (k > 0 or (k == 0 and elem_index == len(ELEMENTS))) and uncompleted:
        return ("inc", random.choice(uncompleted)), elem_index
    if elem_index < len(ELEMENTS):
        return ("modify", elem_index), elem_index + 1
    return ("stop", 0), elem_index


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--bottom-alignment",
        action="store_true",
        help="align the MultiProgress to the bottom",
    )
    args = parser.parse_args()

    mp = MultiProgress()
    mp.set_alignment(
        MultiProgressAlignment.BOTTOM
        if args.bottom_alignment
        else MultiProgressAlignment.TOP
    )

    style_main = progress_style_with_template("{bar:40.green/yellow} {pos:>4}/{len:4}")
    style_aux = progress_style_with_template(
        "[{pos:>2}/{len:2}] {prefix}{spinner:.green} {msg}"
    )
    style_fin = progress_style_with_template("[{pos:>2}/{len:2}] {prefix}{msg}")

    total = 0
    for kind, payload in ELEMENTS:
        if kind == "add":
            assert isinstance(payload, Item)
            total += payload.bar.length() or 0
        else:
            total += 1

    pb_main = mp.add(ProgressBar(total))
    pb_main.set_style(style_main)
    for kind, payload in ELEMENTS:
        if kind == "add":
            assert isinstance(payload, Item)
            payload.bar.set_style(style_aux)

    items: list[Item] = []
    elem_index = 0
    pb_main.force_draw()

    while True:
        (action, index), elem_index = get_action(items, elem_index)

        if action == "stop":
            pb_main.finish()
            return

        if action == "modify":
            kind, payload = ELEMENTS[index]
            if kind == "add":
                item = payload
                assert isinstance(item, Item)
                bar = mp.insert(item.index, item.bar)
                bar.set_prefix("  " * item.indent)
                bar.set_message(item.key)
                items.insert(item.index, item)
            else:
                remove_index = payload
                assert isinstance(remove_index, int)
                item = items.pop(remove_index)
                mp.remove(item.bar)
                pb_main.inc((item.bar.length() or 0) - item.bar.position())
        else:
            item = items[index]
            item.bar.inc(1)
            if item.bar.position() >= (item.bar.length() or 0):
                item.bar.set_style(style_fin)
                item.bar.finish_with_message(green("✔") + " " + item.key)
            pb_main.inc(1)

        time.sleep(0.02)


if __name__ == "__main__":
    main()
