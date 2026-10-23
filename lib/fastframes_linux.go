// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux && (amd64 || arm64)

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"fmt"
	"reflect"
	"unsafe"

	"modernc.org/libc"
)

// On linux, libc keeps the TLS stack as a list of slots, one block of
// memory each, and TLS.Alloc takes the next slot when its block is large
// enough, which it is nearly always once a VM has run for a while. Go does
// not inline TLS.Alloc (it is over the inlining budget) nor, in a large
// function, TLS.Free, so every TLS frame of the engine cost two calls.
// internal/fixlib takes and frees the frames of the generated files of
// linux through tlsTake and tlsDrop instead (see fixFastFrames), which Go
// inlines:
//
//	bp := tlsTake(tls, 48)
//	if bp == 0 {
//		bp = tls.Alloc(48)
//	}
//	...
//	if tlsDrop(tls, 48) {
//		tlsSignals(tls)
//	}
//
// tlsTake does what TLS.Alloc does when the next slot is large enough, and
// returns 0 when it is not, for TLS.Alloc to do the rest. tlsDrop steps back
// a slot as TLS.Free does, and reports whether libc checks for signals on a
// free, which it does once a C signal handler was installed through libc on
// that TLS. tlsSignals then lets TLS.Free run that check. The stack, its
// slots and the addresses the engine gets are the same as through libc, so
// the stack check, which counts the slots (see js_check_stack_overflow),
// sees the same count at every point.
//
// The fields of libc.TLS these read are not exported. Their offsets are
// found by name and checked to be direct fields of the expected types,
// and the program stops there if libc changed them. internal/fixlib checks
// the text of TLS.Alloc, TLS.Free and the TLS struct of the libc release in
// go.mod against fingerprints (fastFrameSites), so a libc release that
// changes how the stack is kept stops it until it is read again.

// tlsSlot is libc's tlsStackSlot: a block of the TLS stack and its usable
// size. On linux size_t has the size of a pointer.
type tlsSlot struct {
	p  uintptr
	sz uintptr
}

// The offsets in libc.TLS of the index of the next slot (sp), of the slots
// (stack) and of whether a free checks for signals (checkSignals).
var tlsSP, tlsStack, tlsSignalsOn uintptr

func init() {
	if err := tlsLayout(); err != nil {
		panic(fmt.Sprintf("slowjs: %v: modernc.org/libc keeps its TLS stack in another way than internal/fixlib expects (see docs/fork-changes/calls-and-tls-frames.md)", err))
	}
}

// tlsLayout finds the fields of libc.TLS that tlsTake and tlsDrop use, and
// checks their types before publishing the offsets.
func tlsLayout() error {
	layout, err := tlsLayoutOf(reflect.TypeFor[libc.TLS]())
	if err != nil {
		return err
	}
	tlsSP, tlsStack, tlsSignalsOn = layout.sp, layout.stack, layout.signals
	return nil
}

type tlsOffsets struct {
	sp, stack, signals uintptr
}

// tlsLayoutOf checks the TLS type without changing the live offsets.
func tlsLayoutOf(t reflect.Type) (tlsOffsets, error) {
	var layout tlsOffsets
	field := func(name string, want reflect.Type) (uintptr, error) {
		f, ok := t.FieldByName(name)
		if !ok {
			return 0, fmt.Errorf("libc.TLS has no field %s", name)
		}
		if len(f.Index) != 1 {
			return 0, fmt.Errorf("libc.TLS.%s is promoted, want a direct field", name)
		}
		if f.Type != want {
			return 0, fmt.Errorf("libc.TLS.%s is a %v, want a %v", name, f.Type, want)
		}
		return f.Offset, nil
	}
	var err error
	if layout.sp, err = field("sp", reflect.TypeFor[int]()); err != nil {
		return tlsOffsets{}, err
	}
	if layout.signals, err = field("checkSignals", reflect.TypeFor[bool]()); err != nil {
		return tlsOffsets{}, err
	}
	f, ok := t.FieldByName("stack")
	if !ok {
		return tlsOffsets{}, fmt.Errorf("libc.TLS has no field stack")
	}
	if len(f.Index) != 1 {
		return tlsOffsets{}, fmt.Errorf("libc.TLS.stack is promoted, want a direct field")
	}
	if f.Type.Kind() != reflect.Slice {
		return tlsOffsets{}, fmt.Errorf("libc.TLS.stack is a %v, want a slice", f.Type)
	}
	s := f.Type.Elem()
	if s.Kind() != reflect.Struct || s.NumField() != 2 || s.Size() != unsafe.Sizeof(tlsSlot{}) {
		return tlsOffsets{}, fmt.Errorf("libc.TLS.stack is a %v, want a slice of {p uintptr, sz size_t}", f.Type)
	}
	p, sz := s.Field(0), s.Field(1)
	if p.Name != "p" || p.Type.Kind() != reflect.Uintptr || p.Offset != unsafe.Offsetof(tlsSlot{}.p) {
		return tlsOffsets{}, fmt.Errorf("libc.TLS.stack is a %v, want a slice of {p uintptr, sz size_t}", f.Type)
	}
	if sz.Name != "sz" || sz.Type.Size() != unsafe.Sizeof(uintptr(0)) || sz.Offset != unsafe.Offsetof(tlsSlot{}.sz) ||
		(sz.Type.Kind() != reflect.Uint32 && sz.Type.Kind() != reflect.Uint64) {
		return tlsOffsets{}, fmt.Errorf("libc.TLS.stack is a %v, want a slice of {p uintptr, sz size_t}", f.Type)
	}
	layout.stack = f.Offset
	return layout, nil
}

// tlsTake takes a TLS frame of n bytes, as TLS.Alloc does, when the next
// slot of the stack is large enough, and returns 0 when it is not.
func tlsTake(tls *libc.TLS, n int) uintptr {
	t := unsafe.Pointer(tls)
	sp := *(*int)(unsafe.Add(t, tlsSP))
	stack := *(*[]tlsSlot)(unsafe.Add(t, tlsStack))
	if uint(sp) < uint(len(stack)) && stack[sp].sz >= uintptr(n) {
		*(*int)(unsafe.Add(t, tlsSP)) = sp + 1
		return stack[sp].p
	}
	return 0
}

// tlsDrop frees the TLS frame of n bytes taken last, as TLS.Free does, and
// reports whether TLS.Free would check for signals now. n is not used: it
// is the size TLS.Free was given, kept so that the call reads as the free
// it stands for.
func tlsDrop(tls *libc.TLS, n int) bool {
	t := unsafe.Pointer(tls)
	*(*int)(unsafe.Add(t, tlsSP))--
	return *(*bool)(unsafe.Add(t, tlsSignalsOn))
}

// tlsSignals runs the check for signals of TLS.Free after tlsDrop: it
// takes the slot back for TLS.Free to free.
//
//go:noinline
func tlsSignals(tls *libc.TLS) {
	*(*int)(unsafe.Add(unsafe.Pointer(tls), tlsSP))++
	tls.Free(0)
}
