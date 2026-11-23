// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faultstate

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestDecide: the engine runs where the CONTEXT windows/arm64 saves at a
// fault, with the budget of the rest of the dispatch, fits in the 4 KiB Go
// keeps below a goroutine stack, and the extended state is none or SVE. The
// sizes past 128-bit SVE are worked out from the layout of winnt.h: each step
// of 128 bits adds 16 bytes to each of the 32 Z registers and 2 to each of the
// 16 P registers and FFR.
func TestDecide(t *testing.T) {
	sveSize := func(bits int) uint32 {
		l := bits / 8
		return uint32(1137 + 32*(l-16) + 17*(l-16)/8)
	}
	for _, c := range []struct {
		name string
		mask uint64
		size uint32
		err  error
		want string // empty: runs
	}{
		{name: "no extended state", mask: 0, size: 959},
		{name: "SVE 128 (Cobalt 100, measured)", mask: 0x4, size: 1137},
		{name: "SVE 256", mask: 0x4, size: sveSize(256)},
		{name: "SVE up to the budget", mask: 0x4, size: StackSystem - Dispatch},
		{name: "SVE one byte past the budget", mask: 0x4, size: StackSystem - Dispatch + 1, want: "more than the 4096"},
		{name: "SVE 512", mask: 0x4, size: sveSize(512), want: "more than the 4096"},
		{name: "SVE 2048", mask: 0x4, size: sveSize(2048), want: "more than the 4096"},
		{name: "SME ZA", mask: 0x4 | 0x8, size: 1137, want: "0x8 of it"},
		{name: "SME TPIDR2", mask: 0x4 | 0x10, size: 1137, want: "0x10 of it"},
		{name: "SME ZT", mask: 0x4 | 0x20, size: 1137, want: "0x20 of it"},
		{name: "unused bit 0", mask: 0x1, size: 959, want: "0x1 of it"},
		{name: "unknown bit", mask: 1 << 40, size: 959, want: "0x10000000000 of it"},
		{name: "smaller than a CONTEXT", mask: 0x4, size: 0x390 - 1, want: "less than the 912"},
		{name: "query error", err: errors.New("InitializeContext2: The parameter is incorrect."), want: "parameter is incorrect"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := Decide(c.mask, c.size, c.err)
			if c.want == "" {
				if err != nil {
					t.Fatalf("Decide(%#x, %d) = %v, want nil", c.mask, c.size, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Decide(%#x, %d) = nil, want an error with %q", c.mask, c.size, c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Decide(%#x, %d) = %q, want it to say %q", c.mask, c.size, err, c.want)
			}
			if c.err != nil && !errors.Is(err, c.err) {
				t.Fatalf("Decide(%v) = %v, does not wrap the error of the query", c.err, err)
			}
			if c.err == nil {
				// The error names the mask and the size it refused.
				for _, s := range []string{"windows/arm64", fmt.Sprintf("%#x", c.mask)} {
					if !strings.Contains(err.Error(), s) {
						t.Fatalf("Decide(%#x, %d) = %q, does not name %s", c.mask, c.size, err, s)
					}
				}
			}
		})
	}
	if got := sveSize(128); got != 1137 {
		t.Fatalf("SVE 128 is %d bytes, want the 1137 measured", got)
	}
}

// TestCheck: Check is nil on every target but windows/arm64, and the same
// value each time.
func TestCheck(t *testing.T) {
	a, b := Check(), Check()
	if a != b {
		t.Fatalf("Check gave %v, then %v", a, b)
	}
	if !queried && a != nil {
		t.Fatalf("Check = %v on a target that has no fault state to ask about", a)
	}
}
