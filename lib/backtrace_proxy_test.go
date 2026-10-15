// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"fmt"
	"testing"

	"modernc.org/libc"
)

// Operations on a proxy of an error that only look at keys or attributes,
// or whose trap checks only need the attributes of the target's 'stack',
// leave the stack pending, so they cannot run out of memory formatting it.
// The proxy may have no trap for the lookup, or wrap another such proxy.
//
// Making the first pending error registers a class and allocates, which
// changes which small allocations still find a free block. So when an
// operation runs out of memory, the same operation on a property other than
// 'stack' (control) must run out at that room too: the failure is then the
// cost of the proxies, not of the stack.
func TestBacktraceProxyChecksDoNotFormat(t *testing.T) {
	ops := []struct{ expr, check, control string }{
		{`Object.keys(P(held, {}))`, `Array.isArray(res) && res.length === 0`, ""},
		{`Object.freeze(P(held, {}))`, `typeof res === "object" && Object.isFrozen(held)`, ""},
		{`Object.prototype.__lookupGetter__.call(P(held, {}), "stack")`, `res === undefined`, ""},
		{`Object.prototype.propertyIsEnumerable.call(P(held, {}), "stack")`, `res === false`, ""},
		{`Object.assign({}, P(held, {}))`, `Object.keys(res).length === 0`, ""},
		{`Object.entries(P(held, {}))`, `Array.isArray(res) && res.length === 0`, ""},
		{`JSON.stringify(P(held, {}))`, `res === "{}"`, ""},
		{`Reflect.get(P(held, { get() { return 17 } }), "stack")`, `res === 17`, ""},
		{`Reflect.set(P(held, { set() { return true } }), "stack", 42)`, `res === true`, ""},
		{`Reflect.defineProperty(P(held, { defineProperty() { return true } }), "stack", { writable: true })`, `res === true`,
			`Reflect.defineProperty(P(held, { defineProperty() { return true } }), "x", { writable: true })`},
	}
	wraps := map[string]string{
		"proxy":  `function P(t, h) { return new Proxy(t, h) }`,
		"nested": `function P(t, h) { return new Proxy(new Proxy(t, {}), h) }`,
	}
	for _, op := range ops {
		for wrap, def := range wraps {
			for _, room := range []int64{-1, 0, 256, 512, 1024, 2048} {
				name := fmt.Sprintf("%s %s, room %d", wrap, op.expr, room)
				r := proxyOp(t, def, op.expr, op.check, room)
				if r.failed != "" && op.control != "" && wrap == "nested" && proxyOp(t, def, op.control, "true", room).failed != "" {
					r.failed = ""
				}
				if r.failed != "" {
					t.Errorf("%s: %s", name, r.failed)
				}
				if r.formatted {
					t.Errorf("%s: formatted the stack", name)
				}
				if r.stack != r.want {
					t.Errorf("%s: read after gave %q, want %q", name, r.stack, r.want)
				}
			}
		}
	}
}

type proxyResult struct {
	failed      string // why the operation failed, or ""
	formatted   bool   // whether it formatted the stack
	stack, want string // the stack read after it, and the stack of another error
}

// proxyOp runs expr, which may use P, defined by def, and held, an error
// whose stack is pending, with room bytes of memory left (any if room < 0),
// then checks the result res with check.
func proxyOp(t *testing.T, def, expr, check string, room int64) (r proxyResult) {
	v := newBtVM(t)
	ref := v.pendingErr()
	r.want = v.stack(ref)
	XFreeValue(v.tls, v.ctx, ref)

	e := v.pendingErr()
	defer XFreeValue(v.tls, v.ctx, e)
	XFreeValue(v.tls, v.ctx, v.eval(def+`; var held, res; function op() { res = `+expr+` }`))
	v.setGlobal("held", XDupValue(v.tls, v.ctx, e))
	f := v.eval("op")
	g := XJS_GetGlobalObject(v.tls, v.ctx)
	v.limit(room)
	res := XJS_Call(v.tls, v.ctx, f, g, 0, 0)
	v.limit(-1)
	XFreeValue(v.tls, v.ctx, g)
	XFreeValue(v.tls, v.ctx, f)
	if tag(res) == EJS_TAG_EXCEPTION {
		r.failed = v.exception()
	} else if ok := v.text(v.eval(`String(` + check + `)`)); ok != "true" {
		r.failed = "result fails " + check
	}
	XFreeValue(v.tls, v.ctx, res)
	r.formatted = !v.stackPending(e)
	r.stack = v.stack(e)
	return r
}

// A proxy trap check that does need the value of the target's 'stack' (a
// frozen data property) formats it and compares the text, as the engine
// did when the stack was formatted at once. Traps are called as often and
// in the same order as before. The expected result is what main gives.
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
