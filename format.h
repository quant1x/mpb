// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_FORMAT_H
#define MPB_FORMAT_H 1

#include <cstdint>
#include <string>

#include "types.h"

namespace mpb {

    std::string formatted_duration(Duration duration);
    std::string human_duration(Duration duration, bool alt = false);
    std::string human_bytes(std::uint64_t value);
    std::string decimal_bytes(std::uint64_t value);
    std::string binary_bytes(std::uint64_t value);
    std::string human_count(std::uint64_t value);
    std::string human_float_count(double value, int precision = 4);

}  // namespace mpb

#endif  // MPB_FORMAT_H
