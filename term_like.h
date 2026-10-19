// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_TERM_LIKE_H
#define MPB_TERM_LIKE_H 1

#include <cstdint>
#include <memory>
#include <mutex>
#include <optional>
#include <ostream>
#include <string>
#include <string_view>
#include <utility>

namespace mpb {

class TermLike {
public:
    virtual ~TermLike() = default;

    virtual std::uint16_t width() const = 0;
    virtual std::uint16_t height() const = 0;
    virtual void move_cursor_up(int n) = 0;
    virtual void move_cursor_down(int n) = 0;
    virtual void move_cursor_right(int n) = 0;
    virtual void move_cursor_left(int n) = 0;
    virtual void write_line(std::string_view text) = 0;
    virtual void write_str(std::string_view text) = 0;
    virtual void clear_line() = 0;
    virtual void flush() = 0;
    virtual bool is_term() const { return false; }
};

class StdTerm final : public TermLike {
public:
    StdTerm(std::ostream& out, int fd, bool buffered, bool is_term);
    ~StdTerm() override = default;

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
    bool is_term() const override { return is_term_; }

private:
    void write_escape(std::string_view text);
    std::optional<std::pair<int, int>> size_checked() const;

    std::ostream& out_;
    int fd_ = -1;
    bool buffered_ = false;
    bool is_term_ = false;
    std::uint16_t fallback_width_ = 80;
    std::uint16_t fallback_height_ = 20;
    mutable std::mutex mutex_;
    std::string buffer_;
};

std::shared_ptr<StdTerm> stdout_term();
std::shared_ptr<StdTerm> stderr_term();
std::shared_ptr<StdTerm> std_term_with_writer(std::ostream& out,
                                              bool buffered = false);

bool is_dumb();

} // namespace mpb

#endif // MPB_TERM_LIKE_H
