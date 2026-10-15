// Collects error stacks in many situations into globalThis.result.
// TestErrorStack compares them with testdata/error_stack.golden.
var out = [];
function log(label, v) { out.push("== " + label + "\n" + (typeof v === "string" ? v : JSON.stringify(v))); }
function st(e) { return e && e.stack; }

function a() { return b(); }
function b() { return c(); }
function c() { return new Error("deep"); }
log("nested", st(a()));
log("Error() call", st((function f() { return Error("call"); })()));
log("native map", st([1].map(function cb() { return new Error("m"); })[0]));
log("native sort", st((function () { var r; [2, 1].sort(function cmp() { r = new Error("s"); return 0; }); return r; })()));
log("eval", st(eval("(function inEval() { return new Error('e') })()")));
log("indirect eval", st((0, eval)("new Error('ie')")));
log("new Function", st(new Function("return new Error('nf')")()));
log("arrow", st((() => new Error("arrow"))()));
log("method", st({ meth() { return new Error("meth"); } }.meth()));
log("computed name", st({ ["comp" + 1]() { return new Error("c"); } }.comp1()));
log("class ctor", st(new (class K { constructor() { this.e = new Error("k"); } })().e));
class MyErr extends Error {}
log("subclass", st(new MyErr("sub")));
log("aggregate", st(new AggregateError([], "agg")));
log("bound", st((function bnd() { return new Error("bound"); }).bind(null)()));
log("getter", st(Object.defineProperty({}, "g", { get: function getg() { return new Error("g"); } }).g));
log("proxy", st(new Proxy({}, { get: function trap() { return new Error("p"); } }).x));
log("tail", st((function t1() { return (function t2() { return new Error("t"); })(); })()));
log("json reviver", st((function () { var r; JSON.parse("[1]", function rev(k, v) { if (!r) r = new Error("j"); return v; }); return r; })()));
log("replace cb", st((function () { var r; "a".replace(/a/, function rcb() { r = new Error("r"); return ""; }); return r; })()));
try { null.x; } catch (e) { log("TypeError engine", st(e)); log("TypeError msg", e.message); }
try { undefinedVar; } catch (e) { log("ReferenceError", st(e)); }
try { (function thr() { throw new RangeError("thrown"); })(); } catch (e) { log("thrown", st(e)); }
try { eval("a b c"); } catch (e) { log("SyntaxError eval", st(e)); log("SyntaxError props", [e.fileName, e.lineNumber, e.columnNumber]); }
try { new Function("{"); } catch (e) { log("SyntaxError Function", st(e)); }
log("deep", st((function rec(n) { return n ? rec(n - 1) : new Error("deep"); })(300)));
try { [].reduce(function r() {}); } catch (e) { log("native TypeError", st(e)); }
try { JSON.parse("{"); } catch (e) { log("JSON SyntaxError", st(e)); }
try { new Array(-1); } catch (e) { log("RangeError native", st(e)); }
try { Symbol() + ""; } catch (e) { log("Symbol TypeError", st(e)); }

// the name is read when the error is made
function renamed() { return new Error("rn"); }
var e1 = renamed();
Object.defineProperty(renamed, "name", { value: "changed" });
log("renamed after", st(e1));
log("renamed before", st(renamed()));
function noname() { return new Error("nn"); }
delete noname.name;
log("deleted name", st(noname()));
function symname() { return new Error("sn"); }
Object.defineProperty(symname, "name", { value: Symbol("s") });
log("symbol name", st(symname()));
function numname() { return new Error("num"); }
Object.defineProperty(numname, "name", { value: 42 });
log("number name", st(numname()));
function getname() { return new Error("gn"); }
Object.defineProperty(getname, "name", { get() { throw new Error("must not run"); } });
log("getter name", st(getname()));
function ropename() { return new Error("rope"); }
Object.defineProperty(ropename, "name", { value: "x".repeat(300) + "y".repeat(300) });
log("long name", st(ropename()));
function emptyname() { return new Error("en"); }
Object.defineProperty(emptyname, "name", { value: "" });
log("empty name", st(emptyname()));

// the stack property itself
var e = new Error("p");
log("desc", Object.getOwnPropertyDescriptor(e, "stack"));
log("keys", Object.keys(e));
log("ownKeys", Reflect.ownKeys(e));
log("names", Object.getOwnPropertyNames(e));
log("json", JSON.stringify(e));
log("spread", Object.keys({ ...e }));
log("hasOwn", [e.hasOwnProperty("stack"), Object.hasOwn(e, "stack"), "stack" in e]);
var forin = []; for (var k in e) forin.push(k); log("forin", forin);
var e2 = new Error("assign"); e2.stack = "custom"; log("assigned", st(e2));
var e3 = new Error("def"); Object.defineProperty(e3, "stack", { value: 7, enumerable: true }); log("defined", [e3.stack, JSON.stringify(e3), Object.keys(e3)]);
var e4 = new Error("del"); delete e4.stack; log("deleted", [e4.stack, e4.hasOwnProperty("stack")]);
var e5 = new Error("frozen"); Object.freeze(e5); log("frozen", [Object.isFrozen(e5), st(e5), Object.getOwnPropertyDescriptor(e5, "stack")]);
var e6 = new Error("sealed"); log("isFrozen before", [Object.isFrozen(e6), Object.isSealed(e6)]); Object.seal(e6); log("sealed", Object.getOwnPropertyDescriptor(e6, "stack"));
var e7 = new Error("twice"); log("twice", st(e7) === st(e7));
var e8 = new Error("getter def"); Object.defineProperty(e8, "stack", { get() { return "g"; } }); log("getter stack", e8.stack);
var e9 = new Error("nonwritable"); Object.defineProperty(e9, "stack", { writable: false }); e9.stack = "no"; log("nonwritable", st(e9));
var e10 = new Error("assign"); log("assign returns", (e10.stack = 5)); log("assign value", e10.stack);
var e11 = new Error("pe"); log("prevent", [Object.preventExtensions(e11) === e11, st(e11)]);
var e12 = new Error("entries"); log("entries", Object.entries(e12)); log("values", Object.getOwnPropertyDescriptors(e12));
var e13 = Object.create(new Error("proto")); log("inherited", [e13.hasOwnProperty("stack"), st(e13)]);
log("toString", String(new Error("ts")));

// error made in a function whose code is gone afterwards
var e14 = eval("(function gone() { return new Error('gone') })()");
log("gone", st(e14));
var e15 = new Function("return (function inner() { return new Error('fn') })()")();
if (typeof gc === "function") gc();
log("gone fn", st(e15));

// generators and async functions
function* gen() { yield 1; yield new Error("gen"); }
var g = gen(); g.next(); log("generator", st(g.next().value));
async function af() { await null; return new Error("async"); }
async function af2() { return await af(); }
af2().then(function (v) { log("async", st(v)); });
Promise.reject(new Error("rejected")).catch(function onrej(e) { log("rejected", st(e)); });
new Promise(function exec(res, rej) { rej(new Error("exec")); }).catch(function (e) { log("executor", st(e)); });
Promise.resolve().then(function th() { throw new TypeError("then"); }).catch(function (e) { log("then throw", st(e)); });
Promise.resolve().then(function th2() { null.y; }).catch(function (e) { log("then engine", st(e)); });
(async function aw() { try { await Promise.reject(new Error("awrej")); } catch (e) { log("await rej", st(e)); } })();

// cycles: an error kept alive by a closure in its own frames
var keep = [];
for (var i = 0; i < 3; i++) {
  (function cyc() { var err = new Error("cyc" + i); err.self = function () { return err; }; keep.push(err.self); })();
}
log("cycle", st(keep[1]()));
keep = null;
if (typeof gc === "function") gc();

// many errors, read some
var many = [];
for (var i = 0; i < 1000; i++) many.push(new Error("m" + i));
log("many", st(many[999]).length);

// stackTraceLimit is not supported, and changing it does nothing
Error.stackTraceLimit = 1;
log("limit", st(a()));
delete Error.stackTraceLimit;

// the result is read once the jobs above have run
Promise.resolve().then(function () {}).then(function () {}).then(function () {}).then(function last() {
  globalThis.result = out.join("\n");
});
