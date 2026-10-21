// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// backtraceSites are the fingerprints of backtraceFuncs (without the calls
// fixBacktrace adds) in each generated file, as reviewed against
// lib/backtrace.go (see checkBacktraceSites).
var backtraceSites = map[string]string{
	"ccgo_darwin_amd64.go":  "9c5bd3cecb1c7fc2",
	"ccgo_darwin_arm64.go":  "9c5bd3cecb1c7fc2",
	"ccgo_freebsd_amd64.go": "b7256d232e5c9dc3",
	"ccgo_freebsd_arm64.go": "9e8273f110efcff9",
	"ccgo_linux_386.go":     "46ae829e931abff0",
	"ccgo_linux_amd64.go":   "b7256d232e5c9dc3",
	"ccgo_linux_arm.go":     "bede4a6eac152b81",
	"ccgo_linux_arm64.go":   "9e8273f110efcff9",
	"ccgo_linux_loong64.go": "b7256d232e5c9dc3",
	"ccgo_linux_ppc64le.go": "9e8273f110efcff9",
	"ccgo_linux_riscv64.go": "9e8273f110efcff9",
	"ccgo_linux_s390x.go":   "6ca053c9d1d7fdd7",
	"ccgo_windows.go":       "99d4ad4e92326116",
}
