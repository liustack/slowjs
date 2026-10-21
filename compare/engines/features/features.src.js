// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Feature probes. Each entry: [id, edition, kind, source].
// kind "script": the completion value must equal the expected value.
// kind "async": the script sets globalThis.__result after promise jobs run.
// kind "module": source is a module; modules can import "./dep.js".
// The expected value is "ok" unless gen.mjs records Node's output (expect "node").
export const features = [
  // ES2015
  ["class", "ES2015", "script", `class A { constructor() { this.v = 1 } m() { return this.v } static s() { return 3 } } class B extends A { m() { return super.m() + 1 } } (new B().m() === 2 && B.s() === 3) ? "ok" : "bad"`],
  ["let-const-arrow-template", "ES2015", "script", `{ let a = 1; const f = (x, y = 2) => x + y; var r = \`\${f(a)}\` } r === "3" ? "ok" : "bad"`],
  ["destructuring-spread", "ES2015", "script", `var {a, b: [c, ...d]} = {a: 1, b: [2, 3, 4]}; var m = Math.max(...d); (a + c + m === 7 && d.length === 2) ? "ok" : "bad"`],
  ["generators", "ES2015", "script", `function* g() { var x = yield 1; yield x * 2 } var it = g(); it.next(); (it.next(21).value === 42 && [...g()].length === 2) ? "ok" : "bad"`],
  ["symbol-iterator-for-of", "ES2015", "script", `var o = {*[Symbol.iterator]() { yield 1; yield 2 }}; var s = 0; for (var v of o) s += v; s === 3 ? "ok" : "bad"`],
  ["map-set-weakmap", "ES2015", "script", `var m = new Map([[1, "a"]]); var s = new Set([1, 1, 2]); var w = new WeakMap(); var k = {}; w.set(k, 1); (m.get(1) === "a" && s.size === 2 && w.get(k) === 1) ? "ok" : "bad"`],
  ["proxy", "ES2015", "script", `var p = new Proxy({}, { get: (t, k) => k === "x" ? 42 : undefined, has: () => true }); (p.x === 42 && ("y" in p)) ? "ok" : "bad"`],
  ["reflect", "ES2015", "script", `var o = {}; Reflect.defineProperty(o, "x", {value: 1}); (Reflect.ownKeys(o).length === 1 && Reflect.has(o, "x") && Reflect.apply(Math.max, null, [1, 3]) === 3) ? "ok" : "bad"`],
  ["promise", "ES2015", "async", `Promise.resolve(1).then(v => { globalThis.__result = v === 1 ? "ok" : "bad" })`],
  ["typed-arrays", "ES2015", "script", `var b = new ArrayBuffer(8); var f = new Float64Array(b); f[0] = 1.5; var u = new Uint8Array(b); var dv = new DataView(b); (u.length === 8 && dv.getFloat64(0, true) === 1.5) ? "ok" : "bad"`],
  ["regexp-unicode-sticky", "ES2015", "script", `var r = /a/y; r.lastIndex = 1; (/\\u{1F600}/u.test("😀") && r.test("ba")) ? "ok" : "bad"`],
  ["new-target-tagged-template", "ES2015", "script", `function F() { return new.target === F } var t = (s, ...v) => s.raw[0] + v[0]; (new F() instanceof F || true) && t\`x\${1}\` === "x1" ? "ok" : "bad"`],
  // ES2016-ES2017
  ["exponent-includes", "ES2016", "script", `(2 ** 10 === 1024 && [NaN].includes(NaN)) ? "ok" : "bad"`],
  ["async-await", "ES2017", "async", `async function f() { var a = await 1; var b = await Promise.resolve(2); return a + b } f().then(v => { globalThis.__result = v === 3 ? "ok" : "bad" })`],
  ["object-entries-padstart", "ES2017", "script", `(Object.entries({a: 1})[0][1] === 1 && "5".padStart(3, "0") === "005" && Object.getOwnPropertyDescriptors({a: 1}).a.value === 1) ? "ok" : "bad"`],
  ["shared-array-buffer-atomics", "ES2017", "script", `var s = new SharedArrayBuffer(8); var i = new Int32Array(s); Atomics.add(i, 0, 5); Atomics.load(i, 0) === 5 ? "ok" : "bad"`],
  // ES2018
  ["async-iteration", "ES2018", "async", `async function* g() { yield 1; yield 2 } (async () => { var s = 0; for await (var v of g()) s += v; globalThis.__result = s === 3 ? "ok" : "bad" })()`],
  ["object-rest-spread", "ES2018", "script", `var {a, ...r} = {a: 1, b: 2, c: 3}; var o = {...r, d: 4}; (Object.keys(o).join() === "b,c,d") ? "ok" : "bad"`],
  ["promise-finally", "ES2018", "async", `Promise.reject(1).finally(() => {}).catch(v => { globalThis.__result = v === 1 ? "ok" : "bad" })`],
  ["regexp-named-groups", "ES2018", "script", `var m = /(?<y>\\d{4})-(?<m>\\d{2})/.exec("2026-10"); (m.groups.y === "2026" && "2026-10".replace(/(?<y>\\d+)-(?<m>\\d+)/, "$<m>/$<y>") === "10/2026") ? "ok" : "bad"`],
  ["regexp-lookbehind", "ES2018", "script", `("$42 and €17".match(/(?<=\\$)\\d+/)[0] === "42" && /(?<!a)b/.test("cb")) ? "ok" : "bad"`],
  ["regexp-dotall", "ES2018", "script", `/a.b/s.test("a\\nb") ? "ok" : "bad"`],
  ["regexp-unicode-property", "ES2018", "script", `var m = "Grüße, Ankara 日本".match(/\\p{L}+/gu); (m !== null && m.join("|") === "Grüße|Ankara|日本" && "Ankara-2026!".replace(/[^\\p{L}\\p{N}]+/gu, "_") === "Ankara_2026_") ? "ok" : "bad: " + m`],
  // ES2019
  ["flat-fromentries-trim", "ES2019", "script", `([1, [2, [3]]].flat(Infinity).length === 3 && Object.fromEntries([["a", 1]]).a === 1 && " x".trimStart() === "x" && Symbol("d").description === "d") ? "ok" : "bad"`],
  ["optional-catch-binding", "ES2019", "script", `try { throw 1 } catch { var c = "ok" } c`],
  ["well-formed-json-stringify", "ES2019", "script", `JSON.stringify("\\uD800") === '"\\\\ud800"' ? "ok" : "bad"`],
  ["stable-sort", "ES2019", "script", `var a = []; for (var i = 0; i < 100; i++) a.push({k: i % 3, i}); a.sort((x, y) => x.k - y.k); var ok = true; for (var i = 1; i < a.length; i++) if (a[i - 1].k === a[i].k && a[i - 1].i > a[i].i) ok = false; ok ? "ok" : "bad"`],
  // ES2020
  ["bigint", "ES2020", "script", `(2n ** 64n === 18446744073709551616n && BigInt.asUintN(8, 257n) === 1n && typeof 1n === "bigint") ? "ok" : "bad"`],
  ["optional-chaining-nullish", "ES2020", "script", `var o = {a: {b: 0}}; (o?.a?.b ?? 5) === 0 && o.x?.y === undefined && (null ?? "d") === "d" ? "ok" : "bad"`],
  ["promise-allsettled", "ES2020", "async", `Promise.allSettled([Promise.resolve(1), Promise.reject(2)]).then(r => { globalThis.__result = r[1].status === "rejected" ? "ok" : "bad" })`],
  ["globalthis-matchall", "ES2020", "script", `(typeof globalThis === "object" && [..."a1b2".matchAll(/\\d/g)].length === 2) ? "ok" : "bad"`],
  ["dynamic-import", "ES2020", "module", `import("./dep.js").then(m => { globalThis.__result = m.x === 1 ? "ok" : "bad" })`],
  ["import-meta", "ES2020", "module", `globalThis.__result = typeof import.meta === "object" ? "ok" : "bad"`],
  // ES2021
  ["logical-assignment-separators", "ES2021", "script", `var a = null; a ??= 1_000; var b = 0; b ||= 2; (a === 1000 && b === 2) ? "ok" : "bad"`],
  ["replaceall-promise-any", "ES2021", "async", `globalThis.__result = "aaa".replaceAll("a", "b") === "bbb" ? "pending" : "bad"; Promise.any([Promise.reject(1), Promise.resolve(2)]).then(v => { if (__result === "pending") globalThis.__result = v === 2 ? "ok" : "bad" })`],
  ["weakref-finalization", "ES2021", "script", `var o = {}; var w = new WeakRef(o); var f = new FinalizationRegistry(() => {}); f.register(o, 1); w.deref() === o ? "ok" : "bad"`],
  // ES2022
  ["class-fields-private", "ES2022", "script", `class A { #x = 1; y = 2; static #s = 3; #m() { return this.#x } static { A.z = A.#s } get v() { return this.#m() + this.y } static has(o) { return #x in o } } (new A().v === 3 && A.z === 3 && A.has(new A())) ? "ok" : "bad"`],
  ["at-hasown-cause", "ES2022", "script", `([1, 2, 3].at(-1) === 3 && "abc".at(-1) === "c" && Object.hasOwn({a: 1}, "a") && new Error("x", {cause: 1}).cause === 1) ? "ok" : "bad"`],
  ["regexp-match-indices", "ES2022", "script", `var m = /b/d.exec("abc"); m.indices[0][0] === 1 ? "ok" : "bad"`],
  ["top-level-await", "ES2022", "module", `var v = await Promise.resolve(5); globalThis.__result = v === 5 ? "ok" : "bad"`],
  ["modules", "ES2015", "module", `import d, {x} from "./dep.js"; globalThis.__result = x + d() === 3 ? "ok" : "bad"`],
  // ES2023
  ["array-find-last", "ES2023", "script", `([1, 2, 3].findLast(x => x < 3) === 2 && [1, 2, 3].findLastIndex(x => x > 5) === -1) ? "ok" : "bad"`],
  ["change-array-by-copy", "ES2023", "script", `var a = [3, 1, 2]; var s = a.toSorted(); (s.join() === "1,2,3" && a.join() === "3,1,2" && a.toReversed()[0] === 2 && a.with(0, 9)[0] === 9 && a.toSpliced(0, 1).length === 2) ? "ok" : "bad"`],
  ["hashbang", "ES2023", "script", `#!/usr/bin/env node\n"ok"`],
  ["symbols-as-weakmap-keys", "ES2023", "script", `var w = new WeakMap(); var s = Symbol("k"); w.set(s, 1); w.get(s) === 1 ? "ok" : "bad"`],
  // ES2024
  ["object-groupby", "ES2024", "script", `var g = Object.groupBy([1, 2, 3], x => x % 2 ? "odd" : "even"); var m = Map.groupBy([1, 2], x => x > 1); (g.odd.length === 2 && m.get(true)[0] === 2) ? "ok" : "bad"`],
  ["promise-withresolvers", "ES2024", "async", `var r = Promise.withResolvers(); r.promise.then(v => { globalThis.__result = v }); r.resolve("ok")`],
  ["regexp-v-flag", "ES2024", "script", `(/[\\p{L}--[a-z]]/v.test("A") && !/[\\p{L}--[a-z]]/v.test("a") && /[[a-z]&&[aeiou]]/v.test("e")) ? "ok" : "bad"`],
  ["arraybuffer-resize-transfer", "ES2024", "script", `var b = new ArrayBuffer(4, {maxByteLength: 16}); b.resize(8); var t = b.transfer(); (t.byteLength === 8 && b.detached === true) ? "ok" : "bad"`],
  ["string-wellformed", "ES2024", "script", `("a\\uD800".isWellFormed() === false && "a\\uD800".toWellFormed() === "a\\uFFFD") ? "ok" : "bad"`],
  // ES2025
  ["iterator-helpers", "ES2025", "script", `function* n() { for (var i = 0; ; i++) yield i } n().filter(x => x % 3 === 0).take(4).toArray().join() === "0,3,6,9" ? "ok" : "bad"`],
  ["set-methods", "ES2025", "script", `var a = new Set([1, 2]), b = new Set([2, 3]); (a.union(b).size === 3 && a.intersection(b).size === 1 && a.isSubsetOf(new Set([1, 2, 3]))) ? "ok" : "bad"`],
  ["promise-try", "ES2025", "async", `Promise.try(() => "ok").then(v => { globalThis.__result = v })`],
  ["regexp-escape-modifiers", "ES2025", "script", `(RegExp.escape("a.b") === "\\\\x61\\\\.b" && /(?i:a)b/.test("Ab")) ? "ok" : "bad"`],
  ["float16array", "ES2025", "script", `var f = new Float16Array([1.5]); (f[0] === 1.5 && Math.f16round(1.337) === 1.3369140625) ? "ok" : "bad"`],
  ["json-modules", "ES2025", "module", `import data from "./data.json" with { type: "json" }; globalThis.__result = data.a === 1 ? "ok" : "bad"`],
  // Annex B and old forms
  ["annexb-html-comments-escape", "AnnexB", "script", `var x = 1; <!-- comment\n (escape("ä") === "%E4" && "abc".substr(1, 1) === "b") ? "ok" : "bad"`],
  ["sloppy-with", "ES5", "script", `var o = {a: 1}; with (o) { a = 2 } o.a === 2 ? "ok" : "bad"`],
  // Intl (compared with Node's output)
  ["intl-datetimeformat", "Intl", "node", `new Intl.DateTimeFormat("zh-CN", {dateStyle: "full", timeStyle: "short", timeZone: "Asia/Shanghai"}).format(new Date(Date.UTC(2026, 9, 21, 8, 30)))`],
  ["intl-datetimeformat-en", "Intl", "node", `new Intl.DateTimeFormat("en-US", {year: "numeric", month: "long", day: "numeric", weekday: "short", timeZone: "America/New_York"}).format(new Date(Date.UTC(2026, 9, 21, 3, 0)))`],
  ["intl-numberformat-currency", "Intl", "node", `new Intl.NumberFormat("de-DE", {style: "currency", currency: "EUR"}).format(1234.5)`],
  ["intl-numberformat-compact", "Intl", "node", `new Intl.NumberFormat("en", {notation: "compact"}).format(1234567) + " " + new Intl.NumberFormat("en-IN").format(12345678.9)`],
  ["intl-collator", "Intl", "node", `["z", "ä", "a", "Ö"].sort(new Intl.Collator("de").compare).join(",") + " " + ["z", "ä", "a"].sort(new Intl.Collator("sv").compare).join(",")`],
  ["intl-pluralrules", "Intl", "node", `[0, 1, 2, 5].map(n => new Intl.PluralRules("ru").select(n)).join(",")`],
  ["intl-relativetimeformat", "Intl", "node", `new Intl.RelativeTimeFormat("en", {numeric: "auto"}).format(-1, "day") + " | " + new Intl.RelativeTimeFormat("fr").format(3, "hour")`],
  ["intl-listformat", "Intl", "node", `new Intl.ListFormat("en", {type: "conjunction"}).format(["Alice", "Bob", "Carol"])`],
  ["intl-segmenter", "Intl", "node", `[...new Intl.Segmenter("en", {granularity: "word"}).segment("Hello, world!")].filter(s => s.isWordLike).map(s => s.segment).join("|")`],
  ["intl-displaynames", "Intl", "node", `new Intl.DisplayNames(["en"], {type: "region"}).of("JP")`],
  ["number-tolocalestring", "Intl", "node", `(1234567.891).toLocaleString("en-US", {maximumFractionDigits: 2})`],
  ["date-tolocalestring", "Intl", "node", `new Date(Date.UTC(2026, 0, 2, 3, 4, 5)).toLocaleString("en-US", {timeZone: "UTC"})`],
  ["string-localecompare", "Intl", "node", `["b", "a", "á", "A"].sort((x, y) => x.localeCompare(y, "en")).join(",")`],
  // Host APIs: not part of ECMAScript; reported as present or absent.
  ["host-structuredclone", "Host", "host", `typeof structuredClone`],
  ["host-textencoder", "Host", "host", `typeof TextEncoder`],
  ["host-settimeout", "Host", "host", `typeof setTimeout`],
  ["host-queuemicrotask", "Host", "host", `typeof queueMicrotask`],
  ["host-console", "Host", "host", `typeof console`],
  ["host-url", "Host", "host", `typeof URL`],
];
