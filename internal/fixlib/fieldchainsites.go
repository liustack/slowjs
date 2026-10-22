// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// fieldChainSites are the fingerprints of the functions of fieldChainFuncs
// for each generated file, as fixFieldChain reads them. Update them with go
// run ./internal/fixlib -fieldchain after reading the functions against
// lib/fieldchain.go.
var fieldChainSites = map[string]string{
	"ccgo_darwin_amd64.go":  "a6499bfd30c5317d",
	"ccgo_darwin_arm64.go":  "a6499bfd30c5317d",
	"ccgo_freebsd_amd64.go": "a6499bfd30c5317d",
	"ccgo_freebsd_arm64.go": "a6499bfd30c5317d",
	"ccgo_linux_386.go":     "8d1d49d97ecb41f5",
	"ccgo_linux_amd64.go":   "a6499bfd30c5317d",
	"ccgo_linux_arm.go":     "d885b76b8881717d",
	"ccgo_linux_arm64.go":   "a6499bfd30c5317d",
	"ccgo_linux_loong64.go": "a6499bfd30c5317d",
	"ccgo_linux_ppc64le.go": "a6499bfd30c5317d",
	"ccgo_linux_riscv64.go": "a6499bfd30c5317d",
	"ccgo_linux_s390x.go":   "afa3acfa4f5e3f31",
	"ccgo_windows.go":       "9e1546be5a93d035",
}
