// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// inlineSites are the fingerprints of the callees of inlineCallees in each
// generated file, as reviewed against fixInline.
var inlineSites = map[string]string{
	"ccgo_darwin_amd64.go": "95724e6dcb1007b1",
	"ccgo_darwin_arm64.go": "95724e6dcb1007b1",
	"ccgo_linux_amd64.go":  "95724e6dcb1007b1",
	"ccgo_linux_arm64.go":  "95724e6dcb1007b1",
	"ccgo_windows.go":      "5875549def2db997",
}
