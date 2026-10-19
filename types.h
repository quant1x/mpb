// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_TYPES_H
#define MPB_TYPES_H 1

#include <chrono>
#include <cstdint>
#include <optional>
#include <string>

namespace mpb {

    using TimePoint = std::chrono::steady_clock::time_point;
    using Duration  = std::chrono::nanoseconds;

    constexpr std::uint64_t kU64Max = ~std::uint64_t{0};

    enum class LineType { text, bar, empty };

    enum class LineAdjustKind { clear, keep };

    struct LineAdjust {
        LineAdjustKind kind  = LineAdjustKind::clear;
        int            count = 0;
    };

    enum class MultiProgressAlignment { top, bottom };

    enum class StatusKind { in_progress, done_visible, done_hidden };

    struct Status {
        StatusKind kind = StatusKind::in_progress;
        Duration   duration{0};

        bool is_finished() const noexcept { return kind != StatusKind::in_progress; }
    };

    enum class ResetMode { eta, elapsed, all };

    enum class FinishKind {
        and_leave,
        with_message,
        and_clear,
        abandon,
        abandon_with_message,
    };

    struct ProgressFinish {
        FinishKind  kind = FinishKind::and_clear;
        std::string message;

        static ProgressFinish default_finish() { return ProgressFinish{FinishKind::and_clear, {}}; }
    };

    enum class InsertLocation {
        end,
        index,
        index_from_back,
        after,
        before,
    };

    inline float f32(float value) noexcept {
        return value;
    }

}  // namespace mpb

#endif  // MPB_TYPES_H
