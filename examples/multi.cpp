// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/multi.rs：MultiProgress 并发更新 + Println；-capture 输出纯文本帧。
#include <chrono>
#include <fstream>
#include <memory>
#include <random>
#include <string>
#include <thread>
#include <vector>

#include "mpb.h"

int run_multi(int argc, char** argv) {
    using namespace std::chrono_literals;
    bool capture = false;
    for (int i = 0; i < argc; ++i) {
        const std::string arg = argv[i];
        if (arg == "-capture" || arg == "--capture") capture = true;
    }

    std::shared_ptr<mpb::InMemoryTerm> memory;
    std::shared_ptr<mpb::MultiProgress> multi;
    if (capture) {
        memory = std::make_shared<mpb::InMemoryTerm>(2000, 200);
        multi = mpb::MultiProgress::with_draw_target(mpb::in_memory_draw_target(memory));
    } else {
        multi = mpb::MultiProgress::default_instance();
    }

    auto style = mpb::progress_style_with_template(
        "[{elapsed_precise}] {bar:40.cyan/blue} {pos:>7}/{len:7} {msg}");
    style.set_progress_chars("##-");

    constexpr std::uint64_t count = 200;
    auto todo = multi->add(mpb::ProgressBar::make(count));
    todo->set_style(style);
    todo->set_message("todo");

    auto finished = multi->add(mpb::ProgressBar::make(count));
    finished->set_style(style);
    finished->set_message("finished");

    auto fast = multi->insert_after(finished, mpb::ProgressBar::make(1024));
    fast->set_style(style);

    multi->println("starting!");

    std::thread fast_thread([multi, fast] {
        for (int i = 0; i < 1024; ++i) {
            std::this_thread::sleep_for(2ms);
            fast->inc(1);
        }
        multi->println("pb3 is done!");
        fast->finish_with_message("done");
    });

    std::mt19937 rng(std::random_device{}());
    std::uniform_real_distribution<double> delay(1.0, 5.0);
    std::vector<std::thread> slow_threads;
    for (std::uint64_t i = 0; i < count; ++i) {
        std::this_thread::sleep_for(15ms);
        if (i == count / 3) {
            std::this_thread::sleep_for(2s);
        }
        todo->inc(1);
        slow_threads.emplace_back([finished, wait = delay(rng)] {
            std::this_thread::sleep_for(std::chrono::duration<double>(wait));
            finished->inc(1);
        });
    }

    todo->finish_with_message("all jobs started");
    for (auto& thread : slow_threads) thread.join();
    fast_thread.join();
    finished->finish_with_message("all jobs done");

    if (capture) {
        std::ofstream out("examples/multi_capture.out", std::ios::binary);
        out << memory->contents();
    } else {
        multi->clear();
    }
    return 0;
}
