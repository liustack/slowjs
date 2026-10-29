// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"fmt"

	"github.com/liustack/slowjs/internal/cengine"
)

const engineName = "C engine as Go assembly (internal/cengine)"

// timedWhat is what cengine.VM.Eval does.
const timedWhat = "Eval, which copies the source into the VM, runs the script and its jobs and turns the result into a Go string"

type cvm struct{ v *cengine.VM }

func newVM(o options) (vm, error) {
	v, err := cengine.New(cengine.Options{Seed: o.seed, StackSize: o.stack})
	if err != nil {
		return nil, err
	}
	if !o.clock.IsZero() {
		v.SetWallClock(o.clock, false)
	}
	return cvm{v}, nil
}

func (c cvm) eval(src, file string) (string, error) { return c.v.Eval(src, file) }
func (c cvm) gc()                                   { c.v.RunGC() }

func (c cvm) release(keep uintptr) (string, error) {
	r, err := c.v.ReleaseMemory(keep)
	return fmt.Sprintf("%d bytes of chunks, %d of pages, %d of the shadow stack", r.Chunks, r.Pages, r.Stack), err
}

func (c cvm) memory() string {
	u := c.v.Usage()
	k := u.Counts
	s := fmt.Sprintf("MemorySize %d, footprint %d in %d chunks (host: %d in %d), grows %d (%d bytes), dedicated frees %d (%d bytes), remaps %d (+%d bytes)",
		u.MemorySize, u.Footprint, u.Chunks, u.Host.Footprint, u.Host.Chunks, k.Grows, k.GrowBytes, k.Frees, k.FreeBytes, k.Remaps, k.RemapBytes)
	if k.GrowFailures+k.FreeFailures+k.RemapFailures > 0 {
		s += fmt.Sprintf(", refused by the OS: %d grows, %d frees, %d remaps", k.GrowFailures, k.FreeFailures, k.RemapFailures)
	}
	return s
}

func (c cvm) close() (string, error) {
	k := c.v.Host().Counts()
	leaked, err := c.v.Close()
	u := c.v.Host().Memory().Usage()
	s := fmt.Sprintf("%d blocks in use, host keeps %d bytes in %d chunks and %d of stack, %d dedicated frees, %d remaps", leaked, u.Footprint, u.Chunks, u.Stack, k.Frees, k.Remaps)
	if err == nil && (leaked != 0 || u.Footprint != 0 || u.Chunks != 0 || u.Stack != 0) {
		err = fmt.Errorf("close left %s", s)
	}
	return s, err
}
