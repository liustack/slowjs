// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// lazyAddrSites are the fingerprints of the functions that get an address
// in a TLS frame fixLazyFrames keeps (see lazyAddrFingerprint) in each
// generated file, as reviewed against addrCallees.
var lazyAddrSites = map[string]string{
	"ccgo_darwin_amd64.go":  "4ecc9889c311e3d7",
	"ccgo_darwin_arm64.go":  "4ecc9889c311e3d7",
	"ccgo_freebsd_amd64.go": "18bc0306172e0b8d",
	"ccgo_freebsd_arm64.go": "c8e6114f5c1630f5",
	"ccgo_linux_386.go":     "ac1c3aed2151027b",
	"ccgo_linux_amd64.go":   "18bc0306172e0b8d",
	"ccgo_linux_arm.go":     "17931b6ef44f515d",
	"ccgo_linux_arm64.go":   "c8e6114f5c1630f5",
	"ccgo_linux_loong64.go": "18bc0306172e0b8d",
	"ccgo_linux_ppc64le.go": "c8e6114f5c1630f5",
	"ccgo_linux_riscv64.go": "c8e6114f5c1630f5",
	"ccgo_linux_s390x.go":   "152814712d9266de",
	"ccgo_windows.go":       "efc484537352fd85",
}
