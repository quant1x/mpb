// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <thread>

#include "platform.h"

#if defined(_WIN32)
#include <windows.h>
#endif

namespace mpb {
    void sleep_for_microseconds(std::int64_t microseconds) {
        if (microseconds <= 0) {
            return;
        }
        // LARGE_INTEGER 以 100ns 为单位，限制 1 小时避免溢出。
        constexpr std::int64_t kMaxMicroseconds = 3600LL * 1000LL * 1000LL;
        if (microseconds > kMaxMicroseconds) {
            microseconds = kMaxMicroseconds;
        }

#if defined(_WIN32)
        // Windows 10 1803+；旧系统返回 nullptr，回退到 sleep_for。
        constexpr DWORD kHighResolutionTimer = 0x00000002;  // CREATE_WAITABLE_TIMER_HIGH_RESOLUTION
        HANDLE          timer = CreateWaitableTimerExW(nullptr, nullptr, kHighResolutionTimer, TIMER_ALL_ACCESS);
        if (timer != nullptr) {
            LARGE_INTEGER due{};
            // 负值表示相对时间，单位 100ns。
            due.QuadPart = -static_cast<LONGLONG>(microseconds * 10);
            if (SetWaitableTimer(timer, &due, 0, nullptr, nullptr, FALSE) != 0) {
                WaitForSingleObject(timer, INFINITE);
                CloseHandle(timer);
                return;
            }
            CloseHandle(timer);
        }
#endif

        std::this_thread::sleep_for(std::chrono::microseconds(microseconds));
    }
}  // namespace mpb
