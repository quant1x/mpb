// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use mpb::ProgressBar;

use crate::support::sleep_ms;

pub fn run() {
    let pb = ProgressBar::new(100);
    for i in 0..100 {
        sleep_ms(25);
        pb.println(format!("[+] finished #{i}"));
        pb.inc(1);
    }
    pb.finish_with_message("done");
}
