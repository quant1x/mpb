// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_PROGRESS_BAR_H
#define MPB_PROGRESS_BAR_H 1

#include <cstdint>
#include <functional>
#include <memory>
#include <optional>
#include <string>

#include "draw_target.h"
#include "state.h"
#include "style.h"

namespace mpb {

class ProgressBar {
public:
    explicit ProgressBar(std::optional<std::uint64_t> length = std::nullopt);

    static std::shared_ptr<ProgressBar> make(std::uint64_t length);
    static std::shared_ptr<ProgressBar> no_length();
    static std::shared_ptr<ProgressBar> hidden();
    static std::shared_ptr<ProgressBar> with_draw_target(
        std::optional<std::uint64_t> length,
        std::shared_ptr<ProgressDrawTarget> target);
    static std::shared_ptr<ProgressBar> spinner();

    std::shared_ptr<BarState> state() const { return state_; }

    void inc(std::uint64_t delta = 1);
    void dec(std::uint64_t delta = 1);
    void set_position(std::uint64_t position);

    bool is_hidden() const;
    bool is_finished() const;

    void finish();
    void finish_with_message(std::string message);
    void finish_and_clear();
    void abandon();
    void abandon_with_message(std::string message);
    void finish_using_style();

    void set_style(ProgressStyle style);
    void enable_steady_tick(Duration interval);
    void disable_steady_tick();
    void set_draw_target(std::shared_ptr<ProgressDrawTarget> target);
    void set_tab_width(int width);
    ProgressBar& with_tab_width(int width);

    std::string message() const;
    std::string prefix() const;
    void set_prefix(std::string prefix);
    void set_message(std::string message);
    void set_elapsed(Duration elapsed);
    void update(const std::function<void(ProgressState&)>& func);
    void unset_length();
    void set_length(std::uint64_t length);
    void inc_length(std::uint64_t delta);
    void dec_length(std::uint64_t delta);

    std::uint64_t position() const;
    std::optional<std::uint64_t> length() const;
    Duration eta() const;
    double per_sec() const;
    Duration elapsed() const;
    Duration duration() const;
    int tab_width() const;
    ProgressStyle style() const;
    std::optional<std::uint64_t> index() const;
    std::optional<std::uint64_t> visual_index() const;

    void force_draw();
    void println(std::string message);
    void suspend(const std::function<void()>& func);
    void reset_eta();
    void reset_elapsed();
    void reset();

    std::shared_ptr<ProgressBar> clone() const;
    std::weak_ptr<BarState> downgrade() const;

    ProgressBar& with_style(ProgressStyle style);
    ProgressBar& with_prefix(std::string prefix);
    ProgressBar& with_message(std::string message);
    ProgressBar& with_position(std::uint64_t position);
    ProgressBar& with_elapsed(Duration elapsed);
    ProgressBar& with_finish(ProgressFinish finish);

private:
    friend class WeakProgressBar;

    ProgressBar(std::shared_ptr<BarState> state, std::shared_ptr<AtomicPosition> pos);

    void tick(TimePoint now);

    std::shared_ptr<AtomicPosition> pos_;
    std::shared_ptr<BarState> state_;
};

class WeakProgressBar {
public:
    WeakProgressBar() = default;
    explicit WeakProgressBar(std::weak_ptr<BarState> state) : state_(std::move(state)) {}

    std::shared_ptr<ProgressBar> upgrade() const;

private:
    std::weak_ptr<BarState> state_;
};

} // namespace mpb

#endif // MPB_PROGRESS_BAR_H
