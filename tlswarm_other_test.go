// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !(linux && (amd64 || arm64))

package quickjs // import "github.com/liustack/quickjs"

import "testing"

// Outside linux, the TLS stack of a runtime holds one block of about
// 1 MiB above its first segment from the start, so frames as deep as that
// go on one block, the same one every time: no call mallocs a segment or
// frees one, which takes the lock of libc's allocator.
func TestTLSWarm(t *testing.T) {
	m := newTestVM(t)
	tls := m.runtime.tls
	const frame, depth = 1024, 900 // 900 KiB
	run := func() []uintptr {
		var ps []uintptr
		for i := 0; i < depth; i++ {
			ps = append(ps, tls.Alloc(frame))
		}
		for range ps {
			tls.Free(frame)
		}
		return ps
	}
	first := run()
	// The first frames may fill the end of the first segment, the rest
	// go on the block in a row.
	k := 1
	for k < len(first) && first[k] == first[k-1]+frame {
		k++
	}
	for i := k + 1; i < len(first); i++ {
		if first[i] != first[i-1]+frame {
			t.Fatalf("frame %d at %#x, frame %d at %#x: not on one block", i-1, first[i-1], i, first[i])
		}
	}
	if k > 4 {
		t.Fatalf("%d frames before the block, want at most 4", k)
	}
	for round := 0; round < 3; round++ {
		again := run()
		for i := range again {
			if again[i] != first[i] {
				t.Fatalf("round %d: frame %d at %#x, before at %#x", round, i, again[i], first[i])
			}
		}
	}
	if got, want := markTLS(tls), m.runtime.base; got != want {
		t.Fatalf("the TLS stack is at %+v, want %+v", got, want)
	}
}
