// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// expectSites is the number of tests of the form of __builtin_expect that
// fixExpect changes in each generated file.
var expectSites = map[string]int{
	"ccgo_darwin_amd64.go":  728,
	"ccgo_darwin_arm64.go":  728,
	"ccgo_freebsd_amd64.go": 728,
	"ccgo_freebsd_arm64.go": 728,
	"ccgo_linux_386.go":     862,
	"ccgo_linux_amd64.go":   728,
	"ccgo_linux_arm.go":     862,
	"ccgo_linux_arm64.go":   728,
	"ccgo_linux_loong64.go": 728,
	"ccgo_linux_ppc64le.go": 728,
	"ccgo_linux_riscv64.go": 728,
	"ccgo_linux_s390x.go":   728,
	"ccgo_windows.go":       728,
}
