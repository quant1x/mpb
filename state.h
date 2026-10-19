// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_STATE_H
#define MPB_STATE_H 1

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstdint>
#include <functional>
#include <memory>
#include <mutex>
#include <optional>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#include "style.h"
#include "term_like.h"
#include "types.h"

namespace mpb {

    class ProgressDrawTarget;
    class BarState;
    class MultiState;

    struct RemoteTargetInfo {
        std::weak_ptr<MultiState> multi;
        std::uint64_t             index = 0;
    };

    // ---------------------------------------------------------------------------
    // AtomicPosition
    // ---------------------------------------------------------------------------

    class AtomicPosition {
    public:
        AtomicPosition();

        void          inc(std::uint64_t delta);
        void          dec(std::uint64_t delta);
        void          set(std::uint64_t value);
        std::uint64_t pos() const;
        bool          allow(TimePoint now);
        void          reset(TimePoint now);

    private:
        std::atomic<std::uint64_t> pos_{0};
        std::atomic<std::uint8_t>  capacity_{10};
        std::atomic<std::uint64_t> prev_{0};
        TimePoint                  start_;
    };

    // ---------------------------------------------------------------------------
    // Estimator
    // ---------------------------------------------------------------------------

    class Estimator {
    public:
        explicit Estimator(TimePoint now);

        void   record(std::uint64_t new_steps, TimePoint now);
        void   reset(TimePoint now);
        double steps_per_second(TimePoint now) const;

    private:
        double        smoothed_        = 0.0;
        double        double_smoothed_ = 0.0;
        std::uint64_t prev_steps_      = 0;
        TimePoint     prev_time_;
        TimePoint     start_time_;
    };

    // ---------------------------------------------------------------------------
    // TabExpandedString
    // ---------------------------------------------------------------------------

    struct TabExpandedString {
        std::string original;
        int         tab_width = 8;

        std::string expanded() const;
        void        set_tab_width(int width) { tab_width = width; }
    };

    // ---------------------------------------------------------------------------
    // Draw state / drawable
    // ---------------------------------------------------------------------------

    struct DrawState {
        std::vector<LineType>    lines;
        std::vector<std::string> contents;
        bool                     move_cursor = false;
        MultiProgressAlignment   alignment   = MultiProgressAlignment::top;
    };

    struct Metrics {
        int height          = 1;
        int last_line_width = 0;
    };

    Metrics wrapped_metrics(std::string_view text, int width);
    int     visual_line_count(const std::vector<LineType> &lines, const std::vector<std::string> &contents, int width);
    int     draw_to_term(DrawState &state, TermLike &term, int previous_lines);

    class DrawStateWrapper {
    public:
        DrawStateWrapper(DrawState *state, std::vector<std::string> *orphan_lines);
        ~DrawStateWrapper();

        DrawStateWrapper(const DrawStateWrapper &)            = delete;
        DrawStateWrapper &operator=(const DrawStateWrapper &) = delete;

        DrawState &state() { return *state_; }
        void       reset();
        void       close();

    private:
        DrawState                *state_        = nullptr;
        std::vector<std::string> *orphan_lines_ = nullptr;
        bool                      closed_       = false;
    };

    class Drawable {
    public:
        virtual ~Drawable() = default;

        virtual std::unique_ptr<DrawStateWrapper> state()       = 0;
        virtual void                              draw()        = 0;
        virtual void                              clear()       = 0;
        virtual std::optional<std::uint16_t>      width() const = 0;
    };

    // ---------------------------------------------------------------------------
    // ProgressState
    // ---------------------------------------------------------------------------

    class ProgressState {
    public:
        ProgressState(std::optional<std::uint64_t> length, std::shared_ptr<AtomicPosition> position);

        bool                            is_finished() const;
        float                           fraction() const;
        Duration                        eta() const;
        Duration                        duration() const;
        double                          per_sec() const;
        Duration                        elapsed() const;
        std::uint64_t                   pos() const;
        void                            set_pos(std::uint64_t pos);
        std::optional<std::uint64_t>    len() const;
        void                            set_len(std::uint64_t len);
        void                            unset_len() { length_.reset(); }
        std::uint64_t                   tick() const { return tick_; }
        void                            increment_tick();
        TimePoint                       started() const { return started_; }
        void                            set_started(TimePoint started) { started_ = started; }
        Status                          status() const { return status_; }
        void                            set_status(Status status) { status_ = status; }
        Estimator                      &estimator() { return estimator_; }
        const Estimator                &estimator() const { return estimator_; }
        const TabExpandedString        &message() const { return message_; }
        const TabExpandedString        &prefix() const { return prefix_; }
        void                            set_message(TabExpandedString message) { message_ = std::move(message); }
        void                            set_prefix(TabExpandedString prefix) { prefix_ = std::move(prefix); }
        void                            set_tab_width(int width);
        std::shared_ptr<AtomicPosition> position() const { return position_; }

    private:
        std::shared_ptr<AtomicPosition> position_;
        std::optional<std::uint64_t>    length_;
        std::uint64_t                   tick_ = 0;
        TimePoint                       started_;
        Status                          status_{};
        Estimator                       estimator_;
        TabExpandedString               message_;
        TabExpandedString               prefix_;
    };

    // ---------------------------------------------------------------------------
    // BarState
    // ---------------------------------------------------------------------------

    class BarState : public std::enable_shared_from_this<BarState> {
    public:
        BarState(std::optional<std::uint64_t>        length,
                 std::shared_ptr<ProgressDrawTarget> draw_target,
                 std::shared_ptr<AtomicPosition>     position);
        ~BarState();

        BarState(const BarState &)            = delete;
        BarState &operator=(const BarState &) = delete;

        void finish_using_style(TimePoint now, const ProgressFinish &finish);
        void reset(TimePoint now, ResetMode mode);
        void update(TimePoint now, const std::function<void(ProgressState &)> &func, bool tick);
        void unset_length(TimePoint now);
        void set_length(TimePoint now, std::uint64_t length);
        void inc_length(TimePoint now, std::uint64_t delta);
        void dec_length(TimePoint now, std::uint64_t delta);
        void set_tab_width(int width);
        void set_style(ProgressStyle style);
        void tick(TimePoint now);
        void draw(bool force);
        void draw_with_now(bool force, TimePoint now);
        void println(TimePoint now, std::string message);
        void suspend(const std::function<void()> &func);

        std::optional<std::uint64_t>    index() const;
        std::optional<std::uint64_t>    visual_index() const;
        std::optional<RemoteTargetInfo> remote_target() const;

        bool                                is_finished() const;
        bool                                is_hidden() const;
        std::optional<std::uint64_t>        length() const;
        void                                set_pos(std::uint64_t value);
        std::uint64_t                       position() const;
        std::string                         message() const;
        std::string                         prefix() const;
        Duration                            eta() const;
        double                              per_sec() const;
        Duration                            elapsed() const;
        Duration                            duration() const;
        ProgressStyle                       style() const;
        void                                set_on_finish(ProgressFinish finish);
        ProgressFinish                      on_finish() const;
        void                                set_prefix(const std::string &prefix);
        void                                set_message(const std::string &message);
        void                                set_elapsed(Duration elapsed);
        void                                set_draw_target(std::shared_ptr<ProgressDrawTarget> target);
        void                                set_draw_target_no_disconnect(std::shared_ptr<ProgressDrawTarget> target);
        std::shared_ptr<ProgressDrawTarget> draw_target() const;
        std::shared_ptr<AtomicPosition>     position_obj() const;
        int                                 tab_width() const;

        std::pair<std::vector<LineType>, std::vector<std::string>> format_for_width(int width) const;

        // Used by RemoteDrawTarget's aggregate path.
        const ProgressStyle &style_ref() const { return style_; }
        const ProgressState &progress_state() const { return state_; }
        ProgressState       &progress_state() { return state_; }

        void start_ticker(Duration interval);
        void stop_ticker();
        bool has_ticker() const;

    private:
        void update_estimate_and_draw(TimePoint now);
        void draw_with_now_locked(bool force, TimePoint now);
        void tick_locked(TimePoint now);

        mutable std::recursive_mutex        mutex_;
        std::shared_ptr<ProgressDrawTarget> draw_target_;
        ProgressFinish                      on_finish_ = ProgressFinish::default_finish();
        ProgressStyle                       style_;
        ProgressState                       state_;
        int                                 tab_width_ = 8;
        std::shared_ptr<class Ticker>       ticker_;
    };

    // ---------------------------------------------------------------------------
    // Ticker
    // ---------------------------------------------------------------------------

    class Ticker {
    public:
        Ticker(Duration interval, std::weak_ptr<BarState> state);
        ~Ticker();

        Ticker(const Ticker &)            = delete;
        Ticker &operator=(const Ticker &) = delete;

        void stop();

    private:
        void run();

        Duration                interval_;
        std::weak_ptr<BarState> state_;
        std::atomic<bool>       stop_{false};
        std::mutex              mutex_;
        std::condition_variable cv_;
        std::thread             thread_;
    };

}  // namespace mpb

#endif  // MPB_STATE_H
