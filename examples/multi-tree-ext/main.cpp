// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/multi-tree-ext.rs：增删节点 + --bottom-alignment。
#include <algorithm>
#include <chrono>
#include <cstdint>
#include <iostream>
#include <memory>
#include <optional>
#include <random>
#include <string>
#include <thread>
#include <vector>

#include "mpb.h"

namespace {

constexpr std::uint64_t kBarLength = 32;

struct Item {
    std::string key;
    int index;
    int indent;
    std::shared_ptr<mpb::ProgressBar> bar;
};

struct Elem {
    bool add = true;
    std::shared_ptr<Item> item;
    int index = 0;
};

Elem add_elem(int indent, int index, const std::string& key) {
    return Elem{true, std::make_shared<Item>(Item{key, index, indent,
                                                  mpb::ProgressBar::make(kBarLength)}),
                0};
}

Elem remove_elem(int index) { return Elem{false, nullptr, index}; }

std::vector<Elem> make_elements() {
    return {
        add_elem(9, 0, "dog"),     add_elem(0, 0, "temp_1"), add_elem(8, 1, "lazy"),
        add_elem(0, 1, "temp_2"),  add_elem(1, 0, "the"),    add_elem(0, 0, "temp_3"),
        add_elem(7, 3, "a"),       add_elem(0, 3, "temp_4"), add_elem(6, 2, "over"),
        remove_elem(6),            remove_elem(4),           remove_elem(3),
        remove_elem(0),            add_elem(0, 2, "temp_5"), add_elem(4, 1, "fox"),
        add_elem(0, 1, "temp_6"),  add_elem(2, 1, "quick"),  add_elem(0, 1, "temp_7"),
        add_elem(5, 5, "jumps"),   add_elem(0, 5, "temp_8"), add_elem(3, 4, "brown"),
        add_elem(0, 3, "temp_9"),  remove_elem(10),          remove_elem(7),
        remove_elem(4),            remove_elem(3),           remove_elem(1),
    };
}

struct Action {
    enum class Kind { stop, modify, inc } kind = Kind::stop;
    std::size_t index = 0;
};

Action get_action(const std::vector<std::shared_ptr<Item>>& items,
                  std::size_t elem_index, const std::vector<Elem>& elements,
                  std::mt19937& rng, std::size_t& next_elem_index) {
    std::vector<std::size_t> uncompleted;
    for (std::size_t i = 0; i < items.size(); ++i) {
        if (items[i]->bar->position() < items[i]->bar->length().value_or(0)) {
            uncompleted.push_back(i);
        }
    }

    const int k = static_cast<int>(rng() % 16);
    if ((k > 0 || (k == 0 && elem_index == elements.size())) && !uncompleted.empty()) {
        const std::size_t index = uncompleted[rng() % uncompleted.size()];
        next_elem_index = elem_index;
        return Action{Action::Kind::inc, index};
    }
    if (elem_index < elements.size()) {
        next_elem_index = elem_index + 1;
        return Action{Action::Kind::modify, elem_index};
    }
    next_elem_index = elem_index;
    return Action{Action::Kind::stop, 0};
}

std::string indent(int level) {
    std::string out;
    for (int i = 0; i < level; ++i) out += "  ";
    return out;
}

std::string green(const std::string& text) { return "\x1b[32m" + text + "\x1b[0m"; }

} // namespace

int main(int argc, char** argv) {
    using namespace std::chrono_literals;
    bool bottom = false;
    for (int i = 1; i < argc; ++i) {
        if (std::string(argv[i]) == "--bottom-alignment") bottom = true;
    }

    auto elements = make_elements();
    auto multi = mpb::MultiProgress::default_instance();
    multi->set_alignment(bottom ? mpb::MultiProgressAlignment::bottom
                                : mpb::MultiProgressAlignment::top);

    auto style_main = mpb::progress_style_with_template("{bar:40.green/yellow} {pos:>4}/{len:4}");
    auto style_aux = mpb::progress_style_with_template(
        "[{pos:>2}/{len:2}] {prefix}{spinner:.green} {msg}");
    auto style_fin = mpb::progress_style_with_template("[{pos:>2}/{len:2}] {prefix}{msg}");

    std::uint64_t total = 0;
    for (const auto& elem : elements) {
        if (elem.add) {
            total += elem.item->bar->length().value_or(0);
        } else {
            total += 1;
        }
    }

    auto main_bar = multi->add(mpb::ProgressBar::make(total));
    main_bar->set_style(style_main);
    for (const auto& elem : elements) {
        if (elem.add) elem.item->bar->set_style(style_aux);
    }

    std::vector<std::shared_ptr<Item>> items;
    std::size_t elem_index = 0;
    std::mt19937 rng(std::random_device{}());
    main_bar->force_draw();

    while (true) {
        const auto action = get_action(items, elem_index, elements, rng, elem_index);
        if (action.kind == Action::Kind::stop) {
            main_bar->finish();
            return 0;
        }

        if (action.kind == Action::Kind::modify) {
            const Elem& elem = elements[action.index];
            if (elem.add) {
                const auto item = elem.item;
                auto bar = multi->insert(static_cast<std::uint64_t>(item->index), item->bar);
                bar->set_prefix(indent(item->indent));
                bar->set_message(item->key);
                items.insert(items.begin() + item->index, item);
            } else {
                const auto item = items[static_cast<std::size_t>(elem.index)];
                items.erase(items.begin() + elem.index);
                multi->remove(item->bar);
                main_bar->inc(item->bar->length().value_or(0) - item->bar->position());
            }
        } else {
            const auto item = items[action.index];
            item->bar->inc(1);
            if (item->bar->position() >= item->bar->length().value_or(0)) {
                item->bar->set_style(style_fin);
                item->bar->finish_with_message(green("✔") + " " + item->key);
            }
            main_bar->inc(1);
        }
        std::this_thread::sleep_for(20ms);
    }
}
