// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// Tests of the stacks lib/backtrace.go formats when they are first read.
// The errors are made by functions called from Go, so that no top-level code
// is on their stack and the stack is not formatted when they are made.

type btVM struct {
	t       *testing.T
	tls     *libc.TLS
	rt, ctx uintptr
}

func newBtVM(t *testing.T) *btVM {
	tls := libc.NewTLS()
	rt := XJS_NewRuntime(tls)
	ctx := XJS_NewContext(tls, rt)
	t.Cleanup(func() {
		XJS_FreeContext(tls, ctx)
		XJS_FreeRuntime(tls, rt)
		tls.Close()
	})
	return &btVM{t, tls, rt, ctx}
}

// usage runs the GC and returns the memory usage of the runtime.
func (v *btVM) usage() TJSMemoryUsage {
	XJS_RunGC(v.tls, v.rt)
	n := int(unsafe.Sizeof(TJSMemoryUsage{}))
	p := v.tls.Alloc(n)
	defer v.tls.Free(n)
	XJS_ComputeMemoryUsage(v.tls, v.rt, p)
	return *(*TJSMemoryUsage)(unsafe.Pointer(p))
}

// limit lets the runtime allocate room more bytes, or any amount if room < 0.
func (v *btVM) limit(room int64) {
	if room < 0 {
		XJS_SetMemoryLimit(v.tls, v.rt, Size_t(^uintptr(0)))
		return
	}
	XJS_SetMemoryLimit(v.tls, v.rt, Size_t(v.usage().Fmalloc_size+room))
}

func (v *btVM) eval(src string) TJSValue {
	v.t.Helper()
	r := eval0(v.t, v.tls, v.ctx, src)
	if tag(r) == EJS_TAG_EXCEPTION {
		v.t.Fatalf("eval: %s", v.exception())
	}
	return r
}

// call calls the global function name from Go and returns its result.
func (v *btVM) call(name string) TJSValue {
	v.t.Helper()
	f := v.eval(name)
	defer XFreeValue(v.tls, v.ctx, f)
	global := XJS_GetGlobalObject(v.tls, v.ctx)
	defer XFreeValue(v.tls, v.ctx, global)
	r := XJS_Call(v.tls, v.ctx, f, global, 0, 0)
	if tag(r) == EJS_TAG_EXCEPTION {
		v.t.Fatalf("call %s: %s", name, v.exception())
	}
	return r
}

// exception takes the pending exception and returns its text, or "" if
// there is none.
func (v *btVM) exception() string {
	if XJS_HasException(v.tls, v.ctx) == 0 {
		return ""
	}
	e := XJS_GetException(v.tls, v.ctx)
	defer XFreeValue(v.tls, v.ctx, e)
	p := XToCString(v.tls, v.ctx, e)
	if p == 0 {
		XFreeValue(v.tls, v.ctx, XJS_GetException(v.tls, v.ctx))
		return "<exception>"
	}
	defer XJS_FreeCString(v.tls, v.ctx, p)
	return libc.GoString(p)
}

// text frees val and returns it as text, or a description of its tag.
func (v *btVM) text(val TJSValue) string {
	defer XFreeValue(v.tls, v.ctx, val)
	switch tag(val) {
	case EJS_TAG_STRING:
		p := XToCString(v.tls, v.ctx, val)
		defer XJS_FreeCString(v.tls, v.ctx, p)
		return libc.GoString(p)
	case EJS_TAG_INT:
		return fmt.Sprint(jsvToInt32(val))
	case EJS_TAG_EXCEPTION:
		return "<exception>"
	}
	return fmt.Sprintf("<tag %d>", tag(val))
}

func (v *btVM) stack(obj TJSValue) string {
	return v.text(XGetProperty(v.tls, v.ctx, obj, EJS_ATOM_stack))
}

const deepError = `
function deep(n) { return n ? deep(n - 1) : new Error("deep") }
function make() { return deep(40) }
`

// pendingErr makes an error in a call from Go, so its stack stays pending,
// and returns it. The caller frees it.
func (v *btVM) pendingErr() TJSValue {
	v.t.Helper()
	XFreeValue(v.tls, v.ctx, v.eval(deepError))
	return v.call("make")
}

var rooms = []int64{0, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384}

// Reading the stacks of errors kept alive must not give each error a shape
// of its own: the formatted stack is a plain data property, as when the
// stack was formatted at once.
func TestBacktraceReadSharesShape(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`
var held
function make() { held = []; for (var i = 0; i < 10000; i++) held.push(new Error("kept")) }
function read() { for (var e of held) void e.stack }
`))
	XFreeValue(v.tls, v.ctx, v.call("make"))
	pending := v.usage()
	XFreeValue(v.tls, v.ctx, v.call("read"))
	read := v.usage()
	t.Logf("shapes: %d pending, %d read", pending.Fshape_count, read.Fshape_count)
	if read.Fshape_count > pending.Fshape_count+2 {
		t.Fatalf("reading 10000 stacks added %d shapes", read.Fshape_count-pending.Fshape_count)
	}
}

// Errors that get another property while their stack is pending also share
// a shape once their stacks are read.
func TestBacktraceReadSharesShapeAfterChange(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`
var held
function make() { held = []; for (var i = 0; i < 1000; i++) { var e = new Error("kept"); e.code = i; held.push(e) } }
function read() { for (var e of held) void e.stack }
`))
	XFreeValue(v.tls, v.ctx, v.call("make"))
	pending := v.usage()
	XFreeValue(v.tls, v.ctx, v.call("read"))
	read := v.usage()
	t.Logf("shapes: %d pending, %d read", pending.Fshape_count, read.Fshape_count)
	if read.Fshape_count > pending.Fshape_count+2 {
		t.Fatalf("reading 1000 stacks added %d shapes", read.Fshape_count-pending.Fshape_count)
	}
}

// When there is no memory to format a stack, reading it throws, and the
// stack stays pending: reading it again later gives the whole text.
func TestBacktraceReadRetriesAfterOOM(t *testing.T) {
	for _, room := range rooms {
		t.Run(fmt.Sprint(room), func(t *testing.T) {
			v := newBtVM(t)
			XFreeValue(v.tls, v.ctx, v.eval(deepError))
			ref := v.call("make")
			want := v.stack(ref)
			XFreeValue(v.tls, v.ctx, ref)

			e := v.call("make")
			defer XFreeValue(v.tls, v.ctx, e)
			v.limit(room)
			first := XGetProperty(v.tls, v.ctx, e, EJS_ATOM_stack)
			v.limit(-1)
			checkRead(t, v, first, want)
			if got := v.stack(e); got != want {
				t.Fatalf("read again: got %q, want %q", got, want)
			}
		})
	}
}

// checkRead checks a read of a stack with little memory: it either gives the
// whole text and leaves no exception, or throws out of memory.
func checkRead(t *testing.T, v *btVM, got TJSValue, want string) {
	t.Helper()
	isException := tag(got) == EJS_TAG_EXCEPTION
	text := v.text(got)
	ex := v.exception()
	switch {
	case isException && !strings.Contains(ex, "out of memory"):
		t.Fatalf("read threw %q, want out of memory", ex)
	case !isException && ex != "":
		t.Fatalf("read gave %q and left the exception %q", text, ex)
	case !isException && text != want:
		t.Fatalf("read gave %q, want %q", text, want)
	}
}

// A function name with characters outside ASCII is converted to UTF-8 when
// the stack is formatted. If that conversion runs out of memory, the read
// throws, and the stack stays pending.
func TestBacktraceUnicodeNameOOM(t *testing.T) {
	for _, ch := range []string{"é", "中", "😀"} {
		for _, room := range rooms {
			t.Run(fmt.Sprintf("%s/%d", ch, room), func(t *testing.T) {
				v := newBtVM(t)
				XFreeValue(v.tls, v.ctx, v.eval(fmt.Sprintf(`
function named() { return new Error("unicode") }
Object.defineProperty(named, "name", { value: %q.repeat(10000) })
`, ch)))
				ref := v.call("named")
				want := v.stack(ref)
				XFreeValue(v.tls, v.ctx, ref)
				if !strings.HasPrefix(want, "    at "+ch) {
					t.Fatalf("stack %.40q does not show the name", want)
				}

				e := v.call("named")
				defer XFreeValue(v.tls, v.ctx, e)
				v.limit(room)
				first := XGetProperty(v.tls, v.ctx, e, EJS_ATOM_stack)
				v.limit(-1)
				checkRead(t, v, first, want)
				if got := v.stack(e); got != want {
					t.Fatalf("read again: got %.60q, want %.60q", got, want)
				}
			})
		}
	}
}

// Assigning or defining 'stack' replaces the pending stack without
// formatting it, so it works with no memory to spare, as it does when the
// stack is a string.
func TestBacktraceOverwriteDoesNotFormat(t *testing.T) {
	for _, kind := range []string{"set", "define"} {
		for _, room := range rooms {
			t.Run(fmt.Sprintf("%s/%d", kind, room), func(t *testing.T) {
				v := newBtVM(t)
				XFreeValue(v.tls, v.ctx, v.eval(deepError))
				e := v.call("make")
				defer XFreeValue(v.tls, v.ctx, e)
				v.limit(room)
				var ret int32
				switch kind {
				case "set":
					ret = XSetProperty(v.tls, v.ctx, e, EJS_ATOM_stack, XNewInt32(v.tls, v.ctx, 42))
				case "define":
					ret = XJS_DefinePropertyValue(v.tls, v.ctx, e, EJS_ATOM_stack, XNewInt32(v.tls, v.ctx, 42), MJS_PROP_CONFIGURABLE|MJS_PROP_WRITABLE)
				}
				v.limit(-1)
				if ex := v.exception(); ret != 1 || ex != "" {
					t.Fatalf("%s returned %d, exception %q", kind, ret, ex)
				}
				if got := v.stack(e); got != "42" {
					t.Fatalf("stack is %q after %s", got, kind)
				}
			})
		}
	}
}

// Printing an error reads its stack from C, through get_prop_string. That
// read formats the stack once and keeps the text, like a read from JS.
func TestBacktraceNativeReadCaches(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(deepError))
	e := v.call("make")
	defer XFreeValue(v.tls, v.ctx, e)
	p1 := _get_prop_string(v.tls, v.ctx, e, EJS_ATOM_stack)
	p2 := _get_prop_string(v.tls, v.ctx, e, EJS_ATOM_stack)
	if p1 == 0 || p2 == 0 {
		t.Fatalf("get_prop_string: %#x %#x", p1, p2)
	}
	defer XJS_FreeCString(v.tls, v.ctx, p1)
	defer XJS_FreeCString(v.tls, v.ctx, p2)
	want := v.stack(e)
	if got := libc.GoString(p1); got != want {
		t.Fatalf("get_prop_string gave %q, want %q", got, want)
	}
	if p1 != p2 {
		t.Fatal("each get_prop_string formatted the stack again")
	}
}

// JS_ComputeMemoryUsage counts the frames of a pending stack.
func TestBacktraceMemoryUsageCountsPending(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`
var kept
function deep(n) { return n ? deep(n - 1) : new Error("deep") }
function make() { kept = deep(1000) }
function drop() { kept = undefined }
`))
	// The first call allocates what stays for later calls.
	XFreeValue(v.tls, v.ctx, v.call("make"))
	XFreeValue(v.tls, v.ctx, v.call("drop"))
	base := v.usage()
	XFreeValue(v.tls, v.ctx, v.call("make"))
	held := v.usage()
	XFreeValue(v.tls, v.ctx, v.call("drop"))
	freed := v.usage()
	frames := int64(1000 * unsafe.Sizeof(backtraceFrame{}))
	t.Logf("memory_used_size: base %d, held %d, freed %d", base.Fmemory_used_size, held.Fmemory_used_size, freed.Fmemory_used_size)
	if d := held.Fmemory_used_size - base.Fmemory_used_size; d < frames {
		t.Fatalf("a pending stack of 1001 frames adds %d bytes of memory_used_size, want at least %d", d, frames)
	}
	if freed.Fmemory_used_size != base.Fmemory_used_size {
		t.Fatalf("memory_used_size %d after freeing, want %d", freed.Fmemory_used_size, base.Fmemory_used_size)
	}
}

// Throwing an error that has no stack gives it one, as JS_DefineProperty
// would: an error that cannot take new properties stays without one.
func TestBacktraceNonExtensibleError(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`
function rethrow(e) { try { throw e } catch (x) { return x } }
function check() {
	var out = []
	for (var lock of ["freeze", "seal", "preventExtensions"]) {
		var e = new Error("locked")
		delete e.stack
		Object[lock](e)
		rethrow(e)
		out.push(lock + ": " + Object.getOwnPropertyNames(e) + " " + Object.isFrozen(e) + " " + Object.isSealed(e))
	}
	var e = new Error("open")
	delete e.stack
	rethrow(e)
	out.push("open: " + Object.getOwnPropertyNames(e) + " " + (typeof e.stack))
	return out.join("\n")
}
`))
	want := "freeze: message true true\nseal: message false true\npreventExtensions: message false false\nopen: message,stack string"
	if got := v.text(v.call("check")); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// After an error gets extra own properties, several errors share the new
// shape. Assigning or defining 'stack' with little memory must still work,
// as it does when the stack is a plain string, and must not need to
// allocate a shape. This is round-2 problem 1.
func TestBacktraceMutationOverwrite(t *testing.T) {
	for _, op := range []string{"set", "define"} {
		for _, room := range []int64{0, 64, 256} {
			t.Run(fmt.Sprintf("%s/%d", op, room), func(t *testing.T) {
				v := newBtVM(t)
				XFreeValue(v.tls, v.ctx, v.eval(`
var held = []
function deep(n) { return n ? deep(n - 1) : new Error("deep") }
function make() { for (var i = 0; i < 2; i++) { var e = deep(40); held.push(e); for (var j = 0; j < 30; j++) e["x" + j] = j } return held[0] }
`))
				e := v.call("make")
				defer XFreeValue(v.tls, v.ctx, e)
				v.limit(room)
				var ret int32
				switch op {
				case "set":
					ret = XSetProperty(v.tls, v.ctx, e, EJS_ATOM_stack, XNewInt32(v.tls, v.ctx, 42))
				case "define":
					ret = XJS_DefinePropertyValue(v.tls, v.ctx, e, EJS_ATOM_stack, XNewInt32(v.tls, v.ctx, 42), MJS_PROP_CONFIGURABLE|MJS_PROP_WRITABLE)
				}
				v.limit(-1)
				if ex := v.exception(); ret != 1 || ex != "" {
					t.Fatalf("%s returned %d, exception %q", op, ret, ex)
				}
				if got := v.text(XGetProperty(v.tls, v.ctx, e, EJS_ATOM_stack)); got != "42" {
					t.Fatalf("stack is %q after %s", got, op)
				}
			})
		}
	}
}

// Replacing an error's prototype must not keep the old prototype, and the
// object graph behind it, alive while the stack is pending. This is round-2
// problem 2.
func TestBacktraceOldPrototypeNotRetained(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`
var kept
function make() {
	function Target() {}
	Target.prototype = { payload: new ArrayBuffer(4 * 1024 * 1024) }
	var e = Reflect.construct(Error, ["retained prototype"], Target)
	Object.setPrototypeOf(e, null)
	kept = e
}
`))
	XFreeValue(v.tls, v.ctx, v.call("make"))
	pending := v.usage()
	// The 4 MiB ArrayBuffer of the old prototype must already be gone.
	if pending.Fmemory_used_size > 2<<20 {
		t.Fatalf("a pending error keeps %d bytes, the old prototype is retained", pending.Fmemory_used_size)
	}
}

// Whatever path reads a pending stack, it reads a string, never the
// internal pending value. Covers [[Get]], descriptors, Reflect, a Proxy,
// and, once 'stack' is made enumerable, the enumerating operations. This
// guards round-2's rule that the pending marker must never reach
// JavaScript.
func TestBacktraceMarkerNeverLeaks(t *testing.T) {
	v := newBtVM(t)
	got := v.text(v.call(`
(function () {
	function deep(n) { return n ? deep(n - 1) : new Error("deep") }
	var out = []
	function check(label, x) { out.push(label + ":" + typeof x) }
	check("get", deep(5).stack)
	check("index", deep(5)["stack"])
	check("descriptorValue", Object.getOwnPropertyDescriptor(deep(5), "stack").value)
	check("descriptorsValue", Object.getOwnPropertyDescriptors(deep(5)).stack.value)
	check("reflectGet", Reflect.get(deep(5), "stack"))
	check("reflectOwn", Reflect.getOwnPropertyDescriptor(deep(5), "stack").value)
	check("proxyGet", new Proxy(deep(5), {}).stack)
	check("proxyOwn", Object.getOwnPropertyDescriptor(new Proxy(deep(5), {}), "stack").value)
	// Make 'stack' show up in enumeration, then read it that way.
	function shown() { var e = deep(5); Object.defineProperty(e, "stack", { enumerable: true }); return e }
	check("values", Object.values(shown()).find(x => typeof x === "string" && x.indexOf("at ") >= 0))
	check("entries", Object.entries(shown()).find(p => p[0] === "stack")[1])
	check("assign", Object.assign({}, shown()).stack)
	check("spread", ({ ...shown() }).stack)
	check("json", typeof JSON.parse(JSON.stringify(shown())).stack)
	return out.join(" ")
})
`))
	for _, field := range strings.Fields(got) {
		if !strings.HasSuffix(field, ":string") {
			t.Fatalf("a read gave a non-string: %q (all: %s)", field, got)
		}
	}
}
