// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build (linux || darwin) && testhooks

package hostmem

import (
	"errors"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

// The setup of a shadow stack maps twice its size, unmaps what lies below
// and above the aligned stack and its guard page, opens the stack, and on
// linux keeps huge pages off it. These tests make each step fail and check
// what the setup gives back.

const setupSize = 16 << 20

// isBelow tells the unmap of the part below the stack: it ends where the
// aligned stack starts.
func isBelow(c call) bool {
	return c.name == "unmap" && (c.addr+c.size)%setupSize == 0
}

// withPartBelow runs try until the reservation of the setup does not come
// aligned, so that there is a part below the stack to unmap. try reports
// whether it saw that part. The OS tends to put a reservation where the one
// before it was, so before each new try a mapping of the same size takes that
// place, kept until the end of the test.
func withPartBelow(t *testing.T, try func() bool) {
	t.Helper()
	n := 2*setupSize + pageSize
	for i := 0; i < 4; i++ {
		if try() {
			return
		}
		p, err := unix.MmapPtr(-1, 0, nil, n, unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { unix.MunmapPtr(p, n) })
	}
	t.Skip("the reservation came aligned 4 times: there was nothing below the stack to unmap")
}

func TestStackSetupRollback(t *testing.T) {
	n := 2*setupSize + pageSize
	steps := []string{"unmap below", "unmap above", "protect"}
	if runtime.GOOS == "linux" {
		steps = append(steps, "nohugepage")
	}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			withPartBelow(t, func() bool {
				var below, above call // the two unmaps of the setup, as they came
				calls := failing(t, func(c call, before []call) error {
					switch {
					case c.name == "protect" || c.name == "nohugepage":
						if step == c.name {
							return errInjected
						}
					case c.name != "unmap", count(before, "protect") > 0:
					case below.size == 0 && above.size == 0 && isBelow(c):
						below = c
						if step == "unmap below" {
							return errInjected
						}
					case above.size == 0 && step != "unmap below":
						above = c
						if step == "unmap above" {
							return errInjected
						}
					}
					return nil
				})
				m, err := New(setupSize)
				fault = nil
				if below.size == 0 {
					if err == nil {
						m.Close()
					}
					return false
				}
				if !errors.Is(err, errInjected) {
					t.Fatalf("New: %v, want the injected error", err)
				}
				// What the setup still held when the step failed.
				start := below.addr
				base := alignUp(start, setupSize)
				lo, hi := base, start+n
				switch step {
				case "unmap below":
					lo = start
				case "protect", "nohugepage":
					hi = base + setupSize + pageSize
				}
				last := (*calls)[len(*calls)-1]
				if last.name != "unmap" || last.addr != lo || last.size != hi-lo {
					t.Fatalf("the setup ended with %+v, want it to unmap the %d bytes at %#x it still held", last, hi-lo, lo)
				}
				return true
			})
		})
	}
}

// TestStackSetupLeavesOtherMappings maps memory where the setup unmapped the
// part below the stack, as another thread may, then makes the next step fail:
// giving back what the setup holds must not unmap it.
func TestStackSetupLeavesOtherMappings(t *testing.T) {
	withPartBelow(t, func() bool {
		var below call
		var other uintptr
		var mapErr error
		calls := failing(t, func(c call, before []call) error {
			if below.size == 0 && c.name == "unmap" && isBelow(c) {
				below = c
				return nil
			}
			if below.size == 0 || other != 0 || mapErr != nil {
				return nil
			}
			// The part below is free now: map there, then fail this step.
			p, err := unix.MmapPtr(-1, 0, pointer(below.addr), below.size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
			if err != nil {
				mapErr = err
				return nil
			}
			if other = uintptr(p); other != below.addr {
				mapErr = errors.New("the OS put the mapping elsewhere")
				return nil
			}
			fill(other, below.size, 11)
			return errInjected
		})
		m, err := New(setupSize)
		fault = nil
		if other != 0 {
			defer unix.MunmapPtr(pointer(other), below.size)
		}
		if below.size == 0 || mapErr != nil {
			if err == nil {
				m.Close()
			}
			if mapErr != nil {
				t.Skipf("could not map where the part below was: %v", mapErr)
			}
			return false
		}
		if !errors.Is(err, errInjected) {
			t.Fatalf("New: %v, want the injected error", err)
		}
		if !readable(other, below.size) {
			other = 0
			t.Fatal("the cleanup unmapped the other mapping")
		}
		if i := same(other, below.size, 11); i >= 0 {
			t.Fatalf("the cleanup changed the other mapping at byte %d", i)
		}
		for _, c := range *calls {
			if c != below && c.name == "unmap" && c.addr < other+below.size && other < c.addr+c.size {
				t.Fatalf("the cleanup unmapped %d bytes at %#x, over the other mapping at %#x", c.size, c.addr, other)
			}
		}
		return true
	})
}

// TestStackSetupCannotGiveBack fails a step and then the cleanup: the setup
// may not lose the range, so it panics with both errors.
func TestStackSetupCannotGiveBack(t *testing.T) {
	errCleanup := errors.New("injected cleanup")
	var held call
	failing(t, func(c call, before []call) error {
		switch {
		case c.name == "protect":
			return errInjected
		case c.name == "unmap" && count(before, "protect") > 0:
			held = c
			return errCleanup
		}
		return nil
	})
	r := panicked(func() { New(setupSize) })
	fault = nil
	if held.size != 0 {
		defer osUnmap(held.addr, held.size)
	}
	err, ok := r.(error)
	if !ok || !errors.Is(err, errInjected) || !errors.Is(err, errCleanup) {
		t.Fatalf("New panicked with %v, want an error with both the step's and the cleanup's", r)
	}
	t.Log(err)
}

// TestStackWithoutHugePages: a kernel built without transparent huge pages
// refuses MADV_NOHUGEPAGE with EINVAL, and the stack is fine without it.
func TestStackWithoutHugePages(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("no transparent huge pages on %s", runtime.GOOS)
	}
	calls := failing(t, func(c call, before []call) error {
		if c.name == "nohugepage" {
			return unix.EINVAL
		}
		return nil
	})
	m, err := New(setupSize)
	if err != nil {
		t.Fatalf("New with EINVAL from MADV_NOHUGEPAGE: %v", err)
	}
	if count(*calls, "nohugepage") != 1 {
		t.Fatalf("the setup made %+v, want one MADV_NOHUGEPAGE", *calls)
	}
	fault = nil
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
