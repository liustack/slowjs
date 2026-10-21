// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// funcShapeSites are the fingerprints of funcShapeFuncs and of JSShape in
// each generated file, as reviewed against lib/funcshape.go (see
// fixFuncShape).
var funcShapeSites = map[string]string{
	"ccgo_darwin_amd64.go":  "c2c010241f217bf7",
	"ccgo_darwin_arm64.go":  "c2c010241f217bf7",
	"ccgo_freebsd_amd64.go": "f398456890f9f8ea",
	"ccgo_freebsd_arm64.go": "f398456890f9f8ea",
	"ccgo_linux_386.go":     "d2ad92e9bfd93ec3",
	"ccgo_linux_amd64.go":   "2f6f35c13805e41e",
	"ccgo_linux_arm.go":     "257d7a68382150be",
	"ccgo_linux_arm64.go":   "2f6f35c13805e41e",
	"ccgo_linux_loong64.go": "2f6f35c13805e41e",
	"ccgo_linux_ppc64le.go": "2f6f35c13805e41e",
	"ccgo_linux_riscv64.go": "2f6f35c13805e41e",
	"ccgo_linux_s390x.go":   "2921f5584f39c3a5",
	"ccgo_windows.go":       "449c771b34d3169e",
}
