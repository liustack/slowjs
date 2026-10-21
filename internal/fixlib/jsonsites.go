// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// jsonSites are the fingerprints of jsonFuncs (without the change
// fixJSONStrings makes) in each generated file, as reviewed against
// lib/jsonstr.go (see fixJSONStrings).
var jsonSites = map[string]string{
	"ccgo_darwin_amd64.go":  "249e4b77f579d766",
	"ccgo_darwin_arm64.go":  "249e4b77f579d766",
	"ccgo_freebsd_amd64.go": "637bbc67fe13a614",
	"ccgo_freebsd_arm64.go": "81362738b2cbb904",
	"ccgo_linux_386.go":     "50a74418213ffa01",
	"ccgo_linux_amd64.go":   "637bbc67fe13a614",
	"ccgo_linux_arm.go":     "40620946c38bba18",
	"ccgo_linux_arm64.go":   "81362738b2cbb904",
	"ccgo_linux_loong64.go": "637bbc67fe13a614",
	"ccgo_linux_ppc64le.go": "81362738b2cbb904",
	"ccgo_linux_riscv64.go": "81362738b2cbb904",
	"ccgo_linux_s390x.go":   "044e96272c231f48",
	"ccgo_windows.go":       "da6f1e35bb79bc69",
}
