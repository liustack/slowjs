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
	"ccgo_darwin_amd64.go": "0454350f1824a1bc",
	"ccgo_darwin_arm64.go": "0454350f1824a1bc",
	"ccgo_linux_amd64.go":  "e47f56e4028995ba",
	"ccgo_linux_arm64.go":  "5396c7fd78d07eab",
	"ccgo_windows.go":      "8e635525d90d494f",
}
