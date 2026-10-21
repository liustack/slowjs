// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// atomKeySites are the fingerprints of the functions of atomKeyFuncs for
// each generated file, as fixAtomKey reads them. Update them with go run
// ./internal/fixlib -atomkey after reading the functions against
// lib/atomkey.go.
var atomKeySites = map[string]string{
	"ccgo_darwin_amd64.go":  "0454350f1824a1bc",
	"ccgo_darwin_arm64.go":  "0454350f1824a1bc",
	"ccgo_freebsd_amd64.go": "12e6b4065b0d2695",
	"ccgo_freebsd_arm64.go": "e2a96ce2322fe1fe",
	"ccgo_linux_386.go":     "5a6da3d6b9cb8765",
	"ccgo_linux_amd64.go":   "e47f56e4028995ba",
	"ccgo_linux_arm.go":     "3e8de2a0c51e1441",
	"ccgo_linux_arm64.go":   "5396c7fd78d07eab",
	"ccgo_linux_loong64.go": "e47f56e4028995ba",
	"ccgo_linux_ppc64le.go": "5396c7fd78d07eab",
	"ccgo_linux_riscv64.go": "5396c7fd78d07eab",
	"ccgo_linux_s390x.go":   "9ae8c182a535bd36",
	"ccgo_windows.go":       "8e635525d90d494f",
}
