// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// A property the host puts on the proxy itself, with
// JS_SetPropertyFunctionList, is an own property of the proxy: lookups of
// it, full or of the attributes only, find it before the handler and never
// call a trap, as on main. Revoking the proxy changes nothing.
func TestBacktraceProxyOwnProperty(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		v := newBtVM(t)
		XFreeValue(v.tls, v.ctx, v.eval(`var log = []
var r = Proxy.revocable({}, { getOwnPropertyDescriptor(t, k) { log.push(k); return undefined } })
var p = r.proxy`))
		p := v.eval("p")
		e := (*TJSCFunctionListEntry)(unsafe.Pointer(v.tls.Alloc(int(unsafe.Sizeof(TJSCFunctionListEntry{})))))
		name, _ := libc.CString("stack")
		str, _ := libc.CString("host value")
		*e = TJSCFunctionListEntry{Fname: name, Fprop_flags: MJS_PROP_C_W_E, Fdef_type: MJS_DEF_PROP_STRING}
		*(*uintptr)(unsafe.Pointer(&e.Fu)) = str
		if XJS_SetPropertyFunctionList(v.tls, v.ctx, p, uintptr(unsafe.Pointer(e)), 1) < 0 {
			t.Fatal(v.exception())
		}
		XFreeValue(v.tls, v.ctx, p)
		if revoke {
			XFreeValue(v.tls, v.ctx, v.eval("r.revoke()"))
		}
		got := v.text(v.eval(`JSON.stringify([
	Object.getOwnPropertyDescriptor(p, "stack").value,
	(() => { try { return Object.prototype.propertyIsEnumerable.call(p, "stack") } catch (e) { return String(e) } })(),
	log])`))
		v.tls.Free(int(unsafe.Sizeof(TJSCFunctionListEntry{})))
		libc.Xfree(v.tls, str)
		libc.Xfree(v.tls, name)
		if want := `["host value",true,[]]`; got != want {
			t.Errorf("revoked %v: got %s, want %s", revoke, got, want)
		}
	}
}

// Lookups and trap checks through a proxy format the target's stack and
// compare the text, as the engine did when the stack was formatted at once.
// Traps are called as often and in the same order as before. The expected
// result is what main gives.
func TestBacktraceProxyChecksCompareText(t *testing.T) {
	v := newBtVM(t)
	XFreeValue(v.tls, v.ctx, v.eval(proxyTextJS))
	got := v.text(v.call("run"))
	const want = "true,true,true,true,true,true,true,true,0,{},false,string " +
		"getOwnPropertyDescriptor,ownKeys,getOwnPropertyDescriptor,getOwnPropertyDescriptor " +
		"message,stack,message,stack,stack,stack true"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

const proxyTextJS = `function deep(n) { return n ? deep(n - 1) : new Error("deep") }
function run() {
	const held = deep(40);
	const log = [];
	Object.freeze(held);
	const text = Object.getOwnPropertyDescriptor(new Proxy(held, {}), "stack").value;
	const out = [];
	const inner = new Proxy(held, {});
	out.push(Reflect.get(new Proxy(inner, { get() { return text } }), "stack") === text);
	try { Reflect.get(new Proxy(inner, { get() { return 17 } }), "stack"); out.push(false) } catch (x) { out.push(x instanceof TypeError) }
	out.push(Reflect.set(new Proxy(inner, { set() { return true } }), "stack", text));
	try { Reflect.set(new Proxy(inner, { set() { return true } }), "stack", 42); out.push(false) } catch (x) { out.push(x instanceof TypeError) }
	out.push(Reflect.defineProperty(new Proxy(inner, { defineProperty() { return true } }), "stack", { value: text }));
	try { Object.defineProperty(new Proxy(inner, { defineProperty() { return true } }), "stack", { value: 42 }); out.push(false) } catch (x) { out.push(x instanceof TypeError) }
	const handler = new Proxy({}, { get(t, k) { log.push(k); return undefined } });
	out.push(Object.getOwnPropertyDescriptor(new Proxy(held, handler), "stack").value === text);
	out.push(Object.keys(new Proxy(held, handler)).length === 0);
	const e2 = deep(3);
	const tlog = [];
	const traps = new Proxy(e2, { getOwnPropertyDescriptor(t, k) { tlog.push(k); return Reflect.getOwnPropertyDescriptor(t, k) } });
	out.push(Object.keys(traps).length, JSON.stringify(traps), Object.prototype.propertyIsEnumerable.call(traps, "stack"));
	out.push(typeof Object.getOwnPropertyDescriptor(traps, "stack").value);
	return out.join() + " " + log.join() + " " + tlog.join() + " " + (text.indexOf("at deep") >= 0);
}
`

func (v *btVM) setGlobal(name string, val TJSValue) {
	g := XJS_GetGlobalObject(v.tls, v.ctx)
	defer XFreeValue(v.tls, v.ctx, g)
	p, _ := libc.CString(name)
	defer libc.Xfree(v.tls, p)
	XJS_SetPropertyStr(v.tls, v.ctx, g, p, val)
}
