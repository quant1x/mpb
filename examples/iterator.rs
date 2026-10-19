// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use mpb::{ProgressIterator, ProgressStyle};

use crate::support::sleep_ms;

pub fn run() {
    // Default styling, attempt to use Iterator::size_hint to count input size.
    for _ in (0..1000).progress() {
        sleep_ms(5);
    }

    // Provide explicit number of elements in iterator.
    for _ in (0..1000).progress_count(1000) {
        sleep_ms(5);
    }

    // Provide a custom bar style.
    let pb = mpb::ProgressBar::new(1000);
    pb.set_style(
        ProgressStyle::with_template(
            "{spinner:.green} [{elapsed_precise}] [{bar:40.cyan/blue}] ({pos}/{len}, ETA {eta})",
        )
        .unwrap(),
    );
    for _ in (0..1000).progress_with(pb) {
        sleep_ms(5);
    }
}
