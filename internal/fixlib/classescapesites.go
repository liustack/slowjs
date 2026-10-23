// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// classEscapeSites are the fingerprints of classEscapeFuncs (without the
// change fixClassEscapes makes) in each generated file, as reviewed (see
// fixClassEscapes).
var classEscapeSites = map[string]string{
	"ccgo_darwin_amd64.go": "b78dde975f924e87",
	"ccgo_darwin_arm64.go": "b78dde975f924e87",
	"ccgo_linux_amd64.go":  "b78dde975f924e87",
	"ccgo_linux_arm64.go":  "b78dde975f924e87",
	"ccgo_windows.go":      "aea57f25b70a0be3",
}
