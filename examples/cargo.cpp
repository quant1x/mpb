// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/cargo.rs：模拟 cargo 编译输出（不真的调用 cargo）。
#include <chrono>
#include <condition_variable>
#include <iomanip>
#include <iostream>
#include <mutex>
#include <optional>
#include <queue>
#include <random>
#include <sstream>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#include "mpb.h"

namespace {

    struct Crate {
        std::string name;
        std::string version;
    };

    const std::vector<Crate> crates = {
        {"console", "v0.14.1"},
        {"lazy_static", "v1.4.0"},
        {"libc", "v0.2.93"},
        {"regex", "v1.4.6"},
        {"regex-syntax", "v0.6.23"},
        {"terminal_size", "v0.1.16"},
        {"libc", "v0.2.93"},
        {"unicode-width", "v0.1.8"},
        {"lazy_static", "v1.4.0"},
        {"number_prefix", "v0.4.0"},
        {"regex", "v1.4.6"},
        {"rand", "v0.8.3"},
        {"getrandom", "v0.2.2"},
        {"cfg-if", "v1.0.0"},
        {"libc", "v0.2.93"},
        {"rand_chacha", "v0.3.0"},
        {"ppv-lite86", "v0.2.10"},
        {"rand_core", "v0.6.2"},
        {"getrandom", "v0.2.2"},
        {"rand_core", "v0.6.2"},
        {"tokio", "v1.5.0"},
        {"bytes", "v1.0.1"},
        {"pin-project-lite", "v0.2.6"},
        {"slab", "v0.4.3"},
        {"indicatif", "v0.15.0"},
    };

    constexpr int kWorkers = 4;

    struct Message {
        int  worker      = 0;
        int  crate_index = -1;
        bool has         = false;
    };

    std::string green_bold(const std::string &text) {
        return "\x1b[1;32m" + text + "\x1b[0m";
    }

    std::string padded_label(const std::string &label) {
        std::ostringstream out;
        out << std::setw(12) << std::right << label;
        return out.str();
    }

}  // namespace

int run_cargo(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    const auto started = std::chrono::steady_clock::now();

    std::string tpl = "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len}";
    if (mpb::stdout_term()->width() > 80) {
        tpl += " {wide_msg}";
    }

    auto pb    = mpb::ProgressBar::make(crates.size());
    auto style = mpb::progress_style_with_template(tpl);
    style.set_progress_chars("=> ");
    pb->set_style(std::move(style));
    pb->set_prefix("Building");

    std::mutex              next_mutex;
    std::size_t             next_index = 0;
    std::queue<Message>     messages;
    std::mutex              queue_mutex;
    std::condition_variable queue_cv;

    auto next_crate = [&]() -> std::optional<std::size_t> {
        std::lock_guard<std::mutex> lock(next_mutex);
        if (next_index >= crates.size())
            return std::nullopt;
        return next_index++;
    };

    std::vector<std::thread> workers;
    for (int worker = 0; worker < kWorkers; ++worker) {
        workers.emplace_back([&, worker] {
            std::mt19937 local(std::random_device{}());
            while (true) {
                const auto crate_index = next_crate();
                {
                    std::lock_guard<std::mutex> lock(queue_mutex);
                    if (!crate_index.has_value()) {
                        messages.push(Message{worker, -1, false});
                    } else {
                        messages.push(Message{worker, static_cast<int>(*crate_index), true});
                    }
                }
                queue_cv.notify_one();
                if (!crate_index.has_value())
                    return;

                const bool last = *crate_index == crates.size() - 1;
                const int  base = last ? 1000 : 250;
                const int  span = last ? 1000 : 750;
                mpb::sleep_for(std::chrono::milliseconds(base + static_cast<int>(local() % span)));
            }
        });
    }

    std::vector<int> processing(kWorkers, -1);
    int              finished = 0;
    while (finished < kWorkers) {
        Message message;
        {
            std::unique_lock<std::mutex> lock(queue_mutex);
            queue_cv.wait(lock, [&] { return !messages.empty(); });
            message = messages.front();
            messages.pop();
        }
        if (!message.has) {
            processing[static_cast<std::size_t>(message.worker)] = -1;
            ++finished;
            continue;
        }

        processing[static_cast<std::size_t>(message.worker)] = message.crate_index;
        std::string names;
        for (int index : processing) {
            if (index >= 0) {
                if (!names.empty())
                    names += ", ";
                names += crates[static_cast<std::size_t>(index)].name;
            }
        }
        pb->set_message(names);

        const auto &crate = crates[static_cast<std::size_t>(message.crate_index)];
        pb->println(green_bold(padded_label("Compiling")) + " " + crate.name + " " + crate.version);
        pb->inc(1);
    }

    for (auto &worker : workers)
        worker.join();
    pb->finish_and_clear();

    const auto elapsed = std::chrono::duration_cast<mpb::Duration>(std::chrono::steady_clock::now() - started);
    std::cout << green_bold(padded_label("Finished")) << " dev [unoptimized + debuginfo] target(s) in "
              << mpb::human_duration(elapsed) << '\n';
    return 0;
}
