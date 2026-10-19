// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Shared helpers for the examples: a deterministic RNG (the upstream examples
//! use `rand`) and short sleeps.

use std::thread;
use std::time::Duration;

pub struct Rng {
    state: u64,
}

impl Rng {
    pub fn new(seed: u64) -> Self {
        Self { state: seed }
    }

    pub fn next(&mut self) -> u64 {
        self.state = self
            .state
            .wrapping_mul(6364136223846793005)
            .wrapping_add(1442695040888963407);
        self.state >> 33
    }

    /// `min..max`, upper bound exclusive.
    pub fn range(&mut self, min: u64, max: u64) -> u64 {
        if max <= min {
            min
        } else {
            min + self.next() % (max - min)
        }
    }

    pub fn pick<'a, T>(&mut self, items: &'a [T]) -> &'a T {
        &items[(self.next() as usize) % items.len()]
    }
}

pub fn sleep_ms(ms: u64) {
    thread::sleep(Duration::from_millis(ms));
}
