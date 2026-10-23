// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// searchSites are the fingerprints of searchFuncs (without the change
// fixStringSearch makes) in each generated file, as reviewed against
// lib/stringsearch.go (see fixStringSearch).
var searchSites = map[string]string{
	"ccgo_darwin_amd64.go":  "4580f00e0e6c4bc9",
	"ccgo_darwin_arm64.go":  "4580f00e0e6c4bc9",
	"ccgo_freebsd_amd64.go": "cac8e76b3a068f69",
	"ccgo_freebsd_arm64.go": "cac8e76b3a068f69",
	"ccgo_linux_386.go":     "b81ae035cf065629",
	"ccgo_linux_amd64.go":   "f001508e7b4a3e9e",
	"ccgo_linux_arm.go":     "1a2712ddfea46546",
	"ccgo_linux_arm64.go":   "f001508e7b4a3e9e",
	"ccgo_linux_loong64.go": "f001508e7b4a3e9e",
	"ccgo_linux_ppc64le.go": "f001508e7b4a3e9e",
	"ccgo_linux_riscv64.go": "f001508e7b4a3e9e",
	"ccgo_linux_s390x.go":   "a02b17b44e3bc7a6",
	"ccgo_windows.go":       "6702d8e52c156d3f",
}
