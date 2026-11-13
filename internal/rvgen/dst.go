// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package rvgen

import (
	"fmt"
	"strings"
)

// dstPrelude adds what the destructurings of Options.Dst use: mis, which a
// default calls to change the array being destructured or the built-ins its
// iterator comes from, fix, which run calls to put the built-ins back, and
// sources that are not plain arrays.
func (g *gen) dstPrelude() {
	g.line(`var AP = Array.prototype, AIP = Object.getPrototypeOf([][Symbol.iterator]()), IP = Object.getPrototypeOf(AIP);`)
	g.line(`var VAL = AP[Symbol.iterator], NXT = AIP.next;`)
	g.line(`function fix() { AIP.next = NXT; AP[Symbol.iterator] = VAL; delete AIP.return; delete IP.return; delete AP[2]; delete AP[3]; }`)
	g.line(`function mis(k, a) {`)
	g.line(`  L.push("mis " + k);`)
	g.line(`  switch (k) {`)
	g.line(`  case 0: a.push(9); break;`)
	g.line(`  case 1: a.pop(); break;`)
	g.line(`  case 2: a.length = 0; break;`)
	g.line(`  case 3: a.length = 6; break;`)
	g.line(`  case 4: a[40] = 4; break;`)
	g.line(`  case 5: AIP.next = function () { L.push("next"); return NXT.call(this); }; break;`)
	g.line(`  case 6: AP[Symbol.iterator] = function () { L.push("iter"); return VAL.call(this); }; break;`)
	g.line(`  case 7: AIP.return = function () { L.push("return"); return {}; }; break;`)
	g.line(`  case 8: Object.defineProperty(IP, "return", { configurable: true, get: function () { L.push("get return"); return undefined; } }); break;`)
	g.line(`  case 9: AP[3] = "p3"; break;`)
	g.line(`  case 10: Object.defineProperty(AP, 2, { configurable: true, get: function () { L.push("get 2"); return "g2"; } }); break;`)
	g.line(`  case 11: delete a[0]; break;`)
	g.line(`  case 12: a.shift(); break;`)
	g.line(`  case 13: delete AP[Symbol.iterator]; break;`)
	g.line(`  case 14: AIP.return = 5; break;`)
	g.line(`  case 15: thr(k);`)
	g.line(`  }`)
	g.line(`  return k;`)
	g.line(`}`)
	g.line(`class DA extends Array {}`)
	g.line(`function dit(n) { var i = 0, it = { next: function () { i++; return { value: i, done: i > n }; }, return: function () { L.push("closed " + i); return {}; } }; it[Symbol.iterator] = function () { return it; }; return it; }`)
}

const dstMis = 16

// dstSource is the value a destructuring takes apart.
func (g *gen) dstSource() string {
	switch g.n(16) {
	case 0, 1, 2, 3, 4, 5:
		n := g.n(6)
		el := make([]string, n)
		for i := range el {
			el[i] = g.expr(1, false)
		}
		return "[" + strings.Join(el, ", ") + "]"
	case 6:
		return []string{"[1, , 3]", "[, 2]", "[1, 2, , ]"}[g.n(3)]
	case 7:
		return []string{`"ab"`, `"x"`, `""`}[g.n(3)]
	case 8:
		return []string{"new Set([1, 2, 3])", "new Map([[1, 2], [3, 4]])"}[g.n(2)]
	case 9:
		return "o1.b"
	case 10:
		return fmt.Sprintf("Array.from({ length: %d }, function (_, i) { return i * 2; })", g.n(40))
	case 11:
		return fmt.Sprintf("dit(%d)", g.n(4))
	case 12:
		return []string{"Object.freeze([1, 2])", "Object.assign([1, 2, 3], { x: 1 })", "DA.from([1, 2, 3])"}[g.n(3)]
	case 13:
		return "(function () { return arguments; })(1, 2, 3)"
	case 14:
		return []string{"null", "5", "o0", "undefined"}[g.n(4)]
	default:
		return "[" + g.expr(1, false) + ", [" + g.expr(1, false) + ", " + g.expr(1, false) + "]]"
	}
}

// dstPattern makes an array pattern over the source s. decl: its targets
// are new bindings, added to names, else locals and properties. depth
// bounds the nesting.
func (g *gen) dstPattern(s string, decl bool, names *[]string, depth int) string {
	n := g.n(5)
	el := make([]string, 0, n+1)
	for i := 0; i < n; i++ {
		var t string
		switch k := g.n(10); {
		case k == 0:
			el = append(el, "")
			continue
		case k == 1 && depth > 0:
			t = g.dstPattern(s, decl, names, depth-1)
		default:
			t = g.dstTarget(decl, names)
		}
		if g.chance(0.4) {
			if g.chance(0.7) {
				t += fmt.Sprintf(" = mis(%d, %s)", g.n(dstMis), s)
			} else {
				t += " = " + g.expr(1, false)
			}
		}
		el = append(el, t)
	}
	if g.chance(0.25) {
		el = append(el, "..."+g.dstTarget(decl, names))
	} else if len(el) > 0 && el[len(el)-1] == "" {
		// a last hole is written with its comma
		el = append(el, "")
	}
	return "[" + strings.Join(el, ", ") + "]"
}

func (g *gen) dstTarget(decl bool, names *[]string) string {
	if decl {
		b := fmt.Sprintf("b%d", g.uniq)
		g.uniq++
		*names = append(*names, b)
		return b
	}
	switch g.n(5) {
	case 0:
		return "o0.a"
	case 1:
		return "o2.s"
	case 2:
		return `o3["c"]`
	default:
		v := g.local()
		*names = append(*names, v)
		return v
	}
}

// dstStmt is a destructuring of an array pattern, in one of the forms the
// parser makes it in, in a try that logs what it throws.
func (g *gen) dstStmt() {
	s := fmt.Sprintf("s%d", g.uniq)
	g.uniq++
	g.line("try {")
	g.ind++
	g.line("var %s = %s;", s, g.dstSource())
	if g.chance(0.2) {
		g.line("mis(%d, %s);", g.n(dstMis), s)
	}
	var names []string
	list := func() string { return "[" + strings.Join(names, ", ") + "]" }
	switch g.n(8) {
	case 0, 1:
		kw := []string{"var", "let", "const"}[g.n(3)]
		p := g.dstPattern(s, true, &names, 2)
		g.line("%s %s = %s;", kw, p, s)
		g.line("rec(%s);", list())
	case 2:
		p := g.dstPattern(s, false, &names, 2)
		g.line("rec(%s = %s);", p, s)
		g.line("rec(%s);", list())
	case 3:
		p := g.dstPattern(s, true, &names, 1)
		g.line("rec((function (%s) { return %s; })(%s));", p, list(), s)
	case 4:
		p := g.dstPattern(s, true, &names, 1)
		g.line("rec(((%s = %s) => %s)());", p, s, list())
	case 5:
		p := g.dstPattern(s, true, &names, 1)
		g.line("for (const %s of [%s, %s]) rec(%s);", p, s, s, list())
	case 6:
		p := g.dstPattern(s, true, &names, 1)
		g.line("try { throw %s; } catch (%s) { rec(%s); }", s, p, list())
	default:
		// a yield in a default, then the generator resumed, returned or
		// thrown into
		x, y := fmt.Sprintf("b%d", g.uniq), fmt.Sprintf("b%d", g.uniq+1)
		g.uniq += 2
		g.line("var it%s = (function* (a) { const [%s = yield mis(%d, a), %s = yield 2] = a; return [%s, %s]; })(%s);",
			s, x, g.n(dstMis), y, x, y, s)
		op := []string{"next", "return", "throw"}[g.n(3)]
		g.line("rec(it%s.next());", s)
		g.line("try { rec(it%s.%s(5)); } catch (e) { rec(e instanceof Error ? e.name : e); }", s, op)
		g.line("rec(it%s.next());", s)
	}
	g.ind--
	g.line("} catch (e) {")
	g.line("  rec(e instanceof Error ? e.%s : e);", g.msg())
	g.line("}")
}
