// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/download.rs：模拟下载进度，演示 bytes/total_bytes 与自定义 eta。
#include <algorithm>
#include <chrono>
#include <iomanip>
#include <sstream>
#include <thread>

#include "mpb.h"

int run_download(int argc, char** argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    constexpr std::uint64_t total_size = 231231231;
    std::uint64_t downloaded = 0;

    auto pb = mpb::ProgressBar::make(total_size);
    auto style = mpb::progress_style_with_template(
        "{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] "
        "{bytes}/{total_bytes} ({eta})");
    style.with_key("eta", [](const mpb::ProgressState& state) {
        std::ostringstream out;
        out << std::fixed << std::setprecision(1)
            << std::chrono::duration<double>(state.eta()).count() << 's';
        return out.str();
    });
    style.set_progress_chars("#>-");
    pb->set_style(std::move(style));

    while (downloaded < total_size) {
        downloaded = std::min(downloaded + 223211, total_size);
        pb->set_position(downloaded);
        std::this_thread::sleep_for(12ms);
    }
    pb->finish_with_message("downloaded");
    return 0;
}
