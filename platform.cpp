// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <thread>

#include "platform.h"

#if defined(_WIN32)
#include <windows.h>
#endif

namespace mpb {
    namespace {
#if defined(_WIN32)
        void enable_vt_for_std(DWORD std_handle) {
            const HANDLE handle = GetStdHandle(std_handle);
            DWORD        mode   = 0;
            if (handle == INVALID_HANDLE_VALUE || handle == nullptr) {
                return;
            }
            if (GetConsoleMode(handle, &mode) == 0) {
                return;
            }
            if ((mode & ENABLE_VIRTUAL_TERMINAL_PROCESSING) != 0) {
                return;
            }
            SetConsoleMode(handle, mode | ENABLE_VIRTUAL_TERMINAL_PROCESSING);
        }
#endif
    }  // namespace

    ConsoleGuard::ConsoleGuard() {
#if defined(_WIN32)
        previous_codepage_ = GetConsoleOutputCP();
        if (previous_codepage_ != 0 && previous_codepage_ != CP_UTF8) {
            SetConsoleOutputCP(CP_UTF8);
        }
        enable_vt_for_std(STD_OUTPUT_HANDLE);
        enable_vt_for_std(STD_ERROR_HANDLE);
#endif
    }

    ConsoleGuard::~ConsoleGuard() {
#if defined(_WIN32)
        if (previous_codepage_ != 0 && previous_codepage_ != CP_UTF8) {
            SetConsoleOutputCP(previous_codepage_);
        }
#endif
    }

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
