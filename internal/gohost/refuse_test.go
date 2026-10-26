// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package gohost

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/liustack/slowjs/internal/hostmem"
)

// These tests make the OS refuse calls through hostmem.SetFault, which exists
// only with the tag testhooks.

var errRefused = errors.New("refused by the test")

// TestLeftovers: memory the OS refuses to unmap at Close stays with the
// package, which lists it and unmaps it on a later try, also when no one
// holds the VM any more.
func TestLeftovers(t *testing.T) {
	t.Cleanup(func() {
		hostmem.SetFault(nil)
		CloseLeftovers()
	})
	lost, kept := newVM(t, small), newVM(t, small)
	for _, v := range []*VM{lost, kept} {
		if MemoryGrow(v.ID(), hostmem.Unit) == 0 {
			t.Fatal("no chunk")
		}
	}
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call == "unmap" {
			return errRefused
		}
		return nil
	})
	for _, v := range []*VM{lost, kept} {
		if err := v.Close(); !errors.Is(err, errRefused) {
			t.Fatalf("Close with unmap refused: %v", err)
		}
	}
	lostID := lost.ID()
	lost = nil
	got := Leftovers()
	if len(got) != 2 || got[0].ID != lostID || got[1].ID != kept.ID() {
		t.Fatalf("leftovers %+v, want VMs %d and %d", got, lostID, kept.ID())
	}
	for _, l := range got {
		if !errors.Is(l.Err, errRefused) || l.Usage.Chunks != 1 || l.Usage.Stack != small {
			t.Errorf("leftover %+v, want one chunk, the stack and the refusal", l)
		}
	}
	if err := CloseLeftovers(); !errors.Is(err, errRefused) || len(Leftovers()) != 2 {
		t.Fatalf("CloseLeftovers with unmap refused: %v, then %+v", err, Leftovers())
	}
	hostmem.SetFault(nil)
	if err := kept.Close(); err != nil {
		t.Fatal(err)
	}
	if got := Leftovers(); len(got) != 1 || got[0].ID != lostID {
		t.Fatalf("after the owner closed its VM again: %+v, want VM %d", got, lostID)
	}
	if err := CloseLeftovers(); err != nil {
		t.Fatal(err)
	}
	if got := Leftovers(); len(got) != 0 {
		t.Fatalf("leftovers %+v after CloseLeftovers", got)
	}
	if u := kept.Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Fatalf("the VM closed again still holds %+v", u)
	}
}

// TestCallReleaseRefused: when the OS refuses to unmap a chunk the engine
// gives back, the request answers as if it went well, the chunk stays mapped
// and counted with its contents, the failure is counted and its error kept,
// and the engine's next request for the same chunk unmaps it.
func TestCallReleaseRefused(t *testing.T) {
	v := newVM(t, small)
	sp, _ := v.Stack()
	const size = 8 * hostmem.Unit
	p := MemoryGrow(v.ID(), size)
	b := unsafe.Slice((*byte)(addr(p)), size)
	b[0], b[size-1] = 7, 9
	args := argv(t, v, p, size)
	usage, counts := v.Memory().Usage(), v.Counts()

	t.Cleanup(func() { hostmem.SetFault(nil) })
	hostmem.SetFault(func(call string, addr, n uintptr) error {
		if call == "unmap" && addr == p {
			return errRefused
		}
		return nil
	})
	r := Call(sp, v.ID(), 0, 2, args, CallMemoryRelease)
	hostmem.SetFault(nil)

	if r != Undefined {
		t.Fatalf("a refused release returned %+v", r)
	}
	if u := v.Memory().Usage(); u != usage {
		t.Fatalf("after the refusal the VM holds %+v, want %+v as before", u, usage)
	}
	if b[0] != 7 || b[size-1] != 9 {
		t.Fatal("the chunk lost its contents")
	}
	c := v.Counts()
	if c.FreeFailures != 1 || c.Frees != counts.Frees || c.FreeBytes != counts.FreeBytes {
		t.Fatalf("counts %+v after a refused release, from %+v: want one failure and no free", c, counts)
	}
	if !errors.Is(v.LastError(), errRefused) {
		t.Fatalf("the error kept is %v, want the refusal", v.LastError())
	}

	// The OS takes it now.
	if r := Call(sp, v.ID(), 0, 2, args, CallMemoryRelease); r != Undefined {
		t.Fatalf("the release again returned %+v", r)
	}
	u := v.Memory().Usage()
	if u.Chunks != usage.Chunks-1 || u.Footprint != usage.Footprint-size {
		t.Fatalf("after the release again the VM holds %+v, want the chunk gone from %+v", u, usage)
	}
	if c := v.Counts(); c.Frees != counts.Frees+1 || c.FreeBytes != counts.FreeBytes+size || c.FreeFailures != 1 {
		t.Fatalf("counts %+v after the release again", c)
	}
}
