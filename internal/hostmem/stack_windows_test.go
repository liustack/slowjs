// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

// The setup of a shadow stack on windows reserves twice its size to find an
// aligned address, gives that back, reserves the stack and its guard page at
// the aligned address, and commits the stack. These tests make each step fail
// and check what the setup gives back.

const setupSize = 16 << 20

func TestStackSetupRollback(t *testing.T) {
	t.Run("first reserve", func(t *testing.T) {
		calls := failing(t, func(c call, before []call) error {
			if c.name == "reserve" {
				return errInjected
			}
			return nil
		})
		if _, err := New(setupSize); !errors.Is(err, errInjected) {
			t.Fatalf("New: %v, want the injected error", err)
		}
		if len(*calls) != 1 {
			t.Fatalf("calls after the first failed: %+v", *calls)
		}
	})

	t.Run("aligned address taken", func(t *testing.T) {
		calls := failing(t, func(c call, before []call) error {
			if c.name == "reserve" && c.addr != 0 {
				return windows.ERROR_INVALID_ADDRESS
			}
			return nil
		})
		_, err := New(setupSize)
		if !errors.Is(err, windows.ERROR_INVALID_ADDRESS) {
			t.Fatalf("New: %v, want ERROR_INVALID_ADDRESS", err)
		}
		if r, u := count(*calls, "reserve"), count(*calls, "unmap"); r != 32 || u != 16 {
			t.Fatalf("%d reserves and %d unmaps, want 16 tries of both reserves and one unmap", r, u)
		}
	})

	t.Run("aligned reserve", func(t *testing.T) {
		calls := failing(t, func(c call, before []call) error {
			if c.name == "reserve" && c.addr != 0 {
				return errInjected
			}
			return nil
		})
		if _, err := New(setupSize); !errors.Is(err, errInjected) {
			t.Fatalf("New: %v, want the injected error", err)
		}
		// The probe was given back before, and nothing else is held.
		if last := (*calls)[len(*calls)-1]; last.name != "reserve" {
			t.Fatalf("the setup ended with %+v", last)
		}
	})

	t.Run("commit", func(t *testing.T) {
		var base uintptr
		calls := failing(t, func(c call, before []call) error {
			switch {
			case c.name == "reserve" && c.addr != 0:
				base = c.addr
			case c.name == "commit":
				return errInjected
			}
			return nil
		})
		if _, err := New(setupSize); !errors.Is(err, errInjected) {
			t.Fatalf("New: %v, want the injected error", err)
		}
		if last := (*calls)[len(*calls)-1]; last.name != "unmap" || last.addr != base {
			t.Fatalf("the setup ended with %+v, want it to give back the reservation at %#x", last, base)
		}
	})
}

// TestStackSetupCannotGiveBack makes giving back a reservation fail: the
// setup may not lose it, so it panics with every error.
func TestStackSetupCannotGiveBack(t *testing.T) {
	errCleanup := errors.New("injected cleanup")
	for _, step := range []string{"probe", "commit"} {
		t.Run(step, func(t *testing.T) {
			var held call
			failing(t, func(c call, before []call) error {
				switch {
				case c.name == "commit" && step == "commit":
					return errInjected
				case c.name == "unmap" && (step == "probe" || count(before, "commit") > 0):
					held = c
					return errCleanup
				}
				return nil
			})
			r := panicked(func() { New(setupSize) })
			fault = nil
			if held.addr != 0 {
				defer windows.VirtualFree(held.addr, 0, windows.MEM_RELEASE)
			}
			err, ok := r.(error)
			if !ok || !errors.Is(err, errCleanup) || step == "commit" && !errors.Is(err, errInjected) {
				t.Fatalf("New panicked with %v, want an error with the cleanup's and the step's", r)
			}
			t.Log(err)
		})
	}
}
