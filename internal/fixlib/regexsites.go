// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// regexSites are the fingerprints of regexFuncs (without the change
// fixRegexSkip makes) in each generated file, as reviewed against
// lib/regexskip.go (see fixRegexSkip).
var regexSites = map[string]string{
	"ccgo_darwin_amd64.go": "693af172dc59bbe5",
	"ccgo_darwin_arm64.go": "693af172dc59bbe5",
	"ccgo_linux_amd64.go":  "2cb06b0bfe5befe5",
	"ccgo_linux_arm64.go":  "1d6936ebd2a55f46",
	"ccgo_windows.go":      "bd78cf78aec441ae",
}
