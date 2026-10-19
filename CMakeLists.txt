cmake_minimum_required(VERSION 3.25)

# 版本号取自 git tag（与 mopr 的做法一致）：
# - 正常 git 工作树：使用最近 tag（例如 v0.1.5 -> 0.1.5）
# - 非 git 源码树 / 打包构建：用 -DMPB_VERSION_OVERRIDE=... 显式指定
include(${CMAKE_CURRENT_SOURCE_DIR}/git_tag_info.cmake)
get_latest_git_tag_info(VERSION_FROM_GIT LATEST_TAG TAG_COMMIT_HASH GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_AUTHOR_DATE)

project(mpb
    VERSION ${VERSION_FROM_GIT}
    DESCRIPTION "mpb - multi-language port of Rust indicatif"
    LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

find_package(Threads REQUIRED)

add_library(mpb STATIC
    format.cpp
    style.cpp
    term_like.cpp
    in_memory.cpp
    state.cpp
    draw_target.cpp
    multi.cpp
    progress_bar.cpp
)
add_library(mpb::mpb ALIAS mpb)

target_include_directories(mpb PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(mpb PUBLIC Threads::Threads)
target_compile_definitions(mpb PUBLIC MPB_VERSION="${PROJECT_VERSION}")

if (MSVC)
    target_compile_options(mpb PRIVATE /utf-8 /W3)
    target_compile_definitions(mpb PRIVATE _CRT_SECURE_NO_WARNINGS)
else ()
    target_compile_options(mpb PRIVATE -Wall -Wextra)
endif ()

option(MPB_BUILD_TESTS "Build mpb C++ tests" ON)
if (MPB_BUILD_TESTS)
    enable_testing()
    add_executable(mpb_tests mpb_test.cpp)
    target_link_libraries(mpb_tests PRIVATE mpb)
    if (MSVC)
        target_compile_options(mpb_tests PRIVATE /utf-8)
    endif ()
    add_test(NAME mpb_tests COMMAND mpb_tests)
endif ()
