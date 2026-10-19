// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use std::thread;
use std::time::Duration;

use mpb::{MultiProgress, ProgressBar, ProgressStyle};

use crate::support::Rng;

const STYLES: [(&str, &str, &str); 5] = [
    ("Rough bar:", "█  ", "red"),
    ("Fine bar: ", "█▉▊▋▌▍▎▏  ", "yellow"),
    ("Vertical: ", "█▇▆▅▄▃▂▁  ", "green"),
    ("Fade in:  ", "█▓▒░  ", "blue"),
    ("Blocky:   ", "█▛▌▖  ", "magenta"),
];

pub fn run() {
    let m = MultiProgress::new();

    let handles: Vec<_> = STYLES
        .iter()
        .enumerate()
        .map(|(index, spec)| {
            let pb = m.add(ProgressBar::new(512));
            pb.set_style(
                ProgressStyle::with_template(&format!(
                    "{{prefix:.bold}}▕{{bar:.{}}}▏{{msg}}",
                    spec.2
                ))
                .unwrap()
                .progress_chars(spec.1),
            );
            pb.set_prefix(spec.0);

            thread::spawn(move || {
                let mut rng = Rng::new(0x1000 + index as u64);
                let wait = Duration::from_millis(rng.range(10, 30));
                for i in 0..512 {
                    thread::sleep(wait);
                    pb.inc(1);
                    pb.set_message(format!("{:3}%", 100 * i / 512));
                }
                pb.finish_with_message("100%");
            })
        })
        .collect();

    for handle in handles {
        let _ = handle.join();
    }
}
