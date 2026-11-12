// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package rvgen

import (
	"fmt"
	"regexp"
	"strconv"
)

// the bodies of the functions of the last-reference calls: they use a and b
// only (the a of a closure becomes x), return, throw, and return through
// finally
// the parameter a of a body, which a closure calls x
var lrParam = regexp.MustCompile(`\ba\b`)

var lrBodies = []string{
	"return a;",
	`throw new RangeError("lr " + typeof a);`,
	"try { return a; } finally { b = 2; }",
	"try { throw a; } catch (e) { return e; } finally { b = 1; }",
	"try { return 1; } finally { return typeof a; }",
	"var s = 0; for (var i = 0; i < 5; i++) s += i; return s + arguments.length;",
}

// widePrelude adds what the statements and expressions of the wide mode
// use: a constructor, two classes, an object whose getter and setter throw,
// regular expressions and an array.
func (g *gen) widePrelude() {
	g.line(`function C(a) { this.a = a; this.k = leaf(a, 1); }`)
	g.line(`C.prototype.m = function (x) { return this.a + x; };`)
	g.line(`class K { constructor(x) { this.x = x; } get y() { return this.x; } m(z) { return this.x + z; } static s(v) { return v; } }`)
	g.line(`class K2 extends K { constructor(x) { super(x + 1); this.z = x; } m(z) { return super.m(z) + 1; } }`)
	g.line(`var o4 = { a: 3, b: [4, 5], get t() { return thr(this.a); }, set u(v) { thr(v); } };`)
	g.line(`var r0 = /a+/g, r1 = /(x)(\d*)/;`)
	g.line(`var ar = [1, 2, 3];`)
	// ar and o1.b can be made very long by a write of a large index: what
	// walks them takes a slice
	// an iterator that logs when for-of closes it
	// calls that hand over the last reference to the function they call: a
	// Function made at run time (the constants of the caller do not keep its
	// bytecode alive), called once to convert it, then called with the only
	// reference in a box, in a property the callee deletes, or in a global
	// the callee overwrites
	g.line(`function lrBox(src, a) { var f = Function("a", "b", src); try { f(a, 1); } catch (e) {} var box = [f]; f = null; return box.pop()(a); }`)
	g.line(`function lrArgs(src, a) { var f = Function("a", "b", src); try { f(a); } catch (e) {} var box = [f]; f = null; return box.pop()(a, 2, 3, 4); }`)
	g.line(`function lrDel(src, a) { var o = {}; var f = Function("a", "o", "delete o.m; " + src); try { f(a, o); } catch (e) {} o.m = f; f = null; return o.m(a, o); }`)
	g.line(`function lrSelf(src, a) { globalThis.__lr = Function("a", "globalThis.__lr = null; " + src); var k = globalThis.__lr; try { k(a); } catch (e) {} globalThis.__lr = k; k = null; return globalThis.__lr(a); }`)
	g.line(`function itr(n) { var i = 0, it = { next: function () { i++; return { value: i, done: i > n }; }, return: function () { L.push("closed " + i); return {}; } }; it[Symbol.iterator] = function () { return it; }; return it; }`)
}

// wideStmt is a statement of the wide mode.
func (g *gen) wideStmt(depth int) {
	v := g.local()
	switch g.n(16) {
	case 0:
		ops := []string{"%s++;", "%s--;", "++%s;", "--%s;", "%s -= 1;", "%s *= 2;", "%s |= 0;", "%s <<= 1;", "%s >>= 1;", "%s %%= 7;"}
		g.line(ops[g.n(len(ops))], v)
		if g.loop == 0 && g.chance(0.3) {
			// a string grows only outside loops
			g.line("%s += %s;", v, g.expr(1, false))
		}
	case 1:
		target := []string{"o1.b", "ar", "o4.b", g.anyVar()}[g.n(4)]
		g.line("%s[%s] = %s;", target, g.expr(1, false), g.capped(g.expr(depth-1, true)))
	case 2:
		target := []string{"o0", "o1", "o2", "o3", "ar", g.anyVar()}[g.n(6)]
		prop := []string{"a", "x", "n", "length"}[g.n(4)]
		g.line("delete %s.%s;", target, prop)
	case 3:
		g.line("o4.u = %s;", g.expr(1, false))
	case 4:
		kv := fmt.Sprintf("x%d", g.uniq)
		g.uniq++
		g.line("for (var %s of %s) {", kv, []string{"ar", "o1.b", "show(" + g.anyVar() + ")", "o4.b"}[g.n(4)])
		g.line("  B.n = B.n + 1;")
		g.line("  if (3000 < B.n) break;")
		g.vars = append(g.vars, kv)
		g.loop++
		g.block(depth - 1)
		g.loop--
		g.line("}")
	case 5:
		g.line("try {")
		g.block(depth - 1)
		g.line("} finally {")
		g.ind++
		g.line("%s = %s;", v, g.capped(g.expr(1, false)))
		g.ind--
		g.line("}")
	case 6:
		g.line("if (%s in %s) {", g.expr(1, false), []string{"o0", "o2", "ar", g.anyVar()}[g.n(4)])
		g.block(depth - 1)
		g.line("}")
	case 7:
		// closures over the variable of each turn of a loop
		fs := fmt.Sprintf("fs%d", g.uniq)
		j := fmt.Sprintf("j%d", g.uniq)
		g.uniq++
		if g.chance(0.5) {
			g.line("var %s = [];", fs)
			g.line("for (let %s = 0; %s < 3; %s++) { %s.push(() => %s + %s); }", j, j, j, fs, j, g.local())
		} else {
			g.line("var %s = [];", fs)
			g.line("for (const %s of ar.slice(0, 4)) { %s.push(function () { return %s; }); }", j, fs, j)
		}
		g.line("%s = %s[0]() + %s[%s.length - 1]();", v, fs, fs, fs)
	case 8:
		// return from try, catch and finally: a return in finally wins
		g.line("try {")
		g.ind++
		g.line("if (%s) return %s;", g.cond(1), g.expr(1, false))
		g.stmt(depth - 1)
		g.ind--
		g.line("} catch (e) {")
		g.ind++
		g.line("if (%s) return e instanceof Error ? e.%s : e;", g.cond(1), g.msg())
		g.ind--
		g.line("} finally {")
		g.ind++
		g.line("%s = %s;", v, g.capped(g.expr(1, false)))
		if g.chance(0.3) {
			g.line("if (%s) return %s;", g.cond(1), v)
		}
		g.ind--
		g.line("}")
	case 9:
		if g.loop == 0 {
			g.line("rec(%s);", g.wideExpr(depth, true))
			return
		}
		// break and continue through finally
		g.line("try {")
		g.ind++
		g.line("if (%s) %s;", g.cond(1), []string{"break", "continue"}[g.n(2)])
		g.stmt(depth - 1)
		g.ind--
		g.line("} finally {")
		g.ind++
		g.line("%s = %s;", v, g.capped(g.expr(1, false)))
		g.ind--
		g.line("}")
	case 10:
		// for-of over an iterator with return(), left early by break, return
		// or a throw
		kv := fmt.Sprintf("x%d", g.uniq)
		g.uniq++
		g.line("for (var %s of itr(%d)) {", kv, 1+g.n(5))
		g.line("  B.n = B.n + 1;")
		g.line("  if (3000 < B.n) break;")
		g.vars = append(g.vars, kv)
		g.ind++
		g.line("if (%s) %s;", g.cond(1), []string{"break", "return " + kv, "thr(" + kv + ")", "continue"}[g.n(4)])
		g.ind--
		g.loop++
		g.block(depth - 1)
		g.loop--
		g.line("}")
	case 11:
		// ++ and -- on fields and elements of every kind of value
		target := []string{"o0.a", "o1.a", "o2.a", "o3.a", "o2.g", "o3.b", "ar[" + g.expr(1, false) + "]", "o4.b[0]", g.anyVar() + ".a"}[g.n(9)]
		g.line([]string{"%s++;", "%s--;", "++%s;", "--%s;"}[g.n(4)], target)
	case 12:
		g.line("%s = %s;", v, g.capped(g.wideLiteral()))
	default:
		g.line("rec(%s);", g.wideExpr(depth, true))
	}
}

// wideLiteral is a value of a kind the other modes do not make.
func (g *gen) wideLiteral() string {
	return []string{"1n", "-1n", "(2n ** 64n)", `Symbol("s")`, "{ valueOf: function () { return \"7\"; } }", "{ valueOf: function () { return 2n; } }",
		"Object.create(null)", "new Number(3)", "new String(\"5\")", "[]", "[9]", "null", "undefined", "-0", "0.5", "\"x\""}[g.n(16)]
}

// wideTop makes, before the strict function of the program, sloppy
// functions: this is the global object, or a primitive is boxed, and
// arguments follows the parameters.
func (g *gen) wideTop() {
	g.line(`function sl0(a) { var t = this; return (t === globalThis ? "global" : t === undefined ? "undefined" : t === null ? "null" : Object.prototype.toString.call(t) + ":" + (typeof t.valueOf === "function" ? typeof t.valueOf() : "-")) + "/" + arguments.length + "/" + typeof a; }`)
	g.line(`function sl1(a, b) { a = b; return arguments[0]; }`)
	g.line(`function sl2(x) { return this === globalThis ? x : this && this.a; }`)
}

// wideExpr is an expression of the wide mode.
func (g *gen) wideExpr(depth int, calls bool) string {
	e := func() string { return g.expr(depth-1, false) }
	switch g.n(21) {
	case 0:
		return fmt.Sprintf("("+[]string{"%s++", "%s--", "++%s", "--%s"}[g.n(4)]+")", g.local())
	case 1:
		return fmt.Sprintf("(%s++)", g.local())
	case 2:
		return "(" + []string{"- ", "+ ", "!", "~", "void "}[g.n(5)] + e() + ")"
	case 3:
		return "(" + e() + " in " + []string{"o0", "o2", "ar", g.anyVar()}[g.n(4)] + ")"
	case 4:
		return "(" + e() + " instanceof " + []string{"C", "K", "K2", "Object", "o0"}[g.n(5)] + ")"
	case 5:
		return "new C(" + e() + ")"
	case 6:
		return []string{"new K(" + e() + ").m(" + e() + ")", "new K2(" + e() + ").y", "new K2(" + e() + ").m(" + e() + ")"}[g.n(3)]
	case 7:
		// constructors that are not classes, and things that are not
		// constructors (a TypeError)
		return []string{"new leaf(" + e() + ")", "new o0.m(" + e() + ")", "new K.s(" + e() + ")", "new Math.max(" + e() + ")", "new (" + e() + ")"}[g.n(5)]
	case 8:
		return []string{
			"r0.test(show(" + e() + "))",
			"/a(b)?/.exec(show(" + e() + "))",
			"show(" + e() + ").replace(/[a-z]/g, \"_\")",
			"r1.exec(show(" + e() + "))",
			"/^s:/.test(show(" + e() + "))",
		}[g.n(5)]
	case 9:
		// a getter that throws, or one that calls back
		return []string{"o4.t", "o2.g", "Object.create(o2).g"}[g.n(3)]
	case 10:
		return []string{"leaf(...[" + e() + ", " + e() + "])", "[...ar.slice(0, 4), " + e() + "]", "{ ...o0, a: " + e() + " }", "[..." + "show(" + e() + ")].length"}[g.n(4)]
	case 11:
		return "`" + "${" + e() + "}-${" + e() + "}`"
	case 12:
		return []string{"ar", "o4.b", "o1.b"}[g.n(3)] + "[" + e() + "]"
	case 13:
		return "(" + e() + ", " + e() + ")"
	case 14:
		return "((x) => x + " + g.local() + ")(" + e() + ")"
	case 15, 16:
		// sloppy functions with this of every kind
		return []string{"sl0.call(" + e() + ", " + e() + ")", "sl0.call(" + g.wideLiteral() + ")", "sl0(" + e() + ")", "sl0.apply(" + e() + ", [" + e() + ", " + e() + "])",
			"({ m: sl0 }).m(" + e() + ")", "new sl0(" + e() + ")", "sl1(" + e() + ", " + e() + ")", "sl2.call(" + g.obj() + ", " + e() + ")", "sl2(" + e() + ")"}[g.n(9)]
	case 17:
		return g.wideLiteral()
	case 19:
		// the last reference to the called function, handed over by the call
		body := lrBodies[g.n(len(lrBodies))]
		if g.chance(0.5) {
			body = `"use strict"; ` + body
		}
		return []string{"lrBox", "lrArgs", "lrDel", "lrSelf"}[g.n(4)] + "(" + strconv.Quote(body) + ", " + e() + ")"
	case 20:
		// a closure made and called once, a method that deletes itself, a
		// function that overwrites the only variable that holds it
		body := "var b = 0; " + lrParam.ReplaceAllString(lrBodies[g.n(len(lrBodies))], "x")
		return []string{
			"(function (x) { " + body + " })(" + e() + ")",
			"[function (x) { " + body + " }].pop()(" + e() + ")",
			"[function (x) { " + body + " }].pop().call(null, " + e() + ", 1, 2)",
			"(function (x) { var o = { m: function (x) { delete o.m; " + body + " } }; return o.m(x); })(" + e() + ")",
			"(function (x) { var g = function (x) { g = null; " + body + " }; return g(x); })(" + e() + ")",
			"[function* (x) { yield x; " + body + " }].pop()(" + e() + ").next().value",
			"typeof [async function (x) { " + body + " }].pop()(" + e() + ").catch(function () {})",
		}[g.n(7)]
	default:
		return []string{g.anyVar() + "?.a", "(" + e() + " ?? " + e() + ")", g.anyVar() + "?.b?.length"}[g.n(3)]
	}
}
