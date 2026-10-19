// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <algorithm>
#include <cmath>
#include <limits>
#include <sstream>
#include <string>
#include <utility>

#include "draw_target.h"
#include "in_memory.h"
#include "multi.h"
#include "state.h"
#include "utf8.h"

namespace mpb {
    namespace {

        constexpr std::uint64_t kAtomicIntervalNs = 1'000'000;
        constexpr std::uint8_t  kAtomicMaxBurst   = 10;

        double to_seconds(Duration duration) {
            return std::chrono::duration<double>(duration).count();
        }

        Duration from_seconds(double seconds) {
            if (!std::isfinite(seconds) || seconds <= 0.0) {
                return Duration{0};
            }
            const long double nanos = static_cast<long double>(seconds) * 1'000'000'000.0L;
            if (nanos >= static_cast<long double>(std::numeric_limits<std::int64_t>::max())) {
                return Duration::max();
            }
            return std::chrono::duration_cast<Duration>(std::chrono::duration<double>(seconds));
        }

        double estimator_weight(double age_seconds) {
            return std::pow(0.1, age_seconds / 15.0);
        }

        std::vector<std::string> split_lines(std::string_view text) {
            std::vector<std::string> out;
            std::size_t              start = 0;
            while (true) {
                const auto pos = text.find('\n', start);
                if (pos == std::string_view::npos) {
                    out.emplace_back(text.substr(start));
                    break;
                }
                out.emplace_back(text.substr(start, pos - start));
                start = pos + 1;
            }
            return out;
        }

        void append_message_lines(std::string_view          message,
                                  std::vector<LineType>    &lines,
                                  std::vector<std::string> &contents) {
            const auto parts = split_lines(message);
            if (parts.empty()) {
                lines.push_back(LineType::empty);
                contents.emplace_back();
                return;
            }
            for (const auto &part : parts) {
                lines.push_back(part.empty() ? LineType::empty : LineType::text);
                contents.push_back(part);
            }
        }

    }  // namespace

    // ---------------------------------------------------------------------------
    // AtomicPosition
    // ---------------------------------------------------------------------------

    AtomicPosition::AtomicPosition() : start_(std::chrono::steady_clock::now()) {
    }

    void AtomicPosition::inc(std::uint64_t delta) {
        pos_.fetch_add(delta, std::memory_order_relaxed);
    }

    void AtomicPosition::dec(std::uint64_t delta) {
        pos_.fetch_sub(delta, std::memory_order_relaxed);
    }

    void AtomicPosition::set(std::uint64_t value) {
        pos_.store(value, std::memory_order_release);
    }

    std::uint64_t AtomicPosition::pos() const {
        return pos_.load(std::memory_order_acquire);
    }

    bool AtomicPosition::allow(TimePoint now) {
        if (now < start_) {
            return false;
        }
        std::uint8_t        capacity = capacity_.load(std::memory_order_acquire);
        const std::uint64_t prev     = prev_.load(std::memory_order_acquire);
        const auto          elapsed  = static_cast<std::uint64_t>(
            std::chrono::duration_cast<std::chrono::nanoseconds>(now - start_).count());
        const std::uint64_t diff = elapsed > prev ? elapsed - prev : 0;

        if (capacity == 0 && diff < kAtomicIntervalNs) {
            return false;
        }

        const std::uint64_t new_intervals = diff / kAtomicIntervalNs;
        const std::uint64_t remainder     = diff % kAtomicIntervalNs;
        const std::uint64_t updated       = static_cast<std::uint64_t>(capacity) + new_intervals - 1;
        capacity = static_cast<std::uint8_t>(std::min<std::uint64_t>(kAtomicMaxBurst, updated));
        capacity_.store(capacity, std::memory_order_release);
        prev_.store(elapsed - remainder, std::memory_order_release);
        return true;
    }

    void AtomicPosition::reset(TimePoint now) {
        set(0);
        const auto elapsed = now > start_ ? now - start_ : Duration{0};
        prev_.store(static_cast<std::uint64_t>(std::chrono::duration_cast<std::chrono::nanoseconds>(elapsed).count()),
                    std::memory_order_release);
    }

    // ---------------------------------------------------------------------------
    // Estimator
    // ---------------------------------------------------------------------------

    Estimator::Estimator(TimePoint now) : prev_time_(now), start_time_(now) {
    }

    void Estimator::record(std::uint64_t new_steps, TimePoint now) {
        if (new_steps <= prev_steps_ || now <= prev_time_) {
            if (new_steps < prev_steps_) {
                prev_steps_ = new_steps;
                reset(now);
            }
            return;
        }

        const std::uint64_t delta_steps = new_steps - prev_steps_;
        const double        delta_t     = to_seconds(now - prev_time_);
        const double        new_sps     = static_cast<double>(delta_steps) / delta_t;
        const double        weight      = estimator_weight(delta_t);
        smoothed_                       = smoothed_ * weight + new_sps * (1.0 - weight);

        const double delta_t_start = to_seconds(now - start_time_);
        const double total_weight  = 1.0 - estimator_weight(delta_t_start);
        const double normalized    = total_weight != 0.0 ? smoothed_ / total_weight : 0.0;
        double_smoothed_           = double_smoothed_ * weight + normalized * (1.0 - weight);
        prev_steps_                = new_steps;
        prev_time_                 = now;
    }

    void Estimator::reset(TimePoint now) {
        smoothed_        = 0.0;
        double_smoothed_ = 0.0;
        prev_time_       = now;
        start_time_      = now;
    }

    double Estimator::steps_per_second(TimePoint now) const {
        const double delta_t       = to_seconds(now - prev_time_);
        const double reweight      = estimator_weight(delta_t);
        const double delta_t_start = to_seconds(now - start_time_);
        const double total_weight  = 1.0 - estimator_weight(delta_t_start);
        const double sps           = total_weight != 0.0 ? smoothed_ * reweight / total_weight : 0.0;
        const double dsps          = double_smoothed_ * reweight + sps * (1.0 - reweight);
        if (total_weight == 0.0) {
            return dsps;
        }
        return dsps / total_weight;
    }

    // ---------------------------------------------------------------------------
    // TabExpandedString
    // ---------------------------------------------------------------------------

    std::string TabExpandedString::expanded() const {
        if (original.find('\t') == std::string::npos) {
            return original;
        }
        std::string out;
        out.reserve(original.size());
        for (char c : original) {
            if (c == '\t') {
                out.append(static_cast<std::size_t>(std::max(0, tab_width)), ' ');
            } else {
                out.push_back(c);
            }
        }
        return out;
    }

    // ---------------------------------------------------------------------------
    // Draw state
    // ---------------------------------------------------------------------------

    Metrics wrapped_metrics(std::string_view text, int width) {
        if (width <= 0) {
            return {1, 0};
        }
        int lines  = 1;
        int column = 0;
        for (const auto &segment : ansi_segments(text)) {
            if (segment.is_ansi)
                continue;
            for (char32_t cp : utf8_decode(segment.text)) {
                const int cp_width = Width::char_width(cp);
                column += cp_width;
                if (column > width) {
                    ++lines;
                    column = cp_width;
                }
            }
        }
        return {lines, column};
    }

    int visual_line_count(const std::vector<LineType> &lines, const std::vector<std::string> &contents, int width) {
        int total = 0;
        for (std::size_t i = 0; i < lines.size(); ++i) {
            const std::string_view content = i < contents.size() ? contents[i] : std::string_view{};
            total += wrapped_metrics(content, width).height;
        }
        return total;
    }

    int draw_to_term(DrawState &state, TermLike &term, int previous_lines) {
        const int n  = previous_lines;
        const int up = std::max(0, previous_lines - 1);

        if (!state.lines.empty() && state.move_cursor) {
            if (up > 0)
                term.move_cursor_up(up);
            term.write_str("\r");
        } else {
            if (up > 0)
                term.move_cursor_up(up);
            for (int i = 0; i < n; ++i) {
                term.clear_line();
                if (i + 1 != n)
                    term.move_cursor_down(1);
            }
            if (up > 0)
                term.move_cursor_up(up);
        }

        const int term_width  = static_cast<int>(term.width());
        const int full_height = visual_line_count(state.lines, state.contents, term_width);

        int shift = 0;
        if (state.alignment == MultiProgressAlignment::bottom && full_height < previous_lines) {
            shift = previous_lines - full_height;
            for (int i = 0; i < shift; ++i) {
                term.write_line("");
            }
        }

        int real_height = 0;
        for (std::size_t i = 0; i < state.lines.size(); ++i) {
            const std::string_view content = i < state.contents.size() ? state.contents[i] : std::string_view{};
            const auto             metrics = wrapped_metrics(content, term_width);
            if (state.lines[i] == LineType::bar) {
                if (real_height + metrics.height > static_cast<int>(term.height())) {
                    break;
                }
                real_height += metrics.height;
            }
            term.write_str(content);
            const int filler = term_width - metrics.last_line_width;
            if (filler > 0) {
                term.write_str(std::string(static_cast<std::size_t>(filler), ' '));
            }
        }
        term.flush();
        return real_height + shift;
    }

    DrawStateWrapper::DrawStateWrapper(DrawState *state, std::vector<std::string> *orphan_lines)
        : state_(state), orphan_lines_(orphan_lines) {
    }

    DrawStateWrapper::~DrawStateWrapper() {
        close();
    }

    void DrawStateWrapper::reset() {
        if (state_ == nullptr)
            return;
        state_->lines.clear();
        state_->contents.clear();
    }

    void DrawStateWrapper::close() {
        if (closed_ || state_ == nullptr || orphan_lines_ == nullptr) {
            closed_ = true;
            return;
        }
        closed_                                     = true;
        const std::vector<LineType>    old_lines    = state_->lines;
        const std::vector<std::string> old_contents = state_->contents;
        std::vector<LineType>          new_lines;
        std::vector<std::string>       new_contents;
        for (std::size_t i = 0; i < old_lines.size(); ++i) {
            const std::string content = i < old_contents.size() ? old_contents[i] : std::string{};
            if (old_lines[i] == LineType::text || old_lines[i] == LineType::empty) {
                orphan_lines_->push_back(content);
            } else {
                new_lines.push_back(old_lines[i]);
                new_contents.push_back(content);
            }
        }
        state_->lines    = std::move(new_lines);
        state_->contents = std::move(new_contents);
    }

    // ---------------------------------------------------------------------------
    // ProgressState
    // ---------------------------------------------------------------------------

    ProgressState::ProgressState(std::optional<std::uint64_t> length, std::shared_ptr<AtomicPosition> position)
        : position_(std::move(position))
        , length_(length)
        , started_(std::chrono::steady_clock::now())
        , estimator_(started_)
        , message_{"", 8}
        , prefix_{"", 8} {
    }

    bool ProgressState::is_finished() const {
        return status_.is_finished();
    }

    float ProgressState::fraction() const {
        const std::uint64_t current = pos();
        if (!length_.has_value())
            return 0.0f;
        if (*length_ == 0)
            return 1.0f;
        if (current == 0)
            return 0.0f;
        float value = static_cast<float>(current) / static_cast<float>(*length_);
        return std::clamp(value, 0.0f, 1.0f);
    }

    Duration ProgressState::eta() const {
        if (is_finished() || !length_.has_value())
            return Duration{0};
        const std::uint64_t current = pos();
        const double        sps     = estimator_.steps_per_second(std::chrono::steady_clock::now());
        if (sps == 0.0)
            return Duration{0};
        const std::uint64_t remaining = *length_ > current ? *length_ - current : 0;
        return from_seconds(static_cast<double>(remaining) / sps);
    }

    Duration ProgressState::duration() const {
        if (status_.kind == StatusKind::done_visible || status_.kind == StatusKind::done_hidden) {
            return status_.duration;
        }
        if (!length_.has_value())
            return Duration{0};
        const Duration elapsed_value = elapsed();
        const Duration eta_value     = eta();
        if (elapsed_value.count() > std::numeric_limits<std::int64_t>::max() - eta_value.count()) {
            return Duration::max();
        }
        return elapsed_value + eta_value;
    }

    double ProgressState::per_sec() const {
        if (status_.kind == StatusKind::in_progress) {
            return estimator_.steps_per_second(std::chrono::steady_clock::now());
        }
        const double seconds = to_seconds(elapsed());
        if (seconds <= 0.0) {
            return pos() > 0 ? std::numeric_limits<double>::infinity() : std::numeric_limits<double>::quiet_NaN();
        }
        return static_cast<double>(pos()) / seconds;
    }

    Duration ProgressState::elapsed() const {
        return std::chrono::duration_cast<Duration>(std::chrono::steady_clock::now() - started_);
    }

    std::uint64_t ProgressState::pos() const {
        return position_->pos();
    }

    void ProgressState::set_pos(std::uint64_t pos) {
        position_->set(pos);
    }

    std::optional<std::uint64_t> ProgressState::len() const {
        return length_;
    }

    void ProgressState::set_len(std::uint64_t len) {
        length_ = len;
    }

    void ProgressState::increment_tick() {
        if (tick_ < kU64Max)
            ++tick_;
    }

    void ProgressState::set_tab_width(int width) {
        message_.set_tab_width(width);
        prefix_.set_tab_width(width);
    }

    // ---------------------------------------------------------------------------
    // BarState
    // ---------------------------------------------------------------------------

    BarState::BarState(std::optional<std::uint64_t>        length,
                       std::shared_ptr<ProgressDrawTarget> draw_target,
                       std::shared_ptr<AtomicPosition>     position)
        : draw_target_(std::move(draw_target))
        , style_(default_progress_style_bar())
        , state_(length, std::move(position)) {
    }

    BarState::~BarState() {
        try {
            auto ticker = std::move(ticker_);
            if (ticker)
                ticker->stop();
            if (!state_.is_finished()) {
                finish_using_style(std::chrono::steady_clock::now(), on_finish_);
            }
            if (draw_target_)
                draw_target_->mark_zombie();
        } catch (...) {
        }
    }

    void BarState::finish_using_style(TimePoint now, const ProgressFinish &finish) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        const Duration duration_value = std::chrono::duration_cast<Duration>(now - state_.started());
        state_.set_status(Status{StatusKind::done_visible, duration_value});

        switch (finish.kind) {
            case FinishKind::and_leave:
            case FinishKind::with_message:
            case FinishKind::and_clear:
                if (auto length = state_.len(); length.has_value()) {
                    state_.position()->set(*length);
                }
                if (finish.kind == FinishKind::with_message) {
                    state_.set_message(TabExpandedString{finish.message, tab_width_});
                } else if (finish.kind == FinishKind::and_clear) {
                    state_.set_status(Status{StatusKind::done_hidden, duration_value});
                }
                break;
            case FinishKind::abandon:
                break;
            case FinishKind::abandon_with_message:
                state_.set_message(TabExpandedString{finish.message, tab_width_});
                break;
        }
        draw_with_now_locked(true, now);
    }

    void BarState::reset(TimePoint now, ResetMode mode) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        state_.estimator().reset(now);
        if (mode == ResetMode::elapsed || mode == ResetMode::all) {
            state_.set_started(now);
        }
        if (mode == ResetMode::all) {
            state_.position()->reset(now);
            state_.set_status(Status{});
            style_.reset_trackers(state_, now);
            draw_with_now_locked(false, now);
        }
    }

    void BarState::update(TimePoint now, const std::function<void(ProgressState &)> &func, bool tick) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        func(state_);
        if (tick)
            tick_locked(now);
    }

    void BarState::unset_length(TimePoint now) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        state_.unset_len();
        update_estimate_and_draw(now);
    }

    void BarState::set_length(TimePoint now, std::uint64_t length) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        state_.set_len(length);
        update_estimate_and_draw(now);
    }

    void BarState::inc_length(TimePoint now, std::uint64_t delta) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (auto length = state_.len(); length.has_value()) {
            const auto updated = *length > kU64Max - delta ? kU64Max : *length + delta;
            state_.set_len(updated);
        }
        update_estimate_and_draw(now);
    }

    void BarState::dec_length(TimePoint now, std::uint64_t delta) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (auto length = state_.len(); length.has_value()) {
            state_.set_len(*length > delta ? *length - delta : 0);
        }
        update_estimate_and_draw(now);
    }

    void BarState::set_tab_width(int width) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        tab_width_ = width;
        state_.set_tab_width(width);
        style_.set_tab_width(width);
    }

    void BarState::set_style(ProgressStyle style) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (draw_target_ && draw_target_->is_stderr()) {
            style.set_for_stderr();
        }
        style_ = std::move(style);
        style_.set_tab_width(tab_width_);
    }

    void BarState::tick(TimePoint now) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        tick_locked(now);
    }

    void BarState::tick_locked(TimePoint now) {
        state_.increment_tick();
        update_estimate_and_draw(now);
    }

    void BarState::update_estimate_and_draw(TimePoint now) {
        state_.estimator().record(state_.pos(), now);
        style_.tick_trackers(state_, now);
        draw_with_now_locked(false, now);
    }

    void BarState::draw(bool force) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        draw_with_now_locked(force, std::chrono::steady_clock::now());
    }

    void BarState::draw_with_now(bool force, TimePoint now) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        draw_with_now_locked(force, now);
    }

    void BarState::draw_with_now_locked(bool force, TimePoint now) {
        force = force || state_.is_finished();
        if (draw_target_ && draw_target_->supports_aggregate()) {
            draw_target_->aggregate_draw_bar(this, force, now);
            return;
        }
        if (!draw_target_)
            return;
        auto drawable = draw_target_->drawable(force, now);
        if (!drawable)
            return;
        auto       wrapper = drawable->state();
        const auto width   = drawable->width();
        if (width.has_value() && state_.status().kind != StatusKind::done_hidden) {
            auto [lines, contents] = style_.format_state(state_, *width, draw_target_->is_stderr());
            wrapper->state().lines.insert(wrapper->state().lines.end(), lines.begin(), lines.end());
            wrapper->state().contents.insert(wrapper->state().contents.end(), contents.begin(), contents.end());
        }
        wrapper->close();
        drawable->draw();
    }

    void BarState::println(TimePoint now, std::string message) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (draw_target_ && draw_target_->supports_aggregate()) {
            std::vector<LineType>    lines;
            std::vector<std::string> contents;
            append_message_lines(message, lines, contents);
            draw_target_->aggregate_draw_origin(lines, contents, this, true, now);
            return;
        }
        if (!draw_target_)
            return;
        const auto width    = draw_target_->width();
        auto       drawable = draw_target_->drawable(true, now);
        if (!drawable)
            return;
        auto                     wrapper = drawable->state();
        std::vector<LineType>    lines;
        std::vector<std::string> contents;
        append_message_lines(message, lines, contents);
        wrapper->state().lines.insert(wrapper->state().lines.end(), lines.begin(), lines.end());
        wrapper->state().contents.insert(wrapper->state().contents.end(), contents.begin(), contents.end());
        if (width.has_value() && state_.status().kind != StatusKind::done_hidden) {
            auto [bar_lines, bar_contents] = style_.format_state(state_, *width, draw_target_->is_stderr());
            wrapper->state().lines.insert(wrapper->state().lines.end(), bar_lines.begin(), bar_lines.end());
            wrapper->state().contents.insert(wrapper->state().contents.end(), bar_contents.begin(), bar_contents.end());
        }
        wrapper->close();
        drawable->draw();
    }

    void BarState::suspend(const std::function<void()> &func) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        const auto                            now = std::chrono::steady_clock::now();
        if (draw_target_) {
            if (auto remote = draw_target_->remote(); remote.has_value()) {
                if (auto multi = remote->multi.lock()) {
                    std::lock_guard<std::recursive_mutex> multi_lock(multi->mutex);
                    multi->suspend(func, now);
                    return;
                }
            }
            if (auto drawable = draw_target_->drawable(true, now)) {
                drawable->clear();
            }
        }
        func();
        draw_with_now_locked(true, now);
    }

    std::optional<std::uint64_t> BarState::index() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (!draw_target_)
            return std::nullopt;
        if (auto remote = draw_target_->remote(); remote.has_value()) {
            return remote->index;
        }
        return std::nullopt;
    }

    std::optional<std::uint64_t> BarState::visual_index() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (!draw_target_)
            return std::nullopt;
        const auto remote = draw_target_->remote();
        if (!remote.has_value())
            return std::nullopt;
        if (auto multi = remote->multi.lock()) {
            std::lock_guard<std::recursive_mutex> multi_lock(multi->mutex);
            for (std::size_t i = 0; i < multi->ordering.size(); ++i) {
                if (multi->ordering[i] == remote->index) {
                    return static_cast<std::uint64_t>(i);
                }
            }
        }
        return std::nullopt;
    }

    std::optional<RemoteTargetInfo> BarState::remote_target() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (!draw_target_)
            return std::nullopt;
        return draw_target_->remote();
    }

    bool BarState::is_finished() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.is_finished();
    }

    bool BarState::is_hidden() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return !draw_target_ || draw_target_->is_hidden();
    }

    std::optional<std::uint64_t> BarState::length() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.len();
    }

    void BarState::set_pos(std::uint64_t value) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        state_.set_pos(value);
    }

    std::uint64_t BarState::position() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.pos();
    }

    std::string BarState::message() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.message().expanded();
    }

    std::string BarState::prefix() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.prefix().expanded();
    }

    Duration BarState::eta() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.eta();
    }

    double BarState::per_sec() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.per_sec();
    }

    Duration BarState::elapsed() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.elapsed();
    }

    Duration BarState::duration() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.duration();
    }

    ProgressStyle BarState::style() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return style_;
    }

    void BarState::set_on_finish(ProgressFinish finish) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        on_finish_ = std::move(finish);
    }

    ProgressFinish BarState::on_finish() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return on_finish_;
    }

    void BarState::set_prefix(const std::string &prefix) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        state_.set_prefix(TabExpandedString{prefix, tab_width_});
        update_estimate_and_draw(std::chrono::steady_clock::now());
    }

    void BarState::set_message(const std::string &message) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        state_.set_message(TabExpandedString{message, tab_width_});
        update_estimate_and_draw(std::chrono::steady_clock::now());
    }

    void BarState::set_elapsed(Duration elapsed) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        const TimePoint                       started = std::chrono::steady_clock::now() - elapsed;
        state_.set_started(started);
        state_.estimator().reset(started);
        update_estimate_and_draw(std::chrono::steady_clock::now());
    }

    void BarState::set_draw_target(std::shared_ptr<ProgressDrawTarget> target) {
        std::shared_ptr<ProgressDrawTarget> old;
        {
            std::lock_guard<std::recursive_mutex> lock(mutex_);
            old          = std::move(draw_target_);
            draw_target_ = std::move(target);
        }
        if (old)
            old->disconnect(std::chrono::steady_clock::now());
    }

    void BarState::set_draw_target_no_disconnect(std::shared_ptr<ProgressDrawTarget> target) {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        draw_target_ = std::move(target);
    }

    std::shared_ptr<ProgressDrawTarget> BarState::draw_target() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return draw_target_;
    }

    std::shared_ptr<AtomicPosition> BarState::position_obj() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return state_.position();
    }

    int BarState::tab_width() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return tab_width_;
    }

    std::pair<std::vector<LineType>, std::vector<std::string>> BarState::format_for_width(int width) const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        if (state_.status().kind == StatusKind::done_hidden) {
            return {{}, {}};
        }
        return style_.format_state(state_, width, draw_target_ ? draw_target_->is_stderr() : false);
    }

    void BarState::start_ticker(Duration interval) {
        if (interval.count() <= 0)
            return;
        std::shared_ptr<Ticker> old;
        {
            std::lock_guard<std::recursive_mutex> lock(mutex_);
            old     = std::move(ticker_);
            ticker_ = std::make_shared<Ticker>(interval, weak_from_this());
        }
        if (old)
            old->stop();
    }

    void BarState::stop_ticker() {
        std::shared_ptr<Ticker> ticker;
        {
            std::lock_guard<std::recursive_mutex> lock(mutex_);
            ticker = std::move(ticker_);
        }
        if (ticker)
            ticker->stop();
    }

    bool BarState::has_ticker() const {
        std::lock_guard<std::recursive_mutex> lock(mutex_);
        return ticker_ != nullptr;
    }

    // ---------------------------------------------------------------------------
    // Ticker
    // ---------------------------------------------------------------------------

    Ticker::Ticker(Duration interval, std::weak_ptr<BarState> state) : interval_(interval), state_(std::move(state)) {
        thread_ = std::thread([this] { run(); });
    }

    Ticker::~Ticker() {
        stop();
    }

    void Ticker::stop() {
        stop_.store(true, std::memory_order_release);
        cv_.notify_all();
        if (thread_.joinable()) {
            thread_.join();
        }
    }

    void Ticker::run() {
        std::unique_lock<std::mutex> lock(mutex_);
        while (!stop_.load(std::memory_order_acquire)) {
            auto state = state_.lock();
            if (!state)
                return;
            if (state->is_finished())
                return;
            state->tick(std::chrono::steady_clock::now());
            cv_.wait_for(lock, interval_, [this] { return stop_.load(std::memory_order_acquire); });
        }
    }

}  // namespace mpb
