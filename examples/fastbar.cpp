// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/fastbar.rs：极大量、极快的更新，用来看限速与耗时。
#include <chrono>
#include <cstdint>
#include <iomanip>
#include <iostream>

#include "mpb.h"

namespace {

    void many_units_of_easy_work(std::uint64_t count, const std::string &label) {
        auto          pb  = mpb::ProgressBar::make(count);
        std::uint64_t sum = 0;
        for (std::uint64_t i = 0; i < count; ++i) {
            sum += 2 * i + 3;
            pb->inc(1);
        }
        pb->finish();
        std::cout << '[' << label << "] Sum (" << sum << ") calculated in " << std::fixed << std::setprecision(6)
                  << std::chrono::duration<double>(pb->elapsed()).count() << "s\n";
    }

}  // namespace

int run_fastbar(int argc, char **argv) {
    (void)argc;
    (void)argv;
    many_units_of_easy_work(std::uint64_t{1} << 20, "Default progress bar ");
    return 0;
}
