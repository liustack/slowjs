// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package v1 is rvgen frozen as it was when the goldens of the fuzz seeds
// were recorded from the old loop (engine/test/regvm/golden/fuzz): the same
// seed must make the same program for good, so this copy is never changed.
// New shapes go into package rvgen.
//
// Package rvgen makes small random JavaScript programs for comparing the two
// interpreter loops of the engine (the old loop and the register loop of
// engine/regvm.h): the same program must give the same output with the
// register loop off and on.
//
// A program is one strict function expression called at once, so that its
// inner functions are what the register pass converts: functions of locals and
// arguments, arithmetic on ints, floats and strings, compares, calls with
// missing and extra arguments, methods, recursion that a depth argument
// bounds, loops with breaks, switches on strings, object and array literals,
// property reads and writes, getters that call back, and, in fewer of them,
// closures, try, catch and finally with throws at different depths, for-in
// and for-of. Every value a function returns, or the exception it throws,
// goes into a log, and the program's value is the log and a checksum of it.
//
// Every program ends: a function calls only functions made before it, and
// itself only with a smaller depth, and every loop counts to a small bound.
// The same seed gives the same program. Each statement is on a line of its
// own, and a block's lines between the line that opens it and the line that
// closes it, so that a shrinker can take lines out (see Lines).
package v1

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Options of a program.
type Options struct {
	Funcs int // functions, at least 2 (default 8)
	Stmts int // statements in a function body, at most (default 6)
	Depth int // nesting of statements and expressions, at most (default 3)
	// Core keeps to what the register pass took first (locals and
	// arguments, add, sub, lt and strict equality, fields, calls, jumps):
	// most of each program then converts. Without it every construct
	// above is used.
	Core bool
	// Wide adds, to the full mode, what the packages of the register loop
	// take later: new of functions, classes and non-constructors, ++, --
	// and compound assignments in expressions, unary operators, in,
	// instanceof, element writes, delete, spread, regular expressions and
	// getters that throw. Programs of the other modes stay as they were.
	Wide bool
}

func (o *Options) fill() {
	if o.Wide {
		o.Core = false
	}
	if o.Funcs < 2 {
		o.Funcs = 8
	}
	if o.Stmts <= 0 {
		o.Stmts = 6
	}
	if o.Depth <= 0 {
		o.Depth = 3
	}
}

type gen struct {
	r    *rand.Rand
	o    Options
	b    strings.Builder
	ind  int
	fn   int      // the function being made
	argc []int    // parameters of each function, after d
	vars []string // locals and parameters in scope
	objs int      // the objects of the pool
	loop int      // loops around the statement
	uniq int
}

// Generate returns the program of the seed.
func Generate(seed uint64, o Options) string {
	o.fill()
	g := &gen{r: rand.New(rand.NewPCG(seed, 0x5eed)), o: o, objs: 4}
	g.program(seed)
	return g.b.String()
}

func (g *gen) n(k int) int { return g.r.IntN(k) }

func (g *gen) chance(p float64) bool { return g.r.Float64() < p }

func (g *gen) line(format string, args ...any) {
	g.b.WriteString(strings.Repeat("  ", g.ind))
	fmt.Fprintf(&g.b, format, args...)
	g.b.WriteByte('\n')
}

func (g *gen) program(seed uint64) {
	g.line("// rvgen seed %d funcs %d stmts %d depth %d core %v%s", seed, g.o.Funcs, g.o.Stmts, g.o.Depth, g.o.Core, map[bool]string{true: " wide"}[g.o.Wide])
	if g.o.Wide {
		g.wideTop()
	}
	g.line(`(function () { "use strict";`)
	g.ind++
	// the log and its helpers: refused by the pass or not, they are not
	// what is tested, but they are called the same way both ways
	g.line(`var L = [];`)
	// a budget of calls for each run, so that the fan-out of the recursion
	// stays small: past it a function throws
	g.line(`var B = { n: 0 };`)
	// show writes a value in at most 300 characters, objects to a depth of 3,
	// so that a value built up in a loop stays cheap to log
	g.line(`function show(v, n) {`)
	g.line(`  if (n === undefined) n = 0;`)
	g.line(`  if (typeof v === "number" && v === 0 && 1 / v < 0) return "-0";`)
	g.line(`  if (typeof v === "symbol" || typeof v === "function") return typeof v;`)
	g.line(`  if (typeof v === "string") return "s:" + (v.length > 300 ? v.slice(0, 300) + "..." + v.length : v);`)
	g.line(`  if (typeof v !== "object" || v === null) return typeof v + ":" + String(v);`)
	g.line(`  if (n > 2) return "{...}";`)
	g.line(`  var ks = Object.keys(v), r = Array.isArray(v) ? "[" : "{";`)
	g.line(`  for (var i = 0; i < ks.length && i < 6; i++) { var d = Object.getOwnPropertyDescriptor(v, ks[i]); r += ks[i] + ":" + ("value" in d ? show(d.value, n + 1) : "get") + ","; }`)
	g.line(`  return r + (Array.isArray(v) ? "]" : "}");`)
	g.line(`}`)
	// cap keeps what a loop builds small: a string doubled in each turn
	// would fill the memory
	g.line(`function cap(v) { if (typeof v === "string" && v.length > 64) return v.slice(0, 64); return v; }`)
	g.line(`function rec(v) { L.push(show(v)); return v; }`)
	g.line(`function thr(x) { throw new RangeError("thr " + show(x)); }`)
	g.line(`function run(name, f, d, a, b, c) {`)
	g.line(`  B.n = 0;
  try { L.push(name + " = " + show(f(d, a, b, c))); } catch (e) { L.push(name + " ! " + (e instanceof Error ? e.name + ": " + e.message : show(e))); }`)
	g.line(`}`)
	// the pool of objects the functions read and write
	g.line(`var o0 = { a: 1, b: "s", c: 2.5, n: null, m: function (x) { return x + this.a; } };`)
	g.line(`var o1 = { a: "x", b: [1, 2, 3], c: { d: 4 }, m: function (x, y) { return this.a + x + y; } };`)
	g.line(`var o2 = { a: 2147483647, b: -0, c: undefined, get g() { return leaf(this.a, 1); }, set s(v) { this.a = v; } };`)
	g.line(`var o3 = { a: 0.1, b: true, c: "10", valueOf: function () { return leaf(3, 4); } };`)
	g.line(`function leaf(x, y) { return x - y; }`)
	if g.o.Wide {
		g.widePrelude()
	}
	g.argc = make([]int, g.o.Funcs)
	for i := range g.o.Funcs {
		g.argc[i] = g.n(3)
	}
	for i := range g.o.Funcs {
		g.function(i)
	}
	// the driver: each function with arguments of every kind
	for i := range g.o.Funcs {
		for k := 0; k < 2; k++ {
			g.vars = nil
			g.fn = g.o.Funcs
			g.line(`run("f%d.%d", f%d, %d, %s, %s, %s);`, i, k, i, g.n(4), g.literal(), g.literal(), g.literal())
		}
	}
	g.line(`run("map", function () { return [1, 2, 3].map(function (x) { return f%d(1, x, x); }); }, 0);`, g.n(g.o.Funcs))
	g.line(`var h = 0;`)
	g.line(`for (var i = 0; i < L.length; i++) for (var j = 0; j < L[i].length; j++) h = (h * 31 + L[i].charCodeAt(j)) | 0;`)
	g.line(`return L.join("\n") + "\nsum " + h;`)
	g.ind--
	g.line(`})()`)
}

func (g *gen) function(i int) {
	g.fn = i
	params := []string{"d"}
	for k := 0; k < g.argc[i]; k++ {
		params = append(params, fmt.Sprintf("p%d", k))
	}
	nloc := 1 + g.n(3)
	g.vars = append([]string(nil), params...)
	g.line("function f%d(%s) {", i, strings.Join(params, ", "))
	g.ind++
	locs := make([]string, nloc)
	for k := range locs {
		locs[k] = fmt.Sprintf("v%d", k)
		locs[k] += " = " + g.expr(1, false)
	}
	g.line("var %s;", strings.Join(locs, ", "))
	for k := range nloc {
		g.vars = append(g.vars, fmt.Sprintf("v%d", k))
	}
	// the depth bounds the recursion, the budget the calls of a run
	g.line("B.n = B.n + 1;")
	g.line("if (3000 < B.n) return thr(B.n);")
	g.line("if (d < 1) return %s;", g.expr(1, false))
	for range 1 + g.n(g.o.Stmts) {
		g.stmt(g.o.Depth)
	}
	g.line("return %s;", g.expr(g.o.Depth, true))
	g.ind--
	g.line("}")
}

func (g *gen) local() string {
	// a local, not a parameter: v0 and up are after d and the parameters
	var l []string
	for _, v := range g.vars {
		if v[0] == 'v' || v[0] == 'i' {
			l = append(l, v)
		}
	}
	if len(l) == 0 {
		return "v0"
	}
	return l[g.n(len(l))]
}

func (g *gen) anyVar() string { return g.vars[g.n(len(g.vars))] }

func (g *gen) obj() string { return fmt.Sprintf("o%d", g.n(g.objs)) }

func (g *gen) stmt(depth int) {
	if g.o.Wide && depth > 0 && g.chance(0.3) {
		g.wideStmt(depth)
		return
	}
	if depth <= 0 {
		g.line("%s = %s;", g.local(), g.expr(2, true))
		return
	}
	k := g.n(100)
	full := !g.o.Core
	switch {
	case k < 22:
		g.line("%s = %s;", g.local(), g.capped(g.expr(depth, true)))
	case k < 30:
		g.line("rec(%s);", g.expr(depth, true))
	case k < 36:
		if full && g.chance(0.5) {
			g.line("%s++;", g.local())
		} else {
			v := g.local()
			g.line("%s = %s + 1;", v, v)
		}
	case k < 44:
		g.line("%s.%s = %s;", g.objOrVar(), g.prop(), g.capped(g.expr(depth-1, true)))
	case k < 56:
		g.line("if (%s) {", g.cond(depth))
		g.block(depth - 1)
		if g.chance(0.5) {
			g.line("} else {")
			g.block(depth - 1)
		}
		g.line("}")
	case k < 66:
		iv := fmt.Sprintf("i%d", g.uniq)
		g.uniq++
		bound := 1 + g.n(6)
		g.line("for (var %s = 0; %s < %d; %s = %s + 1) {", iv, iv, bound, iv, iv)
		g.line("  B.n = B.n + 1;")
		g.line("  if (3000 < B.n) break;")
		g.vars = append(g.vars, iv)
		g.loop++
		g.block(depth - 1)
		if g.chance(0.5) {
			g.line("if (%s) break;", g.cond(1))
		}
		g.loop--
		g.line("}")
	case k < 72 && full:
		g.line("switch (%s) {", g.strExpr())
		for _, c := range g.cases() {
			g.line("case %q:", c)
			g.ind++
			g.stmt(depth - 1)
			if g.chance(0.7) {
				g.line("break;")
			}
			g.ind--
		}
		g.line("default:")
		g.ind++
		g.stmt(depth - 1)
		g.ind--
		g.line("}")
	case k < 76 && g.loop > 0:
		g.line("if (%s) break;", g.cond(1))
	case k < 82 && full:
		g.line("try {")
		g.block(depth - 1)
		g.line("} catch (e) {")
		g.ind++
		g.line("%s = e instanceof Error ? e.message : e;", g.local())
		g.ind--
		if g.chance(0.5) {
			g.line("} finally {")
			g.block(depth - 1)
		}
		g.line("}")
	case k < 86 && full:
		kv := fmt.Sprintf("k%d", g.uniq)
		g.uniq++
		g.line("for (var %s in %s) {", kv, g.obj())
		g.vars = append(g.vars, kv)
		g.ind++
		v := g.local()
		g.line("%s = cap(%s + %s);", v, v, kv)
		g.ind--
		g.line("}")
	case k < 90 && full:
		kv := fmt.Sprintf("x%d", g.uniq)
		g.uniq++
		g.line("for (var %s of [%s, %s, %s]) {", kv, g.expr(1, false), g.expr(1, false), g.expr(1, false))
		g.vars = append(g.vars, kv)
		g.loop++
		g.block(depth - 1)
		g.loop--
		g.line("}")
	case k < 93:
		g.line("if (%s) return %s;", g.cond(1), g.expr(depth, true))
	default:
		g.line("%s = %s;", g.local(), g.expr(depth, true))
	}
}

// capped wraps what a statement in a loop stores in cap.
func (g *gen) capped(e string) string {
	if g.loop > 0 {
		return "cap(" + e + ")"
	}
	return e
}

func (g *gen) block(depth int) {
	g.ind++
	for range 1 + g.n(2) {
		g.stmt(depth)
	}
	g.ind--
}

func (g *gen) objOrVar() string {
	if g.chance(0.7) {
		return g.obj()
	}
	return g.anyVar()
}

var props = []string{"a", "b", "c", "n", "x"}

func (g *gen) prop() string { return props[g.n(len(props))] }

func (g *gen) cases() []string {
	all := []string{"a", "b", "s", "x", "string:s", "10", "number:1"}
	n := 1 + g.n(3)
	g.r.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return all[:n]
}

func (g *gen) strExpr() string {
	switch g.n(4) {
	case 0:
		return g.obj() + ".b"
	case 1:
		return "show(" + g.anyVar() + ")"
	case 2:
		return g.anyVar()
	default:
		return fmt.Sprintf("%q", []string{"a", "b", "s", "x"}[g.n(4)])
	}
}

func (g *gen) cond(depth int) string {
	a, b := g.expr(depth-1, false), g.expr(depth-1, false)
	ops := []string{"<", "===", "<", "==="}
	if !g.o.Core {
		ops = append(ops, "!==", "==", "!=", "<=", ">", ">=")
	}
	op := ops[g.n(len(ops))]
	if !g.o.Core && g.chance(0.15) {
		return "!(" + a + " " + op + " " + b + ")"
	}
	if !g.o.Core && g.chance(0.1) {
		return "typeof " + a + " === \"number\""
	}
	return a + " " + op + " " + b
}

var ints = []string{"0", "1", "2", "-1", "7", "100", "2147483647", "-2147483648", "1073741823", "4294967296"}
var floats = []string{"0.5", "-1.5", "1e300", "-0", "NaN", "Infinity", "0.1"}
var strs = []string{`""`, `"a"`, `"10"`, `"s"`, `"x"`, `"1"`}

var coreInts = []string{"0", "1", "2", "7", "100", "2147483647", "1073741823"}

func (g *gen) literal() string {
	if g.o.Core {
		// no string, float or negative literal: they are constants the
		// pass took later (strings and floats through o0 to o3)
		switch g.n(6) {
		case 0, 1, 2:
			return coreInts[g.n(len(coreInts))]
		case 3:
			return []string{"true", "false", "null", "undefined"}[g.n(4)]
		case 4:
			return g.obj()
		default:
			return g.obj() + "." + g.prop()
		}
	}
	switch g.n(10) {
	case 0, 1, 2, 3:
		return ints[g.n(len(ints))]
	case 4:
		return floats[g.n(len(floats))]
	case 5, 6:
		return strs[g.n(len(strs))]
	case 7:
		return []string{"true", "false", "null", "undefined"}[g.n(4)]
	case 8:
		return g.obj()
	default:
		return fmt.Sprintf("{ a: %s, b: %s }", ints[g.n(len(ints))], strs[g.n(len(strs))])
	}
}

// expr makes an expression. calls says whether it may call functions.
func (g *gen) expr(depth int, calls bool) string {
	if g.o.Wide && depth > 0 && g.chance(0.3) {
		return g.wideExpr(depth, calls)
	}
	if depth <= 0 || g.chance(0.25) {
		if len(g.vars) > 0 && g.chance(0.6) {
			return g.anyVar()
		}
		return g.literal()
	}
	full := !g.o.Core
	k := g.n(100)
	switch {
	case k < 20:
		return "(" + g.expr(depth-1, calls) + " + " + g.expr(depth-1, calls) + ")"
	case k < 30:
		return "(" + g.expr(depth-1, calls) + " - " + g.expr(depth-1, calls) + ")"
	case k < 36:
		return "(" + g.cond(depth) + ")"
	case k < 46:
		// a field: of an object, of what a variable holds (it may be
		// undefined, null or a primitive, and throw), a length
		switch g.n(4) {
		case 0:
			return g.anyVar() + "." + g.prop()
		case 1:
			return g.anyVar() + ".length"
		default:
			return g.obj() + "." + g.prop()
		}
	case k < 64 && calls && g.fn > 0 && g.fn <= g.o.Funcs:
		// a function made before this one: any depth, any number of
		// arguments
		j := g.n(min(g.fn, g.o.Funcs))
		return g.call(fmt.Sprintf("f%d", j), depth)
	case k < 70 && calls && g.fn < g.o.Funcs:
		// itself, one level less deep
		return g.callSelf(depth)
	case k < 76 && calls:
		switch g.n(4) {
		case 0:
			return "o0.m(" + g.expr(depth-1, false) + ")"
		case 1:
			return "o1.m(" + g.expr(depth-1, false) + ", " + g.expr(depth-1, false) + ")"
		case 2:
			return "o2.g"
		default:
			return "leaf(" + g.expr(depth-1, false) + ")"
		}
	case k < 80 && full:
		return "{ a: " + g.expr(depth-1, calls) + ", b: " + g.expr(depth-1, calls) + " }"
	case k < 83 && full:
		return "[" + g.expr(depth-1, calls) + ", " + g.expr(depth-1, calls) + "]"
	case k < 85 && calls:
		return "thr(" + g.expr(depth-1, false) + ")"
	case k < 87:
		return "o3"
	case full && k < 91:
		op := []string{"*", "%", "|", "&", "<<", ">>", "/"}[g.n(7)]
		return "(" + g.expr(depth-1, calls) + " " + op + " " + g.expr(depth-1, calls) + ")"
	case full && k < 93:
		return "(" + g.cond(depth) + " ? " + g.expr(depth-1, calls) + " : " + g.expr(depth-1, calls) + ")"
	case full && k < 95:
		return g.obj() + ".b[" + g.expr(depth-1, false) + "]"
	case full && k < 96 && calls:
		// a closure over a local: its function has captured locals
		return "(function (x) { return x + " + g.local() + "; })(" + g.expr(depth-1, false) + ")"
	case full && k < 98:
		return "typeof " + g.expr(depth-1, calls)
	default:
		return g.literal()
	}
}

func (g *gen) args(depth int) []string {
	n := g.n(5) // missing and extra arguments
	a := make([]string, n)
	for i := range a {
		a[i] = g.expr(depth-1, false)
	}
	return a
}

func (g *gen) call(name string, depth int) string {
	a := append([]string{fmt.Sprint(g.n(3))}, g.args(depth)...)
	return name + "(" + strings.Join(a, ", ") + ")"
}

func (g *gen) callSelf(depth int) string {
	a := append([]string{"d - 1"}, g.args(depth)...)
	return fmt.Sprintf("f%d(%s)", g.fn, strings.Join(a, ", "))
}

// Lines splits a program into its lines, for a shrinker: Blocks gives, for
// each line that opens a block, the line that closes it.
func Lines(prog string) []string {
	return strings.Split(strings.TrimSuffix(prog, "\n"), "\n")
}

// Blocks returns, for each line index that ends with an open brace, the
// index of the line where its block closes (at the same indentation).
func Blocks(lines []string) map[int]int {
	out := map[int]int{}
	var stack []int
	for i, l := range lines {
		t := strings.TrimSpace(l)
		closes := strings.HasPrefix(t, "}")
		opens := strings.HasSuffix(t, "{")
		if closes && len(stack) > 0 && !opens {
			out[stack[len(stack)-1]] = i
			stack = stack[:len(stack)-1]
		}
		if opens && !closes {
			stack = append(stack, i)
		}
	}
	return out
}
