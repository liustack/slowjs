// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// workChargeSites are the fingerprints of workChargeFuncs (without the
// change fixWorkCharge makes) in each generated file, as reviewed against
// lib/charge.go (see fixWorkCharge).
var workChargeSites = map[string]string{
	"ccgo_darwin_amd64.go":  "8bce62c9658cea94",
	"ccgo_darwin_arm64.go":  "8bce62c9658cea94",
	"ccgo_freebsd_amd64.go": "8442eae4650f57c9",
	"ccgo_freebsd_arm64.go": "8442eae4650f57c9",
	"ccgo_linux_386.go":     "aa6e160bf92c7f50",
	"ccgo_linux_amd64.go":   "8442eae4650f57c9",
	"ccgo_linux_arm.go":     "88918f520ef40152",
	"ccgo_linux_arm64.go":   "8442eae4650f57c9",
	"ccgo_linux_loong64.go": "8442eae4650f57c9",
	"ccgo_linux_ppc64le.go": "8442eae4650f57c9",
	"ccgo_linux_riscv64.go": "8442eae4650f57c9",
	"ccgo_linux_s390x.go":   "8442eae4650f57c9",
	"ccgo_windows.go":       "35ce0f3e3c1d81d5",
}
