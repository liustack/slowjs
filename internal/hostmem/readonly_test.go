// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"bytes"
	"testing"
)

// TestReadOnly: MapReadOnly holds a copy of the bytes, which can be read to
// the end of its pages and faults when written, for any size, none too.
func TestReadOnly(t *testing.T) {
	for _, n := range []int{0, 1, int(pageSize), int(pageSize) + 1, 3 * Unit} {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i*7 + 1)
		}
		p, err := MapReadOnly(b)
		if err != nil {
			t.Fatal(err)
		}
		size := ReadOnlySize(n)
		if size < uintptr(n) || size%pageSize != 0 || size == 0 {
			t.Fatalf("ReadOnlySize(%d) = %d, want whole pages that hold the bytes", n, size)
		}
		if got := bytesAt(p, uintptr(n)); !bytes.Equal(got, b) {
			t.Fatalf("the mapping of %d bytes does not hold them", n)
		}
		if !readable(p, size) {
			t.Fatalf("the mapping of %d bytes cannot be read to its end", n)
		}
		for _, addr := range []uintptr{p, p + size - 1} {
			if at, other := faultAt(addr); at != addr || other != nil {
				t.Fatalf("writing the mapping of %d bytes at %#x: fault at %#x, %v, want a fault there", n, addr, at, other)
			}
		}
		if err := UnmapReadOnly(p, n); err != nil {
			t.Fatal(err)
		}
	}
}
