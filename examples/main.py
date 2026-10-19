# -*- coding: utf-8 -*-
# Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
# SPDX-License-Identifier: MIT

"""Python 示例入口。

用法::

    python -m examples <example> [args...]

每个示例的实现是 ``examples/<name>.py``，目录名中的连字符映射为下划线。
"""

from __future__ import annotations

import importlib
import sys

from mpb import enable_console

EXAMPLES = [
    "cargo",
    "cargowrap",
    "download",
    "download-continued",
    "download-speed",
    "fastbar",
    "finebars",
    "iterator",
    "log",
    "long-spinner",
    "morebars",
    "multi",
    "multi-tree",
    "multi-tree-ext",
    "single",
    "tokio",
    "yarnish",
]


def _usage() -> None:
    names = "\n".join("  " + name for name in sorted(EXAMPLES))
    sys.stderr.write("usage: python -m examples <example> [args...]\n")
    sys.stderr.write("examples:\n" + names + "\n")


def main(argv: list[str] | None = None) -> int:
    enable_console()
    args = list(sys.argv[1:] if argv is None else argv)
    if not args or args[0] in ("-h", "--help"):
        _usage()
        return 0 if args else 2

    name = args[0]
    if name not in EXAMPLES:
        sys.stderr.write(f"unknown example {name!r}\n\n")
        _usage()
        return 2

    module = importlib.import_module(f"examples.{name.replace('-', '_')}")
    old_argv = sys.argv
    sys.argv = [old_argv[0], *args[1:]]
    try:
        module.main()
    finally:
        sys.argv = old_argv
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
