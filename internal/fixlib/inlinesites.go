// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// inlineSites are the fingerprints of the callees of inlineCallees in each
// generated file, as reviewed against fixInline.
var inlineSites = map[string]string{
	"ccgo_darwin_amd64.go":  "95724e6dcb1007b1",
	"ccgo_darwin_arm64.go":  "95724e6dcb1007b1",
	"ccgo_freebsd_amd64.go": "95724e6dcb1007b1",
	"ccgo_freebsd_arm64.go": "95724e6dcb1007b1",
	"ccgo_linux_386.go":     "2e23e36b3a60b33a",
	"ccgo_linux_amd64.go":   "95724e6dcb1007b1",
	"ccgo_linux_arm.go":     "2e23e36b3a60b33a",
	"ccgo_linux_arm64.go":   "95724e6dcb1007b1",
	"ccgo_linux_loong64.go": "95724e6dcb1007b1",
	"ccgo_linux_ppc64le.go": "95724e6dcb1007b1",
	"ccgo_linux_riscv64.go": "95724e6dcb1007b1",
	"ccgo_linux_s390x.go":   "95724e6dcb1007b1",
	"ccgo_windows.go":       "5875549def2db997",
}
