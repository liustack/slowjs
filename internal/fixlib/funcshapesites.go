// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// funcShapeSites are the fingerprints of funcShapeFuncs and of JSShape in
// each generated file, as reviewed against lib/funcshape.go (see
// fixFuncShape).
var funcShapeSites = map[string]string{
	"ccgo_darwin_amd64.go": "c2c010241f217bf7",
	"ccgo_darwin_arm64.go": "c2c010241f217bf7",
	"ccgo_linux_amd64.go":  "2f6f35c13805e41e",
	"ccgo_linux_arm64.go":  "2f6f35c13805e41e",
	"ccgo_windows.go":      "449c771b34d3169e",
}
