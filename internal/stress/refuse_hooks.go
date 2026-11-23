// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package main

import (
	"math/rand/v2"
	"sync"

	"github.com/liustack/tifajs/internal/hostmem"
)

// refuse makes the OS refuse perMille of every thousand maps of a chunk, a
// remap or a block of a Go function, at random, in every VM, until the
// function it returns takes the hook away. It needs the tag testhooks
// (hostmem.SetFault), and is set while no VM runs.
func refuse(perMille int, seed uint64, refused func()) (undo func(), err error) {
	var mu sync.Mutex
	rng := rand.New(rand.NewPCG(seed, 1<<32))
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call != "map" && call != "remap" && call != "block" {
			return nil
		}
		mu.Lock()
		no := rng.IntN(1000) < perMille
		mu.Unlock()
		if !no {
			return nil
		}
		refused()
		return errRefused
	})
	return func() { hostmem.SetFault(nil) }, nil
}
