// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// youngSites are the fingerprints of youngFuncs in each generated file, after
// the earlier final changes, as reviewed against lib/younggc.go (see
// fixYoungGC).
var youngSites = map[string]string{
	"ccgo_darwin_amd64.go":  "f9e6dc90efc27ef6",
	"ccgo_darwin_arm64.go":  "f9e6dc90efc27ef6",
	"ccgo_freebsd_amd64.go": "28e2cce123fc38ea",
	"ccgo_freebsd_arm64.go": "28e2cce123fc38ea",
	"ccgo_linux_386.go":     "5673419192716258",
	"ccgo_linux_amd64.go":   "f87da3270130daa2",
	"ccgo_linux_arm.go":     "d9c29241cd33325c",
	"ccgo_linux_arm64.go":   "f87da3270130daa2",
	"ccgo_linux_loong64.go": "f87da3270130daa2",
	"ccgo_linux_ppc64le.go": "f87da3270130daa2",
	"ccgo_linux_riscv64.go": "f87da3270130daa2",
	"ccgo_linux_s390x.go":   "bc8ffacbe3c7ec74",
	"ccgo_windows.go":       "c793dd7528858518",
}
