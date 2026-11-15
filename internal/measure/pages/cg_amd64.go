// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

// clientRequest is in cg_amd64.s.
//
//go:noescape
func clientRequest(args *[6]uint64) uint64

const canMarkWindow = true
