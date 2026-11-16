// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"strings"
	"testing"
	"unsafe"
)

// TestTakeCopiesARoomyBuffer checks that take gives the bytes of a buffer
// that fits them without a copy, and copies those of a buffer a longer part
// grew before, which the string would keep reachable.
func TestTakeCopiesARoomyBuffer(t *testing.T) {
	for _, c := range []struct {
		n, room int
		copied  bool
	}{
		{1, 8, false},
		{100, 256, false},
		{40000, 65536, false},
		{1, 65536, true},
		{100, 4096, true},
	} {
		b := make([]byte, c.n, c.room)
		p := &parser{buf: b}
		s := p.take()
		if len(s) != c.n || p.buf != nil {
			t.Fatalf("%d in %d: %d bytes, buffer %v", c.n, c.room, len(s), p.buf != nil)
		}
		if copied := unsafe.StringData(s) != unsafe.SliceData(b); copied != c.copied {
			t.Errorf("%d bytes in a buffer of %d: copied %v, want %v", c.n, c.room, copied, c.copied)
		}
	}
}

// TestURLHoldsItsBytes parses inputs where ".." takes a long segment off, or
// a short part follows a long one in the buffer of the parser: the URL keeps
// no buffer much larger than its own bytes, and serializes as before.
func TestURLHoldsItsBytes(t *testing.T) {
	long := strings.Repeat("a", 60000)
	for _, c := range []struct{ in, href string }{
		{"http://h/" + long + "/..", "http://h/"},
		{"http://h/" + long + "/..?q#f", "http://h/?q#f"},
		{"http://h/x/" + long + "/../y", "http://h/x/y"},
		{"file:///C:/" + long + "/..", "file:///C:/"},
		{"sc:/" + long + "/..#f", "sc:/#f"},
	} {
		in, ok := NewInput(c.in)
		if !ok {
			t.Fatalf("%.30s: not an input", c.in)
		}
		u, err := Parse(in, nil)
		if err != nil {
			t.Fatalf("%.30s: %v", c.in, err)
		}
		if href, _ := u.Serialize(); href != c.href {
			t.Errorf("%.30s: %q, want %q", c.in, href, c.href)
		}
		if roomy(u.Path.b) {
			t.Errorf("%.30s: a path of %d bytes in a buffer of %d", c.in, len(u.Path.b), cap(u.Path.b))
		}
	}
}
