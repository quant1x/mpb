// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <algorithm>
#include <utility>

#include "draw_target.h"
#include "in_memory.h"

namespace mpb {
    namespace {

        constexpr int kRateMaxBurst = 20;

    }  // namespace

    RateLimiter::RateLimiter(int rate)
        : interval_ms_(rate > 0 ? 1000 / rate : 1000)
        , capacity_(kRateMaxBurst)
        , prev_(std::chrono::steady_clock::now()) {
    }

    bool RateLimiter::allow(TimePoint now) {
        if (now < prev_)
            return false;
        const auto         elapsed     = std::chrono::duration_cast<std::chrono::nanoseconds>(now - prev_);
        const std::int64_t elapsed_ns  = elapsed.count();
        const std::int64_t interval_ns = static_cast<std::int64_t>(interval_ms_) * 1'000'000LL;

        if (capacity_ == 0 && elapsed_ns < interval_ns) {
            return false;
        }

        const std::int64_t elapsed_ms    = elapsed_ns / 1'000'000LL;
        const std::int64_t new_intervals = elapsed_ms / interval_ms_;
        const std::int64_t remainder     = elapsed_ns % interval_ns;
        const int          updated       = capacity_ + static_cast<int>(new_intervals) - 1;
        capacity_                        = std::min(kRateMaxBurst, updated);
        prev_                            = now - std::chrono::nanoseconds(remainder);
        return true;
    }

    TermDrawTargetBase::TermDrawTargetBase(std::shared_ptr<TermLike> term, std::optional<int> rate, bool stderr_flag)
        : term_(std::move(term)), stderr_(stderr_flag) {
        term_is_term_ = term_ && term_->is_term();
        if (rate.has_value()) {
            limiter_.emplace(*rate);
        }
    }

    std::optional<std::uint16_t> TermDrawTargetBase::width() const {
        if (!term_)
            return std::nullopt;
        return term_->width();
    }

    void TermDrawTargetBase::adjust_last_line_count(LineAdjust adjust) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (adjust.kind == LineAdjustKind::clear) {
            last_line_count_ += adjust.count;
        } else {
            last_line_count_ = std::max(0, last_line_count_ - adjust.count);
        }
    }

    void TermDrawTargetBase::set_move_cursor(bool value) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        draw_state_.move_cursor = value;
    }

    bool TermDrawTargetBase::is_move_cursor() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return draw_state_.move_cursor;
    }

    std::unique_ptr<Drawable> TermDrawTargetBase::drawable(bool force, TimePoint now) {
        if (!force && limiter_.has_value()) {
            std::lock_guard<std::recursive_mutex> lock(mutex_);
            if (!limiter_->allow(now)) {
                return nullptr;
            }
        }
        return std::make_unique<TermDrawable>(this);
    }

    std::unique_ptr<DrawStateWrapper> TermDrawable::state() {
        target_->draw_state_.lines.clear();
        target_->draw_state_.contents.clear();
        return std::make_unique<DrawStateWrapper>(&target_->draw_state_, nullptr);
    }

    void TermDrawable::draw() {
        if (!target_ || !target_->term_)
            return;
        std::lock_guard<std::recursive_mutex> lock(target_->mutex_);
        target_->last_line_count_ = draw_to_term(target_->draw_state_, *target_->term_, target_->last_line_count_);
    }

    void TermDrawable::clear() {
        if (!target_ || !target_->term_)
            return;
        std::lock_guard<std::recursive_mutex> lock(target_->mutex_);
        target_->draw_state_.lines.clear();
        target_->draw_state_.contents.clear();
        target_->last_line_count_ = draw_to_term(target_->draw_state_, *target_->term_, target_->last_line_count_);
    }

    std::optional<std::uint16_t> TermDrawable::width() const {
        return target_ ? target_->width() : std::nullopt;
    }

    std::shared_ptr<ProgressDrawTarget>
    term_draw_target(std::shared_ptr<TermLike> term, int refresh_rate, bool stderr_flag) {
        if (!term || !term->is_term() || is_dumb()) {
            return hidden_draw_target();
        }
        return std::make_shared<TermDrawTarget>(std::move(term), refresh_rate, stderr_flag);
    }

    std::shared_ptr<ProgressDrawTarget> term_like_draw_target(std::shared_ptr<TermLike> term,
                                                              std::optional<int>        refresh_rate) {
        return std::make_shared<TermLikeDrawTarget>(std::move(term), refresh_rate);
    }

    std::shared_ptr<ProgressDrawTarget> stdout_draw_target() {
        return stdout_draw_target_with_hz(20);
    }

    std::shared_ptr<ProgressDrawTarget> stderr_draw_target() {
        return stderr_draw_target_with_hz(20);
    }

    std::shared_ptr<ProgressDrawTarget> stdout_draw_target_with_hz(int refresh_rate) {
        return term_draw_target(stdout_term(), refresh_rate, false);
    }

    std::shared_ptr<ProgressDrawTarget> stderr_draw_target_with_hz(int refresh_rate) {
        return term_draw_target(stderr_term(), refresh_rate, true);
    }

    std::shared_ptr<ProgressDrawTarget> term_like_draw_target_with_hz(std::shared_ptr<TermLike> term,
                                                                      int                       refresh_rate) {
        return std::make_shared<TermLikeDrawTarget>(std::move(term), refresh_rate);
    }

    std::shared_ptr<ProgressDrawTarget> hidden_draw_target() {
        return std::make_shared<HiddenDrawTarget>();
    }

    std::shared_ptr<ProgressDrawTarget> in_memory_draw_target(const std::shared_ptr<InMemoryTerm> &term) {
        return std::make_shared<TermLikeDrawTarget>(term, std::nullopt);
    }

}  // namespace mpb
