// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include <cstdlib>
#include <iostream>
#include <stdexcept>
#include <string>

#include "term_like.h"

#if defined(_WIN32)
#include <io.h>
#include <windows.h>
#else
#include <sys/ioctl.h>
#include <unistd.h>
#endif

namespace mpb {
    namespace {

#if defined(_WIN32)
        bool fd_is_tty(int fd) {
            return fd >= 0 && _isatty(fd) != 0;
        }

        HANDLE console_handle(int fd) {
            if (fd != 1 && fd != 2)
                return nullptr;
            const DWORD  handle_id = fd == 2 ? STD_ERROR_HANDLE : STD_OUTPUT_HANDLE;
            const HANDLE handle    = GetStdHandle(handle_id);
            if (handle == INVALID_HANDLE_VALUE || handle == nullptr) {
                return nullptr;
            }
            return handle;
        }

        // Windows 控制台默认按 OEM 代码页（中文系统是 GBK）解释 WriteFile 的字节，
        // UTF-8 的 █░ / ✔ / emoji 会变成乱码，宽度计算的补白随之错位。控制台句柄
        // 改走 WriteConsoleW（先转 UTF-16），与 Rust std 的处理一致。
        bool write_console_utf8(int fd, std::string_view text) {
            const HANDLE handle = console_handle(fd);
            if (handle == nullptr || text.empty()) {
                return false;
            }
            const int length = MultiByteToWideChar(CP_UTF8, 0, text.data(), static_cast<int>(text.size()), nullptr, 0);
            if (length <= 0) {
                return false;
            }
            std::wstring wide(static_cast<std::size_t>(length), L'\0');
            MultiByteToWideChar(CP_UTF8, 0, text.data(), static_cast<int>(text.size()), wide.data(), length);
            DWORD written = 0;
            return WriteConsoleW(handle, wide.data(), static_cast<DWORD>(wide.size()), &written, nullptr) != 0;
        }

        // 控制台还需要打开 VT 处理，否则 \x1b[2K / \x1b[nA 会被当成普通文本打印。
        void enable_virtual_terminal(int fd) {
            const HANDLE handle = console_handle(fd);
            DWORD        mode   = 0;
            if (handle != nullptr && GetConsoleMode(handle, &mode) != 0) {
                SetConsoleMode(handle, mode | ENABLE_VIRTUAL_TERMINAL_PROCESSING);
            }
        }

        std::optional<std::pair<int, int>> query_size(int fd) {
            const HANDLE handle = console_handle(fd);
            if (handle == nullptr) {
                return std::nullopt;
            }
            CONSOLE_SCREEN_BUFFER_INFO info{};
            if (!GetConsoleScreenBufferInfo(handle, &info)) {
                return std::nullopt;
            }
            const int width  = info.srWindow.Right - info.srWindow.Left + 1;
            const int height = info.srWindow.Bottom - info.srWindow.Top + 1;
            if (width <= 0 || height <= 0) {
                return std::nullopt;
            }
            return std::make_pair(width, height);
        }
#else
        bool fd_is_tty(int fd) {
            return fd >= 0 && isatty(fd) != 0;
        }

        std::optional<std::pair<int, int>> query_size(int fd) {
            if (fd < 0)
                return std::nullopt;
            struct winsize size{};
            if (ioctl(fd, TIOCGWINSZ, &size) != 0 || size.ws_col == 0 || size.ws_row == 0) {
                return std::nullopt;
            }
            return std::make_pair(static_cast<int>(size.ws_col), static_cast<int>(size.ws_row));
        }
#endif

        int detect_fd(std::ostream &out) {
            if (&out == &std::cout)
                return 1;
            if (&out == &std::cerr || &out == &std::clog)
                return 2;
            return -1;
        }

    }  // namespace

    StdTerm::StdTerm(std::ostream &out, int fd, bool buffered, bool is_term)
        : out_(out), fd_(fd), buffered_(buffered), is_term_(is_term) {
#if defined(_WIN32)
        if (is_term_) {
            enable_virtual_terminal(fd_);
        }
#endif
    }

    std::optional<std::pair<int, int>> StdTerm::size_checked() const {
        return query_size(fd_);
    }

    void StdTerm::write_raw(std::string_view text) {
#if defined(_WIN32)
        if (is_term_ && fd_ > 0 && write_console_utf8(fd_, text)) {
            return;
        }
#endif
        out_.write(text.data(), static_cast<std::streamsize>(text.size()));
        if (!out_) {
            throw std::runtime_error("StdTerm: write failed");
        }
    }

    std::uint16_t StdTerm::width() const {
        if (const auto size = size_checked(); size.has_value()) {
            return static_cast<std::uint16_t>(size->first);
        }
        return fallback_width_;
    }

    std::uint16_t StdTerm::height() const {
        if (const auto size = size_checked(); size.has_value()) {
            return static_cast<std::uint16_t>(size->second);
        }
        return fallback_height_;
    }

    void StdTerm::write_escape(std::string_view text) {
        if (!buffered_) {
            write_raw(text);
            return;
        }
        std::lock_guard<std::mutex> lock(mutex_);
        buffer_.append(text);
    }

    void StdTerm::move_cursor_up(int n) {
        if (n <= 0)
            return;
        write_escape("\x1b[" + std::to_string(n) + "A");
    }

    void StdTerm::move_cursor_down(int n) {
        if (n <= 0)
            return;
        write_escape("\x1b[" + std::to_string(n) + "B");
    }

    void StdTerm::move_cursor_right(int n) {
        if (n <= 0)
            return;
        write_escape("\x1b[" + std::to_string(n) + "C");
    }

    void StdTerm::move_cursor_left(int n) {
        if (n <= 0)
            return;
        write_escape("\x1b[" + std::to_string(n) + "D");
    }

    void StdTerm::write_line(std::string_view text) {
        write_str(text);
        write_str("\n");
    }

    void StdTerm::write_str(std::string_view text) {
        if (!buffered_) {
            write_raw(text);
            return;
        }
        std::lock_guard<std::mutex> lock(mutex_);
        buffer_.append(text);
    }

    void StdTerm::clear_line() {
        write_escape("\r\x1b[2K");
    }

    void StdTerm::flush() {
        if (buffered_) {
            std::lock_guard<std::mutex> lock(mutex_);
            if (!buffer_.empty()) {
                write_raw(buffer_);
                buffer_.clear();
            }
        }
        out_.flush();
        if (!out_) {
            throw std::runtime_error("StdTerm: flush failed");
        }
    }

    std::shared_ptr<StdTerm> stdout_term() {
        const bool tty = fd_is_tty(1) && query_size(1).has_value();
        return std::make_shared<StdTerm>(std::cout, 1, true, tty);
    }

    std::shared_ptr<StdTerm> stderr_term() {
        const bool tty = fd_is_tty(2) && query_size(2).has_value();
        return std::make_shared<StdTerm>(std::cerr, 2, true, tty);
    }

    std::shared_ptr<StdTerm> std_term_with_writer(std::ostream &out, bool buffered) {
        const int  fd  = detect_fd(out);
        const bool tty = fd >= 0 && fd_is_tty(fd) && query_size(fd).has_value();
        return std::make_shared<StdTerm>(out, fd, buffered, tty);
    }

    bool is_dumb() {
        if (const char *term = std::getenv("TERM"); term != nullptr) {
            return std::string(term) == "dumb";
        }
#if defined(_WIN32)
        return false;
#else
        return true;
#endif
    }

}  // namespace mpb
