// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <stdexcept>
#include <utility>

#include "progress_bar.h"

namespace mpb {

    ProgressBar::ProgressBar(std::optional<std::uint64_t> length)
        : pos_(std::make_shared<AtomicPosition>())
        , state_(std::make_shared<BarState>(length, stderr_draw_target(), pos_)) {
    }

    ProgressBar::ProgressBar(std::shared_ptr<BarState> state, std::shared_ptr<AtomicPosition> pos)
        : pos_(std::move(pos)), state_(std::move(state)) {
    }

    std::shared_ptr<ProgressBar> ProgressBar::make(std::uint64_t length) {
        return std::make_shared<ProgressBar>(std::optional<std::uint64_t>(length));
    }

    std::shared_ptr<ProgressBar> ProgressBar::no_length() {
        return std::make_shared<ProgressBar>(std::nullopt);
    }

    std::shared_ptr<ProgressBar> ProgressBar::hidden() {
        auto pos   = std::make_shared<AtomicPosition>();
        auto state = std::make_shared<BarState>(std::nullopt, hidden_draw_target(), pos);
        return std::shared_ptr<ProgressBar>(new ProgressBar(std::move(state), std::move(pos)));
    }

    std::shared_ptr<ProgressBar> ProgressBar::with_draw_target(std::optional<std::uint64_t>        length,
                                                               std::shared_ptr<ProgressDrawTarget> target) {
        auto pos   = std::make_shared<AtomicPosition>();
        auto state = std::make_shared<BarState>(length, std::move(target), pos);
        return std::shared_ptr<ProgressBar>(new ProgressBar(std::move(state), std::move(pos)));
    }

    std::shared_ptr<ProgressBar> ProgressBar::spinner() {
        auto pb = no_length();
        pb->set_style(default_progress_style_spinner());
        return pb;
    }

    void ProgressBar::inc(std::uint64_t delta) {
        pos_->inc(delta);
        const auto now = std::chrono::steady_clock::now();
        if (pos_->allow(now)) {
            tick(now);
        }
    }

    void ProgressBar::dec(std::uint64_t delta) {
        pos_->dec(delta);
        const auto now = std::chrono::steady_clock::now();
        if (pos_->allow(now)) {
            tick(now);
        }
    }

    void ProgressBar::set_position(std::uint64_t position) {
        pos_->set(position);
        const auto now = std::chrono::steady_clock::now();
        if (pos_->allow(now)) {
            tick(now);
        }
    }

    void ProgressBar::tick(TimePoint now) {
        if (state_->has_ticker())
            return;
        state_->tick(now);
    }

    bool ProgressBar::is_hidden() const {
        return state_->is_hidden();
    }
    bool ProgressBar::is_finished() const {
        return state_->is_finished();
    }

    void ProgressBar::finish() {
        if (const auto length = state_->length(); length.has_value()) {
            pos_->set(*length);
            state_->set_pos(*length);
        }
        state_->finish_using_style(std::chrono::steady_clock::now(), ProgressFinish{FinishKind::and_leave, {}});
    }

    void ProgressBar::finish_with_message(std::string message) {
        state_->finish_using_style(std::chrono::steady_clock::now(),
                                   ProgressFinish{FinishKind::with_message, std::move(message)});
    }

    void ProgressBar::finish_and_clear() {
        if (const auto length = state_->length(); length.has_value()) {
            pos_->set(*length);
            state_->set_pos(*length);
        }
        state_->finish_using_style(std::chrono::steady_clock::now(), ProgressFinish{FinishKind::and_clear, {}});
    }

    void ProgressBar::abandon() {
        state_->finish_using_style(std::chrono::steady_clock::now(), ProgressFinish{FinishKind::abandon, {}});
    }

    void ProgressBar::abandon_with_message(std::string message) {
        state_->finish_using_style(std::chrono::steady_clock::now(),
                                   ProgressFinish{FinishKind::abandon_with_message, std::move(message)});
    }

    void ProgressBar::finish_using_style() {
        state_->finish_using_style(std::chrono::steady_clock::now(), state_->on_finish());
    }

    void ProgressBar::set_style(ProgressStyle style) {
        state_->set_style(std::move(style));
    }

    void ProgressBar::enable_steady_tick(Duration interval) {
        state_->start_ticker(interval);
    }

    void ProgressBar::disable_steady_tick() {
        state_->stop_ticker();
    }

    void ProgressBar::set_draw_target(std::shared_ptr<ProgressDrawTarget> target) {
        state_->set_draw_target(std::move(target));
    }

    void ProgressBar::set_tab_width(int width) {
        state_->set_tab_width(width);
        state_->draw(true);
    }

    ProgressBar &ProgressBar::with_tab_width(int width) {
        state_->set_tab_width(width);
        return *this;
    }

    std::string ProgressBar::message() const {
        return state_->message();
    }
    std::string ProgressBar::prefix() const {
        return state_->prefix();
    }

    void ProgressBar::set_prefix(std::string prefix) {
        state_->set_prefix(prefix);
    }

    void ProgressBar::set_message(std::string message) {
        state_->set_message(message);
    }

    void ProgressBar::set_elapsed(Duration elapsed) {
        if (elapsed.count() < 0) {
            throw std::invalid_argument("elapsed must be >= 0");
        }
        state_->set_elapsed(elapsed);
    }

    void ProgressBar::update(const std::function<void(ProgressState &)> &func) {
        state_->update(std::chrono::steady_clock::now(), func, !state_->has_ticker());
    }

    void ProgressBar::unset_length() {
        state_->unset_length(std::chrono::steady_clock::now());
    }

    void ProgressBar::set_length(std::uint64_t length) {
        state_->set_length(std::chrono::steady_clock::now(), length);
    }

    void ProgressBar::inc_length(std::uint64_t delta) {
        state_->inc_length(std::chrono::steady_clock::now(), delta);
    }

    void ProgressBar::dec_length(std::uint64_t delta) {
        state_->dec_length(std::chrono::steady_clock::now(), delta);
    }

    std::uint64_t ProgressBar::position() const {
        return state_->position();
    }
    std::optional<std::uint64_t> ProgressBar::length() const {
        return state_->length();
    }
    Duration ProgressBar::eta() const {
        return state_->eta();
    }
    double ProgressBar::per_sec() const {
        return state_->per_sec();
    }
    Duration ProgressBar::elapsed() const {
        return state_->elapsed();
    }
    Duration ProgressBar::duration() const {
        return state_->duration();
    }
    int ProgressBar::tab_width() const {
        return state_->tab_width();
    }
    ProgressStyle ProgressBar::style() const {
        return state_->style();
    }
    std::optional<std::uint64_t> ProgressBar::index() const {
        return state_->index();
    }
    std::optional<std::uint64_t> ProgressBar::visual_index() const {
        return state_->visual_index();
    }

    void ProgressBar::force_draw() {
        state_->draw(true);
    }
    void ProgressBar::println(std::string message) {
        state_->println(std::chrono::steady_clock::now(), std::move(message));
    }
    void ProgressBar::suspend(const std::function<void()> &func) {
        state_->suspend(func);
    }
    void ProgressBar::reset_eta() {
        state_->reset(std::chrono::steady_clock::now(), ResetMode::eta);
    }
    void ProgressBar::reset_elapsed() {
        state_->reset(std::chrono::steady_clock::now(), ResetMode::elapsed);
    }
    void ProgressBar::reset() {
        state_->reset(std::chrono::steady_clock::now(), ResetMode::all);
    }

    std::shared_ptr<ProgressBar> ProgressBar::clone() const {
        return std::shared_ptr<ProgressBar>(new ProgressBar(state_, pos_));
    }

    std::weak_ptr<BarState> ProgressBar::downgrade() const {
        return state_;
    }

    ProgressBar &ProgressBar::with_style(ProgressStyle style) {
        set_style(std::move(style));
        return *this;
    }
    ProgressBar &ProgressBar::with_prefix(std::string prefix) {
        set_prefix(std::move(prefix));
        return *this;
    }
    ProgressBar &ProgressBar::with_message(std::string message) {
        set_message(std::move(message));
        return *this;
    }
    ProgressBar &ProgressBar::with_position(std::uint64_t position) {
        set_position(position);
        return *this;
    }
    ProgressBar &ProgressBar::with_elapsed(Duration elapsed) {
        set_elapsed(elapsed);
        return *this;
    }
    ProgressBar &ProgressBar::with_finish(ProgressFinish finish) {
        state_->set_on_finish(std::move(finish));
        return *this;
    }

    std::shared_ptr<ProgressBar> WeakProgressBar::upgrade() const {
        auto state = state_.lock();
        if (!state)
            return nullptr;
        auto pos = state->position_obj();
        return std::shared_ptr<ProgressBar>(new ProgressBar(std::move(state), std::move(pos)));
    }

}  // namespace mpb
