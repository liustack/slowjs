// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// searchSites are the fingerprints of searchFuncs (without the change
// fixStringSearch makes) in each generated file, as reviewed against
// lib/stringsearch.go (see fixStringSearch).
var searchSites = map[string]string{
	"ccgo_darwin_amd64.go": "4580f00e0e6c4bc9",
	"ccgo_darwin_arm64.go": "4580f00e0e6c4bc9",
	"ccgo_linux_amd64.go":  "f001508e7b4a3e9e",
	"ccgo_linux_arm64.go":  "f001508e7b4a3e9e",
	"ccgo_windows.go":      "6702d8e52c156d3f",
}
