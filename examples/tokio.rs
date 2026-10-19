// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use mpb::ProgressBar;

use crate::support::sleep_ms;

pub fn run() {
    // Plain progress bar, totaling 1024 steps.
    let steps: u64 = 1024;
    let pb = ProgressBar::new(steps);

    // The upstream example drives a tokio interval; the same 5ms cadence is
    // expressed here with a plain loop.
    for _ in 0..steps {
        sleep_ms(5);
        pb.inc(1);
    }

    pb.finish();
}
