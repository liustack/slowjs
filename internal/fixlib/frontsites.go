// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// frontSites are the fingerprints of frontFuncs and of JSMallocBlockHeader
// in each generated file, as reviewed against lib/frontcache.go (see
// fixFrontCache).
var frontSites = map[string]string{
	"ccgo_darwin_amd64.go": "dfe42adb332ba38e",
	"ccgo_darwin_arm64.go": "dfe42adb332ba38e",
	"ccgo_linux_amd64.go":  "7514d02b4c4675bb",
	"ccgo_linux_arm64.go":  "7514d02b4c4675bb",
	"ccgo_windows.go":      "f23e8c3998c58a1c",
}
