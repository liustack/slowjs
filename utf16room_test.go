// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// The conversion is the reference's, byte for byte, where the real cuts fall:
// stringStep (262,144 units), where utf16Steps ends a piece and polls, and
// stepScratch (16,384 units), where it ends a write through its buffer. Each
// string is the length of a cut from 9 units short to 9 over, of ASCII (so
// that the 8-unit blocks run across the cut) or of Chinese, with a pair, a
// lone surrogate, a character of two or three bytes or a run of ASCII at
// each place from 2 units before the cut to 1 after.
func TestUTF16RealCuts(t *testing.T) {
	if testing.Short() {
		t.Skip("long strings")
	}
	kinds := []struct {
		name  string
		units []uint16
	}{
		{"pair", []uint16{0xD83D, 0xDE00}},
		{"high", []uint16{0xD800}},
		{"low", []uint16{0xDC00}},
		{"two", []uint16{0xE9}},
		{"three", []uint16{0x4E2D}},
		{"ascii", []uint16{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i'}},
	}
	// stepScratch cuts inside a piece of utf16Steps, so only a string longer
	// than stringStep meets it: its cuts are taken at the first two places
	// of a string of stringStep+40 units.
	type cut struct{ at, length int }
	var cuts []cut
	for d := -9; d <= 9; d++ {
		cuts = append(cuts, cut{stringStep + d, stringStep + d}, cut{stepScratch, stepScratch + d})
	}
	for _, at := range []int{stepScratch, 2 * stepScratch} {
		cuts = append(cuts, cut{at, stringStep + 40})
	}
	for _, c := range cuts {
		for _, fill := range []uint16{'x', 0x6587} {
			for _, k := range kinds {
				for off := -2; off <= 1; off++ {
					u := make([]uint16, c.length)
					for i := range u {
						u[i] = fill
					}
					at := c.at + off
					if at < 0 || at >= len(u) {
						continue
					}
					copy(u[at:], k.units)
					for _, wtf8 := range []bool{false, true} {
						want := utf16StringRef(u, wtf8)
						got, ok := utf16String(nil, u, wtf8)
						if !ok || got != want {
							t.Fatalf("%d units of %#x, %s at %d, wtf8 %v: %s", len(u), fill, k.name, at, wtf8, firstDiff(got, want))
						}
						if n := utf16Len(u); n != len(want) {
							t.Fatalf("%d units of %#x, %s at %d: utf16Len %d, want %d", len(u), fill, k.name, at, n, len(want))
						}
					}
				}
			}
		}
	}
}

func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	return fmt.Sprintf("%d bytes, want %d, first difference at byte %d", len(got), len(want), i)
}

// canary is what the bytes past what a write may touch hold before it.
const canary = 0xA5

// asciiUnits and utf16Write write nothing past what they report: for every
// length up to 40 and every start, of ASCII and of each other kind of unit
// among it, the bytes after the ones written keep their canary, in the room
// of 3 bytes a unit and past it.
func TestUTF16WritesStayInRoom(t *testing.T) {
	fills := []func(i int) uint16{
		func(i int) uint16 { return uint16('a' + i%26) },
		func(i int) uint16 {
			return [...]uint16{'a', 'b', 0xE9, 'c', 0x4E2D, 'd', 0xD83D, 0xDE00, 'e', 0xDC00}[i%10]
		},
		func(i int) uint16 { return [...]uint16{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 0x4E2D}[i%10] },
	}
	for f, fill := range fills {
		for n := 0; n <= 40; n++ {
			u := make([]uint16, n)
			for i := range u {
				u[i] = fill(i)
			}
			for i := 0; i <= n; i++ {
				// asciiUnits from i, written at j = 5 of a buffer that has
				// room for 3 bytes a unit after j and 16 canary bytes after it.
				const j = 5
				room := 3 * (n - i)
				out := make([]byte, j+room+16)
				for b := range out {
					out[b] = canary
				}
				k := asciiUnits(unsafe.Pointer(unsafe.SliceData(out)), j, u, i)
				// First the canary outside the room, whatever k says.
				for b, c := range out {
					if (b < j || b >= j+room) && c != canary {
						t.Fatalf("fill %d, %d units from %d: asciiUnits changed byte %d of %d, outside the room from %d to %d (it says it wrote %d)", f, n, i, b, len(out), j, j+room, k)
					}
				}
				if k < 0 || k%8 != 0 || i+k > n {
					t.Fatalf("fill %d, %d units from %d: asciiUnits wrote %d", f, n, i, k)
				}
				for b, c := range out {
					written := b >= j && b < j+k
					if !written && c != canary {
						t.Fatalf("fill %d, %d units from %d: asciiUnits wrote %d units and changed byte %d", f, n, i, k, b)
					}
					if written && c != byte(u[i+b-j]) {
						t.Fatalf("fill %d, %d units from %d: byte %d is %#x, want %#x", f, n, i, b, c, u[i+b-j])
					}
				}
				if got := asciiUnitsLen(u[i:]); got != k {
					t.Fatalf("fill %d, %d units from %d: asciiUnitsLen %d, asciiUnits %d", f, n, i, got, k)
				}
			}
			for _, wtf8 := range []bool{false, true} {
				buf := make([]byte, 3*n+16)
				for b := range buf {
					buf[b] = canary
				}
				w := utf16Write(buf[:3*n], u, wtf8)
				if want := utf16StringRef(u, wtf8); string(buf[:w]) != want {
					t.Fatalf("fill %d, %d units, wtf8 %v: %q, want %q", f, n, wtf8, buf[:w], want)
				}
				for b := w; b < len(buf); b++ {
					if buf[b] != canary {
						t.Fatalf("fill %d, %d units, wtf8 %v: utf16Write wrote %d bytes and changed byte %d (room ends at %d)", f, n, wtf8, w, b, 3*n)
					}
				}
			}
		}
	}
}

// The scratch memory utf16String keeps in utf16Scratch is written again by
// the next string: a long string of ASCII with a little Chinese, then a
// shorter one, then a longer one, each of its own letters, come out as the
// reference makes them, with no byte of an earlier one. The pool is the real
// one: a buffer is put in before the first string so that the strings share
// it, and the test reads New to see that they did.
func TestUTF16ScratchReused(t *testing.T) {
	saved := utf16Scratch.New
	t.Cleanup(func() { utf16Scratch.New = saved })
	made := 0
	utf16Scratch.New = func() any { made++; return saved() }
	page := func(letter string, n int) []uint16 {
		var b strings.Builder
		for b.Len() < n {
			b.WriteString(`<div class="` + strings.Repeat(letter, 9) + `">` + strings.Repeat(letter, 30) + `</div>`)
			if b.Len()%7 == 0 {
				b.WriteString("中")
			}
		}
		return utf16.Encode([]rune(b.String()[:n]))
	}
	strs := [][]uint16{page("A", 60000), page("b", 3000), page("C", 120000)}
	for try := 0; ; try++ {
		made = 0
		p := new([]byte)
		*p = make([]byte, 3*60000)
		for i := range *p {
			(*p)[i] = 'Z'
		}
		utf16Scratch.Put(p)
		for k, u := range strs {
			want := utf16StringRef(u, false)
			got, _ := utf16String(nil, u, false)
			if got != want {
				t.Fatalf("string %d: %s", k, firstDiff(got, want))
			}
			for _, other := range []string{"A", "b", "C", "Z"} {
				if strings.Contains(got, other) != strings.Contains(want, other) {
					t.Fatalf("string %d holds %q of another string", k, other)
				}
			}
		}
		if made == 0 {
			break
		}
		if try == 9 {
			t.Fatalf("the pool made %d buffers in each of 10 tries: the strings never shared one", made)
		}
	}
}
