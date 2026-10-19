// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/tokio.rs：定时器驱动进度条（5ms × 1024）。
#include <chrono>
#include <thread>

#include "mpb.h"

int run_tokio(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    constexpr int steps = 1024;
    auto          pb    = mpb::ProgressBar::make(steps);
    for (int i = 0; i < steps; ++i) {
        mpb::sleep_for(5ms);
        pb->inc(1);
    }
    pb->finish();
    return 0;
}
