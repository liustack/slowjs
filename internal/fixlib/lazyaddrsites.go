// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// lazyAddrSites are the fingerprints of the functions that get an address
// in a TLS frame fixLazyFrames keeps (see lazyAddrFingerprint) in each
// generated file, as reviewed against addrCallees.
var lazyAddrSites = map[string]string{
	"ccgo_darwin_amd64.go": "4ecc9889c311e3d7",
	"ccgo_darwin_arm64.go": "4ecc9889c311e3d7",
	"ccgo_linux_amd64.go":  "18bc0306172e0b8d",
	"ccgo_linux_arm64.go":  "c8e6114f5c1630f5",
	"ccgo_windows.go":      "efc484537352fd85",
}
