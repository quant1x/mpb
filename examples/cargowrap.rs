// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Spinner + message updates while `cargo build --release` streams its output.

use std::io::{BufRead, BufReader};
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use mpb::{HumanDuration, ProgressBar, ProgressStyle};

pub fn run() {
    let started = Instant::now();

    println!("Compiling package in release mode...");

    let pb = ProgressBar::new_spinner();
    pb.enable_steady_tick(Duration::from_millis(200));
    pb.set_style(
        ProgressStyle::with_template("{spinner:.dim.bold} cargo: {wide_msg}")
            .unwrap()
            .tick_chars("/|\\- "),
    );

    let mut child = Command::new("cargo")
        .arg("build")
        .arg("--release")
        .stderr(Stdio::piped())
        .spawn()
        .expect("failed to start cargo");

    if let Some(stderr) = child.stderr.take() {
        for line in BufReader::new(stderr).lines() {
            let line = line.unwrap_or_default();
            let stripped = line.trim();
            if !stripped.is_empty() {
                pb.set_message(stripped.to_owned());
            }
            pb.tick();
        }
    }

    let _ = child.wait();
    pb.finish_and_clear();

    println!("Done in {}", HumanDuration(started.elapsed()));
}
