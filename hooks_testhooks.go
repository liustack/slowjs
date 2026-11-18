// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks && (((linux || darwin) && (amd64 || arm64)) || (windows && amd64))

package slowjs // import "github.com/liustack/slowjs"

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
