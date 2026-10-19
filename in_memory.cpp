// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include "in_memory.h"

#include <algorithm>
#include <sstream>
#include <stdexcept>
#include <string>
#include <utility>

#include "utf8.h"

namespace mpb {
namespace {

int parse_param(const std::string& raw, std::size_t index, int fallback) {
    std::size_t current = 0;
    std::size_t start = 0;
    while (start <= raw.size()) {
        const auto semi = raw.find(';', start);
        const std::string piece =
            raw.substr(start, semi == std::string::npos ? std::string::npos : semi - start);
        if (current == index) {
            if (piece.empty()) return fallback;
            try {
                const int value = std::stoi(piece);
                return value == 0 ? fallback : value;
            } catch (...) {
                return fallback;
            }
        }
        ++current;
        if (semi == std::string::npos) break;
        start = semi + 1;
    }
    return fallback;
}

std::string debug_escape(std::string_view text) {
    std::string out;
    for (unsigned char c : text) {
        switch (c) {
        case '"': out += "\\\""; break;
        case '\\': out += "\\\\"; break;
        case '\n': out += "\\n"; break;
        case '\r': out += "\\r"; break;
        case '\t': out += "\\t"; break;
        default:
            if (c < 0x20 || c == 0x7F) {
                std::ostringstream encoded;
                encoded << "\\u{" << std::hex << static_cast<int>(c) << '}';
                out += encoded.str();
            } else {
                out.push_back(static_cast<char>(c));
            }
            break;
        }
    }
    return out;
}

} // namespace

InMemoryTerm::InMemoryTerm(int rows, int cols) : width_(cols), height_(rows) {
    if (rows <= 0 || cols <= 0) {
        throw std::invalid_argument("rows and cols must be > 0");
    }
    reset();
}

void InMemoryTerm::reset() {
    std::lock_guard<std::mutex> lock(mutex_);
    cursor_row_ = 0;
    cursor_col_ = 0;
    history_.clear();
    mode_ = ParserMode::normal;
    csi_params_.clear();
    buffer_.assign(static_cast<std::size_t>(height_),
                   std::u32string(static_cast<std::size_t>(width_), U' '));
}

std::string InMemoryTerm::row_text(int row) const {
    const std::u32string& text = buffer_[static_cast<std::size_t>(row)];
    std::size_t end = text.size();
    while (end > 0 && text[end - 1] == U' ') {
        --end;
    }
    return utf8_encode(std::u32string_view(text.data(), end));
}

std::string InMemoryTerm::contents() const {
    std::lock_guard<std::mutex> lock(mutex_);
    std::vector<std::string> lines;
    lines.reserve(static_cast<std::size_t>(height_));
    for (int row = 0; row < height_; ++row) {
        lines.push_back(row_text(row));
    }
    std::size_t end = lines.size();
    while (end > 0 && lines[end - 1].empty()) {
        --end;
    }
    std::string out;
    for (std::size_t i = 0; i < end; ++i) {
        if (i != 0) out.push_back('\n');
        out += lines[i];
    }
    return out;
}

std::string InMemoryTerm::contents_formatted() const {
    std::lock_guard<std::mutex> lock(mutex_);
    std::vector<std::string> lines;
    lines.reserve(static_cast<std::size_t>(height_));
    for (int row = 0; row < height_; ++row) {
        lines.push_back(row_text(row));
    }
    std::size_t end = lines.size();
    while (end > 0 && lines[end - 1].empty()) {
        --end;
    }
    std::string out;
    for (std::size_t i = 0; i < end; ++i) {
        out += lines[i];
        out += "\x1b[m\n";
    }
    if (!out.empty()) {
        out.pop_back();
    }
    return out;
}

std::string InMemoryTerm::moves_since_last_check() {
    std::vector<Move> history;
    {
        std::lock_guard<std::mutex> lock(mutex_);
        history.swap(history_);
    }
    std::ostringstream out;
    for (const auto& move : history) {
        switch (move.kind) {
        case Move::Kind::up: out << "Up(" << move.n << ")\n"; break;
        case Move::Kind::down: out << "Down(" << move.n << ")\n"; break;
        case Move::Kind::left: out << "Left(" << move.n << ")\n"; break;
        case Move::Kind::right: out << "Right(" << move.n << ")\n"; break;
        case Move::Kind::str: out << "Str(\"" << debug_escape(move.s) << "\")\n"; break;
        case Move::Kind::newline: out << "NewLine\n"; break;
        case Move::Kind::clear: out << "Clear\n"; break;
        case Move::Kind::flush: out << "Flush\n"; break;
        }
    }
    return out.str();
}

std::uint16_t InMemoryTerm::width() const {
    std::lock_guard<std::mutex> lock(mutex_);
    return static_cast<std::uint16_t>(width_);
}

std::uint16_t InMemoryTerm::height() const {
    std::lock_guard<std::mutex> lock(mutex_);
    return static_cast<std::uint16_t>(height_);
}

void InMemoryTerm::move_up_locked(int n) {
    cursor_row_ = n > cursor_row_ ? 0 : cursor_row_ - n;
}

void InMemoryTerm::move_down_locked(int n) {
    cursor_row_ += n;
    if (cursor_row_ >= height_) cursor_row_ = height_ - 1;
}

void InMemoryTerm::move_right_locked(int n) {
    cursor_col_ += n;
    if (cursor_col_ > width_) cursor_col_ = width_;
}

void InMemoryTerm::move_left_locked(int n) {
    cursor_col_ = n > cursor_col_ ? 0 : cursor_col_ - n;
}

void InMemoryTerm::move_cursor_up(int n) {
    if (n == 0) return;
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::up, n, {}});
    move_up_locked(n);
}

void InMemoryTerm::move_cursor_down(int n) {
    if (n == 0) return;
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::down, n, {}});
    move_down_locked(n);
}

void InMemoryTerm::move_cursor_right(int n) {
    if (n == 0) return;
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::right, n, {}});
    move_right_locked(n);
}

void InMemoryTerm::move_cursor_left(int n) {
    if (n == 0) return;
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::left, n, {}});
    move_left_locked(n);
}

void InMemoryTerm::write_line(std::string_view text) {
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::str, 0, std::string(text)});
    history_.push_back(Move{Move::Kind::newline, 0, {}});
    feed_text(text);
    feed_text("\r\n");
}

void InMemoryTerm::write_str(std::string_view text) {
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::str, 0, std::string(text)});
    feed_text(text);
}

void InMemoryTerm::clear_line() {
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::clear, 0, {}});
    feed_text("\r\x1b[2K");
}

void InMemoryTerm::flush() {
    std::lock_guard<std::mutex> lock(mutex_);
    history_.push_back(Move{Move::Kind::flush, 0, {}});
}

void InMemoryTerm::feed_text(std::string_view text) {
    for (const char32_t cp : utf8_decode(text)) {
        feed_cp(cp);
    }
}

void InMemoryTerm::feed_cp(char32_t cp) {
    switch (mode_) {
    case ParserMode::normal:
        if (cp == 0x1B) {
            mode_ = ParserMode::esc;
        } else if (cp == 0x9B) {
            mode_ = ParserMode::csi;
            csi_params_.clear();
        } else if (cp == U'\r') {
            cursor_col_ = 0;
        } else if (cp == U'\n') {
            cursor_col_ = 0;
            ++cursor_row_;
            if (cursor_row_ >= height_) cursor_row_ = height_ - 1;
        } else if (cp == U'\b') {
            if (cursor_col_ > 0) --cursor_col_;
        } else if (cp == U'\t') {
            write_cp(U' ');
        } else if (cp >= 0x20 && cp != 0x7F) {
            write_cp(cp);
        }
        break;

    case ParserMode::esc:
        if (cp == U'[') {
            mode_ = ParserMode::csi;
            csi_params_.clear();
        } else if (cp == U']') {
            mode_ = ParserMode::osc;
        } else if (cp == U'(' || cp == U')') {
            mode_ = ParserMode::esc_charset;
        } else {
            mode_ = ParserMode::normal;
        }
        break;

    case ParserMode::esc_charset:
        mode_ = ParserMode::normal;
        break;

    case ParserMode::csi:
        if (cp >= 0x40 && cp <= 0x7E) {
            dispatch_csi(csi_params_, cp);
            mode_ = ParserMode::normal;
            csi_params_.clear();
        } else {
            csi_params_ += utf8_encode(cp);
        }
        break;

    case ParserMode::osc:
        if (cp == 0x07) {
            mode_ = ParserMode::normal;
        } else if (cp == 0x1B) {
            mode_ = ParserMode::osc_esc;
        }
        break;

    case ParserMode::osc_esc:
        mode_ = ParserMode::normal;
        break;
    }
}

void InMemoryTerm::dispatch_csi(const std::string& params, char32_t final) {
    switch (final) {
    case U'A':
        move_up_locked(parse_param(params, 0, 1));
        break;
    case U'B':
        move_down_locked(parse_param(params, 0, 1));
        break;
    case U'C':
        move_right_locked(parse_param(params, 0, 1));
        break;
    case U'D':
        move_left_locked(parse_param(params, 0, 1));
        break;
    case U'H':
    case U'f': {
        int row = parse_param(params, 0, 1) - 1;
        int col = parse_param(params, 1, 1) - 1;
        row = std::clamp(row, 0, height_ - 1);
        col = std::clamp(col, 0, width_);
        cursor_row_ = row;
        cursor_col_ = col;
        break;
    }
    case U'K': {
        std::u32string& row = buffer_[static_cast<std::size_t>(cursor_row_)];
        std::fill(row.begin(), row.end(), U' ');
        break;
    }
    case U'J':
        clear_from_cursor();
        break;
    default:
        break;
    }
}

void InMemoryTerm::clear_from_cursor() {
    std::u32string& row = buffer_[static_cast<std::size_t>(cursor_row_)];
    for (int col = cursor_col_; col < width_; ++col) {
        row[static_cast<std::size_t>(col)] = U' ';
    }
    for (int r = cursor_row_ + 1; r < height_; ++r) {
        std::fill(buffer_[static_cast<std::size_t>(r)].begin(),
                  buffer_[static_cast<std::size_t>(r)].end(), U' ');
    }
}

void InMemoryTerm::write_cp(char32_t cp) {
    if (cursor_col_ >= width_) {
        ++cursor_row_;
        cursor_col_ = 0;
        if (cursor_row_ >= height_) cursor_row_ = height_ - 1;
    }
    buffer_[static_cast<std::size_t>(cursor_row_)]
           [static_cast<std::size_t>(cursor_col_)] = cp;
    ++cursor_col_;
}

} // namespace mpb
