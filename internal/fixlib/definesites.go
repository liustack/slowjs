// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
