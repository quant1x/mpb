// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include "style.h"

#include <algorithm>
#include <atomic>
#include <charconv>
#include <cmath>
#include <cstdint>
#include <cstdlib>
#include <iomanip>
#include <limits>
#include <locale>
#include <sstream>
#include <string>
#include <system_error>
#include <utility>

#include "state.h"
#include "unicode_width_tables.h"
#include "utf8.h"

namespace mpb {
namespace {

std::atomic<bool> g_cjk_wide{false};

template <std::size_t N>
bool in_ranges(const unicode_width::Range (&ranges)[N], char32_t cp) {
    std::size_t lo = 0;
    std::size_t hi = N;
    while (lo < hi) {
        const std::size_t mid = lo + (hi - lo) / 2;
        if (ranges[mid].last < cp) {
            lo = mid + 1;
        } else {
            hi = mid;
        }
    }
    return lo < N && ranges[lo].first <= cp && cp <= ranges[lo].last;
}

std::string repeat(std::string_view text, int count) {
    std::string out;
    if (count <= 0) {
        return out;
    }
    out.reserve(text.size() * static_cast<std::size_t>(count));
    for (int i = 0; i < count; ++i) {
        out += text;
    }
    return out;
}

std::vector<std::string> split_lines(std::string_view text) {
    std::vector<std::string> out;
    std::size_t start = 0;
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

std::string replace_all(std::string text, std::string_view from,
                        std::string_view to) {
    if (from.empty()) {
        return text;
    }
    std::string out;
    out.reserve(text.size());
    std::size_t pos = 0;
    while (true) {
        const auto next = text.find(from, pos);
        if (next == std::string::npos) {
            out.append(text, pos, std::string::npos);
            break;
        }
        out.append(text, pos, next - pos);
        out.append(to);
        pos = next + from.size();
    }
    return out;
}

// ---------------------------------------------------------------------------
// ANSI DFA (console::ansi)
// ---------------------------------------------------------------------------

enum AnsiState {
    as_start = 0,
    as_s1,
    as_s2,
    as_s3,
    as_s4,
    as_s5,
    as_s6,
    as_s7,
    as_s8,
    as_s9,
    as_s10,
    as_s11,
    as_trap,
};

bool ansi_final(AnsiState state) {
    return state == as_s3 || state == as_s5 || state == as_s6 ||
           state == as_s7 || state == as_s8 || state == as_s9 ||
           state == as_s11;
}

bool digit_0_2(char32_t c) { return c >= U'0' && c <= U'2'; }
bool digit_3_9(char32_t c) { return c >= U'3' && c <= U'9'; }

bool final_char(char32_t c) {
    return (c >= U'A' && c <= U'P') || c == U'R' || c == U'Z' || c == U'c' ||
           c == U'q' || c == U'r' || c == U'y' || c == U'=' || c == U'>' ||
           c == U'<' || (c >= U'f' && c <= U'n');
}

AnsiState ansi_transition(AnsiState state, char32_t c) {
    if (c == 0x1B || c == 0x9B) {
        return state == as_start ? as_s1 : as_trap;
    }
    if (c == U'(' || c == U')') {
        if (state == as_s1) return as_s2;
        if (state == as_s2 || state == as_s4) return as_s4;
        return as_trap;
    }
    if (c == U';') {
        if (state == as_s1 || state == as_s2 || state == as_s4) return as_s4;
        if (state == as_s5 || state == as_s6 || state == as_s7 ||
            state == as_s8 || state == as_s10)
            return as_s10;
        return as_trap;
    }
    if (c == U'[' || c == U'#' || c == U'?') {
        if (state == as_s1 || state == as_s2 || state == as_s4) return as_s4;
        return as_trap;
    }
    if (digit_0_2(c)) {
        if (state == as_s1 || state == as_s4) return as_s5;
        if (state == as_s2) return as_s3;
        if (state == as_s5) return as_s6;
        if (state == as_s6) return as_s7;
        if (state == as_s7) return as_s8;
        if (state == as_s8) return as_s9;
        if (state == as_s10) return as_s5;
        return as_trap;
    }
    if (digit_3_9(c)) {
        if (state == as_s1 || state == as_s4) return as_s5;
        if (state == as_s2) return as_s5;
        if (state == as_s5) return as_s6;
        if (state == as_s6) return as_s7;
        if (state == as_s7) return as_s8;
        if (state == as_s8) return as_s9;
        if (state == as_s10) return as_s5;
        return as_trap;
    }
    if (final_char(c)) {
        if (state == as_s1 || state == as_s2 || state == as_s4 ||
            state == as_s5 || state == as_s6 || state == as_s7 ||
            state == as_s8 || state == as_s10)
            return as_s11;
        return as_trap;
    }
    return as_trap;
}

std::size_t match_ansi(std::u32string_view text, std::size_t index) {
    AnsiState state = as_start;
    std::size_t maybe_end = std::string::npos;
    std::size_t i = index;
    while (i < text.size()) {
        state = ansi_transition(state, text[i]);
        if (ansi_final(state)) {
            maybe_end = i;
        }
        if (state == as_trap) {
            break;
        }
        ++i;
    }
    return maybe_end == std::string::npos ? 0 : maybe_end + 1;
}

// ---------------------------------------------------------------------------
// Style spec
// ---------------------------------------------------------------------------

struct DottedStyle {
    int fg = 0;
    bool fg_set = false;
    int fg_256 = -1;
    int fg_rgb[3] = {0, 0, 0};
    bool fg_is_rgb = false;
    bool fg_bright = false;
    int bg = 0;
    bool bg_set = false;
    int bg_256 = -1;
    int bg_rgb[3] = {0, 0, 0};
    bool bg_is_rgb = false;
    bool bg_bright = false;
    std::uint16_t attrs = 0;
};

constexpr std::uint16_t attr_bold = 1u << 0;
constexpr std::uint16_t attr_dim = 1u << 1;
constexpr std::uint16_t attr_italic = 1u << 2;
constexpr std::uint16_t attr_underlined = 1u << 3;
constexpr std::uint16_t attr_blink = 1u << 4;
constexpr std::uint16_t attr_blink_fast = 1u << 5;
constexpr std::uint16_t attr_reverse = 1u << 6;
constexpr std::uint16_t attr_hidden = 1u << 7;
constexpr std::uint16_t attr_strike = 1u << 8;

bool parse_hex_byte(std::string_view text, int& out) {
    if (text.size() != 2) return false;
    unsigned value = 0;
    for (char c : text) {
        unsigned digit = 0;
        if (c >= '0' && c <= '9') {
            digit = static_cast<unsigned>(c - '0');
        } else if (c >= 'a' && c <= 'f') {
            digit = static_cast<unsigned>(c - 'a' + 10);
        } else if (c >= 'A' && c <= 'F') {
            digit = static_cast<unsigned>(c - 'A' + 10);
        } else {
            return false;
        }
        value = value * 16 + digit;
    }
    out = static_cast<int>(value);
    return true;
}

int color_index(std::string_view name) {
    if (name == "black") return 0;
    if (name == "red") return 1;
    if (name == "green") return 2;
    if (name == "yellow") return 3;
    if (name == "blue") return 4;
    if (name == "magenta") return 5;
    if (name == "cyan") return 6;
    if (name == "white") return 7;
    return -1;
}

bool parse_int(std::string_view text, int min_value, int max_value, int& out) {
    if (text.empty()) return false;
    int value = 0;
    const auto* begin = text.data();
    const auto* end = text.data() + text.size();
    const auto result = std::from_chars(begin, end, value);
    if (result.ec != std::errc{} || result.ptr != end || value < min_value ||
        value > max_value) {
        return false;
    }
    out = value;
    return true;
}

DottedStyle parse_dotted_style(std::string_view spec) {
    DottedStyle style;
    std::size_t start = 0;
    while (start <= spec.size()) {
        const auto dot = spec.find('.', start);
        const auto part = spec.substr(
            start, dot == std::string_view::npos ? std::string_view::npos : dot - start);

        if (const int idx = color_index(part); idx >= 0) {
            style.fg = idx;
            style.fg_set = true;
        } else if (part == "bright") {
            style.fg_bright = true;
        } else if (part.size() > 3 && part.substr(0, 3) == "on_") {
            const auto name = part.substr(3);
            if (const int bg_idx = color_index(name); bg_idx >= 0) {
                style.bg = bg_idx;
                style.bg_set = true;
            } else if (name == "bright") {
                style.bg_bright = true;
            } else if (name.size() == 7 && name[0] == '#') {
                int r = 0, g = 0, b = 0;
                if (parse_hex_byte(name.substr(1, 2), r) &&
                    parse_hex_byte(name.substr(3, 2), g) &&
                    parse_hex_byte(name.substr(5, 2), b)) {
                    style.bg_rgb[0] = r;
                    style.bg_rgb[1] = g;
                    style.bg_rgb[2] = b;
                    style.bg_is_rgb = true;
                }
            } else {
                int value = 0;
                if (parse_int(name, 0, 255, value)) {
                    style.bg_256 = value;
                }
            }
        } else if (part == "on_bright") {
            style.bg_bright = true;
        } else if (part.size() == 7 && part[0] == '#') {
            int r = 0, g = 0, b = 0;
            if (parse_hex_byte(part.substr(1, 2), r) &&
                parse_hex_byte(part.substr(3, 2), g) &&
                parse_hex_byte(part.substr(5, 2), b)) {
                style.fg_rgb[0] = r;
                style.fg_rgb[1] = g;
                style.fg_rgb[2] = b;
                style.fg_is_rgb = true;
            }
        } else if (part == "bold") {
            style.attrs |= attr_bold;
        } else if (part == "dim") {
            style.attrs |= attr_dim;
        } else if (part == "italic") {
            style.attrs |= attr_italic;
        } else if (part == "underlined") {
            style.attrs |= attr_underlined;
        } else if (part == "blink") {
            style.attrs |= attr_blink;
        } else if (part == "blink_fast") {
            style.attrs |= attr_blink_fast;
        } else if (part == "reverse") {
            style.attrs |= attr_reverse;
        } else if (part == "hidden") {
            style.attrs |= attr_hidden;
        } else if (part == "strikethrough") {
            style.attrs |= attr_strike;
        } else {
            int value = 0;
            if (parse_int(part, 0, 255, value)) {
                style.fg_256 = value;
            }
        }

        if (dot == std::string_view::npos) {
            break;
        }
        start = dot + 1;
    }
    return style;
}

std::string render_dotted(const DottedStyle& style, std::string_view text) {
    std::string out;
    bool reset = false;

    if (style.fg_is_rgb) {
        out += "\x1b[38;2;" + std::to_string(style.fg_rgb[0]) + ";" +
               std::to_string(style.fg_rgb[1]) + ";" +
               std::to_string(style.fg_rgb[2]) + "m";
        reset = true;
    } else if (style.fg_256 >= 0) {
        out += "\x1b[38;5;" + std::to_string(style.fg_256) + "m";
        reset = true;
    } else if (style.fg_set) {
        if (style.fg_bright) {
            out += "\x1b[38;5;" + std::to_string(style.fg + 8) + "m";
        } else {
            out += "\x1b[" + std::to_string(style.fg + 30) + "m";
        }
        reset = true;
    }

    if (style.bg_is_rgb) {
        out += "\x1b[48;2;" + std::to_string(style.bg_rgb[0]) + ";" +
               std::to_string(style.bg_rgb[1]) + ";" +
               std::to_string(style.bg_rgb[2]) + "m";
        reset = true;
    } else if (style.bg_256 >= 0) {
        out += "\x1b[48;5;" + std::to_string(style.bg_256) + "m";
        reset = true;
    } else if (style.bg_set) {
        if (style.bg_bright) {
            out += "\x1b[48;5;" + std::to_string(style.bg + 8) + "m";
        } else {
            out += "\x1b[" + std::to_string(style.bg + 40) + "m";
        }
        reset = true;
    }

    for (int bit = 0; bit < 9; ++bit) {
        if ((style.attrs & (1u << bit)) != 0) {
            out += "\x1b[" + std::to_string(bit + 1) + "m";
            reset = true;
        }
    }

    out += text;
    if (reset) {
        out += "\x1b[0m";
    }
    return out;
}

std::string apply_ansi_style(std::string_view text, std::string_view spec) {
    if (text.empty() || spec.empty()) {
        return std::string(text);
    }
    return render_dotted(parse_dotted_style(spec), text);
}

std::uint64_t u64_from_float(double value) {
    if (std::isnan(value) || value <= 0.0) return 0;
    if (std::isinf(value)) return kU64Max;
    if (value >= static_cast<double>(kU64Max)) return kU64Max;
    return static_cast<std::uint64_t>(value);
}

std::string percent_text(float percent, int precision) {
    std::ostringstream out;
    out.imbue(std::locale::classic());
    out << std::fixed << std::setprecision(precision) << percent;
    return out.str();
}

bool is_space(char32_t c) {
    return c == U' ' || c == U'\t' || c == U'\n' || c == U'\r' || c == 0x0B ||
           c == 0x0C;
}

bool has_tab(std::string_view text) { return text.find('\t') != std::string::npos; }

std::string expand_tabs(std::string_view text, int tab_width) {
    if (tab_width <= 0 || !has_tab(text)) {
        return std::string(text);
    }
    const std::string tab(static_cast<std::size_t>(tab_width), ' ');
    return replace_all(std::string(text), "\t", tab);
}

enum class ParseState {
    literal,
    maybe_open,
    key,
    align,
    width,
    first_style,
    alt_style,
    double_close,
};

const char* state_name(ParseState state) {
    switch (state) {
    case ParseState::literal: return "Literal";
    case ParseState::maybe_open: return "MaybeOpen";
    case ParseState::key: return "Key";
    case ParseState::align: return "Align";
    case ParseState::width: return "Width";
    case ParseState::first_style: return "FirstStyle";
    case ParseState::alt_style: return "AltStyle";
    case ParseState::double_close: return "DoubleClose";
    }
    return "?";
}

} // namespace

// ---------------------------------------------------------------------------
// Width / ANSI public functions
// ---------------------------------------------------------------------------

void Width::set_default_cjk(bool is_cjk) { g_cjk_wide.store(is_cjk); }

int char_width(char32_t cp, bool cjk) {
    if (cp < 0x20 || (cp >= 0x7F && cp <= 0x9F)) {
        return 0;
    }
    if (in_ranges(unicode_width::kZeroWidth, cp)) {
        return 0;
    }
    if (in_ranges(unicode_width::kWide, cp)) {
        return 2;
    }
    if (in_ranges(unicode_width::kAmbiguous, cp)) {
        return cjk ? 2 : 1;
    }
    return 1;
}

int Width::char_width(char32_t cp) {
    return mpb::char_width(cp, g_cjk_wide.load());
}

int str_width(std::string_view text, bool cjk) {
    int total = 0;
    for (const auto& cp : utf8_decode(text)) {
        total += char_width(cp, cjk);
    }
    return total;
}

int Width::str_width(std::string_view text) {
    return mpb::str_width(text, g_cjk_wide.load());
}

int measure_text_width(std::string_view text) {
    int total = 0;
    for (const auto& segment : ansi_segments(text)) {
        if (!segment.is_ansi) {
            total += Width::str_width(segment.text);
        }
    }
    return total;
}

int Width::ansi_str(std::string_view text) { return measure_text_width(text); }

std::vector<AnsiSegment> ansi_segments(std::string_view text) {
    const auto cps = utf8_decode(text);
    const auto offsets = utf8_codepoints(text);
    std::vector<AnsiSegment> out;
    std::size_t visible_start = 0;
    std::size_t i = 0;

    auto byte_at = [&](std::size_t cp_index) -> std::size_t {
        return cp_index < offsets.size() ? offsets[cp_index].offset : text.size();
    };

    while (i < cps.size()) {
        if (cps[i] == 0x1B || cps[i] == 0x9B) {
            const std::size_t end_cp = match_ansi(cps, i);
            if (end_cp != 0) {
                if (i > visible_start) {
                    out.push_back({std::string(text.substr(byte_at(visible_start),
                                                          byte_at(i) - byte_at(visible_start))),
                                   false});
                }
                const std::size_t begin_byte = byte_at(i);
                const std::size_t end_byte = byte_at(end_cp);
                out.push_back(
                    {std::string(text.substr(begin_byte, end_byte - begin_byte)), true});
                visible_start = end_cp;
                i = end_cp;
                continue;
            }
        }
        ++i;
    }
    if (visible_start < cps.size()) {
        const std::size_t begin_byte = byte_at(visible_start);
        out.push_back({std::string(text.substr(begin_byte, text.size() - begin_byte)), false});
    }
    return out;
}

std::string ansi_slice(std::string_view text, int start, int end) {
    if (start < 0) start = 0;
    if (end <= start) return {};

    std::string out;
    int column = 0;
    for (const auto& segment : ansi_segments(text)) {
        if (segment.is_ansi) {
            out += segment.text;
            continue;
        }
        for (char32_t cp : utf8_decode(segment.text)) {
            const int width = Width::char_width(cp);
            if (start <= column && column + width <= end) {
                out += utf8_encode(cp);
            }
            column += width;
            if (column > end) {
                break;
            }
        }
        if (column > end) {
            break;
        }
    }
    return out;
}

std::string pad_and_truncate(std::string_view text, int width, char align,
                             bool truncate) {
    if (width <= 0) {
        return {};
    }
    std::string value(text);
    int cols = measure_text_width(value);
    if (cols == width) {
        return value;
    }
    if (cols > width) {
        if (!truncate) {
            return value;
        }
        int start_col = 0;
        if (align == '^') {
            start_col = (cols - width) / 2;
        } else if (align == '>') {
            start_col = cols - width;
        }
        if (start_col < 0) start_col = 0;
        value = ansi_slice(value, start_col, start_col + width);
        cols = measure_text_width(value);
    }

    const int pad = width - cols;
    int left = 0;
    int right = 0;
    if (align == '^') {
        left = pad / 2;
        right = pad - left;
    } else if (align == '>') {
        left = pad;
    } else {
        right = pad;
    }
    std::string out;
    out.reserve(static_cast<std::size_t>(left + right) + value.size());
    out.append(static_cast<std::size_t>(left), ' ');
    out += value;
    out.append(static_cast<std::size_t>(right), ' ');
    return out;
}

// ---------------------------------------------------------------------------
// Style
// ---------------------------------------------------------------------------

std::string Style::apply_for(std::string_view text, bool for_stderr) const {
    const std::string& chosen = (for_stderr && !spec_err.empty()) ? spec_err : spec;
    if (chosen.empty()) {
        return std::string(text);
    }
    return apply_ansi_style(text, chosen);
}

// ---------------------------------------------------------------------------
// Template
// ---------------------------------------------------------------------------

TemplateError::TemplateError(std::string state, char32_t next)
    : std::runtime_error("TemplateError: unexpected character in state " + state),
      state_(std::move(state)),
      next_(next) {}

Template Template::from_str(std::string_view text) {
    return template_from_str_with_tab_width(text, 8);
}

Template Template::from_str_with_tab_width(std::string_view text, int tab_width) {
    return template_from_str_with_tab_width(text, tab_width);
}

void Template::set_tab_width(int new_tab_width) {
    if (new_tab_width <= 0 || current_tab_width == new_tab_width) {
        return;
    }
    const std::string tab(static_cast<std::size_t>(new_tab_width), ' ');
    if (original_texts.size() != parts.size()) {
        original_texts.assign(parts.size(), std::string{});
    }
    for (std::size_t i = 0; i < parts.size(); ++i) {
        if (!original_texts[i].empty()) {
            parts[i].text = replace_all(original_texts[i], "\t", tab);
        }
    }
    current_tab_width = new_tab_width;
}

Template template_from_str(std::string_view text) {
    return template_from_str_with_tab_width(text, 8);
}

Template template_from_str_with_tab_width(std::string_view text, int tab_width) {
    if (tab_width <= 0) {
        tab_width = 8;
    }

    std::vector<TemplatePart> parts;
    std::string buf;
    ParseState state = ParseState::literal;

    auto append_literal = [&](std::string value) {
        TemplatePart part;
        part.is_placeholder = false;
        part.text = std::move(value);
        parts.push_back(std::move(part));
    };

    auto last_placeholder = [&]() -> TemplatePart* {
        if (parts.empty() || !parts.back().is_placeholder) {
            return nullptr;
        }
        return &parts.back();
    };

    for (char32_t c : utf8_decode(text)) {
        ParseState new_state = state;
        std::optional<char32_t> keep;

        switch (state) {
        case ParseState::literal:
            if (c == U'{') {
                new_state = ParseState::maybe_open;
            } else if (c == U'\n') {
                if (!buf.empty()) {
                    append_literal(buf);
                    buf.clear();
                }
                append_literal("\n");
                new_state = ParseState::literal;
            } else if (c == U'}') {
                new_state = ParseState::double_close;
                keep = c;
            } else {
                new_state = ParseState::literal;
                keep = c;
            }
            break;

        case ParseState::double_close:
            if (c == U'}') {
                new_state = ParseState::literal;
            } else {
                throw TemplateError(state_name(state), c);
            }
            break;

        case ParseState::maybe_open:
            if (c == U'{') {
                new_state = ParseState::literal;
                keep = c;
            } else if (is_space(c)) {
                append_literal("{" + buf + utf8_encode(c));
                buf.clear();
                new_state = ParseState::literal;
            } else if (c != U'}' && c != U':') {
                new_state = ParseState::key;
                keep = c;
            } else {
                throw TemplateError(state_name(state), c);
            }
            break;

        case ParseState::key:
            if (is_space(c)) {
                append_literal("{" + buf + utf8_encode(c));
                buf.clear();
                new_state = ParseState::literal;
            } else if (c == U'}' || c == U':') {
                new_state = c == U'}' ? ParseState::literal : ParseState::align;
            } else if (c == U'!' && !buf.empty()) {
                TemplatePart part;
                part.is_placeholder = true;
                part.key = buf;
                part.align = '<';
                part.truncate = true;
                parts.push_back(std::move(part));
                buf.clear();
                new_state = ParseState::width;
            } else {
                new_state = ParseState::key;
                keep = c;
            }
            break;

        case ParseState::align:
            if (c == U'<' || c == U'^' || c == U'>') {
                if (auto* part = last_placeholder()) part->align = static_cast<char>(c);
                new_state = ParseState::width;
            } else if (c >= U'0' && c <= U'9') {
                new_state = ParseState::width;
                keep = c;
            } else if (c == U'!') {
                if (auto* part = last_placeholder()) part->truncate = true;
                new_state = ParseState::width;
            } else if (c == U'.') {
                new_state = ParseState::first_style;
            } else if (c == U'}') {
                new_state = ParseState::literal;
            } else {
                throw TemplateError(state_name(state), c);
            }
            break;

        case ParseState::width:
            if (c >= U'0' && c <= U'9') {
                new_state = ParseState::width;
                keep = c;
            } else if (c == U'!') {
                if (auto* part = last_placeholder()) part->truncate = true;
                new_state = ParseState::width;
            } else if (c == U'.') {
                new_state = ParseState::first_style;
            } else if (c == U'}') {
                new_state = ParseState::literal;
            } else {
                throw TemplateError(state_name(state), c);
            }
            break;

        case ParseState::first_style:
            if (c == U'/') {
                new_state = ParseState::alt_style;
            } else if (c == U'}') {
                new_state = ParseState::literal;
            } else {
                new_state = ParseState::first_style;
                keep = c;
            }
            break;

        case ParseState::alt_style:
            if (c == U'}') {
                new_state = ParseState::literal;
            } else {
                new_state = ParseState::alt_style;
                keep = c;
            }
            break;
        }

        if (state == ParseState::maybe_open && new_state == ParseState::key &&
            !buf.empty()) {
            append_literal(buf);
            buf.clear();
        } else if (state == ParseState::key &&
                   (new_state == ParseState::align ||
                    new_state == ParseState::literal) &&
                   !buf.empty()) {
            TemplatePart part;
            part.is_placeholder = true;
            part.key = buf;
            part.align = '<';
            parts.push_back(std::move(part));
            buf.clear();
        } else if (state == ParseState::width &&
                   (new_state == ParseState::first_style ||
                    new_state == ParseState::literal) &&
                   !buf.empty()) {
            if (auto* part = last_placeholder()) {
                std::int64_t value = 0;
                const auto result =
                    std::from_chars(buf.data(), buf.data() + buf.size(), value);
                if (result.ec == std::errc{} &&
                    result.ptr == buf.data() + buf.size() && value >= 0 &&
                    value <= std::numeric_limits<std::int64_t>::max()) {
                    part->width = static_cast<int>(value);
                }
            }
            buf.clear();
        } else if (state == ParseState::first_style &&
                   (new_state == ParseState::alt_style ||
                    new_state == ParseState::literal) &&
                   !buf.empty()) {
            if (auto* part = last_placeholder()) {
                part->style = Style{buf, {}};
            }
            buf.clear();
        } else if (state == ParseState::alt_style &&
                   new_state == ParseState::literal && !buf.empty()) {
            if (auto* part = last_placeholder()) {
                part->alt_style = Style{buf, {}};
            }
            buf.clear();
        }

        state = new_state;
        if (keep.has_value()) {
            buf += utf8_encode(*keep);
        }
    }

    if ((state == ParseState::literal || state == ParseState::double_close) &&
        !buf.empty()) {
        append_literal(buf);
    }

    Template tmpl;
    tmpl.parts = std::move(parts);
    tmpl.current_tab_width = tab_width;
    tmpl.original_texts.assign(tmpl.parts.size(), std::string{});
    for (std::size_t i = 0; i < tmpl.parts.size(); ++i) {
        if (!tmpl.parts[i].is_placeholder && has_tab(tmpl.parts[i].text)) {
            tmpl.original_texts[i] = tmpl.parts[i].text;
        }
    }
    return tmpl;
}

// ---------------------------------------------------------------------------
// Trackers
// ---------------------------------------------------------------------------

TabWriter::TabWriter(int tab_width) : tab_width_(tab_width) {}

void TabWriter::write_str(std::string_view text) {
    if (tab_width_ <= 0 || text.find('\t') == std::string_view::npos) {
        value_.append(text);
        return;
    }
    for (char c : text) {
        if (c == '\t') {
            value_.append(static_cast<std::size_t>(tab_width_), ' ');
        } else {
            value_.push_back(c);
        }
    }
}

std::shared_ptr<ProgressTracker> FuncTracker::clone() const {
    return std::make_shared<FuncTracker>(func_);
}

void FuncTracker::write(const ProgressState& state, TabWriter& writer) const {
    func_(state, writer);
}

// ---------------------------------------------------------------------------
// ProgressStyle
// ---------------------------------------------------------------------------

ProgressStyle::ProgressStyle()
    : ProgressStyle(Template::from_str("{wide_bar} {pos}/{len}")) {}

ProgressStyle::ProgressStyle(const Template& tmpl) : template_(tmpl) {
    const std::string ticks = "⠁⠁⠉⠙⠚⠒⠂⠂⠒⠲⠴⠤⠄⠄⠤⠠⠠⠤⠦⠖⠒⠐⠐⠒⠓⠋⠉⠈⠈ ";
    for (char32_t cp : utf8_decode(ticks)) {
        tick_strings_.push_back(utf8_encode(cp));
    }
    progress_chars_ = {"█", "░"};
    tab_width_ = 8;
    char_width_ = measure_text_width(progress_chars_.front());
    if (char_width_ <= 0) char_width_ = 1;
}

ProgressStyle::ProgressStyle(std::string_view template_text)
    : ProgressStyle(Template::from_str(template_text)) {}

ProgressStyle::ProgressStyle(const ProgressStyle& other)
    : tick_strings_(other.tick_strings_),
      progress_chars_(other.progress_chars_),
      template_(other.template_),
      tab_width_(other.tab_width_),
      char_width_(other.char_width_) {
    for (const auto& [key, tracker] : other.format_map_) {
        if (tracker) {
            format_map_[key] = tracker->clone();
        }
    }
}

ProgressStyle& ProgressStyle::operator=(const ProgressStyle& other) {
    if (this == &other) return *this;
    tick_strings_ = other.tick_strings_;
    progress_chars_ = other.progress_chars_;
    template_ = other.template_;
    tab_width_ = other.tab_width_;
    char_width_ = other.char_width_;
    format_map_.clear();
    for (const auto& [key, tracker] : other.format_map_) {
        if (tracker) {
            format_map_[key] = tracker->clone();
        }
    }
    return *this;
}

void ProgressStyle::set_tab_width(int width) {
    tab_width_ = width;
    template_.set_tab_width(width);
}

void ProgressStyle::set_for_stderr() {
    for (auto& part : template_.parts) {
        if (part.style) part.style->spec_err = part.style->spec;
        if (part.alt_style) part.alt_style->spec_err = part.alt_style->spec;
    }
}

void ProgressStyle::set_progress_chars(std::string_view chars) {
    if (chars.empty()) {
        progress_chars_ = {"#", "-"};
        char_width_ = 1;
        return;
    }
    progress_chars_.clear();
    for (char32_t cp : utf8_decode(chars)) {
        progress_chars_.push_back(utf8_encode(cp));
    }
    char_width_ = measure_text_width(progress_chars_.front());
    if (char_width_ <= 0) char_width_ = 1;
}

ProgressStyle ProgressStyle::with_template(std::string_view template_text) const {
    ProgressStyle copy(*this);
    copy.template_ = Template::from_str(template_text);
    return copy;
}

ProgressStyle ProgressStyle::with_progress_chars(std::string_view chars) const {
    ProgressStyle copy(*this);
    copy.set_progress_chars(chars);
    return copy;
}

ProgressStyle ProgressStyle::with_tick_chars(std::string_view chars) const {
    ProgressStyle copy(*this);
    const auto cps = utf8_decode(chars);
    if (cps.size() >= 2) {
        copy.tick_strings_.clear();
        for (char32_t cp : cps) {
            copy.tick_strings_.push_back(utf8_encode(cp));
        }
    }
    return copy;
}

ProgressStyle ProgressStyle::with_tick_strings(
    const std::vector<std::string>& strings) const {
    ProgressStyle copy(*this);
    if (!strings.empty()) {
        copy.tick_strings_ = strings;
    }
    return copy;
}

ProgressStyle& ProgressStyle::with_key(
    std::string key, std::function<std::string(const ProgressState&)> func) {
    format_map_[std::move(key)] = std::make_shared<FuncTracker>(
        [func = std::move(func)](const ProgressState& state, TabWriter& writer) {
            writer.write_str(func(state));
        });
    return *this;
}

ProgressStyle& ProgressStyle::with_key_style(
    std::string key,
    std::function<std::pair<std::string, std::optional<Style>>(const ProgressState&,
                                                               bool)>
        func) {
    format_map_[std::move(key)] = std::make_shared<FuncTracker>(
        [func = std::move(func)](const ProgressState& state, TabWriter& writer) {
            auto [text, style] = func(state, false);
            (void)style;
            writer.write_str(text);
        });
    return *this;
}

ProgressStyle& ProgressStyle::with_tracker(
    std::string key, const std::shared_ptr<ProgressTracker>& tracker) {
    if (tracker) {
        format_map_[std::move(key)] = tracker->clone();
    }
    return *this;
}

std::string ProgressStyle::current_tick_str(const ProgressState& state) const {
    if (tick_strings_.empty()) return {};
    if (state.is_finished()) return tick_strings_.back();
    if (tick_strings_.size() <= 1) return tick_strings_.front();
    return tick_strings_[static_cast<std::size_t>(
        state.tick() % (tick_strings_.size() - 1))];
}

void ProgressStyle::tick_trackers(const ProgressState& state, TimePoint now) const {
    for (const auto& [key, tracker] : format_map_) {
        (void)key;
        if (tracker) tracker->tick(state, now);
    }
}

void ProgressStyle::reset_trackers(const ProgressState& state, TimePoint now) const {
    for (const auto& [key, tracker] : format_map_) {
        (void)key;
        if (tracker) tracker->reset(state, now);
    }
}

std::string ProgressStyle::emit(const TemplatePart& part, const std::string& buf,
                                const std::optional<Style>& style,
                                bool for_stderr) const {
    std::string value = buf;
    if (part.width > 0) {
        value = pad_and_truncate(value, part.width, part.align, part.truncate);
    }
    if (style) {
        value = style->apply_for(value, for_stderr);
    }
    return value;
}

std::pair<std::vector<LineType>, std::vector<std::string>>
ProgressStyle::format_state(const ProgressState& state, int target_width,
                            bool for_stderr) const {
    const std::uint64_t pos = state.pos();
    const std::uint64_t length = state.len().value_or(pos);

    std::vector<std::string> lines{std::string{}};
    const TemplatePart* wide = nullptr;

    for (std::size_t idx = 0; idx < template_.parts.size(); ++idx) {
        const TemplatePart& part = template_.parts[idx];
        if (!part.is_placeholder) {
            std::string text = part.text;
            if (idx < template_.original_texts.size() &&
                !template_.original_texts[idx].empty()) {
                text = expand_tabs(template_.original_texts[idx],
                                   template_.current_tab_width);
            }
            const auto segments = split_lines(text);
            for (std::size_t i = 0; i < segments.size(); ++i) {
                lines.back() += segments[i];
                if (i + 1 != segments.size()) {
                    lines.emplace_back();
                }
            }
            continue;
        }

        const std::string& key = part.key;
        std::optional<Style> style = part.style;
        if (key == "bar" || key == "wide_bar") {
            style.reset();
        }

        if (const auto it = format_map_.find(key); it != format_map_.end()) {
            TabWriter writer(tab_width_);
            it->second->write(state, writer);
            lines.back() += emit(part, writer.value(), style, for_stderr);
            continue;
        }

        std::string buf;
        if (key == "wide_bar" || key == "wide_msg") {
            buf.assign(1, '\0');
            wide = &part;
        } else if (key == "bar") {
            const int width = part.width > 0 ? part.width : 20;
            buf = format_bar(state.fraction(), width,
                             part.style ? &*part.style : nullptr,
                             part.alt_style ? &*part.alt_style : nullptr,
                             for_stderr);
        } else if (key == "spinner") {
            buf = current_tick_str(state);
        } else if (key == "msg") {
            buf = state.message().expanded();
        } else if (key == "prefix") {
            buf = state.prefix().expanded();
        } else if (key == "pos") {
            buf = std::to_string(pos);
        } else if (key == "len") {
            buf = std::to_string(length);
        } else if (key == "percent") {
            buf = percent_text(f32(state.fraction() * 100.0f), 0);
        } else if (key == "percent_precise") {
            buf = percent_text(f32(state.fraction() * 100.0f), 3);
        } else if (key == "human_pos") {
            buf = human_count(pos);
        } else if (key == "human_len") {
            buf = human_count(length);
        } else if (key == "bytes") {
            buf = human_bytes(pos);
        } else if (key == "total_bytes") {
            buf = human_bytes(length);
        } else if (key == "decimal_bytes") {
            buf = decimal_bytes(pos);
        } else if (key == "decimal_total_bytes") {
            buf = decimal_bytes(length);
        } else if (key == "binary_bytes") {
            buf = binary_bytes(pos);
        } else if (key == "binary_total_bytes") {
            buf = binary_bytes(length);
        } else if (key == "elapsed_precise") {
            buf = formatted_duration(state.elapsed());
        } else if (key == "elapsed") {
            buf = human_duration(state.elapsed(), true);
        } else if (key == "duration_precise") {
            buf = formatted_duration(state.duration());
        } else if (key == "duration") {
            buf = human_duration(state.duration(), true);
        } else if (key == "eta_precise") {
            buf = formatted_duration(state.eta());
        } else if (key == "eta") {
            buf = human_duration(state.eta(), true);
        } else if (key == "per_sec") {
            const double per_sec = state.per_sec();
            buf = human_float_count(per_sec, part.width > 0 ? part.width : 4) + "/s";
        } else if (key == "bytes_per_sec") {
            buf = human_bytes(u64_from_float(state.per_sec())) + "/s";
        } else if (key == "decimal_bytes_per_sec") {
            buf = decimal_bytes(u64_from_float(state.per_sec())) + "/s";
        } else if (key == "binary_bytes_per_sec") {
            buf = binary_bytes(u64_from_float(state.per_sec())) + "/s";
        }
        lines.back() += emit(part, buf, style, for_stderr);
    }

    std::vector<std::string> results;
    const std::string_view nul("\0", 1);
    auto push_line = [&](std::string_view expanded) {
        for (auto& line : split_lines(expanded)) {
            results.push_back(std::move(line));
        }
    };

    for (const auto& line : lines) {
        if (line.find('\0') == std::string::npos || wide == nullptr) {
            push_line(line);
            continue;
        }

        std::string src = line;
        if (line.find('\n') != std::string::npos) {
            for (auto& part : split_lines(line)) {
                if (part.find('\0') != std::string::npos) {
                    src = std::move(part);
                    break;
                }
            }
        }
        int left = target_width - measure_text_width(replace_all(src, nul, ""));
        if (left < 0) left = 0;

        if (wide->key == "wide_msg") {
            std::string rendered = pad_and_truncate(
                state.message().expanded(), left, wide->align, true);
            if (!line.empty() && line.back() == '\0') {
                while (!rendered.empty() && rendered.back() == ' ') {
                    rendered.pop_back();
                }
            }
            push_line(replace_all(line, nul, rendered));
        } else {
            const std::string bar = format_bar(
                state.fraction(), left, wide->style ? &*wide->style : nullptr,
                wide->alt_style ? &*wide->alt_style : nullptr, for_stderr);
            push_line(replace_all(line, nul, bar));
        }
    }

    std::vector<LineType> types(results.size(), LineType::bar);
    return {std::move(types), std::move(results)};
}

std::string ProgressStyle::format_bar(float fract, int width, const Style* style,
                                      const Style* alt_style,
                                      bool for_stderr) const {
    const int cw = char_width_ > 0 ? char_width_ : 1;
    const int clusters = width / cw;

    float fillf = f32(static_cast<float>(clusters) * fract);
    if (fillf < 0.0f) fillf = 0.0f;
    if (fillf > static_cast<float>(clusters)) fillf = static_cast<float>(clusters);
    const int entirely = static_cast<int>(fillf);

    std::optional<int> cur;
    if (fillf > 0.0f && entirely < clusters) {
        const int n = static_cast<int>(progress_chars_.size()) - 2;
        if (n > 0) {
            if (n == 1) {
                cur = 1;
            } else {
                int value = n - static_cast<int>(
                                    f32((fillf - static_cast<float>(entirely)) *
                                        static_cast<float>(n)));
                value = std::clamp(value, 0, n);
                cur = value;
            }
        }
    }

    std::string filled = repeat(progress_chars_.front(), entirely);
    if (cur.has_value()) {
        int idx = *cur;
        if (idx >= static_cast<int>(progress_chars_.size()) - 1) {
            idx = static_cast<int>(progress_chars_.size()) - 2;
        }
        if (idx < 0) idx = 0;
        filled += progress_chars_[static_cast<std::size_t>(idx)];
    }

    int bg = clusters - entirely;
    if (cur.has_value() && bg > 0) --bg;
    if (bg < 0) bg = 0;
    const std::string rest = repeat(progress_chars_.back(), bg);

    std::string filled_str = filled;
    if (style != nullptr) {
        filled_str = style->apply_for(filled, for_stderr);
    }
    const Style* rest_style = alt_style != nullptr ? alt_style : style;
    std::string rest_str = rest;
    if (rest_style != nullptr) {
        rest_str = rest_style->apply_for(rest, for_stderr);
    }
    return filled_str + rest_str;
}

ProgressStyle default_progress_style_bar() {
    return progress_style_with_template("{wide_bar} {pos}/{len}");
}

ProgressStyle default_progress_style_spinner() {
    return progress_style_with_template("{spinner} {msg}");
}

ProgressStyle progress_style_with_template(std::string_view template_text) {
    return ProgressStyle(Template::from_str(template_text));
}

} // namespace mpb
