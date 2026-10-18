// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// defineSites are the fingerprints of JS_DefinePropertyValue (without the
// call fixDefine adds), JS_DefineProperty, JS_CreateProperty, add_property
// and find_own_property1 in each generated file, as reviewed against
// objlitDefine in lib/objlit.go (see fixDefine).
var defineSites = map[string]string{
	"ccgo_darwin_amd64.go":  "02cac81a137e5663",
	"ccgo_darwin_arm64.go":  "02cac81a137e5663",
	"ccgo_freebsd_amd64.go": "b3aadfdcf99f13b0",
	"ccgo_freebsd_arm64.go": "b3aadfdcf99f13b0",
	"ccgo_linux_386.go":     "897b3d31535a9392",
	"ccgo_linux_amd64.go":   "97b21d99b9669aea",
	"ccgo_linux_arm.go":     "3e4615208020ff1e",
	"ccgo_linux_arm64.go":   "97b21d99b9669aea",
	"ccgo_linux_loong64.go": "97b21d99b9669aea",
	"ccgo_linux_ppc64le.go": "97b21d99b9669aea",
	"ccgo_linux_riscv64.go": "97b21d99b9669aea",
	"ccgo_linux_s390x.go":   "5af562b181965e40",
	"ccgo_windows.go":       "e2832e5d77a64102",
}
