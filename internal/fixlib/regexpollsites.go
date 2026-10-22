// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// regexPollSites are the fingerprints of regexPollFuncs (without the change
// fixRegexPoll makes) in each generated file, as reviewed against
// lib/regexpoll.go (see fixRegexPoll).
var regexPollSites = map[string]string{
	"ccgo_darwin_amd64.go":  "3af0f900b56bee6d",
	"ccgo_darwin_arm64.go":  "3af0f900b56bee6d",
	"ccgo_freebsd_amd64.go": "47581ef712d936d1",
	"ccgo_freebsd_arm64.go": "47581ef712d936d1",
	"ccgo_linux_386.go":     "6a5c2f6e3bfd16b6",
	"ccgo_linux_amd64.go":   "47581ef712d936d1",
	"ccgo_linux_arm.go":     "6a5c2f6e3bfd16b6",
	"ccgo_linux_arm64.go":   "47581ef712d936d1",
	"ccgo_linux_loong64.go": "47581ef712d936d1",
	"ccgo_linux_ppc64le.go": "47581ef712d936d1",
	"ccgo_linux_riscv64.go": "47581ef712d936d1",
	"ccgo_linux_s390x.go":   "47581ef712d936d1",
	"ccgo_windows.go":       "012e5e886c778995",
}
