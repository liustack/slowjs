// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// forInSites are the fingerprints of the functions fixForIn depends on
// (without its change) in each generated file, as reviewed against
// lib/forin.go (see fixForIn).
var forInSites = map[string]string{
	"ccgo_darwin_amd64.go":  "5d1b9db979061ea2",
	"ccgo_darwin_arm64.go":  "5d1b9db979061ea2",
	"ccgo_freebsd_amd64.go": "3a37f6873e053594",
	"ccgo_freebsd_arm64.go": "3a37f6873e053594",
	"ccgo_linux_386.go":     "82aac41a438b6996",
	"ccgo_linux_amd64.go":   "06d0db70e4457480",
	"ccgo_linux_arm.go":     "ae30846979210c14",
	"ccgo_linux_arm64.go":   "06d0db70e4457480",
	"ccgo_linux_loong64.go": "06d0db70e4457480",
	"ccgo_linux_ppc64le.go": "06d0db70e4457480",
	"ccgo_linux_riscv64.go": "06d0db70e4457480",
	"ccgo_linux_s390x.go":   "838965d16d8d9351",
	"ccgo_windows.go":       "326cd578141bd752",
}
