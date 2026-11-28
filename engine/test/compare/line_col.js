// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The lines and columns of the code (the pc2line table a compile makes) in
// the stacks of errors, at many places of sources of every shape: functions
// nested deep, lines ended by CRLF, characters of two, three and four bytes
// in UTF-8 and lone surrogates before the place, templates and comments
// across lines, lines longer than a few thousand bytes, and the code of
// eval, direct and indirect, and of new Function. Each source is made by a
// fixed generator and run: every here() in it gives the stack where it is,
// and each group prints how many there were, a hash of all of them and the
// first few. The positions of syntax errors are there too.
var out = []

function fnv(s) {
	var h = 0x811c9dc5
	for (var i = 0; i < s.length; i++) {
		h ^= s.charCodeAt(i)
		h = Math.imul(h, 0x01000193) >>> 0
	}
	return ("0000000" + h.toString(16)).slice(-8)
}

var seed = 1
function rnd(n) {
	seed = (Math.imul(seed, 1103515245) + 12345) >>> 0
	return (seed >>> 8) % n
}

var got = []
globalThis.here = function () {
	var s = String(new Error().stack).split("\n")
	got.push(s.slice(1).join(" / ").replace(/ +/g, " "))
	return 1
}

var lone = String.fromCharCode(0xd800)
// spaces and comments, which go anywhere, and statements of a string or a
// template, which go where a statement may start
var spaces = [" ", "  ", "\t", "\n", "\r\n", "\n\n", " /* a\n b */ ", " /* é */ ", " // c\n", " // 中\r\n",
	" /* 😀" + lone + " */ "]
var literals = [" 'é'; ", " \"中文\"; ", " '😀'; ", " '" + lone + "'; ", " `\n`; ", " `\r\né`; "]
function pad() {
	var s = "", k = rnd(4)
	while (k--) s += spaces[rnd(spaces.length)]
	if (rnd(12) == 0) s += " /*" + " ".repeat(100 + rnd(300)) + "*/ "
	if (rnd(60) == 0) s += " /*" + "x".repeat(4000 + rnd(2000)) + "*/ "
	return s
}
function spad() {
	var s = pad(), k = rnd(3)
	while (k--) s += literals[rnd(literals.length)] + pad()
	return s
}
// an expression that calls here() and the functions in it, depth levels down
function expr(depth) {
	var k = rnd(depth > 0 ? 11 : 3)
	var p = pad, sp = spad, inner = depth > 0 ? function () { return expr(depth - 1) } : function () { return "here()" }
	switch (k) {
	case 0: return "here()" + p()
	case 1: return "(" + p() + "here()" + p() + "+" + p() + "here())"
	case 2: return "`" + p() + "${here()}" + p() + "\n${" + p() + "here()}`.length"
	case 3: return "(function" + p() + "() {" + sp() + "here();" + sp() + "return " + inner() + p() + "})()"
	case 4: return "((a) =>" + p() + inner() + p() + "+ a)(" + p() + "here())"
	case 5: return "((a) => {" + sp() + "here();" + sp() + "var b = " + inner() + ";" + p() + "return b + a })(1)"
	case 6: return "({ m() {" + sp() + "here();" + sp() + "return " + inner() + " } }).m()"
	case 7: return "(class { static m() {" + p() + "return here() +" + p() + inner() + " } }).m()"
	case 8: return "(function* () {" + p() + "yield " + inner() + ";" + p() + "})().next().value"
	case 9: return "({ get g() {" + p() + "return [" + p() + inner() + "," + p() + "here()][0] } }).g"
	case 10: return "(() => { try {" + p() + "throw " + inner() + p() + "} catch (e) { return here() + e } })()"
	}
}
function program(depth, n) {
	var s = spad()
	for (var i = 0; i < n; i++)
		s += (rnd(2) ? "var v" + i + " = " : "") + expr(depth) + ";" + spad() + (rnd(3) ? "\n" : rnd(2) ? "\r\n" : "")
	return s
}
function report(what) {
	var head = got.slice(0, 3).map(function (s) { return "  " + s })
	out.push(what + ": " + got.length + " " + fnv(got.join("\n")))
	out.push.apply(out, head)
	got = []
}

// eval, new Function and direct eval, sources that grow deeper
var depths = [1, 2, 4, 8, 16]
for (var d = 0; d < depths.length; d++) {
	for (var r = 0; r < 6; r++) {
		var src = program(depths[d], 4 + rnd(6))
		var how = r % 3
		try {
			if (how == 0)
				(0, eval)(src)
			else if (how == 1)
				new Function(src)()
			else
				(function () { var local = 1; return eval(src) })()
		} catch (e) {
			got.push("threw " + e + " " + String(e.stack).replace(/\s+/g, " "))
		}
	}
	report("depth " + depths[d])
}

// one line of many functions, longer than the marks are apart many times over
var line = ""
for (var i = 0; i < 300; i++)
	line += "(function f" + i + "() { return here() + " + "'é😀'.length + ".repeat(rnd(3)) + (i % 7 ? "1" : "(() => here())()") + " })();"
;(0, eval)(line)
report("one line")

// functions nested 120 deep on one line and on lines of their own
var open = "", close = ""
for (var i = 0; i < 120; i++) {
	open += "(function () { here(); var x" + i + " = 1;" + (i % 3 ? "" : "\n") + " return "
	close = "; })()" + (i % 5 ? "" : "\r\n") + close
}
;(0, eval)(open + "here()" + close)
report("nested")

// the errors thrown in deep code: every frame of the stack
var src = "", tail = ""
for (var i = 0; i < 40; i++) {
	src += "function g" + i + "() {" + pad() + "return " + (i ? "g" + (i - 1) + "()" : "null.x") + ";" + pad() + "}\n"
}
try { (0, eval)(src + "g39()") } catch (e) { got.push(String(e.stack).replace(/\s+/g, " ")) }
report("thrown")

// syntax errors at many places
var bad = ["\n\n  var 1a", "x = `\n\n${\n)}`", "function f() {\r\n\r\n  return 1 +;\r\n}", "/* é\n😀 */ let let = 1",
	"var s = '中文'; s +* 1", "(function () {\n (function () {\n  (function () {\n   [a, b] =+ 1\n  })\n })\n})",
	"x".repeat(5000) + " +;", "\n".repeat(3) + " ".repeat(200) + "@"]
for (var i = 0; i < bad.length; i++) {
	try { (0, eval)(bad[i]) } catch (e) { got.push(e.name + ": " + e.message + " " + String(e.stack).replace(/\s+/g, " ")) }
	try { new Function(bad[i]) } catch (e) { got.push(e.name + ": " + e.message + " " + String(e.stack).replace(/\s+/g, " ")) }
}
report("syntax errors")

globalThis.result = out.join("\n")
