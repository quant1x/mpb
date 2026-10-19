// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! `mpb` is a zero-implementation alias of [`indicatif`](https://docs.rs/indicatif).
//!
//! The quant1x `mpb` project ports Rust's `indicatif` 0.18.6 to Go, Python, C++,
//! Zig and Rust itself. The other four languages use the package name `mpb`;
//! this crate makes the Rust entry point match them without forking the
//! implementation: every public item below comes straight from `indicatif`, so
//! the semantics are the upstream ones by construction.
//!
//! ```rust
//! use mpb::{MultiProgress, ProgressBar, ProgressStyle};
//!
//! let multi = MultiProgress::new();
//! let style = ProgressStyle::with_template("[{elapsed_precise}] {bar:40.cyan/blue} {pos}/{len}")
//!     .unwrap()
//!     .progress_chars("##-");
//!
//! let bar = multi.add(ProgressBar::new(10));
//! bar.set_style(style);
//! bar.inc(3);
//! bar.finish_with_message("done");
//! ```
//!
//! Anything not listed here (template keys, throttling, `MultiProgress`'s
//! zombie rows, `HumanBytes`, …) behaves exactly as documented by `indicatif`
//! 0.18.6; do not reimplement it in this crate.

#![forbid(unsafe_code)]

pub use indicatif::*;

/// Version of this alias crate (`Cargo.toml`), kept in step with the other
/// language ports.
pub const VERSION: &str = env!("CARGO_PKG_VERSION");

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reexports_indicatif_api() {
        let bar = ProgressBar::hidden();
        bar.set_length(10);
        bar.inc(3);
        assert_eq!(bar.position(), 3);

        let style = ProgressStyle::with_template("{pos}/{len}").unwrap();
        bar.set_style(style);

        assert_eq!(HumanBytes(1500).to_string(), "1.46 KiB");
        assert!(!VERSION.is_empty());
    }
}
