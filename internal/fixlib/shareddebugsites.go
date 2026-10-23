// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// sharedDebugSites are the fingerprints of sharedDebugFuncs in each generated
// file, after the earlier final changes, as reviewed against
// lib/shareddebug.go (see fixSharedDebug).
var sharedDebugSites = map[string]string{
	"ccgo_darwin_amd64.go": "8d93fef66744d156",
	"ccgo_darwin_arm64.go": "8d93fef66744d156",
	"ccgo_linux_amd64.go":  "9c105ae00c57c17c",
	"ccgo_linux_arm64.go":  "9c105ae00c57c17c",
	"ccgo_windows.go":      "065340f3e54ddff6",
}
