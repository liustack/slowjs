// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// lazyAddrSites are the fingerprints of the functions that get an address
// in a TLS frame fixLazyFrames keeps (see lazyAddrFingerprint) in each
// generated file, as reviewed against addrCallees.
var lazyAddrSites = map[string]string{
	"ccgo_darwin_amd64.go":  "e0956278019b0a11",
	"ccgo_darwin_arm64.go":  "e0956278019b0a11",
	"ccgo_freebsd_amd64.go": "0e3c0ab222f076d0",
	"ccgo_freebsd_arm64.go": "f75185e05b8c0763",
	"ccgo_linux_386.go":     "2e64697a2178be25",
	"ccgo_linux_amd64.go":   "0e3c0ab222f076d0",
	"ccgo_linux_arm.go":     "a59685d047d10194",
	"ccgo_linux_arm64.go":   "f75185e05b8c0763",
	"ccgo_linux_loong64.go": "0e3c0ab222f076d0",
	"ccgo_linux_ppc64le.go": "f75185e05b8c0763",
	"ccgo_linux_riscv64.go": "f75185e05b8c0763",
	"ccgo_linux_s390x.go":   "672b0f4e663d978a",
	"ccgo_windows.go":       "116aa1250789e9f8",
}
