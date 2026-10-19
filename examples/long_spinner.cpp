// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/long-spinner.rs：steady tick + 自定义 spinner 帧。
#include <chrono>
#include <thread>
#include <vector>

#include "mpb.h"

int run_long_spinner(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    auto pb = mpb::ProgressBar::spinner();
    pb->enable_steady_tick(120ms);

    auto style = mpb::progress_style_with_template("{spinner:.blue} {msg}");
    style      = style.with_tick_strings({"▹▹▹▹▹", "▸▹▹▹▹", "▹▸▹▹▹", "▹▹▸▹▹", "▹▹▹▸▹", "▹▹▹▹▸", "▪▪▪▪▪"});
    pb->set_style(std::move(style));
    pb->set_message("Calculating...");
    std::this_thread::sleep_for(5s);
    pb->finish_with_message("Done");
    return 0;
}
