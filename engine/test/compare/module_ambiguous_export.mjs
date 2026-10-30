// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// export * from two modules that give the same name is ambiguous only when
// the two names are not the same binding. A binding that a module imported and
// exports again is the binding it imported (also as a namespace object), so
// it is not ambiguous with the module it came from (upstream QuickJS 1aa7429).
// An ambiguous name is left out of the namespace and is an error for an import
// that names it. The file is what Node 24 prints, except the line that starts
// with "spec:". The namespace of amb_a.js that two paths export under one name
// is one binding in the specification and ambiguous in V8.
import * as same from "./modules/amb_same_binding.js"
import * as same2 from "./modules/amb_same_binding2.js"
import * as sameNs from "./modules/amb_same_ns.js"
import * as two from "./modules/amb_two_foo.js"
import * as two2 from "./modules/amb_two_foo2.js"
import * as own from "./modules/amb_own_wins.js"
import * as twoNs from "./modules/amb_two_ns.js"
import * as a from "./modules/amb_a.js"
const out = []
const keys = ns => Object.keys(ns).join()
out.push("same binding: " + keys(same) + " " + same.foo)
out.push("same binding, two names: " + keys(same2) + " " + same2.foo + same2.other)
out.push("spec: same namespace: " + keys(sameNs) + " " + (sameNs.ns === a))
out.push("two foo: " + keys(two))
out.push("two foo, one imported: " + keys(two2))
out.push("own export wins: " + keys(own) + " " + own.ns)
out.push("two namespaces: " + keys(twoNs))
a.inc()
out.push("live binding: " + same.foo + same2.foo + same2.other)
const tries = []
for (const name of ["amb_import_bad", "amb_two_foo", "amb_same_binding"]) {
  tries.push(import("./modules/" + name + ".js").then(m => name + " loads " + keys(m), e => name + " " + e.name))
}
Promise.all(tries).then(r => { globalThis.result = out.join("\n") + "\n" + r.join("\n") })
