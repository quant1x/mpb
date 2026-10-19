// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use std::cmp::min;

use mpb::{ProgressBar, ProgressState, ProgressStyle};

use crate::support::sleep_ms;

pub fn run() {
    let mut downloaded: u64 = 0;
    let total_size: u64 = 231_231_231;

    let pb = ProgressBar::new(total_size);
    pb.set_style(
        ProgressStyle::with_template(
            "{spinner:.green} [{elapsed_precise}] [{wide_bar:.cyan/blue}] {bytes}/{total_bytes} ({eta})",
        )
        .unwrap()
        .with_key("eta", |state: &ProgressState, w: &mut dyn std::fmt::Write| {
            write!(w, "{:.1}s", state.eta().as_secs_f64()).unwrap()
        })
        .progress_chars("#>-"),
    );

    while downloaded < total_size {
        let new = min(downloaded + 223_211, total_size);
        downloaded = new;
        pb.set_position(new);
        sleep_ms(12);
    }
    pb.finish_with_message("downloaded");
}
