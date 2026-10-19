// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_PLATFORM_H
#define MPB_PLATFORM_H 1

#include <chrono>
#include <cstdint>

namespace mpb {

    // 短休眠（微秒）。
    //
    // Windows 的 std::this_thread::sleep_for 受系统默认 15.6ms 定时器粒度支配：
    // sleep_for(5ms) 实测接近 16ms，与 Rust/Zig 的行为不一致（示例整体慢约 3 倍）。
    // 这里改用 CREATE_WAITABLE_TIMER_HIGH_RESOLUTION 可等待定时器（Windows 10
    // 1803+），失败时回退到 sleep_for；做法与 mopr::base::sleep_for_microseconds
    // 相同。macOS/POSIX 的 nanosleep 本身就是微秒级，直接走 sleep_for。
    void sleep_for_microseconds(std::int64_t microseconds);

    template <typename Rep, typename Period>
    void sleep_for(std::chrono::duration<Rep, Period> duration) {
        sleep_for_microseconds(std::chrono::duration_cast<std::chrono::microseconds>(duration).count());
    }

    // 把 Windows 控制台输出代码页切到 UTF-8，并为 stdout/stderr 打开 VT 处理，
    // 析构时恢复原代码页。给示例里的 std::cout 用：进度条自身已经走
    // WriteConsoleW，不依赖代码页，但 `std::cout << "✔"` 仍然经过 WriteFile。
    class ConsoleGuard {
    public:
        ConsoleGuard();
        ~ConsoleGuard();
        ConsoleGuard(const ConsoleGuard &)            = delete;
        ConsoleGuard &operator=(const ConsoleGuard &) = delete;

    private:
        unsigned int previous_codepage_ = 0;
    };

}  // namespace mpb

#endif  // MPB_PLATFORM_H
