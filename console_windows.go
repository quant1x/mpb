// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//go:build windows

package mpb

import (
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Windows 控制台默认按 OEM 代码页（中文系统是 GBK）解释 WriteFile 的字节，
// UTF-8 的 █░ / ✔ / emoji 会变成乱码，宽度计算与补白随之错位；控制台还需要
// ENABLE_VIRTUAL_TERMINAL_PROCESSING 才会执行 \x1b[2K / \x1b[nA。
//
// 这里直接调用 kernel32：控制台句柄走 WriteConsoleW（UTF-8 先转 UTF-16），
// 与 Rust std / indicatif 的 console 依赖做法一致。非控制台（管道、文件、
// MSYS pty）继续按字节写入。
var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
	procWriteConsoleW  = kernel32.NewProc("WriteConsoleW")

	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

const enableVirtualTerminalProcessing = 0x0004

// detectConsole 返回控制台句柄；不是控制台（重定向/pty）时返回 0。
func detectConsole(f *os.File) uintptr {
	if f == nil {
		return 0
	}
	var mode uint32
	ok, _, _ := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return 0
	}
	return f.Fd()
}

// enableVirtualTerminal 打开控制台的 VT 序列处理（幂等）。
func enableVirtualTerminal(handle uintptr) {
	if handle == 0 {
		return
	}
	var mode uint32
	ok, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ok == 0 || mode&enableVirtualTerminalProcessing != 0 {
		return
	}
	_, _, _ = procSetConsoleMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
}

// writeConsole 把 UTF-8 文本转成 UTF-16 后交给 WriteConsoleW。
// 返回 false 表示句柄不可用或写入失败，调用方应回退到普通字节写入。
func writeConsole(handle uintptr, text string) bool {
	if handle == 0 || text == "" {
		return false
	}
	wide := utf16.Encode([]rune(text))
	if len(wide) == 0 {
		return true
	}
	var written uint32
	ok, _, _ := procWriteConsoleW.Call(
		handle,
		uintptr(unsafe.Pointer(&wide[0])),
		uintptr(len(wide)),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	return ok != 0
}

// EnableConsoleUTF8 把控制台输出代码页切到 UTF-8，并为 stdout/stderr 打开 VT
// 处理；返回的函数用于恢复原代码页。
//
// 进度条自身已经通过 WriteConsoleW 输出，不依赖代码页；这个函数是给示例里的
// fmt.Print* / os.Stdout 用的，避免中文、✔、emoji 走 WriteFile 时被按 GBK 解码。
func EnableConsoleUTF8() func() {
	enableVirtualTerminal(uintptr(syscall.Stdout))
	enableVirtualTerminal(uintptr(syscall.Stderr))

	current, _, _ := procGetConsoleOutputCP.Call()
	if current == 0 || current == 65001 {
		return func() {}
	}
	if ok, _, _ := procSetConsoleOutputCP.Call(65001); ok == 0 {
		return func() {}
	}
	return func() {
		_, _, _ = procSetConsoleOutputCP.Call(current)
	}
}
