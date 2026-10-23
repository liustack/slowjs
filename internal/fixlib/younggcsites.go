// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// youngSites are the fingerprints of youngFuncs in each generated file, after
// the earlier final changes, as reviewed against lib/younggc.go (see
// fixYoungGC).
var youngSites = map[string]string{
	"ccgo_darwin_amd64.go": "f9e6dc90efc27ef6",
	"ccgo_darwin_arm64.go": "f9e6dc90efc27ef6",
	"ccgo_linux_amd64.go":  "f87da3270130daa2",
	"ccgo_linux_arm64.go":  "f87da3270130daa2",
	"ccgo_windows.go":      "c793dd7528858518",
}
