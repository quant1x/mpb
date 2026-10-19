// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Mimics cargo's build progress: a shared bar, "Compiling" log lines and a
//! message listing the crates that are currently being built.

use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Instant;

use console::{style, Term};
use mpb::{HumanDuration, ProgressBar, ProgressStyle};

use crate::support::{sleep_ms, Rng};

const CRATES: [(&str, &str); 25] = [
    ("console", "v0.14.1"),
    ("lazy_static", "v1.4.0"),
    ("libc", "v0.2.93"),
    ("regex", "v1.4.6"),
    ("regex-syntax", "v0.6.23"),
    ("terminal_size", "v0.1.16"),
    ("libc", "v0.2.93"),
    ("unicode-width", "v0.1.8"),
    ("lazy_static", "v1.4.0"),
    ("number_prefix", "v0.4.0"),
    ("regex", "v1.4.6"),
    ("rand", "v0.8.3"),
    ("getrandom", "v0.2.2"),
    ("cfg-if", "v1.0.0"),
    ("libc", "v0.2.93"),
    ("rand_chacha", "v0.3.0"),
    ("ppv-lite86", "v0.2.10"),
    ("rand_core", "v0.6.2"),
    ("getrandom", "v0.2.2"),
    ("rand_core", "v0.6.2"),
    ("tokio", "v1.5.0"),
    ("bytes", "v1.0.1"),
    ("pin-project-lite", "v0.2.6"),
    ("slab", "v0.4.3"),
    ("indicatif", "v0.15.0"),
];

const NUM_CPUS: usize = 4;

struct Shared {
    next: usize,
    processing: [Option<usize>; NUM_CPUS],
    rng: Rng,
}

pub fn run() {
    let started = Instant::now();

    let template = if Term::stdout().size().1 > 80 {
        "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len} {wide_msg}"
    } else {
        "{prefix:>12.cyan.bold} [{bar:57}] {pos}/{len}"
    };
    let sty = ProgressStyle::with_template(template)
        .unwrap()
        .progress_chars("=> ");

    let pb = ProgressBar::new(CRATES.len() as u64);
    pb.set_style(sty);
    pb.set_prefix("Building");

    let shared = Arc::new(Mutex::new(Shared {
        next: 0,
        processing: [None; NUM_CPUS],
        rng: Rng::new(0xC0FFEE),
    }));

    let mut handles = Vec::with_capacity(NUM_CPUS);
    for slot in 0..NUM_CPUS {
        let pb = pb.clone();
        let shared = Arc::clone(&shared);
        handles.push(thread::spawn(move || worker(pb, shared, slot)));
    }
    for handle in handles {
        let _ = handle.join();
    }
    pb.finish_and_clear();

    println!(
        "{:>12} dev [unoptimized + debuginfo] target(s) in {}",
        style("Finished").green().bold(),
        HumanDuration(started.elapsed())
    );
}

fn worker(pb: ProgressBar, shared: Arc<Mutex<Shared>>, slot: usize) {
    loop {
        let (index, wait, message) = {
            let mut shared = shared.lock().unwrap();
            if shared.next >= CRATES.len() {
                break;
            }
            let index = shared.next;
            shared.next += 1;
            shared.processing[slot] = Some(index);

            let message = shared
                .processing
                .iter()
                .flatten()
                .map(|&current| CRATES[current].0)
                .collect::<Vec<_>>()
                .join(", ");

            // Last compile and linking is always slow, let's mimic that.
            let wait = if index + 1 == CRATES.len() {
                shared.rng.range(1000, 2000)
            } else {
                shared.rng.range(250, 1000)
            };
            (index, wait, message)
        };

        pb.set_message(message);
        let (name, version) = CRATES[index];
        pb.println(format!(
            "{:>12} {} {}",
            style("Compiling").green().bold(),
            name,
            version
        ));

        sleep_ms(wait);
        pb.inc(1);
    }
}
