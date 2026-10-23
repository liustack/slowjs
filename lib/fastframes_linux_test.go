// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux && (amd64 || arm64)

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

func TestFastFramesLayout(t *testing.T) {
	type slot struct {
		p  uintptr
		sz Tsize_t
	}
	type direct struct {
		pad          uintptr
		sp           int
		stack        []slot
		checkSignals bool
	}
	got, err := tlsLayoutOf(reflect.TypeFor[direct]())
	want := tlsOffsets{unsafe.Offsetof(direct{}.sp), unsafe.Offsetof(direct{}.stack), unsafe.Offsetof(direct{}.checkSignals)}
	if err != nil || got != want {
		t.Fatalf("direct fields: got %+v, %v, want %+v", got, err, want)
	}

	type withSP struct{ sp int }
	type withStack struct{ stack []slot }
	type withSignals struct{ checkSignals bool }
	for _, tc := range []struct {
		name, field string
		typ         reflect.Type
	}{
		{"sp value", "sp", reflect.TypeFor[struct {
			pad uintptr
			withSP
			stack        []slot
			checkSignals bool
		}]()},
		{"sp pointer", "sp", reflect.TypeFor[struct {
			pad uintptr
			*withSP
			stack        []slot
			checkSignals bool
		}]()},
		{"stack value", "stack", reflect.TypeFor[struct {
			sp int
			withStack
			checkSignals bool
		}]()},
		{"stack pointer", "stack", reflect.TypeFor[struct {
			sp int
			*withStack
			checkSignals bool
		}]()},
		{"signals value", "checkSignals", reflect.TypeFor[struct {
			sp    int
			stack []slot
			withSignals
		}]()},
		{"signals pointer", "checkSignals", reflect.TypeFor[struct {
			sp    int
			stack []slot
			*withSignals
		}]()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tlsLayoutOf(tc.typ); err == nil || !strings.Contains(err.Error(), tc.field+" is promoted") {
				t.Fatalf("promoted %s: %v", tc.field, err)
			}
		})
	}
}

// tlsTake and tlsDrop take and free the frames TLS.Alloc and TLS.Free
// would: the same addresses, the same number of slots, and a slot too
// small left to TLS.Alloc.
func TestFastFramesAsLibc(t *testing.T) {
	if err := tlsLayout(); err != nil {
		t.Fatal(err)
	}
	tls := libc.NewTLS()
	defer tls.Close()
	base := tls.StackSlots()
	sizes := []int{16, 48, 1280 + 768, 16, 160}
	// libc makes the slots.
	var want []uintptr
	for _, n := range sizes {
		want = append(want, tls.Alloc(n))
	}
	for range sizes {
		tls.Free(0)
	}
	// The fast path takes the same ones.
	for i, n := range sizes {
		p := tlsTake(tls, n)
		if p != want[i] {
			t.Fatalf("frame %d of %d bytes at %#x, libc gave %#x", i, n, p, want[i])
		}
		if got := tls.StackSlots(); got != base+i+1 {
			t.Fatalf("%d slots after %d frames, want %d", got, i+1, base+i+1)
		}
	}
	// Past the slots libc made, or in a slot too small, it takes none.
	if p := tlsTake(tls, 16); p != 0 {
		t.Fatalf("a frame past the slots libc made at %#x", p)
	}
	for i := len(sizes) - 1; i >= 0; i-- {
		if tlsDrop(tls, sizes[i]) {
			t.Fatal("tlsDrop reports a check for signals on a TLS without a handler")
		}
		if got := tls.StackSlots(); got != base+i {
			t.Fatalf("%d slots after freeing frame %d, want %d", got, i, base+i)
		}
	}
	if p := tlsTake(tls, 1280+768+1); p != 0 {
		t.Fatalf("a frame of %d bytes in a slot of %d at %#x", 1280+768+1, 16, p)
	}
	if got := tls.StackSlots(); got != base {
		t.Fatalf("%d slots after a frame that does not fit, want %d", got, base)
	}
	p := tls.Alloc(4096)
	if p == want[0] || tls.StackSlots() != base+1 {
		t.Fatalf("TLS.Alloc after the fast path: %#x, %d slots", p, tls.StackSlots())
	}
	if q := tlsTake(tls, 48); q != want[1] {
		t.Fatalf("the second slot at %#x after TLS.Alloc grew the first, want %#x", q, want[1])
	}
	tlsDrop(tls, 48)
	tlsDrop(tls, 4096)
	if got := tls.StackSlots(); got != base {
		t.Fatalf("%d slots at the end, want %d", got, base)
	}
}

// With checkSignals set, as libc sets it when a C signal handler is
// installed on the TLS, tlsDrop reports it and tlsSignals leaves the stack
// where tlsDrop left it.
func TestFastFramesSignals(t *testing.T) {
	if err := tlsLayout(); err != nil {
		t.Fatal(err)
	}
	tls := libc.NewTLS()
	defer tls.Close()
	base := tls.StackSlots()
	tls.Alloc(16)
	tls.Alloc(16)
	*(*bool)(unsafe.Add(unsafe.Pointer(tls), tlsSignalsOn)) = true
	defer func() { *(*bool)(unsafe.Add(unsafe.Pointer(tls), tlsSignalsOn)) = false }()
	if !tlsDrop(tls, 16) {
		t.Fatal("tlsDrop does not report the check for signals")
	}
	tlsSignals(tls)
	if got := tls.StackSlots(); got != base+1 {
		t.Fatalf("%d slots after tlsDrop and tlsSignals, want %d", got, base+1)
	}
	if !tlsDrop(tls, 16) {
		t.Fatal("tlsDrop does not report the check for signals")
	}
	tlsSignals(tls)
	if got := tls.StackSlots(); got != base {
		t.Fatalf("%d slots at the end, want %d", got, base)
	}
}
