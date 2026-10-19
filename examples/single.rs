// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use mpb::ProgressBar;

use crate::support::sleep_ms;

pub fn run() {
    let pb = ProgressBar::new(1024);
    for _ in 0..1024 {
        sleep_ms(5);
        pb.inc(1);
    }
    pb.finish_with_message("done");
}
