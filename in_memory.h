// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_IN_MEMORY_H
#define MPB_IN_MEMORY_H 1

#include <cstdint>
#include <mutex>
#include <string>
#include <string_view>
#include <vector>

#include "term_like.h"

namespace mpb {

struct Move {
    enum class Kind { up, down, left, right, str, newline, clear, flush };
    Kind kind = Kind::str;
    int n = 0;
    std::string s;
};

class InMemoryTerm final : public TermLike {
public:
    InMemoryTerm(int rows, int cols);

    void reset();

    std::string contents() const;
    std::string contents_formatted() const;
    std::string moves_since_last_check();

    std::uint16_t width() const override;
    std::uint16_t height() const override;
    void move_cursor_up(int n) override;
    void move_cursor_down(int n) override;
    void move_cursor_right(int n) override;
    void move_cursor_left(int n) override;
    void write_line(std::string_view text) override;
    void write_str(std::string_view text) override;
    void clear_line() override;
    void flush() override;
    bool is_term() const override { return false; }

private:
    enum class ParserMode { normal, esc, esc_charset, csi, osc, osc_esc };

    void feed_text(std::string_view text);
    void feed_cp(char32_t cp);
    void dispatch_csi(const std::string& params, char32_t final);
    void write_cp(char32_t cp);
    void clear_from_cursor();
    void move_up_locked(int n);
    void move_down_locked(int n);
    void move_right_locked(int n);
    void move_left_locked(int n);
    std::string row_text(int row) const;

    mutable std::mutex mutex_;
    int width_ = 0;
    int height_ = 0;
    int cursor_row_ = 0;
    int cursor_col_ = 0;
    std::vector<std::u32string> buffer_;
    std::vector<Move> history_;
    ParserMode mode_ = ParserMode::normal;
    std::string csi_params_;
};

} // namespace mpb

#endif // MPB_IN_MEMORY_H
