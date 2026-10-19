// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/yarnish.rs：模拟 yarn 安装的四段式输出。
#include <chrono>
#include <iostream>
#include <memory>
#include <random>
#include <string>
#include <thread>
#include <vector>

#include "mpb.h"

namespace {

const std::vector<std::string> packages = {
    "fs-events",       "my-awesome-module", "emoji-speaker",
    "wrap-ansi",       "stream-browserify", "acorn-dynamic-import",
};

const std::vector<std::string> commands = {
    "cmake .",       "make",          "make clean", "gcc foo.c -o foo",
    "gcc bar.c -o bar", "./helper.sh rebuild-cache", "make all-clean", "make test",
};

std::string bold_dim(const std::string& text) {
    return "\x1b[1;2m" + text + "\x1b[0m";
}

std::vector<std::string> frames(const std::string& text) {
    std::vector<std::string> out;
    for (char c : text) out.emplace_back(1, c);
    return out;
}

} // namespace

int run_yarnish(int argc, char** argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    const auto started = std::chrono::steady_clock::now();
    auto spinner_style = mpb::progress_style_with_template("{prefix:.bold.dim} {spinner} {wide_msg}");
    spinner_style = spinner_style.with_tick_strings(frames("⠁⠂⠄⡀⢀⠠⠐⠈ "));

    std::cout << bold_dim("[1/4]") << " 🔍  Resolving packages...\n";
    std::cout << bold_dim("[2/4]") << " 🚚  Fetching packages...\n";
    std::cout << bold_dim("[3/4]") << " 🔗  Linking dependencies...\n";

    constexpr int deps = 1232;
    auto pb = mpb::ProgressBar::make(deps);
    for (int i = 0; i < deps; ++i) {
        std::this_thread::sleep_for(3ms);
        pb->inc(1);
    }
    pb->finish_and_clear();

    std::cout << bold_dim("[4/4]") << " 📃  Building fresh packages...\n";

    auto multi = mpb::MultiProgress::default_instance();
    std::mt19937 rng(std::random_device{}());
    std::vector<std::thread> threads;
    for (int i = 0; i < 4; ++i) {
        const int count = 30 + static_cast<int>(rng() % 50);
        auto bar = multi->add(mpb::ProgressBar::make(count));
        bar->set_style(spinner_style);
        bar->set_prefix("[" + std::to_string(i + 1) + "/?]");

        const unsigned seed = static_cast<unsigned>(rng());
        threads.emplace_back([bar, count, seed]() mutable {
            std::mt19937 local(seed);
            const auto package = packages[local() % packages.size()];
            for (int j = 0; j < count; ++j) {
                const auto command = commands[local() % commands.size()];
                std::this_thread::sleep_for(
                    std::chrono::milliseconds(25 + static_cast<int>(local() % 175)));
                bar->set_message(package + ": " + command);
                bar->inc(1);
            }
            bar->finish_with_message("waiting...");
        });
    }
    for (auto& thread : threads) thread.join();
    multi->clear();

    std::cout << "✨  Done in " << mpb::human_duration(
        std::chrono::duration_cast<mpb::Duration>(std::chrono::steady_clock::now() - started))
              << '\n';
    return 0;
}
