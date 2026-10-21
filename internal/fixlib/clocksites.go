// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// clockSites are the fingerprints of clockFuncs in each generated file, as
// reviewed against lib/clock.go (see fixClock).
var clockSites = map[string]string{
	"ccgo_darwin_amd64.go":  "ee0e0804b0d92f2b",
	"ccgo_darwin_arm64.go":  "ee0e0804b0d92f2b",
	"ccgo_freebsd_amd64.go": "377a33c0deac0cf5",
	"ccgo_freebsd_arm64.go": "377a33c0deac0cf5",
	"ccgo_linux_386.go":     "5cb5b0359852f896",
	"ccgo_linux_amd64.go":   "41d36c4696591836",
	"ccgo_linux_arm.go":     "5cb5b0359852f896",
	"ccgo_linux_arm64.go":   "41d36c4696591836",
	"ccgo_linux_loong64.go": "41d36c4696591836",
	"ccgo_linux_ppc64le.go": "41d36c4696591836",
	"ccgo_linux_riscv64.go": "41d36c4696591836",
	"ccgo_linux_s390x.go":   "41d36c4696591836",
	"ccgo_windows.go":       "c7b60ca4459254c5",
}
