// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// jsonSites are the fingerprints of jsonFuncs (without the change
// fixJSONStrings makes) in each generated file, as reviewed against
// lib/jsonstr.go (see fixJSONStrings).
var jsonSites = map[string]string{
	"ccgo_darwin_amd64.go": "1cf2859daf2876db",
	"ccgo_darwin_arm64.go": "1cf2859daf2876db",
	"ccgo_linux_amd64.go":  "a9945843744f53ad",
	"ccgo_linux_arm64.go":  "58b51a203fbfe80c",
	"ccgo_windows.go":      "54cca7a073e119e2",
}
