// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

//go:build !windows

package mpb

import "os"

// detectConsole 在非 Windows 平台上没有“控制台句柄”概念：终端本身就是字节流，
// UTF-8 直接写入即可。
func detectConsole(*os.File) uintptr { return 0 }

func enableVirtualTerminal(uintptr) {}

func writeConsole(uintptr, string) bool { return false }

// EnableConsoleUTF8 在非 Windows 平台上是空操作（终端默认 UTF-8）。
func EnableConsoleUTF8() func() { return func() {} }
