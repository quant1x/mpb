// Package mpb is a Go port of the Rust `indicatif` crate.
//
// This file contains package-level documentation translated from
// `src/lib.rs` in the original Rust project. The actual implementation
// files will be created alongside this file in the project root, one-to-one
// with the Rust sources under `src/` as requested.
//
// The package aims for a 1:1 functional mapping of the Rust API into Go.
// Public types and functions will mirror the Rust crate's exports.
package mpb

// Re-exported types (to be implemented in other Go files in this package):
// - ProgressDrawTarget
// - BinaryBytes, DecimalBytes, FormattedDuration,
//   HumanBytes, HumanCount, HumanDuration, HumanFloatCount
// - ProgressBarIter, ProgressIterator
// - MultiProgress, MultiProgressAlignment
// - ProgressBar, WeakProgressBar
// - ProgressFinish, ProgressState
// - ProgressStyle
// - TermLike

// See the Rust sources under `src/` for documentation details. The port will
// keep the same names and semantics as closely as possible.
