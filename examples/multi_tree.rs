// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Tree-like collection of progress bars: elements are added on the fly and
//! advanced until every element exists and every bar is finished.

use mpb::{MultiProgress, ProgressBar, ProgressStyle};

use crate::support::{sleep_ms, Rng};

const SPECS: [(&str, usize, usize); 9] = [
    ("jumps", 0, 1),
    ("lazy", 1, 2),
    ("the", 0, 0),
    ("dog", 3, 3),
    ("over", 2, 2),
    ("brown", 1, 2),
    ("quick", 1, 1),
    ("a", 5, 3),
    ("fox", 3, 3),
];

const PB_LEN: u64 = 32;

struct Elem {
    key: &'static str,
    index: usize,
    indent: usize,
    bar: ProgressBar,
}

enum Action {
    Add(usize),
    Inc(usize),
    Done,
}

fn get_action(rng: &mut Rng, tree: &[&Elem]) -> Action {
    let list_len = tree.len();
    let sum_free: u64 = tree
        .iter()
        .map(|elem| elem.bar.length().unwrap() - elem.bar.position())
        .sum();

    if sum_free == 0 && list_len == SPECS.len() {
        return Action::Done;
    }
    if sum_free == 0 && list_len < SPECS.len() {
        return Action::Add(list_len);
    }

    loop {
        let k = rng.range(0, 17);
        if k == 0 && list_len < SPECS.len() {
            return Action::Add(list_len);
        }
        if list_len == 0 {
            continue;
        }
        let pos = (k as usize) % list_len;
        let bar = &tree[pos].bar;
        if bar.position() < bar.length().unwrap() {
            return Action::Inc(pos);
        }
    }
}

pub fn run() {
    let mp = MultiProgress::new();
    let sty_main = ProgressStyle::with_template("{bar:40.green/yellow} {pos:>4}/{len:4}").unwrap();
    let sty_aux = ProgressStyle::with_template("{spinner:.green} {msg} {pos:>4}/{len:4}").unwrap();

    let elems: Vec<Elem> = SPECS
        .iter()
        .map(|&(key, index, indent)| Elem {
            key,
            index,
            indent,
            bar: ProgressBar::new(PB_LEN),
        })
        .collect();
    for elem in &elems {
        elem.bar.set_style(sty_aux.clone());
    }

    let pb_main = mp.add(ProgressBar::new(PB_LEN * SPECS.len() as u64));
    pb_main.set_style(sty_main);

    let mut tree: Vec<&Elem> = Vec::with_capacity(SPECS.len());
    let mut rng = Rng::new(0xA11CE);
    pb_main.tick();

    loop {
        sleep_ms(15);
        match get_action(&mut rng, &tree) {
            Action::Done => {
                pb_main.finish();
                break;
            }
            Action::Add(el) => {
                let elem = &elems[el];
                let pb = mp.insert(elem.index + 1, elem.bar.clone());
                pb.set_message(format!("{}  {}", "  ".repeat(elem.indent), elem.key));
                tree.insert(elem.index, elem);
            }
            Action::Inc(pos) => {
                let elem = tree[pos];
                elem.bar.inc(1);
                if elem.bar.position() >= elem.bar.length().unwrap() {
                    elem.bar.finish_with_message(format!(
                        "{}{} {}",
                        "  ".repeat(elem.indent),
                        "✔",
                        elem.key
                    ));
                }
                pb_main.inc(1);
            }
        }
    }

    println!("===============================");
    println!("the tree should be the same as:");
    for elem in &tree {
        println!("{}  {}", "  ".repeat(elem.indent), elem.key);
    }
}
