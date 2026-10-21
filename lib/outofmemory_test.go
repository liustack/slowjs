// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// fillMemory sets the memory limit of rt to what it uses and then takes
// every free block the engine's arenas have left, of each size. It returns
// the blocks, to free with Xjs_free_rt.
func fillMemory(tls *libc.TLS, rt uintptr) map[Tsize_t][]uintptr {
	XJS_SetMemoryLimit(tls, rt, (*TJSRuntime)(unsafe.Pointer(rt)).Fmalloc_ctx.Fmalloc_state.Fmalloc_size)
	blocks := map[Tsize_t][]uintptr{}
	for size := Tsize_t(4096); size >= 8; size -= 8 {
		for {
			p := Xjs_malloc_rt(tls, rt, size)
			if p == 0 {
				break
			}
			blocks[size] = append(blocks[size], p)
		}
	}
	return blocks
}

// When the memory left cannot hold an error, JS_ThrowError2 throws null,
// uncatchable, which the embedder reports as running out of memory. When it
// can hold the error object but not its message, it does the same, and never
// gives the error the exception marker as its message. Each case frees some
// blocks from a full heap, then throws a TypeError.
func TestThrowErrorOutOfMemory(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()

	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)

	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	// A long name makes the message longer than the error object, so that
	// the two come from blocks of different sizes.
	name, err := libc.CString(strings.Repeat("x", 150))
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, name)

	message, err := libc.CString("message")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, message)

	null := jsvMkPtr(EJS_TAG_NULL, 0)
	// Make the atom of the property name, so that reading it needs no
	// memory.
	XFreeValue(tls, ctx, XJS_GetPropertyStr(tls, ctx, null, name))
	XFreeValue(tls, ctx, XJS_GetException(tls, ctx))

	// Each case frees n blocks of each size from min to max.
	type room struct {
		min, max Tsize_t
		n        int
	}
	rooms := []room{{8, 8, 0}}
	for size := Tsize_t(8); size <= 512; size += 8 {
		rooms = append(rooms, room{size, size, 1})
	}
	for max := Tsize_t(64); max <= 4096; max *= 2 {
		for n := 1; n <= 4; n++ {
			rooms = append(rooms, room{8, max, n})
		}
	}

	nulls, whole := 0, 0
	for _, room := range rooms {
		blocks := fillMemory(tls, rt)
		for size, list := range blocks {
			if room.min <= size && size <= room.max {
				k := min(room.n, len(list))
				for _, p := range list[:k] {
					Xjs_free_rt(tls, rt, p)
				}
				blocks[size] = list[k:]
			}
		}

		XFreeValue(tls, ctx, XJS_GetPropertyStr(tls, ctx, null, name))
		uncatchable := XJS_IsUncatchableException(tls, ctx) != 0
		e := XJS_GetException(tls, ctx)
		XJS_SetUncatchableException(tls, ctx, 0)

		for _, list := range blocks {
			for _, p := range list {
				Xjs_free_rt(tls, rt, p)
			}
		}
		XJS_SetMemoryLimit(tls, rt, ^Tsize_t(0))

		switch tag(e) {
		case EJS_TAG_NULL:
			nulls++
			if !uncatchable {
				t.Errorf("room %v: null thrown, want it uncatchable", room)
			}
		case EJS_TAG_OBJECT:
			whole++
			m := XJS_GetPropertyStr(tls, ctx, e, message)
			if tag(m) != EJS_TAG_STRING {
				t.Errorf("room %v: error message has tag %d, want a string", room, tag(m))
			}
			XFreeValue(tls, ctx, m)
		default:
			t.Fatalf("room %v: thrown value has tag %d", room, tag(e))
		}
		XFreeValue(tls, ctx, e)
	}

	if nulls == 0 {
		t.Fatal("no room made the error fail")
	}
	t.Logf("%d rooms threw null, %d a whole error", nulls, whole)
}
