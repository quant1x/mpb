// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/log.rs：推进过程中用 Println 打日志。
#include <chrono>
#include <string>
#include <thread>

#include "mpb.h"

int main() {
    using namespace std::chrono_literals;
    auto pb = mpb::ProgressBar::make(100);
    for (int i = 0; i < 100; ++i) {
        std::this_thread::sleep_for(25ms);
        pb->println("[+] finished #" + std::to_string(i));
        pb->inc(1);
    }
    pb->finish_with_message("done");
    return 0;
}
