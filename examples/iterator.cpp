// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/iterator.rs：三种迭代器包装。
#include <chrono>
#include <cstdint>
#include <iostream>
#include <thread>
#include <vector>

#include "mpb.h"

int run_iterator(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    const std::vector<int> items(1000, 0);

    {
        auto iter  = mpb::progress(items);
        int  value = 0;
        while (iter.next(value)) {
            mpb::sleep_for(5ms);
        }
    }

    {
        auto iter  = mpb::progress_count(items, 1000);
        int  value = 0;
        while (iter.next(value)) {
            mpb::sleep_for(5ms);
        }
    }

    auto pb    = mpb::ProgressBar::make(1000);
    auto style = mpb::progress_style_with_template("{spinner:.green} [{elapsed_precise}] [{bar:40.cyan/blue}] "
                                                   "({pos}/{len}, ETA {eta})");
    pb->set_style(std::move(style));
    auto iter  = mpb::progress_with(items, pb);
    int  value = 0;
    while (iter.next(value)) {
        mpb::sleep_for(5ms);
    }

    std::cout << "done\n";
    return 0;
}
