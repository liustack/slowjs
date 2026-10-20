// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// fixBitwise makes js_binary_logic_slow and js_shr_slow, the slow paths of
// &, |, ^, <<, >> and >>>, try bitwiseNumbers of lib/bitwise.go first. The
// interpreter does the operator itself only for two ints, so a float, which
// a product past 2^31 becomes (as in the string hash of emotion, the
// CSS-in-JS library of MUI), took the whole slow path: a TLS frame, two
// conversions that look for BigInts and objects, and the generic ToInt32.
// bitwiseNumbers does the operator for any two numbers and leaves every
// other pair to the rest of the slow path. The call goes at the entry of
// the two functions, before their TLS frame, and not into the interpreter:
// the interpreter is left as it is, which measured better than calling
// bitwiseNumbers from its six handlers (those made the shop 0.8% slower,
// although the shop barely has floats there).
//
// Only the files of 64-bit targets (cellFiles) change: bitwiseNumbers reads
// the struct JSValue of those, and the 32-bit targets keep the slow path.
// bitwiseNumbers repeats what the two slow functions, JS_ToNumberHintFree
// and JS_ToInt32Free do for numbers, so their text is pinned in
// bitwiseSites: when a release changes one of them, fixBitwise stops, and
// the new code must be read against bitwiseNumbers before the fingerprints
// are updated (go run ./internal/fixlib -bitwise prints them). The
// interpreter must still call js_binary_logic_slow 5 times and js_shr_slow
// once, with the operands on top of the stack.
func fixBitwise(b []byte, file string) ([]byte, error) {
	got, err := bitwiseFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := bitwiseSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: js_binary_logic_slow, js_shr_slow, JS_ToNumberHintFree or JS_ToInt32Free changed (fingerprint %s, want %s): read them against lib/bitwise.go, then update bitwiseSites (go run ./internal/fixlib -bitwise)", file, got, want)
	}
	body, err := funcBody(b, funcHead("_JS_CallInternal"), "_JS_CallInternal")
	if err != nil {
		return nil, err
	}
	logic, shr := len(reBitwiseLogic.FindAll(body, -1)), len(reBitwiseShr.FindAll(body, -1))
	if logic != 5 || shr != 1 {
		return nil, fmt.Errorf("_JS_CallInternal: want 5 calls of js_binary_logic_slow and 1 of js_shr_slow, found %d and %d", logic, shr)
	}
	for _, f := range []struct{ name, op string }{{"_js_binary_logic_slow", "op"}, {"_js_shr_slow", "int32(EOP_shr)"}} {
		body, err := funcBody(b, funcHead(f.name), f.name)
		if err != nil {
			return nil, err
		}
		start := bytes.Index(b, body)
		fixed := unfixBitwise(body)
		head := bytes.IndexByte(fixed[1:], '\n') + 2 // the body starts with the newline before func
		if !bytes.HasPrefix(fixed[head:], []byte("\tbp := tls.Alloc(")) {
			return nil, fmt.Errorf("%s: no TLS frame at its entry", f.name)
		}
		if cellFiles[file] {
			fixed = append(append(append([]byte{}, fixed[:head]...), bitwiseEntry(f.op)...), fixed[head:]...)
		}
		b = append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...)
	}
	return b, nil
}

const bitwiseComment = "// Changed by internal/fixlib: two numbers without the slow path (see fixBitwise).\n"

var (
	reBitwiseLogic = regexp.MustCompile(`(\t+)if _js_binary_logic_slow\(tls, (ctx\d*), sp, opcode\) != 0 \{\n`)
	reBitwiseShr   = regexp.MustCompile(`(\t+)if _js_shr_slow\(tls, (ctx\d*), sp\) != 0 \{\n`)
	reBitwiseEntry = regexp.MustCompile(`\t` + regexp.QuoteMeta(bitwiseComment) + `\tif bitwiseNumbers\(sp, (op|int32\(EOP_shr\))\) \{\n\t\treturn 0\n\t\}\n`)
)

// bitwiseEntry is the text fixBitwise puts at the entry of a slow function,
// for the operator op.
func bitwiseEntry(op string) string {
	return "\t" + bitwiseComment + "\tif bitwiseNumbers(sp, " + op + ") {\n\t\treturn 0\n\t}\n"
}

// unfixBitwise gives text without the calls of bitwiseNumbers.
func unfixBitwise(text []byte) []byte {
	return reBitwiseEntry.ReplaceAll(text, nil)
}

// bitwiseFingerprint hashes the functions bitwiseNumbers stands for, without
// the change of fixBitwise.
func bitwiseFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, []string{"_js_binary_logic_slow", "_js_shr_slow", "_JS_ToNumberHintFree", "_JS_ToInt32Free"}, unfixBitwise)
}
