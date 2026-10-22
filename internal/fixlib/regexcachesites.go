// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// regexCacheSites are the fingerprints of regexCacheFuncs in each generated
// file, as reviewed against lib/regexcache.go (see fixRegexCache).
var regexCacheSites = map[string]string{
	"ccgo_darwin_amd64.go":  "41aef195cca8604f",
	"ccgo_darwin_arm64.go":  "41aef195cca8604f",
	"ccgo_freebsd_amd64.go": "1ee33a78252a95a9",
	"ccgo_freebsd_arm64.go": "28195d2a013d277f",
	"ccgo_linux_386.go":     "27779da6b8e29a28",
	"ccgo_linux_amd64.go":   "41aef195cca8604f",
	"ccgo_linux_arm.go":     "428d95dd8fc3fddf",
	"ccgo_linux_arm64.go":   "c07115b4a4689ea5",
	"ccgo_linux_loong64.go": "41aef195cca8604f",
	"ccgo_linux_ppc64le.go": "c07115b4a4689ea5",
	"ccgo_linux_riscv64.go": "c07115b4a4689ea5",
	"ccgo_linux_s390x.go":   "5e1703ddd9d729c1",
	"ccgo_windows.go":       "929d1008a4f1f52b",
}
