// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/single.rs：最基础的单条进度条。
#include <chrono>
#include <thread>

#include "mpb.h"

int run_single(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    auto pb = mpb::ProgressBar::make(1024);
    for (int i = 0; i < 1024; ++i) {
        mpb::sleep_for(5ms);
        pb->inc(1);
    }
    pb->finish_with_message("done");
    return 0;
}
