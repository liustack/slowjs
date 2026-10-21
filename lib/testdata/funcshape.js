'use strict';
// The own properties of new functions of every kind, made many times so that
// the later ones take the shape funcShape remembers, and of functions changed
// after they were made, beside new ones from the same code. Each line is a
// case and, in JSON, the names and keys of the function, the descriptors of
// length and name, and what its prototype property and its own prototype
// are.
var out = [];
var protos = new Map([[Function.prototype, 'Function.prototype']]);
protos.set(Object.getPrototypeOf(async function () {}), 'AsyncFunction.prototype');
protos.set(Object.getPrototypeOf(function* () {}), 'GeneratorFunction.prototype');
protos.set(Object.getPrototypeOf(async function* () {}), 'AsyncGeneratorFunction.prototype');
protos.set(null, 'null');
function desc(f, k) {
  var d = Object.getOwnPropertyDescriptor(f, k);
  if (!d) return null;
  return [typeof d.value === 'symbol' ? String(d.value) : d.value, d.writable, d.enumerable, d.configurable, typeof d.get, typeof d.set];
}
function show(label, f) {
  var proto = Object.getPrototypeOf(f);
  out.push(label + ' ' + JSON.stringify([
    Object.getOwnPropertyNames(f),
    Reflect.ownKeys(f).map(String),
    desc(f, 'length'),
    desc(f, 'name'),
    desc(f, 'prototype') && typeof f.prototype,
    protos.has(proto) ? protos.get(proto) : typeof proto,
    Object.isExtensible(f),
  ]));
}
function attempt(f) {
  try { return String(f()); } catch (e) { return e.name; }
}
class Base { constructor(a) { this.a = a; } static s(x, y) {} m(a, b, c) {} get g() { return 1; } set g(v) {} }
var kinds = {
  decl: function () { function named(a, b) {} return named; },
  expr: function () { return function (a) {}; },
  namedExpr: function () { return function inner(a, b, c) {}; },
  arrow: function () { return (a, b) => a; },
  arrowNoArgs: function () { return () => 0; },
  rest: function () { return function (a, ...b) {}; },
  defaults: function () { return function (a, b = 1, c) {}; },
  method: function () { return { m(x) {} }.m; },
  computed: function () { var k = 'c' + 1; return { [k]: function () {} }[k]; },
  symbolName: function () { return { [Symbol.iterator]() {} }[Symbol.iterator]; },
  symbolNoDesc: function () { var s = Symbol(); return { [s]: function () {} }[s]; },
  indexName: function () { return { 1() {}, 2: function (a) {} }[1]; },
  indexName2: function () { return { 1() {}, 2: function (a) {} }[2]; },
  getter: function () { return Object.getOwnPropertyDescriptor({ get x() { return 1; } }, 'x').get; },
  setter: function () { return Object.getOwnPropertyDescriptor({ set x(v) {} }, 'x').set; },
  async: function () { return async function (a) {}; },
  asyncArrow: function () { return async (a, b) => a; },
  asyncMethod: function () { return { async m() {} }.m; },
  generator: function () { return function* (a) {}; },
  generatorMethod: function () { return { *g(a, b) {} }.g; },
  asyncGenerator: function () { return async function* (a) {}; },
  klass: function () { return class K { constructor(a, b) {} }; },
  anonClass: function () { return class {}; },
  derived: function () { return class D extends Base {}; },
  staticMethod: function () { return class { static s(a) {} }.s; },
  classMethod: function () { return (class { m(a, b) {} }).prototype.m; },
  classField: function () { return new (class { f = (a) => a; })().f; },
  // The Function constructor makes sloppy functions, which have own
  // arguments and caller in Node: strict ones have not.
  newFunction: function () { return new Function('a', 'b', "'use strict'; return a"); },
  newAsync: function () { return new (Object.getPrototypeOf(async function () {}).constructor)('a', "'use strict'"); },
  bound: function () { return function bf(a, b, c) {}.bind(null, 1); },
  boundArrow: function () { return ((a, b) => a).bind(null); },
  resolve: function () { var r; new Promise(function (res) { r = res; }); return r; },
  reject: function () { var r; new Promise(function (res, rej) { r = rej; }); return r; },
  revoke: function () { return Proxy.revocable({}, {}).revoke; },
  builtin: function () { return Math.max; },
  closure: function () { var x = 1; return function (y) { return x + y; }; },
};
var names = Object.keys(kinds);
for (var round = 0; round < 3; round++) {
  for (var name of names) {
    var made = [];
    for (var i = 0; i < 20; i++) made.push(kinds[name]());
    show(round + ' ' + name, made[0]);
    show(round + ' ' + name + ' last', made[19]);
    var same = made.every(function (f) {
      return JSON.stringify(Object.getOwnPropertyNames(f)) === JSON.stringify(Object.getOwnPropertyNames(made[0])) &&
        f.length === made[0].length && f.name === made[0].name;
    });
    out.push(round + ' ' + name + ' all the same ' + same);
  }
  // Change functions in every way that changes their shape, then make new
  // ones: they must not see it.
  var f = kinds.expr(), g = kinds.namedExpr(), h = kinds.arrow(), k = kinds.async(), m = kinds.generator();
  f.extra = 1;
  out.push(round + ' delete length ' + delete g.length);
  out.push(round + ' write name ' + attempt(function () { h.name = 'x'; }));
  out.push(round + ' write length ' + attempt(function () { h.length = 9; }));
  Object.defineProperty(h, 'name', { value: 'renamed', writable: true });
  Object.defineProperty(k, 'length', { get: function () { return 7; } });
  Object.setPrototypeOf(m, null);
  var frozen = kinds.closure();
  Object.freeze(frozen);
  var sealed = kinds.method();
  Object.preventExtensions(sealed);
  show(round + ' changed f', f);
  show(round + ' changed g', g);
  show(round + ' changed h', h);
  show(round + ' changed k', k);
  show(round + ' changed m', m);
  show(round + ' frozen', frozen);
  show(round + ' sealed', sealed);
  out.push(round + ' names ' + JSON.stringify([h.name, k.length, g.length, f.extra, frozen.name, sealed.length]));
  for (var name of ['expr', 'namedExpr', 'arrow', 'async', 'generator', 'closure', 'method']) show(round + ' after ' + name, kinds[name]());
  // The prototype property of a constructor, made on first use.
  var c1 = kinds.decl(), c2 = kinds.decl();
  var p1 = c1.prototype;
  show(round + ' read prototype', c1);
  show(round + ' not read', c2);
  out.push(round + ' prototype ' + JSON.stringify([Object.getOwnPropertyNames(p1), p1.constructor === c1, c1.prototype === p1]));
  // A property added to the prototype of new functions, which they share.
  Function.prototype['shared' + round] = round;
  show(round + ' after Function.prototype changed', kinds.expr());
  out.push(round + ' shared ' + kinds.expr()['shared' + round]);
  delete Function.prototype['shared' + round];
}
out.join('\n');
