// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// regexCacheSites are the fingerprints of regexCacheFuncs in each generated
// file, as reviewed against lib/regexcache.go (see fixRegexCache).
var regexCacheSites = map[string]string{
	"ccgo_darwin_amd64.go":  "a3cd2f7aec1fe295",
	"ccgo_darwin_arm64.go":  "a3cd2f7aec1fe295",
	"ccgo_freebsd_amd64.go": "2842d95ed5b59219",
	"ccgo_freebsd_arm64.go": "f881f3d77059a39d",
	"ccgo_linux_386.go":     "f928a3c87d2787fe",
	"ccgo_linux_amd64.go":   "a3cd2f7aec1fe295",
	"ccgo_linux_arm.go":     "3401f1e59de27abb",
	"ccgo_linux_arm64.go":   "b01700921b286c51",
	"ccgo_linux_loong64.go": "a3cd2f7aec1fe295",
	"ccgo_linux_ppc64le.go": "b01700921b286c51",
	"ccgo_linux_riscv64.go": "b01700921b286c51",
	"ccgo_linux_s390x.go":   "b442a8fdcc43ac75",
	"ccgo_windows.go":       "5e2fa92f503c1e6d",
}
