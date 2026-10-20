// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// pollSites are the fingerprints of pollFuncs in each generated file, as
// reviewed against lib/poll.go (see fixPoll).
var pollSites = map[string]string{
	"ccgo_darwin_amd64.go":  "57f8cd868914a1cc",
	"ccgo_darwin_arm64.go":  "57f8cd868914a1cc",
	"ccgo_freebsd_amd64.go": "57f8cd868914a1cc",
	"ccgo_freebsd_arm64.go": "57f8cd868914a1cc",
	"ccgo_linux_386.go":     "aae03a18c110f619",
	"ccgo_linux_amd64.go":   "57f8cd868914a1cc",
	"ccgo_linux_arm.go":     "d283d8a39fb4c4e1",
	"ccgo_linux_arm64.go":   "57f8cd868914a1cc",
	"ccgo_linux_loong64.go": "57f8cd868914a1cc",
	"ccgo_linux_ppc64le.go": "57f8cd868914a1cc",
	"ccgo_linux_riscv64.go": "57f8cd868914a1cc",
	"ccgo_linux_s390x.go":   "57f8cd868914a1cc",
	"ccgo_windows.go":       "f4b85f4fe73909a9",
}
