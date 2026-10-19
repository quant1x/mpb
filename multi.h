// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_MULTI_H
#define MPB_MULTI_H 1

#include <cstdint>
#include <memory>
#include <mutex>
#include <optional>
#include <string>
#include <vector>

#include "draw_target.h"
#include "state.h"

namespace mpb {

    class ProgressBar;
    class MultiDrawTarget;
    class AggregatedDrawable;

    struct MultiStateMember {
        std::weak_ptr<BarState>    sb;
        std::shared_ptr<DrawState> draw_state;
        bool                       is_zombie = false;
    };

    class MultiState {
    public:
        explicit MultiState(std::shared_ptr<ProgressDrawTarget> target);

        std::unique_ptr<DrawStateWrapper> draw_state_for(std::uint64_t index);
        int                               member_visual_lines(std::uint64_t index, int width) const;
        void                              mark_zombie(std::uint64_t index);
        void                              draw(bool                            force,
                                               const std::vector<LineType>    &extra_lines,
                                               const std::vector<std::string> &extra_contents,
                                               TimePoint                       now);
        void                              println(std::string message, TimePoint now);
        void                              suspend(const std::function<void()> &func, TimePoint now);
        void                              clear(TimePoint now);

        std::optional<std::uint64_t> visual_index(std::uint64_t index) const;
        std::uint64_t                insert_with_pos(InsertLocation location, std::uint64_t pos);
        void                         remove_idx(std::uint64_t index);
        std::size_t                  len() const;
        std::optional<std::uint16_t> width() const;
        bool                         is_hidden() const;

        std::recursive_mutex             mutex;
        std::vector<MultiStateMember>    members;
        std::vector<std::uint64_t>       free_set;
        std::vector<std::uint64_t>       ordering;
        std::shared_ptr<MultiDrawTarget> draw_target;
        MultiProgressAlignment           alignment = MultiProgressAlignment::top;
        std::vector<std::string>         orphan_lines;
        int                              zombie_lines_count = 0;
    };

    class MultiDrawTarget : public ProgressDrawTarget {
    public:
        MultiDrawTarget(MultiState *multi, std::shared_ptr<ProgressDrawTarget> inner);

        bool                            is_hidden() const override { return inner_->is_hidden(); }
        bool                            is_stderr() const override { return inner_->is_stderr(); }
        std::optional<std::uint16_t>    width() const override { return inner_->width(); }
        std::optional<RemoteTargetInfo> remote() const override { return inner_->remote(); }
        void                            disconnect(TimePoint now) override { inner_->disconnect(now); }
        void                            mark_zombie() override { inner_->mark_zombie(); }
        void                            set_move_cursor(bool value) override { inner_->set_move_cursor(value); }
        bool                            is_move_cursor() const override { return inner_->is_move_cursor(); }
        void adjust_last_line_count(LineAdjust adjust) override { inner_->adjust_last_line_count(adjust); }
        std::unique_ptr<Drawable> drawable(bool force, TimePoint now) override;

        std::shared_ptr<ProgressDrawTarget> inner() const { return inner_; }
        MultiState                         *multi() const { return multi_; }

    private:
        MultiState                         *multi_;
        std::shared_ptr<ProgressDrawTarget> inner_;
    };

    class AggregatedDrawable : public Drawable {
    public:
        explicit AggregatedDrawable(MultiDrawTarget *target) : target_(target) {}

        std::unique_ptr<DrawStateWrapper> state() override;
        void                              draw() override;
        void                              clear() override;
        std::optional<std::uint16_t>      width() const override;

    private:
        MultiDrawTarget *target_;
        DrawState        state_;
    };

    class RemoteDrawTarget : public ProgressDrawTarget {
    public:
        RemoteDrawTarget(std::shared_ptr<MultiState> multi, std::uint64_t index);

        bool                            is_hidden() const override;
        bool                            is_stderr() const override;
        std::optional<std::uint16_t>    width() const override;
        std::optional<RemoteTargetInfo> remote() const override;
        void                            disconnect(TimePoint now) override;
        std::unique_ptr<Drawable>       drawable(bool, TimePoint) override { return nullptr; }
        void                            adjust_last_line_count(LineAdjust adjust) override;
        void                            mark_zombie() override;

        bool supports_aggregate() const override { return true; }
        void aggregate_draw_bar(BarState *bar, bool force, TimePoint now) override;
        void aggregate_draw_origin(const std::vector<LineType>    &lines,
                                   const std::vector<std::string> &contents,
                                   BarState                       *bar,
                                   bool                            force,
                                   TimePoint                       now) override;

    private:
        void aggregate_draw(BarState                       *bar,
                            bool                            force,
                            TimePoint                       now,
                            const std::vector<LineType>    &message_lines,
                            const std::vector<std::string> &message_contents);

        std::weak_ptr<MultiState> multi_;
        std::uint64_t             index_ = 0;
    };

    class MultiProgress {
    public:
        MultiProgress();
        explicit MultiProgress(std::shared_ptr<ProgressDrawTarget> target);

        static std::shared_ptr<MultiProgress> default_instance();
        static std::shared_ptr<MultiProgress> with_draw_target(std::shared_ptr<ProgressDrawTarget> target);

        std::shared_ptr<ProgressBar> add(std::shared_ptr<ProgressBar> pb);
        std::shared_ptr<ProgressBar> insert(std::uint64_t index, std::shared_ptr<ProgressBar> pb);
        std::shared_ptr<ProgressBar> insert_from_back(std::uint64_t index, std::shared_ptr<ProgressBar> pb);
        std::shared_ptr<ProgressBar> insert_before(std::shared_ptr<ProgressBar> before,
                                                   std::shared_ptr<ProgressBar> pb);
        std::shared_ptr<ProgressBar> insert_after(std::shared_ptr<ProgressBar> after, std::shared_ptr<ProgressBar> pb);
        void                         remove(const std::shared_ptr<ProgressBar> &pb);

        void println(std::string message);
        void suspend(const std::function<void()> &func);
        void clear();
        bool is_hidden() const;
        void set_draw_target(std::shared_ptr<ProgressDrawTarget> target);
        void set_move_cursor(bool value);
        void set_alignment(MultiProgressAlignment alignment);

        std::shared_ptr<MultiState> state;

    private:
        std::shared_ptr<ProgressBar>
        internalize(InsertLocation location, std::uint64_t pos, std::shared_ptr<ProgressBar> pb);
    };

}  // namespace mpb

#endif  // MPB_MULTI_H
