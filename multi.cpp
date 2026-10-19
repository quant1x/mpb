// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <algorithm>
#include <cstddef>
#include <string>
#include <utility>

#include "multi.h"
#include "progress_bar.h"

namespace mpb {
    namespace {

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

    }  // namespace

    // ---------------------------------------------------------------------------
    // MultiState
    // ---------------------------------------------------------------------------

    MultiState::MultiState(std::shared_ptr<ProgressDrawTarget> target)
        : draw_target(std::make_shared<MultiDrawTarget>(this, std::move(target))) {
    }

    std::unique_ptr<DrawStateWrapper> MultiState::draw_state_for(std::uint64_t index) {
        if (index >= members.size())
            return nullptr;
        MultiStateMember &member = members[static_cast<std::size_t>(index)];
        if (!member.draw_state) {
            member.draw_state = std::make_shared<DrawState>();
        }
        return std::make_unique<DrawStateWrapper>(member.draw_state.get(), &orphan_lines);
    }

    int MultiState::member_visual_lines(std::uint64_t index, int width) const {
        if (index >= members.size())
            return 0;
        const MultiStateMember &member = members[static_cast<std::size_t>(index)];
        if (!member.draw_state)
            return 0;
        return visual_line_count(member.draw_state->lines, member.draw_state->contents, width);
    }

    void MultiState::mark_zombie(std::uint64_t index) {
        if (index >= members.size())
            return;

        if (ordering.empty() || index != ordering.front()) {
            members[static_cast<std::size_t>(index)].is_zombie = true;
            return;
        }

        int line_count = 0;
        if (const auto current_width = width(); current_width.has_value()) {
            line_count = member_visual_lines(index, *current_width);
        }
        zombie_lines_count += line_count;
        draw_target->adjust_last_line_count(LineAdjust{LineAdjustKind::keep, line_count});
        remove_idx(index);
    }

    void MultiState::draw(bool                            force,
                          const std::vector<LineType>    &extra_lines,
                          const std::vector<std::string> &extra_contents,
                          TimePoint                       now) {
        std::vector<std::uint64_t> reap_indices;
        int                        adjust = 0;

        for (std::uint64_t index : ordering) {
            if (index >= members.size())
                continue;
            const MultiStateMember &member = members[static_cast<std::size_t>(index)];
            if (!member.is_zombie)
                break;
            int line_count = 0;
            if (const auto current_width = width(); current_width.has_value()) {
                line_count = member_visual_lines(index, *current_width);
            }
            zombie_lines_count += line_count;
            adjust += line_count;
            reap_indices.push_back(index);
        }

        const bool extra = !extra_lines.empty();
        if (extra) {
            draw_target->adjust_last_line_count(LineAdjust{LineAdjustKind::clear, zombie_lines_count});
            zombie_lines_count = 0;
        }

        const auto current_width = width();
        if (!current_width.has_value())
            return;
        const int width_value = *current_width;

        std::vector<LineType> orphan_types;
        orphan_types.reserve(orphan_lines.size());
        for (const auto &line : orphan_lines) {
            orphan_types.push_back(line.empty() ? LineType::empty : LineType::text);
        }
        const int orphan_visual = visual_line_count(orphan_types, orphan_lines, width_value);
        force                   = force || orphan_visual > 0;

        auto drawable = draw_target->drawable(force, now);
        if (!drawable)
            return;
        auto       wrapper = drawable->state();
        DrawState &ds      = wrapper->state();
        ds.lines.clear();
        ds.contents.clear();
        ds.alignment   = alignment;
        ds.move_cursor = draw_target->is_move_cursor();

        if (extra) {
            for (std::size_t i = 0; i < extra_lines.size(); ++i) {
                ds.lines.push_back(extra_lines[i]);
                ds.contents.push_back(i < extra_contents.size() ? extra_contents[i] : std::string{});
            }
        }

        for (const auto &line : orphan_lines) {
            ds.lines.push_back(line.empty() ? LineType::empty : LineType::text);
            ds.contents.push_back(line);
        }
        orphan_lines.clear();

        for (std::uint64_t index : ordering) {
            if (index >= members.size())
                continue;
            const MultiStateMember &member = members[static_cast<std::size_t>(index)];
            if (!member.draw_state)
                continue;
            ds.lines.insert(ds.lines.end(), member.draw_state->lines.begin(), member.draw_state->lines.end());
            ds.contents.insert(
                ds.contents.end(), member.draw_state->contents.begin(), member.draw_state->contents.end());
        }

        drawable->draw();

        for (std::uint64_t index : reap_indices) {
            remove_idx(index);
        }
        if (!extra) {
            draw_target->adjust_last_line_count(LineAdjust{LineAdjustKind::keep, adjust});
        }
    }

    void MultiState::println(std::string message, TimePoint now) {
        std::vector<LineType>    lines;
        std::vector<std::string> contents;
        if (message.empty()) {
            lines.push_back(LineType::empty);
            contents.emplace_back();
        } else {
            for (auto &line : split_lines(message)) {
                lines.push_back(line.empty() ? LineType::empty : LineType::text);
                contents.push_back(std::move(line));
            }
        }
        draw(true, lines, contents, now);
    }

    void MultiState::suspend(const std::function<void()> &func, TimePoint now) {
        clear(now);
        func();
        draw(true, {}, {}, std::chrono::steady_clock::now());
    }

    void MultiState::clear(TimePoint now) {
        auto drawable = draw_target->drawable(true, now);
        if (!drawable)
            return;
        draw_target->adjust_last_line_count(LineAdjust{LineAdjustKind::clear, zombie_lines_count});
        zombie_lines_count = 0;
        drawable->clear();
    }

    std::optional<std::uint64_t> MultiState::visual_index(std::uint64_t index) const {
        for (std::size_t i = 0; i < ordering.size(); ++i) {
            if (ordering[i] == index)
                return static_cast<std::uint64_t>(i);
        }
        return std::nullopt;
    }

    std::uint64_t MultiState::insert_with_pos(InsertLocation location, std::uint64_t pos) {
        std::uint64_t index = 0;
        if (!free_set.empty()) {
            index = free_set.back();
            free_set.pop_back();
            members[static_cast<std::size_t>(index)] = MultiStateMember{};
        } else {
            members.emplace_back();
            index = static_cast<std::uint64_t>(members.size() - 1);
        }

        switch (location) {
            case InsertLocation::end:
                ordering.push_back(index);
                break;
            case InsertLocation::index: {
                const auto position = static_cast<std::size_t>(std::min<std::uint64_t>(pos, ordering.size()));
                ordering.insert(ordering.begin() + static_cast<std::ptrdiff_t>(position), index);
                break;
            }
            case InsertLocation::index_from_back: {
                const std::uint64_t length   = ordering.size();
                const auto          position = static_cast<std::size_t>(pos < length ? length - pos : 0);
                ordering.insert(ordering.begin() + static_cast<std::ptrdiff_t>(position), index);
                break;
            }
            case InsertLocation::after: {
                auto it = std::find(ordering.begin(), ordering.end(), pos);
                if (it == ordering.end()) {
                    ordering.push_back(index);
                } else {
                    ordering.insert(it + 1, index);
                }
                break;
            }
            case InsertLocation::before: {
                auto it = std::find(ordering.begin(), ordering.end(), pos);
                if (it == ordering.end()) {
                    ordering.push_back(index);
                } else {
                    ordering.insert(it, index);
                }
                break;
            }
        }
        return index;
    }

    void MultiState::remove_idx(std::uint64_t index) {
        if (std::find(free_set.begin(), free_set.end(), index) != free_set.end()) {
            return;
        }
        if (index >= members.size())
            return;
        members[static_cast<std::size_t>(index)] = MultiStateMember{};
        free_set.push_back(index);
        const auto it = std::find(ordering.begin(), ordering.end(), index);
        if (it != ordering.end())
            ordering.erase(it);
    }

    std::size_t MultiState::len() const {
        return members.size() - free_set.size();
    }

    std::optional<std::uint16_t> MultiState::width() const {
        return draw_target->width();
    }

    bool MultiState::is_hidden() const {
        return draw_target->is_hidden();
    }

    // ---------------------------------------------------------------------------
    // MultiDrawTarget / AggregatedDrawable
    // ---------------------------------------------------------------------------

    MultiDrawTarget::MultiDrawTarget(MultiState *multi, std::shared_ptr<ProgressDrawTarget> inner)
        : multi_(multi), inner_(std::move(inner)) {
    }

    std::unique_ptr<Drawable> MultiDrawTarget::drawable(bool force, TimePoint now) {
        if (!inner_->drawable(force, now))
            return nullptr;
        return std::make_unique<AggregatedDrawable>(this);
    }

    std::unique_ptr<DrawStateWrapper> AggregatedDrawable::state() {
        return std::make_unique<DrawStateWrapper>(&state_, nullptr);
    }

    std::optional<std::uint16_t> AggregatedDrawable::width() const {
        return target_->inner()->width();
    }

    void AggregatedDrawable::draw() {
        auto inner = target_->inner();
        if (auto *base = dynamic_cast<TermDrawTargetBase *>(inner.get())) {
            std::lock_guard<std::recursive_mutex> lock(base->mutex_);
            base->last_line_count_ = draw_to_term(state_, *base->term(), base->last_line_count_);
            return;
        }

        auto drawable = inner->drawable(false, std::chrono::steady_clock::now());
        if (drawable) {
            auto wrapper              = drawable->state();
            wrapper->state().lines    = state_.lines;
            wrapper->state().contents = state_.contents;
            wrapper->close();
            drawable->draw();
        }
    }

    void AggregatedDrawable::clear() {
        state_.lines.clear();
        state_.contents.clear();
        draw();
    }

    // ---------------------------------------------------------------------------
    // RemoteDrawTarget
    // ---------------------------------------------------------------------------

    RemoteDrawTarget::RemoteDrawTarget(std::shared_ptr<MultiState> multi, std::uint64_t index)
        : multi_(std::move(multi)), index_(index) {
    }

    bool RemoteDrawTarget::is_hidden() const {
        if (auto multi = multi_.lock())
            return multi->is_hidden();
        return true;
    }

    bool RemoteDrawTarget::is_stderr() const {
        if (auto multi = multi_.lock())
            return multi->draw_target->is_stderr();
        return false;
    }

    std::optional<std::uint16_t> RemoteDrawTarget::width() const {
        if (auto multi = multi_.lock())
            return multi->width();
        return std::nullopt;
    }

    std::optional<RemoteTargetInfo> RemoteDrawTarget::remote() const {
        return RemoteTargetInfo{multi_, index_};
    }

    void RemoteDrawTarget::disconnect(TimePoint now) {
        auto multi = multi_.lock();
        if (!multi)
            return;
        std::lock_guard<std::recursive_mutex> lock(multi->mutex);
        auto                                  wrapper = multi->draw_state_for(index_);
        if (!wrapper)
            return;
        wrapper->reset();
        multi->draw(true, {}, {}, now);
    }

    void RemoteDrawTarget::adjust_last_line_count(LineAdjust adjust) {
        if (auto multi = multi_.lock()) {
            multi->draw_target->adjust_last_line_count(adjust);
        }
    }

    void RemoteDrawTarget::mark_zombie() {
        if (auto multi = multi_.lock()) {
            std::lock_guard<std::recursive_mutex> lock(multi->mutex);
            multi->mark_zombie(index_);
        }
    }

    void RemoteDrawTarget::aggregate_draw_bar(BarState *bar, bool force, TimePoint now) {
        aggregate_draw(bar, force, now, {}, {});
    }

    void RemoteDrawTarget::aggregate_draw_origin(const std::vector<LineType>    &lines,
                                                 const std::vector<std::string> &contents,
                                                 BarState                       *bar,
                                                 bool                            force,
                                                 TimePoint                       now) {
        aggregate_draw(bar, force, now, lines, contents);
    }

    void RemoteDrawTarget::aggregate_draw(BarState                       *bar,
                                          bool                            force,
                                          TimePoint                       now,
                                          const std::vector<LineType>    &message_lines,
                                          const std::vector<std::string> &message_contents) {
        auto multi = multi_.lock();
        if (!multi)
            return;
        std::lock_guard<std::recursive_mutex> lock(multi->mutex);
        auto                                  wrapper = multi->draw_state_for(index_);
        if (!wrapper)
            return;
        DrawState &ds = wrapper->state();
        ds.lines.clear();
        ds.contents.clear();

        for (std::size_t i = 0; i < message_lines.size(); ++i) {
            ds.lines.push_back(message_lines[i]);
            ds.contents.push_back(i < message_contents.size() ? message_contents[i] : std::string{});
        }

        if (const auto width = multi->width();
            width.has_value() && bar->progress_state().status().kind != StatusKind::done_hidden) {
            auto [lines, contents] = bar->style_ref().format_state(
                bar->progress_state(), *width, multi->draw_target->is_stderr());
            ds.lines.insert(ds.lines.end(), lines.begin(), lines.end());
            ds.contents.insert(ds.contents.end(), contents.begin(), contents.end());
        }

        wrapper->close();
        multi->draw(force, {}, {}, now);
    }

    // ---------------------------------------------------------------------------
    // MultiProgress
    // ---------------------------------------------------------------------------

    MultiProgress::MultiProgress() : MultiProgress(stderr_draw_target()) {
    }

    MultiProgress::MultiProgress(std::shared_ptr<ProgressDrawTarget> target)
        : state(std::make_shared<MultiState>(std::move(target))) {
    }

    std::shared_ptr<MultiProgress> MultiProgress::default_instance() {
        return std::make_shared<MultiProgress>();
    }

    std::shared_ptr<MultiProgress> MultiProgress::with_draw_target(std::shared_ptr<ProgressDrawTarget> target) {
        return std::make_shared<MultiProgress>(std::move(target));
    }

    std::shared_ptr<ProgressBar> MultiProgress::add(std::shared_ptr<ProgressBar> pb) {
        return internalize(InsertLocation::end, 0, std::move(pb));
    }

    std::shared_ptr<ProgressBar> MultiProgress::insert(std::uint64_t index, std::shared_ptr<ProgressBar> pb) {
        return internalize(InsertLocation::index, index, std::move(pb));
    }

    std::shared_ptr<ProgressBar> MultiProgress::insert_from_back(std::uint64_t index, std::shared_ptr<ProgressBar> pb) {
        return internalize(InsertLocation::index_from_back, index, std::move(pb));
    }

    std::shared_ptr<ProgressBar> MultiProgress::insert_before(std::shared_ptr<ProgressBar> before,
                                                              std::shared_ptr<ProgressBar> pb) {
        if (const auto index = before->index(); index.has_value()) {
            return internalize(InsertLocation::before, *index, std::move(pb));
        }
        return add(std::move(pb));
    }

    std::shared_ptr<ProgressBar> MultiProgress::insert_after(std::shared_ptr<ProgressBar> after,
                                                             std::shared_ptr<ProgressBar> pb) {
        if (const auto index = after->index(); index.has_value()) {
            return internalize(InsertLocation::after, *index, std::move(pb));
        }
        return add(std::move(pb));
    }

    std::shared_ptr<ProgressBar>
    MultiProgress::internalize(InsertLocation location, std::uint64_t pos, std::shared_ptr<ProgressBar> pb) {
        auto          bar_state = pb->state();
        std::uint64_t index     = 0;
        {
            std::lock_guard<std::recursive_mutex> lock(state->mutex);
            index = state->insert_with_pos(location, pos);
            if (index < state->members.size()) {
                state->members[static_cast<std::size_t>(index)].sb = bar_state;
            }
        }
        pb->set_draw_target(std::make_shared<RemoteDrawTarget>(state, index));
        return pb;
    }

    void MultiProgress::remove(const std::shared_ptr<ProgressBar> &pb) {
        auto       bar_state = pb->state();
        const auto remote    = bar_state->remote_target();
        if (!remote.has_value())
            return;
        auto multi = remote->multi.lock();
        if (!multi || multi.get() != state.get())
            return;

        bar_state->set_draw_target_no_disconnect(hidden_draw_target());
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->remove_idx(remote->index);
    }

    void MultiProgress::println(std::string message) {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->println(std::move(message), std::chrono::steady_clock::now());
    }

    void MultiProgress::suspend(const std::function<void()> &func) {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->suspend(func, std::chrono::steady_clock::now());
    }

    void MultiProgress::clear() {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->clear(std::chrono::steady_clock::now());
    }

    bool MultiProgress::is_hidden() const {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        return state->is_hidden();
    }

    void MultiProgress::set_draw_target(std::shared_ptr<ProgressDrawTarget> target) {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->draw_target->disconnect(std::chrono::steady_clock::now());
        state->draw_target = std::make_shared<MultiDrawTarget>(state.get(), std::move(target));
    }

    void MultiProgress::set_move_cursor(bool value) {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->draw_target->set_move_cursor(value);
    }

    void MultiProgress::set_alignment(MultiProgressAlignment alignment) {
        std::lock_guard<std::recursive_mutex> lock(state->mutex);
        state->alignment = alignment;
    }

}  // namespace mpb
