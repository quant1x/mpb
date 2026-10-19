// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/cargowrap.rs：把 cargo build --release 的输出逐行喂给 spinner。
#include <chrono>
#include <cstdio>
#include <cstring>
#include <iostream>
#include <string>
#include <vector>

#include "mpb.h"

#if defined(_WIN32)
#define MPB_POPEN _popen
#define MPB_PCLOSE _pclose
#else
#define MPB_POPEN popen
#define MPB_PCLOSE pclose
#endif

namespace {

    std::vector<std::string> frames(const std::string &text) {
        std::vector<std::string> out;
        for (char c : text)
            out.emplace_back(1, c);
        return out;
    }

    std::string trim(std::string text) {
        const auto first = text.find_first_not_of(" \t\r\n");
        if (first == std::string::npos)
            return {};
        const auto last = text.find_last_not_of(" \t\r\n");
        return text.substr(first, last - first + 1);
    }

}  // namespace

int run_cargo_wrap(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    const auto started = std::chrono::steady_clock::now();
    std::cout << "Compiling package in release mode...\n";

    auto pb = mpb::ProgressBar::spinner();
    pb->enable_steady_tick(200ms);

    auto style = mpb::progress_style_with_template("{spinner:.dim.bold} cargo: {wide_msg}");
    style      = style.with_tick_strings(frames("/|\\- "));
    pb->set_style(std::move(style));

    std::FILE *pipe = MPB_POPEN("cargo build --release 2>&1", "r");
    if (pipe == nullptr) {
        std::cerr << "failed to start cargo\n";
        return 1;
    }

    char buffer[4096];
    while (std::fgets(buffer, sizeof(buffer), pipe) != nullptr) {
        const std::string line = trim(buffer);
        if (!line.empty()) {
            pb->set_message(line);
        }
        pb->force_draw();
    }
    MPB_PCLOSE(pipe);

    pb->finish_and_clear();
    const auto elapsed = std::chrono::duration_cast<mpb::Duration>(std::chrono::steady_clock::now() - started);
    std::cout << "Done in " << mpb::human_duration(elapsed) << '\n';
    return 0;
}
