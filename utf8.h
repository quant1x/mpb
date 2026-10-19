// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_UTF8_H
#define MPB_UTF8_H 1

#include <cstddef>
#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace mpb {

    struct Utf8Codepoint {
        char32_t    value  = U'\uFFFD';
        std::size_t offset = 0;
        std::size_t length = 1;
    };

    inline std::vector<Utf8Codepoint> utf8_codepoints(std::string_view text) {
        std::vector<Utf8Codepoint> out;
        out.reserve(text.size());
        std::size_t i = 0;
        while (i < text.size()) {
            const auto  first = static_cast<unsigned char>(text[i]);
            char32_t    cp    = U'\uFFFD';
            std::size_t len   = 1;
            if (first < 0x80) {
                cp = first;
            } else if ((first & 0xE0) == 0xC0) {
                len = 2;
                cp  = first & 0x1F;
            } else if ((first & 0xF0) == 0xE0) {
                len = 3;
                cp  = first & 0x0F;
            } else if ((first & 0xF8) == 0xF0) {
                len = 4;
                cp  = first & 0x07;
            } else {
                out.push_back({U'\uFFFD', i, 1});
                ++i;
                continue;
            }

            if (i + len > text.size()) {
                out.push_back({U'\uFFFD', i, 1});
                ++i;
                continue;
            }

            bool valid = true;
            for (std::size_t k = 1; k < len; ++k) {
                const auto next = static_cast<unsigned char>(text[i + k]);
                if ((next & 0xC0) != 0x80) {
                    valid = false;
                    break;
                }
                cp = (cp << 6) | (next & 0x3F);
            }

            // Reject overlong forms, surrogates and values beyond Unicode.
            if (!valid || (len == 2 && cp < 0x80) || (len == 3 && cp < 0x800) || (len == 4 && cp < 0x10000) ||
                (cp >= 0xD800 && cp <= 0xDFFF) || cp > 0x10FFFF) {
                out.push_back({U'\uFFFD', i, 1});
                ++i;
                continue;
            }

            out.push_back({cp, i, len});
            i += len;
        }
        return out;
    }

    inline std::u32string utf8_decode(std::string_view text) {
        std::u32string out;
        out.reserve(text.size());
        for (const auto &cp : utf8_codepoints(text)) {
            out.push_back(cp.value);
        }
        return out;
    }

    inline std::string utf8_encode(char32_t cp) {
        std::string out;
        if (cp <= 0x7F) {
            out.push_back(static_cast<char>(cp));
        } else if (cp <= 0x7FF) {
            out.push_back(static_cast<char>(0xC0 | (cp >> 6)));
            out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
        } else if (cp <= 0xFFFF) {
            out.push_back(static_cast<char>(0xE0 | (cp >> 12)));
            out.push_back(static_cast<char>(0x80 | ((cp >> 6) & 0x3F)));
            out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
        } else {
            out.push_back(static_cast<char>(0xF0 | (cp >> 18)));
            out.push_back(static_cast<char>(0x80 | ((cp >> 12) & 0x3F)));
            out.push_back(static_cast<char>(0x80 | ((cp >> 6) & 0x3F)));
            out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
        }
        return out;
    }

    inline std::string utf8_encode(std::u32string_view text) {
        std::string out;
        out.reserve(text.size());
        for (const char32_t cp : text) {
            out += utf8_encode(cp);
        }
        return out;
    }

}  // namespace mpb

#endif  // MPB_UTF8_H
