// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// frontSites are the fingerprints of frontFuncs and of JSMallocBlockHeader
// in each generated file, as reviewed against lib/frontcache.go (see
// fixFrontCache).
var frontSites = map[string]string{
	"ccgo_darwin_amd64.go":  "dfe42adb332ba38e",
	"ccgo_darwin_arm64.go":  "dfe42adb332ba38e",
	"ccgo_freebsd_amd64.go": "8f584b9f5e536d48",
	"ccgo_freebsd_arm64.go": "8f584b9f5e536d48",
	"ccgo_linux_386.go":     "983aa05ae0a7689c",
	"ccgo_linux_amd64.go":   "7514d02b4c4675bb",
	"ccgo_linux_arm.go":     "983aa05ae0a7689c",
	"ccgo_linux_arm64.go":   "7514d02b4c4675bb",
	"ccgo_linux_loong64.go": "7514d02b4c4675bb",
	"ccgo_linux_ppc64le.go": "7514d02b4c4675bb",
	"ccgo_linux_riscv64.go": "7514d02b4c4675bb",
	"ccgo_linux_s390x.go":   "50e244e346fdf1b9",
	"ccgo_windows.go":       "f23e8c3998c58a1c",
}
