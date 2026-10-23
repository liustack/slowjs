// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// regexCacheSites are the fingerprints of regexCacheFuncs in each generated
// file, as reviewed against lib/regexcache.go (see fixRegexCache).
var regexCacheSites = map[string]string{
	"ccgo_darwin_amd64.go": "41aef195cca8604f",
	"ccgo_darwin_arm64.go": "41aef195cca8604f",
	"ccgo_linux_amd64.go":  "41aef195cca8604f",
	"ccgo_linux_arm64.go":  "c07115b4a4689ea5",
	"ccgo_windows.go":      "929d1008a4f1f52b",
}
