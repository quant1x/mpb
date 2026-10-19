// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_ITER_H
#define MPB_ITER_H 1

#include <algorithm>
#include <atomic>
#include <cstddef>
#include <cstdint>
#include <istream>
#include <memory>
#include <optional>
#include <ostream>
#include <string>
#include <string_view>
#include <thread>
#include <type_traits>
#include <utility>
#include <vector>

#include "progress_bar.h"

namespace mpb {

    class SeekMax {
    public:
        SeekMax() : buffer_(kHistory, 0) {}

        std::uint64_t update_seq(std::uint64_t prev, std::uint64_t delta) {
            const std::uint64_t new_pos = prev + delta;
            if (!active_)
                return new_pos;
            ++seq_;
            if (seq_ >= kReset) {
                reset();
                return new_pos;
            }
            update(new_pos);
            return max();
        }

        std::uint64_t update_seek(std::uint64_t new_pos) {
            active_ = true;
            seq_    = 0;
            update(new_pos);
            return max();
        }

        void reset() {
            active_  = false;
            seq_     = 0;
            head_    = 0;
            max_pos_ = 0;
            std::fill(buffer_.begin(), buffer_.end(), 0);
        }

        std::uint64_t max() const { return buffer_[static_cast<std::size_t>(max_pos_)]; }

    private:
        static constexpr int kReset   = 5;
        static constexpr int kHistory = 10;

        void update(std::uint64_t value) {
            const std::uint64_t prev_max             = buffer_[static_cast<std::size_t>(max_pos_)];
            buffer_[static_cast<std::size_t>(head_)] = value;
            if (value > prev_max) {
                max_pos_ = head_;
            } else if (max_pos_ == head_ && value < prev_max) {
                int best = 0;
                for (int i = 1; i < static_cast<int>(buffer_.size()); ++i) {
                    if (buffer_[static_cast<std::size_t>(i)] > buffer_[static_cast<std::size_t>(best)]) {
                        best = i;
                    }
                }
                max_pos_ = best;
            }
            head_ = (head_ + 1) % static_cast<int>(buffer_.size());
        }

        std::vector<std::uint64_t> buffer_;
        int                        head_    = 0;
        int                        max_pos_ = 0;
        int                        seq_     = 0;
        bool                       active_  = false;
    };

    template <typename T>
    class ProgressBarIter {
    public:
        ProgressBarIter(std::vector<T> items, std::shared_ptr<ProgressBar> progress)
            : items_(std::move(items)), progress_(std::move(progress)), end_(items_.size()) {}

        bool next(T &out) {
            if (index_ >= end_) {
                finish();
                return false;
            }
            out = items_[index_++];
            if (progress_)
                progress_->inc(1);
            return true;
        }

        bool next_back(T &out) {
            if (index_ >= end_) {
                finish();
                return false;
            }
            out = items_[--end_];
            if (progress_)
                progress_->inc(1);
            return true;
        }

        std::size_t len() const { return end_ - index_; }
        std::size_t size() const { return len(); }

        const std::vector<T>        &items() const { return items_; }
        std::size_t                  index() const { return index_; }
        std::size_t                  end() const { return end_; }
        std::shared_ptr<ProgressBar> progress_bar() const { return progress_; }
        void                         consume_all() { index_ = end_; }

        ProgressBarIter &with_style(ProgressStyle style) {
            if (progress_)
                progress_->set_style(std::move(style));
            return *this;
        }
        ProgressBarIter &with_prefix(std::string prefix) {
            if (progress_)
                progress_->set_prefix(std::move(prefix));
            return *this;
        }
        ProgressBarIter &with_message(std::string message) {
            if (progress_)
                progress_->set_message(std::move(message));
            return *this;
        }
        ProgressBarIter &with_position(std::uint64_t position) {
            if (progress_)
                progress_->set_position(position);
            return *this;
        }
        ProgressBarIter &with_elapsed(Duration elapsed) {
            if (progress_)
                progress_->set_elapsed(elapsed);
            return *this;
        }
        ProgressBarIter &with_finish(ProgressFinish finish) {
            if (progress_)
                progress_->with_finish(std::move(finish));
            return *this;
        }

    private:
        void finish() {
            if (progress_ && !progress_->is_finished()) {
                progress_->finish_using_style();
            }
        }

        std::vector<T>               items_;
        std::shared_ptr<ProgressBar> progress_;
        std::size_t                  index_ = 0;
        std::size_t                  end_   = 0;
    };

    template <typename T>
    ProgressBarIter<T> progress(std::vector<T> items) {
        auto pb = ProgressBar::make(static_cast<std::uint64_t>(items.size()));
        return ProgressBarIter<T>(std::move(items), std::move(pb));
    }

    template <typename T>
    ProgressBarIter<T> progress_count(std::vector<T> items, std::uint64_t count) {
        auto pb = ProgressBar::make(count);
        return ProgressBarIter<T>(std::move(items), std::move(pb));
    }

    template <typename T>
    ProgressBarIter<T> progress_with(std::vector<T> items, std::shared_ptr<ProgressBar> progress_bar) {
        return ProgressBarIter<T>(std::move(items), std::move(progress_bar));
    }

    template <typename T>
    ProgressBarIter<T> progress_with_style(std::vector<T> items, ProgressStyle style) {
        auto pb = ProgressBar::make(static_cast<std::uint64_t>(items.size()));
        pb->set_style(std::move(style));
        return ProgressBarIter<T>(std::move(items), std::move(pb));
    }

    template <typename T, typename F>
    auto map_parallel(ProgressBarIter<T> &iter, unsigned workers, F &&func)
        -> std::vector<std::invoke_result_t<F, const T &>> {
        using Result = std::invoke_result_t<F, const T &>;
        if (workers == 0)
            workers = 1;

        const std::size_t begin = iter.index();
        const std::size_t end   = iter.end();
        const std::size_t count = end > begin ? end - begin : 0;
        if (count == 0) {
            iter.consume_all();
            return {};
        }

        std::vector<std::optional<Result>> results(count);
        std::atomic<std::size_t>           next{0};
        std::vector<std::thread>           threads;
        threads.reserve(workers);
        const auto &items    = iter.items();
        auto        progress = iter.progress_bar();

        for (unsigned worker = 0; worker < workers; ++worker) {
            threads.emplace_back([&] {
                while (true) {
                    const std::size_t i = next.fetch_add(1, std::memory_order_relaxed);
                    if (i >= count)
                        return;
                    if (progress)
                        progress->inc(1);
                    results[i].emplace(func(items[begin + i]));
                }
            });
        }
        for (auto &thread : threads)
            thread.join();
        iter.consume_all();

        std::vector<Result> out;
        out.reserve(count);
        for (auto &result : results) {
            out.push_back(std::move(*result));
        }
        return out;
    }

    class ProgressReader {
    public:
        ProgressReader(std::istream &input, std::shared_ptr<ProgressBar> progress)
            : input_(&input), progress_(std::move(progress)) {}

        std::size_t read(char *buffer, std::size_t size) {
            input_->read(buffer, static_cast<std::streamsize>(size));
            const auto count = static_cast<std::size_t>(input_->gcount());
            progress_->set_position(seek_max_.update_seq(progress_->position(), count));
            return count;
        }

    private:
        std::istream                *input_;
        std::shared_ptr<ProgressBar> progress_;
        SeekMax                      seek_max_;
    };

    class ProgressWriter {
    public:
        ProgressWriter(std::ostream &output, std::shared_ptr<ProgressBar> progress)
            : output_(&output), progress_(std::move(progress)) {}

        void write(std::string_view text) {
            output_->write(text.data(), static_cast<std::streamsize>(text.size()));
            progress_->set_position(seek_max_.update_seq(progress_->position(), text.size()));
        }

        void flush() { output_->flush(); }

    private:
        std::ostream                *output_;
        std::shared_ptr<ProgressBar> progress_;
        SeekMax                      seek_max_;
    };

}  // namespace mpb

#endif  // MPB_ITER_H
