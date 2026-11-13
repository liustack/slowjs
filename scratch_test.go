// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf16"
)

// TestScratchPutBack checks the buffer utf16String puts back into
// utf16Scratch for the next string (see docs/changes/url.md): a string of at
// most utf16Short units takes none, a longer one takes one, and puts back the
// one it took, as it was when it had room for 3 bytes a unit, or else grown to
// exactly 3 bytes a unit. A string longer than stringStep takes none: it is
// written a piece at a time through a buffer of its own (utf16Steps). So no
// read of n units puts back a buffer larger than the one it took and 3n
// bytes. The test hands each call a buffer
// of its choice through New of a pool nothing was put into, which Get calls
// then, and reads the buffer after the call. What the pool does with what is
// put back, keep it or drop it, is Go's and not tested.
func TestScratchPutBack(t *testing.T) {
	saved := utf16Scratch.New
	t.Cleanup(func() { utf16Scratch = sync.Pool{New: saved} })
	read := func(n, c int) (made []*[]byte, s string) {
		utf16Scratch = sync.Pool{New: func() any {
			p := new([]byte)
			*p = make([]byte, 0, c)
			made = append(made, p)
			return p
		}}
		u := utf16.Encode([]rune(strings.Repeat("一", n)))
		s, _ = utf16String(nil, u, false)
		return made, s
	}
	for _, n := range []int{1, utf16Short, utf16Short + 1, 1000, 1 << 16, stringStep, stringStep + 1, 3*stringStep + 5} {
		need := 3 * n
		for _, c := range []int{0, need - 1, need, need + 7, 4 * need} {
			made, s := read(n, c)
			if s != strings.Repeat("一", n) {
				t.Fatalf("%d units: the string is not the one read", n)
			}
			if n <= utf16Short || n > stringStep {
				if len(made) != 0 {
					t.Errorf("%d units took %d buffers, want none", n, len(made))
				}
				continue
			}
			if len(made) != 1 {
				t.Fatalf("%d units took %d buffers, want 1", n, len(made))
			}
			want := c
			if c < need {
				want = need
			}
			if got := cap(*made[0]); got != want {
				t.Errorf("%d units, a buffer of %d bytes: put back %d bytes, want %d", n, c, got, want)
			}
		}
	}
}
