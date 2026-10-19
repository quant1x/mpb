# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Tree-shaped MultiProgress (port of ``examples/multi-tree.rs``)."""

import random
import time
from dataclasses import dataclass

from mpb import MultiProgress, ProgressBar, progress_style_with_template


@dataclass
class Elem:
    key: str
    index: int
    indent: int
    bar: ProgressBar


ELEMENTS = [
    Elem("jumps", 0, 1, ProgressBar(32)),
    Elem("lazy", 1, 2, ProgressBar(32)),
    Elem("the", 0, 0, ProgressBar(32)),
    Elem("dog", 3, 3, ProgressBar(32)),
    Elem("over", 2, 2, ProgressBar(32)),
    Elem("brown", 1, 2, ProgressBar(32)),
    Elem("quick", 1, 1, ProgressBar(32)),
    Elem("a", 5, 3, ProgressBar(32)),
    Elem("fox", 3, 3, ProgressBar(32)),
]


def get_action(tree: list[Elem]) -> tuple[bool, int] | None:
    """Return ``(add, idx)`` or ``None`` when the tree is complete."""
    total_free = sum((e.bar.length() or 0) - e.bar.position() for e in tree)
    if total_free == 0 and len(tree) == len(ELEMENTS):
        return None
    if total_free == 0 and len(tree) < len(ELEMENTS):
        return True, len(tree)

    while True:
        k = random.randrange(17)
        if k == 0 and len(tree) < len(ELEMENTS):
            return True, len(tree)
        if not tree:
            return True, 0
        index = k % len(tree)
        elem = tree[index]
        if elem.bar.position() < (elem.bar.length() or 0):
            return False, index


def main() -> None:
    mp = MultiProgress()
    style_main = progress_style_with_template("{bar:40.green/yellow} {pos:>4}/{len:4}")
    style_aux = progress_style_with_template("{spinner:.green} {msg} {pos:>4}/{len:4}")

    total = sum((e.bar.length() or 0) for e in ELEMENTS)
    pb_main = mp.add(ProgressBar(total))
    pb_main.set_style(style_main)
    for elem in ELEMENTS:
        elem.bar.set_style(style_aux)

    tree: list[Elem] = []
    pb_main.force_draw()

    while True:
        time.sleep(0.015)
        action = get_action(tree)
        if action is None:
            pb_main.finish()
            break

        add, index = action
        if add:
            elem = ELEMENTS[index]
            bar = mp.insert(elem.index + 1, elem.bar)
            bar.set_message("  " * elem.indent + "  " + elem.key)
            tree.insert(elem.index, elem)
        else:
            elem = tree[index]
            elem.bar.inc(1)
            if elem.bar.position() >= (elem.bar.length() or 0):
                elem.bar.finish_with_message("  " * elem.indent + "✔ " + elem.key)
            pb_main.inc(1)

    print("===============================")
    print("the tree should be the same as:")
    for elem in tree:
        print("  " * elem.indent + "  " + elem.key)


if __name__ == "__main__":
    main()
