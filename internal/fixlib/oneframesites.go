// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// oneFrameSites are the fingerprints of the text of JS_CallInternal
// between its entry and the block fixOneFrame drops (see
// oneFrameFingerprint), in each generated file, as reviewed: nothing there
// writes again the parameter copies and the pointers into the frame the
// block makes. fixOneFrame checks them in the files it changes.
var oneFrameSites = map[string]string{
	"ccgo_darwin_amd64.go": "0b761a802ac89677",
	"ccgo_darwin_arm64.go": "123c96df1846e437",
	"ccgo_linux_amd64.go":  "670b786d70aea597",
	"ccgo_linux_arm64.go":  "bc43a62147b09dc2",
	"ccgo_windows.go":      "9d7d7514d3ea536c",
}
