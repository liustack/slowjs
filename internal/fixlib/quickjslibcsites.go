// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// quickjsLibcSites are the fingerprints of the functions and variables of
// quickjs-libc.c in each generated file as upstream writes them, and of the
// text of those that stay, as reviewed against quickjsLibcRoots (see
// fixQuickJSLibc).
var quickjsLibcSites = map[string]libcSite{
	"ccgo_darwin_amd64.go": {names: "29fdd2184ad4387a", kept: "5b5e17e3001cfc13"},
	"ccgo_darwin_arm64.go": {names: "29fdd2184ad4387a", kept: "5b5e17e3001cfc13"},
	"ccgo_linux_amd64.go":  {names: "29fdd2184ad4387a", kept: "42c502eaf9755904"},
	"ccgo_linux_arm64.go":  {names: "29fdd2184ad4387a", kept: "6fc36f5e6b481544"},
	"ccgo_windows.go":      {names: "9f08de2dacee9337", kept: "e1ff6461c87689ef"},
}
