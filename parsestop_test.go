// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The eval timeout stops the parse of a script or a JSON text of millions of
// spaces, newlines, comment characters, identifier characters or characters
// of a string, a template or a regexp within a few polls, wherever in the
// call it comes: the tokenizer reads the interrupt flag every 65,536 bytes of
// them, and so do the look at the next token, the copies of the buffer of an
// identifier, its atom, the line and column of the code after a long token
// and the message of the error it throws. Before, eval of 16 Mi spaces ran
// for tens of milliseconds without reading it, and the atom and the message
// of a long identifier ran for thousands of polls, past the grace of the
// kill.
func TestParseStops(t *testing.T) {
	// flat joins its parts in one flat string: a rope would be made flat by
	// the eval, in steps that read the flag
	const n = `function flat(...a) { return a.join('') }; var n = 1 << 24`
	for _, c := range []struct{ name, setup, call string }{
		{"spaces", n + `, s = flat(' '.repeat(n), '1')`, `(0, eval)(s)`},
		{"tabs", n + `, s = flat('\t'.repeat(n), '1')`, `(0, eval)(s)`},
		{"newlines", n + `, s = flat('\n'.repeat(n), '1')`, `(0, eval)(s)`},
		{"CRLF", n + `, s = flat('\r\n'.repeat(n >> 1), '1')`, `(0, eval)(s)`},
		{"wide spaces", n + `, s = flat('　'.repeat(n >> 2), '1')`, `(0, eval)(s)`},
		{"line separators", n + `, s = flat(' '.repeat(n >> 2), '1')`, `(0, eval)(s)`},
		{"block comment", n + `, s = flat('/*', 'a'.repeat(n), '*/1')`, `(0, eval)(s)`},
		{"block comment of wide characters", n + `, s = flat('/*', 'é'.repeat(n >> 1), '*/1')`, `(0, eval)(s)`},
		{"line comment", n + `, s = flat('//', 'a'.repeat(n))`, `(0, eval)(s)`},
		{"HTML comment", n + `, s = flat('-->', 'a'.repeat(n))`, `(0, eval)(s)`},
		{"identifier", n + `, s = 'a'.repeat(n)`, `(0, eval)(s)`},
		{"identifier of wide characters", n + `, s = 'é'.repeat(n >> 1)`, `(0, eval)(s)`},
		{"private name", n + `, s = flat('class A { #', 'a'.repeat(n), ' }')`, `(0, eval)(s)`},
		{"property name", n + `, s = flat('({', 'a'.repeat(n), ': 1})')`, `(0, eval)(s)`},
		{"string", n + `, s = flat('"', 'a'.repeat(n), '"')`, `(0, eval)(s)`},
		{"string of wide characters", n + `, s = flat('"', '　'.repeat(n >> 2), '"')`, `(0, eval)(s)`},
		{"string of escapes", n + `, s = flat('"', '\\x41'.repeat(n >> 2), '"')`, `(0, eval)(s)`},
		{"directive", n + `, s = flat('"', 'a'.repeat(n), '"; 1')`, `(0, eval)(s)`},
		{"template", n + `, s = flat('` + "`" + `', 'a'.repeat(n), '` + "`" + `')`, `(0, eval)(s)`},
		{"tagged template", n + `, s = flat('String.raw` + "`" + `', 'a'.repeat(n), '` + "`" + `')`, `(0, eval)(s)`},
		{"regexp", n + `, s = flat('/', 'a'.repeat(n), '/')`, `(0, eval)(s)`},
		{"regexp flags", n + `, s = flat('/a/', 'g'.repeat(n))`, `(0, eval)(s)`},
		{"syntax error after a long line", n + `, s = flat('a'.repeat(n), ' +')`, `(0, eval)(s)`},
		{"spaces after async", n + `, s = flat('async', ' '.repeat(n), 'function f() {}')`, `(0, eval)(s)`},
		{"comment after async", n + `, s = flat('async /*', 'a'.repeat(n), '*/ function f() {}')`, `(0, eval)(s)`},
		{"line comment after let", n + `, s = flat('let //', 'a'.repeat(n), '\n[a] = [1]')`, `(0, eval)(s)`},
		{"spaces before an arrow", n + `, s = flat('x', ' '.repeat(n), '=> 1')`, `(0, eval)(s)`},
		{"JSON.parse of a word", n + `, s = flat('t', 'r'.repeat(n))`, `JSON.parse(s)`},
		{"long code point escape in a string", n + `, s = flat('"\\u{', '0'.repeat(n), '41}"')`, `(0, eval)(s)`},
		{"long code point escape in a template", n + `, s = flat('` + "`" + `\\u{', '0'.repeat(n), '41}` + "`" + `')`, `(0, eval)(s)`},
		{"long code point escape starting an identifier", n + `, s = flat('\\u{', '0'.repeat(n), '61}')`, `(0, eval)(s)`},
		{"long code point escape in an identifier", n + `, s = flat('a\\u{', '0'.repeat(n), '62}')`, `(0, eval)(s)`},
		{"long code point escape in a private name", n + `, s = flat('class A { #\\u{', '0'.repeat(n), '61} }')`, `(0, eval)(s)`},
		{"long code point escape after async", n + `, s = flat('async \\u{', '0'.repeat(n), '61}')`, `(0, eval)(s)`},
		{"hashbang", n + `, s = flat('#!', 'a'.repeat(n), '\n1')`, `(0, eval)(s)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			const caught = `try { %s } catch (e) { if (!(e instanceof SyntaxError || e instanceof ReferenceError)) throw e }`
			// one call, then a stop at each sixth of it: the scan, the
			// atom, the code and the error after it each read the flag
			start := time.Now()
			if _, err := m.Eval(fmt.Sprintf(caught, c.call), EvalGlobal); err != nil {
				t.Fatal(err)
			}
			call := time.Since(start)
			for k := 1; k <= 5; k++ {
				if err := m.SetEvalTimeout(max(call*time.Duration(k)/6, time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				_, err := m.Eval(`for (;;) `+fmt.Sprintf(caught, c.call), EvalGlobal)
				took := time.Since(start)
				if errors.Is(err, ErrKilled) {
					t.Fatalf("killed after %v of a call of %v, want the interrupt", took, call)
				}
				var e *Error
				if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
					t.Fatalf("got %v after %v, want interrupted", err, took)
				}
				if n := m.h.StopPolls(); n > parseStopPolls {
					t.Errorf("%d polls saw the request to stop, want at most %d (stopped %v after the start, in a call of %v)", n, parseStopPolls, took, call)
				}
				t.Logf("stopped after %v of a call of %v, %d polls saw the request", took, call, m.h.StopPolls())
				if err := m.SetEvalTimeout(0); err != nil {
					t.Fatal(err)
				}
				if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
					t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
				}
			}
		})
	}
}

// parseStopPolls is the most polls TestParseStops lets see the request, two
// steps of numberStopPolls. Where peek_token looks past a run for the token
// after it and reads the flag at the end of its step, it gives TOK_EOF, and
// next_token goes over the same run again from its start up to its own
// first step. The atom of a long name is hashed and compared with the atom
// of the same hash that the call before made, in steps of 65,536 characters
// that do more for each than a scan. Both stop within 13 polls on amd64.
const parseStopPolls = 2 * numberStopPolls

// A parse that reads the flag on its way gives what it gave: sources of
// 200,000 spaces, comment, identifier, string, template or regexp characters
// and more, which cross the places where it reads it.
func TestParseLongResults(t *testing.T) {
	m := newTestVM(t)
	const sp = `' '.repeat(200000)`
	for _, c := range []struct{ expr, want string }{
		{`(0, eval)(` + sp + ` + '1 +' + ` + sp + ` + '2')`, `3`},
		{`(0, eval)('\r\n'.repeat(100000) + '4')`, `4`},
		{`(0, eval)('var q = 1, r = 1; q' + '\r\n'.repeat(100000) + '++r; r')`, `2`},
		{`(0, eval)('　 '.repeat(100000) + '5')`, `5`},
		{`(0, eval)('/*' + 'aé\n'.repeat(100000) + '*/ 6')`, `6`},
		{`(0, eval)('// ' + 'aé'.repeat(100000) + '\n7')`, `7`},
		{`(0, eval)('-->' + 'a'.repeat(200000) + '\n8')`, `8`},
		{`(0, eval)('var ' + 'aé'.repeat(100000) + ' = 9; ' + 'aé'.repeat(100000))`, `9`},
		{`(0, eval)('class A { #' + 'b'.repeat(200000) + ' = 10; get() { return this.#' + 'b'.repeat(200000) + ' } }; new A().get()')`, `10`},
		{`typeof (0, eval)('async' + ` + sp + ` + 'function f() {}; f')`, `function`},
		{`(0, eval)('async /*' + 'a'.repeat(200000) + '*/ function g() {}; g').constructor.name`, `AsyncFunction`},
		{`(0, eval)('let //' + 'a'.repeat(200000) + '\n[a] = [11]; a')`, `11`},
		{`(0, eval)('x' + ` + sp + ` + '=> 12')(0)`, `12`},
		{`JSON.parse(` + sp + ` + 'true' + ` + sp + `)`, `true`},
		{`Function(` + sp + ` + 'return 13')()`, `13`},
		{`(0, eval)('"' + 'aé\\x41\\u{1F600}'.repeat(50000) + '"') === 'aéA\u{1F600}'.repeat(50000)`, `true`},
		{`(0, eval)('"' + 'a'.repeat(200000) + '"; 14')`, `14`},
		{`(0, eval)('"use strict"; (function () { return this })()')`, `undefined`},
		{`(0, eval)('` + "`" + `' + 'aé\r\n'.repeat(70000) + '` + "`" + `') === 'aé\n'.repeat(70000)`, `true`},
		{`(0, eval)('String.raw` + "`" + `' + 'a\\n\r'.repeat(70000) + '` + "`" + `') === 'a\\n\n'.repeat(70000)`, `true`},
		{`(0, eval)('/' + 'aé'.repeat(100000) + '/u').source.length`, `200000`},
		{`(0, eval)('/[' + 'aé'.repeat(100000) + ']/u').test('é')`, `true`},
		{`Object.keys((0, eval)('({' + 'aé'.repeat(100000) + ': 1})'))[0].length`, `200000`},
		{`(() => { try { (0, eval)('\n'.repeat(3) + 'é'.repeat(100000) + ' +') } catch (e) { return e.stack.match(/:(\d+):(\d+)/).slice(1).join() } })()`, `4,100003`},
		{`(() => { try { (0, eval)('1;\n' + 'a'.repeat(200000)) } catch (e) { return e.message } })() === "'" + 'a'.repeat(58) + "' is not defined"`, `true`},
		{`(() => { try { (0, eval)('b'.repeat(1000)) } catch (e) { return e.message } })() === "'" + 'b'.repeat(254)`, `true`},
		{`(0, eval)('"\\u{' + '0'.repeat(200000) + '41}"')`, `A`},
		{`(0, eval)('` + "`" + `\\u{' + '0'.repeat(200000) + '42}` + "`" + `')`, `B`},
		{`(0, eval)('var \\u{' + '0'.repeat(200000) + '61}x\\u{' + '0'.repeat(200000) + '62} = 15; axb')`, `15`},
		{`(0, eval)('class A { #\\u{' + '0'.repeat(200000) + '61} = 16; get() { return this.#a } }; new A().get()')`, `16`},
		{`(0, eval)('async \\u{' + '0'.repeat(200000) + '61} => 17')(0) instanceof Promise`, `true`},
		{`(0, eval)('#!' + 'aé'.repeat(100000) + '\n18')`, `18`},
	} {
		v, err := m.Eval(`String(`+c.expr+`)`, EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if v != c.want {
			t.Errorf("%s = %v, want %s", c.expr, v, c.want)
		}
	}
}

// The source a function keeps is decoded in steps when it is long, and gives
// what the whole decode gives: a run of bytes that are not UTF-8 is one
// U+FFFD, however it falls across the steps. Eval hands the bytes of the Go
// string to the parser as they are.
func TestParseLongRawBytes(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		pre, run int
		want     string
	}{
		{0, 65536, `1`},
		{65000, 2000, `1`},
		{65530, 200000, `1`},
		{100, 300000, `1`},
	} {
		src := "(function f() {/*" + strings.Repeat("a", c.pre) + strings.Repeat("\x80", c.run) + "*/}).toString().split('\\uFFFD').length - 1"
		v, err := m.Eval(`String(`+src+`)`, EvalGlobal)
		if err != nil || v != c.want {
			t.Errorf("U+FFFD in the source of a function with %d bytes 0x80 after %d: got %v, %v, want %s", c.run, c.pre, v, err, c.want)
		}
	}
}
