// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

#include "mpb.h"

#include <chrono>
#include <cmath>
#include <cstdlib>
#include <iostream>
#include <limits>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

namespace {

int count_substring(const std::string& text, const std::string& needle) {
    int count = 0;
    std::size_t pos = 0;
    while ((pos = text.find(needle, pos)) != std::string::npos) {
        ++count;
        pos += needle.size();
    }
    return count;
}

void require(bool condition, const char* message) {
    if (!condition) {
        std::cerr << "FAIL: " << message << '\n';
        std::exit(1);
    }
}

void test_format() {
    require(mpb::human_bytes(1536) == "1.50 KiB", "human_bytes");
    require(mpb::decimal_bytes(1500) == "1.50 kB", "decimal_bytes");
    require(mpb::human_count(1234567) == "1,234,567", "human_count");
    require(mpb::human_float_count(1234.5678, 2) == "1,234.57", "human_float_count");
    require(mpb::human_duration(std::chrono::seconds(5)) == "5 seconds",
            "human_duration");
    require(mpb::human_duration(std::chrono::seconds(5), true) == "5s",
            "human_duration alt");
    require(mpb::formatted_duration(std::chrono::seconds(3661)) == "01:01:01",
            "formatted_duration");
    require(mpb::human_float_count(std::numeric_limits<double>::infinity()) == "inf",
            "inf formatting");
    require(mpb::human_float_count(-std::numeric_limits<double>::infinity()) == "-inf",
            "negative inf formatting");
}

void test_width_and_style() {
    mpb::Width::set_default_cjk(false);
    require(mpb::Width::char_width(U'█') == 1, "ambiguous width default");
    require(mpb::Width::char_width(U'あ') == 2, "wide char width");
    mpb::Width::set_default_cjk(true);
    require(mpb::Width::char_width(U'█') == 2, "ambiguous width cjk");
    mpb::Width::set_default_cjk(false);

    mpb::Style style{"italic", {}};
    require(style.apply_for("x") == "\x1b[3mx\x1b[0m", "italic style");
    require(mpb::Width::str_width("👨\u200d👩") == 2, "emoji zwj width");
    require(mpb::Width::str_width("👨\u200d👩\u200d👧") == 2, "emoji multi zwj width");
    require(mpb::Width::str_width("🏳️\u200d🌈") == 2, "emoji flag zwj width");
    require(mpb::Width::str_width("❤️") == 2, "emoji presentation width");
    require(mpb::Width::str_width("1️⃣") == 2, "keycap width");
    require(mpb::Width::str_width("🇨🇳") == 2, "flag width");
    require(mpb::Width::str_width("a\u0301") == 1, "combining width");
}

void test_single_bar() {
    auto term = std::make_shared<mpb::InMemoryTerm>(10, 80);
    auto pb = mpb::ProgressBar::with_draw_target(10, mpb::in_memory_draw_target(term));
    pb->set_style(mpb::default_progress_style_bar());
    pb->inc(3);
    pb->force_draw();
    require(term->contents().find("3/10") != std::string::npos, "single bar render");
    pb->abandon();
}

void test_wide_bar() {
    auto term = std::make_shared<mpb::InMemoryTerm>(24, 165);
    auto pb = mpb::ProgressBar::with_draw_target(std::nullopt,
                                                 mpb::in_memory_draw_target(term));
    pb->set_length(1024);
    pb->set_style(mpb::progress_style_with_template("{wide_bar} {pos:>4}/{len}"));
    pb->set_position(651);
    pb->force_draw();
    const std::string line = term->contents();
    const int bars = count_substring(line, "█") + count_substring(line, "░");
    require(bars == 155, "wide bar fills terminal");
    require(line.find("651/1024") != std::string::npos, "wide bar counter");
    pb->abandon();
}

void test_multi_and_drop() {
    auto term = std::make_shared<mpb::InMemoryTerm>(10, 60);
    auto mp = mpb::MultiProgress::with_draw_target(mpb::in_memory_draw_target(term));
    auto p1 = mp->add(mpb::ProgressBar::make(10));
    auto p2 = mp->add(mpb::ProgressBar::make(20));
    p1->inc(3);
    p2->inc(5);
    const std::string text = term->contents();
    require(text.find("3/10") != std::string::npos, "multi first bar");
    require(text.find("5/20") != std::string::npos, "multi second bar");

    mp->println("hello");
    require(term->contents().find("hello") != std::string::npos, "multi println");

    p1.reset();
    require(term->contents().find("3/10") == std::string::npos,
            "dropped bar should be cleared");
    require(mp->state->ordering.size() == 1, "dropped bar should be reaped");
    p2->abandon();
}

void test_template_and_style_copy() {
    auto base = mpb::progress_style_with_template("{pos}");
    auto changed = base.with_template("{len}");
    require(base.template_value().parts[0].key == "pos", "style with_template copies");
    require(changed.template_value().parts[0].key == "len", "style with_template result");

    const auto overflow = mpb::template_from_str("{a:>9999999999999999999999999999}");
    require(overflow.parts[0].width == 0, "template width overflow");

    require(mpb::measure_text_width("\x1b[31m█\x1b[0m") == 1, "ansi width");
    require(mpb::measure_text_width("１") == 2, "fullwidth width");

    auto position = std::make_shared<mpb::AtomicPosition>();
    mpb::ProgressState state(10, position);
    state.set_message(mpb::TabExpandedString{"abcdef", 8});
    auto style = mpb::progress_style_with_template("{msg:>4!}");
    auto [types, contents] = style.format_state(state, 80);
    (void)types;
    require(contents.size() == 1 && contents[0] == "cdef", "message truncation");
}

void test_multi_ordering_and_clear() {
    auto term = std::make_shared<mpb::InMemoryTerm>(10, 60);
    auto mp = mpb::MultiProgress::with_draw_target(mpb::in_memory_draw_target(term));
    auto p1 = mp->add(mpb::ProgressBar::make(10));
    auto p2 = mp->add(mpb::ProgressBar::make(10));
    auto p3 = mp->insert_before(p2, mpb::ProgressBar::make(5));
    auto p4 = mp->insert_after(p1, mpb::ProgressBar::make(5));

    require(p1->visual_index() == 0, "visual index p1");
    require(p4->visual_index() == 1, "visual index p4");
    require(p3->visual_index() == 2, "visual index p3");
    require(p2->visual_index() == 3, "visual index p2");

    p1->inc(3);
    p2->inc(4);
    mp->remove(p1);
    p2->inc(1);
    require(term->contents().find("3/10") == std::string::npos, "removed bar gone");
    require(term->contents().find("5/10") != std::string::npos, "remaining bar");

    bool empty_inside = false;
    mp->suspend([&] { empty_inside = term->contents().empty(); });
    require(empty_inside, "suspend clears");
    require(term->contents().find("5/10") != std::string::npos, "suspend redraws");

    mp->clear();
    require(term->contents().empty(), "multi clear");
    p2->abandon();
    p3->abandon();
    p4->abandon();
}

void test_iterators_and_seek() {
    auto pb = mpb::ProgressBar::make(3);
    mpb::ProgressBarIter<int> it(std::vector<int>{1, 2, 3}, pb);
    int value = 0;
    require(it.next(value) && value == 1, "iter next 1");
    require(it.next_back(value) && value == 3, "iter next_back");
    require(it.next(value) && value == 2, "iter next 2");
    require(!it.next(value), "iter exhausted");
    require(it.len() == 0, "iter len");
    require(pb->is_finished(), "iter finishes bar");

    mpb::SeekMax seek;
    require(seek.update_seq(10, 5) == 15, "seek inactive");
    require(seek.update_seek(100) == 100, "seek update");
    require(seek.update_seq(100, 1) == 101, "seek max 101");
    require(seek.update_seq(50, 1) == 101, "seek keeps max");

    auto writer_bar = mpb::ProgressBar::make(100);
    std::ostringstream output;
    mpb::ProgressWriter writer(output, writer_bar);
    writer.write("hello");
    require(writer_bar->position() == 5, "writer position");
    require(output.str() == "hello", "writer output");

    auto parallel_bar = mpb::ProgressBar::make(10);
    mpb::ProgressBarIter<int> parallel(std::vector<int>{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
                                       parallel_bar);
    const auto mapped = mpb::map_parallel(
        parallel, 4, [](const int& value) { return value * value; });
    require(mapped.size() == 10 && mapped[3] == 9 && mapped[9] == 81,
            "map_parallel order");
    require(parallel_bar->position() == 10, "map_parallel position");
    require(parallel.len() == 0, "map_parallel consumed");
    require(!parallel_bar->is_finished(), "map_parallel does not finish");
}

void test_in_memory_and_limiter() {
    mpb::InMemoryTerm term(5, 10);
    term.write_str("abcdef");
    term.move_cursor_left(3);
    term.clear_line();
    require(term.contents().empty(), "in-memory clear line");

    mpb::InMemoryTerm formatted(2, 10);
    formatted.write_str("ab");
    require(formatted.contents_formatted() == "ab\x1b[m", "formatted contents");

    mpb::RateLimiter limiter(20);
    const auto base = std::chrono::steady_clock::now();
    int allowed = 0;
    for (int i = 0; i < 1000; ++i) {
        if (limiter.allow(base + std::chrono::milliseconds(i))) {
            ++allowed;
        }
    }
    require(allowed >= 30 && allowed <= 50, "rate limiter 20Hz");
}

} // namespace

int main() {
    test_format();
    test_width_and_style();
    test_single_bar();
    test_wide_bar();
    test_multi_and_drop();
    test_template_and_style_copy();
    test_multi_ordering_and_clear();
    test_iterators_and_seek();
    test_in_memory_and_limiter();
    std::cout << "mpb C++ tests passed\n";
    return 0;
}
