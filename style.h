// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_STYLE_H
#define MPB_STYLE_H 1

#include <cstdint>
#include <functional>
#include <map>
#include <memory>
#include <optional>
#include <stdexcept>
#include <string>
#include <string_view>
#include <vector>

#include "format.h"
#include "types.h"

namespace mpb {

class ProgressState;

// ---------------------------------------------------------------------------
// Width / ANSI
// ---------------------------------------------------------------------------

struct AnsiSegment {
    std::string text;
    bool is_ansi = false;
};

std::vector<AnsiSegment> ansi_segments(std::string_view text);
std::string ansi_slice(std::string_view text, int start, int end);
int char_width(char32_t cp, bool cjk = false);
int str_width(std::string_view text, bool cjk = false);
int measure_text_width(std::string_view text);
std::string pad_and_truncate(std::string_view text, int width, char align,
                             bool truncate);

class Width {
public:
    static void set_default_cjk(bool is_cjk);
    static int char_width(char32_t cp);
    static int str_width(std::string_view text);
    static int ansi_str(std::string_view text);
};

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

struct Style {
    std::string spec;
    std::string spec_err;

    std::string apply_for(std::string_view text, bool for_stderr = false) const;
};

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

struct TemplatePart {
    bool is_placeholder = false;
    std::string text;
    std::string key;
    char align = '<';
    int width = 0;
    bool truncate = false;
    std::optional<Style> style;
    std::optional<Style> alt_style;
};

class TemplateError : public std::runtime_error {
public:
    TemplateError(std::string state, char32_t next);

    const std::string& state() const noexcept { return state_; }
    char32_t next() const noexcept { return next_; }

private:
    std::string state_;
    char32_t next_;
};

struct Template {
    std::vector<TemplatePart> parts;
    std::vector<std::string> original_texts;
    int current_tab_width = 0;

    static Template from_str(std::string_view text);
    static Template from_str_with_tab_width(std::string_view text, int tab_width);

    void set_tab_width(int new_tab_width);
    Template clone() const { return *this; }
};

Template template_from_str(std::string_view text);
Template template_from_str_with_tab_width(std::string_view text, int tab_width);

// ---------------------------------------------------------------------------
// Trackers
// ---------------------------------------------------------------------------

class TabWriter {
public:
    explicit TabWriter(int tab_width);

    void write_str(std::string_view text);
    const std::string& value() const noexcept { return value_; }

private:
    int tab_width_;
    std::string value_;
};

class ProgressTracker {
public:
    virtual ~ProgressTracker() = default;

    virtual std::shared_ptr<ProgressTracker> clone() const = 0;
    virtual void tick(const ProgressState&, TimePoint) {}
    virtual void reset(const ProgressState&, TimePoint) {}
    virtual void write(const ProgressState& state, TabWriter& writer) const = 0;
};

class FuncTracker : public ProgressTracker {
public:
    using Func = std::function<void(const ProgressState&, TabWriter&)>;

    explicit FuncTracker(Func func) : func_(std::move(func)) {}

    std::shared_ptr<ProgressTracker> clone() const override;
    void write(const ProgressState& state, TabWriter& writer) const override;

private:
    Func func_;
};

// ---------------------------------------------------------------------------
// ProgressStyle
// ---------------------------------------------------------------------------

class ProgressStyle {
public:
    ProgressStyle();
    explicit ProgressStyle(const Template& tmpl);
    explicit ProgressStyle(std::string_view template_text);

    ProgressStyle(const ProgressStyle& other);
    ProgressStyle(ProgressStyle&& other) noexcept = default;
    ProgressStyle& operator=(const ProgressStyle& other);
    ProgressStyle& operator=(ProgressStyle&& other) noexcept = default;
    ~ProgressStyle() = default;

    ProgressStyle clone() const { return *this; }

    void set_tab_width(int width);
    void set_for_stderr();
    void set_progress_chars(std::string_view chars);

    ProgressStyle with_template(std::string_view template_text) const;
    ProgressStyle with_progress_chars(std::string_view chars) const;
    ProgressStyle with_tick_chars(std::string_view chars) const;
    ProgressStyle with_tick_strings(const std::vector<std::string>& strings) const;

    ProgressStyle& with_key(std::string key,
                            std::function<std::string(const ProgressState&)> func);
    ProgressStyle& with_key_style(
        std::string key,
        std::function<std::pair<std::string, std::optional<Style>>(
            const ProgressState&, bool)> func);
    ProgressStyle& with_tracker(std::string key,
                                const std::shared_ptr<ProgressTracker>& tracker);

    std::string current_tick_str(const ProgressState& state) const;

    void tick_trackers(const ProgressState& state, TimePoint now) const;
    void reset_trackers(const ProgressState& state, TimePoint now) const;

    std::pair<std::vector<LineType>, std::vector<std::string>> format_state(
        const ProgressState& state, int target_width,
        bool for_stderr = false) const;

    std::string format_bar(float fract, int width, const Style* style,
                           const Style* alt_style, bool for_stderr) const;

    const std::vector<std::string>& tick_strings() const noexcept { return tick_strings_; }
    const std::vector<std::string>& progress_chars() const noexcept { return progress_chars_; }
    const Template& template_value() const noexcept { return template_; }
    int tab_width() const noexcept { return tab_width_; }
    int char_width() const noexcept { return char_width_; }

private:
    std::vector<std::string> tick_strings_;
    std::vector<std::string> progress_chars_;
    Template template_;
    int tab_width_ = 8;
    int char_width_ = 1;
    std::map<std::string, std::shared_ptr<ProgressTracker>> format_map_;

    std::string emit(const TemplatePart& part, const std::string& buf,
                     const std::optional<Style>& style, bool for_stderr) const;
};

ProgressStyle default_progress_style_bar();
ProgressStyle default_progress_style_spinner();
ProgressStyle progress_style_with_template(std::string_view template_text);

} // namespace mpb

#endif // MPB_STYLE_H
