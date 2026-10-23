// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// fixEq writes the usual pairs of ===, !==, == and != out in the
// interpreter, before its calls of js_strict_eq_slow and js_eq_slow. The
// interpreter compares two ints itself and calls the slow function for
// every other pair, which for === goes on to js_strict_eq2. React compares
// with null and undefined (minified as null != x and void 0 === x, which the
// compiler does not turn into is_null or is_undefined), compares types,
// symbols and objects by identity, and switches over strings. Go does not
// inline a helper into a function as big as the interpreter, so the pairs
// are written out in place, with no call:
//
//   - ===: two values of the same tag that are objects or symbols (the same
//     pointer), null or undefined (equal), booleans (the same value), two
//     strings with the same pointer (equal), of different lengths, or that
//     are both string atoms (different: the atom table holds each text of
//     that type once, while the description of a symbol is an atom of
//     another type and can have the text of a string atom), and two values
//     of different tags when one of them is null, undefined, a boolean, an
//     object or a symbol (different). js_strict_eq2 gives the same for
//     each, and frees both.
//   - ==: null or undefined beside null or undefined (equal) or beside
//     anything else (equal only to an object that is IsHTMLDDA), and the
//     same pairs of objects, symbols, booleans and strings as for ===.
//     js_eq_slow gives the same for each: two values of one tag go to
//     js_strict_eq2, and null or undefined beside a value of another kind
//     ends in its last branch, which tests IsHTMLDDA (a boolean is made a
//     number first, which ends there too).
//
// Each case drops a reference to both values as JS_FreeValue does, frees
// a value with no reference left through eqFree of lib/eq.go, which reads
// it from the stack, and stores the boolean. The text passes eqFree the
// stack pointer rather than the values, and keeps only the result alive
// across that call, so that Go adds next to nothing to the frame of the
// interpreter (8 bytes more on amd64 and 16 fewer on arm64, on v0.26.16).
// No case can throw or call a script. Every other pair calls the slow
// function as before.
//
// Only the files of 64-bit targets (cellFiles) change: the text reads their
// struct JSValue. The bit fields it reads come from the file itself: the
// length of a string from js_string_eq, its atom type from
// __JS_FreeValueRT and IsHTMLDDA from JS_IsHTMLDDA. So does the layout of a
// value (see eqFrame): where its reference count sits, from JS_FreeValue,
// and the size of a value on the stack and how a boolean is stored, from
// js_strict_eq_slow, which the slow calls must also step by. The text of those, of
// js_strict_eq_slow, js_strict_eq2, js_eq_slow and __JS_FreeValue is pinned
// in eqSites: when a release changes one of them, fixEq stops, and the new
// code must be read against the cases above before the fingerprints are
// updated (go run ./internal/fixlib -eq prints them).
func fixEq(b []byte, file string) ([]byte, error) {
	got, err := eqFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := eqSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against the cases of fixEq, then update eqSites (go run ./internal/fixlib -eq)", file, strings.Join(eqFuncs, ", "), got, want)
	}
	bits, err := eqBits(b)
	if err != nil {
		return nil, err
	}
	if cellFiles[file] {
		if bits, err = eqFrame(b, bits); err != nil {
			return nil, err
		}
	}
	body, err := funcBody(b, funcHead("_JS_CallInternal"), "_JS_CallInternal")
	if err != nil {
		return nil, err
	}
	start := bytes.Index(b, body)
	fixed, err := unfixEq(body, bits)
	if err != nil {
		return nil, err
	}
	calls := findAllSubmatch(reEqCall, fixed)
	if len(calls) != 4 {
		return nil, fmt.Errorf("_JS_CallInternal: want 4 calls of js_strict_eq_slow and js_eq_slow, found %d", len(calls))
	}
	seen := map[string]bool{}
	for _, m := range calls {
		seen[string(m[2])+" "+string(m[4])] = true
		if cellFiles[file] && string(m[5]) != fmt.Sprint(bits.val) {
			return nil, fmt.Errorf("_JS_CallInternal: a call of js_strict_eq_slow or js_eq_slow pops %s bytes, and js_strict_eq_slow stores its result %d bytes below the top", m[5], 2*bits.val)
		}
	}
	if len(seen) != 4 {
		return nil, fmt.Errorf("_JS_CallInternal: the calls of js_strict_eq_slow and js_eq_slow are not one for each of ===, !==, == and !=")
	}
	if cellFiles[file] {
		fixed = reEqCall.ReplaceAllFunc(fixed, func(call []byte) []byte {
			m := reEqCall.FindSubmatch(call)
			return []byte(eqText(string(m[1]), string(m[3]), string(m[2]) == "strict_", string(m[4]) != "0", bits))
		})
	}
	return append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...), nil
}

var (
	eqFuncs = []string{"_js_strict_eq_slow", "_js_strict_eq2", "_js_eq_slow", "_js_string_eq", "x__JS_FreeValueRT", "x__JS_FreeValue", "_JS_IsHTMLDDA"}

	// The slow path of the four comparisons, as ccgo writes it: the
	// indent, strict_ or not, the context, is_neq and the size of a value
	// on the stack, 16 bytes on the targets fixEq changes and 8 on the
	// others.
	reEqCall = regexp.MustCompile(`(\t+)\(\*TJSStackFrame\)\(unsafe\.Pointer\(sf\)\)\.Fcur_pc = pc\n\t+if _js_(strict_)?eq_slow\(tls, (ctx\d*), sp, (0|int32\(1\))\) != 0 \{\n\t+goto exception\n\t+\}\n\t+sp -= (16|8)\n`)
	// The start of the text fixEq writes: the indent and the operator.
	reEqFixed = regexp.MustCompile(`(\t+)// Changed by internal/fixlib: the usual pairs of (===|!==|==|!=) without a call \(see fixEq\)\.\n`)

	reEqLen  = regexp.MustCompile(`\n\tif int32\(\*\(\*uint32\)\(unsafe\.Pointer\(p1 \+ 0\)\)&(0x[0-9a-f]+)>>(\d+)\) != int32\(\*\(\*uint32\)\(unsafe\.Pointer\(p2 \+ 0\)\)&(0x[0-9a-f]+)>>(\d+)\) \{\n\t\treturn [^\n]+\n\t\}\n\tif p1 == p2 \{\n`)
	reEqAtom = regexp.MustCompile(`\n\t\tif int32\(Tuint8_t\(\*\(\*uint32\)\(unsafe\.Pointer\(p1 \+ 4\)\)&(0x[0-9a-f]+)>>(\d+)\)\) != 0 \{\n\t\t\t_JS_FreeAtomStruct\(tls, rt1, p1\)\n`)
	// JS_FreeValue: the test for a value with a reference count, the
	// count as an int32 that many bytes below the pointer, and the free
	// when it drops to 0.
	reEqRef = regexp.MustCompile(`\n\tif (?:libc\.Uint32FromInt32|uint32)\(int32\(v\d+\.Ftag\)\) >= libc\.Uint32FromInt32\(EJS_TAG_FIRST\) \{\n\t\tv\d+ = [^\n]+ - libc\.UintptrFromInt32\(1\)\*(\d+)\n\t\tgoto _\d+\n\t_\d+:\n\t\tp = v\d+\n\t\tv\d+ = p\n\t\t\*\(\*int32\)\(unsafe\.Pointer\(v\d+\)\) = \*\(\*int32\)\(unsafe\.Pointer\(v\d+\)\) - 1\n\t\tv\d+ = \*\(\*int32\)\(unsafe\.Pointer\(v\d+\)\)\n\t\tif v\d+ <= libc\.Int32FromInt32\(0\) \{\n\t\t\tx__JS_FreeValue\(tls, ctx\d*, v\d+\)\n`)
	// js_strict_eq_slow: a boolean is its value in the whole union and the
	// tag, stored in the value that many bytes times 2 below the top.
	reEqBool   = regexp.MustCompile(`\n\tv1 = TJSValue\{\n\t\tFu:   \*\(\*TJSValueUnion\)\(unsafe\.Pointer\(&struct\{ f Tuint64_t \}\{f: uint64\(libc\.BoolUint32\(res\^is_neq != libc\.Int32FromInt32\(0\)\)\)\}\)\),\n\t\tFtag: (int64\(EJS_TAG_BOOL\)),\n\t\}\n\tgoto _2\n_2:\n\t\*\(\*TJSValue\)\(unsafe\.Pointer\(sp \+ uintptr\(-libc\.Int32FromInt32\(2\)\)\*(\d+)\)\) = v1\n`)
	reEqHTMLDD = regexp.MustCompile(`\n\treturn int32\(\*\(\*uint8\)\(unsafe\.Pointer\(p \+ (\d+)\)\) & (0x[0-9a-f]+) >> (\d+)\)\n}\n$`)
)

// eqLayout holds the bit fields the text of fixEq reads, as Go expressions
// of a pointer named by %[1]s, and the layout of a value from eqFrame.
type eqLayout struct {
	len, atom, htmldda string
	// ref is how many bytes below the pointer of a value its int32
	// reference count sits, val the size of a value on the stack and
	// boolTag the tag of a boolean as the engine writes it.
	ref, val int
	boolTag  string
}

// eqBits reads the bit fields from the functions of the file that read
// them.
func eqBits(b []byte) (eqLayout, error) {
	var l eqLayout
	get := func(name string, re *regexp.Regexp) ([][]byte, error) {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, err
		}
		m := findAllSubmatch(re, body)
		if len(m) != 1 {
			return nil, fmt.Errorf("%s changed shape: want 1 read of the bit field fixEq takes from it, found %d", strings.TrimLeft(name, "_x"), len(m))
		}
		return m[0], nil
	}
	m, err := get("_js_string_eq", reEqLen)
	if err != nil {
		return l, err
	}
	if !bytes.Equal(m[1], m[3]) || !bytes.Equal(m[2], m[4]) {
		return l, fmt.Errorf("js_string_eq reads the length of its two strings differently")
	}
	l.len = fmt.Sprintf("*(*uint32)(unsafe.Pointer(%%[1]s))&%s>>%s", m[1], m[2])
	if m, err = get("x__JS_FreeValueRT", reEqAtom); err != nil {
		return l, err
	}
	l.atom = fmt.Sprintf("*(*uint32)(unsafe.Pointer(%%[1]s + 4))&%s>>%s", m[1], m[2])
	if m, err = get("_JS_IsHTMLDDA", reEqHTMLDD); err != nil {
		return l, err
	}
	if !isBit(m[3], m[2]) {
		return l, fmt.Errorf("JS_IsHTMLDDA reads the bit with mask %s and shift %s", m[2], m[3])
	}
	l.htmldda = fmt.Sprintf("*(*uint8)(unsafe.Pointer(%%[1]s + %s))&%s != 0", m[1], m[2])
	return l, nil
}

// eqFrame adds to l the layout of a value, from JS_FreeValue and
// js_strict_eq_slow, for a file fixEq changes.
func eqFrame(b []byte, l eqLayout) (eqLayout, error) {
	for _, c := range []struct {
		fn, name, what string
		re             *regexp.Regexp
	}{
		{"XFreeValue", "JS_FreeValue", "drop of a reference", reEqRef},
		{"_js_strict_eq_slow", "js_strict_eq_slow", "store of a boolean result", reEqBool},
	} {
		body, err := funcBody(b, funcHead(c.fn), c.fn)
		if err != nil {
			return l, err
		}
		m := findAllSubmatch(c.re, body)
		if len(m) != 1 {
			return l, fmt.Errorf("%s changed shape: want 1 %s that fixEq can read, found %d: read it against the text of fixEq", c.name, c.what, len(m))
		}
		if c.re == reEqRef {
			fmt.Sscan(string(m[0][1]), &l.ref)
		} else {
			l.boolTag = string(m[0][1])
			fmt.Sscan(string(m[0][2]), &l.val)
		}
	}
	return l, nil
}

// eqSlow is the slow path of a comparison as ccgo writes it.
func eqSlow(indent, ctx string, strict, neq bool, val int) string {
	fn, arg := "_js_eq_slow", "0"
	if strict {
		fn = "_js_strict_eq_slow"
	}
	if neq {
		arg = "int32(1)"
	}
	return indent + "(*TJSStackFrame)(unsafe.Pointer(sf)).Fcur_pc = pc\n" +
		indent + "if " + fn + "(tls, " + ctx + ", sp, " + arg + ") != 0 {\n" +
		indent + "\tgoto exception\n" +
		indent + "}\n" +
		indent + fmt.Sprintf("sp -= %d\n", val)
}

// eqText is the text fixEq writes in place of the slow path of a
// comparison.
func eqText(indent, ctx string, strict, neq bool, l eqLayout) string {
	op := map[[2]bool]string{{true, false}: "===", {true, true}: "!==", {false, false}: "==", {false, true}: "!="}[[2]bool{strict, neq}]
	str := func(p string) string { return fmt.Sprintf(l.len, p) }
	atom := func(p string) string { return fmt.Sprintf(l.atom, p) }
	same := `	case eqTA == eqTB && (eqTA == EJS_TAG_OBJECT || eqTA == EJS_TAG_SYMBOL):
		eqRes = eqPA == eqPB
	case eqTA == eqTB && eqTA == EJS_TAG_BOOL:
		eqRes = int32(*(*uint64)(unsafe.Pointer(&eqA.Fu))) == int32(*(*uint64)(unsafe.Pointer(&eqB.Fu)))
	case eqTA == eqTB && eqTA == EJS_TAG_STRING && (eqPA == eqPB || ` + str("eqPA") + ` != ` + str("eqPB") + ` || ` + atom("eqPA") + ` == EJS_ATOM_TYPE_STRING && ` + atom("eqPB") + ` == EJS_ATOM_TYPE_STRING):
		eqRes = eqPA == eqPB
`
	var cases string
	if strict {
		cases = `	case eqTA == eqTB && (eqTA == EJS_TAG_NULL || eqTA == EJS_TAG_UNDEFINED):
		eqRes = true
` + same + `	case eqTA != eqTB && (eqTA == EJS_TAG_NULL || eqTA == EJS_TAG_UNDEFINED || eqTA == EJS_TAG_BOOL || eqTA == EJS_TAG_OBJECT || eqTA == EJS_TAG_SYMBOL ||
		eqTB == EJS_TAG_NULL || eqTB == EJS_TAG_UNDEFINED || eqTB == EJS_TAG_BOOL || eqTB == EJS_TAG_OBJECT || eqTB == EJS_TAG_SYMBOL):
`
	} else {
		cases = `	case (eqTA == EJS_TAG_NULL || eqTA == EJS_TAG_UNDEFINED) && (eqTB == EJS_TAG_NULL || eqTB == EJS_TAG_UNDEFINED):
		eqRes = true
	case eqTA == EJS_TAG_NULL || eqTA == EJS_TAG_UNDEFINED:
		eqRes = eqTB == EJS_TAG_OBJECT && ` + fmt.Sprintf(l.htmldda, "eqPB") + `
	case eqTB == EJS_TAG_NULL || eqTB == EJS_TAG_UNDEFINED:
		eqRes = eqTA == EJS_TAG_OBJECT && ` + fmt.Sprintf(l.htmldda, "eqPA") + `
` + same
	}
	// The cases are written one tab in above, for reading. A case goes
	// at the depth of its switch.
	cases = strings.ReplaceAll("\n"+cases, "\n\t", "\n")[1:]
	res := "eqRes"
	if neq {
		res = "!eqRes"
	}
	text := `// Changed by internal/fixlib: the usual pairs of ` + op + ` without a call (see fixEq).
eqA, eqB := (*TJSValue)(unsafe.Pointer(sp-` + fmt.Sprint(2*l.val) + `)), (*TJSValue)(unsafe.Pointer(sp-` + fmt.Sprint(l.val) + `))
eqTA, eqTB := eqA.Ftag, eqB.Ftag
eqPA, eqPB := *(*uintptr)(unsafe.Pointer(&eqA.Fu)), *(*uintptr)(unsafe.Pointer(&eqB.Fu))
eqDone, eqRes := true, false
switch {
` + cases + `default:
	eqDone = false
}
if eqDone {
	eqZA := libc.Uint32FromInt32(int32(eqTA)) >= libc.Uint32FromInt32(EJS_TAG_FIRST)
	if eqZA {
		*(*int32)(unsafe.Pointer(eqPA - ` + fmt.Sprint(l.ref) + `))--
		eqZA = *(*int32)(unsafe.Pointer(eqPA - ` + fmt.Sprint(l.ref) + `)) <= 0
	}
	eqZB := libc.Uint32FromInt32(int32(eqTB)) >= libc.Uint32FromInt32(EJS_TAG_FIRST)
	if eqZB {
		*(*int32)(unsafe.Pointer(eqPB - ` + fmt.Sprint(l.ref) + `))--
		eqZB = *(*int32)(unsafe.Pointer(eqPB - ` + fmt.Sprint(l.ref) + `)) <= 0
	}
	if eqZA || eqZB {
		eqFree(tls, ` + ctx + `, sp, eqZA, eqZB)
	}
	*(*uint64)(unsafe.Pointer(sp - ` + fmt.Sprint(2*l.val) + `)) = uint64(libc.BoolUint32(` + res + `))
	(*TJSValue)(unsafe.Pointer(sp - ` + fmt.Sprint(2*l.val) + `)).Ftag = ` + l.boolTag + `
	sp -= ` + fmt.Sprint(l.val) + `
} else {
`
	var out strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		if line != "" {
			out.WriteString(indent + line)
		}
	}
	out.WriteString(eqSlow(indent+"\t", ctx, strict, neq, l.val))
	out.WriteString(indent + "}\n")
	return out.String()
}

// unfixEq gives body, the text of JS_CallInternal, without the text of
// fixEq, which it must find exactly as fixEq writes it.
func unfixEq(body []byte, l eqLayout) ([]byte, error) {
	for {
		loc := reEqFixed.FindSubmatchIndex(body)
		if loc == nil {
			return body, nil
		}
		indent, op := string(body[loc[2]:loc[3]]), string(body[loc[4]:loc[5]])
		strict, neq := op == "===" || op == "!==", op[0] == '!'
		m := regexp.MustCompile(`eqFree\(tls, (ctx\d*), sp, eqZA, eqZB\)`).FindSubmatch(body[loc[0]:])
		if m == nil {
			return nil, fmt.Errorf("_JS_CallInternal: the text of fixEq for %s has no context", op)
		}
		want := eqText(indent, string(m[1]), strict, neq, l)
		if !bytes.HasPrefix(body[loc[0]:], []byte(want)) {
			return nil, fmt.Errorf("_JS_CallInternal: the text of fixEq for %s is not what fixEq writes", op)
		}
		body = append(append(append([]byte{}, body[:loc[0]]...), eqSlow(indent, string(m[1]), strict, neq, l.val)...), body[loc[0]+len(want):]...)
	}
}

// eqFingerprint hashes the functions the text of fixEq stands for.
func eqFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, eqFuncs, func(text []byte) []byte { return text })
}
