// Edge cases of defining new properties: object literals, spread, class
// fields, JSON.parse, built-ins that make objects. objlit_test.go compares
// the transcript this script returns with objlit.txt, the output of the
// engine before the fast path of lib/objlit.go.
var out = [];
function d(o) {
  var r = [];
  var ks = Reflect.ownKeys(o);
  for (var i = 0; i < ks.length; i++) {
    var k = ks[i], desc = Object.getOwnPropertyDescriptor(o, k);
    r.push(String(k) + ':' + ('value' in desc ? typeof desc.value + '=' + (typeof desc.value === 'object' && desc.value ? '[obj]' : String(desc.value)) : 'get/set') + ':' + (desc.writable ? 'w' : '') + (desc.enumerable ? 'e' : '') + (desc.configurable ? 'c' : ''));
  }
  return r.join(',');
}
function t(name, f) { try { out.push(name + ' => ' + f()); } catch (e) { out.push(name + ' !! ' + e.name + ': ' + e.message); } }
var s = 'x' + 1;
t('plain', function () { return d({ a: 1, b: s, c: null, d: undefined }); });
t('dup keys', function () { return d({ a: 1, b: 2, a: 3 }); });
t('numeric keys', function () { return d({ 2: 'two', 0: 'zero', b: 1, 1: 'one', '-1': 'neg' }); });
t('proto literal', function () { var o = { __proto__: { inh: 1 }, a: 1 }; return d(o) + ' inh=' + o.inh; });
t('proto null', function () { var o = { __proto__: null, a: 1, b: 2 }; return d(o) + ' proto=' + Object.getPrototypeOf(o); });
t('computed', function () { var k = 'z'; return d({ [k]: 1, a: 2, [k + k]: 3 }); });
t('getter in literal', function () { return d({ a: 1, get b() { return 2; }, c: 3 }); });
t('method', function () { var o = { a: 1, m() { return this.a; } }; return d(o) + ' m=' + o.m(); });
t('spread getters', function () { var n = 0; var src = { get a() { n++; return n; }, b: 2 }; var o = { ...src, c: 3 }; return d(o) + ' n=' + n; });
t('spread over existing', function () { return d({ a: 1, ...{ a: 2, b: 3 }, b: 4 }); });
t('setter on proto not called', function () { var called = 0; Object.defineProperty(Object.prototype, 'zz', { set: function () { called++; }, configurable: true }); var o = { zz: 1 }; delete Object.prototype.zz; return d(o) + ' called=' + called; });
t('readonly on proto ignored', function () { Object.defineProperty(Object.prototype, 'ro', { value: 0, writable: false, configurable: true }); var o = { ro: 1 }; delete Object.prototype.ro; return d(o); });
t('class fields', function () { class A { x = 1; y = s; static z = 3; } return d(new A()) + ' | ' + d(A).replace(/prototype:[^,]*,?/, ''); });
t('class field on non-extensible', function () { class B { constructor() { Object.preventExtensions(this); } } class C extends B { f = 1; } new C(); return 'no throw'; });
t('class field on frozen base', function () { class B { constructor() { Object.freeze(this); } } class C extends B { f = 1; } new C(); return 'no throw'; });
t('class field redefines', function () { class B { constructor() { this.f = 0; } } class C extends B { f = 1; } return d(new C()); });
t('class field on non-configurable', function () { class B { constructor() { Object.defineProperty(this, 'f', { value: 0, writable: true, configurable: false, enumerable: true }); } } class C extends B { f = 1; } return d(new C()); });
t('class field on array', function () { class A extends Array { f = 1; } var a = new A(); a.push(5); return d(a); });
t('class field index on array', function () { class A extends Array { 0 = 'z'; } var a = new A(); return d(a) + ' len=' + a.length; });
t('class field on proxy this', function () { var log = []; class B { constructor() { return new Proxy({}, { defineProperty: function (t, k, desc) { log.push(String(k)); return Reflect.defineProperty(t, k, desc); } }); } } class C extends B { f = 1; g = 2; } var c = new C(); return d(c) + ' trap=' + log.join(''); });
t('class field on typed array', function () { class B { constructor() { return new Uint8Array(2); } } class C extends B { f = 1; } return d(new C()); });
t('class field on string object', function () { class B { constructor() { return new String('ab'); } } class C extends B { 2 = 'q'; f = 1; } class D extends B { 1 = 'q'; } var r = 'no throw'; try { new D(); } catch (e) { r = e.name + ': ' + e.message; } return d(new C()) + ' | ' + r; });
t('class field on global', function () { class B { constructor() { return globalThis; } } class C extends B { zzfield = 7; } new C(); var r = d({ v: Object.getOwnPropertyDescriptor(globalThis, 'zzfield').value }); delete globalThis.zzfield; return r; });
t('class field private', function () { class A { #p = 1; q = 2; get p() { return this.#p; } } var a = new A(); return d(a) + ' p=' + a.p; });
t('json', function () { return d(JSON.parse('{"a":1,"b":"s","a":3,"__proto__":5,"0":0}')); });
t('json reviver', function () { return d(JSON.parse('{"a":1,"b":2}', function (k, v) { return k === 'a' ? undefined : v; })); });
t('fromEntries', function () { return d(Object.fromEntries([['a', 1], ['b', 2], ['a', 3]])); });
t('iterator result', function () { function* g() { yield 1; } return d(g().next()); });
t('function props', function () { var f = function (a, b) {}; var h = (x) => x; return d(f) + ' | ' + d(h); });
t('bound', function () { var f = function named(a, b) {}.bind(null, 1); return d(f); });
t('regexp exec', function () { return d(/(?<n>a)/.exec('a')).replace(/input:[^,]*,/, ''); });
t('groupBy', function () { return d(Object.groupBy([1, 2, 3], function (x) { return x % 2 ? 'odd' : 'even'; })); });
t('promise withResolvers', function () { return Object.keys(Promise.withResolvers()).join(); });
t('error', function () { var e = new Error('m', { cause: 1 }); return Object.getOwnPropertyNames(e).sort().join(); });
t('literal stack key', function () { return d({ stack: 1, message: 2 }); });
t('arguments', function () { return (function () { return d(arguments); })(1, 2); });
t('getOwnPropertyDescriptors', function () { return d(Object.getOwnPropertyDescriptors({ a: 1 }).a); });
t('many keys', function () { var o = {}; for (var i = 0; i < 40; i++) o['k' + i] = i; var p = { ...o }; return Object.keys(p).length + ' ' + p.k39; });
t('literal 20 keys', function () { var o = { a0: 0, a1: 1, a2: 2, a3: 3, a4: 4, a5: 5, a6: 6, a7: 7, a8: 8, a9: 9, a10: 10, a11: 11, a12: 12, a13: 13, a14: 14, a15: 15, a16: 16, a17: 17, a18: 18, a19: 19 }; return Object.keys(o).join(); });
t('for-in order after literal', function () { var r = []; for (var k in { b: 1, a: 2, 1: 3 }) r.push(k); return r.join(); });
t('symbols', function () { var sy = Symbol('s'); return d({ [sy]: 1, a: 2 }); });
t('shape sharing', function () { var a = { x: 1, y: 2 }, b = { x: 3, y: 4 }; b.z = 5; delete a.x; return d(a) + ' | ' + d(b); });
t('object freeze after literal', function () { var o = Object.freeze({ a: 1 }); try { 'use strict'; o.b = 2; } catch (e) {} return d(o) + ' ext=' + Object.isExtensible(o); });
t('template object', function () { function tag(s) { return d(s); } return tag`a${1}b`; });
t('defineProperties', function () { return d(Object.defineProperties({}, { a: { value: 1, enumerable: true }, b: { get: function () {} } })); });
t('structured via assign', function () { return d(Object.assign({}, { a: 1 }, null, { b: 2 })); });
out.join("\n");
