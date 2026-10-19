// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Example dispatcher: `cargo run --example examples -- <name>`.

mod support;

mod cargo;
mod cargowrap;
mod download;
mod download_continued;
mod download_speed;
mod fastbar;
mod finebars;
mod iterator;
mod log;
mod long_spinner;
mod morebars;
mod multi;
mod multi_tree;
mod multi_tree_ext;
mod single;
mod tokio;
mod yarnish;

const EXAMPLES: [&str; 17] = [
    "single",
    "fastbar",
    "finebars",
    "log",
    "morebars",
    "multi",
    "multi_tree",
    "multi_tree_ext",
    "cargo",
    "cargowrap",
    "download",
    "download_continued",
    "download_speed",
    "iterator",
    "long_spinner",
    "tokio",
    "yarnish",
];

fn usage() {
    println!("usage: cargo run --example examples -- <example> [args]");
    println!("examples: {}", EXAMPLES.join(" "));
}

fn main() {
    let mut args = std::env::args().skip(1);
    let name = match args.next() {
        Some(name) => name,
        None => {
            usage();
            return;
        }
    };
    let rest: Vec<String> = args.collect();

    match name.as_str() {
        "single" => single::run(),
        "fastbar" => fastbar::run(),
        "finebars" => finebars::run(),
        "log" => log::run(),
        "morebars" => morebars::run(),
        "multi" => multi::run(),
        "multi_tree" => multi_tree::run(),
        "multi_tree_ext" => multi_tree_ext::run(&rest),
        "cargo" => cargo::run(),
        "cargowrap" => cargowrap::run(),
        "download" => download::run(),
        "download_continued" => download_continued::run(),
        "download_speed" => download_speed::run(),
        "iterator" => iterator::run(),
        "long_spinner" => long_spinner::run(),
        "tokio" => tokio::run(),
        "yarnish" => yarnish::run(),
        _ => usage(),
    }
}
