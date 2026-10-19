// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// 对应上游 examples/multi-tree.rs：树形排列的 MultiProgress。
#include <chrono>
#include <iostream>
#include <memory>
#include <optional>
#include <random>
#include <string>
#include <thread>
#include <vector>

#include "mpb.h"

namespace {

    struct Elem {
        std::string                       key;
        int                               index;
        int                               indent;
        std::shared_ptr<mpb::ProgressBar> bar;
    };

    std::vector<std::shared_ptr<Elem>> make_elements() {
        return {
            std::make_shared<Elem>(Elem{"jumps", 0, 1, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"lazy", 1, 2, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"the", 0, 0, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"dog", 3, 3, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"over", 2, 2, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"brown", 1, 2, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"quick", 1, 1, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"a", 5, 3, mpb::ProgressBar::make(32)}),
            std::make_shared<Elem>(Elem{"fox", 3, 3, mpb::ProgressBar::make(32)}),
        };
    }

    std::optional<std::pair<bool, std::size_t>> get_action(const std::vector<std::shared_ptr<Elem>> &tree,
                                                           const std::vector<std::shared_ptr<Elem>> &elements,
                                                           std::mt19937                             &rng) {
        std::int64_t total_free = 0;
        for (const auto &elem : tree) {
            const auto length = elem->bar->length().value_or(0);
            total_free += static_cast<std::int64_t>(length) - static_cast<std::int64_t>(elem->bar->position());
        }
        if (total_free == 0 && tree.size() == elements.size()) {
            return std::nullopt;
        }
        if (total_free == 0 && tree.size() < elements.size()) {
            return std::make_pair(true, tree.size());
        }

        while (true) {
            const int k = static_cast<int>(rng() % 17);
            if (k == 0 && tree.size() < elements.size()) {
                return std::make_pair(true, tree.size());
            }
            if (tree.empty()) {
                return std::make_pair(true, 0);
            }
            const std::size_t index = static_cast<std::size_t>(k) % tree.size();
            const auto        bar   = tree[index]->bar;
            if (bar->position() < bar->length().value_or(0)) {
                return std::make_pair(false, index);
            }
        }
    }

    std::string indent(int level) {
        std::string out;
        for (int i = 0; i < level; ++i)
            out += "  ";
        return out;
    }

}  // namespace

int run_multi_tree(int argc, char **argv) {
    (void)argc;
    (void)argv;
    using namespace std::chrono_literals;
    auto elements   = make_elements();
    auto multi      = mpb::MultiProgress::default_instance();
    auto style_main = mpb::progress_style_with_template("{bar:40.green/yellow} {pos:>4}/{len:4}");
    auto style_aux  = mpb::progress_style_with_template("{spinner:.green} {msg} {pos:>4}/{len:4}");

    std::uint64_t total = 0;
    for (const auto &elem : elements) {
        total += elem->bar->length().value_or(0);
    }

    auto main_bar = multi->add(mpb::ProgressBar::make(total));
    main_bar->set_style(style_main);
    for (const auto &elem : elements) {
        elem->bar->set_style(style_aux);
    }

    std::vector<std::shared_ptr<Elem>> tree;
    std::mt19937                       rng(std::random_device{}());
    main_bar->force_draw();

    while (true) {
        std::this_thread::sleep_for(15ms);
        const auto action = get_action(tree, elements, rng);
        if (!action.has_value()) {
            main_bar->finish();
            break;
        }
        const auto [add, index] = *action;
        if (add) {
            const auto elem = elements[index];
            auto       bar  = multi->insert(static_cast<std::uint64_t>(elem->index) + 1, elem->bar);
            bar->set_message(indent(elem->indent) + "  " + elem->key);
            tree.insert(tree.begin() + elem->index, elem);
        } else {
            const auto elem = tree[index];
            elem->bar->inc(1);
            if (elem->bar->position() >= elem->bar->length().value_or(0)) {
                elem->bar->finish_with_message(indent(elem->indent) + "✔ " + elem->key);
            }
            main_bar->inc(1);
        }
    }

    std::cout << "===============================\n";
    std::cout << "the tree should be the same as:\n";
    for (const auto &elem : tree) {
        std::cout << indent(elem->indent) << "  " << elem->key << '\n';
    }
    return 0;
}
