// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_DRAW_TARGET_H
#define MPB_DRAW_TARGET_H 1

#include <cstdint>
#include <memory>
#include <mutex>
#include <optional>
#include <string>
#include <vector>

#include "state.h"
#include "term_like.h"

namespace mpb {

    class InMemoryTerm;

    class RateLimiter {
    public:
        explicit RateLimiter(int rate);

        bool allow(TimePoint now);

    private:
        int       interval_ms_ = 1000;
        int       capacity_    = 20;
        TimePoint prev_;
    };

    class ProgressDrawTarget {
    public:
        virtual ~ProgressDrawTarget() = default;

        virtual bool                            is_hidden() const = 0;
        virtual bool                            is_stderr() const { return false; }
        virtual std::optional<std::uint16_t>    width() const = 0;
        virtual std::optional<RemoteTargetInfo> remote() const { return std::nullopt; }
        virtual void                            disconnect(TimePoint) {}
        virtual void                            adjust_last_line_count(LineAdjust) {}
        virtual void                            mark_zombie() {}
        virtual std::unique_ptr<Drawable>       drawable(bool force, TimePoint now) = 0;

        virtual void set_move_cursor(bool) {}
        virtual bool is_move_cursor() const { return false; }

        virtual bool supports_aggregate() const { return false; }
        virtual void aggregate_draw_bar(BarState *, bool, TimePoint) {}
        virtual void aggregate_draw_origin(
            const std::vector<LineType> &, const std::vector<std::string> &, BarState *, bool, TimePoint) {}
    };

    class TermDrawTargetBase : public ProgressDrawTarget {
    public:
        TermDrawTargetBase(std::shared_ptr<TermLike> term, std::optional<int> rate, bool stderr_flag);

        bool                         is_stderr() const override { return stderr_; }
        std::optional<std::uint16_t> width() const override;
        void                         adjust_last_line_count(LineAdjust adjust) override;
        void                         set_move_cursor(bool value) override;
        bool                         is_move_cursor() const override;
        std::unique_ptr<Drawable>    drawable(bool force, TimePoint now) override;

        std::shared_ptr<TermLike> term() const { return term_; }

    protected:
        mutable std::recursive_mutex mutex_;
        std::shared_ptr<TermLike>    term_;
        bool                         stderr_       = false;
        bool                         term_is_term_ = false;
        std::optional<RateLimiter>   limiter_;
        int                          last_line_count_ = 0;
        DrawState                    draw_state_;

        friend class TermDrawable;
        friend class AggregatedDrawable;
    };

    class TermDrawable : public Drawable {
    public:
        explicit TermDrawable(TermDrawTargetBase *target) : target_(target) {}

        std::unique_ptr<DrawStateWrapper> state() override;
        void                              draw() override;
        void                              clear() override;
        std::optional<std::uint16_t>      width() const override;

    private:
        TermDrawTargetBase *target_;
    };

    class TermDrawTarget : public TermDrawTargetBase {
    public:
        TermDrawTarget(std::shared_ptr<TermLike> term, int rate, bool stderr_flag)
            : TermDrawTargetBase(std::move(term), rate, stderr_flag) {}

        bool is_hidden() const override { return !term_is_term_; }
    };

    class TermLikeDrawTarget : public TermDrawTargetBase {
    public:
        TermLikeDrawTarget(std::shared_ptr<TermLike> term, std::optional<int> rate)
            : TermDrawTargetBase(std::move(term), rate, false) {}

        bool is_hidden() const override { return false; }
    };

    class HiddenDrawTarget : public ProgressDrawTarget {
    public:
        bool                         is_hidden() const override { return true; }
        std::optional<std::uint16_t> width() const override { return std::nullopt; }
        std::unique_ptr<Drawable>    drawable(bool, TimePoint) override { return nullptr; }
    };

    std::shared_ptr<ProgressDrawTarget>
    term_draw_target(std::shared_ptr<TermLike> term, int refresh_rate = 20, bool stderr_flag = false);
    std::shared_ptr<ProgressDrawTarget> term_like_draw_target(std::shared_ptr<TermLike> term,
                                                              std::optional<int>        refresh_rate = std::nullopt);
    std::shared_ptr<ProgressDrawTarget> stdout_draw_target();
    std::shared_ptr<ProgressDrawTarget> stderr_draw_target();
    std::shared_ptr<ProgressDrawTarget> stdout_draw_target_with_hz(int refresh_rate);
    std::shared_ptr<ProgressDrawTarget> stderr_draw_target_with_hz(int refresh_rate);
    std::shared_ptr<ProgressDrawTarget> term_like_draw_target_with_hz(std::shared_ptr<TermLike> term, int refresh_rate);
    std::shared_ptr<ProgressDrawTarget> hidden_draw_target();
    std::shared_ptr<ProgressDrawTarget> in_memory_draw_target(const std::shared_ptr<InMemoryTerm> &term);

}  // namespace mpb

#endif  // MPB_DRAW_TARGET_H
