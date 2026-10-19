// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use std::cmp::min;

use mpb::{ProgressBar, ProgressStyle};

use crate::support::sleep_ms;

pub fn run() {
    let mut downloaded: u64 = 69_369_369;
    let total_size: u64 = 231_231_231;

    let pb = ProgressBar::new(total_size);
    pb.set_style(
        ProgressStyle::with_template(
            "{spinner:.green} [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})",
        )
        .unwrap()
        .progress_chars("#>-"),
    );
    pb.set_position(downloaded);
    pb.reset_eta();

    while downloaded < total_size {
        downloaded = min(downloaded + 123_211, total_size);
        pb.set_position(downloaded);
        sleep_ms(12);
    }
    pb.finish_with_message("downloaded");
}
