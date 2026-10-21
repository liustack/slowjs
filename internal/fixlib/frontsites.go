// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// frontSites are the fingerprints of frontFuncs and of JSMallocBlockHeader
// in each generated file, as reviewed against lib/frontcache.go (see
// fixFrontCache).
var frontSites = map[string]string{
	"ccgo_darwin_amd64.go":  "1807640fa342f575",
	"ccgo_darwin_arm64.go":  "1807640fa342f575",
	"ccgo_freebsd_amd64.go": "08774c180aa51dd2",
	"ccgo_freebsd_arm64.go": "08774c180aa51dd2",
	"ccgo_linux_386.go":     "c5cc1e87f7d26592",
	"ccgo_linux_amd64.go":   "e0d24cfc4f16d4be",
	"ccgo_linux_arm.go":     "c5cc1e87f7d26592",
	"ccgo_linux_arm64.go":   "e0d24cfc4f16d4be",
	"ccgo_linux_loong64.go": "e0d24cfc4f16d4be",
	"ccgo_linux_ppc64le.go": "e0d24cfc4f16d4be",
	"ccgo_linux_riscv64.go": "e0d24cfc4f16d4be",
	"ccgo_linux_s390x.go":   "5a7f7ec9fa511e63",
	"ccgo_windows.go":       "fb84c33bc0b98692",
}
