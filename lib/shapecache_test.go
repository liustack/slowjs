// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// shapeVM is a runtime and a context for the tests of the shape cache.
type shapeVM struct {
	t   testing.TB
	tls *libc.TLS
	rt  uintptr
	ctx uintptr
}

func newShapeVM(t testing.TB, n int32) *shapeVM {
	tls := libc.NewTLS()
	rt := XJS_NewRuntime(tls)
	ctx := XJS_NewContext(tls, rt)
	v := &shapeVM{t, tls, rt, ctx}
	if XJS_SetShapeCache(tls, ctx, n) != 0 {
		t.Fatal("XJS_SetShapeCache: out of memory")
	}
	return v
}

func (v *shapeVM) close() {
	XJS_SetShapeCache(v.tls, v.ctx, 0)
	XJS_FreeContext(v.tls, v.ctx)
	XJS_FreeRuntime(v.tls, v.rt)
	v.tls.Close()
}

// run evaluates js and returns its result as text.
func (v *shapeVM) run(js string) string {
	return eval1(v.t, v.tls, v.ctx, js)
}

// kept returns the shapes in the ring, oldest first.
func (v *shapeVM) kept() (r []uintptr) {
	sc := shapeCacheOf(v.rt)
	for i := range sc.n {
		if sh := *sc.slot((sc.next + i) % sc.n); sh != 0 {
			r = append(r, sh)
		}
	}
	if len(r) != int(sc.count) {
		v.t.Fatalf("%d shapes in the ring, count %d", len(r), sc.count)
	}
	return r
}

// shapes returns the number of shapes of the runtime.
func (v *shapeVM) shapes() int64 {
	// The engine writes the result, so it goes in C memory.
	n := int(unsafe.Sizeof(TJSMemoryUsage{}))
	p := v.tls.Alloc(n)
	defer v.tls.Free(n)
	XJS_ComputeMemoryUsage(v.tls, v.rt, p)
	return (*TJSMemoryUsage)(unsafe.Pointer(p)).Fshape_count
}

// inGCList reports whether the GC object at p is in the list of GC objects
// of the runtime or in its young list (see younggc.go), where a live shape
// is.
func (v *shapeVM) inGCList(p uintptr) bool {
	for _, head := range []uintptr{v.rt + unsafe.Offsetof(TJSRuntime{}.Fgc_obj_list), youngHead(v.rt)} {
		for el := (*Tlist_head)(unsafe.Pointer(head)).Fnext; el != head; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
			if el == p {
				return true
			}
		}
	}
	return false
}

// The offset of gc_phase that shapeKeep reads is the one JS_NewRuntime2
// writes.
func TestShapeCacheGCPhase(t *testing.T) {
	b, err := os.ReadFile(fmt.Sprintf("ccgo_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if os.IsNotExist(err) && runtime.GOOS == "windows" {
		b, err = os.ReadFile("ccgo_windows.go")
	}
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`libc\.SetBitFieldPtr8Int32\(rt\+(\d+), int32\(EJS_GC_PHASE_NONE\), 0, 0xff\)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("JS_NewRuntime2 does not set gc_phase")
	}
	if n, _ := strconv.Atoi(string(m[1])); uintptr(n) != gcPhaseOffset {
		t.Fatalf("gc_phase at %d, gcPhaseOffset %d", n, gcPhaseOffset)
	}
}

// The shapes an object literal goes through stay in the cache once the
// object is gone, and the next literal with the same keys goes through the
// same shapes: it makes no new one.
func TestShapeCacheKeeps(t *testing.T) {
	v := newShapeVM(t, 64)
	defer v.close()
	v.run("function mk(i) { return {a: i, b: 2, c: 3} }; 0")
	if got := v.run("mk(1); 0"); got != "0" {
		t.Fatal(got)
	}
	// The shapes of 1, 2 and 3 keys, last, and the empty shape before
	// them when no other object used it.
	kept := v.kept()
	if len(kept) < 3 || len(kept) > 4 {
		t.Fatalf("%d shapes kept, want 3 or 4", len(kept))
	}
	for i, sh := range kept {
		s := (*TJSShape)(unsafe.Pointer(sh))
		if want := int32(3 - len(kept) + 1 + i); s.Fprop_count != want || s.Fis_hashed == 0 || !v.inGCList(sh) {
			t.Errorf("shape %d: %d keys, want %d, hashed %d", i, s.Fprop_count, want, s.Fis_hashed)
		}
		if n := (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(v.tls, sh))).Fref_count; n != 1 {
			t.Errorf("shape %d: count %d, want 1", i, n)
		}
	}
	shapes := v.shapes()
	obj := eval0(t, v.tls, v.ctx, "mk(2)")
	defer XFreeValue(v.tls, v.ctx, obj)
	if sh := (*TJSObject)(unsafe.Pointer(jsvPtr(obj))).Fshape; sh != kept[len(kept)-1] {
		t.Errorf("the second object has the shape %#x, want the kept %#x", sh, kept[len(kept)-1])
	}
	if got := v.shapes(); got != shapes {
		t.Errorf("the second object made %d shapes", got-shapes)
	}
	if got := len(v.kept()); got != len(kept) {
		t.Errorf("%d shapes kept after the second object, want %d", got, len(kept))
	}
	if got := v.run("JSON.stringify([mk(3), mk(4)])"); got != `[{"a":3,"b":2,"c":3},{"a":4,"b":2,"c":3}]` {
		t.Errorf("objects read %s", got)
	}
}

// Shapes of objects of another prototype are not kept, nor are shapes with
// more than shapeMaxProps keys.
func TestShapeCacheWhich(t *testing.T) {
	v := newShapeVM(t, 64)
	defer v.close()
	// The prototype of the class is a plain object.
	v.run("class A { constructor() { this.a = 1; this.b = 2 } }; var o; 0")
	base := len(v.kept())
	v.run("o = Object.create(null); o.a = 1; o.b = 2; o = null; new A(); 0")
	if got := len(v.kept()); got != base {
		t.Errorf("%d shapes of other prototypes kept", got-base)
	}
	v.run("o = {k0: 0}; for (let i = 1; i < 40; i++) o['k' + i] = i; o = null; 0")
	kept := v.kept()[base:]
	if len(kept) != shapeMaxProps-1 {
		t.Errorf("%d shapes kept, want %d", len(kept), shapeMaxProps-1)
	}
	for _, sh := range kept {
		if n := (*TJSShape)(unsafe.Pointer(sh)).Fprop_count; n > shapeMaxProps {
			t.Errorf("a shape with %d keys kept", n)
		}
	}
}

// The ring keeps the last n shapes: when it is full, the oldest is let go.
// Turning the cache off lets go of every kept shape.
func TestShapeCacheRing(t *testing.T) {
	v := newShapeVM(t, 0)
	defer v.close()
	// hold keeps the empty shape in use, so the cache never keeps it.
	v.run("var hold = {}; function work() { for (let i = 0; i < 10; i++) { const o = {}; o['r' + i] = 1 } }; work(); 0")
	XJS_RunGC(v.tls, v.rt)
	base := v.shapes()

	if XJS_SetShapeCache(v.tls, v.ctx, 4) != 0 {
		t.Fatal("out of memory")
	}
	v.run("work(); 0")
	kept := v.kept()
	if len(kept) != 4 {
		t.Fatalf("%d shapes kept, want 4", len(kept))
	}
	for i, sh := range kept {
		if want := fmt.Sprintf("r%d", 6+i); v.atomName((*TJSShapeProperty)(unsafe.Pointer(_get_shape_prop(v.tls, sh))).Fatom) != want {
			t.Errorf("shape %d: not the shape of %s", i, want)
		}
	}
	if got := v.shapes(); got != base+4 {
		t.Errorf("%d shapes with 4 kept, %d without", got, base)
	}

	XJS_SetShapeCache(v.tls, v.ctx, 0)
	XJS_RunGC(v.tls, v.rt)
	if got := v.shapes(); got != base {
		t.Errorf("%d shapes after the cache is off, want %d", got, base)
	}
	if sc := shapeCacheOf(v.rt); *sc != (TshapeCache{}) {
		t.Errorf("the cache is not off: %+v", *sc)
	}
}

func (v *shapeVM) atomName(a TJSAtom) string {
	p := XJS_AtomToCStringLen(v.tls, v.ctx, 0, a)
	defer XJS_FreeCString(v.tls, v.ctx, p)
	return libc.GoString(p)
}

// The cycle collector frees the shapes of a cycle itself: the cache keeps
// none of them, and the shapes it kept before stay alive.
func TestShapeCacheCycles(t *testing.T) {
	v := newShapeVM(t, 64)
	defer v.close()
	v.run("({p: 1, q: 2}); 0")
	before := v.kept()
	v.run("for (let i = 0; i < 100; i++) { const a = {x: i}, b = {y: a}; a.b = b }; 0")
	during := v.kept()
	XJS_RunGC(v.tls, v.rt)
	after := v.kept()
	if len(after) != len(during) {
		t.Errorf("%d shapes kept before the GC, %d after", len(during), len(after))
	}
	for _, sh := range after {
		if !v.inGCList(sh) {
			t.Errorf("a kept shape is not alive after the GC")
		}
	}
	if len(before) == 0 || len(after) < len(before) {
		t.Fatalf("%d shapes kept before the cycles, %d after the GC", len(before), len(after))
	}
	for i, sh := range before {
		if after[i] != sh {
			t.Errorf("the GC let go of kept shape %d", i)
		}
	}
}

// JS_FreeRuntime lets go of the kept shapes, even when the cache is still on
// after the context is freed.
func TestShapeCacheFreeRuntime(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	ctx := XJS_NewContext(tls, rt)
	if XJS_SetShapeCache(tls, ctx, 64) != 0 {
		t.Fatal("out of memory")
	}
	if got := eval1(t, tls, ctx, "({a: 1, b: 2}); 0"); got != "0" {
		t.Fatal(got)
	}
	if shapeCacheOf(rt).count == 0 {
		t.Fatal("no shape kept")
	}
	XJS_FreeContext(tls, ctx)
	XJS_FreeRuntime(tls, rt)
}

// Without memory for the ring, the cache stays off.
func TestShapeCacheOutOfMemory(t *testing.T) {
	v := newShapeVM(t, 8)
	defer v.close()
	ms := &(*TJSRuntime)(unsafe.Pointer(v.rt)).Fmalloc_ctx.Fmalloc_state
	XJS_SetMemoryLimit(v.tls, v.rt, ms.Fmalloc_size)
	defer XJS_SetMemoryLimit(v.tls, v.rt, ^Tsize_t(0))
	if XJS_SetShapeCache(v.tls, v.ctx, 1<<20) != -1 {
		t.Fatal("a ring of 8 MiB past the memory limit")
	}
	if sc := shapeCacheOf(v.rt); *sc != (TshapeCache{}) {
		t.Errorf("the cache is not off: %+v", *sc)
	}
}

// Each runtime has a cache of its own, so runtimes on several goroutines
// keep and reuse shapes without sharing one.
func TestShapeCacheRuntimes(t *testing.T) {
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for g := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := newShapeVM(t, 16)
			defer v.close()
			js := fmt.Sprintf("(() => { let s = ''; for (let i = 0; i < 2000; i++) { const o = {g%d: i}; o['k' + (i %% 40)] = i; s = JSON.stringify(o) }; return s })()", g)
			want := fmt.Sprintf(`{"g%d":1999,"k39":1999}`, g)
			for range 5 {
				if got := v.run(js); got != want {
					errs <- fmt.Errorf("runtime %d: %s, want %s", g, got, want)
					return
				}
				XJS_RunGC(v.tls, v.rt)
			}
			if got := len(v.kept()); got != 16 {
				errs <- fmt.Errorf("runtime %d: %d shapes kept, want 16", g, got)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// A shape with a symbol key that is not registered is not kept: it would
// keep the symbol alive, and a WeakRef or FinalizationRegistry with the
// symbol as its target would see it live on after the last object with the
// key is gone. Registered symbols cannot be such targets, and stay kept.
func TestShapeCacheSymbols(t *testing.T) {
	for _, n := range []int32{0, 64} {
		v := newShapeVM(t, n)
		v.run(`var wr; (function () {
			const s = Symbol("weak key"), o = {a: 1}
			o[s] = 2
			wr = new WeakRef(s)
		})(); 0`)
		XJS_RunGC(v.tls, v.rt)
		if got := v.run("String(wr.deref())"); got != "undefined" {
			t.Errorf("cache of %d: the symbol lives on after its object: %s", n, got)
		}
		for _, sh := range v.kept() {
			s := (*TJSShape)(unsafe.Pointer(sh))
			for i := range s.Fprop_count {
				a := (*TJSShapeProperty)(unsafe.Pointer(_get_shape_prop(v.tls, sh) + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{}))).Fatom
				if v.atomName(a) == "weak key" {
					t.Errorf("cache of %d: a shape with the symbol key is kept", n)
				}
			}
		}
		before := len(v.kept())
		v.run(`(function () { const o = {b: 1}; o[Symbol.for("r")] = 2 })(); 0`)
		if n != 0 && len(v.kept()) == before {
			t.Errorf("the shape with a registered symbol key is not kept")
		}
		v.close()
	}
}

// A ring whose size in bytes does not fit in a size_t is refused, as one
// past the memory limit is, and the cache stays off.
func TestShapeCacheTooLarge(t *testing.T) {
	v := newShapeVM(t, 8)
	defer v.close()
	ms := &(*TJSRuntime)(unsafe.Pointer(v.rt)).Fmalloc_ctx.Fmalloc_state
	// On a 64-bit target the ring fits in a size_t: the memory limit refuses
	// it there, without an allocation of 8 GiB. On a 32-bit target its size
	// would wrap around to 4 bytes, which the limit leaves room for.
	XJS_SetMemoryLimit(v.tls, v.rt, ms.Fmalloc_size+1<<20)
	defer XJS_SetMemoryLimit(v.tls, v.rt, ^Tsize_t(0))
	if XJS_SetShapeCache(v.tls, v.ctx, 1<<30+1) != -1 {
		t.Fatal("a ring of 2^30+1 shapes")
	}
	if sc := shapeCacheOf(v.rt); *sc != (TshapeCache{}) {
		t.Errorf("the cache is not off: %+v", *sc)
	}
}

// An object that gets keys no other object has, one at a time, has its
// shape copied for each key only while shapes found pay for the copies:
// once the credit is spent its shape grows in place, and only its last
// shape is kept. Objects whose keys come again still end up on kept
// shapes, without a copy, and earn the credit back.
func TestShapeCacheCredit(t *testing.T) {
	v := newShapeVM(t, 4096)
	defer v.close()
	// hold keeps the empty shape in use, so the cache never keeps it.
	v.run(`var hold = {}
	function unique(from, n) {
		for (let r = from; r < from + n; r++) {
			const o = {}
			for (let k = 0; k < 8; k++) o["u" + r + "_" + k] = k
		}
	}
	function mk(i) { return {a: i, b: 2, c: 3, d: 4} }
	unique(0, 100); 0`)
	if sc := shapeCacheOf(v.rt); sc.credit != 0 {
		t.Fatalf("credit %d after objects of keys no other has", sc.credit)
	}
	before := len(v.kept())
	v.run("unique(100, 100); 0")
	// Copying every key would keep 8 shapes an object. A shape found on
	// the way, of another object, may pay for a copy.
	if got := len(v.kept()) - before; got < 100 || got > 110 {
		t.Errorf("100 objects of 8 keys no other has: %d shapes kept, want 100 to 110", got)
	}

	v.run("for (let i = 0; i < 10; i++) mk(i); 0")
	if sc := shapeCacheOf(v.rt); sc.credit == 0 {
		t.Error("no credit earned by objects with the same keys")
	}
	shapes := v.shapes()
	obj := eval0(t, v.tls, v.ctx, "mk(10)")
	defer XFreeValue(v.tls, v.ctx, obj)
	if got := v.shapes(); got != shapes {
		t.Errorf("the eleventh object with the same keys made %d shapes", got-shapes)
	}
}
