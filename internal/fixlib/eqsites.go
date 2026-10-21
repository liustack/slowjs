// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// eqSites are the fingerprints of the functions the text of fixEq stands
// for, in each generated file, as reviewed against it (see fixEq).
var eqSites = map[string]string{
	"ccgo_darwin_amd64.go":  "e77b300d37cd5ff0",
	"ccgo_darwin_arm64.go":  "e77b300d37cd5ff0",
	"ccgo_freebsd_amd64.go": "9fb4b308d944e1df",
	"ccgo_freebsd_arm64.go": "9fb4b308d944e1df",
	"ccgo_linux_386.go":     "487cd70c92bff43a",
	"ccgo_linux_amd64.go":   "b8b4703eb6bcd9be",
	"ccgo_linux_arm.go":     "8169715fe99c9304",
	"ccgo_linux_arm64.go":   "b8b4703eb6bcd9be",
	"ccgo_linux_loong64.go": "b8b4703eb6bcd9be",
	"ccgo_linux_ppc64le.go": "b8b4703eb6bcd9be",
	"ccgo_linux_riscv64.go": "b8b4703eb6bcd9be",
	"ccgo_linux_s390x.go":   "731e0a59472a6707",
	"ccgo_windows.go":       "985070bfbb61ef9e",
}
