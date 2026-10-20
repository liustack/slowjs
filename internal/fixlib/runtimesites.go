// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// runtimeSites are the fingerprints of JSRuntime and XJS_NewRuntime2
// (without the change fixRuntimeField makes) in each generated file, as
// reviewed against lib/runtime.go (see fixRuntimeField).
var runtimeSites = map[string]string{
	"ccgo_darwin_amd64.go":  "13153f26e4487c33",
	"ccgo_darwin_arm64.go":  "13153f26e4487c33",
	"ccgo_freebsd_amd64.go": "eb39d71bdd9702b8",
	"ccgo_freebsd_arm64.go": "eb39d71bdd9702b8",
	"ccgo_linux_386.go":     "c2ebc7953369bc75",
	"ccgo_linux_amd64.go":   "eb39d71bdd9702b8",
	"ccgo_linux_arm.go":     "7adc5111a34b5e8a",
	"ccgo_linux_arm64.go":   "eb39d71bdd9702b8",
	"ccgo_linux_loong64.go": "eb39d71bdd9702b8",
	"ccgo_linux_ppc64le.go": "eb39d71bdd9702b8",
	"ccgo_linux_riscv64.go": "eb39d71bdd9702b8",
	"ccgo_linux_s390x.go":   "eb39d71bdd9702b8",
	"ccgo_windows.go":       "d444d334f8c1e813",
}
