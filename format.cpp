// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include "format.h"

#include <algorithm>
#include <cmath>
#include <iomanip>
#include <locale>
#include <sstream>
#include <string_view>

namespace mpb {
namespace {

constexpr double kSecond = 1.0;
constexpr double kMinute = 60.0;
constexpr double kHour = 3600.0;
constexpr double kDay = 24.0 * kHour;
constexpr double kWeek = 7.0 * kDay;
constexpr double kYear = 365.0 * kDay;

struct Unit {
    double seconds;
    const char* name;
    const char* short_name;
};

constexpr Unit kUnits[] = {
    {kYear, "year", "y"},
    {kWeek, "week", "w"},
    {kDay, "day", "d"},
    {kHour, "hour", "h"},
    {kMinute, "minute", "m"},
    {kSecond, "second", "s"},
};

long long round_half_away(double value) noexcept {
    return value >= 0.0 ? static_cast<long long>(std::floor(value + 0.5))
                        : static_cast<long long>(std::ceil(value - 0.5));
}

std::string grouped_uint(std::uint64_t value) {
    const std::string digits = std::to_string(value);
    std::string out;
    out.reserve(digits.size() + digits.size() / 3);
    const std::size_t length = digits.size();
    for (std::size_t i = 0; i < length; ++i) {
        out.push_back(digits[i]);
        const std::size_t pos = length - i - 1;
        if (pos > 0 && pos % 3 == 0) {
            out.push_back(',');
        }
    }
    return out;
}

std::string prefix(std::uint64_t value, std::uint64_t base,
                   const char* const* units, std::size_t count) {
    if (value < base) {
        return std::to_string(value) + " B";
    }
    const double number = static_cast<double>(value);
    double divisor = static_cast<double>(base);
    for (std::size_t i = 0; i < count; ++i) {
        if (number < divisor * static_cast<double>(base) || i == count - 1) {
            std::ostringstream out;
            out.imbue(std::locale::classic());
            out << std::fixed << std::setprecision(2) << number / divisor << ' '
                << units[i];
            return out.str();
        }
        divisor *= static_cast<double>(base);
    }
    return std::to_string(value) + " B";
}

} // namespace

std::string formatted_duration(Duration duration) {
    const auto total_seconds = static_cast<long long>(duration.count() / 1'000'000'000LL);
    long long seconds = total_seconds % 60;
    long long total = total_seconds / 60;
    long long minutes = total % 60;
    total /= 60;
    long long hours = total % 24;
    total /= 24;

    std::ostringstream out;
    out.imbue(std::locale::classic());
    if (total > 0) {
        out << total << "d " << std::setw(2) << std::setfill('0') << hours << ':'
            << std::setw(2) << minutes << ':' << std::setw(2) << seconds;
    } else {
        out << std::setw(2) << std::setfill('0') << hours << ':' << std::setw(2)
            << minutes << ':' << std::setw(2) << seconds;
    }
    return out.str();
}

std::string human_duration(Duration duration, bool alt) {
    double seconds = static_cast<double>(duration.count()) / 1'000'000'000.0;
    if (!std::isfinite(seconds)) {
        seconds = 0.0;
    }

    std::size_t index = 0;
    constexpr std::size_t count = sizeof(kUnits) / sizeof(kUnits[0]);
    for (std::size_t i = 0; i < count; ++i) {
        index = i;
        if (i + 1 < count) {
            const double current = kUnits[i].seconds;
            const double next = kUnits[i + 1].seconds;
            if (seconds + next / 2.0 >= current + current / 2.0) {
                break;
            }
        }
    }

    const Unit& unit = kUnits[index];
    long long value = round_half_away(seconds / unit.seconds);
    if (index + 1 < count && value < 2) {
        value = 2;
    }

    if (alt) {
        return std::to_string(value) + unit.short_name;
    }
    if (value == 1) {
        return std::to_string(value) + " " + unit.name;
    }
    return std::to_string(value) + " " + unit.name + "s";
}

std::string human_bytes(std::uint64_t value) {
    static const char* const units[] = {"KiB", "MiB", "GiB", "TiB", "PiB"};
    return prefix(value, 1024, units, 5);
}

std::string binary_bytes(std::uint64_t value) {
    static const char* const units[] = {"KiB", "MiB", "GiB", "TiB", "PiB"};
    return prefix(value, 1024, units, 5);
}

std::string decimal_bytes(std::uint64_t value) {
    static const char* const units[] = {"kB", "MB", "GB", "TB", "PB"};
    return prefix(value, 1000, units, 5);
}

std::string human_count(std::uint64_t value) { return grouped_uint(value); }

std::string human_float_count(double value, int precision) {
    std::string text;
    if (std::isnan(value)) {
        text = "NaN";
    } else if (std::isinf(value)) {
        text = value > 0 ? "inf" : "-inf";
    } else {
        std::ostringstream out;
        out.imbue(std::locale::classic());
        out << std::fixed << std::setprecision(std::max(0, precision)) << value;
        text = out.str();
    }

    std::string int_part = text;
    std::string frac_part;
    const auto dot = text.find('.');
    if (dot != std::string::npos) {
        int_part = text.substr(0, dot);
        frac_part = text.substr(dot + 1);
        while (!frac_part.empty() && frac_part.back() == '0') {
            frac_part.pop_back();
        }
    }

    std::string sign;
    std::string digits = int_part;
    if (!digits.empty() && digits.front() == '-') {
        sign = "-";
        digits.erase(digits.begin());
    }
    if (frac_part.empty() &&
        digits.find_first_not_of('0') == std::string::npos) {
        sign.clear();
    }

    std::string out;
    out.reserve(sign.size() + digits.size() + digits.size() / 3 + frac_part.size() + 1);
    out += sign;
    const std::size_t length = digits.size();
    for (std::size_t i = 0; i < length; ++i) {
        out.push_back(digits[i]);
        const std::size_t pos = length - i - 1;
        if (pos > 0 && pos % 3 == 0) {
            out.push_back(',');
        }
    }
    if (!frac_part.empty()) {
        out.push_back('.');
        out += frac_part;
    }
    return out;
}

} // namespace mpb
