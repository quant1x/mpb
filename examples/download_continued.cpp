// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/download-continued.rs：断点续传 + ResetEta。
#include <algorithm>
#include <chrono>
#include <thread>

#include "mpb.h"

int run_download_continued(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    std::uint64_t           downloaded = 69369369;
    constexpr std::uint64_t total_size = 231231231;

    auto pb    = mpb::ProgressBar::make(total_size);
    auto style = mpb::progress_style_with_template(
        "{spinner:.green} [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})");
    style.set_progress_chars("#>-");
    pb->set_style(std::move(style));
    pb->set_position(downloaded);
    pb->reset_eta();

    while (downloaded < total_size) {
        downloaded = std::min(downloaded + 123211, total_size);
        pb->set_position(downloaded);
        mpb::sleep_for(12ms);
    }
    pb->finish_with_message("downloaded");
    return 0;
}
