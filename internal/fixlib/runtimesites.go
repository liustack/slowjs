// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// runtimeSites are the fingerprints of JSRuntime and XJS_NewRuntime2
// (without the change fixRuntimeField makes) in each generated file, as
// reviewed against lib/runtime.go (see fixRuntimeField).
var runtimeSites = map[string]string{
	"ccgo_darwin_amd64.go": "13153f26e4487c33",
	"ccgo_darwin_arm64.go": "13153f26e4487c33",
	"ccgo_linux_amd64.go":  "eb39d71bdd9702b8",
	"ccgo_linux_arm64.go":  "eb39d71bdd9702b8",
	"ccgo_windows.go":      "d444d334f8c1e813",
}
