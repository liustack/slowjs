// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -call
// The stack of an error made in a function the host called is pending until it
// is read (the script runs as such a function), and the engine formats it
// when something reads it. Whatever reads it, the script sees the text and
// never the internal value that stands for a pending stack.
var out = []
function deep(n) { return n ? deep(n - 1) : new Error("deep") }
function mk() { return new Error("x") }
function mkSub(E) { return new E("x") }

// Every syntax that reads a property hands the script the text of a pending
// stack. The interpreter has several inline copies of the property read
// (get_field, get_field2 for compound assignments and method calls,
// get_length), and each must format the stack first. Each case returns true
// when it sees what it would see if the stack had been formatted when the
// error was made.
var checks = {
	get: function (e) { let v = e.stack; return typeof v === "string" && v.startsWith("    at ") },
	index: function (e) { let k = "stack"; let v = e[k]; return typeof v === "string" },
	typeof: function (e) { return typeof e.stack === "string" },
	length: function (e) { return e.stack.length > 0 },
	optional: function (e) { return typeof e?.stack === "string" && e.stack?.length > 0 },
	orAssign: function (e) { let v = (e.stack ||= 42); return typeof v === "string" && v === e.stack },
	nullishAssign: function (e) { let v = (e.stack ??= 42); return typeof v === "string" && v === e.stack },
	andAssign: function (e) { let s = String(e.stack); let v = (e.stack &&= s + "!"); return v === s + "!" },
	orAssignInherited: function (e) { let o = Object.create(e); let v = (o.stack ||= 42); return typeof v === "string" && v === e.stack },
	orAssignIndex: function (e) { let k = "stack"; let v = (e[k] ||= 42); return typeof v === "string" && v === e.stack },
	plusAssign: function (e) { let s = String(e.stack); let v = (e.stack += "!"); return v === s + "!" },
	minusAssign: function (e) { let v = (e.stack -= 1); return Number.isNaN(v) },
	orBitsAssign: function (e) { let v = (e.stack |= 0); return v === 0 },
	postIncrement: function (e) { let v = e.stack++; return Number.isNaN(v) && Number.isNaN(e.stack) },
	preDecrement: function (e) { let v = --e.stack; return Number.isNaN(v) },
	subclass: function (e) { class E extends Error {} ; let c = mkSub(E); let v = (c.stack ||= 1); return typeof v === "string" },
	destructure: function (e) { let { stack } = e; return typeof stack === "string" },
	with: function (e) { let s; with (e) { s = stack } return typeof s === "string" },
	super: function (e) { let o = { __proto__: e, get() { return super.stack } }; return typeof o.get() === "string" },
	reflect: function (e) { return typeof Reflect.get(e, "stack") === "string" },
	template: function (e) { return `${e.stack}`.startsWith("    at ") },
	stringCall: function (e) { return String(e.stack).startsWith("    at ") },
	methodOnStack: function (e) { return e.stack.toString().startsWith("    at ") },
	descriptor: function (e) { return typeof Object.getOwnPropertyDescriptor(e, "stack").value === "string" },
	descriptors: function (e) { return typeof Object.getOwnPropertyDescriptors(e).stack.value === "string" },
	proxy: function (e) { return typeof new Proxy(e, {}).stack === "string" },
	proxyOrAssign: function (e) { let p = new Proxy(e, {}); let v = (p.stack ||= 1); return typeof v === "string" },
}
for (var name in checks) {
	var got
	try { got = String(checks[name](mk())) } catch (x) { got = "threw " + x }
	out.push("read " + name + ": " + got)
}

// What a compound assignment reads is the value it leaves on the stack, and a
// function can hand that to the host. It must be the text.
function leak() { const e = new Error("x"); return (e.stack ||= 42) }
out.push("compound assignment gives " + typeof leak())

// Whatever path reads a pending stack, it reads a string. Covers [[Get]],
// descriptors, Reflect, a Proxy, and, once 'stack' is made enumerable, the
// enumerating operations.
function check(label, x) { out.push("never the marker, " + label + ": " + typeof x) }
check("get", deep(5).stack)
check("index", deep(5)["stack"])
check("descriptorValue", Object.getOwnPropertyDescriptor(deep(5), "stack").value)
check("descriptorsValue", Object.getOwnPropertyDescriptors(deep(5)).stack.value)
check("reflectGet", Reflect.get(deep(5), "stack"))
check("reflectOwn", Reflect.getOwnPropertyDescriptor(deep(5), "stack").value)
check("proxyGet", new Proxy(deep(5), {}).stack)
check("proxyOwn", Object.getOwnPropertyDescriptor(new Proxy(deep(5), {}), "stack").value)
function shown() { var e = deep(5); Object.defineProperty(e, "stack", { enumerable: true }); return e }
check("values", Object.values(shown()).find(x => typeof x === "string" && x.indexOf("at ") >= 0))
check("entries", Object.entries(shown()).find(p => p[0] === "stack")[1])
check("assign", Object.assign({}, shown()).stack)
check("spread", ({ ...shown() }).stack)
check("json", typeof JSON.parse(JSON.stringify(shown())).stack)

// Redefining a non-writable, non-configurable 'stack' with the same text is
// allowed, as it is when the stack is already a string: SameValue compares the
// text, not the pending marker.
function two() { const es = []; for (let i = 0; i < 2; i++) es.push(new Error("x")); return es }
var es = two()
var text = es[0].stack
Object.defineProperty(es[1], "stack", { writable: false, configurable: false })
var reflected = Reflect.defineProperty(es[1], "stack", { value: text })
var defined = true
try { Object.defineProperty(es[1], "stack", { value: text }) } catch (x) { defined = String(x) }
var different = Reflect.defineProperty(es[1], "stack", { value: text + "!" })
out.push("redefine with the same text: " + [reflected, defined, different, es[1].stack === text].join())

// Throwing an error that has no stack gives it one, as JS_DefineProperty
// would: an error that cannot take new properties stays without one.
function rethrow(e) { try { throw e } catch (x) { return x } }
var locked = []
for (var lock of ["freeze", "seal", "preventExtensions"]) {
	var e = new Error("locked")
	delete e.stack
	Object[lock](e)
	rethrow(e)
	locked.push(lock + ": " + Object.getOwnPropertyNames(e) + " " + Object.isFrozen(e) + " " + Object.isSealed(e))
}
var open = new Error("open")
delete open.stack
rethrow(open)
locked.push("open: " + Object.getOwnPropertyNames(open) + " " + (typeof open.stack))
out.push("errors that cannot take a stack:\n" + locked.join("\n"))

// Lookups and trap checks through a proxy format the target's stack and
// compare the text, as the engine did when the stack was formatted at once.
// Traps are called as often and in the same order as before.
function proxyChecks() {
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
out.push("proxy checks: " + proxyChecks())

globalThis.result = out.join("\n")
