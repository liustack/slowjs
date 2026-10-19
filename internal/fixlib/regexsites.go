// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// regexSites are the fingerprints of regexFuncs (without the change
// fixRegexSkip makes) in each generated file, as reviewed against
// lib/regexskip.go (see fixRegexSkip).
var regexSites = map[string]string{
	"ccgo_darwin_amd64.go":  "4fa59ab2edbecb0f",
	"ccgo_darwin_arm64.go":  "4fa59ab2edbecb0f",
	"ccgo_freebsd_amd64.go": "5397df7004f4a5e6",
	"ccgo_freebsd_arm64.go": "0af3b41cc28361d3",
	"ccgo_linux_386.go":     "7b2de1767207be2b",
	"ccgo_linux_amd64.go":   "5397df7004f4a5e6",
	"ccgo_linux_arm.go":     "c7eba1c9ebf44138",
	"ccgo_linux_arm64.go":   "0af3b41cc28361d3",
	"ccgo_linux_loong64.go": "5397df7004f4a5e6",
	"ccgo_linux_ppc64le.go": "0af3b41cc28361d3",
	"ccgo_linux_riscv64.go": "0af3b41cc28361d3",
	"ccgo_linux_s390x.go":   "b08091890b9f2e56",
	"ccgo_windows.go":       "b0b1d18d12bfda44",
}
