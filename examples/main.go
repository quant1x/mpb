// Copyright (C) 2026 Quant1X <wangfengxy@sina.cn>
// SPDX-License-Identifier: MIT

// examples 是唯一的 Go 示例入口：go run ./examples <示例名> [参数...]。
package main

import (
	"fmt"
	"os"
	"sort"
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
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	name := os.Args[1]
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
