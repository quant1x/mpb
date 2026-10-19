// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/finebars.rs：5 条不同粒度/颜色的进度条并行推进。
#include <chrono>
#include <iomanip>
#include <memory>
#include <random>
#include <sstream>
#include <string>
#include <thread>
#include <vector>

#include "mpb.h"

int run_finebars(int argc, char** argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    struct StyleSpec {
        const char* prefix;
        const char* chars;
        const char* color;
    };
    const std::vector<StyleSpec> styles = {
        {"Rough bar:", "█  ", "red"},
        {"Fine bar: ", "█▉▊▋▌▍▎▏  ", "yellow"},
        {"Vertical: ", "█▇▆▅▄▃▂▁  ", "green"},
        {"Fade in:  ", "█▓▒░  ", "blue"},
        {"Blocky:   ", "█▛▌▖  ", "magenta"},
    };

    auto multi = mpb::MultiProgress::default_instance();
    std::vector<std::thread> threads;
    std::mt19937 rng(std::random_device{}());
    std::uniform_int_distribution<int> delay(10, 30);

    for (const auto& spec : styles) {
        auto pb = multi->add(mpb::ProgressBar::make(512));
        auto style = mpb::progress_style_with_template(
            std::string("{prefix:.bold}▕{bar:.") + spec.color + "}▏{msg}");
        style.set_progress_chars(spec.chars);
        pb->set_style(std::move(style));
        pb->set_prefix(spec.prefix);

        const auto wait = std::chrono::milliseconds(delay(rng));
        threads.emplace_back([pb, wait] {
            for (int i = 0; i < 512; ++i) {
                std::this_thread::sleep_for(wait);
                pb->inc(1);
                std::ostringstream message;
                message << std::setw(3) << (100 * i / 512) << '%';
                pb->set_message(message.str());
            }
            pb->finish_with_message("100%");
        });
    }

    for (auto& thread : threads) thread.join();
    return 0;
}
