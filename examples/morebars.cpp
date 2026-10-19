// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/morebars.rs：一条总进度条统着多条子进度条。
#include <chrono>
#include <thread>

#include "mpb.h"

int run_morebars(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    auto multi = mpb::MultiProgress::default_instance();
    auto style = mpb::progress_style_with_template("{bar:40.green/yellow} {pos:>7}/{len:7}");

    auto overall = multi->add(mpb::ProgressBar::make(5));
    overall->set_style(style);
    overall->force_draw();

    for (int i = 0; i < 5; ++i) {
        auto child = multi->add(mpb::ProgressBar::make(128));
        child->set_style(style);
        for (int j = 0; j < 128; ++j) {
            std::this_thread::sleep_for(5ms);
            child->inc(1);
        }
        child->finish();
        overall->inc(1);
    }
    overall->finish_with_message("done");
    return 0;
}
