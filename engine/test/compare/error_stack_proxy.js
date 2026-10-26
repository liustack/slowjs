// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -call
// Runs as the body of a function the host calls, so that no top-level code is
// on the stack of the errors and their stacks are not formatted when they are
// made.
//
// On proxies of errors whose stacks are pending, the operations that look
// only at attributes and the trap checks give what they give for a stack that
// was formatted at once, and the stacks read the same afterwards.
var held = []
function deep(n) { return n ? deep(n - 1) : new Error("deep") }
var ops = [
	(e, P) => Object.keys(P(e, {})).length === 0,
	(e, P) => Object.freeze(P(e, {})) !== undefined,
	(e, P) => Object.prototype.__lookupGetter__.call(P(e, {}), "stack") === undefined,
	(e, P) => Object.prototype.propertyIsEnumerable.call(P(e, {}), "stack") === false,
	(e, P) => Object.keys(Object.assign({}, P(e, {}))).length === 0,
	(e, P) => Object.entries(P(e, {})).length === 0,
	(e, P) => JSON.stringify(P(e, {})) === "{}",
	(e, P) => Reflect.get(P(e, { get() { return 17 } }), "stack") === 17,
	(e, P) => Reflect.set(P(e, { set() { return true } }), "stack", 42),
	(e, P) => Reflect.defineProperty(P(e, { defineProperty() { return true } }), "stack", { writable: true }),
	(e, P) => typeof Object.getOwnPropertyDescriptor(P(e, {}), "stack").value === "string",
]
var wraps = [(t, h) => new Proxy(t, h), (t, h) => new Proxy(new Proxy(t, {}), h)]
// runOps runs each operation on its own error, from held[first] on.
function runOps(first) {
	var out = [], i = first
	for (var w of wraps) for (var op of ops) {
		try { out.push(op(held[i++], w)) } catch (x) { out.push("threw") }
	}
	return out.join()
}
for (var i = 0; i < 45; i++) held.push(deep(40))

var want = "true,".repeat(22).slice(0, -1)
var got = runOps(0)
var lines = held[44].stack.split("\n").length
globalThis.result = "operations: " + (got === want ? "all 22 true" : "FAIL " + got) + "\n" +
	"stacks: " + (held.every(e => e.stack === held[44].stack) && lines === 43 ? "45 equal, 43 lines" : "FAIL " + lines)
