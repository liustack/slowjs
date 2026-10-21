// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// clockSites are the fingerprints of clockFuncs in each generated file, as
// reviewed against lib/clock.go (see fixClock).
var clockSites = map[string]string{
	"ccgo_darwin_amd64.go":  "202857511f7f6c5d",
	"ccgo_darwin_arm64.go":  "202857511f7f6c5d",
	"ccgo_freebsd_amd64.go": "132f27b095a224bf",
	"ccgo_freebsd_arm64.go": "132f27b095a224bf",
	"ccgo_linux_386.go":     "9f109b45056f262c",
	"ccgo_linux_amd64.go":   "8348fe398d4af3ef",
	"ccgo_linux_arm.go":     "9f109b45056f262c",
	"ccgo_linux_arm64.go":   "8348fe398d4af3ef",
	"ccgo_linux_loong64.go": "8348fe398d4af3ef",
	"ccgo_linux_ppc64le.go": "8348fe398d4af3ef",
	"ccgo_linux_riscv64.go": "8348fe398d4af3ef",
	"ccgo_linux_s390x.go":   "8348fe398d4af3ef",
	"ccgo_windows.go":       "a2f0415a72834865",
}
