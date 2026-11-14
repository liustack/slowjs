// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"strings"
	"testing"

	"golang.org/x/arch/x86/x86asm"
)

// TestCheckAlign16: the post-link check refuses an SSE instruction that needs
// its operand on 16 bytes (movaps, movdqa, xorpd with a memory operand) when
// the address it reaches in the binary is not, as JS_RegvmRun's xorpd read
// its pool 8 bytes off in a layout moved by 8. The moves that need no
// alignment and the 8-byte accesses pass at any address.
func TestCheckAlign16(t *testing.T) {
	rip := []byte{0x05, 0, 0, 0, 0} // (%rip) with a displacement of 0
	for _, c := range []struct {
		name  string
		op    []byte
		needs bool
	}{
		{"movaps", []byte{0x0f, 0x28}, true},
		{"movdqa", []byte{0x66, 0x0f, 0x6f}, true},
		{"xorpd", []byte{0x66, 0x0f, 0x57}, true},
		{"paddd", []byte{0x66, 0x0f, 0xfe}, true},
		{"movups", []byte{0x0f, 0x10}, false},
		{"movdqu", []byte{0xf3, 0x0f, 0x6f}, false},
		{"movsd", []byte{0xf2, 0x0f, 0x10}, false},
		{"ucomisd", []byte{0x66, 0x0f, 0x2e}, false},
	} {
		in, err := x86asm.Decode(append(append([]byte{}, c.op...), rip...), 64)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.HasPrefix(strings.ToLower(in.Op.String()), c.name) {
			t.Fatalf("%s decodes as %v", c.name, in)
		}
		if err := checkAlign16(in, 0x5a8a40); err != nil {
			t.Errorf("%s at 0x5a8a40: %v", c.name, err)
		}
		for _, at := range []uint64{0x5a8a48, 0x5a8a44, 0x5a8a41} {
			err := checkAlign16(in, at)
			if c.needs && (err == nil || !strings.Contains(err.Error(), "not on the 16 bytes")) {
				t.Errorf("%s at %#x: %v, want refused", c.name, at, err)
			}
			if !c.needs && err != nil {
				t.Errorf("%s at %#x: %v, want passed", c.name, at, err)
			}
		}
	}
}
