// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks && ((((linux && !android) || (darwin && !ios)) && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package tifajs // import "github.com/liustack/tifajs"

// What a program of this module that tests a VM from outside the package,
// such as the render stress (internal/stress), asks of its host. They exist
// only in a build with the tag testhooks.

// TestPolls is how many polls the engine of m made so far.
func (m *VM) TestPolls() int64 { return m.h.Polls() }

// TestAbortAfterPolls makes the n-th poll of m from now end the call into the
// engine as the engine's abort does: the call panics and m is good only for
// Close. 0 turns it off. Call it between calls into the engine.
func (m *VM) TestAbortAfterPolls(n int64) {
	if n == 0 {
		m.h.AbortAtPoll(0)
		return
	}
	m.h.AbortAtPoll(m.h.Polls() + n)
}
