// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

use std::thread;
use std::time::Instant;

use console::{style, Emoji};
use mpb::{HumanDuration, MultiProgress, ProgressBar, ProgressStyle};

use crate::support::{sleep_ms, Rng};

const PACKAGES: [&str; 6] = [
    "fs-events",
    "my-awesome-module",
    "emoji-speaker",
    "wrap-ansi",
    "stream-browserify",
    "acorn-dynamic-import",
];

const COMMANDS: [&str; 8] = [
    "cmake .",
    "make",
    "make clean",
    "gcc foo.c -o foo",
    "gcc bar.c -o bar",
    "./helper.sh rebuild-cache",
    "make all-clean",
    "make test",
];

static LOOKING_GLASS: Emoji<'_, '_> = Emoji("🔍  ", "");
static TRUCK: Emoji<'_, '_> = Emoji("🚚  ", "");
static CLIP: Emoji<'_, '_> = Emoji("🔗  ", "");
static PAPER: Emoji<'_, '_> = Emoji("📃  ", "");
static SPARKLE: Emoji<'_, '_> = Emoji("✨ ", ":-)");

pub fn run() {
    let started = Instant::now();
    let mut rng = Rng::new(0x7A11);

    let spinner_style = ProgressStyle::with_template("{prefix:.bold.dim} {spinner} {wide_msg}")
        .unwrap()
        .tick_chars("⠁⠂⠄⡀⢀⠠⠐⠈ ");

    println!(
        "{} {}Resolving packages...",
        style("[1/4]").bold().dim(),
        LOOKING_GLASS
    );
    println!(
        "{} {}Fetching packages...",
        style("[2/4]").bold().dim(),
        TRUCK
    );
    println!(
        "{} {}Linking dependencies...",
        style("[3/4]").bold().dim(),
        CLIP
    );

    let deps: u64 = 1232;
    let pb = ProgressBar::new(deps);
    for _ in 0..deps {
        sleep_ms(3);
        pb.inc(1);
    }
    pb.finish_and_clear();

    println!(
        "{} {}Building fresh packages...",
        style("[4/4]").bold().dim(),
        PAPER
    );

    let m = MultiProgress::new();
    let handles: Vec<_> = (0..4u32)
        .map(|i| {
            let count = rng.range(30, 80);
            let pb = m.add(ProgressBar::new(count));
            pb.set_style(spinner_style.clone());
            pb.set_prefix(format!("[{}/?]", i + 1));
            thread::spawn(move || {
                let mut rng = Rng::new(0x1000 + u64::from(i));
                let package = *rng.pick(&PACKAGES);
                for _ in 0..count {
                    let command = *rng.pick(&COMMANDS);
                    sleep_ms(rng.range(25, 200));
                    pb.set_message(format!("{package}: {command}"));
                    pb.inc(1);
                }
                pb.finish_with_message("waiting...");
            })
        })
        .collect();

    for handle in handles {
        let _ = handle.join();
    }
    m.clear().unwrap();

    println!("{} Done in {}", SPARKLE, HumanDuration(started.elapsed()));
}
