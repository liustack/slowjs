// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// quickjsLibcSites are the fingerprints of the functions and variables of
// quickjs-libc.c in each generated file as upstream writes them, and of the
// text of those that stay, as reviewed against quickjsLibcRoots (see
// fixQuickJSLibc).
var quickjsLibcSites = map[string]libcSite{
	"ccgo_darwin_amd64.go":  {names: "29fdd2184ad4387a", kept: "5b5e17e3001cfc13"},
	"ccgo_darwin_arm64.go":  {names: "29fdd2184ad4387a", kept: "5b5e17e3001cfc13"},
	"ccgo_freebsd_amd64.go": {names: "29fdd2184ad4387a", kept: "879628acd0c9baac"},
	"ccgo_freebsd_arm64.go": {names: "29fdd2184ad4387a", kept: "f8074c1396ea24c1"},
	"ccgo_linux_386.go":     {names: "29fdd2184ad4387a", kept: "dd50af6d954df5ee"},
	"ccgo_linux_amd64.go":   {names: "29fdd2184ad4387a", kept: "42c502eaf9755904"},
	"ccgo_linux_arm.go":     {names: "29fdd2184ad4387a", kept: "8e9118ee95054f12"},
	"ccgo_linux_arm64.go":   {names: "29fdd2184ad4387a", kept: "6fc36f5e6b481544"},
	"ccgo_linux_loong64.go": {names: "29fdd2184ad4387a", kept: "42c502eaf9755904"},
	"ccgo_linux_ppc64le.go": {names: "29fdd2184ad4387a", kept: "6fc36f5e6b481544"},
	"ccgo_linux_riscv64.go": {names: "29fdd2184ad4387a", kept: "6fc36f5e6b481544"},
	"ccgo_linux_s390x.go":   {names: "29fdd2184ad4387a", kept: "6fc36f5e6b481544"},
	"ccgo_windows.go":       {names: "9f08de2dacee9337", kept: "e1ff6461c87689ef"},
}
