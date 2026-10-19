// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#pragma once
#ifndef MPB_MPB_H
#define MPB_MPB_H 1

#ifndef MPB_VERSION
#define MPB_VERSION "0.0.0"
#endif

#include <string>

#include "draw_target.h"
#include "format.h"
#include "in_memory.h"
#include "iter.h"
#include "multi.h"
#include "platform.h"
#include "progress_bar.h"
#include "state.h"
#include "style.h"
#include "term_like.h"
#include "types.h"

namespace mpb {

    inline std::string version() {
        return MPB_VERSION;
    }

}  // namespace mpb

#endif  // MPB_MPB_H
