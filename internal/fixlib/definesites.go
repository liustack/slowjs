// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// defineSites are the fingerprints of JS_DefinePropertyValue (without the
// call fixDefine adds), JS_DefineProperty, JS_CreateProperty, add_property,
// find_own_property and find_own_property1 in each generated file, as
// reviewed against objlitDefine in lib/objlit.go (see fixDefine).
var defineSites = map[string]string{
	"ccgo_darwin_amd64.go":  "6faf839eab0d4a14",
	"ccgo_darwin_arm64.go":  "6faf839eab0d4a14",
	"ccgo_freebsd_amd64.go": "9f26021de19f574d",
	"ccgo_freebsd_arm64.go": "9f26021de19f574d",
	"ccgo_linux_386.go":     "94d3cb397ef4234c",
	"ccgo_linux_amd64.go":   "0b0f8e65a0a090f1",
	"ccgo_linux_arm.go":     "a3013360e4f2ed0b",
	"ccgo_linux_arm64.go":   "0b0f8e65a0a090f1",
	"ccgo_linux_loong64.go": "0b0f8e65a0a090f1",
	"ccgo_linux_ppc64le.go": "0b0f8e65a0a090f1",
	"ccgo_linux_riscv64.go": "0b0f8e65a0a090f1",
	"ccgo_linux_s390x.go":   "69758fb7413389dd",
	"ccgo_windows.go":       "833016d750cc844b",
}
