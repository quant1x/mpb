// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// examples 是唯一的 C++ 示例入口：./mpb_examples <example> [args...]
#include <algorithm>
#include <iostream>
#include <string>
#include <vector>

#include "mpb.h"

int run_cargo(int argc, char **argv);
int run_cargo_wrap(int argc, char **argv);
int run_download(int argc, char **argv);
int run_download_continued(int argc, char **argv);
int run_download_speed(int argc, char **argv);
int run_fastbar(int argc, char **argv);
int run_finebars(int argc, char **argv);
int run_iterator(int argc, char **argv);
int run_log(int argc, char **argv);
int run_long_spinner(int argc, char **argv);
int run_morebars(int argc, char **argv);
int run_multi(int argc, char **argv);
int run_multi_tree(int argc, char **argv);
int run_multi_tree_ext(int argc, char **argv);
int run_single(int argc, char **argv);
int run_tokio(int argc, char **argv);
int run_yarnish(int argc, char **argv);

namespace {

    std::vector<std::string> example_names() {
        return {
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
        };
    }

    void usage() {
        auto names = example_names();
        std::sort(names.begin(), names.end());
        std::cerr << "usage: mpb_examples <example> [args...]\n";
        std::cerr << "examples:\n";
        for (const auto &name : names) {
            std::cerr << "  " << name << '\n';
        }
    }

}  // namespace

int main(int argc, char **argv) {
    // std::cout 的 UTF-8 输出需要控制台代码页配合；进度条自身走 WriteConsoleW。
    mpb::ConsoleGuard console;

    if (argc < 2) {
        usage();
        return 2;
    }

    // Accept both `multi-tree` (upstream name) and `multi_tree` (C++ file name).
    std::string name = argv[1];
    std::replace(name.begin(), name.end(), '_', '-');
    const int rest_argc = argc - 2;
    char    **rest_argv = argv + 2;

    if (name == "cargo")
        return run_cargo(rest_argc, rest_argv);
    if (name == "cargowrap")
        return run_cargo_wrap(rest_argc, rest_argv);
    if (name == "download")
        return run_download(rest_argc, rest_argv);
    if (name == "download-continued")
        return run_download_continued(rest_argc, rest_argv);
    if (name == "download-speed")
        return run_download_speed(rest_argc, rest_argv);
    if (name == "fastbar")
        return run_fastbar(rest_argc, rest_argv);
    if (name == "finebars")
        return run_finebars(rest_argc, rest_argv);
    if (name == "iterator")
        return run_iterator(rest_argc, rest_argv);
    if (name == "log")
        return run_log(rest_argc, rest_argv);
    if (name == "long-spinner")
        return run_long_spinner(rest_argc, rest_argv);
    if (name == "morebars")
        return run_morebars(rest_argc, rest_argv);
    if (name == "multi")
        return run_multi(rest_argc, rest_argv);
    if (name == "multi-tree")
        return run_multi_tree(rest_argc, rest_argv);
    if (name == "multi-tree-ext")
        return run_multi_tree_ext(rest_argc, rest_argv);
    if (name == "single")
        return run_single(rest_argc, rest_argv);
    if (name == "tokio")
        return run_tokio(rest_argc, rest_argv);
    if (name == "yarnish")
        return run_yarnish(rest_argc, rest_argv);

    std::cerr << "unknown example \"" << name << "\"\n\n";
    usage();
    return 2;
}
