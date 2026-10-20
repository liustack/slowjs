// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
