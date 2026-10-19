// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//! Extension of `multi_tree`: bars can be both inserted and removed, and
//! `--bottom-alignment` selects the vertical alignment.

use console::style;
use mpb::{MultiProgress, MultiProgressAlignment, ProgressBar, ProgressStyle};

use crate::support::{sleep_ms, Rng};

const PB_LEN: u64 = 32;

enum Spec {
    Add(&'static str, usize, usize),
    Remove(usize),
}

const SPECS: [Spec; 27] = [
    Spec::Add("dog", 0, 9),
    Spec::Add("temp_1", 0, 0),
    Spec::Add("lazy", 1, 8),
    Spec::Add("temp_2", 1, 0),
    Spec::Add("the", 0, 1),
    Spec::Add("temp_3", 0, 0),
    Spec::Add("a", 3, 7),
    Spec::Add("temp_4", 3, 0),
    Spec::Add("over", 2, 6),
    Spec::Remove(6),
    Spec::Remove(4),
    Spec::Remove(3),
    Spec::Remove(0),
    Spec::Add("temp_5", 2, 0),
    Spec::Add("fox", 1, 4),
    Spec::Add("temp_6", 1, 0),
    Spec::Add("quick", 1, 2),
    Spec::Add("temp_7", 1, 0),
    Spec::Add("jumps", 5, 5),
    Spec::Add("temp_8", 5, 0),
    Spec::Add("brown", 4, 3),
    Spec::Add("temp_9", 3, 0),
    Spec::Remove(10),
    Spec::Remove(7),
    Spec::Remove(4),
    Spec::Remove(3),
    Spec::Remove(1),
];

struct Elem {
    key: &'static str,
    indent: usize,
    bar: ProgressBar,
}

enum Action {
    ModifyTree(usize),
    IncProgressBar(usize),
    Stop,
}

fn get_action(rng: &mut Rng, next: &mut usize, items: &[&Elem]) -> Action {
    let elem_idx = *next;
    let uncompleted: Vec<usize> = items
        .iter()
        .enumerate()
        .filter(|(_, elem)| elem.bar.position() < elem.bar.length().unwrap())
        .map(|(index, _)| index)
        .collect();

    let k = rng.range(0, 16);
    if (k > 0 || (k == 0 && elem_idx == SPECS.len())) && !uncompleted.is_empty() {
        let idx = rng.range(0, uncompleted.len() as u64) as usize;
        Action::IncProgressBar(uncompleted[idx])
    } else if elem_idx < SPECS.len() {
        *next += 1;
        Action::ModifyTree(elem_idx)
    } else {
        Action::Stop
    }
}

pub fn run(args: &[String]) {
    let bottom_alignment = args.iter().any(|arg| arg == "--bottom-alignment");

    let mp = MultiProgress::new();
    mp.set_alignment(if bottom_alignment {
        MultiProgressAlignment::Bottom
    } else {
        MultiProgressAlignment::Top
    });

    let sty_main = ProgressStyle::with_template("{bar:40.green/yellow} {pos:>4}/{len:4}").unwrap();
    let sty_aux =
        ProgressStyle::with_template("[{pos:>2}/{len:2}] {prefix}{spinner:.green} {msg}").unwrap();
    let sty_fin = ProgressStyle::with_template("[{pos:>2}/{len:2}] {prefix}{msg}").unwrap();

    let total: u64 = SPECS
        .iter()
        .map(|spec| match spec {
            Spec::Add(..) => PB_LEN,
            Spec::Remove(_) => 1,
        })
        .sum();
    let pb_main = mp.add(ProgressBar::new(total));
    pb_main.set_style(sty_main);

    let elems: Vec<Elem> = SPECS
        .iter()
        .map(|spec| match spec {
            Spec::Add(key, _, indent) => Elem {
                key,
                indent: *indent,
                bar: ProgressBar::new(PB_LEN),
            },
            Spec::Remove(_) => Elem {
                key: "",
                indent: 0,
                bar: ProgressBar::new(PB_LEN),
            },
        })
        .collect();
    for (elem, spec) in elems.iter().zip(SPECS.iter()) {
        if matches!(spec, Spec::Add(..)) {
            elem.bar.set_style(sty_aux.clone());
        }
    }

    let mut items: Vec<&Elem> = Vec::with_capacity(elems.len());
    let mut rng = Rng::new(0xBEEF);
    let mut next_elem = 0;
    pb_main.tick();

    loop {
        match get_action(&mut rng, &mut next_elem, &items) {
            Action::Stop => {
                pb_main.finish();
                return;
            }
            Action::ModifyTree(elem_idx) => match &SPECS[elem_idx] {
                Spec::Add(_, index, _) => {
                    let elem = &elems[elem_idx];
                    let pb = mp.insert(*index, elem.bar.clone());
                    pb.set_prefix("  ".repeat(elem.indent));
                    pb.set_message(elem.key);
                    items.insert(*index, elem);
                }
                Spec::Remove(index) => {
                    let elem = items.remove(*index);
                    mp.remove(&elem.bar);
                    pb_main.inc(elem.bar.length().unwrap() - elem.bar.position());
                }
            },
            Action::IncProgressBar(item_idx) => {
                let elem = items[item_idx];
                elem.bar.inc(1);
                if elem.bar.position() >= elem.bar.length().unwrap() {
                    elem.bar.set_style(sty_fin.clone());
                    elem.bar
                        .finish_with_message(format!("{} {}", style("✔").green(), elem.key));
                }
                pb_main.inc(1);
            }
        }
        sleep_ms(20);
    }
}
