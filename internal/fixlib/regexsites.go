// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// regexSites are the fingerprints of regexFuncs (without the change
// fixRegexSkip makes) in each generated file, as reviewed against
// lib/regexskip.go (see fixRegexSkip).
var regexSites = map[string]string{
	"ccgo_darwin_amd64.go":  "693af172dc59bbe5",
	"ccgo_darwin_arm64.go":  "693af172dc59bbe5",
	"ccgo_freebsd_amd64.go": "2cb06b0bfe5befe5",
	"ccgo_freebsd_arm64.go": "1d6936ebd2a55f46",
	"ccgo_linux_386.go":     "22973f1ba38b347f",
	"ccgo_linux_amd64.go":   "2cb06b0bfe5befe5",
	"ccgo_linux_arm.go":     "c19841db5535cb75",
	"ccgo_linux_arm64.go":   "1d6936ebd2a55f46",
	"ccgo_linux_loong64.go": "2cb06b0bfe5befe5",
	"ccgo_linux_ppc64le.go": "1d6936ebd2a55f46",
	"ccgo_linux_riscv64.go": "1d6936ebd2a55f46",
	"ccgo_linux_s390x.go":   "2ea3a71014c33bf9",
	"ccgo_windows.go":       "bd78cf78aec441ae",
}
