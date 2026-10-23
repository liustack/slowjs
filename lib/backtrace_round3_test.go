// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// stackPending reports whether the own 'stack' of obj still holds the
// pending marker, that is, whether nothing has formatted it yet.
func (v *btVM) stackPending(obj TJSValue) bool {
	ppr := v.tls.Alloc(int(unsafe.Sizeof(uintptr(0))))
	defer v.tls.Free(int(unsafe.Sizeof(uintptr(0))))
	if _find_own_property(v.tls, ppr, jsvPtr(obj), EJS_ATOM_stack) == 0 {
		return false
	}
	return backtraceMarker(*(*TJSValue)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(ppr)))))
}

// Every syntax that reads a property hands JavaScript the text of a
// pending stack, never the marker. The interpreter has several inline
// copies of the property read (get_field, get_field2 for compound
// assignments and method calls, get_length); each must format the stack
// first. Each case returns true when it sees what it would see if the stack
// had been formatted when the error was made.
func TestBacktraceEveryReadSeesText(t *testing.T) {
	cases := []struct{ name, body string }{
		{"get", `let v = e.stack; return typeof v === "string" && v.startsWith("    at ")`},
		{"index", `let k = "stack"; let v = e[k]; return typeof v === "string"`},
		{"typeof", `return typeof e.stack === "string"`},
		{"length", `return e.stack.length > 0`},
		{"optional", `return typeof e?.stack === "string" && e.stack?.length > 0`},
		{"orAssign", `let v = (e.stack ||= 42); return typeof v === "string" && v === e.stack`},
		{"nullishAssign", `let v = (e.stack ??= 42); return typeof v === "string" && v === e.stack`},
		{"andAssign", `let s = String(e.stack); let v = (e.stack &&= s + "!"); return v === s + "!"`},
		{"orAssignInherited", `let o = Object.create(e); let v = (o.stack ||= 42); return typeof v === "string" && v === e.stack`},
		{"orAssignIndex", `let k = "stack"; let v = (e[k] ||= 42); return typeof v === "string" && v === e.stack`},
		{"plusAssign", `let s = String(e.stack); let v = (e.stack += "!"); return v === s + "!"`},
		{"minusAssign", `let v = (e.stack -= 1); return Number.isNaN(v)`},
		{"orBitsAssign", `let v = (e.stack |= 0); return v === 0`},
		{"postIncrement", `let v = e.stack++; return Number.isNaN(v) && Number.isNaN(e.stack)`},
		{"preDecrement", `let v = --e.stack; return Number.isNaN(v)`},
		{"subclass", `class E extends Error {} ; let c = mkSub(E); let v = (c.stack ||= 1); return typeof v === "string"`},
		{"destructure", `let { stack } = e; return typeof stack === "string"`},
		{"with", `let s; with (e) { s = stack } return typeof s === "string"`},
		{"super", `let o = { __proto__: e, get() { return super.stack } }; return typeof o.get() === "string"`},
		{"reflect", `return typeof Reflect.get(e, "stack") === "string"`},
		{"template", "return `${e.stack}`.startsWith(\"    at \")"},
		{"stringCall", `return String(e.stack).startsWith("    at ")`},
		{"methodOnStack", `return e.stack.toString().startsWith("    at ")`},
		{"descriptor", `return typeof Object.getOwnPropertyDescriptor(e, "stack").value === "string"`},
		{"descriptors", `return typeof Object.getOwnPropertyDescriptors(e).stack.value === "string"`},
		{"proxy", `return typeof new Proxy(e, {}).stack === "string"`},
		{"proxyOrAssign", `let p = new Proxy(e, {}); let v = (p.stack ||= 1); return typeof v === "string"`},
	}
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`
function mk() { return new Error("x") }
function mkSub(E) { return new E("x") }
`))
	var src strings.Builder
	src.WriteString("var checks = {\n")
	for _, c := range cases {
		fmt.Fprintf(&src, "%q: function (e) { %s },\n", c.name, c.body)
	}
	src.WriteString("}\nfunction run(name) { return checks[name](mk()) }\n")
	XFreeValue(v.tls, v.ctx, v.eval(src.String()))
	for _, c := range cases {
		got := v.text(v.call(fmt.Sprintf(`(function () { try { return String(run(%q)) } catch (x) { return "threw " + x } })`, c.name)))
		if got != "true" {
			t.Errorf("%s: got %s", c.name, got)
		}
	}
}

// What a compound assignment reads is the value it leaves on the stack, and
// a function can hand that to Go. It must be the text.
func TestBacktraceCompoundAssignReturnsText(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(`function leak() { const e = new Error("x"); return (e.stack ||= 42) }`))
	r := v.call("leak")
	defer XFreeValue(v.tls, v.ctx, r)
	if tag(r) != EJS_TAG_STRING {
		t.Fatalf("e.stack ||= 42 gave tag %d, want a string", tag(r))
	}
}

// Redefining a non-writable, non-configurable 'stack' with the same text is
// allowed, as it is when the stack is already a string: SameValue compares
// the text, not the pending marker.
func TestBacktraceRedefineSameText(t *testing.T) {
	v := newBtVM(t)
	got := v.text(v.call(`
(function () {
	function two() { const es = []; for (let i = 0; i < 2; i++) es.push(new Error("x")); return es }
	const es = two()
	const text = es[0].stack
	Object.defineProperty(es[1], "stack", { writable: false, configurable: false })
	const reflected = Reflect.defineProperty(es[1], "stack", { value: text })
	let defined = true
	try { Object.defineProperty(es[1], "stack", { value: text }) } catch (x) { defined = String(x) }
	const different = Reflect.defineProperty(es[1], "stack", { value: text + "!" })
	return [reflected, defined, different, es[1].stack === text].join()
})
`))
	if got != "true,true,false,true" {
		t.Fatalf("got %s, want true,true,false,true", got)
	}
}

// Operations that look only at whether 'stack' exists, its attributes or
// whether it is a getter do not need its text, so they leave the stack
// pending and cannot fail for lack of memory to format it.
func TestBacktraceAttributeChecksDoNotFormat(t *testing.T) {
	ops := []string{
		`Object.keys(e)`,
		`e.__lookupGetter__("stack")`,
		`e.__lookupSetter__("stack")`,
		`e.propertyIsEnumerable("stack")`,
		`Object.isFrozen(e)`,
		`Object.isSealed(e)`,
		`Object.seal(e)`,
		`Object.freeze(e)`,
		`({ ...e })`,
		`Object.assign({}, e)`,
		`Object.entries(e)`,
		`new Proxy(e, {}).hasOwnProperty("stack")`,
		`"stack" in new Proxy(e, { has() { return false } })`,
		`Reflect.ownKeys(new Proxy(e, { ownKeys(t) { return Reflect.ownKeys(t) } }))`,
		`Reflect.deleteProperty(new Proxy(e, { deleteProperty() { return true } }), "stack")`,
	}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			v := newBtVM(t)
			ref := v.pendingErr()
			want := v.stack(ref)
			XFreeValue(v.tls, v.ctx, ref)

			e := v.pendingErr()
			defer XFreeValue(v.tls, v.ctx, e)
			XFreeValue(v.tls, v.ctx, v.eval(`var held; function op() { const e = held; return `+op+` }`))
			g := XJS_GetGlobalObject(v.tls, v.ctx)
			name, _ := libc.CString("held")
			XJS_SetPropertyStr(v.tls, v.ctx, g, name, XDupValue(v.tls, v.ctx, e))
			libc.Xfree(v.tls, name)
			XFreeValue(v.tls, v.ctx, g)

			XFreeValue(v.tls, v.ctx, v.call("op"))
			if !v.stackPending(e) {
				t.Fatalf("%s formatted the stack", op)
			}
			if got := v.stack(e); got != want {
				t.Fatalf("read after %s gave %q, want %q", op, got, want)
			}
		})
	}
}

// Printing an error reads its stack through get_prop_string, which formats a
// pending stack and then converts it to UTF-8. When either step runs out of
// memory the print goes on without the stack and leaves no exception.
func TestBacktracePrintUnicodeOOMLeavesNoException(t *testing.T) {
	src := `
function named() { return new Error("printed") }
Object.defineProperty(named, "name", { value: "a".repeat(1000) + "中" })
`
	ref := func(v *btVM) string {
		XFreeValue(v.tls, v.ctx, v.eval(src))
		e := v.call("named")
		defer XFreeValue(v.tls, v.ctx, e)
		return v.stack(e)
	}(newBtVM(t))
	for room := int64(0); room <= 16384; room += 128 {
		t.Run(fmt.Sprint(room), func(t *testing.T) {
			v := newBtVM(t)
			XFreeValue(v.tls, v.ctx, v.eval(src))
			e := v.call("named")
			defer XFreeValue(v.tls, v.ctx, e)
			v.limit(room)
			p := _get_prop_string(v.tls, v.ctx, e, EJS_ATOM_stack)
			had := XJS_HasException(v.tls, v.ctx)
			v.limit(-1)
			if p != 0 {
				defer XJS_FreeCString(v.tls, v.ctx, p)
			}
			if had != 0 {
				t.Fatalf("get_prop_string left %q, returned %#x", v.exception(), p)
			}
			if p != 0 && libc.GoString(p) != ref {
				t.Fatalf("get_prop_string gave %.40q, want %.40q", libc.GoString(p), ref)
			}
			if got := v.stack(e); got != ref {
				t.Fatalf("read after the print gave %.40q, want %.40q", got, ref)
			}
		})
	}
}

// The class definition handed to JS_NewClass has no call or exotic
// handlers, whatever the memory it is built in held before, so the marker
// is never taken for a function.
func TestBacktraceClassHasNoHandlers(t *testing.T) {
	v := newBtVM(t)
	const n = 1 << 16
	junk := v.tls.Alloc(n)
	for i := range unsafe.Slice((*byte)(unsafe.Pointer(junk)), n) {
		*(*byte)(unsafe.Pointer(junk + uintptr(i))) = 0xa5
	}
	v.tls.Free(n)
	id := backtraceEnsureClass(v.tls, v.ctx)
	if id == 0 {
		t.Fatal("class not registered")
	}
	cl := (*TJSClass)(unsafe.Pointer((*TJSRuntime)(unsafe.Pointer(v.rt)).Fclass_array + uintptr(id)*unsafe.Sizeof(TJSClass{})))
	if cl.Fcall != 0 || cl.Fexotic != 0 {
		t.Fatalf("class call %#x, exotic %#x, want both 0", cl.Fcall, cl.Fexotic)
	}
}

// The marker class id is set before any VM exists, so reading it never
// races with the first VM that makes a pending stack. The check runs in a
// fresh process, where no test has made a stack yet.
func TestBacktraceClassIDSetAtStart(t *testing.T) {
	if os.Getenv("BACKTRACE_CLASS_ID_CHILD") == "1" {
		if backtraceClassID == 0 {
			fmt.Println("class id not set")
			os.Exit(1)
		}
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBacktraceClassIDSetAtStart$")
	cmd.Env = append(os.Environ(), "BACKTRACE_CLASS_ID_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// The value of a property is only a JSValue when the property is a plain
// data property: an accessor holds two object pointers there and a global
// variable a pointer to its variable reference, with whatever bytes were
// there before after it. The pending-stack checks must look at the kind of
// the property first, and never read such bytes as a marker.
func TestBacktracePendingNeedsDataProperty(t *testing.T) {
	v := newBtVM(t)
	e := v.pendingErr()
	defer XFreeValue(v.tls, v.ctx, e)
	XFreeValue(v.tls, v.ctx, v.eval(`var stack = 1`))

	ppr := v.tls.Alloc(int(unsafe.Sizeof(uintptr(0))))
	defer v.tls.Free(int(unsafe.Sizeof(uintptr(0))))
	g := XJS_GetGlobalObject(v.tls, v.ctx)
	defer XFreeValue(v.tls, v.ctx, g)
	prs := _find_own_property(v.tls, ppr, jsvPtr(g), EJS_ATOM_stack)
	if prs == 0 || _js_backtrace_prop_flags(v.tls, prs)&MJS_PROP_TMASK != MJS_PROP_VARREF {
		t.Fatal("the global 'stack' is not a variable reference")
	}

	// A buffer that looks like a marker value, standing in for the bytes of
	// a variable reference property.
	if _find_own_property(v.tls, ppr, jsvPtr(e), EJS_ATOM_stack) == 0 {
		t.Fatal("no stack")
	}
	marker := XDupValue(v.tls, v.ctx, *(*TJSValue)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(ppr)))))
	// TJSValue is a struct on 64-bit platforms and a uint64 on 32-bit ones,
	// so take its size from a value rather than a composite literal.
	fake := v.tls.Alloc(int(unsafe.Sizeof(marker)))
	defer v.tls.Free(int(unsafe.Sizeof(marker)))
	*(*TJSValue)(unsafe.Pointer(fake)) = marker
	// The fake holds the reference taken above; settle would replace it.
	defer func() { XFreeValue(v.tls, v.ctx, *(*TJSValue)(unsafe.Pointer(fake))) }()

	if _js_backtrace_settle(v.tls, v.ctx, jsvPtr(g), prs, fake) != 0 || *(*TJSValue)(unsafe.Pointer(fake)) != marker {
		t.Fatal("settle read a variable reference as a pending stack")
	}
	n := int(unsafe.Sizeof(TJSMemoryUsage{}))
	s := v.tls.Alloc(n)
	defer v.tls.Free(n)
	*(*TJSMemoryUsage)(unsafe.Pointer(s)) = TJSMemoryUsage{}
	_js_backtrace_memory_usage(v.tls, s, 0, prs, fake)
	if (*TJSMemoryUsage)(unsafe.Pointer(s)).Fmemory_used_count != 0 {
		t.Fatal("memory usage counted a variable reference as a pending stack")
	}
	if _js_backtrace_own_flags(v.tls, v.ctx, 0, jsvPtr(g), EJS_ATOM_stack) != 1 {
		t.Fatal("no global stack")
	}
}
