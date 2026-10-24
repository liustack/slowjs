// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// asyncContextSites are the fingerprints of asyncContextFuncs in each
// generated file, after the earlier final changes, as reviewed against
// lib/asynccontext.go (see fixAsyncContext).
var asyncContextSites = map[string]string{
	"ccgo_darwin_amd64.go": "450e01d76fa6a98f",
	"ccgo_darwin_arm64.go": "450e01d76fa6a98f",
	"ccgo_linux_amd64.go":  "8d7f3fe87b1cfa4f",
	"ccgo_linux_arm64.go":  "8d7f3fe87b1cfa4f",
	"ccgo_windows.go":      "9cdaf581f06d6e93",
}
