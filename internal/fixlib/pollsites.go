// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// pollSites are the fingerprints of pollFuncs in each generated file, as
// reviewed against lib/poll.go (see fixPoll).
var pollSites = map[string]string{
	"ccgo_darwin_amd64.go":  "57f8cd868914a1cc",
	"ccgo_darwin_arm64.go":  "57f8cd868914a1cc",
	"ccgo_freebsd_amd64.go": "57f8cd868914a1cc",
	"ccgo_freebsd_arm64.go": "57f8cd868914a1cc",
	"ccgo_linux_386.go":     "aae03a18c110f619",
	"ccgo_linux_amd64.go":   "57f8cd868914a1cc",
	"ccgo_linux_arm.go":     "d283d8a39fb4c4e1",
	"ccgo_linux_arm64.go":   "57f8cd868914a1cc",
	"ccgo_linux_loong64.go": "57f8cd868914a1cc",
	"ccgo_linux_ppc64le.go": "57f8cd868914a1cc",
	"ccgo_linux_riscv64.go": "57f8cd868914a1cc",
	"ccgo_linux_s390x.go":   "57f8cd868914a1cc",
	"ccgo_windows.go":       "f4b85f4fe73909a9",
}
