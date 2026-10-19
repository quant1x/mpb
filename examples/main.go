// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// examples 是唯一的 Go 示例入口：go run ./examples <示例名> [参数...]。
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/quant1x/mpb"
)

var exampleNames = []string{
	"cargo", "cargowrap", "download", "download-continued", "download-speed",
	"fastbar", "finebars", "iterator", "log", "long-spinner", "morebars",
	"multi", "multi-tree", "multi-tree-ext", "single", "tokio", "yarnish",
}

func printUsage() {
	sort.Strings(exampleNames)
	fmt.Fprintln(os.Stderr, "usage: go run ./examples <example> [args...]")
	fmt.Fprintln(os.Stderr, "examples:")
	for _, name := range exampleNames {
		fmt.Fprintln(os.Stderr, "  "+name)
	}
}

func main() {
	// Windows 控制台默认按 GBK 解释字节；切到 UTF-8，让 fmt.Print* 输出中文、
	// ✔、emoji 时不乱码。进度条自身的 WriteConsoleW 不依赖这一步。
	restore := mpb.EnableConsoleUTF8()
	defer restore()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	// Accept both `multi-tree` (upstream name) and `multi_tree` (Go file name).
	name := strings.ReplaceAll(os.Args[1], "_", "-")
	args := os.Args[2:]
	switch name {
	case "cargo":
		runCargo()
	case "cargowrap":
		runCargoWrap()
	case "download":
		runDownload()
	case "download-continued":
		runDownloadContinued()
	case "download-speed":
		runDownloadSpeed()
	case "fastbar":
		runFastbar()
	case "finebars":
		runFinebars()
	case "iterator":
		runIterator()
	case "log":
		runLog()
	case "long-spinner":
		runLongSpinner()
	case "morebars":
		runMorebars()
	case "multi":
		runMulti(args)
	case "multi-tree":
		runMultiTree()
	case "multi-tree-ext":
		runMultiTreeExt(args)
	case "single":
		runSingle()
	case "tokio":
		runTokio()
	case "yarnish":
		runYarnish()
	default:
		fmt.Fprintf(os.Stderr, "unknown example %q\n\n", name)
		printUsage()
		os.Exit(2)
	}
}
