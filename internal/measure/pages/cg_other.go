// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !amd64

package pages

// Only amd64 has the client requests: callgrind counts the window there.
func clientRequest(*[6]uint64) uint64 {
	panic("pages: no client request of valgrind on this architecture")
}

const canMarkWindow = false
